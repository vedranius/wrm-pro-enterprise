// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRedfish is a Redfish service like a Dell iDRAC.
type fakeRedfish struct {
	mu     sync.Mutex
	power  string
	resets []string
}

func (f *fakeRedfish) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "root" || p != "calvin" {
			w.WriteHeader(401)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		sys := "/redfish/v1/Systems/System.Embedded.1"
		member := func(id string) map[string]interface{} {
			return map[string]interface{}{"Members": []map[string]string{{"@odata.id": id}}}
		}
		var out interface{}
		switch r.URL.Path {
		case "/redfish/v1/Systems":
			out = member(sys)
		case sys:
			out = map[string]interface{}{"PowerState": f.power, "Status": map[string]string{"Health": "OK", "HealthRollup": "Warning", "State": "Enabled"},
				"Manufacturer": "Dell Inc.", "Model": "PowerEdge R650", "SerialNumber": "7XYZ123", "BiosVersion": "1.10.2", "HostName": "web-01",
				"ProcessorSummary": map[string]interface{}{"Count": 2, "Model": "Intel Xeon Silver 4314"}, "MemorySummary": map[string]interface{}{"TotalSystemMemoryGiB": 256},
				"Actions": map[string]interface{}{"#ComputerSystem.Reset": map[string]interface{}{"target": sys + "/Actions/ComputerSystem.Reset",
					"ResetType@Redfish.AllowableValues": []string{"On", "ForceOff", "GracefulShutdown", "ForceRestart", "PowerCycle", "Nmi"}}}}
		case sys + "/Actions/ComputerSystem.Reset":
			var in struct{ ResetType string }
			json.NewDecoder(r.Body).Decode(&in)
			f.resets = append(f.resets, in.ResetType)
			if in.ResetType == "ForceOff" {
				f.power = "Off"
			}
			w.WriteHeader(204)
			return
		case "/redfish/v1/Managers":
			out = member("/redfish/v1/Managers/iDRAC.Embedded.1")
		case "/redfish/v1/Managers/iDRAC.Embedded.1":
			out = map[string]interface{}{"FirmwareVersion": "7.00.00.00", "Model": "16G Monolithic", "Name": "iDRAC"}
		case "/redfish/v1/Chassis":
			out = member("/redfish/v1/Chassis/System.Embedded.1")
		case "/redfish/v1/Chassis/System.Embedded.1/Power":
			out = map[string]interface{}{"PowerControl": []map[string]interface{}{{"PowerConsumedWatts": 212}}}
		case "/redfish/v1/Chassis/System.Embedded.1/Thermal":
			out = map[string]interface{}{"Temperatures": []map[string]interface{}{{"Name": "CPU1 Temp", "ReadingCelsius": 51}, {"Name": "System Board Inlet Temp", "ReadingCelsius": 22}}}
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"message":"not found"}}`))
			return
		}
		json.NewEncoder(w).Encode(out)
	}
}

func TestRedfishStatusAndPower(t *testing.T) {
	fr := &fakeRedfish{power: "On"}
	rs := httptest.NewTLSServer(fr.handler())
	defer rs.Close()
	bmcAddr := strings.TrimPrefix(rs.URL, "https://")
	srv := newTestServer(t)
	u := newTestUser(t, srv, "bmc-user", false)
	other := newTestUser(t, srv, "bmc-other", false)
	var v connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "web-01", "host": "10.1.0.11", "bmc": map[string]interface{}{"type": "redfish",
		"host": "https://" + bmcAddr + "/", "username": "root", "password": "calvin"}}, &v); code != 201 || v.BMC == nil || !v.BMC.HasPassword || v.BMC.Password != "" || v.BMC.Host != bmcAddr {
		t.Fatalf("create with BMC: %d %+v", code, v.BMC)
	}
	var raw string
	db.QueryRow(`SELECT bmc FROM connections WHERE id=?`, v.ID).Scan(&raw)
	if strings.Contains(raw, "calvin") {
		t.Fatalf("BMC password not encrypted: %s", raw)
	}
	var st bmcStatus
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &st); code != 200 {
		t.Fatalf("status: %d", code)
	}
	if st.Power != "on" || st.Health != "Warning" || st.Model != "PowerEdge R650" || st.Serial != "7XYZ123" || st.CPUs != 2 || st.MemoryGiB != 256 ||
		st.BMCFirmware != "7.00.00.00" || st.PowerWatts != 212 || st.InletC != 22 || st.Console != "console com2" {
		t.Fatalf("status: %+v", st)
	}
	if strings.Join(st.Actions, ",") != "on,shutdown,off,reset,cycle,nmi" {
		t.Fatalf("actions: %v", st.Actions)
	}
	// the self-signed certificate is pinned on first use
	if _, ok := lookupHostKey("bmc://" + normalizeHostKeyHost(bmcAddr)); !ok {
		t.Fatalf("certificate not pinned")
	}
	var res map[string]interface{}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/bmc", v.ID), map[string]string{"action": "off"}, &res); code != 200 {
		t.Fatalf("power off: %d %v", code, res)
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/bmc", v.ID), map[string]string{"action": "restart"}, &res); code != 502 || !strings.Contains(fmt.Sprint(res["error"]), "does not offer") {
		t.Fatalf("graceful restart is not offered: %d %v", code, res)
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/bmc", v.ID), map[string]string{"action": "format-disk"}, &res); code != 400 {
		t.Fatalf("unknown action: %d", code)
	}
	if strings.Join(fr.resets, ",") != "ForceOff" {
		t.Fatalf("resets: %v", fr.resets)
	}
	u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &st)
	if st.Power != "off" {
		t.Fatalf("power after off: %+v", st)
	}
	if code := other.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &res); code != 404 {
		t.Fatalf("other user: %d", code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='bmc.power' AND conn_id=?`, v.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("audit bmc.power: %d", n)
	}
	// a changed certificate is refused like a changed host key
	db.Exec(`UPDATE known_hosts SET fingerprint='SHA256:other' WHERE host=?`, "bmc://"+normalizeHostKeyHost(bmcAddr))
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &res); code != 502 || res["hostkey"] == nil {
		t.Fatalf("changed certificate: %d %v", code, res)
	}
	db.Exec(`DELETE FROM known_hosts WHERE host=?`, "bmc://"+normalizeHostKeyHost(bmcAddr))
	// wrong password; edit keeps the stored password when none is sent
	u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "web-01", "host": "10.1.0.11", "bmc": map[string]interface{}{"type": "redfish", "host": bmcAddr, "username": "root", "password": "nope"}}, nil)
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &res); code != 502 || !strings.Contains(fmt.Sprint(res["error"]), "refused the login") {
		t.Fatalf("wrong password: %d %v", code, res)
	}
	// through a jump host (the BMC network is only reachable from the bastion), with a vault credential
	sshAddr := startTestSSHServer(t)
	jump := u.addConnection("bastion", sshAddr)
	var cr credential
	u.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "idrac-root", "username": "root", "password": "calvin"}, &cr)
	before := directTCPIPCount.Load()
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "web-01", "host": "10.1.0.11", "jump_id": jump,
		"bmc": map[string]interface{}{"type": "redfish", "host": bmcAddr, "credential_id": cr.ID, "via_jump": true}}, &res); code != 200 {
		t.Fatalf("via jump: %d %v", code, res)
	}
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &st); code != 200 || st.Power != "off" {
		t.Fatalf("status via jump: %d %+v", code, st)
	}
	if directTCPIPCount.Load() <= before {
		t.Fatalf("the BMC was not reached through the jump host")
	}
	u.jsonDo("GET", fmt.Sprintf("/api/connections/%d", v.ID), nil, &v)
	if v.BMC == nil || v.BMC.CredName != "idrac-root" || v.BMC.HasPassword {
		t.Fatalf("BMC with credential: %+v", v.BMC)
	}
	// removing the BMC
	u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "web-01", "host": "10.1.0.11", "bmc": map[string]interface{}{"type": ""}}, nil)
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &res); code != 404 {
		t.Fatalf("removed BMC: %d", code)
	}
}

