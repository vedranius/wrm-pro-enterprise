// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// logSettings holds what log discovery looks at on one server. It is kept apart
// from the Zabbix monitoring profile so the two can be configured separately:
// metrics need an agent, logs only need the SSH account.
type logSettings struct {
	// Roots are explicit log folders or journal units; their labels win over
	// everything discovery guesses.
	Roots []monitorLogRoot `json:"roots"`
	// ScanPaths are the folders automatic discovery searches for log files.
	ScanPaths []string `json:"scan_paths"`
	// Exclude holds folder or file name patterns (shell globs) that are skipped.
	Exclude []string `json:"exclude"`
	// Patterns label files by their path: /home/*/{operator}/{service}/{logdir}/**.
	Patterns []string `json:"patterns"`
	// MainFolders are the tops of the folder tree in the log list (/home/skripte):
	// everything below one is grouped under it.
	MainFolders []string `json:"main_folders"`
	// GuessOperators links logs to operators from words that repeat across
	// service and folder names (nova, novatv).
	GuessOperators bool `json:"guess_operators"`
	// Extra holds additional file name patterns that count as logs (*.trace).
	Extra      []string `json:"extra"`
	MaxDepth   int      `json:"max_depth"`
	MaxFiles   int      `json:"max_files"`
	MaxAgeDays int      `json:"max_age_days"`
	TimeBudget int      `json:"time_budget"`
	Rotated    bool     `json:"rotated"`
	Compressed bool     `json:"compressed"`
	SkipEmpty  bool     `json:"skip_empty"`
	Journal    bool     `json:"journal"`
	Systemd    bool     `json:"systemd"`
	Docker     bool     `json:"docker"`
	Supervisor bool     `json:"supervisor"`
	// Method is auto (one SSH find command, SFTP when commands are not
	// available), ssh or sftp.
	Method   string `json:"method"`
	Timezone string `json:"timezone"`
}

var defaultLogScanPaths = []string{"/var/log", "/home", "/srv", "/opt"}

var defaultLogExclude = []string{"node_modules", ".git", ".svn", ".hg", "venv", ".venv", "__pycache__", "site-packages", "dist-packages", "vendor", ".cache", ".npm", ".cargo", ".rustup", ".m2", ".gradle", "*.egg-info"}

// Default label patterns, most specific first. {logdir} only matches a log
// folder name (log, logs, app-logs …), so /home/<a>/<b>/logs and
// /home/<a>/<b>/<c>/logs are told apart without naming any project.
var defaultLogPatterns = []string{
	"/home/*/{operator}/{service}/{logdir}/**",
	"/home/{operator}/{service}/{logdir}/**",
	"/home/{service}/{logdir}/**",
	"/srv/{operator}/{service}/{logdir}/**",
	"/srv/{service}/{logdir}/**",
	"/opt/{operator}/{service}/{logdir}/**",
	"/opt/{service}/{logdir}/**",
	"/var/log/{service}/**",
}

func defaultLogSettings() logSettings {
	return logSettings{Roots: []monitorLogRoot{}, ScanPaths: append([]string(nil), defaultLogScanPaths...), Exclude: append([]string(nil), defaultLogExclude...),
		Patterns: []string{}, MainFolders: []string{}, GuessOperators: true, Extra: []string{}, MaxDepth: 7, MaxFiles: 5000, TimeBudget: 45, Rotated: true, Journal: true, Systemd: true, Docker: true, Supervisor: true, Method: "auto"}
}

func initLogSettings() {
	ensureTable("log_settings", `CREATE TABLE IF NOT EXISTS log_settings(conn_id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL,config TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL DEFAULT '')`, "conn_id", "user_id", "config", "updated_at")
}

// loadLogSettings returns the server's log settings. A server configured before
// v13.5.0 keeps the log roots it had in its monitoring profile.
func loadLogSettings(id int) logSettings {
	s := defaultLogSettings()
	var raw string
	if db.QueryRow(`SELECT config FROM log_settings WHERE conn_id=?`, id).Scan(&raw) == nil {
		json.Unmarshal([]byte(decryptValue(raw)), &s)
		if s.Roots == nil {
			s.Roots = []monitorLogRoot{}
		}
		return s
	}
	for _, r := range loadMonitorConfig(id).Roots {
		// /var/log was the old default root; it is now a scan path.
		if r.Kind == "" && r.Path == "/var/log" && r.Service == "" && r.Operator == "" {
			continue
		}
		s.Roots = append(s.Roots, r)
	}
	return s
}

var reLogPatternToken = regexp.MustCompile(`^\{(service|operator|project|logdir)\}$`)

