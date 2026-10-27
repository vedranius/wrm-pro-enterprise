// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type monitorLogRoot struct {
	Path     string `json:"path"`
	Service  string `json:"service"`
	Operator string `json:"operator"`
	// Kind is "journal" for a systemd unit read through journalctl; empty or
	// "file" means a directory of log files on the SSH host.
	Kind string `json:"kind,omitempty"`
	Unit string `json:"unit,omitempty"`
}
type monitorService struct {
	Kind string `json:"kind"` // systemd | docker
	Name string `json:"name"`
}
type monitorConfig struct {
	Enabled     bool             `json:"enabled"`
	Mode        string           `json:"mode"` // ssh | direct | api
	Address     string           `json:"address"`
	Interval    int              `json:"interval"`
	TLS         string           `json:"tls"` // none | certificate | psk-helper
	ServerName  string           `json:"server_name"`
	CA          string           `json:"ca,omitempty"`
	Certificate string           `json:"certificate,omitempty"`
	PrivateKey  string           `json:"private_key,omitempty"`
	PSKIdentity string           `json:"psk_identity,omitempty"`
	PSKFile     string           `json:"psk_file,omitempty"` // existing remote file; never retrieve it
	APIURL      string           `json:"api_url"`
	APIHost     string           `json:"api_host"`
	APIToken    string           `json:"api_token,omitempty"`
	HasSecrets  bool             `json:"has_secrets"`
	Roots       []monitorLogRoot `json:"roots"`
	Services    []monitorService `json:"services,omitempty"`
}

type monitorMetric struct {
	Value *float64 `json:"value"`
	Text  string   `json:"text,omitempty"`
	Error string   `json:"error,omitempty"`
	At    string   `json:"at,omitempty"`
}
type monitorSample struct {
	ConnID   int                      `json:"conn_id"`
	Hostname string                   `json:"hostname"`
	At       string                   `json:"at"`
	State    string                   `json:"state"`
	Error    string                   `json:"error,omitempty"`
	Metrics  map[string]monitorMetric `json:"metrics"`
	Services map[string]string        `json:"services,omitempty"` // "systemd:nginx" -> active
}

