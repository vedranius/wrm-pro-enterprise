// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Fast log discovery (v13.5.0): one read-only SSH command collects systemd,
// Supervisor and Docker metadata, a second one runs find over the scan paths.
// Every path that reaches the remote shell is quoted with shellQuote and comes
// from the server's log settings or from the metadata just read, never from the
// request. Servers without a POSIX shell, or SFTP-only accounts, fall back to
// the SFTP walk in monitorDiscoverRoots.

const logMetaScript = `LC_ALL=C; export LC_ALL
echo '##UNITS'
systemctl list-units --type=service --all --no-legend --plain --no-pager 2>/dev/null | head -n 3000
echo '##UNITFILES'
grep -H -s -E '^[[:space:]]*(StandardOutput|StandardError|WorkingDirectory|LogsDirectory)[[:space:]]*=' /etc/systemd/system/*.service /etc/systemd/system/*/*.service /usr/lib/systemd/system/*.service /lib/systemd/system/*.service 2>/dev/null | head -n 6000
echo '##SUPERVISOR'
grep -H -s -E '^[[:space:]]*(\[program:|directory|stdout_logfile|stderr_logfile)' /etc/supervisor/conf.d/*.conf /etc/supervisor/supervisord.conf /etc/supervisord.d/*.ini /etc/supervisord.d/*.conf /etc/supervisord.conf 2>/dev/null | head -n 3000
echo '##DOCKER'
for f in /var/lib/docker/containers/*/config.v2.json; do [ -r "$f" ] || continue; n=$(grep -o -E '"Name":"/[^"]*"' "$f" | head -n 1); l=$(grep -o -E '"LogPath":"[^"]*"' "$f" | head -n 1); printf '%s\t%s\t%s\n' "$f" "$n" "$l"; done 2>/dev/null | head -n 1000
echo '##END'
`

type logMeta struct {
	units      map[string]string   // unit -> active state (running, failed, exited, dead …)
	unitDirs   map[string][]string // unit -> log folders from StandardOutput/LogsDirectory/WorkingDirectory
	unitJrnl   map[string]bool     // unit writes to the journal (default unless redirected)
	supervisor map[string][]string // program -> log folders
	docker     map[string]string   // container name -> log folder
}

