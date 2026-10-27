// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

type monitorLogFile struct {
	Root     int    `json:"root"`
	Path     string `json:"path"`
	Service  string `json:"service"`
	Operator string `json:"operator"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	// Kind is "journal" for a systemd unit read through journalctl; empty means a
	// regular log file. Unit carries the journal unit name.
	Kind string `json:"kind,omitempty"`
	Unit string `json:"unit,omitempty"`
	// Main is the main folder the file lives under (a scan path, a configured
	// main folder or a Git workspace server root); Place is "logs" for a log
	// folder, "app" for an application or script folder and "journal".
	Main  string `json:"main,omitempty"`
	Place string `json:"place,omitempty"`
	// OpGroup links similar operators (nova, novatv); OpGuess marks an operator
	// guessed from repeated words in folder and service names.
	OpGroup string `json:"op_group,omitempty"`
	OpGuess bool   `json:"op_guess,omitempty"`
}
type monitorLogChunk struct {
	Text         string `json:"text"`
	Modified     int64  `json:"modified"`
	Rows         int    `json:"rows"`
	FirstTime    string `json:"first_time,omitempty"`
	LastTime     string `json:"last_time,omitempty"`
	Start        int64  `json:"start"`
	EventTime    string `json:"event_time,omitempty"`
	Offset       int64  `json:"offset"`
	Signature    string `json:"signature"`
	Size         int64  `json:"size"`
	Reset        bool   `json:"reset"`
	Truncated    bool   `json:"truncated"`
	UnknownTimes int    `json:"unknown_times"`
	Matches      int    `json:"matches"`
	Timezone     string `json:"timezone"`
	Note         string `json:"note"`
	// Kind is "journal" when the text came from journalctl rather than a file
	// read; byte offsets and signatures do not apply to journal sources.
	Kind string `json:"kind,omitempty"`
	Unit string `json:"unit,omitempty"`
}

// A systemd unit name may contain letters, digits, dot, underscore, at, colon
// and dash. Anything else is rejected before it can reach a command line.
var monitorJournalUnit = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:\-]{0,199}$`)

func openMonitorSFTP(ctx context.Context, c Connection) (*sftp.Client, func(), error) {
	cl, err := dialSSH(c, nil)
	if err != nil {
		return nil, nil, err
	}
	fs, err := sftp.NewClient(cl)
	if err != nil {
		cl.Close()
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { cl.Close() })
	return fs, func() { stop(); fs.Close(); cl.Close() }, nil
}

