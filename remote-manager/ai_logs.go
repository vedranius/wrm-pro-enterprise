// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// ─── AI: LOG TOOLS ───────────────────────────────────
//
// The assistant reads logs through WRM's own discovery instead of guessing
// paths: find_logs reports the services WRM found (the same inventory the
// Monitoring → Logs tab shows) and read_log opens one service's newest log, or
// its journal unit. Both run over the session's existing SSH connection, are
// evaluated by the normal read policy, land in the audit log and are redacted
// like every other read.

// aiLogCandidate is one log file a service writes.
type aiLogCandidate struct {
	Path     string
	Size     int64
	Modified int64
}

// aiLogSource returns the discovery for this connection, reusing the cache the
// Logs tab filled. Without a cache the connection owner gets one scan over this
// session's SSH connection; no second connection is opened and no shell command
// is run. A share never starts a scan of somebody else's server.
func (s *aiSession) aiLogSource(ctx context.Context) (monitorRootCache, error) {
	if x, ok := monitorDiscoveryView(s.conn); ok {
		return x, nil
	}
	if s.ShareID != 0 {
		return monitorRootCache{}, fmt.Errorf("logs have not been indexed for this connection yet; the connection owner must run Find logs first (or ask the assistant from that connection)")
	}
	cl, err := s.sshClient()
	if err != nil {
		return monitorRootCache{}, err
	}
	if set := loadLogSettings(s.conn.ID); set.Method != "sftp" {
		key := logSettingsKey(s.conn.ID)
		dctx, cancel := context.WithTimeout(ctx, time.Duration(set.TimeBudget)*time.Second)
		res, err := logFastDiscover(dctx, cl, s.conn, set)
		cancel()
		if err == nil {
			res.Config, res.Until = key, time.Now().Add(time.Hour)
			if logSettingsKey(s.conn.ID) == key {
				monitorDiscoveredRoots.Store(s.conn.ID, res)
			}
			return res, nil
		}
	}
	fs, err := sftp.NewClient(cl)
	if err != nil {
		return monitorRootCache{}, fmt.Errorf("SFTP log access failed: %v", err)
	}
	stop := context.AfterFunc(ctx, func() { fs.Close() })
	defer func() { stop(); fs.Close() }()
	monitorDiscoverRoots(ctx, fs, s.conn)
	x, ok := monitorDiscoveryView(s.conn)
	if !ok {
		return monitorRootCache{}, fmt.Errorf("log discovery did not complete")
	}
	return x, nil
}

// findLogs lists the services WRM knows about, with the folders or unit each one
// logs to. The model uses the names with read_log or the paths with tail_log.
func (s *aiSession) findLogs(ctx context.Context, in aiToolInput) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	x, err := s.aiLogSource(ctx)
	if err != nil {
		return "", 1, err
	}
	filter := strings.ToLower(strings.TrimSpace(in.Filter))
	if len(filter) > 200 || strings.ContainsAny(filter, "\x00\r\n") {
		return "", 1, fmt.Errorf("invalid filter")
	}
	limit := aiClamp(in.Limit, 100, 1, 300)
	var b strings.Builder
	age := "just now"
	if !x.At.IsZero() {
		age = time.Since(x.At).Round(time.Second).String() + " ago"
	}
	fmt.Fprintf(&b, "Log sources for %s (%s): %d services, %d log roots, indexed %s.\n", s.ConnName, s.Host, len(x.Services), len(x.Roots), age)
	shown := 0
	for _, svc := range x.Services {
		line := svc.Name + " (" + svc.Kind + ", " + svc.Logs + ")"
		if svc.Unit != "" {
			line += " unit=" + svc.Unit
		}
		if len(svc.Roots) > 0 {
			line += " " + strings.Join(svc.Roots, ", ")
		}
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			continue
		}
		if shown >= limit {
			b.WriteString(fmt.Sprintf("[stopped at %d services; raise limit or filter]\n", limit))
			break
		}
		b.WriteString("- " + truncateStr(line, 400) + "\n")
		shown++
	}
	if shown == 0 {
		b.WriteString("(no service matched)\n")
	}
	for i, w := range x.Warnings {
		if i >= 5 {
			break
		}
		b.WriteString("[warning] " + truncateStr(w, 300) + "\n")
	}
	fmt.Fprintf(&b, "[services shown: %d; call read_log with a service name, or tail_log with a path]\n", shown)
	return b.String(), 0, nil
}

