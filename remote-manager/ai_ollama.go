// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── OLLAMA: guided local AI ─────────────────────────
//
// Ollama runs open models on the machine WRM runs on and speaks an OpenAI-compatible API
// (/v1), so it is added as an ordinary "openai_compatible" provider without an API key.
//
//	GET  /api/ai/ollama/status[?url=]   detection: running, version, installed models, binary,
//	                                    per-OS commands (any user who may use the assistant;
//	                                    only admins may probe another URL)
//	POST /api/ai/ollama/action          {action: install|start|pull, model?, confirm: true}
//	                                    admin only AND policy ai_ollama_manage = 1; runs in the
//	                                    background, returns {run_id}
//	GET  /api/ai/ollama/runs[/{id}]     recorded runs with their output (admin only)
//
// Installing, starting and pulling are explicit, confirmed, server-side checked actions.
// Only fixed commands run: the single user input is a validated model name. Every request,
// denial and result is audited (admin.ai_ollama_*) and every run is stored in ai_ollama_runs.

const ollamaDefaultURL = "http://127.0.0.1:11434"

var ollamaModelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,120}$`)

// Replaced by tests.
var (
	ollamaOS  = runtime.GOOS
	ollamaRun = func(ctx context.Context, name string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		return buf.String(), err
	}
	ollamaStartDetached = func(bin string) error {
		cmd := exec.Command(bin, "serve")
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}
	ollamaLookPath  = exec.LookPath
	ollamaStat      = os.Stat
	ollamaStartWait = 20 * time.Second
)

var ollamaBusy sync.Mutex

func initOllamaSchema() {
	ensureTable("ai_ollama_runs", `CREATE TABLE IF NOT EXISTS ai_ollama_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		target TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'running',
		output TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL DEFAULT '',
		ended_at TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '')`,
		"id", "user_id", "username", "action", "target", "status", "output", "started_at", "ended_at", "client_ip")
	db.Exec(`UPDATE ai_ollama_runs SET status='failed', output=output || ' [server restarted]', ended_at=? WHERE status='running'`, time.Now().UTC().Format(time.RFC3339))
}

type ollamaModel struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type ollamaProbe struct {
	Running bool          `json:"running"`
	Version string        `json:"version"`
	Models  []ollamaModel `json:"models"`
	Error   string        `json:"error,omitempty"`
}

// ollamaNormalizeURL returns a clean http(s) base URL without credentials, path or query.
func ollamaNormalizeURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ollamaDefaultURL, nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", fmt.Errorf("the Ollama URL must be an http(s) URL without credentials")
	}
	return u.Scheme + "://" + u.Host, nil
}

func ollamaGetJSON(ctx context.Context, base, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// ollamaProbeServer detects a running Ollama and lists its installed models.
func ollamaProbeServer(ctx context.Context, base string) ollamaProbe {
	var ver struct {
		Version string `json:"version"`
	}
	if err := ollamaGetJSON(ctx, base, "/api/version", &ver); err != nil {
		return ollamaProbe{Models: []ollamaModel{}, Error: "not reachable at " + base}
	}
	pr := ollamaProbe{Running: true, Version: truncateStr(ver.Version, 40), Models: []ollamaModel{}}
	var tags struct {
		Models []ollamaModel `json:"models"`
	}
	if err := ollamaGetJSON(ctx, base, "/api/tags", &tags); err == nil {
		for _, m := range tags.Models {
			if len(pr.Models) < 200 {
				pr.Models = append(pr.Models, ollamaModel{Name: truncateStr(m.Name, 120), Size: m.Size})
			}
		}
		sort.Slice(pr.Models, func(i, j int) bool { return pr.Models[i].Name < pr.Models[j].Name })
	}
	return pr
}

func ollamaBinary() string {
	if p, err := ollamaLookPath("ollama"); err == nil {
		return p
	}
	var cands []string
	switch ollamaOS {
	case "windows":
		cands = []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Ollama", "ollama.exe")}
	case "darwin":
		cands = []string{"/Applications/Ollama.app/Contents/Resources/ollama", "/opt/homebrew/bin/ollama", "/usr/local/bin/ollama"}
	default:
		cands = []string{"/usr/local/bin/ollama", "/usr/bin/ollama"}
	}
	for _, c := range cands {
		if c != "" && filepath.IsAbs(c) {
			if st, err := ollamaStat(c); err == nil && !st.IsDir() {
				return c
			}
		}
	}
	return ""
}

type ollamaStep struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}

// ollamaGuide lists the commands per OS, shown as the manual alternative.
func ollamaGuide(goos string) (name string, steps []ollamaStep, auto bool) {
	switch goos {
	case "linux":
		return "Linux", []ollamaStep{
			{"install", "curl -fsSL https://ollama.com/install.sh | sh"},
			{"start", "sudo systemctl enable --now ollama"},
			{"pull", "ollama pull qwen2.5-coder:7b"}}, true
	case "darwin":
		return "macOS", []ollamaStep{
			{"install", "brew install ollama"},
			{"start", "brew services start ollama"},
			{"pull", "ollama pull qwen2.5-coder:7b"}}, true
	case "windows":
		return "Windows", []ollamaStep{
			{"install", "winget install --id Ollama.Ollama -e"},
			{"start", "ollama serve"},
			{"pull", "ollama pull qwen2.5-coder:7b"}}, true
	}
	return goos, []ollamaStep{{"install", "https://ollama.com/download"}, {"start", "ollama serve"}, {"pull", "ollama pull qwen2.5-coder:7b"}}, false
}

func ollamaCanManage(userID int) bool {
	return isAdminUser(userID) && settingBool("ai_ollama_manage")
}

func apiOllamaHandler(w http.ResponseWriter, r *http.Request, userID int, parts []string) {
	if len(parts) == 0 {
		jsonError(w, "Not found", 404)
		return
	}
	switch parts[0] {
	case "status":
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		if ok, why := aiUserAllowed(userID); !ok {
			jsonError(w, why, 403)
			return
		}
		raw := ""
		if isAdminUser(userID) {
			raw = r.URL.Query().Get("url")
		}
		base, err := ollamaNormalizeURL(raw)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		osName, steps, auto := ollamaGuide(ollamaOS)
		pr := ollamaProbeServer(r.Context(), base)
		jsonOK(w, map[string]interface{}{
			"url": base, "running": pr.Running, "version": pr.Version, "models": pr.Models, "error": pr.Error,
			"binary": ollamaBinary() != "", "os": osName, "os_id": ollamaOS, "steps": steps, "auto_supported": auto,
			"is_admin": isAdminUser(userID), "policy_enabled": settingBool("ai_ollama_manage"), "can_manage": ollamaCanManage(userID) && auto,
			"openai_base": base + "/v1",
		})
	case "action":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		ollamaActionHandler(w, r, userID)
	case "runs":
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		if !isAdminUser(userID) {
			jsonError(w, "Admin access required", 403)
			return
		}
		if len(parts) == 2 {
			id, _ := strconv.Atoi(parts[1])
			run, ok := ollamaLoadRun(id)
			if !ok {
				jsonError(w, "Not found", 404)
				return
			}
			jsonOK(w, run)
			return
		}
		rows, err := db.Query(`SELECT id, username, action, target, status, output, started_at, ended_at FROM ai_ollama_runs ORDER BY id DESC LIMIT 20`)
		out := []map[string]interface{}{}
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int
				var u, a, tg, st, o, s, e string
				rows.Scan(&id, &u, &a, &tg, &st, &o, &s, &e)
				out = append(out, map[string]interface{}{"id": id, "username": u, "action": a, "target": tg, "status": st, "output": o, "started_at": s, "ended_at": e})
			}
		}
		jsonOK(w, out)
	default:
		jsonError(w, "Not found", 404)
	}
}

func ollamaLoadRun(id int) (map[string]interface{}, bool) {
	var u, a, tg, st, o, s, e string
	if err := db.QueryRow(`SELECT username, action, target, status, output, started_at, ended_at FROM ai_ollama_runs WHERE id=?`, id).Scan(&u, &a, &tg, &st, &o, &s, &e); err != nil {
		return nil, false
	}
	return map[string]interface{}{"id": id, "username": u, "action": a, "target": tg, "status": st, "output": o, "started_at": s, "ended_at": e}, true
}

func ollamaDeny(w http.ResponseWriter, r *http.Request, userID int, action, why string, code int) {
	auditLog(r, userID, "admin.ai_ollama_denied", action, map[string]interface{}{"action": action, "reason": why})
	jsonError(w, why, code)
}

func ollamaActionHandler(w http.ResponseWriter, r *http.Request, userID int) {
	var in struct {
		Action  string `json:"action"`
		Model   string `json:"model"`
		URL     string `json:"url"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	if !isAdminUser(userID) {
		ollamaDeny(w, r, userID, in.Action, "Admin access required", 403)
		return
	}
	if !settingBool("ai_ollama_manage") {
		ollamaDeny(w, r, userID, in.Action, "Managing Ollama from WRM is switched off by the policy (ai_ollama_manage)", 403)
		return
	}
	if !oneOf(in.Action, "install", "start", "pull") {
		jsonError(w, "Unknown action", 400)
		return
	}
	if !in.Confirm {
		jsonError(w, "Explicit confirmation is required", 400)
		return
	}
	if _, _, auto := ollamaGuide(ollamaOS); !auto {
		ollamaDeny(w, r, userID, in.Action, "Automatic setup is not supported on this operating system; follow the manual steps", 400)
		return
	}
	base, err := ollamaNormalizeURL(in.URL)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	model := strings.TrimSpace(in.Model)
	if in.Action == "pull" && !ollamaModelRe.MatchString(model) {
		jsonError(w, "Invalid model name", 400)
		return
	}
	if in.Action != "pull" {
		model = ""
	}
	if !ollamaBusy.TryLock() {
		jsonError(w, "Another Ollama action is already running", 409)
		return
	}
	target := model
	if target == "" {
		target = ollamaOS
	}
	res, err := db.Exec(`INSERT INTO ai_ollama_runs (user_id, username, action, target, status, started_at, client_ip) VALUES (?,?,?,?, 'running', ?, ?)`,
		userID, usernameOf(userID), in.Action, target, time.Now().UTC().Format(time.RFC3339), clientIP(r))
	if err != nil {
		ollamaBusy.Unlock()
		jsonError(w, "Could not record the action", 500)
		return
	}
	runID, _ := res.LastInsertId()
	auditLog(r, userID, "admin.ai_ollama_"+in.Action, target, map[string]interface{}{"run_id": runID, "os": ollamaOS, "model": model, "confirmed": true})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		out, err := ollamaExecute(ctx, in.Action, base, model)
		status := "done"
		if err != nil {
			status = "failed"
			out = strings.TrimSpace(out + "\n" + err.Error())
		}
		auditLog(r, userID, "admin.ai_ollama_finished", target, map[string]interface{}{"run_id": runID, "action": in.Action, "status": status})
		// Publish completion only once the command and audit are finished and the
		// next action can acquire the lock. A visible done/failed status must not
		// leave callers hitting a transient "already running" response.
		ollamaBusy.Unlock()
		db.Exec(`UPDATE ai_ollama_runs SET status=?, output=?, ended_at=? WHERE id=?`, status, truncateStr(out, 8000), time.Now().UTC().Format(time.RFC3339), runID)
	}()
	jsonOK(w, map[string]interface{}{"run_id": runID, "status": "running"})
}

