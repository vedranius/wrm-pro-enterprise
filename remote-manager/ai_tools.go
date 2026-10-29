// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── AI: TOOLS ───────────────────────────────────────
//
// The model never gets a shell. It can only call these tools; each call becomes an
// aiAction that the permission engine evaluates before WRM runs anything over its own SSH
// connection. Read tools run fixed scripts with the arguments passed as shell-quoted
// positional parameters; run_command runs exactly the command that was evaluated (or
// approved, or edited by the user); file edits are shown as a unified diff and written
// only if the file did not change since the diff was made.

type aiToolSpec struct {
	Def     aiToolDef
	Kind    string // read | command | write
	Modes   []string
	Expose  bool // offered to the model
	MCPOnly bool // offered to MCP clients only (needs a second session of the same AI connection)
	Host    bool // acts on WRM itself, not on the session's server (administrators only)
}

func obj(props map[string]interface{}, required ...string) map[string]interface{} {
	if required == nil {
		required = []string{}
	}
	return map[string]interface{}{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func strProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}

func intProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": desc}
}

var aiTools = []aiToolSpec{
	{Kind: "read", Def: aiToolDef{Name: "inventory_files", Description: "Read-only bounded recursive file inventory and literal content search using SFTP, without a shell command. Skips symlinks and sensitive paths. Prefer this for .ini/.py configuration inventories. Reports limits and permission failures.", Schema: obj(map[string]interface{}{"path": strProp("Absolute root directory or ~/"), "filter": strProp("Comma-separated extensions, e.g. ini,py,conf; empty means ini,py,conf,json,yaml,yml,log,txt"), "grep": strProp("Optional literal text to find in file contents"), "limit": intProp("Maximum files, 1-3000, default 500")}, "path")}},
	{Kind: "read", Def: aiToolDef{Name: "system_info", Description: "Show the server's current date and time, operating system, kernel, uptime, memory and CPU count.", Schema: obj(map[string]interface{}{})}},
	{Kind: "read", Def: aiToolDef{Name: "read_file", Description: "Read a text file on the server (size-limited). Sensitive files (password hashes, private keys, credentials) are refused.",
		Schema: obj(map[string]interface{}{"path": strProp("Absolute path, or ~/ for the home directory"), "offset": intProp("Byte offset to start at (default 0)"), "max_bytes": intProp("Bytes to read (default and maximum: the policy limit)")}, "path")}},
	{Kind: "read", Def: aiToolDef{Name: "list_directory", Description: "List a directory with sizes, owners, permissions and times.",
		Schema: obj(map[string]interface{}{"path": strProp("Absolute path, or ~/")}, "path")}},
	{Kind: "read", Def: aiToolDef{Name: "tail_log", Description: "Show the last lines of a log file.",
		Schema: obj(map[string]interface{}{"path": strProp("Absolute path of the log file"), "lines": intProp("Number of lines (1-2000, default 200)")}, "path")}},
	{Kind: "read", Def: aiToolDef{Name: "find_logs", Description: "List the log sources WRM discovered on this server: services with the folders or journal unit each one logs to. Uses the same discovery as Monitoring → Logs (cached for an hour; the connection owner may trigger one scan over this session's connection). Start here when asked why a service fails or where its logs are.",
		Schema: obj(map[string]interface{}{"filter": strProp("Only services whose name, kind or path contains this text"), "limit": intProp("Maximum services to list (1-300, default 100)")})}},
	{Kind: "read", Def: aiToolDef{Name: "read_log", Description: "Read the log of a service found with find_logs, or a file path. With a service name it reads that service's newest log file, or its systemd journal; the other files of the service are named at the end. Optional literal grep keeps matching lines only. Output is size-limited and redacted like every read.",
		Schema: obj(map[string]interface{}{"service": strProp("Service name from find_logs, with or without .service"), "path": strProp("Log file to read instead of a service (absolute, or ~/)"),
			"lines": intProp("Number of lines (1-2000, default 200)"), "grep": strProp("Only lines containing this literal text")})}},
	{Kind: "read", Def: aiToolDef{Name: "search_logs", Description: "Search many log files at once for a literal text (case-insensitive), on the server with grep. It searches the log files WRM discovered (call find_logs first); narrow it with a service name or a path filter. Each result line is file:line:text.",
		Schema: obj(map[string]interface{}{"grep": strProp("The literal text to find, e.g. an error message, an id or an IP address"), "service": strProp("Only the log files of this service (from find_logs)"),
			"filter": strProp("Only log files whose path, service or operator contains this text"), "limit": intProp("Maximum result lines (1-1000, default 200)")}, "grep")}},
	{Kind: "read", Def: aiToolDef{Name: "journal", Description: "Read the systemd journal (journalctl), optionally for one unit, since a time, at a priority, matching a pattern.",
		Schema: obj(map[string]interface{}{"unit": strProp("systemd unit, e.g. nginx.service"), "lines": intProp("Number of lines (1-2000, default 200)"),
			"since": strProp("e.g. '1 hour ago', 'today', '2025-01-31 10:00'"), "priority": strProp("emerg, alert, crit, err, warning, notice, info or debug"), "grep": strProp("Only lines matching this pattern")})}},
	{Kind: "read", Def: aiToolDef{Name: "service_status", Description: "Show the status of a service, or list all services when no name is given.",
		Schema: obj(map[string]interface{}{"name": strProp("Service name, e.g. nginx")})}},
	{Kind: "read", Def: aiToolDef{Name: "list_packages", Description: "List installed packages (dpkg, rpm, apk or pacman), optionally filtered.",
		Schema: obj(map[string]interface{}{"filter": strProp("Only packages whose line contains this text")})}},
	{Kind: "read", Def: aiToolDef{Name: "processes", Description: "List the processes using the most CPU or memory.",
		Schema: obj(map[string]interface{}{"sort": strProp("cpu (default) or mem"), "limit": intProp("Number of processes (default 25)")})}},
	{Kind: "read", Def: aiToolDef{Name: "disk_usage", Description: "Show file system usage, and the largest entries of a directory when a path is given.",
		Schema: obj(map[string]interface{}{"path": strProp("Directory to summarise (optional)")})}},
	{Kind: "read", Def: aiToolDef{Name: "network_status", Description: "Show addresses, routes and listening sockets.", Schema: obj(map[string]interface{}{})}},
	{Kind: "command", Def: aiToolDef{Name: "run_command", Description: "Run one shell command on the server. Read-only commands run directly; commands that change the system need the user's approval (or the session's automatic-mode allow list) and are refused in read-only mode. Destructive commands are always refused. Non-interactive only: no pagers, editors or password prompts (use sudo -n).",
		Schema: obj(map[string]interface{}{"command": strProp("The exact command line"), "reason": strProp("One sentence for the user: why this command")}, "command", "reason")}},
	{Kind: "write", Def: aiToolDef{Name: "write_file", Description: "Create or replace a text file. The user sees a unified diff and must approve it (unless the session's automatic mode allows the path).",
		Schema: obj(map[string]interface{}{"path": strProp("Absolute path, or ~/"), "content": strProp("The complete new content"), "reason": strProp("One sentence for the user: why this change")}, "path", "content", "reason")}},
	{Kind: "write", Def: aiToolDef{Name: "edit_file", Description: "Replace one exact occurrence of a text in a file. The user sees a unified diff and must approve it (unless the session's automatic mode allows the path).",
		Schema: obj(map[string]interface{}{"path": strProp("Absolute path, or ~/"), "old_text": strProp("The exact text to replace; it must occur exactly once"),
			"new_text": strProp("The replacement"), "reason": strProp("One sentence for the user: why this change")}, "path", "old_text", "new_text", "reason")}},
	{Kind: "write", MCPOnly: true, Def: aiToolDef{Name: "transfer_file", Description: "Copy one file from another server of this AI connection to this session's server, through WRM (SFTP). The destination is treated like a file edit: refused in read-only mode, otherwise the user approves it (unless the automatic mode allows the path). Sensitive source files are refused.",
		Schema: obj(map[string]interface{}{"path": strProp("Destination path on this session's server (absolute, or ~/)"), "from_path": strProp("Source path on the other server (absolute, or ~/)"),
			"reason": strProp("One sentence for the user: why this transfer")}, "path", "from_path", "reason")}},
	// The addon loop works on WRM itself, not on the session's server. Every
	// change waits for the user's approval in every mode, and a draft is never
	// enabled by these tools.
	{Kind: "read", Host: true, Def: aiToolDef{Name: "addon_list", Description: "List the addons installed in WRM and the addon drafts in the AI workspace. Drafts are files waiting for the user's review in Settings → Addons; they are never served or executed.",
		Schema: obj(map[string]interface{}{})}},
	{Kind: "read", Host: true, Def: aiToolDef{Name: "addon_files", Description: "List the files of an installed addon or of a draft, with sizes and the manifest; give a path to read one text file. Use it to inspect what is installed before writing a draft.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id"), "draft": map[string]interface{}{"type": "boolean", "description": "Read the draft instead of the installed addon"},
			"path": strProp("Optional relative file to read")}, "id")}},
	{Kind: "write", Host: true, Def: aiToolDef{Name: "addon_create", Description: "Create or replace an addon draft in WRM's AI workspace from a set of text files (addon.json at the root plus the files it declares). The draft is validated exactly like a ZIP; nothing is installed, served or executed. The user reviews the diff in Settings → Addons and installs it; with install=true the same call also installs it (a new addon stays disabled). Every call needs the user's approval.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id, matching addon.json"), "files": map[string]interface{}{"type": "object", "description": "Relative path → file content", "additionalProperties": map[string]interface{}{"type": "string"}},
			"install": map[string]interface{}{"type": "boolean", "description": "Also install the validated draft after writing it (disabled for a new addon)"},
			"reason":  strProp("One sentence for the user: why this addon")}, "id", "files", "reason")}},
	{Kind: "write", Host: true, Def: aiToolDef{Name: "addon_write", Description: "Write one file into an existing addon draft. The draft is validated again exactly like a ZIP; a broken write leaves the previous draft untouched. Every call needs the user's approval.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id"), "path": strProp("Relative path inside the addon"), "content": strProp("New text content"),
			"content_base64": strProp("Base64 content instead of content, for a binary file such as plugin.wasm"), "install": map[string]interface{}{"type": "boolean", "description": "Also install the validated draft after writing it"},
			"reason": strProp("One sentence for the user: why this change")}, "id", "path", "reason")}},
	{Kind: "write", Host: true, Def: aiToolDef{Name: "addon_enable", Description: "Enable an installed addon. Needs the user's approval in every mode.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id"), "reason": strProp("One sentence for the user: why enable it")}, "id", "reason")}},
	{Kind: "write", Host: true, Def: aiToolDef{Name: "addon_disable", Description: "Disable an installed addon. Needs the user's approval in every mode.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id"), "reason": strProp("One sentence for the user: why disable it")}, "id", "reason")}},
	{Kind: "write", Host: true, Def: aiToolDef{Name: "addon_delete", Description: "Remove an installed addon and delete its files. Needs the user's approval in every mode.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id"), "reason": strProp("One sentence for the user: why remove it")}, "id", "reason")}},
	{Kind: "command", Host: true, Def: aiToolDef{Name: "addon_run", Description: "Run one export of an enabled addon's WebAssembly module in WRM's sandbox (no files, no network, 64 MiB, five seconds). Needs the user's approval in every mode.",
		Schema: obj(map[string]interface{}{"id": strProp("Addon id"), "export": strProp("Export declared in addon.json"), "input": strProp("JSON request for the export (default {})"),
			"reason": strProp("One sentence for the user: why run it")}, "id", "export", "reason")}},
	{Kind: "write", Host: true, Def: aiToolDef{Name: "addon_install_community", Description: "Install one addon from the community index. The download must match the SHA-256 in the index and then passes the same archive checks as an upload. Needs the user's approval in every mode.",
		Schema: obj(map[string]interface{}{"community_id": strProp("Addon id from the community index"), "reason": strProp("One sentence for the user: why install it")}, "community_id", "reason")}},
}