func parseLogMeta(out string) logMeta {
	m := logMeta{units: map[string]string{}, unitDirs: map[string][]string{}, unitJrnl: map[string]bool{}, supervisor: map[string][]string{}, docker: map[string]string{}}
	section := ""
	supProgram := map[string]string{}
	supDir := map[string]string{}
	addDir := func(dst map[string][]string, k, d string) {
		d = path.Clean(d)
		if !logAbsDir(d) || strings.Contains(d, "%") {
			return
		}
		for _, x := range dst[k] {
			if x == d {
				return
			}
		}
		dst[k] = append(dst[k], d)
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "##") {
			section = strings.TrimPrefix(line, "##")
			continue
		}
		switch section {
		case "UNITS":
			// unit load active sub description
			f := strings.Fields(line)
			if len(f) >= 4 && strings.HasSuffix(f[0], ".service") {
				m.units[strings.TrimSuffix(f[0], ".service")] = f[3]
			}
		case "UNITFILES":
			file, kv, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			unit := strings.TrimSuffix(path.Base(file), ".service")
			if strings.HasSuffix(unit, "@") {
				continue
			}
			k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), "\"'")
			switch k {
			case "StandardOutput", "StandardError":
				kind, target, has := strings.Cut(v, ":")
				if has && oneOf(kind, "append", "file", "truncate") && strings.HasPrefix(target, "/") {
					addDir(m.unitDirs, unit, path.Dir(target))
				} else if oneOf(kind, "journal", "journal+console", "journal-or-kmsg", "inherit", "") {
					m.unitJrnl[unit] = true
				}
			case "LogsDirectory":
				for _, d := range strings.Fields(v) {
					addDir(m.unitDirs, unit, path.Join("/var/log", d))
				}
			case "WorkingDirectory":
				v = strings.TrimPrefix(v, "-")
				if strings.HasPrefix(v, "/") {
					for _, sub := range []string{"log", "logs", "var/log"} {
						addDir(m.unitDirs, unit, path.Join(v, sub))
					}
				}
			}
		case "SUPERVISOR":
			file, kv, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			kv = strings.TrimSpace(kv)
			if strings.HasPrefix(kv, "[program:") {
				supProgram[file] = strings.TrimSuffix(strings.TrimPrefix(kv, "[program:"), "]")
				supDir[file] = ""
				continue
			}
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), "\"'")
			prog := supProgram[file]
			if prog == "" {
				prog = strings.TrimSuffix(path.Base(file), path.Ext(file))
			}
			switch k {
			case "directory":
				supDir[file] = v
			case "stdout_logfile", "stderr_logfile":
				if v == "AUTO" || v == "NONE" || strings.Contains(v, "%(") {
					continue
				}
				if !path.IsAbs(v) && supDir[file] != "" {
					v = path.Join(supDir[file], v)
				}
				if path.IsAbs(v) {
					addDir(m.supervisor, prog, path.Dir(v))
				}
			}
		case "DOCKER":
			parts := strings.Split(line, "\t")
			if len(parts) < 3 {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(parts[1], `"Name":"/`), `"`)
			lp := strings.TrimSuffix(strings.TrimPrefix(parts[2], `"LogPath":"`), `"`)
			if name == "" || strings.ContainsAny(name, "\"\\") {
				name = "container-" + path.Base(path.Dir(parts[0]))[:min(12, len(path.Base(path.Dir(parts[0]))))]
			}
			if strings.HasPrefix(lp, "/") {
				m.docker[name] = path.Dir(lp)
			} else {
				m.docker[name] = path.Dir(parts[0])
			}
		}
	}
	return m
}

// logFindScript builds the find command for the scan paths. Names come from
// the settings; each one is quoted. GNU find prints size, mtime and path with
// -printf; other finds (BusyBox) use stat.
func logFindScript(paths []string, s logSettings) string {
	q := func(v string) string { return shellQuote(v) }
	prune := []string{}
	for _, e := range s.Exclude {
		prune = append(prune, "-name "+q(e))
	}
	names := []string{"*log*", "*.out", "*.err", "*.trace", "*.jsonl", "*.ndjson", "*.audit", "messages*", "secure*", "maillog*", "cron*", "*access*", "*error*", "stdout*", "stderr*"}
	names = append(names, s.Extra...)
	match := []string{}
	for _, n := range names {
		match = append(match, "-iname "+q(n))
	}
	for _, d := range []string{"*/log/*", "*/logs/*", "*_log/*", "*_logs/*", "*-log/*", "*-logs/*", "*/log-*/*", "*/logs-*/*", "*/log_*/*", "*/logs_*/*"} {
		match = append(match, "-ipath "+q(d))
	}
	args := []string{}
	for _, p := range paths {
		args = append(args, q(p))
	}
	size := ""
	if s.SkipEmpty {
		size = " -size +0"
	}
	pruneExpr := ""
	if len(prune) > 0 {
		pruneExpr = `\( -type d \( ` + strings.Join(prune, " -o ") + ` \) -prune \) -o `
	}
	expr := pruneExpr + `\( -type f` + size + ` \( ` + strings.Join(match, " -o ") + ` \) `
	limit := strconv.Itoa(s.MaxFiles*4 + 100)
	depth := strconv.Itoa(s.MaxDepth)
	return "LC_ALL=C; export LC_ALL\n" +
		"if find / -maxdepth 0 -printf '' >/dev/null 2>&1; then\n" +
		"  find -P " + strings.Join(args, " ") + " -maxdepth " + depth + " " + expr + `-printf '%s\t%T@\t%p\n' \) 2>/dev/null | head -n ` + limit + "\n" +
		"else\n" +
		"  find " + strings.Join(args, " ") + " -maxdepth " + depth + " " + expr + `-exec stat -c '%s %Y %n' {} + \) 2>/dev/null | head -n ` + limit + "\n" +
		"fi\n"
}

