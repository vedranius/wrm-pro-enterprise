// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"strings"
)

// ─── AI: DESTRUCTIVE COMMANDS (always blocked) ───────
//
// aiDestructive checks a command line against the maintained list of destructive
// patterns. They are refused in every mode, also after an approval: wiping file systems
// and disks, rm -rf of / or system directories, fork bombs, cutting off SSH access,
// disabling the audit, writing to password files, boot files, raw disks or
// authorized_keys, piping downloads into a shell. Power commands (shutdown, reboot …) are
// blocked unless the ai_allow_power policy lets them through with an approval (never
// automatically).
//
// The check runs twice: on the parsed command (exact words, redirections) and on a
// "loose" split of the raw text with quotes removed, so a pattern hidden inside
// sh -c "…" or eval is still caught. A false positive only means the user runs that
// command themselves.

type aiBlock struct {
	Rule  string // stable id, e.g. "rm_root"
	Why   string
	Power bool // shutdown / reboot: may be allowed with an approval by policy
}

// aiDestructiveRules documents the list (Admin → AI shows it; tests cover each entry).
var aiDestructiveRules = []struct{ ID, What string }{
	{"rm_root", "rm -r of /, a top-level system directory, ~, . or * (and --no-preserve-root)"},
	{"mv_root", "mv of / or a top-level system directory"},
	{"chmod_root", "recursive chmod / chown / chgrp of / or a system directory"},
	{"mkfs", "mkfs, mke2fs, mkswap, wipefs, blkdiscard and other file system / disk wipers"},
	{"partition", "fdisk, sfdisk, parted, gdisk … (except listing)"},
	{"dd_disk", "dd writing to a device (of=/dev/…)"},
	{"shred_disk", "shred of a device or a system file"},
	{"fork_bomb", "fork bombs"},
	{"power", "shutdown, reboot, poweroff, halt, kexec, init 0 / 6 (policy ai_allow_power)"},
	{"isolate", "systemctl isolate / emergency / rescue"},
	{"ssh_lockout", "stopping, disabling or killing the SSH server; flushing the firewall or a DROP policy"},
	{"kill_init", "killing PID 1 or every process"},
	{"audit_off", "stopping auditd / journald / rsyslog, auditctl -D / -e 0"},
	{"crontab_r", "crontab -r"},
	{"root_account", "deleting, locking or removing the password of root"},
	{"protected_write", "writing to password and group files, sudoers, /boot, raw disks, /dev/mem, sysrq, authorized_keys, login records"},
	{"pipe_shell", "piping into a shell or interpreter (curl … | sh)"},
	{"admin_pattern", "patterns added by the administrator (ai_blocked_commands)"},
}

var aiSystemDirs = map[string]bool{
	"/": true, "/bin": true, "/boot": true, "/dev": true, "/etc": true, "/home": true, "/lib": true, "/lib32": true, "/lib64": true,
	"/libx32": true, "/media": true, "/mnt": true, "/opt": true, "/proc": true, "/root": true, "/run": true, "/sbin": true,
	"/srv": true, "/sys": true, "/usr": true, "/usr/bin": true, "/usr/lib": true, "/usr/lib64": true, "/usr/local": true,
	"/usr/sbin": true, "/usr/share": true, "/var": true, "/var/lib": true, "/var/log": true, "/snap": true, "/efi": true,
	"~": true, "$HOME": true, "${HOME}": true, ".": true, "..": true, "*": true, "": true,
}

// isSystemTarget reports whether an rm / chmod / mv target is / or a system directory.
func isSystemTarget(t string) bool {
	t = strings.TrimSpace(t)
	if t == "/" || t == "/*" || t == "/." {
		return true
	}
	for {
		n := strings.TrimSuffix(strings.TrimSuffix(t, "/*"), "/")
		n = strings.TrimSuffix(n, "/.")
		if n == t {
			break
		}
		t = n
	}
	if strings.HasPrefix(t, "/") {
		t = cleanRemotePath(t)
		if t == "/" {
			return true
		}
	}
	return aiSystemDirs[t]
}

