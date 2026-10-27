// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Runtime checks use the same catalog and SSH access as Git comparisons. They never
// start/restart services or execute the catalog's deployment check commands.
type gitFunctionalitySpec struct {
	Units       []gitUnit `json:"units,omitempty"`
	HTTP        string    `json:"http,omitempty"`
	TCP         string    `json:"tcp,omitempty"`
	ConfigFiles []string  `json:"config_files,omitempty"`
}

type gitRuntimeCheck struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	State  string `json:"state"` // healthy | failed | unknown
	Status string `json:"status"`
	Log    string `json:"log,omitempty"`
}

type gitFunctionality struct {
	State     string            `json:"state"` // healthy | warning | failed | unknown
	CheckedAt string            `json:"checked_at"`
	Checks    []gitRuntimeCheck `json:"checks"`
	Warnings  []string          `json:"warnings"`
}

type gitServerHealth struct {
	Hostname string `json:"hostname"`
	Folder   string `json:"folder"`
	ConnID   int    `json:"conn_id"`
	Name     string `json:"name"`
	gitFunctionality
}

var gitRuntimeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@+*-]{0,199}$`)

func (s *gitFunctionalitySpec) normalize() error {
	if len(s.Units) > 64 || len(s.ConfigFiles) > 20 {
		return fmt.Errorf("at most 64 units and 20 configuration files per service")
	}
	for _, u := range s.Units {
		if (u.Kind != "systemd" && u.Kind != "supervisor") || !gitRuntimeNameRe.MatchString(u.Name) || strings.Contains(u.Name, "*") {
			return fmt.Errorf("invalid runtime unit: %s %s", u.Kind, u.Name)
		}
	}
	s.HTTP = strings.TrimSpace(s.HTTP)
	if s.HTTP != "" {
		u, err := url.Parse(s.HTTP)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(s.HTTP) > 1000 {
			return fmt.Errorf("health HTTP URL must be http(s), without credentials or a fragment")
		}
	}
	s.TCP = strings.TrimSpace(s.TCP)
	if s.TCP != "" {
		if host, port, err := net.SplitHostPort(s.TCP); err != nil || host == "" || !validHealthPort(port) {
			return fmt.Errorf("health TCP address must be host:port")
		}
	}
	for i, f := range s.ConfigFiles {
		if strings.HasPrefix(f, "/") || strings.ContainsAny(f, "\x00\n\r\t") {
			return fmt.Errorf("configuration paths must be relative to the installation")
		}
		f = cleanRel(f)
		if f == "" || f == ".." || strings.HasPrefix(f, "../") {
			return fmt.Errorf("invalid configuration path")
		}
		s.ConfigFiles[i] = f
	}
	return nil
}
func validHealthPort(s string) bool { n, e := strconv.Atoi(s); return e == nil && n > 0 && n < 65536 }

const gitRuntimePrelude = `enc() { printf '%s' "$1" | head -c 16384 | base64 | tr -d '\n'; }
record() { printf 'C\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$(enc "$4")" "$(enc "$5")"; }
bounded() { if command -v timeout >/dev/null 2>&1; then timeout 6 "$@"; else "$@"; fi; }
`

func runtimeUnitsScript(units []gitUnit, all bool, dir string) string {
	var b strings.Builder
	b.WriteString(gitRuntimePrelude)
	// One supervisor status call can list every worker, including numprocs groups.
	b.WriteString(`supervisors() {
 o=$(bounded supervisorctl status "$@" 2>&1)
 if [ "$(printf '%s\n' "$o" | wc -l)" -gt 256 ]; then record supervisor inventory unknown 'Worker limit reached (256)' ''; fi
 printf '%s\n' "$o" | head -n 256 | while IFS= read -r line; do
  name=${line%% *}; rest=${line#* }; rest=$(printf '%s' "$rest" | sed 's/^[[:space:]]*//'); status=${rest%% *}
  case "$status" in RUNNING) state=healthy;; STOPPED|STARTING|BACKOFF|STOPPING|EXITED|FATAL|UNKNOWN) state=failed;; *) record supervisor "$*" unknown "$o" ''; break;; esac
  case "$name" in ''|*[!a-zA-Z0-9_.:@+-]*) record supervisor inventory unknown 'Unsupported worker name; inspect with supervisorctl' "$line"; continue;; esac
  log=''
  if [ "$state" != healthy ]; then log=$(bounded supervisorctl tail "$name" stderr 2>&1 | tail -n 40); fi
  record supervisor "$name" "$state" "$rest" "$log"
 done
}
systemunit() {
 o=$(bounded systemctl is-active -- "$1" 2>&1); state=unknown
 case "$o" in active) state=healthy;; inactive|failed|activating|deactivating|reloading) state=failed;; esac
 log=''
 if [ "$state" != healthy ]; then
  detail=$(bounded systemctl show --no-pager -p LoadState -p SubState -p Result -p ExecMainStatus -p NRestarts -- "$1" 2>&1)
  log=$(bounded journalctl --no-pager -n 40 -u "$1" 2>&1)
  o="$o
