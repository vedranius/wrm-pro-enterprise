---
name: linux-incident-triage
description: Diagnose why a Linux server or service misbehaves (down, failing, slow, erroring, disk full, out of memory). Reads first, finds the cause with evidence, then proposes one fix and asks before running it. Works through WRM PRO or a plain SSH shell.
license: Same terms as WRM PRO (LICENSE in the repository root)
---

# Linux incident triage

Goal: find out what is wrong, show the evidence, and fix it only with the user's approval. Stay
read-only until you have a finding.

## 1. Pin down the symptom

Ask only what you cannot find out yourself: which server, which service or site, what the user
sees (error message, timeout, slowness) and since when. Note anything that changed recently
(deploy, update, configuration edit, reboot).

## 2. Gather facts (read only)

Check these in order and stop digging as soon as the cause is clear:

1. **Service state**: is it running, restarting in a loop, or failed? (`systemctl status <unit>`,
   `docker ps -a`)
2. **Its log around the time of the symptom**: the last 200 lines, then search for `error`,
   `fatal`, `denied`, `refused`, `timeout`, `killed`. (`journalctl -u <unit> -n 200 --no-pager`,
   the service's own log files)
3. **Resources**: free disk and inodes (`df -h`, `df -i`), memory and swap (`free -m`), load and
   the top processes (`uptime`, `ps aux --sort=-%cpu | head`), and the kernel log for the OOM
   killer (`journalctl -k -n 100 --no-pager`).
4. **Network**: is the service listening on the expected port (`ss -ltnp`), and can it reach what
   it depends on (database, upstream, DNS)?
5. **Recent changes**: package updates (`/var/log/apt/history.log`, `dnf history`), edited
   configuration files (`ls -lt /etc/<service>`), certificate expiry.

## 3. Write the finding

In a few lines: what is wrong, the log lines or numbers that prove it, how sure you are, and what
you have not checked yet. If there are two plausible causes, say which one you would test first.

## 4. Fix with approval

Propose one fix at a time. Show the exact command or the file diff, what it changes and how to
undo it, then wait for the user's approval. After the fix, check the service state and the log
again and report whether the symptom is gone.

Common safe first fixes: restart a crashed service after the cause is known; free disk space by
rotating or vacuuming logs (`journalctl --vacuum-size=…`) rather than deleting files you do not
understand; roll back the last configuration change. Never delete data, kill unknown processes or
reboot without an explicit yes.

## If you have WRM PRO

When the WRM PRO tools are available, use them instead of a shell, following the
`wrm-safe-operator` skill: `open_session` for the server, `service_status` for step 2.1,
`find_logs`, `search_logs` and `read_log` (or `journal`) for step 2.2, `disk_usage`, `processes` and
`system_info` for step 2.3, `network_status` for step 2.4 and `list_packages` and `read_file` for
step 2.5. Fixes go through `run_command` or `edit_file`, which WRM shows to the user for approval
and records in its audit log. Never ask for server credentials: WRM holds them.