// aiScopesFor lists the MCP scopes that allow a tool (any one of them is enough).
func aiScopesFor(name string) []string {
	spec, ok := aiToolSpecFor(name)
	if !ok {
		return nil
	}
	if spec.Host {
		return []string{"addons.manage"}
	}
	switch {
	case spec.Kind == "read":
		return []string{"read_logs"}
	case spec.Kind == "command":
		return []string{"run_readonly", "run_with_approval"}
	case name == "transfer_file":
		return []string{"transfer"}
	}
	return []string{"edit_file_with_approval"}
}

// aiToolInScopes reports whether a session's scopes allow a tool; nil scopes (the
// built-in panel) allow every tool except the MCP-only ones.
func aiToolInScopes(name string, scopes []string) bool {
	if scopes == nil {
		spec, ok := aiToolSpecFor(name)
		return ok && !spec.MCPOnly
	}
	for _, sc := range aiScopesFor(name) {
		if oneOf(sc, scopes...) {
			return true
		}
	}
	return false
}

type aiCtxKey int

const aiTransferSourceKey aiCtxKey = 1

// withTransferSource passes the source session of a transfer_file call to the engine.
func withTransferSource(ctx context.Context, src *aiSession) context.Context {
	return context.WithValue(ctx, aiTransferSourceKey, src)
}

