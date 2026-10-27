// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── OUT-OF-BAND MANAGEMENT (BMC: iDRAC, iLO, XClarity, IPMI…) ───
//
// A connection can name the BMC of its server: Redfish (HTTPS REST API of iDRAC, iLO,
// XClarity, Supermicro, OpenBMC…) or IPMI over LAN (through the ipmitool program). WRM
// then shows the power state and health of the machine and switches it on, off or
// restarts it — also when the operating system does not answer any more.
//
// The BMC is reached directly from the WRM server or through the connection's jump hosts
// (Redfish over an SSH channel of the last hop; ipmitool runs on the last hop, because IPMI
// uses UDP). Redfish certificates are pinned on first use like SSH host keys. The BMC
// password is stored encrypted, or a vault credential is used.
//
// Serial console (console.go): over the BMC's own SSH (iDRAC "console com2", iLO "vsp"),
// or IPMI Serial-over-LAN (ipmitool sol activate).

type bmcConfig struct {
	Type         string `json:"type"` // redfish | ipmi ("" = none)
	Host         string `json:"host"` // BMC address, host or host:port
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"` // write-only; encrypted at rest
	ClearPass    bool   `json:"clear_password,omitempty"`
	CredentialID *int   `json:"credential_id,omitempty"`
	ViaJump      bool   `json:"via_jump"`           // through the connection's jump hosts
	Console      string `json:"console,omitempty"`  // command for the console over the BMC's SSH
	SSHPort      int    `json:"ssh_port,omitempty"` // SSH port of the BMC (default 22)
	HasPassword  bool   `json:"has_password"`       // views only
	CredName     string `json:"credential_name,omitempty"`
}

