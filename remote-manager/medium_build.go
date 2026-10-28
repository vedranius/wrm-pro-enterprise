// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── MEDIUM: where a project with a build step is built ───
//
// The original tool builds in its runtime. The native engine can build on the WRM host
// (a POSIX host with the project's toolchain) or on a chosen Linux build server over SSH.
// The build server only receives the prepared source (tracked files, no Git metadata and
// no WRM SSH keys); its temporary directory is removed afterwards. The built source_dir is
// fetched back and then transferred to the destinations by the normal native deployment.
//
// A project can also be set to "none": the native engine then deploys source_dir from the
// commit, exactly as it does for projects without a build step.

const (
	mdBuildTimeout   = 30 * time.Minute
	mdBuildLog       = 8 << 10
	mdBuildMaxOutput = 512 << 20
)

type mdBuildSetting struct {
	Location string `json:"location"` // "", host, server, none
	ConnID   int    `json:"conn_id,omitempty"`
	ConnName string `json:"conn_name,omitempty"`
}

type mdBuildOutcome struct {
	Tree     mdTree
	Location string // host, server, none
	Server   string
	User     string // medium_build_user of a WRM-host build
	Platform string
	Arch     string
	Log      string
	cleanup  func()
}

func (o *mdBuildOutcome) Close() {
	if o != nil && o.cleanup != nil {
		o.cleanup()
		o.cleanup = nil
	}
}

// mdCleanupBuildDirs removes temporary build trees left by a crash or restart. It runs at
// startup; only directories older than two hours are touched, so a second WRM process is
// not disturbed.
func mdCleanupBuildDirs() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "wrm-medium-build-") {
			continue
		}
		info, err := e.Info()
		if err == nil && time.Since(info.ModTime()) > 2*time.Hour {
			os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
		}
	}
}

func mdBuildSchema() {
	ensureTable("medium_build_settings", `CREATE TABLE IF NOT EXISTS medium_build_settings (
		user_id INTEGER NOT NULL,
		project TEXT NOT NULL,
		location TEXT NOT NULL DEFAULT '',
		conn_id INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, project))`,
		"user_id", "project", "location", "conn_id", "updated_at")
}

func mdBuildOf(uid int, project string) mdBuildSetting {
	s := mdBuildSetting{}
	if db.QueryRow(`SELECT location, conn_id FROM medium_build_settings WHERE user_id=? AND project=?`, uid, project).Scan(&s.Location, &s.ConnID) != nil {
		return mdBuildSetting{}
	}
	if s.ConnID != 0 {
		s.ConnName = mdConnName(uid, s.ConnID)
	}
	return s
}

func mdBuildSettings(uid int) map[string]mdBuildSetting {
	out := map[string]mdBuildSetting{}
	rows, err := db.Query(`SELECT project, location, conn_id FROM medium_build_settings WHERE user_id=?`, uid)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var s mdBuildSetting
		if rows.Scan(&p, &s.Location, &s.ConnID) == nil {
			out[p] = s
		}
	}
	return out
}