// aiReadRealTools print the real path before any content, so the engine can
// refuse a path that resolves to a sensitive file.
var aiReadRealTools = map[string]bool{"read_file": true, "tail_log": true, "read_log": true}

func aiToolSpecFor(name string) (aiToolSpec, bool) {
	for _, t := range aiTools {
		if t.Def.Name == name {
			return t, true
		}
	}
	return aiToolSpec{}, false
}

// aiToolDefsFor lists the tools offered to the model in a mode: read-only sessions are
// not offered the write tools at all (run_command stays: read-only commands are allowed).
func aiToolDefsFor(mode string) []aiToolDef {
	return aiToolDefsForUser(mode, true)
}

// aiToolDefsForUser also hides the host-side addon tools from non-administrators.
func aiToolDefsForUser(mode string, admin bool) []aiToolDef {
	var out []aiToolDef
	for _, t := range aiTools {
		if (t.Kind == "write" && mode == aiModeReadOnly) || t.MCPOnly || (t.Host && !admin) {
			continue
		}
		out = append(out, t.Def)
	}
	return out
}

func (s *aiSession) toolDefs() []aiToolDef {
	defs := aiToolDefsForUser(s.Mode, isAdminUser(s.UserID))
	if _, ok := aiPurposeTools[s.Purpose]; !ok {
		return defs
	}
	out := defs[:0]
	for _, d := range defs {
		if s.purposeAllows(d.Name) {
			out = append(out, d)
		}
	}
	return out
}