func (s *logSettings) normalize() error {
	if len(s.Roots) > 50 {
		return fmt.Errorf("at most 50 log roots")
	}
	for i := range s.Roots {
		x := &s.Roots[i]
		x.Service, x.Operator = strings.TrimSpace(x.Service), strings.TrimSpace(x.Operator)
		if len(x.Service) > 120 || len(x.Operator) > 120 || strings.ContainsAny(x.Service+x.Operator, "\x00\r\n") {
			return fmt.Errorf("invalid log root labels")
		}
		if x.Kind == "journal" {
			if !monitorJournalUnit.MatchString(x.Unit) {
				return fmt.Errorf("invalid journal unit name")
			}
			x.Path = "journal:" + x.Unit
			continue
		}
		x.Kind, x.Unit = "", ""
		x.Path = path.Clean(x.Path)
		if !logAbsDir(x.Path) {
			return fmt.Errorf("invalid log root %q; use an absolute folder other than /", x.Path)
		}
	}
	clean := func(list []string, max int, what string, check func(string) bool) ([]string, error) {
		out := []string{}
		seen := map[string]bool{}
		for _, v := range list {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			if len(out) >= max || len(v) > 512 || strings.ContainsAny(v, "\x00\r\n") || !check(v) {
				return nil, fmt.Errorf("invalid %s %q", what, v)
			}
			seen[v] = true
			out = append(out, v)
		}
		return out, nil
	}
	var err error
	if s.ScanPaths, err = clean(s.ScanPaths, 30, "scan path", func(v string) bool { return logAbsDir(path.Clean(v)) }); err != nil {
		return err
	}
	for i := range s.ScanPaths {
		s.ScanPaths[i] = path.Clean(s.ScanPaths[i])
	}
	if s.MainFolders, err = clean(s.MainFolders, 50, "main folder", func(v string) bool { return logAbsDir(path.Clean(v)) && !strings.ContainsAny(v, "*?[") }); err != nil {
		return err
	}
	for i := range s.MainFolders {
		s.MainFolders[i] = path.Clean(s.MainFolders[i])
	}
	if s.Exclude, err = clean(s.Exclude, 100, "exclude pattern", func(v string) bool { _, e := path.Match(v, "x"); return e == nil && !strings.Contains(v, "/") }); err != nil {
		return err
	}
	if s.Extra, err = clean(s.Extra, 50, "log name pattern", func(v string) bool { _, e := path.Match(v, "x"); return e == nil && !strings.Contains(v, "/") }); err != nil {
		return err
	}
	if s.Patterns, err = clean(s.Patterns, 50, "label pattern", func(v string) bool { _, e := parseLogPattern(v); return e == nil }); err != nil {
		return err
	}
	if s.MaxDepth == 0 {
		s.MaxDepth = 7
	}
	if s.MaxFiles == 0 {
		s.MaxFiles = 5000
	}
	if s.TimeBudget == 0 {
		s.TimeBudget = 45
	}
	if s.MaxDepth < 1 || s.MaxDepth > 20 || s.MaxFiles < 50 || s.MaxFiles > 20000 || s.TimeBudget < 5 || s.TimeBudget > 300 || s.MaxAgeDays < 0 || s.MaxAgeDays > 3650 {
		return fmt.Errorf("limits out of range (depth 1-20, files 50-20000, time 5-300 s, age 0-3650 days)")
	}
	if s.Method == "" {
		s.Method = "auto"
	}
	if !oneOf(s.Method, "auto", "ssh", "sftp") {
		return fmt.Errorf("discovery method is auto, ssh or sftp")
	}
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil || len(s.Timezone) > 64 {
			return fmt.Errorf("unknown timezone %q", s.Timezone)
		}
	}
	return nil
}

func logAbsDir(p string) bool {
	return strings.HasPrefix(p, "/") && p != "/" && len(p) <= 1024 && !strings.ContainsAny(p, "\x00\r\n")
}

// parseLogPattern splits a label pattern into path segments. Segments are
// literal names, shell globs (*, app-*), ** (any number of folders) or one of
// {service}, {operator}, {project} and {logdir}.
func parseLogPattern(p string) ([]string, error) {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") || len(p) > 512 {
		return nil, fmt.Errorf("a pattern starts with /")
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	labels := 0
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return nil, fmt.Errorf("empty or relative segment")
		}
		if strings.HasPrefix(s, "{") {
			if !reLogPatternToken.MatchString(s) {
				return nil, fmt.Errorf("unknown placeholder %s", s)
			}
			if s == "{service}" || s == "{operator}" {
				labels++
			}
			continue
		}
		if s != "**" {
			if _, err := path.Match(s, "x"); err != nil {
				return nil, err
			}
		}
	}
	if labels == 0 {
		return nil, fmt.Errorf("a pattern needs {service} or {operator}")
	}
	return segs, nil
}