// aiServiceLogs resolves a service name to its log files (newest first) or to
// its journal unit. Names may be given with or without the ".service" suffix.
func (s *aiSession) aiServiceLogs(ctx context.Context, name string) ([]aiLogCandidate, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	x, err := s.aiLogSource(ctx)
	if err != nil {
		return nil, "", err
	}
	want := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".service")
	match := -1
	for i, svc := range x.Services {
		if strings.TrimSuffix(strings.ToLower(svc.Name), ".service") != want {
			continue
		}
		if match < 0 || (x.Services[match].Logs == "none" && svc.Logs != "none") {
			match = i
		}
	}
	if match < 0 {
		return nil, "", fmt.Errorf("no service %q was found; call find_logs to see the known services", name)
	}
	svc := x.Services[match]
	if svc.Unit != "" && (svc.Logs == "journal" || len(svc.Roots) == 0) {
		if !monitorJournalUnit.MatchString(svc.Unit) {
			return nil, "", fmt.Errorf("invalid journal unit for service %q", name)
		}
		return nil, svc.Unit, nil
	}
	if x.Files != nil {
		// The SSH discovery already listed the files with their times.
		found := []aiLogCandidate{}
		for _, f := range x.Files {
			if f.Kind == "" && strings.EqualFold(f.Service, svc.Name) && aiReadDenied(f.Path) == "" {
				found = append(found, aiLogCandidate{Path: f.Path, Size: f.Size, Modified: f.Modified})
			}
		}
		if len(found) > 0 {
			sort.Slice(found, func(i, j int) bool {
				if found[i].Modified != found[j].Modified {
					return found[i].Modified > found[j].Modified
				}
				return found[i].Path < found[j].Path
			})
			if len(found) > 10 {
				found = found[:10]
			}
			return found, "", nil
		}
	}
	set := loadLogSettings(s.conn.ID)
	cl, err := s.sshClient()
	if err != nil {
		return nil, "", err
	}
	fs, err := sftp.NewClient(cl)
	if err != nil {
		return nil, "", fmt.Errorf("SFTP log access failed: %v", err)
	}
	stop := context.AfterFunc(ctx, func() { fs.Close() })
	defer func() { stop(); fs.Close() }()
	found := []aiLogCandidate{}
	for _, root := range svc.Roots {
		if ctx.Err() != nil {
			break
		}
		if err := sftpNoSymlinks(fs, root); err != nil {
			continue
		}
		real, err := fs.RealPath(root)
		if err != nil || aiReadDenied(real) != "" {
			continue
		}
		entries, err := fs.ReadDir(real)
		if err != nil {
			continue
		}
		// Same rule as the Logs tab: a folder that holds a *.log file is a log
		// folder, so its extensionless files count too.
		hasLog := false
		for _, f := range entries {
			if !f.IsDir() && monitorLogName(f.Name()) {
				hasLog = true
				break
			}
		}
		for _, f := range entries {
			p := path.Join(real, f.Name())
			if !f.Mode().IsRegular() || aiReadDenied(p) != "" {
				continue
			}
			if !logFileAccepted(f.Name(), hasLog || monitorLogDirName(path.Base(real)), set) {
				continue
			}
			if oneOf(strings.ToLower(path.Ext(f.Name())), ".gz", ".bz2", ".xz", ".zip", ".zst") {
				continue
			}
			found = append(found, aiLogCandidate{Path: p, Size: f.Size(), Modified: f.ModTime().Unix()})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Modified != found[j].Modified {
			return found[i].Modified > found[j].Modified
		}
		return found[i].Path < found[j].Path
	})
	if len(found) > 10 {
		found = found[:10]
	}
	return found, "", nil
}

// aiTailScript builds the read used by tail_log and read_log(path): the real
// path is printed first so a symlink to a sensitive file is caught before any
// content is shown, then the size, then the last lines (optionally filtered).
func aiTailScript(p string, lines int, grep string) string {
	body := `tail -n ` + strconv.Itoa(aiClamp(lines, 200, 1, 2000)) + ` -- "$r"`
	if grep != "" {
		body += ` | grep -F -- ` + shellQuote(grep) + ` || echo "(no line matched)"`
	}
	return `f=` + shellQuote(p) + `; r=$(readlink -f -- "$f" 2>/dev/null || printf %s "$f"); printf 'WRM-REAL %s\n' "$r"; ` +
		`[ -e "$r" ] || { echo "No such file: $f"; exit 2; }; [ -d "$r" ] && { echo "Is a directory: $f"; exit 2; }; ` +
		`printf 'WRM-SIZE %s\n' "$(wc -c < "$r" 2>/dev/null)"; ` + body
}