// aiProtectedWrite are files and devices the assistant never writes.
var aiProtectedWrite = []string{
	"/etc/shadow*", "/etc/gshadow*", "/etc/passwd*", "/etc/group", "/etc/group-", "/etc/sudoers", "/etc/sudoers.d/**",
	"/etc/security/opasswd", "/boot/**", "/boot", "/efi/**", "/dev/sd*", "/dev/hd*", "/dev/vd*", "/dev/xvd*", "/dev/nvme*",
	"/dev/mmcblk*", "/dev/disk/**", "/dev/mapper/**", "/dev/md*", "/dev/dm-*", "/dev/loop*", "/dev/mem", "/dev/kmem", "/dev/port",
	"/proc/sysrq-trigger", "/proc/sys/kernel/sysrq", "**/.ssh/authorized_keys*", "/root/.ssh/**", "/var/log/wtmp", "/var/log/btmp",
	"/var/log/lastlog", "/var/log/audit/**", "/etc/audit/**",
}

// isProtectedWrite reports whether p (a path with or without wildcards) is a protected file.
func isProtectedWrite(p string) bool {
	if p == "" || p == "/dev/null" {
		return false
	}
	c := p
	if !hasGlob(p) && strings.HasPrefix(p, "/") {
		c = cleanRemotePath(p)
	}
	for _, pat := range aiProtectedWrite {
		if globMatch(pat, c, true) || (hasGlob(c) && globsOverlap(pat, c, true)) {
			return true
		}
		// ~/.ssh/authorized_keys and relative .ssh/authorized_keys
		if strings.HasPrefix(pat, "**/") && (globMatch(strings.TrimPrefix(pat, "**/"), c, true) || globMatch("*"+strings.TrimPrefix(pat, "**"), c, false)) {
			return true
		}
	}
	return false
}

func hasRecursive(args []string) bool {
	return hasOpt(args, "rR", "--recursive")
}

var aiPowerCommands = map[string]bool{"shutdown": true, "reboot": true, "poweroff": true, "halt": true, "kexec": true}

// blockWords checks one command (its words, starting at the command name).
func blockWords(words []string) *aiBlock {
	// wrappers that run the rest of the line: env VAR=x, nohup, exec, command, setsid …
	for len(words) > 0 {
		w, _ := cmdName(words[0])
		switch {
		case strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "-"):
		case oneOf(w, "env", "nohup", "exec", "command", "builtin", "setsid", "stdbuf", "unbuffer", "doas", "chroot", "eval"):
			if w == "chroot" && len(words) > 1 {
				words = words[1:]
			}
		case strings.HasPrefix(words[0], "-") && len(words) > 1 && !oneOf(w, "-c"):
		default:
			goto done
		}
		words = words[1:]
	}
