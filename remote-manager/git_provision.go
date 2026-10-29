// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── GIT INSTALL AND TRANSFER: new servers ───────────
//
// Install writes the files of a service at a ref (Git API or an imported bundle) into a
// chosen directory of any SSH connection, fills the protected files (templates, a copy from
// another installation or by hand; secrets from the vault), writes VERSION.md and the
// updates.jsonl line, and optionally creates a systemd unit or a supervisor program and
// starts it. Transfer copies an installation from server A to server B with its per-host
// configuration, but without logs, .deploy-bak, caches, ignored and backup-looking paths.
//
// Both are runs (git_runs) like updates: pre-checks on the target (free space, writable
// directory, tar, python3 / node, an existing installation is refused unless the overwrite
// is confirmed and then backed up), the files arrive as one tar stream in a staging
// directory inside the target, are verified by hash, moved into place (rename) and checked;
// any failure rolls back. Policies git_install and git_transfer; audit git.install and
// git.transfer.

const (
	gitMaxInstallServices = 20
	gitProvisionTimeout   = 15 * time.Minute
	gitStagingPrefix      = ".wrm-incoming-"
)

// gitFieldValue is the value of a template field: typed, or a credential of the vault.
type gitFieldValue struct {
	Value  string `json:"value,omitempty"`
	CredID int    `json:"cred_id,omitempty"`
	Part   string `json:"part,omitempty"` // password (default) | username
}

// gitFillSpec says how one protected file of a new installation is filled.
type gitFillSpec struct {
	Path        string                   `json:"path"`
	Mode        string                   `json:"mode"` // template | copy | manual | skip
	Template    string                   `json:"template,omitempty"`
	Values      map[string]gitFieldValue `json:"values,omitempty"`
	FromInstall int                      `json:"from_install,omitempty"`
	Content     string                   `json:"content,omitempty"`
}

// gitUnitSpec is the optional systemd unit or supervisor program of an installation.
type gitUnitSpec struct {
	Kind    string `json:"kind"` // systemd | supervisor
	Name    string `json:"name"`
	User    string `json:"user,omitempty"`
	Command string `json:"command"`
	WorkDir string `json:"work_dir,omitempty"`
	Enable  bool   `json:"enable,omitempty"` // systemd: start at boot
}

type gitInstallSvc struct {
	App   string        `json:"app"`
	Ref   string        `json:"ref,omitempty"` // "" = the service's target, bundle:<source id>, or a branch / tag:…
	Path  string        `json:"path"`
	Env   string        `json:"env,omitempty"`
	Files []gitFillSpec `json:"files,omitempty"`
	Unit  *gitUnitSpec  `json:"unit,omitempty"`
}

// gitProvision is the request of an install or a transfer run.
type gitProvision struct {
	ConnID        int              `json:"conn_id"` // target server
	Services      []*gitInstallSvc `json:"services,omitempty"`
	SourceInstall int              `json:"source_install,omitempty"` // transfer: the installation to copy
	Path          string           `json:"path,omitempty"`           // transfer: target directory
	Env           string           `json:"env,omitempty"`            // transfer
	Unit          *gitUnitSpec     `json:"unit,omitempty"`           // transfer
	Overwrite     bool             `json:"overwrite,omitempty"`      // an existing installation at the target is replaced (backed up)
	Confirm       string           `json:"confirm,omitempty"`        // typed target server name (production or overwrite)
}

// ─── validation ──────────────────────────────────────

var (
	gitUnitNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,99}$`)
	gitUnixUserRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,31}$`)
	// system directories that are never an installation directory (or its parent for /proc …)
	gitSystemDirs = map[string]bool{"/bin": true, "/sbin": true, "/lib": true, "/lib64": true, "/etc": true, "/usr": true, "/usr/bin": true, "/usr/sbin": true,
		"/usr/lib": true, "/usr/lib64": true, "/usr/local": true, "/usr/local/bin": true, "/usr/share": true, "/var": true, "/var/lib": true, "/var/log": true,
		"/home": true, "/root": true, "/opt": true, "/srv": true, "/tmp": true, "/var/tmp": true, "/mnt": true, "/media": true}
	gitSystemTrees = []string{"/proc", "/sys", "/dev", "/boot", "/run"}
)

// validInstallDir checks a target directory: absolute, clean, two levels at least, not a
// system directory.
func validInstallDir(p string) (string, error) {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\n\r\t\x00") || len(p) > 400 {
		return "", fmt.Errorf("enter an absolute directory path")
	}
	c := path.Clean(p)
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("the path must not contain ..")
		}
	}
	if strings.Count(c, "/") < 2 || gitSystemDirs[c] {
		return "", fmt.Errorf("%s is not a service directory: choose a directory below a server root (/opt/<service>)", c)
	}
	for _, tr := range gitSystemTrees {
		if c == tr || strings.HasPrefix(c, tr+"/") {
			return "", fmt.Errorf("%s is a system directory", c)
		}
	}
	return c, nil
}

func (u *gitUnitSpec) normalize(dir string) error {
	u.Name, u.User, u.Command, u.WorkDir = strings.TrimSpace(u.Name), strings.TrimSpace(u.User), strings.TrimSpace(u.Command), strings.TrimSpace(u.WorkDir)
	switch u.Kind {
	case "systemd":
		u.Name = strings.TrimSuffix(u.Name, ".service")
	case "supervisor":
		u.Enable = false
	default:
		return fmt.Errorf("the unit kind must be systemd or supervisor")
	}
	if !gitUnitNameRe.MatchString(u.Name) {
		return fmt.Errorf("invalid unit name %q (letters, digits, _ . @ -)", u.Name)
	}
	if u.User != "" && !gitUnixUserRe.MatchString(u.User) {
		return fmt.Errorf("invalid user name %q", u.User)
	}
	if u.Command == "" || len(u.Command) > 500 || strings.ContainsAny(u.Command, "\n\r\x00") {
		return fmt.Errorf("the unit command must be one line of at most 500 characters")
	}
	if u.WorkDir == "" {
		u.WorkDir = dir
	}
	if !strings.HasPrefix(u.WorkDir, "/") || strings.ContainsAny(u.WorkDir, "\n\r\t\x00") {
		return fmt.Errorf("the working directory must be an absolute path")
	}
	return nil
}

// unitFile returns the file name and content of a unit (a small template).
func (u *gitUnitSpec) file() (string, string) {
	var b strings.Builder
	if u.Kind == "systemd" {
		fmt.Fprintf(&b, "[Unit]\nDescription=%s (installed by WRM)\nAfter=network.target\n\n[Service]\nType=simple\n", u.Name)
		if u.User != "" {
			fmt.Fprintf(&b, "User=%s\n", u.User)
		}
		fmt.Fprintf(&b, "WorkingDirectory=%s\nExecStart=%s\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n", u.WorkDir, strings.ReplaceAll(u.Command, "%", "%%"))
		return u.Name + ".service", b.String()
	}
	fmt.Fprintf(&b, "[program:%s]\ncommand=%s\ndirectory=%s\n", u.Name, strings.ReplaceAll(u.Command, "%", "%%"), strings.ReplaceAll(u.WorkDir, "%", "%%"))
	if u.User != "" {
		fmt.Fprintf(&b, "user=%s\n", u.User)
	}
	b.WriteString("autostart=true\nautorestart=true\nstopasgroup=true\nkillasgroup=true\n")
	return u.Name, b.String()
}