$detail"
 fi
 record systemd "$1" "$state" "$o" "$log"
}
`)
	if all {
		b.WriteString(`if command -v supervisorctl >/dev/null 2>&1; then supervisors; fi
if command -v systemctl >/dev/null 2>&1; then
 o=$(bounded systemctl list-units --state=failed --type=service --no-legend --plain --no-pager 2>&1); rc=$?
 if [ "$rc" = 0 ]; then
  record systemd inventory healthy 'Failed systemd services listed' ''
  if [ "$(printf '%s\n' "$o" | wc -l)" -gt 64 ]; then record systemd inventory unknown 'Failed unit limit reached (64)' ''; fi
  printf '%s\n' "$o" | head -n 64 | while read -r name rest; do
   case "$name" in *.service) systemunit "$name";; esac
  done
 else record systemd system unknown "$o" ''; fi
fi
`)
	}
	for _, u := range units {
		if !gitRuntimeNameRe.MatchString(u.Name) || strings.Contains(u.Name, "*") {
			continue
		}
		if u.Kind == "systemd" {
			fmt.Fprintf(&b, "systemunit %s\n", shellQuote(u.Name))
		} else if u.Kind == "supervisor" {
			// A running singleton must not hide a failed sibling in a process group.
			fmt.Fprintf(&b, `o=$(bounded supervisorctl status %s 2>&1)
if printf '%%s' "$o" | grep -qE '^[^ ]+[[:space:]]+(RUNNING|STOPPED|STARTING|BACKOFF|STOPPING|EXITED|FATAL|UNKNOWN)'; then supervisors %s; else supervisors %s; fi
`, shellQuote(u.Name+":*"), shellQuote(u.Name+":*"), shellQuote(u.Name))
		}
	}
	if dir != "" {
		fmt.Fprintf(&b, `o=$(bounded df -Pk %s 2>&1); rc=$?
if [ "$rc" = 0 ]; then pct=$(printf '%%s\n' "$o" | awk 'NR==2 {gsub(/%%/,"",$5); print $5}'); record disk installation healthy "$pct" ''; else record disk installation unknown "$o" ''; fi
`, shellQuote(dir))
	}
	b.WriteString(`if [ -r /proc/meminfo ]; then
 pct=$(awk '/^MemTotal:/ {t=$2} /^MemAvailable:/ {a=$2} END {if(t>0 && a>0) printf "%.0f",100*(t-a)/t}' /proc/meminfo)
 if [ -n "$pct" ]; then record memory server healthy "$pct" ''; fi