// aiToolInput is the union of every tool's arguments.
type aiToolInput struct {
	Path     string `json:"path"`
	FilePath string `json:"file_path"` // accepted as an alias for path: many models know it from other tools
	Offset   int64  `json:"offset"`
	MaxBytes int64  `json:"max_bytes"`
	Lines    int    `json:"lines"`
	Unit     string `json:"unit"`
	Since    string `json:"since"`
	Priority string `json:"priority"`
	Grep     string `json:"grep"`
	Name     string `json:"name"`
	Service  string `json:"service"`
	Filter   string `json:"filter"`
	Sort     string `json:"sort"`
	Limit    int    `json:"limit"`
	Command  string `json:"command"`
	Reason   string `json:"reason"`
	Content  string `json:"content"`
	OldText  string `json:"old_text"`
	NewText  string `json:"new_text"`
	FromPath string `json:"from_path"`
	// addon tools (host)
	ID            string            `json:"id"`
	Files         map[string]string `json:"files"`
	Install       bool              `json:"install"`
	Draft         bool              `json:"draft"`
	Export        string            `json:"export"`
	Input         string            `json:"input"`
	ContentBase64 string            `json:"content_base64"`
	CommunityID   string            `json:"community_id"`
}

var (
	reUnit     = regexp.MustCompile(`^[A-Za-z0-9@._:\\-]{1,200}$`)
	reSince    = regexp.MustCompile(`^[A-Za-z0-9 :.+-]{1,60}$`)
	rePriority = regexp.MustCompile(`^(emerg|alert|crit|err|warning|notice|info|debug|[0-7])(\.\.(emerg|alert|crit|err|warning|notice|info|debug|[0-7]))?$`)
)

func aiClamp(v, def, lo, hi int) int {
	if v == 0 {
		v = def
	}
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	return v
}

// aiPlanned is a tool call turned into something to evaluate and run.
type aiPlanned struct {
	Act     aiAction
	Rule    string // the policy rule that asked for approval (audited with the answer)
	Script  string // command line to run (read tools and run_command)
	Display string // what the user sees (command or tool summary)
	// writes
	Write     bool
	Path      string
	Old       string
	OldHash   string // "new" when the file does not exist
	NewText   string
	Diff      string
	Inventory *aiToolInput
	// Logs (find_logs): the discovery listing, produced in run
	Logs *aiToolInput
	// Host: an addon tool that acts on WRM itself, not on the session's server
	Host *aiToolInput
	// transfers (transfer_file): the source session, its real path and the size
	Transfer bool
	From     *aiSession
	FromPath string
	Size     int64
	// NoEdit: approved or refused as it is (an OpenCode sandbox request)
	NoEdit bool
	// set by the engine
	Approved    bool // the user approved this call
	ForceRedact bool // redact secret-looking values for the model whatever ai_redact_output says
}

func aiRequirePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("a path is required")
	}
	if !strings.HasPrefix(p, "/") && p != "~" && !strings.HasPrefix(p, "~/") {
		return "", fmt.Errorf("use an absolute path (or ~/…)")
	}
	if strings.ContainsAny(p, "\x00\n\r") || len(p) > 1024 {
		return "", fmt.Errorf("invalid path")
	}
	return p, nil
}

// aiReadMax is the read limit for files (bytes).
func aiReadMax() int64 { return int64(settingInt("ai_output_max_kb")) << 10 }