// createUnit writes the unit file (never over an existing one) as root or with sudo -n,
// reloads systemd (and enables the unit) or makes supervisor read it. With start the
// supervisor program is added (and started) right away.
func createUnit(cl *ssh.Client, u *gitUnitSpec, start bool) (gitUnit, []string, error) {
	name, content := u.file()
	var sb strings.Builder
	sb.WriteString(gitSudoPrelude + "\none() { printf '%s' \"$1\" | tr '\\n\\t\\r' '   ' | cut -c1-300; }\n")
	if u.Kind == "systemd" {
		fmt.Fprintf(&sb, "D=%s; F=\"$D\"/%s\n[ -d \"$D\" ] || { echo \"E\tno systemd unit directory $D\"; exit 0; }\n", shellQuote(gitSystemdDirs[0]), shellQuote(name))
	} else {
		var dirs []string
		for _, d := range gitSupervisorDirs {
			if d != "/etc" && d != "/etc/supervisor" {
				dirs = append(dirs, shellQuote(d))
			}
		}
		fmt.Fprintf(&sb, "F=\nfor D in %s; do if [ -d \"$D\" ]; then case $D in *supervisord.d) F=\"$D\"/%s.ini;; *) F=\"$D\"/%s.conf;; esac; break; fi; done\n", strings.Join(dirs, " "), shellQuote(name), shellQuote(name))
		sb.WriteString("[ -n \"$F\" ] || { echo \"E\tno supervisor configuration directory (is supervisor installed?)\"; exit 0; }\n")
	}
	sb.WriteString("[ -e \"$F\" ] && { echo \"E\t$F exists already: choose another name\"; exit 0; }\n")
	sb.WriteString("if ! o=$($SU tee \"$F\" 2>&1 >/dev/null); then echo \"E\tcannot write $F (root or sudo -n is needed): $(one \"$o\")\"; exit 0; fi\n")
	sb.WriteString("echo \"P\t$F\"\n")
	q := shellQuote(name)
	if u.Kind == "systemd" {
		sb.WriteString("o=$($SU systemctl daemon-reload 2>&1) || echo \"W\tsystemctl daemon-reload: $(one \"$o\")\"\n")
		if u.Enable {
			fmt.Fprintf(&sb, "o=$($SU systemctl enable %s 2>&1) || echo \"W\tsystemctl enable: $(one \"$o\")\"\n", q)
		}
	} else {
		sb.WriteString("o=$($SU supervisorctl reread 2>&1) || echo \"W\tsupervisorctl reread: $(one \"$o\")\"\n")
		if start {
			fmt.Fprintf(&sb, "o=$($SU supervisorctl update %s 2>&1) || echo \"W\tsupervisorctl update: $(one \"$o\")\"\n", q)
		}
	}
	sb.WriteString("echo WRM_OK\n")
	out, err := runRemoteScript(cl, sb.String(), content, gitStepTimeout)
	var warns []string
	where := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "E\t"):
			return gitUnit{}, nil, errors.New(line[2:])
		case strings.HasPrefix(line, "W\t"):
			warns = append(warns, line[2:])
		case strings.HasPrefix(line, "P\t"):
			where = line[2:]
		}
	}
	if err != nil {
		return gitUnit{}, warns, err
	}
	if !strings.Contains(out, "WRM_OK") {
		return gitUnit{}, warns, fmt.Errorf("the unit was not written completely")
	}
	if u.Kind == "supervisor" && !start {
		warns = append(warns, "supervisorctl update "+u.Name+" adds and starts the program")
	}
	return gitUnit{Kind: u.Kind, Name: name}, append([]string{where}, warns...), nil
}

// ─── pre-checks on the target ────────────────────────

// gitDirCheck is the state of a target directory and its server.
type gitDirCheck struct {
	Path     string            `json:"path"`
	Exists   bool              `json:"exists"`
	Existing bool              `json:"existing"` // files are there (an installation)
	Files    int               `json:"files"`
	Version  string            `json:"version,omitempty"` // its VERSION.md
	App      string            `json:"app,omitempty"`
	Parent   string            `json:"parent"` // nearest existing directory
	Writable bool              `json:"writable"`
	FreeKB   int64             `json:"free_kb"`
	NeedKB   int64             `json:"need_kb"`
	Tools    map[string]string `json:"tools"` // python3 / node → version, tar → yes
	Needs    []string          `json:"needs"`
	Errors   []string          `json:"errors"`
	Warnings []string          `json:"warnings"`
	notDir   bool
	owner    string
}

// precheckDirs looks at the target directories of one server; it returns the host and the
// SSH user too.
func precheckDirs(cl *ssh.Client, dirs []string) (map[string]*gitDirCheck, string, string, error) {
	var sb strings.Builder
	sb.WriteString(`echo "H	$(hostname 2>/dev/null || uname -n)"` + "\n" + `echo "U	$(id -un 2>/dev/null || whoami)"` + "\n")
	sb.WriteString("for x in python3 node; do command -v $x >/dev/null 2>&1 && printf 'T\\t%s\\t%s\\n' $x \"$($x --version 2>&1 | head -n 1)\"; done\n")
	sb.WriteString("command -v tar >/dev/null 2>&1 && printf 'T\\ttar\\tyes\\n'\n")
	for i, d := range dirs {
		fmt.Fprintf(&sb, "D=%s; I=%d\n", shellQuote(d), i)
		sb.WriteString(`if [ -e "$D" ] && [ ! -d "$D" ]; then printf '%s\tX\tnotdir\n' $I; fi
if [ -d "$D" ]; then printf '%s\tX\texists\n' $I; printf '%s\tN\t%s\n' $I "$(find "$D" -type f 2>/dev/null | head -n 100000 | wc -l | tr -d ' ')"
  [ -f "$D/VERSION.md" ] && head -n 12 "$D/VERSION.md" | sed "s/^/$I	V	/"; fi
P=$D; while [ ! -d "$P" ]; do P=${P%/*}; [ -n "$P" ] || P=/; done
printf '%s\tP\t%s\n' $I "$P"
if [ -w "$P" ]; then printf '%s\tW\t1\n' $I; else printf '%s\tW\t0\n' $I; fi
printf '%s\tF\t%s\n' $I "$(df -Pk "$P" 2>/dev/null | awk 'NR==2 {print $4}')"
printf '%s\tO\t%s\n' $I "$(ls -ldn "$P" 2>/dev/null | awk '{print $3":"$4}')"
`)
	}
	sb.WriteString("echo WRM_DONE\n")
	out, err := runRemoteScript(cl, sb.String(), "", gitStepTimeout)
	if err != nil {
		return nil, "", "", err
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, "", "", fmt.Errorf("the server did not answer completely")
	}
	res := map[string]*gitDirCheck{}
	tools := map[string]string{}
	host, user := "", ""
	vlines := map[int][]string{}
	for _, d := range dirs {
		res[d] = &gitDirCheck{Path: d, Tools: tools, Needs: []string{}, Errors: []string{}, Warnings: []string{}}
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		switch {
		case len(parts) >= 2 && parts[0] == "H":
			host = strings.TrimSpace(parts[1])
		case len(parts) >= 2 && parts[0] == "U":
			user = strings.TrimSpace(parts[1])
		case len(parts) == 3 && parts[0] == "T":
			tools[parts[1]] = strings.TrimSpace(parts[2])
		case len(parts) == 3:
			i, err := strconv.Atoi(parts[0])
			if err != nil || i < 0 || i >= len(dirs) {
				continue
			}
			c := res[dirs[i]]
			v := parts[2]
			switch parts[1] {
			case "X":
				c.notDir = c.notDir || v == "notdir"
				c.Exists = c.Exists || v == "exists"
			case "N":
				c.Files = atoiDefault(v, 0)
			case "V":
				vlines[i] = append(vlines[i], v)
			case "P":
				c.Parent = v
			case "W":
				c.Writable = v == "1"
			case "F":
				c.FreeKB = int64(atoiDefault(v, 0))
			case "O":
				c.owner = strings.TrimSpace(v)
			}
		}
	}
	for i, d := range dirs {
		c := res[d]
		c.Existing = c.Exists && c.Files > 0
		if v := vlines[i]; len(v) > 0 {
			info := parseVersionMD(v)
			c.Version, c.App = info.Version, info.App
		}
	}
	return res, host, user, nil
}

// evaluate adds the errors and warnings of a target directory for files of needBytes and
// the relative paths rels (which tools they need).
func (c *gitDirCheck) evaluate(needBytes int64, rels []string, overwrite bool) {
	c.Errors, c.Warnings, c.Needs = []string{}, []string{}, []string{}
	c.NeedKB = needBytes*11/10/1024 + 1024
	if c.notDir {
		c.Errors = append(c.Errors, c.Path+" exists and is not a directory")
	}
	if !c.Writable {
		c.Errors = append(c.Errors, fmt.Sprintf("%s is not writable for the SSH user", c.Parent))
	}
	if c.FreeKB > 0 && c.FreeKB < c.NeedKB {
		c.Errors = append(c.Errors, fmt.Sprintf("not enough free space in %s: %d MB free, %d MB needed", c.Parent, c.FreeKB/1024, c.NeedKB/1024+1))
	} else if c.FreeKB == 0 {
		c.Warnings = append(c.Warnings, "the free space could not be read (df)")
	}
	if c.Tools["tar"] == "" {
		c.Errors = append(c.Errors, "tar is needed on the server")
	}
	py, js := false, false
	for _, r := range rels {
		switch strings.ToLower(path.Ext(r)) {
		case ".py":
			py = true
		case ".js", ".mjs", ".cjs":
			js = true
		}
		js = js || path.Base(r) == "package.json"
	}
	for _, x := range []struct {
		tool string
		need bool
	}{{"python3", py}, {"node", js}} {
		if !x.need {
			continue
		}
		c.Needs = append(c.Needs, x.tool)
		if c.Tools[x.tool] == "" {
			c.Warnings = append(c.Warnings, x.tool+" is not installed on the server (the service has such files; their checks are skipped)")
		}
	}
	if c.Existing && !overwrite {
		v := ""
		if c.Version != "" {
			v = ", VERSION.md " + c.Version
		}
		c.Errors = append(c.Errors, fmt.Sprintf("%s already holds an installation (%d files%s): confirm the overwrite (the replaced files are backed up first)", c.Path, c.Files, v))
	}
}

