// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/ssh"
)

var monitorKeys = map[string]string{"agent": "agent.ping", "version": "agent.version", "hostname": "system.hostname", "cpu_idle": "system.cpu.util[,idle]", "cpu_count": "system.cpu.num", "load": "system.cpu.load[all,avg1]", "memory_total": "vm.memory.size[total]", "memory_available": "vm.memory.size[available]", "uptime": "system.uptime", "os": "system.uname", "filesystems": "vfs.fs.get", "interfaces": "net.if.discovery"}

func monitorTLS(cfg monitorConfig) (*tls.Config, error) {
	t := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.ServerName}
	if t.ServerName == "" {
		t.ServerName, _, _ = net.SplitHostPort(cfg.Address)
	}
	if cfg.CA != "" {
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(cfg.CA)) {
			return nil, fmt.Errorf("invalid agent CA certificate")
		}
		t.RootCAs = pool
	}
	if cfg.Certificate != "" || cfg.PrivateKey != "" {
		cert, err := tls.X509KeyPair([]byte(cfg.Certificate), []byte(cfg.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("invalid agent client certificate/key")
		}
		t.Certificates = []tls.Certificate{cert}
	}
	return t, nil
}
func zabbixFrame(data []byte) []byte {
	b := []byte{'Z', 'B', 'X', 'D', 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(b[5:9], uint32(len(data)))
	return append(b, data...)
}
func zabbixRead(r io.Reader) ([]byte, error) {
	h := make([]byte, 13)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, err
	}
	if string(h[:4]) != "ZBXD" || h[4]&^byte(3) != 0 || h[4]&1 == 0 {
		return nil, fmt.Errorf("invalid or unsupported Zabbix header")
	}
	n := binary.LittleEndian.Uint32(h[5:9])
	reserved := binary.LittleEndian.Uint32(h[9:13])
	if n > 1<<20 || reserved > 1<<20 {
		return nil, fmt.Errorf("Zabbix response exceeds 1 MiB")
	}
	b := make([]byte, int(n))
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	if h[4]&2 != 0 {
		z, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer z.Close()
		b, err = io.ReadAll(io.LimitReader(z, (1<<20)+1))
		if err != nil || len(b) > 1<<20 || len(b) != int(reserved) {
			return nil, fmt.Errorf("invalid compressed Zabbix response")
		}
	}
	return b, nil
}
func zabbixValue(b []byte) (string, bool, error) {
	var res struct {
		Data []struct {
			Value json.RawMessage `json:"value"`
			Error string          `json:"error"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &res) == nil && len(res.Data) == 1 {
		d := res.Data[0]
		if d.Error != "" {
			return "", true, fmt.Errorf("%s", truncateStr(d.Error, 500))
		}
		var s string
		if json.Unmarshal(d.Value, &s) == nil {
			return s, true, nil
		}
		if len(d.Value) > 0 {
			return string(d.Value), true, nil
		}
		return "", true, fmt.Errorf("agent returned no value")
	}
	text := strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", ": "))
	if strings.HasPrefix(text, "ZBX_NOTSUPPORTED") {
		return "", false, fmt.Errorf("%s", truncateStr(text, 500))
	}
	return text, false, nil
}
func monitorSSHCommand(ctx context.Context, cl *ssh.Client, cmd string) (string, error) {
	return monitorSSHCommandInput(ctx, cl, cmd, "")
}

// monitorSSHCommandInput runs cmd with input on its standard input, so text a
// user typed (a search pattern) reaches the remote shell as data and never as
// part of the command line.
func monitorSSHCommandInput(ctx context.Context, cl *ssh.Client, cmd, input string) (string, error) {
	sess, err := cl.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	if input != "" {
		sess.Stdin = strings.NewReader(input)
	}
	buf := &lockedBuf{buf: capBuffer{max: 1 << 20}}
	sess.Stdout = buf
	sess.Stderr = buf
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		sess.Close()
		err = ctx.Err()
	}
	buf.mu.Lock()
	defer buf.mu.Unlock()
	return buf.buf.String(), err
}
func collectMonitor(ctx context.Context, c Connection, cfg monitorConfig) monitorSample {
	s := monitorSample{ConnID: c.ID, Hostname: hostOnly(c.Host), At: nowRFC(), State: "unknown", Metrics: map[string]monitorMetric{}}
	if cfg.Mode == "api" {
		err := monitorAPI(ctx, cfg, &s)
		if err != nil {
			s.Error = err.Error()
			s.State = "failed"
		}
		finishMonitor(&s)
		return s
	}
	var cl *ssh.Client
	var route *targetRoute
	var err error
	if cfg.Mode == "ssh" {
		cl, err = dialSSH(c, nil)
		if cl != nil {
			defer cl.Close()
		}
	} else if needsRoute(c) {
		route, err = openTargetRoute(c)
		if route != nil {
			defer route.Close()
		}
	}
	if err != nil {
		s.State = "failed"
		s.Error = "Monitoring route unavailable: " + err.Error()
		return s
	}
	protocol := "json"
	query := func(key string) (string, error) {
		qctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		if cfg.TLS == "psk-helper" {
			host, port, _ := net.SplitHostPort(cfg.Address)
			cmd := "zabbix_get -s " + shellQuote(host) + " -p " + shellQuote(port) + " -t 5 --tls-connect psk --tls-psk-identity " + shellQuote(cfg.PSKIdentity) + " --tls-psk-file " + shellQuote(cfg.PSKFile) + " -k " + shellQuote(key)
			out, e := monitorSSHCommand(qctx, cl, cmd)
			if e != nil {
				return "", fmt.Errorf("remote zabbix_get failed; check installed helper, PSK file permissions, agent allowlist and TLS identity")
			}
			v, _, e := zabbixValue([]byte(out))
			return v, e
		}
		exchange := func(payload []byte) ([]byte, error) {
			var conn net.Conn
			var e error
			if cl != nil {
				conn, e = cl.DialContext(qctx, "tcp", cfg.Address)
			} else if route != nil {
				conn, e = route.Dial("tcp", cfg.Address)
			} else {
				conn, e = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(qctx, "tcp", cfg.Address)
			}
			if e != nil {
				return nil, e
			}
			defer conn.Close()
			stop := context.AfterFunc(qctx, func() { conn.Close() })
			defer stop()
			conn.SetDeadline(time.Now().Add(7 * time.Second))
			if cfg.TLS == "certificate" {
				t, e := monitorTLS(cfg)
				if e != nil {
					return nil, e
				}
				tc := tls.Client(conn, t)
				if e = tc.HandshakeContext(qctx); e != nil {
					return nil, fmt.Errorf("agent TLS verification failed: %w", e)
				}
				conn = tc
			}
			if _, e = conn.Write(zabbixFrame(payload)); e != nil {
				return nil, e
			}
			return zabbixRead(conn)
		}
		payload := []byte(key + "\n")
		if protocol == "json" {
			payload = jsonMarshal(map[string]interface{}{"request": "passive checks", "data": []map[string]interface{}{{"key": key, "timeout": 5}}})
		}
		b, e := exchange(payload)
		if e != nil {
			return "", e
		}
		v, isJSON, e := zabbixValue(b)
		if protocol == "json" && !isJSON && strings.Contains(string(b), "ZBX_NOTSUPPORTED") {
			protocol = "legacy"
			b, e = exchange([]byte(key + "\n"))
			if e != nil {
				return "", e
			}
			v, _, e = zabbixValue(b)
		}
		return v, e
	}
	ping, err := query("agent.ping")
	if err != nil || strings.TrimSpace(ping) != "1" {
		s.State = "failed"
		if err != nil {
			s.Error = "Agent unavailable: " + err.Error()
		} else {
			s.Error = "Agent did not confirm agent.ping"
		}
		return s
	}
	s.Metrics["agent"] = metricNumber(ping, s.At)
	discovery := map[string]string{}
	for name, key := range monitorKeys {
		if name == "agent" {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		raw, e := query(key)
		if e != nil {
			s.Metrics[name] = monitorMetric{Error: e.Error(), At: s.At}
			continue
		}
		if name == "filesystems" || name == "interfaces" {
			discovery[name] = raw
			continue
		}
		s.Metrics[name] = metricNumber(raw, s.At)
	}
	if h := s.Metrics["hostname"].Text; h != "" {
		s.Hostname = truncateStr(h, 200)
	}
	// Preserve discovery errors separately; never display a partial discovery as zero.
	if raw := discovery["filesystems"]; raw != "" {
		monitorFilesystems(raw, s.Metrics, s.At)
	}
	if raw := discovery["interfaces"]; raw != "" {
		var entries []map[string]interface{}
		if json.Unmarshal([]byte(raw), &entries) != nil {
			var wrapper struct {
				Data []map[string]interface{} `json:"data"`
			}
			json.Unmarshal([]byte(raw), &wrapper)
			entries = wrapper.Data
		}
		for i, x := range entries {
			if i >= 16 {
				break
			}
			name, _ := x["{#IFNAME}"].(string)
			if !monitorInterfaceName(name) {
				continue
			}
			for _, direction := range []string{"in", "out"} {
				key := "net.if." + direction + "[" + string(jsonMarshal(name)) + ",bytes]"
				raw, e := query(key)
				if e != nil {
					s.Metrics["network:"+name+":"+direction] = monitorMetric{Error: e.Error(), At: s.At}
				} else {
					s.Metrics["network:"+name+":"+direction] = metricNumber(raw, s.At)
				}
			}
		}
	}
	delete(s.Metrics, "filesystems")
	delete(s.Metrics, "interfaces")
	finishMonitor(&s)
	// Service state is read over the same SSH connection. direct/api transports
	// have no shell, so a service list only applies to the ssh transport.
	if cl != nil && len(cfg.Services) > 0 {
		collectMonitorServices(ctx, cl, cfg.Services, &s)
	}
	return s
}

func monitorInterfaceName(name string) bool {
	return name != "" && len(name) <= 256 && strings.IndexFunc(name, unicode.IsControl) < 0
}

func monitorFilesystems(raw string, metrics map[string]monitorMetric, at string) {
	var list []struct {
		FSName string `json:"fsname"`
		Bytes  struct {
			Total float64 `json:"total"`
			Used  float64 `json:"used"`
			PUsed float64 `json:"pused"`
		} `json:"bytes"`
	}
	if json.Unmarshal([]byte(raw), &list) != nil {
		return
	}
	for i, x := range list {
		if i >= 32 {
			break
		}
		v := x.Bytes.PUsed
		if x.Bytes.Total > 0 && v == 0 {
			v = x.Bytes.Used / x.Bytes.Total * 100
		}
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			metrics["disk:"+truncateStr(x.FSName, 100)] = monitorMetric{Value: &v, At: at}
		}
	}
}
func finishMonitor(s *monitorSample) {
	if s.Error != "" {
		return
	}
	s.State = "healthy"
	for _, name := range []string{"cpu_idle", "memory_total", "memory_available", "uptime"} {
		m, ok := s.Metrics[name]
		if name == "cpu_idle" && s.Metrics["cpu_percent"].Value != nil {
			continue
		}
		if !ok || m.Value == nil || m.Error != "" {
			s.State = "warning"
		}
	}
	if m := s.Metrics["cpu_idle"]; m.Value != nil {
		v := 100 - *m.Value
		s.Metrics["cpu_percent"] = monitorMetric{Value: &v, At: m.At}
		if v > 90 {
			s.State = "warning"
		}
	}
	total, available := s.Metrics["memory_total"], s.Metrics["memory_available"]
	if total.Value != nil && available.Value != nil && *total.Value > 0 {
		v := 100 * (1 - *available.Value / *total.Value)
		s.Metrics["memory_percent"] = monitorMetric{Value: &v, At: total.At}
		if v > 90 {
			s.State = "warning"
		}
	}
	for k, m := range s.Metrics {
		if strings.HasPrefix(k, "disk:") && m.Value != nil && *m.Value > 90 {
			s.State = "warning"
		}
		if m.At != "" {
			at, e := time.Parse(time.RFC3339, m.At)
			if e == nil && time.Since(at) > 5*time.Minute {
				s.State = "warning"
			}
		}
	}
}
func addMonitorNetworkRates(s *monitorSample, old monitorSample) {
	at, _ := time.Parse(time.RFC3339, s.At)
	before, _ := time.Parse(time.RFC3339, old.At)
	seconds := at.Sub(before).Seconds()
	if seconds <= 0 || seconds > 7200 {
		return
	}
	for k, m := range s.Metrics {
		if !strings.HasPrefix(k, "network:") || m.Value == nil {
			continue
		}
		previous := old.Metrics[k]
		if previous.Value != nil && *m.Value >= *previous.Value {
			v := (*m.Value - *previous.Value) / seconds
			s.Metrics["rate:"+k] = monitorMetric{Value: &v, At: s.At}
		}
	}
}
func monitorAPI(ctx context.Context, cfg monitorConfig, s *monitorSample) error {
	payload := jsonMarshal(map[string]interface{}{"jsonrpc": "2.0", "method": "item.get", "id": 1, "params": map[string]interface{}{"hostids": []string{cfg.APIHost}, "output": []string{"key_", "lastvalue", "lastclock", "state", "error", "units"}, "limit": 500}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("invalid Zabbix API URL")
	}
	req.Header.Set("Content-Type", "application/json-rpc")
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	t, err := monitorTLS(cfg)
	if err != nil {
		return err
	}
	u, _ := url.Parse(cfg.APIURL)
	t.ServerName = u.Hostname()
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = t
	defer tr.CloseIdleConnections()
	client := &http.Client{Timeout: 30 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Zabbix API unavailable or TLS verification failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Zabbix API answered HTTP %d", resp.StatusCode)
	}
	var result struct {
		Result []struct {
			Key   string `json:"key_"`
			Value string `json:"lastvalue"`
			Clock string `json:"lastclock"`
			State string `json:"state"`
			Error string `json:"error"`
			Units string `json:"units"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil || len(result.Error) > 0 {
		return fmt.Errorf("Zabbix API refused item.get; check token, host id and permissions")
	}
	reverse := map[string]string{}
	for name, key := range monitorKeys {
		reverse[key] = name
	}
	for _, item := range result.Result {
		name := reverse[item.Key]
		if name == "" && item.Key == "system.cpu.util" {
			name = "cpu_percent"
		}
		if name == "" && (strings.HasPrefix(item.Key, "net.if.in[") || strings.HasPrefix(item.Key, "net.if.out[")) && oneOf(item.Units, "Bps", "B/s") {
			name = "rate:" + item.Key
		}
		if name == "" {
			if strings.HasPrefix(item.Key, "vfs.fs.size[") && strings.HasSuffix(item.Key, ",pused]") {
				name = "disk:" + strings.TrimSuffix(strings.TrimPrefix(item.Key, "vfs.fs.size["), ",pused]")
			} else {
				continue
			}
		}
		clock, _ := strconv.ParseInt(item.Clock, 10, 64)
		at := time.Unix(clock, 0).UTC().Format(time.RFC3339)
		if item.State != "0" {
			s.Metrics[name] = monitorMetric{Error: truncateStr(item.Error, 500), At: at}
		} else {
			s.Metrics[name] = metricNumber(item.Value, at)
		}
	}
	if len(s.Metrics) == 0 {
		return fmt.Errorf("No supported items are visible for this host; configure the Zabbix agent template and read permissions")
	}
	if h := s.Metrics["hostname"].Text; h != "" {
		s.Hostname = truncateStr(h, 200)
	}
	return nil
}
