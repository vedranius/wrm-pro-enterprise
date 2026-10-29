// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ─── AI: PROVIDERS (configuration) ───────────────────
//
// A provider is a model endpoint with credentials: the organisation's (scope org,
// managed by administrators) or a user's own (scope user, when ai_personal_keys allows
// it). Kinds:
//   anthropic          Anthropic Messages API (api.anthropic.com or a gateway base URL)
//   openai             OpenAI Chat Completions with tools
//   azure              Azure OpenAI (deployment + api-version, or the v1 API)
//   bedrock            AWS Bedrock: Converse API, or the Messages API endpoint; SigV4 or an API key
//   vertex             Google Vertex AI: Claude (rawPredict) or the OpenAI-compatible endpoint; service account
//   openai_compatible  any OpenAI-compatible base URL (local models)
//   opencode           OpenCode Zen: free models without a key, every model with one (ai_opencode.go)
//   opencode_go        OpenCode Go: a subscription key on Go's own endpoints (ai_opencode_go.go)
//
// Secrets (API keys, AWS secret keys, service account JSON) are stored in one encrypted
// JSON value (encryptValue) and never returned to the browser: the API reports only which
// secrets are set.

type aiProviderSecret struct {
	APIKey         string `json:"api_key,omitempty"`
	AccessKeyID    string `json:"access_key_id,omitempty"`
	SecretKey      string `json:"secret_access_key,omitempty"`
	SessionToken   string `json:"session_token,omitempty"`
	ServiceAccount string `json:"service_account,omitempty"` // Google service account JSON
}