// ─── targets and contents ────────────────────────────

// installTarget resolves the ref of an install: "" = the service's current target,
// bundle:<source id> = that imported bundle, otherwise a branch or tag (Git API).
func installTarget(userID int, app, ref string) (gitTarget, error) {
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "":
		t, ok := loadGitTarget(userID, app)
		if !ok || t.Error != "" || len(t.Files) == 0 {
			return t, fmt.Errorf("%s: no target yet (refresh the targets)", app)
		}
		return t, nil
	case strings.HasPrefix(ref, "bundle:"):
		id, _ := strconv.Atoi(strings.TrimPrefix(ref, "bundle:"))
		src, found := loadGitSource(userID, id)
		if !found || src.Kind != "bundle" {
			return gitTarget{}, fmt.Errorf("bundle not found")
		}
		m, err := loadBundleManifest(userID, id)
		if err != nil {
			return gitTarget{}, err
		}
		t, ok := bundleTarget(src, m, app)
		if !ok {
			return gitTarget{}, fmt.Errorf("the bundle %s does not contain %s", m.BundleID, app)
		}
		return t, nil
	}
	cat, st := loadGitWorkspace(userID)
	return upgradeTarget(userID, cat, st, app, ref)
}

// installRels are the files an install writes: the target without protected files.
func installRels(t gitTarget) []string {
	var out []string
	for rel := range t.Files {
		if !toolFiles[rel] && !globHit(rel, t.Protected) {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

func targetBytes(t gitTarget, rels []string) int64 {
	var n int64
	for _, r := range rels {
		n += t.Files[r].Size
	}
	return n
}

func fileMode(data []byte) int64 {
	if bytes.HasPrefix(data, []byte("#!")) {
		return 0o755
	}
	return 0o644
}

// tarOf packs files (rel → content) with their modes into an uncompressed tar.
func tarOf(files map[string][]byte, modes map[string]int64) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	rels := keysOf(files)
	sort.Strings(rels)
	now := time.Now().Truncate(time.Second) // whole seconds: plain ustar headers where the names allow it
	for _, rel := range rels {
		// The names end up in tar headers that the target host extracts; a bundle key with
		// ".." or an absolute path would write outside the staging directory.
		if !validBundleRel(rel) {
			return nil, fmt.Errorf("refusing to pack %q: the name is not a plain relative path", rel)
		}
		data := files[rel]
		if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: modes[rel], Size: int64(len(data)), ModTime: now, Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// vaultValue reads a credential for a template field: one the user may use, and only
// for servers its host patterns allow.
func vaultValue(userID int, v gitFieldValue, conn Connection) (string, error) {
	cr, err := loadCredential(v.CredID)
	if err != nil || !credentialAccessible(cr, userID) {
		return "", fmt.Errorf("credential not found")
	}
	if !hostAllowed(cr.Hosts, conn.Host) {
		return "", fmt.Errorf("the credential %s is not allowed for %s", cr.Name, conn.Name)
	}
	if v.Part == "username" {
		return cr.Username, nil
	}
	pw := decryptValue(cr.password)
	if pw == "" {
		return "", fmt.Errorf("the credential %s has no password", cr.Name)
	}
	return pw, nil
}

// readInstallFile reads a (protected) file of another installation of the user.
func readInstallFile(userID, installID int, rel string, clients map[int]*ssh.Client) ([]byte, error) {
	list := loadGitInstalls(userID, false, "i.id=?", installID)
	if len(list) == 0 {
		return nil, fmt.Errorf("installation not found")
	}
	in := list[0]
	cl := clients[in.ConnID]
	if cl == nil {
		conn, err := loadConnection(in.ConnID)
		if err != nil || conn.UserID != userID || !isSSHProtocol(conn) {
			return nil, fmt.Errorf("connection not found")
		}
		if cl, err = dialSSH(conn, nil); err != nil {
			return nil, fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
		}
		clients[in.ConnID] = cl
	}
	out, err := runRemoteScript(cl, fmt.Sprintf(`f=%s; [ -f "$f" ] || { echo WRM_NOFILE; exit 0; }; s=$(wc -c < "$f"); [ "$s" -le %d ] || { echo WRM_TOOBIG; exit 0; }; echo WRM_FILE; cat -- "$f"`,
		shellQuote(in.Path+"/"+rel), gitMaxFillFile), "", time.Minute)
	switch {
	case err != nil:
		return nil, err
	case strings.HasPrefix(out, "WRM_NOFILE"):
		return nil, fmt.Errorf("%s does not exist in %s on %s", rel, in.Path, in.ConnName)
	case strings.HasPrefix(out, "WRM_TOOBIG"):
		return nil, fmt.Errorf("%s in %s on %s is too large", rel, in.Path, in.ConnName)
	case !strings.HasPrefix(out, "WRM_FILE\n"):
		return nil, fmt.Errorf("%s could not be read", rel)
	}
	return []byte(strings.TrimPrefix(out, "WRM_FILE\n")), nil
}

// ─── staging, placing and the tar stream ─────────────

// receiveScript creates the target directory (printing the topmost directory it created),
// unpacks a tar from stdin into a staging directory inside it and hashes what arrived.
func receiveScript(dir, staging, owner string) string {
	var sb strings.Builder
	sb.WriteString(gitHashPrelude)
	fmt.Fprintf(&sb, "D=%s; T=%s\n", shellQuote(dir), shellQuote(staging))
	sb.WriteString(`if [ -e "$D" ] && [ ! -d "$D" ]; then echo "E	$D is not a directory"; exit 0; fi
if [ ! -d "$D" ]; then m=$D; while [ -n "${m%/*}" ] && [ ! -d "${m%/*}" ]; do m=${m%/*}; done
  mkdir -p "$D" || { echo "E	cannot create $D"; exit 0; }; printf 'M\t%s\n' "$m"; fi
rm -rf "$T"; mkdir "$T" || { echo "E	cannot create the staging directory $T"; exit 0; }
if ! o=$(tar -xof - -C "$T" 2>&1); then printf 'E\tunpacking failed: %s\n' "$(printf '%s' "$o" | tr '\n\t\r' '   ' | cut -c1-300)"; exit 0; fi
`)
	if owner != "" {
		fmt.Fprintf(&sb, "chown -R %s \"$T\" 2>/dev/null\n", shellQuote(owner))
	}
	sb.WriteString(`cd "$T" || exit 3
find . -type f | while IFS= read -r f; do printf 'F\t%s\t%s\n' "$(tr -d '\r' < "$f" | $S | cut -c1-64)" "${f#./}"; done
echo WRM_DONE
`)
	return sb.String()
}

// parseReceive reads the answer of receiveScript into the created directory and the hashes.
func (s *deploySession) parseReceive(out string, err error) (map[string]string, error) {
	got := map[string]string{}
	var fail string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "M\t"):
			s.created = append(s.created, line[2:])
		case strings.HasPrefix(line, "E\t"):
			fail = line[2:]
		case strings.HasPrefix(line, "F\t"):
			if p := strings.SplitN(line[2:], "\t", 2); len(p) == 2 {
				got[p[1]] = p[0]
			}
		}
	}
	switch {
	case fail != "":
		return nil, errors.New(fail)
	case err != nil:
		return nil, err
	case !strings.Contains(out, "WRM_DONE"):
		return nil, fmt.Errorf("the transfer did not finish")
	}
	return got, nil
}

// verifyStaged compares the hashes of the staged files with the expected ones.
func verifyStaged(got, want map[string]string) error {
	var bad []string
	for rel, h := range want {
		switch g, ok := got[rel]; {
		case !ok:
			bad = append(bad, rel+" (missing)")
		case g != h:
			bad = append(bad, rel+" (differs)")
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	if len(bad) > 10 {
		bad = append(bad[:10], fmt.Sprintf("+%d", len(bad)-10))
	}
	return fmt.Errorf("the files did not arrive intact: %s", strings.Join(bad, ", "))
}

// dropStaging removes the staging directory and the directories the receive created.
func (s *deploySession) dropStaging(staging string) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "rm -rf %s\n", shellQuote(staging))
	for i := len(s.created) - 1; i >= 0; i-- {
		fmt.Fprintf(&sb, "find %s -depth -type d -exec rmdir {} \\; 2>/dev/null\n", shellQuote(s.created[i]))
	}
	s.run(sb.String(), "", time.Minute)
}

// runRemotePipe streams the output of a script on one server into a script on another
// (tar -c on A → tar -x on B) and returns the output of the second.
func runRemotePipe(src *ssh.Client, srcScript, srcStdin string, dst *ssh.Client, dstScript string, timeout time.Duration) (string, error) {
	ss, err := src.NewSession()
	if err != nil {
		return "", err
	}
	defer ss.Close()
	ds, err := dst.NewSession()
	if err != nil {
		return "", err
	}
	defer ds.Close()
	pr, pw := io.Pipe()
	srcErr := &capBuffer{max: 16 << 10}
	out, dstErr := &capBuffer{max: 4 << 20}, &capBuffer{max: 16 << 10}
	ss.Stdin, ss.Stdout, ss.Stderr = strings.NewReader(srcStdin), pw, srcErr
	ds.Stdin, ds.Stdout, ds.Stderr = pr, out, dstErr
	if err := ds.Start("sh -c " + shellQuote(dstScript)); err != nil {
		return "", err
	}
	if err := ss.Start("sh -c " + shellQuote(srcScript)); err != nil {
		pw.Close()
		return "", err
	}
	srcDone := make(chan error, 1)
	go func() {
		err := ss.Wait()
		pw.Close()
		srcDone <- err
	}()
	dstDone := make(chan error, 1)
	go func() { dstDone <- ds.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var dErr, sErr error
	select {
	case dErr = <-dstDone:
	case <-timer.C:
		ss.Close()
		ds.Close()
		pr.Close()
		return "", fmt.Errorf("the transfer did not finish within %s", timeout)
	}
	pr.CloseWithError(io.ErrClosedPipe) // the source stops when the target is gone
	select {
	case sErr = <-srcDone:
	case <-time.After(30 * time.Second):
		ss.Close()
		sErr = fmt.Errorf("the source did not finish")
	}
	if dErr != nil {
		msg := strings.TrimSpace(dstErr.String())
		if msg == "" {
			msg = dErr.Error()
		}
		return out.String(), errors.New(truncateStr(msg, 400))
	}
	if sErr != nil {
		msg := strings.TrimSpace(srcErr.String())
		if msg == "" {
			msg = sErr.Error()
		}
		return out.String(), fmt.Errorf("the source: %s", truncateStr(msg, 400))
	}
	return out.String(), nil
}

// placeStaged moves verified staged files into the installation: a backup of the files it
// replaces (when the directory held some), the directories, the renames, the checks;
// everything is rolled back on failure.
func (c itemCtx) placeStaged(s *deploySession, staging string, rels []string, existing map[string]string, bundle string, now time.Time, custom string) ([]*deployOp, error) {
	var ops []*deployOp
	var replaced, check []string
	for _, rel := range rels {
		_, had := existing[rel]
		ops = append(ops, &deployOp{Rel: rel, Existed: had, tmp: staging + "/" + rel})
		if had {
			replaced = append(replaced, rel)
		}
		switch strings.ToLower(path.Ext(rel)) {
		case ".py", ".js", ".mjs", ".cjs", ".sh":
			check = append(check, rel)
		}
	}
	_, vmd := existing["VERSION.md"]
	if len(replaced) > 0 || vmd {
		c.step("backup")
		if err := s.makeBackup(bundle, now, replaced, true); err != nil {
			c.log("backup", "fail", err.Error())
			s.dropStaging(staging)
			return nil, err
		}
		c.run.set(func() { c.it.MadeBackup = s.backup })
		c.log("backup", "ok", fmt.Sprintf("%s (%d files)", s.backup, len(replaced)))
	}
	c.step("write")
	rollback := func(cause error) error {
		c.step("rollback")
		s.run("rm -rf "+shellQuote(staging), "", time.Minute)
		if rerr := s.restore(ops, true); rerr != nil {
			c.log("rollback", "fail", rerr.Error())
			return fmt.Errorf("%v; THE AUTOMATIC ROLLBACK FAILED: %v", cause, rerr)
		}
		c.log("rollback", "ok", "the previous state is back")
		c.run.set(func() { c.it.State, c.it.MadeBackup = "rolled_back", "" })
		return fmt.Errorf("%v (rolled back)", cause)
	}
	dirs := map[string]bool{}
	for _, rel := range rels {
		if d := path.Dir(rel); d != "." {
			dirs[d] = true
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "D=%s\ncd \"$D\" || exit 3\n", shellQuote(s.in.Path))
	for _, d := range keys(dirs) {
		fmt.Fprintf(&sb, "d=%s; if [ ! -d \"$d\" ]; then m=$d; while [ \"${m%%/*}\" != \"$m\" ] && [ ! -d \"${m%%/*}\" ]; do m=${m%%/*}; done; mkdir -p \"$d\" || exit 3; printf 'M\\t%%s\\n' \"$D/$m\"; fi\n", shellQuote(d))
	}
	sb.WriteString("echo WRM_OK\n")
	out, err := s.run(sb.String(), "", gitStepTimeout)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "M\t") {
			s.created = append(s.created, line[2:])
		}
	}
	if err == nil && !strings.Contains(out, "WRM_OK") {
		err = fmt.Errorf("the directories could not be created")
	}
	if err == nil {
		err = s.commit(ops)
	}
	if err != nil {
		c.log("write", "fail", err.Error())
		return nil, rollback(err)
	}
	s.run("rm -rf "+shellQuote(staging), "", time.Minute)
	c.log("write", "ok", fmt.Sprintf("%d files in %s", len(ops), s.in.Path))
	c.step("checks")
	passed, err := s.checks(check, custom)
	if err != nil {
		c.log("checks", "fail", err.Error())
		return nil, rollback(fmt.Errorf("check failed: %v", err))
	}
	if len(passed) > 0 {
		c.log("checks", "ok", fmt.Sprintf("%d passed", len(passed)))
	} else {
		c.log("checks", "skip", "no checks for these files")
	}
	return ops, nil
}

// ─── runs ────────────────────────────────────────────

func (c itemCtx) dial(connID int, step string) (Connection, *ssh.Client, error) {
	conn, err := loadConnection(connID)
	if err != nil || conn.UserID != c.run.UserID || !isSSHProtocol(conn) {
		return conn, nil, fmt.Errorf("connection not found")
	}
	c.step(step)
	cl, err := dialSSH(conn, nil)
	if err != nil {
		c.log(step, "fail", err.Error())
		return conn, nil, fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
	}
	c.log(step, "ok", conn.Name)
	return conn, cl, nil
}

// targetCheck runs the pre-checks of the target directory and reads the existing files.
func (c itemCtx) targetCheck(s *deploySession, needBytes int64, rels []string, overwrite bool) (map[string]string, error) {
	c.step("precheck")
	checks, host, user, err := precheckDirs(s.cl, []string{s.in.Path})
	if err != nil {
		c.log("precheck", "fail", err.Error())
		return nil, err
	}
	dc := checks[s.in.Path]
	dc.evaluate(needBytes, rels, overwrite)
	s.host, s.user, s.tools = host, user, map[string]bool{}
	if s.host == "" {
		s.host = s.in.ConnName
	}
	for k, v := range dc.Tools {
		s.tools[k] = v != ""
	}
	if len(dc.Errors) > 0 {
		c.log("precheck", "fail", strings.Join(dc.Errors, "; "))
		return nil, errors.New(strings.Join(dc.Errors, "; "))
	}
	msg := fmt.Sprintf("%s: %d MB free, %d MB needed", dc.Parent, dc.FreeKB/1024, dc.NeedKB/1024+1)
	if len(dc.Warnings) > 0 {
		msg += "; " + strings.Join(dc.Warnings, "; ")
	}
	c.log("precheck", "ok", msg)
	existing := map[string]string{}
	if dc.Exists {
		s.owner = dc.owner
		scans, err := scanOn(s.cl, []string{s.in.Path})
		if err != nil {
			return nil, err
		}
		if sr := scans[s.in.Path]; sr != nil {
			existing = sr.Files
			if sr.Capped {
				return nil, fmt.Errorf("the target has more than %d files", gitMaxInstallFiles)
			}
			if from := parseVersionMD(sr.Version).Version; from != "" {
				c.run.set(func() { c.it.FromVersion = from })
			}
		}
		if len(existing) > 0 {
			c.log("precheck", "info", fmt.Sprintf("an installation is replaced (%d files): the replaced files are backed up", len(existing)))
		}
	}
	return existing, nil
}

// register stores the new installation (added by hand, so discovery keeps it), compares it
// and creates the unit; it returns the installation with its units.
func (c itemCtx) register(s *deploySession, unit *gitUnitSpec) error {
	c.step("register")
	db.Exec(`INSERT INTO git_installs (user_id, conn_id, app, path, env, manual, state, data, checked_at, discovered_at) VALUES (?,?,?,?,?,1,'unknown','','',?)
		ON CONFLICT(user_id, conn_id, path) DO UPDATE SET app=excluded.app, env=excluded.env, manual=1`,
		c.run.UserID, s.in.ConnID, s.in.App, s.in.Path, truncateStr(s.in.Env, 40), nowStamp())
	db.QueryRow(`SELECT id FROM git_installs WHERE user_id=? AND conn_id=? AND path=?`, c.run.UserID, s.in.ConnID, s.in.Path).Scan(&s.in.ID)
	c.run.set(func() { c.it.InstallID = s.in.ID })
	c.log("register", "ok", fmt.Sprintf("installation #%d", s.in.ID))
	var unitErr error
	if unit != nil {
		c.step("unit")
		start := c.run.P.Restart.Mode == "now"
		if _, notes, err := createUnit(s.cl, unit, start); err != nil {
			c.log("unit", "fail", err.Error())
			unitErr = fmt.Errorf("unit: %v (the files are installed)", err)
		} else {
			c.log("unit", "ok", strings.Join(notes, "; "))
		}
	}
	refreshInstallState(c.run.UserID, s.in, s.cl)
	if list := loadGitInstalls(c.run.UserID, false, "i.id=?", s.in.ID); len(list) == 1 {
		s.in.Units = list[0].Units
	}
	c.run.set(func() { c.it.Units = s.in.Units })
	if len(s.in.Units) > 0 && c.run.P.Restart.Mode != "now" {
		db.Exec(`INSERT INTO git_pending_restarts (install_id, user_id, since, run_id) VALUES (?,?,?,?)
			ON CONFLICT(install_id) DO UPDATE SET since=excluded.since, run_id=excluded.run_id`, s.in.ID, c.run.UserID, nowStamp(), c.run.ID)
	}
	if unitErr != nil {
		c.run.set(func() { c.it.State, c.it.Step = "failed", "" })
		return unitErr
	}
	c.run.set(func() { c.it.State, c.it.Step = "ok", "" })
	return nil
}

// installItem installs one service of an install run.
func (run *gitRun) installItem(idx int, it *gitRunItem) error {
	c := itemCtx{run, it}
	pv := run.P.Provision
	if pv == nil || idx >= len(pv.Services) {
		return fmt.Errorf("the request is incomplete")
	}
	svc := pv.Services[idx]
	t, ok := run.P.Targets[svc.App]
	if !ok {
		return fmt.Errorf("no target for %s", svc.App)
	}
	run.set(func() { it.FromVersion, it.ToVersion = "-", t.Version })
	conn, cl, err := c.dial(pv.ConnID, "connect")
	if err != nil {
		return err
	}
	defer cl.Close()
	s := &deploySession{cl: cl, in: gitInstall{ConnID: conn.ID, ConnName: conn.Name, App: svc.App, Path: svc.Path, Env: svc.Env}}

	// contents: the files of the ref and the filled protected files
	c.step("fetch")
	providers := map[int]*gitProvider{}
	files, modes, want := map[string][]byte{}, map[string]int64{}, map[string]string{}
	rels := installRels(t)
	if len(rels) == 0 {
		return fmt.Errorf("the target has no files to install")
	}
	for _, rel := range rels {
		data, err := gitFileContent(run.UserID, t, rel, providers)
		if err != nil {
			c.log("fetch", "fail", err.Error())
			return err
		}
		files[rel], modes[rel] = data, fileMode(data)
	}
	clients := map[int]*ssh.Client{}
	defer func() {
		for _, x := range clients {
			x.Close()
		}
	}()
	var filled []string
	for _, f := range svc.Files {
		var data []byte
		switch f.Mode {
		case "skip":
			continue
		case "manual":
			data = []byte(f.Content)
		case "copy":
			if data, err = readInstallFile(run.UserID, f.FromInstall, f.Path, clients); err != nil {
				c.log("fetch", "fail", err.Error())
				return err
			}
		case "template":
			tmpl, err := gitFileContent(run.UserID, t, f.Template, providers)
			if err != nil {
				c.log("fetch", "fail", err.Error())
				return err
			}
			values := map[string]string{}
			for id, v := range f.Values {
				if v.CredID != 0 {
					if v.Value, err = vaultValue(run.UserID, v, conn); err != nil {
						c.log("fetch", "fail", f.Path+": "+err.Error())
						return fmt.Errorf("%s: %v", f.Path, err)
					}
				}
				values[id] = v.Value
			}
			var left []string
			data, left = fillTemplate(tmpl, values)
			if len(left) > 0 {
				c.log("fetch", "info", fmt.Sprintf("%s: placeholders left unfilled: %s", f.Path, strings.Join(left, ", ")))
			}
		}
		files[f.Path], modes[f.Path] = data, 0o640
		filled = append(filled, f.Path)
	}
	var total int64
	for rel, data := range files {
		want[rel] = trHash(data)
		total += int64(len(data))
	}
	all := keysOf(files)
	sort.Strings(all)
	msg := fmt.Sprintf("%d files of %s (%s)", len(rels), t.Version, t.Source)
	if len(filled) > 0 {
		msg += "; per-host: " + strings.Join(filled, ", ")
	}
	c.log("fetch", "ok", msg)

	existing, err := c.targetCheck(s, total, all, pv.Overwrite)
	if err != nil {
		return err
	}
	payload, err := tarOf(files, modes)
	if err != nil {
		return err
	}
	staging := svc.Path + "/" + gitStagingPrefix + run.Label
	c.step("receive")
	owner := ""
	if len(existing) > 0 {
		owner = s.owner
	}
	out, err := s.run(receiveScript(svc.Path, staging, owner), string(payload), gitProvisionTimeout)
	got, err := s.parseReceive(out, err)
	if err == nil {
		err = verifyStaged(got, want)
	}
	if err != nil {
		c.log("receive", "fail", err.Error())
		s.dropStaging(staging)
		return err
	}
	c.log("receive", "ok", fmt.Sprintf("%d files, %d KB", len(files), total/1024+1))
	now := time.Now()
	bundle := bundleOf(t, run.Label)
	ops, err := c.placeStaged(s, staging, all, existing, bundle, now, run.P.Checks[svc.App])
	if err != nil {
		return err
	}
	written := map[string]bool{}
	for _, r := range all {
		written[r] = true
	}
	_, vmdExists := existing["VERSION.md"]
	if err := c.writeVersion(s, ops, versionMD(t, versionRows(t, nil, written), now, s.host, s.user, bundle, false), vmdExists); err != nil {
		return err
	}
	run.set(func() { it.Written = all })
	c.updateLog(s, map[string]interface{}{"app": svc.App, "backup": s.backup, "branch": t.Branch, "bundle": bundle, "env": svc.Env,
		"files": all, "from_version": it.FromVersion, "host": s.host, "install": svc.Path, "services": []string{},
		"time": now.Format("2006-01-02T15:04:05"), "to_version": fmt.Sprintf("%s (%s)", short10(t.Commit), t.CommitDate), "user": s.user})
	return c.register(s, svc.Unit)
}

// transferExcluded tells why a file of an installation is not transferred ("" = it is):
// logs, caches, .deploy-bak, the catalog's ignored directories and backup-looking paths.
func transferExcluded(rel string, ignore, words []string) string {
	segs := strings.Split(rel, "/")
	for _, seg := range segs[:len(segs)-1] {
		switch strings.ToLower(seg) {
		case "log", "logs":
			return "log"
		case "__pycache__", ".pytest_cache", ".mypy_cache", ".cache":
			return "cache"
		case gitBackupDir, ".git":
			return "backup"
		}
		if strings.HasPrefix(seg, gitStagingPrefix) {
			return "cache"
		}
		if looksLikeBackup(seg, words) {
			return "backup"
		}
		for _, g := range ignore {
			if fnmatch(seg, g) || fnmatch(strings.ToLower(seg), strings.ToLower(g)) {
				return "ignored"
			}
		}
	}
	base := strings.ToLower(segs[len(segs)-1])
	switch {
	case base == "updates.jsonl" || strings.HasSuffix(base, ".log") || strings.Contains(base, ".log.") || fnmatch(base, "*.log[0-9]*"):
		return "log"
	case strings.HasSuffix(base, ".pyc") || strings.HasSuffix(base, ".pyo") || strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".wrm-tmp") || strings.HasSuffix(base, ".wrm-back"):
		return "cache"
	case strings.HasSuffix(base, "~"):
		return "backup"
	}
	// a backup word after the first token of the name: run.py.bak, config_old.ini
	toks := splitTokens(base)
	for i, tok := range toks {
		if i == 0 {
			continue
		}
		for _, w := range words {
			if tok == w || (strings.HasPrefix(tok, w) && len(tok) > len(w) && tok[len(w)] >= '0' && tok[len(w)] <= '9') {
				return "backup"
			}
		}
	}
	return ""
}

