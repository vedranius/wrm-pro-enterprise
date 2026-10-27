//go:build !windows

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
)

// mdBuildAccount is the unprivileged OS account WRM-host builds run as (medium_build_user).
type mdBuildAccount struct {
	Name string
	Home string
	UID  uint32
	GID  uint32
	GIDs []uint32
}

// mdResolveBuildUser checks medium_build_user. Empty means builds run with the WRM account.
// A build user needs WRM to run as root (only root can switch accounts), must not be root,
// and must not be able to read the WRM database, its key or the Medium data directory.
func mdResolveBuildUser() (*mdBuildAccount, error) {
	name := getSetting("medium_build_user")
	if name == "" {
		return nil, nil
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("the build user %s is set, but WRM does not run as root and cannot switch to it; run WRM as root or clear the build user (Admin → Policies → Git workspace)", name)
	}
	u, err := user.Lookup(name)
	if err != nil {
		return nil, fmt.Errorf("the build user %s does not exist on the WRM host; create it (for example useradd --system --create-home %s) or clear the setting", name, name)
	}
	uid, err1 := strconv.ParseUint(u.Uid, 10, 31) // fits an int on every platform
	gid, err2 := strconv.ParseUint(u.Gid, 10, 31)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("the build user %s has an unexpected user or group id", name)
	}
	if uid == 0 {
		return nil, fmt.Errorf("the build user %s is root; choose an unprivileged account", name)
	}
	acc := &mdBuildAccount{Name: name, Home: u.HomeDir, UID: uint32(uid), GID: uint32(gid)}
	if ids, err := u.GroupIds(); err == nil {
		for _, s := range ids {
			if g, err := strconv.ParseUint(s, 10, 31); err == nil {
				if g == 0 {
					return nil, fmt.Errorf("the build user %s is in the root group; choose an account outside it", name)
				}
				acc.GIDs = append(acc.GIDs, uint32(g))
			}
		}
	}
	if gid == 0 {
		return nil, fmt.Errorf("the build user %s has the root group as its primary group; choose an account outside it", name)
	}
	db := resolveDBPath()
	for _, p := range []string{db, db + "-wal", db + "-shm", db + ".key", mediumRoot()} {
		if mdAccountCanRead(acc, p) {
			return nil, fmt.Errorf("the build user %s can read %s; make the WRM data files readable only by the WRM account (chmod o-rwx, g-rwx) or choose another build user", name, p)
		}
	}
	return acc, nil
}

// mdAccountCanRead reports whether the account may read path by its permission bits.
// A missing path cannot be read.
func mdAccountCanRead(acc *mdBuildAccount, p string) bool {
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	mode := fi.Mode().Perm()
	var rd fs.FileMode = 0o4
	if uint32(st.Uid) == acc.UID {
		return mode&(rd<<6) != 0
	}
	inGroup := uint32(st.Gid) == acc.GID
	for _, g := range acc.GIDs {
		if g == uint32(st.Gid) {
			inGroup = true
		}
	}
	if inGroup && mode&(rd<<3) != 0 {
		return true
	}
	return mode&rd != 0
}

// mdApplyBuildUser runs cmd as the build account with a clean environment and hands the
// build directory to it.
func mdApplyBuildUser(cmd *exec.Cmd, acc *mdBuildAccount, root string) error {
	if acc == nil {
		return nil
	}
	err := mdWalkInRoot(root, func(r *os.Root, rel string, fi fs.FileInfo) error {
		return r.Lchown(rel, int(acc.UID), int(acc.GID))
	})
	if err != nil {
		return fmt.Errorf("hand the build directory to %s: %v", acc.Name, err)
	}
	home := acc.Home
	if fi, err := os.Stat(home); home == "" || err != nil || !fi.IsDir() {
		home = root
	}
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=" + home, "USER=" + acc.Name, "LOGNAME=" + acc.Name, "LANG=C.UTF-8", "TMPDIR=" + root}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: acc.UID, Gid: acc.GID, Groups: acc.GIDs}}
	return nil
}

// mdReclaimBuildDir takes the build directory back from the build account before WRM
// reads the result, so a process the build left behind cannot swap files afterwards. A
// regular file with more than one link that the build account does not own is refused: it
// could be a hard link to a file the account cannot read itself, which WRM (as root) would
// otherwise read and deploy.
func mdReclaimBuildDir(acc *mdBuildAccount, root string) error {
	if acc == nil {
		return nil
	}
	var bad string
	err := mdWalkInRoot(root, func(r *os.Root, rel string, fi fs.FileInfo) error {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && fi.Mode().IsRegular() && st.Nlink > 1 && st.Uid != acc.UID && bad == "" {
			bad = rel
			return nil
		}
		if bad != "" {
			return nil
		}
		// a directory is taken over before its entries are listed, so nothing new appears in it
		if err := r.Lchown(rel, os.Geteuid(), os.Getegid()); err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink == 0 && fi.Mode().Perm()&0o022 != 0 {
			return r.Chmod(rel, fi.Mode().Perm()&^0o022)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("take the build directory back from %s: %v", acc.Name, err)
	}
	if bad != "" {
		return fmt.Errorf("the build left a hard link (%s); WRM does not deploy hard links from a build that ran as %s", filepath.ToSlash(bad), acc.Name)
	}
	return nil
}

// mdWalkInRoot visits root and everything below it, a directory before its entries, with
// every operation scoped to an os.Root so no path can lead outside the build directory.
func mdWalkInRoot(root string, fn func(r *os.Root, rel string, fi fs.FileInfo) error) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	var walk func(rel string) error
	walk = func(rel string) error {
		fi, err := r.Lstat(rel)
		if err != nil {
			return err
		}
		if err := fn(r, rel, fi); err != nil {
			return err
		}
		if !fi.IsDir() {
			return nil
		}
		d, err := r.Open(rel)
		if err != nil {
			return err
		}
		names, err := d.Readdirnames(-1)
		d.Close()
		if err != nil {
			return err
		}
		for _, n := range names {
			if err := walk(path.Join(rel, n)); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(".")
}
