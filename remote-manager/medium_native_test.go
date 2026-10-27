// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ─── deploy-config fixtures (invented names) ───

func mdWriteConfig(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const mdServersYAML = `servers:
  app-01:
    host: app-01.example.com
    user: deploy
  app-02:
    host: 192.0.2.12
    user: deploy
    port: 2222
    key: id_demo
`

const mdEnvsYAML = `environments:
  dev: {}
  prod:
    confirm: true
    allowed_refs: [refs/heads/main, "refs/tags/v*"]
`

func mdValidConfig(t *testing.T, extra map[string]string) string {
	files := map[string]string{
		"servers.yaml":      mdServersYAML,
		"environments.yaml": mdEnvsYAML,
		"projects/demo-api.yaml": `name: demo-api
repo: git@git.example.com:team/demo-api.git
ignore: [".env", "storage/", "*.log"]
targets:
  dev: app-01:/srv/www/demo-api
  prod:
    destinations:
      - app-01:/srv/www/demo-api-prod
      - {server: app-02, path: /srv/www/demo-api, ignore_extra: [uploads/], post_deploy: null}
    ignore_extra: [/config/local.php]
    post_deploy: sudo systemctl reload php-fpm
    allowed_refs: null
`,
		"projects/demo-web.yaml": `name: demo-web
repo: https://git.example.com/team/demo-web.git
source_dir: dist
build: npm ci && npm run build
targets:
  dev:
    - app-02:/srv/www/demo-web
`,
	}
	for k, v := range extra {
		if v == "" {
			delete(files, k)
		} else {
			files[k] = v
		}
	}
	return mdWriteConfig(t, files)
}

func TestMediumConfigValid(t *testing.T) {
	cfg, err := mdLoadConfig(mdValidConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 2 || cfg.server("app-02").Port != 2222 || cfg.server("app-01").Port != 22 {
		t.Fatalf("servers: %+v", cfg.Servers)
	}
	if cfg.Envs[0].Name != "dev" || cfg.Envs[1].Name != "prod" || !cfg.Envs[1].Confirm {
		t.Fatalf("environment order or rules: %+v", cfg.Envs)
	}
	api := cfg.project("demo-api")
	prod := api.target("prod")
	if !prod.Confirm || prod.AllowedRefs != nil {
		t.Fatalf("prod: confirm is inherited, allowed_refs: null lifts the limit: %+v", prod)
	}
	if dev := api.target("dev"); dev.AllowedRefs != nil || dev.Confirm {
		t.Fatalf("dev: %+v", dev)
	}
	a, b := prod.destination("app-01"), prod.destination("app-02")
	if a.PostDeploy == nil || *a.PostDeploy != "sudo systemctl reload php-fpm" || b.PostDeploy != nil {
		t.Fatalf("post_deploy inheritance: %v %v", a.PostDeploy, b.PostDeploy)
	}
	if got := strings.Join(api.ignoreFor("prod", "app-02"), " "); got != ".env storage/ storage *.log /config/local.php uploads/ uploads" {
		t.Fatalf("ignore_for: %s", got)
	}
	if got := strings.Join(api.ignoreFor("prod", "app-01"), " "); got != ".env storage/ storage *.log /config/local.php" {
		t.Fatalf("ignore_for app-01: %s", got)
	}
	if a.statePath() != "/srv/www/.deploy/demo-api-prod" {
		t.Fatalf("state path: %s", a.statePath())
	}
	web := cfg.project("demo-web")
	if web.Build == nil || web.SourceDir != "dist" || web.target("dev").Destinations[0].Server != "app-02" {
		t.Fatalf("demo-web: %+v", web)
	}
}

func TestMediumStatePath(t *testing.T) {
	for _, c := range []struct{ path, stateDir, want string }{
		{"/var/www/billing", "", "/var/www/.deploy/billing"},
		{"/var/www/billing/", "", "/var/www/.deploy/billing"},
		{"/var/www//billing", "/srv/state", "/srv/state/var/www/billing"},
		{"/app", "", "/.deploy/app"},
	} {
		d := &mdDestination{Path: c.path, StateDir: c.stateDir}
		if got := d.statePath(); got != c.want {
			t.Fatalf("%s %s: %s, want %s", c.path, c.stateDir, got, c.want)
		}
	}
}

// Each broken configuration is rejected with the file and key it is in.
func TestMediumConfigErrors(t *testing.T) {
	project := func(body string) map[string]string {
		return map[string]string{"projects/demo-api.yaml": "name: demo-api\nrepo: git@git.example.com:team/demo-api.git\n" + body}
	}
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"duplicate key", map[string]string{"servers.yaml": mdServersYAML + "  app-01:\n    host: x\n    user: y\n"}, "given more than once"},
		{"unknown server key", map[string]string{"servers.yaml": "servers:\n  app-01: {host: h, user: u, hots: x}\n"}, "servers.app-01.hots: unknown key"},
		{"password", map[string]string{"servers.yaml": "servers:\n  app-01: {host: h, user: u, password: x}\n"}, "password login is not supported"},
		{"bad host", map[string]string{"servers.yaml": "servers:\n  app-01: {host: -oProxy, user: u}\n"}, "servers.app-01.host: invalid address"},
		{"bad port", map[string]string{"servers.yaml": "servers:\n  app-01: {host: h, user: u, port: 70000}\n"}, "port must be between 1 and 65535"},
		{"public key", map[string]string{"servers.yaml": "servers:\n  app-01: {host: h, user: u, key: id_demo.pub}\n"}, "that is the public key"},
		{"server name", map[string]string{"servers.yaml": "servers:\n  App_01: {host: h, user: u}\n"}, "invalid server name 'App_01'"},
		{"envs list", map[string]string{"environments.yaml": "environments: [dev, prod]\n"}, "not a list"},
		{"no envs", map[string]string{"environments.yaml": "environments: {}\n"}, "there is no environment"},
		{"empty refs", map[string]string{"environments.yaml": "environments:\n  dev: {allowed_refs: []}\n"}, "empty list would forbid"},
		{"refs prefix", map[string]string{"environments.yaml": "environments:\n  dev: {allowed_refs: [refs/remotes/x]}\n"}, "must start with refs/heads/"},
		{"missing envs", map[string]string{"environments.yaml": ""}, "environments.yaml: the file does not exist"},
		{"unknown env", project("targets:\n  qa: app-01:/srv/www/a\n"), "the environment 'qa' is not listed"},
		{"name mismatch", map[string]string{"projects/demo-api.yaml": "name: other\nrepo: r\ntargets:\n  dev: app-01:/srv/a\n"}, "must equal the file name (demo-api)"},
		{"unknown server", project("targets:\n  dev: app-09:/srv/www/a\n"), "the server 'app-09' is not defined"},
		{"relative path", project("targets:\n  dev: app-01:srv/www\n"), "must be absolute"},
		{"spaced destination", project("targets:\n  dev:\n    app-01: /srv/www/a\n"), "without a space after the colon"},
		{"old form", project("targets:\n  dev: {servers: [app-01], path: /srv/www/a}\n"), "old environment form"},
		{"root path", project("targets:\n  dev: app-01:/\n"), "must not be the root"},
		{"dotdot", project("targets:\n  dev: app-01:/srv/../etc\n"), "must not contain '..'"},
		{"duplicate destination server", project("targets:\n  dev: [app-01:/srv/a, app-01:/srv/b]\n"), "listed more than once"},
		{"state in app", project("targets:\n  dev: [{server: app-01, path: /srv/a, state_dir: /srv/a/state}]\n"), "overlaps the application directory"},
		{"rsync rule", project("ignore: ['+ keep']\ntargets:\n  dev: app-01:/srv/a\n"), "would read the pattern '+ keep' as a rule"},
		{"source_dir", project("source_dir: ../x\ntargets:\n  dev: app-01:/srv/a\n"), "must not leave the branch export"},
		{"unknown project key", project("sorce_dir: x\ntargets:\n  dev: app-01:/srv/a\n"), "sorce_dir: unknown key"},
		{"empty list", project("targets:\n  dev: []\n"), "the destination list is empty"},
		{"yml extension", map[string]string{"projects/demo-x.yml": "name: demo-x\n"}, "use the .yaml extension"},
		{"notepad", map[string]string{"projects/demo-x.yaml.txt": "name: demo-x\n"}, "rename it to demo-x.yaml"},
		{"tab", map[string]string{"servers.yaml": "servers:\n\tapp-01: {host: h, user: u}\n"}, "spaces, not tabs"},
		{"same path twice", map[string]string{"projects/demo-other.yaml": "name: demo-other\nrepo: r\ntargets:\n  dev: app-01:/srv/www/demo-api\n"}, "two destinations must not have the same server and path"},
		{"nested app", map[string]string{"projects/demo-other.yaml": "name: demo-other\nrepo: r\ntargets:\n  dev: app-01:/srv/www/demo-api/shop\n"}, "would delete the whole application in /shop"},
		{"shared state", map[string]string{"projects/demo-other.yaml": "name: demo-other\nrepo: r\ntargets:\n  dev: [{server: app-02, path: /srv/other/demo-web, state_dir: /srv/www/.deploy/x}]\n"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := mdLoadConfig(mdValidConfig(t, c.files))
			if c.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
	// a nested application covered by the outer project's ignore is fine
	ok := map[string]string{"projects/demo-other.yaml": "name: demo-other\nrepo: r\ntargets:\n  dev: [{server: app-01, path: /srv/www/demo-api/shop, state_dir: /srv/state}]\n",
		"projects/demo-api.yaml": "name: demo-api\nrepo: r\nignore: [/shop]\ntargets:\n  dev: app-01:/srv/www/demo-api\n"}
	if _, err := mdLoadConfig(mdValidConfig(t, ok)); err != nil {
		t.Fatalf("ignored nested app: %v", err)
	}
	// a YAML merge key and an anchor are read like PyYAML does
	merged := map[string]string{"servers.yaml": "base: &b {user: deploy}\n"}
	if _, err := mdLoadConfig(mdValidConfig(t, merged)); err == nil || !strings.Contains(err.Error(), "base: unknown key") {
		t.Fatalf("extra top-level key: %v", err)
	}
	anchors := map[string]string{"servers.yaml": "servers:\n  app-01: &s {host: app-01.example.com, user: deploy}\n  app-02:\n    <<: *s\n    host: 192.0.2.12\n"}
	cfg, err := mdLoadConfig(mdValidConfig(t, anchors))
	if err != nil || cfg.server("app-02").User != "deploy" || cfg.server("app-02").Host != "192.0.2.12" {
		t.Fatalf("merge key: %v", err)
	}
}

func TestMediumRefAllowed(t *testing.T) {
	refs := []string{"refs/heads/main", "refs/tags/v*", "release-*"}
	for ref, want := range map[string]bool{
		"refs/heads/main": true, "refs/tags/v1.2.0": true, "refs/heads/v2-test": false,
		"refs/heads/release-1": true, "refs/tags/release-2": true, "main": false, "": false,
	} {
		if got := mdRefAllowed(ref, refs); got != want {
			t.Fatalf("%q: %v, want %v", ref, got, want)
		}
	}
	if !mdRefAllowed("refs/heads/x", nil) {
		t.Fatal("nil allows everything")
	}
}

func TestMediumRepoKey(t *testing.T) {
	for repo, want := range map[string]string{
		"git@git.example.com:team/demo-api.git":            "git.example.com team/demo-api",
		"ssh://git@Git.Example.com:2222/team/demo-api.git": "git.example.com team/demo-api",
		"https://git.example.com/team/sub/demo-api.git":    "git.example.com team/sub/demo-api",
		"https://user:tok@git.example.com/team/demo-api":   "git.example.com team/demo-api",
		"/srv/repos/demo-api":                              " ",
	} {
		h, p := mdRepoKey(repo)
		if got := h + " " + p; got != want {
			t.Fatalf("%s: %q, want %q", repo, got, want)
		}
	}
}

func TestMediumEscapePath(t *testing.T) {
	for _, p := range []string{"a/b.txt", "gre\x9aka.txt", `odd\#name`, "čšž/\xff\xfe", ""} {
		if back := mdUnescapePath(mdEscapePath(p)); back != p {
			t.Fatalf("%q → %q → %q", p, mdEscapePath(p), back)
		}
	}
	if got := mdEscapePath("gre\x9aka"); got != `gre\#232ka` {
		t.Fatalf("escape: %s", got)
	}
}

func TestMediumSHA256SumParse(t *testing.T) {
	h := strings.Repeat("a", 64)
	out := h + "  plain.txt\n\\" + h + "  new\\nline\\\\x\n"
	got := mdParseSHA256Sum(out)
	if got["plain.txt"] != h || got["new\nline\\x"] != h {
		t.Fatalf("%v", got)
	}
}

// ─── rsync rules and the comparison, checked against real rsync ───

// mdParseItemize is the original tool's parse_itemize (test reference).
func mdParseItemize(out string) []mdChange {
	itemRe := regexp.MustCompile(`^([<>ch.])([fdLDS])(.{9}) (.+)$`)
	delRe := regexp.MustCompile(`^\*deleting {3}(.+)$`)
	keptRe := regexp.MustCompile(`^cannot delete non-empty directory: (.+)$`)
	kinds := map[string]string{"f": "file", "d": "dir", "L": "symlink", "D": "other", "S": "other"}
	lines := strings.Split(out, "\n")
	kept := map[string]bool{}
	for _, l := range lines {
		if m := keptRe.FindStringSubmatch(l); m != nil {
			kept[strings.TrimRight(mdUnescapePath(m[1]), "/")+"/"] = true
		}
	}
	var res []mdChange
	for _, l := range lines {
		l = strings.TrimRight(l, "\r")
		if m := delRe.FindStringSubmatch(l); m != nil {
			p := mdUnescapePath(m[1])
			if kept[p] {
				continue
			}
			kind := "file"
			if strings.HasSuffix(p, "/") {
				kind = "dir"
			}
			res = append(res, mdChange{Path: p, Action: "deleted", Kind: kind})
			continue
		}
		m := itemRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		update, typ, attrs, name := m[1], m[2], m[3], m[4]
		kind := kinds[typ]
		link := ""
		if kind == "symlink" && strings.Contains(name, " -> ") {
			i := strings.Index(name, " -> ")
			name, link = name[:i], mdUnescapePath(name[i+4:])
		}
		p := mdUnescapePath(name)
		if p == "./" || p == "." {
			continue
		}
		action := "changed"
		if attrs == "+++++++++" {
			action = "new"
		} else if update == "." && attrs[0] != 'c' && attrs[1] != 's' {
			continue
		}
		res = append(res, mdChange{Path: p, Action: action, Kind: kind, Link: link})
	}
	return res
}

func mdRsync(t *testing.T, src, dst string, excludes []string) []mdChange {
	t.Helper()
	changes, failed := mdRsyncRC(t, src, dst, excludes)
	if failed {
		return []mdChange{{Path: "rsync exit code 23", Action: "error"}}
	}
	return changes
}

func mdRsyncRC(t *testing.T, src, dst string, excludes []string) ([]mdChange, bool) {
	t.Helper()
	args := []string{"-rlc", "--delete", "--itemize-changes", "--protect-args", "--8-bit-output", "--dry-run"}
	for _, e := range excludes {
		args = append(args, "--exclude="+e)
	}
	args = append(args, src+"/", dst+"/")
	out, err := exec.Command("rsync", args...).CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 23 {
			t.Fatalf("rsync %v: %v\n%s", excludes, err, out)
		}
		return nil, true
	}
	return mdParseItemize(string(out)), false
}

func mdNative(t *testing.T, src, dst string, excludes []string) []mdChange {
	t.Helper()
	tree, err := mdLocalTree(src)
	if err != nil {
		t.Fatal(err)
	}
	rules := mdCompileRules(excludes)
	filtered := mdFilterTree(tree, rules)
	d, keep, _, err := mdWalk(mdLocal{}, dst, rules, filtered)
	if err != nil {
		t.Fatal(err)
	}
	if err := mdFillHashes(mdLocal{}, dst, filtered, d); err != nil {
		t.Fatal(err)
	}
	changes, conflicts := mdDiff(filtered, d, keep)
	if len(conflicts) > 0 {
		return []mdChange{{Path: "rsync exit code 23", Action: "error"}}
	}
	return changes
}

func mdSame(t *testing.T, label string, want, got []mdChange) {
	t.Helper()
	c := mdCompareChanges(want, got)
	if !c.Same {
		show := func(l []mdChange) string {
			var s []string
			for _, x := range l {
				s = append(s, x.Action+":"+x.Kind+":"+x.Path)
			}
			sort.Strings(s)
			return strings.Join(s, "\n  ")
		}
		t.Fatalf("%s\nrsync only:\n  %s\nnative only:\n  %s\nkind differs: %v\nrsync:\n  %s", label, show(c.OnlyReference), show(c.OnlyNative), c.KindDiffers, show(want))
	}
}

func needRsync(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync is not installed")
	}
}

func mdMk(t *testing.T, root string, spec map[string]string) {
	t.Helper()
	for p, v := range spec {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0755)
		switch {
		case v == "/":
			os.MkdirAll(full, 0755)
		case strings.HasPrefix(v, "->"):
			os.Symlink(strings.TrimPrefix(v, "->"), full)
		default:
			os.WriteFile(full, []byte(v), 0644)
		}
	}
}