type gitSourceFile struct {
	Hash string
	Size int64
}

// listForTransfer lists the files of an installation with their hashes and sizes (the
// usual prunes: .git, .deploy-bak, node_modules, __pycache__, virtual environments).
func listForTransfer(cl *ssh.Client, dir string) (map[string]gitSourceFile, []string, error) {
	var sb strings.Builder
	sb.WriteString(gitHashPrelude)
	fmt.Fprintf(&sb, "D=%s\n[ -d \"$D\" ] || { echo \"E\tthe directory $D does not exist\"; exit 0; }\ncd \"$D\" || exit 3\n", shellQuote(dir))
	fmt.Fprintf(&sb, "find . %s -o -type f -print 2>/dev/null | head -n %d | while IFS= read -r f; do\n", findPrune(), gitMaxInstallFiles+1)
	sb.WriteString(`  if [ -r "$f" ]; then h=$(tr -d '\r' < "$f" | $S | cut -c1-64); n=$(wc -c < "$f" | tr -d ' '); else h=-; n=0; fi
  printf 'F\t%s\t%s\t%s\n' "$h" "$n" "${f#./}"
done
[ -f VERSION.md ] && head -n 40 VERSION.md | sed 's/^/V	/'
echo WRM_DONE
`)
	out, err := runRemoteScript(cl, sb.String(), "", gitScanTimeout)
	if err != nil {
		return nil, nil, err
	}
	files := map[string]gitSourceFile{}
	var vlines []string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "E\t"):
			return nil, nil, errors.New(line[2:])
		case strings.HasPrefix(line, "F\t"):
			p := strings.SplitN(line[2:], "\t", 3)
			if len(p) == 3 {
				n, _ := strconv.ParseInt(p[1], 10, 64)
				files[p[2]] = gitSourceFile{Hash: p[0], Size: n}
			}
		case strings.HasPrefix(line, "V\t"):
			vlines = append(vlines, line[2:])
		}
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, nil, fmt.Errorf("the source did not answer completely")
	}
	if len(files) > gitMaxInstallFiles {
		return nil, nil, fmt.Errorf("the installation has more than %d files", gitMaxInstallFiles)
	}
	return files, vlines, nil
}

