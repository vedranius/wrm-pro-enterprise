// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ─── AI: READ-ONLY COMMAND CLASSIFIER ────────────────
//
// classifyReadOnly decides whether a command line only reads. It is an allowlist: a
// command is read-only when it parses cleanly (no substitution, here-document, group or
// background job), every simple command in it is a known reading command with arguments
// that keep it reading (systemctl status, not systemctl restart; sed without -i and
// without w/e commands; find without -delete/-exec …), no redirection writes anywhere but
// /dev/null, and no argument names a sensitive file (aiReadDenied). Anything else —
// including everything the classifier does not know — is "not read-only".

type roCheck func(args []string) (bool, string)

// shortCluster reports whether args holds a short option cluster (-abc) containing any of
// the letters, or one of the long options.
func hasOpt(args []string, letters string, long ...string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if strings.HasPrefix(a, "--") {
			name := a
			if i := strings.Index(a, "="); i > 0 {
				name = a[:i]
			}
			for _, l := range long {
				if name == l {
					return true
				}
			}
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 && letters != "" {
			if strings.ContainsAny(a[1:], letters) {
				return true
			}
		}
	}
	return false
}

// positionals returns the arguments that are not options. takesValue lists options whose
// value is the next argument.
func positionals(args []string, takesValue ...string) []string {
	var out []string
	skip := false
	for i, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--" {
			return append(out, args[i+1:]...)
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			for _, t := range takesValue {
				if a == t {
					skip = true
				}
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func firstPos(args []string, takesValue ...string) string {
	if p := positionals(args, takesValue...); len(p) > 0 {
		return p[0]
	}
	return ""
}

func oneOf(s string, list ...string) bool {
	for _, l := range list {
		if s == l {
			return true
		}
	}
	return false
}

func okIf(ok bool, why string) (bool, string) {
	if ok {
		return true, ""
	}
	return false, why
}

// subcommand checks that the first positional argument is one of the read-only ones.
func subcommand(allowEmpty bool, list ...string) roCheck {
	return func(args []string) (bool, string) {
		s := firstPos(args)
		if s == "" {
			return okIf(allowEmpty, "a subcommand is required")
		}
		return okIf(oneOf(s, list...), "subcommand "+s+" may change the system")
	}
}

func always(args []string) (bool, string) { return true, "" }

var roCommands map[string]roCheck

func init() {
	plain := []string{
		"cat", "head", "grep", "egrep", "fgrep", "zgrep", "zcat", "bzcat", "xzcat", "ls", "stat", "file", "wc", "du", "df",
		"ps", "pgrep", "pidof", "uptime", "free", "uname", "id", "whoami", "groups", "w", "who", "users", "last", "lastb",
		"printenv", "lsblk", "blkid", "findmnt", "lscpu", "lspci", "lsusb", "lsmod", "lsof", "ss", "netstat", "vmstat",
		"iostat", "mpstat", "pidstat", "nproc", "arch", "getconf", "pwd", "readlink", "realpath", "basename", "dirname",
		"namei", "getfacl", "lsattr", "md5sum", "sha1sum", "sha224sum", "sha256sum", "sha384sum", "sha512sum", "cksum",
		"b2sum", "sum", "strings", "hexdump", "od", "cut", "tr", "column", "nl", "tac", "rev", "diff", "cmp", "comm",
		"test", "[", "true", "false", "echo", "printf", "which", "whereis", "type", "locate", "nslookup", "dig", "host",
		"tracepath", "traceroute", "ping", "ping6", "seq", "expr", "jq", "fold", "fmt", "paste", "join", "numfmt",
		"base64", "lsns", "lsipc", "lslocks", "lslogins", "dmidecode", "tty", "locale", "sestatus", "getenforce",
		"aa-status", "apparmor_status", "zipinfo", "dpkg-query", "atq", "lvs", "vgs", "pvs", "lvdisplay", "vgdisplay",
		"pvdisplay", "iptables-save", "ip6tables-save", "cd", "uptime", "nstat", "lsinitramfs", "lshw", "hwinfo",
		"cal", "ncal", "id", "logname", "getcap", "ldd", "nm", "objdump", "readelf", "sleep",
	}
	roCommands = map[string]roCheck{}
	for _, c := range plain {
		roCommands[c] = always
	}
	roCommands["sleep"] = func(a []string) (bool, string) {
		n, err := strconv.ParseFloat(strings.TrimRight(firstPos(a), "s"), 64)
		return okIf(err == nil && n <= 30, "sleep longer than 30 seconds")
	}
	roCommands["ldd"] = func(a []string) (bool, string) { return false, "ldd may run the program" }
	roCommands["tail"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "fF", "--follow", "--retry"), "tail -f never ends")
	}
	roCommands["sort"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "o", "--output", "--compress-program"), "sort -o writes a file")
	}
	roCommands["uniq"] = func(a []string) (bool, string) {
		return okIf(len(positionals(a, "-f", "-s", "-w")) <= 1, "uniq with two files writes the second")
	}
	roCommands["tree"] = func(a []string) (bool, string) { return okIf(!hasOpt(a, "o"), "tree -o writes a file") }
	roCommands["sar"] = func(a []string) (bool, string) { return okIf(!hasOpt(a, "o"), "sar -o writes a file") }
	roCommands["lastlog"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "CS", "--clear", "--set"), "lastlog -C / -S change the log")
	}
	roCommands["xxd"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "r", "-revert") && len(positionals(a, "-s", "-l", "-c", "-g", "-o", "-n")) <= 1, "xxd -r or an output file writes")
	}
	roCommands["yq"] = func(a []string) (bool, string) { return okIf(!hasOpt(a, "i", "--inplace"), "yq -i writes the file") }
	roCommands["getent"] = func(a []string) (bool, string) {
		db := firstPos(a)
		return okIf(db != "shadow" && db != "gshadow", "getent "+db+" returns password hashes")
	}
	roCommands["env"] = func(a []string) (bool, string) { return okIf(len(a) == 0, "env with arguments runs a program") }
	roCommands["date"] = func(a []string) (bool, string) {
		for i := 0; i < len(a); i++ {
			x := a[i]
			switch {
			case strings.HasPrefix(x, "+"), x == "-u", x == "--utc", x == "-R", x == "--rfc-email", strings.HasPrefix(x, "-I"),
				strings.HasPrefix(x, "--iso-8601"), strings.HasPrefix(x, "--rfc-3339"), strings.HasPrefix(x, "--date="):
			case x == "-d" || x == "--date" || x == "-r" || x == "--reference":
				i++
			default:
				return false, "date " + x + " may set the clock"
			}
		}
		return true, ""
	}
	roCommands["hostname"] = func(a []string) (bool, string) {
		for _, x := range a {
			if !strings.HasPrefix(x, "-") || x == "-F" || strings.HasPrefix(x, "--file") || x == "-b" || x == "--boot" {
				return false, "hostname with a name sets it"
			}
		}
		return true, ""
	}
	roCommands["top"] = func(a []string) (bool, string) {
		return okIf(hasOpt(a, "b", "--batch") && hasOpt(a, "n", "--iterations"), "top needs -b -n 1")
	}
	roCommands["dmesg"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "cCDEnw", "--clear", "--read-clear", "--console-off", "--console-on", "--console-level", "--follow", "--follow-new"), "this dmesg option changes the kernel log")
	}
	roCommands["sysctl"] = func(a []string) (bool, string) {
		if hasOpt(a, "wpf", "--write", "--load", "--system") {
			return false, "sysctl -w / -p changes kernel settings"
		}
		for _, x := range a {
			if strings.Contains(x, "=") {
				return false, "sysctl key=value changes kernel settings"
			}
		}
		return true, ""
	}
	roCommands["find"] = func(a []string) (bool, string) {
		for _, x := range a {
			if oneOf(x, "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls") {
				return false, "find " + x + " changes files or runs programs"
			}
		}
		return true, ""
	}
	roCommands["sed"] = func(a []string) (bool, string) {
		if hasOpt(a, "i", "--in-place") {
			return false, "sed -i edits the file"
		}
		var scripts []string
		hasE := false
		for i := 0; i < len(a); i++ {
			switch {
			case a[i] == "-e" || a[i] == "--expression":
				if i+1 < len(a) {
					scripts = append(scripts, a[i+1])
				}
				hasE = true
				i++
			case strings.HasPrefix(a[i], "--expression="):
				scripts = append(scripts, strings.TrimPrefix(a[i], "--expression="))
				hasE = true
			case a[i] == "-f" || a[i] == "--file" || strings.HasPrefix(a[i], "--file="):
				return false, "sed -f runs a script file"
			}
		}
		if !hasE {
			if p := positionals(a, "-l", "--line-length"); len(p) > 0 {
				scripts = append(scripts, p[0])
			}
		}
		for _, s := range scripts {
			if !sedScriptReadOnly(s) {
				return false, "the sed script writes files or runs commands"
			}
		}
		return true, ""
	}
	awk := func(a []string) (bool, string) {
		if hasOpt(a, "fi", "--file", "--include", "--load", "--exec") {
			return false, "awk -f / -i runs other code"
		}
		prog := firstPos(a, "-F", "-v", "--field-separator", "--assign")
		for _, bad := range []string{"system", "getline", "|", ">", "close", "fflush"} {
			if strings.Contains(prog, bad) {
				return false, "the awk program may write files or run commands"
			}
		}
		return true, ""
	}
	roCommands["awk"], roCommands["gawk"], roCommands["mawk"], roCommands["nawk"] = awk, awk, awk, awk
	roCommands["tar"] = func(a []string) (bool, string) {
		if len(a) == 0 {
			return false, "tar without a mode"
		}
		mode := a[0]
		list := hasOpt(a, "", "--list") || (!strings.HasPrefix(mode, "--") && strings.Contains(mode, "t"))
		bad := hasOpt(a, "", "--create", "--extract", "--get", "--append", "--update", "--concatenate", "--catenate", "--delete", "--to-command", "--use-compress-program", "--checkpoint-action", "--info-script", "--new-volume-script") ||
			(!strings.HasPrefix(mode, "--") && strings.ContainsAny(mode, "cxruAI"))
		return okIf(list && !bad, "only tar -t (list) is read-only")
	}
	roCommands["unzip"] = func(a []string) (bool, string) {
		return okIf(hasOpt(a, "lvZtp") && !hasOpt(a, "d"), "only unzip -l / -v / -t / -p is read-only")
	}
	roCommands["command"] = func(a []string) (bool, string) {
		return okIf(len(a) > 0 && (a[0] == "-v" || a[0] == "-V"), "command runs a program")
	}
	roCommands["ip"] = func(a []string) (bool, string) {
		p := positionals(a, "-n", "-netns", "-f", "-family", "-c", "-color", "-rc", "-rcvbuf", "-l", "-loops")
		if len(p) == 0 {
			return false, "ip needs an object"
		}
		obj := p[0]
		if !oneOf(obj, "a", "addr", "address", "l", "link", "r", "ro", "route", "rule", "n", "neigh", "neighbor", "neighbour", "maddr", "maddress", "mroute", "tunnel", "netconf", "ntable", "tcp_metrics", "token", "macsec", "-V") {
			return false, "ip " + obj + " is not a read-only object"
		}
		if len(p) > 1 && !oneOf(p[1], "show", "sh", "s", "list", "lst", "ls", "l", "get", "dev") && !(obj == "route" || obj == "r" || obj == "ro") {
			return false, "ip " + obj + " " + p[1] + " may change the network"
		}
		if len(p) > 1 && (obj == "route" || obj == "r" || obj == "ro") && !oneOf(p[1], "show", "sh", "s", "list", "lst", "ls", "l", "get", "table") {
			return false, "ip route " + p[1] + " may change routes"
		}
		return true, ""
	}
	roCommands["systemctl"] = subcommand(true, "status", "show", "cat", "list-units", "list-unit-files", "list-timers", "list-sockets",
		"list-dependencies", "list-jobs", "list-machines", "list-automounts", "list-paths", "is-active", "is-enabled", "is-failed", "is-system-running",
		"get-default", "show-environment", "help")
	roCommands["service"] = func(a []string) (bool, string) {
		if len(a) == 1 && a[0] == "--status-all" {
			return true, ""
		}
		return okIf(len(a) == 2 && a[1] == "status", "only service <name> status is read-only")
	}
	roCommands["journalctl"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "f", "--follow", "--vacuum-size", "--vacuum-time", "--vacuum-files", "--rotate", "--flush", "--sync",
			"--relinquish-var", "--smart-relinquish-var", "--setup-keys", "--update-catalog"), "this journalctl option changes the journal or never ends")
	}
	roCommands["dpkg"] = func(a []string) (bool, string) {
		if len(a) == 0 {
			return false, "dpkg needs an action"
		}
		return okIf(oneOf(a[0], "-l", "--list", "-L", "--listfiles", "-s", "--status", "-S", "--search", "-p", "--print-avail",
			"--get-selections", "--print-architecture", "--print-foreign-architectures", "--audit", "-C", "--verify", "-V", "--compare-versions"),
			"dpkg "+a[0]+" may change packages")
	}
	roCommands["rpm"] = func(a []string) (bool, string) {
		if len(a) == 0 {
			return false, "rpm needs a mode"
		}
		m := a[0]
		ok := strings.HasPrefix(m, "-q") || m == "--query" || m == "-V" || m == "--verify" || strings.HasPrefix(m, "-V")
		bad := hasOpt(a, "eiUF", "--erase", "--install", "--upgrade", "--freshen", "--import", "--rebuilddb", "--initdb", "--setperms", "--setugids", "--restore", "--delsign", "--addsign", "--resign")
		if strings.HasPrefix(m, "-q") || strings.HasPrefix(m, "-V") {
			bad = hasOpt(a[1:], "eiUF", "--erase", "--install", "--upgrade", "--freshen", "--import", "--rebuilddb", "--initdb", "--setperms", "--setugids", "--restore")
		}
		return okIf(ok && !bad, "only rpm -q / -V is read-only")
	}
	roCommands["apt"] = subcommand(false, "list", "show", "policy", "search", "depends", "rdepends", "showsrc", "changelog")
	roCommands["apt-cache"] = subcommand(false, "policy", "show", "showpkg", "showsrc", "search", "depends", "rdepends", "madison", "stats", "pkgnames", "unmet", "dotty")
	roCommands["apt-mark"] = subcommand(false, "showmanual", "showauto", "showhold")
	dnf := subcommand(false, "list", "info", "search", "repolist", "repoinfo", "provides", "whatprovides", "deplist", "repoquery", "check-update", "updateinfo", "history")
	roCommands["yum"] = func(a []string) (bool, string) {
		p := positionals(a)
		if len(p) > 1 && p[0] == "history" && !oneOf(p[1], "list", "info", "summary", "package-list") {
			return false, "yum history " + p[1] + " changes packages"
		}
		return dnf(a)
	}
	roCommands["dnf"] = roCommands["yum"]
	roCommands["apk"] = subcommand(false, "info", "list", "search", "policy", "version", "stats", "dot")
	roCommands["pacman"] = func(a []string) (bool, string) {
		if len(a) == 0 {
			return false, "pacman needs an operation"
		}
		m := a[0]
		return okIf(strings.HasPrefix(m, "-Q") || m == "--query" || oneOf(m, "-Ss", "-Si", "-Sl", "-Sii"), "only pacman -Q / -Ss / -Si is read-only")
	}
	roCommands["zypper"] = subcommand(false, "search", "se", "info", "if", "lr", "repos", "list-updates", "lu", "patches", "pch", "packages", "pa", "products", "pd", "patterns", "pt", "list-patches", "lp")
	roCommands["snap"] = subcommand(false, "list", "info", "services", "version", "changes", "connections", "find")
	roCommands["flatpak"] = subcommand(false, "list", "info", "remotes", "history", "remote-ls", "search")
	container := func(a []string) (bool, string) {
		if hasOpt(a, "", "--follow") || (hasOpt(a, "f") && oneOf(firstPos(a), "logs")) {
			return false, "following logs never ends"
		}
		p := positionals(a, "-H", "--host", "--context", "-c", "--config", "--log-level", "-l", "--tail", "-n", "--since", "--until", "--format", "--filter", "-f")
		if len(p) == 0 {
			return false, "a subcommand is required"
		}
		sub := p[0]
		switch sub {
		case "ps", "images", "inspect", "logs", "version", "info", "top", "port", "diff", "history", "search":
			return true, ""
		case "stats":
			return okIf(hasOpt(a, "", "--no-stream"), "stats needs --no-stream")
		case "image", "network", "volume", "container", "system", "compose", "context", "node", "service", "stack", "plugin", "secret", "config":
			if len(p) < 2 {
				return false, "a subcommand is required"
			}
			if sub == "secret" {
				return false, "secrets are not read by the assistant"
			}
			if sub == "compose" && oneOf(p[1], "ps", "logs", "config", "ls", "images", "top", "version", "port") {
				return true, ""
			}
			if sub == "system" && oneOf(p[1], "df", "info", "events") {
				return okIf(p[1] != "events", "events never ends")
			}
			return okIf(oneOf(p[1], "ls", "list", "inspect", "history", "ps", "logs", "top", "port", "diff"), sub+" "+p[1]+" may change containers")
		}
		return false, sub + " may change containers"
	}
	roCommands["docker"], roCommands["podman"], roCommands["nerdctl"] = container, container, container
	roCommands["kubectl"] = func(a []string) (bool, string) {
		if hasOpt(a, "f", "--follow", "--watch", "-w") && oneOf(firstPos(a), "logs", "get") {
			return false, "following never ends"
		}
		p := positionals(a, "-n", "--namespace", "--context", "--kubeconfig", "-o", "--output", "-l", "--selector", "-c", "--container", "--tail", "--since", "--cluster", "--user")
		if len(p) == 0 {
			return false, "a subcommand is required"
		}
		for _, x := range p {
			if strings.HasPrefix(strings.ToLower(x), "secret") {
				return false, "secrets are not read by the assistant"
			}
		}
		switch p[0] {
		case "get", "describe", "logs", "top", "version", "cluster-info", "api-resources", "api-versions", "explain", "events":
			return true, ""
		case "auth":
			return okIf(len(p) > 1 && p[1] == "can-i", "only kubectl auth can-i is read-only")
		case "config":
			return okIf(len(p) > 1 && oneOf(p[1], "get-contexts", "current-context", "get-clusters"), "kubectl config "+strings.Join(p[1:], " ")+" is not read-only")
		}
		return false, "kubectl " + p[0] + " may change the cluster"
	}
	roCommands["git"] = func(a []string) (bool, string) {
		if hasOpt(a, "c", "--exec-path", "--config-env", "--output", "--ext-diff", "--upload-pack", "--receive-pack") {
			return false, "this git option may run programs or write files"
		}
		p := positionals(a, "-C", "--git-dir", "--work-tree", "--namespace")
		if len(p) == 0 {
			return false, "a subcommand is required"
		}
		switch p[0] {
		case "status", "log", "diff", "show", "rev-parse", "describe", "ls-files", "ls-tree", "blame", "shortlog", "cat-file", "rev-list", "grep", "whatchanged", "ls-remote":
			return true, ""
		case "branch":
			return okIf(len(p) == 1 && !hasOpt(a, "dDmMcC", "--delete", "--move", "--copy", "--set-upstream-to", "--unset-upstream", "--edit-description"), "git branch with a name changes branches")
		case "tag":
			return okIf(len(p) == 1 || hasOpt(a, "l", "--list"), "git tag with a name creates a tag")
		case "remote":
			return okIf(len(p) == 1 || oneOf(p[1], "show", "get-url"), "git remote "+strings.Join(p[1:], " ")+" changes remotes")
		case "reflog":
			return okIf(len(p) == 1 || p[1] == "show", "git reflog "+p[1]+" changes the reflog")
		case "config":
			return okIf(hasOpt(a, "l", "--get", "--get-all", "--list", "--get-regexp"), "git config without --get changes settings")
		case "stash":
			return okIf(len(p) > 1 && oneOf(p[1], "list", "show"), "git stash changes the work tree")
		}
		return false, "git " + p[0] + " may change the repository"
	}
	roCommands["crontab"] = func(a []string) (bool, string) {
		return okIf(hasOpt(a, "l") && !hasOpt(a, "re"), "only crontab -l is read-only")
	}
	ipt := func(a []string) (bool, string) {
		if hasOpt(a, "ADIRFZNXPE", "--append", "--delete", "--insert", "--replace", "--flush", "--zero", "--new-chain", "--delete-chain", "--policy", "--rename-chain") {
			return false, "this iptables option changes the firewall"
		}
		return okIf(hasOpt(a, "LS", "--list", "--list-rules"), "only iptables -L / -S is read-only")
	}
	roCommands["iptables"], roCommands["ip6tables"] = ipt, ipt
	roCommands["iptables-save"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "f", "--file"), "iptables-save -f writes a file")
	}
	roCommands["ip6tables-save"] = roCommands["iptables-save"]
	roCommands["nft"] = func(a []string) (bool, string) {
		return okIf(firstPos(a, "-f", "--file", "-I", "--includepath") == "list" && !hasOpt(a, "f", "--file"), "only nft list is read-only")
	}
	roCommands["firewall-cmd"] = func(a []string) (bool, string) {
		if len(a) == 0 {
			return false, "firewall-cmd needs an option"
		}
		for _, x := range a {
			if !(strings.HasPrefix(x, "--list-") || strings.HasPrefix(x, "--get-") || strings.HasPrefix(x, "--query-") || strings.HasPrefix(x, "--info-") ||
				x == "--state" || strings.HasPrefix(x, "--zone=") || x == "--permanent") {
				return false, "firewall-cmd " + x + " may change the firewall"
			}
		}
		return true, ""
	}
	roCommands["ufw"] = func(a []string) (bool, string) {
		p := positionals(a)
		return okIf(len(p) > 0 && (p[0] == "status" || p[0] == "show" || (p[0] == "app" && len(p) > 1 && oneOf(p[1], "list", "info"))), "only ufw status / show is read-only")
	}
	roCommands["hostnamectl"] = subcommand(true, "status")
	roCommands["timedatectl"] = subcommand(true, "status", "show", "timesync-status", "show-timesync", "list-timezones")
	roCommands["localectl"] = subcommand(true, "status", "list-locales", "list-keymaps", "list-x11-keymap-models", "list-x11-keymap-layouts")
	roCommands["loginctl"] = subcommand(true, "list-sessions", "list-users", "list-seats", "show-session", "show-user", "show-seat", "session-status", "user-status", "seat-status")
	roCommands["networkctl"] = subcommand(true, "list", "status", "lldp", "label")
	roCommands["resolvectl"] = subcommand(true, "status", "query", "statistics", "show-cache", "show-server-state")
	roCommands["systemd-analyze"] = subcommand(true, "time", "blame", "critical-chain", "security", "calendar", "timespan", "timestamp", "verify", "unit-paths")
	roCommands["mount"] = func(a []string) (bool, string) {
		return okIf(len(positionals(a, "-t")) == 0 && !hasOpt(a, "aoBMrwR", "--all", "--options", "--bind", "--move", "--rbind"), "mount with a device mounts it")
	}
	roCommands["curl"] = func(a []string) (bool, string) {
		// Fetching a URL is network egress: the model can put file contents into the query
		// string after reading them. It needs an approval (or an automatic-mode pattern).
		return false, "network requests are a change, not a read"
	}
	roCommands["openssl"] = func(a []string) (bool, string) {
		sub := ""
		if len(a) > 0 {
			sub = a[0]
		}
		if hasOpt(a, "", "-out", "-keyout", "-signkey", "-passout") || oneOf("-out", a...) || oneOf("-keyout", a...) {
			return false, "openssl -out writes a file"
		}
		switch sub {
		case "s_client", "x509", "version", "ciphers", "verify", "crl", "list", "s_time":
			return true, ""
		case "req":
			return okIf(oneOf("-noout", a...) && !oneOf("-new", a...) && !oneOf("-newkey", a...), "only openssl req -noout is read-only")
		}
		return false, "openssl " + sub + " is not read-only (or prints private keys)"
	}
	roCommands["mdadm"] = func(a []string) (bool, string) {
		for _, x := range a {
			if strings.HasPrefix(x, "-") && !oneOf(x, "--detail", "-D", "--examine", "-E", "--query", "-Q", "--detail-platform", "--scan", "-s", "--brief", "-b", "--verbose", "-v") {
				return false, "mdadm " + x + " may change arrays"
			}
		}
		return okIf(hasOpt(a, "DEQ", "--detail", "--examine", "--query", "--detail-platform"), "only mdadm --detail / --examine is read-only")
	}
	roCommands["smartctl"] = func(a []string) (bool, string) {
		return okIf(!hasOpt(a, "tsoXS", "--test", "--smart", "--offlineauto", "--saveauto", "--set", "--abort"), "this smartctl option changes the drive")
	}
	roCommands["nvme"] = subcommand(false, "list", "smart-log", "id-ctrl", "id-ns", "error-log", "list-subsys", "list-ns", "fw-log")
	roCommands["zpool"] = subcommand(false, "status", "list", "iostat", "get", "history")
	roCommands["zfs"] = subcommand(false, "list", "get")
	partList := func(a []string) (bool, string) {
		return okIf((hasOpt(a, "l", "--list", "--print", "--list-free", "--dump", "--show-size") || oneOf("print", a...)) &&
			!hasOpt(a, "", "--delete", "--zap", "--zap-all", "--new", "--write") && !oneOf("rm", a...) && !oneOf("mkpart", a...), "only listing partitions is read-only")
	}
	roCommands["fdisk"], roCommands["sfdisk"], roCommands["parted"], roCommands["gdisk"], roCommands["sgdisk"] = partList, partList, partList, partList, partList
}