// logListScript lists every regular file directly inside the given folders.
func logListScript(dirs []string, s logSettings) string {
	args := []string{}
	for _, d := range dirs {
		args = append(args, shellQuote(d))
	}
	size := ""
	if s.SkipEmpty {
		size = " -size +0"
	}
	limit := strconv.Itoa(s.MaxFiles*4 + 100)
	return "LC_ALL=C; export LC_ALL\n" +
		"if find / -maxdepth 0 -printf '' >/dev/null 2>&1; then\n" +
		"  find -P " + strings.Join(args, " ") + " -mindepth 1 -maxdepth 1 -type f" + size + ` -printf '%s\t%T@\t%p\n' 2>/dev/null | head -n ` + limit + "\n" +
		"else\n" +
		"  find " + strings.Join(args, " ") + " -mindepth 1 -maxdepth 1 -type f" + size + ` -exec stat -c '%s %Y %n' {} + 2>/dev/null | head -n ` + limit + "\n" +
		"fi\n"
}

type logFoundFile struct {
	path    string
	size    int64
	mtime   int64
	service string
	op      string
	kind    string // systemd | supervisor | docker | git | folder | root
}

// logFastDiscover runs the SSH discovery. It returns an error when commands
// cannot run on this server so the caller can fall back to SFTP.
func logFastDiscover(ctx context.Context, cl *ssh.Client, c Connection, s logSettings) (monitorRootCache, error) {
	out := monitorRootCache{Owner: c.UserID, At: time.Now(), Method: "ssh"}
	metaOut, err := monitorSSHCommand(ctx, cl, logMetaScript)
	if err != nil && !strings.Contains(metaOut, "##END") {
		return out, fmt.Errorf("remote commands unavailable: %v", err)
	}
	meta := parseLogMeta(metaOut)
	// Folders the metadata names are searched too, even outside the scan paths.
	dirLabel := map[string]logFoundFile{}
	extra := []string{}
	note := func(d, service, kind string) {
		d = path.Clean(d)
		// A unit that names a broad folder (WorkingDirectory=/var gives
		// /var/log) does not own every log in it.
		if !logAbsDir(d) || aiReadDenied(d) != "" || strings.Count(d, "/") < 3 && !strings.HasPrefix(d, "/var/log/") || oneOf(d, "/var/log", "/tmp/log", "/root/log", "/root/logs") {
			return
		}
		if _, ok := dirLabel[d]; !ok {
			dirLabel[d] = logFoundFile{service: service, kind: kind}
			extra = append(extra, d)
		}
	}
	if s.Systemd {
		for u, dirs := range meta.unitDirs {
			for _, d := range dirs {
				note(d, u, "systemd")
			}
		}
	}
	if s.Supervisor {
		for p, dirs := range meta.supervisor {
			for _, d := range dirs {
				note(d, p, "supervisor")
			}
		}
	}
	if s.Docker {
		for n, d := range meta.docker {
			note(d, n, "docker")
		}
	}
	profiles := loadGitConfigProfiles(c.UserID)
	for _, in := range loadGitInstalls(c.UserID, false, "i.conn_id=?", c.ID) {
		op := in.Env
		for _, p := range profiles {
			if p.App == in.App && p.Operator != "" {
				op = p.Operator
				break
			}
		}
		for _, sub := range []string{"log", "logs"} {
			d := path.Join(in.Path, sub)
			note(d, in.App, "git")
			x := dirLabel[d]
			x.op = op
			dirLabel[d] = x
		}
	}
	roots := map[string]monitorLogRoot{}
	for _, r := range s.Roots {
		if r.Kind == "journal" {
			continue
		}
		roots[r.Path] = r
	}
	cat, _ := loadGitWorkspace(c.UserID)
	bases := append(append([]string{}, s.ScanPaths...), cat.ServerRoots...)
	scan := []string{}
	seen := map[string]bool{}
	for _, p := range append(append(append([]string{}, bases...), func() []string {
		v := []string{}
		for p := range roots {
			v = append(v, p)
		}
		sort.Strings(v)
		return v
	}()...), extra...) {
		p = path.Clean(p)
		if seen[p] || !logAbsDir(p) || aiReadDenied(p) != "" {
			continue
		}
		// a folder inside another scan path is already covered
		covered := false
		for q := range seen {
			if strings.HasPrefix(p, strings.TrimSuffix(q, "/")+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		seen[p] = true
		scan = append(scan, p)
	}
	found := []logFoundFile{}
	if len(scan) > 0 {
		raw, err := monitorSSHCommand(ctx, cl, logFindScript(scan, s))
		if err != nil && strings.TrimSpace(raw) == "" {
			if ctx.Err() != nil {
				out.Warnings = append(out.Warnings, "Log discovery time budget reached; raise it in Log settings or narrow the scan paths")
			} else {
				return out, fmt.Errorf("find unavailable: %v", err)
			}
		}
		if ctx.Err() != nil && strings.TrimSpace(raw) != "" {
			out.Warnings = append(out.Warnings, "Log discovery time budget reached; the list may be incomplete")
		}
		for _, line := range strings.Split(raw, "\n") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) != 3 {
				parts = strings.SplitN(line, " ", 3) // stat fallback
			}
			if len(parts) != 3 || !strings.HasPrefix(parts[2], "/") {
				continue
			}
			size, _ := strconv.ParseInt(parts[0], 10, 64)
			mt, _ := strconv.ParseFloat(parts[1], 64)
			found = append(found, logFoundFile{path: path.Clean(parts[2]), size: size, mtime: int64(mt)})
		}
	}
	// A folder that already holds a *.log file is a log folder: its other files
	// (Serilog "info", dated .txt) are logs too.
	logDir := map[string]bool{}
	for _, f := range found {
		d := path.Dir(f.path)
		if monitorLogDirName(path.Base(d)) || monitorLogName(path.Base(f.path)) && strings.Contains(strings.ToLower(path.Base(f.path)), ".log") {
			logDir[d] = true
		}
	}
	for d := range dirLabel {
		logDir[d] = true
	}
	for p := range roots {
		logDir[p] = true
	}
	// Second pass: every regular file directly inside those log folders, so
	// names find did not match (Serilog "info", "warn") are listed too. The
	// scan paths themselves (/var/log) are not log folders of one service.
	isBase := map[string]bool{}
	for _, b := range bases {
		isBase[path.Clean(b)] = true
	}
	dirs := []string{}
	for d := range logDir {
		if !isBase[d] && logAbsDir(d) && aiReadDenied(d) == "" {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	if len(dirs) > 1500 {
		dirs = dirs[:1500]
	}
	if len(dirs) > 0 && ctx.Err() == nil {
		one := s
		one.MaxDepth = 1
		one.Exclude = nil
		raw, _ := monitorSSHCommand(ctx, cl, logListScript(dirs, one))
		known := map[string]bool{}
		for _, f := range found {
			known[f.path] = true
		}
		for _, line := range strings.Split(raw, "\n") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) != 3 {
				parts = strings.SplitN(line, " ", 3)
			}
			if len(parts) != 3 || !strings.HasPrefix(parts[2], "/") || known[path.Clean(parts[2])] {
				continue
			}
			size, _ := strconv.ParseInt(parts[0], 10, 64)
			mt, _ := strconv.ParseFloat(parts[1], 64)
			known[path.Clean(parts[2])] = true
			found = append(found, logFoundFile{path: path.Clean(parts[2]), size: size, mtime: int64(mt)})
		}
	}
	cut := int64(0)
	if s.MaxAgeDays > 0 {
		cut = time.Now().AddDate(0, 0, -s.MaxAgeDays).Unix()
	}
	labeler := newLogLabeler(s, bases...)
	kept := []logFoundFile{}
	dup := map[string]bool{}
	for _, f := range found {
		if dup[f.path] || aiReadDenied(f.path) != "" || (cut > 0 && f.mtime < cut) || (s.SkipEmpty && f.size == 0) {
			continue
		}
		d := path.Dir(f.path)
		inLog := logDir[d]
		if !inLog {
			for x := path.Dir(d); x != "/" && x != "."; x = path.Dir(x) {
				if monitorLogDirName(path.Base(x)) && path.Base(x) != "var" {
					inLog = true
					break
				}
			}
		}
		if !logFileAccepted(path.Base(f.path), inLog, s) {
			continue
		}
		dup[f.path] = true
		// Labels: an explicit root wins, then what the service metadata says,
		// then the path patterns, then the folder name.
		best := ""
		for p := range roots {
			if (d == p || strings.HasPrefix(d, strings.TrimSuffix(p, "/")+"/")) && len(p) > len(best) {
				best = p
			}
		}
		svc, op := labeler.label(d)
		f.kind = "folder"
		meta := ""
		for x := range dirLabel {
			if (d == x || strings.HasPrefix(d, x+"/")) && len(x) > len(meta) {
				meta = x
			}
		}
		if meta != "" {
			l := dirLabel[meta]
			svc, f.kind = l.service, l.kind
			if l.op != "" {
				op = l.op
			}
		}
		if best != "" {
			r := roots[best]
			if r.Service != "" {
				svc = r.Service
			}
			if r.Operator != "" {
				op = r.Operator
			}
		}
		f.service, f.op = svc, op
		kept = append(kept, f)
	}
	// Newest first so the cap keeps what is being written now.
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].mtime > kept[j].mtime })
	if len(kept) > s.MaxFiles {
		out.Warnings = append(out.Warnings, fmt.Sprintf("Showing the %d most recently written log files; raise the file limit in Log settings to see more", s.MaxFiles))
		kept = kept[:s.MaxFiles]
	}
	// One root per log folder, so reads stay confined to that folder.
	rootIdx := map[string]int{}
	services := map[string]*monitorServiceInfo{}
	addSvc := func(name, kind, logs, unit, root string) {
		if name == "" {
			return
		}
		k := kind + "\x00" + name
		x := services[k]
		if x == nil {
			x = &monitorServiceInfo{Name: name, Kind: kind, Logs: logs, Unit: unit}
			services[k] = x
		}
		if root != "" {
			for _, r := range x.Roots {
				if r == root {
					return
				}
			}
			x.Roots = append(x.Roots, root)
			x.Logs = "file"
		}
	}
	for _, f := range kept {
		d := path.Dir(f.path)
		i, ok := rootIdx[d]
		if !ok {
			i = len(out.Roots)
			rootIdx[d] = i
			out.Roots = append(out.Roots, monitorLogRoot{Path: d, Service: f.service, Operator: f.op})
		}
		out.Files = append(out.Files, monitorLogFile{Root: i, Path: f.path, Service: f.service, Operator: f.op, Size: f.size, Modified: f.mtime})
		addSvc(f.service, f.kind, "file", "", d)
	}
	// Explicit roots stay addressable even when they hold no file right now.
	for _, r := range s.Roots {
		if r.Kind == "journal" {
			continue
		}
		if _, ok := rootIdx[r.Path]; !ok {
			rootIdx[r.Path] = len(out.Roots)
			out.Roots = append(out.Roots, r)
			addSvc(r.Service, "folder", "none", "", "")
		}
	}
	// Journal sources: configured units, then running or failed units whose
	// output goes to the journal and that have no file log.
	journal := map[string]bool{}
	addJournal := func(unit, service, op string) {
		if journal[unit] || !monitorJournalUnit.MatchString(unit) {
			return
		}
		journal[unit] = true
		i := len(out.Roots)
		out.Roots = append(out.Roots, monitorLogRoot{Path: "journal:" + unit, Kind: "journal", Unit: unit, Service: nonEmpty(service, unit), Operator: op})
		out.Files = append(out.Files, monitorLogFile{Root: i, Path: "journal:" + unit, Service: nonEmpty(service, unit), Operator: op, Kind: "journal", Unit: unit})
		addSvc(nonEmpty(service, unit), "systemd", "journal", unit, "")
	}
	for _, r := range s.Roots {
		if r.Kind == "journal" {
			addJournal(r.Unit, r.Service, r.Operator)
		}
	}
	if s.Journal {
		units := []string{}
		for u, state := range meta.units {
			if oneOf(state, "running", "failed", "auto-restart", "activating") {
				units = append(units, u)
			}
		}
		sort.Strings(units)
		for _, u := range units {
			if len(meta.unitDirs[u]) > 0 && services["systemd\x00"+u] != nil && services["systemd\x00"+u].Logs == "file" {
				continue
			}
			addJournal(u, "", "")
		}
	}
	if s.Systemd {
		for u, dirs := range meta.unitDirs {
			if services["systemd\x00"+u] == nil && len(dirs) > 0 && meta.units[u] != "" {
				addSvc(u, "systemd", "none", "", "")
			}
		}
	}
	for _, x := range services {
		out.Services = append(out.Services, *x)
	}
	sort.SliceStable(out.Services, func(i, j int) bool {
		if out.Services[i].Kind != out.Services[j].Kind {
			return out.Services[i].Kind < out.Services[j].Kind
		}
		return strings.ToLower(out.Services[i].Name) < strings.ToLower(out.Services[j].Name)
	})
	if out.Files == nil {
		out.Files = []monitorLogFile{}
	}
	logDecorate(out.Files, logMainFolders(c, s), s.GuessOperators)
	return out, nil
}

