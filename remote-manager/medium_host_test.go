// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// mdLocal as an mdHost: the native engine's tests deploy to local directories.

func (mdLocal) Lstat(p string) (os.FileInfo, error)  { return os.Lstat(p) }
func (mdLocal) Mkdir(p string) error                 { return os.Mkdir(p, 0o777) }
func (mdLocal) MkdirAll(p string) error              { return os.MkdirAll(p, 0o777) }
func (mdLocal) Rename(from, to string) error         { return os.Rename(from, to) }
func (mdLocal) RemoveDir(p string) error             { return mdLocalRmdir(p) }
func (mdLocal) Remove(p string) error                { return os.Remove(p) }
func (mdLocal) Symlink(target, p string) error       { return os.Symlink(target, p) }
func (mdLocal) Chmod(p string, m os.FileMode) error  { return os.Chmod(p, m) }
func (mdLocal) Chown(p string, uid, gid int) error   { return os.Lchown(p, uid, gid) }
func (mdLocal) WriteNew(p string, r io.Reader) error { return mdLocalWriteNew(p, r) }
func (mdLocal) Login(ctx context.Context, command string, out io.Writer) (int, error) {
	return mdLocalRun(ctx, command, nil, out, out)
}
func (mdLocal) Sh(ctx context.Context, script string, stdin []byte) (int, []byte, string, error) {
	var out, errb bytes.Buffer
	code, err := mdLocalRun(ctx, script, stdin, &out, &errb)
	return code, out.Bytes(), errb.String(), err
}
func (mdLocal) Stream(ctx context.Context, script string, out io.Writer) (int, string, error) {
	var errb bytes.Buffer
	code, err := mdLocalRun(ctx, script, nil, out, &errb)
	return code, errb.String(), err
}

func mdLocalRmdir(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", p)
	}
	return os.Remove(p)
}

func mdLocalWriteNew(p string, r io.Reader) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(p)
		return err
	}
	return f.Close()
}

func mdLocalRun(ctx context.Context, script string, stdin []byte, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ctx.Err() == nil {
		return ee.ExitCode(), nil
	}
	if ctx.Err() != nil {
		return -1, fmt.Errorf("the command did not finish in time")
	}
	return -1, err
}
