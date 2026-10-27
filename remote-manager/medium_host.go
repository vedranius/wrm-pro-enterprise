// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── MEDIUM: a destination the native engine may change ───
//
// mdHost adds writes to mdRemote: SFTP operations for the files themselves (no file name
// ever passes through a shell), a POSIX sh for the few steps the original tool also does
// with one remote command (the lock, the manifests, the history), and the login shell for
// post_deploy, exactly as "ssh host <command>" runs it. Tests use the local file system.

type mdHost interface {
	mdRemote
	Lstat(p string) (os.FileInfo, error)
	Mkdir(p string) error
	MkdirAll(p string) error
	Rename(from, to string) error // replaces to
	RemoveDir(p string) error
	Remove(p string) error
	Symlink(target, p string) error
	Chmod(p string, m os.FileMode) error
	Chown(p string, uid, gid int) error
	WriteNew(p string, r io.Reader) error // creates p, which must not exist
	// Sh runs script with a POSIX sh: its exit code, stdout and stderr.
	Sh(ctx context.Context, script string, stdin []byte) (int, []byte, string, error)
	// Stream runs script with a POSIX sh and streams stdout (for build output).
	Stream(ctx context.Context, script string, out io.Writer) (int, string, error)
	// Login runs command through the account's login shell and streams the output.
	Login(ctx context.Context, command string, out io.Writer) (int, error)
}

func mdIsNotExist(err error) bool {
	return err != nil && (errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "no such file"))
}

// ─── SSH ───

func (m mdSFTP) Lstat(p string) (os.FileInfo, error) { return m.cl.Lstat(p) }
func (m mdSFTP) Mkdir(p string) error                { return m.cl.Mkdir(p) }
func (m mdSFTP) MkdirAll(p string) error             { return m.cl.MkdirAll(p) }
func (m mdSFTP) RemoveDir(p string) error            { return m.cl.RemoveDirectory(p) }
func (m mdSFTP) Remove(p string) error               { return m.cl.Remove(p) }

// Symlink creates the link over SFTP and checks its target: some SFTP servers resolve a
// relative target against their working directory, so then the link is made with ln.
func (m mdSFTP) Symlink(target, p string) error {
	if err := m.cl.Symlink(target, p); err != nil {
		return err
	}
	if got, err := m.cl.ReadLink(p); err == nil && got == target {
		return nil
	}
	if err := m.cl.Remove(p); err != nil {
		return err
	}
	code, _, stderr, err := m.Sh(context.Background(), "ln -s -- "+shellQuote(target)+" "+shellQuote(p), nil)
	if err == nil && code != 0 {
		err = fmt.Errorf("ln exit code %d: %s", code, strings.TrimSpace(stderr))
	}
	return err
}
func (m mdSFTP) Chmod(p string, mode os.FileMode) error {
	return m.cl.Chmod(p, mode)
}
func (m mdSFTP) Chown(p string, uid, gid int) error { return m.cl.Chown(p, uid, gid) }

// Rename uses posix-rename (it replaces the target); a move to another file system (a
// state_dir on another disk) falls back to mv on the server.
func (m mdSFTP) Rename(from, to string) error {
	err := m.cl.PosixRename(from, to)
	if err == nil {
		return nil
	}
	code, _, stderr, e := m.Sh(context.Background(), "mv -f -- "+shellQuote(from)+" "+shellQuote(to), nil)
	if e == nil && code == 0 {
		return nil
	}
	if stderr != "" {
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(stderr))
	}
	return err
}

func (m mdSFTP) WriteNew(p string, r io.Reader) error {
	f, err := m.cl.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		m.cl.Remove(p)
		return err
	}
	return f.Close()
}

func mdSSHRun(ctx context.Context, cl *ssh.Client, command string, stdin []byte, stdout, stderr io.Writer) (int, error) {
	s, err := cl.NewSession()
	if err != nil {
		return -1, err
	}
	defer s.Close()
	if stdin != nil {
		s.Stdin = bytes.NewReader(stdin)
	}
	s.Stdout, s.Stderr = stdout, stderr
	done := make(chan error, 1)
	go func() { done <- s.Run(command) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		s.Close()
		return -1, fmt.Errorf("the server did not finish in time")
	}
	if err == nil {
		return 0, nil
	}
	var ee *ssh.ExitError
	if errors.As(err, &ee) {
		return ee.ExitStatus(), nil
	}
	return -1, err
}

func (m mdSFTP) Sh(ctx context.Context, script string, stdin []byte) (int, []byte, string, error) {
	out, errb := &capBuffer{max: 64 << 20}, &capBuffer{max: 64 << 10}
	code, err := mdSSHRun(ctx, m.ssh, "sh -c "+shellQuote(script), stdin, out, errb)
	return code, out.Bytes(), errb.String(), err
}

func (m mdSFTP) Stream(ctx context.Context, script string, out io.Writer) (int, string, error) {
	errb := &capBuffer{max: 64 << 10}
	code, err := mdSSHRun(ctx, m.ssh, "sh -c "+shellQuote(script), nil, out, errb)
	return code, errb.String(), err
}

func (m mdSFTP) Login(ctx context.Context, command string, out io.Writer) (int, error) {
	return mdSSHRun(ctx, m.ssh, command, nil, out, out)
}

// ─── helpers ───

func mdCtx(seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}

// mdShCheck runs a script that must succeed.
func mdShCheck(h mdHost, script string, stdin []byte, what string) ([]byte, error) {
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	code, out, stderr, err := h.Sh(ctx, script, stdin)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", what, err)
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", code)
		}
		return out, fmt.Errorf("%s: %s", what, truncateStr(msg, 400))
	}
	return out, nil
}

// mdLinkOnPath: is a component of rel (below root, the last one excluded) a symbolic link
// on the server? Nothing is written through such a link (it may lead outside the path).
func mdLinkOnPath(h mdHost, root, rel string) bool {
	parts := strings.Split(rel, "/")
	p := root
	for _, c := range parts[:len(parts)-1] {
		p = path.Join(p, c)
		fi, err := h.Lstat(p)
		if err != nil {
			return false
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return true
		}
	}
	return false
}