// matchLogPattern matches a folder (split into segments) against a pattern and
// returns the captured labels.
func matchLogPattern(pat, segs []string) (map[string]string, bool) {
	var rec func(i, j int, got map[string]string) (map[string]string, bool)
	rec = func(i, j int, got map[string]string) (map[string]string, bool) {
		if i == len(pat) {
			return got, j == len(segs)
		}
		p := pat[i]
		if p == "**" {
			for k := j; k <= len(segs); k++ {
				if m, ok := rec(i+1, k, got); ok {
					return m, true
				}
			}
			return nil, false
		}
		if j >= len(segs) {
			return nil, false
		}
		s := segs[j]
		switch p {
		case "{logdir}":
			if !monitorLogDirName(s) {
				return nil, false
			}
		case "{service}", "{operator}", "{project}":
			if strings.HasPrefix(s, ".") || monitorLogDirName(s) {
				return nil, false
			}
			n := map[string]string{}
			for k, v := range got {
				n[k] = v
			}
			n[strings.Trim(p, "{}")] = s
			return rec(i+1, j+1, n)
		default:
			if ok, _ := path.Match(p, s); !ok {
				return nil, false
			}
		}
		return rec(i+1, j+1, got)
	}
	return rec(0, 0, map[string]string{})
}

// logLabeler turns a log file's folder into service and operator labels.
type logLabeler struct {
	pats [][]string
}

// newLogLabeler uses the server's own patterns first, then
// <base>/{operator}/{service}/<log folder> for every main folder and other base
// folder (scan paths and the Git workspace's server roots), then the defaults.
func newLogLabeler(s logSettings, bases ...string) logLabeler {
	l := logLabeler{}
	// The server's own folders come before the defaults, so a main folder such
	// as /home/skripte is not read as /home/{operator}.
	pats := append([]string{}, s.Patterns...)
	for _, b := range append(append([]string{}, s.MainFolders...), bases...) {
		b = path.Clean(b)
		if !logAbsDir(b) || strings.ContainsAny(b, "*?[]{}") || oneOf(b, "/home", "/srv", "/opt", "/var/log") {
			continue
		}
		pats = append(pats, b+"/{operator}/{service}/{logdir}/**", b+"/{service}/{logdir}/**")
	}
	pats = append(pats, defaultLogPatterns...)
	for _, p := range pats {
		if segs, err := parseLogPattern(p); err == nil {
			l.pats = append(l.pats, segs)
		}
	}
	return l
}

func (l logLabeler) label(dir string) (service, operator string) {
	segs := strings.Split(strings.Trim(path.Clean(dir), "/"), "/")
	for _, p := range l.pats {
		if m, ok := matchLogPattern(p, segs); ok {
			return m["service"], m["operator"]
		}
	}
	// A folder that is not a log folder name says what it holds
	// (/var/www/shop/var/log -> shop, /data/mirror -> mirror).
	for i := len(segs) - 1; i >= 0; i-- {
		if s := segs[i]; !monitorLogDirName(s) && !oneOf(s, "var", "data", "home", "srv", "opt", "storage", "app", "current") {
			return s, ""
		}
	}
	return "", ""
}

// Script, code, configuration and binary files never count as logs, even when
// they sit in a log folder (a cleanup.py next to its output).
var logDeniedExt = map[string]bool{}

func init() {
	for _, e := range strings.Fields(`.py .pyc .pyo .pyd .ipynb .js .mjs .cjs .ts .tsx .jsx .vue .svelte .sh .bash .zsh .fish .ksh .csh .ps1 .psm1 .bat .cmd .vbs
		.php .phtml .rb .erb .pl .pm .go .java .class .jar .war .ear .c .h .cc .cpp .hpp .cs .fs .rs .swift .kt .kts .scala .groovy .gradle .lua .r .m .dart .ex .exs .erl .hs .clj
		.html .htm .css .scss .sass .less .md .rst .adoc .tex .yml .yaml .toml .ini .cfg .conf .config .env .properties .xml .xsd .xsl .json .json5 .lock .pid .sock .socket
		.db .sqlite .sqlite3 .mdb .so .dll .dylib .exe .bin .o .a .lib .elf .img .iso .png .jpg .jpeg .gif .bmp .webp .svg .ico .pdf .doc .docx .xls .xlsx .ppt .pptx .odt
		.tar .7z .rar .deb .rpm .apk .whl .egg .map .woff .woff2 .ttf .otf .eot .crt .cer .pem .key .csr .p12 .pfx .jks .keystore .gpg .asc .sig .swp .swo .tmp .bak .orig .rej
		.journal .journal~ .mp3 .mp4 .mkv .avi .mov .wav .flac .ogg .webm .ts.map .patch .diff .service .timer .socket .mount .desktop .plist .reg .sql`) {
		logDeniedExt[e] = true
	}
}