// plan turns a tool call into an action. File edits read the current file first (through
// a read-only script) to build the diff.
func (s *aiSession) plan(ctx context.Context, name string, in aiToolInput) (*aiPlanned, error) {
	if spec, ok := aiToolSpecFor(name); ok && spec.Host {
		return s.planHostTool(name, in)
	}
	q := shellQuote
	switch name {
	case "inventory_files":
		p, err := aiRequirePath(in.Path)
		if err != nil {
			return nil, err
		}
		in.Path = aiExpandHome(p, s.home)
		if len(in.Grep) > 200 || strings.ContainsAny(in.Grep, "\x00\r\n") {
			return nil, fmt.Errorf("invalid search text")
		}
		if _, err := inventoryExtensions(in.Filter); err != nil {
			return nil, err
		}
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read", Path: in.Path}, Display: "file inventory " + in.Path, Inventory: &in}, nil
	case "system_info":
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "system information",
			Script: `echo "Server date: $(date '+%Y-%m-%d %H:%M:%S %Z (%z)' 2>/dev/null || date)"; echo; uname -a; echo; cat /etc/os-release 2>/dev/null; echo; uptime; echo; free -m 2>/dev/null; echo; echo "CPUs: $(nproc 2>/dev/null)"`}, nil
	case "read_file", "tail_log":
		p, err := aiRequirePath(in.Path)
		if err != nil {
			return nil, err
		}
		p = aiExpandHome(p, s.home)
		if name == "tail_log" {
			return &aiPlanned{Act: aiAction{Tool: name, Kind: "read", Path: p}, Display: name + " " + p, Script: aiTailScript(p, in.Lines, "")}, nil
		}
		max := aiReadMax()
		n := in.MaxBytes
		if n <= 0 || n > max {
			n = max
		}
		off := in.Offset
		if off < 0 {
			off = 0
		}
		body := `tail -c +` + strconv.FormatInt(off+1, 10) + ` -- "$r" | head -c ` + strconv.FormatInt(n, 10)
		// the real path is printed first so a symlink to a sensitive file is caught
		script := `f=` + q(p) + `; r=$(readlink -f -- "$f" 2>/dev/null || printf %s "$f"); printf 'WRM-REAL %s\n' "$r"; ` +
			`[ -e "$r" ] || { echo "No such file: $f"; exit 2; }; [ -d "$r" ] && { echo "Is a directory: $f"; exit 2; }; ` +
			`printf 'WRM-SIZE %s\n' "$(wc -c < "$r" 2>/dev/null)"; ` + body
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read", Path: p}, Display: name + " " + p, Script: script}, nil
	case "find_logs":
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "find logs " + in.Filter, Logs: &in}, nil
	case "search_logs":
		return s.planSearchLogs(in)
	case "read_log":
		grep := in.Grep
		if len(grep) > 200 || strings.ContainsAny(grep, "\x00\r\n") {
			return nil, fmt.Errorf("invalid pattern")
		}
		if in.Path != "" {
			p, err := aiRequirePath(in.Path)
			if err != nil {
				return nil, err
			}
			p = aiExpandHome(p, s.home)
			return &aiPlanned{Act: aiAction{Tool: name, Kind: "read", Path: p}, Display: "read_log " + p, Script: aiTailScript(p, in.Lines, grep)}, nil
		}
		service := strings.TrimSpace(in.Service)
		if service == "" {
			return nil, fmt.Errorf("give a service name (see find_logs) or a path")
		}
		if len(service) > 200 || strings.ContainsAny(service, "\x00\r\n") {
			return nil, fmt.Errorf("invalid service name")
		}
		files, unit, err := s.aiServiceLogs(ctx, service)
		if err != nil {
			return nil, err
		}
		if unit != "" {
			cmd := "journalctl --no-pager -o short-iso -n " + strconv.Itoa(aiClamp(in.Lines, 200, 1, 2000)) + " -u " + shellQuote(unit)
			if grep != "" {
				cmd += " -g " + shellQuote(grep)
			}
			return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "read_log journal " + unit, Script: cmd + " 2>&1"}, nil
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no readable log file was found for service %q; call find_logs to see which services have logs", service)
		}
		chosen := files[0]
		script := aiTailScript(chosen.Path, in.Lines, grep)
		if len(files) > 1 {
			others := []string{}
			for _, f := range files[1:] {
				others = append(others, f.Path)
			}
			note := fmt.Sprintf("[newest log of %s; other files: %s]", service, strings.Join(others, ", "))
			script += `; printf '\n%s\n' ` + shellQuote(note)
		}
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read", Path: chosen.Path}, Display: "read_log " + service + " (" + chosen.Path + ")", Script: script}, nil
	case "list_directory":
		p, err := aiRequirePath(in.Path)
		if err != nil {
			return nil, err
		}
		p = aiExpandHome(p, s.home)
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "ls -la " + p,
			Script: `ls -la --time-style=long-iso -- ` + q(p) + ` 2>/dev/null || ls -la -- ` + q(p)}, nil
	case "journal":
		args := []string{"journalctl", "--no-pager", "-o", "short-iso", "-n", strconv.Itoa(aiClamp(in.Lines, 200, 1, 2000))}
		if in.Unit != "" {
			if !reUnit.MatchString(in.Unit) {
				return nil, fmt.Errorf("invalid unit name")
			}
			args = append(args, "-u", q(in.Unit))
		}
		if in.Since != "" {
			if !reSince.MatchString(in.Since) {
				return nil, fmt.Errorf("invalid since value")
			}
			args = append(args, "--since", q(in.Since))
		}
		if in.Priority != "" {
			if !rePriority.MatchString(in.Priority) {
				return nil, fmt.Errorf("invalid priority")
			}
			args = append(args, "-p", in.Priority)
		}
		if in.Grep != "" {
			if len(in.Grep) > 200 || strings.ContainsAny(in.Grep, "\x00\n") {
				return nil, fmt.Errorf("invalid pattern")
			}
			args = append(args, "-g", q(in.Grep))
		}
		cmd := strings.Join(args, " ")
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: cmd, Script: cmd + " 2>&1"}, nil
	case "service_status":
		if in.Name == "" {
			return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "list services",
				Script: `systemctl list-units --type=service --all --no-pager --no-legend 2>/dev/null || service --status-all 2>&1 || rc-status -a 2>&1`}, nil
		}
		if !reUnit.MatchString(in.Name) {
			return nil, fmt.Errorf("invalid service name")
		}
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "systemctl status " + in.Name,
			Script: `systemctl status --no-pager --lines=30 -- ` + q(in.Name) + ` 2>&1 || service ` + q(in.Name) + ` status 2>&1`}, nil
	case "list_packages":
		filter := `cat`
		if in.Filter != "" {
			if len(in.Filter) > 100 || strings.ContainsAny(in.Filter, "\x00\n") {
				return nil, fmt.Errorf("invalid filter")
			}
			filter = `grep -i -F -- ` + q(in.Filter)
		}
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "list packages " + in.Filter,
			Script: `{ if command -v dpkg-query >/dev/null 2>&1; then dpkg-query -W -f='${Package} ${Version} ${Status}\n'; ` +
				`elif command -v rpm >/dev/null 2>&1; then rpm -qa --qf '%{NAME} %{VERSION}-%{RELEASE}\n'; ` +
				`elif command -v apk >/dev/null 2>&1; then apk info -v; elif command -v pacman >/dev/null 2>&1; then pacman -Q; ` +
				`else echo "no known package manager"; fi; } 2>/dev/null | ` + filter + ` | head -n 1000`}, nil
	case "processes":
		key := "-%cpu"
		if in.Sort == "mem" {
			key = "-%mem"
		}
		n := aiClamp(in.Limit, 25, 1, 200)
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "ps aux --sort=" + key,
			Script: `ps aux --sort=` + key + ` 2>/dev/null | head -n ` + strconv.Itoa(n+1) + ` || ps aux | head -n ` + strconv.Itoa(n+1)}, nil
	case "disk_usage":
		script := `df -hP 2>/dev/null || df -h`
		disp := "df -h"
		if in.Path != "" {
			p, err := aiRequirePath(in.Path)
			if err != nil {
				return nil, err
			}
			p = aiExpandHome(p, s.home)
			script += `; echo; du -xsh -- ` + q(p) + `/* ` + q(p) + `/.[!.]* 2>/dev/null | sort -h | tail -n 25`
			disp += "; du -xsh " + p + "/*"
		}
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: disp, Script: script}, nil
	case "network_status":
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "read"}, Display: "addresses, routes, listening sockets",
			Script: `(ip -brief addr 2>/dev/null || ifconfig -a 2>/dev/null); echo; (ip route 2>/dev/null || netstat -rn 2>/dev/null); echo; (ss -tulpn 2>/dev/null || netstat -tuln 2>/dev/null)`}, nil
	case "run_command":
		cmd := strings.TrimSpace(in.Command)
		if cmd == "" {
			return nil, fmt.Errorf("the command is empty")
		}
		if len(cmd) > 8000 || strings.ContainsRune(cmd, 0) {
			return nil, fmt.Errorf("the command is too long")
		}
		return &aiPlanned{Act: aiAction{Tool: name, Kind: "command", Command: cmd}, Display: cmd, Script: cmd}, nil
	case "write_file", "edit_file":
		p, err := aiRequirePath(in.Path)
		if err != nil {
			return nil, err
		}
		p = aiExpandHome(p, s.home)
		old, hash, real, err := s.readForEdit(ctx, p)
		if err != nil {
			return nil, err
		}
		newText := in.Content
		if name == "edit_file" {
			if hash == "new" {
				return nil, fmt.Errorf("the file does not exist (use write_file to create it)")
			}
			if in.OldText == "" {
				return nil, fmt.Errorf("old_text is empty")
			}
			switch strings.Count(old, in.OldText) {
			case 0:
				return nil, fmt.Errorf("old_text was not found in the file")
			case 1:
			default:
				return nil, fmt.Errorf("old_text occurs more than once; include more context")
			}
			newText = strings.Replace(old, in.OldText, in.NewText, 1)
		}
		if len(newText) > 2<<20 {
			return nil, fmt.Errorf("the new content is larger than 2 MB")
		}
		pl := &aiPlanned{Act: aiAction{Tool: name, Kind: "write", Path: real}, Write: true, Path: p, Old: old, OldHash: hash, NewText: newText}
		pl.Diff = aiUnifiedDiff(p, old, newText, hash == "new")
		pl.Display = "edit " + p
		if real != p {
			pl.Display += " (→ " + real + ")"
		}
		return pl, nil
	case "transfer_file":
		return s.planTransfer(ctx, in)
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}