func mdConnName(uid int, id int) string {
	for _, c := range mdUserConnections(uid) {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

func mdSetBuild(uid int, project, location string, connID int) (mdBuildSetting, error) {
	switch location {
	case "", "none":
		connID = 0
	case "host":
		if !isAdminUser(uid) {
			return mdBuildSetting{}, errors.New("building on the WRM host is limited to administrators; choose a Linux build server or no build")
		}
		if ok, why := mdHostBuildSupported(); !ok {
			return mdBuildSetting{}, errors.New(why)
		}
		connID = 0
	case "server":
		if connID == 0 || mdConnName(uid, connID) == "" {
			return mdBuildSetting{}, fmt.Errorf("choose an SSH connection for the build server")
		}
	default:
		return mdBuildSetting{}, fmt.Errorf("unknown build location %q", location)
	}
	if location == "" {
		db.Exec(`DELETE FROM medium_build_settings WHERE user_id=? AND project=?`, uid, project)
		return mdBuildSetting{}, nil
	}
	if _, err := db.Exec(`INSERT OR REPLACE INTO medium_build_settings (user_id, project, location, conn_id, updated_at) VALUES (?,?,?,?,?)`,
		uid, project, location, connID, nowStamp()); err != nil {
		return mdBuildSetting{}, err
	}
	return mdBuildSetting{Location: location, ConnID: connID, ConnName: mdConnName(uid, connID)}, nil
}

// mdHostBuildSupported says whether the WRM host can run a project's build command.
// The build command is a POSIX shell command; Windows uses a Linux build server instead.
func mdHostBuildSupported() (bool, string) {
	if runtime.GOOS == "windows" {
		return false, "WRM host builds need a POSIX shell and the project's Linux toolchain. On Windows choose a Linux build server, or no build."
	}
	if _, err := exec.LookPath("sh"); err != nil {
		return false, "sh is not installed on the WRM host. Choose a Linux build server, or no build."
	}
	return true, ""
}

func mdHostBuildInfo() map[string]interface{} {
	ok, why := mdHostBuildSupported()
	return map[string]interface{}{"supported": ok, "reason": why, "platform": runtime.GOOS + "/" + runtime.GOARCH}
}

// mdBuildProject prepares source_dir for deployment. Without a build it is the Git tree
// below source_dir. With a build, the whole commit is materialised, the configured command
// runs, and source_dir is read back from the result. The caller keeps outcome.Tree's files
// alive until the preview run is discarded (outcome.Close).
func mdBuildProject(uid int, match mdSourceMatch, proj *mdProject, commit string, setting mdBuildSetting, stage func(string)) (*mdBuildOutcome, error) {
	if proj.Build == nil {
		tree, err := mdGitSourceTree(match, proj, commit)
		if err != nil {
			return nil, err
		}
		return &mdBuildOutcome{Tree: tree}, nil
	}
	if strings.TrimSpace(*proj.Build) == "" {
		return nil, fmt.Errorf("project %s has an empty build command; remove build from its deploy-config to deploy the commit without a build", proj.Name)
	}
	// A host build runs a shell command with the WRM account. That is an administrator
	// action, not something an operator with git_medium should be able to request.
	if setting.Location == "host" && !isAdminUser(uid) {
		return nil, fmt.Errorf("building on the WRM host is limited to administrators; choose a Linux build server or no build")
	}
	switch setting.Location {
	case "none":
		tree, err := mdGitSourceTree(match, proj, commit)
		if err != nil {
			return nil, err
		}
		return &mdBuildOutcome{Tree: tree, Location: "none"}, nil
	case "host", "server":
	default:
		return nil, fmt.Errorf("project %s has a build step; choose where to build it (WRM host, a Linux build server, or no build) in the native engine panel", proj.Name)
	}
	stage("source")
	fullProj := *proj
	fullProj.SourceDir = "."
	full, err := mdGitSourceTree(match, &fullProj, commit)
	if err != nil {
		return nil, err
	}
	stage("build")
	switch setting.Location {
	case "host":
		return mdBuildOnHost(full, proj)
	case "server":
		return mdBuildOnServer(uid, setting.ConnID, full, proj)
	}
	return nil, fmt.Errorf("unknown build location %q", setting.Location)
}

// mdBuildEnv is the WRM process environment without the secrets WRM itself needs
// (encryption key, database path, WRM_* service settings). Proxy, locale and toolchain
// variables a normal shell build expects stay.
func mdBuildEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "ENCRYPTION_KEY", "ENCRYPTION_KEY_FILE", "DB_PATH":
			continue
		}
		if strings.HasPrefix(k, "WRM_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// mdBuildOnHost materialises the commit in a private temporary directory, runs the build
// command there with the WRM OS account (or medium_build_user), and reads source_dir back.
func mdBuildOnHost(full mdTree, proj *mdProject) (*mdBuildOutcome, error) {
	if ok, why := mdHostBuildSupported(); !ok {
		return nil, errors.New(why)
	}
	acc, err := mdResolveBuildUser()
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "wrm-medium-build-")
	if err != nil {
		return nil, fmt.Errorf("create a build directory: %v", err)
	}
	out := &mdBuildOutcome{Location: "host", Platform: runtime.GOOS, Arch: mdNormalizeArch(runtime.GOARCH)}
	out.cleanup = func() { os.RemoveAll(root) }
	// With a build user the build gets a subdirectory of a directory it cannot change, so
	// it cannot replace the directory WRM reads the result from.
	work := root
	if acc != nil {
		work = filepath.Join(root, "work")
		if err := os.Chmod(root, 0o711); err != nil {
			out.Close()
			return nil, fmt.Errorf("prepare the build directory: %v", err)
		}
	}
	if err := mdWriteTree(work, full); err != nil {
		out.Close()
		return nil, fmt.Errorf("prepare the build source: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), mdBuildTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", *proj.Build)
	cmd.Dir = work
	cmd.Env = mdBuildEnv()
	if err := mdApplyBuildUser(cmd, acc, work); err != nil {
		out.Close()
		return nil, err
	}
	if acc != nil {
		out.User = acc.Name
	}
	stdout, stderr := &mdTailWriter{max: mdBuildLog}, &mdTailWriter{max: mdBuildLog}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	if rerr := mdReclaimBuildDir(acc, work); rerr != nil {
		out.Close()
		return nil, rerr
	}
	out.Log = mdBuildLogText(stdout.String(), stderr.String())
	if ctx.Err() != nil {
		out.Close()
		return nil, fmt.Errorf("the build did not finish in %s", mdBuildTimeout)
	}
	if err != nil {
		out.Close()
		return nil, fmt.Errorf("the build failed: %v%s", err, mdBuildLogSuffix(out.Log))
	}
	tree, err := mdReadBuildOutput(work, proj.SourceDir)
	if err != nil {
		out.Close()
		return nil, err
	}
	out.Tree = tree
	return out, nil
}

// mdBuildOnServer sends the prepared source to a temporary directory on a Linux build
// server, runs the build there, and streams source_dir back. Only the tracked commit is
// sent; no Git metadata, credentials or WRM SSH files are included.
func mdBuildOnServer(uid int, connID int, full mdTree, proj *mdProject) (*mdBuildOutcome, error) {
	work, err := os.MkdirTemp("", "wrm-medium-build-")
	if err != nil {
		return nil, fmt.Errorf("create a build directory: %v", err)
	}
	out := &mdBuildOutcome{Location: "server"}
	out.cleanup = func() { os.RemoveAll(work) }
	archive := filepath.Join(work, "source.tar.gz")
	if err := mdWriteTreeArchive(archive, full); err != nil {
		out.Close()
		return nil, fmt.Errorf("pack the build source: %v", err)
	}
	var platform, arch string
	err = mdOpenConnHost(uid, connID, func(h mdHost, name string) error {
		out.Server = name
		p, a, err := mdHostPlatform(h)
		if err != nil {
			return fmt.Errorf("check the build server %s: %v", name, err)
		}
		if p != "linux" {
			return fmt.Errorf("the build server %s runs %s; choose a Linux build server", name, p)
		}
		platform, arch = p, a
		ctx, cancel := context.WithTimeout(context.Background(), settingSeconds("medium_command_timeout", 300))
		defer cancel()
		code, baseOut, stderr, err := h.Sh(ctx, "mktemp -d", nil)
		if err != nil {
			return fmt.Errorf("create a temporary directory on %s: %v", name, err)
		}
		if code != 0 {
			return fmt.Errorf("create a temporary directory on %s: %s", name, mdShMsg(code, stderr))
		}
		base := strings.TrimSpace(string(baseOut))
		if !strings.HasPrefix(base, "/") || strings.ContainsAny(base, "\r\n") {
			return fmt.Errorf("the build server %s returned an unexpected temporary directory", name)
		}
		defer func() {
			c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			h.Sh(c, "rm -rf -- "+shellQuote(base), nil)
		}()
		srcDir := path.Join(base, "src")
		srcArchive := path.Join(base, "source.tar.gz")
		if err := h.MkdirAll(srcDir); err != nil {
			return fmt.Errorf("create the build directory on %s: %v", name, err)
		}
		f, err := os.Open(archive)
		if err != nil {
			return err
		}
		err = h.WriteNew(srcArchive, f)
		f.Close()
		if err != nil {
			return fmt.Errorf("send the source to %s: %v", name, err)
		}
		if _, err := mdShCheck(h, "tar -xzf "+shellQuote(srcArchive)+" -C "+shellQuote(srcDir)+" && rm -f "+shellQuote(srcArchive), nil, "unpack the source on "+name); err != nil {
			return err
		}
		buildCtx, buildCancel := context.WithTimeout(context.Background(), mdBuildTimeout)
		defer buildCancel()
		// The login shell keeps the account's PATH and profile, so a toolchain installed
		// for that user (not only in /usr/bin) is found, as it is for post_deploy.
		logw := &mdTailWriter{max: mdBuildLog}
		code, err = h.Login(buildCtx, "cd "+shellQuote(srcDir)+" && "+*proj.Build, logw)
		out.Log = logw.String()
		if buildCtx.Err() != nil {
			return fmt.Errorf("the build on %s did not finish in %s", name, mdBuildTimeout)
		}
		if err != nil {
			return fmt.Errorf("the build on %s failed: %v%s", name, err, mdBuildLogSuffix(out.Log))
		}
		if code != 0 {
			return fmt.Errorf("the build on %s failed (exit code %d)%s", name, code, mdBuildLogSuffix(out.Log))
		}
		srcRel := mdNorm(proj.SourceDir)
		if srcRel != "." && !mdSafeTreePath(srcRel) {
			return fmt.Errorf("source_dir %s is not a safe relative path", proj.SourceDir)
		}
		outDir := path.Join(srcDir, strings.TrimPrefix(srcRel, "./"))
		if srcRel == "." {
			outDir = srcDir
		}
		if _, err := mdShCheck(h, "test -d "+shellQuote(outDir)+" && test ! -L "+shellQuote(outDir), nil, "read source_dir on "+name); err != nil {
			return fmt.Errorf("source_dir %s is not a directory after the build on %s", proj.SourceDir, name)
		}
		localTar := filepath.Join(work, "output.tar.gz")
		lf, err := os.Create(localTar)
		if err != nil {
			return err
		}
		code, stderr, err = h.Stream(buildCtx, "tar -czf - -C "+shellQuote(outDir)+" .", &mdLimitWriter{w: lf, max: mdBuildMaxOutput + (16 << 20)})
		closeErr := lf.Close()
		if err != nil {
			return fmt.Errorf("download the build result from %s: %v", name, err)
		}
		if code != 0 {
			return fmt.Errorf("download the build result from %s: %s", name, mdShMsg(code, stderr))
		}
		if closeErr != nil {
			return closeErr
		}
		outRoot := filepath.Join(work, "out")
		if err := mdExtractTarGz(localTar, outRoot); err != nil {
			return fmt.Errorf("unpack the build result from %s: %v", name, err)
		}
		tree, err := mdLocalTree(outRoot)
		if err != nil {
			return fmt.Errorf("read the build result from %s: %v", name, err)
		}
		out.Tree, out.Platform, out.Arch = tree, platform, mdNormalizeArch(arch)
		return nil
	})
	if err != nil {
		out.Close()
		return nil, err
	}
	if out.Tree == nil {
		out.Close()
		return nil, fmt.Errorf("the build server did not return a result")
	}
	return out, nil
}

var mdOpenConnHost = func(uid int, connID int, fn func(mdHost, string) error) error {
	conn, err := loadConnection(connID)
	if err != nil || conn.UserID != uid || !isSSHProtocol(conn) {
		return fmt.Errorf("the build server connection is not available")
	}
	p, err := acquireSFTP(conn)
	if err != nil {
		return fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
	}
	err = fn(mdSFTP{cl: p.sftp, ssh: p.ssh}, conn.Name)
	releaseSFTP(p, err)
	return err
}

func settingSeconds(key string, fallback int) time.Duration {
	n := settingInt(key)
	if n <= 0 {
		n = fallback
	}
	return time.Duration(n) * time.Second
}

func mdHostPlatform(h mdHost) (string, string, error) {
	ctx, cancel := mdCtx(settingInt("medium_command_timeout"))
	defer cancel()
	code, out, stderr, err := h.Sh(ctx, "uname -s; uname -m", nil)
	if err != nil {
		return "", "", err
	}
	if code != 0 {
		return "", "", fmt.Errorf("uname exit code %d: %s", code, strings.TrimSpace(stderr))
	}
	lines := strings.Fields(string(out))
	if len(lines) < 2 {
		return "", "", fmt.Errorf("unexpected uname output %q", strings.TrimSpace(string(out)))
	}
	return strings.ToLower(lines[0]), mdNormalizeArch(lines[1]), nil
}

func mdNormalizeArch(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armv7":
		return "armv7"
	case "armv6l", "armv6":
		return "armv6"
	case "i386", "i486", "i586", "i686", "x86":
		return "386"
	case "ppc64le", "s390x", "riscv64":
		return strings.ToLower(strings.TrimSpace(a))
	default:
		return strings.ToLower(strings.TrimSpace(a))
	}
}

func mdCheckBuildPlatforms(res *mdPlanResult, location, platform, arch string) {
	if location != "host" && location != "server" {
		return
	}
	for i := range res.Servers {
		sp := &res.Servers[i]
		if sp.Error != "" {
			continue
		}
		if sp.Platform == "" || sp.Arch == "" {
			sp.Warnings = append(sp.Warnings, "the destination's platform and architecture could not be read, so the build output was not checked against it")
			continue
		}
		if !strings.EqualFold(sp.Platform, platform) || sp.Arch != arch {
			sp.Error = fmt.Sprintf("the build ran on %s/%s, but %s is %s/%s; choose a build location with the same platform and architecture", platform, arch, sp.Server, sp.Platform, sp.Arch)
		}
	}
}

// mdReadBuildOutput reads source_dir from a local build root after checking that it is a
// real directory inside the root (not a symlink leading elsewhere).
func mdReadBuildOutput(root, sourceDir string) (mdTree, error) {
	rel := mdNorm(sourceDir)
	if rel == "." {
		rel = ""
	}
	if rel != "" && !mdSafeTreePath(rel) {
		return nil, fmt.Errorf("source_dir %s is not a safe relative path", sourceDir)
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	inside, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dir))
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("source_dir %s points outside the build directory", sourceDir)
	}
	// every component is checked, so a link the build created anywhere on the way cannot
	// point the read-back at a directory outside the build
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("read the build directory: %v", err)
	}
	defer r.Close()
	cur := "."
	for _, part := range append([]string{""}, strings.Split(rel, "/")...) {
		if part != "" {
			cur = path.Join(cur, part)
		}
		st, err := r.Lstat(cur)
		if err != nil {
			return nil, fmt.Errorf("source_dir %s does not exist after the build", sourceDir)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("source_dir %s goes through a symbolic link after the build; WRM reads build results only from real directories", sourceDir)
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("source_dir %s is not a directory after the build", sourceDir)
		}
	}
	return mdLocalTree(dir)
}