// transferSelection splits the files of a source installation into copied and excluded.
func transferSelection(files map[string]gitSourceFile, ignore []string) ([]string, map[string]string) {
	words := backupWords()
	var keep []string
	excluded := map[string]string{}
	for rel := range files {
		if strings.ContainsAny(rel, "\n\t") {
			excluded[rel] = "name"
			continue
		}
		if why := transferExcluded(rel, ignore, words); why != "" {
			excluded[rel] = why
			continue
		}
		keep = append(keep, rel)
	}
	sort.Strings(keep)
	return keep, excluded
}

var (
	vmdHostRe = regexp.MustCompile(`(?m)^(- \*\*Host\*\*:[ \t]*)[^\r\n]*`)
	vmdUserRe = regexp.MustCompile(`(?m)^(- \*\*Korisnik\*\*:[ \t]*)[^\r\n]*`)
)

// retargetVersionMD sets the host and user of a transferred VERSION.md.
func retargetVersionMD(content []byte, host, user string) []byte {
	content = vmdHostRe.ReplaceAllFunc(content, func(m []byte) []byte { return append(vmdHostRe.FindSubmatch(m)[1], host...) })
	return vmdUserRe.ReplaceAllFunc(content, func(m []byte) []byte { return append(vmdUserRe.FindSubmatch(m)[1], user...) })
}