// SFTP REALPATH implementations differ in whether they resolve symlinks. Reject
// symlink components explicitly before opening any root or log file.
func sftpNoSymlinks(fs *sftp.Client, p string) error {
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("absolute path required")
	}
	current := "/"
	for _, part := range strings.Split(strings.TrimPrefix(path.Clean(p), "/"), "/") {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		info, err := fs.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path components are not allowed")
		}
	}
	return nil
}
func monitorSafePath(fs *sftp.Client, root, p string) (string, error) {
	if err := sftpNoSymlinks(fs, root); err != nil {
		return "", err
	}
	if err := sftpNoSymlinks(fs, p); err != nil {
		return "", err
	}
	realRoot, err := fs.RealPath(root)
	if err != nil {
		return "", fmt.Errorf("log root unavailable")
	}
	real, err := fs.RealPath(p)
	if err != nil {
		return "", fmt.Errorf("log path unavailable")
	}
	if realRoot == "/" || !(real == realRoot || strings.HasPrefix(real, strings.TrimSuffix(realRoot, "/")+"/")) || aiReadDenied(real) != "" {
		return "", fmt.Errorf("log path escapes configured root or is sensitive")
	}
	return real, nil
}
func monitorLogsHandler(w http.ResponseWriter, r *http.Request, c Connection, acc *connAccess, rest string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	cfg := loadMonitorConfig(c.ID)
	cfg.Roots = monitorAvailableRoots(c)
	discovery, discovered := monitorDiscoveryView(c)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	// SFTP is opened lazily: a journal-only source needs an SSH command, not a
	// file session, and listing files for an undiscovered connection should not
	// pay for a session it cannot use.
	var fs *sftp.Client
	var closeFS func()
	defer func() {
		if closeFS != nil {
			closeFS()
		}
	}()
	openFS := func() bool {
		if fs != nil {
			return true
		}
		var err error
		fs, closeFS, err = openMonitorSFTP(ctx, c)
		if err != nil {
			jsonError(w, "SSH/SFTP log access failed: "+err.Error(), 502)
			return false
		}
		return true
	}
	if rest == "logs" && discovered && discovery.Files != nil {
		jsonOK(w, map[string]interface{}{"files": discovery.Files, "main_folders": logMainFolders(c, loadLogSettings(c.ID)), "roots": discovery.Roots, "warnings": discovery.Warnings, "incomplete": false, "discovered": true, "discovered_at": discovery.At, "services": discovery.Services, "method": discovery.Method, "limits": map[string]int{"files": loadLogSettings(c.ID).MaxFiles}})
		return
	}
	if rest == "logs" {
		if !openFS() {
			return
		}
		logSet := loadLogSettings(c.ID)
		files := []monitorLogFile{}
		warnings := append([]string(nil), discovery.Warnings...)
		visited := 0
		seenFiles := map[string]bool{}
		capped := false
		type dir struct {
			name        string
			depth, root int
		}
		for root, x := range cfg.Roots {
			if x.Kind == "journal" {
				if monitorJournalUnit.MatchString(x.Unit) {
					files = append(files, monitorLogFile{Root: root, Path: "journal:" + x.Unit, Service: x.Service, Operator: x.Operator, Kind: "journal", Unit: x.Unit})
				}
				continue
			}
			real, err := monitorSafePath(fs, x.Path, x.Path)
			if err != nil {
				warnings = append(warnings, x.Path+": "+err.Error())
				continue
			}
			// A discovered service log folder is already scoped to logs, so accept
			// the usual log names plus .txt (the FTP mirror, for example, writes
			// daily .txt files). Broad roots such as /var/log keep the name filter.
			includeTxt := oneOf(strings.ToLower(path.Base(x.Path)), "log", "logs") &&
				(x.Service != "" || x.Operator != "") &&
				strings.TrimRight(x.Path, "/") != "/var/log"
			queue := []dir{{real, 0, root}}
			for len(queue) > 0 && len(files) < 2000 && visited < 10000 {
				d := queue[0]
				queue = queue[1:]
				if ctx.Err() != nil {
					warnings = append(warnings, "Discovery time limit reached")
					capped = true
					break
				}
				entries, err := fs.ReadDir(d.name)
				if err != nil {
					warnings = append(warnings, d.name+": not readable")
					continue
				}
				// A folder that already holds a *.log file is a service log folder:
				// its other files (Serilog "info", "warn", extensionless output) belong
				// to the same service and are shown too.
				hasLog := false
				for _, f := range entries {
					if !f.IsDir() && monitorLogName(f.Name()) {
						hasLog = true
						break
					}
				}
				for _, f := range entries {
					visited++
					if visited > 10000 || len(files) >= 2000 {
						capped = true
						break
					}
					p := path.Join(d.name, f.Name())
					lower := strings.ToLower(f.Name())
					if f.IsDir() {
						if strings.HasPrefix(f.Name(), ".") || oneOf(lower, "archive", "backups", "backup", "old", "node_modules", "venv", "__pycache__", "temp", "tmp") {
							continue
						}
						if d.depth < 8 {
							queue = append(queue, dir{p, d.depth + 1, root})
						} else {
							capped = true
						}
						continue
					}
					if !f.Mode().IsRegular() || aiReadDenied(p) != "" {
						continue
					}
					if !logFileAccepted(f.Name(), includeTxt || (hasLog && d.depth > 0) || monitorLogDirName(path.Base(d.name)), logSet) {
						continue
					}
					if oneOf(strings.ToLower(path.Ext(f.Name())), ".gz", ".bz2", ".xz", ".zip", ".zst") {
						continue
					}
					if seenFiles[p] {
						continue
					}
					seenFiles[p] = true
					// Files in a subfolder of a broad root belong to that folder;
					// label them so the service filter and column are useful
					// (/var/log/media-api/error.log -> media-api).
					service := x.Service
					if service == "" && d.depth > 0 {
						rel := strings.TrimPrefix(d.name, strings.TrimSuffix(real, "/")+"/")
						if i := strings.Index(rel, "/"); i > 0 {
							rel = rel[:i]
						}
						if rel != "" && rel != "." && !strings.Contains(rel, "..") {
							service = rel
						}
					}
					files = append(files, monitorLogFile{Root: root, Path: p, Service: service, Operator: x.Operator, Size: f.Size(), Modified: f.ModTime().Unix()})
				}
			}
			if len(queue) > 0 {
				capped = true
			}
		}
		mains := logMainFolders(c, logSet)
		logDecorate(files, mains, logSet.GuessOperators)
		jsonOK(w, map[string]interface{}{"files": files, "main_folders": mains, "roots": cfg.Roots, "warnings": warnings, "incomplete": capped, "discovered": discovered, "discovered_at": discovery.At, "services": discovery.Services, "limits": map[string]int{"files": 2000, "entries": 10000, "depth": 8}})
		return
	}
	root, err := strconv.Atoi(r.URL.Query().Get("root"))
	if rp := r.URL.Query().Get("root_path"); rp != "" {
		root = -1
		for i, x := range cfg.Roots {
			if x.Path == rp {
				root = i
				err = nil
				break
			}
		}
	}
	if err != nil || root < 0 || root >= len(cfg.Roots) {
		jsonError(w, "Choose a configured log root", 400)
		return
	}
	if cfg.Roots[root].Kind == "journal" {
		monitorJournalLogHandler(w, r, c, cfg.Roots[root])
		return
	}
	if !openFS() {
		return
	}
	p := r.URL.Query().Get("path")
	if strings.ContainsAny(p, "\x00\r\n") || len(p) > 2048 {
		jsonError(w, "Invalid log path", 400)
		return
	}
	real, err := monitorSafePath(fs, cfg.Roots[root].Path, p)
	if err != nil {
		jsonError(w, err.Error(), 403)
		return
	}
	if oneOf(strings.ToLower(path.Ext(real)), ".gz", ".bz2", ".xz", ".zip", ".zst") {
		jsonError(w, "Compressed logs need to be extracted before text/live viewing; use file download", 415)
		return
	}
	f, err := fs.Open(real)
	if err != nil {
		jsonError(w, "Log is not readable by this SSH account", 403)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		jsonError(w, "Log must be a regular file", 400)
		return
	}
	prefix := make([]byte, 256)
	n, _ := f.ReadAt(prefix, 0)
	sum := sha256.Sum256(prefix[:n])
	signature := hex.EncodeToString(sum[:])
	offset := -int64(1)
	if x := r.URL.Query().Get("offset"); x != "" {
		offset, err = strconv.ParseInt(x, 10, 64)
		if err != nil || offset < -1 {
			jsonError(w, "Invalid offset", 400)
			return
		}
	}
	oldSignature := r.URL.Query().Get("signature")
	previousSize, _ := strconv.ParseInt(r.URL.Query().Get("previous_size"), 10, 64)
	reset := offset > st.Size() || previousSize > st.Size()
	if oldSignature != "" && oldSignature != signature {
		changed := true
		parts := strings.SplitN(oldSignature, ":", 2)
		if len(parts) == 2 {
			oldN, _ := strconv.Atoi(parts[0])
			if oldN >= 0 && oldN <= n {
				oldSum := sha256.Sum256(prefix[:oldN])
				changed = hex.EncodeToString(oldSum[:]) != parts[1]
			}
		}
		reset = reset || changed
	}
	signature = fmt.Sprintf("%d:%s", n, signature)
	if reset {
		offset = -1
	}
	scan := r.URL.Query().Get("scan") == "1"
	capBytes := int64(64 << 10)
	if scan {
		capBytes = 8 << 20
		if offset < 0 {
			offset = 0
		}
	}
	if offset < 0 {
		capBytes = 256 << 10
		offset = max(int64(0), st.Size()-capBytes)
	}
	if offset > st.Size() {
		offset = st.Size()
	}
	b := make([]byte, int(min(capBytes, st.Size()-offset)))
	n, err = f.ReadAt(b, offset)
	if err != nil && err != io.EOF {
		jsonError(w, "Cannot read log chunk", 502)
		return
	}
	b = b[:n]
	if strings.ContainsRune(string(b), '\x00') {
		jsonError(w, "Binary logs cannot be displayed as text", 415)
		return
	}
	text := string(b)
	if scan {
		// Offsets count raw bytes, so a scan keeps the bytes as they are (invalid UTF-8,
		// such as a CP1250 letter, is replaced only in the returned lines). A segment
		// that does not reach the end of the file stops after its last complete line, so
		// the next segment starts at a line; without any newline, only a rune split at
		// the end is left for the next segment.
		if offset+int64(n) < st.Size() {
			if i := strings.LastIndexByte(text, '\n'); i >= 0 {
				text = text[:i+1]
			} else {
				text = trimSplitRune(text)
			}
		}
		n = len(text)
	} else {
		text = strings.ToValidUTF8(text, "�")
	}
	tz := r.URL.Query().Get("timezone")
	if tz == "" {
		tz = "UTC"
	}
	location, err := time.LoadLocation(tz)
	if err != nil {
		jsonError(w, "Unknown log timezone", 400)
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) > 200 {
		jsonError(w, "Search text too long", 400)
		return
	}
	from, err := monitorLogTimeBound(r.URL.Query().Get("from"), location)
	if err != nil {
		jsonError(w, "Invalid from date/time", 400)
		return
	}
	to, err := monitorLogTimeBound(r.URL.Query().Get("to"), location)
	if err != nil {
		jsonError(w, "Invalid to date/time", 400)
		return
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		jsonError(w, "From must not be after until", 400)
		return
	}
	chunk := monitorLogChunk{Modified: st.ModTime().Unix(), Start: offset, Offset: offset + int64(n), Signature: signature, Size: st.Size(), Reset: reset, Truncated: offset > 0 || offset+int64(n) < st.Size(), Timezone: tz, Note: "Bounded text chunk; timestamp-less lines are retained. Syslog dates use the current year in the selected timezone. Live offsets refer to raw bytes."}
	if q != "" || !from.IsZero() || !to.IsZero() {
		if scan {
			initialTime, _ := time.Parse(time.RFC3339, r.URL.Query().Get("event_time"))
			filtered, unknown, matches, consumed, lastTime := filterMonitorLogSegment(text, q, from, to, location, initialTime)
			chunk.Text = filtered
			chunk.UnknownTimes = unknown
			chunk.Matches = matches
			chunk.Offset = offset + int64(consumed)
			if !lastTime.IsZero() {
				chunk.EventTime = lastTime.Format(time.RFC3339)
			}
			if consumed < len(text) {
				chunk.Truncated = true
			}
		} else {
			chunk.Text, chunk.UnknownTimes, chunk.Matches = filterMonitorLog(text, q, from, to, location)
		}
	} else {
		if scan {
			filtered, unknown, matches, consumed, lastTime := filterMonitorLogSegment(text, q, from, to, location, time.Time{})
			chunk.Text = filtered
			chunk.UnknownTimes = unknown
			chunk.Matches = matches
			chunk.Offset = offset + int64(consumed)
			if !lastTime.IsZero() {
				chunk.EventTime = lastTime.Format(time.RFC3339)
			}
			if consumed < len(text) {
				chunk.Truncated = true
			}
		} else {
			chunk.Text = text
		}
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines > 0 {
		parts := strings.Split(chunk.Text, "\n")
		keep := min(lines, 2000)
		if len(parts) > keep {
			chunk.Text = strings.Join(parts[len(parts)-keep:], "\n")
			chunk.Truncated = true
		}
	}
	for _, line := range strings.Split(chunk.Text, "\n") {
		if strings.TrimSpace(line) != "" {
			chunk.Rows++
		}
	}
	if q == "" {
		chunk.Matches = chunk.Rows
	}
	initial, _ := time.Parse(time.RFC3339, r.URL.Query().Get("event_time"))
	if !scan {
		initial = time.Time{}
	}
	chunk.FirstTime, chunk.LastTime = monitorEventRange(text[:min(len(text), int(chunk.Offset-offset))], q, from, to, location, initial)

	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", `attachment; filename="log-excerpt.txt"`)
		io.WriteString(w, chunk.Text)
		return
	}
	jsonOK(w, chunk)
}