// writeFakeIpmitool installs a fake ipmitool that checks IPMI_PASSWORD and logs its arguments.
func writeFakeIpmitool(t *testing.T, password string) (dir, logFile string) {
	dir = t.TempDir()
	logFile = filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
echo "$*" >> ` + logFile + `
if [ "$IPMI_PASSWORD" != "` + password + `" ]; then echo "Error: Unable to establish IPMI v2 / RMCP+ session" >&2; exit 1; fi
case "$*" in
  *"chassis status"*) printf 'System Power         : on\nPower Overload       : false\nMain Power Fault     : false\nLast Power Event     : command\nChassis Intrusion    : inactive\nDrive Fault          : true\n' ;;
  *"mc info"*) printf 'Firmware Revision         : 2.50\nManufacturer Name         : Supermicro\n' ;;
  *"fru print 0"*) printf ' Product Name          : X11DPi-NT\n Product Serial        : S12345\n' ;;
  *"chassis power"*) echo "Chassis Power Control: Up/On" ;;
  *"sol deactivate"*) exit 0 ;;
  *"sol activate"*) echo "[SOL Session operational.  Use ~? for help]"; exec cat ;;
  *) echo "unknown" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ipmitool"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, logFile
}

func TestIPMIStatusPowerAndSOL(t *testing.T) {
	dir, logFile := writeFakeIpmitool(t, "ipmi-secret")
	old := ipmitoolPath
	ipmitoolPath = filepath.Join(dir, "ipmitool")
	defer func() { ipmitoolPath = old }()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	srv := newTestServer(t)
	u := newTestUser(t, srv, "ipmi-user", false)
	var v connView
	u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "sm-01", "host": "10.2.0.5", "bmc": map[string]interface{}{"type": "ipmi",
		"host": "10.9.9.9", "username": "ADMIN", "password": "ipmi-secret"}}, &v)
	var st bmcStatus
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &st); code != 200 || st.Power != "on" || st.Health != "Warning" ||
		strings.Join(st.Faults, ",") != "Drive Fault" || st.BMCFirmware != "2.50" || st.Model != "X11DPi-NT" || st.Serial != "S12345" || st.Manufacturer != "Supermicro" {
		t.Fatalf("ipmi status: %d %+v", code, st)
	}
	var res map[string]interface{}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/bmc", v.ID), map[string]string{"action": "cycle"}, &res); code != 200 {
		t.Fatalf("cycle: %d %v", code, res)
	}
	if code := u.jsonDo("POST", fmt.Sprintf("/api/connections/%d/bmc", v.ID), map[string]string{"action": "restart"}, &res); code != 502 {
		t.Fatalf("restart over IPMI: %d", code)
	}
	calls, _ := os.ReadFile(logFile)
	if !strings.Contains(string(calls), "-I lanplus -H 10.9.9.9 -p 623 -U ADMIN -E") || !strings.Contains(string(calls), "chassis power cycle") || strings.Contains(string(calls), "ipmi-secret") {
		t.Fatalf("ipmitool calls:\n%s", calls)
	}
	// Serial-over-LAN on the WRM server (pseudo terminal)
	if _, _, err := openPTYPair(); err == nil {
		tc := u.openTerminalQ(v.ID, "&console=sol")
		tc.waitFor("SOL Session operational")
		tc.send("hello-console\r")
		tc.waitFor("hello-console")
		tc.ws.Close()
	}
	// through a jump host that has ipmitool: the password goes over stdin, never shown
	h := startFakeHost(t, "ops", "ops-pass")
	jump := u.addConnection("bastion", h.addr)
	db.Exec(`UPDATE connections SET username='ops', password=? WHERE id=?`, encryptValue("ops-pass"), jump)
	if code := u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "sm-01", "host": "10.2.0.5", "jump_id": jump,
		"bmc": map[string]interface{}{"type": "ipmi", "host": "10.9.9.9:6230", "username": "ADMIN", "via_jump": true}}, &res); code != 200 {
		t.Fatalf("via jump: %d %v", code, res)
	}
	os.Remove(logFile)
	if code := u.jsonDo("GET", fmt.Sprintf("/api/connections/%d/bmc", v.ID), nil, &st); code != 200 || st.Power != "on" {
		t.Fatalf("status via jump: %d %+v", code, st)
	}
	calls, _ = os.ReadFile(logFile)
	if !strings.Contains(string(calls), "-p 6230") {
		t.Fatalf("ipmitool on the jump host: %s", calls)
	}
	tc := u.openTerminalQ(v.ID, "&console=sol")
	tc.waitFor("SOL Session operational")
	tc.send("via-jump\n")
	tc.waitFor("via-jump")
	tc.mu.Lock()
	out := tc.out.String()
	tc.mu.Unlock()
	if strings.Contains(out, solMarker) || strings.Contains(out, "ipmi-secret") {
		t.Fatalf("marker or password in the console output:\n%s", out)
	}
	tc.ws.Close()
	var proto string
	db.QueryRow(`SELECT protocol FROM terminal_sessions WHERE conn_id=? ORDER BY id DESC LIMIT 1`, v.ID).Scan(&proto)
	if proto != "sol" {
		t.Fatalf("session protocol: %q", proto)
	}
}

