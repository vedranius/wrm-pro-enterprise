---
name: wrm-safe-operator
description: Work on servers through WRM PRO over MCP. Use whenever the WRM tools are available and the user asks to look at, diagnose or change a server, so that sessions, reads, approvals and changes follow WRM's rules.
license: Same terms as WRM PRO (LICENSE in the repository root)
---

# Working on servers through WRM PRO

WRM PRO is a self-hosted remote server manager. It holds the SSH credentials, decides what an AI
connection may do, asks the user to approve changes and records every call in its audit log. You
reach servers only through its MCP tools; you never see or need the server passwords or keys.

## Start

1. Call `list_connections` to see the servers this AI connection may use and the mode WRM applies
   to each (read-only, ask before changes, or automatic within limits). If the user did not say
   which server, call `select_server` or ask; do not guess between similar names.
2. Call `open_session` with the connection id or name. Keep the returned `session_id` for the
   other tools and note the mode and scopes it reports.
3. When the work is done, call `close_session`.

If a tool is missing from your list, the AI connection does not allow it. Say which permission is
missing (for example running commands) instead of working around it.

## Read before you change

- Prefer the dedicated read tools over shell commands: `system_info`, `service_status`,
  `find_logs`, `read_log`, `tail_log`, `journal`, `disk_usage`, `processes`, `network_status`,
  `list_packages`, `list_directory` and `read_file`. They are size-limited and redacted, and they
  work in read-only mode.
- Use `find_logs` before guessing a log path, and `read_log` with a service name to get its newest
  log or its systemd journal.
- To look for an error across many logs at once, use `search_logs` (a literal text, optionally
  narrowed to a service or a path filter). It greps every log WRM discovered on the server in one
  call; then read around the interesting lines with `read_log` or `tail_log`.
- Summarise what you found with the evidence lines before proposing anything.

## Changing something

- Before every change, tell the user in plain words what you will run or edit, on which server,
  and what it changes. Show the exact command, or the diff for a file. Then call the tool.
- Changes go through `run_command`, `edit_file`, `write_file` or `transfer_file`. In the "ask"
  mode WRM shows the change to the user, who may approve, edit or deny it. A denial is an answer:
  do not retry the same change in another form.
- One change at a time, then check the result (service status, the log, the file) before the next.
- For a configuration file, keep a backup copy next to it and run the program's own check (for
  example `nginx -t`, `sshd -t`, `apachectl configtest`) before a reload.
- Never run commands that wait for input. Pass non-interactive flags instead.

## What not to do

- Do not ask the user for server passwords, private keys or tokens, and do not print secrets that
  appear in output. WRM already masks the common ones; treat anything that looks like a secret as
  one.
- Do not copy data from one server to another or out of WRM unless the user asked for it.
- Do not try to bypass a refusal from WRM (blocked command, unreadable path, missing scope). Tell
  the user what was refused and why, and what an administrator could change in WRM.
