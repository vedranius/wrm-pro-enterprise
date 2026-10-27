// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

// ─── GIT DEPLOY: update, upgrade, rollback, stamp and restart over SSH ───
//
// Everything runs over WRM's SSH connections (jump hosts, vault credentials, proxies) with
// POSIX tools. For one installation and its selected files:
//
//  1. a fresh scan and comparison (a file changed by hand since the check stops the run);
//  2. a check for dangling Python imports;
//  3. a backup of the replaced files to <install>/.deploy-bak/<bundle or run id>-<time>/;
//  4. atomic writes: a temporary file in the same directory (copied from the old file, so
//     mode and owner stay), then a rename; line endings follow the existing file (or most
//     files of the installation);
//  5. checks (py_compile, node --check, sh -n, or a custom command);
//  6. an automatic rollback from the backup on any failure;
//  7. VERSION.md and a line in updates.jsonl, in the formats of the file-based deploy tool.
//
// Restarts of the systemd / supervisor units are a separate, opt-in step.

const (
	gitStepTimeout    = 2 * time.Minute
	gitChecksTimeout  = 5 * time.Minute
	gitMaxDeployFiles = 2000
	gitBackupDir      = ".deploy-bak"
)

var (
	// first writable wins; {install} is the installation directory (a variable for tests)
	gitUpdateLogPaths = []string{"/var/log/deploytool/updates.jsonl", "{install}/" + gitBackupDir + "/updates.jsonl", "/tmp/deploytool-updates.jsonl"}
	gitRestartSettle  = 2 * time.Second
	// how restarts get root: as root nothing, else sudo -n when it is allowed (a variable for tests)
	gitSudoPrelude = `if [ "$(id -u)" = 0 ]; then SU=; elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then SU="sudo -n"; else SU=; fi`
)

// deployOp is one change of a file: write content, copy from a server path, or delete.
type deployOp struct {
	Rel     string
	Content []byte
	From    string
	Delete  bool
	Existed bool
	Mode    string // new files
	tmp     string
}

// deploySession is the state of one installation during a run.
type deploySession struct {
	cl      *ssh.Client
	in      gitInstall
	host    string
	user    string
	owner   string
	tools   map[string]bool
	crlf    map[string]bool // existing files → has CR
	imports []pyImport
	backup  string   // backup directory (absolute) of this run
	created []string // directories created by the writes
}

func (s *deploySession) abs(rel string) string { return s.in.Path + "/" + rel }

func (s *deploySession) run(script, stdin string, timeout time.Duration) (string, error) {
	return runRemoteScript(s.cl, script, stdin, timeout)
}

// ─── file selection ──────────────────────────────────