// transferItem copies an installation from its server to the target of the run.
func (run *gitRun) transferItem(it *gitRunItem) error {
	c := itemCtx{run, it}
	pv := run.P.Provision
	if pv == nil {
		return fmt.Errorf("the request is incomplete")
	}
	list := loadGitInstalls(run.UserID, false, "i.id=?", pv.SourceInstall)
	if len(list) == 0 {
		return fmt.Errorf("the source installation is not known any more")
	}
	src := list[0]
	_, scl, err := c.dial(src.ConnID, "source")
	if err != nil {
		return err
	}
	defer scl.Close()
	files, vlines, err := listForTransfer(scl, src.Path)
	if err != nil {
		c.log("source", "fail", err.Error())
		return err
	}
	cat := loadGitCatalog(run.UserID)
	rels, excluded := transferSelection(files, cat.IgnoreDirs)
	if len(rels) == 0 {
		return fmt.Errorf("nothing to transfer from %s", src.Path)
	}
	want := map[string]string{}
	var total int64
	for _, rel := range rels {
		f := files[rel]
		if f.Hash == "-" {
			c.log("source", "fail", rel+" cannot be read")
			return fmt.Errorf("%s cannot be read on %s", rel, src.ConnName)
		}
		want[rel] = f.Hash
		total += f.Size
	}
	srcInfo := parseVersionMD(vlines)
	toV := srcInfo.Version
	if toV == "" {
		toV = "-"
	}
	run.set(func() { it.FromVersion, it.ToVersion = "-", toV })
	c.log("source", "ok", fmt.Sprintf("%d files (%d KB) from %s, %d excluded", len(rels), total/1024+1, src.Path, len(excluded)))

	conn, cl, err := c.dial(pv.ConnID, "connect")
	if err != nil {
		return err
	}
	defer cl.Close()
	s := &deploySession{cl: cl, in: gitInstall{ConnID: conn.ID, ConnName: conn.Name, App: src.App, Path: pv.Path, Env: pv.Env}}
	existing, err := c.targetCheck(s, total, rels, pv.Overwrite)
	if err != nil {
		return err
	}
	staging := pv.Path + "/" + gitStagingPrefix + run.Label
	owner := ""
	if len(existing) > 0 {
		owner = s.owner
	}
	c.step("receive")
	var list2 strings.Builder
	for _, rel := range rels {
		list2.WriteString("./" + rel + "\n")
	}
	srcScript := fmt.Sprintf("cd %s || exit 3\nexec tar -cf - -T -\n", shellQuote(src.Path))
	out, err := runRemotePipe(scl, srcScript, list2.String(), cl, receiveScript(pv.Path, staging, owner), gitProvisionTimeout)
	got, err := s.parseReceive(out, err)
	if err == nil {
		err = verifyStaged(got, want)
	}
	if err != nil {
		c.log("receive", "fail", err.Error())
		s.dropStaging(staging)
		return err
	}
	c.log("receive", "ok", fmt.Sprintf("%d files, %d KB", len(rels), total/1024+1))
	now := time.Now()
	ops, err := c.placeStaged(s, staging, rels, existing, run.Label, now, run.P.Checks[src.App])
	if err != nil {
		return err
	}
	if _, has := want["VERSION.md"]; has {
		vout, verr := s.run("cat "+shellQuote(pv.Path+"/VERSION.md"), "", time.Minute)
		if verr == nil {
			if err := c.writeVersion(s, ops, retargetVersionMD([]byte(vout), s.host, s.user), true); err != nil {
				return err
			}
		}
	}
	run.set(func() { it.Written = rels })
	branch := strings.TrimSpace(strings.Split(srcInfo.Ref, " (")[0])
	c.updateLog(s, map[string]interface{}{"app": src.App, "backup": s.backup, "branch": branch, "bundle": run.Label, "env": pv.Env,
		"files": rels, "from_version": it.FromVersion, "host": s.host, "install": pv.Path, "services": []string{},
		"time": now.Format("2006-01-02T15:04:05"), "to_version": toV, "user": s.user})
	return c.register(s, pv.Unit)
}