func TestMediumDiffMatchesRsyncCases(t *testing.T) {
	needRsync(t)
	root := t.TempDir()
	src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
	mdMk(t, src, map[string]string{"a/f": "hi", "filedir": "x", "dirfile/in": "1", "link1": "->target", "linkchg": "->t2",
		"same/s": "s", "linkreplaced": "new", "dirlink/q": "q", "sz": "z", "d2": "/", "storage/app/x": "1", "linkdir": "->a",
		"čćž/ü.txt": "u", "cache/keep": "k", "x.log": "l", "config/local.php": "src", "sub/config/local.php": "s2"})
	mdMk(t, dst, map[string]string{"a/f": "hi", "old/sub/o": "o", "filedir/inner": "y", "dirfile": "1", "linkchg": "->other",
		"same/s": "s", "linkreplaced": "->x", "dirlink": "->/tmp", "sz": "zz", "oldlink": "->q", "keep/cache/c": "c",
		"keep/stale": "s", "storage": "->/srv/shared", "linkdir/z": "z", "x/y/cache/c": "c", "x/y/o": "o", "x/t": "t",
		"config/local.php": "server", "sub/config/local.php": "server2", "app.log": "old"})
	for _, ex := range [][]string{nil, {"cache"}, {"cache/", "storage/", "storage"}, {"*.log", "/config/local.php"}, {"config/local.php"}, {"**/cache", "x/***"}, {"[a-c]*"}, {"*/keep/"}} {
		mdSame(t, fmt.Sprint(ex), mdRsync(t, src, dst, ex), mdNative(t, src, dst, ex))
	}
	// a destination that does not exist yet: everything is new
	mdSame(t, "missing destination", mdRsync(t, src, filepath.Join(root, "missing"), nil), mdNative(t, src, filepath.Join(root, "missing"), nil))
}

