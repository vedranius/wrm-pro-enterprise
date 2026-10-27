// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ─── FILE SEARCH ──────────────────────────────────────
//
// GET /api/remote/search?id=&path=&q=&mode=name|content&max=&hidden=1&share_token=
//
// Streams NDJSON:
//   {"type":"match","path":"/etc/nginx/nginx.conf","name":"nginx.conf","size":123,"is_dir":false,"mtime":…}
//   {"type":"progress","scanned":1200,"dir":"/var/log"}
//   {"type":"done","count":12,"scanned":5000,"truncated":false,"elapsed_ms":800}
//   {"type":"error","error":"…"}
//
// Name mode walks the tree over SFTP/FTP (works everywhere). Content mode runs grep on
// the server over SSH (fast, needs a POSIX shell with grep).

const (
	searchDefaultMax = 500
	searchHardMax    = 5000
	searchMaxScanned = 1000000
	searchTimeout    = 3 * time.Minute
)

// Virtual / huge filesystems that make a name search from "/" useless or endless.
var searchSkipDirs = map[string]bool{"/proc": true, "/sys": true, "/dev": true, "/run": true, "/snap": true}

type ndjsonWriter struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	flusher http.Flusher
}

func newNDJSON(w http.ResponseWriter) *ndjsonWriter {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	f, _ := w.(http.Flusher)
	return &ndjsonWriter{w: w, flusher: f}
}

func (n *ndjsonWriter) send(v interface{}) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := json.NewEncoder(n.w).Encode(v); err != nil {
		return err
	}
	if n.flusher != nil {
		n.flusher.Flush()
	}
	return nil
}

// nameMatcher supports case-insensitive substring search, or glob patterns (*, ?, [ ]).
func nameMatcher(q string) func(string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if strings.ContainsAny(q, "*?[") {
		if _, err := path.Match(q, "x"); err == nil {
			return func(name string) bool {
				ok, _ := path.Match(q, strings.ToLower(name))
				return ok
			}
		}
	}
	return func(name string) bool { return strings.Contains(strings.ToLower(name), q) }
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func searchRemoteHandler(w http.ResponseWriter, r *http.Request) {
	c, _, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	if query == "" {
		jsonError(w, "Search text is required", 400)
		return
	}
	root := q.Get("path")
	if root == "" {
		root = "."
	}
	mode := q.Get("mode")
	if mode == "" {
		mode = "name"
	}
	max, _ := strconv.Atoi(q.Get("max"))
	if max <= 0 {
		max = searchDefaultMax
	}
	if max > searchHardMax {
		max = searchHardMax
	}
	includeHidden := q.Get("hidden") != "0"

	ctx, cancel := context.WithTimeout(r.Context(), searchTimeout)
	defer cancel()
	out := newNDJSON(w)
	start := time.Now()

	var (
		count, scanned int
		truncated      bool
		err            error
	)
	emit := func(p, name string, size int64, isDir bool, mtime int64) bool {
		if count >= max {
			truncated = true
			return false
		}
		count++
		return out.send(map[string]interface{}{"type": "match", "path": p, "name": name, "size": size, "is_dir": isDir, "mtime": mtime}) == nil
	}

	switch {
	case mode == "content" && isFTP(c):
		err = fmt.Errorf("content search is not available for FTP connections")
	case mode == "content":
		err = searchContent(ctx, c, root, query, max, includeHidden, emit, &truncated)
	case isFTP(c):
		err = withFTP(c, func(fc *ftp.ServerConn) error {
			return searchNamesFTP(ctx, fc, root, nameMatcher(query), includeHidden, emit, out, &scanned, &truncated)
		})
	default:
		err = withSFTP(c, false, func(sc *sftp.Client) error {
			return searchNamesSFTP(ctx, sc, root, nameMatcher(query), includeHidden, emit, out, &scanned, &truncated)
		})
	}
	if ctx.Err() == context.DeadlineExceeded {
		truncated = true
		err = nil
	}
	if err != nil && r.Context().Err() == nil {
		out.send(map[string]interface{}{"type": "error", "error": err.Error()})
	}
	out.send(map[string]interface{}{"type": "done", "count": count, "scanned": scanned, "truncated": truncated, "elapsed_ms": time.Since(start).Milliseconds()})
}

type matchEmitter func(p, name string, size int64, isDir bool, mtime int64) bool

func searchNamesSFTP(ctx context.Context, sc *sftp.Client, root string, match func(string) bool, hidden bool,
	emit matchEmitter, out *ndjsonWriter, scanned *int, truncated *bool) error {
	if root == "." || root == "~" {
		if rp, err := sc.RealPath("."); err == nil {
			root = rp
		}
	} else if rp, err := sc.RealPath(root); err == nil {
		root = rp
	}
	fromRoot := root == "/"
	queue := []string{root}
	lastProgress := time.Now()
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return nil
		}
		dir := queue[0]
		queue = queue[1:]
		entries, err := sc.ReadDir(dir)
		if err != nil {
			if isConnLostErr(err) {
				return err
			}
			continue // permission denied etc.
		}
		for _, e := range entries {
			*scanned++
			name := e.Name()
			if !hidden && strings.HasPrefix(name, ".") {
				continue
			}
			full := joinRemote(dir, name)
			if match(name) {
				if !emit(full, name, e.Size(), e.IsDir(), e.ModTime().Unix()) {
					return nil
				}
			}
			if e.IsDir() && !(fromRoot && searchSkipDirs[full]) {
				queue = append(queue, full)
			}
		}
		if *scanned > searchMaxScanned {
			*truncated = true
			return nil
		}
		if time.Since(lastProgress) > 400*time.Millisecond {
			lastProgress = time.Now()
			if out.send(map[string]interface{}{"type": "progress", "scanned": *scanned, "dir": dir}) != nil {
				return nil
			}
		}
	}
	return nil
}

