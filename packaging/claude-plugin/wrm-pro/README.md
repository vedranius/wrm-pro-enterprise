# WRM PRO — Claude plugin

Work on your servers through your own WRM PRO from Claude: logs, files, services and commands,
under WRM's permissions, approvals and audit. Claude never receives the server credentials. The
plugin needs no settings and works in the Claude app (desktop and claude.ai, Cowork) and in
Claude Code.

## Install

**Claude app:** *Customize → Plugins → Add → Add marketplace*, enter
`vedranius/web-browser-rdm-public` in lowercase, then install **WRM PRO**. If adding the
marketplace fails, *Add → Upload plugin* with `claude-plugin-wrm-pro.zip` from the
[latest WRM release](https://github.com/vedranius/web-browser-RDM-public/releases/latest)
installs the same plugin.

If the Claude app only says *Failed to add marketplace*:

- Type the repository in lowercase. The Claude app names the marketplace after the repository,
  and GitHub accepts the repository name in any case.
- Look under *Customize → Plugins* for a marketplace that is already there (an earlier attempt or
  another repository with the same name): remove it, or use *Check for updates* on it.
- The real reason is in the app's log: on Windows `%APPDATA%\Claude\logs\claude.ai-web.log`,
  on macOS `~/Library/Logs/Claude/claude.ai-web.log`; look for `MARKETPLACE_ERROR` or
  `REMOTE_SYNC_FAILED`.

**Claude Code** (2.1.275 or later):

```
/plugin install wrm-pro --marketplace vedranius/web-browser-RDM-public
```

or in a terminal `claude plugin install wrm-pro --marketplace vedranius/web-browser-RDM-public`.
Older versions: `/plugin marketplace add vedranius/web-browser-RDM-public`, then
`/plugin install wrm-pro@wrm-pro`.

## Use

Type **`/wrm-pro:wrm`** (typing `/wrm` finds it).

1. The first time, Claude asks for your WRM address, the one you open WRM with in the browser
   (for example `http://192.168.1.10:8780` or `https://wrm.example.com`).
2. WRM opens in the browser. Sign in and choose which servers, mode (read-only, ask before
   changes, automatic within limits), permissions and lifetime this app gets, then *Allow*.
3. Back in Claude, `/wrm-pro:wrm` lists your servers and everything WRM can do. Ask for what you
   need, for example "why is nginx down on demo-web".

When the access expires, or to switch to another WRM, run `/wrm-pro:wrm` or `/wrm-pro:connect`
again. An administrator turns on AI connections, with OAuth sign-in, in WRM → Settings → AI
connections.

## What is in it

- **MCP server `wrm`** (`server/wrm.mcpb`): a small Node.js bridge, packed as an
  [MCP bundle](https://github.com/modelcontextprotocol/mcpb), that runs on your computer and talks
  to your WRM's `/mcp`. The Claude app runs it with its built-in Node.js, so it reaches a WRM on
  your own network and over plain HTTP. Until it is connected it offers one tool,
  `wrm_connect`, which signs in through WRM in the browser. Every tool call runs on WRM: it
  checks the connection's mode and scopes, asks you to approve changes, redacts secrets and
  writes the audit log.
- **Skill `wrm`** (`/wrm-pro:wrm`): connect when needed, show the servers and what WRM offers,
  and how to work through it.
- **Skill `wrm-safe-operator`**: sessions, reading before changing, saying what will run before
  every change, what to do after a denial.
- **Skill `linux-incident-triage`**: a read-first checklist for a server or service that is down,
  slow, erroring, out of disk or memory.
- **Command `/wrm-pro:connect`**: sign in again or switch to another WRM.

In the Claude app the bundled server runs in Cowork; chat in the desktop app ignores local
servers of plugins, so for chat use WRM → Settings → AI connections → *Claude Desktop* (the
extension) or *All AI apps at once* (a script that also connects Claude Code, Codex, Gemini CLI,
VS Code, Cursor, Windsurf and OpenCode with the same sign-in).

## Privacy

The plugin sends nothing anywhere on its own. The bridge talks only to the WRM address you give
it, and WRM talks to your servers over SSH. The sign-in token is stored on your computer in
`~/.wrm/mcp.json` (readable only by you) and never passes through the chat. Server passwords and
keys stay in WRM and are never given to Claude. What Claude reads (command output, log lines,
files) goes to the model provider of your Claude account, like anything else in the chat, after
WRM's redaction. Revoke the access at any time in WRM → Settings → AI connections.

## Without the plugin

```sh
claude mcp add --transport http --scope user wrm https://wrm.example.com/mcp \
  --header "Authorization: Bearer wrm_pat_…"
```

with a token from WRM → Settings → AI connections, or from this folder:

```sh
./scripts/connect-claude.sh https://wrm.example.com/mcp wrm_pat_…
.\scripts\connect-claude.ps1 -Url https://wrm.example.com/mcp -Token wrm_pat_…
```

Claude Code opens its OAuth sign-in page only over HTTPS, so a local `http://` WRM needs the
token header there.

## Skills without the plugin

The skills are plain Agent Skills folders (`skills/<name>/SKILL.md`). Agents that read skills
from a folder can use them directly; the skills CLI installs them from this repository:

```sh
npx skills add vedranius/web-browser-RDM-public
```

## MCP Registry

`server.json` in the repository root describes the WRM MCP endpoint for the
[MCP Registry](https://registry.modelcontextprotocol.io) as `io.github.vedranius/wrm-pro`: a
remote server whose host is yours (`https://{wrm_host}/mcp`) and an `Authorization` header with
your token. Every release publishes it automatically through
`.github/workflows/mcp-registry.yml` (GitHub Actions OIDC, no stored secret).
