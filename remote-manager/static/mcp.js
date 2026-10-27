/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// AI desktop apps (MCP) of WRM PRO: Settings → AI connections (tokens and setup snippets),
// the "AI connected" indicator with one-click revoke, approval cards for MCP sessions, the
// OAuth consent dialog and the administrators' AI connections tab. Loaded on demand by
// index.html (loadMCP); uses its helpers (t, esc, showToast, apiError, uiConfirm, fmtTime,
// copyText, currentUser, settingRowHTML, adminSettingsCache, adminTab).
// The server decides everything (scopes, modes, approvals); this file only shows it.
'use strict';

Object.assign(LANGS.en, {
  'mcp_sc_addons.manage': 'Manage WRM addons (administrators; every change needs approval)', mcp_quick_all: 'All AI apps at once', mcpw_c_all_apps: 'All AI apps on this computer', mcpw_via_all: 'one script: Claude, Codex, Gemini, VS Code, Cursor, OpenCode…',
  mcpw_all_h: 'Download the script for the computer where your AI apps run and start it there. It signs in to this WRM in the browser (you choose the servers and approve), saves the connection once in ~/.wrm/mcp.json and adds WRM to every AI app it finds: Claude Code (as the WRM PRO plugin), the Claude app, Codex CLI, Gemini CLI, VS Code, Cursor, Windsurf and OpenCode. Files it changes are backed up first. The script holds no token. It needs Node.js 18 or later.',
  mcpw_all_win: 'Windows (.ps1)', mcpw_all_unix: 'Linux / macOS (.sh)',
  mcpw_all_run: 'Windows: right-click the folder with the file → Open in Terminal, then:\npowershell -ExecutionPolicy Bypass -File .\\wrm-setup-all.ps1\nLinux / macOS:\nsh wrm-setup-all.sh\nThen start the AI apps again. When the access expires, wrm_connect in any app signs in again for all of them.',
  mcpw_all_claude_app: 'For skills and /wrm-pro:wrm in the Claude app add the plugin too: Customize → Plugins → Add marketplace → vedranius/wrm-pro-enterprise, or Upload plugin with claude-plugin-wrm-pro.zip from the WRM release.',
});
Object.assign(LANGS.hr, {
  'mcp_sc_addons.manage': 'Upravljanje WRM dodacima (administratori; svaka promjena traži odobrenje)', mcp_quick_all: 'Sve AI aplikacije odjednom', mcpw_c_all_apps: 'Sve AI aplikacije na ovom računalu', mcpw_via_all: 'jedna skripta: Claude, Codex, Gemini, VS Code, Cursor, OpenCode…',
  mcpw_all_h: 'Preuzmite skriptu na računalu na kojem rade vaše AI aplikacije i pokrenite je ondje. Prijavljuje se u ovaj WRM u pregledniku (birate poslužitelje i odobravate), vezu sprema jednom u ~/.wrm/mcp.json i dodaje WRM u svaku AI aplikaciju koju pronađe: Claude Code (kao WRM PRO dodatak), Claude aplikaciju, Codex CLI, Gemini CLI, VS Code, Cursor, Windsurf i OpenCode. Datoteke koje mijenja prvo kopira. Skripta ne sadrži token. Treba Node.js 18 ili noviji.',
  mcpw_all_win: 'Windows (.ps1)', mcpw_all_unix: 'Linux / macOS (.sh)',
  mcpw_all_run: 'Windows: desni klik na mapu s datotekom → Open in Terminal, zatim:\npowershell -ExecutionPolicy Bypass -File .\\wrm-setup-all.ps1\nLinux / macOS:\nsh wrm-setup-all.sh\nZatim ponovno pokrenite AI aplikacije. Kad pristup istekne, wrm_connect u bilo kojoj aplikaciji ponovno prijavljuje sve.',
  mcpw_all_claude_app: 'Za vještine i /wrm-pro:wrm u Claude aplikaciji dodajte i dodatak: Customize → Plugins → Add marketplace → vedranius/wrm-pro-enterprise, ili Upload plugin s datotekom claude-plugin-wrm-pro.zip iz WRM izdanja.',
});
Object.assign(LANGS.en, {
  mcpw_public_needed:'This cloud app needs a reachable HTTPS WRM address. Ask the WRM administrator to set the public MCP URL and reverse proxy; localhost or a private address cannot be reached from the cloud.', mcpw_local_url:'A localhost address works only when the AI client runs on the WRM host. Use the server address when it runs on another computer.', mcpw_installer:'Download setup script', mcpw_installer_d:'Run on the computer with the AI app. The script detects its CLI, backs up existing settings where available, adds WRM and lists the result. Enter the token when asked; it is not embedded in the script.', mcpw_oauth_cli:'Use OAuth sign-in (no copied token)', mcpw_cursor_install:'Add to Cursor', mcpw_slash:'In WRM assistant use /wrm or /wrm server. MCP clients expose the wrm prompt in their own command menu; if unsupported, type “wrm server” in chat to list authorized servers.',
  mcp_edit_note: 'The token and expiry stay the same. Current sessions and pending approvals stop; reopen them with the updated permissions.',
  mcp_title: 'AI connections', mcp_intro: 'Let an AI app on your computer or in the browser (Claude, ChatGPT and other MCP clients) work on servers you choose. WRM checks every action against the mode and the scopes below, asks you before changes, and records everything.',
  mcp_off: 'AI connections are not available', mcp_new: 'New AI connection', mcp_name: 'Name', mcp_name_ph: 'e.g. Claude Desktop on my laptop',
  mcp_conns: 'Servers', mcp_conns_filter: 'Filter servers…', mcp_mode: 'Mode', mcp_scopes: 'Scopes', mcp_hours: 'Valid for (hours)', mcp_hours_max: 'at most {n} hours',
  mcp_create: 'Create token', mcp_none: 'No AI connections yet.', mcp_revoke: 'Revoke', mcp_revoke_all: 'Revoke all', mcp_revoke_q: 'Revoke this AI connection? Its sessions end at once.',
  mcp_revoke_all_q: 'Revoke every AI connection? All AI apps lose access at once.', mcp_revoked_n: '{n} revoked',
  mcp_mode_read_only: 'Read-only', mcp_mode_ask: 'Ask before every change', mcp_mode_auto: 'Automatic within limits',
  mcp_mode_read_only_d: 'The app can only read: logs, files, services and read-only commands. Every change is refused.',
  mcp_mode_ask_d: 'Reads run directly. Every change waits for your approval, in WRM or in the app.',
  mcp_mode_auto_d: 'Changes on your allow list run without asking, within a time and action limit per session. Everything else still asks.',
  mcp_sc_read_logs: 'Read logs, files and system information', mcp_sc_run_readonly: 'Run read-only commands', mcp_sc_run_with_approval: 'Run commands that change something (with approval)',
  mcp_sc_edit_file_with_approval: 'Edit files (with approval, shown as a diff)', mcp_sc_transfer: 'Copy files between the chosen servers (with approval)',
  mcp_sc_addons_manage: 'Draft and manage WRM addons (administrators only, every change with approval)',
  mcp_sc_full_access: '⚠ Full access: nothing is refused, but everything WRM would block (destructive commands, deny lists, secret files, shutdown) asks you first, every time',
  mcp_auto_allow: 'Commands that may run without asking (one pattern per line)', mcp_auto_path_allow: 'Files that may be changed without asking (/etc/nginx/**)',
  mcp_auto_minutes: 'Time limit per session (minutes)', mcp_auto_actions: 'Maximum actions per session', mcp_auto_confirm: 'I allow these changes without asking, within these limits.',
  mcp_auto_all: 'Allow everything', mcp_auto_all_d: 'Every command and file on the chosen servers, without asking. Blocked commands and sensitive files still stay blocked.',
  mcp_auto_grant: 'No time or action limit: automatic for as long as this AI connection is valid',
  mcp_token_once: 'Copy the token now. WRM stores only a hash and cannot show it again.', mcp_token: 'Token', mcp_copy: 'Copy', mcp_url: 'MCP server address',
  mcp_quick_title: 'Connect Claude in one command',
  mcp_quick_d: 'Claude Code: the command below (or the WRM PRO plugin) with a token from the wizard. Claude Desktop: download the extension. claude.ai and ChatGPT: add a custom connector with the MCP address and sign in.',
  mcp_quick_cmd: 'Claude Code command (replace YOUR_TOKEN)',
  mcp_quick_wizard: 'Open the step-by-step wizard',
  mcp_quick_cc: 'Claude Code, step by step',
  mcp_quick_desk: 'Claude Desktop extension',
  mcp_quick_cloud: 'claude.ai / ChatGPT connector',
  mcp_quick_which: 'Easiest: “All AI apps at once” connects every AI app on your computer with one script and a browser sign-in. The Claude app can also add the WRM PRO plugin (Customize → Plugins → Add marketplace → vedranius/wrm-pro-enterprise); Claude Code uses /plugin or the claude command.',
  mcpw_cc_shell: 'In Claude Code, /plugin is a slash command typed at the Claude prompt, not in PowerShell. In PowerShell run the command below (no leading slash), or run `claude` first and type `/plugin …` there. After adding it, restart Claude Code or run `/mcp` to reconnect.',
  mcpw_desk_no_plugin: 'Two ways: the .mcpb extension below (token filled in, for chat), or the WRM PRO plugin in the Claude app (Customize → Plugins → Add marketplace → vedranius/wrm-pro-enterprise, or Upload plugin with claude-plugin-wrm-pro.zip from the WRM release). The plugin needs no token: type /wrm-pro:wrm and approve the sign-in in the browser.',
  mcp_setup: 'Set up your app', mcp_setup_claude_desktop: 'Claude Desktop (local bridge)', mcp_setup_remote: 'Claude (custom connector)', mcp_setup_chatgpt: 'ChatGPT (connector)',
  mcp_setup_generic: 'Other MCP clients', mcp_setup_cli: 'Test from a shell',
  mcp_h_claude_desktop: 'Copy the WRM program (the same file as the server, for your computer\'s system) to your computer, then add this to claude_desktop_config.json (Claude → Settings → Developer → Edit config) and restart Claude. Replace the path with the program\'s full path.',
  mcp_h_remote: 'In Claude (claude.ai or the desktop app): Settings → Connectors → Add custom connector, and enter the address below. Claude then opens WRM, where you sign in and choose the servers, the mode and the scopes. WRM must be reachable from the internet over HTTPS for claude.ai.',
  mcp_h_chatgpt: 'In ChatGPT: Settings → Apps & Connectors → Advanced settings → Developer mode, then Create (or Add custom connector). Enter the address below and choose OAuth. ChatGPT then opens WRM, where you approve the connection. WRM must be reachable from the internet over HTTPS.',
  mcp_h_generic: 'For clients that support remote MCP servers over HTTP with a header (for example Claude Code: claude mcp add --transport http …):',
  mcp_h_cli: 'Check that the token works:', mcp_oauth_box: 'Apps with OAuth (Claude, ChatGPT) need no token: add a custom connector with this address and approve it in WRM.',
  mcp_oauth_off: 'OAuth is turned off by the administrator: use a token.', mcp_kind_pat: 'token', mcp_kind_oauth: 'OAuth',
  mcp_expires: 'expires {time}', mcp_expired: 'expired', mcp_revoked: 'revoked', mcp_last_used: 'last used {time}', mcp_never_used: 'not used yet',
  mcp_live: 'connected', mcp_sessions_n: '{n} open sessions',
  mcp_ind_title: 'Connected AI apps', mcp_on: 'on {servers}',
  // approvals
  mcp_ap_title: '{client} asks to act on {server}', mcp_ap_cmd: 'Command', mcp_ap_edit_file: 'Edit {path}', mcp_ap_why: 'Why WRM asks', mcp_ap_reason: 'Reason given by the AI',
  mcp_ap_approve: 'Approve', mcp_ap_deny: 'Deny', mcp_ap_edit: 'Edit', mcp_ap_approve_edited: 'Approve edited', mcp_ap_expires: 'expires in {s} s', mcp_ap_note: 'Note for the AI (optional)',
  mcp_ap_transfer: 'File transfer',
  // consent
  mcp_cs_title: 'Connect an AI app to WRM', mcp_cs_asks: '{client} wants to work on your servers through WRM.',
  mcp_cs_redirect: 'After your answer you go back to {host}. Approve only if you started this connection in that app just now.',
  mcp_cs_approve: 'Allow', mcp_cs_deny: 'Deny', mcp_cs_expired: 'This request has expired. Start the connection in the app again.', mcp_cs_done: 'Returning to the app…',
  // admin
  mcp_adm_active: 'Active AI connections', mcp_adm_none: 'No active AI connections.', mcp_adm_clients: 'Registered OAuth apps', mcp_adm_remove: 'Remove',
  mcp_adm_remove_q: 'Remove this app? Its AI connections are revoked; it must register again.', mcp_adm_policies: 'Policies', mcp_adm_redirects: 'Redirect addresses',
  mcp_client: 'App', mcp_user: 'User', mcp_state: 'State', mcp_created: 'Created',
  mcp_adm_help: 'MCP clients reach WRM at {url}. The kill switch of the AI assistant also stops every AI connection.',
  p_ai_mcp_enabled: 'AI connections (MCP)', p_ai_mcp_enabled_d: 'Serve the MCP endpoint /mcp and the OAuth endpoints for AI apps.',
  p_ai_mcp: 'Who may use AI connections', p_ai_mcp_d: 'Who may create tokens and approve AI apps.', po_ai_mcp_off: 'Nobody', po_ai_mcp_admins: 'Administrators', po_ai_mcp_all: 'Everybody',
  p_ai_mcp_scopes: 'Allowed scopes', p_ai_mcp_scopes_d: 'read_logs, run_readonly, run_with_approval, edit_file_with_approval, transfer, addons.manage (administrators)', p_ai_mcp_scopes_ph: 'read_logs,run_readonly',
  p_ai_mcp_modes: 'Allowed modes', p_ai_mcp_modes_d: 'read_only, ask, auto. The AI assistant\'s mode rules narrow them per connection.', p_ai_mcp_modes_ph: 'read_only,ask',
  p_ai_mcp_token_hours: 'Default lifetime (hours)', p_ai_mcp_token_hours_d: 'Lifetime of a new token or OAuth approval when the user does not choose one.',
  p_ai_mcp_max_token_hours: 'Maximum lifetime (hours)', p_ai_mcp_max_token_hours_d: 'The longest lifetime a user may choose.',
  p_ai_mcp_oauth: 'OAuth for AI apps', p_ai_mcp_oauth_d: 'Dynamic client registration and OAuth 2.1 with PKCE (Claude, ChatGPT connectors). Off: tokens only.',
  p_ai_mcp_redirect_uris: 'Allowed redirect addresses', p_ai_mcp_redirect_uris_d: 'OAuth apps may only send users back to these addresses (* = any text; https, or http on localhost).', p_ai_mcp_redirect_uris_ph: 'https://claude.ai/api/mcp/auth_callback',
  p_ai_mcp_allowed_origins: 'Allowed browser origins', p_ai_mcp_allowed_origins_d: 'Web pages (besides WRM) that may call /mcp from a browser (CORS). Empty: none.', p_ai_mcp_allowed_origins_ph: 'https://app.example.com',
  p_ai_mcp_public_url: 'Public address', p_ai_mcp_public_url_d: 'The address AI apps use for WRM, when it differs from what WRM sees (reverse proxy).', p_ai_mcp_public_url_ph: 'https://wrm.example.com',
  p_ai_full_access: 'Full access for the AI', p_ai_full_access_d: 'One switch: commands and files WRM normally refuses (destructive and blocked commands, deny lists, shutdown, secret files) are asked for instead, each time, never automatically. Read-only sessions stay read-only.',
  p_ai_full_access_mode: 'Full access applies to', p_ai_full_access_mode_d: 'Every AI session (the switch alone), only AI connections with the Full access scope, or users who turn it on for themselves in Settings → AI connections.',
  p_ai_full_access_blocked: 'Blocked commands with full access', p_ai_full_access_blocked_d: 'Your own list of blocked commands (ai_blocked_commands) under full access: ask for approval like everything else, or stay blocked even with full access. The built-in destructive list always asks.',
  po_ai_full_access_blocked_ask: 'Ask for approval', po_ai_full_access_blocked_deny: 'Stay blocked',
  po_ai_full_access_mode_all: 'Every AI session', po_ai_full_access_mode_connection: 'AI connections with the Full access scope', po_ai_full_access_mode_user: 'Users who turn it on themselves',
  mcp_fa_all: '⚠ Full access is on for all AI sessions: nothing is refused outright, but everything WRM would block asks you first, every time.', mcp_fa_conn: '⚠ Full access is available: tick "Full access" on an AI connection (Edit) to have everything WRM would block asked instead of refused.',
  mcp_fa_user: '⚠ Full access for my AI sessions: nothing is refused outright, but everything WRM would block (destructive commands, deny lists, shutdown, secret files) asks me first, every time.',
  p_ai_mcp_elicitation: 'Approvals in the AI app', p_ai_mcp_elicitation_d: 'Ask for approvals also in the AI app (MCP elicitation) when it supports it. Approvals in WRM always work.',
  p_ai_mcp_transfer_max_mb: 'Transfer limit (MB)', p_ai_mcp_transfer_max_mb_d: 'The largest file the transfer scope copies.',
  mcpw_title: 'Connect an AI app (step by step)', mcpw_intro: 'WRM prepares the token and the exact setting for your app. You only copy it.',
  mcpw_s_client: 'App', mcpw_s_access: 'Servers and access', mcpw_s_connect: 'Connect', mcpw_s_test: 'Test',
  mcpw_pick: 'Which app do you want to connect?', mcpw_via_token: 'with a token (WRM creates it)', mcpw_via_oauth: 'sign in through WRM (no token)',
  mcpw_c_codex: 'Codex CLI', mcpw_c_codex_oauth:'Codex CLI (OAuth)', mcpw_c_claude_code: 'Claude Code (terminal or Code tab)', mcpw_c_opencode: 'OpenCode', mcpw_h_opencode_json: '2. Add this to ~/.config/opencode/opencode.json (or opencode.json in the project). OpenCode reads the token from the environment variable:', mcpw_h_opencode_oauth: 'Or without a token, with OAuth sign-in: this entry without headers, then these commands:', mcpw_c_gemini: 'Gemini CLI', mcpw_c_cursor: 'Cursor / VS Code / other', mcpw_c_claude_connector: 'Claude.ai (cloud connector, public HTTPS)', mcpw_c_chatgpt: 'ChatGPT',
  mcpw_access: 'What may the app do?', mcpw_p_read: 'Look only (recommended)', mcpw_p_read_d: 'Read logs, files and system information and run read-only commands. Nothing is changed.',
  mcpw_p_ask: 'Look and change, with your approval', mcpw_p_ask_d: 'Also run commands and edit files, but every change waits for your approval in WRM.',
  mcpw_valid: 'The token is valid for {n} hours. You can revoke it at any time below.',
  mcpw_h_token: 'The token was created. Do the steps below on the computer where the app runs, then press Next to test the connection.',
  mcpw_h_oauth: 'This app signs in through WRM, no token is needed. Do the steps below; the app then opens WRM, where you choose the servers and approve.',
  mcpw_h_env: '1. Keep the token in an environment variable (so it is not written into a file):', mcpw_h_codex_cmd: '2. Add WRM to Codex (one command):', mcpw_h_codex_toml: 'Or add this to ~/.codex/config.toml:',
  mcpw_h_claude_cmd: 'Run this command:', mcpw_h_gemini_cmd: 'Run this command:', mcpw_h_gemini_json: 'Or add this to ~/.gemini/settings.json:', mcpw_h_json: 'Add this to the MCP settings of the app (for example .cursor/mcp.json or .vscode/mcp.json):',
  mcpw_h_remote: 'In Claude: Settings → Connectors → Add custom connector, and enter this address:', mcpw_h_chatgpt: 'In ChatGPT: Settings → Apps & Connectors → Advanced settings → Developer mode → Create, enter this address and choose OAuth:',
  mcpw_h_instr: 'Recommended: save this text as {file} (global or in your project) so the app knows what "wrm" is:', mcpw_download: 'Download',
  mcpw_test: 'Test the connection', mcpw_test_d: 'This browser tests the endpoint and token. Also verify the setup from the computer running the AI app; cloud/network reachability is a separate check.',
  mcpw_test_ok: '{name} answered. {n} tools are available to the app.', mcpw_test_fail: 'The test failed: {err}. If WRM is behind a proxy or a different public address, check the address and try the app itself.',
  mcpw_create: 'Create token', mcpw_next: 'Next', mcpw_back: 'Back', mcpw_done: 'Done', mcpw_pick_server: 'Choose at least one server.', mcpw_grant_name: '{client} (setup wizard)',
  mcpw_advanced: 'Advanced: create a token with custom mode and scopes',
});
Object.assign(LANGS.en, {
  mcpw_c_claude_desktop: 'Claude Desktop app (chat)', mcpw_via_extension: 'one-click extension (works on your network)',
  mcpw_desk_h: 'Download the WRM PRO extension and open it: Claude Desktop shows an Install button. The address and the token are already filled in, so there is nothing to type. Works with WRM on your own network and over plain HTTP, because Claude Desktop talks to WRM from this computer.',
  mcpw_desk_dl: 'Download Claude Desktop extension (wrm-pro.mcpb)', mcpw_desk_dl_note: 'The file contains your token. Delete it after installing.',
  mcpw_desk_steps: '1. Open the downloaded wrm-pro.mcpb (double-click). If it does not open Claude: Claude → Settings → Extensions → Advanced settings → Install extension… and choose the file.\n2. Press Install, then turn the extension on. Leave the settings as they are.\n3. In a new chat, ask for example: “wrm: list my servers”.',
  mcpw_desk_token: 'Only if Claude asks for the token:', mcpw_desk_noapp: 'Claude Desktop does not need the WRM program on your computer; it brings its own runtime for the extension.',
  mcpw_cc_plugin: 'Or install the WRM PRO plugin (skills and /wrm-pro:wrm; Claude Code 2.1.275 or later). It needs no token: on first use /wrm-pro:wrm asks for the WRM address and you approve the sign-in in the browser:',
  mcp_w_watch: 'Watch live', mcp_w_auto: 'Open the live view when an AI app starts working on a server', mcp_w_title: '{client} on {server}',
  mcp_w_empty: 'Connected. What the AI runs on the server and its output appear here.', mcp_w_wait: 'Waiting for your approval', mcp_w_runs: 'runs', mcp_w_refused: 'refused by WRM', mcp_w_ask: 'needs your approval',
  mcp_w_later: 'Not approved in time, or the app declined the question: the AI no longer waits, but you can still approve it here. The AI then repeats it and it runs once.', mcp_w_later_ok: 'runs when the AI repeats it',
  ad_title: 'AI live view', ad_layout: 'Layout', ad_right: 'Right', ad_bottom: 'Bottom', ad_float: 'Floating',
  ad_right_t: 'Dock on the right, as a tab next to Sessions and Snippets (or drag the AI tab anywhere)', ad_bottom_t: 'Dock at the bottom, under the windows',
  ad_float_t: 'Floating window: move it by the empty part of its tab bar, drag the AI tab to dock it again', ad_min_t: 'Hide the panel (approvals still pop up)',
  ad_show: 'Show the AI panel', ad_chip_t: 'The AI live view is hidden. Click to show it.',
  ad_waiting: '{n} waiting for your approval', ad_box_min: 'Collapse this session',
  mcp_ap_later: 'Not approved in time, or the app declined the question. Approve it to let the AI run exactly this once when it repeats it (valid for an hour).',
  mcp_w_o_approved: 'approved', mcp_w_o_denied: 'denied: not run', mcp_w_o_timeout: 'no answer: not run', mcp_w_o_cancelled: 'cancelled', mcp_w_exit: 'exit code {code}', mcp_w_output: 'Output',
  mcp_w_ended: 'Session ended: {reason}', mcp_w_min: 'Minimize', mcp_w_close: 'Close (the AI keeps working; reopen it from the 🤖 button)', mcp_w_reason: 'Why',
  mcp_valid_for: 'Valid for', mcp_unit_hours: 'hours', mcp_unit_days: 'days', mcp_max_days: 'at most {n} days', mcpw_hours_d: 'When the token expires the app loses access until you create a new one. You can revoke it at any time below. An administrator sets the longest lifetime (Admin panel → AI connections).',
});
Object.assign(LANGS.hr, {
  mcpw_public_needed:'Ova aplikacija u oblaku treba dostupnu HTTPS WRM adresu. Administrator treba postaviti javnu MCP adresu i reverse proxy; localhost ili privatna adresa nisu dostupni iz oblaka.', mcpw_local_url:'Localhost adresa radi samo kad je AI aplikacija na WRM računalu. Na drugom računalu koristite adresu servera.', mcpw_installer:'Preuzmi skriptu za postavljanje', mcpw_installer_d:'Pokrenite na računalu s AI aplikacijom. Skripta pronalazi CLI, radi kopiju postojećih postavki gdje je dostupno, dodaje WRM i prikazuje rezultat. Token unosite kad ga traži; nije ugrađen u skriptu.', mcpw_oauth_cli:'Koristi OAuth prijavu (bez kopiranja tokena)', mcpw_cursor_install:'Dodaj u Cursor', mcpw_slash:'U WRM asistentu koristite /wrm ili /wrm server. MCP klijenti nude wrm prompt u svojim izbornicima; ako nije podržano, upišite “wrm server” u chat za popis odobrenih servera.',
  mcp_edit_note: 'Token i istek ostaju isti. Aktivne sesije i odobrenja se prekidaju; ponovno ih otvorite s novim ovlastima.',
  mcp_title: 'AI veze', mcp_intro: 'Dopustite AI aplikaciji na računalu ili u pregledniku (Claude, ChatGPT i drugi MCP klijenti) da radi na poslužiteljima koje odaberete. WRM provjerava svaku radnju prema načinu rada i opsezima u nastavku, pita vas prije promjena i sve bilježi.',
  mcp_off: 'AI veze nisu dostupne', mcp_new: 'Nova AI veza', mcp_name: 'Naziv', mcp_name_ph: 'npr. Claude Desktop na mom laptopu',
  mcp_conns: 'Poslužitelji', mcp_conns_filter: 'Filtriraj poslužitelje…', mcp_mode: 'Način rada', mcp_scopes: 'Opsezi', mcp_hours: 'Vrijedi (sati)', mcp_hours_max: 'najviše {n} sati',
  mcp_create: 'Stvori token', mcp_none: 'Još nema AI veza.', mcp_revoke: 'Opozovi', mcp_revoke_all: 'Opozovi sve', mcp_revoke_q: 'Opozvati ovu AI vezu? Njezine sesije odmah završavaju.',
  mcp_revoke_all_q: 'Opozvati sve AI veze? Sve AI aplikacije odmah gube pristup.', mcp_revoked_n: 'Opozvano: {n}',
  mcp_mode_read_only: 'Samo čitanje', mcp_mode_ask: 'Pitaj prije svake promjene', mcp_mode_auto: 'Automatski u granicama',
  mcp_mode_read_only_d: 'Aplikacija može samo čitati: zapise, datoteke, servise i naredbe koje samo čitaju. Svaka promjena se odbija.',
  mcp_mode_ask_d: 'Čitanje se izvršava odmah. Svaka promjena čeka vaše odobrenje, u WRM-u ili u aplikaciji.',
  mcp_mode_auto_d: 'Promjene s vašeg popisa dopuštenih izvršavaju se bez pitanja, unutar vremenskog ograničenja i ograničenja broja radnji po sesiji. Sve ostalo i dalje pita.',
  mcp_sc_read_logs: 'Čitanje zapisa, datoteka i podataka o sustavu', mcp_sc_run_readonly: 'Izvršavanje naredbi koje samo čitaju', mcp_sc_run_with_approval: 'Izvršavanje naredbi koje nešto mijenjaju (uz odobrenje)',
  mcp_sc_edit_file_with_approval: 'Uređivanje datoteka (uz odobrenje, prikazano kao razlike)', mcp_sc_transfer: 'Kopiranje datoteka između odabranih poslužitelja (uz odobrenje)',
  mcp_sc_addons_manage: 'Skiciranje i upravljanje WRM dodacima (samo administratori, svaka promjena uz odobrenje)',
  mcp_sc_full_access: '⚠ Potpuni pristup: ništa se ne odbija, ali sve što bi WRM blokirao (destruktivne naredbe, liste zabrana, tajne datoteke, gašenje) prvo pita tebe, svaki put',
  mcp_auto_allow: 'Naredbe koje se smiju izvršiti bez pitanja (jedan uzorak po retku)', mcp_auto_path_allow: 'Datoteke koje se smiju mijenjati bez pitanja (/etc/nginx/**)',
  mcp_auto_minutes: 'Vremensko ograničenje po sesiji (minute)', mcp_auto_actions: 'Najviše radnji po sesiji', mcp_auto_confirm: 'Dopuštam ove promjene bez pitanja, unutar ovih ograničenja.',
  mcp_auto_all: 'Dopusti sve', mcp_auto_all_d: 'Sve naredbe i datoteke na odabranim serverima, bez pitanja. Blokirane naredbe i osjetljive datoteke i dalje ostaju blokirane.',
  mcp_auto_grant: 'Bez ograničenja vremena i broja radnji: automatski dok god vrijedi ova AI veza',
  mcp_token_once: 'Kopirajte token sada. WRM sprema samo sažetak i ne može ga ponovno prikazati.', mcp_token: 'Token', mcp_copy: 'Kopiraj', mcp_url: 'Adresa MCP poslužitelja',
  mcp_quick_title: 'Poveži Claude jednom naredbom',
  mcp_quick_d: 'Claude Code: naredba ispod (ili WRM PRO dodatak) s tokenom iz čarobnjaka. Claude Desktop: preuzmite proširenje. claude.ai i ChatGPT: dodajte prilagođeni konektor s MCP adresom i prijavite se.',
  mcp_quick_cmd: 'Naredba za Claude Code (zamijenite YOUR_TOKEN)',
  mcp_quick_wizard: 'Otvori čarobnjak korak po korak',
  mcp_quick_cc: 'Claude Code, korak po korak',
  mcp_quick_desk: 'Proširenje za Claude Desktop',
  mcp_quick_cloud: 'claude.ai / ChatGPT konektor',
  mcp_quick_which: 'Najlakše: „Sve AI aplikacije odjednom” jednom skriptom i prijavom u pregledniku povezuje sve AI aplikacije na računalu. Claude aplikacija može dodati i WRM PRO dodatak (Customize → Plugins → Add marketplace → vedranius/wrm-pro-enterprise); Claude Code koristi /plugin ili claude naredbu.',
  mcpw_cc_shell: 'U Claude Codeu je /plugin kosa crta naredba na Claude promptu, ne u PowerShellu. U PowerShellu pokrenite naredbu ispod (bez kose crte) ili prvo pokrenite `claude` pa upišite `/plugin …`. Nakon dodavanja restartajte Claude Code ili pokrenite `/mcp` za ponovno spajanje.',
  mcpw_desk_no_plugin: 'Dva načina: .mcpb proširenje ispod (token je upisan, za chat) ili WRM PRO dodatak u Claude aplikaciji (Customize → Plugins → Add marketplace → vedranius/wrm-pro-enterprise, ili Upload plugin s datotekom claude-plugin-wrm-pro.zip iz WRM izdanja). Dodatak ne treba token: upišite /wrm-pro:wrm i odobrite prijavu u pregledniku.',
  mcp_setup: 'Postavljanje aplikacije', mcp_setup_claude_desktop: 'Claude Desktop (lokalni most)', mcp_setup_remote: 'Claude (prilagođeni konektor)', mcp_setup_chatgpt: 'ChatGPT (konektor)',
  mcp_setup_generic: 'Drugi MCP klijenti', mcp_setup_cli: 'Provjera iz ljuske',
  mcp_h_claude_desktop: 'Kopirajte program WRM (istu datoteku kao poslužitelj, za sustav vašeg računala) na računalo, zatim ovo dodajte u claude_desktop_config.json (Claude → Settings → Developer → Edit config) i ponovno pokrenite Claude. Zamijenite putanju punom putanjom programa.',
  mcp_h_remote: 'U Claudeu (claude.ai ili aplikacija za računalo): Settings → Connectors → Add custom connector, i upišite adresu u nastavku. Claude zatim otvara WRM, gdje se prijavite i odaberete poslužitelje, način rada i opsege. Za claude.ai WRM mora biti dostupan s interneta preko HTTPS-a.',
  mcp_h_chatgpt: 'U ChatGPT-u: Settings → Apps & Connectors → Advanced settings → Developer mode, zatim Create (ili Add custom connector). Upišite adresu u nastavku i odaberite OAuth. ChatGPT zatim otvara WRM, gdje odobravate vezu. WRM mora biti dostupan s interneta preko HTTPS-a.',
  mcp_h_generic: 'Za klijente koji podržavaju udaljene MCP poslužitelje preko HTTP-a sa zaglavljem (na primjer Claude Code: claude mcp add --transport http …):',
  mcp_h_cli: 'Provjerite radi li token:', mcp_oauth_box: 'Aplikacije s OAuth-om (Claude, ChatGPT) ne trebaju token: dodajte prilagođeni konektor s ovom adresom i odobrite ga u WRM-u.',
  mcp_oauth_off: 'Administrator je isključio OAuth: koristite token.', mcp_kind_pat: 'token', mcp_kind_oauth: 'OAuth',
  mcp_expires: 'istječe {time}', mcp_expired: 'isteklo', mcp_revoked: 'opozvano', mcp_last_used: 'zadnja uporaba {time}', mcp_never_used: 'još nije korišteno',
  mcp_live: 'povezano', mcp_sessions_n: 'Otvorenih sesija: {n}',
  mcp_ind_title: 'Povezane AI aplikacije', mcp_on: 'na {servers}',
  mcp_ap_title: '{client} traži radnju na {server}', mcp_ap_cmd: 'Naredba', mcp_ap_edit_file: 'Uređivanje {path}', mcp_ap_why: 'Zašto WRM pita', mcp_ap_reason: 'Razlog koji je naveo AI',
  mcp_ap_approve: 'Odobri', mcp_ap_deny: 'Odbij', mcp_ap_edit: 'Uredi', mcp_ap_approve_edited: 'Odobri uređeno', mcp_ap_expires: 'istječe za {s} s', mcp_ap_note: 'Napomena za AI (neobavezno)',
  mcp_ap_transfer: 'Prijenos datoteke',
  mcp_cs_title: 'Povezivanje AI aplikacije s WRM-om', mcp_cs_asks: '{client} želi raditi na vašim poslužiteljima preko WRM-a.',
  mcp_cs_redirect: 'Nakon odgovora vraćate se na {host}. Odobrite samo ako ste upravo pokrenuli ovu vezu u toj aplikaciji.',
  mcp_cs_approve: 'Dopusti', mcp_cs_deny: 'Odbij', mcp_cs_expired: 'Ovaj zahtjev je istekao. Ponovno pokrenite povezivanje u aplikaciji.', mcp_cs_done: 'Povratak u aplikaciju…',
  mcp_adm_active: 'Aktivne AI veze', mcp_adm_none: 'Nema aktivnih AI veza.', mcp_adm_clients: 'Registrirane OAuth aplikacije', mcp_adm_remove: 'Ukloni',
  mcp_adm_remove_q: 'Ukloniti ovu aplikaciju? Njezine AI veze se opozivaju; mora se ponovno registrirati.', mcp_adm_policies: 'Pravila', mcp_adm_redirects: 'Adrese za povratak',
  mcp_client: 'Aplikacija', mcp_user: 'Korisnik', mcp_state: 'Stanje', mcp_created: 'Stvoreno',
  mcp_adm_help: 'MCP klijenti pristupaju WRM-u na {url}. Prekidač za zaustavljanje AI asistenta zaustavlja i sve AI veze.',
  p_ai_mcp_enabled: 'AI veze (MCP)', p_ai_mcp_enabled_d: 'Posluži MCP krajnju točku /mcp i OAuth krajnje točke za AI aplikacije.',
  p_ai_mcp: 'Tko smije koristiti AI veze', p_ai_mcp_d: 'Tko smije stvarati tokene i odobravati AI aplikacije.', po_ai_mcp_off: 'Nitko', po_ai_mcp_admins: 'Administratori', po_ai_mcp_all: 'Svi',
  p_ai_mcp_scopes: 'Dopušteni opsezi', p_ai_mcp_scopes_d: 'read_logs, run_readonly, run_with_approval, edit_file_with_approval, transfer, addons.manage (administratori)', p_ai_mcp_scopes_ph: 'read_logs,run_readonly',
  p_ai_mcp_modes: 'Dopušteni načini rada', p_ai_mcp_modes_d: 'read_only, ask, auto. Pravila načina rada AI asistenta ih sužavaju po vezi.', p_ai_mcp_modes_ph: 'read_only,ask',
  p_ai_mcp_token_hours: 'Zadano trajanje (sati)', p_ai_mcp_token_hours_d: 'Trajanje novog tokena ili OAuth odobrenja kad ga korisnik ne odabere.',
  p_ai_mcp_max_token_hours: 'Najdulje trajanje (sati)', p_ai_mcp_max_token_hours_d: 'Najdulje trajanje koje korisnik smije odabrati.',
  p_ai_mcp_oauth: 'OAuth za AI aplikacije', p_ai_mcp_oauth_d: 'Dinamička registracija klijenata i OAuth 2.1 s PKCE-om (konektori za Claude, ChatGPT). Isključeno: samo tokeni.',
  p_ai_mcp_redirect_uris: 'Dopuštene adrese za povratak', p_ai_mcp_redirect_uris_d: 'OAuth aplikacije smiju korisnike vratiti samo na ove adrese (* = bilo koji tekst; https, ili http na localhostu).', p_ai_mcp_redirect_uris_ph: 'https://claude.ai/api/mcp/auth_callback',
  p_ai_mcp_allowed_origins: 'Dopušteni izvori preglednika', p_ai_mcp_allowed_origins_d: 'Web stranice (osim WRM-a) koje smiju pozivati /mcp iz preglednika (CORS). Prazno: nijedna.', p_ai_mcp_allowed_origins_ph: 'https://app.example.com',
  p_ai_mcp_public_url: 'Javna adresa', p_ai_mcp_public_url_d: 'Adresa kojom AI aplikacije pristupaju WRM-u, kad se razlikuje od one koju WRM vidi (obrnuti proxy).', p_ai_mcp_public_url_ph: 'https://wrm.example.com',
  p_ai_full_access: 'Potpuni pristup za AI', p_ai_full_access_d: 'Jedan prekidač: naredbe i datoteke koje WRM inače odbija (destruktivne i blokirane naredbe, liste zabrana, gašenje, tajne datoteke) umjesto toga traže odobrenje, svaki put, nikad automatski. Sesije samo za čitanje ostaju samo za čitanje.',
  p_ai_full_access_mode: 'Potpuni pristup vrijedi za', p_ai_full_access_mode_d: 'Sve AI sesije (samo prekidač), samo AI veze s opsegom Potpuni pristup ili korisnike koji ga sami uključe u Postavke → AI veze.',
  p_ai_full_access_blocked: 'Blokirane naredbe uz potpuni pristup', p_ai_full_access_blocked_d: 'Tvoja lista blokiranih naredbi (ai_blocked_commands) uz potpuni pristup: traže odobrenje kao sve ostalo ili ostaju blokirane i uz potpuni pristup. Ugrađena lista destruktivnih naredbi uvijek pita.',
  po_ai_full_access_blocked_ask: 'Traže odobrenje', po_ai_full_access_blocked_deny: 'Ostaju blokirane',
  po_ai_full_access_mode_all: 'Sve AI sesije', po_ai_full_access_mode_connection: 'AI veze s opsegom Potpuni pristup', po_ai_full_access_mode_user: 'Korisnike koji ga sami uključe',
  mcp_fa_all: '⚠ Potpuni pristup je uključen za sve AI sesije: ništa se ne odbija, ali sve što bi WRM blokirao prvo pita tebe, svaki put.', mcp_fa_conn: '⚠ Potpuni pristup je dostupan: označi "Potpuni pristup" na AI vezi (Uredi) pa će se sve što bi WRM blokirao pitati umjesto odbiti.',
  mcp_fa_user: '⚠ Potpuni pristup za moje AI sesije: ništa se ne odbija, ali sve što bi WRM blokirao (destruktivne naredbe, liste zabrana, gašenje, tajne datoteke) prvo pita mene, svaki put.',
  p_ai_mcp_elicitation: 'Odobrenja u AI aplikaciji', p_ai_mcp_elicitation_d: 'Traži odobrenja i u AI aplikaciji (MCP elicitation) kad je podržava. Odobrenja u WRM-u uvijek rade.',
  p_ai_mcp_transfer_max_mb: 'Ograničenje prijenosa (MB)', p_ai_mcp_transfer_max_mb_d: 'Najveća datoteka koju opseg transfer kopira.',
  mcpw_title: 'Povezivanje AI aplikacije (korak po korak)', mcpw_intro: 'WRM priprema token i točnu postavku za vašu aplikaciju. Vi je samo kopirate.',
  mcpw_s_client: 'Aplikacija', mcpw_s_access: 'Poslužitelji i pristup', mcpw_s_connect: 'Povezivanje', mcpw_s_test: 'Test',
  mcpw_pick: 'Koju aplikaciju želite povezati?', mcpw_via_token: 'tokenom (WRM ga stvara)', mcpw_via_oauth: 'prijava preko WRM-a (bez tokena)',
  mcpw_c_codex: 'Codex CLI', mcpw_c_codex_oauth:'Codex CLI (OAuth)', mcpw_c_claude_code: 'Claude Code (terminal ili Code kartica)', mcpw_c_opencode: 'OpenCode', mcpw_h_opencode_json: '2. Dodajte ovo u ~/.config/opencode/opencode.json (ili opencode.json u projektu). OpenCode token čita iz varijable okoline:', mcpw_h_opencode_oauth: 'Ili bez tokena, s OAuth prijavom: ovakav unos bez zaglavlja, zatim ove naredbe:', mcpw_c_gemini: 'Gemini CLI', mcpw_c_cursor: 'Cursor / VS Code / ostalo', mcpw_c_claude_connector: 'Claude.ai (konektor u oblaku, javni HTTPS)', mcpw_c_chatgpt: 'ChatGPT',
  mcpw_access: 'Što aplikacija smije raditi?', mcpw_p_read: 'Samo pregled (preporučeno)', mcpw_p_read_d: 'Čita zapise, datoteke i podatke o sustavu te izvršava naredbe koje samo čitaju. Ništa se ne mijenja.',
  mcpw_p_ask: 'Pregled i promjene, uz vaše odobrenje', mcpw_p_ask_d: 'Može i izvršavati naredbe i uređivati datoteke, ali svaka promjena čeka vaše odobrenje u WRM-u.',
  mcpw_valid: 'Token vrijedi {n} sati. Možete ga opozvati bilo kada u nastavku.',
  mcpw_h_token: 'Token je stvoren. Napravite korake u nastavku na računalu na kojem radi aplikacija, zatim pritisnite Dalje za test veze.',
  mcpw_h_oauth: 'Ova se aplikacija prijavljuje preko WRM-a, token nije potreban. Napravite korake u nastavku; aplikacija zatim otvara WRM, gdje birate poslužitelje i odobravate.',
  mcpw_h_env: '1. Token spremite u varijablu okoline (da se ne zapisuje u datoteku):', mcpw_h_codex_cmd: '2. Dodajte WRM u Codex (jedna naredba):', mcpw_h_codex_toml: 'Ili ovo dodajte u ~/.codex/config.toml:',
  mcpw_h_claude_cmd: 'Pokrenite ovu naredbu:', mcpw_h_gemini_cmd: 'Pokrenite ovu naredbu:', mcpw_h_gemini_json: 'Ili ovo dodajte u ~/.gemini/settings.json:', mcpw_h_json: 'Dodajte ovo u MCP postavke aplikacije (na primjer .cursor/mcp.json ili .vscode/mcp.json):',
  mcpw_h_remote: 'U Claudeu: Settings → Connectors → Add custom connector, i upišite ovu adresu:', mcpw_h_chatgpt: 'U ChatGPT-u: Settings → Apps & Connectors → Advanced settings → Developer mode → Create, upišite ovu adresu i odaberite OAuth:',
  mcpw_h_instr: 'Preporučeno: spremite ovaj tekst kao {file} (globalno ili u projekt) da aplikacija zna što je "wrm":', mcpw_download: 'Preuzmi',
  mcpw_test: 'Testiraj vezu', mcpw_test_d: 'Ovaj preglednik provjerava adresu i token. Provjerite postavljanje i na računalu s AI aplikacijom; pristup iz oblaka/mreže zasebna je provjera.',
  mcpw_test_ok: '{name} je odgovorio. Aplikaciji je dostupno alata: {n}.', mcpw_test_fail: 'Test nije uspio: {err}. Ako je WRM iza proxyja ili ima drugu javnu adresu, provjerite adresu i isprobajte samu aplikaciju.',
  mcpw_create: 'Stvori token', mcpw_next: 'Dalje', mcpw_back: 'Natrag', mcpw_done: 'Gotovo', mcpw_pick_server: 'Odaberite barem jedan poslužitelj.', mcpw_grant_name: '{client} (čarobnjak za postavljanje)',
  mcpw_advanced: 'Napredno: stvori token s vlastitim načinom rada i opsezima',
});
Object.assign(LANGS.hr, {
  mcpw_c_claude_desktop: 'Claude Desktop aplikacija (chat)', mcpw_via_extension: 'proširenje u jednom kliku (radi u vašoj mreži)',
  mcpw_desk_h: 'Preuzmite WRM PRO proširenje i otvorite ga: Claude Desktop pokazuje gumb Install. Adresa i token već su upisani, pa ništa ne trebate tipkati. Radi i s WRM-om u vlastitoj mreži i preko običnog HTTP-a jer Claude Desktop s WRM-om razgovara s ovog računala.',
  mcpw_desk_dl: 'Preuzmi proširenje za Claude Desktop (wrm-pro.mcpb)', mcpw_desk_dl_note: 'Datoteka sadrži vaš token. Obrišite je nakon instalacije.',
  mcpw_desk_steps: '1. Otvorite preuzetu datoteku wrm-pro.mcpb (dvoklik). Ako se Claude ne otvori: Claude → Settings → Extensions → Advanced settings → Install extension… i odaberite datoteku.\n2. Pritisnite Install, zatim uključite proširenje. Postavke ostavite kakve jesu.\n3. U novom razgovoru zatražite na primjer: „wrm: popiši moje poslužitelje”.',
  mcpw_desk_token: 'Samo ako Claude zatraži token:', mcpw_desk_noapp: 'Claude Desktopu ne treba WRM program na računalu; za proširenje donosi vlastito okruženje.',
  mcpw_cc_plugin: 'Ili instalirajte WRM PRO dodatak (vještine i /wrm-pro:wrm; Claude Code 2.1.275 ili noviji). Ne treba token: pri prvoj upotrebi /wrm-pro:wrm pita adresu WRM-a, a prijavu odobrite u pregledniku:',
  mcp_w_watch: 'Prati uživo', mcp_w_auto: 'Otvori prikaz uživo kad AI aplikacija počne raditi na poslužitelju', mcp_w_title: '{client} na {server}',
  mcp_w_empty: 'Povezano. Ovdje se vidi što AI pokreće na poslužitelju i što dobiva kao izlaz.', mcp_w_wait: 'Čeka vaše odobrenje', mcp_w_runs: 'pokreće se', mcp_w_refused: 'WRM je odbio', mcp_w_ask: 'treba vaše odobrenje',
  mcp_w_later: 'Nije odobreno na vrijeme ili je aplikacija odbila pitanje: AI više ne čeka, ali i dalje to možeš odobriti ovdje. AI će to ponoviti i izvršit će se jednom.', mcp_w_later_ok: 'izvršit će se kad AI ponovi',
  ad_title: 'AI uživo', ad_layout: 'Raspored', ad_right: 'Desno', ad_bottom: 'Dolje', ad_float: 'Plutajući',
  ad_right_t: 'Usidri desno, kao karticu uz Sesije i Snippete (ili povuci AI karticu bilo kamo)', ad_bottom_t: 'Usidri dolje, ispod prozora',
  ad_float_t: 'Plutajući prozor: pomakni ga za prazni dio trake kartica, AI karticu povuci da ga opet usidriš', ad_min_t: 'Sakrij panel (odobrenja i dalje iskaču)',
  ad_show: 'Prikaži AI panel', ad_chip_t: 'AI prikaz uživo je skriven. Klikni da ga prikažeš.',
  ad_waiting: 'Čeka tvoje odobrenje: {n}', ad_box_min: 'Smanji ovu sesiju',
  mcp_ap_later: 'Nije odobreno na vrijeme ili je aplikacija odbila pitanje. Odobri da AI točno ovo izvrši jednom kad ponovi zahtjev (vrijedi sat vremena).',
  mcp_w_o_approved: 'odobreno', mcp_w_o_denied: 'odbijeno: nije pokrenuto', mcp_w_o_timeout: 'bez odgovora: nije pokrenuto', mcp_w_o_cancelled: 'otkazano', mcp_w_exit: 'izlazni kod {code}', mcp_w_output: 'Izlaz',
  mcp_w_ended: 'Sesija je završila: {reason}', mcp_w_min: 'Smanji', mcp_w_close: 'Zatvori (AI nastavlja raditi; ponovno otvori preko gumba 🤖)', mcp_w_reason: 'Zašto',
  mcp_valid_for: 'Vrijedi', mcp_unit_hours: 'sati', mcp_unit_days: 'dana', mcp_max_days: 'najviše {n} dana', mcpw_hours_d: 'Kad token istekne, aplikacija gubi pristup dok ne stvorite novi. Možete ga opozvati bilo kada u nastavku. Najdulje trajanje postavlja administrator (Admin panel → AI veze).',
});