done:
	inner, _ := innerCommand(words)
	if len(inner) == 0 {
		inner = words
	}
	if len(inner) == 0 {
		return nil
	}
	name, _ := cmdName(inner[0])
	args := inner[1:]
	pos := positionals(args)
	switch {
	case name == "rm" || name == "rmdir":
		if hasOpt(args, "", "--no-preserve-root") {
			return &aiBlock{Rule: "rm_root", Why: "rm --no-preserve-root"}
		}
		if name == "rm" && hasRecursive(args) {
			for _, t := range pos {
				if isSystemTarget(t) {
					return &aiBlock{Rule: "rm_root", Why: "recursive rm of " + t}
				}
			}
		}
	case name == "mv":
		for _, t := range pos {
			if isSystemTarget(t) && t != "." && t != ".." && t != "*" {
				return &aiBlock{Rule: "mv_root", Why: "mv of " + t}
			}
		}
	case name == "chmod" || name == "chown" || name == "chgrp" || name == "setfacl":
		for i, t := range pos {
			if i == 0 && name != "setfacl" {
				continue // the mode / owner
			}
			if t == "/" || (hasRecursive(args) && isSystemTarget(t)) {
				return &aiBlock{Rule: "chmod_root", Why: name + " -R of " + t}
			}
		}
	case strings.HasPrefix(name, "mkfs") || oneOf(name, "mke2fs", "mkswap", "wipefs", "blkdiscard", "mkntfs", "mkdosfs", "mkexfatfs", "zpool-destroy") ||
		(name == "zpool" && len(pos) > 0 && oneOf(pos[0], "destroy", "labelclear")) || (name == "lvremove" || name == "vgremove" || name == "pvremove"):
		return &aiBlock{Rule: "mkfs", Why: name + " wipes data"}
	case oneOf(name, "fdisk", "sfdisk", "cfdisk", "parted", "gdisk", "sgdisk", "cgdisk"):
		if name != "cfdisk" && name != "cgdisk" && (hasOpt(args, "l", "--list", "--print", "--list-free", "--dump", "--show-size") || oneOf("print", args...) || (name == "sgdisk" && hasOpt(args, "p"))) && !hasOpt(args, "", "--delete", "--zap", "--zap-all", "--new", "--write") {
			return nil
		}
		return &aiBlock{Rule: "partition", Why: name + " changes partitions"}
	case name == "dd":
		for _, a := range args {
			if strings.HasPrefix(a, "of=") {
				t := strings.TrimPrefix(a, "of=")
				if (strings.HasPrefix(t, "/dev/") && t != "/dev/null" && t != "/dev/stdout" && t != "/dev/stderr") || isProtectedWrite(t) {
					return &aiBlock{Rule: "dd_disk", Why: "dd to " + t}
				}
			}
		}
	case name == "shred":
		for _, t := range pos {
			if strings.HasPrefix(t, "/dev/") || isProtectedWrite(t) || isSystemTarget(t) {
				return &aiBlock{Rule: "shred_disk", Why: "shred of " + t}
			}
		}
	case aiPowerCommands[name]:
		if name == "shutdown" && hasOpt(args, "c", "--cancel") {
			return nil
		}
		return &aiBlock{Rule: "power", Why: name, Power: true}
	case name == "init" || name == "telinit":
		if len(pos) > 0 && oneOf(pos[0], "0", "6", "1", "s", "S", "single", "emergency") {
			return &aiBlock{Rule: "power", Why: name + " " + pos[0], Power: pos[0] == "0" || pos[0] == "6"}
		}
	case name == "systemctl":
		if len(pos) == 0 {
			return nil
		}
		sub := pos[0]
		if oneOf(sub, "reboot", "poweroff", "halt", "kexec", "soft-reboot", "suspend", "hibernate", "hybrid-sleep", "suspend-then-hibernate") {
			return &aiBlock{Rule: "power", Why: "systemctl " + sub, Power: true}
		}
		if oneOf(sub, "isolate", "emergency", "rescue", "default") && (sub != "isolate" || len(pos) < 2 || !strings.HasPrefix(pos[1], "multi-user")) {
			return &aiBlock{Rule: "isolate", Why: "systemctl " + sub + " cuts off remote access"}
		}
		if oneOf(sub, "stop", "disable", "mask", "kill") {
			for _, u := range pos[1:] {
				u = strings.TrimSuffix(strings.TrimSuffix(u, ".service"), ".socket")
				if oneOf(u, "ssh", "sshd", "openssh-server", "dropbear") {
					return &aiBlock{Rule: "ssh_lockout", Why: "systemctl " + sub + " " + u + " cuts off SSH access"}
				}
				if oneOf(u, "auditd", "systemd-journald", "rsyslog", "syslog", "syslog-ng") {
					return &aiBlock{Rule: "audit_off", Why: "systemctl " + sub + " " + u + " stops logging"}
				}
			}
		}
	case name == "service" || name == "rc-service":
		if len(pos) >= 2 && oneOf(pos[1], "stop") {
			if oneOf(pos[0], "ssh", "sshd", "dropbear") {
				return &aiBlock{Rule: "ssh_lockout", Why: "service " + pos[0] + " stop cuts off SSH access"}
			}
			if oneOf(pos[0], "auditd", "rsyslog", "syslog") {
				return &aiBlock{Rule: "audit_off", Why: "service " + pos[0] + " stop stops logging"}
			}
		}
	case name == "kill":
		for _, t := range pos {
			if t == "1" || t == "-1" {
				return &aiBlock{Rule: "kill_init", Why: "kill " + t}
			}
		}
		for _, a := range args {
			if a == "-1" && len(pos) == 0 {
				return &aiBlock{Rule: "kill_init", Why: "kill -1"}
			}
		}
		if len(args) >= 2 && args[len(args)-1] == "-1" {
			return &aiBlock{Rule: "kill_init", Why: "kill of every process"}
		}
	case name == "killall5":
		return &aiBlock{Rule: "kill_init", Why: "killall5 kills every process"}
	case name == "pkill" || name == "killall":
		for _, t := range pos {
			if oneOf(t, "sshd", "ssh", "init", "systemd") {
				return &aiBlock{Rule: "ssh_lockout", Why: name + " " + t}
			}
		}
		if hasOpt(args, "u", "--user") && oneOf("root", args...) && len(pos) == 1 && pos[0] == "root" {
			return &aiBlock{Rule: "kill_init", Why: name + " -u root"}
		}
	case name == "iptables" || name == "ip6tables":
		if hasOpt(args, "F", "--flush") && len(pos) == 0 {
			return &aiBlock{Rule: "ssh_lockout", Why: name + " -F removes every rule"}
		}
		for i, a := range args {
			if (a == "-P" || a == "--policy") && i+2 < len(args) && strings.ToUpper(args[i+1]) == "INPUT" && oneOf(strings.ToUpper(args[i+2]), "DROP", "REJECT") {
				return &aiBlock{Rule: "ssh_lockout", Why: name + " -P INPUT " + args[i+2]}
			}
		}
	case name == "nft":
		if len(pos) >= 2 && pos[0] == "flush" && pos[1] == "ruleset" {
			return &aiBlock{Rule: "ssh_lockout", Why: "nft flush ruleset"}
		}
	case name == "ufw":
		if len(pos) >= 3 && pos[0] == "default" && oneOf(pos[1], "deny", "reject") && pos[2] == "incoming" {
			return &aiBlock{Rule: "ssh_lockout", Why: "ufw default deny incoming"}
		}
	case name == "auditctl":
		if hasOpt(args, "D") || (oneOf("-e", args...) && oneOf("0", args...)) {
			return &aiBlock{Rule: "audit_off", Why: "auditctl " + strings.Join(args, " ")}
		}
	case name == "crontab":
		if hasOpt(args, "r", "--remove") {
			return &aiBlock{Rule: "crontab_r", Why: "crontab -r deletes every job"}
		}
	case oneOf(name, "userdel", "deluser"):
		if oneOf("root", pos...) {
			return &aiBlock{Rule: "root_account", Why: name + " root"}
		}
	case name == "passwd" || name == "usermod" || name == "chage":
		if oneOf("root", pos...) && hasOpt(args, "dlLeE", "--delete", "--lock", "--expire", "--expiredate") {
			return &aiBlock{Rule: "root_account", Why: name + " " + strings.Join(args, " ")}
		}
	}
	return nil
}