func initMonitoring() {
	ensureTable("monitor_profiles", `CREATE TABLE IF NOT EXISTS monitor_profiles(conn_id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL,config TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL DEFAULT '')`, "conn_id", "user_id", "config", "updated_at")
	ensureTable("monitor_samples", `CREATE TABLE IF NOT EXISTS monitor_samples(id INTEGER PRIMARY KEY AUTOINCREMENT,conn_id INTEGER NOT NULL,user_id INTEGER NOT NULL,at TEXT NOT NULL,data TEXT NOT NULL)`, "id", "conn_id", "user_id", "at", "data")
	db.Exec(`CREATE INDEX IF NOT EXISTS ix_monitor_samples ON monitor_samples(conn_id,id)`)
}
func defaultMonitorConfig() monitorConfig {
	return monitorConfig{Mode: "ssh", Address: "127.0.0.1:10050", Interval: 60, TLS: "none", Roots: []monitorLogRoot{{Path: "/var/log"}}}
}
func loadMonitorConfig(id int) monitorConfig {
	c := defaultMonitorConfig()
	var raw string
	if db.QueryRow(`SELECT config FROM monitor_profiles WHERE conn_id=?`, id).Scan(&raw) == nil {
		json.Unmarshal([]byte(decryptValue(raw)), &c)
	}
	return c
}
func monitorConfigView(c monitorConfig) monitorConfig {
	c.HasSecrets = c.PrivateKey != "" || c.APIToken != ""
	c.PrivateKey = ""
	c.APIToken = ""
	return c
}
func (c *monitorConfig) normalize() error {
	if c.Interval == 0 {
		c.Interval = 60
	}
	if c.Interval < 30 || c.Interval > 3600 {
		return fmt.Errorf("poll interval must be 30–3600 seconds")
	}
	if !oneOf(c.Mode, "ssh", "direct", "api") || !oneOf(c.TLS, "none", "certificate", "psk-helper") {
		return fmt.Errorf("unsupported monitoring transport")
	}
	if c.Mode != "api" {
		h, p, e := net.SplitHostPort(c.Address)
		if e != nil || h == "" || !validHealthPort(p) || strings.ContainsAny(h, "\x00\r\n") {
			return fmt.Errorf("agent address must be host:port")
		}
	}
	if c.TLS == "psk-helper" && (c.Mode != "ssh" || c.PSKIdentity == "" || !strings.HasPrefix(c.PSKFile, "/") || strings.ContainsAny(c.PSKFile+c.PSKIdentity, "\x00\r\n")) {
		return fmt.Errorf("PSK requires SSH and an existing remote absolute PSK file and identity for zabbix_get")
	}
	if c.Mode == "api" {
		u, e := url.Parse(c.APIURL)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || c.APIHost == "" {
			return fmt.Errorf("active-agent API requires an HTTPS api_jsonrpc.php URL and Zabbix host id")
		}
	}
	if len(c.Roots) > 20 {
		return fmt.Errorf("at most 20 log roots")
	}
	for i := range c.Roots {
		x := &c.Roots[i]
		if x.Kind == "journal" {
			if !monitorJournalUnit.MatchString(x.Unit) {
				return fmt.Errorf("invalid journal unit name")
			}
			// The unit is the whole address of this source; keep Path canonical so
			// the UI, audit records and root matching agree on one spelling.
			x.Path = "journal:" + x.Unit
			if len(x.Service) > 120 || len(x.Operator) > 120 {
				return fmt.Errorf("invalid log root labels")
			}
			continue
		}
		x.Kind = ""
		x.Unit = ""
		x.Path = path.Clean(x.Path)
		if !strings.HasPrefix(x.Path, "/") || x.Path == "/" || len(x.Path) > 1024 || strings.ContainsAny(x.Path, "\x00\r\n") || len(x.Service) > 120 || len(x.Operator) > 120 {
			return fmt.Errorf("invalid log root; use an absolute directory other than /")
		}
	}
	if len(c.CA)+len(c.Certificate)+len(c.PrivateKey) > 128<<10 || len(c.APIToken) > 4096 {
		return fmt.Errorf("monitoring credentials too large")
	}
	if len(c.Services) > 20 {
		return fmt.Errorf("at most 20 monitored services")
	}
	for i := range c.Services {
		x := &c.Services[i]
		x.Name = strings.TrimSpace(x.Name)
		if x.Kind == "" {
			x.Kind = "systemd"
		}
		switch x.Kind {
		case "systemd":
			if !monitorUnitPattern.MatchString(x.Name) {
				return fmt.Errorf("invalid systemd unit name %q", x.Name)
			}
		case "docker":
			if !monitorContainerPattern.MatchString(x.Name) {
				return fmt.Errorf("invalid Docker container name %q", x.Name)
			}
		default:
			return fmt.Errorf("a monitored service is systemd or docker")
		}
	}
	return nil
}
func latestMonitorSample(id int) monitorSample {
	s := monitorSample{ConnID: id, State: "unknown", Metrics: map[string]monitorMetric{}}
	var raw string
	if db.QueryRow(`SELECT data FROM monitor_samples WHERE conn_id=? ORDER BY id DESC LIMIT 1`, id).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	return s
}
func apiMonitoringHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/monitoring"), "/")
	if rest == "" {
		uid, ok := requireAuth(w, r)
		if !ok {
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		out := []map[string]interface{}{}
		for _, c := range loadUserConnections(uid) {
			if c.Protocol != "SSH" && c.Protocol != "SFTP" {
				continue
			}
			cfg := loadMonitorConfig(c.ID)
			out = append(out, map[string]interface{}{"connection": c.view(), "config": monitorConfigView(cfg), "sample": latestMonitorSample(c.ID)})
		}
		jsonOK(w, out)
		return
	}
	c, acc, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	if c.Protocol != "SSH" && c.Protocol != "SFTP" {
		jsonError(w, "Monitoring needs SSH/SFTP", 400)
		return
	}
	if rest == "config" {
		if r.Method == http.MethodGet {
			jsonOK(w, monitorConfigView(loadMonitorConfig(c.ID)))
			return
		}
		uid, auth := requireAuth(w, r)
		if !auth {
			return
		}
		if !requireMethod(w, r, http.MethodPut) || uid != c.UserID {
			if uid != c.UserID {
				jsonError(w, "Only the owner can configure monitoring", 403)
			}
			return
		}
		var cfg monitorConfig
		if json.NewDecoder(r.Body).Decode(&cfg) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		old := loadMonitorConfig(c.ID)
		if cfg.PrivateKey == "" {
			cfg.PrivateKey = old.PrivateKey
		}
		if cfg.APIToken == "" {
			cfg.APIToken = old.APIToken
		}
		if err := cfg.normalize(); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if _, err := db.Exec(`INSERT INTO monitor_profiles(conn_id,user_id,config,updated_at) VALUES(?,?,?,?) ON CONFLICT(conn_id) DO UPDATE SET config=excluded.config,updated_at=excluded.updated_at`, c.ID, uid, encryptValue(string(jsonMarshal(cfg))), nowRFC()); err != nil {
			jsonError(w, "Cannot store monitoring settings", 500)
			return
		}
		auditFileOp(r, acc, c, "monitor.configured", c.Name, map[string]interface{}{"mode": cfg.Mode, "roots": len(cfg.Roots)})
		jsonOK(w, monitorConfigView(cfg))
		return
	}
	switch rest {
	case "detect":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if acc.Share != nil {
			jsonError(w, "Only the owner can detect the agent setup", 403)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		cl, err := dialSSH(c, nil)
		if err != nil {
			jsonError(w, "SSH detection unavailable", 502)
			return
		}
		defer cl.Close()
		raw, err := monitorSSHCommand(ctx, cl, `uname -s; cat /etc/os-release 2>/dev/null; command -v zabbix_agent2; command -v zabbix_agentd; command -v zabbix_get; command -v apt-get; command -v dnf; command -v apk; true`)
		if err != nil {
			raw, err = monitorSSHCommand(ctx, cl, `powershell.exe -NoProfile -NonInteractive -Command "[Environment]::OSVersion.VersionString; Get-Service -Name '*Zabbix*' -ErrorAction SilentlyContinue | Select-Object Name,Status | Format-Table -HideTableHeaders"`)
		}
		if err != nil {
			jsonError(w, "Detection unavailable; inspect the operating system and installed agent manually", 409)
			return
		}
		auditFileOp(r, acc, c, "monitor.detected", c.Name, nil)
		jsonOK(w, map[string]string{"detected": truncateStr(redactText(raw), 8192)})
	case "check":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if acc.Share != nil {
			jsonError(w, "Only the owner can request a metrics poll", 403)
			return
		}
		// A closed browser tab must not turn into a "failed" sample and a problem
		// notification: the poll finishes on its own deadline.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
		defer cancel()
		s, err := pollMonitor(ctx, c)
		if err != nil {
			jsonError(w, err.Error(), 409)
			return
		}
		jsonOK(w, s)
	case "history":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		rows, err := db.Query(`SELECT data FROM monitor_samples WHERE conn_id=? ORDER BY id DESC LIMIT 1440`, c.ID)
		out := []monitorSample{}
		if err == nil {
			for rows.Next() {
				var raw string
				rows.Scan(&raw)
				var s monitorSample
				if json.Unmarshal([]byte(raw), &s) == nil {
					out = append(out, s)
				}
			}
			rows.Close()
		}
		jsonOK(w, out)
	case "logs/discover":
		monitorLogsDiscoverHandler(w, r, c, acc)
	case "logs/settings":
		logSettingsHandler(w, r, c, acc)
	case "logs/grep":
		logGrepHandler(w, r, c, acc)
	case "logs/lines":
		logLinesHandler(w, r, c, acc)
	case "logs", "log":
		monitorLogsHandler(w, r, c, acc, rest)
	default:
		jsonError(w, "Not found", 404)
	}
}

var monitorPolls = struct {
	sync.Mutex
	m map[int]bool
}{m: map[int]bool{}}
var monitorSlots = make(chan struct{}, 4)

func pollMonitor(ctx context.Context, c Connection) (monitorSample, error) {
	monitorPolls.Lock()
	if monitorPolls.m[c.ID] {
		monitorPolls.Unlock()
		return monitorSample{}, fmt.Errorf("this server is already being checked")
	}
	monitorPolls.m[c.ID] = true
	monitorPolls.Unlock()
	defer func() { monitorPolls.Lock(); delete(monitorPolls.m, c.ID); monitorPolls.Unlock() }()
	select {
	case monitorSlots <- struct{}{}:
		defer func() { <-monitorSlots }()
	case <-ctx.Done():
		return monitorSample{}, ctx.Err()
	}
	cfg := loadMonitorConfig(c.ID)
	s := collectMonitor(ctx, c, cfg)
	old := latestMonitorSample(c.ID)
	if cfg.Mode != "api" {
		addMonitorNetworkRates(&s, old)
	}
	data := string(jsonMarshal(s))
	db.Exec(`INSERT INTO monitor_samples(conn_id,user_id,at,data) VALUES(?,?,?,?)`, c.ID, c.UserID, s.At, data)
	db.Exec(`DELETE FROM monitor_samples WHERE conn_id=? AND id NOT IN (SELECT id FROM monitor_samples WHERE conn_id=? ORDER BY id DESC LIMIT 2880)`, c.ID, c.ID)
	if old.State != s.State && (old.At != "" || s.State != "healthy") {
		event := "monitor.problem"
		if s.State == "healthy" {
			event = "monitor.recovered"
		}
		notifyUser(c.UserID, event, func(lang string) (string, string) {
			if lang == "hr" {
				return "WRM nadzor: " + c.Name, "Poslužitelj " + c.Name + " (" + c.Host + "): " + s.State + "\n" + s.Error
			}
			return "WRM Monitoring: " + c.Name, "Server " + c.Name + " (" + c.Host + "): " + s.State + "\n" + s.Error
		})
	}
	return s, nil
}
func runMonitoring() {
	for range time.Tick(15 * time.Second) {
		rows, err := db.Query(`SELECT p.conn_id,p.user_id FROM monitor_profiles p JOIN connections c ON c.id=p.conn_id AND c.user_id=p.user_id JOIN users u ON u.id=p.user_id AND u.disabled=0`)
		if err != nil {
			continue
		}
		ids := []int{}
		for rows.Next() {
			var id, uid int
			rows.Scan(&id, &uid)
			ids = append(ids, id)
		}
		rows.Close()
		work := make(chan Connection)
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for c := range work {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					pollMonitor(ctx, c)
					cancel()
				}
			}()
		}
		for _, id := range ids {
			c, err := loadConnection(id)
			if err != nil {
				continue
			}
			cfg := loadMonitorConfig(id)
			last := latestMonitorSample(id)
			at, _ := time.Parse(time.RFC3339, last.At)
			if !cfg.Enabled || time.Since(at) < time.Duration(cfg.Interval)*time.Second {
				continue
			}
			work <- c
		}
		close(work)
		wg.Wait()
	}
}
func metricNumber(raw string, at string) monitorMetric {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return monitorMetric{Text: truncateStr(raw, 500), At: at}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return monitorMetric{Error: "not a finite number", At: at}
	}
	return monitorMetric{Value: &v, At: at}
}