// ─── creating a run ──────────────────────────────────

// redactedProvision is what is stored with the run: no typed values or contents.
func redactedProvision(pv *gitProvision) *gitProvision {
	cp := *pv
	cp.Confirm = ""
	cp.Services = nil
	for _, svc := range pv.Services {
		x := *svc
		x.Files = nil
		for _, f := range svc.Files {
			g := f
			g.Content = ""
			g.Values = nil
			for id, v := range f.Values {
				if v.CredID != 0 {
					if g.Values == nil {
						g.Values = map[string]gitFieldValue{}
					}
					g.Values[id] = gitFieldValue{CredID: v.CredID, Part: v.Part}
				}
			}
			x.Files = append(x.Files, g)
		}
		cp.Services = append(cp.Services, &x)
	}
	return &cp
}

// validateFills checks the protected-file choices of a service against its target.
func validateFills(userID int, t gitTarget, svc *gitInstallSvc) error {
	seen := map[string]bool{}
	var keep []gitFillSpec
	for _, f := range svc.Files {
		f.Path = cleanRel(f.Path)
		if f.Path == "" || strings.HasPrefix(f.Path, "..") || toolFiles[f.Path] || seen[f.Path] {
			return fmt.Errorf("invalid protected file %q", f.Path)
		}
		seen[f.Path] = true
		if _, regular := t.Files[f.Path]; regular && !globHit(f.Path, t.Protected) {
			return fmt.Errorf("%s is a file of the repository, not a per-host file", f.Path)
		}
		switch f.Mode {
		case "skip":
			continue
		case "manual":
			if len(f.Content) > gitMaxFillFile {
				return fmt.Errorf("%s: at most 1 MB", f.Path)
			}
		case "copy":
			list := loadGitInstalls(userID, false, "i.id=?", f.FromInstall)
			if len(list) == 0 || list[0].App != t.App {
				return fmt.Errorf("%s: choose an installation of %s to copy from", f.Path, t.App)
			}
		case "template":
			if _, ok := t.Files[f.Template]; !ok {
				return fmt.Errorf("%s: the template %s is not in the target", f.Path, f.Template)
			}
			if len(f.Values) > gitMaxFields {
				return fmt.Errorf("%s: too many values", f.Path)
			}
			for id, v := range f.Values {
				if len(id) > 200 || len(v.Value) > 4096 || (v.Part != "" && v.Part != "password" && v.Part != "username") {
					return fmt.Errorf("%s: invalid value", f.Path)
				}
			}
		default:
			return fmt.Errorf("%s: choose template, copy, manual or skip", f.Path)
		}
		keep = append(keep, f)
	}
	if len(keep) > gitMaxSlots {
		return fmt.Errorf("too many protected files")
	}
	svc.Files = keep
	return nil
}

func connTags(connID int) []string {
	var tags string
	db.QueryRow(`SELECT COALESCE(tags,'') FROM connections WHERE id=?`, connID).Scan(&tags)
	return parseTags(tags)
}

// createProvisionRun validates an install or transfer request, pins the targets and starts
// the run (it never waits for a schedule: it is set up while the user watches).
func createProvisionRun(r *http.Request, userID int, p gitRunParams) (int, int, error) {
	key := gitKindPolicy[p.Kind]
	if !gitActionAllowed(userID, key) {
		return 0, 403, fmt.Errorf("this action is not allowed for your account")
	}
	pv := p.Provision
	if pv == nil {
		return 0, 400, fmt.Errorf("the request is incomplete")
	}
	if strings.TrimSpace(p.ScheduleAt) != "" {
		return 0, 400, fmt.Errorf("an %s runs right away", p.Kind)
	}
	switch p.Restart.Mode {
	case "", "none":
		p.Restart = gitRestartChoice{Mode: "none"}
	case "now":
		if !gitActionAllowed(userID, "git_restart") {
			return 0, 403, fmt.Errorf("restarts are not allowed for your account")
		}
		p.Restart = gitRestartChoice{Mode: "now"}
	default:
		return 0, 400, fmt.Errorf("a new installation can only be started right away")
	}
	checks := map[string]string{}
	for app, c := range p.Checks {
		if c = strings.TrimSpace(c); c != "" {
			if len(c) > 500 || strings.ContainsAny(c, "\n\r") {
				return 0, 400, fmt.Errorf("the check command must be one line of at most 500 characters")
			}
			checks[app] = c
		}
	}
	p.Checks = checks
	conn, err := loadConnection(pv.ConnID)
	if err != nil || conn.UserID != userID || !isSSHProtocol(conn) {
		return 0, 404, fmt.Errorf("choose an SSH connection of yours as the target")
	}
	envs := []string{}
	p.Items, p.Targets = nil, map[string]gitTarget{}
	switch p.Kind {
	case "install":
		if len(pv.Services) == 0 || len(pv.Services) > gitMaxInstallServices {
			return 0, 400, fmt.Errorf("choose 1 to %d services", gitMaxInstallServices)
		}
		known := map[string]bool{}
		for _, a := range effectiveApps(userID, loadGitCatalog(userID)) {
			known[a.Name] = true
		}
		paths := map[string]bool{}
		for _, svc := range pv.Services {
			if svc == nil || !known[svc.App] {
				return 0, 400, fmt.Errorf("unknown service")
			}
			if p.Targets[svc.App].App != "" {
				return 0, 400, fmt.Errorf("%s is listed twice", svc.App)
			}
			dir, err := validInstallDir(svc.Path)
			if err != nil {
				return 0, 400, fmt.Errorf("%s: %v", svc.App, err)
			}
			for other := range paths {
				if dir == other || strings.HasPrefix(dir, other+"/") || strings.HasPrefix(other, dir+"/") {
					return 0, 400, fmt.Errorf("the directories of the services must not overlap")
				}
			}
			paths[dir] = true
			svc.Path, svc.Env = dir, truncateStr(strings.TrimSpace(svc.Env), 40)
			t, err := installTarget(userID, svc.App, svc.Ref)
			if err != nil {
				return 0, 400, err
			}
			if len(installRels(t)) == 0 {
				return 0, 400, fmt.Errorf("%s: the target has no files to install", svc.App)
			}
			if err := validateFills(userID, t, svc); err != nil {
				return 0, 400, fmt.Errorf("%s: %v", svc.App, err)
			}
			if svc.Unit != nil {
				if err := svc.Unit.normalize(dir); err != nil {
					return 0, 400, fmt.Errorf("%s: %v", svc.App, err)
				}
			}
			p.Targets[svc.App] = t
			envs = append(envs, svc.Env)
			p.Items = append(p.Items, &gitRunItem{ConnID: conn.ID, ConnName: conn.Name, App: svc.App, Path: dir, Env: svc.Env, State: "pending", Log: []gitStepLog{}})
		}
	case "transfer":
		list := loadGitInstalls(userID, false, "i.id=?", pv.SourceInstall)
		if len(list) == 0 || !userOwnsConnection(list[0].ConnID, userID) {
			return 0, 404, fmt.Errorf("installation not found")
		}
		src := list[0]
		if strings.TrimSpace(pv.Path) == "" {
			pv.Path = src.Path
		}
		dir, err := validInstallDir(pv.Path)
		if err != nil {
			return 0, 400, err
		}
		if conn.ID == src.ConnID && (dir == src.Path || strings.HasPrefix(dir, src.Path+"/") || strings.HasPrefix(src.Path, dir+"/")) {
			return 0, 400, fmt.Errorf("the target must not overlap the source installation")
		}
		pv.Path, pv.Env, pv.Services = dir, truncateStr(strings.TrimSpace(pv.Env), 40), nil
		if pv.Env == "" {
			pv.Env = src.Env
		}
		if pv.Unit != nil {
			if err := pv.Unit.normalize(dir); err != nil {
				return 0, 400, err
			}
		}
		envs = append(envs, pv.Env)
		p.Targets = nil
		p.Items = []*gitRunItem{{ConnID: conn.ID, ConnName: conn.Name, App: src.App, Path: dir, Env: pv.Env, State: "pending", Log: []gitStepLog{},
			SourceConnName: src.ConnName, SourcePath: src.Path}}
	default:
		return 0, 400, fmt.Errorf("unknown kind")
	}
	prod := false
	for _, env := range envs {
		prod = prod || isProdInstall(gitInstall{Tags: connTags(conn.ID), Env: env})
	}
	if (prod || pv.Overwrite) && !strings.EqualFold(strings.TrimSpace(pv.Confirm), strings.TrimSpace(conn.Name)) {
		why := "production"
		if pv.Overwrite {
			why = "an overwrite"
		}
		return 0, 400, fmt.Errorf("%s on %s: type the server name %q to confirm", why, conn.Name, conn.Name)
	}
	for _, it := range p.Items {
		it.Prod = prod
	}
	gitLive.Lock()
	busy := gitLive.busy[userID] != 0
	gitLive.Unlock()
	if busy {
		return 0, 409, fmt.Errorf("another run is working: wait until it ends")
	}
	stored := p
	stored.Provision = redactedProvision(pv)
	pv.Confirm = ""
	now := time.Now()
	label := gitRunLabel(now)
	res, err := db.Exec(`INSERT INTO git_runs (user_id, kind, state, label, parent_id, params, result, message, created_at, scheduled_at, started_at, ended_at, notified_at)
		VALUES (?,?,'starting',?,0,?,'','',?,'','','','')`, userID, p.Kind, label, string(jsonMarshal(stored)), nowStamp())
	if err != nil {
		return 0, 500, err
	}
	id64, _ := res.LastInsertId()
	id := int(id64)
	pruneGitRuns(userID)
	if err := startGitRun(userID, id, r, &p); err != nil {
		db.Exec(`UPDATE git_runs SET state='failed', message=?, ended_at=? WHERE id=?`, err.Error(), nowStamp(), id)
		return 0, 409, err
	}
	return id, 200, nil
}