// logDiscover runs discovery for a connection with its log settings: the SSH
// command path first, SFTP when commands are not available (or when chosen).
// The result is cached for an hour for the connection owner.
func logDiscover(ctx context.Context, c Connection) (monitorRootCache, error) {
	s := loadLogSettings(c.ID)
	key := logSettingsKey(c.ID)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.TimeBudget)*time.Second)
	defer cancel()
	var res monitorRootCache
	var fastErr error
	if s.Method != "sftp" {
		cl, err := dialSSH(c, nil)
		if err != nil {
			return res, fmt.Errorf("SSH log access failed: %v", err)
		}
		stop := context.AfterFunc(ctx, func() { cl.Close() })
		res, fastErr = logFastDiscover(ctx, cl, c, s)
		stop()
		cl.Close()
		if fastErr == nil {
			res.Config, res.Until = key, time.Now().Add(time.Hour)
			if logSettingsKey(c.ID) == key {
				monitorDiscoveredRoots.Store(c.ID, res)
			}
			return res, nil
		}
		if s.Method == "ssh" {
			return res, fastErr
		}
	}
	fs, closeFS, err := openMonitorSFTP(ctx, c)
	if err != nil {
		return res, fmt.Errorf("SSH/SFTP log access failed: %v", err)
	}
	defer closeFS()
	roots, services, warnings := monitorDiscoverRoots(ctx, fs, c)
	if fastErr != nil {
		warnings = append(warnings, "Commands are not available on this server, so logs were found over SFTP (slower)")
	}
	if x, ok := monitorDiscoveryView(c); ok {
		x.Warnings = warnings
		x.Method = "sftp"
		monitorDiscoveredRoots.Store(c.ID, x)
		return x, nil
	}
	return monitorRootCache{Owner: c.UserID, At: time.Now(), Roots: roots, Services: services, Warnings: warnings, Method: "sftp"}, nil
}

func logSettingsKey(id int) string {
	b, _ := json.Marshal(loadLogSettings(id))
	return string(b)
}
