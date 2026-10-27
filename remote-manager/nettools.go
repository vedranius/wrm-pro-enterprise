// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── NETWORK TOOLS ───────────────────────────────────
//
// Port check, ping, traceroute, DNS lookup and an HTTP/TLS check — from the WRM server or
// from one of the user's SSH connections (through its jump hosts): "can app-01 reach the
// database on 5432?". Port checks and HTTP checks from a server use SSH channels
// (direct-tcpip), so nothing has to be installed there; ping, traceroute and DNS run the
// usual commands on it.

const maxCheckPorts = 100

var toolHostRe = regexp.MustCompile(`^[A-Za-z0-9_.:\[\]-]{1,253}$`)

var toolsRunning = struct {
	sync.Mutex
	m map[int]bool
}{m: map[int]bool{}}

func networkToolsAllowed(userID int) bool {
	return policyWhoAllows(userID, "network_tools")
}

func validToolHost(h string) error {
	h = strings.TrimSpace(h)
	if !toolHostRe.MatchString(h) || strings.HasPrefix(h, "-") {
		return fmt.Errorf("enter a host name or IP address")
	}
	return nil
}

// parsePorts reads "22, 80,443 8000-8010".
func parsePorts(s string) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		lo, hi := part, part
		if i := strings.Index(part, "-"); i > 0 {
			lo, hi = part[:i], part[i+1:]
		}
		a, err1 := strconv.Atoi(lo)
		b, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || a < 1 || b > 65535 || a > b {
			return nil, fmt.Errorf("invalid port %q", part)
		}
		for p := a; p <= b; p++ {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
				if len(out) > maxCheckPorts {
					return nil, fmt.Errorf("at most %d ports at once", maxCheckPorts)
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("enter one or more ports, e.g. 22, 443, 8000-8010")
	}
	return out, nil
}

type portResult struct {
	Port  int    `json:"port"`
	State string `json:"state"` // open | closed | filtered | error
	Ms    int64  `json:"ms"`
	Info  string `json:"info,omitempty"`
}

func classifyDialError(err error) (string, string) {
	if err == nil {
		return "open", ""
	}
	msg := err.Error()
	var ne net.Error
	switch {
	case strings.Contains(msg, "refused"):
		return "closed", "connection refused"
	case errors.As(err, &ne) && ne.Timeout(), strings.Contains(msg, "timed out"), strings.Contains(msg, "timeout"):
		return "filtered", "no answer (filtered?)"
	case strings.Contains(msg, "no route") || strings.Contains(msg, "unreachable"):
		return "filtered", "unreachable"
	}
	return "error", shortNetError(err)
}

// checkPorts connects to host:port for every port, from WRM (cl == nil) or through cl.
func checkPorts(cl *ssh.Client, host string, ports []int) []portResult {
	res := make([]portResult, len(ports))
	sem := make(chan struct{}, 20)
	var wg sync.WaitGroup
	for i, p := range ports {
		wg.Add(1)
		go func(i, p int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			addr := net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(p))
			start := time.Now()
			var err error
			if cl == nil {
				var c net.Conn
				if c, err = net.DialTimeout("tcp", addr, 3*time.Second); err == nil {
					c.Close()
				}
			} else {
				done := make(chan error, 1)
				go func() {
					c, e := cl.Dial("tcp", addr)
					if e == nil {
						c.Close()
					}
					done <- e
				}()
				select {
				case err = <-done:
				case <-time.After(5 * time.Second):
					err = fmt.Errorf("i/o timeout")
				}
			}
			st, info := classifyDialError(err)
			res[i] = portResult{Port: p, State: st, Ms: time.Since(start).Milliseconds(), Info: info}
		}(i, p)
	}
	wg.Wait()
	return res
}

// localCommand runs a program on the WRM server and returns its combined output.
func localCommand(timeout time.Duration, name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is not installed on the WRM server", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("stopped after %s", timeout)
	}
	if err != nil && len(out) == 0 {
		return "", err
	}
	return string(out), nil // ping exits 1 when nothing answers: the output tells
}

func pingArgs(host string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"-n", "4", "-w", "2000", host}
	case "linux", "android":
		return []string{"-c", "4", "-W", "2", host}
	}
	return []string{"-c", "4", host}
}

func runPing(cl *ssh.Client, host string) (string, error) {
	if cl == nil {
		return localCommand(25*time.Second, "ping", pingArgs(host)...)
	}
	q := shellQuote(host)
	out, err := runRemoteScript(cl, `command -v ping >/dev/null 2>&1 || { echo "ping is not installed on this server"; exit 127; }
ping -c 4 -W 2 `+q+` 2>&1 || ping -c 4 `+q+` 2>&1; exit 0`, "", 30*time.Second)
	return out, err
}