// readForEdit reads a file before an edit: content, SHA-256 ("new" when missing) and the
// real path behind symlinks.
func (s *aiSession) readForEdit(ctx context.Context, p string) (string, string, string, error) {
	script := `f=` + shellQuote(p) + `; r=$(readlink -f -- "$f" 2>/dev/null || printf %s "$f"); printf 'WRM-REAL %s\n' "$r"; ` +
		`if [ ! -e "$r" ]; then echo WRM-NEW; exit 0; fi; [ -f "$r" ] || { echo "WRM-ERR not a regular file"; exit 0; }; ` +
		`n=$(wc -c < "$r"); [ "$n" -le 2097152 ] || { echo "WRM-ERR larger than 2 MB"; exit 0; }; echo WRM-BEGIN; cat -- "$r"`
	out, code, err := s.exec(ctx, script, "", 0)
	if err != nil {
		return "", "", "", err
	}
	if code != 0 {
		return "", "", "", fmt.Errorf("cannot read the file (exit %d)", code)
	}
	real := p
	if strings.HasPrefix(out, "WRM-REAL ") {
		i := strings.Index(out, "\n")
		if i < 0 {
			return "", "", "", fmt.Errorf("unexpected answer from the server")
		}
		real = out[len("WRM-REAL "):i]
		out = out[i+1:]
	}
	switch {
	case strings.HasPrefix(out, "WRM-NEW"):
		if why := aiReadDenied(real); why != "" {
			return "", "", "", fmt.Errorf("the file is not writable by the assistant (%s)", why)
		}
		return "", "new", real, nil
	case strings.HasPrefix(out, "WRM-ERR "):
		return "", "", "", fmt.Errorf("%s", strings.TrimSpace(out[8:]))
	case strings.HasPrefix(out, "WRM-BEGIN\n"):
		content := out[len("WRM-BEGIN\n"):]
		if why := aiReadDenied(real); why != "" {
			return "", "", "", fmt.Errorf("the file is not readable by the assistant (%s)", why)
		}
		sum := sha256.Sum256([]byte(content))
		return content, hex.EncodeToString(sum[:]), real, nil
	}
	return "", "", "", fmt.Errorf("unexpected answer from the server")
}

