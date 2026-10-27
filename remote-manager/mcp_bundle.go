// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ─── MCP: CLAUDE DESKTOP EXTENSION (.mcpb) ───────────
//
//   POST /api/mcp/claude-desktop {token}  →  wrm-pro.mcpb
//
// Claude Desktop starts local (stdio) MCP servers only, and its custom connectors connect
// from Anthropic's cloud, which cannot reach a WRM on a private network or without a public
// HTTPS certificate. The one-click way in is an MCP bundle (a zip with manifest.json) that
// Claude Desktop installs with a double click: its server is the small Node.js bridge in
// mcpb/server/index.js (Claude Desktop ships the Node.js runtime), which talks to this WRM's
// /mcp over HTTP with the token. The address of this WRM is filled in; the token the wizard
// just created can be filled in too (it is then stored by Claude Desktop in the operating
// system's secure storage). The bundle carries nothing else of the user.

//go:embed mcpb/server/index.js
var mcpbBridgeJS string

const mcpbTokenMax = 512

func mcpbVersion() string {
	v := strings.TrimPrefix(AppVersion, "v")
	if v == "" {
		return "0.0.0"
	}
	return v
}

// mcpbManifest is the bundle's manifest.json (MCP bundle manifest version 0.2).
func mcpbManifest(mcpURL, token string) map[string]interface{} {
	urlCfg := map[string]interface{}{
		"type": "string", "title": "WRM MCP address", "required": true, "default": mcpURL,
		"description": "The MCP address of your WRM PRO (Settings → AI connections), for example https://wrm.example.com/mcp.",
	}
	tokCfg := map[string]interface{}{
		"type": "string", "title": "WRM token", "required": true, "sensitive": true,
		"description": "The token of an AI connection (wrm_pat_…), created in WRM → Settings → AI connections.",
	}
	if token != "" {
		tokCfg["default"] = token
	}
	return map[string]interface{}{
		"manifest_version": "0.2",
		"name":             "wrm-pro",
		"display_name":     "WRM PRO",
		"version":          mcpbVersion(),
		"description":      "Work on your servers through WRM PRO: logs, files, services and commands, under WRM's permissions and approvals.",
		"long_description": mcpServerDescription,
		"author":           map[string]interface{}{"name": "WRM PRO", "url": "https://github.com/vedranius/wrm-pro-enterprise"},
		"homepage":         "https://github.com/vedranius/wrm-pro-enterprise",
		"license":          "Apache-2.0",
		"icon":             "icon.png",
		"server": map[string]interface{}{
			"type":        "node",
			"entry_point": "server/index.js",
			"mcp_config": map[string]interface{}{
				"command": "node",
				"args":    []string{"${__dirname}/server/index.js"},
				"env": map[string]string{
					"WRM_MCP_URL":     "${user_config.wrm_url}",
					"WRM_MCP_TOKEN":   "${user_config.wrm_token}",
					"WRM_MCP_CA_FILE": "${user_config.wrm_ca_file}",
				},
			},
		},
		"user_config": map[string]interface{}{
			"wrm_url":   urlCfg,
			"wrm_token": tokCfg,
			"wrm_ca_file": map[string]interface{}{
				"type": "file", "title": "CA certificate (optional)", "required": false,
				"description": "Only for a WRM with a self-signed or private-CA HTTPS certificate: the PEM file to trust.",
			},
		},
		"compatibility": map[string]interface{}{
			"platforms": []string{"darwin", "win32", "linux"},
			"runtimes":  map[string]string{"node": ">=18.0.0"},
		},
		"keywords": []string{"wrm", "ssh", "servers", "logs", "devops"},
	}
}

// pluginMCPBVersion is the version of the WRM PRO Claude plugin
// (packaging/claude-plugin/wrm-pro/.claude-plugin/plugin.json) and of the bridge bundle it
// carries. Raise it together with plugin.json when the bridge or the plugin changes.
const pluginMCPBVersion = "1.2.1"