// planFile is one file of a plan: its state against the target and the default choice.
type planFile struct {
	Path     string `json:"path"`
	State    string `json:"state"` // ok | old | changed | missing | modified | protected
	Behind   int    `json:"behind,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Date     string `json:"date,omitempty"`
	Selected bool   `json:"selected"`
	Local    bool   `json:"local,omitempty"` // changed by hand on the server
}

// planFiles classifies the files of an installation for an update (newT = the target it
// follows) or an upgrade (newT = another ref; cur = the target it follows now, to tell
// local changes from files of the old branch). Default selection: old + missing.
func planFiles(kind string, newT gitTarget, server map[string]string, cur *gitTarget, ignore []string) []planFile {
	fs, _, _ := compareInstall(newT, server, ignore)
	curState := map[string]string{}
	if kind == "upgrade" && cur != nil && len(cur.Files) > 0 {
		cs, _, _ := compareInstall(*cur, server, ignore)
		for _, f := range cs {
			curState[f.Path] = f.State
		}
	}
	out := []planFile{}
	for _, f := range fs {
		if f.State == "extra" || toolFiles[f.Path] {
			continue
		}
		p := planFile{Path: f.Path, State: f.State, Behind: f.Behind, Tag: f.Tag, Date: f.Date}
		switch f.State {
		case "old", "missing":
			p.Selected = true
		case "modified":
			if kind == "upgrade" && f.Note == "" {
				if st, known := curState[f.Path]; known && st != "modified" {
					p.State, p.Selected = "changed", true
					break
				}
			}
			p.Local = true
		}
		out = append(out, p)
	}
	return out
}

// selectFiles picks the files of a run: the default selection, or the files the user
// ticked (a file changed by hand only when it was ticked as such).
func selectRunFiles(plan []planFile, it *gitRunItem) ([]planFile, error) {
	var sel []planFile
	if len(it.Files) == 0 {
		for _, p := range plan {
			if p.Selected {
				sel = append(sel, p)
			}
		}
		return sel, nil
	}
	by := map[string]planFile{}
	for _, p := range plan {
		by[p.Path] = p
	}
	okMod := map[string]bool{}
	for _, m := range it.ModifiedOK {
		okMod[m] = true
	}
	for _, rel := range it.Files {
		p, found := by[rel]
		switch {
		case !found:
			return nil, fmt.Errorf("%s is not a file of the target", rel)
		case p.State == "ok":
			continue
		case p.State == "protected":
			return nil, fmt.Errorf("%s is protected (per-host) and is never overwritten", rel)
		case p.Local && !okMod[rel]:
			return nil, fmt.Errorf("%s was changed on the server since the check: check again and tick it explicitly", rel)
		}
		sel = append(sel, p)
	}
	return sel, nil
}

// ─── content ─────────────────────────────────────────

// gitFileContent returns a target file's content (blob cache, else the Git API).
func gitFileContent(userID int, t gitTarget, rel string, providers map[int]*gitProvider) ([]byte, error) {
	if loadGitRelayMeta(t.SourceID).OwnerID > 0 && !relaySourceAllowed(userID, t.SourceID) {
		return nil, fmt.Errorf("shared Git source access has been removed")
	}
	f, ok := t.Files[rel]
	if !ok || f.Blob == "" {
		return nil, fmt.Errorf("%s: the content is not available (refresh the target)", rel)
	}
	if data, have := blobContent(t.SourceID, f.Blob); have {
		return data, nil
	}
	src, found := loadGitSource(userID, t.SourceID)
	if !found || src.Kind == "bundle" {
		return nil, fmt.Errorf("%s: the content is not available (large file or a bundle without payload)", rel)
	}
	p := providers[src.ID]
	if p == nil {
		var err error
		if p, err = newGitProvider(src); err != nil {
			return nil, err
		}
		providers[src.ID] = p
	}
	data, err := p.blob(t.Project, f.Blob)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", rel, err)
	}
	storeBlob(src.ID, f.Blob, data)
	return data, nil
}

// withLineEndings converts text to LF or CRLF; binary content stays as it is.
func withLineEndings(data []byte, crlf bool) []byte {
	if bytes.IndexByte(data, 0) >= 0 {
		return data
	}
	lf := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if crlf {
		return bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
	}
	return lf
}

// mostlyCRLF tells whether most existing files of the installation use CRLF.
func (s *deploySession) mostlyCRLF() bool {
	n := 0
	for _, c := range s.crlf {
		if c {
			n++
		}
	}
	return len(s.crlf) > 0 && n*2 > len(s.crlf)
}

func (s *deploySession) crlfFor(rel string) bool {
	if c, ok := s.crlf[rel]; ok {
		return c
	}
	return s.mostlyCRLF()
}

// ─── server steps ────────────────────────────────────

// prepare reads the host name, the SSH user, the owner of the installation, the tools for
// checks, the line endings of the existing files and (for Python) the import lines.
func (s *deploySession) prepare(existing []string, wantImports bool) error {
	q := shellQuote(s.in.Path)
	var sb strings.Builder
	fmt.Fprintf(&sb, "[ -d %s ] || { echo 'E\tthe directory does not exist'; exit 0; }\n", q)
	sb.WriteString(`echo "H	$(hostname 2>/dev/null || uname -n)"` + "\n")
	sb.WriteString(`echo "U	$(id -un 2>/dev/null || whoami)"` + "\n")
	fmt.Fprintf(&sb, "echo \"O\t$(ls -ldn %s | awk '{print $3\":\"$4}')\"\n", q)
	sb.WriteString("for x in python3 node bash; do command -v $x >/dev/null 2>&1 && echo \"T\t$x\"; done\n")
	fmt.Fprintf(&sb, "cd %s || exit 3\n", q)
	sb.WriteString("while IFS= read -r f; do [ -f \"$f\" ] || continue; n=$(tr -cd '\\r' < \"$f\" | wc -c); if [ \"$n\" -gt 0 ]; then echo \"C\t1\t$f\"; else echo \"C\t0\t$f\"; fi; done\n")
	if wantImports {
		fmt.Fprintf(&sb, "find . %s -o -type f -name '*.py' -print 2>/dev/null | head -n %d | while IFS= read -r f; do\n", findPrune(), gitMaxInstallFiles)
		sb.WriteString(`awk -v F="${f#./}" 'c { b = b " " $0; if (index($0, ")")) { print "I\t" F "\t" b; c = 0 }; next }
/^[ \t]*from[ \t]+[.A-Za-z0-9_]+[ \t]+import[ \t(]/ { if (index($0, "(") && !index($0, ")")) { c = 1; b = $0; next }; print "I\t" F "\t" $0 }' "$f"` + "\n")
		sb.WriteString("done\n")
	}
	sb.WriteString("echo WRM_DONE\n")
	out, err := s.run(sb.String(), strings.Join(existing, "\n")+"\n", gitStepTimeout)
	if err != nil {
		return err
	}
	s.tools, s.crlf = map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "E":
			return fmt.Errorf("%s", parts[1])
		case "H":
			s.host = strings.TrimSpace(parts[1])
		case "U":
			s.user = strings.TrimSpace(parts[1])
		case "O":
			s.owner = strings.TrimSpace(parts[1])
		case "T":
			s.tools[parts[1]] = true
		case "C":
			if len(parts) == 3 {
				s.crlf[parts[2]] = parts[1] == "1"
			}
		case "I":
			if len(parts) == 3 {
				s.imports = append(s.imports, pyImport{File: parts[1], Line: parts[2]})
			}
		}
	}
	if !strings.Contains(out, "WRM_DONE") {
		return fmt.Errorf("the server did not answer completely")
	}
	if s.host == "" {
		s.host = s.in.ConnName
	}
	return nil
}