func searchNamesFTP(ctx context.Context, fc *ftp.ServerConn, root string, match func(string) bool, hidden bool,
	emit matchEmitter, out *ndjsonWriter, scanned *int, truncated *bool) error {
	if root == "." || root == "" {
		if wd, err := fc.CurrentDir(); err == nil {
			root = wd
		}
	}
	walker := fc.Walk(root)
	lastProgress := time.Now()
	for walker.Next() {
		if ctx.Err() != nil {
			return nil
		}
		e := walker.Stat()
		*scanned++
		if !hidden && strings.HasPrefix(e.Name, ".") {
			if e.Type == ftp.EntryTypeFolder {
				walker.SkipDir()
			}
			continue
		}
		if match(e.Name) {
			if !emit(walker.Path(), e.Name, int64(e.Size), e.Type == ftp.EntryTypeFolder, e.Time.Unix()) {
				return nil
			}
		}
		if *scanned > searchMaxScanned {
			*truncated = true
			return nil
		}
		if time.Since(lastProgress) > 400*time.Millisecond {
			lastProgress = time.Now()
			out.send(map[string]interface{}{"type": "progress", "scanned": *scanned, "dir": path.Dir(walker.Path())})
		}
	}
	return walker.Err()
}

func searchContent(ctx context.Context, c Connection, root, query string, max int, hidden bool, emit matchEmitter, truncated *bool) error {
	return withSSHClient(c, func(client *ssh.Client) error {
		sess, err := client.NewSession()
		if err != nil {
			return err
		}
		defer sess.Close()
		dir := root
		if dir == "" || dir == "." || dir == "~" {
			dir = "."
		}
		excludes := "--exclude-dir=proc --exclude-dir=sys --exclude-dir=dev"
		if !hidden {
			excludes += " --exclude-dir='.*' --exclude='.*'"
		}
		// -r recursive, -I skip binaries, -l only file names, -i case-insensitive, -F fixed string.
		cmd := fmt.Sprintf("cd %s 2>/dev/null && grep -rIliF %s -e %s -- . 2>/dev/null | head -n %d; echo \"__WRM_PWD__$(pwd)\"",
			shellQuote(dir), excludes, shellQuote(query), max+1)
		stdout, err := sess.StdoutPipe()
		if err != nil {
			return err
		}
		if err := sess.Start(cmd); err != nil {
			return err
		}
		go func() {
			<-ctx.Done()
			sess.Signal(ssh.SIGTERM)
			sess.Close()
		}()
		var lines []string
		base := ""
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "__WRM_PWD__") {
				base = strings.TrimPrefix(line, "__WRM_PWD__")
				continue
			}
			lines = append(lines, line)
		}
		sess.Wait()
		if base == "" && len(lines) == 0 && ctx.Err() == nil {
			return fmt.Errorf("cannot search in %s (folder missing or no grep on server)", dir)
		}
		for i, l := range lines {
			if i >= max {
				*truncated = true
				break
			}
			rel := strings.TrimPrefix(l, "./")
			full := rel
			if base != "" {
				full = joinRemote(base, rel)
			}
			if !emit(full, path.Base(rel), -1, false, 0) {
				break
			}
		}
		return nil
	})
}