var (
	reForkBomb   = regexp.MustCompile(`([A-Za-z_:.][A-Za-z0-9_:.]*)\(\)\{`)
	rePipeShell  = regexp.MustCompile(`\|\s*(sudo\s+(-[A-Za-z-]+\s+)*)?(env\s+)?(/usr/bin/|/bin/|/usr/local/bin/)?(ba|z|da|k|c|tc|fi|a)?sh($|[\s;&|)])`)
	rePipeInterp = regexp.MustCompile(`\|\s*(sudo\s+(-[A-Za-z-]+\s+)*)?(/usr/bin/|/bin/)?(python[0-9.]*|perl|ruby|node|php|lua)\s*(-\s*)?($|[;&|)])`)
	reWriteDev   = regexp.MustCompile(`>\s*(/dev/(sd|hd|vd|xvd|nvme|mmcblk|disk/|mapper/|md|dm-|mem\b|kmem\b|port\b)|/etc/(shadow|gshadow|passwd|sudoers|group)\b|/proc/sysrq-trigger|/boot/)`)
)

// forkBomb looks for a function that pipes itself into itself in the background.
func forkBomb(raw string) bool {
	s := strings.Join(strings.Fields(raw), "")
	if strings.Contains(s, ":(){:|:&};:") || strings.Contains(s, ":(){:|:&}") {
		return true
	}
	for _, m := range reForkBomb.FindAllStringSubmatchIndex(s, -1) {
		name := s[m[2]:m[3]]
		body := s[m[1]:]
		if strings.Contains(body, name+"|"+name+"&") || strings.Contains(body, name+"&"+name) {
			return true
		}
	}
	return false
}