var bmcHostRe = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]{1,255}$`)

// loadBMC returns the BMC settings of a connection with the password decrypted.
func loadBMC(connID int) (bmcConfig, bool) {
	var raw string
	var cfg bmcConfig
	if db.QueryRow(`SELECT COALESCE(bmc,'') FROM connections WHERE id=?`, connID).Scan(&raw) != nil || raw == "" {
		return cfg, false
	}
	if json.Unmarshal([]byte(raw), &cfg) != nil || cfg.Type == "" {
		return cfg, false
	}
	cfg.Password = decryptValue(cfg.Password)
	cfg.HasPassword = cfg.Password != ""
	return cfg, true
}

// view is the BMC as the browser sees it (no password).
func (b bmcConfig) view() *bmcConfig {
	v := b
	v.Password, v.ClearPass = "", false
	if v.CredentialID != nil {
		db.QueryRow(`SELECT name FROM credentials WHERE id=?`, *v.CredentialID).Scan(&v.CredName)
	}
	return &v
}

// saveBMC validates and stores the BMC settings of connection c (owned by userID).
// An empty password keeps the stored one.
func saveBMC(c Connection, in *bmcConfig, userID int) error {
	if in == nil {
		return nil
	}
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	if in.Type == "" || in.Type == "none" {
		_, err := db.Exec(`UPDATE connections SET bmc='' WHERE id=?`, c.ID)
		return err
	}
	if in.Type != "redfish" && in.Type != "ipmi" {
		return fmt.Errorf("BMC type must be redfish or ipmi")
	}
	in.Host = strings.TrimSpace(in.Host)
	in.Host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(in.Host, "https://"), "http://"), "/")
	if !bmcHostRe.MatchString(in.Host) {
		return fmt.Errorf("invalid BMC address (host or host:port)")
	}
	in.Username = truncateStr(strings.TrimSpace(in.Username), 120)
	in.Console = truncateStr(strings.TrimSpace(in.Console), 200)
	if in.SSHPort < 0 || in.SSHPort > 65535 {
		return fmt.Errorf("invalid SSH port")
	}
	if strings.ContainsAny(in.Console, "\r\n") {
		return fmt.Errorf("the console command must be one line")
	}
	if in.CredentialID != nil && *in.CredentialID <= 0 {
		in.CredentialID = nil
	}
	if in.CredentialID != nil {
		if _, err := credentialFor(Connection{UserID: userID, Host: in.Host, CredentialID: in.CredentialID, JumpID: bmcJump(c, *in)}); err != nil {
			return fmt.Errorf("BMC: %v", err)
		}
	}
	if in.ViaJump && !needsRoute(c) {
		in.ViaJump = false
	}
	if err := ipmiProxyRefused(c, *in); err != nil {
		return err
	}
	cur, _ := loadBMC(c.ID)
	pw := in.Password
	if pw == "" && !in.ClearPass {
		pw = cur.Password
	}
	if in.CredentialID != nil {
		pw = ""
	}
	store := *in
	store.Password, store.ClearPass, store.HasPassword, store.CredName = encryptValue(pw), false, false, ""
	_, err := db.Exec(`UPDATE connections SET bmc=? WHERE id=?`, string(jsonMarshal(store)), c.ID)
	return err
}

// ipmiProxyRefused: IPMI and Serial-over-LAN use UDP, which a SOCKS / HTTP proxy cannot carry.
func ipmiProxyRefused(c Connection, b bmcConfig) error {
	if b.Type == "ipmi" && b.ViaJump && c.ProxyID != nil && *c.ProxyID > 0 {
		return fmt.Errorf("IPMI and Serial-over-LAN use UDP: they cannot go through the proxy of this connection. Use Redfish, or turn off \"through the jump host\" (ipmitool then runs on the WRM server)")
	}
	return nil
}

// bmcProxy is the proxy the BMC is reached through (the connection's, when "through the
// route of this connection" is on).
func bmcProxy(c Connection, b bmcConfig) *int {
	if b.ViaJump {
		return c.ProxyID
	}
	return nil
}

func bmcJump(c Connection, b bmcConfig) *int {
	if b.ViaJump {
		return c.JumpID
	}
	return nil
}

// bmcLogin returns the user name and password for the BMC.
func bmcLogin(c Connection, b bmcConfig) (string, string, error) {
	if b.CredentialID == nil {
		if b.Username == "" {
			return "", "", fmt.Errorf("no BMC user name set")
		}
		return b.Username, b.Password, nil
	}
	cr, err := credentialFor(Connection{UserID: c.UserID, Host: b.Host, CredentialID: b.CredentialID, JumpID: bmcJump(c, b)})
	if err != nil {
		return "", "", err
	}
	return firstNonEmpty(cr.Username, b.Username), decryptValue(cr.password), nil
}

// bmcHop returns the SSH client of the last jump host when the BMC is reached through it.
func bmcHop(c Connection, b bmcConfig) (*ssh.Client, error) {
	if !b.ViaJump || c.JumpID == nil {
		return nil, nil
	}
	chain, err := jumpChain(c)
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, nil
	}
	cl, err := dialSSH(chain[len(chain)-1], nil)
	if err != nil {
		return nil, fmt.Errorf("jump host %s: %v", chain[len(chain)-1].Name, err)
	}
	return cl, nil
}

// pinnedTLSConfig accepts certificates from public CAs for the host name, and otherwise
// pins the certificate on first use (BMCs and internal FTPS servers are self-signed).
func pinnedTLSConfig(prefix, hostport string) *tls.Config {
	serverName := hostOnly(hostport)
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: true, // verified below: public CA chain, or pinned certificate
		MinVersion:         tls.VersionTLS12,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("server sent no certificate")
			}
			policy := getSetting("host_key_policy")
			leaf := cs.PeerCertificates[0]
			inter := x509.NewCertPool()
			for _, ic := range cs.PeerCertificates[1:] {
				inter.AddCert(ic)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{DNSName: serverName, Intermediates: inter}); err == nil || policy == "off" {
				return nil
			}
			host := prefix + normalizeHostKeyHost(hostport)
			sum := sha256.Sum256(leaf.Raw)
			fp := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
			stored, ok := lookupHostKey(host)
			if !ok {
				if policy == "strict" {
					rememberPendingKey(host, fp, leaf.Raw, "tls-cert")
					return &hostKeyError{Kind: "unknown", Host: host, KeyType: "tls-cert", Got: fp}
				}
				storeHostKey(host, "tls-cert", fp, leaf.Raw, "tofu")
				auditLogAs(nil, 0, "", "hostkey.trusted", host, map[string]string{"fingerprint": fp, "type": "tls-cert", "how": "first use"})
				return nil
			}
			if stored.Fingerprint == fp {
				return nil
			}
			rememberPendingKey(host, fp, leaf.Raw, "tls-cert")
			return &hostKeyError{Kind: "mismatch", Host: host, KeyType: "tls-cert", Expected: stored.Fingerprint, Got: fp}
		},
	}
}

// ── status and actions ──

type bmcStatus struct {
	Type         string   `json:"type"`
	Power        string   `json:"power"`  // on | off | unknown (lower case)
	Health       string   `json:"health"` // OK | Warning | Critical
	State        string   `json:"state,omitempty"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	Serial       string   `json:"serial,omitempty"`
	BIOS         string   `json:"bios,omitempty"`
	HostName     string   `json:"host_name,omitempty"`
	CPUs         int      `json:"cpus,omitempty"`
	CPUModel     string   `json:"cpu_model,omitempty"`
	MemoryGiB    float64  `json:"memory_gib,omitempty"`
	BMCModel     string   `json:"bmc_model,omitempty"`
	BMCFirmware  string   `json:"bmc_firmware,omitempty"`
	PowerWatts   float64  `json:"power_watts,omitempty"`
	InletC       float64  `json:"inlet_c,omitempty"`
	Faults       []string `json:"faults,omitempty"`
	Actions      []string `json:"actions"`           // power actions the BMC offers
	Console      string   `json:"console,omitempty"` // suggested console command (vendor)
	Details      []string `json:"details,omitempty"` // further lines (IPMI)
}