func TestBMCConsoleOverSSH(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "bmccons-user", false)
	sshAddr := startTestSSHServer(t)
	_, port, _ := strings.Cut(sshAddr, ":")
	var v connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "r650", "host": "10.3.0.5", "bmc": map[string]interface{}{"type": "redfish",
		"host": "127.0.0.1", "ssh_port": atoi(port), "username": testSSHUser, "password": testSSHPass, "console": "console com2"}}, &v); code != 201 {
		t.Fatalf("create: %d", code)
	}
	tc := u.openTerminalQ(v.ID, "&console=bmc")
	tc.waitFor("you typed: console com2")
	tc.ws.Close()
	// share members never get consoles
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "x", "host": "10.3.0.6", "bmc": map[string]interface{}{"type": "redfish", "host": "bad host!"}}, &map[string]interface{}{}); code != 400 {
		t.Fatalf("invalid BMC address accepted: %d", code)
	}
}

func atoi(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func TestSerialPortConnection(t *testing.T) {
	master, slavePath, err := openPTYPair()
	if err != nil {
		t.Skip("no pseudo terminals:", err)
	}
	defer master.Close()
	dir := t.TempDir()
	if err := os.Symlink(slavePath, filepath.Join(dir, "ttyWRM0")); err != nil {
		t.Fatal(err)
	}
	old := serialDevDir
	serialDevDir = dir + "/"
	defer func() { serialDevDir = old }()
	srv := newTestServer(t)
	admin := newTestUser(t, srv, "serial-admin", true)
	user := newTestUser(t, srv, "serial-user", false)
	body := map[string]interface{}{"name": "switch-console", "protocol": "SERIAL", "host": "ttyWRM0", "options": map[string]string{"baud": "115200", "parity": "none"}}
	if code := user.jsonDo("POST", "/api/connections", body, &map[string]interface{}{}); code != 403 {
		t.Fatalf("non-admin serial port: %d", code)
	}
	bad := map[string]interface{}{"name": "x", "protocol": "SERIAL", "host": "../etc/passwd"}
	if code := admin.jsonDo("POST", "/api/connections", bad, &map[string]interface{}{}); code != 400 {
		t.Fatalf("bad device accepted: %d", code)
	}
	bad = map[string]interface{}{"name": "x", "protocol": "SERIAL", "host": "ttyS0", "options": map[string]string{"baud": "12345"}}
	if code := admin.jsonDo("POST", "/api/connections", bad, &map[string]interface{}{}); code != 400 {
		t.Fatalf("bad speed accepted: %d", code)
	}
	var v connView
	if code := admin.jsonDo("POST", "/api/connections", body, &v); code != 201 || v.Options["baud"] != "115200" || v.Options["flow"] != "none" {
		t.Fatalf("serial connection: %d %+v", code, v)
	}
	var tr map[string]interface{}
	admin.jsonDo("POST", "/api/connections/test", map[string]interface{}{"name": "t", "protocol": "SERIAL", "host": "ttyWRM0", "options": map[string]string{"baud": "9600"}}, &tr)
	if tr["ok"] != true || !strings.Contains(fmt.Sprint(tr["message"]), "9600 8N1") {
		t.Fatalf("test serial: %v", tr)
	}
	tc := admin.openTerminal(v.ID)
	tc.waitFor("Serial port")
	time.Sleep(200 * time.Millisecond)
	master.Write([]byte("Switch> "))
	tc.waitFor("Switch> ")
	tc.send("show version\r")
	buf := make([]byte, 64)
	got := ""
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(got, "show version\r") && time.Now().Before(deadline) {
		master.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := master.Read(buf)
		got += string(buf[:n])
	}
	if !strings.Contains(got, "show version\r") {
		t.Fatalf("serial device received %q", got)
	}
	tc.ws.Close()
	// the status monitor never probes serial ports
	for _, c := range loadMonitoredConns() {
		if c.id == v.ID {
			t.Fatalf("serial connection monitored")
		}
	}
}