// makeBackup copies the files that exist (and VERSION.md) to a new backup directory
// <bundle>-<time>; the directory is claimed with mkdir, a clash moves to the next second.
func (s *deploySession) makeBackup(bundle string, now time.Time, rels []string, withVersion bool) error {
	all := append([]string(nil), rels...)
	if withVersion {
		all = append(all, "VERSION.md")
	}
	for try := 0; try < 10; try++ {
		s.backup = s.in.Path + "/" + gitBackupDir + "/" + gitBackupName(bundle, now.Add(time.Duration(try)*time.Second))
		var sb strings.Builder
		fmt.Fprintf(&sb, "B=%s\nmkdir -p \"${B%%/*}\" || exit 3\nmkdir \"$B\" 2>/dev/null || { echo WRM_EXISTS; exit 0; }\ncd %s || exit 3\n", shellQuote(s.backup), shellQuote(s.in.Path))
		for _, rel := range all {
			q := shellQuote(rel)
			fmt.Fprintf(&sb, "if [ -f %s ]; then mkdir -p \"$B\"/%s && cp -p %s \"$B\"/%s || { %s; exit 3; }; fi\n", q, shellQuote(path.Dir(rel)), q, q, shOut("F", rel))
		}
		sb.WriteString("echo WRM_OK\n")
		out, err := s.run(sb.String(), "", gitStepTimeout)
		if err == nil && strings.Contains(out, "WRM_EXISTS") {
			continue
		}
		if err != nil || !strings.Contains(out, "WRM_OK") {
			s.run("rm -rf "+shellQuote(s.backup), "", time.Minute)
			if err == nil {
				err = fmt.Errorf("the backup failed")
			}
			return fmt.Errorf("backup: %v", err)
		}
		return nil
	}
	return fmt.Errorf("backup: no free backup directory name")
}

// stage writes the new contents to temporary files next to the real ones (nothing live
// changes yet) and verifies them.
func (s *deploySession) stage(ops []*deployOp) error {
	for i, op := range ops {
		if op.Delete {
			continue
		}
		f := s.abs(op.Rel)
		op.tmp = path.Dir(f) + "/." + path.Base(f) + ".wrm-tmp"
		var sb strings.Builder
		sb.WriteString(gitHashPrelude)
		fmt.Fprintf(&sb, "f=%s; t=%s; d=%s\n", shellQuote(f), shellQuote(op.tmp), shellQuote(path.Dir(f)))
		sb.WriteString("if [ ! -d \"$d\" ]; then m=$d; while [ ! -d \"${m%/*}\" ]; do m=${m%/*}; done; mkdir -p \"$d\" || exit 3; echo \"M\t$m\"; fi\n")
		if op.From != "" {
			fmt.Fprintf(&sb, "cp -p %s \"$t\" 2>/dev/null || cp %s \"$t\" || exit 3\n", shellQuote(op.From), shellQuote(op.From))
		} else {
			sb.WriteString("if [ -f \"$f\" ]; then cp -p \"$f\" \"$t\" 2>/dev/null || cp \"$f\" \"$t\" || exit 3; fi\n")
			sb.WriteString("cat > \"$t\" || exit 3\n")
			if !op.Existed {
				fmt.Fprintf(&sb, "chmod %s \"$t\"\n", op.Mode)
				if s.owner != "" {
					fmt.Fprintf(&sb, "chown %s \"$t\" 2>/dev/null\n", shellQuote(s.owner))
				}
			}
		}
		sb.WriteString("echo \"S\t$(tr -d '\\r' < \"$t\" | $S | cut -c1-64)\"\n")
		out, err := s.run(sb.String(), string(op.Content), gitStepTimeout)
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "M\t") {
				s.created = append(s.created, line[2:])
			}
		}
		if err == nil && strings.HasPrefix(strings.TrimSpace(out), "E\t") {
			err = fmt.Errorf("%s", strings.TrimPrefix(strings.TrimSpace(out), "E\t"))
		}
		if err != nil {
			s.cleanTmp(ops[:i+1])
			return fmt.Errorf("%s: %v", op.Rel, err)
		}
		if op.From == "" {
			got := ""
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "S\t") {
					got = line[2:]
				}
			}
			if got != trHash(op.Content) {
				s.cleanTmp(ops[:i+1])
				return fmt.Errorf("%s: the written file does not match (transfer error)", op.Rel)
			}
		}
	}
	return nil
}

func (s *deploySession) cleanTmp(ops []*deployOp) {
	var sb strings.Builder
	for _, op := range ops {
		if op.tmp != "" {
			fmt.Fprintf(&sb, "rm -f %s\n", shellQuote(op.tmp))
		}
	}
	if sb.Len() > 0 {
		s.run(sb.String(), "", time.Minute)
	}
}

// commit renames the temporary files over the real ones and deletes files to delete.
func (s *deploySession) commit(ops []*deployOp) error {
	var sb strings.Builder
	for _, op := range ops {
		f := shellQuote(s.abs(op.Rel))
		if op.Delete {
			fmt.Fprintf(&sb, "rm -f %s || { %s; exit 3; }\n", f, shOut("F", op.Rel))
			// directories left empty go too (up to the installation)
			for d := path.Dir(op.Rel); d != "." && d != "/"; d = path.Dir(d) {
				fmt.Fprintf(&sb, "rmdir %s 2>/dev/null &&\n", shellQuote(s.abs(d)))
			}
			sb.WriteString(":\n")
		} else {
			fmt.Fprintf(&sb, "mv -f %s %s || { %s; exit 3; }\n", shellQuote(op.tmp), f, shOut("F", op.Rel))
		}
	}
	sb.WriteString("echo WRM_OK\n")
	out, err := s.run(sb.String(), "", gitStepTimeout)
	if err == nil && !strings.Contains(out, "WRM_OK") {
		err = fmt.Errorf("the rename did not finish")
	}
	return err
}