// aiWriteScript writes stdin to the file if it still has the hash the diff was made from.
func aiWriteScript(p, oldHash string) string {
	return `f=` + shellQuote(p) + `; want=` + shellQuote(oldHash) + `; ` +
		`if [ "$want" = new ]; then [ -e "$f" ] && { echo "The file was created meanwhile; not written."; exit 3; }; ` +
		`d=$(dirname -- "$f"); [ -d "$d" ] || { echo "No such directory: $d"; exit 2; }; ` +
		`else cur=$( (sha256sum -- "$f" 2>/dev/null || shasum -a 256 -- "$f" 2>/dev/null || openssl dgst -sha256 -r -- "$f") | cut -d' ' -f1); ` +
		`[ "$cur" = "$want" ] || { echo "The file changed since the diff was made; not written."; exit 3; }; fi; ` +
		`cat > "$f" && echo "Written: $f"`
}

// aiUnifiedDiff renders a unified diff of a file change.
func aiUnifiedDiff(p, a, b string, created bool) string {
	var sb strings.Builder
	from := "a" + p
	if created {
		from = "/dev/null"
	}
	sb.WriteString("--- " + from + "\n+++ b" + p + "\n")
	lines, _ := unifiedDiff(splitLines(a), splitLines(b), 3)
	if len(lines) == 0 {
		sb.WriteString("(no changes)\n")
		return sb.String()
	}
	for _, l := range lines {
		switch l.Op {
		case "@":
			fmt.Fprintf(&sb, "@@ -%d +%d @@\n", l.A, l.B)
		default:
			sb.WriteString(l.Op + l.S + "\n")
		}
	}
	return sb.String()
}

