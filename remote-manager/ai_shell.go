// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path"
	"strings"
)

// ─── AI: SHELL COMMAND PARSING ───────────────────────
//
// The permission engine never trusts a command it cannot read. shParse splits a POSIX
// shell command line into simple commands (segments) joined by | && || ; & and newlines,
// removes quotes, collects redirections, and flags everything that makes the command
// opaque: command or process substitution, here-documents, subshells and groups,
// background jobs. The classifier treats an opaque command as "not read-only".

type shRedir struct {
	Op     string // "<" ">" ">>" ">|" ">&" "&>" "&>>" "<>" with an optional fd prefix ("2>")
	Target string
}

// writes reports whether the redirection writes to its target.
func (r shRedir) writes() bool {
	op := strings.TrimLeft(r.Op, "0123456789")
	if op == ">&" {
		// 2>&1 duplicates a descriptor; >&file writes a file
		return !isAllDigits(r.Target) && r.Target != "-"
	}
	return op != "<"
}

type shSegment struct {
	Words  []string
	Redirs []shRedir
	Op     string // operator after this segment: "|" "&&" "||" ";" "&" or ""
}

type shParsed struct {
	Segs       []shSegment
	Subst      bool // $( … ), `…`, <( … ), >( … ), $(( … ))
	Heredoc    bool
	Group      bool // ( … ) or { … }
	Background bool
	Vars       bool // $NAME / ${NAME} expansions
	Glob       bool // unquoted * ? [
	Err        string
}