// restore puts the backed-up files back (atomically), deletes files that did not exist
// before and removes directories the run created. With drop the backup is removed after a
// complete restore.
func (s *deploySession) restore(ops []*deployOp, drop bool) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "B=%s; bad=0\n", shellQuote(s.backup))
	for _, op := range ops {
		f := s.abs(op.Rel)
		if op.tmp != "" {
			fmt.Fprintf(&sb, "rm -f %s\n", shellQuote(op.tmp))
		}
		if op.Existed {
			t := path.Dir(f) + "/." + path.Base(f) + ".wrm-back"
			fmt.Fprintf(&sb, "{ mkdir -p %s && cp -p \"$B\"/%s %s && mv -f %s %s; } || { %s; bad=1; }\n", shellQuote(path.Dir(f)), shellQuote(op.Rel), shellQuote(t), shellQuote(t), shellQuote(f), shOut("F", op.Rel))
		} else {
			fmt.Fprintf(&sb, "rm -f %s\n", shellQuote(f))
		}
	}
	for i := len(s.created) - 1; i >= 0; i-- {
		fmt.Fprintf(&sb, "find %s -depth -type d -exec rmdir {} \\; 2>/dev/null\n", shellQuote(s.created[i]))
	}
	if drop {
		sb.WriteString("[ $bad = 0 ] && rm -rf \"$B\"\n")
	}
	sb.WriteString("echo WRM_OK\n")
	out, err := s.run(sb.String(), "", gitStepTimeout)
	if err != nil {
		return err
	}
	var failed []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "F\t") {
			failed = append(failed, line[2:])
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("could not restore %s (the backup stays in %s)", strings.Join(failed, ", "), s.backup)
	}
	return nil
}

// checks runs the language checks of the written files and the custom command.
func (s *deploySession) checks(rels []string, custom string) ([]string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "cd %s || exit 3\n", shellQuote(s.in.Path))
	sb.WriteString("one() { printf '%s' \"$1\" | tr '\\n\\t\\r' '   ' | cut -c1-400; }\n")
	// py_compile writes its .pyc files to a temporary tree, not into the installation
	sb.WriteString("PC=$(mktemp -d 2>/dev/null || echo /tmp/wrm-pyc.$$)\n")
	n := 0
	for _, rel := range rels {
		q := shellQuote(rel)
		var cmd string
		switch strings.ToLower(path.Ext(rel)) {
		case ".py":
			if !s.tools["python3"] {
				continue
			}
			cmd = "PYTHONPYCACHEPREFIX=\"$PC\" python3 -m py_compile " + q
		case ".js", ".mjs", ".cjs":
			if !s.tools["node"] {
				continue
			}
			cmd = "node --check " + q
		case ".sh":
			cmd = "if head -n 1 " + q + " | grep -q bash && command -v bash >/dev/null 2>&1; then bash -n " + q + "; else sh -n " + q + "; fi"
		default:
			continue
		}
		n++
		fmt.Fprintf(&sb, "if o=$( { %s; } 2>&1 ); then printf 'K\\tok\\t%%s\\n' %s; else printf 'K\\tfail\\t%%s\\t%%s\\n' %s \"$(one \"$o\")\"; fi\n", cmd, q, q)
	}
	if strings.TrimSpace(custom) != "" {
		n++
		fmt.Fprintf(&sb, "if o=$(sh -c %s 2>&1); then echo 'K\tok\t(custom)'; else echo \"K\tfail\t(custom)\t$(one \"$o\")\"; fi\n", shellQuote(custom))
	}
	if n == 0 {
		return nil, nil
	}
	sb.WriteString("rm -rf \"$PC\"\necho WRM_DONE\n")
	out, err := s.run(sb.String(), "", gitChecksTimeout)
	if err != nil {
		return nil, err
	}
	var passed, failed []string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 3 || parts[0] != "K" {
			continue
		}
		if parts[1] == "ok" {
			passed = append(passed, parts[2])
		} else {
			msg := parts[2]
			if len(parts) == 4 && strings.TrimSpace(parts[3]) != "" {
				msg += ": " + strings.TrimSpace(parts[3])
			}
			failed = append(failed, msg)
		}
	}
	if len(failed) > 0 {
		return passed, fmt.Errorf("%s", strings.Join(failed, "; "))
	}
	if !strings.Contains(out, "WRM_DONE") {
		return passed, fmt.Errorf("the checks did not finish")
	}
	return passed, nil
}

// appendUpdateLog appends a line to the first writable updates.jsonl and returns its path.
func (s *deploySession) appendUpdateLog(line string) (string, error) {
	var paths []string
	for _, p := range gitUpdateLogPaths {
		paths = append(paths, shellQuote(strings.ReplaceAll(p, "{install}", s.in.Path)))
	}
	script := "L=" + shellQuote(line) + "\nfor f in " + strings.Join(paths, " ") + `; do
d=${f%/*}
if { [ -f "$f" ] && [ -w "$f" ]; } || { [ ! -e "$f" ] && [ -d "$d" ] && [ -w "$d" ]; }; then
if printf '%s\n' "$L" >> "$f" 2>/dev/null; then echo "J	$f"; exit 0; fi
fi
done
echo "J	"
`
	out, err := s.run(script, "", time.Minute)
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "J\t") {
			if l[2:] == "" {
				return "", fmt.Errorf("no writable updates.jsonl location")
			}
			return l[2:], nil
		}
	}
	return "", fmt.Errorf("no answer")
}

