// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── JUMP HOSTS (ProxyJump) ──────────────────────────
//
// A connection can be reached through another saved SSH connection, the "jump host"
// (bastion). That one can have its own jump host, and so on (up to maxJumpDepth hops):
//
//   WRM ──SSH──► jump 1 ──SSH (inside)──► jump 2 ──SSH (inside)──► target
//
// Like OpenSSH's ProxyJump, WRM logs in to the first hop, opens a direct-tcpip channel
// through it to the next hop and runs a new, separately encrypted SSH handshake inside
// that channel. Every hop is authenticated with its own credentials and its host key is
// verified. No port is opened anywhere. Terminals, file manager, search, server-to-server
// transfers, tunnels and connection tests all use dialSSH, so they all work through jump
// hosts. Closing the target client closes the whole chain.

const maxJumpDepth = 5

// jumpChain returns the jump hosts of c, outermost (first to dial) first.
func jumpChain(c Connection) ([]Connection, error) {
	var chain []Connection
	seen := map[int]bool{}
	if c.ID > 0 {
		seen[c.ID] = true
	}
	next := c.JumpID
	for next != nil && *next > 0 {
		if len(chain) >= maxJumpDepth {
			return nil, fmt.Errorf("the jump host chain of %q has more than %d hops", c.Name, maxJumpDepth)
		}
		if seen[*next] {
			return nil, fmt.Errorf("the jump host chain of %q is a loop", c.Name)
		}
		seen[*next] = true
		j, err := loadConnection(*next)
		if err != nil {
			return nil, fmt.Errorf("the jump host of %q no longer exists", c.Name)
		}
		if c.UserID > 0 && j.UserID != c.UserID {
			return nil, fmt.Errorf("the jump host of %q belongs to another user", c.Name)
		}
		if !isSSHProtocol(j) {
			return nil, fmt.Errorf("jump host %q is not an SSH connection", j.Name)
		}
		chain = append([]Connection{j}, chain...)
		next = j.JumpID
	}
	return chain, nil
}

// jumpPath describes the route to c for logs and the UI: its jump hosts and proxies,
// e.g. "bastion → socks5://10.1.1.1:1080" (the target itself is not included).
func jumpPath(c Connection) string {
	chain, err := jumpChain(c)
	if err != nil {
		return ""
	}
	var parts []string
	for i, hop := range append(chain, c) {
		if hop.ProxyID != nil && *hop.ProxyID > 0 {
			if p, err := loadProxy(*hop.ProxyID); err == nil {
				parts = append(parts, p.URL)
			} else {
				parts = append(parts, "proxy?")
			}
		}
		if i < len(chain) {
			parts = append(parts, hop.Name)
		}
	}
	return strings.Join(parts, " → ")
}

// fullRoute is jumpPath with the target, e.g. "bastion → socks5://10.1.1.1:1080 → app-01".
func fullRoute(c Connection) string {
	if r := jumpPath(c); r != "" {
		return r + " → " + c.Name
	}
	return c.Name
}

func isSSHProtocol(c Connection) bool {
	p := strings.ToUpper(c.Protocol)
	return p == "SSH" || p == "SFTP" || p == ""
}