// ─── prepare: what the wizard shows before a run ─────

type gitRootInfo struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Writable bool   `json:"writable"`
	FreeKB   int64  `json:"free_kb"`
}

type gitPrepareSvc struct {
	App     string          `json:"app"`
	Ref     string          `json:"ref"`
	Path    string          `json:"path,omitempty"`
	Target  *gitTargetBrief `json:"target,omitempty"`
	Files   int             `json:"files"`
	Bytes   int64           `json:"bytes"`
	Check   *gitDirCheck    `json:"check,omitempty"`
	Slots   []gitSlot       `json:"slots"`
	Kind    string          `json:"kind"`
	Default string          `json:"default_path"`
	Error   string          `json:"error,omitempty"`
}

type gitPrepareReq struct {
	Kind     string `json:"kind"`
	ConnID   int    `json:"conn_id"`
	Services []struct {
		App  string `json:"app"`
		Ref  string `json:"ref"`
		Path string `json:"path"`
	} `json:"services"`
	SourceInstall int    `json:"source_install"`
	Path          string `json:"path"`
	Overwrite     bool   `json:"overwrite"`
}

// gitPrepare resolves the targets, reads the server roots and pre-checks the directories.
func gitPrepare(userID int, in gitPrepareReq) (map[string]interface{}, int, error) {
	key, ok := gitKindPolicy[in.Kind]
	if !ok || (in.Kind != "install" && in.Kind != "transfer") {
		return nil, 400, fmt.Errorf("unknown kind")
	}
	if !gitActionAllowed(userID, key) {
		return nil, 403, fmt.Errorf("this action is not allowed for your account")
	}
	conn, err := loadConnection(in.ConnID)
	if err != nil || conn.UserID != userID || !isSSHProtocol(conn) {
		return nil, 404, fmt.Errorf("choose an SSH connection of yours as the target")
	}
	cat := loadGitCatalog(userID)
	res := map[string]interface{}{"conn_id": conn.ID, "conn_name": conn.Name, "prod": isProdInstall(gitInstall{Tags: connTags(conn.ID)})}
	dirs := append([]string{}, cat.ServerRoots...)
	var svcs []*gitPrepareSvc
	var targets []gitTarget
	var srcFiles map[string]gitSourceFile
	var keep []string
	if in.Kind == "install" {
		if len(in.Services) == 0 || len(in.Services) > gitMaxInstallServices {
			return nil, 400, fmt.Errorf("choose 1 to %d services", gitMaxInstallServices)
		}
		providers := map[int]*gitProvider{}
		for _, x := range in.Services {
			ps := &gitPrepareSvc{App: x.App, Ref: x.Ref, Slots: []gitSlot{}}
			if len(cat.ServerRoots) > 0 {
				ps.Default = strings.TrimRight(cat.ServerRoots[0], "/") + "/" + x.App
			}
			t, err := installTarget(userID, x.App, x.Ref)
			targets = append(targets, t)
			svcs = append(svcs, ps)
			if err != nil {
				ps.Error = err.Error()
				continue
			}
			b := t.brief()
			ps.Target, ps.Kind = &b, t.Kind
			rels := installRels(t)
			ps.Files, ps.Bytes = len(rels), targetBytes(t, rels)
			ps.Slots = installSlots(userID, t, providers)
			if strings.TrimSpace(x.Path) != "" {
				dir, err := validInstallDir(x.Path)
				if err != nil {
					ps.Error = err.Error()
					continue
				}
				ps.Path = dir
				dirs = append(dirs, dir)
			}
		}
	} else {
		list := loadGitInstalls(userID, false, "i.id=?", in.SourceInstall)
		if len(list) == 0 || !userOwnsConnection(list[0].ConnID, userID) {
			return nil, 404, fmt.Errorf("installation not found")
		}
		src := list[0]
		sconn, err := loadConnection(src.ConnID)
		if err != nil {
			return nil, 404, fmt.Errorf("connection not found")
		}
		scl, err := dialSSH(sconn, nil)
		if err != nil {
			return nil, 502, fmt.Errorf("cannot log in to %s: %v", sconn.Name, err)
		}
		files, vlines, err := listForTransfer(scl, src.Path)
		scl.Close()
		if err != nil {
			return nil, 502, err
		}
		srcFiles = files
		var excluded map[string]string
		keep, excluded = transferSelection(files, cat.IgnoreDirs)
		var total int64
		for _, rel := range keep {
			total += files[rel].Size
		}
		ex := keysOf(excluded)
		sort.Strings(ex)
		exList := []map[string]string{}
		for _, rel := range ex {
			if len(exList) >= 200 {
				break
			}
			exList = append(exList, map[string]string{"path": rel, "why": excluded[rel]})
		}
		res["source"] = map[string]interface{}{"install_id": src.ID, "conn_name": src.ConnName, "path": src.Path, "app": src.App, "env": src.Env,
			"version": parseVersionMD(vlines).Version, "files": len(keep), "bytes": total, "excluded": len(excluded), "excluded_list": exList, "units": src.Units}
		if p := strings.TrimSpace(in.Path); p != "" {
			dir, err := validInstallDir(p)
			if err != nil {
				return nil, 400, err
			}
			in.Path = dir
			dirs = append(dirs, dir)
		}
	}
	cl, err := dialSSH(conn, nil)
	if err != nil {
		return nil, 502, fmt.Errorf("cannot log in to %s: %v", conn.Name, err)
	}
	defer cl.Close()
	checks, host, user, err := precheckDirs(cl, dirs)
	if err != nil {
		return nil, 502, err
	}
	res["host"], res["user"] = host, user
	roots := []gitRootInfo{}
	for _, r := range cat.ServerRoots {
		c := checks[r]
		roots = append(roots, gitRootInfo{Path: r, Exists: c.Exists, Writable: c.Writable && c.Exists, FreeKB: c.FreeKB})
	}
	res["roots"] = roots
	if in.Kind == "install" {
		for i, ps := range svcs {
			if ps.Path == "" || ps.Error != "" {
				continue
			}
			c := *checks[ps.Path]
			rels := installRels(targets[i])
			c.evaluate(ps.Bytes, rels, in.Overwrite)
			ps.Check = &c
		}
		res["services"] = svcs
	} else if in.Path != "" {
		c := *checks[in.Path]
		var total int64
		for _, rel := range keep {
			total += srcFiles[rel].Size
		}
		c.evaluate(total, keep, in.Overwrite)
		res["check"] = c
	}
	return res, 200, nil
}