// decodeToolInput parses a model's arguments strictly (unknown fields are refused).
func decodeToolInput(raw json.RawMessage) (aiToolInput, error) {
	var in aiToolInput
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return in, fmt.Errorf("invalid arguments: %v", err)
	}
	// Models trained on other tools often send file_path; treat it as path so a
	// correct edit still reaches the approval step instead of failing on a name.
	if in.Path == "" && in.FilePath != "" {
		in.Path = in.FilePath
	}
	return in, nil
}

// planTransfer checks both ends of a transfer_file call: the source must be a readable,
// not sensitive regular file within the size limit; the destination is evaluated as a
// write of its real path.
func (s *aiSession) planTransfer(ctx context.Context, in aiToolInput) (*aiPlanned, error) {
	src, _ := ctx.Value(aiTransferSourceKey).(*aiSession)
	if src == nil {
		return nil, fmt.Errorf("from_session_id is required: open a session to the source server with the same AI connection first")
	}
	dst, err := aiRequirePath(in.Path)
	if err != nil {
		return nil, err
	}
	from, err := aiRequirePath(in.FromPath)
	if err != nil {
		return nil, fmt.Errorf("from_path: %v", err)
	}
	src.mu.Lock()
	srcHome := src.home
	src.mu.Unlock()
	if srcHome == "" {
		src.gather(ctx)
		src.mu.Lock()
		srcHome = src.home
		src.mu.Unlock()
	}
	from = aiExpandHome(from, srcHome)
	dst = aiExpandHome(dst, s.home)
	out, code, err := src.exec(ctx, `f=`+shellQuote(from)+`; r=$(readlink -f -- "$f" 2>/dev/null || printf %s "$f"); printf 'WRM-REAL %s\n' "$r"; `+
		`[ -f "$r" ] || { echo "WRM-ERR not a regular file"; exit 0; }; [ -r "$r" ] || { echo "WRM-ERR not readable"; exit 0; }; printf 'WRM-SIZE %s\n' "$(wc -c < "$r")"`, "", 30*time.Second)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("cannot check the source file (exit %d)", code)
	}
	var real string
	var size int64 = -1
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "WRM-REAL "):
			real = strings.TrimPrefix(l, "WRM-REAL ")
		case strings.HasPrefix(l, "WRM-ERR "):
			return nil, fmt.Errorf("source %s: %s", from, strings.TrimPrefix(l, "WRM-ERR "))
		case strings.HasPrefix(l, "WRM-SIZE "):
			size, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(l, "WRM-SIZE ")), 10, 64)
		}
	}
	if real == "" || size < 0 {
		return nil, fmt.Errorf("unexpected answer from the source server")
	}
	if why := aiReadDenied(real); why != "" {
		return nil, fmt.Errorf("%s is not readable by the assistant (%s)", real, why)
	}
	if max := int64(settingInt("ai_mcp_transfer_max_mb")) << 20; size > max {
		return nil, fmt.Errorf("the file is larger than the transfer limit (%d MB)", max>>20)
	}
	out, _, err = s.exec(ctx, `f=`+shellQuote(dst)+`; r=$(readlink -f -- "$f" 2>/dev/null || printf %s "$f"); printf 'WRM-REAL %s\n' "$r"; [ -e "$r" ] && echo WRM-EXISTS; [ -d "$r" ] && echo WRM-DIR; true`, "", 30*time.Second)
	if err != nil {
		return nil, err
	}
	realDst := dst
	exists := false
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "WRM-REAL "):
			realDst = strings.TrimPrefix(l, "WRM-REAL ")
		case l == "WRM-EXISTS":
			exists = true
		case l == "WRM-DIR":
			return nil, fmt.Errorf("the destination is a directory: give the full file path")
		}
	}
	verb := "create"
	if exists {
		verb = "replace"
	}
	pl := &aiPlanned{Act: aiAction{Tool: "transfer_file", Kind: "write", Path: realDst}, Transfer: true, Path: realDst, From: src, FromPath: real, Size: size}
	pl.Display = fmt.Sprintf("transfer %s:%s → %s:%s (%s, %d bytes)", src.ConnName, real, s.ConnName, realDst, verb, size)
	return pl, nil
}
