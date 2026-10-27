// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ─── APPLICATION SETTINGS (admin policies) ───────────
//
// Settings live in the app_settings table and are edited in Admin panel → Policies.
// Every setting can also be forced with an environment variable WRM_<KEY IN UPPER CASE>
// (e.g. WRM_REGISTRATION=open). A setting forced by the environment is shown as locked
// in the admin panel and cannot be changed there — useful for configuration management.
// Some settings also accept a shorter alias (e.g. AUDIT_ENABLED).

type settingSpec struct {
	Key     string   `json:"key"`
	Default string   `json:"default"`
	Kind    string   `json:"kind"` // bool | int | enum | string | json
	Enum    []string `json:"enum,omitempty"`
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
	Restart bool     `json:"-"`
	Alias   string   `json:"-"` // additional environment variable name
}

var settingSpecs = []settingSpec{
	// Accounts & sign-in
	{Key: "security_level", Default: "home", Kind: "enum", Enum: securityLevels},
	{Key: "registration", Default: "closed", Kind: "enum", Enum: []string{"closed", "open"}},
	{Key: "require_https", Default: "0", Kind: "bool"},
	{Key: "require_2fa", Default: "off", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "require_step_up", Default: "0", Kind: "bool"},
	{Key: "step_up_minutes", Default: "10", Kind: "int", Min: 1, Max: 240},
	// Screen lock (lock.go, v13.7)
	{Key: "screen_lock_mode", Default: "server", Kind: "enum", Enum: []string{"browser", "remember", "server"}},
	{Key: "screen_lock_max_minutes", Default: "0", Kind: "int", Min: 0, Max: 1440},
	{Key: "screen_lock_pin", Default: "1", Kind: "bool"},
	{Key: "screen_lock_max_failures", Default: "0", Kind: "int", Min: 0, Max: 100},
	// Appearance (static/appearance.js, v13.7): the theme new users start with and whether
	// users may add their own CSS to their own interface.
	{Key: "appearance_default_theme", Default: "graphite", Kind: "string"},
	{Key: "appearance_custom_css", Default: "1", Kind: "bool"},
	{Key: "break_glass_ips", Default: "", Kind: "string"},
	{Key: "local_login", Default: "all", Kind: "enum", Enum: []string{"all", "break_glass"}},
	{Key: "oidc_revalidate_minutes", Default: "60", Kind: "int", Min: 5, Max: 1440},
	{Key: "oidc_unverified_hours", Default: "12", Kind: "int", Min: 1, Max: 720},
	{Key: "ldap_sync_minutes", Default: "15", Kind: "int", Min: 5, Max: 1440},
	// Security alerts (alerts.go)
	{Key: "alert_failed_signins", Default: "10", Kind: "int", Min: 0, Max: 1000},
	{Key: "alert_failed_signins_minutes", Default: "10", Kind: "int", Min: 1, Max: 1440},
	{Key: "alert_break_glass", Default: "1", Kind: "bool"},
	{Key: "alert_new_admin", Default: "1", Kind: "bool"},
	{Key: "alert_secret_reveal", Default: "1", Kind: "bool"},
	{Key: "alert_ai_full_access", Default: "1", Kind: "bool"},
	{Key: "alert_audit_off", Default: "1", Kind: "bool"},
	{Key: "password_min_length", Default: "8", Kind: "int", Min: 6, Max: 128},
	{Key: "session_idle_hours", Default: "168", Kind: "int", Min: 1, Max: 8760},
	{Key: "session_max_days", Default: "30", Kind: "int", Min: 1, Max: 365},
	{Key: "login_max_failures", Default: "10", Kind: "int", Min: 3, Max: 100},
	// Connections
	{Key: "allow_server_keys", Default: "0", Kind: "bool"},
	{Key: "host_key_policy", Default: "tofu", Kind: "enum", Enum: []string{"tofu", "strict", "off"}},
	{Key: "max_upload_mb", Default: "0", Kind: "int", Min: 0, Max: 1048576},
	{Key: "allow_secret_export", Default: "1", Kind: "bool"},
	// Sharing & collaboration
	{Key: "allow_link_shares", Default: "1", Kind: "bool"},
	{Key: "link_share_max_days", Default: "0", Kind: "int", Min: 0, Max: 3650},
	{Key: "chat_file_max_mb", Default: "5", Kind: "int", Min: 0, Max: 25},
	{Key: "voice_enabled", Default: "1", Kind: "bool"},
	{Key: "voice_max_participants", Default: "12", Kind: "int", Min: 2, Max: 50},
	{Key: "ice_servers", Default: `[{"urls":["stun:stun.l.google.com:19302","stun:stun.cloudflare.com:3478"]}]`, Kind: "json"},
	{Key: "turn_enabled", Default: "1", Kind: "bool", Restart: true},
	{Key: "turn_port", Default: "3478", Kind: "int", Min: 1, Max: 65535, Restart: true},
	{Key: "turn_public_ip", Default: "", Kind: "string", Restart: true},
	{Key: "turn_host", Default: "", Kind: "string"},
	{Key: "turn_relay_ports", Default: "49152-65535", Kind: "string", Restart: true},
	{Key: "turn_allow_private", Default: "0", Kind: "bool", Restart: true},
	// SSH tunnels (port forwarding)
	{Key: "tunnels_enabled", Default: "1", Kind: "bool", Alias: "TUNNELS_ENABLED"},
	{Key: "tunnel_users", Default: "all", Kind: "enum", Enum: []string{"all", "admins"}},
	{Key: "tunnel_bind_any", Default: "0", Kind: "bool"},
	{Key: "tunnel_remote_forward", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "tunnel_idle_minutes", Default: "0", Kind: "int", Min: 0, Max: 10080},
	// Terminal productivity
	{Key: "broadcast_enabled", Default: "1", Kind: "bool", Alias: "BROADCAST_ENABLED"},
	// Remote desktop (RDP / VNC / Telnet through guacd)
	{Key: "desktop_enabled", Default: "1", Kind: "bool", Alias: "REMOTE_DESKTOP_ENABLED"},
	{Key: "guacd_address", Default: "127.0.0.1:4822", Kind: "string", Alias: "GUACD_ADDRESS"},
	{Key: "guacd_auto", Default: "1", Kind: "bool"},
	{Key: "desktop_tunnel_bind", Default: "127.0.0.1", Kind: "string"},
	// Out-of-band management (BMC) and serial ports
	{Key: "bmc_enabled", Default: "1", Kind: "bool", Alias: "BMC_ENABLED"},
	{Key: "serial_ports", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	// Network tools (port check, ping, traceroute, DNS, HTTP/TLS)
	{Key: "network_tools", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	// Saved proxies (SOCKS / HTTP CONNECT): who may define them (everybody may use shared ones)
	{Key: "proxies", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	// Live up/down status of connections
	{Key: "status_enabled", Default: "1", Kind: "bool", Alias: "STATUS_ENABLED"},
	{Key: "status_interval_seconds", Default: "60", Kind: "int", Min: 15, Max: 3600},
	{Key: "status_jump_checks", Default: "0", Kind: "bool"},
	// Workboard: disk, CPU and RAM read over SSH for servers without a fresh Monitoring sample
	{Key: "workboard_ssh_metrics", Default: "1", Kind: "bool"},
	// Addons: who may install personal addons (visible only to themselves)
	{Key: "addon_personal", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	// Who may use addons that reach web sites through WRM (http capability).
	{Key: "addon_http", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	// Notifications (e-mail, chat and push channels)
	{Key: "notifications_enabled", Default: "1", Kind: "bool", Alias: "NOTIFICATIONS_ENABLED"},
	{Key: "notify_min_interval_seconds", Default: "60", Kind: "int", Min: 0, Max: 86400},
	// Git workspace (compare services on servers with GitLab / GitHub or offline bundles)
	{Key: "git_enabled", Default: "1", Kind: "bool", Alias: "GIT_ENABLED"},
	{Key: "git_checks", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_check_interval_minutes", Default: "60", Kind: "int", Min: 0, Max: 10080},
	{Key: "git_backup_words", Default: defaultBackupWords, Kind: "string"},
	{Key: "git_check_parallel", Default: "8", Kind: "int", Min: 1, Max: 64},
	{Key: "git_check_per_server", Default: "2", Kind: "int", Min: 1, Max: 8},
	{Key: "git_update", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_upgrade", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_rollback", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_restart", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_install", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_medium", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "medium_log_output", Default: "all", Kind: "enum", Enum: []string{"none", "admins", "all"}},
	// Medium native engine: who deploys, rolls back and releases locks; time limits of one step
	{Key: "medium_native", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "medium_command_timeout", Default: "300", Kind: "int", Min: 30, Max: 86400},
	{Key: "medium_default_engine", Default: "native", Kind: "enum", Enum: []string{"native", "original"}},
	{Key: "medium_build_user", Default: "", Kind: "string"},
	{Key: "medium_transfer_timeout", Default: "3600", Kind: "int", Min: 600, Max: 86400},
	{Key: "medium_docker_install", Default: "1", Kind: "bool"},
	{Key: "medium_host_ca", Default: "1", Kind: "bool"},
	{Key: "medium_ca_file", Default: "", Kind: "string"},
	{Key: "git_transfer", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_gitignore_mr", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "git_schedule_grace_minutes", Default: "30", Kind: "int", Min: 1, Max: 1440},
	// Updates: check the GitHub releases of update_repo daily; one-click self-update
	{Key: "update_check", Default: "1", Kind: "bool"},
	{Key: "update_repo", Default: defaultUpdateRepo, Kind: "string"},
	{Key: "update_proxy", Default: "", Kind: "string"},
	{Key: "update_signing_keys", Default: "", Kind: "string"},
	{Key: "self_update", Default: "admins", Kind: "enum", Enum: []string{"off", "admins"}},
	{Key: "bitwarden", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "bitwarden_idle_minutes", Default: "15", Kind: "int", Min: 1, Max: 240},
	// AI assistant: who may use it, permission modes, providers, data handling, kill switch
	{Key: "ai_assistant", Default: "off", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "ai_kill_switch", Default: "0", Kind: "bool"},
	{Key: "ai_modes", Default: "read_only,ask", Kind: "string"},
	{Key: "ai_default_mode", Default: "read_only", Kind: "enum", Enum: []string{"read_only", "ask"}},
	{Key: "ai_mode_rules", Default: "[]", Kind: "json"},
	{Key: "ai_personal_keys", Default: "off", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "ai_provider_kinds", Default: aiAllProviderKinds, Kind: "string"},
	{Key: "ai_models", Default: "", Kind: "string"},
	{Key: "ai_ollama_manage", Default: "0", Kind: "bool"},
	{Key: "ai_delegate", Default: "0", Kind: "bool"},
	{Key: "ai_delegate_provider", Default: "0", Kind: "int", Min: 0, Max: 2147483647},
	{Key: "ai_delegate_models", Default: "", Kind: "string"},
	{Key: "ai_delegate_max_kb", Default: "64", Kind: "int", Min: 1, Max: 1024},
	{Key: "ai_delegate_training", Default: "user", Kind: "enum", Enum: []string{"off", "user", "all"}},
	{Key: "ai_opencode", Default: "all", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "ai_opencode_version", Default: opencodeDefaultVersion, Kind: "string"},
	{Key: "ai_opencode_registry", Default: "https://registry.npmjs.org", Kind: "string"},
	{Key: "ai_opencode_path", Default: "", Kind: "string"},
	{Key: "ai_opencode_max_sessions", Default: "4", Kind: "int", Min: 1, Max: 50},
	{Key: "ai_opencode_sandbox", Default: "off", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "ai_opencode_sandbox_image", Default: opencodeSandboxDefaultImage, Kind: "string"},
	{Key: "ai_redact_output", Default: "1", Kind: "bool"},
	{Key: "ai_show_thinking", Default: "1", Kind: "bool"},
	{Key: "ai_auto_titles", Default: "1", Kind: "bool"},
	{Key: "ai_keep_saved_sessions", Default: "1", Kind: "bool"},
	{Key: "ai_share_notes", Default: "1", Kind: "bool"},
	{Key: "ai_transcript_retention_days", Default: "90", Kind: "int", Min: 1, Max: 3650},
	{Key: "ai_approval_timeout_seconds", Default: "300", Kind: "int", Min: 30, Max: 3600},
	{Key: "ai_auto_max_minutes", Default: "60", Kind: "int", Min: 1, Max: 1440},
	{Key: "ai_auto_max_actions", Default: "50", Kind: "int", Min: 1, Max: 1000},
	{Key: "ai_auto_until_grant", Default: "1", Kind: "bool"},
	{Key: "ai_sensitive_reads", Default: "ask", Kind: "enum", Enum: []string{"ask", "deny"}},
	{Key: "ai_full_access", Default: "0", Kind: "bool"},
	{Key: "ai_full_access_mode", Default: "all", Kind: "enum", Enum: []string{"all", "connection", "user"}},
	{Key: "ai_full_access_blocked", Default: "ask", Kind: "enum", Enum: []string{"ask", "deny"}},
	{Key: "ai_allow_power", Default: "off", Kind: "enum", Enum: []string{"off", "ask"}},
	{Key: "ai_blocked_commands", Default: "", Kind: "string"},
	{Key: "ai_read_deny_paths", Default: "", Kind: "string"},
	{Key: "ai_provider_header_seconds", Default: "900", Kind: "int", Min: 30, Max: 7200},
	{Key: "ai_provider_request_seconds", Default: "3600", Kind: "int", Min: 60, Max: 14400},
	{Key: "ai_provider_stream_idle_seconds", Default: "600", Kind: "int", Min: 30, Max: 7200},
	{Key: "ai_command_timeout_seconds", Default: "60", Kind: "int", Min: 5, Max: 3600},
	{Key: "ai_output_max_kb", Default: "64", Kind: "int", Min: 4, Max: 1024},
	{Key: "ai_max_steps", Default: "25", Kind: "int", Min: 1, Max: 200},
	{Key: "ai_idle_minutes", Default: "60", Kind: "int", Min: 5, Max: 1440},
	// AI desktop apps (MCP)
	{Key: "ai_mcp_enabled", Default: "0", Kind: "bool"},
	{Key: "ai_mcp", Default: "admins", Kind: "enum", Enum: []string{"off", "admins", "all"}},
	{Key: "ai_mcp_scopes", Default: mcpAllScopes, Kind: "string"},
	{Key: "ai_mcp_modes", Default: "read_only,ask,auto", Kind: "string"},
	{Key: "ai_mcp_token_hours", Default: "168", Kind: "int", Min: 1, Max: 8760},
	{Key: "ai_mcp_max_token_hours", Default: "720", Kind: "int", Min: 1, Max: 8760},
	{Key: "ai_mcp_oauth", Default: "1", Kind: "bool"},
	{Key: "ai_mcp_redirect_uris", Default: mcpDefaultRedirectURIs, Kind: "string"},
	{Key: "ai_mcp_allowed_origins", Default: "", Kind: "string"},
	{Key: "ai_mcp_public_url", Default: "", Kind: "string"},
	{Key: "ai_mcp_elicitation", Default: "1", Kind: "bool"},
	{Key: "ai_mcp_later_approval", Default: "run", Kind: "enum", Enum: []string{"run", "repeat"}},
	{Key: "ai_mcp_transfer_max_mb", Default: "100", Kind: "int", Min: 1, Max: 10240},
	// Audit & session recording
	{Key: "audit_enabled", Default: "1", Kind: "bool", Alias: "AUDIT_ENABLED"},
	{Key: "audit_retention_days", Default: "365", Kind: "int", Min: 7, Max: 3650},
	// File edit history (file_edits.go, v13.14.0): diffs of files saved from the editor or written by the AI
	{Key: "file_edit_history", Default: "1", Kind: "bool"},
	{Key: "file_edit_max_kb", Default: "1024", Kind: "int", Min: 16, Max: 10240},
	{Key: "session_recording", Default: "1", Kind: "bool", Alias: "SESSION_RECORDING_ENABLED"},
	{Key: "session_recording_input", Default: "0", Kind: "bool"},
	{Key: "recording_retention_days", Default: "90", Kind: "int", Min: 7, Max: 3650},
	{Key: "recording_max_mb", Default: "100", Kind: "int", Min: 0, Max: 102400},
}

var settingsStore = struct {
	sync.RWMutex
	values map[string]string
}{values: map[string]string{}}

func settingSpecFor(key string) (settingSpec, bool) {
	for _, s := range settingSpecs {
		if s.Key == key {
			return s, true
		}
	}
	return settingSpec{}, false
}

func settingEnvName(key string) string { return "WRM_" + strings.ToUpper(key) }

// settingEnv returns the environment override of a setting and the variable it came from.
func settingEnv(key string) (string, string) {
	if v := os.Getenv(settingEnvName(key)); v != "" {
		return v, settingEnvName(key)
	}
	if s, ok := settingSpecFor(key); ok && s.Alias != "" {
		if v := os.Getenv(s.Alias); v != "" {
			return v, s.Alias
		}
	}
	return "", ""
}

// loadAppSettings reads every stored setting into memory (called once at startup).
func loadAppSettings() {
	rows, err := db.Query(`SELECT key, value FROM app_settings`)
	if err != nil {
		log.Printf("settings: %v", err)
		return
	}
	defer rows.Close()
	settingsStore.Lock()
	defer settingsStore.Unlock()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			settingsStore.values[k] = v
		}
	}
	// v13.4.0: an installation that saved the old "all provider kinds" value gets
	// OpenCode Go without editing the policy. A custom list stays untouched.
	if v, ok := settingsStore.values["ai_provider_kinds"]; ok {
		if n, err := normalizeAIKinds(v); err == nil && n == aiProviderKindsOldDefault {
			settingsStore.values["ai_provider_kinds"] = aiAllProviderKinds
		}
	}
	for _, s := range settingSpecs {
		if env, name := settingEnv(s.Key); env != "" {
			if _, err := validateSetting(s, env); err != nil {
				log.Printf("WARNING: ignoring %s=%q: %v", name, env, err)
			}
		}
	}
}

// getSetting returns the effective value: environment override, stored value or default.
func getSetting(key string) string {
	s, known := settingSpecFor(key)
	if known {
		if env, _ := settingEnv(key); env != "" {
			if v, err := validateSetting(s, env); err == nil {
				return v
			}
		}
	}
	settingsStore.RLock()
	v, ok := settingsStore.values[key]
	settingsStore.RUnlock()
	if ok {
		return v
	}
	return s.Default
}

func settingBool(key string) bool { return getSetting(key) == "1" }

func settingInt(key string) int {
	n, err := strconv.Atoi(getSetting(key))
	if err != nil {
		s, _ := settingSpecFor(key)
		n, _ = strconv.Atoi(s.Default)
	}
	return n
}

func settingLocked(key string) bool {
	env, _ := settingEnv(key)
	return env != ""
}

func validateSetting(s settingSpec, v string) (string, error) {
	v = strings.TrimSpace(v)
	switch s.Kind {
	case "bool":
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return "1", nil
		case "0", "false", "no", "off", "":
			return "0", nil
		}
		return "", fmt.Errorf("expected true/false")
	case "int":
		n, err := strconv.Atoi(v)
		if err != nil {
			return "", fmt.Errorf("expected a number")
		}
		if n < s.Min || (s.Max > 0 && n > s.Max) {
			return "", fmt.Errorf("must be between %d and %d", s.Min, s.Max)
		}
		return strconv.Itoa(n), nil
	case "enum":
		for _, e := range s.Enum {
			if strings.EqualFold(v, e) {
				return e, nil
			}
		}
		if whoPolicies[s.Key] {
			if w, err := validateWhoValue(v); err == nil {
				return w, nil
			} else if strings.HasPrefix(strings.ToLower(v), "teams:") {
				return "", err
			}
			return "", fmt.Errorf("must be one of %s, operators or teams:<name>,<name>", strings.Join(s.Enum, ", "))
		}
		return "", fmt.Errorf("must be one of %s", strings.Join(s.Enum, ", "))
	case "json":
		if v == "" {
			return "[]", nil
		}
		var tmp []map[string]interface{}
		if err := json.Unmarshal([]byte(v), &tmp); err != nil {
			return "", fmt.Errorf("invalid JSON: %v", err)
		}
		if s.Key == "ai_mode_rules" {
			return validateAIModeRules(v)
		}
		return v, nil
	}
	if len(v) > 4096 {
		return "", fmt.Errorf("value too long")
	}
	switch s.Key {
	case "break_glass_ips":
		return validateIPList(v)
	case "medium_build_user":
		if v != "" && !regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`).MatchString(v) {
			return "", fmt.Errorf("use an OS account name such as wrm-build")
		}
		if v == "root" {
			return "", fmt.Errorf("the build user must not be root")
		}
	case "turn_public_ip":
		if v != "" && net.ParseIP(v) == nil {
			return "", fmt.Errorf("not an IP address")
		}
	case "turn_relay_ports":
		if _, _, err := parsePortRange(v); err != nil {
			return "", err
		}
	case "ai_modes", "ai_mcp_modes":
		return normalizeAIModes(v)
	case "ai_mcp_scopes":
		return normalizeMCPScopes(v)
	case "ai_mcp_redirect_uris":
		return validateRedirectPatterns(v)
	case "ai_mcp_allowed_origins":
		var out []string
		for _, o := range splitPatterns(v) {
			u, err := url.Parse(o)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.User != nil {
				return "", fmt.Errorf("use origins such as https://app.example.com: %s", o)
			}
			out = append(out, u.Scheme+"://"+u.Host)
		}
		return strings.Join(out, ","), nil
	case "ai_mcp_public_url":
		if v != "" {
			u, err := url.Parse(v)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" {
				return "", fmt.Errorf("use the public address of WRM, such as https://wrm.example.com")
			}
			return strings.TrimRight(v, "/"), nil
		}
	case "ai_provider_kinds":
		return normalizeAIKinds(v)
	case "ai_opencode_version":
		if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$`).MatchString(v) {
			return "", fmt.Errorf("use a version such as %s", opencodeDefaultVersion)
		}
	case "ai_opencode_registry":
		u, err := url.Parse(v)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" {
			return "", fmt.Errorf("use an https registry URL such as https://registry.npmjs.org")
		}
		return strings.TrimRight(v, "/"), nil
	case "ai_opencode_sandbox_image":
		if v == "" {
			v = opencodeSandboxDefaultImage
		}
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9._/:@-]{0,254}$`).MatchString(v) || strings.Contains(v, "..") {
			return "", fmt.Errorf("use a Docker image reference such as %s or registry.example.com/team/image:tag", opencodeSandboxDefaultImage)
		}
		return v, nil
	case "ai_opencode_path":
		if v != "" && !filepath.IsAbs(v) {
			return "", fmt.Errorf("use an absolute path to the opencode binary")
		}
	case "ai_delegate_models":
		var out []string
		for _, m := range splitPatterns(v) {
			if len(m) > 200 || strings.ContainsAny(m, " \t\"'<>") {
				return "", fmt.Errorf("invalid model name %q", m)
			}
			out = append(out, m)
		}
		return strings.Join(out, ","), nil
	case "appearance_default_theme":
		// A built-in theme id, or addon:<addon>:<theme>; the page falls back to graphite for
		// a theme it does not know (for example an addon that was removed).
		if v == "" {
			v = "graphite"
		}
		if !appearanceThemeRe.MatchString(v) {
			return "", fmt.Errorf("use a theme id such as graphite, daylight or addon:my-addon:my-theme")
		}
	case "update_repo":
		if !updateRepoRe.MatchString(v) {
			return "", fmt.Errorf("use the form owner/repository")
		}
	case "update_signing_keys":
		keys, err := parseReleaseKeyList(v)
		if err != nil {
			return "", err
		}
		var out []string
		for _, k := range keys {
			out = append(out, formatReleasePublicKey(k))
		}
		return strings.Join(out, ","), nil
	case "update_proxy":
		if v != "" {
			u, err := url.Parse(v)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
				return "", fmt.Errorf("use a proxy URL such as http://proxy.example.com:3128")
			}
		}
	}
	return v, nil
}

var appearanceThemeRe = regexp.MustCompile(`^(addon:[a-z0-9][a-z0-9._-]{0,63}:)?[a-z0-9][a-z0-9-]{0,31}$`)

func parsePortRange(v string) (uint16, uint16, error) {
	parts := strings.SplitN(strings.TrimSpace(v), "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("use the form 49152-65535")
	}
	lo, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	hi, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || lo < 1024 || hi > 65535 || lo > hi {
		return 0, 0, fmt.Errorf("invalid port range (1024-65535, low-high)")
	}
	return uint16(lo), uint16(hi), nil
}

func setSetting(key, value string) (string, error) {
	s, ok := settingSpecFor(key)
	if !ok {
		return "", fmt.Errorf("unknown setting %q", key)
	}
	if _, name := settingEnv(key); name != "" {
		return "", fmt.Errorf("%s is set by the environment variable %s", key, name)
	}
	v, err := validateSetting(s, value)
	if err != nil {
		return "", fmt.Errorf("%s: %v", key, err)
	}
	if _, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, v); err != nil {
		return "", err
	}
	settingsStore.Lock()
	settingsStore.values[key] = v
	settingsStore.Unlock()
	return v, nil
}

// Internal (non-admin-editable) values such as the server secret share the table.
func getInternal(key string) string {
	var v string
	db.QueryRow(`SELECT value FROM app_settings WHERE key=?`, "_"+key).Scan(&v)
	return v
}

func setInternal(key, value string) error {
	_, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "_"+key, value)
	return err
}

// ─── ADMIN API ───────────────────────────────────────

// GET  /api/admin/settings → every setting with value, default and lock state
// PUT  /api/admin/settings → {"key": "value", ...}
func apiAdminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		out := []map[string]interface{}{}
		devs := securityDeviations()
		for _, s := range settingSpecs {
			envName := settingEnvName(s.Key)
			if _, name := settingEnv(s.Key); name != "" {
				envName = name
			}
			item := map[string]interface{}{
				"key": s.Key, "value": getSetting(s.Key), "default": s.Default, "kind": s.Kind,
				"enum": s.Enum, "min": s.Min, "max": s.Max, "locked": settingLocked(s.Key),
				"env": envName, "restart": s.Restart,
			}
			if whoPolicies[s.Key] {
				item["who"] = true // also accepts operators and teams:<names>
			}
			if need := levelNeed(securityLevel(), s.Key); need != "" {
				item["required"] = need
			}
			if d, ok := devs[s.Key]; ok {
				item["deviation"] = d
			}
			out = append(out, item)
		}
		jsonOK(w, out)
	case http.MethodPut:
		if !requireStepUp(w, r, "settings") {
			return
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		// A value looser than the security level needs a note: {"_deviation_notes": {"key": "why"}}.
		notes := map[string]string{}
		if raw, ok := payload["_deviation_notes"].(map[string]interface{}); ok {
			for k, v := range raw {
				if s, ok := v.(string); ok {
					notes[k] = strings.TrimSpace(s)
				}
			}
		}
		delete(payload, "_deviation_notes")
		if _, ok := payload["security_level"]; ok {
			jsonError(w, "Change the security level in Admin → Security, which first shows what changes", 400)
			return
		}
		values := map[string]string{}
		for k, raw := range payload {
			var v string
			switch x := raw.(type) {
			case string:
				v = x
			case bool:
				v = map[bool]string{true: "1", false: "0"}[x]
			case float64:
				v = strconv.FormatFloat(x, 'f', -1, 64)
			default:
				b, _ := json.Marshal(x)
				v = string(b)
			}
			if getSetting(k) == strings.TrimSpace(v) {
				continue
			}
			spec, ok := settingSpecFor(k)
			if !ok {
				jsonError(w, fmt.Sprintf("unknown setting %q", k), 400)
				return
			}
			nv, err := validateSetting(spec, v)
			if err != nil {
				jsonError(w, fmt.Sprintf("%s: %v", k, err), 400)
				return
			}
			if need := levelNeed(securityLevel(), k); need != "" && !levelMeets(k, nv, need) && len(notes[k]) < 3 && deviationNote(k) == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":     fmt.Sprintf("%s: the security level %s requires %s; a looser value is a deviation and needs a note", k, securityLevel(), need),
					"deviation": map[string]string{"key": k, "value": nv, "required": need, "level": securityLevel()},
				})
				return
			}
			values[k] = v
		}
		changed := map[string]string{}
		for k, v := range values {
			nv, err := setSetting(k, v)
			if err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
			changed[k] = nv
		}
		if len(changed) > 0 {
			auditLog(r, adminID, "admin.settings", "", changed)
		}
		for k := range changed {
			syncDeviation(r, adminID, k, notes[k])
		}
		settingsChanged(adminID, changed)
		jsonOK(w, map[string]interface{}{"ok": true, "changed": changed})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// settingsChanged applies what changed settings mean for running sessions, tunnels and AI.
func settingsChanged(adminID int, changed map[string]string) {
	turnChanged := false
	for k := range changed {
		if strings.HasPrefix(k, "turn_") {
			turnChanged = true
		}
	}
	if turnChanged {
		restartTURN()
	}
	if v, ok := changed["ai_kill_switch"]; ok && v == "1" {
		aiKillAll(adminID, "kill switch turned on by an administrator")
	}
	if v, ok := changed["ai_assistant"]; ok && v == "off" {
		aiKillWhere(func(s *aiSession) bool { return s.Transport != "mcp" }, adminID, "the AI assistant was turned off")
	} else if ok && v != "all" {
		aiKillWhere(func(s *aiSession) bool { return s.Transport != "mcp" && !policyAllows(s.UserID, v) }, adminID, "the policy no longer allows the AI assistant for the user")
	}
	if v, ok := changed["ai_mcp_enabled"]; ok && v == "0" {
		mcpShutdown(adminID, "AI connections (MCP) were turned off", nil)
	}
	if v, ok := changed["ai_opencode"]; ok && v != "all" {
		aiKillWhere(func(s *aiSession) bool {
			return s.Transport == "opencode" && !policyAllows(s.UserID, v)
		}, adminID, "the policy no longer allows OpenCode agents")
	}
	if v, ok := changed["ai_opencode_sandbox"]; ok && v != "all" {
		aiKillWhere(func(s *aiSession) bool {
			s.mu.Lock()
			sb := s.oc != nil && s.oc.sandbox
			s.mu.Unlock()
			return sb && !policyAllows(s.UserID, v)
		}, adminID, "the policy no longer allows OpenCode's Docker sandbox")
	}
	if _, ok := changed["ai_mcp"]; ok {
		mcpShutdown(adminID, "the policy no longer allows AI connections for the user", func(uid int) bool { ok, _ := mcpUserAllowed(uid); return !ok })
	}
	for k := range changed {
		if strings.HasPrefix(k, "tunnel") {
			tunnelMgr.applyPolicy()
			break
		}
	}
	for key := range changed {
		if strings.HasPrefix(key, "ai_") {
			hub.sendAll([]byte(`{"type":"ai_policy_changed"}`))
			break
		}
	}
}
