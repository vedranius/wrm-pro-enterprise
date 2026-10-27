---
name: wrm
description: Start here for anything WRM PRO can do. Use when the user types /wrm, mentions WRM or WRM PRO, asks which servers they can reach, or wants to look at, diagnose or change a server through WRM; also when WRM tools are missing and only wrm_connect is offered.
license: Same terms as WRM PRO (LICENSE in the repository root)
---

# WRM PRO

WRM PRO is the user's self-hosted remote server manager. It holds the SSH credentials, decides
what this AI app may do on which server, asks the user to approve changes and records every call
in its audit log. You reach servers only through its MCP tools and never see the passwords or
keys.

## 1. Connect (only when needed)

If the only WRM tool you have is `wrm_connect`, this computer is not connected to WRM yet (or its
access expired):

1. Ask the user for the WRM address, the one they open WRM with in the browser (for example
   `http://192.168.1.10:8780` or `https://wrm.example.com`).
2. Call `wrm_connect` with `address`. WRM opens in the browser; the user signs in and chooses
   which servers, mode, permissions and lifetime this app gets. The token goes straight to this
   computer (`~/.wrm/mcp.json`), never into the chat. Never ask the user to paste a token.
3. If the result says the sign-in is still open, tell the user to finish it in the browser (give
   the link from the result if no window opened), then call `wrm_connect` again to check.
4. When it reports "Connected", the WRM tools below appear. Continue with step 2.

If a tool answers that the token is wrong, expired or revoked, run `wrm_connect` again (the last
address is remembered). To switch to another WRM, call `wrm_connect` with the new address.

## 2. Show what WRM offers

When the user typed `/wrm` without a task, call `list_connections` and answer with:

- the servers this app may use, each with its mode (read-only, ask before changes, automatic
  within limits) and any open session;
- what they can ask for, grouped like below, mentioning only tools that are in your tool list
  (a missing tool means this AI connection does not allow it);
- one line: changes are always shown first and approved in WRM, and WRM → Settings → AI
  connections changes what this app may do.

Then ask which server and task to start with. If the user named a server or task after `/wrm`,
skip the overview and start the work.

## 3. Everything WRM can do

| Area | Tools | What for |
|---|---|---|
| Servers and sessions | `list_connections`, `select_server`, `open_session`, `close_session` | Which servers this app may use; open one (keep the `session_id`), close it when done |
| System overview | `system_info`, `processes`, `disk_usage`, `network_status`, `list_packages` | OS, uptime, load, memory, busy processes, full disks, listening ports, installed packages |
| Services | `service_status` | State and recent output of a systemd service |
| Logs | `find_logs`, `read_log`, `tail_log`, `search_logs`, `journal` | Find a service's logs, read or follow them, grep every discovered log at once, read the journal |
| Files | `list_directory`, `read_file`, `inventory_files` | Browse and read files (size-limited, secrets redacted) |
| Commands | `run_command` | Run a command; in ask mode the user approves, edits or denies it in WRM |
| Changes to files | `edit_file`, `write_file` | Edit or write a file, shown as a diff for approval |
| Transfer | `transfer_file` | Copy a file between the user's servers through WRM |
| Connection | `wrm_connect` | Sign in to WRM, again after expiry, or to another WRM |

The WRM server may also offer a `wrm` prompt (WRM help and server selection) in the app's
command menu; it gives the same list of authorized servers.

## 4. How to work

- Read before you change: prefer the dedicated read tools over shell commands; use `find_logs`
  before guessing a log path and `search_logs` to look for an error across many logs.
- Before every change, say in plain words what you will run or edit, on which server and what it
  changes, with the exact command or diff. Then call the tool and let WRM ask the user.
- One change at a time, then check the result before the next. A denial is an answer: do not try
  the same change another way; ask the user how to go on.
- Do not ask for server passwords, keys or tokens and do not repeat secrets that appear in output.
- The `wrm-safe-operator` and `linux-incident-triage` skills of this plugin cover careful changes
  and incident triage in more detail.