// looseSegments splits the raw text with quotes removed on every separator, so commands
// nested in sh -c "…", eval or $( … ) become segments too.
func looseSegments(raw string) [][]string {
	s := strings.NewReplacer(`'`, " ", `"`, " ", "\\\n", " ", `\`, "").Replace(raw)
	parts := strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(";&|()`\n{}", r) || r == '$' })
	var out [][]string
	for _, p := range parts {
		w := strings.Fields(p)
		if len(w) > 0 {
			out = append(out, w)
		}
	}
	return out
}

// aiDestructive returns why a command is blocked (nil when it is not destructive).
func aiDestructive(cmd string) *aiBlock {
	if forkBomb(cmd) {
		return &aiBlock{Rule: "fork_bomb", Why: "fork bomb"}
	}
	norm := strings.NewReplacer(`'`, "", `"`, "").Replace(cmd)
	if rePipeShell.MatchString(norm) || rePipeInterp.MatchString(norm) {
		return &aiBlock{Rule: "pipe_shell", Why: "piping into a shell or interpreter"}
	}
	if reWriteDev.MatchString(norm) {
		return &aiBlock{Rule: "protected_write", Why: "redirect to a protected file or device"}
	}
	for _, pat := range splitPatterns(getSetting("ai_blocked_commands")) {
		if globMatch(pat, strings.TrimSpace(cmd), false) || globMatch("*"+pat+"*", cmd, false) && !strings.Contains(pat, "*") {
			return &aiBlock{Rule: "admin_pattern", Why: "blocked by the administrator: " + pat}
		}
	}
	var power *aiBlock
	check := func(words []string, redirs []shRedir) *aiBlock {
		for _, r := range redirs {
			if r.writes() && isProtectedWrite(r.Target) {
				return &aiBlock{Rule: "protected_write", Why: "writes to " + r.Target}
			}
		}
		if b := blockWords(words); b != nil {
			if !b.Power {
				return b
			}
			if power == nil {
				power = b
			}
		}
		// a command that changes something must not name a protected file
		if len(words) > 0 && !classifySegment(shSegment{Words: words}).ReadOnly {
			inner, _ := innerCommand(words)
			if inner == nil {
				inner = words
			}
			name := ""
			if len(inner) > 0 {
				name, _ = cmdName(inner[0])
			}
			targets := inner
			if oneOf(name, "cp", "scp", "rsync", "install", "ln") {
				if p := positionals(inner[1:], "-t", "--target-directory", "-m", "--mode", "-o", "-g", "-e"); len(p) > 0 {
					targets = []string{p[len(p)-1]}
				}
				for i, a := range inner {
					if (a == "-t" || a == "--target-directory") && i+1 < len(inner) {
						targets = append(targets, inner[i+1])
					}
				}
			} else if len(inner) > 0 {
				targets = inner[1:]
			}
			for _, t := range targets {
				v := t
				if i := strings.Index(t, "="); i > 0 {
					v = t[i+1:]
				}
				if isProtectedWrite(v) {
					return &aiBlock{Rule: "protected_write", Why: name + " on " + v}
				}
			}
		}
		return nil
	}
	p := shParse(cmd)
	if p.Err == "" {
		for _, s := range p.Segs {
			if b := check(s.Words, s.Redirs); b != nil {
				return b
			}
		}
	}
	for _, w := range looseSegments(cmd) {
		for i := range w {
			// a nested command starts the segment or follows sh -c, eval, sudo, xargs …
			if i > 0 {
				prev, _ := cmdName(w[i-1])
				if !(strings.HasPrefix(prev, "-") && strings.HasSuffix(prev, "c") && !strings.HasPrefix(prev, "--")) &&
					!oneOf(prev, "eval", "exec", "sudo", "doas", "xargs", "env", "nohup", "timeout", "nice", "watch", "setsid", "su", "runuser", "command", "time") {
					continue
				}
			}
			if b := blockWords(w[i:]); b != nil {
				if b.Power {
					if power == nil {
						power = b
					}
					continue
				}
				return b
			}
		}
	}
	return power
}

// aiDestructivePath checks a file the assistant wants to write.
func aiDestructivePath(p string) *aiBlock {
	if isProtectedWrite(p) {
		return &aiBlock{Rule: "protected_write", Why: "writes to " + p}
	}
	return nil
}
