// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Workboard (v12.6.9): a page of small tiles. The server side holds what the tiles need
// and WRM did not keep before:
//   - an in-app list of each user's WRM events (/api/me/events), so notifications can be
//     read in WRM even when nothing is sent to e-mail or chat;
//   - disk, CPU and RAM of a server (/api/workboard/metrics): the latest Zabbix sample of
//     Monitoring when it is fresh, otherwise read over SSH with read-only commands.
// The tiles and their settings are the user's interface state (/api/me/ui/workboard).

const userEventsKeep = 200

// recordUserEvent keeps an event in the user's in-app list (whatever their notification
// subscriptions) and tells their open pages.
func recordUserEvent(userID int, event, title, body string) {
	if userID <= 0 || title == "" {
		return
	}
	// Reminders and "new version" are re-sent each round until delivered somewhere; the list
	// keeps one copy a day.
	if strings.HasPrefix(event, "credential.") || event == "git.new_version" || event == "git.job_upcoming" {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=? AND event=? AND title=? AND body=? AND created_at>?`,
			userID, event, truncateStr(title, 300), truncateStr(body, 2000), time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339)).Scan(&n)
		if n > 0 {
			return
		}
	}
	db.Exec(`INSERT INTO user_events (user_id, event, title, body, created_at, read) VALUES (?,?,?,?,?,0)`,
		userID, event, truncateStr(title, 300), truncateStr(body, 2000), time.Now().UTC().Format(time.RFC3339))
	db.Exec(`DELETE FROM user_events WHERE user_id=? AND id NOT IN (SELECT id FROM user_events WHERE user_id=? ORDER BY id DESC LIMIT ?)`,
		userID, userID, userEventsKeep)
	if hub != nil {
		hub.sendTo(userID, []byte(`{"type":"events_changed"}`))
	}
}

type userEvent struct {
	ID    int    `json:"id"`
	Event string `json:"event"`
	Title string `json:"title"`
	Body  string `json:"body"`
	At    string `json:"at"`
	Read  bool   `json:"read"`
}

// GET /api/me/events?limit=N   → {events: [...newest first], unread}
// POST /api/me/events/read     body {"ids": [..]} or {"all": true}
// DELETE /api/me/events        clears the list
func apiUserEventsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuth(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/me/events"), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > userEventsKeep {
			limit = 50
		}
		out := []userEvent{}
		rows, err := db.Query(`SELECT id, event, title, body, created_at, read FROM user_events WHERE user_id=? ORDER BY id DESC LIMIT ?`, uid, limit)
		if err == nil {
			for rows.Next() {
				var e userEvent
				var rd int
				if rows.Scan(&e.ID, &e.Event, &e.Title, &e.Body, &e.At, &rd) == nil {
					e.Read = rd != 0
					out = append(out, e)
				}
			}
			rows.Close()
		}
		var unread int
		db.QueryRow(`SELECT COUNT(*) FROM user_events WHERE user_id=? AND read=0`, uid).Scan(&unread)
		jsonOK(w, map[string]interface{}{"events": out, "unread": unread})
	case rest == "read" && r.Method == http.MethodPost:
		var req struct {
			IDs []int `json:"ids"`
			All bool  `json:"all"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if req.All {
			db.Exec(`UPDATE user_events SET read=1 WHERE user_id=?`, uid)
		} else {
			for i, id := range req.IDs {
				if i >= userEventsKeep {
					break
				}
				db.Exec(`UPDATE user_events SET read=1 WHERE user_id=? AND id=?`, uid, id)
			}
		}
		hub.sendTo(uid, []byte(`{"type":"events_changed"}`))
		jsonOK(w, map[string]interface{}{"ok": true})
	case rest == "" && r.Method == http.MethodDelete:
		db.Exec(`DELETE FROM user_events WHERE user_id=?`, uid)
		hub.sendTo(uid, []byte(`{"type":"events_changed"}`))
		jsonOK(w, map[string]interface{}{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

// ─── server metrics for the Workboard ──────────────────

type wbDisk struct {
	Mount   string  `json:"mount"`
	Percent float64 `json:"percent"`
	SizeKB  int64   `json:"size_kb,omitempty"`
}

type wbMetrics struct {
	ConnID    int      `json:"conn_id"`
	At        string   `json:"at"`
	Source    string   `json:"source"` // zabbix | ssh
	CPU       *float64 `json:"cpu_percent,omitempty"`
	Memory    *float64 `json:"memory_percent,omitempty"`
	MemTotal  int64    `json:"memory_total,omitempty"` // bytes
	Load      *float64 `json:"load,omitempty"`
	CPUCount  int      `json:"cpu_count,omitempty"`
	UptimeSec int64    `json:"uptime,omitempty"`
	Disks     []wbDisk `json:"disks"`
	Error     string   `json:"error,omitempty"`
}

// The read-only commands WRM runs for a server without a fresh Monitoring sample. Linux
// only (/proc); the CPU figure is the busy share over one second.
const wbMetricsScript = `LC_ALL=C; export LC_ALL; uname -s; echo @@; cat /proc/loadavg; echo @@; head -n1 /proc/stat; sleep 1; head -n1 /proc/stat; echo @@; grep -E '^(MemTotal|MemAvailable):' /proc/meminfo; echo @@; df -P -k -x tmpfs -x devtmpfs -x squashfs -x overlay 2>/dev/null || df -P -k; echo @@; cat /proc/uptime; echo @@; nproc 2>/dev/null || getconf _NPROCESSORS_ONLN`

var wbCache = struct {
	sync.Mutex
	data map[int]wbMetrics
	at   map[int]time.Time
	busy map[int]*sync.Mutex
}{data: map[int]wbMetrics{}, at: map[int]time.Time{}, busy: map[int]*sync.Mutex{}}

const wbCacheTTL = 25 * time.Second

func wbFromSample(s monitorSample) wbMetrics {
	m := wbMetrics{ConnID: s.ConnID, At: s.At, Source: "zabbix", Disks: []wbDisk{}, Error: s.Error}
	num := func(k string) *float64 {
		if v, ok := s.Metrics[k]; ok && v.Value != nil && v.Error == "" {
			x := *v.Value
			return &x
		}
		return nil
	}
	m.CPU, m.Memory, m.Load = num("cpu_percent"), num("memory_percent"), num("load")
	if v := num("memory_total"); v != nil {
		m.MemTotal = int64(*v)
	}
	if v := num("cpu_count"); v != nil {
		m.CPUCount = int(*v)
	}
	if v := num("uptime"); v != nil {
		m.UptimeSec = int64(*v)
	}
	for k, v := range s.Metrics {
		if strings.HasPrefix(k, "disk:") && v.Value != nil && v.Error == "" {
			m.Disks = append(m.Disks, wbDisk{Mount: strings.TrimPrefix(k, "disk:"), Percent: *v.Value})
		}
	}
	sort.Slice(m.Disks, func(i, j int) bool { return m.Disks[i].Mount < m.Disks[j].Mount })
	return m
}

// parseWBMetrics reads the output of wbMetricsScript.
func parseWBMetrics(out string) (wbMetrics, error) {
	m := wbMetrics{Source: "ssh", Disks: []wbDisk{}}
	parts := strings.Split(out, "@@")
	if len(parts) < 7 {
		return m, fmt.Errorf("unexpected output")
	}
	if os := strings.TrimSpace(parts[0]); os != "Linux" {
		return m, fmt.Errorf("reading metrics over SSH needs Linux (this server reports %q); set up Monitoring (Zabbix agent) for it", truncateStr(os, 40))
	}
	if f := strings.Fields(parts[1]); len(f) > 0 {
		if v, err := strconv.ParseFloat(f[0], 64); err == nil {
			m.Load = &v
		}
	}
	var cpu [][]float64
	for _, ln := range strings.Split(strings.TrimSpace(parts[2]), "\n") {
		f := strings.Fields(ln)
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		var vals []float64
		for _, x := range f[1:] {
			v, _ := strconv.ParseFloat(x, 64)
			vals = append(vals, v)
		}
		cpu = append(cpu, vals)
	}
	if len(cpu) == 2 {
		var t0, t1 float64
		for _, v := range cpu[0] {
			t0 += v
		}
		for _, v := range cpu[1] {
			t1 += v
		}
		idle := func(v []float64) float64 {
			s := v[3]
			if len(v) > 4 {
				s += v[4] // iowait
			}
			return s
		}
		if dt := t1 - t0; dt > 0 {
			p := (1 - (idle(cpu[1])-idle(cpu[0]))/dt) * 100
			p = float64(int(p*10+0.5)) / 10
			if p < 0 {
				p = 0
			}
			m.CPU = &p
		}
	}
	var total, avail float64
	for _, ln := range strings.Split(strings.TrimSpace(parts[3]), "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 {
			v, _ := strconv.ParseFloat(f[1], 64)
			switch f[0] {
			case "MemTotal:":
				total = v
			case "MemAvailable:":
				avail = v
			}
		}
	}
	if total > 0 {
		m.MemTotal = int64(total) * 1024
		p := float64(int((1-avail/total)*1000+0.5)) / 10
		m.Memory = &p
	}
	seen := map[string]bool{}
	for i, ln := range strings.Split(strings.TrimSpace(parts[4]), "\n") {
		f := strings.Fields(ln)
		if i == 0 || len(f) < 6 {
			continue
		}
		size, _ := strconv.ParseInt(f[1], 10, 64)
		pct, err := strconv.ParseFloat(strings.TrimSuffix(f[4], "%"), 64)
		mount := strings.Join(f[5:], " ")
		if err != nil || size <= 0 || seen[mount] || strings.HasPrefix(mount, "/snap/") || strings.HasPrefix(mount, "/run") || strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/sys") || strings.HasPrefix(mount, "/proc") {
			continue
		}
		seen[mount] = true
		m.Disks = append(m.Disks, wbDisk{Mount: mount, Percent: pct, SizeKB: size})
		if len(m.Disks) >= 20 {
			break
		}
	}
	if f := strings.Fields(parts[5]); len(f) > 0 {
		if v, err := strconv.ParseFloat(f[0], 64); err == nil {
			m.UptimeSec = int64(v)
		}
	}
	m.CPUCount, _ = strconv.Atoi(strings.TrimSpace(parts[6]))
	return m, nil
}

func wbCollect(ctx context.Context, c Connection) wbMetrics {
	cl, err := dialSSH(c, nil)
	if err != nil {
		return wbMetrics{ConnID: c.ID, At: nowRFC(), Source: "ssh", Disks: []wbDisk{}, Error: "SSH: " + truncateStr(err.Error(), 300)}
	}
	defer cl.Close()
	out, err := monitorSSHCommand(ctx, cl, wbMetricsScript)
	m, perr := parseWBMetrics(out)
	if perr != nil && err == nil {
		err = perr
	}
	m.ConnID, m.At, m.Source = c.ID, nowRFC(), "ssh"
	if err != nil {
		m.Error = truncateStr(redactText(err.Error()), 300)
	}
	return m
}

// GET /api/workboard/metrics?id=N — the user's own SSH/SFTP connections.
func apiWorkboardMetrics(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireAuth(w, r)
	if !ok || !requireMethod(w, r, http.MethodGet) {
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	c, err := loadConnection(id)
	if err != nil || c.UserID != uid {
		jsonError(w, "Connection not found", 404)
		return
	}
	if c.Protocol != "SSH" && c.Protocol != "SFTP" {
		jsonError(w, "Metrics need an SSH or SFTP connection", 400)
		return
	}
	// a fresh Monitoring (Zabbix) sample wins
	if loadMonitorConfig(c.ID).Enabled {
		s := latestMonitorSample(c.ID)
		if t, err := time.Parse(time.RFC3339, s.At); err == nil && time.Since(t) < 5*time.Minute && len(s.Metrics) > 0 {
			jsonOK(w, wbFromSample(s))
			return
		}
	}
	if !settingBool("workboard_ssh_metrics") {
		jsonError(w, "Reading server metrics over SSH is turned off by the administrator (workboard_ssh_metrics); set up Monitoring (Zabbix agent) for this server", 403)
		return
	}
	wbCache.Lock()
	mu := wbCache.busy[c.ID]
	if mu == nil {
		mu = &sync.Mutex{}
		wbCache.busy[c.ID] = mu
	}
	wbCache.Unlock()
	mu.Lock() // one collection per server at a time; the others take its result
	defer mu.Unlock()
	wbCache.Lock()
	if m, ok := wbCache.data[c.ID]; ok && time.Since(wbCache.at[c.ID]) < wbCacheTTL && r.URL.Query().Get("refresh") != "1" {
		wbCache.Unlock()
		jsonOK(w, m)
		return
	}
	wbCache.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 25*time.Second)
	defer cancel()
	m := wbCollect(ctx, c)
	wbCache.Lock()
	wbCache.data[c.ID], wbCache.at[c.ID] = m, time.Now()
	wbCache.Unlock()
	jsonOK(w, m)
}