// mdWriteTree writes a source tree to a new local directory. Symlinks are created after
// files and directories so no file is ever written through a link from the tree.
func mdWriteTree(root string, tree mdTree) error {
	paths := make([]string, 0, len(tree))
	for p := range tree {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	write := func(symlinks bool) error {
		for _, rel := range paths {
			if !mdSafeTreePath(rel) {
				return fmt.Errorf("unsafe source path %q", rel)
			}
			e := tree[rel]
			if e == nil {
				continue
			}
			if (e.Type == "symlink") != symlinks {
				continue
			}
			p := filepath.Join(root, filepath.FromSlash(rel))
			switch e.Type {
			case "dir":
				if err := os.MkdirAll(p, 0o750); err != nil {
					return err
				}
			case "file":
				if e.open == nil {
					return fmt.Errorf("the content of %s is not available", rel)
				}
				b, err := e.open()
				if err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
					return err
				}
				mode := os.FileMode(0o644)
				if e.exec {
					mode = 0o755
				}
				if err := os.WriteFile(p, b, mode); err != nil {
					return err
				}
				if err := os.Chmod(p, mode); err != nil {
					return err
				}
			case "symlink":
				if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
					return err
				}
				if err := os.Symlink(e.Link, p); err != nil {
					return err
				}
			case "other":
				// Git has no other entry types; ignore them rather than creating surprises.
			default:
				return fmt.Errorf("unknown source entry type %q for %s", e.Type, rel)
			}
		}
		return nil
	}
	if err := write(false); err != nil {
		return err
	}
	return write(true)
}

