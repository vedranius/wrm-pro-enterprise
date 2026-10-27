// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestStatusMonitor(t *testing.T) {
	sshAddr := startTestSSHServer(t)
	// a port nobody listens on
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l.Addr().String()
	l.Close()

	srv := httptest.NewServer(newRouter())
	defer srv.Close()
	user := newTestUser(t, srv, "status-user", false)
	other := newTestUser(t, srv, "status-other", false)
	up := user.addConnection("up-01", sshAddr)
	down := user.addConnection("down-01", closed)
	behind := user.addConnectionVia("behind-01", sshAddr, up)
	behindDown := user.addConnectionVia("behind-down", sshAddr, down)
	off := user.addConnection("not-monitored", closed)
	db.Exec(`UPDATE connections SET monitor=0 WHERE id=?`, off)
	theirs := other.addConnection("theirs", sshAddr)

	setSetting("status_jump_checks", "0")
	statusMon.round()

	var st struct {
		Enabled     bool                  `json:"enabled"`
		Connections map[string]hostStatus `json:"connections"`
	}
	if code := user.jsonDo("GET", "/api/status", nil, &st); code != 200 || !st.Enabled {
		t.Fatalf("status: %d %+v", code, st)
	}
	get := func(id int) hostStatus { return st.Connections[strconv.Itoa(id)] }
	if s := get(up); s.State != "up" || !strings.HasPrefix(s.Banner, "SSH-2.0-") || s.Since == "" {
		t.Fatalf("up connection: %+v", s)
	}
	if s := get(down); s.State != "down" || s.Error != "connection refused" {
		t.Fatalf("down connection: %+v", s)
	}
	if s := get(behind); s.State != "unknown" || s.Via != "up-01" || s.ViaState != "up" {
		t.Fatalf("connection behind an up jump host: %+v", s)
	}
	if s := get(behindDown); s.State != "down" || s.Via != "down-01" {
		t.Fatalf("connection behind a down jump host: %+v", s)
	}
	if _, ok := st.Connections[strconv.Itoa(off)]; ok {
		t.Fatalf("connection with monitoring off is reported")
	}
	if _, ok := st.Connections[strconv.Itoa(theirs)]; ok {
		t.Fatalf("another user's connection is reported")
	}
	// one check per host:port, not per connection: both users' connections share the result
	statusMon.mu.Lock()
	k1, k2 := statusMon.conns[up], statusMon.conns[theirs]
	statusMon.mu.Unlock()
	if k1 == "" || k1 != k2 {
		t.Fatalf("connections to the same host:port use different checks: %q %q", k1, k2)
	}

	// checks through the jump host
	setSetting("status_jump_checks", "1")
	defer setSetting("status_jump_checks", "0")
	statusMon.round()
	user.jsonDo("GET", "/api/status", nil, &st)
	if s := get(behind); s.State != "up" || s.Via != "up-01" || !strings.HasPrefix(s.Banner, "SSH-2.0-") {
		t.Fatalf("checked through the jump host: %+v", s)
	}
	if s := get(behindDown); s.State != "unknown" || s.ViaState != "down" {
		t.Fatalf("jump host down: %+v", s)
	}

	// check now: own connections only, through jump hosts, rate limited
	var now struct {
		Connections map[string]hostStatus `json:"connections"`
	}
	if code := user.jsonDo("POST", "/api/status/check", map[string]interface{}{"ids": []int{up, down, behind, theirs}}, &now); code != 200 {
		t.Fatalf("check now: %d", code)
	}
	if len(now.Connections) != 3 || now.Connections[strconv.Itoa(up)].State != "up" || now.Connections[strconv.Itoa(behind)].State != "up" ||
		now.Connections[strconv.Itoa(down)].State != "down" {
		t.Fatalf("check now result: %+v", now.Connections)
	}
	if code := user.jsonDo("POST", "/api/status/check", map[string]interface{}{"ids": []int{up}}, &map[string]interface{}{}); code != 429 {
		t.Fatalf("check now not rate limited: %d", code)
	}

	// turning monitoring on again through the API, and the flag in the connection view
	var cv connView
	if code := user.jsonDo("PUT", "/api/connections/"+strconv.Itoa(off), map[string]interface{}{"name": "not-monitored", "protocol": "SSH", "host": closed,
		"username": "x", "auth_method": "PASSWORD", "monitor": true}, nil); code != 200 {
		t.Fatalf("update monitor flag: %d", code)
	}
	var list []connView
	user.jsonDo("GET", "/api/connections", nil, &list)
	for _, c := range list {
		if c.ID == off {
			cv = c
		}
	}
	if !cv.Monitor {
		t.Fatalf("monitor flag not saved: %+v", cv)
	}
}

func TestProbeFTPGreeting(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan string, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("220 test FTP ready\r\n"))
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		got <- string(buf[:n])
	}()
	r := probe(statusTarget{addr: l.Addr().String(), proto: "FTP"}, net.Dial)
	if r.State != "up" || r.Banner != "220 test FTP ready" {
		t.Fatalf("ftp probe: %+v", r)
	}
	if q := <-got; q != "QUIT\r\n" {
		t.Fatalf("ftp probe did not quit: %q", q)
	}
}