// dialSSH opens an SSH client to c, through its jump hosts when configured.
// onNewKey (optional) is told about host keys trusted on first use, with the host name.
func dialSSH(c Connection, onNewKey func(host, fp string)) (*ssh.Client, error) {
	chain, err := jumpChain(c)
	if err != nil {
		return nil, err
	}
	hops := append(chain, c)
	var opened []*ssh.Client
	closeAll := func() {
		for i := len(opened) - 1; i >= 0; i-- {
			opened[i].Close()
		}
	}
	for i, hop := range hops {
		hop.Host = ensurePort(hop.Host, hop.Protocol)
		if usesStoredSecretRef(hop.AuthMethod) && hop.authErr == "" {
			resolveConnectionAuth(&hop) // also sets the user name of a credential
		}
		am, err := buildAuthMethods(hop)
		if err != nil {
			closeAll()
			return nil, hopError(hop, i < len(hops)-1, err)
		}
		host := hop.Host
		cfg := sshClientConfig(hop, am, func(fp string) {
			if onNewKey != nil {
				onNewKey(host, fp)
			}
		})
		var cl *ssh.Client
		rp, perr := proxyOf(hop, i > 0)
		if perr != nil {
			closeAll()
			return nil, hopError(hop, i < len(hops)-1, perr)
		}
		if rp != nil {
			// The proxy is reached from WRM, or through the previous hop.
			var base dialFunc = directDial
			if i > 0 {
				base = opened[i-1].Dial
			}
			var conn net.Conn
			if conn, err = rp.dial(base, hop.Host); err == nil {
				cl, err = sshOverConn(conn, hop.Host, cfg)
			}
		} else if i == 0 {
			cl, err = ssh.Dial("tcp", hop.Host, cfg)
		} else {
			cl, err = dialThrough(opened[i-1], hop.Host, cfg)
			if err != nil && asHostKeyError(err) == nil && !strings.Contains(err.Error(), "unable to authenticate") {
				err = fmt.Errorf("%s cannot reach %s: %w", hops[i-1].Name, hop.Host, err)
			}
		}
		if err != nil {
			closeAll()
			if strings.Contains(err.Error(), "unable to authenticate") {
				if hint := certLoginHint(hop); hint != "" {
					err = fmt.Errorf("%w (%s)", err, hint)
				}
			}
			return nil, hopError(hop, i < len(hops)-1, err)
		}
		opened = append(opened, cl)
	}
	target := opened[len(opened)-1]
	if len(opened) > 1 {
		go func() {
			target.Wait() // the target connection ended: close the hops it went through
			for i := len(opened) - 2; i >= 0; i-- {
				opened[i].Close()
			}
		}()
	}
	return target, nil
}

func hopError(hop Connection, isJump bool, err error) error {
	if !isJump {
		return err
	}
	return fmt.Errorf("jump host %q: %w", hop.Name, err)
}

// sshOverConn runs the SSH handshake on an open connection (e.g. through a proxy).
func sshOverConn(conn net.Conn, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	conn.SetDeadline(time.Now().Add(2 * timeout))
	timer := time.AfterFunc(2*timeout, func() { conn.Close() }) // SSH channels have no deadlines
	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if !timer.Stop() && err == nil {
		ncc.Close()
		return nil, fmt.Errorf("timeout")
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return ssh.NewClient(ncc, chans, reqs), nil
}

// dialThrough opens an SSH client to addr through an already connected client.
func dialThrough(via *ssh.Client, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	type result struct {
		cl  *ssh.Client
		err error
	}
	done := make(chan result, 1)
	var conn net.Conn
	connCh := make(chan net.Conn, 1)
	go func() {
		c, err := via.Dial("tcp", addr)
		if err != nil {
			done <- result{nil, err}
			return
		}
		connCh <- c
		ncc, chans, reqs, err := ssh.NewClientConn(c, addr, cfg)
		if err != nil {
			c.Close()
			done <- result{nil, err}
			return
		}
		done <- result{ssh.NewClient(ncc, chans, reqs), nil}
	}()
	timer := time.NewTimer(2 * timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.cl, r.err
	case <-timer.C:
		select {
		case conn = <-connCh:
			conn.Close() // aborts the handshake
		default:
		}
		go func() { // close a client that may still arrive after the timeout
			if r := <-done; r.cl != nil {
				r.cl.Close()
			}
		}()
		return nil, fmt.Errorf("timeout")
	}
}

// validateJump checks a jump host choice when a connection is saved.
func validateJump(userID, connID int, jumpID *int) error {
	if jumpID == nil || *jumpID <= 0 {
		return nil
	}
	if connID > 0 && *jumpID == connID {
		return fmt.Errorf("a connection cannot be its own jump host")
	}
	if !userOwnsConnection(*jumpID, userID) {
		return fmt.Errorf("jump host not found")
	}
	cur := *jumpID
	for depth := 1; ; depth++ {
		if depth > maxJumpDepth {
			return fmt.Errorf("the jump host chain would have more than %d hops", maxJumpDepth)
		}
		j, err := loadConnectionRaw(cur)
		if err != nil {
			return fmt.Errorf("jump host not found")
		}
		if !isSSHProtocol(j) {
			return fmt.Errorf("the jump host must be an SSH connection")
		}
		if j.JumpID == nil || *j.JumpID <= 0 {
			return nil
		}
		if connID > 0 && *j.JumpID == connID {
			return fmt.Errorf("this jump host leads back to the connection itself (loop)")
		}
		cur = *j.JumpID
	}
}