// bmcActions are WRM's power actions with their Redfish ResetType and ipmitool command.
var bmcActions = []struct{ Name, Redfish, IPMI string }{
	{"on", "On", "on"},
	{"shutdown", "GracefulShutdown", "soft"},
	{"off", "ForceOff", "off"},
	{"restart", "GracefulRestart", ""},
	{"reset", "ForceRestart", "reset"},
	{"cycle", "PowerCycle", "cycle"},
	{"nmi", "Nmi", "diag"},
}

// consoleFor suggests the command that starts the serial console on a BMC's SSH.
func consoleFor(manufacturer, bmcModel string) string {
	m := strings.ToLower(manufacturer + " " + bmcModel)
	switch {
	case strings.Contains(m, "dell") || strings.Contains(m, "idrac"):
		return "console com2"
	case strings.Contains(m, "hpe") || strings.Contains(m, "hewlett") || strings.Contains(m, "ilo"):
		return "vsp"
	case strings.Contains(m, "lenovo") || strings.Contains(m, "xclarity") || strings.Contains(m, "imm"):
		return "console 1"
	case strings.Contains(m, "supermicro"):
		return "cd system1/sol1; start"
	}
	return ""
}

// ── Redfish ──

type redfishClient struct {
	base string
	hc   *http.Client
	user string
	pass string
	hop  *targetRoute
}

func newRedfish(c Connection, b bmcConfig) (*redfishClient, error) {
	user, pass, err := bmcLogin(c, b)
	if err != nil {
		return nil, err
	}
	hostport := b.Host
	if _, _, err := net.SplitHostPort(hostport); err != nil {
		hostport = net.JoinHostPort(strings.Trim(hostport, "[]"), "443")
	}
	var hop *targetRoute
	if b.ViaJump && needsRoute(c) {
		// Redfish (HTTPS, TCP) goes through the jump hosts and the proxy of the connection.
		if hop, err = openTargetRoute(c); err != nil {
			return nil, err
		}
	}
	tr := &http.Transport{TLSClientConfig: pinnedTLSConfig("bmc://", hostport), TLSHandshakeTimeout: 15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second, MaxIdleConns: 2, DisableCompression: false}
	if hop != nil {
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) { return hop.Dial("tcp", addr) }
	} else {
		d := &net.Dialer{Timeout: 10 * time.Second}
		tr.DialContext = d.DialContext
	}
	hc := &http.Client{Timeout: 90 * time.Second, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != hostport || len(via) > 3 {
			return fmt.Errorf("the BMC redirected to another address")
		}
		return nil
	}}
	return &redfishClient{base: "https://" + hostport, hc: hc, user: user, pass: pass, hop: hop}, nil
}

func (rc *redfishClient) close() {
	rc.hc.CloseIdleConnections()
	if rc.hop != nil {
		rc.hop.Close()
	}
}

