// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFunctionalityWorkerGroupsAndLogs(t *testing.T) {
	e := newDeployEnv(t, "runtime-health")
	writeFile(t, e.dir+"/run.py", runV110)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	writeFile(t, e.dir+"/config.ini", "[main]\n# Turn the feature on\nenabled = true\nport = 8080\npassword = \"do not expose this full phrase\"\n")
	// Two programs share one Supervisor file: only the matching stanza belongs to this install.
	writeFile(t, e.units+"/workers.conf", "[program:demo-workers]\ndirectory="+e.dir+"\ncommand=python run.py\nnumprocs=2\n[program:other]\ndirectory="+e.dir+"_old\ncommand=other\n")
	bin := t.TempDir()
	writeFile(t, bin+"/supervisorctl", `#!/bin/sh
if [ "$1" = tail ]; then echo 'error: failed queue; password=example-secret'; exit 0; fi
printf 'demo-workers:worker_00 RUNNING pid 100, uptime 1:00\ndemo-workers:worker_01 FATAL missing dependency\n'
exit 3
`)
	os.Chmod(bin+"/supervisorctl", 0755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	in := e.check(t)
	if len(in.Units) != 2 || in.Functionality == nil || in.Functionality.State != "failed" {
		t.Fatalf("runtime result: %+v", in)
	}
	for _, u := range in.Units {
		if u.Name == "other" {
			t.Fatal("unrelated Supervisor program associated")
		}
	}
	failed := false
	for _, c := range in.Functionality.Checks {
		if c.Name == "demo-workers:worker_01" {
			failed = true
			if !strings.Contains(c.Log, "failed queue") || strings.Contains(c.Log, "example-secret") {
				t.Fatalf("log: %+v", c)
			}
		}
	}
	if !failed {
		t.Fatal("healthy sibling hid the failed worker")
	}
	all := loadGitServerHealth(e.u.userID)
	if len(all) != 1 || all[0].State != "failed" {
		t.Fatalf("server health: %+v", all)
	}
	var conf struct {
		Files []gitConfigFile `json:"files"`
	}
	if code := e.u.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/configuration", in.ID), nil, &conf); code != 200 {
		t.Fatal(code)
	}
	raw := string(jsonMarshal(conf))
	if strings.Contains(raw, "do not expose") || !strings.Contains(raw, "Turn the feature on") || !strings.Contains(raw, "8080") {
		t.Fatalf("config: %s", raw)
	}
	// Gone auto-discovered installs stay visible and warn, rather than silently disappearing.
	os.RemoveAll(e.dir)
	e.u.jsonDo("POST", "/api/git/check", map[string]interface{}{"conn_ids": []int{e.conn}, "discover": true}, nil)
	list := loadGitInstalls(e.u.userID, true, "i.id=?", in.ID)
	if len(list) != 1 || list[0].State != "gone" || list[0].Functionality.State != "failed" {
		t.Fatalf("missing install hidden: %+v", list)
	}
}

func TestConfigurationRedactionAndBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"config.ini", "[main]\nAPI_KEY = 'phrase with spaces'\nurl=https://demo:secretpass@example.com\n[credentials]\nuser_secret = longsecret\n"},
		{"config.json", `{"token":{"nested":"hidden-value"},"server":{"port":9000,"enabled":true},"auth":["one","two"]}`},
		{"config.py", "PASSWORD = '''first secret line\nsecond secret line\n'''\n# Returns options\ndef options():\n    return {}\nENABLED = False\n"},
	} {
		f := parseGitConfig(tc.name, []byte(tc.data))
		b, _ := json.Marshal(f)
		for _, secret := range []string{"phrase with spaces", "secretpass", "longsecret", "hidden-value", "first secret", "second secret"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("secret leaked in %s: %s", tc.name, b)
			}
		}
	}
	e := newDeployEnv(t, "config-boundaries")
	writeFile(t, e.dir+"/run.py", runV110)
	writeFile(t, e.dir+"/lib/util.py", utilV1)
	outside := filepath.Join(t.TempDir(), "config.ini")
	writeFile(t, outside, "password=outside-secret")
	os.Symlink(outside, e.dir+"/linked.ini")
	cat := loadGitCatalog(e.u.userID)
	cat.Apps[0].Functionality = &gitFunctionalitySpec{ConfigFiles: []string{"linked.ini", "missing.ini"}}
	saveGitWorkspace(e.u.userID, cat, nil)
	in := e.check(t)
	cl, err := dialInstall(e.u.userID, in)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	// A symlink in an intermediate path also cannot escape the installation.
	os.Symlink(filepath.Dir(outside), e.dir+"/escape")
	sf, err := readServerFile(cl, e.dir, "escape/config.ini", 256<<10)
	if err != nil || !sf.Outside {
		t.Fatalf("escape: %+v %v", sf, err)
	}
	files, err := inspectGitConfigs(e.u.userID, in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonMarshal(files)), "outside-secret") {
		t.Fatal("symlink contents returned")
	}
	errors := map[string]string{}
	for _, f := range files {
		errors[f.Path] = f.Error
	}
	if errors["linked.ini"] == "" || errors["missing.ini"] == "" {
		t.Fatalf("problems not explicit: %v", errors)
	}
	other := newTestUser(t, e.u.srv, "config-foreign", false)
	if code := other.jsonDo("GET", fmt.Sprintf("/api/git/installs/%d/configuration", in.ID), nil, nil); code != 404 {
		t.Fatalf("foreign config: %d", code)
	}
}