fi
`)
	b.WriteString("echo WRM_RUNTIME_DONE\n")
	return b.String()
}

func parseRuntimeOutput(out string) ([]gitRuntimeCheck, error) {
	if !strings.HasSuffix(strings.TrimSpace(out), "WRM_RUNTIME_DONE") {
		return nil, fmt.Errorf("runtime check did not finish (timeout or output limit)")
	}
	checks := []gitRuntimeCheck{}
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		p := strings.Split(line, "\t")
		if len(p) != 6 || p[0] != "C" || len(checks) >= 256 {
			continue
		}
		if p[3] != "healthy" && p[3] != "failed" && p[3] != "unknown" {
			continue
		}
		key := p[1] + ":" + p[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		status, e1 := base64.StdEncoding.DecodeString(p[4])
		log, e2 := base64.StdEncoding.DecodeString(p[5])
		if e1 != nil || e2 != nil {
			return nil, fmt.Errorf("invalid runtime response")
		}
		c := gitRuntimeCheck{Kind: p[1], Name: p[2], State: p[3], Status: redactText(string(status)), Log: redactText(string(log))}
		if c.Kind == "disk" || c.Kind == "memory" {
			if n, e := strconv.Atoi(c.Status); e == nil && n >= 90 {
				c.State = "failed"
				c.Status = fmt.Sprintf("%d%% %s used", n, c.Kind)
			} else if e == nil {
				c.Status = fmt.Sprintf("%d%% %s used", n, c.Kind)
			} else {
				c.State = "unknown"
			}
		}
		if c.State == "unknown" && ((c.Kind == "supervisor" && strings.Contains(strings.ToLower(c.Status), "no such process")) || (c.Kind == "systemd" && strings.Contains(c.Status, "LoadState=not-found"))) {
			c.State = "failed"
		}
		checks = append(checks, c)
	}
	return checks, nil
}

func runtimeSummary(checks []gitRuntimeCheck) gitFunctionality {
	h := gitFunctionality{State: "healthy", CheckedAt: nowStamp(), Checks: checks, Warnings: []string{}}
	checked := false
	for _, c := range checks {
		if c.State == "failed" {
			h.State = "failed"
			h.Warnings = append(h.Warnings, c.Name+": "+c.Status)
		}
		if c.State == "unknown" {
			if h.State != "failed" {
				h.State = "unknown"
			}
			h.Warnings = append(h.Warnings, c.Name+": "+c.Status)
		}
		if c.Kind == "systemd" || c.Kind == "supervisor" || c.Kind == "http" || c.Kind == "tcp" {
			checked = true
		}
	}
	if !checked && h.State == "healthy" {
		h.State = "unknown"
		h.Warnings = append(h.Warnings, "No service manager or runtime endpoint could be verified.")
	}
	return h
}

func checkGitFunctionality(cl *ssh.Client, in gitInstall, app *gitCatalogApp, state string, d *gitInstallData) {
	units := append([]gitUnit{}, d.Units...)
	spec := gitFunctionalitySpec{}
	if app != nil && app.Functionality != nil {
		spec = *app.Functionality
		units = append(units, spec.Units...)
	}
	unitsCapped := len(units) > 64
	if unitsCapped {
		units = units[:64]
	}
	out, err := runRemoteScript(cl, runtimeUnitsScript(units, false, in.Path), "", 30*time.Second)
	checks := []gitRuntimeCheck{}
	if err == nil {
		checks, err = parseRuntimeOutput(out)
	}
	if err != nil {
		checks = append(checks, gitRuntimeCheck{Kind: "runtime", Name: "runtime", State: "unknown", Status: redactText(err.Error())})
	}
	if spec.HTTP != "" {
		checks = append(checks, checkRuntimeEndpoint(cl, "http", spec.HTTP))
	}
	if spec.TCP != "" {
		checks = append(checks, checkRuntimeEndpoint(cl, "tcp", spec.TCP))
	}
	for _, unit := range units {
		if (unit.Kind != "systemd" && unit.Kind != "supervisor") || !gitRuntimeNameRe.MatchString(unit.Name) || strings.Contains(unit.Name, "*") {
			checks = append(checks, gitRuntimeCheck{Kind: "runtime", Name: unit.Name, State: "unknown", Status: "Unsupported runtime unit name or kind; this unit was not checked"})
		}
	}
	if unitsCapped {
		checks = append(checks, gitRuntimeCheck{Kind: "runtime", Name: "unit-limit", State: "unknown", Status: "Runtime unit limit reached (64); additional units were not checked"})
	}
	h := runtimeSummary(checks)
	if len(units) == 0 && spec.HTTP == "" && spec.TCP == "" {
		h.Warnings = append(h.Warnings, "No runtime unit or endpoint is configured; functionality is unverified.")
	}
	switch state {
	case "gone", "error":
		h.State = "failed"
		h.Warnings = append(h.Warnings, d.Error)
	case "unknown":
		if h.State == "healthy" {
			h.State = "unknown"
		}
		h.Warnings = append(h.Warnings, d.Error)
	case "review", "update":
		if h.State == "healthy" {
			h.State = "warning"
		}
		h.Warnings = append(h.Warnings, fmt.Sprintf("Installation comparison: %s (%d missing, %d modified, %d old files)", state, d.Counts["missing"], d.Counts["modified"], d.Counts["old"]))
	}
	for _, f := range d.Files {
		if f.State == "protected" && f.Note == "missing" {
			if h.State == "healthy" {
				h.State = "warning"
			}
			h.Warnings = append(h.Warnings, "Protected configuration is missing: "+f.Path)
		}
	}
	if d.Capped {
		if h.State == "healthy" {
			h.State = "warning"
		}
		h.Warnings = append(h.Warnings, "Installation file limit reached; comparison is incomplete.")
	}
	d.Functionality = &h
}

func checkRuntimeEndpoint(cl *ssh.Client, kind, address string) gitRuntimeCheck {
	// The request originates on the selected server through SSH direct-tcpip. No redirects,
	// credentials or arbitrary shell commands; TLS is verified with the system trust store.
	c := gitRuntimeCheck{Kind: kind, Name: kind, State: "failed"}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return cl.DialContext(ctx, network, addr)
	}
	if kind == "tcp" {
		conn, err := dial(ctx, "tcp", address)
		if err != nil {
			c.Status = redactText(err.Error())
		} else {
			conn.Close()
			c.State = "healthy"
			c.Status = "TCP connection succeeded"
		}
		return c
	}
	tr := &http.Transport{DialContext: dial, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err == nil {
		var resp *http.Response
		resp, err = client.Do(req)
		if err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			c.Status = fmt.Sprintf("HTTP %d", resp.StatusCode)
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				c.State = "healthy"
			}
			return c
		}
	}
	c.Status = redactText(err.Error())
	return c
}

func initGitFunctionalitySchema() {
	mustExec(`CREATE TABLE IF NOT EXISTS git_server_health (user_id INTEGER NOT NULL, conn_id INTEGER NOT NULL, data TEXT NOT NULL DEFAULT '', PRIMARY KEY(user_id,conn_id))`)
}
func checkGitServerHealth(userID, connID int, cl *ssh.Client) {
	out, err := runRemoteScript(cl, runtimeUnitsScript(nil, true, "/"), "", 30*time.Second)
	checks := []gitRuntimeCheck{}
	if err == nil {
		checks, err = parseRuntimeOutput(out)
	}
	if err != nil {
		checks = append(checks, gitRuntimeCheck{Kind: "server", Name: "server", State: "unknown", Status: redactText(err.Error())})
	}
	h := runtimeSummary(checks)
	saveGitServerHealth(userID, connID, h)
}
func loadGitServerHealth(userID int) []gitServerHealth {
	out := []gitServerHealth{}
	rows, err := db.Query(`SELECT h.conn_id,c.name,h.data,COALESCE((SELECT hostname FROM git_hostnames WHERE conn_id=c.id),c.host),c.folder_id FROM git_server_health h JOIN connections c ON c.id=h.conn_id AND c.user_id=h.user_id WHERE h.user_id=? ORDER BY c.name`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var h gitServerHealth
		var data string
		var folder *int
		if rows.Scan(&h.ConnID, &h.Name, &data, &h.Hostname, &folder) == nil {
			h.Folder = folderPathOf(userID, folder)
			json.Unmarshal([]byte(data), &h.gitFunctionality)
			out = append(out, h)
		}
	}
	return out
}

// Configuration values are read on demand, never persisted with comparisons.
type gitConfigEntry struct {
	Section     string `json:"section"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Line        int    `json:"line,omitempty"`
	Secret      bool   `json:"secret"`
	Kind        string `json:"kind"`
}
type gitConfigFile struct {
	Path     string           `json:"path"`
	Entries  []gitConfigEntry `json:"entries"`
	Error    string           `json:"error,omitempty"`
	Unparsed int              `json:"unparsed,omitempty"`
}

