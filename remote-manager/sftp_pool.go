// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// The file manager used to open a brand-new SSH + SFTP connection for every single
// request (list, download, rename…). The pool keeps one SFTP session per connection
// alive for a few minutes so browsing, searching and editing are fast.

type pooledSFTP struct {
	connID   int
	key      string
	ssh      *ssh.Client
	sftp     *sftp.Client
	users    int
	lastUsed time.Time
	broken   bool
	closed   bool
}

func (p *pooledSFTP) close() {
	if p.closed {
		return
	}
	p.closed = true
	p.sftp.Close()
	p.ssh.Close()
}

var sftpPool = struct {
	sync.Mutex
	m map[int]*pooledSFTP
}{m: map[int]*pooledSFTP{}}

const sftpPoolIdle = 5 * time.Minute

func init() {
	go func() {
		for range time.Tick(time.Minute) {
			sftpPool.Lock()
			for id, p := range sftpPool.m {
				if p.users == 0 && (p.broken || time.Since(p.lastUsed) > sftpPoolIdle) {
					p.close()
					delete(sftpPool.m, id)
				}
			}
			sftpPool.Unlock()
		}
	}()
}

func connFingerprint(c Connection) string {
	h := sha256.Sum256([]byte(c.Host + "\x00" + c.Username + "\x00" + c.AuthMethod + "\x00" + c.Password + "\x00" + c.PrivateKey + "\x00" + c.KeyPath))
	return hex.EncodeToString(h[:])
}

func acquireSFTP(c Connection) (*pooledSFTP, error) {
	key := connFingerprint(c)
	sftpPool.Lock()
	if p, ok := sftpPool.m[c.ID]; ok && p.key == key && !p.broken {
		p.users++
		idle := time.Since(p.lastUsed)
		p.lastUsed = time.Now()
		sftpPool.Unlock()
		// A connection that sat idle may have been silently dropped by a NAT/firewall;
		// probe it quickly instead of letting the next request hang on a dead TCP socket.
		if idle < 30*time.Second || sshAlive(p.ssh, 5*time.Second) {
			return p, nil
		}
		releaseSFTP(p, sftp.ErrSSHFxConnectionLost)
	} else {
		sftpPool.Unlock()
	}

	client, err := getSSHClient(c)
	if err != nil {
		return nil, err
	}
	sc, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, err
	}
	p := &pooledSFTP{connID: c.ID, key: key, ssh: client, sftp: sc, users: 1, lastUsed: time.Now()}
	// Mark the entry broken as soon as the underlying SSH connection dies.
	go func() {
		client.Wait()
		sftpPool.Lock()
		p.broken = true
		if p.users == 0 {
			p.close()
			if sftpPool.m[p.connID] == p {
				delete(sftpPool.m, p.connID)
			}
		}
		sftpPool.Unlock()
	}()

	sftpPool.Lock()
	if old, ok := sftpPool.m[c.ID]; ok && old != p {
		old.broken = true
		if old.users == 0 {
			old.close()
		}
	}
	sftpPool.m[c.ID] = p
	sftpPool.Unlock()
	return p, nil
}

func releaseSFTP(p *pooledSFTP, err error) {
	sftpPool.Lock()
	defer sftpPool.Unlock()
	p.users--
	p.lastUsed = time.Now()
	if isConnLostErr(err) {
		p.broken = true
	}
	if p.broken && p.users <= 0 {
		p.close()
		if sftpPool.m[p.connID] == p {
			delete(sftpPool.m, p.connID)
		}
	}
}

// withSFTP runs fn with a pooled SFTP client. When retry is true and fn fails because the
// pooled connection was dead, it is retried once on a fresh connection. Only pass
// retry=true for operations that have not written anything to the client yet.
func withSFTP(c Connection, retry bool, fn func(*sftp.Client) error) error {
	attempts := 1
	if retry {
		attempts = 2
	}
	var err error
	for i := 0; i < attempts; i++ {
		var p *pooledSFTP
		p, err = acquireSFTP(c)
		if err != nil {
			return err
		}
		err = fn(p.sftp)
		releaseSFTP(p, err)
		if err == nil || !isConnLostErr(err) {
			return err
		}
		log.Printf("SFTP pool: connection %d lost (%v), reconnecting", c.ID, err)
	}
	return err
}

// withSSHClient runs fn with the pooled SSH client (e.g. to open an exec session).
func withSSHClient(c Connection, fn func(*ssh.Client) error) error {
	p, err := acquireSFTP(c)
	if err != nil {
		return err
	}
	err = fn(p.ssh)
	releaseSFTP(p, err)
	return err
}

// sshAlive sends an OpenSSH keepalive request and waits up to timeout for any reply.
func sshAlive(c *ssh.Client, timeout time.Duration) bool {
	ch := make(chan error, 1)
	go func() {
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		ch <- err
	}()
	select {
	case err := <-ch:
		return err == nil
	case <-time.After(timeout):
		return false
	}
}