// pluginMCPBManifest is the manifest of the bundle inside the Claude plugin: no settings at
// all, because Claude Desktop, Cowork and chat cannot ask for plugin settings. The bridge
// finds the address and token in the connection file (~/.wrm/mcp.json) or signs in with the
// wrm_connect tool.
func pluginMCPBManifest() map[string]interface{} {
	return map[string]interface{}{
		"manifest_version": "0.2",
		"name":             "wrm",
		"display_name":     "WRM PRO",
		"version":          pluginMCPBVersion,
		"description":      "Work on your servers through WRM PRO: logs, files, services and commands, under WRM's permissions and approvals.",
		"long_description": mcpServerDescription + "\n\nNo settings are needed: on first use the wrm_connect tool opens WRM in the browser for sign-in, or WRM's setup script for all AI apps saves the connection on this computer.",
		"author":           map[string]interface{}{"name": "WRM PRO", "url": "https://github.com/vedranius/wrm-pro-enterprise"},
		"homepage":         "https://github.com/vedranius/wrm-pro-enterprise",
		"license":          "Apache-2.0",
		"icon":             "icon.png",
		"server": map[string]interface{}{
			"type":        "node",
			"entry_point": "server/index.js",
			"mcp_config": map[string]interface{}{
				"command": "node",
				"args":    []string{"${__dirname}/server/index.js"},
			},
		},
		"compatibility": map[string]interface{}{
			"platforms": []string{"darwin", "win32", "linux"},
			"runtimes":  map[string]string{"node": ">=18.0.0"},
		},
		"keywords": []string{"wrm", "ssh", "servers", "logs", "devops"},
	}
}

// buildMCPB returns the bundle: manifest.json, server/index.js and icon.png.
func buildMCPB(mcpURL, token string) ([]byte, error) {
	return zipMCPB(mcpbManifest(mcpURL, token), mcpbVersion(), time.Now())
}