var monitorISOTime = regexp.MustCompile(`^\s*\[?(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)`)

// Other timestamp shapes seen in service logs, tried near the start of a line:
// 2026/10/22 14:03:01, 22.10.2026 14:03:01, [22/Oct/2026:14:03:01 +0200]
// (nginx/Apache) and ISO times after a level or a bracket.
var (
	monitorSlashTime = regexp.MustCompile(`^\s*\[?(\d{4})/(\d{2})/(\d{2})[ T](\d{2}):(\d{2}):(\d{2})`)
	monitorEUTime    = regexp.MustCompile(`^\s*\[?(\d{1,2})\.(\d{1,2})\.(\d{4})\.?[ T](\d{1,2}):(\d{2}):(\d{2})`)
	monitorCLFTime   = regexp.MustCompile(`\[(\d{2}/[A-Z][a-z]{2}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4})\]`)
	monitorISOInside = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)`)
)

func parseMonitorISO(v string, loc *time.Location) (time.Time, bool) {
	v = strings.Replace(strings.Replace(v, " ", "T", 1), ",", ".", 1)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700", "2006-01-02T15:04:05.999999999"} {
		if at, err := time.ParseInLocation(layout, v, loc); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}

func parseMonitorLogTime(line string, loc *time.Location) (time.Time, bool) {
	if m := monitorISOTime.FindStringSubmatch(line); len(m) > 1 {
		if at, ok := parseMonitorISO(m[1], loc); ok {
			return at, true
		}
	}
	num := func(m []string) []int {
		out := make([]int, len(m)-1)
		for i, v := range m[1:] {
			out[i], _ = strconv.Atoi(v)
		}
		return out
	}
	if m := monitorSlashTime.FindStringSubmatch(line); m != nil {
		n := num(m)
		return time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], 0, loc), true
	}
	if m := monitorEUTime.FindStringSubmatch(line); m != nil {
		n := num(m)
		if n[1] >= 1 && n[1] <= 12 && n[0] >= 1 && n[0] <= 31 {
			return time.Date(n[2], time.Month(n[1]), n[0], n[3], n[4], n[5], 0, loc), true
		}
	}
	if len(line) >= 15 {
		if at, err := time.ParseInLocation("2006 Jan _2 15:04:05", fmt.Sprintf("%d %s", time.Now().In(loc).Year(), line[:15]), loc); err == nil {
			return at, true
		}
	}
	head := line
	if len(head) > 160 {
		head = head[:160]
	}
	if m := monitorCLFTime.FindStringSubmatch(head); m != nil {
		if at, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[1]); err == nil {
			return at.In(loc), true
		}
	}
	if len(head) > 64 {
		head = head[:64]
	}
	if m := monitorISOInside.FindStringSubmatch(head); m != nil {
		if at, ok := parseMonitorISO(m[1], loc); ok {
			return at, true
		}
	}
	return time.Time{}, false
}
func monitorLogTimeBound(v string, loc *time.Location) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	for _, format := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if at, err := time.ParseInLocation(format, v, loc); err == nil {
			return at, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp")
}
func filterMonitorLog(text, q string, from, to time.Time, loc *time.Location) (string, int, int) {
	lines := strings.Split(text, "\n")
	out := []string{}
	unknown, matches := 0, 0
	var eventTime time.Time
	for _, line := range lines {
		at, ok := parseMonitorLogTime(line, loc)
		if ok {
			eventTime = at
		} else {
			unknown++
		}
		if !eventTime.IsZero() && ((!from.IsZero() && eventTime.Before(from)) || (!to.IsZero() && eventTime.After(to))) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(q)) {
			continue
		}
		if q != "" {
			matches += strings.Count(strings.ToLower(line), strings.ToLower(q))
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), unknown, matches
}

// trimSplitRune drops an incomplete UTF-8 sequence at the end of text (at most 3 bytes).
func trimSplitRune(text string) string {
	for i := len(text) - 1; i >= 0 && i >= len(text)-utf8.UTFMax; i-- {
		if utf8.RuneStart(text[i]) {
			if !utf8.FullRuneInString(text[i:]) {
				return text[:i]
			}
			break
		}
	}
	return text
}

// Search advances a bounded byte segment. Stop before an undisplayed matching
// line when the output cap is reached, so Next segment can resume at that line.
func filterMonitorLogSegment(text, q string, from, to time.Time, loc *time.Location, eventTime time.Time) (string, int, int, int, time.Time) {
	var out strings.Builder
	unknown, matches, consumed, rows := 0, 0, 0, 0
	for _, raw := range strings.SplitAfter(text, "\n") {
		line := strings.TrimSuffix(raw, "\n")
		at, known := parseMonitorLogTime(line, loc)
		nextTime := eventTime
		if known {
			nextTime = at
		}
		keep := nextTime.IsZero() || ((from.IsZero() || !nextTime.Before(from)) && (to.IsZero() || !nextTime.After(to)))
		if q != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(q)) {
			keep = false
		}
		if keep && (out.Len()+len(raw) > 256<<10 || rows >= 2000) {
			break
		}
		eventTime = nextTime
		consumed += len(raw)
		if !known {
			unknown++
		}
		if keep {
			out.WriteString(strings.ToValidUTF8(raw, "�"))
			rows++
			if q != "" {
				matches += strings.Count(strings.ToLower(line), strings.ToLower(q))
			}
		}
	}
	return out.String(), unknown, matches, consumed, eventTime
}

// monitorLogsDiscoverHandler runs the explicit "Find logs" action. It is the
// only place that scans the host; the file list endpoint reads the cached
// result so opening the tab never starts a scan on its own.
func monitorLogsDiscoverHandler(w http.ResponseWriter, r *http.Request, c Connection, acc *connAccess) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if acc.Share != nil {
		jsonError(w, "Only the owner can search for logs", 403)
		return
	}
	start := time.Now()
	res, err := logDiscover(r.Context(), c)
	if err != nil {
		jsonError(w, err.Error(), 502)
		return
	}
	auditFileOp(r, acc, c, "monitor.logs_discovered", c.Name, map[string]interface{}{"roots": len(res.Roots), "services": len(res.Services), "files": len(res.Files), "method": res.Method, "ms": time.Since(start).Milliseconds()})
	jsonOK(w, map[string]interface{}{"roots": res.Roots, "services": res.Services, "warnings": res.Warnings, "discovered": true, "discovered_at": res.At, "method": res.Method, "files": len(res.Files), "ms": time.Since(start).Milliseconds()})
}

// monitorJournalLogHandler reads a systemd unit through journalctl. The unit is
// matched against a strict name pattern, every other argument is generated here
// and timestamps are formatted from parsed values, so no request text reaches
// the remote shell.
func monitorJournalLogHandler(w http.ResponseWriter, r *http.Request, c Connection, root monitorLogRoot) {
	if !monitorJournalUnit.MatchString(root.Unit) {
		jsonError(w, "Invalid journal unit", 400)
		return
	}
	tz := r.URL.Query().Get("timezone")
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		jsonError(w, "Unknown log timezone", 400)
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) > 200 {
		jsonError(w, "Search text too long", 400)
		return
	}
	from, err := monitorLogTimeBound(r.URL.Query().Get("from"), loc)
	if err != nil {
		jsonError(w, "Invalid from date/time", 400)
		return
	}
	to, err := monitorLogTimeBound(r.URL.Query().Get("to"), loc)
	if err != nil {
		jsonError(w, "Invalid to date/time", 400)
		return
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		jsonError(w, "From must not be after until", 400)
		return
	}
	lower := from
	if since, err := time.Parse(time.RFC3339, r.URL.Query().Get("since")); err == nil && (lower.IsZero() || since.After(lower)) {
		lower = since.In(loc)
	}
	lines := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && v > 0 {
		lines = min(v, 2000)
	}
	args := []string{"journalctl", "--no-pager", "--output=short-iso", "--unit=" + root.Unit, "--lines=" + strconv.Itoa(lines)}
	if !lower.IsZero() {
		// @seconds is timezone-free, so a browser timezone can never shift the
		// window relative to the server's journal.
		args = append(args, "--since=@"+strconv.FormatInt(lower.Unix(), 10))
	}
	if !to.IsZero() {
		args = append(args, "--until=@"+strconv.FormatInt(to.Unix(), 10))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cl, err := dialSSH(c, nil)
	if err != nil {
		jsonError(w, "SSH log access failed: "+err.Error(), 502)
		return
	}
	defer cl.Close()
	raw, err := monitorSSHCommand(ctx, cl, strings.Join(args, " "))
	raw = strings.ToValidUTF8(raw, "�")
	if err != nil && strings.TrimSpace(raw) == "" {
		jsonError(w, "journalctl failed: "+truncateStr(redactText(err.Error()), 300), 502)
		return
	}
	kept := []string{}
	for _, line := range strings.Split(raw, "\n") {
		// journalctl prints "-- Logs begin at …" banners and rotation notes.
		if strings.HasPrefix(line, "-- ") || strings.ContainsRune(line, '\x00') {
			continue
		}
		kept = append(kept, line)
	}
	text := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	chunk := monitorLogChunk{Kind: "journal", Unit: root.Unit, Timezone: tz, Note: "systemd journal read with journalctl --output=short-iso. The last " + strconv.Itoa(lines) + " entries are read, then filtered; byte offsets and live signatures do not apply. A unit that writes to journald needs the SSH account to have journal read access."}
	chunk.Text = text
	if q != "" || !from.IsZero() || !to.IsZero() {
		var unknown, matches int
		chunk.Text, unknown, matches = filterMonitorLog(text, q, from, to, loc)
		chunk.UnknownTimes = unknown
		chunk.Matches = matches
	}
	for _, line := range strings.Split(chunk.Text, "\n") {
		if strings.TrimSpace(line) != "" {
			chunk.Rows++
		}
	}
	if q == "" {
		chunk.Matches = chunk.Rows
	}
	chunk.FirstTime, chunk.LastTime = monitorEventRange(chunk.Text, "", time.Time{}, time.Time{}, loc, time.Time{})
	chunk.EventTime = chunk.LastTime
	chunk.Signature = root.Unit + ":" + chunk.LastTime
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", `attachment; filename="journal-excerpt.txt"`)
		io.WriteString(w, chunk.Text)
		return
	}
	jsonOK(w, chunk)
}

// Matching continuation lines inherit their event timestamp even when the
// keyword appears only on the continuation, not on the timestamped header.
func monitorEventRange(text, q string, from, to time.Time, loc *time.Location, at time.Time) (string, string) {
	first, last := "", ""
	for _, line := range strings.Split(text, "\n") {
		if v, ok := parseMonitorLogTime(line, loc); ok {
			at = v
		}
		if at.IsZero() || strings.TrimSpace(line) == "" || (!from.IsZero() && at.Before(from)) || (!to.IsZero() && at.After(to)) || (q != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(q))) {
			continue
		}
		if first == "" {
			first = at.Format(time.RFC3339Nano)
		}
		last = at.Format(time.RFC3339Nano)
	}
	return first, last
}