func TestRuntimeResponsesAndProbeValidation(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	out := "C\tsupervisor\tworker\tfailed\t" + b64("FATAL") + "\t" + b64("token=secret-value\nWRM_RUNTIME_DONE\nC\tfake") + "\nWRM_RUNTIME_DONE\n"
	c, err := parseRuntimeOutput(out)
	if err != nil || len(c) != 1 || strings.Contains(c[0].Log, "secret-value") {
		t.Fatalf("response: %v %v", c, err)
	}
	if _, err := parseRuntimeOutput(strings.TrimSuffix(out, "WRM_RUNTIME_DONE\n")); err == nil {
		t.Fatal("truncated response accepted")
	}
	for _, s := range []gitFunctionalitySpec{{HTTP: "file:///etc/passwd"}, {HTTP: "http://user:pass@example.com/"}, {TCP: "example.com:70000"}, {ConfigFiles: []string{"../outside.ini"}}, {Units: []gitUnit{{Kind: "supervisor", Name: "--help"}}}} {
		if s.normalize() == nil {
			t.Fatalf("invalid spec accepted: %+v", s)
		}
	}
	if err := (&gitFunctionalitySpec{HTTP: "https://example.com/health", TCP: "localhost:8080", Units: []gitUnit{{Kind: "systemd", Name: "demo.service"}}}).normalize(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeUnitLimitIsUnverified(t *testing.T) {
	e := newDeployEnv(t, "runtime-limit")
	writeFile(t, e.dir+"/run.py", runV110)
	in := gitInstall{Path: e.dir, ConnID: e.conn, App: "demo-api"}
	cl, err := dialInstall(e.u.userID, in)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	d := gitInstallData{}
	for i := 0; i < 65; i++ {
		d.Units = append(d.Units, gitUnit{Kind: "systemd", Name: fmt.Sprintf("demo-%d.service", i)})
	}
	checkGitFunctionality(cl, in, nil, "ok", &d)
	if d.Functionality.State != "unknown" || !strings.Contains(strings.Join(d.Functionality.Warnings, " "), "limit reached") {
		t.Fatalf("partial check reported healthy: %+v", d.Functionality)
	}
}

func TestRuntimeHTTPAndTCPProbeSemantics(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "runtime-probes", false)
	conn := u.addConnection("probe-source", startTestSSHServer(t))
	c, err := loadConnection(conn)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := dialSSH(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	hits := make(chan string, 10)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.URL.Path
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(204)
		case "/redirect":
			w.Header().Set("Location", "/ok")
			w.WriteHeader(302)
		default:
			w.WriteHeader(503)
		}
	}))
	defer target.Close()
	if got := checkRuntimeEndpoint(cl, "http", target.URL+"/ok"); got.State != "healthy" || got.Status != "HTTP 204" {
		t.Fatalf("healthy HTTP: %+v", got)
	}
	if got := checkRuntimeEndpoint(cl, "http", target.URL+"/bad"); got.State != "failed" {
		t.Fatalf("failed HTTP: %+v", got)
	}
	if got := checkRuntimeEndpoint(cl, "http", target.URL+"/redirect"); got.State != "failed" || got.Status != "HTTP 302" {
		t.Fatalf("redirect followed: %+v", got)
	}
	if len(hits) != 3 {
		t.Fatalf("unexpected requests (possible redirect): %d", len(hits))
	}
	if got := checkRuntimeEndpoint(cl, "tcp", strings.TrimPrefix(target.URL, "http://")); got.State != "healthy" {
		t.Fatalf("TCP through SSH: %+v", got)
	}
	tlsTarget := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer tlsTarget.Close()
	if got := checkRuntimeEndpoint(cl, "http", tlsTarget.URL); got.State != "failed" {
		t.Fatalf("untrusted TLS accepted: %+v", got)
	}
}