// sedScriptReadOnly accepts sed scripts made of addresses, s/// with harmless flags and
// the printing / flow commands; w, W, e, r, R (files and commands) are refused.
func sedScriptReadOnly(s string) bool {
	rs := []rune(s)
	allowed := "0123456789,$!pdqQnNgGhHxlz=;{}~+ \t\n"
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '/' || c == '\\':
			// address regex: /re/ or \cREc
			d := '/'
			if c == '\\' {
				if i+1 >= len(rs) {
					return false
				}
				i++
				d = rs[i]
			}
			i++
			for i < len(rs) && rs[i] != d {
				if rs[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(rs) {
				return false
			}
			if i+1 < len(rs) && (rs[i+1] == 'I' || rs[i+1] == 'M') {
				i++
			}
		case c == 's' || c == 'y':
			if i+1 >= len(rs) {
				return false
			}
			d := rs[i+1]
			i += 2
			for part := 0; part < 2; part++ {
				for i < len(rs) && rs[i] != d {
					if rs[i] == '\\' {
						i++
					}
					i++
				}
				if i >= len(rs) {
					return false
				}
				i++
			}
			for i < len(rs) && strings.ContainsRune("gpiIm0123456789", rs[i]) {
				i++
			}
			i--
		case strings.ContainsRune(allowed, c):
		default:
			return false
		}
	}
	return true
}