func mdWriteTreeArchive(name string, tree mdTree) error {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	err = mdWriteTreeTar(tw, tree)
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	return err
}

func mdWriteTreeTar(tw *tar.Writer, tree mdTree) error {
	paths := make([]string, 0, len(tree))
	for p := range tree {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	now := time.Now()
	for _, rel := range paths {
		e := tree[rel]
		if e == nil {
			continue
		}
		hdr := &tar.Header{Name: rel, Mode: 0o644, ModTime: now, Format: tar.FormatPAX}
		switch e.Type {
		case "dir":
			hdr.Name, hdr.Typeflag, hdr.Mode = rel+"/", tar.TypeDir, 0o755
		case "file":
			if e.open == nil {
				return fmt.Errorf("the content of %s is not available", rel)
			}
			b, err := e.open()
			if err != nil {
				return err
			}
			hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(b))
			if e.exec {
				hdr.Mode = 0o755
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := tw.Write(b); err != nil {
				return err
			}
			continue
		case "symlink":
			hdr.Typeflag, hdr.Linkname, hdr.Mode = tar.TypeSymlink, e.Link, 0o777
		case "other":
			continue
		default:
			return fmt.Errorf("unknown source entry type %q for %s", e.Type, rel)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
	}
	return nil
}

func mdExtractTarGz(name, root string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	// os.Root confines every operation below root, including symlinks. A build
	// result may not write through a link or point a link outside the result.
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootFS.Close()
	tr := tar.NewReader(gz)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		rel := path.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if rel == "." || rel == "" {
			continue
		}
		if !mdSafeTreePath(rel) {
			return fmt.Errorf("the build result has an unsafe path %q", hdr.Name)
		}
		dir := path.Dir(rel)
		if dir != "." {
			if err := rootFS.MkdirAll(dir, 0o750); err != nil {
				return err
			}
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := rootFS.MkdirAll(rel, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if hdr.Size > mdBuildMaxOutput-total {
				return fmt.Errorf("the build result is too large (more than %d MB)", mdBuildMaxOutput>>20)
			}
			total += hdr.Size
			out, err := rootFS.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
			if err != nil {
				return err
			}
			n, cpErr := io.CopyN(out, tr, hdr.Size)
			closeErr := out.Close()
			if cpErr != nil {
				return cpErr
			}
			if n != hdr.Size {
				return fmt.Errorf("the build result ended early in %s", rel)
			}
			if closeErr != nil {
				return closeErr
			}
			mode := os.FileMode(0o644)
			if hdr.Mode&0o100 != 0 {
				mode = 0o755
			}
			if err := rootFS.Chmod(rel, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := rootFS.Symlink(hdr.Linkname, rel); err != nil {
				return fmt.Errorf("the build result has an unsafe symlink %s -> %s: %v", rel, hdr.Linkname, err)
			}
		case tar.TypeLink:
			target := path.Clean(strings.TrimPrefix(hdr.Linkname, "./"))
			if !mdSafeTreePath(target) {
				return fmt.Errorf("the build result has an unsafe hard link %q", hdr.Linkname)
			}
			if err := rootFS.Link(target, rel); err != nil {
				return err
			}
		default:
			// GNU tar's metadata entries are consumed by archive/tar itself.
		}
	}
	return nil
}

// mdTailWriter keeps the last max bytes, for a build log that stays useful when it is long.
type mdLimitWriter struct {
	w   io.Writer
	n   int64
	max int64
}

func (l *mdLimitWriter) Write(p []byte) (int, error) {
	if l.n+int64(len(p)) > l.max {
		return 0, fmt.Errorf("the build result is too large (more than %d MB)", l.max>>20)
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}

type mdTailWriter struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (w *mdTailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.b = append(w.b, p...)
	if len(w.b) > w.max {
		w.b = append([]byte(nil), w.b[len(w.b)-w.max:]...)
	}
	return len(p), nil
}

func (w *mdTailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.b)
}

func mdBuildLogText(stdout, stderr string) string {
	s := strings.TrimSpace(stdout)
	e := strings.TrimSpace(stderr)
	if e != "" {
		if s != "" {
			s += "\n"
		}
		s += e
	}
	if len(s) > mdBuildLog {
		s = s[len(s)-mdBuildLog:]
	}
	return s
}

func mdBuildLogSuffix(log string) string {
	if strings.TrimSpace(log) == "" {
		return ""
	}
	return "\n" + log
}

func mdShMsg(code int, stderr string) string {
	if msg := strings.TrimSpace(stderr); msg != "" {
		return msg
	}
	return fmt.Sprintf("exit code %d", code)
}
