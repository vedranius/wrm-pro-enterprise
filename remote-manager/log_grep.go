// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// logGrepRequest searches many log files at once on the server. grep runs
// remotely (one SSH command), so a week of logs is searched without copying it
// to WRM; the time range is applied here on the matched lines.
type logGrepRequest struct {
	Files    []string `json:"files"`
	Q        string   `json:"q"`
	Regex    bool     `json:"regex"`
	Case     bool     `json:"case"`
	Word     bool     `json:"word"`
	Context  int      `json:"context"`
	Max      int      `json:"max"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Timezone string   `json:"timezone"`
}

type logGrepHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
	Time    string `json:"time,omitempty"`
	Context bool   `json:"context,omitempty"`
}

func logGrepHandler(w http.ResponseWriter, r *http.Request, c Connection, acc *connAccess) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var in logGrepRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	in.Q = strings.TrimSpace(in.Q)
	if in.Q == "" || len(in.Q) > 500 || strings.ContainsAny(in.Q, "\x00\r\n") {
		jsonError(w, "Enter the text to search (up to 500 characters, one line)", 400)
		return
	}
	if in.Regex {
		if _, err := regexp.Compile(in.Q); err != nil {
			jsonError(w, "Invalid regular expression: "+err.Error(), 400)
			return
		}
	}
	if len(in.Files) == 0 || len(in.Files) > 2000 {
		jsonError(w, "Choose 1-2000 log files", 400)
		return
	}
	in.Context = max(0, min(in.Context, 5))
	if in.Max <= 0 {
		in.Max = 200
	}
	in.Max = min(in.Max, 2000)
	tz := nonEmpty(in.Timezone, "UTC")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		jsonError(w, "Unknown log timezone", 400)
		return
	}
	from, err1 := monitorLogTimeBound(in.From, loc)
	to, err2 := monitorLogTimeBound(in.To, loc)
	if err1 != nil || err2 != nil || (!from.IsZero() && !to.IsZero() && from.After(to)) {
		jsonError(w, "Invalid time range", 400)
		return
	}
	files, units, ok := logSearchable(c, in.Files)
	if !ok {
		jsonError(w, "Find logs first: the search uses the discovered file list", 409)
		return
	}
	if len(files)+len(units) == 0 {
		jsonError(w, "None of the chosen files is a discovered log", 400)
		return
	}
	flags := []string{"-H", "-n", "-I", "-s"}
	if !in.Regex {
		flags = append(flags, "-F")
	} else {
		flags = append(flags, "-E")
	}
	if !in.Case {
		flags = append(flags, "-i")
	}
	if in.Word {
		flags = append(flags, "-w")
	}
	if in.Context > 0 {
		flags = append(flags, "-C", strconv.Itoa(in.Context))
	}
	flags = append(flags, "-m", strconv.Itoa(in.Max))
	var sb strings.Builder
	sb.WriteString("LC_ALL=C; export LC_ALL\nIFS= read -r WRM_Q || exit 0\n")
	if len(files) > 0 {
		// The paths follow the pattern on standard input, one per line; links
		// that appeared since discovery are dropped.
		sb.WriteString("while IFS= read -r f; do [ -f \"$f\" ] && [ ! -h \"$f\" ] && printf '%s\\0' \"$f\"; done | xargs -0 grep " + strings.Join(flags, " ") + " -e \"$WRM_Q\" -- 2>/dev/null | head -n 20000\n")
	}
	for _, u := range units {
		since := "-n 50000"
		if !from.IsZero() {
			since = "--since=@" + strconv.FormatInt(from.Unix(), 10)
		}
		if !to.IsZero() {
			since += " --until=@" + strconv.FormatInt(to.Unix(), 10)
		}
		jflags := []string{}
		for _, f := range flags {
			if f != "-H" {
				jflags = append(jflags, f)
			}
		}
		sb.WriteString("journalctl --no-pager --output=short-iso --unit=" + u + " " + since + " 2>/dev/null | grep " + strings.Join(jflags, " ") + " -e \"$WRM_Q\" 2>/dev/null | head -n " + strconv.Itoa(in.Max*(1+2*in.Context)) + " | sed 's|^|journal:" + u + ":|'\n")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	cl, err := dialSSH(c, nil)
	if err != nil {
		jsonError(w, "SSH log access failed: "+err.Error(), 502)
		return
	}
	defer cl.Close()
	start := time.Now()
	input := in.Q + "\n"
	if len(files) > 0 {
		input += strings.Join(files, "\n") + "\n"
	}
	raw, err := monitorSSHCommandInput(ctx, cl, sb.String(), input)
	if err != nil && strings.TrimSpace(raw) == "" && ctx.Err() == nil {
		// grep exits 1 when nothing matched; that is not an error.
		raw = ""
	}
	raw = strings.ToValidUTF8(raw, "�")
	known := append(append([]string{}, files...), func() []string {
		v := []string{}
		for _, u := range units {
			v = append(v, "journal:"+u)
		}
		return v
	}()...)
	sort.Slice(known, func(i, j int) bool { return len(known[i]) > len(known[j]) })
	hits := []logGrepHit{}
	perFile := map[string]int{}
	truncated := ctx.Err() != nil || len(raw) >= 1<<20-1024
	for _, line := range strings.Split(raw, "\n") {
		if line == "" || line == "--" {
			continue
		}
		var h logGrepHit
		okLine := false
		for _, k := range known {
			rest, ok := strings.CutPrefix(line, k)
			if !ok || rest == "" || (rest[0] != ':' && rest[0] != '-') {
				continue
			}
			sep := rest[0]
			num, text, ok := strings.Cut(rest[1:], string(sep))
			n, err := strconv.Atoi(num)
			if !ok || err != nil {
				continue
			}
			h = logGrepHit{Path: k, Line: n, Text: truncateStr(text, 4000), Context: sep == '-'}
			okLine = true
			break
		}
		if !okLine {
			continue
		}
		if at, ok := parseMonitorLogTime(h.Text, loc); ok {
			if (!from.IsZero() && at.Before(from)) || (!to.IsZero() && at.After(to)) {
				continue
			}
			h.Time = at.Format(time.RFC3339)
		}
		if !h.Context {
			perFile[h.Path]++
		}
		hits = append(hits, h)
		if len(hits) >= 20000 {
			truncated = true
			break
		}
	}
	auditFileOp(r, acc, c, "monitor.logs_searched", c.Name, map[string]interface{}{"files": len(files), "units": len(units), "matches": len(hits), "regex": in.Regex})
	jsonOK(w, map[string]interface{}{"hits": hits, "per_file": perFile, "truncated": truncated, "ms": time.Since(start).Milliseconds(), "timezone": tz})
}

// logSearchable keeps the requested paths that this server's discovery listed
// (or, after an SFTP discovery, paths inside a configured root) and the
// journal units it found. ok is false when nothing was discovered yet.
func logSearchable(c Connection, req []string) (files, units []string, ok bool) {
	view, ok := monitorDiscoveryView(c)
	if !ok {
		return nil, nil, false
	}
	allowed := map[string]bool{}
	journal := map[string]bool{}
	for _, f := range view.Files {
		if f.Kind == "journal" {
			journal[f.Unit] = true
		} else {
			allowed[f.Path] = true
		}
	}
	for _, rt := range view.Roots {
		if rt.Kind == "journal" {
			journal[rt.Unit] = true
		}
	}
	files, units = []string{}, []string{}
	for _, p := range req {
		if u, ok := strings.CutPrefix(p, "journal:"); ok {
			if journal[u] && monitorJournalUnit.MatchString(u) {
				units = append(units, u)
			}
			continue
		}
		p = path.Clean(p)
		if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\x00\r\n") || aiReadDenied(p) != "" {
			continue
		}
		if view.Files == nil {
			inRoot := false
			for _, rt := range view.Roots {
				if rt.Kind == "" && strings.HasPrefix(p, strings.TrimSuffix(rt.Path, "/")+"/") {
					inRoot = true
					break
				}
			}
			if !inRoot {
				continue
			}
		} else if !allowed[p] {
			continue
		}
		files = append(files, p)
	}
	return files, units, true
}

// logLinesHandler serves the log viewer (GET ?id=&path=): numbered lines of a
// discovered log file or journal unit, read on the server with sed/tail/
// journalctl. Modes:
//   - mode=tail&n=N: the last N lines (default);
//   - mode=around&line=L&before=B&after=A: lines around one line (search hits);
//   - mode=since&bytes=B (files) or mode=since&cursor=C (journal): what was
//     appended after an earlier read, for follow. reset=true means the file
//     shrank (rotated/truncated) and the viewer starts again from the tail.
func logLinesHandler(w http.ResponseWriter, r *http.Request, c Connection, acc *connAccess) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	files, units, ok := logSearchable(c, []string{q.Get("path")})
	if !ok {
		jsonError(w, "Find logs first: the viewer uses the discovered file list", 409)
		return
	}
	if len(files)+len(units) != 1 {
		jsonError(w, "Not a discovered log file", 400)
		return
	}
	num := func(k string, def, lo, hi int) int {
		v, err := strconv.Atoi(q.Get(k))
		if err != nil {
			v = def
		}
		return max(lo, min(v, hi))
	}
	mode := nonEmpty(q.Get("mode"), "tail")
	if mode != "tail" && mode != "around" && mode != "since" {
		jsonError(w, "Unknown mode", 400)
		return
	}
	n := num("n", 1000, 1, 20000)
	const capBytes = 900000
	var cmd string
	journal := len(units) == 1
	if journal {
		u := units[0]
		cmd = "journalctl --no-pager --output=short-iso --show-cursor --unit=" + u + " "
		switch mode {
		case "since":
			cur := q.Get("cursor")
			if !logJournalCursor.MatchString(cur) {
				jsonError(w, "Bad journal cursor", 400)
				return
			}
			cmd += "--after-cursor=" + shellQuote(cur) + " -n 20000"
		default:
			cmd += "-n " + strconv.Itoa(n)
		}
		cmd += " 2>/dev/null | tail -c " + strconv.Itoa(capBytes) + "\n"
	} else {
		// The path comes on standard input, so it is never part of the command.
		f := "IFS= read -r f || exit 0\n" +
			"find -P \"$f\" -maxdepth 0 -type f 2>/dev/null | grep -q . || { echo WRM-NOFILE; exit 0; }\n" +
			"size=$(wc -c < \"$f\" 2>/dev/null | tr -d ' ')\n"
		switch mode {
		case "tail":
			cmd = f + "total=$(head -c \"$size\" \"$f\" 2>/dev/null | wc -l | tr -d ' ')\n" +
				"printf 'WRM-HEAD %s %s\\n' \"$size\" \"$total\"\n" +
				"head -c \"$size\" \"$f\" 2>/dev/null | tail -n " + strconv.Itoa(n) + " | tail -c " + strconv.Itoa(capBytes) + "\n"
		case "around":
			line := num("line", 1, 1, 1<<30)
			first := max(1, line-num("before", 200, 0, 5000))
			last := line + num("after", 300, 0, 5000)
			cmd = f + "total=$(head -c \"$size\" \"$f\" 2>/dev/null | wc -l | tr -d ' ')\n" +
				"printf 'WRM-HEAD %s %s\\n' \"$size\" \"$total\"\n" +
				"sed -n '" + strconv.Itoa(first) + "," + strconv.Itoa(last) + "p;" + strconv.Itoa(last+1) + "q' \"$f\" 2>/dev/null | head -c " + strconv.Itoa(capBytes) + "\n"
		case "since":
			b, err := strconv.ParseInt(q.Get("bytes"), 10, 64)
			if err != nil || b < 0 {
				jsonError(w, "Bad byte offset", 400)
				return
			}
			bs := strconv.FormatInt(b, 10)
			cmd = f + "printf 'WRM-HEAD %s 0\\n' \"$size\"\n" +
				"[ \"$size\" -ge " + bs + " ] 2>/dev/null || { echo WRM-RESET; exit 0; }\n" +
				"tail -c +" + strconv.FormatInt(b+1, 10) + " \"$f\" 2>/dev/null | head -c " + strconv.Itoa(capBytes) + "\n"
		}
		cmd = "LC_ALL=C; export LC_ALL\n" + cmd
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	cl, err := dialSSH(c, nil)
	if err != nil {
		jsonError(w, "SSH log access failed: "+err.Error(), 502)
		return
	}
	defer cl.Close()
	input := ""
	if !journal {
		input = files[0] + "\n"
	}
	raw, err := monitorSSHCommandInput(ctx, cl, cmd, input)
	if err != nil && raw == "" {
		jsonError(w, "Reading the log failed: "+err.Error(), 502)
		return
	}
	out := map[string]interface{}{"path": nonEmpty(strings.Join(files, ""), "journal:"+strings.Join(units, "")), "mode": mode}
	if journal {
		// The last line is "-- cursor: …" when journalctl printed any entry.
		body := strings.TrimSuffix(raw, "\n")
		if at := strings.LastIndex(body, "\n-- cursor: "); at >= 0 || strings.HasPrefix(body, "-- cursor: ") {
			cur := strings.TrimPrefix(body[at+1:], "-- cursor: ")
			if at < 0 {
				cur, body = strings.TrimPrefix(body, "-- cursor: "), ""
			} else {
				body = body[:at]
			}
			if logJournalCursor.MatchString(cur) {
				out["cursor"] = cur
			}
		}
		if strings.HasPrefix(body, "-- No entries --") {
			body = ""
		}
		out["text"] = strings.ToValidUTF8(body, "�")
		auditFileOp(r, acc, c, "monitor.log_lines", c.Name, map[string]interface{}{"unit": units[0], "mode": mode})
		jsonOK(w, out)
		return
	}
	if strings.HasPrefix(raw, "WRM-NOFILE") {
		jsonError(w, "The log file is gone or is no longer a regular file", 404)
		return
	}
	head, body, _ := strings.Cut(raw, "\n")
	var size int64
	total := 0
	if f := strings.Fields(strings.TrimPrefix(head, "WRM-HEAD ")); strings.HasPrefix(head, "WRM-HEAD ") && len(f) == 2 {
		size, _ = strconv.ParseInt(f[0], 10, 64)
		total, _ = strconv.Atoi(f[1])
	}
	switch mode {
	case "since":
		if strings.HasPrefix(body, "WRM-RESET") {
			out["reset"] = true
			body = ""
		}
		// Only whole lines are returned; a partly written last line waits for
		// the next poll. bytes is the offset to continue from.
		b, _ := strconv.ParseInt(q.Get("bytes"), 10, 64)
		if at := strings.LastIndexByte(body, '\n'); at >= 0 {
			body = body[:at+1]
		} else {
			body = ""
		}
		out["bytes"] = b + int64(len(body))
	case "tail":
		lines := strings.Count(body, "\n")
		if body != "" && !strings.HasSuffix(body, "\n") {
			lines++
		}
		if len(body) >= capBytes {
			// The cap cut the oldest line; drop the partial first line.
			if at := strings.IndexByte(body, '\n'); at >= 0 {
				body = body[at+1:]
				lines--
			}
		}
		out["first"] = max(1, total-lines+1)
		out["bytes"] = size
	case "around":
		line := num("line", 1, 1, 1<<30)
		out["first"] = max(1, line-num("before", 200, 0, 5000))
		out["bytes"] = size
	}
	out["text"] = strings.ToValidUTF8(strings.TrimSuffix(body, "\n"), "�")
	out["total"] = total
	out["size"] = size
	auditFileOp(r, acc, c, "monitor.log_lines", c.Name, map[string]interface{}{"path": files[0], "mode": mode})
	jsonOK(w, out)
}

// A journal cursor as journalctl prints it (s=…;i=…;b=…;m=…;t=…;x=…).
var logJournalCursor = regexp.MustCompile(`^[A-Za-z0-9=;_-]{1,512}$`)