// ─── formats of the file-based deploy tool ───────────

// gitBackupName is <bundle id or run id>-<YYYYMMDD-HHMMSS>.
func gitBackupName(bundle string, now time.Time) string {
	return bundle + "-" + now.Format("20060102-150405")
}

// gitRunLabel is the run id used where a bundle id would be: wrm-<YYYYMMDD-HHMM>.
func gitRunLabel(now time.Time) string { return "wrm-" + now.Format("20060102-1504") }

// bundleOf returns the bundle id of a target from a bundle, else the run id.
func bundleOf(t gitTarget, runLabel string) string {
	if strings.HasPrefix(t.Source, "bundle:") {
		if b := strings.TrimPrefix(t.Source, "bundle:"); b != "" {
			return b
		}
	}
	return runLabel
}

// pyJSONString quotes like Python's json.dumps (ensure_ascii).
func pyJSONString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			switch {
			case r < 0x20 || (r > 0x7e && r < 0x80):
				fmt.Fprintf(&b, `\u%04x`, r)
			case r >= 0x80 && r <= 0xffff:
				fmt.Fprintf(&b, `\u%04x`, r)
			case r > 0xffff:
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// pyJSONLine is one line of updates.jsonl: json.dumps(record, sort_keys=True).
func pyJSONLine(rec map[string]interface{}) string {
	keys := make([]string, 0, len(rec))
	for k := range rec {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		var v string
		switch x := rec[k].(type) {
		case []string:
			items := make([]string, len(x))
			for i, s := range x {
				items[i] = pyJSONString(s)
			}
			v = "[" + strings.Join(items, ", ") + "]"
		case string:
			v = pyJSONString(x)
		default:
			v = pyJSONString(fmt.Sprint(x))
		}
		parts = append(parts, pyJSONString(k)+": "+v)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func serviceNames(units []gitUnit) []string {
	out := []string{}
	for _, u := range units {
		if u.Kind == "systemd" {
			out = append(out, "systemctl "+strings.TrimSuffix(u.Name, ".service"))
		} else {
			out = append(out, "supervisorctl "+u.Name)
		}
	}
	return out
}

// versionRow is one line of the VERSION.md table.
type versionRow struct{ Path, Version, Date string }

// versionMD builds VERSION.md like the deploy tool (Croatian labels on purpose).
func versionMD(t gitTarget, rows []versionRow, updated time.Time, host, user, bundle string, crlf bool) []byte {
	return versionMDNote(t, rows, updated, host, user, bundle, "", crlf)
}

// versionMDNote is versionMD with an optional "Napomena" field (a partial stamp).
func versionMDNote(t gitTarget, rows []versionRow, updated time.Time, host, user, bundle, note string, crlf bool) []byte {
	ver := t.Version
	if ver == "" {
		ver = short10(t.Commit)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
	var b strings.Builder
	fmt.Fprintf(&b, "# %s - %s\n\n", t.App, ver)
	field := func(label, val string) {
		head := "- **" + label + "**:"
		fmt.Fprintf(&b, "%s%s%s\n", head, strings.Repeat(" ", max(1, 19-utf8.RuneCountInString(head))), val)
	}
	field("Servis", t.App)
	field("Verzija", ver)
	field("Grana/ref", fmt.Sprintf("%s (%s)", t.Branch, t.RefKind))
	field("Commit", fmt.Sprintf("%s (%s)", short10(t.Commit), t.CommitDate))
	field("Azurirano", updated.Format("2006-01-02 15:04:05"))
	field("Host", host)
	field("Korisnik", user)
	field("Bundle", bundle)
	if note != "" {
		field("Napomena", note)
	}
	fmt.Fprintf(&b, "\n## Services (%d)\n\n", len(rows))
	w := [3]int{utf8.RuneCountInString("Datoteka"), utf8.RuneCountInString("Verzija"), utf8.RuneCountInString("Datum")}
	for _, r := range rows {
		for i, v := range []string{r.Path, r.Version, r.Date} {
			if n := utf8.RuneCountInString(v); n > w[i] {
				w[i] = n
			}
		}
	}
	pad := func(s string, n int) string { return s + strings.Repeat(" ", n-utf8.RuneCountInString(s)) }
	line := func(a, c, d string) {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", pad(a, w[0]), pad(c, w[1]), pad(d, w[2]))
	}
	line("Datoteka", "Verzija", "Datum")
	line(strings.Repeat("-", w[0]), strings.Repeat("-", w[1]), strings.Repeat("-", w[2]))
	for _, r := range rows {
		line(r.Path, r.Version, r.Date)
	}
	return withLineEndings([]byte(b.String()), crlf)
}

// versionRows describes every target file after a run: written or current files carry the
// target's version, older files the version they match, protected files "(per-host)".
func versionRows(t gitTarget, plan []planFile, written map[string]bool) []versionRow {
	state := map[string]planFile{}
	for _, p := range plan {
		state[p.Path] = p
	}
	label := func(e gitHistEntry) (string, string) {
		v := e.Tag
		if v == "" {
			v = e.Commit
		}
		d := e.Date
		if d == "" {
			d = "-"
		}
		return v, d
	}
	var rows []versionRow
	for rel, f := range t.Files {
		if toolFiles[rel] {
			continue
		}
		p, known := state[rel]
		switch {
		case globHit(rel, t.Protected) || (known && p.State == "protected"):
			rows = append(rows, versionRow{rel, "(per-host)", "-"})
		case written[rel] || !known || p.State == "ok":
			if len(f.History) == 0 {
				rows = append(rows, versionRow{rel, short10(t.Commit), t.CommitDate})
				continue
			}
			v, d := label(f.History[0])
			rows = append(rows, versionRow{rel, v, d})
		case p.State == "old":
			if e, ok := historyAtVersion(f, p.Behind); ok {
				v, d := label(e)
				rows = append(rows, versionRow{rel, v, d})
			}
		case p.State == "missing":
		default:
			rows = append(rows, versionRow{rel, "(local)", "-"})
		}
	}
	return rows
}

// ─── dangling Python imports ─────────────────────────

type pyImport struct{ File, Line string }

var (
	pyFromRe   = regexp.MustCompile(`^\s*from\s+([.\w]+)\s+import\s+(.*)$`)
	pyDefRe    = regexp.MustCompile(`^\s*(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)`)
	pyAssignRe = regexp.MustCompile(`^\s*([A-Za-z_][\w\s,()\[\]*]*?)\s*(?::[^=]*)?=[^=]`)
	pyAnnRe    = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*:\s*[^=]+$`)
	pyImportRe = regexp.MustCompile(`^\s*import\s+(.+)$`)
	pyNameRe   = regexp.MustCompile(`^[A-Za-z_]\w*$`)
	pyBlockRe  = regexp.MustCompile(`^(if|elif|else|try|except|finally|with|for|while)\b`)
)

// pyImportNames returns the names of "from mod import a, b as c" ("*" for a star import).
func pyImportNames(list string) []string {
	if i := strings.Index(list, "#"); i >= 0 {
		list = list[:i]
	}
	list = strings.NewReplacer("(", " ", ")", " ", "\\", " ").Replace(list)
	var out []string
	for _, part := range strings.Split(list, ",") {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		out = append(out, f[0])
	}
	return out
}

// pyModuleNames scans a module's top level for the names it defines; ok is false when it
// cannot tell (a star import or a module __getattr__).
func pyModuleNames(src []byte) (map[string]bool, bool) {
	names := map[string]bool{}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	inBlock := false // inside a top-level if / try / with / for (definitions there count)
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		trim := strings.TrimSpace(l)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indented := l[0] == ' ' || l[0] == '\t'
		if !indented {
			inBlock = pyBlockRe.MatchString(trim)
		} else if !inBlock {
			continue
		}
		// join continuation lines of an import in parentheses
		for strings.Contains(trim, "(") && !strings.Contains(trim, ")") && strings.HasPrefix(trim, "from ") && i+1 < len(lines) {
			i++
			trim += " " + strings.TrimSpace(lines[i])
		}
		if m := pyFromRe.FindStringSubmatch(trim); m != nil {
			for _, n := range pyImportNamesAs(m[2]) {
				if n == "*" {
					return names, false
				}
				names[n] = true
			}
			continue
		}
		if m := pyImportRe.FindStringSubmatch(trim); m != nil {
			for _, part := range strings.Split(m[1], ",") {
				f := strings.Fields(part)
				if len(f) >= 3 && f[1] == "as" {
					names[f[2]] = true
				} else if len(f) >= 1 {
					names[strings.Split(f[0], ".")[0]] = true
				}
			}
			continue
		}
		if m := pyDefRe.FindStringSubmatch(trim); m != nil {
			if m[1] == "__getattr__" && !indented {
				return names, false
			}
			names[m[1]] = true
			continue
		}
		if m := pyAssignRe.FindStringSubmatch(trim); m != nil {
			for _, n := range strings.FieldsFunc(m[1], func(r rune) bool { return strings.ContainsRune(" ,()[]*", r) }) {
				if pyNameRe.MatchString(n) {
					names[n] = true
				}
			}
			continue
		}
		if m := pyAnnRe.FindStringSubmatch(trim); m != nil {
			names[m[1]] = true
		}
	}
	return names, true
}

// pyImportNamesAs is pyImportNames for the importing side: the bound name (alias).
func pyImportNamesAs(list string) []string {
	if i := strings.Index(list, "#"); i >= 0 {
		list = list[:i]
	}
	list = strings.NewReplacer("(", " ", ")", " ", "\\", " ").Replace(list)
	var out []string
	for _, part := range strings.Split(list, ",") {
		f := strings.Fields(part)
		switch {
		case len(f) >= 3 && f[1] == "as":
			out = append(out, f[2])
		case len(f) >= 1:
			out = append(out, f[0])
		}
	}
	return out
}

// pyDotted turns lib/util.py into lib.util (a package's __init__.py into the package).
func pyDotted(rel string) string {
	d := strings.ReplaceAll(strings.TrimSuffix(rel, ".py"), "/", ".")
	if d == "__init__" {
		return ""
	}
	return strings.TrimSuffix(d, ".__init__")
}

// pyResolve gives the dotted module of an import in file (relative imports resolved).
func pyResolve(file, mod string) (string, bool) {
	if !strings.HasPrefix(mod, ".") {
		return mod, false
	}
	level := len(mod) - len(strings.TrimLeft(mod, "."))
	rest := mod[level:]
	base := strings.Split(path.Dir(file), "/")
	if path.Dir(file) == "." {
		base = nil
	}
	if level-1 > len(base) {
		return "", true
	}
	base = base[:len(base)-(level-1)]
	if rest != "" {
		base = append(base, rest)
	}
	return strings.Join(base, "."), true
}

// danglingImports finds "from mod import name" in other files whose mod is one of the new
// modules and whose name the new module no longer defines. contents are the new modules
// (rel → content); imports are the import lines of the installation (files that are
// rewritten contribute their new lines); files are all files of the installation.
func danglingImports(contents map[string][]byte, imports []pyImport, files map[string]bool) []string {
	type mod struct {
		rel   string
		names map[string]bool
	}
	mods := map[string]mod{}
	for rel, c := range contents {
		if !strings.HasSuffix(rel, ".py") {
			continue
		}
		names, ok := pyModuleNames(c)
		if !ok {
			continue
		}
		if d := pyDotted(rel); d != "" {
			mods[d] = mod{rel, names}
		}
	}
	if len(mods) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, im := range imports {
		m := pyFromRe.FindStringSubmatch(strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(im.Line)))
		if m == nil {
			continue
		}
		full, relative := pyResolve(im.File, m[1])
		for d, md := range mods {
			if md.rel == im.File {
				continue
			}
			if !(full == d || (!relative && strings.HasSuffix(d, "."+full))) {
				continue
			}
			for _, n := range pyImportNames(m[2]) {
				if n == "*" || md.names[n] {
					continue
				}
				// a submodule of a package: from pkg import sub
				if strings.HasSuffix(md.rel, "__init__.py") {
					dir := path.Dir(md.rel)
					if files[dir+"/"+n+".py"] || files[dir+"/"+n+"/__init__.py"] {
						continue
					}
				}
				msg := fmt.Sprintf("%s: from %s import %s (%s no longer defines %s)", im.File, m[1], n, md.rel, n)
				if !seen[msg] {
					seen[msg] = true
					out = append(out, msg)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// pyImportsOf extracts the "from … import" statements of a file's content.
func pyImportsOf(rel string, content []byte) []pyImport {
	var out []pyImport
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if !pyFromRe.MatchString(l) {
			continue
		}
		for strings.Contains(l, "(") && !strings.Contains(l, ")") && i+1 < len(lines) {
			i++
			l += " " + lines[i]
		}
		out = append(out, pyImport{File: rel, Line: l})
	}
	return out
}

// ─── backups on the server ───────────────────────────

type gitBackup struct {
	Name     string   `json:"name"`
	Time     string   `json:"time"` // YYYY-MM-DD HH:MM:SS from the name
	Files    []string `json:"files"`
	Count    int      `json:"count"`
	Version  string   `json:"version,omitempty"` // VERSION.md in the backup
	Commit   string   `json:"commit,omitempty"`
	Branch   string   `json:"branch,omitempty"`
	LogFiles []string `json:"log_files,omitempty"` // "files" of the updates.jsonl line
	FromVer  string   `json:"from_version,omitempty"`
	ToVer    string   `json:"to_version,omitempty"`
	User     string   `json:"user,omitempty"`
}

var backupTimeRe = regexp.MustCompile(`(\d{8})-(\d{6})$`)

// listBackups reads the backups of an installation and the matching updates.jsonl lines.
func listBackups(cl *ssh.Client, dir string) ([]gitBackup, error) {
	b := dir + "/" + gitBackupDir
	var paths []string
	for _, p := range gitUpdateLogPaths {
		paths = append(paths, shellQuote(strings.ReplaceAll(p, "{install}", dir)))
	}
	script := fmt.Sprintf(`B=%s
[ -d "$B" ] || { echo WRM_DONE; exit 0; }
for d in "$B"/*/; do
  [ -d "$d" ] || continue
  n=${d%%/}; n=${n##*/}
  echo "K	$n"
  (cd "$d" && find . -type f 2>/dev/null | head -n 500 | sed "s|^\./|F	$n	|")
  [ -f "$d/VERSION.md" ] && head -n 12 "$d/VERSION.md" | sed "s|^|V	$n	|"
done
for f in %s; do [ -r "$f" ] && grep -F -- %s "$f" 2>/dev/null | tail -n 500 | sed 's/^/L	/'; done
echo WRM_DONE
`, shellQuote(b), strings.Join(paths, " "), shellQuote(`"install": `+pyJSONString(dir)))
	out, err := runRemoteScript(cl, script, "", gitStepTimeout)
	if err != nil {
		return nil, err
	}
	by := map[string]*gitBackup{}
	var order []string
	vlines := map[string][]string{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		switch {
		case len(parts) == 2 && parts[0] == "K":
			if parts[1] == "backups" || parts[1] == "deploys" {
				continue // environments: rollback backups and deploy manifests
			}
			if by[parts[1]] == nil {
				by[parts[1]] = &gitBackup{Name: parts[1], Files: []string{}}
				order = append(order, parts[1])
			}
		case len(parts) == 3 && parts[0] == "F":
			if x := by[parts[1]]; x != nil {
				x.Count++
				if parts[2] != "VERSION.md" {
					x.Files = append(x.Files, parts[2])
				}
			}
		case len(parts) == 3 && parts[0] == "V":
			vlines[parts[1]] = append(vlines[parts[1]], parts[2])
		case len(parts) >= 2 && parts[0] == "L":
			rec := map[string]interface{}{}
			jsonUnmarshalString(strings.TrimPrefix(line, "L\t"), &rec)
			bk, _ := rec["backup"].(string)
			x := by[path.Base(bk)]
			if x == nil || path.Dir(bk) != b {
				continue
			}
			x.LogFiles = []string{}
			if fl, ok := rec["files"].([]interface{}); ok {
				for _, f := range fl {
					if s, ok := f.(string); ok {
						x.LogFiles = append(x.LogFiles, s)
					}
				}
			}
			x.FromVer, _ = rec["from_version"].(string)
			x.ToVer, _ = rec["to_version"].(string)
			x.User, _ = rec["user"].(string)
		}
	}
	if !strings.Contains(out, "WRM_DONE") {
		return nil, fmt.Errorf("the server did not answer completely")
	}
	list := []gitBackup{}
	for _, n := range order {
		x := by[n]
		if m := backupTimeRe.FindStringSubmatch(n); m != nil {
			if t, err := time.Parse("20060102150405", m[1]+m[2]); err == nil {
				x.Time = t.Format("2006-01-02 15:04:05")
			}
		}
		if v := vlines[n]; len(v) > 0 {
			info := parseVersionMD(v)
			x.Version, x.Commit = info.Version, info.Commit
			x.Branch = strings.TrimSpace(strings.Split(info.Ref, " (")[0])
		}
		sort.Strings(x.Files)
		list = append(list, *x)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Time > list[j].Time })
	if len(list) > 50 {
		list = list[:50]
	}
	return list, nil
}

// ─── restart and health ──────────────────────────────

type gitHealth struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Status string `json:"status"`
}

// restartUnits restarts the units (sudo -n when not root and allowed) and checks their
// health afterwards (systemctl is-active / supervisorctl status).
func restartUnits(cl *ssh.Client, units []gitUnit) ([]gitHealth, error) {
	var sb strings.Builder
	sb.WriteString(gitSudoPrelude + `
one() { printf '%s' "$1" | tr '\n\t\r' '   ' | cut -c1-300; }
`)
	for _, u := range units {
		q := shellQuote(u.Name)
		if u.Kind == "systemd" {
			fmt.Fprintf(&sb, "if o=$($SU systemctl restart %s 2>&1); then printf 'R\\tok\\t%%s\\n' %s; else printf 'R\\tfail\\t%%s\\t%%s\\n' %s \"$(one \"$o\")\"; fi\n", q, q, q)
		} else {
			fmt.Fprintf(&sb, "o=$($SU supervisorctl restart %s 2>&1); if [ $? = 0 ] && ! printf '%%s' \"$o\" | grep -qi error; then printf 'R\\tok\\t%%s\\n' %s; else printf 'R\\tfail\\t%%s\\t%%s\\n' %s \"$(one \"$o\")\"; fi\n", q, q, q)
		}
	}
	if s := int(gitRestartSettle / time.Second); s > 0 {
		fmt.Fprintf(&sb, "sleep %d\n", s)
	}
	for _, u := range units {
		q := shellQuote(u.Name)
		if u.Kind == "systemd" {
			fmt.Fprintf(&sb, "printf 'A\\t%%s\\t%%s\\n' %s \"$(one \"$(systemctl is-active %s 2>&1)\")\"\n", q, q)
		} else {
			fmt.Fprintf(&sb, "printf 'A\\t%%s\\t%%s\\n' %s \"$(one \"$($SU supervisorctl status %s 2>&1)\")\"\n", q, q)
		}
	}
	sb.WriteString("echo WRM_DONE\n")
	out, err := runRemoteScript(cl, sb.String(), "", gitStepTimeout)
	if err != nil {
		return nil, err
	}
	restartErr := map[string]string{}
	health := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		switch {
		case len(parts) >= 3 && parts[0] == "R" && parts[1] == "fail":
			msg := "restart failed"
			if len(parts) == 4 && strings.TrimSpace(parts[3]) != "" {
				msg = strings.TrimSpace(parts[3])
			}
			restartErr[parts[2]] = msg
		case len(parts) >= 2 && parts[0] == "A":
			st := ""
			if len(parts) >= 3 {
				st = strings.TrimSpace(strings.Join(parts[2:], " "))
			}
			health[parts[1]] = st
		}
	}
	var res []gitHealth
	var bad []string
	for _, u := range units {
		h := gitHealth{Kind: u.Kind, Name: u.Name, Status: health[u.Name]}
		if u.Kind == "systemd" {
			h.OK = h.Status == "active"
		} else {
			h.OK = strings.Contains(h.Status, "RUNNING")
		}
		if e, failed := restartErr[u.Name]; failed {
			h.OK = false
			h.Status = e
		}
		if !h.OK {
			bad = append(bad, u.Name+": "+h.Status)
		}
		res = append(res, h)
	}
	if len(bad) > 0 {
		return res, fmt.Errorf("unhealthy after the restart: %s", strings.Join(bad, "; "))
	}
	return res, nil
}

// shOut prints "<tag>\t<value>" without the shell expanding the value.
func shOut(tag, val string) string {
	return fmt.Sprintf("printf '%s\\t%%s\\n' %s", tag, shellQuote(val))
}

func unitNames(units []gitUnit) string {
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name
	}
	return strings.Join(names, ", ")
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return d
}
