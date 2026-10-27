// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── WORKING DIRECTORY OF A TERMINAL'S SHELL ─────────
//
// The 📂 menu of an SSH terminal opens the file manager "here". Shells that report their
// directory with OSC 7 are the fast path (the browser keeps it). Otherwise the browser asks
// the server ({"type":"cwd"} on the terminal WebSocket) and WRM opens a separate exec channel
// on the terminal's own SSH connection and runs cwdLookupScript:
//
//  1. the process table: /proc/<pid>/stat (Linux, also with BusyBox) or
//     ps -A -o pid= -o ppid= -o tty= -o tpgid= (BSD, macOS, other Unix);
//  2. the interactive shell is a sibling of the lookup: OpenSSH (and Dropbear) run every
//     channel of one connection under the same session process, so walking up from the
//     lookup's own shell, the first ancestor with a child on a terminal (a tty other than the
//     lookup's own, which has none) is the SSH session, and that child is the shell;
//  3. the terminal's foreground process (tpgid: the shell at its prompt, or e.g. an editor)
//     is preferred, the shell itself is the fallback (e.g. after sudo -i, whose root shell
//     cannot be read);
//  4. its working directory: readlink /proc/<pid>/cwd, pwdx, procstat -f (FreeBSD) or
//     lsof -d cwd (macOS, others).
//
// The answer is one line, "WRMCWD:<path>" or "WRMCWD-ERR:<reason>".

const cwdLookupScript = `if [ -r /proc/self/stat ]; then
  P=$(cat /proc/[0-9]*/stat 2>/dev/null | awk '{p=$1; sub(/^.*\) /, ""); print p, $2, $5, $6}')
else
  P=$(ps -A -o pid= -o ppid= -o tty= -o tpgid= 2>/dev/null)
fi
[ -n "$P" ] || { echo "WRMCWD-ERR:ps"; exit 0; }
pick=$(printf '%s\n' "$P" | awk -v me="$$" '
  function none(x) { return x == "" || x == "0" || x == "?" || x == "??" || x == "-" }
  NF >= 4 { pp[$1] = $2; tt[$1] = $3; tg[$1] = $4 }
  END {
    mine = tt[me]; chain[me] = 1; a = pp[me]
    for (lvl = 0; lvl < 4 && a != "" && a + 0 > 1; lvl++) {
      best = ""
      for (p in pp) if (pp[p] == a && !(p in chain) && !none(tt[p]) && tt[p] != mine && (best == "" || p + 0 > best + 0)) best = p
      if (best != "") { fg = tg[best] + 0 > 0 ? tg[best] : best; print best, fg; exit }
      chain[a] = 1; a = pp[a]
    }
  }')
[ -n "$pick" ] || { echo "WRMCWD-ERR:noshell"; exit 0; }
set -- $pick
cwdof() {
  d=$(readlink "/proc/$1/cwd" 2>/dev/null)
  if [ -z "$d" ] && command -v pwdx >/dev/null 2>&1; then d=$(pwdx "$1" 2>/dev/null | sed -n 's/^[0-9]*: //p'); fi
  if [ -z "$d" ] && command -v procstat >/dev/null 2>&1; then d=$(procstat -h -f "$1" 2>/dev/null | awk '$3 == "cwd" { s = $10; for (i = 11; i <= NF; i++) s = s " " $i; print s; exit }'); fi
  if [ -z "$d" ] && command -v lsof >/dev/null 2>&1; then d=$(lsof -a -p "$1" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1); fi
  case "$d" in /*) printf 'WRMCWD:%s\n' "$d"; return 0 ;; esac
  return 1
}
cwdof "$2" || cwdof "$1" || echo "WRMCWD-ERR:unreadable"
`

// cwdLookupCommand runs the script with sh, whatever the account's login shell is.
var cwdLookupCommand = "sh -c " + shQuote(cwdLookupScript)

const cwdLookupTimeout = 10 * time.Second

// errCwdUnknown: the directory could not be determined (the browser offers home or /).
type errCwdUnknown struct{ reason string }

func (e errCwdUnknown) Error() string { return e.reason }

// terminalCwd asks the server for the working directory of the terminal's shell over a new
// exec channel of the same SSH connection.
func terminalCwd(cl *ssh.Client) (string, error) {
	if cl == nil {
		return "", errCwdUnknown{"this terminal has no SSH connection"}
	}
	sess, err := cl.NewSession()
	if err != nil {
		return "", errCwdUnknown{"the server refused another channel on this connection (" + err.Error() + ")"}
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdout = &out
	done := make(chan error, 1)
	go func() { done <- sess.Run(cwdLookupCommand) }()
	select {
	case <-done:
	case <-time.After(cwdLookupTimeout):
		return "", errCwdUnknown{"the server did not answer in time"}
	}
	return parseCwdAnswer(out.String())
}

func parseCwdAnswer(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if p, ok := strings.CutPrefix(line, "WRMCWD:"); ok && strings.HasPrefix(p, "/") {
			return p, nil
		}
		if r, ok := strings.CutPrefix(line, "WRMCWD-ERR:"); ok {
			switch r {
			case "ps":
				return "", errCwdUnknown{"the server has neither /proc nor ps"}
			case "noshell":
				return "", errCwdUnknown{"the terminal's shell was not found among the server's processes"}
			case "unreadable":
				return "", errCwdUnknown{"the shell's directory cannot be read (no /proc, pwdx, procstat or lsof, or no permission)"}
			}
			return "", errCwdUnknown{r}
		}
	}
	return "", errCwdUnknown{fmt.Sprintf("unexpected answer from the server (%q)", truncateStr(strings.TrimSpace(out), 80))}
}
