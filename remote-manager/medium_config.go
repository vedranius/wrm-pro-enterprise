// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// ─── MEDIUM: deploy-config reader and validator ───
//
// The native Medium engine reads the original deploy tool's configuration directory as it
// is, without conversion:
//
//	deploy-config/
//	├── servers.yaml        servers: {name: {host, user, port, key}}
//	├── environments.yaml   environments: {name: {confirm, allowed_refs, servers}} (order = UI order)
//	└── projects/<name>.yaml  name, repo, source_dir, build, ignore, targets
//
// A project environment (a key under targets) is "server:/path", a list of destinations
// (text or {server, path, state_dir, ignore_extra, post_deploy}) or a mapping with
// destinations and the environment settings (ignore_extra, post_deploy, state_dir,
// allowed_refs, confirm). Project settings override the environment rules, and destination
// settings override the project environment's ones. The validation follows the original
// tool rule by rule, so a configuration it rejects is rejected here too, and the first
// problem is reported with the file and key it is in.

var mdNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
var mdHostRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,252}$`)
var mdUserRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}$`)
var mdControlRe = regexp.MustCompile("[\x00-\x1f\x7f]")

const (
	mdServersFile = "servers.yaml"
	mdEnvsFile    = "environments.yaml"
	mdProjectsDir = "projects"
	mdStateDir    = ".deploy"
)

type mdServer struct {
	Name string `json:"name"`
	Host string `json:"host"`
	User string `json:"user"`
	Port int    `json:"port"`
	Key  string `json:"key,omitempty"` // a key file of the original tool; the native engine uses WRM keys
}

type mdEnvironment struct {
	Name        string   `json:"name"`
	Confirm     bool     `json:"confirm"`
	AllowedRefs []string `json:"allowed_refs"` // nil = every branch and tag
	Servers     []string `json:"servers"`      // nil = every server
}

type mdDestination struct {
	Server      string   `json:"server"`
	Path        string   `json:"path"`
	StateDir    string   `json:"state_dir,omitempty"`
	IgnoreExtra []string `json:"ignore_extra"`
	PostDeploy  *string  `json:"post_deploy"` // effective command on this server (nil = none)
	postSet     bool     // the destination names post_deploy itself (null included)
}

type mdTarget struct {
	Name         string           `json:"name"`
	Destinations []*mdDestination `json:"destinations"`
	IgnoreExtra  []string         `json:"ignore_extra"`
	PostDeploy   *string          `json:"post_deploy"`
	StateDir     string           `json:"state_dir,omitempty"`
	AllowedRefs  []string         `json:"allowed_refs"`
	Confirm      bool             `json:"confirm"`
}

type mdProject struct {
	Name      string      `json:"name"`
	Repo      string      `json:"repo"`
	SourceDir string      `json:"source_dir"`
	Build     *string     `json:"build"`
	Ignore    []string    `json:"ignore"`
	Targets   []*mdTarget `json:"targets"` // in file order
}

type mdConfig struct {
	Servers  []*mdServer      `json:"servers"`
	Envs     []*mdEnvironment `json:"environments"`
	Projects []*mdProject     `json:"projects"`
}

type mdConfigError struct{ msg string }

func (e *mdConfigError) Error() string { return e.msg }

func mdErr(format string, a ...interface{}) error { return &mdConfigError{fmt.Sprintf(format, a...)} }