var gitConfigAssignRe = regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][A-Za-z0-9_.-]*)\s*[:=]\s*(.*)$`)
var gitConfigFunctionRe = regexp.MustCompile(`^(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

var gitConfigSecretRe = regexp.MustCompile(`(?i)dsn|connection.?string|cookie|bearer|access.?key|account.?key`)

func configSecret(key string) bool {
	return tmplSecretRe.MatchString(key) || gitConfigSecretRe.MatchString(key)
}
func configEntry(section, key, value, description, kind string, line int) gitConfigEntry {
	e := gitConfigEntry{Section: section, Key: key, Value: value, Description: redactText(description), Kind: kind, Line: line}
	e.Secret = configSecret(section + "." + key)
	if e.Secret {
		e.Value = aiRedacted
		e.Description = ""
	} else {
		e.Value = redactText(value)
	}
	return e
}
func parseGitConfig(rel string, data []byte) gitConfigFile {
	f := gitConfigFile{Path: rel, Entries: []gitConfigEntry{}}
	if isBinary(data) {
		f.Error = "Binary configuration is not displayed"
		return f
	}
	if strings.HasSuffix(strings.ToLower(rel), ".json") {
		var value interface{}
		if err := json.Unmarshal(data, &value); err != nil {
			f.Error = "Invalid JSON configuration"
			return f
		}
		var walk func(string, interface{}, int)
		walk = func(key string, v interface{}, depth int) {
			if len(f.Entries) >= 2000 || depth > 32 {
				f.Error = "Configuration entry/depth limit reached; inspection is incomplete"
				f.Unparsed++
				return
			}
			if configSecret(key) {
				f.Entries = append(f.Entries, configEntry("", key, "", "", "setting", 0))
				return
			}
			switch x := v.(type) {
			case map[string]interface{}:
				keys := keysOf(x)
				sort.Strings(keys)
				for _, k := range keys {
					prefix := key
					if prefix != "" {
						prefix += "."
					}
					walk(prefix+k, x[k], depth+1)
				}
			case []interface{}:
				for i, y := range x {
					walk(fmt.Sprintf("%s[%d]", key, i), y, depth+1)
				}
			default:
				b, _ := json.Marshal(v)
				f.Entries = append(f.Entries, configEntry("", key, string(b), "", "setting", 0))
			}
		}
		walk("", value, 0)
		return f
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "PuTTY-User-Key-File-") {
		e := configEntry("", "private_key", "", "", "secret", 1)
		e.Secret = true
		e.Value = aiRedacted
		f.Entries = append(f.Entries, e)
		return f
	}
	section, description := "", ""
	multilineSecret := false
	privateBlock, secretQuote := false, ""
	for n, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if len(f.Entries) >= 2000 {
			f.Error = "Configuration entry limit reached (2000); inspection is incomplete"
			break
		}
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		line = strings.TrimSpace(line)
		if privateBlock {
			if strings.Contains(line, "-----END ") && strings.Contains(line, "PRIVATE KEY") {
				privateBlock = false
			}
			continue
		}
		if secretQuote != "" {
			if configQuoteClosed(line, secretQuote, 0) {
				secretQuote = ""
			}
			continue
		}
		if strings.Contains(line, "-----BEGIN ") && strings.Contains(line, "PRIVATE KEY") {
			key := "private_key"
			if m := gitConfigAssignRe.FindStringSubmatch(line); m != nil {
				key = m[1]
			}
			e := configEntry(section, key, "", "", "secret", n+1)
			e.Secret = true
			e.Value = aiRedacted
			f.Entries = append(f.Entries, e)
			privateBlock = !(strings.Contains(line, "-----END ") && strings.Contains(line, "PRIVATE KEY"))
			description = ""
			continue
		}
		// INI indented continuation lines belong to the preceding secret value.
		if multilineSecret && indented {
			continue
		}
		if line == "" {
			multilineSecret = false
			description = ""
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			description = truncateStr(description+" "+strings.TrimSpace(line[1:]), 1000)
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			multilineSecret = false
			description = ""
			continue
		}
		if m := gitConfigAssignRe.FindStringSubmatch(line); m != nil {
			e := configEntry(section, m[1], m[2], strings.TrimSpace(description), "setting", n+1)
			f.Entries = append(f.Entries, e)
			multilineSecret = e.Secret
			if e.Secret {
				v := strings.TrimSpace(m[2])
				for _, quote := range []string{strings.Repeat(string(rune(34)), 3), strings.Repeat(string(rune(39)), 3), string(rune(34)), string(rune(39))} {
					if strings.HasPrefix(v, quote) && !configQuoteClosed(v, quote, len(quote)) {
						secretQuote = quote
						break
					}
				}
			}
			description = ""
		} else if m := gitConfigFunctionRe.FindStringSubmatch(line); m != nil {
			f.Entries = append(f.Entries, configEntry(section, m[1], "defined", strings.TrimSpace(description), "function", n+1))
			description = ""
			multilineSecret = false
		} else {
			f.Unparsed++
			if !multilineSecret {
				description = ""
			}
		}
		if len(f.Entries) >= 2000 {
			f.Error = "Configuration entry limit reached (2000); inspection is incomplete"
			f.Unparsed++
			break
		}
	}
	if f.Error == "" && (privateBlock || secretQuote != "") {
		f.Error = "Unterminated secret block; remaining contents were masked"
	}
	return f
}

