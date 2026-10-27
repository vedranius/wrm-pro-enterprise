// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

// A discovery grants no remote privileges: every read still checks the current
// connection permission, sensitive paths and all symlink components. Nothing
// here executes a remote command; systemd units and container metadata are
// read as files.
type monitorRootCache struct {
	Owner    int
	Config   string
	Until    time.Time
	At       time.Time
	Roots    []monitorLogRoot
	Services []monitorServiceInfo
	Warnings []string
	// Files is the file list found by the SSH discovery (nil after an SFTP
	// discovery, which lists files per request).
	Files  []monitorLogFile
	Method string
}

// monitorServiceInfo explains what a service is and where its logs live, so a
// service with no readable file log (journal-only units) is still visible
// instead of silently missing from the file list.
type monitorServiceInfo struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"` // systemd | docker | supervisor | git | folder
	Logs   string   `json:"logs"` // file | journal | none
	Roots  []string `json:"roots,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Detail string   `json:"detail,omitempty"`
}

var monitorDiscoveredRoots sync.Map

func monitorRootConfig(id int) string {
	return logSettingsKey(id)
}

// monitorDiscoveryView returns the last on-demand discovery for this user and
// configuration. It never runs a scan: discovery is an explicit action.
func monitorDiscoveryView(c Connection) (monitorRootCache, bool) {
	v, ok := monitorDiscoveredRoots.Load(c.ID)
	if !ok {
		return monitorRootCache{}, false
	}
	x := v.(monitorRootCache)
	if x.Owner != c.UserID || x.Config != monitorRootConfig(c.ID) || !time.Now().Before(x.Until) {
		return monitorRootCache{}, false
	}
	return x, true
}

func monitorAvailableRoots(c Connection) []monitorLogRoot {
	if x, ok := monitorDiscoveryView(c); ok {
		return x.Roots
	}
	// Without a discovery only the configured roots are listed; scan paths
	// are searched only when the user asks for it (Find logs).
	return loadLogSettings(c.ID).Roots
}

func monitorLogName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, ".log") || strings.HasSuffix(n, ".out") || strings.HasSuffix(n, ".err") || oneOf(n, "syslog", "messages", "auth.log", "kern.log", "daemon.log", "debug", "access", "error", "stdout", "stderr")
}

// monitorLogDirName accepts the log folder names used in practice. Suffix
// matching is deliberately restricted to separators so a project called
// "catalog" or "dialog" is not mistaken for a log folder.
func monitorLogDirName(name string) bool {
	n := strings.ToLower(name)
	if n == "log" || n == "logs" || n == "var" {
		return true
	}
	for _, p := range []string{"log-", "logs-", "log_", "logs_"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	for _, s := range []string{"_log", "_logs", "-log", "-logs"} {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}

// monitorUnitField reads one systemd directive, including repeated keys such as
// StandardOutput=append:/x.
func monitorUnitField(conf, key string) []string {
	out := []string{}
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), key) {
			continue
		}
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

func monitorDiscoverRoots(ctx context.Context, fs *sftp.Client, c Connection) ([]monitorLogRoot, []monitorServiceInfo, []string) {
	cfg := loadLogSettings(c.ID)
	configKey := logSettingsKey(c.ID)
	cfg.Roots = monitorAvailableRootsFrom(cfg)
	roots := []monitorLogRoot{}
	services := []monitorServiceInfo{}
	seen := map[string]bool{}
	serviceIndex := map[string]int{}
	warnings := []string{}
	add := func(p, service, operator string) string {
		p = path.Clean(p)
		if len(roots) >= 200 || seen[p] || !strings.HasPrefix(p, "/") || p == "/" || aiReadDenied(p) != "" || strings.ContainsAny(p, "\x00\r\n") {
			return ""
		}
		// Candidate metadata is cheap. Full ancestor/REALPATH validation runs
		// before directory listing and again before every content read.
		st, err := fs.Lstat(p)
		if err != nil || !st.IsDir() {
			return ""
		}
		seen[p] = true
		roots = append(roots, monitorLogRoot{Path: p, Service: service, Operator: operator})
		return p
	}
	// addService records a service inventory row and links the roots that belong
	// to it. Journal-only services get an explicit, openable source instead of
	// disappearing from the list.
	addService := func(name, kind, logs, unit, detail string, paths ...string) {
		if name == "" {
			return
		}
		key := kind + "\x00" + name
		i, ok := serviceIndex[key]
		if !ok {
			i = len(services)
			serviceIndex[key] = i
			services = append(services, monitorServiceInfo{Name: name, Kind: kind, Logs: logs, Unit: unit, Detail: detail})
		}
		for _, p := range paths {
			if p == "" {
				continue
			}
			dup := false
			for _, x := range services[i].Roots {
				if x == p {
					dup = true
					break
				}
			}
			if !dup {
				services[i].Roots = append(services[i].Roots, p)
			}
		}
		if services[i].Logs == "none" && len(services[i].Roots) > 0 {
			services[i].Logs = "file"
		}
	}
	for _, r := range cfg.Roots {
		if r.Kind == "journal" {
			addService(r.Service, "systemd", "journal", r.Unit, "", "")
			continue
		}
		p := add(r.Path, r.Service, r.Operator)
		if p == "" {
			warnings = append(warnings, r.Path+": directory unavailable or not readable")
			continue
		}
		addService(r.Service, "folder", "file", "", "", p)
		// A broad root such as /var/log holds one folder per application. List
		// the folders that actually contain a *.log file so they show up as
		// services instead of only as unlabeled rows in the file list.
		if r.Service == "" && r.Operator == "" {
			if entries, err := fs.ReadDir(p); err == nil {
				subs := 0
				for _, e := range entries {
					if subs >= 60 || ctx.Err() != nil {
						break
					}
					if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
						continue
					}
					sub := path.Join(p, e.Name())
					filesIn, err := fs.ReadDir(sub)
					if err != nil {
						continue
					}
					hasLog := false
					for _, f := range filesIn {
						if !f.IsDir() && monitorLogName(f.Name()) {
							hasLog = true
							break
						}
					}
					if !hasLog {
						continue
					}
					subs++
					addService(e.Name(), "folder", "file", "", "", sub)
				}
			}
		}
	}
	// Known installations provide labels without requiring another expensive Git
	// comparison. Their small log directories are preferred over broad roots.
	profiles := loadGitConfigProfiles(c.UserID)
	for _, in := range loadGitInstalls(c.UserID, false, "i.conn_id=?", c.ID) {
		if ctx.Err() != nil {
			break
		}
		operator := in.Env
		for _, p := range profiles {
			if p.App == in.App && p.Operator != "" {
				operator = p.Operator
				break
			}
		}
		found := []string{}
		for _, suffix := range []string{"log", "logs"} {
			if p := add(path.Join(in.Path, suffix), in.App, operator); p != "" {
				found = append(found, p)
			}
		}
		addService(in.App, "git", "none", "", in.Path, found...)
	}
	// systemd units: file logs (append:/path) become roots; journal output
	// becomes an openable journal source. Only unit files are read.
	units := 0
	for _, dir := range []string{"/etc/systemd/system", "/usr/lib/systemd/system", "/lib/systemd/system"} {
		if ctx.Err() != nil || units >= 300 {
			break
		}
		if _, err := monitorSafePath(fs, dir, dir); err != nil {
			continue
		}
		entries, err := fs.ReadDir(dir)
		if err != nil {
			continue
		}
		for i, e := range entries {
			if i >= 800 || ctx.Err() != nil || units >= 300 {
				break
			}
			if !e.Mode().IsRegular() || !strings.HasSuffix(strings.ToLower(e.Name()), ".service") {
				continue
			}
			unit := strings.TrimSuffix(e.Name(), ".service")
			if strings.HasSuffix(unit, "@") || strings.HasPrefix(unit, "system-") {
				continue
			}
			p := path.Join(dir, e.Name())
			f, err := fs.Open(p)
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(io.LimitReader(f, 64<<10))
			f.Close()
			units++
			conf := string(b)
			found := []string{}
			journal := false
			for _, v := range append(monitorUnitField(conf, "StandardOutput"), monitorUnitField(conf, "StandardError")...) {
				v = strings.Trim(strings.TrimSpace(v), "\"'")
				kind, target, ok := strings.Cut(v, ":")
				if !ok {
					if oneOf(v, "journal", "journal-or-kmsg", "journal+console", "inherit", "null", "tty", "socket") {
						journal = true
					}
					continue
				}
				if oneOf(kind, "append", "file", "truncate") && strings.HasPrefix(target, "/") {
					if r := add(path.Dir(target), unit, ""); r != "" {
						found = append(found, r)
					}
				}
				if oneOf(kind, "journal", "journal-or-kmsg") {
					journal = true
				}
			}
			// A unit that keeps its files next to its working directory is common
			// for hand-deployed services and is cheap to probe.
			for _, wd := range monitorUnitField(conf, "WorkingDirectory") {
				wd = strings.Trim(strings.TrimSpace(wd), "\"'")
				if !strings.HasPrefix(wd, "/") || strings.Contains(wd, "%") {
					continue
				}
				for _, sub := range []string{"log", "logs", "var/log", "var/logs"} {
					if r := add(path.Join(wd, sub), unit, ""); r != "" {
						found = append(found, r)
					}
				}
			}
			detail := ""
			if len(found) == 0 {
				detail = strings.Join(monitorUnitField(conf, "ExecStart"), " ")
			}
			logs := "none"
			if len(found) > 0 {
				logs = "file"
			} else if journal {
				logs = "journal"
			}
			addService(unit, "systemd", logs, unitOrEmpty(logs, unit), truncateStr(detail, 200), found...)
		}
	}
	// Read only bounded Supervisor metadata; no commands or configurations run.
	for _, dir := range []string{"/etc/supervisor/conf.d", "/etc/supervisord.d"} {
		if ctx.Err() != nil {
			break
		}
		if _, err := monitorSafePath(fs, dir, dir); err != nil {
			continue
		}
		entries, err := fs.ReadDir(dir)
		if err != nil {
			continue
		}
		for i, e := range entries {
			if i >= 40 || ctx.Err() != nil {
				break
			}
			if !e.Mode().IsRegular() || !oneOf(strings.ToLower(path.Ext(e.Name())), ".conf", ".ini") {
				continue
			}
			p := path.Join(dir, e.Name())
			if _, err := monitorSafePath(fs, dir, p); err != nil {
				continue
			}
			f, err := fs.Open(p)
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(io.LimitReader(f, 64<<10))
			f.Close()
			service := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
			work := ""
			logs := []string{}
			flush := func() {
				found := []string{}
				for _, v := range logs {
					v = strings.ReplaceAll(v, "%(here)s", dir)
					if strings.Contains(v, "%(") {
						continue
					}
					if !path.IsAbs(v) && work != "" {
						v = path.Join(work, v)
					}
					if path.IsAbs(v) {
						if r := add(path.Dir(v), service, ""); r != "" {
							found = append(found, r)
						}
					}
				}
				addService(service, "supervisor", "none", "", "", found...)
				logs = nil
			}
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "[") {
					flush()
					work = ""
					if strings.HasPrefix(line, "[program:") {
						service = strings.TrimSuffix(strings.TrimPrefix(line, "[program:"), "]")
					}
					continue
				}
				k, v, ok := strings.Cut(line, "=")
				if !ok {
					continue
				}
				v = strings.Trim(strings.TrimSpace(v), "\"'")
				switch strings.TrimSpace(k) {
				case "directory":
					work = v
				case "stdout_logfile", "stderr_logfile":
					if v != "AUTO" && v != "NONE" {
						logs = append(logs, v)
					}
				}
			}
			flush()
		}
	}
	// Docker keeps a per-container metadata file next to its JSON log. Reading
	// it names the container without running docker or the daemon socket.
	containerDir := "/var/lib/docker/containers"
	if _, err := monitorSafePath(fs, containerDir, containerDir); err == nil {
		if entries, err := fs.ReadDir(containerDir); err == nil {
			for i, e := range entries {
				if i >= 200 || ctx.Err() != nil {
					break
				}
				if !e.IsDir() {
					continue
				}
				dir := path.Join(containerDir, e.Name())
				name, logPath := "", ""
				if f, err := fs.Open(path.Join(dir, "config.v2.json")); err == nil {
					b, _ := io.ReadAll(io.LimitReader(f, 512<<10))
					f.Close()
					var meta struct {
						Name    string `json:"Name"`
						LogPath string `json:"LogPath"`
					}
					if json.Unmarshal(b, &meta) == nil {
						name = strings.TrimPrefix(meta.Name, "/")
						logPath = meta.LogPath
					}
				}
				found := []string{}
				if strings.HasPrefix(logPath, "/") {
					if r := add(path.Dir(logPath), name, ""); r != "" {
						found = append(found, r)
					}
				}
				if len(found) == 0 {
					if r := add(dir, name, ""); r != "" {
						found = append(found, r)
					}
				}
				if name == "" {
					name = "container-" + e.Name()[:min(12, len(e.Name()))]
				}
				addService(name, "docker", "none", "", "docker json-file log", found...)
			}
		}
	}
	cat, _ := loadGitWorkspace(c.UserID)
	bases := []string{}
	for _, p := range cfg.ScanPaths {
		if p != "/var/log" {
			bases = append(bases, p)
		}
	}
	bases = append(bases, cat.ServerRoots...)
	// Bounded operator/service discovery, never a recursive /home scan. On the
	// deployment layout /home/<operator>/<operator>/<service>/log(s) the first level
	// is the operator and the second level is the service.
	type candidate struct {
		p, service, operator string
		depth                int
		skip                 bool // one container level under /home (project) carries no label
	}
	queue := []candidate{}
	baseSeen := map[string]bool{}
	for _, p := range bases {
		p = path.Clean(p)
		if p == "/" || baseSeen[p] || !strings.HasPrefix(p, "/") {
			continue
		}
		baseSeen[p] = true
		queue = append(queue, candidate{p: p, skip: p == "/home"})
	}
	visited := 0
	for len(queue) > 0 && visited < 400 && len(roots) < 200 && ctx.Err() == nil {
		d := queue[0]
		queue = queue[1:]
		visited++
		if d.depth == 0 {
			if _, err := monitorSafePath(fs, d.p, d.p); err != nil {
				continue
			}
		}
		entries, err := fs.ReadDir(d.p)
		if err != nil {
			continue
		}
		for i, e := range entries {
			if i >= 300 {
				break
			}
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			lower := strings.ToLower(n)
			if strings.HasPrefix(n, ".") || strings.HasSuffix(lower, ".egg-info") || oneOf(lower, "node_modules", "venv", "__pycache__", "backups", "build", "dist", "archive", "workingdir", "temp", "privkey") {
				continue
			}
			p := path.Join(d.p, n)
			if d.skip && d.depth == 0 {
				// /home/<project>/<operator>/<service>: the container level is not a label.
				queue = append(queue, candidate{p: p})
				continue
			}
			if monitorLogDirName(n) {
				if r := add(p, d.service, d.operator); r != "" {
					addService(effectiveServiceName(d.service, n), "folder", "file", "", "", r)
				}
			} else if d.depth < 2 {
				service, operator := d.service, d.operator
				if d.depth == 0 {
					operator = n
				} else {
					service = n
				}
				queue = append(queue, candidate{p, service, operator, d.depth + 1, false})
			}
		}
	}
	if ctx.Err() != nil {
		warnings = append(warnings, "Automatic folder discovery time budget reached; readable known roots are listed below")
	}
	if len(queue) > 0 || len(roots) >= 200 {
		warnings = append(warnings, "Automatic root discovery reached its directory/root limit")
	}
	sort.SliceStable(roots, func(i, j int) bool { return len(roots[i].Path) > len(roots[j].Path) })
	sort.SliceStable(services, func(i, j int) bool {
		if services[i].Kind != services[j].Kind {
			return services[i].Kind < services[j].Kind
		}
		return strings.ToLower(services[i].Name) < strings.ToLower(services[j].Name)
	})
	if monitorRootConfig(c.ID) == configKey {
		monitorDiscoveredRoots.Store(c.ID, monitorRootCache{Owner: c.UserID, Config: configKey, Until: time.Now().Add(time.Hour), At: time.Now(), Roots: roots, Services: services, Warnings: warnings, Method: "sftp"})
	} else {
		warnings = append(warnings, "Monitoring roots changed during discovery; find logs again")
	}
	return roots, services, warnings
}

// monitorAvailableRootsFrom lists the configured roots plus /var/log when it is
// a scan path, which the SFTP walk treats as a broad root.
func monitorAvailableRootsFrom(s logSettings) []monitorLogRoot {
	roots := append([]monitorLogRoot{}, s.Roots...)
	for _, p := range s.ScanPaths {
		if p == "/var/log" {
			roots = append(roots, monitorLogRoot{Path: p})
		}
	}
	return roots
}

func unitOrEmpty(logs, unit string) string {
	if logs == "journal" {
		return unit
	}
	return ""
}

// effectiveServiceName labels a plain log folder with the service folder it
// belongs to (…/<service>/logs) instead of an empty label.
func effectiveServiceName(service, dirName string) string {
	if service != "" {
		return service
	}
	return dirName
}