// Random trees and patterns: the native comparison gives exactly rsync's change list.
func TestMediumDiffMatchesRsyncRandom(t *testing.T) {
	needRsync(t)
	names := []string{"a", "b", "cache", "keep", "node_modules", "x.log", "y.txt", "data", "a.bin", "z", "storage", "čvor", "[x]", "sp ace"}
	patterns := []string{"*.log", "cache/", "cache", "/a", "/a/b", "node_modules", "**/keep", "data/*.bin", "a/b", "[a-c].txt", "z/***",
		"*/keep/", "/storage/", "storage", "?.bin", "**.log", "b/**/z", "/sp ace", "\\[x]", "čvor/"}
	rng := rand.New(rand.NewSource(12345))
	for iter := 0; iter < 150; iter++ {
		root := t.TempDir()
		src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)
		gen := func(base string, depth int) map[string]string {
			spec := map[string]string{}
			var walk func(prefix string, d int)
			walk = func(prefix string, d int) {
				n := 1 + rng.Intn(4)
				for i := 0; i < n; i++ {
					p := names[rng.Intn(len(names))]
					if prefix != "" {
						p = prefix + "/" + p
					}
					switch k := rng.Intn(10); {
					case k < 5:
						spec[p] = fmt.Sprintf("c%d", rng.Intn(3))
					case k < 6:
						spec[p] = "->" + names[rng.Intn(len(names))]
					default:
						if d < depth {
							walk(p, d+1)
						} else {
							spec[p] = "/"
						}
					}
				}
			}
			walk("", 0)
			return spec
		}
		srcSpec, dstSpec := gen(src, 3), gen(dst, 3)
		// share some paths so files are compared, not only added or deleted
		for p, v := range srcSpec {
			if rng.Intn(2) == 0 {
				dstSpec[p] = v
			}
		}
		mdMkOrdered(t, src, srcSpec)
		mdMkOrdered(t, dst, dstSpec)
		var ex []string
		for _, p := range patterns {
			if rng.Intn(4) == 0 {
				ex = append(ex, p)
			}
		}
		mdSame(t, fmt.Sprintf("iteration %d, excludes %q", iter, ex), mdRsync(t, src, dst, ex), mdNative(t, src, dst, ex))
	}
}