func configFileCandidate(rel string) bool {
	base := strings.ToLower(path.Base(rel))
	ext := path.Ext(base)
	return ext == ".ini" || ext == ".conf" || ext == ".cfg" || base == ".env" || strings.HasPrefix(base, ".env.") || ((strings.HasPrefix(base, "config") || strings.HasPrefix(base, "settings")) && (ext == ".json" || ext == ".py" || ext == ".yaml" || ext == ".yml" || ext == ""))
}
func inspectGitConfigs(userID int, in gitInstall) ([]gitConfigFile, error) {
	cl, err := dialInstall(userID, in)
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	paths := []string{}
	if a, ok := loadGitCatalog(userID).app(in.App); ok && a.Functionality != nil {
		paths = append(paths, a.Functionality.ConfigFiles...)
	}
	// Fresh listing also includes protected files omitted from the comparison.
	if len(paths) == 0 {
		script := fmt.Sprintf(`cd %s 2>/dev/null || exit 1
find . %s -o -type f -print 2>/dev/null | head -n %d
`, shellQuote(in.Path), findPrune(), gitMaxInstallFiles+1)
		out, err := runRemoteScript(cl, script, "", 15*time.Second)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(out, "\n") {
			rel := strings.TrimPrefix(line, "./")
			if configFileCandidate(rel) {
				paths = append(paths, rel)
			}
		}
	}
	paths = cleanList(paths, 10000)
	files := []gitConfigFile{}
	if len(paths) > 20 {
		files = append(files, gitConfigFile{Path: "", Error: "Configuration file limit reached (20); set explicit catalog paths to prioritize files.", Entries: []gitConfigEntry{}})
		paths = paths[:20]
	}
	for _, rel := range paths {
		file := gitConfigFile{Path: rel, Entries: []gitConfigEntry{}}
		sf, e := readServerFile(cl, in.Path, rel, 256<<10)
		switch {
		case e != nil:
			file.Error = redactText(e.Error())
		case sf.problem() != nil:
			file.Error = sf.problem().Error()
		case !sf.Exists:
			file.Error = "Configuration file is missing"
		case sf.TooBig:
			file.Error = "Configuration exceeds 256 KiB; contents are not displayed"
		default:
			file = parseGitConfig(rel, sf.Data)
		}
		files = append(files, file)
	}
	return files, nil
}