// buildPluginMCPB returns the bundle committed in the Claude plugin. It is reproducible (fixed
// file times) so a test can tell whether the committed copy is current.
func buildPluginMCPB() ([]byte, error) {
	return zipMCPB(pluginMCPBManifest(), pluginMCPBVersion, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

func zipMCPB(manifest map[string]interface{}, version string, modified time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified})
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	man, _ := json.MarshalIndent(manifest, "", "  ")
	js := strings.Replace(mcpbBridgeJS, "const VERSION = '0.0.0';", "const VERSION = '"+version+"';", 1)
	icon, _ := staticFiles.ReadFile("static/brand/icon-512.png")
	for _, f := range []struct {
		name string
		data []byte
	}{{"manifest.json", man}, {"server/index.js", []byte(js)}, {"icon.png", icon}} {
		if err := add(f.name, f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mcpClaudeDesktopHandler(w http.ResponseWriter, r *http.Request, userID int) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if ok, why := mcpUserAllowed(userID); !ok {
		jsonError(w, why, 403)
		return
	}
	var q struct {
		Token string `json:"token"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
	}
	q.Token = strings.TrimSpace(q.Token)
	// only a token of this WRM may be filled in; the bundle never carries anything else
	if q.Token != "" && (len(q.Token) > mcpbTokenMax || !strings.HasPrefix(q.Token, mcpPATPrefix) || strings.ContainsAny(q.Token, " \t\r\n\"\\")) {
		jsonError(w, "Not a WRM token", 400)
		return
	}
	data, err := buildMCPB(mcpBaseURL(r)+"/mcp", q.Token)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	auditLog(r, userID, "mcp.desktop_extension", "", map[string]interface{}{"with_token": q.Token != ""})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="wrm-pro.mcpb"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// ─── MCP: SETUP SCRIPT FOR ALL AI APPS ───────────────
//
//   POST /api/mcp/setup-all {os: "windows"|"unix"}  →  wrm-setup-all.ps1 / wrm-setup-all.sh
//
// One script connects every AI app on the user's computer: it writes the bridge
// (mcpb/server/index.js) to ~/.wrm/wrm-mcp.js and runs it with --setup, which signs in to this
// WRM in the browser (OAuth; or uses WRM_MCP_TOKEN when set), saves ~/.wrm/mcp.json once and
// registers the bridge as the "wrm" MCP server in Claude Code (as the plugin), the Claude app,
// Codex, Gemini CLI, VS Code, Cursor, Windsurf and OpenCode. The script carries this WRM's
// address and the bridge, never a token.

func shSingleQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
func psSingleQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func mcpSetupScript(osName, mcpURL string) []byte {
	js := strings.Replace(mcpbBridgeJS, "const VERSION = '0.0.0';", "const VERSION = '"+mcpbVersion()+"';", 1)
	comment := strings.NewReplacer("\r", "", "\n", "").Replace(mcpURL) // the address also appears in a comment line
	if osName == "windows" {
		s := "# WRM PRO: connect every AI app on this computer to " + comment + "\r\n" +
			"# Run: powershell -ExecutionPolicy Bypass -File .\\wrm-setup-all.ps1\r\n" +
			"# Sign-in happens in the browser; set $env:WRM_MCP_TOKEN first to use a token instead.\r\n" +
			"$ErrorActionPreference = 'Stop'\r\n" +
			"$wrmUrl = " + psSingleQuote(mcpURL) + "\r\n" +
			"if (-not (Get-Command node -ErrorAction SilentlyContinue)) {\r\n" +
			"  Write-Host 'Node.js 18 or later is needed (the AI apps start WRM with it). Install it, for example: winget install OpenJS.NodeJS.LTS, then run this script again.'\r\n" +
			"  exit 1\r\n}\r\n" +
			"$wrmDir = Join-Path $HOME '.wrm'\r\n" +
			"New-Item -ItemType Directory -Force -Path $wrmDir | Out-Null\r\n" +
			"$wrmBridge = Join-Path $wrmDir 'wrm-mcp.js'\r\n" +
			"$wrmJs = @'\r\n" + strings.ReplaceAll(js, "\n", "\r\n") + "\r\n'@\r\n" +
			"[IO.File]::WriteAllText($wrmBridge, $wrmJs, (New-Object Text.UTF8Encoding $false))\r\n" +
			"& node $wrmBridge --setup --url $wrmUrl\r\n" +
			"exit $LASTEXITCODE\r\n"
		// with a byte order mark, Windows PowerShell 5 reads the script as UTF-8
		return append([]byte("\xef\xbb\xbf"), s...)
	}
	return []byte("#!/bin/sh\n" +
		"# WRM PRO: connect every AI app on this computer to " + comment + "\n" +
		"# Run: sh wrm-setup-all.sh   (sign-in happens in the browser; export WRM_MCP_TOKEN first to use a token instead)\n" +
		"set -eu\n" +
		"wrm_url=" + shSingleQuote(mcpURL) + "\n" +
		"command -v node >/dev/null 2>&1 || { echo 'Node.js 18 or later is needed (the AI apps start WRM with it). Install it with your package manager or from nodejs.org, then run this script again.'; exit 1; }\n" +
		"umask 077\n" +
		"mkdir -p \"$HOME/.wrm\"\n" +
		"cat > \"$HOME/.wrm/wrm-mcp.js\" <<'WRM_BRIDGE_EOF'\n" + js + "\nWRM_BRIDGE_EOF\n" +
		"exec node \"$HOME/.wrm/wrm-mcp.js\" --setup --url \"$wrm_url\"\n")
}

func mcpSetupAllHandler(w http.ResponseWriter, r *http.Request, userID int) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if ok, why := mcpUserAllowed(userID); !ok {
		jsonError(w, why, 403)
		return
	}
	var q struct {
		OS string `json:"os"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); err != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
	}
	osName, name := "unix", "wrm-setup-all.sh"
	if q.OS == "windows" {
		osName, name = "windows", "wrm-setup-all.ps1"
	}
	auditLog(r, userID, "mcp.setup_script", "", map[string]interface{}{"os": osName})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(mcpSetupScript(osName, mcpBaseURL(r)+"/mcp"))
}
