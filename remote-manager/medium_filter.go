// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"strings"
	"sync"
)

// ─── MEDIUM: rsync exclude rules ───
//
// The original tool compares and deploys with
//
//	rsync -rlc --delete --exclude=<pattern>...
//
// so the native engine has to decide exactly as rsync does which paths a pattern excludes.
// mdRule follows rsync's exclude.c and wildmatch.c:
//   - a trailing "/" matches directories only (a symlink is not a directory with -l);
//   - a leading "/" anchors the pattern to the transfer root (the destination path);
//   - a pattern without "/" and without "**" matches the last path component only;
//   - an unanchored pattern with an inner "/" and no "**" matches the last N+1 components
//     (N = number of slashes), one with "**" (not leading) is tried after every slash;
//   - "*" and "?" do not match "/", "**" matches anything, "[...]" is a character class,
//     and a backslash escapes the next character only when the pattern has wildcards;
//   - a pattern starting with "**" may also match at the very start ("**/x" matches "x");
//   - "dir/***" matches the directory dir itself and everything below it.
// An excluded directory is skipped with everything inside it, on both sides: its contents
// are neither sent nor deleted.

type mdRule struct {
	raw       string
	dirOnly   bool
	anchored  bool
	slashes   int  // slashes of the pattern without the trailing one (leading one included)
	wild      bool // *, ? or [ present
	wild2     bool // ** present
	wild2Pref bool // starts with **
	pattern   string
	re        *regexp.Regexp
	alt       *mdRule // "dir/***": the rule for "dir" itself
	exact     bool    // a path the operator excluded from one deployment: it and everything below
}

// mdExactRule excludes one path (relative to the root, without a trailing slash) and
// everything below it, taken literally (never as a wildcard pattern).
func mdExactRule(rel string) *mdRule {
	return &mdRule{raw: rel, exact: true, pattern: strings.Trim(rel, "/")}
}

var mdRuleCache sync.Map

func mdCompileRule(raw string) *mdRule {
	if r, ok := mdRuleCache.Load(raw); ok {
		return r.(*mdRule)
	}
	r := &mdRule{raw: raw}
	p := raw
	if strings.HasSuffix(p, "/***") && len(p) > 4 {
		// "x/***" = "x/" (the directory itself) and "x/**"
		base := p[:len(p)-4]
		r.alt = mdCompileRule(base + "/")
		p = base + "/**"
	}
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
		r.dirOnly = true
	}
	r.slashes = strings.Count(p, "/")
	r.wild = strings.ContainsAny(p, "*?[")
	r.wild2 = strings.Contains(p, "**")
	if strings.HasPrefix(p, "/") {
		r.anchored = true
		p = p[1:]
	}
	r.wild2Pref = strings.HasPrefix(p, "**")
	r.pattern = p
	if r.wild {
		r.re = regexp.MustCompile("^" + mdWildRegexp(p) + "$")
	}
	mdRuleCache.Store(raw, r)
	return r
}