// v13.8.0: an approval given after the AI stopped waiting runs at once (ai_mcp_later_approval)
Object.assign(LANGS.en, {
  mcp_w_later_run: 'Not approved in time, or the app closed the question: the AI no longer waits, but you can still approve it here. WRM then runs it at once, shows the output here and gives it to the AI when it asks again.',
  mcp_ap_later_run: 'Not approved in time, or the app closed the question. Approve it and WRM runs exactly this at once (within an hour); the AI gets the output when it repeats the call.',
  mcp_w_later_running: 'running now: the AI gets the output when it repeats the call', mcp_w_later_handed: 'the AI repeated the call and got the output of the run approved at {time} (not run again)',
  mcp_w_mcp_app: 'in the AI app', p_ai_mcp_later_approval: 'Approval after the AI stopped waiting', p_ai_mcp_later_approval_d: 'When an AI app stops waiting before anybody approves, the request stays open in WRM. Run: approving it runs the call at once and the AI gets its output when it repeats the call. Repeat: approving only lets the AI repeat exactly that call once.',
  po_ai_mcp_later_approval_run: 'Run it at once', po_ai_mcp_later_approval_repeat: 'Allow the AI to repeat it once',
});
Object.assign(LANGS.hr, {
  mcp_w_later_run: 'Nije odobreno na vrijeme ili je aplikacija zatvorila pitanje: AI više ne čeka, ali to i dalje možeš odobriti ovdje. WRM ga tada odmah izvrši, izlaz prikaže ovdje i preda ga AI-u kad ponovno pita.',
  mcp_ap_later_run: 'Nije odobreno na vrijeme ili je aplikacija zatvorila pitanje. Odobri i WRM točno ovo odmah izvrši (unutar sat vremena); AI dobije izlaz kad ponovi poziv.',
  mcp_w_later_running: 'izvršava se: AI dobije izlaz kad ponovi poziv', mcp_w_later_handed: 'AI je ponovio poziv i dobio izlaz izvršavanja odobrenog u {time} (nije ponovno pokrenuto)',
  mcp_w_mcp_app: 'u AI aplikaciji', p_ai_mcp_later_approval: 'Odobrenje nakon što AI prestane čekati', p_ai_mcp_later_approval_d: 'Kad AI aplikacija prestane čekati prije nego itko odobri, zahtjev ostaje otvoren u WRM-u. Izvrši: odobrenje odmah pokrene poziv, a AI dobije izlaz kad ga ponovi. Ponovi: odobrenje samo dopušta AI-u da točno taj poziv ponovi jednom.',
  po_ai_mcp_later_approval_run: 'Izvrši odmah', po_ai_mcp_later_approval_repeat: 'Dopusti AI-u da ponovi jednom',
});
(function () {
  const $ = id => document.getElementById(id);
  const tf = (k, vars) => { let s = t(k); for (const [a, b] of Object.entries(vars || {})) s = s.split('{' + a + '}').join(String(b)); return s; };
  async function api(method, url, body) {
    const res = await fetch(url, {method, headers: body ? {'Content-Type': 'application/json'} : {}, body: body ? JSON.stringify(body) : undefined});
    if (!res.ok) throw new Error(await apiError(res));
    return res.json().catch(() => ({}));
  }
  const css = `
.mcp-form .field { margin: 8px 0; }
.mcp-form .field > label { font-size: 11px; color: var(--text2); display: block; margin-bottom: 3px; }
.mcp-form input[type=text], .mcp-form input[type=number], .mcp-form select, .mcp-form textarea { width: 100%; box-sizing: border-box; }
.mcp-form textarea { min-height: 60px; font: 11px var(--font-mono); }
.mcp-row { display: flex; gap: 10px; } .mcp-row > * { flex: 1; min-width: 0; }
.mcp-conns { max-height: 170px; overflow: auto; border: 1px solid var(--border2); border-radius: 8px; padding: 4px 8px; background: var(--bg); }
.mcp-check { display: flex; align-items: flex-start; gap: 8px; font-size: 12px; padding: 3px 0; color: var(--text2); cursor: pointer; }
.mcp-check input { margin-top: 2px; }
.mcpw .field label.mcp-check {display:flex;text-transform:none;letter-spacing:normal;font-size:12px;font-weight:400;margin:0;color:var(--text2);}
.mcpw .field .mcp-check input {width:auto;padding:0;flex:none;}
.mcp-check > span {min-width:0;overflow-wrap:anywhere;} .mcp-check .sub { font-size: 10.5px; color: var(--text3); }
.mcp-form .field label.mcp-check, .mcp-form label.mcp-check { display: flex; text-transform: none; letter-spacing: normal; font-size: 12px; font-weight: 400; margin: 0; color: var(--text2); }
.mcp-form .field .mcp-check input, .mcp-form .mcp-check input { width: auto; padding: 0; flex: none; }
.mcp-card input[type=text], .mcp-card textarea { width: 100%; padding: 6px 9px; background: var(--bg3); border: 1px solid var(--border2); color: var(--text); border-radius: 6px; }
.mcp-grant { border: 1px solid var(--border2); border-radius: 8px; padding: 9px 11px; margin: 6px 0; background: var(--bg); display: flex; gap: 10px; align-items: flex-start; }
.mcp-grant .g-main { flex: 1; min-width: 0; font-size: 12px; } .mcp-grant .g-main b { color: var(--text); }
.mcp-grant .sub { font-size: 10.5px; color: var(--text3); margin-top: 3px; word-break: break-word; }
.mcp-snip { position: relative; } .mcp-snip pre { background: var(--bg); border: 1px solid var(--border2); border-radius: 8px; padding: 9px 11px; font: 11px var(--font-mono); white-space: pre-wrap; word-break: break-all; margin: 6px 0; max-height: 220px; overflow: auto; }
.mcp-snip .btn-sm { position: absolute; top: 6px; right: 6px; }
.mcp-tabs { display: flex; flex-wrap: wrap; gap: 4px; margin: 8px 0 4px; } .mcp-tabs button { font-size: 11px; padding: 4px 9px; }
.mcp-tabs button.active { background: var(--accent); color: #fff; border-color: var(--accent); }
.mcpw { border: 1px solid var(--border2); border-radius: 10px; padding: 10px 14px 12px; margin: 10px 0; background: var(--bg2); }
.mcpw-steps { display: flex; flex-wrap: wrap; gap: 6px; margin: 6px 0 10px; font-size: 11px; } .mcpw-steps span { padding: 3px 9px; border-radius: 999px; border: 1px solid var(--border2); color: var(--text3); }
.mcpw-steps span.on { background: var(--accent); border-color: var(--accent); color: #fff; } .mcpw-steps span.done { color: var(--text2); }
.mcpw-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 8px; margin-top: 8px; }
.mcpw-card { display: flex; flex-direction: column; align-items: flex-start; gap: 3px; text-align: left; padding: 9px 11px; height: auto; white-space: normal; }
.mcpw-card.active { border-color: var(--accent); }
.mcpw-nav { display: flex; gap: 8px; justify-content: flex-end; margin-top: 12px; }
#mcpw-auto-form .mcp-form > .field, #mcpw-auto-form .mcp-form > .mcp-row, #mcpw-auto-form .mcp-form > .sub {display:none;}
.mcpw-adv { margin-top: 12px; } .mcpw-adv > summary { cursor: pointer; font-size: 12px; color: var(--text2); }
#mcp-cards { position: fixed; right: 16px; bottom: 16px; z-index: 9700; display: flex; flex-direction: column; gap: 10px; max-width: min(460px, calc(100vw - 32px)); max-height: calc(100vh - 32px); overflow: auto; }
.mcp-card { background: var(--bg2); border: 1px solid var(--yellow); border-radius: 10px; padding: 12px 14px; box-shadow: 0 10px 30px rgba(0,0,0,.45); font-size: 12px; color: var(--text2); }
.mcp-card h4 { margin: 0 0 8px; font-size: 13px; color: var(--text); } .mcp-card pre { background: var(--bg); border: 1px solid var(--border2); border-radius: 6px; padding: 7px 9px; font: 11px var(--font-mono); white-space: pre-wrap; word-break: break-all; max-height: 200px; overflow: auto; margin: 4px 0 8px; }
.mcp-card pre .a { color: var(--green-t); } .mcp-card pre .d { color: var(--red-t); } .mcp-card pre .h { color: var(--text3); }
.mcp-card > .sub { display: block; margin-top: 3px; font-size: 11px; color: var(--text3); }
.mcp-card .acts { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 8px; } .mcp-card textarea, .mcp-card input[type=text] { width: 100%; box-sizing: border-box; font: 11px var(--font-mono); }
.mcp-card textarea { min-height: 70px; }
#mcp-pop { position: fixed; z-index: 9650; top: 52px; right: 12px; width: min(420px, calc(100vw - 24px)); background: var(--bg2); border: 1px solid var(--border2); border-radius: 10px; padding: 12px 14px; box-shadow: 0 12px 34px rgba(0,0,0,.5); display: none; }
#mcp-pop.open { display: block; }
#mcp-watch { display: none; flex-direction: column; min-height: 0; min-width: 0; font-size: 12px; color: var(--text2); }
#mcp-watch.on { display: flex; }
#mcp-watch .ad-head { display: flex; align-items: center; gap: 4px; padding: 5px 8px; border-bottom: 1px solid var(--border2); user-select: none; flex: none; touch-action: none; }
#mcp-watch .ad-head .ad-t { flex: 1; min-width: 0; font-weight: 700; color: var(--text); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
#mcp-watch .ad-head .ad-t .ad-wait { color: var(--yellow); font-weight: 600; margin-left: 6px; }
#mcp-watch .ad-head .icon-btn { width: 24px; height: 22px; padding: 0; font-size: 12px; }
#mcp-watch .ad-head .icon-btn.on { color: var(--accent); border-color: var(--accent); }
#mcp-watch .ad-body { flex: 1; min-height: 0; overflow: auto; padding: 8px; display: flex; flex-direction: column; gap: 8px; }
#mcp-watch.m-float { position: fixed; left: 16px; bottom: 16px; z-index: 9690; width: min(560px, calc(100vw - 32px)); max-height: calc(100vh - 80px); min-width: 280px; min-height: 120px; background: var(--bg); border: 1px solid var(--border2); border-radius: 10px; box-shadow: 0 14px 40px rgba(0,0,0,.5); resize: both; overflow: hidden; }
#mcp-watch.m-float .ad-head { cursor: move; }
#mcp-watch.m-right, #mcp-watch.m-bottom { flex: 1; }
#mcp-watch.m-right .mcp-wbox, #mcp-watch.m-bottom .mcp-wbox { box-shadow: none; }
#mcp-watch .mcp-wbox:only-child:not(.min) { flex: 1; } #mcp-watch .mcp-wbox:only-child .wb { max-height: none; flex: 1; }
#mcp-watch.m-float .mcp-wbox:only-child { flex: 1 1 auto; }
#mcp-watch.m-bottom .ad-body { flex-direction: row; align-items: stretch; }
#mcp-watch.m-bottom .mcp-wbox { flex: 1 1 340px; min-width: 300px; } #mcp-watch.m-bottom .mcp-wbox .wb { max-height: none; flex: 1; }
#mcp-watch.m-bottom .mcp-wbox.min { flex: 0 0 auto; min-width: 180px; }
#rp-ai { flex: 1; min-height: 0; display: none; flex-direction: column; }
#rp-ai > #mcp-watch { flex: 1; }
#ai-dock-chip { position: fixed; left: 16px; bottom: 16px; z-index: 9690; display: none; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 999px; background: var(--bg2); border: 1px solid var(--border2); box-shadow: 0 6px 20px rgba(0,0,0,.4); color: var(--text); font-size: 12px; cursor: pointer; }
#ai-dock-chip.on { display: flex; } #ai-dock-chip.wait { border-color: var(--yellow); animation: adPulse 1.6s ease-in-out infinite; }
@keyframes adPulse { 50% { box-shadow: 0 0 0 4px rgba(var(--yellow-rgb),.25); } }
.rp-badge { display: inline-block; min-width: 14px; padding: 0 4px; margin-left: 3px; border-radius: 7px; background: var(--bg4); color: var(--text2); font-size: 9px; line-height: 14px; text-align: center; }
.rp-badge.wait { background: var(--yellow); color: #111; }
.ad-seg { display: inline-flex; gap: 2px; background: var(--bg); border: 1px solid var(--border); border-radius: 7px; padding: 2px; }
.ad-seg button { background: transparent; border: none; color: var(--text3); font-size: 11px; padding: 3px 8px; border-radius: 5px; cursor: pointer; } .ad-seg button.active { background: var(--bg3); color: var(--text); }
@media (max-width: 759px) {
  #mcp-watch.m-float { left: 8px !important; right: 8px; top: auto !important; bottom: 8px !important; width: auto !important; height: auto !important; max-height: 55vh; resize: none; }
  #mcp-watch.m-float .ad-head { cursor: default; }
  #ai-dock-chip { left: 8px; bottom: 8px; }
}
.mcp-wbox { background: var(--bg2); border: 1px solid var(--border2); border-radius: 10px; box-shadow: 0 10px 30px rgba(0,0,0,.45); font-size: 12px; color: var(--text2); display: flex; flex-direction: column; min-height: 0; }
.mcp-wbox .wh { display: flex; align-items: center; gap: 6px; padding: 8px 10px; border-bottom: 1px solid var(--border2); } .mcp-wbox .wh b { flex: 1; color: var(--text); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mcp-wbox .wb { overflow: auto; max-height: 340px; padding: 8px 10px; display: flex; flex-direction: column; gap: 8px; } .mcp-wbox.min .wb { display: none; }
.mcp-wbox .we { border-left: 3px solid var(--border2); padding-left: 8px; } .mcp-wbox .we.ask { border-color: var(--yellow); } .mcp-wbox .we.bad { border-color: var(--red); } .mcp-wbox .we.ok { border-color: var(--green); }
.mcp-wbox pre { background: var(--bg); border: 1px solid var(--border2); border-radius: 6px; padding: 6px 8px; font: 11px var(--font-mono); white-space: pre-wrap; word-break: break-all; max-height: 180px; overflow: auto; margin: 4px 0; }
.mcp-wbox .acts { display: flex; gap: 6px; margin-top: 4px; }
@media (max-width: 480px) { .mcp-row { flex-direction: column; gap: 0; } #mcp-cards { right: 8px; left: 8px; bottom: 8px; max-width: none; } }
`;
  if (!$('mcp-style')) { const s = document.createElement('style'); s.id = 'mcp-style'; s.textContent = css; document.head.appendChild(s); }

  let cfg = null;
  function rel(iso) {
    const d = new Date(iso);
    if (isNaN(d)) return '';
    return fmtTime(iso);
  }
  function diffHTML(d) {
    return String(d || '').split('\n').map(l => {
      const c = l.startsWith('@@') ? 'h' : l.startsWith('+') && !l.startsWith('+++') ? 'a' : l.startsWith('-') && !l.startsWith('---') ? 'd' : '';
      return c ? `<span class="${c}">${esc(l)}</span>` : esc(l);
    }).join('\n');
  }

  // ── the grant form (settings and consent) ─────────────
  // token lifetime: a number with a unit (hours or days); the API always takes hours
  function lifetimeHTML(id, hours) {
    const days = hours >= 24 && hours % 24 === 0;
    return `<div style="display:flex;gap:6px;"><input type="number" id="${id}" min="1" value="${days ? hours / 24 : hours}" style="flex:1;min-width:70px;">
      <select id="${id}-unit" style="width:auto;"><option value="1" ${days ? '' : 'selected'}>${esc(t('mcp_unit_hours'))}</option><option value="24" ${days ? 'selected' : ''}>${esc(t('mcp_unit_days'))}</option></select></div>`;
  }
  function lifetimeValue(id) {
    const n = parseInt(($(id) || {}).value, 10), u = +(($(id + '-unit') || {}).value || 1);
    return n > 0 ? n * u : 0;
  }
  function lifetimeMax(h) { return h >= 24 && h % 24 === 0 ? tf('mcp_max_days', {n: h / 24}) : tf('mcp_hours_max', {n: h}); }
  function formHTML(prefix, c, pre) {
    pre = pre || {};
    const scopes = pre.scopes && pre.scopes.length ? pre.scopes : c.scopes.filter(s => s === 'read_logs' || s === 'run_readonly');
    const conns = c.connections.map(x => `<label class="mcp-check" data-name="${esc((x.name + ' ' + x.host).toLowerCase())}"><input type="checkbox" class="${prefix}-conn" value="${x.id}">
      <span>${esc(x.name)} <span class="sub">${esc(x.host)}${x.modes && x.modes.length ? '' : ' · ' + esc(t('mcp_off'))}</span></span></label>`).join('');
    return `<div class="mcp-form">
      ${pre.noName ? '' : `<div class="field"><label>${esc(t('mcp_name'))}</label><input type="text" id="${prefix}-name" maxlength="100" placeholder="${esc(t('mcp_name_ph'))}"></div>`}
      <div class="field"><label>${esc(t('mcp_conns'))}</label>${c.connections.length > 8 ? `<input type="text" id="${prefix}-filter" placeholder="${esc(t('mcp_conns_filter'))}" style="margin-bottom:4px;" oninput="wrmMCP.filter('${prefix}')">` : ''}
        <div class="mcp-conns" id="${prefix}-conns">${conns || `<div class="sub">${esc(t('none'))}</div>`}</div></div>
      <div class="mcp-row"><div class="field"><label>${esc(t('mcp_mode'))}</label><select id="${prefix}-mode" onchange="wrmMCP.modeChanged('${prefix}')">${c.modes.map(m => `<option value="${m}" ${m === 'read_only' ? 'selected' : ''}>${esc(t('mcp_mode_' + m))}</option>`).join('')}</select></div>
        <div class="field"><label>${esc(t('mcp_valid_for'))} <span class="sub">(${esc(lifetimeMax(c.max_hours))})</span></label>${lifetimeHTML(prefix + '-hours', c.default_hours)}</div></div>
      <div class="sub" id="${prefix}-mode-d" style="font-size:11px;color:var(--text3);">${esc(t('mcp_mode_read_only_d'))}</div>
      <div id="${prefix}-auto" style="display:none;">
        <div style="display:flex;gap:10px;align-items:center;margin:6px 0 10px;"><button type="button" class="btn-sec btn-sm" style="flex:none;" onclick="wrmMCP.allowAll('${prefix}')">${esc(t('mcp_auto_all'))}</button>
          <span class="sub" style="font-size:11px;color:var(--text3);">${esc(t('mcp_auto_all_d'))}</span></div>
        <div class="field"><label>${esc(t('mcp_auto_allow'))}</label><textarea id="${prefix}-auto-allow" placeholder="systemctl restart nginx"></textarea></div>
        <div class="field"><label>${esc(t('mcp_auto_path_allow'))}</label><textarea id="${prefix}-auto-paths"></textarea></div>
        <div class="mcp-row"><div class="field"><label>${esc(t('mcp_auto_minutes'))}</label><input type="number" id="${prefix}-auto-min" min="1" max="${c.auto_max_minutes}" value="${Math.min(30, c.auto_max_minutes)}"></div>
          <div class="field"><label>${esc(t('mcp_auto_actions'))}</label><input type="number" id="${prefix}-auto-act" min="1" max="${c.auto_max_actions}" value="${Math.min(20, c.auto_max_actions)}"></div></div>
        ${c.auto_until_grant ? `<label class="mcp-check"><input type="checkbox" id="${prefix}-auto-grant" onchange="wrmMCP.untilGrant('${prefix}')"><span>${esc(t('mcp_auto_grant'))}</span></label>` : ''}
        <label class="mcp-check"><input type="checkbox" id="${prefix}-auto-ok"><span>${esc(t('mcp_auto_confirm'))}</span></label>
      </div>
      <div class="field"><label>${esc(t('mcp_scopes'))}</label>${c.scopes.map(s => `<label class="mcp-check"><input type="checkbox" class="${prefix}-scope" value="${s}" ${scopes.includes(s) ? 'checked' : ''}>
        <span>${esc(t('mcp_sc_' + s))} <span class="sub mono">${esc(s)}</span></span></label>`).join('')}</div>
    </div>`;
  }
  function filter(prefix) {
    const q = ($(prefix + '-filter').value || '').toLowerCase();
    document.querySelectorAll(`#${prefix}-conns .mcp-check`).forEach(l => { l.style.display = !q || l.dataset.name.includes(q) ? '' : 'none'; });
  }
  function modeChanged(prefix) {
    const m = $(prefix + '-mode').value;
    $(prefix + '-mode-d').textContent = t('mcp_mode_' + m + '_d');
    $(prefix + '-auto').style.display = m === 'auto' ? '' : 'none';
  }
  // "Allow everything": every command and file without asking, for as long as the AI connection
  // is valid. The blocked commands, sensitive files and the chosen servers still apply.
  function allowAll(prefix) {
    $(prefix + '-auto-allow').value = '*'; $(prefix + '-auto-paths').value = '/**';
    const g = $(prefix + '-auto-grant'); if (g) { g.checked = true; untilGrant(prefix); }
  }
  function untilGrant(prefix) {
    const on = !!($(prefix + '-auto-grant') || {}).checked;
    [prefix + '-auto-min', prefix + '-auto-act'].forEach(id => { const el = $(id); if (el) { el.disabled = on; el.closest('.field').style.opacity = on ? .5 : ''; } });
  }
  function formValue(prefix) {
    const lines = id => ($(id).value || '').split('\n').map(x => x.trim()).filter(Boolean);
    const body = {
      conn_ids: [...document.querySelectorAll(`.${prefix}-conn:checked`)].map(x => +x.value),
      mode: $(prefix + '-mode').value, hours: lifetimeValue(prefix + '-hours'),
      scopes: [...document.querySelectorAll(`.${prefix}-scope:checked`)].map(x => x.value),
    };
    if ($(prefix + '-name')) body.name = $(prefix + '-name').value.trim();
    if (body.mode === 'auto') {
      body.auto = {allow: lines(prefix + '-auto-allow'), path_allow: lines(prefix + '-auto-paths'), minutes: +$(prefix + '-auto-min').value, max_actions: +$(prefix + '-auto-act').value};
      if (($(prefix + '-auto-grant') || {}).checked) body.auto.until_grant = true;
      body.confirm_auto = $(prefix + '-auto-ok').checked;
    }
    return body;
  }

  // ── setup snippets ────────────────────────────────────
  function snippets(url, tok) {
    const base = url.replace(/\/mcp$/, '');
    const out = {
      claude_desktop: {help: t('mcp_h_claude_desktop'), code: JSON.stringify({mcpServers: {wrm: {command: '/path/to/wrm-pro', args: ['-mcp-stdio', '-url', url], env: {WRM_MCP_TOKEN: tok}}}}, null, 2)},
      remote: {help: t('mcp_h_remote'), code: url},
      chatgpt: {help: t('mcp_h_chatgpt'), code: url},
      generic: {help: t('mcp_h_generic'), code: JSON.stringify({mcpServers: {wrm: {type: 'http', url, headers: {Authorization: 'Bearer ' + tok}}}}, null, 2) +
        `\n\nclaude mcp add --transport http wrm ${url} --header "Authorization: Bearer ${tok}"`},
      cli: {help: t('mcp_h_cli'), code: `curl -sS ${url} -H "Authorization: Bearer ${tok}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \\\n  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl"}}}'`},
    };
    void base;
    return out;
  }
  let snipState = {};
  function snippetsHTML(url, tok, which) {
    snipState = {url, tok};
    const all = snippets(url, tok);
    const keys = ['claude_desktop', 'remote', 'chatgpt', 'generic', 'cli'];
    which = which || keys[0];
    return `<div class="adm-h">🧩 ${esc(t('mcp_setup'))}</div><div class="mcp-tabs">${keys.map(k => `<button class="btn-sec btn-sm ${k === which ? 'active' : ''}" onclick="wrmMCP.snip('${k}')">${esc(t('mcp_setup_' + k))}</button>`).join('')}</div>
      <div class="hint-box">${esc(all[which].help)}</div><div class="mcp-snip"><pre id="mcp-snip-code">${esc(all[which].code)}</pre><button class="btn-sec btn-sm" onclick="wrmMCP.copy('mcp-snip-code')">${esc(t('mcp_copy'))}</button></div>`;
  }
  function snip(k) { const box = $('mcp-snips'); if (box) box.innerHTML = snippetsHTML(snipState.url, snipState.tok, k); }
  function copy(id) { const el = $(id); if (el) copyText(el.textContent); }

  // ── setup wizard: client → servers and access → ready config → test ──
  // WRM cannot write files on the user's computer, so it prepares everything: the token, the
  // exact command or file for the chosen app, the instructions that explain "wrm" to the AI,
  // and a test that talks to the MCP endpoint like the app will.
  const WCLIENTS = [
    {id: 'all_apps', icon: '🧰', token: false, via: 'mcpw_via_all'}, {id:'codex_oauth',icon:'⌨️',token:false}, {id: 'codex', icon: '⌨️', token: true}, {id: 'claude_desktop', icon: '🖥️', token: true, via: 'mcpw_via_extension'}, {id: 'claude_code', icon: '🟠', token: true}, {id: 'gemini', icon: '✨', token: true},
    {id: 'opencode', icon: '⬛', token: true}, {id: 'cursor', icon: '🧩', token: true}, {id: 'claude_connector', icon: '🟠', token: false}, {id: 'chatgpt', icon: '💬', token: false},
  ];
  const WINSTR = {opencode: 'AGENTS.md', codex: 'AGENTS.md', claude_code: 'CLAUDE.md', gemini: 'GEMINI.md', cursor: 'AGENTS.md'};
  let W = {step: 1, client: null, token: '', preset: 'read', hours: 0, conn_ids: []};
  function wizSteps() {
    const c = WCLIENTS.find(x => x.id === W.client);
    return c && !c.token ? ['mcpw_s_client', 'mcpw_s_connect'] : ['mcpw_s_client', 'mcpw_s_access', 'mcpw_s_connect', 'mcpw_s_test'];
  }
  function wizConfig(client, url, tok) {
    const tk = tok || 'YOUR_TOKEN';
    const hdr = {Authorization: 'Bearer ' + tk};
    const out = {};
    out.codex = [
      {h: 'mcpw_h_env', code: `export WRM_MCP_TOKEN='${tk}'\n# Windows PowerShell:\n$env:WRM_MCP_TOKEN='${tk}'`},
      {h: 'mcpw_h_codex_cmd', code: `codex mcp add wrm --url ${url} --bearer-token-env-var WRM_MCP_TOKEN`},
      {h: 'mcpw_h_codex_toml', code: `[mcp_servers.wrm]\nurl = "${url}"\nbearer_token_env_var = "WRM_MCP_TOKEN"\ntool_timeout_sec = 1200`, file: 'config.toml'},
    ];
    out.claude_code = [{h: 'mcpw_h_claude_cmd', code: `claude mcp add --transport http wrm ${url} --header "Authorization: Bearer ${tk}"`},
      {h: 'mcpw_cc_plugin', code: `claude plugin install wrm-pro --marketplace vedranius/wrm-pro-enterprise`}];
    out.claude_desktop = [{h: 'mcpw_desk_token', code: tk}];
    out.gemini = [
      {h: 'mcpw_h_gemini_cmd', code: `gemini mcp add --scope user --transport http wrm ${url} -H "Authorization: Bearer ${tk}"`},
      {h: 'mcpw_h_gemini_json', code: JSON.stringify({mcpServers: {wrm: {httpUrl: url, headers: hdr, description: cfg.description, trust: false}}}, null, 2), file: 'settings.json'},
    ];
    out.cursor = [{h: 'mcpw_h_json', code: JSON.stringify({mcpServers: {wrm: {type: 'http', url, headers: hdr}}}, null, 2), file: 'mcp.json'}];
    out.codex_oauth=[{h:'mcpw_h_codex_cmd',code:`codex mcp add wrm --url ${shellQuoteJS(url)}\ncodex mcp login wrm`}];
    out.opencode = [
      {h: 'mcpw_h_env', code: `export WRM_MCP_TOKEN='${tk}'\n# Windows PowerShell:\n$env:WRM_MCP_TOKEN='${tk}'`},
      {h: 'mcpw_h_opencode_json', code: JSON.stringify({$schema: 'https://opencode.ai/config.json', mcp: {wrm: {type: 'remote', url, enabled: true, headers: {Authorization: 'Bearer {env:WRM_MCP_TOKEN}'}}}}, null, 2), file: 'opencode.json'},
      {h: 'mcpw_h_opencode_oauth', code: JSON.stringify({$schema: 'https://opencode.ai/config.json', mcp: {wrm: {type: 'remote', url, enabled: true}}}, null, 2) + '\n\nopencode mcp auth wrm\nopencode mcp list'},
    ];
    out.claude_connector = [{h: 'mcpw_h_remote', code: url}];
    out.chatgpt = [{h: 'mcpw_h_chatgpt', code: url}];
    return out[client] || [];
  }
  // Claude Desktop: WRM builds the extension (manifest, bridge, icon) with this address and token
  async function desktopExtension() {
    try {
      const res = await fetch('/api/mcp/claude-desktop', {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json', 'X-WRM-Request': '1'}, body: JSON.stringify({token: W.token})});
      if (!res.ok) { let m = 'HTTP ' + res.status; try { m = (await res.json()).error || m; } catch (e) {} throw new Error(m); }
      const blob = await res.blob(), a = document.createElement('a');
      a.href = URL.createObjectURL(blob); a.download = 'wrm-pro.mcpb'; document.body.appendChild(a); a.click(); a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    } catch (e) { showToast(e.message, 'error', 6000); }
  }
  // all AI apps on this computer: the server builds the script with this WRM's address (no token)
  async function setupAll(os) {
    try {
      const res = await fetch('/api/mcp/setup-all', {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json', 'X-WRM-Request': '1'}, body: JSON.stringify({os})});
      if (!res.ok) { let m = 'HTTP ' + res.status; try { m = (await res.json()).error || m; } catch (e) {} throw new Error(m); }
      const blob = await res.blob(), a = document.createElement('a');
      a.href = URL.createObjectURL(blob); a.download = os === 'windows' ? 'wrm-setup-all.ps1' : 'wrm-setup-all.sh'; document.body.appendChild(a); a.click(); a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 1000);
    } catch (e) { showToast(e.message, 'error', 6000); }
  }
  function downloadSetup(name,text){const blob=new Blob([text],{type:'text/plain'}),a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(a.href),1000);}
  function cursorInstall(){const config={url:cfg.url,headers:{Authorization:'Bearer '+W.token}};location.href='cursor://anysphere.cursor-deeplink/mcp/install?name=wrm&config='+encodeURIComponent(btoa(unescape(encodeURIComponent(JSON.stringify(config)))));}
  function installer(kind){
    const app=W.client.startsWith('codex')?'codex':W.client==='claude_code'?'claude':'gemini',oauth=W.client==='codex_oauth';
    const bashQuote=s=>"'"+s.replaceAll("'", "'\\''")+"'",psQuote=s=>"'"+s.replaceAll("'","''")+"'";
    if(kind==='sh'){
      const commands=app==='codex'?`codex mcp add wrm --url ${bashQuote(cfg.url)}${oauth?'':' --bearer-token-env-var WRM_MCP_TOKEN'}\n${oauth?'codex mcp login wrm':'codex mcp list\nexec codex'}`:app==='claude'?`claude mcp add --scope user --transport http wrm ${bashQuote(cfg.url)} --header "Authorization: Bearer $WRM_MCP_TOKEN"\nclaude mcp list`:`gemini mcp add --scope user --transport http wrm ${bashQuote(cfg.url)} -H "Authorization: Bearer $WRM_MCP_TOKEN"\ngemini mcp list`;
      const backup=app==='codex'?`wrm_config="\${CODEX_HOME:-$HOME/.codex}/config.toml"\nif [ -f "$wrm_config" ]; then cp "$wrm_config" "$wrm_config.wrm-before-$(date +%Y%m%d%H%M%S)"; fi\nif codex mcp get wrm >/dev/null 2>&1; then codex mcp remove wrm; fi\n`:'';
      downloadSetup('wrm-setup-'+app+'.sh',`#!/usr/bin/env bash\nset -euo pipefail\numask 077\ncommand -v ${app} >/dev/null || { echo '${app} CLI is not installed. Install it using the vendor documentation, then run this script again.'; exit 1; }\n${oauth?'':'if [ -z "${WRM_MCP_TOKEN:-}" ]; then read -r -s -p "WRM token: " WRM_MCP_TOKEN; echo; fi\nexport WRM_MCP_TOKEN\n'}${backup}${commands}\n`);
    }else{
      const commands=app==='codex'?`& codex mcp add wrm --url ${psQuote(cfg.url)}${oauth?'':' --bearer-token-env-var WRM_MCP_TOKEN'}\nif ($LASTEXITCODE -ne 0) { throw 'WRM setup failed; restore the saved config backup if needed.' }\n${oauth?'& codex mcp login wrm':'& codex mcp list\n& codex'}`:app==='claude'?`& claude mcp add --scope user --transport http wrm ${psQuote(cfg.url)} --header "Authorization: Bearer $env:WRM_MCP_TOKEN"\n& claude mcp list`:`& gemini mcp add --scope user --transport http wrm ${psQuote(cfg.url)} -H "Authorization: Bearer $env:WRM_MCP_TOKEN"\n& gemini mcp list`;
      const backup=app==='codex'?`$wrmDir = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $env:USERPROFILE '.codex' }\n$wrmConfig = Join-Path $wrmDir 'config.toml'\nif (Test-Path $wrmConfig) { Copy-Item $wrmConfig ($wrmConfig + '.wrm-before-' + (Get-Date -Format yyyyMMddHHmmss)) }\n& codex mcp get wrm 2>$null | Out-Null\nif ($LASTEXITCODE -eq 0) { & codex mcp remove wrm }\n`:'';
      downloadSetup('wrm-setup-'+app+'.ps1',`$ErrorActionPreference = 'Stop'\nif (-not (Get-Command ${app} -ErrorAction SilentlyContinue)) { throw '${app} CLI is not installed. Install it using the vendor documentation first.' }\n${oauth?'':`if (-not $env:WRM_MCP_TOKEN) { $wrmSecure = Read-Host 'WRM token' -AsSecureString; $wrmPtr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($wrmSecure); try { $env:WRM_MCP_TOKEN = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($wrmPtr) } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($wrmPtr) } }\n`}${backup}${commands}\n`);
    }
  }
  function wizInstructions() { return `# WRM PRO (wrm)\n\n${cfg.instructions}\n`; }
  function wizPre(i, code) {
    return `<div class="mcp-snip"><pre id="mcpw-c${i}">${esc(code)}</pre><button class="btn-sec btn-sm" onclick="wrmMCP.copy('mcpw-c${i}')">${esc(t('mcp_copy'))}</button></div>`;
  }
  function wizHTML() {
    const steps = wizSteps();
    const bar = `<div class="mcpw-steps">${steps.map((s, i) => `<span class="${i + 1 === W.step ? 'on' : i + 1 < W.step ? 'done' : ''}">${i + 1}. ${esc(t(s))}</span>`).join('')}</div>`;
    const key = steps[W.step - 1];
    let body = '';
    if (key === 'mcpw_s_client') {
      body = `<div class="sub">${esc(t('mcpw_pick'))}</div><div class="mcpw-grid">${WCLIENTS.map(c => `<button class="btn-sec mcpw-card ${W.client === c.id ? 'active' : ''}" onclick="wrmMCP.wizClient('${c.id}')">
        <span>${c.icon} <b>${esc(t('mcpw_c_' + c.id))}</b></span><span class="sub">${esc(t(c.via || (c.token ? 'mcpw_via_token' : 'mcpw_via_oauth')))}</span></button>`).join('')}</div>`;
    } else if (key === 'mcpw_s_access') {
      const can = cfg.modes.includes('ask');
      body = `<div class="field"><label>${esc(t('mcp_conns'))}</label>${cfg.connections.length > 8 ? `<input type="text" id="mcpw-filter" placeholder="${esc(t('mcp_conns_filter'))}" style="margin-bottom:4px;" oninput="wrmMCP.filter('mcpw')">` : ''}
        <div class="mcp-conns" id="mcpw-conns">${cfg.connections.map(x => `<label class="mcp-check" data-name="${esc((x.name + ' ' + x.host).toLowerCase())}"><input type="checkbox" class="mcpw-conn" value="${x.id}" ${W.conn_ids.includes(x.id) ? 'checked' : ''}>
          <span>${esc(x.name)} <span class="sub">${esc(x.host)}</span></span></label>`).join('') || `<div class="sub">${esc(t('none'))}</div>`}</div></div>
        <div class="field"><label>${esc(t('mcpw_access'))}</label>
          <label class="mcp-check"><input type="radio" name="mcpw-preset" value="read" onchange="wrmMCP.wizPreset(this.value)" ${W.preset === 'read' ? 'checked' : ''}><span>${esc(t('mcpw_p_read'))} <span class="sub">${esc(t('mcpw_p_read_d'))}</span></span></label>
          ${cfg.modes.includes('auto') ? `<label class="mcp-check"><input type="radio" name="mcpw-preset" value="auto" onchange="wrmMCP.wizPreset(this.value)" ${W.preset === 'auto' ? 'checked' : ''}><span>${esc(t('mcp_mode_auto'))} <span class="sub">${esc(t('mcp_mode_auto_d'))}</span></span></label>` : ''}
          ${can ? `<label class="mcp-check"><input type="radio" name="mcpw-preset" value="ask" onchange="wrmMCP.wizPreset(this.value)" ${W.preset === 'ask' ? 'checked' : ''}><span>${esc(t('mcpw_p_ask'))} <span class="sub">${esc(t('mcpw_p_ask_d'))}</span></span></label>` : ''}</div>
        <div id="mcpw-auto-form" style="display:none;">${formHTML('mcpwa', cfg, {noName:true})}</div>
        <div class="field" style="max-width:260px;"><label>${esc(t('mcp_valid_for'))} <span class="sub">(${esc(lifetimeMax(cfg.max_hours))})</span></label>${lifetimeHTML('mcpw-hours', W.hours || cfg.default_hours)}</div>
        <div class="sub">${esc(t('mcpw_hours_d'))}</div>`;
    } else if (key === 'mcpw_s_connect') {
      const c = WCLIENTS.find(x => x.id === W.client);
      const parts = wizConfig(W.client, cfg.url, W.token);
      const cloud=['chatgpt','claude_connector'].includes(W.client), host=new URL(cfg.url).hostname, local=/^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.)/.test(host)||host==='[::1]'||host.endsWith('.local');
      const warning=cloud&&(new URL(cfg.url).protocol!=='https:'||local)?'mcpw_public_needed':local?'mcpw_local_url':'';
      body = `${warning?`<div class="hint-box warn">${esc(t(warning))}</div>`:''}${W.client==='claude_code'?`<div class="hint-box warn">${esc(t('mcpw_cc_shell'))}</div>`:''}${W.client==='claude_desktop'?`<div class="hint-box warn">${esc(t('mcpw_desk_no_plugin'))}</div>`:''}<div class="hint-box">${esc(t('mcpw_h_' + (c.token ? 'token' : 'oauth')))}</div>${c.token ? `<div class="hint-box warn">${esc(t('mcp_token_once'))}</div>` : ''}` +
        (W.client === 'all_apps' ? `<div class="hint-box">${esc(t('mcpw_all_h'))}</div><div style="margin:8px 0;"><button onclick="wrmMCP.setupAll('windows')">⬇ ${esc(t('mcpw_all_win'))}</button> <button onclick="wrmMCP.setupAll('unix')">⬇ ${esc(t('mcpw_all_unix'))}</button></div><div class="hint-box" style="white-space:pre-line;">${esc(t('mcpw_all_run'))}</div><div class="sub">${esc(t('mcpw_all_claude_app'))}</div>` : '') +
        (W.client === 'claude_desktop' ? `<div class="hint-box">${esc(t('mcpw_desk_h'))}</div><div style="margin:8px 0;"><button id="mcpw-desk-dl" onclick="wrmMCP.desktopExtension()">⬇ ${esc(t('mcpw_desk_dl'))}</button></div><div class="sub">${esc(t('mcpw_desk_dl_note'))}</div><div class="hint-box" style="white-space:pre-line;">${esc(t('mcpw_desk_steps'))}</div><div class="sub">${esc(t('mcpw_desk_noapp'))}</div>` : '') +
        parts.map((p, i) => `<div class="sub" style="margin-top:8px;">${esc(t(p.h))}</div>${wizPre(i, p.code)}${p.file ? `<button class="btn-sec btn-sm" onclick="wrmMCP.wizDownload(${i})">⬇ ${esc(t('mcpw_download'))} ${esc(p.file)}</button>` : ''}`).join('') +
        (['codex','codex_oauth','claude_code','gemini'].includes(W.client)?`<div class="hint-box">${esc(t('mcpw_installer_d'))}</div><button class="btn-sec btn-sm" onclick="wrmMCP.installer('sh')">${esc(t('mcpw_installer'))} (.sh)</button> <button class="btn-sec btn-sm" onclick="wrmMCP.installer('ps1')">${esc(t('mcpw_installer'))} (.ps1)</button>`:'')+
        (W.client==='cursor'?`<button class="btn-sec" onclick="wrmMCP.cursorInstall()">${esc(t('mcpw_cursor_install'))}</button>`:'')+`<div class="hint-box">${esc(t('mcpw_slash'))}</div>`+
        (WINSTR[W.client] ? `<div class="sub" style="margin-top:10px;">${esc(tf('mcpw_h_instr', {file: WINSTR[W.client]}))}</div>${wizPre('i', wizInstructions())}<button class="btn-sec btn-sm" onclick="wrmMCP.wizDownload('i')">⬇ ${esc(t('mcpw_download'))} ${esc(WINSTR[W.client])}</button>` : '');
    } else {
      body = `<div class="sub">${esc(t('mcpw_test_d'))}</div><div style="margin:8px 0;"><button onclick="wrmMCP.wizTest()">▶ ${esc(t('mcpw_test'))}</button></div><div id="mcpw-result"></div>`;
    }
    const last = W.step === steps.length;
    const nextLabel = key === 'mcpw_s_access' ? t('mcpw_create') : t('mcpw_next');
    const nav = `<div class="mcpw-nav">${W.step > 1 && !(key === 'mcpw_s_connect' && W.token) && !(key === 'mcpw_s_test') ? `<button class="btn-sec btn-sm" onclick="wrmMCP.wizGo(-1)">← ${esc(t('mcpw_back'))}</button>` : ''}
      ${last ? `<button class="btn-sm" onclick="wrmMCP.wizReset()">✓ ${esc(t('mcpw_done'))}</button>` : key === 'mcpw_s_client' ? '' : `<button class="btn-sm" onclick="wrmMCP.wizNext()">${esc(nextLabel)} →</button>`}</div>`;
    return `<div class="adm-h">🧭 ${esc(t('mcpw_title'))}</div><div class="sub" style="margin-bottom:6px;">${esc(t('mcpw_intro'))}</div>${bar}${body}${nav}`;
  }
  function wizRender() { const el = $('mcpw'); if (el) el.innerHTML = wizHTML(); if($('mcpwa-mode')){$('mcpwa-mode').value='auto';modeChanged('mcpwa');const h=$('mcpwa-hours');if(h)h.closest('.field').style.display='none';} }
  function wizPreset(mode){W.preset=mode;$('mcpw-auto-form').style.display=mode==='auto'?'':'none';}
  function wizClient(id) { W.client = id; W.step = 2; W.token = ''; wizRender(); }
  function wizGo(d) { W.step = Math.max(1, W.step + d); wizRender(); }
  function wizReset() { W = {step: 1, client: null, token: '', preset: 'read', hours: 0, conn_ids: []}; wizRender(); settingsPane(); }
  async function wizNext() {
    const key = wizSteps()[W.step - 1];
    if (key === 'mcpw_s_access') {
      W.conn_ids = [...document.querySelectorAll('.mcpw-conn:checked')].map(x => +x.value);
      const pr = document.querySelector('input[name=mcpw-preset]:checked');
      W.preset = pr ? pr.value : 'read';
      if ($('mcpw-hours')) W.hours = Math.min(lifetimeValue('mcpw-hours') || cfg.default_hours, cfg.max_hours);
      if (!W.conn_ids.length) { showToast(t('mcpw_pick_server'), 'error'); return; }
      const scopes = (W.preset !== 'read' ? ['read_logs', 'run_readonly', 'run_with_approval', 'edit_file_with_approval'] : ['read_logs', 'run_readonly']).filter(s => cfg.scopes.includes(s));
      const c = WCLIENTS.find(x => x.id === W.client);
      let r;
      if (W.preset === 'auto') { $('mcpwa-mode').value = 'auto'; modeChanged('mcpwa'); }
      try { r = await api('POST', '/api/mcp/grants', { ...(W.preset === 'auto' ? formValue('mcpwa') : {}), name: tf('mcpw_grant_name', {client: t('mcpw_c_' + c.id)}), conn_ids: W.conn_ids, mode: W.preset === 'auto' ? 'auto' : W.preset === 'ask' ? 'ask' : 'read_only', scopes, hours: W.hours || cfg.default_hours}); }
      catch (e) { showToast(e.message, 'error', 6000); return; }
      W.token = r.token;
    }
    W.step++;
    wizRender();
  }
  function wizDownload(i) {
    const parts = wizConfig(W.client, cfg.url, W.token);
    const isInstr = i === 'i';
    const name = isInstr ? WINSTR[W.client] : parts[i].file;
    const blob = new Blob([isInstr ? wizInstructions() : parts[i].code + '\n'], {type: 'text/plain'});
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob); a.download = name; document.body.appendChild(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  }
  // the test calls the MCP endpoint with the new token, exactly like the app will
  async function wizTest() {
    const out = $('mcpw-result');
    if (!out) return;
    out.innerHTML = `<div class="sub">${esc(t('loading'))}</div>`;
    const H = {'Content-Type': 'application/json', Accept: 'application/json, text/event-stream', Authorization: 'Bearer ' + W.token};
    const rpc = async (body, extra) => {
      const res = await fetch(cfg.url, {method: 'POST', headers: Object.assign({}, H, extra || {}), body: JSON.stringify(body)});
      if (!res.ok) throw new Error('HTTP ' + res.status);
      return res;
    };
    try {
      const r1 = await rpc({jsonrpc: '2.0', id: 1, method: 'initialize', params: {protocolVersion: '2025-06-18', capabilities: {}, clientInfo: {name: 'WRM setup test'}}});
      const sid = r1.headers.get('Mcp-Session-Id') || '';
      const j1 = await r1.json();
      const sh = sid ? {'Mcp-Session-Id': sid} : {};
      await rpc({jsonrpc: '2.0', method: 'notifications/initialized'}, sh).catch(() => {});
      const j2 = await (await rpc({jsonrpc: '2.0', id: 2, method: 'tools/list'}, sh)).json();
      const n = ((j2.result || {}).tools || []).length;
      const name = ((j1.result || {}).serverInfo || {}).title || 'WRM';
      out.innerHTML = `<div class="hint-box" style="border-color:var(--green,var(--green));">✅ ${esc(tf('mcpw_test_ok', {name, n}))}</div>`;
    } catch (e) {
      out.innerHTML = `<div class="hint-box warn">❌ ${esc(tf('mcpw_test_fail', {err: e.message}))}</div>`;
    }
  }

  // ── Settings → AI connections ─────────────────────────
  async function settingsPane() {
    const pane = $('mcp-pane');
    if (!pane) return;
    let grants;
    try { [cfg, grants] = await Promise.all([api('GET', '/api/mcp/config'), api('GET', '/api/mcp/grants')]); }
    catch (e) { pane.innerHTML = `<div class="fm-error">${esc(e.message)}</div>`; return; }
    if (!cfg.enabled) { pane.innerHTML = `<div class="hint-box warn">${esc(t('mcp_off'))}: ${esc(cfg.reason)}</div>${fullAccessHTML(cfg.full_access)}`; return; }
    pane.innerHTML = `<div class="hint-box">${esc(t('mcp_intro'))}</div>
      <div class="hint-box" style="margin:8px 0;">
        <b>🔌 ${esc(t('mcp_quick_title'))}</b>
        <div class="sub" style="margin:3px 0 6px;">${esc(t('mcp_quick_d'))}</div>
        <div class="sub" style="margin:0 0 6px;font-size:10.5px;color:var(--text3);">${esc(t('mcp_quick_which'))}</div>
        <div class="sub" style="font-size:10.5px;color:var(--text3);">${esc(t('mcp_quick_cmd'))}</div>
        <div class="mcp-snip"><pre id="mcp-quick-cmd">claude mcp add --transport http --scope user wrm ${esc(cfg.url)} --header "Authorization: Bearer YOUR_TOKEN"</pre><button class="btn-sec btn-sm" onclick="wrmMCP.copy('mcp-quick-cmd')">${esc(t('mcp_copy'))}</button></div>
        <div style="display:flex;gap:6px;flex-wrap:wrap;margin-top:6px;">
          <button class="btn-sm" onclick="wrmMCP.wizClient('all_apps')">🧰 ${esc(t('mcp_quick_all'))} →</button>
          <button class="btn-sec btn-sm" onclick="wrmMCP.wizClient('claude_code')">${esc(t('mcp_quick_cc'))} →</button>
          <button class="btn-sec btn-sm" onclick="wrmMCP.wizClient('claude_desktop')">${esc(t('mcp_quick_desk'))} →</button>
          <button class="btn-sec btn-sm" onclick="wrmMCP.wizClient('claude_connector')">${esc(t('mcp_quick_cloud'))} →</button>
        </div>
      </div>
      <div id="mcpw" class="mcpw"></div>
      <div class="field" style="margin:8px 0;"><label style="font-size:11px;color:var(--text2);">${esc(t('mcp_url'))}</label><div class="mcp-snip"><pre id="mcp-url">${esc(cfg.url)}</pre><button class="btn-sec btn-sm" onclick="wrmMCP.copy('mcp-url')">${esc(t('mcp_copy'))}</button></div>
        <div class="sub" style="font-size:11px;color:var(--text3);">${esc(cfg.oauth ? t('mcp_oauth_box') : t('mcp_oauth_off'))}</div></div>
      ${fullAccessHTML(cfg.full_access)}
      <div style="display:flex;align-items:center;gap:8px;"><div class="adm-h" style="flex:1;">🔌 ${esc(t('mcp_title'))}</div>${grants.some(g => g.state === 'active') ? `<button class="btn-danger btn-sm" onclick="wrmMCP.revokeAll()">${esc(t('mcp_revoke_all'))}</button>` : ''}</div>
      <div id="mcp-grants">${grantsHTML(grants)}</div>
      <details class="mcpw-adv"><summary>${esc(t('mcpw_advanced'))}</summary>
      <div class="adm-h">＋ ${esc(t('mcp_new'))}</div>${formHTML('mcpn', cfg)}
      <div style="margin-top:8px;"><button onclick="wrmMCP.create()">${esc(t('mcp_create'))}</button></div>
      <div id="mcp-created"></div></details>`;
    wizRender();
  }
  // full access as the administrator set it up: on for everyone, per AI connection, or per user
  function fullAccessHTML(fa) {
    if (!fa || !fa.on) return '';
    if (fa.mode === 'user') return `<label class="mcp-check hint-box warn" style="margin:8px 0;"><input type="checkbox" ${fa.mine ? 'checked' : ''} onchange="wrmMCP.setFullAccess(this.checked)"><span>${esc(t('mcp_fa_user'))}</span></label>`;
    return `<div class="hint-box warn" style="margin:8px 0;">${esc(t(fa.mode === 'connection' ? 'mcp_fa_conn' : 'mcp_fa_all'))}</div>`;
  }
  async function setFullAccess(on) {
    try { await api('POST', '/api/mcp/full-access', {on}); } catch (e) { showToast(e.message, 'error', 6000); }
    settingsPane();
  }
  let grantViews = [];
  function grantsHTML(list) {
    if (!list.length) return `<div class="sub" style="font-size:11.5px;color:var(--text3);">${esc(t('mcp_none'))}</div>`;
    grantViews = list;
    return list.map(g => {
      const st = g.state === 'active' ? (g.live ? `<span class="pill ok">● ${esc(t('mcp_live'))}</span>` : '') : `<span class="pill bad">${esc(t('mcp_' + g.state))}</span>`;
      return `<div class="mcp-grant"><div class="g-main"><b>${esc(g.name)}</b> <span class="pill">${esc(t('mcp_kind_' + g.kind))}</span> ${st}
        <div class="sub">${esc(g.connections.map(c => c.name).join(', ') || '—')} · ${esc(t('mcp_mode_' + g.mode))} · <span class="mono">${esc(g.scopes.join(', '))}</span></div>
        <div class="sub">${g.state === 'active' ? esc(tf('mcp_expires', {time: rel(g.expires_at)})) : ''}${g.last_used_at ? ' · ' + esc(tf('mcp_last_used', {time: rel(g.last_used_at)})) : ' · ' + esc(t('mcp_never_used'))}${g.hint ? ` · …${esc(g.hint)}` : ''}${g.sessions.length ? ' · ' + esc(tf('mcp_sessions_n', {n: g.sessions.length})) : ''}</div></div>
        ${g.state === 'active' ? `<button class="btn-sec btn-sm" onclick="wrmMCP.editGrant(${g.id})">${esc(t('edit'))}</button><button class="btn-danger btn-sm" onclick="wrmMCP.revoke(${g.id})">${esc(t('mcp_revoke'))}</button>` : ''}</div>`;
    }).join('');
  }
  async function create() {
    const body = formValue('mcpn');
    let r;
    try { r = await api('POST', '/api/mcp/grants', body); } catch (e) { showToast(e.message, 'error', 6000); return; }
    await settingsPane();
    $('mcp-created').innerHTML = `<div class="hint-box warn" style="margin-top:12px;">${esc(t('mcp_token_once'))}</div>
      <div class="mcp-snip"><pre id="mcp-token">${esc(r.token)}</pre><button class="btn-sec btn-sm" onclick="wrmMCP.copy('mcp-token')">${esc(t('mcp_copy'))}</button></div>
      <div id="mcp-snips">${snippetsHTML(r.url, r.token)}</div>`;
    $('mcp-created').scrollIntoView({block: 'nearest'});
  }
  async function editGrant(id) {
    const g = grantViews.find(x => x.id === id); if (!g) return;
    const box = document.createElement('div'); box.id = 'mcp-edit'; box.className = 'modal-overlay open';
    box.innerHTML = `<div class="modal" style="width:560px;max-height:90vh;overflow:auto;"><h3>${esc(t('edit'))} · ${esc(g.name)}</h3>${formHTML('mcpe', cfg, g)}<div class="hint-box">${esc(t('mcp_edit_note'))}</div><div class="modal-footer"><button class="btn-sec" onclick="document.getElementById('mcp-edit').remove()">${esc(t('cancel'))}</button><button onclick="wrmMCP.saveGrant(${id})">${esc(t('save'))}</button></div></div>`;
    document.body.appendChild(box);
    $('mcpe-name').value = g.name; $('mcpe-mode').value = g.mode;
    document.querySelectorAll('.mcpe-conn').forEach(x => x.checked = g.connections.some(c => c.id === +x.value));
    $('mcpe-auto-allow').value = (g.auto?.allow || []).join('\n'); $('mcpe-auto-paths').value = (g.auto?.path_allow || []).join('\n');
    if (g.auto) { $('mcpe-auto-min').value = g.auto.minutes || 30; $('mcpe-auto-act').value = g.auto.max_actions || 20; }
    if (g.auto?.until_grant && $('mcpe-auto-grant')) { $('mcpe-auto-grant').checked = true; untilGrant('mcpe'); }
    $('mcpe-hours').closest('.field').style.display = 'none'; modeChanged('mcpe');
  }
  async function saveGrant(id) {
    try { await api('PUT', '/api/mcp/grants/' + id, formValue('mcpe')); $('mcp-edit').remove(); after(); }
    catch(e) { showToast(e.message, 'error'); }
  }
  async function revoke(id) {
    if (!(await uiConfirm(t('mcp_revoke_q'), {okText: t('mcp_revoke'), danger: true}))) return;
    try { await api('DELETE', `/api/mcp/grants/${id}`); } catch (e) { showToast(e.message, 'error'); }
    after();
  }
  async function revokeAll() {
    if (!(await uiConfirm(t('mcp_revoke_all_q'), {okText: t('mcp_revoke_all'), danger: true}))) return;
    try { const r = await api('POST', '/api/mcp/revoke-all'); showToast(tf('mcp_revoked_n', {n: r.revoked}), 'success'); } catch (e) { showToast(e.message, 'error'); }
    after();
  }
  function after() {
    if ($('mcp-pane') && $('st-mcp') && $('st-mcp').classList.contains('active')) settingsPane();
    if (typeof adminTab !== 'undefined' && adminTab === 'mcp' && $('admin-body')) adminMCP();
    refresh();
  }

  // ── the "AI connected" indicator and approval cards ───
  let lastActive = {live: [], pending: []};
  async function refresh() {
    let d;
    try { d = await api('GET', '/api/mcp/active'); } catch (_) { return; }
    indicator(d);
  }
  function indicator(d) {
    lastActive = d;
    const btn = $('btn-mcp');
    if (btn) {
      // Only touch what changed: every write is a mutation the top bar re-measures itself for.
      const on = !!(d.live.length || d.pending.length), disp = on ? '' : 'none';
      if (btn.style.display !== disp) btn.style.display = disp;
      const bar = $('top-bar'); if (bar && bar.classList.contains('mcp-on') !== on) bar.classList.toggle('mcp-on', on);
      const b = $('mcp-badge');
      if (b) {
        const n = String(d.pending.length || d.live.length || ''), bg = d.pending.length ? 'var(--yellow)' : '';
        if (b.textContent !== n) b.textContent = n;
        if (b.style.background !== bg) b.style.background = bg;
        const bd = n ? '' : 'none';
        if (b.style.display !== bd) b.style.display = bd;
      }
    }
    // A new approval is announced with a sound, a toast and a desktop notification:
    // the live view may be hidden, so the card alone is not enough.
    const now = new Set((d.pending || []).map(p => p.approval && p.approval.id).filter(Boolean));
    for (const id of Object.keys(seenPending)) if (!now.has(id)) delete seenPending[id];
    for (const p of d.pending || []) {
      const a = p.approval || {};
      if (!a.id || seenPending[a.id]) continue;
      seenPending[a.id] = true;
      if (window.wrmAiAlert) {
        const what = a.command || a.path || a.transfer || a.tool || '';
        wrmAiAlert('approval', tf('mcp_ap_title', {client: p.client, server: p.connection}) + (what ? ' · ' + what : ''), 'approval-' + a.id);
      }
    }
    if ($('mcp-pop') && $('mcp-pop').classList.contains('open')) renderPop();
    cards(d.pending);
    // a new MCP session opens its live view once (closing it is respected)
    for (const g of d.live) for (const x of g.sessions || []) {
      if (seenSess[x.id]) continue;
      seenSess[x.id] = true;
      if (watchAuto()) watch(x.id, false, g.client_name || g.name);
    }
  }

  // ── live view of MCP sessions: what the AI runs on the server, approvals and output ──
  const seenSess = {}, W8 = {}, seenPending = {};
  function watchAuto(on) {
    try {
      if (on === undefined) return localStorage.getItem('wrm_mcp_watch') !== '0';
      localStorage.setItem('wrm_mcp_watch', on ? '1' : '0');
    } catch (e) { /* storage unavailable: default on */ }
    return true;
  }
  // ── AI panel layout: the live view is a panel of the page layout (static/layout.js), so
  // the user docks it left, right or at the bottom, stacks it with other panels, lets it float
  // or hides it like any other panel. On a phone it is a sheet at the bottom instead. Approval
  // cards (#mcp-cards) always pop up, whatever the layout.
  let sheetMin = false;
  const useLayout = () => !!(window.wrmLayout && wrmLayout.ready() && !(typeof isCompact === 'function' && isCompact()));
  const aiHidden = () => (useLayout() ? wrmLayout.isHidden('ai') : sheetMin);
  function dockRoot() {
    let root = $('mcp-watch');
    if (root) return root;
    root = document.createElement('div'); root.id = 'mcp-watch';
    root.innerHTML = `<div class="ad-head" id="ad-head"><span>🤖</span><span class="ad-t" id="ad-title"></span>
      <button class="icon-btn" data-mode="right" title="${esc(t('ad_right_t'))}" aria-label="${esc(t('ad_right_t'))}" onclick="wrmMCP.dockMode('right')">▕</button>
      <button class="icon-btn" data-mode="bottom" title="${esc(t('ad_bottom_t'))}" aria-label="${esc(t('ad_bottom_t'))}" onclick="wrmMCP.dockMode('bottom')">▁</button>
      <button class="icon-btn" data-mode="float" title="${esc(t('ad_float_t'))}" aria-label="${esc(t('ad_float_t'))}" onclick="wrmMCP.dockMode('float')">⧉</button>
      <button class="icon-btn" title="${esc(t('ad_min_t'))}" aria-label="${esc(t('ad_min_t'))}" onclick="wrmMCP.dockHide(true)">–</button></div>
      <div class="ad-body" id="ad-body"></div>`;
    document.body.appendChild(root);
    return root;
  }
  function dockCount() { return Object.keys(W8).length; }
  function dockWaiting() { return document.querySelectorAll('#mcp-watch .wa .acts').length; }
  // dockPlace puts the live view where the layout says. fresh: a session was just added or the
  // user asked for it, so the panel is brought to the front of its group.
  function dockPlace(fresh) {
    const root = dockRoot(), n = dockCount();
    if (useLayout()) {
      const host = $('rp-ai');
      if (host && root.parentElement !== host) host.appendChild(root);
      root.style.left = root.style.top = root.style.bottom = root.style.width = root.style.height = '';
      wrmLayout.refresh();
      if (fresh && n > 0 && !wrmLayout.isHidden('ai')) wrmLayout.activate('ai', {reveal: true});
    } else {
      if (root.parentElement !== document.body) document.body.appendChild(root);
      if (window.wrmLayout && wrmLayout.ready()) wrmLayout.refresh();
    }
    dockStyle();
    if (typeof scheduleRelayout === 'function') scheduleRelayout();
  }
  // dockStyle follows the panel's place without moving anything (layout.js calls it after
  // every change): a row of sessions at the bottom, a column elsewhere, a sheet on a phone.
  function dockStyle() {
    const root = $('mcp-watch');
    if (!root) return;
    const lay = useLayout(), w = lay ? wrmLayout.where('ai') : 'float';
    const mode = !lay ? 'float' : w === 'bottom' ? 'bottom' : 'right';
    root.classList.remove('m-right', 'm-bottom', 'm-float');
    root.classList.add('m-' + mode);
    root.classList.toggle('on', dockCount() > 0 && !aiHidden());
    root.querySelectorAll('.ad-head [data-mode]').forEach(b => { b.classList.toggle('on', lay && b.dataset.mode === (w === 'left' ? 'right' : w)); b.style.display = lay ? '' : 'none'; });
    dockBadges();
  }
  function dockBadges() {
    const n = dockCount(), w = dockWaiting();
    const title = $('ad-title');
    if (title) title.innerHTML = `${esc(t('ad_title'))} · ${n}${w ? `<span class="ad-wait">⏳ ${esc(tf('ad_waiting', {n: w}))}</span>` : ''}`;
    const b = $('rp-ai-badge');
    if (b) { b.textContent = n || ''; b.classList.toggle('wait', w > 0); b.title = w ? tf('ad_waiting', {n: w}) : ''; }
    let chip = $('ai-dock-chip');
    if (!chip) {
      chip = document.createElement('button'); chip.id = 'ai-dock-chip'; chip.type = 'button';
      chip.onclick = () => dockHide(false);
      document.body.appendChild(chip);
    }
    chip.title = t('ad_chip_t');
    chip.innerHTML = `🤖 <span>${esc(t('ad_show'))}</span> <span class="rp-badge${w ? ' wait' : ''}">${w || n}</span>`;
    chip.classList.toggle('on', n > 0 && aiHidden());
    chip.classList.toggle('wait', w > 0);
  }
  function dockMode(mode) {
    if (!['right', 'bottom', 'float', 'left'].includes(mode)) return;
    if (useLayout()) {
      if (wrmLayout.isHidden('ai')) wrmLayout.show('ai');
      if (wrmLayout.where('ai') !== mode || mode === 'float') wrmLayout.moveTo('ai', mode);
    }
    dockPlace(true);
    if ($('mcp-pop') && $('mcp-pop').classList.contains('open')) renderPop();
  }
  function dockHide(min) {
    if (useLayout()) { if (min) wrmLayout.hide('ai'); else wrmLayout.show('ai'); } else sheetMin = !!min;
    dockPlace(!min);
    if ($('mcp-pop') && $('mcp-pop').classList.contains('open')) renderPop();
  }
  (function dockInit() {
    let compact = typeof isCompact === 'function' && isCompact();
    window.addEventListener('resize', () => {
      const c = typeof isCompact === 'function' && isCompact();
      if (c !== compact) { compact = c; if (dockCount()) setTimeout(() => dockPlace(false), 80); }
    });
  })();

  function watch(sid, focus, client) {
    dockRoot();
    if (W8[sid]) {
      W8[sid].el.classList.remove('min');
      if (focus) { if (aiHidden()) dockHide(false); else dockPlace(true); W8[sid].el.scrollIntoView({block: 'nearest'}); }
      return;
    }
    const el = document.createElement('div');
    el.className = 'mcp-wbox'; el.id = 'mcp-w-' + sid;
    el.innerHTML = `<div class="wh"><span>🤖</span><b id="mcp-wt-${sid}">${esc(client || 'AI')}</b>
      <button class="icon-btn" title="${esc(t('ad_box_min'))}" onclick="this.closest('.mcp-wbox').classList.toggle('min')">▾</button>
      <button class="icon-btn" title="${esc(t('mcp_w_close'))}" onclick="wrmMCP.unwatch(${sid})">✕</button></div>
      <div class="wb" id="mcp-wb-${sid}"><div class="sub">${esc(t('mcp_w_empty'))}</div></div>`;
    $('ad-body').appendChild(el);
    const st = W8[sid] = {el, seq: 0, calls: {}, client: client || 'AI'};
    if (focus && aiHidden()) dockHide(false);
    else dockPlace(true);
    const es = st.es = new EventSource(`/api/ai/sessions/${sid}/events?since=0`);
    ['session', 'tool_call', 'approval_required', 'approval_resolved', 'approval_deferred', 'tool_result', 'error', 'ended']
      .forEach(ty => es.addEventListener(ty, ev => { try { watchEvent(sid, JSON.parse(ev.data)); } catch (e) { console.warn(e); } }));
    es.onerror = () => { if (st.ended) es.close(); };
  }
  function unwatch(sid) {
    const st = W8[sid];
    if (!st) return;
    if (st.es) st.es.close();
    st.el.remove();
    delete W8[sid];
    dockPlace(false);
  }
  function wAdd(sid, html) {
    const b = $('mcp-wb-' + sid);
    if (!b) return null;
    const near = b.scrollHeight - b.scrollTop - b.clientHeight < 120;
    b.insertAdjacentHTML('beforeend', html);
    if (near) b.scrollTop = b.scrollHeight;
    return b.lastElementChild;
  }
  function watchEvent(sid, ev) {
    const st = W8[sid];
    if (!st || ev.seq <= st.seq) return;
    st.seq = ev.seq;
    const d = ev.data || {};
    switch (ev.type) {
      case 'session': {
        const tt = $('mcp-wt-' + sid);
        if (tt) tt.textContent = tf('mcp_w_title', {client: st.client, server: d.connection || ''}) + (d.mode ? ' · ' + t('mcp_mode_' + d.mode) : '');
        if (d.ended) st.ended = true;
        break;
      }
      case 'tool_call': {
        const cls = d.decision === 'deny' ? 'bad' : d.decision === 'approve' ? 'ask' : '';
        const lbl = d.decision === 'deny' ? t('mcp_w_refused') : d.decision === 'approve' ? t('mcp_w_ask') : t('mcp_w_runs');
        st.calls[d.call_id] = wAdd(sid, `<div class="we ${cls}"><div><b>${esc(d.tool)}</b> · <span class="sub">${esc(lbl)}</span></div>
          ${d.display ? `<pre>${esc(d.display)}</pre>` : ''}${d.diff && d.decision !== 'approve' ? `<pre>${diffHTML(d.diff)}</pre>` : ''}
          ${d.reason ? `<div class="sub">${esc(t('mcp_ap_reason'))}: ${esc(d.reason)}</div>` : ''}${d.why && d.decision !== 'allow' ? `<div class="sub">${esc(t('mcp_w_reason'))}: ${esc(d.why)}</div>` : ''}
          <div class="wa"></div><div class="wr"></div></div>`);
        break;
      }
      case 'approval_required': {
        const a = d.approval || {}, el = st.calls[a.call_id];
        if (el) el.querySelector('.wa').innerHTML = `<div class="sub">⏳ ${esc(t('mcp_w_wait'))}</div><div class="acts">
          <button class="btn-sm" onclick="wrmMCP.decide(${sid}, '${esc(a.id)}', true)">✔ ${esc(t('mcp_ap_approve'))}</button>
          <button class="btn-danger btn-sm" onclick="wrmMCP.decide(${sid}, '${esc(a.id)}', false)">✕ ${esc(t('mcp_ap_deny'))}</button></div>`;
        st.el.classList.remove('min');
        dockBadges();
        break;
      }
      case 'approval_deferred': {
        // nobody answered in time (or the app declined): it can still be approved here
        const a = d.approval || {}, el = st.calls[a.call_id];
        if (el) {
          el.classList.remove('bad'); el.classList.add('ask');
          el.querySelector('.wa').innerHTML = `<div class="sub">⏳ ${esc(t(a.run_now ? 'mcp_w_later_run' : 'mcp_w_later'))}</div><div class="acts">
          <button class="btn-sm" onclick="wrmMCP.decide(${sid}, '${esc(a.id)}', true)">✔ ${esc(t('mcp_ap_approve'))}</button>
          <button class="btn-danger btn-sm" onclick="wrmMCP.decide(${sid}, '${esc(a.id)}', false)">✕ ${esc(t('mcp_ap_deny'))}</button></div>`;
        }
        st.el.classList.remove('min');
        dockBadges();
        break;
      }
      case 'approval_resolved': {
        const el = st.calls[d.call_id];
        if (d.later && d.outcome !== 'approved' && d.outcome !== 'denied') break; // approval_deferred follows
        if (el) {
          el.classList.remove('ask'); el.classList.add(d.outcome === 'approved' ? 'ok' : 'bad');
          el.querySelector('.wa').innerHTML = `<div class="sub">${d.outcome === 'approved' ? '✔' : '✕'} ${esc(t('mcp_w_o_' + d.outcome))}${d.by ? ' · ' + esc(d.by) : ''}${String(d.via || '').startsWith('mcp') ? ' · ' + esc(t('mcp_w_mcp_app')) : ''}${d.later && d.outcome === 'approved' ? ' · ' + esc(t(d.run_now ? 'mcp_w_later_running' : 'mcp_w_later_ok')) : ''}</div>`;
        }
        dockBadges();
        break;
      }
      case 'tool_result': {
        if (d.later_run) {
          const el = st.calls[d.call_id];
          const html = `<div class="sub">↪ ${esc(tf('mcp_w_later_handed', {time: fmtTime(d.ran_at)}))}</div>`;
          if (el) el.querySelector('.wr').innerHTML = html; else wAdd(sid, `<div class="we">${html}</div>`);
          break;
        }
        const out = d.output != null ? String(d.output) : '';
        const html = d.error ? `<div class="sub" style="color:var(--red-t);">${esc(d.error)}</div>`
          : `<div class="sub">${esc(tf('mcp_w_exit', {code: d.exit_code}))}</div>${out ? `<details ${out.length < 800 ? 'open' : ''}><summary>${esc(t('mcp_w_output'))} (${out.length} B)</summary><pre>${esc(out.length > 20000 ? out.slice(0, 20000) + '\n…' : out)}</pre></details>` : ''}`;
        const el = st.calls[d.call_id];
        if (el) el.querySelector('.wr').innerHTML = html; else wAdd(sid, `<div class="we">${html}</div>`);
        break;
      }
      case 'error': wAdd(sid, `<div class="we bad"><div class="sub">${esc(d.text || d.error || '')}</div></div>`); break;
      case 'ended':
        st.ended = true;
        if (st.es) st.es.close();
        wAdd(sid, `<div class="sub">⏻ ${esc(tf('mcp_w_ended', {reason: d.reason || ''}))}</div>`);
        break;
    }
  }
  function renderPop() {
    const d = lastActive;
    $('mcp-pop').innerHTML = `<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;"><b style="flex:1;font-size:13px;">🤖 ${esc(t('mcp_ind_title'))}</b>
      ${d.live.length ? `<button class="btn-danger btn-sm" onclick="wrmMCP.revokeAll()">⏻ ${esc(t('mcp_revoke_all'))}</button>` : ''}<button class="icon-btn" onclick="wrmMCP.togglePop(false)">✕</button></div>` +
      (d.live.length ? d.live.map(g => `<div class="mcp-grant"><div class="g-main"><b>${esc(g.client_name || g.name)}</b> <span class="pill ok">● ${esc(t('mcp_live'))}</span>
        <div class="sub">${esc(tf('mcp_on', {servers: (g.sessions.length ? g.sessions.map(s => s.connection) : g.connections.map(c => c.name)).join(', ')}))} · ${esc(t('mcp_mode_' + g.mode))}</div>
        <div class="sub">${esc(tf('mcp_expires', {time: rel(g.expires_at)}))}</div>
        ${(g.sessions || []).map(x => `<button class="btn-sec btn-sm" style="margin-top:4px;" onclick="wrmMCP.watch(${x.id}, true)">👁 ${esc(t('mcp_w_watch'))}: ${esc(x.connection)}</button>`).join(' ')}</div><button class="btn-danger btn-sm" onclick="wrmMCP.revoke(${g.id})">${esc(t('mcp_revoke'))}</button></div>`).join('')
        : `<div class="sub" style="font-size:11.5px;color:var(--text3);">${esc(t('mcp_none'))}</div>`) +
      `<label class="mcp-check" style="margin-top:8px;"><input type="checkbox" ${watchAuto() ? 'checked' : ''} onchange="wrmMCP.watchAuto(this.checked)"><span>${esc(t('mcp_w_auto'))}</span></label>` +
      (() => { const lay = useLayout(), w = lay ? wrmLayout.where('ai') : ''; return lay ? `<div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-top:8px;"><span class="sub" style="font-size:11.5px;">${esc(t('ad_layout'))}:</span><span class="ad-seg">${['right', 'bottom', 'float'].map(m =>
        `<button type="button" class="${w === m ? 'active' : ''}" title="${esc(t('ad_' + m + '_t'))}" onclick="wrmMCP.dockMode('${m}')">${esc(t('ad_' + m))}</button>`).join('')}</span>${aiHidden() && dockCount() ? `<button class="btn-sec btn-sm" onclick="wrmMCP.dockHide(false)">🤖 ${esc(t('ad_show'))}</button>` : ''}</div>` : (sheetMin && dockCount() ? `<div style="margin-top:8px;"><button class="btn-sec btn-sm" onclick="wrmMCP.dockHide(false)">🤖 ${esc(t('ad_show'))}</button></div>` : ''); })() +
      `<div style="margin-top:8px;"><button class="btn-sec btn-sm" onclick="wrmMCP.togglePop(false); openSettings('st-mcp')">⚙ ${esc(t('mcp_title'))}</button></div>`;
  }
  function togglePop(open) {
    let p = $('mcp-pop');
    if (!p) { p = document.createElement('div'); p.id = 'mcp-pop'; document.body.appendChild(p); }
    if (open === undefined) open = !p.classList.contains('open');
    p.classList.toggle('open', open);
    if (open) { renderPop(); refresh(); }
  }
  const shown = {};
  let cardTimer = null;
  function cards(pending) {
    let box = $('mcp-cards');
    if (!box) { box = document.createElement('div'); box.id = 'mcp-cards'; document.body.appendChild(box); }
    const ids = new Set(pending.map(p => p.approval.id));
    for (const id of Object.keys(shown)) if (!ids.has(id)) { const el = $('mcp-card-' + id); if (el) el.remove(); delete shown[id]; if (window.wrmAiAlertDone) wrmAiAlertDone('approval-' + id); }
    for (const p of pending) {
      const a = p.approval;
      // a request that became "kept for later" is shown again: other text, no editing
      if (shown[a.id] && !!shown[a.id].approval.later === !!a.later) continue;
      if (shown[a.id]) { const old = $('mcp-card-' + a.id); if (old) old.remove(); }
      shown[a.id] = p;
      const el = document.createElement('div');
      el.className = 'mcp-card'; el.id = 'mcp-card-' + a.id;
      const what = a.transfer ? `<div>${esc(t('mcp_ap_transfer'))}</div><pre>${esc(a.transfer)}</pre>`
        : a.command ? `<div>${esc(t('mcp_ap_cmd'))}</div><pre id="mcp-cmd-${a.id}">${esc(a.command)}</pre>`
        : `<div>${esc(tf('mcp_ap_edit_file', {path: a.path}))}</div><pre>${diffHTML(a.diff)}</pre>`;
      el.innerHTML = `<h4>🤖 ${esc(tf('mcp_ap_title', {client: p.client, server: p.connection}))}</h4>${a.later ? `<div class="hint-box warn" style="margin:4px 0;">${esc(t(a.run_now ? 'mcp_ap_later_run' : 'mcp_ap_later'))}</div>` : ''}${what}
        ${a.why ? `<div class="sub">${esc(t('mcp_ap_why'))}: ${esc(a.why)}</div>` : ''}${a.reason ? `<div class="sub">${esc(t('mcp_ap_reason'))}: ${esc(a.reason)}</div>` : ''}
        <div id="mcp-edit-${a.id}"></div>
        <input type="text" id="mcp-note-${a.id}" maxlength="500" placeholder="${esc(t('mcp_ap_note'))}" style="margin-top:6px;">
        <div class="acts"><button class="btn-sm" onclick="wrmMCP.decide(${p.session}, '${a.id}', true)">✔ ${esc(t('mcp_ap_approve'))}</button>
          <button class="btn-danger btn-sm" onclick="wrmMCP.decide(${p.session}, '${a.id}', false)">✕ ${esc(t('mcp_ap_deny'))}</button>
          ${a.transfer || a.later ? '' : `<button class="btn-sec btn-sm" onclick="wrmMCP.edit('${a.id}')">✎ ${esc(t('mcp_ap_edit'))}</button>`}
          <span class="sub" style="margin-left:auto;align-self:center;" data-exp="${esc(a.expires)}"></span></div>`;
      box.appendChild(el);
    }
    if (!cardTimer && Object.keys(shown).length) cardTimer = setInterval(tickCards, 1000);
    tickCards();
  }
  function tickCards() {
    const els = document.querySelectorAll('#mcp-cards [data-exp]');
    if (!els.length && cardTimer) { clearInterval(cardTimer); cardTimer = null; return; }
    els.forEach(el => { const s = Math.max(0, Math.round((new Date(el.dataset.exp) - Date.now()) / 1000)); el.textContent = tf('mcp_ap_expires', {s}); });
  }
  function edit(aid) {
    const p = shown[aid];
    if (!p) return;
    const a = p.approval, box = $('mcp-edit-' + aid);
    if (box.innerHTML) return;
    box.innerHTML = a.command ? `<input type="text" id="mcp-edv-${aid}" value="${esc(a.command)}">` : `<textarea id="mcp-edv-${aid}">${esc(a.content || '')}</textarea>`;
    const acts = box.parentElement.querySelector('.acts button');
    acts.textContent = '✔ ' + t('mcp_ap_approve_edited');
  }
  async function decide(sid, aid, approve) {
    const p = shown[aid];
    const body = {decision: approve ? 'approve' : 'deny', note: ($('mcp-note-' + aid) || {}).value || ''};
    const ed = $('mcp-edv-' + aid);
    if (approve && ed && p) { if (p.approval.command) body.command = ed.value; else body.content = ed.value; }
    // a request already decided elsewhere (the AI app, another tab) is reported as such, not as an error
    const r = await wrmAISessions.decide(`/api/ai/sessions/${sid}/approvals/${aid}`, body);
    const el = $('mcp-card-' + aid); if (el) el.remove();
    delete shown[aid];
    if (r.decided) {
      for (const w of Object.values(W8)) {
        const box = w.el.querySelector('.we.ask .wa [onclick*="' + aid + '"]');
        if (box) box.closest('.wa').innerHTML = `<div class="sub">${esc(wrmAISessions.decidedText(r.decided))}</div>`;
      }
    }
    refresh();
  }

  // ── OAuth consent ─────────────────────────────────────
  async function consent(id) {
    let req, c;
    try { [req, c] = await Promise.all([api('GET', `/api/mcp/authorize/${encodeURIComponent(id)}`), api('GET', '/api/mcp/config')]); }
    catch (e) { showModal(`<h3>🤖 ${esc(t('mcp_cs_title'))}</h3><div class="hint-box warn">${esc(e.message || t('mcp_cs_expired'))}</div><div class="modal-footer"><button onclick="wrmMCP.closeModal()">${esc(t('close'))}</button></div>`); clearParam(); return; }
    cfg = c;
    if (!req.enabled) { showModal(`<h3>🤖 ${esc(t('mcp_cs_title'))}</h3><div class="hint-box warn">${esc(t('mcp_off'))}: ${esc(req.reason)}</div><div class="modal-footer"><button onclick="wrmMCP.answer('${esc(id)}', false)">${esc(t('mcp_cs_deny'))}</button></div>`); return; }
    showModal(`<h3>🤖 ${esc(t('mcp_cs_title'))}</h3>
      <div class="hint-box"><b>${esc(tf('mcp_cs_asks', {client: req.client_name}))}</b><br>${esc(tf('mcp_cs_redirect', {host: req.redirect_host}))}</div>
      ${formHTML('mcpc', c, {scopes: (req.scopes || []).filter(s => c.scopes.includes(s)), noName: true})}
      <div class="modal-footer"><button class="btn-sec" onclick="wrmMCP.answer('${esc(id)}', false)">${esc(t('mcp_cs_deny'))}</button><button onclick="wrmMCP.answer('${esc(id)}', true)">${esc(t('mcp_cs_approve'))}</button></div>`);
  }
  async function answer(id, approve) {
    const body = approve ? Object.assign({decision: 'approve'}, formValue('mcpc')) : {decision: 'deny'};
    let r;
    try { r = await api('POST', `/api/mcp/authorize/${encodeURIComponent(id)}`, body); } catch (e) { showToast(e.message, 'error', 6000); return; }
    clearParam();
    $('mcp-modal').querySelector('.modal').innerHTML = `<h3>🤖 ${esc(t('mcp_cs_title'))}</h3><div class="hint-box">${esc(t('mcp_cs_done'))}</div>`;
    location.href = r.redirect;
  }
  function clearParam() {
    try { const u = new URL(location.href); u.searchParams.delete('mcp_authorize'); history.replaceState(null, '', u.pathname + u.search + u.hash); } catch (_) {}
  }
  function showModal(html) {
    let m = $('mcp-modal');
    if (!m) { m = document.createElement('div'); m.id = 'mcp-modal'; m.className = 'modal-overlay'; m.style.zIndex = 9650; document.body.appendChild(m); }
    m.innerHTML = `<div class="modal" style="width:560px;max-width:calc(100vw - 32px);max-height:calc(100vh - 32px);overflow:auto;">${html}</div>`;
    m.classList.add('open');
  }
  function closeModal() { const m = $('mcp-modal'); if (m) m.classList.remove('open'); }

  // ── Admin panel → AI connections ──────────────────────
  const MCP_POLICY_KEYS = ['ai_mcp_enabled', 'ai_mcp', 'ai_mcp_modes', 'ai_mcp_scopes', 'ai_full_access', 'ai_full_access_mode', 'ai_full_access_blocked', 'ai_mcp_token_hours', 'ai_mcp_max_token_hours', 'ai_mcp_oauth',
    'ai_mcp_elicitation', 'ai_mcp_later_approval', 'ai_mcp_redirect_uris', 'ai_mcp_allowed_origins', 'ai_mcp_public_url', 'ai_mcp_transfer_max_mb'];
  async function adminMCP() {
    const body = $('admin-body');
    let grants, clients, settings, c;
    try {
      [grants, clients, settings, c] = await Promise.all([api('GET', '/api/admin/mcp/grants'), api('GET', '/api/admin/mcp/clients'), api('GET', '/api/admin/settings'), api('GET', '/api/mcp/config')]);
    } catch (e) { body.innerHTML = `<div class="fm-error">${esc(e.message)}</div>`; return; }
    adminSettingsCache = settings;
    const active = grants.filter(g => g.state === 'active');
    body.innerHTML = `<div class="hint-box">${esc(tf('mcp_adm_help', {url: c.url}))}</div>
      <div style="display:flex;gap:8px;flex-wrap:wrap;margin:8px 0;"><button class="btn-sm" onclick="openSettings('st-mcp')">🔌 ${esc(t('mcp_quick_wizard'))}</button></div>
      <div style="display:flex;align-items:center;gap:8px;"><div class="adm-h" style="flex:1;">🔌 ${esc(t('mcp_adm_active'))}</div>${active.length ? `<button class="btn-danger btn-sm" onclick="wrmMCP.adminRevokeAll()">⏻ ${esc(t('mcp_revoke_all'))}</button>` : ''}</div>
      ${active.length ? `<div class="tbl-scroll"><table class="admin-table"><thead><tr><th>${esc(t('mcp_user'))}</th><th>${esc(t('mcp_client'))}</th><th>${esc(t('mcp_conns'))}</th><th>${esc(t('mcp_mode'))}</th><th>${esc(t('mcp_state'))}</th><th></th></tr></thead><tbody>` +
        active.map(g => `<tr><td>${esc(g.user)}</td><td>${esc(g.client_name || g.name)} <span class="pill">${esc(t('mcp_kind_' + g.kind))}</span><div class="sub">${esc(tf('mcp_expires', {time: rel(g.expires_at)}))}</div></td>
          <td>${esc(g.connections.map(x => x.name).join(', '))}<div class="sub mono">${esc(g.scopes.join(', '))}</div></td><td>${esc(t('mcp_mode_' + g.mode))}</td>
          <td>${g.live ? `<span class="pill ok">● ${esc(t('mcp_live'))}</span>` : ''}<div class="sub">${g.last_used_at ? esc(tf('mcp_last_used', {time: rel(g.last_used_at)})) : esc(t('mcp_never_used'))}${g.last_ip ? ' · ' + esc(g.last_ip) : ''}</div></td>
          <td><button class="btn-danger btn-sm" onclick="wrmMCP.adminRevoke(${g.id})">${esc(t('mcp_revoke'))}</button></td></tr>`).join('') + '</tbody></table></div>'
        : `<div class="chat-empty">${esc(t('mcp_adm_none'))}</div>`}
      <div class="adm-h">⚙ ${esc(t('mcp_adm_policies'))}</div>
      <div class="policy-group">${MCP_POLICY_KEYS.map(k => settingRowHTML(adminSettingsCache.find(x => x.key === k))).join('')}</div>
      <div class="adm-h">🧩 ${esc(t('mcp_adm_clients'))}</div>
      ${clients.length ? `<div class="tbl-scroll"><table class="admin-table"><thead><tr><th>${esc(t('mcp_client'))}</th><th>${esc(t('mcp_adm_redirects'))}</th><th>${esc(t('mcp_created'))}</th><th></th></tr></thead><tbody>` +
        clients.map(x => `<tr><td>${esc(x.client_name)}<div class="sub mono">${esc(x.client_id)}</div>${x.active ? `<span class="pill ok">${x.active}</span>` : ''}</td>
          <td class="mono" style="font-size:10.5px;word-break:break-all;">${esc((x.redirect_uris || []).join(' '))}</td><td>${esc(rel(x.created_at))}<div class="sub">${esc(x.created_ip)}</div></td>
          <td><button class="btn-sec btn-sm" onclick="wrmMCP.adminRemoveClient(${x.id})">${esc(t('mcp_adm_remove'))}</button></td></tr>`).join('') + '</tbody></table></div>'
        : `<div class="chat-empty">${esc(t('none'))}</div>`}`;
  }
  async function adminRevoke(id) {
    if (!(await uiConfirm(t('mcp_revoke_q'), {okText: t('mcp_revoke'), danger: true}))) return;
    try { await api('DELETE', `/api/admin/mcp/grants/${id}`); } catch (e) { showToast(e.message, 'error'); }
    adminMCP();
  }
  async function adminRevokeAll() {
    if (!(await uiConfirm(t('mcp_revoke_all_q'), {okText: t('mcp_revoke_all'), danger: true}))) return;
    try { const r = await api('POST', '/api/admin/mcp/revoke-all'); showToast(tf('mcp_revoked_n', {n: r.revoked}), 'success'); } catch (e) { showToast(e.message, 'error'); }
    adminMCP();
  }
  async function adminRemoveClient(id) {
    if (!(await uiConfirm(t('mcp_adm_remove_q'), {okText: t('mcp_adm_remove'), danger: true}))) return;
    try { await api('DELETE', `/api/admin/mcp/clients/${id}`); } catch (e) { showToast(e.message, 'error'); }
    adminMCP();
  }

  window.wrmMCP = {
    settingsPane, create, editGrant, saveGrant, revoke, revokeAll, filter, modeChanged, allowAll, untilGrant, setFullAccess, snip, copy, refresh, indicator, togglePop, decide, edit, watch, unwatch, watchAuto, dockMode, dockHide, dockPlace,
    dockStyle, dockAvailable: () => dockCount() > 0,
    installer, cursorInstall, desktopExtension, setupAll, wizPreset, wizClient, wizGo, wizNext, wizReset, wizDownload, wizTest,
    consent, answer, closeModal, adminTab: adminMCP, adminRevoke, adminRevokeAll, adminRemoveClient,
  };
})();