func runTraceroute(cl *ssh.Client, host string) (string, error) {
	if cl == nil {
		if runtime.GOOS == "windows" {
			return localCommand(90*time.Second, "tracert", "-d", "-w", "2000", "-h", "20", host)
		}
		if _, err := exec.LookPath("traceroute"); err == nil {
			return localCommand(90*time.Second, "traceroute", "-n", "-w", "2", "-q", "1", "-m", "20", host)
		}
		return localCommand(90*time.Second, "tracepath", "-n", host)
	}
	q := shellQuote(host)
	return runRemoteScript(cl, `if command -v traceroute >/dev/null 2>&1; then traceroute -n -w 2 -q 1 -m 20 `+q+` 2>&1
elif command -v tracepath >/dev/null 2>&1; then tracepath -n `+q+` 2>&1
else echo "neither traceroute nor tracepath is installed on this server"; exit 127; fi; exit 0`, "", 90*time.Second)
}

type dnsResult struct {
	Addresses []string `json:"addresses,omitempty"`
	CNAME     string   `json:"cname,omitempty"`
	MX        []string `json:"mx,omitempty"`
	TXT       []string `json:"txt,omitempty"`
	PTR       []string `json:"ptr,omitempty"`
	Error     string   `json:"error,omitempty"`
}

func runDNS(cl *ssh.Client, host string) (*dnsResult, string, error) {
	h := strings.Trim(host, "[]")
	if cl != nil {
		q := shellQuote(h)
		out, err := runRemoteScript(cl, `if command -v getent >/dev/null 2>&1; then getent ahosts `+q+` | awk '{print $1}' | sort -u; fi
if command -v dig >/dev/null 2>&1; then echo "--- dig"; dig +noall +answer `+q+` A `+q+` AAAA `+q+` MX 2>&1
elif command -v nslookup >/dev/null 2>&1; then echo "--- nslookup"; nslookup `+q+` 2>&1; fi; exit 0`, "", 30*time.Second)
		return nil, out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r := &dnsResult{}
	var res net.Resolver
	if ip := net.ParseIP(h); ip != nil {
		names, err := res.LookupAddr(ctx, h)
		if err != nil {
			r.Error = shortNetError(err)
		}
		r.PTR = names
		return r, "", nil
	}
	addrs, err := res.LookupIPAddr(ctx, h)
	if err != nil {
		r.Error = shortNetError(err)
	}
	for _, a := range addrs {
		r.Addresses = append(r.Addresses, a.IP.String())
	}
	if cn, err := res.LookupCNAME(ctx, h); err == nil && strings.TrimSuffix(cn, ".") != strings.TrimSuffix(h, ".") {
		r.CNAME = cn
	}
	if mx, err := res.LookupMX(ctx, h); err == nil {
		for _, m := range mx {
			r.MX = append(r.MX, fmt.Sprintf("%d %s", m.Pref, m.Host))
		}
	}
	if txt, err := res.LookupTXT(ctx, h); err == nil {
		for _, t := range txt {
			r.TXT = append(r.TXT, truncateStr(t, 300))
		}
	}
	return r, "", nil
}

type httpResult struct {
	URL        string   `json:"url"`
	Status     int      `json:"status"`
	StatusText string   `json:"status_text"`
	Ms         int64    `json:"ms"`
	Server     string   `json:"server,omitempty"`
	Location   string   `json:"location,omitempty"`
	TLS        string   `json:"tls,omitempty"`
	Subject    string   `json:"subject,omitempty"`
	Issuer     string   `json:"issuer,omitempty"`
	Names      []string `json:"names,omitempty"`
	NotAfter   string   `json:"not_after,omitempty"`
	DaysLeft   int      `json:"days_left,omitempty"`
	Trusted    bool     `json:"trusted"`
	TrustError string   `json:"trust_error,omitempty"`
}

// runHTTPCheck requests the URL once (no redirects) and describes the answer and the certificate.
func runHTTPCheck(cl *ssh.Client, raw string) (*httpResult, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || validToolHost(u.Hostname()) != nil {
		return nil, fmt.Errorf("enter a URL like https://10.0.0.5/ or a host name")
	}
	var state *tls.ConnectionState
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, //nolint:gosec // the certificate is checked and reported below
		VerifyConnection: func(cs tls.ConnectionState) error { state = &cs; return nil }},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, DisableKeepAlives: true}
	if cl != nil {
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) { return cl.Dial("tcp", addr) }
	} else {
		tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	hc := &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := time.Now()
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("User-Agent", "WRM-check/"+AppVersion)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", shortNetError(err))
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	r := &httpResult{URL: u.String(), Status: resp.StatusCode, StatusText: http.StatusText(resp.StatusCode), Ms: time.Since(start).Milliseconds(),
		Server: resp.Header.Get("Server"), Location: resp.Header.Get("Location")}
	if state != nil && len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		r.TLS = map[uint16]string{tls.VersionTLS10: "TLS 1.0", tls.VersionTLS11: "TLS 1.1", tls.VersionTLS12: "TLS 1.2", tls.VersionTLS13: "TLS 1.3"}[state.Version] + " · " + tls.CipherSuiteName(state.CipherSuite)
		r.Subject, r.Issuer = leaf.Subject.String(), leaf.Issuer.String()
		r.Names = leaf.DNSNames
		if len(r.Names) > 8 {
			r.Names = append(r.Names[:8], fmt.Sprintf("… +%d", len(leaf.DNSNames)-8))
		}
		r.NotAfter = leaf.NotAfter.UTC().Format(time.RFC3339)
		r.DaysLeft = int(time.Until(leaf.NotAfter).Hours() / 24)
		inter := x509.NewCertPool()
		for _, c := range state.PeerCertificates[1:] {
			inter.AddCert(c)
		}
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: u.Hostname(), Intermediates: inter}); err != nil {
			r.TrustError = truncateStr(err.Error(), 200)
		} else {
			r.Trusted = true
		}
	}
	return r, nil
}