// mdWildRegexp translates an rsync wildcard pattern into a regular expression.
func mdWildRegexp(p string) string {
	var sb strings.Builder
	rs := []rune(p)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch c {
		case '\\':
			if i+1 < len(rs) {
				i++
				sb.WriteString(regexp.QuoteMeta(string(rs[i])))
			} else {
				sb.WriteString(`\\`)
			}
		case '*':
			if i+1 < len(rs) && rs[i+1] == '*' {
				for i+1 < len(rs) && rs[i+1] == '*' {
					i++
				}
				sb.WriteString(`.*`)
			} else {
				sb.WriteString(`[^/]*`)
			}
		case '?':
			sb.WriteString(`[^/]`)
		case '[':
			cls, n := mdWildClass(rs[i:])
			if n == 0 {
				sb.WriteString(`\[`)
				continue
			}
			sb.WriteString(cls)
			i += n - 1
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return sb.String()
}

// mdWildClass translates "[...]" (with "!" or "^" negation, ranges and [:class:]) and
// returns how many runes it used; 0 when the bracket is not closed (then it is literal).
var mdPosixClasses = map[string]bool{"alnum": true, "alpha": true, "blank": true, "cntrl": true, "digit": true, "graph": true, "lower": true, "print": true, "punct": true, "space": true, "upper": true, "xdigit": true}

func mdWildClass(rs []rune) (string, int) {
	i := 1
	neg := false
	if i < len(rs) && (rs[i] == '!' || rs[i] == '^') {
		neg = true
		i++
	}
	var sb strings.Builder
	first := true
	for i < len(rs) {
		c := rs[i]
		if c == ']' && !first {
			out := "["
			if neg {
				out += "^"
			}
			return out + sb.String() + "]", i + 1
		}
		first = false
		if c == '[' && i+1 < len(rs) && rs[i+1] == ':' {
			rest := string(rs[i+2:])
			if end := strings.Index(rest, ":]"); end >= 0 && mdPosixClasses[rest[:end]] {
				sb.WriteString("[:" + rest[:end] + ":]")
				i += 2 + len([]rune(rest[:end])) + 2
				continue
			}
		}
		if c == '\\' && i+1 < len(rs) {
			i++
			c = rs[i]
		}
		if c == '\\' || c == ']' || c == '[' || c == '^' {
			sb.WriteRune('\\')
		}
		sb.WriteRune(c)
		i++
	}
	return "", 0
}

// matches tells whether the rule excludes rel (relative to the transfer root, no leading
// or trailing slash); isDir is the type on the side being examined.
func (r *mdRule) matches(rel string, isDir bool) bool {
	if r.exact {
		return rel == r.pattern || strings.HasPrefix(rel, r.pattern+"/")
	}
	if r.alt != nil && r.alt.matches(rel, isDir) {
		return true
	}
	if r.dirOnly && !isDir {
		return false
	}
	name := rel
	if r.slashes == 0 && !r.wild2 {
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
	}
	if r.wild2Pref && !r.anchored {
		name = "/" + name // rsync lets "**/" match at the start of the path
	}
	match := func(s string) bool {
		if r.wild {
			return r.re.MatchString(s)
		}
		return s == r.pattern
	}
	switch {
	case !r.anchored && r.slashes > 0 && !r.wild2:
		// the last slashes+1 components
		parts := strings.Split(name, "/")
		if len(parts) < r.slashes+1 {
			return false
		}
		return match(strings.Join(parts[len(parts)-r.slashes-1:], "/"))
	case !r.anchored && !r.wild2Pref && r.wild2:
		// tried at the start and after every slash
		s := name
		for {
			if match(s) {
				return true
			}
			i := strings.Index(s, "/")
			if i < 0 {
				return false
			}
			s = s[i+1:]
		}
	}
	return match(name)
}

// mdExcluded tells whether any of the rules excludes rel.
func mdExcluded(rules []*mdRule, rel string, isDir bool) bool {
	for _, r := range rules {
		if r.matches(rel, isDir) {
			return true
		}
	}
	return false
}

func mdCompileRules(patterns []string) []*mdRule {
	out := make([]*mdRule, 0, len(patterns))
	for _, p := range patterns {
		if p == "" {
			continue
		}
		out = append(out, mdCompileRule(p))
	}
	return out
}

func mdHasWildcardDirPattern(pattern string) bool {
	stripped := strings.TrimRight(pattern, "/")
	if stripped == pattern || stripped == "" {
		return false
	}
	last := stripped[strings.LastIndex(stripped, "/")+1:]
	return strings.ContainsAny(last, "*?[")
}

// mdExcludeVariants is the original exclude_variants: "storage/" also as "storage", so a
// symlink on the server is protected like a directory; not for a wildcard last component.
func mdExcludeVariants(pattern string) []string {
	stripped := strings.TrimRight(pattern, "/")
	if stripped == pattern || stripped == "" || mdHasWildcardDirPattern(pattern) {
		return []string{pattern}
	}
	return []string{pattern, stripped}
}

// mdIsIgnored is the original tool's approximate is_ignored, used by its configuration
// check for nested applications: a pattern with a leading or inner "/" is anchored to the
// root (and covers everything below a matching prefix), any other matches a component.
func mdIsIgnored(rel string, patterns []string) bool {
	var parts []string
	for _, p := range strings.Split(rel, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	for _, raw := range patterns {
		pattern := strings.TrimRight(raw, "/")
		anchored := strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "/")
		pattern = strings.TrimLeft(pattern, "/")
		if pattern == "" {
			continue
		}
		if anchored {
			for i := range parts {
				if pyFnmatch(strings.Join(parts[:i+1], "/"), pattern) {
					return true
				}
			}
		} else {
			for _, part := range parts {
				if pyFnmatch(part, pattern) {
					return true
				}
			}
		}
	}
	return false
}

var pyFnmatchCache sync.Map

// pyFnmatch is Python's fnmatch.fnmatchcase: "*" matches anything ("/" included), "?" one
// character, "[seq]" and "[!seq]" a class; there is no backslash escape.
func pyFnmatch(name, pattern string) bool {
	if re, ok := pyFnmatchCache.Load(pattern); ok {
		return re.(*regexp.Regexp).MatchString(name)
	}
	var sb strings.Builder
	sb.WriteString(`(?s)^`)
	rs := []rune(pattern)
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; c {
		case '*':
			sb.WriteString(`.*`)
		case '?':
			sb.WriteString(`.`)
		case '[':
			j := i + 1
			if j < len(rs) && rs[j] == '!' {
				j++
			}
			if j < len(rs) && rs[j] == ']' {
				j++
			}
			for j < len(rs) && rs[j] != ']' {
				j++
			}
			if j >= len(rs) {
				sb.WriteString(`\[`)
				continue
			}
			body := rs[i+1 : j]
			var cls strings.Builder
			cls.WriteString("[")
			for k, r := range body {
				if k == 0 && r == '!' {
					cls.WriteString("^")
					continue
				}
				if r == '\\' || r == '[' || r == ']' || (r == '^' && k == 0) {
					cls.WriteRune('\\')
				}
				cls.WriteRune(r)
			}
			cls.WriteString("]")
			sb.WriteString(cls.String())
			i = j
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString(`$`)
	re, err := regexp.Compile(sb.String())
	if err != nil {
		re = regexp.MustCompile("^" + regexp.QuoteMeta(pattern) + "$")
	}
	pyFnmatchCache.Store(pattern, re)
	return re.MatchString(name)
}

// mdRefAllowed is the original ref_allowed: a full pattern (refs/heads/…, refs/tags/…)
// limits the kind too, a short one matches a branch and a tag of that name.
func mdRefAllowed(ref string, allowed []string) bool {
	if ref == "" {
		return false
	}
	if allowed == nil {
		return true
	}
	short := ref
	for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
		if strings.HasPrefix(ref, prefix) {
			short = ref[len(prefix):]
			break
		}
	}
	full := strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/tags/")
	for _, p := range allowed {
		if strings.HasPrefix(p, "refs/") {
			if full && pyFnmatch(ref, p) {
				return true
			}
		} else if pyFnmatch(short, p) {
			return true
		}
	}
	return false
}