func notifyGitFunctionality(id int, h *gitFunctionality) {
	if h == nil {
		return
	}
	var uid int
	var app, dir, name, data string
	if db.QueryRow(`SELECT i.user_id,i.app,i.path,COALESCE(c.name,''),i.data FROM git_installs i LEFT JOIN connections c ON c.id=i.conn_id WHERE i.id=?`, id).Scan(&uid, &app, &dir, &name, &data) != nil {
		return
	}
	var old gitInstallData
	json.Unmarshal([]byte(data), &old)
	if old.Functionality != nil && old.Functionality.State == h.State && strings.Join(old.Functionality.Warnings, "\n") == strings.Join(h.Warnings, "\n") {
		return
	}
	event := "git.functionality_problem"
	if h.State == "healthy" {
		if old.Functionality == nil || old.Functionality.State == "healthy" {
			return
		}
		event = "git.functionality_recovered"
	}
	notifyUser(uid, event, func(lang string) (string, string) {
		v := map[string]string{"app": app, "server": name, "path": dir, "error": truncateStr(strings.Join(h.Warnings, "; "), 1000)}
		return notifyText(lang, event, v), notifyText(lang, event+".body", v)
	})
}

func saveGitServerHealthError(userID, connID int, msg string) {
	h := gitFunctionality{State: "failed", CheckedAt: nowStamp(), Checks: []gitRuntimeCheck{}, Warnings: []string{redactText(msg)}}
	saveGitServerHealth(userID, connID, h)
}