// mdMkOrdered creates the shortest paths first, so a file does not block a directory
// generated for one of its descendants (the generator can produce both).
func mdMkOrdered(t *testing.T, root string, spec map[string]string) {
	paths := make([]string, 0, len(spec))
	for p := range spec {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], "/") < strings.Count(paths[j], "/") || strings.Count(paths[i], "/") == strings.Count(paths[j], "/") && paths[i] < paths[j]
	})
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		if _, err := os.Lstat(filepath.Dir(full)); err == nil {
			if fi, _ := os.Lstat(filepath.Dir(full)); !fi.IsDir() {
				continue
			}
		}
		os.MkdirAll(filepath.Dir(full), 0755)
		if _, err := os.Lstat(full); err == nil {
			continue
		}
		v := spec[p]
		switch {
		case v == "/":
			os.MkdirAll(full, 0755)
		case strings.HasPrefix(v, "->"):
			os.Symlink(strings.TrimPrefix(v, "->"), full)
		default:
			os.WriteFile(full, []byte(v), 0644)
		}
	}
}

func TestMediumRuleMatching(t *testing.T) {
	cases := []struct {
		pattern, path string
		dir, want     bool
	}{
		{"*.log", "a/b/x.log", false, true},
		{"cache/", "a/cache", false, false},
		{"cache/", "a/cache", true, true},
		{"/config/local.php", "config/local.php", false, true},
		{"/config/local.php", "x/config/local.php", false, false},
		{"config/local.php", "x/config/local.php", false, true},
		{"config/local.php", "local.php", false, false},
		{"**/node_modules", "a/b/node_modules", true, true},
		{"a/*", "a/b/c", false, false},
		{"a/**", "a/b/c", false, true},
		{"z/***", "z", true, true},
		{"z/***", "z", false, false},
		{"z/***", "z/a/b", false, true},
		{"**/cache", "cache", true, true},
		{"[a-c].txt", "b.txt", false, true},
		{"[!a-c].txt", "b.txt", false, false},
		{`\[x]`, "[x]", false, true},
		{"[[:digit:]]*", "1abc", false, true},
	}
	for _, c := range cases {
		if got := mdCompileRule(c.pattern).matches(c.path, c.dir); got != c.want {
			t.Fatalf("%q vs %q (dir %v): %v, want %v", c.pattern, c.path, c.dir, got, c.want)
		}
	}
	if got := strings.Join(mdExcludeVariants("storage/"), ","); got != "storage/,storage" {
		t.Fatal(got)
	}
	if got := strings.Join(mdExcludeVariants("cache*/"), ","); got != "cache*/" {
		t.Fatal(got)
	}
}