// opaque reports whether the command hides what it runs.
func (p shParsed) opaque() bool {
	return p.Subst || p.Heredoc || p.Group || p.Background || p.Err != ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func shParse(cmd string) shParsed {
	var p shParsed
	var seg shSegment
	var word strings.Builder
	inWord, quotedWord := false, false
	pendingRedir := "" // operator waiting for its target
	rs := []rune(cmd)
	n := len(rs)

	endWord := func() {
		if !inWord {
			return
		}
		w := word.String()
		word.Reset()
		inWord, quotedWord = false, false
		if pendingRedir != "" {
			seg.Redirs = append(seg.Redirs, shRedir{Op: pendingRedir, Target: w})
			pendingRedir = ""
			return
		}
		seg.Words = append(seg.Words, w)
	}
	endSeg := func(op string) {
		endWord()
		if pendingRedir != "" {
			p.Err = "redirection without a target"
			pendingRedir = ""
		}
		seg.Op = op
		if len(seg.Words) > 0 || len(seg.Redirs) > 0 {
			p.Segs = append(p.Segs, seg)
		} else if op != ";" && op != "" {
			p.Err = "empty command before " + op
		}
		seg = shSegment{}
	}
	startRedir := func(op string) {
		// a word made only of digits right before > or < is the descriptor ("2>")
		if inWord && !quotedWord && isAllDigits(word.String()) {
			op = word.String() + op
			word.Reset()
			inWord = false
		} else {
			endWord()
		}
		if pendingRedir != "" {
			p.Err = "redirection without a target"
		}
		pendingRedir = op
	}
	dollar := func(i int, quoted bool) int {
		// rs[i] == '$'
		if i+1 < n {
			switch c := rs[i+1]; {
			case c == '(':
				p.Subst = true
			case c == '{' || c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '?' || c == '@' || c == '*' || c == '#' || c == '!' || c == '$' || c == '-':
				p.Vars = true
			}
		}
		word.WriteRune('$')
		inWord = true
		_ = quoted
		return i
	}

	for i := 0; i < n; i++ {
		c := rs[i]
		switch {
		case c == '\\':
			if i+1 < n {
				if rs[i+1] == '\n' { // line continuation
					i++
					continue
				}
				word.WriteRune(rs[i+1])
				inWord, quotedWord = true, true
				i++
			}
		case c == '\'':
			j := i + 1
			for j < n && rs[j] != '\'' {
				j++
			}
			if j >= n {
				p.Err = "unterminated quote"
				return p
			}
			word.WriteString(string(rs[i+1 : j]))
			inWord, quotedWord = true, true
			i = j
		case c == '"':
			j := i + 1
			for ; j < n && rs[j] != '"'; j++ {
				switch rs[j] {
				case '\\':
					if j+1 < n && strings.ContainsRune("\"\\$`\n", rs[j+1]) {
						j++
						if rs[j] != '\n' {
							word.WriteRune(rs[j])
						}
						continue
					}
					word.WriteRune('\\')
				case '`':
					p.Subst = true
					word.WriteRune('`')
				case '$':
					dollar(j, true)
				default:
					word.WriteRune(rs[j])
				}
			}
			if j >= n {
				p.Err = "unterminated quote"
				return p
			}
			inWord, quotedWord = true, true
			i = j
		case c == '`':
			p.Subst = true
			word.WriteRune(c)
			inWord = true
		case c == '$':
			dollar(i, false)
		case c == ' ' || c == '\t':
			endWord()
		case c == '\n' || c == ';':
			if c == ';' && i+1 < n && rs[i+1] == ';' {
				p.Err = "unexpected ;;"
				return p
			}
			endSeg(";")
		case c == '|':
			if i+1 < n && rs[i+1] == '|' {
				endSeg("||")
				i++
			} else {
				if i+1 < n && rs[i+1] == '&' { // |& pipes stderr too
					i++
				}
				endSeg("|")
			}
		case c == '&':
			switch {
			case i+1 < n && rs[i+1] == '&':
				endSeg("&&")
				i++
			case i+1 < n && rs[i+1] == '>':
				op := "&>"
				i++
				if i+1 < n && rs[i+1] == '>' {
					op = "&>>"
					i++
				}
				startRedir(op)
			default:
				p.Background = true
				endSeg("&")
			}
		case c == '>':
			op := ">"
			if i+1 < n {
				switch rs[i+1] {
				case '>':
					op = ">>"
					i++
				case '|':
					op = ">|"
					i++
				case '&':
					op = ">&"
					i++
				case '(':
					p.Subst = true
				}
			}
			startRedir(op)
		case c == '<':
			op := "<"
			if i+1 < n {
				switch rs[i+1] {
				case '<':
					p.Heredoc = true
					op = "<<"
					i++
				case '(':
					p.Subst = true
				case '>':
					op = "<>"
					i++
				case '&':
					op = "<&"
					i++
				}
			}
			startRedir(op)
		case c == '(' || c == ')':
			p.Group = true
			endWord()
		case c == '#' && !inWord:
			// comment to the end of the line
			for i+1 < n && rs[i+1] != '\n' {
				i++
			}
		default:
			if (c == '{' || c == '}') && !inWord && (i+1 >= n || rs[i+1] == ' ' || rs[i+1] == '\t' || rs[i+1] == ';' || rs[i+1] == '\n') {
				p.Group = true
				continue
			}
			if c == '*' || c == '?' || c == '[' {
				p.Glob = true
			}
			word.WriteRune(c)
			inWord = true
		}
	}
	endSeg("")
	if len(p.Segs) == 0 && p.Err == "" {
		p.Err = "empty command"
	}
	return p
}

// text renders a segment back as one line (words and redirections separated by spaces).
func (s shSegment) text() string {
	parts := append([]string{}, s.Words...)
	for _, r := range s.Redirs {
		parts = append(parts, r.Op+r.Target)
	}
	return strings.Join(parts, " ")
}

// cmdName returns the base name of a command word and whether it may be trusted as that
// command: a bare name or one of the standard binary directories (not ./ls or /tmp/ls).
func cmdName(w string) (string, bool) {
	if !strings.Contains(w, "/") {
		return w, w != ""
	}
	dir, base := path.Split(w)
	switch strings.TrimSuffix(dir, "/") {
	case "/bin", "/usr/bin", "/sbin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin":
		return base, base != ""
	}
	return base, false
}

// ─── glob matching ───────────────────────────────────

// globMatch matches s against a pattern with * (any run of characters) and ? (one
// character). In path mode * does not cross "/" and ** does.
func globMatch(pattern, s string, pathMode bool) bool {
	p, t := []rune(pattern), []rune(s)
	memo := map[[2]int]bool{}
	var m func(i, j int) bool
	m = func(i, j int) bool {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		var r bool
		switch {
		case i == len(p):
			r = j == len(t)
		case p[i] == '*':
			double := i+1 < len(p) && p[i+1] == '*'
			next := i + 1
			if double {
				next = i + 2
			}
			r = m(next, j)
			for k := j; !r && k < len(t); k++ {
				if pathMode && !double && t[k] == '/' {
					break
				}
				r = m(next, k+1)
			}
		case j == len(t):
			r = false
		case p[i] == '?':
			r = (!pathMode || t[j] != '/') && m(i+1, j+1)
		default:
			r = p[i] == t[j] && m(i+1, j+1)
		}
		memo[key] = r
		return r
	}
	return m(0, 0)
}

// globsOverlap reports whether two glob patterns (* ? and [..] treated as ?) can match a
// common string. It is used to refuse a wildcard argument that could expand to a
// sensitive file (cat /etc/sha*). In path mode a single * does not cross "/" (as in the
// shell) and ** does.
func globsOverlap(a, b string, pathMode bool) bool {
	const dstar = '\x00'
	norm := func(s string) []rune {
		var out []rune
		rs := []rune(s)
		for i := 0; i < len(rs); i++ {
			switch {
			case rs[i] == '[':
				j := i + 1
				for j < len(rs) && rs[j] != ']' {
					j++
				}
				if j < len(rs) {
					out = append(out, '?')
					i = j
					continue
				}
			case rs[i] == '*' && i+1 < len(rs) && rs[i+1] == '*':
				out = append(out, dstar)
				i++
				continue
			}
			out = append(out, rs[i])
		}
		return out
	}
	p, q := norm(a), norm(b)
	isStar := func(c rune) bool { return c == '*' || c == dstar }
	// can the star c consume the character (or wildcard) x?
	eats := func(c, x rune) bool { return c == dstar || !pathMode || x != '/' }
	memo := map[[2]int]bool{}
	var m func(i, j int) bool
	m = func(i, j int) bool {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		memo[key] = false
		var r bool
		switch {
		case i == len(p) && j == len(q):
			r = true
		case i < len(p) && isStar(p[i]):
			r = m(i+1, j) || (j < len(q) && eats(p[i], q[j]) && m(i, j+1))
		case j < len(q) && isStar(q[j]):
			r = m(i, j+1) || (i < len(p) && eats(q[j], p[i]) && m(i+1, j))
		case i == len(p) || j == len(q):
			r = false
		case p[i] == '?' || q[j] == '?':
			r = (!pathMode || (p[i] != '/' && q[j] != '/')) && m(i+1, j+1)
		case p[i] == q[j]:
			r = m(i+1, j+1)
		}
		memo[key] = r
		return r
	}
	return m(0, 0)
}

func hasGlob(s string) bool { return strings.ContainsAny(s, "*?[") }