// ─── sensitive files ─────────────────────────────────

type pathRule struct {
	Pattern string
	Base    bool // match against the base name only
	Why     string
}

// aiReadDenyDefaults are files whose content the assistant never reads, whatever the mode:
// password hashes, private keys, cloud and tool credentials. Administrators add their own
// patterns with the ai_read_deny_paths policy.
var aiReadDenyDefaults = []pathRule{
	{"/etc/shadow*", false, "password hashes"},
	{"/etc/gshadow*", false, "password hashes"},
	{"/etc/security/opasswd", false, "password hashes"},
	{"shadow", true, "password hashes"},
	{"gshadow", true, "password hashes"},
	{"id_rsa*", true, "SSH private key"},
	{"id_dsa*", true, "SSH private key"},
	{"id_ecdsa*", true, "SSH private key"},
	{"id_ed25519*", true, "SSH private key"},
	{"ssh_host_*_key", true, "SSH host private key"},
	{"*.ppk", true, "PuTTY private key"},
	{"/etc/ssl/private/**", false, "TLS private keys"},
	{"/etc/pki/tls/private/**", false, "TLS private keys"},
	{"*.key", true, "private key file"},
	{"/proc/*/environ", false, "process environment (may hold secrets)"},
	{"**/.aws/credentials", false, "cloud credentials"},
	{".aws", true, "cloud credentials"},
	{"**/.config/gcloud/**", false, "cloud credentials"},
	{"**/.azure/**", false, "cloud credentials"},
	{"**/.docker/config.json", false, "registry credentials"},
	{".netrc", true, "credentials"},
	{".pgpass", true, "database credentials"},
	{".my.cnf", true, "database credentials"},
	{".git-credentials", true, "Git credentials"},
	{"**/.kube/config", false, "cluster credentials"},
	{"**/.vault-token", false, "Vault token"},
	{"**/.gnupg/**", false, "GnuPG keys"},
	{".gnupg", true, "GnuPG keys"},
	{".env", true, "environment file (may hold secrets)"},
	{".env.*", true, "environment file (may hold secrets)"},
	{"*.p12", true, "private key bundle"},
	{"*.pfx", true, "private key bundle"},
	{".npmrc", true, "registry token"},
	{".pypirc", true, "registry token"},
	{"wp-config.php", true, "WordPress database secrets"},
	{"terraform.tfvars", true, "infrastructure secrets"},
	{"credentials.json", true, "cloud credentials"},
	{"**/config/database.yml", false, "database credentials"},
}