var reLogNumExt = regexp.MustCompile(`^\.\d+$`)

// reLogWord finds "log" as a word of a file name (access_log, error-log.txt,
// app_logs) but not inside another word (ruby_logo, catalog).
var reLogWord = regexp.MustCompile(`(?i)(^|[._-])logs?([._-]|$)`)

// Binary login records live in /var/log but are not text logs.
var logBinaryNames = map[string]bool{"wtmp": true, "btmp": true, "utmp": true, "lastlog": true, "faillog": true, "tallylog": true, "wtmp.1": true, "btmp.1": true}

var logCompressedExt = []string{".gz", ".bz2", ".xz", ".zst", ".lz4", ".z"}

var reLogRotated = regexp.MustCompile(`(?i)(\.log|\.out|\.err|\.txt|_log|-log|^syslog|^messages|^secure|^maillog|^cron)[._-](\d+|\d{4}-?\d{2}-?\d{2}([._-]?\d+)?)$`)

// logFileAccepted decides whether a file is a log. inLogDir is true when the
// file sits in a folder that holds logs (a log folder name, or a folder with a
// *.log file), where extensionless files such as Serilog's "info" are logs too.
func logFileAccepted(name string, inLogDir bool, s logSettings) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, ".") {
		return false
	}
	for _, g := range s.Extra {
		if ok, _ := path.Match(strings.ToLower(g), n); ok {
			return true
		}
	}
	for _, g := range s.Exclude {
		if ok, _ := path.Match(strings.ToLower(g), n); ok {
			return false
		}
	}
	for _, ce := range logCompressedExt {
		if strings.HasSuffix(n, ce) {
			if !s.Compressed {
				return false
			}
			n = strings.TrimSuffix(n, ce)
			break
		}
	}
	if m := reLogRotated.FindStringSubmatch(n); m != nil {
		return s.Rotated
	}
	ext := path.Ext(n)
	if logDeniedExt[ext] && !(ext == ".json" && strings.Contains(n, "log")) {
		return false
	}
	if logBinaryNames[n] {
		return false
	}
	if monitorLogName(n) || reLogWord.MatchString(n) || oneOf(ext, ".out", ".err", ".trace", ".jsonl", ".ndjson", ".audit") {
		return true
	}
	if inLogDir {
		// Inside a log folder: extensionless names, dated or numbered files and
		// .txt are written by the service; anything with a code or config
		// extension was rejected above.
		return ext == "" || ext == ".txt" || ext == ".json" || reLogNumExt.MatchString(ext)
	}
	return false
}

// logSettingsHandler serves GET/PUT /api/monitoring/logs/settings?id=.
func logSettingsHandler(w http.ResponseWriter, r *http.Request, c Connection, acc *connAccess) {
	switch r.Method {
	case http.MethodGet:
		s := loadLogSettings(c.ID)
		jsonOK(w, map[string]interface{}{"settings": s, "defaults": map[string]interface{}{"scan_paths": defaultLogScanPaths, "exclude": defaultLogExclude, "patterns": defaultLogPatterns}, "can_edit": acc.Share == nil})
	case http.MethodPut:
		if acc.Share != nil {
			jsonError(w, "Only the owner can change log settings", 403)
			return
		}
		var s logSettings
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&s) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if err := s.normalize(); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if _, err := db.Exec(`INSERT INTO log_settings(conn_id,user_id,config,updated_at) VALUES(?,?,?,?) ON CONFLICT(conn_id) DO UPDATE SET config=excluded.config,updated_at=excluded.updated_at`, c.ID, c.UserID, encryptValue(string(jsonMarshal(s))), nowRFC()); err != nil {
			jsonError(w, "Cannot store log settings", 500)
			return
		}
		monitorDiscoveredRoots.Delete(c.ID)
		auditFileOp(r, acc, c, "monitor.log_settings", c.Name, map[string]interface{}{"roots": len(s.Roots), "scan_paths": len(s.ScanPaths), "patterns": len(s.Patterns), "method": s.Method})
		jsonOK(w, map[string]interface{}{"settings": s})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}