func (rc *redfishClient) do(method, path string, body interface{}) (map[string]interface{}, error) {
	if !strings.HasPrefix(path, "/redfish/") {
		return nil, fmt.Errorf("unexpected Redfish path %q", path)
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(jsonMarshal(body))
	}
	req, _ := http.NewRequest(method, rc.base+path, rd)
	req.SetBasicAuth(rc.user, rc.pass)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := rc.hc.Do(req)
	if err != nil {
		if hk := asHostKeyError(err); hk != nil {
			return nil, hk
		}
		return nil, fmt.Errorf("BMC not reachable: %v", shortNetError(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	switch {
	case resp.StatusCode == 401:
		return nil, fmt.Errorf("the BMC refused the login (user name or password)")
	case resp.StatusCode == 403:
		return nil, fmt.Errorf("the BMC user may not do this (403)")
	case resp.StatusCode >= 300:
		msg := ""
		var e struct {
			Error struct {
				Message  string `json:"message"`
				Extended []struct {
					Message string `json:"Message"`
				} `json:"@Message.ExtendedInfo"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil {
			msg = e.Error.Message
			if len(e.Error.Extended) > 0 && e.Error.Extended[0].Message != "" {
				msg = e.Error.Extended[0].Message
			}
		}
		return nil, fmt.Errorf("BMC answered %d %s", resp.StatusCode, truncateStr(firstNonEmpty(msg, strings.TrimSpace(string(data))), 200))
	}
	out := map[string]interface{}{}
	if len(bytes.TrimSpace(data)) > 0 {
		json.Unmarshal(data, &out)
	}
	return out, nil
}

func jsStr(m map[string]interface{}, path ...string) string {
	var cur interface{} = m
	for _, p := range path {
		mm, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	switch v := cur.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func jsNum(m map[string]interface{}, path ...string) float64 {
	f, _ := strconv.ParseFloat(jsStr(m, path...), 64)
	return f
}

// firstMember returns the @odata.id of the first member of a collection.
func (rc *redfishClient) firstMember(path string) (string, error) {
	col, err := rc.do("GET", path, nil)
	if err != nil {
		return "", err
	}
	members, _ := col["Members"].([]interface{})
	if len(members) == 0 {
		return "", fmt.Errorf("the BMC lists no %s", strings.TrimPrefix(path, "/redfish/v1/"))
	}
	m, _ := members[0].(map[string]interface{})
	id := jsStr(m, "@odata.id")
	if id == "" {
		return "", fmt.Errorf("unexpected Redfish answer")
	}
	return id, nil
}

func (rc *redfishClient) system() (string, map[string]interface{}, error) {
	id, err := rc.firstMember("/redfish/v1/Systems")
	if err != nil {
		return "", nil, err
	}
	sys, err := rc.do("GET", id, nil)
	return id, sys, err
}

func redfishStatus(c Connection, b bmcConfig) (*bmcStatus, error) {
	rc, err := newRedfish(c, b)
	if err != nil {
		return nil, err
	}
	defer rc.close()
	_, sys, err := rc.system()
	if err != nil {
		return nil, err
	}
	st := &bmcStatus{Type: "redfish", Power: strings.ToLower(firstNonEmpty(jsStr(sys, "PowerState"), "unknown")),
		Health: firstNonEmpty(jsStr(sys, "Status", "HealthRollup"), jsStr(sys, "Status", "Health")), State: jsStr(sys, "Status", "State"),
		Manufacturer: jsStr(sys, "Manufacturer"), Model: jsStr(sys, "Model"), Serial: firstNonEmpty(jsStr(sys, "SerialNumber"), jsStr(sys, "SKU")),
		BIOS: jsStr(sys, "BiosVersion"), HostName: jsStr(sys, "HostName"), CPUModel: jsStr(sys, "ProcessorSummary", "Model"),
		MemoryGiB: jsNum(sys, "MemorySummary", "TotalSystemMemoryGiB")}
	st.CPUs = int(jsNum(sys, "ProcessorSummary", "Count"))
	st.Actions = redfishAllowed(sys)
	// BMC firmware, power draw and inlet temperature: best effort
	if id, err := rc.firstMember("/redfish/v1/Managers"); err == nil {
		if m, err := rc.do("GET", id, nil); err == nil {
			st.BMCFirmware, st.BMCModel = jsStr(m, "FirmwareVersion"), firstNonEmpty(jsStr(m, "Model"), jsStr(m, "Name"))
		}
	}
	if id, err := rc.firstMember("/redfish/v1/Chassis"); err == nil {
		if p, err := rc.do("GET", strings.TrimRight(id, "/")+"/Power", nil); err == nil {
			if pcs, ok := p["PowerControl"].([]interface{}); ok && len(pcs) > 0 {
				if pc, ok := pcs[0].(map[string]interface{}); ok {
					st.PowerWatts = jsNum(pc, "PowerConsumedWatts")
				}
			}
		}
		if th, err := rc.do("GET", strings.TrimRight(id, "/")+"/Thermal", nil); err == nil {
			if ts, ok := th["Temperatures"].([]interface{}); ok {
				for _, x := range ts {
					t, _ := x.(map[string]interface{})
					if strings.Contains(strings.ToLower(jsStr(t, "Name")), "inlet") && jsNum(t, "ReadingCelsius") > 0 {
						st.InletC = jsNum(t, "ReadingCelsius")
						break
					}
				}
			}
		}
	}
	st.Console = consoleFor(st.Manufacturer, st.BMCModel)
	return st, nil
}

// redfishAllowed maps the ResetType values a system accepts to WRM's actions.
func redfishAllowed(sys map[string]interface{}) []string {
	act, _ := sys["Actions"].(map[string]interface{})
	reset, _ := act["#ComputerSystem.Reset"].(map[string]interface{})
	allowed := map[string]bool{}
	if vals, ok := reset["ResetType@Redfish.AllowableValues"].([]interface{}); ok {
		for _, v := range vals {
			if s, ok := v.(string); ok {
				allowed[s] = true
			}
		}
	}
	out := []string{}
	for _, a := range bmcActions {
		if len(allowed) == 0 || allowed[a.Redfish] {
			out = append(out, a.Name)
		}
	}
	return out
}

func redfishPower(c Connection, b bmcConfig, action string) error {
	rc, err := newRedfish(c, b)
	if err != nil {
		return err
	}
	defer rc.close()
	id, sys, err := rc.system()
	if err != nil {
		return err
	}
	resetType := ""
	for _, a := range bmcActions {
		if a.Name == action {
			resetType = a.Redfish
		}
	}
	ok := false
	for _, a := range redfishAllowed(sys) {
		ok = ok || a == action
	}
	if resetType == "" || !ok {
		return fmt.Errorf("this BMC does not offer %q", action)
	}
	target := jsStr(sys, "Actions", "#ComputerSystem.Reset", "target")
	if target == "" {
		target = strings.TrimRight(id, "/") + "/Actions/ComputerSystem.Reset"
	}
	_, err = rc.do("POST", target, map[string]string{"ResetType": resetType})
	return err
}

// ── IPMI (ipmitool) ──

var ipmitoolPath = "ipmitool"

func ipmiArgs(b bmcConfig, user string) []string {
	host, port := b.Host, "623"
	if h, p, err := net.SplitHostPort(b.Host); err == nil {
		host, port = h, p
	}
	return []string{"-I", "lanplus", "-H", strings.Trim(host, "[]"), "-p", port, "-U", user, "-E", "-N", "3", "-R", "2"}
}

// runIPMI runs ipmitool with args on the WRM server, or on the last jump host.
func runIPMI(c Connection, b bmcConfig, args ...string) (string, error) {
	user, pass, err := bmcLogin(c, b)
	if err != nil {
		return "", err
	}
	full := append(ipmiArgs(b, user), args...)
	if err := ipmiProxyRefused(c, b); err != nil {
		return "", err
	}
	hop, err := bmcHop(c, b)
	if err != nil {
		return "", err
	}
	if hop != nil {
		defer hop.Close()
		q := make([]string, len(full))
		for i, a := range full {
			q[i] = shellQuote(a)
		}
		script := `IFS= read -r IPMI_PASSWORD; export IPMI_PASSWORD
command -v ipmitool >/dev/null 2>&1 || { echo "ipmitool is not installed on the jump host" >&2; exit 127; }
exec ipmitool ` + strings.Join(q, " ")
		out, err := runRemoteScript(hop, script, pass+"\n", 60*time.Second)
		return out, err
	}
	path, lerr := exec.LookPath(ipmitoolPath)
	if lerr != nil {
		return "", fmt.Errorf("ipmitool is not installed on the WRM server (apt install ipmitool), or reach the BMC through a jump host that has it")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, full...)
	cmd.Env = append(cmd.Environ(), "IPMI_PASSWORD="+pass)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("ipmitool: %s", truncateStr(msg, 300))
	}
	return stdout.String(), nil
}

func ipmiFields(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, ":"); i > 0 {
			k := strings.TrimSpace(l[:i])
			if _, ok := m[k]; !ok {
				m[k] = strings.TrimSpace(l[i+1:])
			}
		}
	}
	return m
}

func ipmiStatus(c Connection, b bmcConfig) (*bmcStatus, error) {
	out, err := runIPMI(c, b, "chassis", "status")
	if err != nil {
		return nil, err
	}
	f := ipmiFields(out)
	st := &bmcStatus{Type: "ipmi", Power: strings.ToLower(firstNonEmpty(f["System Power"], "unknown")), Health: "OK", Faults: []string{}}
	for _, k := range []string{"Power Overload", "Main Power Fault", "Power Control Fault", "Drive Fault", "Cooling/Fan Fault", "Chassis Intrusion"} {
		if v := strings.ToLower(f[k]); v == "true" || v == "active" {
			st.Faults = append(st.Faults, k)
		}
	}
	if len(st.Faults) > 0 {
		st.Health = "Warning"
	}
	if v := f["Last Power Event"]; v != "" {
		st.Details = append(st.Details, "Last power event: "+v)
	}
	if out, err := runIPMI(c, b, "mc", "info"); err == nil {
		mf := ipmiFields(out)
		st.BMCFirmware, st.Manufacturer = mf["Firmware Revision"], mf["Manufacturer Name"]
	}
	if out, err := runIPMI(c, b, "fru", "print", "0"); err == nil {
		ff := ipmiFields(out)
		st.Model = firstNonEmpty(ff["Product Name"], ff["Board Product"])
		st.Serial = firstNonEmpty(ff["Product Serial"], ff["Board Serial"])
		st.Manufacturer = firstNonEmpty(ff["Product Manufacturer"], ff["Board Mfg"], st.Manufacturer)
	}
	for _, a := range bmcActions {
		if a.IPMI != "" {
			st.Actions = append(st.Actions, a.Name)
		}
	}
	st.Actions = append(st.Actions, "sol")
	return st, nil
}

func ipmiPower(c Connection, b bmcConfig, action string) error {
	for _, a := range bmcActions {
		if a.Name == action && a.IPMI != "" {
			_, err := runIPMI(c, b, "chassis", "power", a.IPMI)
			return err
		}
	}
	return fmt.Errorf("IPMI does not offer %q", action)
}

// ── API ──

// GET  /api/connections/{id}/bmc             power state, health and inventory
// POST /api/connections/{id}/bmc {action}    on | shutdown | off | restart | reset | cycle | nmi
func bmcHandler(w http.ResponseWriter, r *http.Request, userID int, c Connection) {
	if !settingBool("bmc_enabled") {
		jsonError(w, "Out-of-band management is turned off by the administrator", 403)
		return
	}
	b, ok := loadBMC(c.ID)
	if !ok {
		jsonError(w, "No BMC is set for this connection (Edit → Out-of-band management)", 404)
		return
	}
	full, err := loadConnection(c.ID)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var st *bmcStatus
		if b.Type == "ipmi" {
			st, err = ipmiStatus(full, b)
		} else {
			st, err = redfishStatus(full, b)
		}
		if err != nil {
			if hk := asHostKeyError(err); hk != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(502)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": hk.Error(), "hostkey": map[string]string{"kind": hk.Kind, "host": hk.Host,
					"key_type": hk.KeyType, "expected": hk.Expected, "fingerprint": hk.Got}})
				return
			}
			jsonError(w, err.Error(), 502)
			return
		}
		if b.Console != "" {
			st.Console = b.Console
		}
		sort.Strings(st.Faults)
		jsonOK(w, st)
	case http.MethodPost:
		var in struct {
			Action string `json:"action"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		known := false
		for _, a := range bmcActions {
			known = known || a.Name == in.Action
		}
		if !known {
			jsonError(w, "Unknown action", 400)
			return
		}
		if b.Type == "ipmi" {
			err = ipmiPower(full, b, in.Action)
		} else {
			err = redfishPower(full, b, in.Action)
		}
		details := map[string]interface{}{"action": in.Action, "bmc": b.Host, "type": b.Type}
		if err != nil {
			details["error"] = err.Error()
			auditLogRef(r, userID, usernameOf(userID), "bmc.power_failed", c.Name, details, auditRef{ConnID: c.ID})
			jsonError(w, err.Error(), 502)
			return
		}
		auditLogRef(r, userID, usernameOf(userID), "bmc.power", c.Name, details, auditRef{ConnID: c.ID})
		statusMon.poke()
		jsonOK(w, map[string]interface{}{"ok": true, "action": in.Action})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// userOwnsBMC: one of the user's connections has a BMC at host:port (for accepting its certificate).
func userOwnsBMC(userID int, hostport string) bool {
	rows, err := db.Query(`SELECT id FROM connections WHERE user_id=? AND bmc<>''`, userID)
	if err != nil {
		return false
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if b, ok := loadBMC(id); ok {
			hp := b.Host
			if _, _, err := net.SplitHostPort(hp); err != nil {
				hp = net.JoinHostPort(strings.Trim(hp, "[]"), "443")
			}
			if normalizeHostKeyHost(hp) == hostport {
				return true
			}
		}
	}
	return false
}