type aiProvider struct {
	ID           int     `json:"id"`
	Scope        string  `json:"scope"` // org | user
	UserID       int     `json:"-"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	BaseURL      string  `json:"base_url"`    // endpoint override (gateway, private endpoint, local server)
	Region       string  `json:"region"`      // bedrock / vertex
	Project      string  `json:"project"`     // vertex
	Deployment   string  `json:"deployment"`  // azure
	APIVersion   string  `json:"api_version"` // azure
	APIMode      string  `json:"api_mode"`    // bedrock: converse | messages; vertex: anthropic | openai
	Models       string  `json:"models"`      // comma-separated list offered to users
	DefaultModel string  `json:"default_model"`
	PriceIn      float64 `json:"price_in"`  // USD per million input tokens (0 = built-in table / unknown)
	PriceOut     float64 `json:"price_out"` // USD per million output tokens
	MaxTokens    int     `json:"max_tokens"`
	Enabled      bool    `json:"enabled"`
	Fallback     bool    `json:"refusal_fallback"` // Anthropic: server-side refusal fallbacks
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	secret       aiProviderSecret
	// what the browser may know about the secrets
	HasAPIKey         bool `json:"has_api_key"`
	HasAccessKey      bool `json:"has_access_key"`
	HasServiceAccount bool `json:"has_service_account"`
}

const aiProviderCols = `id, scope, COALESCE(user_id,0), name, kind, base_url, region, project, deployment, api_version, api_mode, models,
	default_model, price_in, price_out, max_tokens, enabled, refusal_fallback, secret, created_at, updated_at`

func initAISchema() {
	db.Exec(`ALTER TABLE users ADD COLUMN ai_blocked INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE users ADD COLUMN ai_delegate_training INTEGER NOT NULL DEFAULT 0`)
	initOllamaSchema()
	ensureTable("ai_providers", `CREATE TABLE IF NOT EXISTS ai_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scope TEXT NOT NULL DEFAULT 'org',
		user_id INTEGER DEFAULT NULL,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT '',
		base_url TEXT NOT NULL DEFAULT '',
		region TEXT NOT NULL DEFAULT '',
		project TEXT NOT NULL DEFAULT '',
		deployment TEXT NOT NULL DEFAULT '',
		api_version TEXT NOT NULL DEFAULT '',
		api_mode TEXT NOT NULL DEFAULT '',
		models TEXT NOT NULL DEFAULT '',
		default_model TEXT NOT NULL DEFAULT '',
		price_in REAL NOT NULL DEFAULT 0,
		price_out REAL NOT NULL DEFAULT 0,
		max_tokens INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		refusal_fallback INTEGER NOT NULL DEFAULT 1,
		secret TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`,
		"id", "scope", "user_id", "name", "kind", "base_url", "region", "project", "deployment", "api_version", "api_mode", "models",
		"default_model", "price_in", "price_out", "max_tokens", "enabled", "refusal_fallback", "secret", "created_at", "updated_at")
	ensureTable("ai_sessions", `CREATE TABLE IF NOT EXISTS ai_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uid TEXT NOT NULL DEFAULT '',
		user_id INTEGER DEFAULT NULL,
		username TEXT NOT NULL DEFAULT '',
		conn_id INTEGER DEFAULT NULL,
		conn_name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		share_id INTEGER DEFAULT NULL,
		transport TEXT NOT NULL DEFAULT 'panel',
		provider_id INTEGER DEFAULT NULL,
		provider_name TEXT NOT NULL DEFAULT '',
		provider_kind TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT 'read_only',
		auto_limits TEXT NOT NULL DEFAULT '',
		share_notes INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		end_reason TEXT NOT NULL DEFAULT '',
		client_ip TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL DEFAULT '',
		ended_at TEXT NOT NULL DEFAULT '',
		last_activity TEXT NOT NULL DEFAULT '',
		requests INTEGER NOT NULL DEFAULT 0,
		tool_calls INTEGER NOT NULL DEFAULT 0,
		approvals INTEGER NOT NULL DEFAULT 0,
		denials INTEGER NOT NULL DEFAULT 0,
		tokens_in INTEGER NOT NULL DEFAULT 0,
		tokens_out INTEGER NOT NULL DEFAULT 0,
		cost_usd REAL NOT NULL DEFAULT 0,
		term_session_id INTEGER DEFAULT NULL)`,
		"id", "uid", "user_id", "username", "conn_id", "conn_name", "host", "share_id", "transport", "provider_id", "provider_name",
		"provider_kind", "model", "mode", "auto_limits", "share_notes", "status", "end_reason", "client_ip", "started_at", "ended_at",
		"last_activity", "requests", "tool_calls", "approvals", "denials", "tokens_in", "tokens_out", "cost_usd", "term_session_id")
	ensureTable("ai_messages", `CREATE TABLE IF NOT EXISTS ai_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		seq INTEGER NOT NULL DEFAULT 0,
		ts TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT '',
		actor TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		meta TEXT NOT NULL DEFAULT '')`,
		"id", "session_id", "seq", "ts", "kind", "actor", "content", "meta")
	for _, q := range []string{
		`CREATE INDEX IF NOT EXISTS ix_ai_providers_user ON ai_providers(scope, user_id)`,
		`CREATE INDEX IF NOT EXISTS ix_ai_sessions_user ON ai_sessions(user_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS ix_ai_sessions_started ON ai_sessions(started_at)`,
		`CREATE INDEX IF NOT EXISTS ix_ai_messages_session ON ai_messages(session_id, seq)`,
	} {
		db.Exec(q)
	}
	// sessions left active by a crash or restart
	db.Exec(`UPDATE ai_sessions SET status='ended', end_reason='server restarted', ended_at=? WHERE status='active'`, time.Now().UTC().Format(time.RFC3339))
}

func scanAIProvider(sc interface{ Scan(...interface{}) error }) (aiProvider, error) {
	var p aiProvider
	var sec string
	err := sc.Scan(&p.ID, &p.Scope, &p.UserID, &p.Name, &p.Kind, &p.BaseURL, &p.Region, &p.Project, &p.Deployment, &p.APIVersion,
		&p.APIMode, &p.Models, &p.DefaultModel, &p.PriceIn, &p.PriceOut, &p.MaxTokens, &p.Enabled, &p.Fallback, &sec, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if sec != "" {
		json.Unmarshal([]byte(decryptValue(sec)), &p.secret)
	}
	p.HasAPIKey = p.secret.APIKey != ""
	p.HasAccessKey = p.secret.AccessKeyID != "" && p.secret.SecretKey != ""
	p.HasServiceAccount = p.secret.ServiceAccount != ""
	return p, nil
}

func loadAIProvider(id int) (aiProvider, error) {
	return scanAIProvider(db.QueryRow(`SELECT `+aiProviderCols+` FROM ai_providers WHERE id=?`, id))
}

func listAIProviders(where string, args ...interface{}) []aiProvider {
	out := []aiProvider{}
	rows, err := db.Query(`SELECT `+aiProviderCols+` FROM ai_providers WHERE `+where+` ORDER BY scope, name, id`, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if p, err := scanAIProvider(rows); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func (p aiProvider) modelList() []string {
	var out []string
	for _, m := range splitPatterns(p.Models) {
		if !oneOf(m, out...) {
			out = append(out, m)
		}
	}
	if p.DefaultModel != "" && !oneOf(p.DefaultModel, out...) {
		out = append([]string{p.DefaultModel}, out...)
	}
	return out
}

// usableBy reports whether a user may use the provider now (policy, scope, kind).
func (p aiProvider) usableBy(userID int) (bool, string) {
	if !p.Enabled {
		return false, "the provider is disabled"
	}
	if p.Scope == "user" {
		if p.UserID != userID {
			return false, "not your provider"
		}
		if !aiPersonalKeysAllowed(userID) {
			return false, "personal API keys are not allowed by the policy"
		}
	}
	if !aiKindAllowed(p.Kind) {
		return false, "the policy does not allow " + p.Kind + " providers"
	}
	return true, ""
}

// providersFor lists the providers a user may choose, with the models the policy allows.
func providersFor(userID int) []map[string]interface{} {
	out := []map[string]interface{}{}
	list := listAIProviders(`scope='org'`)
	if aiPersonalKeysAllowed(userID) {
		list = append(list, listAIProviders(`scope='user' AND user_id=?`, userID)...)
	}
	for _, p := range list {
		if ok, _ := p.usableBy(userID); !ok {
			continue
		}
		models := []string{}
		for _, m := range p.modelList() {
			if aiModelAllowed(m) {
				models = append(models, m)
			}
		}
		if len(models) == 0 {
			continue
		}
		def := p.DefaultModel
		if !oneOf(def, models...) {
			def = models[0]
		}
		out = append(out, map[string]interface{}{"id": p.ID, "name": p.Name, "kind": p.Kind, "scope": p.Scope, "models": models, "default_model": def})
	}
	return out
}

// ─── validation and storage ──────────────────────────

type aiProviderInput struct {
	Name         *string  `json:"name"`
	Kind         *string  `json:"kind"`
	BaseURL      *string  `json:"base_url"`
	Region       *string  `json:"region"`
	Project      *string  `json:"project"`
	Deployment   *string  `json:"deployment"`
	APIVersion   *string  `json:"api_version"`
	APIMode      *string  `json:"api_mode"`
	Models       *string  `json:"models"`
	DefaultModel *string  `json:"default_model"`
	PriceIn      *float64 `json:"price_in"`
	PriceOut     *float64 `json:"price_out"`
	MaxTokens    *int     `json:"max_tokens"`
	Enabled      *bool    `json:"enabled"`
	Fallback     *bool    `json:"refusal_fallback"`
	// secrets: nil = unchanged, "" = remove
	APIKey         *string `json:"api_key"`
	AccessKeyID    *string `json:"access_key_id"`
	SecretKey      *string `json:"secret_access_key"`
	SessionToken   *string `json:"session_token"`
	ServiceAccount *string `json:"service_account"`
}

var aiRegionRe = func(s string) bool {
	if s == "" || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func (in aiProviderInput) apply(p *aiProvider) error {
	set := func(dst *string, v *string, max int) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
			if len(*dst) > max {
				*dst = (*dst)[:max]
			}
		}
	}
	set(&p.Name, in.Name, 100)
	set(&p.Kind, in.Kind, 40)
	set(&p.BaseURL, in.BaseURL, 500)
	set(&p.Region, in.Region, 40)
	set(&p.Project, in.Project, 100)
	set(&p.Deployment, in.Deployment, 200)
	set(&p.APIVersion, in.APIVersion, 40)
	set(&p.APIMode, in.APIMode, 20)
	set(&p.Models, in.Models, 2000)
	set(&p.DefaultModel, in.DefaultModel, 200)
	if in.PriceIn != nil {
		p.PriceIn = *in.PriceIn
	}
	if in.PriceOut != nil {
		p.PriceOut = *in.PriceOut
	}
	if in.MaxTokens != nil {
		p.MaxTokens = *in.MaxTokens
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	if in.Fallback != nil {
		p.Fallback = *in.Fallback
	}
	sec := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	sec(&p.secret.APIKey, in.APIKey)
	sec(&p.secret.AccessKeyID, in.AccessKeyID)
	sec(&p.secret.SecretKey, in.SecretKey)
	sec(&p.secret.SessionToken, in.SessionToken)
	sec(&p.secret.ServiceAccount, in.ServiceAccount)

	if p.Name == "" {
		return fmt.Errorf("a name is required")
	}
	if !oneOf(p.Kind, strings.Split(aiAllProviderKinds, ",")...) {
		return fmt.Errorf("unknown provider kind %q", p.Kind)
	}
	if p.BaseURL != "" {
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return fmt.Errorf("the base URL must be an http(s) URL without credentials")
		}
		if err := guardEgressURL(p.BaseURL); err != nil {
			return err
		}
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
	}
	if p.PriceIn < 0 || p.PriceOut < 0 || p.PriceIn > 10000 || p.PriceOut > 10000 {
		return fmt.Errorf("prices must be between 0 and 10000 USD per million tokens")
	}
	if p.MaxTokens < 0 || p.MaxTokens > 256000 {
		return fmt.Errorf("max tokens must be between 0 and 256000")
	}
	if len(p.modelList()) == 0 {
		return fmt.Errorf("at least one model is required")
	}
	for _, m := range p.modelList() {
		if len(m) > 200 || strings.ContainsAny(m, " \t\"'<>") {
			return fmt.Errorf("invalid model name %q", m)
		}
	}
	switch p.Kind {
	case "anthropic", "openai":
		if p.secret.APIKey == "" && p.BaseURL == "" {
			return fmt.Errorf("an API key is required")
		}
	case "azure":
		if p.BaseURL == "" {
			return fmt.Errorf("the Azure OpenAI endpoint (base URL) is required")
		}
		if p.secret.APIKey == "" {
			return fmt.Errorf("an API key is required")
		}
	case "bedrock":
		if !aiRegionRe(p.Region) {
			return fmt.Errorf("an AWS region is required")
		}
		if p.APIMode == "" {
			p.APIMode = "converse"
		}
		if !oneOf(p.APIMode, "converse", "messages") {
			return fmt.Errorf("the Bedrock API must be converse or messages")
		}
		if p.APIMode == "messages" && p.secret.APIKey == "" {
			return fmt.Errorf("the Bedrock Messages endpoint needs a Bedrock API key")
		}
		if p.secret.APIKey == "" && (p.secret.AccessKeyID == "" || p.secret.SecretKey == "") {
			return fmt.Errorf("an AWS access key and secret key, or a Bedrock API key, are required")
		}
	case "vertex":
		if !aiRegionRe(p.Region) || p.Project == "" {
			return fmt.Errorf("the Google Cloud project and region are required")
		}
		if p.APIMode == "" {
			p.APIMode = "anthropic"
		}
		if !oneOf(p.APIMode, "anthropic", "openai") {
			return fmt.Errorf("the Vertex API must be anthropic or openai")
		}
		if p.secret.ServiceAccount == "" {
			return fmt.Errorf("a service account key (JSON) is required")
		}
		if _, err := parseServiceAccount(p.secret.ServiceAccount); err != nil {
			return err
		}
	case "openai_compatible":
		if p.BaseURL == "" {
			return fmt.Errorf("the base URL is required")
		}
	case "opencode":
		// Zen works without a key for its free models (the key "public").
	case "opencode_go":
		if p.secret.APIKey == "" {
			return fmt.Errorf("an OpenCode Go API key is required (OpenCode Console → Go)")
		}
	}
	return nil
}

func saveAIProvider(p *aiProvider) error {
	now := time.Now().UTC().Format(time.RFC3339)
	sec := ""
	if b, _ := json.Marshal(p.secret); string(b) != "{}" {
		sec = encryptValue(string(b))
	}
	var uid interface{}
	if p.Scope == "user" {
		uid = p.UserID
	}
	if p.ID == 0 {
		p.CreatedAt = now
		res, err := db.Exec(`INSERT INTO ai_providers (scope, user_id, name, kind, base_url, region, project, deployment, api_version, api_mode,
			models, default_model, price_in, price_out, max_tokens, enabled, refusal_fallback, secret, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.Scope, uid, p.Name, p.Kind, p.BaseURL, p.Region, p.Project, p.Deployment, p.APIVersion, p.APIMode, p.Models, p.DefaultModel,
			p.PriceIn, p.PriceOut, p.MaxTokens, boolInt(p.Enabled), boolInt(p.Fallback), sec, now, now)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		p.ID = int(id)
	} else {
		if _, err := db.Exec(`UPDATE ai_providers SET name=?, kind=?, base_url=?, region=?, project=?, deployment=?, api_version=?, api_mode=?,
			models=?, default_model=?, price_in=?, price_out=?, max_tokens=?, enabled=?, refusal_fallback=?, secret=?, updated_at=? WHERE id=?`,
			p.Name, p.Kind, p.BaseURL, p.Region, p.Project, p.Deployment, p.APIVersion, p.APIMode, p.Models, p.DefaultModel,
			p.PriceIn, p.PriceOut, p.MaxTokens, boolInt(p.Enabled), boolInt(p.Fallback), sec, now, p.ID); err != nil {
			return err
		}
	}
	p.UpdatedAt = now
	p.HasAPIKey = p.secret.APIKey != ""
	p.HasAccessKey = p.secret.AccessKeyID != "" && p.secret.SecretKey != ""
	p.HasServiceAccount = p.secret.ServiceAccount != ""
	return nil
}