// ─── server state ───

func TestMediumReadState(t *testing.T) {
	root := t.TempDir()
	d := &mdDestination{Server: "app-01", Path: filepath.ToSlash(filepath.Join(root, "www", "demo-api"))}
	state := filepath.Join(root, "www", ".deploy", "demo-api")
	os.MkdirAll(filepath.Join(state, "lock"), 0755)
	os.WriteFile(filepath.Join(state, "current.json"), []byte(`{"project":"demo-api","environment":"prod","version":"v1.2.0","commit":"`+strings.Repeat("a", 40)+`","branch":"v1.2.0","deploy_id":"20261015-120000-aaaaaaa","deployed_at":"2026-10-15T12:00:00Z","deployed_by":"ana","path":"`+d.appDir()+`"}`), 0644)
	os.WriteFile(filepath.Join(state, "history.jsonl"), []byte(`{"deploy_id":"1","new":["a"],"path":"/other"}`+"\n"+`{"deploy_id":"2","new":["b"],"counts":{"new":1}}`+"\n"), 0644)
	os.WriteFile(filepath.Join(state, "lock", "owner.json"), []byte(`{"user":"ana","host":"laptop","since":"2026-10-15T12:00:00Z"}`), 0644)
	st, err := mdReadState(mdLocal{}, d)
	if err != nil {
		t.Fatal(err)
	}
	if st.Current["version"] != "v1.2.0" || len(st.History) != 1 || st.History[0]["deploy_id"] != "2" || st.History[0]["new"] != nil || st.Lock["user"] != "ana" {
		t.Fatalf("state: %+v", st)
	}
	// a record of another application is refused
	os.WriteFile(filepath.Join(state, "current.json"), []byte(`{"path":"/srv/elsewhere"}`), 0644)
	if _, err := mdReadState(mdLocal{}, d); err == nil || !strings.Contains(err.Error(), "share their state") {
		t.Fatalf("foreign state: %v", err)
	}
}