func TestUnsupportedRuntimeNamesRemainUnverified(t *testing.T) {
	e := newDeployEnv(t, "runtime-names")
	writeFile(t, e.dir+"/run.py", runV110)
	in := gitInstall{Path: e.dir, ConnID: e.conn, App: "demo-api"}
	cl, err := dialInstall(e.u.userID, in)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	d := gitInstallData{Units: []gitUnit{{Kind: "systemd", Name: "demo.service"}, {Kind: "systemd", Name: `demo\x20worker.service`}}}
	checkGitFunctionality(cl, in, nil, "ok", &d)
	if d.Functionality.State != "unknown" || !strings.Contains(strings.Join(d.Functionality.Warnings, " "), "Unsupported") {
		t.Fatalf("unsupported unit hidden: %+v", d.Functionality)
	}
	bin := t.TempDir()
	writeFile(t, bin+"/supervisorctl", "#!/bin/sh\nprintf 'demo:worker_00 RUNNING pid 100\\ndemo:worker_é RUNNING pid 101\\n'\n")
	os.Chmod(bin+"/supervisorctl", 0755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	checkGitServerHealth(e.u.userID, e.conn, cl)
	list := loadGitServerHealth(e.u.userID)
	if len(list) != 1 || list[0].State != "unknown" {
		t.Fatalf("unsupported worker inventory hidden: %+v", list)
	}
}

func TestRuntimeRedactionOfQuotedSecrets(t *testing.T) {
	text := `worker failed: password="a full secret phrase"; API_KEY='another secret phrase'; {"token":"escaped \" secret value"}`
	clean := redactText(text)
	for _, value := range []string{"full secret phrase", "another secret phrase", "secret value"} {
		if strings.Contains(clean, value) {
			t.Fatalf("quoted secret partly leaked: %s", clean)
		}
	}
	if !strings.Contains(clean, "worker failed") {
		t.Fatal("error context lost")
	}
}

func TestConfigurationPrivateBlocksAndContinuations(t *testing.T) {
	q := strings.Repeat(string(rune(34)), 3)
	for _, data := range []string{
		"[tls]\nprivate_key = -----BEGIN PRIVATE KEY-----\nPRIVATECHUNKMATERIAL=\n-----END PRIVATE KEY-----\nport = 8080\n",
		"[main]\npassword = first\n  SECRETCONTINUATION=\nport = 8080\n",
		"PASSWORD = " + q + "\nPRIVATECHUNKMATERIAL=\n" + q + "\nport = 8080\n",
		"private_key = -----BEGIN PRIVATE KEY-----\nPRIVATECHUNKMATERIAL=\n",
	} {
		f := parseGitConfig("config.ini", []byte(data))
		out := string(jsonMarshal(f))
		if strings.Contains(out, "PRIVATECHUNKMATERIAL") || strings.Contains(out, "SECRETCONTINUATION") {
			t.Fatalf("private material became a field name: %s", out)
		}
		if strings.Contains(data, "-----BEGIN PRIVATE KEY-----") && !strings.Contains(data, "-----END PRIVATE KEY-----") && f.Error == "" {
			t.Fatal("unterminated private block not reported")
		}
		if strings.Contains(data, "8080") && !strings.Contains(out, "8080") {
			t.Fatalf("ordinary settings lost: %s", out)
		}
	}
	f := parseGitConfig("key.ini", []byte("PuTTY-User-Key-File-3: ssh-ed25519\nPrivate-Lines: 1\nPRIVATECHUNKMATERIAL=\nPrivate-MAC: example\n"))
	if strings.Contains(string(jsonMarshal(f)), "PRIVATECHUNKMATERIAL") || len(f.Entries) != 1 || !f.Entries[0].Secret {
		t.Fatalf("PuTTY key not masked: %+v", f)
	}
}

func TestConfigurationEscapedQuotesAndEntryCap(t *testing.T) {
	q := string(rune(34))
	slash := string(rune(92))
	data := "password = " + q + "first " + slash + q + " escaped quote\nPRIVATECHUNKMATERIAL=\n" + q + "\nport = 8080\n"
	f := parseGitConfig("config.ini", []byte(data))
	out := string(jsonMarshal(f))
	if strings.Contains(out, "PRIVATECHUNKMATERIAL") || !strings.Contains(out, "8080") {
		t.Fatalf("escaped quote ended masking early: %s", out)
	}
	f = parseGitConfig("config.ini", []byte(strings.Repeat("key=value\n", 2001)))
	if len(f.Entries) != 2000 || f.Error == "" {
		t.Fatalf("entry cap not explicit: %d %s", len(f.Entries), f.Error)
	}
	f = parseGitConfig("config.ini", []byte(strings.Repeat("-----BEGIN PRIVATE KEY-----MASKED-----END PRIVATE KEY-----\n", 2001)))
	if len(f.Entries) != 2000 || f.Error == "" {
		t.Fatalf("private block bypassed entry cap: %d %s", len(f.Entries), f.Error)
	}
}