// ollamaExecute runs one fixed action; the only variable part is the validated model name.
func ollamaExecute(ctx context.Context, action, base, model string) (string, error) {
	switch action {
	case "install":
		if ollamaBinary() != "" {
			return "Ollama is already installed.", nil
		}
		switch ollamaOS {
		case "linux":
			if _, err := ollamaLookPath("curl"); err != nil {
				return "", fmt.Errorf("curl is required for the official install script")
			}
			return ollamaRun(ctx, "sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
		case "darwin":
			if _, err := ollamaLookPath("brew"); err != nil {
				return "", fmt.Errorf("Homebrew is required; install Ollama from https://ollama.com/download instead")
			}
			return ollamaRun(ctx, "brew", "install", "ollama")
		case "windows":
			if _, err := ollamaLookPath("winget"); err != nil {
				return "", fmt.Errorf("winget is required; install Ollama from https://ollama.com/download instead")
			}
			return ollamaRun(ctx, "winget", "install", "--id", "Ollama.Ollama", "-e", "--silent", "--accept-package-agreements", "--accept-source-agreements")
		}
		return "", fmt.Errorf("unsupported operating system")
	case "start":
		if pr := ollamaProbeServer(ctx, base); pr.Running {
			return "Ollama is already running (version " + pr.Version + ").", nil
		}
		bin := ollamaBinary()
		if bin == "" {
			return "", fmt.Errorf("Ollama is not installed")
		}
		var log string
		started := false
		switch ollamaOS {
		case "linux":
			if _, err := ollamaLookPath("systemctl"); err == nil {
				o, err := ollamaRun(ctx, "systemctl", "enable", "--now", "ollama")
				log += o
				started = err == nil
				if err != nil {
					log += "\nsystemd service not available, starting `ollama serve` instead\n"
				}
			}
		case "darwin":
			if _, err := ollamaLookPath("brew"); err == nil {
				o, err := ollamaRun(ctx, "brew", "services", "start", "ollama")
				log += o
				started = err == nil
			}
		}
		if !started {
			if err := ollamaStartDetached(bin); err != nil {
				return log, err
			}
			log += "started `ollama serve` in the background\n"
		}
		deadline := time.Now().Add(ollamaStartWait)
		for time.Now().Before(deadline) {
			if pr := ollamaProbeServer(ctx, base); pr.Running {
				return log + "Ollama is running (version " + pr.Version + ").", nil
			}
			time.Sleep(500 * time.Millisecond)
		}
		return log, fmt.Errorf("Ollama did not answer at %s after starting", base)
	case "pull":
		if pr := ollamaProbeServer(ctx, base); !pr.Running {
			return "", fmt.Errorf("Ollama is not running at %s; start it first", base)
		}
		body, _ := json.Marshal(map[string]interface{}{"model": model, "stream": false})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/pull", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		var res struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		json.Unmarshal(b, &res)
		if resp.StatusCode != 200 || res.Error != "" {
			msg := res.Error
			if msg == "" {
				msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			return "", fmt.Errorf("pull failed: %s", truncateStr(msg, 300))
		}
		return "pulled " + model + ": " + nonEmpty(res.Status, "success"), nil
	}
	return "", fmt.Errorf("unknown action")
}

// ─── model list of any OpenAI/Anthropic-style provider ───

// aiListModels asks the provider for its model list (GET {base}/models).
func aiListModels(ctx context.Context, p aiProvider) ([]string, error) {
	var u string
	hdr := http.Header{}
	switch p.Kind {
	case "anthropic":
		base := nonEmpty(p.BaseURL, "https://api.anthropic.com")
		u = base + "/v1/models?limit=200"
		hdr.Set("anthropic-version", "2023-06-01")
		if p.secret.APIKey != "" {
			hdr.Set("x-api-key", p.secret.APIKey)
		}
	case "openai", "openai_compatible":
		u = nonEmpty(p.BaseURL, "https://api.openai.com/v1") + "/models"
		if p.secret.APIKey != "" {
			hdr.Set("Authorization", "Bearer "+p.secret.APIKey)
		}
	case "opencode":
		list, err := opencodeListModels(ctx, p)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(list))
		for _, m := range list {
			out = append(out, m.ID)
		}
		return out, nil
	case "opencode_go":
		list, err := opencodeGoListModels(ctx, p)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(list))
		for _, m := range list {
			out = append(out, m.ID)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("this provider type has no model list; type the model or deployment name")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header = hdr
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the provider: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		return nil, &aiAPIError{Status: resp.StatusCode, Msg: aiErrorMessage(b)}
	}
	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("the provider returned an unexpected model list")
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
		if id == "" || len(id) > 200 || strings.ContainsAny(id, " \t\"'<>,") || seen[id] || len(out) >= 500 {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, m := range res.Data {
		add(m.ID)
	}
	for _, m := range res.Models {
		add(nonEmpty(m.ID, m.Name))
	}
	sort.Strings(out)
	return out, nil
}