// ─── the whole comparison run, with the original tool and the servers faked ───

func TestMediumNativeCompareRun(t *testing.T) {
	needRsync(t)
	uid := 990001
	cfgDir := mediumConfigDir(uid)
	os.RemoveAll(cfgDir)
	t.Cleanup(func() { os.RemoveAll(mediumUserDir(uid)) })
	root := t.TempDir()
	srv := filepath.ToSlash(filepath.Join(root, "srv"))
	files := map[string]string{
		"servers.yaml":           mdServersYAML,
		"environments.yaml":      mdEnvsYAML,
		"projects/demo-api.yaml": "name: demo-api\nrepo: git@git.example.com:team/demo-api.git\nignore: [.env, storage/]\ntargets:\n  prod:\n    destinations:\n      - app-01:" + srv + "/a/demo-api\n      - {server: app-02, path: " + srv + "/b/demo-api, ignore_extra: [uploads/]}\n",
	}
	for name, content := range files {
		p := filepath.Join(cfgDir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, []byte(content), 0600)
	}
	src := filepath.Join(root, "export")
	mdMk(t, src, map[string]string{"index.php": "v2", "lib/a.php": "a", "uploads/x": "src", "storage": "/", "link": "->lib"})
	mdMk(t, filepath.Join(root, "srv", "a", "demo-api"), map[string]string{"index.php": "v1", "lib/a.php": "a", "old.php": "o", ".env": "secret", "storage": "->/srv/shared"})
	mdMk(t, filepath.Join(root, "srv", "b", "demo-api"), map[string]string{"index.php": "v2", "uploads/y": "server"})
	os.MkdirAll(filepath.Join(root, "srv", "a", ".deploy", "demo-api"), 0755)
	os.WriteFile(filepath.Join(root, "srv", "a", ".deploy", "demo-api", "current.json"), []byte(`{"deploy_id":"20261001-000000-aaaaaaa"}`), 0644)

	cfg, err := mdLoadConfig(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	proj := cfg.project("demo-api")
	oldRef, oldOpen := mdReferencePlan, mdOpenDestination
	t.Cleanup(func() { mdReferencePlan, mdOpenDestination = oldRef, oldOpen })
	mdReferencePlan = func(uid int, req mdPlanRequest) (*mdRefResult, error) {
		tree, _ := mdLocalTree(src)
		res := &mdRefResult{Ref: "refs/tags/v1.2.0", Commit: strings.Repeat("b", 40), Version: "v1.2.0", Confirm: true, RefAllowed: true}
		for p, e := range tree {
			res.Source = append(res.Source, mdRefSource{Path: p, Type: e.Type, Size: e.Size, Hash: e.Hash, Link: e.Link})
		}
		for _, d := range proj.target("prod").Destinations {
			ex := proj.ignoreFor("prod", d.Server)
			rs := mdRefServer{Server: d.Server, Path: d.appDir(), Excludes: ex, Changes: mdRsync(t, src, d.appDir(), ex)}
			if d.Server == "app-01" {
				rs.Current = map[string]interface{}{"deploy_id": "20261001-000000-aaaaaaa"}
			}
			res.Servers = append(res.Servers, rs)
		}
		return res, nil
	}
	mdOpenDestination = func(uid int, cfg *mdConfig, server string, fn func(mdRemote, string) error) error {
		return fn(mdLocal{}, "local "+server)
	}
	res, _, _, err := mdExecute(uid, mdPlanRequest{Project: "demo-api", Environment: "prod", Ref: "v1.2.0", Compare: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Same || len(res.Servers) != 2 {
		t.Fatalf("not the same: %s", jsonMarshal(res))
	}
	a := res.Servers[0]
	if a.Counts["changed"] != 1 || a.Counts["deleted"] != 1 || a.State == nil || a.State.Current["deploy_id"] != "20261001-000000-aaaaaaa" {
		t.Fatalf("app-01: %s", jsonMarshal(a))
	}
	// storage/ was a symlink on the server: it is protected (storage and storage/ excluded)
	for _, c := range a.Changes {
		if strings.HasPrefix(c.Path, "storage") || c.Path == ".env" {
			t.Fatalf("excluded path in the plan: %+v", c)
		}
	}
	// a plan without the original tool needs a Git source for the project
	if _, _, _, err := mdExecute(uid, mdPlanRequest{Project: "demo-api", Environment: "prod", Ref: "main"}, func(string) {}); err == nil || !strings.Contains(err.Error(), "no WRM Git source") {
		t.Fatalf("no source: %v", err)
	}
	// unknown server selection
	if _, _, _, err := mdExecute(uid, mdPlanRequest{Project: "demo-api", Environment: "prod", Ref: "main", Servers: []string{"app-09"}}, func(string) {}); err == nil {
		t.Fatal("unknown server accepted")
	}
}

func TestMediumLinksAutoMatch(t *testing.T) {
	uid := 990002
	db.Exec(`INSERT INTO users (id, username, password_hash) VALUES (?, 'md-links', 'x')`, uid)
	res, err := db.Exec(`INSERT INTO connections (name, protocol, host, username, auth_method, password, private_key, key_path, user_id) VALUES ('demo app-02','SSH','192.0.2.12:2222','deploy','password','','','',?)`, uid)
	if err != nil {
		t.Skip("connections table:", err)
	}
	id, _ := res.LastInsertId()
	cfg, err := mdLoadConfig(mdValidConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	links := mdLinks(uid, cfg, mdUserConnections(uid))
	if l, ok := links["app-02"]; !ok || l.ConnID != int(id) || !l.Auto {
		t.Fatalf("auto link: %+v", links)
	}
	if _, ok := links["app-01"]; ok {
		t.Fatalf("app-01 has no matching connection: %+v", links)
	}
	db.Exec(`INSERT OR REPLACE INTO medium_server_links (user_id, server, conn_id) VALUES (?,?,?)`, uid, "app-01", id)
	if l := mdLinks(uid, cfg, mdUserConnections(uid))["app-01"]; l.ConnID != int(id) || l.Auto {
		t.Fatalf("chosen link: %+v", l)
	}
}

// TestMediumGitSource covers the native source: a deploy-config repository address
// finds the WRM Git source, refs resolve to commits and the tree below source_dir
// carries SHA-256 hashes (GitLab, GitHub and Gitea APIs).
func TestMediumGitSource(t *testing.T) {
	srv := newTestServer(t)
	for _, kind := range []string{"gitlab", "github", "gitea"} {
		t.Run(kind, func(t *testing.T) {
			g := newFakeGit(t, kind)
			u := setupGitUser(t, srv, "md-"+kind, g)
			host := strings.Split(strings.TrimPrefix(g.srv.URL, "http://"), ":")[0]
			if _, ok := mdFindSource(u.userID, "git@"+host+":demo/other.git"); !ok {
				t.Fatal("the address should match the source's host")
			}
			if _, ok := mdFindSource(u.userID, "git@elsewhere.example.com:demo/demo-api.git"); ok {
				t.Fatal("another host must not match")
			}
			m, ok := mdFindSource(u.userID, "ssh://git@"+host+":2222/demo/demo-api.git")
			if !ok || m.project != "demo/demo-api" {
				t.Fatalf("source: %+v %v", m, ok)
			}
			p, err := newGitProvider(m.src)
			if err != nil {
				t.Fatal(err)
			}
			for ref, want := range map[string][2]string{
				"main":               {commitSHA("c4"), "refs/heads/main"},
				"v1.10":              {commitSHA("c2"), "refs/tags/v1.10"},
				"refs/tags/v1.9":     {commitSHA("c1"), "refs/tags/v1.9"},
				"refs/heads/feature": {commitSHA("f1"), "refs/heads/feature"},
				commitSHA("c3"):      {commitSHA("c3"), commitSHA("c3")},
			} {
				sha, full, err := mdResolveRef(p, m.project, ref)
				if err != nil || sha != want[0] || full != want[1] {
					t.Errorf("%s → %s %s %v", ref, sha, full, err)
				}
			}
			for _, bad := range []string{"", "-x", "nope"} {
				if _, _, err := mdResolveRef(p, m.project, bad); err == nil {
					t.Errorf("%q should not resolve", bad)
				}
			}
			tree, err := mdGitSourceTree(m, &mdProject{SourceDir: "linux"}, commitSHA("c2"))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(runV110))
			if e := tree["run.py"]; e == nil || e.Type != "file" || e.Hash != hex.EncodeToString(sum[:]) || e.Size != int64(len(runV110)) {
				t.Fatalf("run.py: %+v", tree["run.py"])
			}
			if e := tree["lib"]; e == nil || e.Type != "dir" {
				t.Fatalf("lib: %+v", e)
			}
			if tree["README.md"] != nil || tree["linux/run.py"] != nil || len(tree) != 4 {
				t.Fatalf("tree below source_dir: %v", tree)
			}
			before := g.treeReqs.Load()
			if _, err := mdGitSourceTree(m, &mdProject{SourceDir: "."}, commitSHA("c2")); err != nil {
				t.Fatal(err)
			}
			if g.treeReqs.Load() != before {
				t.Fatal("the tree of a commit should come from the cache")
			}
			if _, err := mdGitSourceTree(m, &mdProject{SourceDir: "missing"}, commitSHA("c2")); err == nil {
				t.Fatal("a missing source_dir must fail")
			}
		})
	}
}