// POST /api/nettools {tool: ports|ping|traceroute|dns|http, target, ports, source_id}
func apiNetToolsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if !networkToolsAllowed(userID) {
		jsonError(w, "Network tools are turned off for this account (policy network_tools)", 403)
		return
	}
	var in struct {
		Tool     string `json:"tool"`
		Target   string `json:"target"`
		Ports    string `json:"ports"`
		SourceID int    `json:"source_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	in.Target = strings.TrimSpace(in.Target)
	if in.Tool != "http" {
		if h, p, err := net.SplitHostPort(in.Target); err == nil {
			in.Target = h
			if in.Tool == "ports" && strings.TrimSpace(in.Ports) == "" {
				in.Ports = p
			}
		}
		if err := validToolHost(in.Target); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
	}
	var ports []int
	if in.Tool == "ports" {
		var err error
		if ports, err = parsePorts(in.Ports); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
	}
	toolsRunning.Lock()
	if toolsRunning.m[userID] {
		toolsRunning.Unlock()
		jsonError(w, "Another check of yours is still running", 429)
		return
	}
	toolsRunning.m[userID] = true
	toolsRunning.Unlock()
	defer func() { toolsRunning.Lock(); delete(toolsRunning.m, userID); toolsRunning.Unlock() }()

	source := "WRM server"
	var cl *ssh.Client
	if in.SourceID > 0 {
		if !userOwnsConnection(in.SourceID, userID) {
			jsonError(w, "Connection not found", 404)
			return
		}
		c, err := loadConnection(in.SourceID)
		if err != nil || !isSSHProtocol(c) {
			jsonError(w, "The source must be an SSH connection", 400)
			return
		}
		if cl, err = dialSSH(c, nil); err != nil {
			jsonError(w, "Cannot log in to "+c.Name+": "+err.Error(), 502)
			return
		}
		defer cl.Close()
		source = c.Name
	}
	start := time.Now()
	out := map[string]interface{}{"tool": in.Tool, "target": in.Target, "source": source}
	var err error
	switch in.Tool {
	case "ports":
		res := checkPorts(cl, in.Target, ports)
		sort.Slice(res, func(i, j int) bool { return res[i].Port < res[j].Port })
		out["ports"] = res
	case "ping":
		out["output"], err = runPing(cl, in.Target)
	case "traceroute":
		out["output"], err = runTraceroute(cl, in.Target)
	case "dns":
		var res *dnsResult
		var text string
		res, text, err = runDNS(cl, in.Target)
		out["dns"], out["output"] = res, text
	case "http":
		out["http"], err = runHTTPCheck(cl, in.Target)
	default:
		jsonError(w, "Unknown tool", 400)
		return
	}
	out["ms"] = time.Since(start).Milliseconds()
	auditLog(r, userID, "nettool.run", in.Target, map[string]interface{}{"tool": in.Tool, "source": source, "ports": len(ports)})
	if err != nil {
		out["error"] = err.Error()
	}
	jsonOK(w, out)
}