func saveGitServerHealth(userID, connID int, h gitFunctionality) {
	var data, name string
	db.QueryRow(`SELECT data FROM git_server_health WHERE user_id=? AND conn_id=?`, userID, connID).Scan(&data)
	var old gitFunctionality
	json.Unmarshal([]byte(data), &old)
	if (old.State != h.State || strings.Join(old.Warnings, "\n") != strings.Join(h.Warnings, "\n")) && !(old.State == "" && h.State == "healthy") {
		db.QueryRow(`SELECT name FROM connections WHERE id=? AND user_id=?`, connID, userID).Scan(&name)
		event := "git.server_problem"
		if h.State == "healthy" {
			event = "git.server_recovered"
		}
		notifyUser(userID, event, func(lang string) (string, string) {
			vars := map[string]string{"server": name, "error": truncateStr(strings.Join(h.Warnings, "; "), 1000)}
			return notifyText(lang, event, vars), notifyText(lang, event+".body", vars)
		})
	}
	db.Exec(`INSERT INTO git_server_health(user_id,conn_id,data) VALUES(?,?,?) ON CONFLICT(user_id,conn_id) DO UPDATE SET data=excluded.data`, userID, connID, string(jsonMarshal(h)))
}

// Match a closing delimiter without treating an escaped quote as the end of a secret.
func configQuoteClosed(s, quote string, start int) bool {
	for i := start; i <= len(s)-len(quote); i++ {
		if s[i] == 92 {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], quote) {
			return true
		}
	}
	return false
}