// aiReadDenied returns why a path must not be read ("" when it may be read). A path with
// wildcards is refused when it could expand to a sensitive file.
func aiReadDenied(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasSuffix(p, ".pub") && !hasGlob(p) {
		return ""
	}
	clean := p
	if !hasGlob(p) && strings.HasPrefix(p, "/") {
		clean = cleanRemotePath(p)
	}
	base := clean
	if i := strings.LastIndex(strings.TrimSuffix(clean, "/"), "/"); i >= 0 {
		base = strings.TrimSuffix(clean, "/")[i+1:]
	}
	rules := append([]pathRule{}, aiReadDenyDefaults...)
	for _, x := range splitPatterns(getSetting("ai_read_deny_paths")) {
		rules = append(rules, pathRule{x, !strings.Contains(x, "/"), "blocked by the administrator"})
	}
	dirGlob := hasGlob(clean[:len(clean)-len(base)])
	for _, r := range rules {
		target := clean
		if r.Base {
			target = base
		}
		if globMatch(r.Pattern, target, false) {
			return r.Why
		}
		if hasGlob(clean) {
			// a wildcard could expand to the file: compare with the exact names (id_rsa, not id_rsa.log)
			pat := r.Pattern
			if strings.HasSuffix(pat, "*") && !strings.HasSuffix(pat, "**") {
				pat = strings.TrimSuffix(pat, "*")
			}
			if globsOverlap(pat, target, !r.Base) {
				return r.Why
			}
			if r.Base && dirGlob && globsOverlap("**/"+pat, clean, true) {
				return r.Why
			}
		}
	}
	return ""
}

