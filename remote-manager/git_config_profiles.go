// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type gitConfigProfile struct {
	ID        int           `json:"id"`
	Name      string        `json:"name"`
	App       string        `json:"app"`
	Path      string        `json:"path"`
	Operator  string        `json:"operator"`
	Reference gitConfigFile `json:"reference"`
}
type gitConfigProfileResult struct {
	InstallID        int           `json:"install_id"`
	Server           string        `json:"server"`
	Connection       string        `json:"connection"`
	App              string        `json:"app"`
	Environment      string        `json:"environment"`
	Operator         string        `json:"operator"`
	InstallationPath string        `json:"installation_path"`
	CheckedAt        string        `json:"checked_at"`
	File             gitConfigFile `json:"file"`
}

func initGitConfigProfiles() {
	_, e := db.Exec("CREATE TABLE IF NOT EXISTS git_config_profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, data TEXT NOT NULL)")
	if e != nil {
		panic(e)
	}
}
func loadGitConfigProfiles(uid int) []gitConfigProfile {
	out := []gitConfigProfile{}
	rows, e := db.Query("SELECT id,data FROM git_config_profiles WHERE user_id=? ORDER BY id", uid)
	if e != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p gitConfigProfile
		var raw string
		var id int
		if rows.Scan(&id, &raw) == nil && json.Unmarshal([]byte(decryptValue(raw)), &p) == nil {
			p.ID = id
			out = append(out, p)
		}
	}
	return out
}
func validateConfigProfilePath(v string) error {
	if v == "" || len(v) > 500 || path.IsAbs(v) || path.Clean(v) != v || v == "." || v == ".." || strings.HasPrefix(v, "../") || strings.ContainsAny(v, "\\\x00\r\n") {
		return fmt.Errorf("choose a relative configuration path inside the installation")
	}
	return nil
}
func apiGitConfigProfiles(w http.ResponseWriter, r *http.Request, uid int, rest string) bool {
	if rest != "config-profiles" && !strings.HasPrefix(rest, "config-profiles/") {
		return false
	}
	if !gitChecksAllowed(uid) {
		jsonError(w, "Configuration inspection is not allowed", 403)
		return true
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		if r.Method == "GET" {
			jsonOK(w, loadGitConfigProfiles(uid))
			return true
		}
		if r.Method != "POST" {
			jsonError(w, "Method not allowed", 405)
			return true
		}
		var in struct {
			Name     string `json:"name"`
			App      string `json:"app"`
			Path     string `json:"path"`
			Operator string `json:"operator"`
			Content  string `json:"content"`
		}
		if !decodeGitJSON(w, r, &in) {
			return true
		}
		in.Name = strings.TrimSpace(in.Name)
		in.Path = strings.TrimSpace(in.Path)
		if in.Name == "" || len(in.Name) > 120 || len(in.App) > 120 || len(in.Operator) > 120 || len(in.Content) == 0 || len(in.Content) > 256<<10 {
			jsonError(w, "Name and a reference file up to 256 KiB are required", 400)
			return true
		}
		if e := validateConfigProfilePath(in.Path); e != nil {
			jsonError(w, e.Error(), 400)
			return true
		}
		if len(loadGitConfigProfiles(uid)) >= 50 {
			jsonError(w, "At most 50 configuration profiles", 400)
			return true
		}
		reference := parseGitConfig(in.Path, []byte(in.Content))
		if len(reference.Entries) == 0 {
			jsonError(w, "No readable settings found in this file", 400)
			return true
		}
		p := gitConfigProfile{Name: in.Name, App: strings.TrimSpace(in.App), Path: in.Path, Operator: strings.TrimSpace(in.Operator), Reference: reference}
		result, e := db.Exec("INSERT INTO git_config_profiles (user_id,data) VALUES (?,?)", uid, encryptValue(string(jsonMarshal(p))))
		if e != nil {
			jsonError(w, "Cannot save profile", 500)
			return true
		}
		id, _ := result.LastInsertId()
		p.ID = int(id)
		auditLog(r, uid, "git.config_profile_added", p.Name, map[string]interface{}{"profile_id": p.ID, "app": p.App, "path": p.Path})
		w.WriteHeader(201)
		jsonOK(w, p)
		return true
	}
	id, e := strconv.Atoi(parts[1])
	if e != nil {
		jsonError(w, "Not found", 404)
		return true
	}
	var p gitConfigProfile
	found := false
	for _, v := range loadGitConfigProfiles(uid) {
		if v.ID == id {
			p = v
			found = true
			break
		}
	}
	if !found {
		jsonError(w, "Not found", 404)
		return true
	}
	if len(parts) == 2 && r.Method == "DELETE" {
		db.Exec("DELETE FROM git_config_profiles WHERE id=? AND user_id=?", id, uid)
		auditLog(r, uid, "git.config_profile_deleted", p.Name, nil)
		jsonOK(w, map[string]bool{"ok": true})
		return true
	}
	if len(parts) != 3 || parts[2] != "check" || r.Method != "POST" {
		jsonError(w, "Method not allowed", 405)
		return true
	}
	var in struct {
		IDs []int `json:"ids"`
	}
	if !decodeGitJSON(w, r, &in) {
		return true
	}
	selected := map[int]bool{}
	for _, id := range in.IDs {
		selected[id] = true
	}
	installs := []gitInstall{}
	for _, v := range loadGitInstalls(uid, false, "") {
		if (p.App == "" || p.App == v.App) && (len(selected) == 0 || selected[v.ID]) {
			installs = append(installs, v)
			delete(selected, v.ID)
		}
	}
	if len(selected) > 0 {
		jsonError(w, "Selection contains an inaccessible installation or another service", 400)
		return true
	}
	if len(installs) > 50 {
		jsonError(w, "Select at most 50 installations per configuration check", 400)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	out := make([]gitConfigProfileResult, len(installs))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for index, in := range installs {
		wg.Add(1)
		go func(index int, in gitInstall) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				out[index] = gitConfigProfileResult{InstallID: in.ID, Server: in.Hostname, File: gitConfigFile{Path: p.Path, Error: "Check canceled or timed out", Entries: []gitConfigEntry{}}}
				return
			}
			row := gitConfigProfileResult{InstallID: in.ID, Server: in.Hostname, Connection: in.ConnName, App: in.App, Environment: in.Env, Operator: p.Operator, InstallationPath: in.Path, CheckedAt: nowStamp(), File: gitConfigFile{Path: p.Path, Entries: []gitConfigEntry{}}}
			if row.Server == "" {
				row.Server = in.ConnName
			}
			cl, e := dialInstall(uid, in)
			if e != nil {
				row.File.Error = redactText(e.Error())
				out[index] = row
				return
			}
			defer cl.Close()
			stop := context.AfterFunc(ctx, func() { cl.Close() })
			defer stop()
			f, e := readServerFile(cl, in.Path, p.Path, 256<<10)
			switch {
			case e != nil:
				row.File.Error = redactText(e.Error())
			case f.problem() != nil:
				row.File.Error = f.problem().Error()
			case !f.Exists:
				row.File.Error = "Configuration file is missing"
			case f.TooBig:
				row.File.Error = "Configuration exceeds 256 KiB"
			default:
				row.File = parseGitConfig(p.Path, f.Data)
			}
			out[index] = row
		}(index, in)
	}
	wg.Wait()
	auditLog(r, uid, "git.config_profile_checked", p.Name, map[string]interface{}{"profile_id": p.ID, "installations": len(installs)})
	jsonOK(w, map[string]interface{}{"profile": p, "rows": out})
	return true
}