// handleAIProviders serves the provider list of one scope: org (admins) or user (own).
//
//	GET    …                 list
//	POST   …                 create
//	PUT    …/{id}            update (secrets: omitted = unchanged, "" = remove)
//	DELETE …/{id}
//	POST   …/{id}/test       one small request to check the endpoint and key
func handleAIProviders(w http.ResponseWriter, r *http.Request, userID int, scope, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	where, args := `scope='org'`, []interface{}{}
	if scope == "user" {
		where, args = `scope='user' AND user_id=?`, []interface{}{userID}
		if !aiPersonalKeysAllowed(userID) && r.Method != http.MethodGet && r.Method != http.MethodDelete {
			jsonError(w, "Personal API keys are not allowed by the policy", 403)
			return
		}
	}
	if parts[0] == "" {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, listAIProviders(where, args...))
		case http.MethodPost:
			var in aiProviderInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				jsonError(w, "Bad JSON", 400)
				return
			}
			p := aiProvider{Scope: scope, UserID: userID, Enabled: true, Fallback: true}
			if err := in.apply(&p); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			if scope == "user" && !aiKindAllowed(p.Kind) {
				jsonError(w, "The policy does not allow "+p.Kind+" providers", 403)
				return
			}
			if err := saveAIProvider(&p); err != nil {
				jsonError(w, "Could not save the provider", 500)
				return
			}
			auditLog(r, userID, "ai.provider_created", p.Name, map[string]interface{}{"id": p.ID, "kind": p.Kind, "scope": scope, "models": p.Models})
			jsonOK(w, p)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	p, err := scanAIProvider(db.QueryRow(`SELECT `+aiProviderCols+` FROM ai_providers WHERE id=? AND `+where, append([]interface{}{id}, args...)...))
	if err == sql.ErrNoRows || err != nil {
		jsonError(w, "Provider not found", 404)
		return
	}
	if len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost {
		var in struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		model := in.Model
		if model == "" {
			model = p.DefaultModel
			if model == "" {
				model = p.modelList()[0]
			}
		}
		res, err := aiTestProvider(r.Context(), p, model)
		d := map[string]interface{}{"id": p.ID, "kind": p.Kind, "model": model, "ok": err == nil}
		if err != nil {
			d["error"] = truncateStr(err.Error(), 300)
		}
		auditLog(r, userID, "ai.provider_tested", p.Name, d)
		if err != nil {
			jsonOK(w, map[string]interface{}{"ok": false, "error": err.Error()})
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true, "reply": truncateStr(res, 200)})
		return
	}
	if len(parts) == 2 && parts[1] == "models" && r.Method == http.MethodGet {
		list, err := aiListModels(r.Context(), p)
		d := map[string]interface{}{"id": p.ID, "kind": p.Kind, "ok": err == nil, "count": len(list)}
		if err != nil {
			d["error"] = truncateStr(err.Error(), 300)
		}
		auditLog(r, userID, "ai.provider_models_listed", p.Name, d)
		if err != nil {
			jsonOK(w, map[string]interface{}{"ok": false, "error": err.Error(), "models": []string{}})
			return
		}
		out := map[string]interface{}{"ok": true, "models": list}
		if p.Kind == "opencode" || p.Kind == "opencode_go" {
			info := func(id string) opencodeModel {
				if p.Kind == "opencode_go" {
					return opencodeGoModelInfo(r.Context(), id)
				}
				return opencodeModelInfo(r.Context(), id)
			}
			free := []string{}
			for _, id := range list {
				if info(id).Free {
					free = append(free, id)
				}
			}
			out["free"] = free
			out["keyless"] = p.Kind == "opencode" && p.secret.APIKey == ""
			du := map[string]string{}
			for _, id := range list {
				du[id] = info(id).DataUse
			}
			out["data_use"] = du
		}
		jsonOK(w, out)
		return
	}
	if len(parts) != 1 {
		jsonError(w, "Not found", 404)
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, p)
	case http.MethodPut:
		var in aiProviderInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if err := in.apply(&p); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if err := saveAIProvider(&p); err != nil {
			jsonError(w, "Could not save the provider", 500)
			return
		}
		changedSecrets := []string{}
		for name, v := range map[string]*string{"api_key": in.APIKey, "access_key": in.AccessKeyID, "aws_secret": in.SecretKey, "session_token": in.SessionToken, "service_account": in.ServiceAccount} {
			if v != nil {
				changedSecrets = append(changedSecrets, name)
			}
		}
		auditLog(r, userID, "ai.provider_updated", p.Name, map[string]interface{}{"id": p.ID, "kind": p.Kind, "scope": scope, "enabled": p.Enabled, "changed_secrets": strings.Join(changedSecrets, ",")})
		jsonOK(w, p)
	case http.MethodDelete:
		db.Exec(`DELETE FROM ai_providers WHERE id=?`, p.ID)
		auditLog(r, userID, "ai.provider_deleted", p.Name, map[string]interface{}{"id": p.ID, "kind": p.Kind, "scope": scope})
		aiKillWhere(func(s *aiSession) bool { return s.provider.ID == p.ID }, userID, "the provider was deleted")
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}