// cleanRemotePath normalises an absolute remote path (., .., double slashes).
func cleanRemotePath(p string) string {
	if p == "" {
		return p
	}
	abs := strings.HasPrefix(p, "/")
	var out []string
	for _, part := range strings.Split(p, "/") {
		switch part {
		case "", ".":
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, part)
		}
	}
	s := strings.Join(out, "/")
	if abs {
		return "/" + s
	}
	return s
}

// splitPatterns splits a policy value on commas and new lines.
func splitPatterns(s string) []string {
	var out []string
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// ─── the classifier ──────────────────────────────────

// aiNoContentCommands look at names and metadata, not at file contents, so the
// sensitive-file rules do not apply to their arguments.
var aiNoContentCommands = map[string]bool{
	"ls": true, "stat": true, "du": true, "find": true, "namei": true, "getfacl": true, "lsattr": true, "readlink": true,
	"realpath": true, "basename": true, "dirname": true, "test": true, "[": true, "file": true, "echo": true, "printf": true,
	"cd": true, "which": true, "whereis": true, "type": true, "locate": true, "df": true, "getcap": true,
}

type roVerdict struct {
	ReadOnly bool
	Reason   string
}

// innerCommand unwraps sudo -n, timeout, nice, ionice and xargs to the command they run.
// It returns nil when the wrapper itself is used in a way that is not understood.
func innerCommand(words []string) ([]string, string) {
	for len(words) > 0 {
		name, trusted := cmdName(words[0])
		if !trusted {
			return words, ""
		}
		args := words[1:]
		switch name {
		case "sudo":
			i := 0
			nonInteractive := false
			for i < len(args) && strings.HasPrefix(args[i], "-") {
				switch args[i] {
				case "-n", "--non-interactive":
					nonInteractive = true
				case "-u", "--user", "-g", "--group":
					i++
				default:
					return nil, "sudo " + args[i] + " is not allowed (use sudo -n <command>)"
				}
				i++
			}
			if !nonInteractive {
				return nil, "use sudo -n (sudo must not wait for a password)"
			}
			words = args[i:]
		case "timeout":
			i := 0
			for i < len(args) && strings.HasPrefix(args[i], "-") {
				if args[i] == "-s" || args[i] == "--signal" || args[i] == "-k" || args[i] == "--kill-after" {
					i++
				}
				i++
			}
			if i >= len(args) {
				return nil, "timeout without a command"
			}
			words = args[i+1:]
		case "nice", "ionice":
			i := 0
			for i < len(args) && strings.HasPrefix(args[i], "-") {
				if oneOf(args[i], "-n", "-c", "--adjustment", "--class", "--classdata") {
					i++
				}
				i++
			}
			words = args[i:]
		case "xargs":
			i := 0
			for i < len(args) && strings.HasPrefix(args[i], "-") {
				if oneOf(args[i], "-n", "-I", "-d", "-P", "-L", "-s", "-E", "-a", "--max-args", "--max-procs", "--delimiter", "--arg-file", "--max-lines") {
					i++
				}
				i++
			}
			if i >= len(args) {
				return []string{"echo"}, ""
			}
			words = args[i:]
		default:
			return words, ""
		}
	}
	return nil, "no command"
}

// classifySegment classifies one simple command.
func classifySegment(seg shSegment) roVerdict {
	for _, r := range seg.Redirs {
		if r.writes() && r.Target != "/dev/null" {
			return roVerdict{false, "writes to " + r.Target}
		}
		if !r.writes() && strings.TrimLeft(r.Op, "0123456789") == "<" {
			if why := aiReadDenied(r.Target); why != "" {
				return roVerdict{false, r.Target + ": " + why}
			}
		}
	}
	words := seg.Words
	if len(words) == 0 {
		return roVerdict{true, ""}
	}
	// LC_ALL=C cmd: harmless locale settings only
	for len(words) > 0 && strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "=") {
		k := words[0][:strings.Index(words[0], "=")]
		if !oneOf(k, "LC_ALL", "LANG", "LANGUAGE", "TZ", "COLUMNS", "SYSTEMD_PAGER", "PAGER", "SYSTEMD_COLORS") || (k == "PAGER" || k == "SYSTEMD_PAGER") && words[0] != k+"=" && words[0] != k+"=cat" {
			return roVerdict{false, "setting " + k + " may change what runs"}
		}
		words = words[1:]
	}
	inner, why := innerCommand(words)
	if inner == nil {
		return roVerdict{false, why}
	}
	if len(inner) == 0 {
		return roVerdict{true, ""}
	}
	name, trusted := cmdName(inner[0])
	if !trusted {
		return roVerdict{false, inner[0] + " is not a standard command"}
	}
	check, ok := roCommands[name]
	if !ok {
		return roVerdict{false, name + " is not a known read-only command"}
	}
	if ok, why := check(inner[1:]); !ok {
		return roVerdict{false, why}
	}
	if aiNoContentCommands[name] {
		return roVerdict{true, ""}
	}
	for _, a := range aiPathOperands(name, inner[1:]) {
		v := a
		if i := strings.Index(a, "="); strings.HasPrefix(a, "-") && i > 0 {
			v = a[i+1:]
		}
		if why := aiReadDenied(v); why != "" {
			return roVerdict{false, v + ": " + why}
		}
	}
	return roVerdict{true, ""}
}