func (c *mdConfig) server(name string) *mdServer {
	for _, s := range c.Servers {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (c *mdConfig) env(name string) *mdEnvironment {
	for _, e := range c.Envs {
		if e.Name == name {
			return e
		}
	}
	return nil
}

func (c *mdConfig) project(name string) *mdProject {
	for _, p := range c.Projects {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (p *mdProject) target(env string) *mdTarget {
	for _, t := range p.Targets {
		if t.Name == env {
			return t
		}
	}
	return nil
}

func (t *mdTarget) destination(server string) *mdDestination {
	for _, d := range t.Destinations {
		if d.Server == server {
			return d
		}
	}
	return nil
}

// ignoreFor is the original Project.ignore_for: project ignore, the environment's
// ignore_extra and, with a server, that destination's ignore_extra, without duplicates and
// with "dir/" also as "dir" (exclude_variants).
func (p *mdProject) ignoreFor(env, server string) []string {
	t := p.target(env)
	if t == nil {
		return nil
	}
	all := append(append([]string{}, p.Ignore...), t.IgnoreExtra...)
	if server != "" {
		if d := t.destination(server); d != nil {
			all = append(all, d.IgnoreExtra...)
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, pattern := range all {
		for _, v := range mdExcludeVariants(pattern) {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

// ─── paths ───

// mdNorm is posixpath.normpath for an absolute path ("/var/www//app/" → "/var/www/app").
func mdNorm(p string) string {
	if p == "" {
		return "."
	}
	n := path.Clean(p)
	if strings.HasPrefix(n, "//") {
		n = "/" + strings.TrimLeft(n, "/")
	}
	return n
}

func mdWithin(child, parent string) bool {
	return child == parent || strings.HasPrefix(child, strings.TrimRight(parent, "/")+"/")
}

func (d *mdDestination) appDir() string { return mdNorm(d.Path) }
func (d *mdDestination) label() string  { return d.Server + ":" + d.appDir() }

// stateRoot is state_dir, or "<parent of path>/.deploy".
func (d *mdDestination) stateRoot() string {
	if d.StateDir != "" {
		return mdNorm(d.StateDir)
	}
	return path.Join(path.Dir(d.appDir()), mdStateDir)
}

// statePath is where current.json, history.jsonl, lock/ and backups/ are: by default
// "<parent>/.deploy/<basename>", with state_dir "<state_dir>/<whole path>".
func (d *mdDestination) statePath() string {
	if d.StateDir != "" {
		return path.Join(d.stateRoot(), strings.TrimLeft(d.appDir(), "/"))
	}
	return path.Join(d.stateRoot(), path.Base(d.appDir()))
}

// mdParseDestination splits "server:/path" at the first colon.
func mdParseDestination(text string) (string, string, error) {
	i := strings.Index(text, ":")
	if i < 0 {
		return "", "", fmt.Errorf("%s – expected server:/path (for example dev-app1:/var/www/app)", pyRepr(text))
	}
	server, p := text[:i], text[i+1:]
	if strings.TrimSpace(server) == "" {
		return "", "", fmt.Errorf("%s – the server name is missing before ':'", pyRepr(text))
	}
	if !strings.HasPrefix(p, "/") {
		return "", "", fmt.Errorf("%s – the path after ':' must be absolute (start with /)", pyRepr(text))
	}
	return server, p, nil
}

// pyRepr quotes a value the way the original tool's messages show it ('value').
func pyRepr(v interface{}) string {
	switch x := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(x, `\`, `\\`), "'", `\'`) + "'"
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case *mdYAML:
		return x.repr()
	}
	return fmt.Sprint(v)
}

// ─── YAML values ───

// mdYAML is a YAML value with mapping keys in file order (environments, targets).
type mdYAML struct {
	kind  string // map | seq | str | int | float | bool | null
	keys  []string
	vals  []*mdYAML
	items []*mdYAML
	str   string
	i     int64
	f     float64
	b     bool
	line  int
	nkey  bool // a mapping with a key that is not text
}

func (v *mdYAML) get(key string) (*mdYAML, bool) {
	if v == nil || v.kind != "map" {
		return nil, false
	}
	for i, k := range v.keys {
		if k == key {
			return v.vals[i], true
		}
	}
	return nil, false
}

func (v *mdYAML) has(key string) bool { _, ok := v.get(key); return ok }

func (v *mdYAML) typeName() string {
	switch v.kind {
	case "seq":
		return "a list"
	case "str":
		return "text"
	case "int", "float":
		return "a number"
	case "bool":
		return "a boolean"
	case "null":
		return "empty"
	}
	return "a mapping"
}

func (v *mdYAML) repr() string {
	var s string
	switch v.kind {
	case "str":
		s = pyRepr(v.str)
	case "int":
		s = strconv.FormatInt(v.i, 10)
	case "float":
		s = strconv.FormatFloat(v.f, 'g', -1, 64)
	case "bool":
		s = pyRepr(v.b)
	case "null":
		s = "None"
	case "seq":
		parts := make([]string, len(v.items))
		for i, it := range v.items {
			parts[i] = it.repr()
		}
		s = "[" + strings.Join(parts, ", ") + "]"
	default:
		parts := make([]string, len(v.keys))
		for i, k := range v.keys {
			parts[i] = pyRepr(k) + ": " + v.vals[i].repr()
		}
		s = "{" + strings.Join(parts, ", ") + "}"
	}
	if len(s) > 60 {
		s = truncateStr(s, 57) + "..."
	}
	return s
}

func mdFromNode(n *yaml.Node, depth int) (*mdYAML, error) {
	if depth > 64 {
		return nil, fmt.Errorf("nesting is too deep")
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return &mdYAML{kind: "null"}, nil
		}
		return mdFromNode(n.Content[0], depth+1)
	case yaml.AliasNode:
		return mdFromNode(n.Alias, depth+1)
	case yaml.SequenceNode:
		v := &mdYAML{kind: "seq", line: n.Line}
		for _, c := range n.Content {
			it, err := mdFromNode(c, depth+1)
			if err != nil {
				return nil, err
			}
			v.items = append(v.items, it)
		}
		return v, nil
	case yaml.MappingNode:
		v := &mdYAML{kind: "map", line: n.Line}
		seen := map[string]bool{}
		var merged []*mdYAML
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, val := n.Content[i], n.Content[i+1]
			if k.Kind == yaml.ScalarNode && k.Tag == "!!merge" {
				m, err := mdFromNode(val, depth+1)
				if err != nil {
					return nil, err
				}
				if m.kind == "seq" {
					merged = append(merged, m.items...)
				} else {
					merged = append(merged, m)
				}
				continue
			}
			kv, err := mdFromNode(k, depth+1)
			if err != nil {
				return nil, err
			}
			var key string
			switch kv.kind {
			case "str":
				key = kv.str
			case "int", "float", "bool", "null":
				key = kv.repr()
				v.nkey = true
			default:
				return nil, fmt.Errorf("a key must be text (line %d)", k.Line)
			}
			if seen[key] {
				return nil, fmt.Errorf("key %s is given more than once (line %d, column %d)", pyRepr(key), k.Line, k.Column)
			}
			seen[key] = true
			vv, err := mdFromNode(val, depth+1)
			if err != nil {
				return nil, err
			}
			v.keys = append(v.keys, key)
			v.vals = append(v.vals, vv)
		}
		// "<<" merge: keys of the merged mappings that the mapping does not set itself
		for _, m := range merged {
			if m.kind != "map" {
				return nil, fmt.Errorf("a merge (<<) needs a mapping (line %d)", n.Line)
			}
			for i, k := range m.keys {
				if !seen[k] {
					seen[k] = true
					v.keys = append(v.keys, k)
					v.vals = append(v.vals, m.vals[i])
				}
			}
		}
		return v, nil
	case yaml.ScalarNode:
		v := &mdYAML{line: n.Line}
		switch n.ShortTag() {
		case "!!null":
			v.kind = "null"
		case "!!bool":
			v.kind = "bool"
			var b bool
			if n.Decode(&b) != nil {
				return nil, fmt.Errorf("invalid boolean %q (line %d)", n.Value, n.Line)
			}
			v.b = b
		case "!!int":
			v.kind = "int"
			var i int64
			if n.Decode(&i) != nil {
				v.kind, v.f = "float", 0
				var f float64
				if n.Decode(&f) != nil {
					return nil, fmt.Errorf("invalid number %q (line %d)", n.Value, n.Line)
				}
				v.f = f
			}
			v.i = i
		case "!!float":
			v.kind = "float"
			var f float64
			if n.Decode(&f) != nil {
				return nil, fmt.Errorf("invalid number %q (line %d)", n.Value, n.Line)
			}
			v.f = f
		case "!!str", "!!timestamp", "!!binary":
			v.kind, v.str = "str", n.Value
			// YAML 1.1 (the original tool) reads these plain words as booleans
			if n.Style == 0 && n.ShortTag() == "!!str" {
				switch n.Value {
				case "yes", "Yes", "YES", "on", "On", "ON":
					v.kind, v.b = "bool", true
				case "no", "No", "NO", "off", "Off", "OFF":
					v.kind, v.b = "bool", false
				}
			}
		default:
			return nil, fmt.Errorf("unsupported YAML tag %s (line %d)", n.Tag, n.Line)
		}
		return v, nil
	}
	return nil, fmt.Errorf("unsupported YAML node (line %d)", n.Line)
}

// mdReadYAML reads one configuration file (UTF-8, an optional BOM) into ordered values.
func mdReadYAML(file, rel string) (*mdYAML, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, mdErr("%s: cannot read the file (%v).", rel, err)
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(b) {
		return nil, mdErr("%s: the file is not UTF-8 encoded.", rel)
	}
	return mdParseYAML(b, rel)
}

func mdParseYAML(b []byte, rel string) (*mdYAML, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		msg := strings.TrimPrefix(err.Error(), "yaml: ")
		hint := ""
		if strings.Contains(msg, "found character that cannot start any token") && bytes.Contains(b, []byte("\t")) {
			hint = " Hint: YAML needs spaces, not tabs."
		} else if strings.Contains(msg, "did not find expected alphabetic or numeric character") || strings.Contains(msg, "unknown anchor") {
			hint = ` Hint: put a pattern that starts with * in quotes, for example "*.log".`
		} else if strings.Contains(msg, "mapping values are not allowed in this context") {
			hint = " Hint: write server:/path without a space after the colon, and quote text that contains ': '."
		}
		return nil, mdErr("%s: invalid YAML: %s.%s", rel, msg, hint)
	}
	if n.Kind == 0 {
		return &mdYAML{kind: "null"}, nil
	}
	v, err := mdFromNode(&n, 0)
	if err != nil {
		return nil, mdErr("%s: %v.", rel, err)
	}
	return v, nil
}

func mdReadMapping(file, rel string) (*mdYAML, error) {
	v, err := mdReadYAML(file, rel)
	if err != nil {
		return nil, err
	}
	if v.kind == "null" {
		return nil, mdErr("%s: the file is empty.", rel)
	}
	if v.kind != "map" {
		return nil, mdErr("%s: the content must be a mapping (key: value), not %s.", rel, v.typeName())
	}
	return v, nil
}

func mdTopLevel(file, rel, key, shape string) (*mdYAML, *mdYAML, error) {
	data, err := mdReadMapping(file, rel)
	if err != nil {
		return nil, nil, err
	}
	for _, k := range data.keys {
		if k != key {
			return nil, nil, mdErr("%s: %s: unknown key; the file has only the key %s.", rel, k, key)
		}
	}
	v, ok := data.get(key)
	if !ok {
		return nil, nil, mdErr("%s: %s: required key is missing.", rel, key)
	}
	if v.kind != "map" {
		return data, nil, mdErr("%s: %s: must be a mapping (%s).", rel, key, shape)
	}
	return data, v, nil
}

// ─── field checks ───

type mdFields struct {
	rel, prefix string
	v           *mdYAML
}

func (f mdFields) key(k string) string {
	if f.prefix == "" {
		return k
	}
	return f.prefix + "." + k
}

// unknown reports the first key that is not allowed (the original's extra="forbid").
func (f mdFields) unknown(allowed ...string) error {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for _, k := range f.v.keys {
		if !ok[k] {
			shown := []string{}
			for _, a := range allowed {
				if a != "name" {
					shown = append(shown, a)
				}
			}
			return mdErr("%s: %s: unknown key (allowed: %s).", f.rel, f.key(k), strings.Join(shown, ", "))
		}
	}
	return nil
}

func (f mdFields) bad(k, what string, v *mdYAML) error {
	return mdErr("%s: %s: %s (got: %s).", f.rel, f.key(k), what, v.repr())
}

func (f mdFields) str(k string, required bool) (string, bool, error) {
	v, ok := f.v.get(k)
	if !ok {
		if required {
			return "", false, mdErr("%s: %s: required key is missing.", f.rel, f.key(k))
		}
		return "", false, nil
	}
	if v.kind != "str" {
		return "", true, f.bad(k, "must be text", v)
	}
	return v.str, true, nil
}

// optStr is "str | None": nil for a missing key or null.
func (f mdFields) optStr(k string) (*string, bool, error) {
	v, ok := f.v.get(k)
	if !ok || v.kind == "null" {
		return nil, ok, nil
	}
	if v.kind != "str" {
		return nil, true, f.bad(k, "must be text", v)
	}
	s := v.str
	return &s, true, nil
}

func (f mdFields) strList(k string) ([]string, bool, error) {
	v, ok := f.v.get(k)
	if !ok {
		return nil, false, nil
	}
	if v.kind != "seq" {
		return nil, true, f.bad(k, "must be a list", v)
	}
	out := []string{}
	for i, it := range v.items {
		if it.kind != "str" {
			return nil, true, mdErr("%s: %s[%d]: must be text (got: %s).", f.rel, f.key(k), i, it.repr())
		}
		out = append(out, it.str)
	}
	return out, true, nil
}

func (f mdFields) boolean(k string) (bool, bool, error) {
	v, ok := f.v.get(k)
	if !ok {
		return false, false, nil
	}
	switch v.kind {
	case "bool":
		return v.b, true, nil
	case "int":
		if v.i == 0 || v.i == 1 {
			return v.i == 1, true, nil
		}
	case "str":
		switch strings.ToLower(v.str) {
		case "t", "y", "on", "yes", "true", "1":
			return true, true, nil
		case "f", "n", "no", "off", "false", "0":
			return false, true, nil
		}
	}
	return false, true, f.bad(k, "must be true or false", v)
}

func (f mdFields) integer(k string, def int) (int, error) {
	v, ok := f.v.get(k)
	if !ok {
		return def, nil
	}
	switch v.kind {
	case "int":
		return int(v.i), nil
	case "float":
		if v.f == float64(int64(v.f)) {
			return int(v.f), nil
		}
	case "str":
		if n, err := strconv.Atoi(strings.TrimSpace(v.str)); err == nil {
			return n, nil
		}
	}
	return 0, f.bad(k, "must be a whole number", v)
}

func mdCheckName(name, rel, key, what string) error {
	if !mdNameRe.MatchString(name) || strings.HasSuffix(name, "\n") {
		return mdErr("%s: %s: invalid %s name %s; use lower-case letters, digits, '.', '_' and '-', at most 63 characters, starting with a letter or digit.", rel, key, what, pyRepr(name))
	}
	return nil
}

func mdCheckFilledName(v *mdYAML, name, rel, key string) error {
	if n, ok := v.get("name"); ok && !(n.kind == "str" && n.str == name) {
		return mdErr("%s: %s.name: the name %s differs from the key %s; leave it out.", rel, key, n.repr(), pyRepr(name))
	}
	return nil
}

func mdCheckLines(values []string, rel, key, what string) error {
	for i, v := range values {
		if strings.TrimSpace(v) == "" {
			return mdErr("%s: %s[%d]: empty %s.", rel, key, i, what)
		}
		if strings.ContainsAny(v, "\r\n") {
			return mdErr("%s: %s[%d]: %s %s must be on one line.", rel, key, i, what, pyRepr(v))
		}
		if mdControlRe.MatchString(v) {
			return mdErr("%s: %s[%d]: %s %s contains control characters.", rel, key, i, what, pyRepr(v))
		}
	}
	return nil
}

// mdCheckIgnore rejects patterns rsync would read as rules: "+ x" includes, "- x" excludes
// and "!" clears every exclude.
func mdCheckIgnore(values []string, rel, key string) error {
	if err := mdCheckLines(values, rel, key, "pattern"); err != nil {
		return err
	}
	for i, v := range values {
		if strings.HasPrefix(v, "+ ") || strings.HasPrefix(v, "- ") || v == "!" {
			return mdErr("%s: %s[%d]: rsync would read the pattern %s as a rule ('+ ' include, '- ' exclude, '!' clear); give only the pattern.", rel, key, i, pyRepr(v))
		}
	}
	return nil
}

func mdCheckAllowedRefs(values []string, rel, key string) error {
	if values == nil {
		return nil
	}
	if len(values) == 0 {
		return mdErr("%s: %s: an empty list would forbid every deployment; leave the key out when there is no limit.", rel, key)
	}
	if err := mdCheckLines(values, rel, key, "pattern"); err != nil {
		return err
	}
	for i, p := range values {
		if strings.HasPrefix(p, "refs/") && !strings.HasPrefix(p, "refs/heads/") && !strings.HasPrefix(p, "refs/tags/") {
			return mdErr("%s: %s[%d]: %s – a full name must start with refs/heads/ (branches) or refs/tags/ (tags).", rel, key, i, pyRepr(p))
		}
	}
	return nil
}

var mdDriveRe = regexp.MustCompile(`^[A-Za-z]:`)

func mdCheckAbsPath(value, rel, key string) error {
	problem := ""
	switch {
	case value != strings.TrimSpace(value):
		problem = "a space at the start or end of the path"
	case strings.Contains(value, `\`):
		problem = `use '/' instead of '\' (a path on a Linux server)`
	case !strings.HasPrefix(value, "/"):
		problem = "must be an absolute path on the server (starts with /)"
	case mdControlRe.MatchString(value):
		problem = "contains control characters (for example a new line)"
	case mdHasDotDot(value):
		problem = "must not contain '..'"
	}
	if problem != "" {
		return mdErr("%s: %s: %s – %s.", rel, key, pyRepr(value), problem)
	}
	return nil
}

func mdHasDotDot(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func mdCheckKey(value, rel, key string) error {
	if strings.Contains(value, "PRIVATE KEY") || strings.Contains(value, "-----BEGIN") || len(value) > 512 || mdControlRe.MatchString(value) {
		return mdErr("%s: %s: key takes the path of a private key file (for example id_ed25519), not the key itself; the value is not shown.", rel, key)
	}
	problem := ""
	switch {
	case strings.TrimSpace(value) == "":
		problem = "empty path; leave key out to use the agent or ~/.ssh/config"
	case value != strings.TrimSpace(value):
		problem = "a space at the start or end of the path"
	case strings.Contains(value, `\`) || mdDriveRe.MatchString(value):
		problem = "a Windows path; give the file name of a key in ~/.ssh"
	case strings.HasPrefix(value, "-"):
		problem = "must not start with '-'"
	case strings.HasPrefix(value, "~") && !strings.HasPrefix(value, "~/"):
		problem = "'~' is allowed only as '~/'"
	case strings.Contains(value, "/") && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "~/"):
		problem = "a relative path; give only the file name (a key in ~/.ssh), ~/... or an absolute path"
	case strings.HasSuffix(value, "/"):
		problem = "a directory; give the private key file"
	case strings.HasSuffix(value, ".pub"):
		problem = "that is the public key; give the private one (" + strings.TrimSuffix(value, ".pub") + ")"
	case strings.HasSuffix(strings.ToLower(value), ".ppk"):
		problem = "a PuTTY key, which OpenSSH cannot read; export it as an OpenSSH key"
	}
	if problem != "" {
		return mdErr("%s: %s: %s – %s.", rel, key, pyRepr(value), problem)
	}
	return nil
}

// ─── loading ───

// mdLoadConfig reads and validates servers.yaml, environments.yaml and projects/*.yaml.
func mdLoadConfig(dir string) (*mdConfig, error) {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil, mdErr("The configuration directory %s does not exist. Import a deploy-config ZIP or set the configuration repository.", dir)
	}
	entries, _ := os.ReadDir(dir)
	visible := false
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			visible = true
			break
		}
	}
	if !visible {
		return nil, mdErr("The configuration directory %s is empty. Import a deploy-config ZIP or set the configuration repository.", dir)
	}
	cfg := &mdConfig{}
	if cfg.Servers, err = mdLoadServers(dir); err != nil {
		return nil, err
	}
	if cfg.Envs, err = mdLoadEnvironments(dir, cfg.Servers); err != nil {
		return nil, err
	}
	var placed []mdPlaced
	if cfg.Projects, err = mdLoadProjects(dir, cfg, &placed); err != nil {
		return nil, err
	}
	if err := mdCheckDestinations(placed, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func mdMissingFile(dir, name string) string {
	yml := strings.TrimSuffix(name, ".yaml") + ".yml"
	for _, other := range []string{yml, name + ".txt", yml + ".txt"} {
		if _, err := os.Stat(filepath.Join(dir, other)); err == nil {
			return fmt.Sprintf("%s: the file does not exist (there is %s – rename it to %s).", name, other, name)
		}
	}
	return fmt.Sprintf("%s: the file does not exist.", name)
}

func mdLoadServers(dir string) ([]*mdServer, error) {
	rel := mdServersFile
	file := filepath.Join(dir, rel)
	if st, err := os.Stat(file); err != nil || st.IsDir() {
		if err != nil {
			if nested := mdNestedExample(dir); nested != "" {
				return nil, mdErr("%s: the file is not in the configuration directory but in its subdirectory %s/; move the contents of %s/ one level up.", rel, nested, nested)
			}
		}
		return nil, mdErr("%s", mdMissingFile(dir, rel))
	}
	_, entries, err := mdTopLevel(file, rel, "servers", "name: {host, user, port, key}")
	if err != nil {
		return nil, err
	}
	var out []*mdServer
	for i, name := range entries.keys {
		entry := entries.vals[i]
		key := "servers." + name
		if err := mdCheckName(name, rel, key, "server"); err != nil {
			return nil, err
		}
		if entry.kind != "map" {
			return nil, mdErr("%s: %s: must be a mapping with the keys host, user and port.", rel, key)
		}
		if err := mdCheckFilledName(entry, name, rel, key); err != nil {
			return nil, err
		}
		if entry.has("password") {
			return nil, mdErr("%s: %s.password: password login is not supported; the tool logs in with keys only.", rel, key)
		}
		f := mdFields{rel, key, entry}
		if err := f.unknown("name", "host", "user", "port", "key"); err != nil {
			return nil, err
		}
		s := &mdServer{Name: name}
		if s.Host, _, err = f.str("host", true); err != nil {
			return nil, err
		}
		if s.User, _, err = f.str("user", true); err != nil {
			return nil, err
		}
		if s.Port, err = f.integer("port", 22); err != nil {
			return nil, err
		}
		k, _, err := f.optStr("key")
		if err != nil {
			return nil, err
		}
		if !mdHostRe.MatchString(s.Host) {
			return nil, mdErr("%s: %s.host: invalid address %s; expected a host name or an IPv4 address (no spaces, not starting with '-').", rel, key, pyRepr(s.Host))
		}
		if !mdUserRe.MatchString(s.User) {
			return nil, mdErr("%s: %s.user: invalid user %s; letters, digits, '.', '_' and '-' are allowed (not starting with '-').", rel, key, pyRepr(s.User))
		}
		if s.Port < 1 || s.Port > 65535 {
			return nil, mdErr("%s: %s.port: the port must be between 1 and 65535 (got: %d).", rel, key, s.Port)
		}
		if k != nil {
			if err := mdCheckKey(*k, rel, key+".key"); err != nil {
				return nil, err
			}
			s.Key = *k
		}
		out = append(out, s)
	}
	return out, nil
}

func mdNestedExample(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.IsDir() {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, e.Name(), mdServersFile)); err == nil && !st.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func mdLoadEnvironments(dir string, servers []*mdServer) ([]*mdEnvironment, error) {
	rel := mdEnvsFile
	file := filepath.Join(dir, rel)
	if st, err := os.Stat(file); err != nil || st.IsDir() {
		return nil, mdErr("%s Environments (their UI order and the confirm, allowed_refs and servers rules) are listed in %s.", mdMissingFile(dir, rel), rel)
	}
	data, err := mdReadMapping(file, rel)
	if err != nil {
		return nil, err
	}
	if v, ok := data.get("environments"); ok && v.kind == "seq" {
		return nil, mdErr("%s: environments: must be a mapping (environment name: rules), not a list; an environment without rules is written as 'dev: {}'.", rel)
	}
	_, entries, err := mdTopLevel(file, rel, "environments", "environment name: {confirm, allowed_refs, servers}")
	if err != nil {
		return nil, err
	}
	if len(entries.keys) == 0 {
		return nil, mdErr("%s: environments: there is no environment (for example dev: {}).", rel)
	}
	var out []*mdEnvironment
	for i, name := range entries.keys {
		raw := entries.vals[i]
		key := "environments." + name
		if err := mdCheckName(name, rel, key, "environment"); err != nil {
			return nil, err
		}
		if raw.kind == "null" {
			raw = &mdYAML{kind: "map"}
		}
		if raw.kind != "map" {
			return nil, mdErr("%s: %s: must be a mapping with the rules confirm, allowed_refs and servers, or {} for an environment without rules (got: %s).", rel, key, raw.repr())
		}
		if err := mdCheckFilledName(raw, name, rel, key); err != nil {
			return nil, err
		}
		f := mdFields{rel, key, raw}
		if err := f.unknown("name", "confirm", "allowed_refs", "servers"); err != nil {
			return nil, err
		}
		e := &mdEnvironment{Name: name}
		if e.Confirm, _, err = f.boolean("confirm"); err != nil {
			return nil, err
		}
		if v, ok := raw.get("servers"); ok && v.kind != "null" {
			if e.Servers, _, err = f.strList("servers"); err != nil {
				return nil, err
			}
		}
		if v, ok := raw.get("allowed_refs"); ok && v.kind != "null" {
			if e.AllowedRefs, _, err = f.strList("allowed_refs"); err != nil {
				return nil, err
			}
		}
		if err := mdCheckAllowedRefs(e.AllowedRefs, rel, key+".allowed_refs"); err != nil {
			return nil, err
		}
		if err := mdCheckEnvironmentServers(e.Servers, servers, rel, key+".servers"); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// mdCheckEnvironmentServers mirrors the deploy-tool's environment restriction: no servers key
// (nil) means every configured server; a listed server must exist exactly once, and an empty
// list is rejected because it would block every deploy.
func mdCheckEnvironmentServers(names []string, servers []*mdServer, rel, key string) error {
	if names == nil {
		return nil
	}
	if len(names) == 0 {
		return mdErr("%s: %s: an empty list would forbid every server; leave the key out to allow all servers.", rel, key)
	}
	known := make(map[string]bool, len(servers))
	knownNames := make([]string, 0, len(servers))
	for _, s := range servers {
		known[s.Name] = true
		knownNames = append(knownNames, s.Name)
	}
	if len(knownNames) == 0 {
		knownNames = []string{"-"}
	}
	seen := make(map[string]bool, len(names))
	for i, name := range names {
		itemKey := fmt.Sprintf("%s[%d]", key, i)
		if err := mdCheckName(name, rel, itemKey, "server"); err != nil {
			return err
		}
		if !known[name] {
			return mdErr("%s: %s: server %s is not defined in %s (there are: %s).", rel, itemKey, pyRepr(name), mdServersFile, strings.Join(knownNames, ", "))
		}
		if seen[name] {
			return mdErr("%s: %s: server %s is listed more than once.", rel, itemKey, pyRepr(name))
		}
		seen[name] = true
	}
	return nil
}

// mdPlaced is a destination with its place in the configuration, for the global checks.
type mdPlaced struct {
	rel, key, env string
	dest          *mdDestination
	ignore        []string // ignore_for(environment, server): what deploying it leaves alone
}

func mdLoadProjects(dir string, cfg *mdConfig, placed *[]mdPlaced) ([]*mdProject, error) {
	pdir := filepath.Join(dir, mdProjectsDir)
	st, err := os.Stat(pdir)
	if err != nil {
		return nil, nil
	}
	if !st.IsDir() {
		return nil, mdErr("%s: must be a directory of <project>.yaml files.", mdProjectsDir)
	}
	entries, err := os.ReadDir(pdir)
	if err != nil {
		return nil, mdErr("%s: cannot read the directory (%v).", mdProjectsDir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []*mdProject
	for _, e := range entries {
		name := e.Name()
		file := filepath.Join(pdir, name)
		if fi, err := os.Stat(file); strings.HasPrefix(name, ".") || err != nil || !fi.Mode().IsRegular() {
			continue
		}
		rel := mdProjectsDir + "/" + name
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".yaml.txt") || strings.HasSuffix(lower, ".yml.txt") {
			return nil, mdErr("%s: rename it to %s; with the .txt extension the project would not be loaded.", rel, name[:len(name)-4])
		}
		ext := filepath.Ext(name)
		if strings.ToLower(ext) != ".yaml" && strings.ToLower(ext) != ".yml" {
			continue
		}
		if ext != ".yaml" {
			return nil, mdErr("%s: use the .yaml extension (such a file would not be loaded).", rel)
		}
		stem := strings.TrimSuffix(name, ext)
		if err := mdCheckName(stem, rel, "file name", "project"); err != nil {
			return nil, err
		}
		p, err := mdLoadProject(file, rel, stem, cfg, placed)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func mdLoadProject(file, rel, stem string, cfg *mdConfig, placed *[]mdPlaced) (*mdProject, error) {
	data, err := mdReadMapping(file, rel)
	if err != nil {
		return nil, err
	}
	p := &mdProject{SourceDir: "."}
	var own []mdPlaced
	targets, hasTargets := data.get("targets")
	if hasTargets && targets.kind == "map" {
		for i, env := range targets.keys {
			key := "targets." + env
			if err := mdCheckName(env, rel, key, "environment"); err != nil {
				return nil, err
			}
			e := cfg.env(env)
			if e == nil {
				known := []string{}
				for _, x := range cfg.Envs {
					known = append(known, x.Name)
				}
				return nil, mdErr("%s: %s: the environment %s is not listed in %s (there are: %s); add it there or fix the name.", rel, key, pyRepr(env), mdEnvsFile, strings.Join(known, ", "))
			}
			t, err := mdLoadTarget(targets.vals[i], e, cfg, rel, key, &own)
			if err != nil {
				return nil, err
			}
			p.Targets = append(p.Targets, t)
		}
	}
	f := mdFields{rel, "", data}
	if err := f.unknown("name", "repo", "source_dir", "build", "ignore", "targets"); err != nil {
		return nil, err
	}
	if p.Name, _, err = f.str("name", true); err != nil {
		return nil, err
	}
	if p.Repo, _, err = f.str("repo", true); err != nil {
		return nil, err
	}
	if sd, ok, err := f.str("source_dir", false); err != nil {
		return nil, err
	} else if ok {
		p.SourceDir = sd
	}
	if p.Build, _, err = f.optStr("build"); err != nil {
		return nil, err
	}
	if ig, _, err := f.strList("ignore"); err != nil {
		return nil, err
	} else {
		p.Ignore = append([]string{}, ig...)
	}
	if !hasTargets {
		return nil, mdErr("%s: targets: required key is missing.", rel)
	}
	if targets.kind != "map" {
		return nil, f.bad("targets", "must be a mapping (key: value)", targets)
	}
	if p.Name != stem {
		return nil, mdErr("%s: name: the project name %s must equal the file name (%s).", rel, pyRepr(p.Name), stem)
	}
	switch {
	case strings.TrimSpace(p.Repo) == "":
		return nil, mdErr("%s: repo: the repository address is empty.", rel)
	case strings.HasPrefix(p.Repo, "-"):
		return nil, mdErr("%s: repo: the repository address must not start with '-'.", rel)
	case mdControlRe.MatchString(p.Repo):
		return nil, mdErr("%s: repo: the address contains control characters (for example a new line).", rel)
	}
	problem := ""
	switch sd := p.SourceDir; {
	case strings.TrimSpace(sd) == "":
		problem = "empty; use '.' for the whole branch export"
	case strings.Contains(sd, `\`):
		problem = `use '/' instead of '\'`
	case strings.HasPrefix(sd, "/") || mdDriveRe.MatchString(sd):
		problem = "must be relative to the branch export (for example dist)"
	case mdHasDotDot(sd):
		problem = "must not leave the branch export ('..')"
	case mdControlRe.MatchString(sd):
		problem = "contains control characters (for example a new line)"
	}
	if problem != "" {
		return nil, mdErr("%s: source_dir: %s – %s.", rel, pyRepr(p.SourceDir), problem)
	}
	if err := mdCheckIgnore(p.Ignore, rel, "ignore"); err != nil {
		return nil, err
	}
	for _, o := range own {
		o.ignore = p.ignoreFor(o.env, o.dest.Server)
		*placed = append(*placed, o)
	}
	return p, nil
}

var mdTargetSettings = []string{"ignore_extra", "post_deploy", "state_dir", "allowed_refs", "confirm"}

func mdIsDestinationKey(k string) bool {
	switch k {
	case "server", "servers", "path", "destinations", "ignore_extra", "post_deploy", "state_dir", "allowed_refs", "confirm":
		return true
	}
	return false
}

// mdSpacedDestination explains "server: /path" (a space after the colon makes it a mapping).
func mdSpacedDestination(raw *mdYAML, cfg *mdConfig) string {
	if len(raw.keys) != 1 {
		return ""
	}
	name, value := raw.keys[0], raw.vals[0]
	if raw.nkey || value.kind != "str" || !strings.HasPrefix(value.str, "/") {
		return ""
	}
	if cfg.server(name) == nil && (mdIsDestinationKey(name) || !mdNameRe.MatchString(name)) {
		return ""
	}
	return fmt.Sprintf("write %s:%s without a space after the colon (YAML reads a space after ':' as the mapping {%s: %s}, not a destination).", name, value.str, name, value.str)
}

func mdLoadTarget(raw *mdYAML, env *mdEnvironment, cfg *mdConfig, rel, key string, placed *[]mdPlaced) (*mdTarget, error) {
	var items []*mdYAML
	var itemKeys []string
	settings := &mdYAML{kind: "map"}
	switch raw.kind {
	case "str":
		items, itemKeys = []*mdYAML{raw}, []string{key}
	case "seq":
		if len(raw.items) == 0 {
			return nil, mdErr("%s: %s: the destination list is empty.", rel, key)
		}
		items = raw.items
		for i := range raw.items {
			itemKeys = append(itemKeys, fmt.Sprintf("%s[%d]", key, i))
		}
	case "map":
		dests, ok := raw.get("destinations")
		if !ok {
			if s := mdSpacedDestination(raw, cfg); s != "" {
				return nil, mdErr("%s: %s: %s", rel, key, s)
			}
			if raw.has("servers") {
				return nil, mdErr("%s: %s: the old environment form (servers and path) is not supported any more; write destinations as server:/path.", rel, key)
			}
			if raw.has("server") || raw.has("path") {
				return nil, mdErr("%s: %s: a single destination written as a mapping goes in a list: %s: [{server: …, path: …}], or briefly %s: server:/path.", rel, key, env.Name, env.Name)
			}
			return nil, mdErr("%s: %s.destinations: required key is missing (a list of server:/path destinations); the short form is %s: server:/path.", rel, key, env.Name)
		}
		if err := mdCheckFilledName(raw, env.Name, rel, key); err != nil {
			return nil, err
		}
		for i, k := range raw.keys {
			if k != "destinations" && k != "name" {
				settings.keys = append(settings.keys, k)
				settings.vals = append(settings.vals, raw.vals[i])
			}
		}
		destKey := key + ".destinations"
		switch dests.kind {
		case "str":
			items, itemKeys = []*mdYAML{dests}, []string{destKey}
		case "seq":
			if len(dests.items) == 0 {
				return nil, mdErr("%s: %s: the destination list is empty.", rel, destKey)
			}
			items = dests.items
			for i := range dests.items {
				itemKeys = append(itemKeys, fmt.Sprintf("%s[%d]", destKey, i))
			}
		default:
			return nil, mdErr("%s: %s: must be a list of server:/path destinations (got: %s).", rel, destKey, dests.repr())
		}
	default:
		return nil, mdErr("%s: %s: expected a server:/path destination, a list of destinations or a mapping with destinations (got: %s).", rel, key, raw.repr())
	}

	t := &mdTarget{Name: env.Name}
	for i, item := range items {
		d, err := mdLoadDestination(item, cfg, rel, itemKeys[i])
		if err != nil {
			return nil, err
		}
		if env.Servers != nil {
			allowed := false
			for _, name := range env.Servers {
				if name == d.Server {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, mdErr("%s: %s: server %s is not allowed in environment %s; allowed servers: %s.", rel, itemKeys[i], pyRepr(d.Server), pyRepr(env.Name), strings.Join(env.Servers, ", "))
			}
		}
		t.Destinations = append(t.Destinations, d)
	}
	f := mdFields{rel, key, settings}
	if err := f.unknown(append([]string{"name", "destinations"}, mdTargetSettings...)...); err != nil {
		return nil, err
	}
	var err error
	if t.IgnoreExtra, _, err = f.strList("ignore_extra"); err != nil {
		return nil, err
	}
	if t.IgnoreExtra == nil {
		t.IgnoreExtra = []string{}
	}
	if t.PostDeploy, _, err = f.optStr("post_deploy"); err != nil {
		return nil, err
	}
	sd, _, err := f.optStr("state_dir")
	if err != nil {
		return nil, err
	}
	if sd != nil {
		t.StateDir = *sd
	}
	refsSet := false
	if v, ok := settings.get("allowed_refs"); ok {
		refsSet = true
		if v.kind != "null" {
			if t.AllowedRefs, _, err = f.strList("allowed_refs"); err != nil {
				return nil, err
			}
		}
	} else {
		t.AllowedRefs = env.AllowedRefs
	}
	if c, ok, err := f.boolean("confirm"); err != nil {
		return nil, err
	} else if ok {
		t.Confirm = c
	} else {
		t.Confirm = env.Confirm
	}
	// inheritance: state_dir for destinations without one, post_deploy where the key is absent
	for _, d := range t.Destinations {
		if t.StateDir != "" && d.StateDir == "" {
			d.StateDir = t.StateDir
		}
		if !d.postSet {
			d.PostDeploy = t.PostDeploy
		}
	}
	for i, d := range t.Destinations {
		for _, prev := range t.Destinations[:i] {
			if prev.Server == d.Server {
				return nil, mdErr("%s: %s: the server %s is listed more than once; an environment has at most one destination per server.", rel, itemKeys[i], pyRepr(d.Server))
			}
		}
	}
	if sd != nil {
		if err := mdCheckAbsPath(t.StateDir, rel, key+".state_dir"); err != nil {
			return nil, err
		}
	}
	if err := mdCheckIgnore(t.IgnoreExtra, rel, key+".ignore_extra"); err != nil {
		return nil, err
	}
	if refsSet {
		if err := mdCheckAllowedRefs(t.AllowedRefs, rel, key+".allowed_refs"); err != nil {
			return nil, err
		}
	}
	for i, d := range t.Destinations {
		app, state := d.appDir(), d.statePath()
		if mdWithin(state, app) || mdWithin(app, state) {
			return nil, mdErr("%s: %s: the state directory %s overlaps the application directory %s (%s); a backup must not be in the web root. Set state_dir outside the application directory.", rel, itemKeys[i], state, app, d.label())
		}
		*placed = append(*placed, mdPlaced{rel: rel, key: itemKeys[i], env: env.Name, dest: d})
	}
	return t, nil
}

func mdLoadDestination(item *mdYAML, cfg *mdConfig, rel, key string) (*mdDestination, error) {
	d := &mdDestination{IgnoreExtra: []string{}}
	serverKey, pathKey := key, key
	switch item.kind {
	case "str":
		s, p, err := mdParseDestination(item.str)
		if err != nil {
			return nil, mdErr("%s: %s: %v.", rel, key, err)
		}
		d.Server, d.Path = s, p
	case "map":
		if s := mdSpacedDestination(item, cfg); s != "" {
			return nil, mdErr("%s: %s: %s", rel, key, s)
		}
		f := mdFields{rel, key, item}
		if err := f.unknown("server", "path", "state_dir", "ignore_extra", "post_deploy"); err != nil {
			return nil, err
		}
		var err error
		if d.Server, _, err = f.str("server", true); err != nil {
			return nil, err
		}
		if d.Path, _, err = f.str("path", true); err != nil {
			return nil, err
		}
		sd, _, err := f.optStr("state_dir")
		if err != nil {
			return nil, err
		}
		if sd != nil {
			d.StateDir = *sd
		}
		if ig, _, err := f.strList("ignore_extra"); err != nil {
			return nil, err
		} else if ig != nil {
			d.IgnoreExtra = ig
		}
		if d.PostDeploy, d.postSet, err = f.optStr("post_deploy"); err != nil {
			return nil, err
		}
		serverKey, pathKey = key+".server", key+".path"
	default:
		return nil, mdErr("%s: %s: a destination must be server:/path or a mapping {server, path, state_dir, ignore_extra, post_deploy} (got: %s).", rel, key, item.repr())
	}
	if err := mdCheckName(d.Server, rel, serverKey, "server"); err != nil {
		return nil, err
	}
	if cfg.server(d.Server) == nil {
		known := []string{}
		for _, s := range cfg.Servers {
			known = append(known, s.Name)
		}
		if len(known) == 0 {
			known = []string{"-"}
		}
		return nil, mdErr("%s: %s: the server %s is not defined in %s (there are: %s).", rel, serverKey, pyRepr(d.Server), mdServersFile, strings.Join(known, ", "))
	}
	if err := mdCheckAbsPath(d.Path, rel, pathKey); err != nil {
		return nil, err
	}
	if d.appDir() == "/" {
		return nil, mdErr("%s: %s: the application path must not be the root '/'.", rel, pathKey)
	}
	if d.StateDir != "" {
		if err := mdCheckAbsPath(d.StateDir, rel, key+".state_dir"); err != nil {
			return nil, err
		}
	}
	if err := mdCheckIgnore(d.IgnoreExtra, rel, key+".ignore_extra"); err != nil {
		return nil, err
	}
	return d, nil
}

// mdCheckDestinations runs the checks across the whole configuration, per physical server
// (host and port): no two destinations share a path or a state directory, one application
// is not inside another unless the outer one's ignore covers it, and no state directory is
// inside another destination's application.
func mdCheckDestinations(placed []mdPlaced, cfg *mdConfig) error {
	machine := func(d *mdDestination) string {
		s := cfg.server(d.Server)
		return strings.ToLower(s.Host) + ":" + strconv.Itoa(s.Port)
	}
	for i, first := range placed {
		for _, second := range placed[i+1:] {
			a, b := first.dest, second.dest
			if machine(a) != machine(b) {
				continue
			}
			where := second.rel + ": " + second.key
			other := fmt.Sprintf("%s (%s)", first.rel, first.key)
			if a.appDir() == b.appDir() {
				return mdErr("%s: the destination %s is already listed in %s; two destinations must not have the same server and path.", where, b.label(), other)
			}
			for _, pair := range [][2]mdPlaced{{first, second}, {second, first}} {
				if err := mdCheckNested(pair[0], pair[1]); err != nil {
					return err
				}
			}
			if a.statePath() == b.statePath() {
				return mdErr("%s: the destinations %s and %s share the state directory %s (current.json, lock, backups; the other one is in %s); set a different state_dir.", where, a.label(), b.label(), b.statePath(), other)
			}
			for _, pair := range [][2]*mdDestination{{a, b}, {b, a}} {
				dest, foreign := pair[0], pair[1]
				if mdWithin(dest.statePath(), foreign.appDir()) {
					return mdErr("%s: the state directory %s of %s is inside the application directory of %s (the other destination is in %s); a backup must not be in a web root. Set state_dir for %s outside %s.", where, dest.statePath(), dest.label(), foreign.label(), other, dest.label(), foreign.appDir())
				}
			}
		}
	}
	return nil
}

func mdCheckNested(outer, inner mdPlaced) error {
	out, inn := outer.dest, inner.dest
	if out.appDir() == inn.appDir() || !mdWithin(inn.appDir(), out.appDir()) {
		return nil
	}
	rel := strings.TrimPrefix(inn.appDir(), strings.TrimRight(out.appDir(), "/")+"/")
	if mdIsIgnored(rel, outer.ignore) {
		return nil
	}
	return mdErr("%s: %s: the destination %s is inside the application directory of %s (the outer destination is in %s (%s)); deploying %s with --delete would delete the whole application in /%s. Add /%s to the ignore of the outer project or move the application out of that directory.", inner.rel, inner.key, inn.label(), out.label(), outer.rel, outer.key, out.label(), rel, rel)
}