// aiPathOperands drops the arguments that are patterns or programs, not files: grep's
// pattern (and -e values), sed's and awk's script, find's name/path patterns. A regular
// expression such as '^command=.*' is then not mistaken for a file name.
func aiPathOperands(name string, args []string) []string {
	out := []string{}
	switch name {
	case "grep", "egrep", "fgrep", "zgrep", "rg":
		explicit := false
		for i := 0; i < len(args); i++ {
			if a := args[i]; a == "-e" || a == "--regexp" || a == "-f" || a == "--file" {
				explicit = true
				if a == "-f" || a == "--file" {
					out = append(out, args[min(i+1, len(args)-1)])
				}
				i++
			} else if strings.HasPrefix(a, "--regexp=") || (strings.HasPrefix(a, "-e") && len(a) > 2 && !strings.HasPrefix(a, "--")) {
				explicit = true
			}
		}
		pattern := !explicit
		for i := 0; i < len(args); i++ {
			a := args[i]
			switch {
			case a == "-e" || a == "--regexp" || a == "-f" || a == "--file" || a == "-m" || a == "-A" || a == "-B" || a == "-C" || a == "--max-count" || a == "-g" || a == "-t" || a == "-T":
				i++ // the option's value
			case a == "--":
				for _, r := range args[i+1:] {
					if pattern {
						pattern = false
						continue
					}
					out = append(out, r)
				}
				return out
			case strings.HasPrefix(a, "-"):
			case pattern:
				pattern = false
			default:
				out = append(out, a)
			}
		}
		return out
	case "sed", "awk", "gawk", "mawk":
		script := true
		for _, a := range args {
			if a == "-e" || a == "-f" || strings.HasPrefix(a, "--expression") || strings.HasPrefix(a, "--file") {
				script = false
			}
		}
		skip := false
		for _, a := range args {
			switch {
			case skip:
				skip = false
			case a == "-e" || a == "-v" || a == "-F":
				skip = true
			case strings.HasPrefix(a, "-"):
			case script:
				script = false
			default:
				out = append(out, a)
			}
		}
		return out
	case "find":
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "-name", "-iname", "-path", "-ipath", "-wholename", "-iwholename", "-regex", "-iregex", "-lname", "-ilname":
				i++
			default:
				out = append(out, args[i])
			}
		}
		return out
	}
	return args
}

// classifyReadOnly classifies a whole command line.
func classifyReadOnly(cmd string) roVerdict {
	if strings.TrimSpace(cmd) == "" {
		return roVerdict{false, "empty command"}
	}
	if len(cmd) > 4000 {
		return roVerdict{false, "command too long"}
	}
	p := shParse(cmd)
	switch {
	case p.Err != "":
		return roVerdict{false, "cannot parse: " + p.Err}
	case p.Subst:
		return roVerdict{false, "command substitution hides what runs"}
	case p.Heredoc:
		return roVerdict{false, "here-documents are not read-only"}
	case p.Group:
		return roVerdict{false, "subshells and groups are not read-only"}
	case p.Background:
		return roVerdict{false, "background jobs are not allowed"}
	case p.Vars:
		return roVerdict{false, "variables hide which files are read"}
	}
	for _, s := range p.Segs {
		if v := classifySegment(s); !v.ReadOnly {
			return v
		}
	}
	return roVerdict{true, ""}
}

func (v roVerdict) String() string {
	if v.ReadOnly {
		return "read-only"
	}
	return fmt.Sprintf("not read-only (%s)", v.Reason)
}
