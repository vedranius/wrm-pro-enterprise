/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// AI assistant of WRM PRO: the panel next to a terminal or file manager window, personal
// API keys and the administrators' AI tab. Loaded on demand by openAI() / the admin panel
// in index.html; uses its helpers (t, esc, showToast, apiError, uiConfirm, fmtTime,
// winObjects, winConn, winCan, currentUser, fitTerm, openReplay, settingRowHTML).
// The server decides everything (modes, approvals, limits); this file only shows it.
'use strict';

Object.assign(LANGS.en, {
  ai_dock_left: 'Dock left', ai_dock_right: 'Dock right', ai_dock_top: 'Dock to the top', ai_dock_bottom: 'Dock to the bottom',
  ai_float: 'Float (drag the title bar to move, drag the corner to resize)', ai_expand: 'Expand over the whole terminal window', ai_restore: 'Restore size',
  ai_title: 'AI assistant', ai_start: 'Start', ai_provider: 'Provider', ai_model: 'Model', ai_mode: 'Mode',
  ai_thinking: 'Processing your request…',
  ai_worklog: 'Work log',
  ai_worklog_working: 'Working…',
  ai_worklog_steps: '{n} steps',
  ai_mode_read_only: 'Read-only', ai_mode_ask: 'Ask before every change', ai_mode_auto: 'Automatic within limits',
  ai_mode_read_only_d: 'The assistant reads logs, files, services and packages. Every change is refused.',
  ai_mode_ask_d: 'Reads run directly. Every command that changes something and every file edit waits for your approval.',
  ai_mode_auto_d: 'Changes that match your allow list run without asking until the time or action limit. Everything else still asks. Destructive commands are always refused.',
  ai_disabled: 'The AI assistant is not available', ai_no_providers: 'No AI provider is configured. An administrator adds the organisation\'s provider in Admin panel → AI assistant.',
  ai_add_own_key: 'Add my own API key', ai_my_keys: 'My API keys', ai_share_notes: 'Share my notes for this connection with the assistant',
  ai_auto_allow: 'Allowed commands (one pattern per line, * = anything)', ai_auto_deny: 'Denied commands', ai_auto_path_allow: 'Files it may change (/etc/nginx/**)',
  ai_auto_path_deny: 'Files it must not change', ai_auto_minutes: 'Time limit (minutes)', ai_auto_actions: 'Maximum actions',
  ai_auto_confirm: 'I allow the assistant to make these changes without asking, within these limits.', ai_auto_need_confirm: 'Confirm the automatic mode and its limits first.',
  ai_see: 'What can the AI see?', ai_see_tools: 'Read tools', ai_see_cmd: 'Commands: read-only ones always (in every mode); changes only as the mode allows',
  ai_see_write: 'File edits (shown as a diff)', ai_see_redacted: 'Secrets in output are replaced by [REDACTED] before they are sent to the provider.',
  ai_see_not_redacted: 'Output is sent to the provider as it is (redaction is off by policy).', ai_see_limit: 'Output and files are limited to {kb} KB per call.',
  ai_see_never: 'Never: passwords and keys stored in WRM, your WRM sign-in, other connections, your terminal screen.', ai_see_denied: '{n} kinds of sensitive files (password hashes, private keys, credentials) are never read.',
  ai_see_provider: 'Everything the assistant sees is sent to {provider}.', ai_see_notes_on: 'Your notes for this connection are shared.',
  ai_placeholder: 'Ask about this server…', ai_send: 'Send', ai_stop: 'Stop', ai_end: 'End the AI session', ai_end_q: 'End this AI session? A running command is stopped.',
  ai_sound_on: 'Sound is on: a beep for an approval and for every reply', ai_sound_off: 'Sound is off', ai_sound_on_toast: 'Sound on', ai_sound_off_toast: 'Sound off',
  ai_pending: 'Approval needed', ai_pending_more: '+{n} more', ai_pending_details: 'Details',
  ai_close_panel: 'Hide the panel (the session keeps running)', ai_history: 'My AI sessions', ai_continue: 'Continue session', ai_new: 'New session',
  ai_approve: 'Approve', ai_deny: 'Deny', ai_edit: 'Edit', ai_note_ph: 'Note for the assistant (optional)', ai_expires: 'expires in {s} s',
  ai_wait_approval: 'Waiting for your approval', ai_d_allow: 'ran', ai_d_readonly: 'read-only', ai_d_auto: 'automatic', ai_d_approve: 'needs approval', ai_d_deny: 'denied',
  ai_o_approved: 'approved', ai_o_denied: 'denied', ai_o_timeout: 'timed out', ai_o_cancelled: 'cancelled', ai_edited: 'edited',
  ai_exit: 'exit {code}', ai_redactions: '{n} redacted', ai_output: 'Output', ai_reason: 'Why',
  ai_n_stopped: 'Stopped.', ai_n_retry: 'The provider is busy; trying again shortly…', ai_n_shrunk: 'The request was too large for the provider; long command output was shortened and the request is retried.', ai_n_refusal: 'The model declined this request.', ai_n_step_limit: 'The assistant stopped after the maximum number of steps.',
  ai_ended: 'The AI session has ended', ai_mode_changed: 'Mode: {mode}', ai_by: 'by {who}', ai_usage: '{in} in · {out} out tokens',
  ai_r_user: 'ended by you', ai_r_admin: 'stopped by an administrator', ai_r_killall: 'every AI session was stopped by an administrator',
  ai_r_switch: 'the kill switch was turned on', ai_r_off: 'the AI assistant was turned off', ai_r_blocked: 'an administrator turned the assistant off for you',
  ai_r_idle: 'ended after inactivity', ai_r_policy: 'the policy no longer allows the assistant on this connection',
  ai_fm_ctx: 'Directory open in the file manager', ai_active_on: 'AI active ({mode})',
  ai_auto_until: 'until {time} · {used}/{max} actions', ai_mode_auto_setup: 'Automatic mode limits',
  // personal / org providers
  ai_prov_kind: 'Type', ai_prov_name: 'Name', ai_prov_models: 'Models (comma-separated)', ai_prov_default: 'Default model',
  ai_prov_key: 'API key', ai_prov_key_keep: 'unchanged (stored encrypted)', ai_prov_base: 'Base URL (optional: gateway, private endpoint)',
  ai_prov_base_req: 'Base URL', ai_prov_endpoint: 'Endpoint (https://<resource>.openai.azure.com)', ai_prov_deployment: 'Deployment (empty: the model name)',
  ai_prov_apiver: 'API version (empty: the v1 API)', ai_prov_region: 'Region', ai_prov_project: 'Google Cloud project', ai_prov_api: 'API',
  ai_prov_ak: 'AWS access key ID', ai_prov_sk: 'AWS secret access key', ai_prov_st: 'AWS session token (optional)', ai_prov_bkey: 'Bedrock API key (instead of access keys)',
  ai_prov_sa: 'Service account key (JSON)', ai_prov_price: 'Price per million tokens (USD, input / output; empty: built-in for Anthropic)',
  ai_prov_maxtok: 'Max output tokens (0 = 16000)', ai_prov_fallback: 'Refusal fallback (Anthropic API)', ai_prov_enabled: 'Enabled', ai_prov_test: 'Test',
  ai_prov_test_ok: 'The provider answered', ai_prov_add: 'Add provider', ai_prov_edit: 'Edit provider', ai_prov_del_q: 'Delete this provider? Its sessions end.',
  ai_prov_none: 'No providers yet.', ai_models_col: 'Models', ai_prov_secrets: 'Keys', ai_kind_anthropic: 'Anthropic API', ai_kind_openai: 'OpenAI API', ai_kind_azure: 'Azure OpenAI',
  ai_kind_bedrock: 'AWS Bedrock', ai_kind_vertex: 'Google Vertex AI', ai_kind_openai_compatible: 'OpenAI-compatible (local models)', ai_kind_opencode: 'OpenCode Zen (pay-as-you-go; free models without a key)',
  ai_kind_opencode_go: 'OpenCode Go (subscription key)',
  ai_subscriptions_note: 'Consumer Claude.ai and ChatGPT chat subscriptions cannot be used by other applications. Use an API key or an enterprise gateway here, or connect the chat app to WRM over MCP (Settings → AI connections).',
  aiw_title: 'Add an AI provider (step by step)', aiw_s_pick: 'Provider', aiw_s_key: 'Key and model', aiw_s_test: 'Test', aiw_s_done: 'Done',
  aiw_pick: 'Which AI service do you want to use?', aiw_p_anthropic: 'Anthropic (Claude)', aiw_p_openai: 'OpenAI', aiw_p_gemini: 'Google Gemini', aiw_p_groq: 'Groq', aiw_p_openrouter: 'OpenRouter', aiw_p_mistral: 'Mistral',
  aiw_p_ollama: 'Ollama (local, free)', aiw_p_custom: 'Other OpenAI-compatible', aiw_p_advanced: 'Azure, AWS Bedrock, Vertex (advanced form)',
  aiw_key: 'API key', aiw_key_opt: 'API key (optional for local models)', aiw_endpoint: 'Address (base URL)', aiw_model: 'Model', aiw_model_ph: 'e.g. {m}', aiw_name: 'Name',
  aiw_key_note: 'The key is stored encrypted and is never shown again.', aiw_ollama_note: 'Ollama must run on the same computer as WRM (or be reachable from it). Install it from ollama.com, then run: ollama pull {m}',
  aiw_testing: 'Saving and testing…', aiw_ok: 'It works. The provider answered{reply}.', aiw_fail: 'The provider did not answer: {err}', aiw_fix: 'Go back and check the key, the address and the model name.',
  aiw_next: 'Save and test', aiw_back: 'Back', aiw_finish: 'Done', aiw_use: 'Open an AI assistant window on a server connection and choose this provider. Every session is recorded and audited.', aiw_wizard: 'Setup wizard', aiw_need: 'Fill in the required fields.',
  aiw_s_ollama: 'Ollama on this server', aiw_p_xai: 'xAI (Grok)', aiw_p_deepseek: 'DeepSeek', aiw_p_together: 'Together AI', aiw_p_perplexity: 'Perplexity', aiw_p_cohere: 'Cohere',
  aiw_p_fireworks: 'Fireworks AI', aiw_p_cerebras: 'Cerebras', aiw_p_opencode: 'OpenCode Zen (pay-as-you-go; free models)', aiw_p_opencode_go: 'OpenCode Go (subscription)', aiw_free: 'free', ai_deleg_setup: 'Helper models: {p}, {n} in use', ai_deleg_setup_d: 'The assistant may hand text subtasks to these models; WRM picks the best one for each task, free ones first. They get only redacted text and cannot run anything.',
  ai_deleg_training_me: 'Also use free models that may use my prompts for training', ai_deleg_training_all: 'The administrator allows free models that may train on prompts for everyone.', ai_deleg_training_off: 'The administrator does not allow free models that may train on prompts.',
  ai_deleg_excluded: 'Not used (may train on prompts):',
  ai_du_zero_retention: 'keeps nothing', ai_du_zero_retention_d: 'Zero retention: prompts are not stored or used for training.', ai_du_trains: 'may train', ai_du_trains_d: 'During the free period the provider may use prompts and answers to improve its models.',
  ai_du_unknown: 'terms unknown', ai_du_unknown_d: 'WRM does not know this free model’s data terms; it is treated like a model that may train.', ai_du_paid: 'paid', ai_du_paid_d: 'Billed to the OpenCode Zen workspace; Zen states zero retention (OpenAI and Anthropic keep requests 30 days).',
  ai_du_provider: 'own provider', ai_du_provider_d: 'Your organisation’s own provider and its terms.',
  p_ai_delegate_training: 'Free models that may train on prompts', p_ai_delegate_training_d: 'Off: never used. User: each user decides in the assistant (default off). All: used for everyone.',
  po_ai_delegate_training_off: 'Never', po_ai_delegate_training_user: 'Each user decides', po_ai_delegate_training_all: 'Everyone', ai_deleg_h: 'Delegation to helper models', ai_deleg_help: 'The assistant can hand self-contained text subtasks (summarise a long log, explain an error, draft a snippet) to a cheaper or free helper model. The helper only gets the text the assistant passes, with secrets redacted; it has no tools and cannot run anything on a server. Every delegation appears in the live view, the transcript and the audit log with model, size, tokens and cost. Free OpenCode Zen models can use what they receive to improve their models: only allow models whose terms fit your data.',
  ai_deleg_off: 'Delegation is off.', ai_deleg_noprov: 'Delegation is on, but no helper provider is chosen.', ai_deleg_disabled: 'The chosen helper provider is disabled.', ai_deleg_on: 'Active: {p} with {m}', ai_deleg_load: 'Load models',
  p_ai_delegate: 'Allow delegation to helper models', p_ai_delegate_d: 'Offers the assistant the delegate_task tool (built-in assistant only; MCP clients use their own agents).',
  p_ai_delegate_provider: 'Helper provider', p_ai_delegate_provider_d: 'An organisation provider, for example OpenCode Zen (free models work without a key).',
  p_ai_delegate_models: 'Helper models', p_ai_delegate_models_d: 'Comma-separated; the first is the default. Empty: automatic, every usable model of the provider (for OpenCode Zen the current catalog). WRM picks the one that fits each task.', p_ai_delegate_models_ph: 'big-pickle',
  p_ai_delegate_max_kb: 'Delegation size limit (KB)', p_ai_delegate_max_kb_d: 'The most text one delegation may send to the helper.',
  aiw_n_opencode: 'Without a key only the free OpenCode Zen models work (limited per IP address; some free models may use prompts to improve their models, so do not send confidential data). With a key from opencode.ai every Zen model works, billed to that workspace. Use "Load models" after the test to pick from the current list.',
  aiw_n_opencode_go: 'Uses the OpenCode Go subscription endpoints, not Zen: paste the Go API key from opencode.ai/console/go. A Go key has no pay-as-you-go balance, so Zen would answer "insufficient funds". Use "Load models" to pick a model from the Go list.',
  aiw_model_note: 'Model names change often. Type any model name (several separated by commas; the first one is the default), or save and use “Load the model list” on the last step.',
  aiw_n_perplexity: 'Perplexity models cannot call tools, so the assistant may be unable to run checks with them.',
  aiw_o_intro: 'Ollama runs free AI models on a computer you control. WRM talks to it through its OpenAI-compatible address and needs no API key. The assistant still works through WRM, so every session is recorded and audited.',
  aiw_o_checking: 'Looking for Ollama…', aiw_o_running: 'Ollama {v} is running at {url}.', aiw_o_notrunning: 'Ollama was not found running at {url}.',
  aiw_o_models: 'Installed models', aiw_o_nomodels: 'No model is installed yet. Download one (step 3 below).', aiw_o_use_model: 'Use',
  aiw_o_manual: 'Run these commands on the server yourself, or let WRM do it (administrators only):', aiw_o_os: 'Operating system of the WRM server: {os}',
  aiw_o_s_install: '1. Install Ollama', aiw_o_s_start: '2. Start it as a background service', aiw_o_s_pull: '3. Download a model',
  aiw_o_do_install: 'Install with WRM', aiw_o_do_start: 'Start with WRM', aiw_o_do_pull: 'Download with WRM', aiw_o_run: 'Run it',
  aiw_o_confirm: 'WRM will run this on the WRM server:\n\n{cmd}\n\nThe action is recorded in the audit log. Continue?',
  aiw_o_noadmin: 'Only an administrator can install or start Ollama from WRM. Ask one, or run the commands above yourself.',
  aiw_o_nopolicy: 'Installing from WRM is switched off. An administrator can enable the policy “Let administrators install and start Ollama from WRM” in Admin → AI.',
  aiw_o_nosupport: 'Automatic setup is not available on this operating system. Follow the steps from ollama.com/download.',
  aiw_o_run_running: 'Running…', aiw_o_run_done: 'Finished.', aiw_o_run_failed: 'Failed.', aiw_o_refresh: 'Check again', aiw_o_copy: 'Copy', aiw_o_copied: 'Copied.',
  aiw_o_history: 'Recent actions', aiw_o_model_ph: 'model, e.g. qwen2.5-coder:7b', aiw_o_next: 'Continue',
  aiw_load_models: 'Load the model list', aiw_models_loaded: 'Models offered by the provider (click to add):', aiw_models_none: 'The provider returned no models.', aiw_models_added: 'Model added.',
  p_ai_ollama_manage: 'Let administrators install and start Ollama from WRM', p_ai_ollama_manage_d: 'Allows administrators to install Ollama, start it as a background service and download models on the WRM server from the provider wizard. Every action is confirmed, recorded and audited. Off by default.',
  // admin
  adm_ai_kill: 'Kill switch', ai_kill_all: 'Stop every AI session now', ai_kill_all_q: 'End every AI session of every user now?', ai_killed_n: '{n} sessions ended',
  ai_kill_switch_on: 'The kill switch is ON: nobody can use the AI assistant.', ai_kill_switch_off: 'The kill switch is off.',
  ai_policies: 'Policies', ai_providers_org: 'Organisation providers', ai_sessions: 'AI sessions', ai_usage_users: 'Usage per user', ai_destructive: 'Always blocked (destructive patterns)',
  ai_transcript: 'Transcript', ai_replay: 'Replay', ai_kill: 'Kill', ai_block: 'Turn off for this user', ai_unblock: 'Turn on again', ai_blocked: 'off',
  ai_live: 'live', ai_cost: 'Cost', ai_tokens: 'Tokens', ai_requests: 'Requests', ai_tool_calls: 'Tool calls', ai_mode_rules_help: 'Mode rules (most restrictive wins): JSON, e.g. [{"match":"tag","value":"prod","modes":["read_only"]}, {"match":"folder","value":"Production","modes":["read_only","ask"]}]. match: tag, folder, connection (id), host (wildcards).',
  // policies (Admin panel → AI assistant)
  p_ai_assistant: 'AI assistant', p_ai_assistant_d: 'Who may use the built-in AI assistant.', po_ai_assistant_off: 'Off', po_ai_assistant_admins: 'Administrators', po_ai_assistant_all: 'Everybody',
  p_ai_kill_switch: 'Kill switch', p_ai_kill_switch_d: 'Ends every AI session at once and blocks new ones until it is turned off.',
  p_ai_modes: 'Allowed modes', p_ai_modes_d: 'Global default, comma-separated: read_only, ask, auto. Mode rules can only narrow it.', p_ai_modes_ph: 'read_only,ask',
  p_ai_default_mode: 'Default mode', p_ai_default_mode_d: 'Mode of a new session when it is allowed.', po_ai_default_mode_read_only: 'Read-only', po_ai_default_mode_ask: 'Ask before every change',
  p_ai_mode_rules: 'Mode rules', p_ai_mode_rules_d: 'Per tag, folder, connection or host; the most restrictive matching rule wins.',
  p_ai_personal_keys: 'Personal API keys', p_ai_personal_keys_d: 'Who may add their own provider keys.', po_ai_personal_keys_off: 'Nobody', po_ai_personal_keys_admins: 'Administrators', po_ai_personal_keys_all: 'Everybody',
  p_ai_provider_kinds: 'Allowed provider types', p_ai_provider_kinds_d: 'anthropic, openai, azure, bedrock, vertex, openai_compatible, opencode (Zen), opencode_go (Go subscription)', p_ai_provider_kinds_ph: 'anthropic,azure',
  p_ai_models: 'Allowed models', p_ai_models_d: 'Comma-separated patterns (claude-*); empty = every model of the providers.', p_ai_models_ph: 'claude-*',
  p_ai_redact_output: 'Redact output', p_ai_redact_output_d: 'Replace secrets (passwords, tokens, private keys …) before output is sent to the provider. Transcripts and the audit are always redacted.',
  p_ai_share_notes: 'Allow sharing notes', p_ai_share_notes_d: 'Users may opt in to send their connection notes to the assistant.',
  p_ai_transcript_retention_days: 'Transcript retention (days)', p_ai_transcript_retention_days_d: 'AI transcripts are deleted after this time (recordings follow the recording retention).',
  p_ai_approval_timeout_seconds: 'Approval timeout (seconds)', p_ai_approval_timeout_seconds_d: 'An approval that is not answered in time is denied.',
  p_ai_auto_max_minutes: 'Automatic mode: max minutes', p_ai_auto_max_minutes_d: 'The longest time limit a user may choose.',
  p_ai_auto_max_actions: 'Automatic mode: max actions', p_ai_auto_max_actions_d: 'The largest number of automatic actions per opt-in.',
  p_ai_auto_until_grant: 'AI connections: automatic without limits', p_ai_auto_until_grant_d: 'Users may choose automatic mode with no time or action limit for an AI connection; it then lasts as long as the connection is valid.',
  p_ai_sensitive_reads: 'Files that may hold secrets', p_ai_sensitive_reads_d: 'Keys, credentials and the paths of ai_read_deny_paths: readable after the user approves each read (secret-looking values are redacted), or always blocked.', po_ai_sensitive_reads_ask: 'With an approval', po_ai_sensitive_reads_deny: 'Always blocked',
  p_ai_allow_power: 'Shutdown and reboot', p_ai_allow_power_d: 'Blocked, or allowed with an approval of each command (never automatic).', po_ai_allow_power_off: 'Always blocked', po_ai_allow_power_ask: 'With an approval',
  p_ai_blocked_commands: 'Extra blocked commands', p_ai_blocked_commands_d: 'Comma-separated patterns refused in every mode, in addition to the built-in list.', p_ai_blocked_commands_ph: 'docker system prune*',
  p_ai_read_deny_paths: 'Extra unreadable files', p_ai_read_deny_paths_d: 'Comma-separated path patterns the assistant never reads (a pattern without / matches the file name).', p_ai_read_deny_paths_ph: '/srv/app/.env, *.p12',
  p_ai_provider_header_seconds: 'Provider header wait (seconds)', p_ai_provider_header_seconds_d: 'Allows model loading, prompt evaluation and queued requests before the first response.',
  p_ai_provider_request_seconds: 'Provider request limit (seconds)', p_ai_provider_request_seconds_d: 'Total time for one model request, including streaming. Stop still cancels immediately.',
  p_ai_provider_stream_idle_seconds: 'Provider stream idle limit (seconds)', p_ai_provider_stream_idle_seconds_d: 'Stop a stream that sends no data for this long; requests and tool actions are never repeated automatically.',
  p_ai_command_timeout_seconds: 'Command time limit (seconds)', p_ai_command_timeout_seconds_d: 'A tool call is stopped after this time.',
  p_ai_output_max_kb: 'Output limit (KB)', p_ai_output_max_kb_d: 'Per tool call, also the size limit for reading files.',
  p_ai_max_steps: 'Steps per request', p_ai_max_steps_d: 'Model requests per user message.',
  p_ai_idle_minutes: 'Idle timeout (minutes)', p_ai_idle_minutes_d: 'An idle AI session ends after this time.',
});
Object.assign(LANGS.hr, {
  ai_dock_left: 'Usidri lijevo', ai_dock_right: 'Usidri desno', ai_dock_top: 'Usidri gore', ai_dock_bottom: 'Usidri dolje',
  ai_float: 'Slobodan prozor (povuci naslovnu traku za pomicanje, povuci kut za promjenu veličine)', ai_expand: 'Proširi preko cijelog prozora terminala', ai_restore: 'Vrati veličinu',
  ai_title: 'AI asistent', ai_start: 'Pokreni', ai_provider: 'Pružatelj', ai_model: 'Model', ai_mode: 'Način rada',
  ai_thinking: 'Obrađujem upit…',
  ai_worklog: 'Tijek rada',
  ai_worklog_working: 'Radim…',
  ai_worklog_steps: '{n} koraka',
  ai_mode_read_only: 'Samo čitanje', ai_mode_ask: 'Pitaj prije svake promjene', ai_mode_auto: 'Automatski u granicama',
  ai_mode_read_only_d: 'Asistent čita logove, datoteke, servise i pakete. Svaka promjena se odbija.',
  ai_mode_ask_d: 'Čitanje se izvršava odmah. Svaka naredba koja nešto mijenja i svaka izmjena datoteke čeka tvoje odobrenje.',
  ai_mode_auto_d: 'Promjene s tvoje liste dopuštenih izvršavaju se bez pitanja do isteka vremena ili broja akcija. Za sve ostalo i dalje pita. Destruktivne naredbe su uvijek odbijene.',
  ai_disabled: 'AI asistent nije dostupan', ai_no_providers: 'Nijedan AI pružatelj nije podešen. Administrator dodaje pružatelja organizacije u Admin panel → AI asistent.',
  ai_add_own_key: 'Dodaj vlastiti API ključ', ai_my_keys: 'Moji API ključevi', ai_share_notes: 'Podijeli s asistentom moje bilješke za ovu vezu',
  ai_auto_allow: 'Dopuštene naredbe (jedan uzorak po retku, * = bilo što)', ai_auto_deny: 'Zabranjene naredbe', ai_auto_path_allow: 'Datoteke koje smije mijenjati (/etc/nginx/**)',
  ai_auto_path_deny: 'Datoteke koje ne smije mijenjati', ai_auto_minutes: 'Vremensko ograničenje (minute)', ai_auto_actions: 'Najviše akcija',
  ai_auto_confirm: 'Dopuštam asistentu da ove promjene radi bez pitanja, unutar ovih granica.', ai_auto_need_confirm: 'Najprije potvrdi automatski način i njegove granice.',
  ai_see: 'Što AI vidi?', ai_see_tools: 'Alati za čitanje', ai_see_cmd: 'Naredbe: one samo za čitanje uvijek (u svakom načinu); promjene samo koliko način dopušta',
  ai_see_write: 'Izmjene datoteka (prikazane kao diff)', ai_see_redacted: 'Tajne u izlazu zamjenjuju se s [REDACTED] prije slanja pružatelju.',
  ai_see_not_redacted: 'Izlaz se pružatelju šalje kakav jest (redakcija je isključena politikom).', ai_see_limit: 'Izlaz i datoteke ograničeni su na {kb} KB po pozivu.',
  ai_see_never: 'Nikada: lozinke i ključevi spremljeni u WRM, tvoja WRM prijava, druge veze, tvoj zaslon terminala.', ai_see_denied: '{n} vrsta osjetljivih datoteka (hashevi lozinki, privatni ključevi, vjerodajnice) nikada se ne čita.',
  ai_see_provider: 'Sve što asistent vidi šalje se pružatelju {provider}.', ai_see_notes_on: 'Tvoje bilješke za ovu vezu su podijeljene.',
  ai_placeholder: 'Pitaj o ovom serveru…', ai_send: 'Pošalji', ai_stop: 'Zaustavi', ai_end: 'Završi AI sesiju', ai_end_q: 'Završiti ovu AI sesiju? Naredba koja se izvršava bit će zaustavljena.',
  ai_sound_on: 'Zvuk je uključen: kratki signal za odobrenje i za svaki odgovor', ai_sound_off: 'Zvuk je isključen', ai_sound_on_toast: 'Zvuk uključen', ai_sound_off_toast: 'Zvuk isključen',
  ai_pending: 'Potrebno je odobrenje', ai_pending_more: '+{n} više', ai_pending_details: 'Detalji',
  ai_close_panel: 'Sakrij panel (sesija nastavlja raditi)', ai_history: 'Moje AI sesije', ai_continue: 'Nastavi sesiju', ai_new: 'Nova sesija',
  ai_approve: 'Odobri', ai_deny: 'Odbij', ai_edit: 'Uredi', ai_note_ph: 'Napomena za asistenta (neobavezno)', ai_expires: 'istječe za {s} s',
  ai_wait_approval: 'Čeka tvoje odobrenje', ai_d_allow: 'izvršeno', ai_d_readonly: 'samo čitanje', ai_d_auto: 'automatski', ai_d_approve: 'treba odobrenje', ai_d_deny: 'odbijeno',
  ai_o_approved: 'odobreno', ai_o_denied: 'odbijeno', ai_o_timeout: 'isteklo', ai_o_cancelled: 'otkazano', ai_edited: 'uređeno',
  ai_exit: 'izlaz {code}', ai_redactions: 'redigirano: {n}', ai_output: 'Izlaz', ai_reason: 'Zašto',
  ai_n_stopped: 'Zaustavljeno.', ai_n_retry: 'Pružatelj je zauzet; pokušavam ponovno za koji trenutak…', ai_n_shrunk: 'Zahtjev je bio prevelik za pružatelja; dugi ispis naredbi je skraćen i zahtjev se ponavlja.', ai_n_refusal: 'Model je odbio ovaj zahtjev.', ai_n_step_limit: 'Asistent je stao nakon najvećeg broja koraka.',
  ai_ended: 'AI sesija je završena', ai_mode_changed: 'Način rada: {mode}', ai_by: 'promijenio: {who}', ai_usage: '{in} ulaznih · {out} izlaznih tokena',
  ai_r_user: 'završio si je', ai_r_admin: 'zaustavio ju je administrator', ai_r_killall: 'administrator je zaustavio sve AI sesije',
  ai_r_switch: 'uključen je prekidač za hitno zaustavljanje', ai_r_off: 'AI asistent je isključen', ai_r_blocked: 'administrator ti je isključio asistenta',
  ai_r_idle: 'završena zbog neaktivnosti', ai_r_policy: 'politika više ne dopušta asistenta na ovoj vezi',
  ai_fm_ctx: 'Direktorij otvoren u pregledniku datoteka', ai_active_on: 'AI aktivan ({mode})',
  ai_auto_until: 'do {time} · {used}/{max} akcija', ai_mode_auto_setup: 'Granice automatskog načina',
  ai_prov_kind: 'Vrsta', ai_prov_name: 'Naziv', ai_prov_models: 'Modeli (odvojeni zarezom)', ai_prov_default: 'Zadani model',
  ai_prov_key: 'API ključ', ai_prov_key_keep: 'nepromijenjen (spremljen šifrirano)', ai_prov_base: 'Osnovni URL (neobavezno: gateway, privatni endpoint)',
  ai_prov_base_req: 'Osnovni URL', ai_prov_endpoint: 'Endpoint (https://<resurs>.openai.azure.com)', ai_prov_deployment: 'Deployment (prazno: naziv modela)',
  ai_prov_apiver: 'Verzija API-ja (prazno: v1 API)', ai_prov_region: 'Regija', ai_prov_project: 'Google Cloud projekt', ai_prov_api: 'API',
  ai_prov_ak: 'AWS access key ID', ai_prov_sk: 'AWS secret access key', ai_prov_st: 'AWS session token (neobavezno)', ai_prov_bkey: 'Bedrock API ključ (umjesto access ključeva)',
  ai_prov_sa: 'Ključ servisnog računa (JSON)', ai_prov_price: 'Cijena po milijun tokena (USD, ulaz / izlaz; prazno: ugrađena za Anthropic)',
  ai_prov_maxtok: 'Najviše izlaznih tokena (0 = 16000)', ai_prov_fallback: 'Zamjenski model kod odbijanja (Anthropic API)', ai_prov_enabled: 'Uključen', ai_prov_test: 'Testiraj',
  ai_prov_test_ok: 'Pružatelj je odgovorio', ai_prov_add: 'Dodaj pružatelja', ai_prov_edit: 'Uredi pružatelja', ai_prov_del_q: 'Obrisati ovog pružatelja? Njegove sesije završavaju.',
  ai_prov_none: 'Još nema pružatelja.', ai_models_col: 'Modeli', ai_prov_secrets: 'Ključevi', ai_kind_anthropic: 'Anthropic API', ai_kind_openai: 'OpenAI API', ai_kind_azure: 'Azure OpenAI',
  ai_kind_bedrock: 'AWS Bedrock', ai_kind_vertex: 'Google Vertex AI', ai_kind_openai_compatible: 'OpenAI-kompatibilan (lokalni modeli)', ai_kind_opencode: 'OpenCode Zen (plaćanje po upotrebi; besplatni modeli bez ključa)',
  ai_kind_opencode_go: 'OpenCode Go (ključ pretplate)',
  ai_subscriptions_note: 'Potrošačke pretplate na Claude.ai i ChatGPT chat ne mogu koristiti druge aplikacije. Ovdje koristi API ključ ili enterprise gateway, ili poveži chat aplikaciju s WRM-om preko MCP-a (Postavke → AI veze).',
  aiw_title: 'Dodavanje AI pružatelja (korak po korak)', aiw_s_pick: 'Pružatelj', aiw_s_key: 'Ključ i model', aiw_s_test: 'Test', aiw_s_done: 'Gotovo',
  aiw_pick: 'Koju AI uslugu želite koristiti?', aiw_p_anthropic: 'Anthropic (Claude)', aiw_p_openai: 'OpenAI', aiw_p_gemini: 'Google Gemini', aiw_p_groq: 'Groq', aiw_p_openrouter: 'OpenRouter', aiw_p_mistral: 'Mistral',
  aiw_p_ollama: 'Ollama (lokalno, besplatno)', aiw_p_custom: 'Ostalo, kompatibilno s OpenAI-jem', aiw_p_advanced: 'Azure, AWS Bedrock, Vertex (napredni obrazac)',
  aiw_key: 'API ključ', aiw_key_opt: 'API ključ (neobavezno za lokalne modele)', aiw_endpoint: 'Adresa (osnovni URL)', aiw_model: 'Model', aiw_model_ph: 'npr. {m}', aiw_name: 'Naziv',
  aiw_key_note: 'Ključ se sprema šifriran i više se nikada ne prikazuje.', aiw_ollama_note: 'Ollama mora raditi na istom računalu kao WRM (ili biti dostupan s njega). Instalirajte ga s ollama.com, zatim pokrenite: ollama pull {m}',
  aiw_testing: 'Spremanje i testiranje…', aiw_ok: 'Radi. Pružatelj je odgovorio{reply}.', aiw_fail: 'Pružatelj nije odgovorio: {err}', aiw_fix: 'Vratite se i provjerite ključ, adresu i naziv modela.',
  aiw_next: 'Spremi i testiraj', aiw_back: 'Natrag', aiw_finish: 'Gotovo', aiw_use: 'Otvorite prozor AI asistenta na vezi prema poslužitelju i odaberite ovog pružatelja. Svaka sesija se bilježi i evidentira u reviziji.', aiw_wizard: 'Čarobnjak za postavljanje', aiw_need: 'Ispunite obavezna polja.',
  aiw_s_ollama: 'Ollama na ovom poslužitelju', aiw_p_xai: 'xAI (Grok)', aiw_p_deepseek: 'DeepSeek', aiw_p_together: 'Together AI', aiw_p_perplexity: 'Perplexity', aiw_p_cohere: 'Cohere',
  aiw_p_fireworks: 'Fireworks AI', aiw_p_cerebras: 'Cerebras', aiw_p_opencode: 'OpenCode Zen (plaćanje po upotrebi; besplatni modeli)', aiw_p_opencode_go: 'OpenCode Go (pretplata)', aiw_free: 'besplatno', ai_deleg_setup: 'Pomoćni modeli: {p}, u upotrebi {n}', ai_deleg_setup_d: 'Asistent može tekstualne podzadatke predati ovim modelima; WRM za svaki zadatak bira najprikladniji, prvo besplatne. Dobivaju samo tekst s maskiranim tajnama i ne mogu ništa pokrenuti.',
  ai_deleg_training_me: 'Koristi i besplatne modele koji moje upite mogu koristiti za treniranje', ai_deleg_training_all: 'Administrator svima dopušta besplatne modele koji mogu trenirati na upitima.', ai_deleg_training_off: 'Administrator ne dopušta besplatne modele koji mogu trenirati na upitima.',
  ai_deleg_excluded: 'Ne koriste se (mogu trenirati na upitima):',
  ai_du_zero_retention: 'ne čuva ništa', ai_du_zero_retention_d: 'Bez zadržavanja: upiti se ne spremaju i ne koriste za treniranje.', ai_du_trains: 'može trenirati', ai_du_trains_d: 'Tijekom besplatnog razdoblja pružatelj može koristiti upite i odgovore za poboljšanje svojih modela.',
  ai_du_unknown: 'uvjeti nepoznati', ai_du_unknown_d: 'WRM ne zna uvjete za podatke ovog besplatnog modela; tretira se kao model koji može trenirati.', ai_du_paid: 'plaćeno', ai_du_paid_d: 'Naplaćuje se OpenCode Zen radnom prostoru; Zen navodi da ne zadržava podatke (OpenAI i Anthropic čuvaju zahtjeve 30 dana).',
  ai_du_provider: 'vlastiti pružatelj', ai_du_provider_d: 'Vlastiti pružatelj vaše organizacije i njegovi uvjeti.',
  p_ai_delegate_training: 'Besplatni modeli koji mogu trenirati na upitima', p_ai_delegate_training_d: 'Nikad: ne koriste se. Korisnik: svaki korisnik bira u asistentu (zadano isključeno). Svi: koriste se za sve.',
  po_ai_delegate_training_off: 'Nikad', po_ai_delegate_training_user: 'Svaki korisnik bira', po_ai_delegate_training_all: 'Svi', ai_deleg_h: 'Delegiranje pomoćnim modelima', ai_deleg_help: 'Asistent može samostalne tekstualne podzadatke (sažeti dugi log, objasniti grešku, napisati prijedlog isječka) predati jeftinijem ili besplatnom pomoćnom modelu. Pomoćni model dobiva samo tekst koji mu asistent preda, s maskiranim tajnama; nema alata i ne može ništa pokrenuti na serveru. Svako delegiranje vidi se u live prikazu, zapisu sesije i revizijskom zapisu s modelom, veličinom, tokenima i cijenom. Besplatni OpenCode Zen modeli mogu primljeno koristiti za poboljšanje svojih modela: dopustite samo modele čiji uvjeti odgovaraju vašim podacima.',
  ai_deleg_off: 'Delegiranje je isključeno.', ai_deleg_noprov: 'Delegiranje je uključeno, ali pomoćni pružatelj nije odabran.', ai_deleg_disabled: 'Odabrani pomoćni pružatelj je isključen.', ai_deleg_on: 'Aktivno: {p} s {m}', ai_deleg_load: 'Učitaj modele',
  p_ai_delegate: 'Dopusti delegiranje pomoćnim modelima', p_ai_delegate_d: 'Asistentu nudi alat delegate_task (samo ugrađeni asistent; MCP klijenti koriste svoje agente).',
  p_ai_delegate_provider: 'Pomoćni pružatelj', p_ai_delegate_provider_d: 'Pružatelj organizacije, na primjer OpenCode Zen (besplatni modeli rade bez ključa).',
  p_ai_delegate_models: 'Pomoćni modeli', p_ai_delegate_models_d: 'Odvojeni zarezom; prvi je zadani. Prazno: automatski, svi upotrebljivi modeli pružatelja (za OpenCode Zen aktualni katalog). WRM za svaki zadatak bira odgovarajući.', p_ai_delegate_models_ph: 'big-pickle',
  p_ai_delegate_max_kb: 'Ograničenje veličine delegiranja (KB)', p_ai_delegate_max_kb_d: 'Najviše teksta koje jedno delegiranje smije poslati pomoćnom modelu.',
  aiw_n_opencode: 'Bez ključa rade samo besplatni OpenCode Zen modeli (ograničeno po IP adresi; neki besplatni modeli mogu koristiti upite za poboljšanje svojih modela, zato ne šaljite povjerljive podatke). S ključem s opencode.ai rade svi Zen modeli, a naplaćuju se tom radnom prostoru. Nakon testa upotrijebite "Učitaj modele" za odabir s aktualnog popisa.',
  aiw_n_opencode_go: 'Koristi OpenCode Go pretplatničke endpointe, ne Zen: zalijepite Go API ključ s opencode.ai/console/go. Go ključ nema pay-as-you-go saldo, pa bi Zen odgovorio "insufficient funds". Upotrijebite "Učitaj modele" za odabir s Go popisa.',
  aiw_model_note: 'Nazivi modela se često mijenjaju. Upišite bilo koji naziv modela (više njih odvojite zarezom; prvi je zadani) ili spremite i na zadnjem koraku odaberite „Učitaj popis modela”.',
  aiw_n_perplexity: 'Perplexity modeli ne mogu pozivati alate pa asistent s njima možda neće moći izvoditi provjere.',
  aiw_o_intro: 'Ollama pokreće besplatne AI modele na računalu koje kontrolirate. WRM mu pristupa preko adrese kompatibilne s OpenAI-jem i ne treba API ključ. Asistent i dalje radi preko WRM-a, pa se svaka sesija bilježi i evidentira u reviziji.',
  aiw_o_checking: 'Tražim Ollamu…', aiw_o_running: 'Ollama {v} radi na {url}.', aiw_o_notrunning: 'Ollama nije pronađena na {url}.',
  aiw_o_models: 'Instalirani modeli', aiw_o_nomodels: 'Još nije instaliran nijedan model. Preuzmite ga (3. korak dolje).', aiw_o_use_model: 'Koristi',
  aiw_o_manual: 'Ove naredbe pokrenite sami na poslužitelju ili ih prepustite WRM-u (samo administratori):', aiw_o_os: 'Operacijski sustav WRM poslužitelja: {os}',
  aiw_o_s_install: '1. Instalirajte Ollamu', aiw_o_s_start: '2. Pokrenite je kao pozadinsku uslugu', aiw_o_s_pull: '3. Preuzmite model',
  aiw_o_do_install: 'Instaliraj preko WRM-a', aiw_o_do_start: 'Pokreni preko WRM-a', aiw_o_do_pull: 'Preuzmi preko WRM-a', aiw_o_run: 'Pokreni',
  aiw_o_confirm: 'WRM će na WRM poslužitelju pokrenuti:\n\n{cmd}\n\nRadnja se bilježi u reviziji. Nastaviti?',
  aiw_o_noadmin: 'Ollamu iz WRM-a može instalirati i pokrenuti samo administrator. Zatražite ga ili sami pokrenite gornje naredbe.',
  aiw_o_nopolicy: 'Instalacija iz WRM-a je isključena. Administrator može uključiti pravilo „Dopusti administratorima instalaciju i pokretanje Ollame iz WRM-a” u Admin → AI.',
  aiw_o_nosupport: 'Automatsko postavljanje nije dostupno na ovom operacijskom sustavu. Slijedite korake s ollama.com/download.',
  aiw_o_run_running: 'U tijeku…', aiw_o_run_done: 'Završeno.', aiw_o_run_failed: 'Neuspjelo.', aiw_o_refresh: 'Provjeri ponovno', aiw_o_copy: 'Kopiraj', aiw_o_copied: 'Kopirano.',
  aiw_o_history: 'Nedavne radnje', aiw_o_model_ph: 'model, npr. qwen2.5-coder:7b', aiw_o_next: 'Nastavi',
  aiw_load_models: 'Učitaj popis modela', aiw_models_loaded: 'Modeli koje nudi pružatelj (kliknite za dodavanje):', aiw_models_none: 'Pružatelj nije vratio nijedan model.', aiw_models_added: 'Model je dodan.',
  p_ai_ollama_manage: 'Dopusti administratorima instalaciju i pokretanje Ollame iz WRM-a', p_ai_ollama_manage_d: 'Dopušta administratorima da iz čarobnjaka za pružatelje instaliraju Ollamu, pokrenu je kao pozadinsku uslugu i preuzmu modele na WRM poslužitelju. Svaka radnja se potvrđuje, bilježi i evidentira u reviziji. Isključeno prema zadanim postavkama.',
  adm_ai_kill: 'Prekidač za hitno zaustavljanje', ai_kill_all: 'Odmah zaustavi sve AI sesije', ai_kill_all_q: 'Odmah završiti sve AI sesije svih korisnika?', ai_killed_n: 'Završeno sesija: {n}',
  ai_kill_switch_on: 'Prekidač je UKLJUČEN: nitko ne može koristiti AI asistenta.', ai_kill_switch_off: 'Prekidač je isključen.',
  ai_policies: 'Politike', ai_providers_org: 'Pružatelji organizacije', ai_sessions: 'AI sesije', ai_usage_users: 'Potrošnja po korisniku', ai_destructive: 'Uvijek blokirano (destruktivni uzorci)',
  ai_transcript: 'Transkript', ai_replay: 'Reprodukcija', ai_kill: 'Prekini', ai_block: 'Isključi za ovog korisnika', ai_unblock: 'Ponovno uključi', ai_blocked: 'isključen',
  ai_live: 'aktivna', ai_cost: 'Trošak', ai_tokens: 'Tokeni', ai_requests: 'Zahtjevi', ai_tool_calls: 'Pozivi alata', ai_mode_rules_help: 'Pravila načina rada (najstroža pobjeđuju): JSON, npr. [{"match":"tag","value":"prod","modes":["read_only"]}, {"match":"folder","value":"Production","modes":["read_only","ask"]}]. match: tag, folder, connection (id), host (zamjenski znakovi).',
  p_ai_assistant: 'AI asistent', p_ai_assistant_d: 'Tko smije koristiti ugrađenog AI asistenta.', po_ai_assistant_off: 'Isključen', po_ai_assistant_admins: 'Administratori', po_ai_assistant_all: 'Svi',
  p_ai_kill_switch: 'Prekidač za hitno zaustavljanje', p_ai_kill_switch_d: 'Odmah završava sve AI sesije i blokira nove dok se ne isključi.',
  p_ai_modes: 'Dopušteni načini rada', p_ai_modes_d: 'Globalna zadana vrijednost, odvojeno zarezom: read_only, ask, auto. Pravila je mogu samo suziti.', p_ai_modes_ph: 'read_only,ask',
  p_ai_default_mode: 'Zadani način rada', p_ai_default_mode_d: 'Način rada nove sesije kad je dopušten.', po_ai_default_mode_read_only: 'Samo čitanje', po_ai_default_mode_ask: 'Pitaj prije svake promjene',
  p_ai_mode_rules: 'Pravila načina rada', p_ai_mode_rules_d: 'Po oznaci, mapi, vezi ili hostu; pobjeđuje najstrože pravilo koje odgovara.',
  p_ai_personal_keys: 'Osobni API ključevi', p_ai_personal_keys_d: 'Tko smije dodati vlastite ključeve pružatelja.', po_ai_personal_keys_off: 'Nitko', po_ai_personal_keys_admins: 'Administratori', po_ai_personal_keys_all: 'Svi',
  p_ai_provider_kinds: 'Dopuštene vrste pružatelja', p_ai_provider_kinds_d: 'anthropic, openai, azure, bedrock, vertex, openai_compatible, opencode (Zen), opencode_go (Go pretplata)', p_ai_provider_kinds_ph: 'anthropic,azure',
  p_ai_models: 'Dopušteni modeli', p_ai_models_d: 'Uzorci odvojeni zarezom (claude-*); prazno = svi modeli pružatelja.', p_ai_models_ph: 'claude-*',
  p_ai_redact_output: 'Rediguj izlaz', p_ai_redact_output_d: 'Zamijeni tajne (lozinke, tokene, privatne ključeve …) prije slanja izlaza pružatelju. Transkripti i audit uvijek su redigirani.',
  p_ai_share_notes: 'Dopusti dijeljenje bilješki', p_ai_share_notes_d: 'Korisnici mogu izabrati da asistentu pošalju bilješke veze.',
  p_ai_transcript_retention_days: 'Čuvanje transkripata (dani)', p_ai_transcript_retention_days_d: 'AI transkripti brišu se nakon tog vremena (snimke prate čuvanje snimki).',
  p_ai_approval_timeout_seconds: 'Rok za odobrenje (sekunde)', p_ai_approval_timeout_seconds_d: 'Odobrenje na koje se ne odgovori na vrijeme se odbija.',
  p_ai_auto_max_minutes: 'Automatski način: najviše minuta', p_ai_auto_max_minutes_d: 'Najdulje vremensko ograničenje koje korisnik smije izabrati.',
  p_ai_auto_max_actions: 'Automatski način: najviše akcija', p_ai_auto_max_actions_d: 'Najveći broj automatskih akcija po uključivanju.',
  p_ai_auto_until_grant: 'AI veze: automatski bez ograničenja', p_ai_auto_until_grant_d: 'Korisnici smiju za AI vezu odabrati automatski način bez ograničenja vremena i broja radnji; tada traje dok god veza vrijedi.',
  p_ai_sensitive_reads: 'Datoteke koje mogu sadržavati tajne', p_ai_sensitive_reads_d: 'Ključevi, vjerodajnice i putanje iz ai_read_deny_paths: čitljive nakon što korisnik odobri svako čitanje (vrijednosti nalik tajnama se skrivaju) ili uvijek blokirane.', po_ai_sensitive_reads_ask: 'Uz odobrenje', po_ai_sensitive_reads_deny: 'Uvijek blokirano',
  p_ai_allow_power: 'Gašenje i ponovno pokretanje', p_ai_allow_power_d: 'Blokirano ili dopušteno uz odobrenje svake naredbe (nikad automatski).', po_ai_allow_power_off: 'Uvijek blokirano', po_ai_allow_power_ask: 'Uz odobrenje',
  p_ai_blocked_commands: 'Dodatno blokirane naredbe', p_ai_blocked_commands_d: 'Uzorci odvojeni zarezom, odbijeni u svakom načinu, uz ugrađeni popis.', p_ai_blocked_commands_ph: 'docker system prune*',
  p_ai_read_deny_paths: 'Dodatne nečitljive datoteke', p_ai_read_deny_paths_d: 'Uzorci putanja odvojeni zarezom koje asistent nikad ne čita (uzorak bez / odgovara nazivu datoteke).', p_ai_read_deny_paths_ph: '/srv/app/.env, *.p12',
  p_ai_provider_header_seconds: 'Čekanje odgovora pružatelja (sekunde)', p_ai_provider_header_seconds_d: 'Vrijeme za učitavanje modela, obradu upita i čekanje u redu prije prvog odgovora.',
  p_ai_provider_request_seconds: 'Ograničenje zahtjeva pružatelju (sekunde)', p_ai_provider_request_seconds_d: 'Ukupno trajanje zahtjeva uključujući prijenos odgovora. Zaustavljanje i dalje odmah prekida zahtjev.',
  p_ai_provider_stream_idle_seconds: 'Ograničenje mirovanja odgovora (sekunde)', p_ai_provider_stream_idle_seconds_d: 'Prekida odgovor koji ne šalje podatke. Zahtjevi i radnje ne ponavljaju se automatski.',
  p_ai_command_timeout_seconds: 'Vremensko ograničenje naredbe (sekunde)', p_ai_command_timeout_seconds_d: 'Poziv alata zaustavlja se nakon tog vremena.',
  p_ai_output_max_kb: 'Ograničenje izlaza (KB)', p_ai_output_max_kb_d: 'Po pozivu alata, ujedno ograničenje veličine za čitanje datoteka.',
  p_ai_max_steps: 'Koraka po zahtjevu', p_ai_max_steps_d: 'Zahtjeva prema modelu po poruci korisnika.',
  p_ai_idle_minutes: 'Neaktivnost (minute)', p_ai_idle_minutes_d: 'Neaktivna AI sesija završava nakon tog vremena.',
});

Object.assign(LANGS.en, {
 ai_status_ready:'The assistant is enabled for this account.', ai_status_blocked:'The assistant is unavailable for this account.', ai_status_test:'A successful provider test confirms connectivity. The assistant also needs account permission and an SSH or SFTP server connection.',
 ai_status_admin:'Administrators can enable the assistant in Admin → AI assistant. Environment-forced settings are shown there.', ai_status_open:'Open a terminal or file window on an SSH/SFTP connection, then click AI in its header.', ai_status_no_modes:'No AI permission mode is enabled.', ai_status_providers:'Available providers', ai_status_refresh:'Refresh status', ai_status_setup:'Open AI administration', ai_status_connections:'Open server windows', ai_status_config_error:'Availability could not be checked. Refresh status before using the assistant.',
});
Object.assign(LANGS.hr, {
 ai_status_ready:'Asistent je omogućen za ovaj račun.', ai_status_blocked:'Asistent nije dostupan za ovaj račun.', ai_status_test:'Uspješan test pružatelja potvrđuje povezanost. Za asistenta su potrebni i dopuštenje računa te SSH ili SFTP veza prema serveru.',
 ai_status_admin:'Administratori mogu omogućiti asistenta u Admin → AI asistent. Tamo su označene i postavke zadane varijablama okruženja.', ai_status_open:'Otvorite terminal ili datoteke na SSH/SFTP vezi pa kliknite AI u zaglavlju prozora.', ai_status_no_modes:'Nije omogućen nijedan način dopuštenja za AI.', ai_status_providers:'Dostupni pružatelji', ai_status_refresh:'Osvježi stanje', ai_status_setup:'Otvori AI administraciju', ai_status_connections:'Otvoreni prozori servera', ai_status_config_error:'Dostupnost nije provjerena. Osvježite stanje prije korištenja asistenta.',
});

Object.assign(LANGS.en, {
 ai_engine: 'Engine', ai_engine_wrm: 'WRM assistant', ai_engine_oc: 'OpenCode agent',
 ai_engine_wrm_d: 'The built-in assistant: WRM talks to the model and runs every tool itself.',
 ai_engine_oc_d: 'OpenCode runs on the WRM host as a managed helper with its own agent loop and prompts. Its own shell, file and web tools are switched off: it reaches the server only through WRM tools, so every command is shown and approved here like with the WRM assistant (unless full access is on). Works with OpenCode Zen, Anthropic, OpenAI and OpenAI-compatible providers.',
 ai_oc_agent: 'OpenCode agent', ai_oc_agent_build: 'build (does the work)', ai_oc_agent_plan: 'plan (looks and plans, changes nothing)',
 ai_oc_unavail: 'OpenCode is not available', ai_oc_starting: 'Starting OpenCode…', ai_oc_mode_fixed: 'The mode of an OpenCode agent is chosen when it starts.',
 ai_oc_kind_off: 'not usable with OpenCode', ai_n_delegate_retry: 'The helper model failed; trying the next one.',
 ai_oc_h: 'OpenCode agents', ai_oc_help: 'Users can pick OpenCode as the engine of the assistant. WRM downloads the official OpenCode build once (from the npm registry, MIT licence, about 50 to 150 MB), checks its sha512 against the registry and keeps it next to the database. Each agent runs as a local helper on 127.0.0.1 with its own folder; its shell, file and web tools are off and WRM is its only tool server.',
 ai_oc_installed: 'Installed: OpenCode {v}', ai_oc_custom: 'Using the binary from ai_opencode_path: {p}', ai_oc_missing: 'Not installed yet.', ai_oc_unsupported: 'OpenCode has no build for {os}; set a binary path instead.',
 ai_oc_installing: 'Installing…', ai_oc_install: 'Download and install', ai_oc_reinstall: 'Install the selected version', ai_oc_refresh: 'Refresh',
 ai_oc_install_q: 'Download OpenCode {v} ({pkg}) from {reg}? WRM checks the sha512 checksum from the registry before using it. OpenCode is MIT licensed. The file stays in {root}.',
 ai_oc_by: 'by {u}, {t}', ai_oc_sessions: 'Running agents: {n} of {max}', ai_oc_log: 'Installation log', ai_oc_failed: 'Installation failed',
 p_ai_opencode: 'OpenCode agents', p_ai_opencode_d: 'Who may use OpenCode as the engine of the assistant. The normal AI assistant permissions still apply.',
 po_ai_opencode_off: 'Nobody', po_ai_opencode_admins: 'Administrators', po_ai_opencode_all: 'Everyone allowed to use the assistant',
 p_ai_opencode_version: 'OpenCode version', p_ai_opencode_version_d: 'The version WRM installs. After a change press Install again.', p_ai_opencode_version_ph: '1.19.3',
 p_ai_opencode_registry: 'Download registry', p_ai_opencode_registry_d: 'An npm registry over HTTPS (a mirror or proxy in closed networks).', p_ai_opencode_registry_ph: 'https://registry.npmjs.org',
 p_ai_opencode_path: 'OpenCode binary', p_ai_opencode_path_d: 'Absolute path of an OpenCode binary you installed yourself. Empty: the one WRM installed.', p_ai_opencode_path_ph: '/usr/local/bin/opencode',
 p_ai_opencode_max_sessions: 'Agents at the same time', p_ai_opencode_max_sessions_d: 'The most OpenCode agents running at once on this WRM host (each one is a separate process).',
});
Object.assign(LANGS.hr, {
 ai_engine: 'Pogon', ai_engine_wrm: 'WRM asistent', ai_engine_oc: 'OpenCode agent',
 ai_engine_wrm_d: 'Ugrađeni asistent: WRM razgovara s modelom i sam pokreće svaki alat.',
 ai_engine_oc_d: 'OpenCode radi na WRM hostu kao upravljani pomoćni program sa svojom petljom agenta i uputama. Njegovi vlastiti alati za ljusku, datoteke i web su isključeni: do servera dolazi samo preko WRM alata, pa se svaka naredba prikazuje i odobrava ovdje kao kod WRM asistenta (osim ako je uključen puni pristup). Radi s OpenCode Zen, Anthropic, OpenAI i OpenAI-kompatibilnim pružateljima.',
 ai_oc_agent: 'OpenCode agent', ai_oc_agent_build: 'build (obavlja posao)', ai_oc_agent_plan: 'plan (gleda i planira, ništa ne mijenja)',
 ai_oc_unavail: 'OpenCode nije dostupan', ai_oc_starting: 'Pokrećem OpenCode…', ai_oc_mode_fixed: 'Način rada OpenCode agenta bira se pri pokretanju.',
 ai_oc_kind_off: 'ne radi s OpenCodeom', ai_n_delegate_retry: 'Pomoćni model nije uspio; pokušavam sljedeći.',
 ai_oc_h: 'OpenCode agenti', ai_oc_help: 'Korisnici mogu odabrati OpenCode kao pogon asistenta. WRM jednom preuzme službenu OpenCode verziju (iz npm registra, MIT licenca, oko 50 do 150 MB), provjeri njen sha512 prema registru i čuva je pokraj baze. Svaki agent radi kao lokalni pomoćni program na 127.0.0.1 sa svojom mapom; alati za ljusku, datoteke i web su mu isključeni, a WRM mu je jedini poslužitelj alata.',
 ai_oc_installed: 'Instalirano: OpenCode {v}', ai_oc_custom: 'Koristi se program iz ai_opencode_path: {p}', ai_oc_missing: 'Još nije instaliran.', ai_oc_unsupported: 'OpenCode nema verziju za {os}; postavite putanju do programa.',
 ai_oc_installing: 'Instaliram…', ai_oc_install: 'Preuzmi i instaliraj', ai_oc_reinstall: 'Instaliraj odabranu verziju', ai_oc_refresh: 'Osvježi',
 ai_oc_install_q: 'Preuzeti OpenCode {v} ({pkg}) s {reg}? WRM prije korištenja provjerava sha512 kontrolni zbroj iz registra. OpenCode ima MIT licencu. Datoteka ostaje u {root}.',
 ai_oc_by: '{u}, {t}', ai_oc_sessions: 'Pokrenuti agenti: {n} od {max}', ai_oc_log: 'Zapis instalacije', ai_oc_failed: 'Instalacija nije uspjela',
 p_ai_opencode: 'OpenCode agenti', p_ai_opencode_d: 'Tko smije koristiti OpenCode kao pogon asistenta. Obična dopuštenja AI asistenta i dalje vrijede.',
 po_ai_opencode_off: 'Nitko', po_ai_opencode_admins: 'Administratori', po_ai_opencode_all: 'Svi kojima je asistent dopušten',
 p_ai_opencode_version: 'Verzija OpenCodea', p_ai_opencode_version_d: 'Verzija koju WRM instalira. Nakon promjene ponovno pritisnite Instaliraj.', p_ai_opencode_version_ph: '1.19.3',
 p_ai_opencode_registry: 'Registar za preuzimanje', p_ai_opencode_registry_d: 'npm registar preko HTTPS-a (zrcalo ili proxy u zatvorenim mrežama).', p_ai_opencode_registry_ph: 'https://registry.npmjs.org',
 p_ai_opencode_path: 'OpenCode program', p_ai_opencode_path_d: 'Apsolutna putanja do OpenCode programa koji ste sami instalirali. Prazno: onaj koji je instalirao WRM.', p_ai_opencode_path_ph: '/usr/local/bin/opencode',
 p_ai_opencode_max_sessions: 'Agenata istovremeno', p_ai_opencode_max_sessions_d: 'Najviše OpenCode agenata koji istovremeno rade na ovom WRM hostu (svaki je zaseban proces).',
});

Object.assign(LANGS.en, {
 ai_oc_sandbox: 'Also OpenCode\'s own tools, in a Docker sandbox',
 ai_oc_sandbox_d: 'OpenCode can then also use its own shell, files and web fetch, but only in an isolated Docker container on the WRM host ({image}; only this session\'s folder, no root, limited memory and CPU). Every command, file change and web fetch there waits for your approval and cannot be edited, only approved or refused. The server is still reached only through WRM.',
 ai_oc_sandbox_off: 'Docker sandbox unavailable', ai_oc_sb_on: 'Docker sandbox', ai_oc_noedit: 'OpenCode runs it exactly as shown: approve or refuse.',
 ai_oc_sb_h: 'Docker sandbox', ai_oc_sb_ok: 'Docker {docker} · image {image} · OpenCode for the sandbox: ready', ai_oc_sb_err: 'Docker sandbox not ready: {e}',
 p_ai_opencode_sandbox: 'OpenCode\'s own tools in a Docker sandbox', p_ai_opencode_sandbox_d: 'Who may start an OpenCode agent that also uses its own shell, files and web fetch inside a Docker container on the WRM host. Each use asks the user. Needs Docker; Install then also prepares the sandbox.',
 po_ai_opencode_sandbox_off: 'Nobody', po_ai_opencode_sandbox_admins: 'Administrators', po_ai_opencode_sandbox_all: 'Everyone allowed to use OpenCode agents',
 p_ai_opencode_sandbox_image: 'Sandbox image', p_ai_opencode_sandbox_image_d: 'The Docker image the sandbox runs (glibc Linux). A minimal one is safest; add tools the agent may use.', p_ai_opencode_sandbox_image_ph: 'debian:stable-slim',
});
Object.assign(LANGS.hr, {
 ai_oc_sandbox: 'I vlastiti alati OpenCodea, u Docker sandboxu',
 ai_oc_sandbox_d: 'OpenCode tada može koristiti i svoju ljusku, datoteke i dohvat weba, ali samo u izoliranom Docker kontejneru na WRM hostu ({image}; samo mapa ove sesije, bez roota, ograničena memorija i CPU). Svaka naredba, promjena datoteke i dohvat weba ondje čeka tvoje odobrenje i ne može se uređivati, samo odobriti ili odbiti. Do servera i dalje dolazi samo preko WRM-a.',
 ai_oc_sandbox_off: 'Docker sandbox nije dostupan', ai_oc_sb_on: 'Docker sandbox', ai_oc_noedit: 'OpenCode pokreće točno ovo što vidiš: odobri ili odbij.',
 ai_oc_sb_h: 'Docker sandbox', ai_oc_sb_ok: 'Docker {docker} · slika {image} · OpenCode za sandbox: spreman', ai_oc_sb_err: 'Docker sandbox nije spreman: {e}',
 p_ai_opencode_sandbox: 'Vlastiti alati OpenCodea u Docker sandboxu', p_ai_opencode_sandbox_d: 'Tko smije pokrenuti OpenCode agenta koji koristi i svoju ljusku, datoteke i dohvat weba unutar Docker kontejnera na WRM hostu. Svaka upotreba pita korisnika. Treba Docker; Instaliraj tada pripremi i sandbox.',
 po_ai_opencode_sandbox_off: 'Nitko', po_ai_opencode_sandbox_admins: 'Administratori', po_ai_opencode_sandbox_all: 'Svi kojima su OpenCode agenti dopušteni',
 p_ai_opencode_sandbox_image: 'Slika sandboxa', p_ai_opencode_sandbox_image_d: 'Docker slika koju sandbox pokreće (glibc Linux). Minimalna je najsigurnija; dodaj alate koje agent smije koristiti.', p_ai_opencode_sandbox_image_ph: 'debian:stable-slim',
});

Object.assign(LANGS.en, {
  ai_continue_chat: 'Continue chat', ai_resumed_from: 'Continued from session #{n}', ai_resumed_msgs: 'earlier questions', ai_rename: 'Rename the session',
  ai_recent: 'Recent chats on this server (continue where you stopped):', ai_purpose_logs: 'log assistant', ai_purpose_file: 'file assistant',
  p_ai_show_thinking: 'Show the assistant\'s thinking', p_ai_show_thinking_d: 'Ask models that support it (Claude 4.6 and later, reasoning models of OpenAI-compatible servers) for a summary of their reasoning and show it live in the chat\'s work log.',
  p_ai_auto_titles: 'Generate session titles', p_ai_auto_titles_d: 'After the first answer one short extra model request writes a title for the chat (otherwise the first question is used). Titles can be renamed.',
});
Object.assign(LANGS.hr, {
  ai_continue_chat: 'Nastavi chat', ai_resumed_from: 'Nastavak sesije #{n}', ai_resumed_msgs: 'ranijih pitanja', ai_rename: 'Preimenuj sesiju',
  ai_recent: 'Nedavni chatovi na ovom serveru (nastavite gdje ste stali):', ai_purpose_logs: 'asistent za logove', ai_purpose_file: 'asistent za file',
  p_ai_show_thinking: 'Prikaži razmišljanje asistenta', p_ai_show_thinking_d: 'Modele koji to podržavaju (Claude 4.6 i noviji, modeli s razmišljanjem na OpenAI-kompatibilnim serverima) traži sažetak razmišljanja i prikazuje ga uživo u radnom logu chata.',
  p_ai_auto_titles: 'Generiraj naslove sesija', p_ai_auto_titles_d: 'Nakon prvog odgovora jedan kratki dodatni zahtjev modelu napiše naslov chata (inače se koristi prvo pitanje). Naslove možete preimenovati.',
});
// v13.8.0: personal AI settings (Settings → AI assistant) and saved sessions
Object.assign(LANGS.en, {
  aip_title: 'My AI settings', aip_d: 'These settings are yours alone: every WRM user chooses their own defaults, their own provider accounts and their own AI apps.',
  aip_engine: 'Default engine', aip_provider: 'Default provider', aip_provider_first: 'The first one offered', aip_model: 'Default model', aip_model_prov: 'The provider\'s default',
  aip_mode: 'Default permission mode', aip_mode_conn: 'As the connection allows', aip_thinking: 'Show the assistant\'s thinking in the work log', aip_thinking_off: 'Turned off by the administrator for everybody.',
  aip_save: 'Save my AI settings', aip_saved: 'Your AI settings are saved', aip_accounts: 'My own AI accounts (API keys)', aip_accounts_d: 'Your own provider keys (Anthropic, OpenAI, Azure, Bedrock, Vertex, OpenCode, an OpenAI-compatible server or Ollama). Only you can use them; they are stored encrypted and never sent to the browser.',
  aip_accounts_off: 'Your administrator has not allowed personal API keys (Admin → AI assistant → Personal API keys). You can use the organisation\'s providers or an AI app below.',
  aip_accounts_none: 'You have no own AI account yet.', aip_manage: 'Manage my keys', aip_apps: 'My Claude, ChatGPT or other AI app', aip_apps_d: 'To work with your own Claude, ChatGPT or Cursor subscription, connect that app to WRM over MCP. It runs with your account, sees only the servers you choose, and WRM asks you before every change.',
  aip_apps_btn: 'AI connections', aip_apps_off: 'AI connections over MCP are turned off by the administrator.', aip_sessions: 'My AI sessions', aip_sessions_d: 'Every AI session you had (assistant, log and file assistants, OpenCode, AI apps) is in the 🧠 AI sessions panel next to Sessions: open, continue, save or export it.',
  aip_sessions_btn: 'Show AI sessions', ai_resumed_steps: '{n} earlier steps of an AI app', p_ai_keep_saved_sessions: 'Keep saved AI sessions', p_ai_keep_saved_sessions_d: 'Sessions a user saved (☆ in AI sessions) are kept beyond the transcript retention. Turn off to delete every session after the retention.',
});
Object.assign(LANGS.hr, {
  aip_title: 'Moje AI postavke', aip_d: 'Ove postavke su samo tvoje: svaki WRM korisnik bira svoje zadane vrijednosti, svoje račune kod pružatelja i svoje AI aplikacije.',
  aip_engine: 'Zadani pogon', aip_provider: 'Zadani pružatelj', aip_provider_first: 'Prvi ponuđeni', aip_model: 'Zadani model', aip_model_prov: 'Zadani model pružatelja',
  aip_mode: 'Zadani način dopuštenja', aip_mode_conn: 'Kako veza dopušta', aip_thinking: 'Prikaži razmišljanje asistenta u tijeku rada', aip_thinking_off: 'Administrator je to isključio za sve.',
  aip_save: 'Spremi moje AI postavke', aip_saved: 'Tvoje AI postavke su spremljene', aip_accounts: 'Moji AI računi (API ključevi)', aip_accounts_d: 'Tvoji vlastiti ključevi pružatelja (Anthropic, OpenAI, Azure, Bedrock, Vertex, OpenCode, OpenAI-kompatibilni server ili Ollama). Možeš ih koristiti samo ti; spremljeni su šifrirano i nikad se ne šalju pregledniku.',
  aip_accounts_off: 'Administrator nije dopustio osobne API ključeve (Admin → AI asistent → Osobni API ključevi). Možeš koristiti pružatelje organizacije ili AI aplikaciju niže.',
  aip_accounts_none: 'Još nemaš vlastiti AI račun.', aip_manage: 'Upravljaj mojim ključevima', aip_apps: 'Moj Claude, ChatGPT ili druga AI aplikacija', aip_apps_d: 'Za rad sa svojom Claude, ChatGPT ili Cursor pretplatom poveži tu aplikaciju s WRM-om preko MCP-a. Radi s tvojim računom, vidi samo poslužitelje koje odabereš, a WRM te pita prije svake promjene.',
  aip_apps_btn: 'AI veze', aip_apps_off: 'Administrator je isključio AI veze preko MCP-a.', aip_sessions: 'Moje AI sesije', aip_sessions_d: 'Sve tvoje AI sesije (asistent, asistenti za logove i datoteke, OpenCode, AI aplikacije) su u panelu 🧠 AI sesije pokraj Sesija: otvori, nastavi, spremi ili izvezi.',
  aip_sessions_btn: 'Prikaži AI sesije', ai_resumed_steps: '{n} ranijih koraka AI aplikacije', p_ai_keep_saved_sessions: 'Čuvaj spremljene AI sesije', p_ai_keep_saved_sessions_d: 'Sesije koje je korisnik spremio (☆ u AI sesijama) čuvaju se i nakon roka čuvanja transkripata. Isključi da se sve sesije brišu nakon roka.',
});
(function () {
  const $ = id => document.getElementById(id);
  const S = {}; // per window: {sid, es, seq, view, cfg, pol, busy}
  let cfg = null;
  const MODES = ['read_only', 'ask', 'auto'];

  const css = `
.ai-panel { position: absolute; right: 0; bottom: 0; width: 380px; max-width: 100%; display: flex; flex-direction: column; background: var(--bg2); border-left: 1px solid var(--border2); z-index: 5; border-radius: 0 0 11px 0; min-width: 0; }
@container (max-width: 760px) { .win.ai-open .ai-panel { width: 100%; border-left: none; border-radius: 0 0 11px 11px; } }
.ai-panel[data-dock=left] { border-left: none; border-right: 1px solid var(--border2); border-radius: 0 0 0 11px; }
.ai-panel[data-dock=top] { border-left: none; border-bottom: 1px solid var(--border2); border-radius: 0; }
.ai-panel[data-dock=bottom] { border-left: none; border-top: 1px solid var(--border2); border-radius: 0 0 11px 11px; }
.ai-panel[data-dock=max] { border: none; border-radius: 0 0 11px 11px; }
.ai-panel[data-dock=float] { border: 1px solid var(--border2); border-radius: 10px; box-shadow: var(--shadow-md, 0 8px 32px rgba(0,0,0,.5)); overflow: hidden; }
.ai-panel:not(.ai-compact) .ai-head { cursor: move; user-select: none; touch-action: none; }
.ai-panel[data-dock=max] .ai-head { cursor: default; }
.ai-panel.ai-compact .ai-dockb, .ai-panel.ai-compact .ai-rs, .ai-panel[data-dock=max] .ai-rs, .ai-panel[data-dock=max] .ai-dockb:not(.ai-xb) { display: none; }
.ai-ib.on { background: var(--bg4); color: var(--accent); }
.ai-rs { position: absolute; z-index: 6; touch-action: none; }
.ai-panel[data-dock=right] .ai-rs { left: -3px; top: 0; bottom: 0; width: 7px; cursor: ew-resize; }
.ai-panel[data-dock=left] .ai-rs { right: -3px; top: 0; bottom: 0; width: 7px; cursor: ew-resize; }
.ai-panel[data-dock=top] .ai-rs { left: 0; right: 0; bottom: -3px; height: 7px; cursor: ns-resize; }
.ai-panel[data-dock=bottom] .ai-rs { left: 0; right: 0; top: -3px; height: 7px; cursor: ns-resize; }
.ai-panel[data-dock=float] .ai-rs { right: 0; bottom: 0; width: 16px; height: 16px; cursor: nwse-resize; background: linear-gradient(135deg, transparent 55%, var(--text3, #888) 55%, var(--text3, #888) 62%, transparent 62%, transparent 76%, var(--text3, #888) 76%, var(--text3, #888) 83%, transparent 83%); }
.ai-panel input[type=text], .ai-panel input[type=number], .ai-panel select, .ai-panel textarea, .ai-modal input[type=text], .ai-modal input[type=password], .ai-modal input[type=number], .ai-modal select, .ai-modal textarea {
  background: var(--bg); border: 1px solid var(--border2); color: var(--text); border-radius: 6px; padding: 6px 8px; font-size: 12px; outline: none; font-family: inherit; user-select: text; }
.ai-panel input:focus, .ai-panel select:focus, .ai-panel textarea:focus, .ai-modal input:focus, .ai-modal select:focus, .ai-modal textarea:focus { border-color: var(--accent); }
.ai-panel select, .ai-modal select { background: var(--bg3); cursor: pointer; }
.ai-head { display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-bottom: 1px solid var(--border); background: var(--bg3); min-width: 0; }
.ai-head .ai-ttl { font-weight: 700; font-size: 12px; white-space: nowrap; }
.ai-head .ai-sp { flex: 1; }
.ai-ib { background: transparent; border: 1px solid transparent; color: var(--text2); border-radius: 6px; padding: 2px 6px; font-size: 12px; line-height: 18px; }
.ai-ib:hover { background: var(--bg4); color: var(--text); }
.ai-ib.danger:hover { color: var(--red); border-color: rgba(var(--red-rgb),.35); }
.ai-body { flex: 1; overflow-y: auto; padding: 8px; display: flex; flex-direction: column; gap: 8px; min-height: 0; user-select: text; }
.ai-body > * { flex-shrink: 0; }
.ai-think { display: block; max-height: 9.5em; overflow-y: auto; white-space: pre-wrap; overflow-wrap: anywhere; }
.ai-foot { border-top: 1px solid var(--border); padding: 6px; display: flex; gap: 6px; align-items: flex-end; }
.ai-foot textarea { flex: 1; min-height: 36px; max-height: 140px; resize: vertical; font-size: 12.5px; }
.ai-msg { border-radius: 9px; padding: 7px 9px; font-size: 12.5px; line-height: 1.5; word-wrap: break-word; overflow-wrap: anywhere; }
.ai-msg.user { background: var(--accent-d); border: 1px solid var(--accent-b, rgba(var(--accent-rgb),.3)); align-self: flex-end; max-width: 92%; white-space: pre-wrap; }
.ai-msg.assistant { background: var(--bg3); border: 1px solid var(--border2); }
.ai-msg.assistant pre, .ai-card pre { background: var(--bg0); border: 1px solid var(--border); border-radius: 6px; padding: 6px 8px; overflow-x: auto; font: 11.5px/1.45 var(--font-mono); white-space: pre; margin: 5px 0; max-height: 320px; }
.ai-msg code { font: 11.5px var(--font-mono); background: var(--bg4); padding: 0 4px; border-radius: 4px; }
.ai-note { font-size: 11px; color: var(--text2); text-align: center; }
.ai-note.err { color: var(--red-t); }
.ai-card { border: 1px solid var(--border2); border-radius: 9px; background: var(--bg); padding: 7px 8px; font-size: 12px; display: flex; flex-direction: column; gap: 5px; min-width: 0; }
.ai-card.wait { border-color: rgba(var(--yellow-rgb),.55); box-shadow: 0 0 0 1px rgba(var(--yellow-rgb),.15); }
.ai-card.deny { border-color: rgba(var(--red-rgb),.4); }
.ai-card .ai-ch { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.ai-card .ai-tool { font-weight: 700; font-size: 11px; color: var(--text2); text-transform: uppercase; letter-spacing: .04em; }
.ai-card .ai-cmd { font: 12px/1.45 var(--font-mono); color: var(--text); word-break: break-all; white-space: pre-wrap; }
.ai-card .ai-why { font-size: 11px; color: var(--text2); }
.ai-card textarea { font: 12px/1.4 var(--font-mono); min-height: 54px; width: 100%; }
.ai-card .ai-acts { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.ai-card .ai-acts input { flex: 1; min-width: 120px; font-size: 11.5px; }
.ai-diff .a { color: var(--green-t); } .ai-diff .d { color: var(--red-t); } .ai-diff .h { color: var(--accent-t); }
.ai-card details summary { cursor: pointer; font-size: 11px; color: var(--text2); }
.ai-setup { display: flex; flex-direction: column; gap: 9px; font-size: 12px; }
.ai-setup label { font-size: 11px; color: var(--text2); display: block; margin-bottom: 3px; }
.ai-setup select, .ai-setup input[type=text], .ai-setup input[type=number], .ai-setup textarea { width: 100%; }
.ai-setup textarea { min-height: 52px; font: 11.5px var(--font-mono); }
.ai-row { display: flex; gap: 8px; } .ai-row > div { flex: 1; min-width: 0; }
.aip { max-width: 760px; } .aip h4 { margin: 14px 0 4px; } .aip .ai-row { margin: 8px 0; flex-wrap: wrap; } .aip .ai-row > div { min-width: 180px; }
.aip label:not(.ai-check) { font-size: 11px; color: var(--text2); display: block; margin-bottom: 3px; }
.aip select { width: 100%; background: var(--bg3); border: 1px solid var(--border2); color: var(--text); border-radius: 6px; padding: 6px 8px; font-size: 12px; }
.ai-check { display: flex; gap: 7px; align-items: flex-start; font-size: 12px; color: var(--text); }
.ai-check input { margin-top: 2px; flex: none; }
.ai-see { font-size: 11.5px; color: var(--text2); line-height: 1.5; }
.ai-see li { margin-left: 16px; }
.ai-bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 11px; color: var(--text2); position: sticky; top: -8px; z-index: 2; background: var(--bg2); padding: 6px 0 4px; margin-top: -6px; }
.ai-pend { position: sticky; top: 22px; z-index: 3; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 2px 0 6px; padding: 7px 9px; border: 1px solid var(--yellow); border-radius: 8px; background: var(--bg3); box-shadow: 0 4px 14px rgba(0,0,0,.25); font-size: 12px; }
.ai-pend b { color: var(--yellow); }
.ai-pend .ai-pend-w { flex: 1; min-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ai-bar select { font-size: 11.5px; padding: 2px 6px; max-width: 100%; }
#admin-body button.btn-sm.danger, .ai-modal button.btn-sm.danger, .ai-panel button.danger { background: var(--red); border-color: var(--red); color: #fff; }
#admin-body button.btn-sm.danger:hover, .ai-modal button.btn-sm.danger:hover { background: #b91c1c; }
.ai-modal .modal { width: 760px; max-width: calc(100vw - 24px); max-height: calc(100vh - 40px); overflow-y: auto; }
.ai-tr { display: flex; flex-direction: column; gap: 6px; }
.ai-tr .k { font-size: 10px; font-weight: 700; color: var(--text3); text-transform: uppercase; }
.ai-form .field { margin-bottom: 9px; }
.ai-wsteps { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 10px; font-size: 11px; } .ai-wsteps span { padding: 3px 9px; border-radius: 999px; border: 1px solid var(--border2); color: var(--text3); }
.ai-wsteps span.on { background: var(--accent); border-color: var(--accent); color: #fff; } .ai-wsteps span.done { color: var(--text2); }
.ai-wgrid { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 8px; margin: 8px 0; } .ai-wgrid button { height: auto; white-space: normal; text-align: left; padding: 9px 11px; }
.ai-form .field label { font-size: 11px; color: var(--text2); display: block; margin-bottom: 3px; }
.ai-form input[type=text], .ai-form input[type=password], .ai-form input[type=number], .ai-form select, .ai-form textarea { width: 100%; }
.ai-form textarea { min-height: 90px; font: 11px var(--font-mono); }
@media (max-width: 480px) { .ai-row { flex-direction: column; gap: 6px; } .ai-modal .modal { padding: 14px; } }
`;
  if (!$('ai-style')) { const s = document.createElement('style'); s.id = 'ai-style'; s.textContent = css; document.head.appendChild(s); }

  const tf = (k, vars) => { let s = t(k); for (const [a, b] of Object.entries(vars || {})) s = s.split('{' + a + '}').join(String(b)); return s; };
  const winOf = id => winObjects.find(w => w.id === id);
  const shareQ = wo => (wo && wo.shareToken ? '?share_token=' + encodeURIComponent(wo.shareToken) : '');
  async function api(method, url, body) {
    const res = await fetch(url, {method, headers: body ? {'Content-Type': 'application/json'} : {}, body: body ? JSON.stringify(body) : undefined});
    if (!res.ok) throw new Error(await apiError(res));
    return res.json().catch(() => ({}));
  }
  function md(s) {
    // escape first, then code blocks, inline code, bold and line breaks
    const parts = String(s || '').split('```');
    return parts.map((p, i) => {
      if (i % 2) return '<pre>' + esc(p.replace(/^[a-z0-9_+-]*\n/i, '')) + '</pre>';
      return esc(p).replace(/`([^`\n]+)`/g, '<code>$1</code>').replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>').replace(/\n/g, '<br>');
    }).join('');
  }
  function diffHTML(d) {
    return '<pre class="ai-diff">' + String(d || '').split('\n').map(l => {
      const c = l.startsWith('@@') ? 'h' : l.startsWith('+') && !l.startsWith('+++') ? 'a' : l.startsWith('-') && !l.startsWith('---') ? 'd' : '';
      return c ? `<span class="${c}">${esc(l)}</span>` : esc(l);
    }).join('\n') + '</pre>';
  }

  async function loadCfg() { cfg = await api('GET', '/api/ai/config'); return cfg; }

  // ── panel ────────────────────────────────────────────
  function panelEl(id) { return $('aip-' + id); }

  function ensurePanel(wo) {
    let p = panelEl(wo.id);
    if (p) return p;
    p = document.createElement('div');
    p.className = 'ai-panel';
    p.id = 'aip-' + wo.id;
    p.innerHTML = `<div class="ai-head"><span class="ai-ttl">✦ ${esc(t('ai_title'))}</span><span class="pill info" id="aipm-${wo.id}" style="display:none;"></span><span class="ai-sp"></span>
      <button class="ai-ib ai-dockb" data-d="left" title="${esc(t('ai_dock_left'))}" onclick="wrmAI.dock(${wo.id},'left')">◧</button>
      <button class="ai-ib ai-dockb" data-d="top" title="${esc(t('ai_dock_top'))}" onclick="wrmAI.dock(${wo.id},'top')">⬒</button>
      <button class="ai-ib ai-dockb" data-d="bottom" title="${esc(t('ai_dock_bottom'))}" onclick="wrmAI.dock(${wo.id},'bottom')">⬓</button>
      <button class="ai-ib ai-dockb" data-d="right" title="${esc(t('ai_dock_right'))}" onclick="wrmAI.dock(${wo.id},'right')">◨</button>
      <button class="ai-ib ai-dockb" data-d="float" title="${esc(t('ai_float'))}" onclick="wrmAI.dock(${wo.id},'float')">❐</button>
      <button class="ai-ib ai-dockb ai-xb" title="${esc(t('ai_expand'))}" onclick="wrmAI.expand(${wo.id})">⛶</button>
      <button class="ai-ib" id="aisnd-${wo.id}" title="${esc(t(wrmSoundOn() ? 'ai_sound_on' : 'ai_sound_off'))}" onclick="wrmAI.sound(${wo.id})">${wrmSoundOn() ? '🔊' : '🔇'}</button>
      <button class="ai-ib" title="${esc(t('ai_see'))}" onclick="wrmAI.see(${wo.id})">👁</button>
      <button class="ai-ib" title="${esc(t('ai_history'))}" onclick="wrmAI.history(${wo.id})">🕘</button>
      <button class="ai-ib danger" id="aipk-${wo.id}" title="${esc(t('ai_end'))}" style="display:none;" onclick="wrmAI.end(${wo.id})">⏻</button>
      <button class="ai-ib" title="${esc(t('ai_close_panel'))}" onclick="wrmAI.toggle(${wo.id})">✕</button></div>
      <div class="ai-body" id="aib-${wo.id}"></div><div class="ai-foot" id="aif-${wo.id}" style="display:none;"></div>`;
    p.insertAdjacentHTML('beforeend', '<div class="ai-rs"></div>');
    wo.el.appendChild(p);
    const head = p.querySelector('.ai-head'), rs = p.querySelector('.ai-rs');
    head.addEventListener('pointerdown', e => dragStart(wo, e, 'move'));
    head.addEventListener('dblclick', e => { if (!e.target.closest('button')) expand(wo.id); });
    rs.addEventListener('pointerdown', e => dragStart(wo, e, 'size'));
    if (window.ResizeObserver) new ResizeObserver(() => { if (wo.el.classList.contains('ai-open')) layout(wo, true); }).observe(wo.el);
    return p;
  }

  // ── position, size, dock (remembered per user) ───────
  const DOCKS = ['left', 'right', 'top', 'bottom', 'float'];
  const MAX = {}; // per window: expanded over the whole terminal window (not remembered)
  const wkey = () => 'wrm_ai_win_' + ((typeof currentUser !== 'undefined' && currentUser && currentUser.id) || 0);
  const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
  function getPref() {
    let p = {};
    try { p = JSON.parse(localStorage.getItem(wkey()) || '{}') || {}; } catch (_) {}
    const n = (v, d) => (isFinite(+v) && +v > 0 ? +v : d);
    return {dock: DOCKS.includes(p.dock) ? p.dock : 'right', w: n(p.w, 380), h: n(p.h, 280), fw: n(p.fw, 420), fh: n(p.fh, 360),
      fx: typeof p.fx === 'number' ? p.fx : null, fy: typeof p.fy === 'number' ? p.fy : null};
  }
  function setPref(p) { try { localStorage.setItem(wkey(), JSON.stringify(p)); } catch (_) {} }

  function layout(wo, noFit) {
    const p = panelEl(wo.id), wc = $('wc-' + wo.id);
    if (p && wc) {
      const hdr = $('wh-' + wo.id), ht = hdr ? hdr.offsetHeight : 34;
      const W = wo.el.clientWidth, H = wo.el.clientHeight - ht, s = p.style;
      const compact = W <= 760;
      p.classList.toggle('ai-compact', compact);
      ['left', 'right', 'top', 'bottom', 'width', 'height'].forEach(k => { s[k] = ''; });
      wc.style.margin = '';
      s.top = ht + 'px';
      const pr = getPref(), mx = !compact && !!MAX[wo.id];
      p.dataset.dock = compact ? 'right' : (mx ? 'max' : pr.dock);
      p.querySelectorAll('.ai-dockb[data-d]').forEach(b => b.classList.toggle('on', !mx && b.dataset.d === pr.dock));
      const xb = p.querySelector('.ai-xb');
      if (xb) { xb.classList.toggle('on', mx); xb.textContent = mx ? '🗗' : '⛶'; xb.title = t(mx ? 'ai_restore' : 'ai_expand'); }
      if (!compact && wo.el.classList.contains('ai-open')) {
        const w = clamp(pr.w, 260, Math.max(260, W - 160)), h = clamp(pr.h, 140, Math.max(140, H - 100));
        if (mx) { s.left = '0'; s.right = '0'; s.bottom = '0'; s.width = 'auto'; }
        else if (pr.dock === 'right') { s.right = '0'; s.bottom = '0'; s.width = w + 'px'; wc.style.marginRight = w + 'px'; }
        else if (pr.dock === 'left') { s.left = '0'; s.bottom = '0'; s.width = w + 'px'; s.right = 'auto'; wc.style.marginLeft = w + 'px'; }
        else if (pr.dock === 'top') { s.left = '0'; s.right = '0'; s.width = 'auto'; s.height = h + 'px'; wc.style.marginTop = h + 'px'; }
        else if (pr.dock === 'bottom') { s.left = '0'; s.right = '0'; s.top = 'auto'; s.width = 'auto'; s.bottom = '0'; s.height = h + 'px'; wc.style.marginBottom = h + 'px'; }
        else {
          const fw = clamp(pr.fw, 260, Math.max(260, W - 8)), fh = clamp(pr.fh, 160, Math.max(160, H - 8));
          const fx = clamp(pr.fx === null ? W - fw - 12 : pr.fx, 0, Math.max(0, W - fw)), fy = clamp(pr.fy === null ? 8 : pr.fy, 0, Math.max(0, H - fh));
          s.right = 'auto'; s.bottom = 'auto'; s.left = fx + 'px'; s.top = (ht + fy) + 'px'; s.width = fw + 'px'; s.height = fh + 'px';
        }
      }
    }
    if (!noFit) setTimeout(() => { try { fitTerm(wo); } catch (_) {} }, 60);
  }

  function dock(id, d) {
    const wo = winOf(id); if (!wo || !DOCKS.includes(d)) return;
    const pr = getPref(), p = panelEl(id);
    if (d === 'float' && p && pr.dock !== 'float') { pr.fw = clamp(p.offsetWidth, 260, 460); pr.fh = clamp(p.offsetHeight, 240, 520); pr.fx = null; pr.fy = null; }
    pr.dock = d; MAX[id] = false; setPref(pr); layout(wo);
  }
  function expand(id) {
    const wo = winOf(id); if (!wo || wo.el.clientWidth <= 760) return;
    MAX[id] = !MAX[id]; layout(wo);
  }

  function dragStart(wo, e, kind) {
    if (e.button !== 0 || wo.el.clientWidth <= 760 || MAX[wo.id]) return;
    if (kind === 'move' && e.target.closest('button')) return;
    const p = panelEl(wo.id), ht = ($('wh-' + wo.id) || {}).offsetHeight || 34, pr = getPref();
    const src = e.currentTarget, x0 = e.clientX, y0 = e.clientY;
    const start = {w: p.offsetWidth, h: p.offsetHeight, x: p.offsetLeft, y: p.offsetTop - ht};
    if (kind === 'move' && pr.dock !== 'float') { pr.dock = 'float'; pr.fw = clamp(start.w, 260, 460); pr.fh = clamp(start.h, 240, 520); }
    if (kind === 'move') { pr.fx = start.x; pr.fy = start.y; }
    e.preventDefault();
    try { src.setPointerCapture(e.pointerId); } catch (_) {}
    const mv = ev => {
      const dx = ev.clientX - x0, dy = ev.clientY - y0;
      if (kind === 'move') { pr.fx = start.x + dx; pr.fy = start.y + dy; }
      else if (pr.dock === 'right') pr.w = start.w - dx;
      else if (pr.dock === 'left') pr.w = start.w + dx;
      else if (pr.dock === 'top') pr.h = start.h + dy;
      else if (pr.dock === 'bottom') pr.h = start.h - dy;
      else { pr.fw = start.w + dx; pr.fh = start.h + dy; }
      setPref(pr); layout(wo, true);
    };
    const up = () => {
      src.removeEventListener('pointermove', mv); src.removeEventListener('pointerup', up); src.removeEventListener('pointercancel', up);
      layout(wo);
    };
    src.addEventListener('pointermove', mv); src.addEventListener('pointerup', up); src.addEventListener('pointercancel', up);
  }

  async function queueContext(id,text){const wo=winOf(id);if(!wo)return;if(!wo.el.classList.contains('ai-open'))await toggle(id);const input=$('ai-in-'+id);if(input){input.value=text;input.focus()}else wo.pendingAIContext=text;}
  // openPanel shows the assistant panel of a window (and its setup when nothing runs there).
  async function openPanel(id) { const wo = winOf(id); if (wo && !wo.el.classList.contains('ai-open')) await toggle(id); }
  function busyIn(id) { const st = S[id]; return !!(st && st.sid && !st.ended); }
  // focusSession brings the window that shows a running session to the front.
  function focusSession(sid) {
    for (const [id, st] of Object.entries(S)) {
      if (st && st.sid === sid) { const wo = winOf(Number(id)); if (!wo) continue; if (typeof focusWindow === 'function') focusWindow(wo.id); openPanel(wo.id); return true; }
    }
    return false;
  }
  async function toggle(id) {
    const wo = winOf(id);
    if (!wo) return;
    if (wo.el.classList.contains('ai-open')) {
      wo.el.classList.remove('ai-open');
      const p = panelEl(id); if (p) p.style.display = 'none';
      layout(wo); indicators();
      return;
    }
    const p = ensurePanel(wo);
    p.style.display = '';
    wo.el.classList.add('ai-open');
    layout(wo);
    const st = S[id];
    if (st && st.sid && !st.ended) { indicators(); return; }
    await renderSetup(wo);
  }

  function availabilityHTML(config) {
    if (!config) return `<div class="hint-box warn">${esc(t('ai_status_config_error'))}</div>`;
    const ready = config.enabled && (config.modes || []).length > 0;
    return `<div class="hint-box ${ready ? 'ok' : 'warn'}">${esc(t(ready ? 'ai_status_ready' : 'ai_status_blocked'))}${config.reason ? `<div>${esc(config.reason)}</div>` : ''}${config.enabled && !(config.modes || []).length ? `<div>${esc(t('ai_status_no_modes'))}</div>` : ''}</div>
      <div class="muted-text">${esc(t('ai_status_test'))}</div>${ready ? `<div class="setting-row"><b>${esc(t('ai_status_providers'))}</b><span>${(config.providers || []).length}</span></div><div>${esc((config.providers || []).length ? t('ai_status_open') : t('ai_no_providers'))}</div>` : `<div>${esc(t('ai_status_admin'))}</div>`}`;
  }
  async function statusPane() {
    const pane = $('ai-status-pane'); if (!pane) return;
    pane.innerHTML = `<div class="fm-loading">${esc(t('loading'))}</div>`;
    try {
      await loadCfg();
      if (typeof refreshUserPolicy === 'function') await refreshUserPolicy();
      pane.innerHTML = availabilityHTML(cfg) + `<div style="display:flex;gap:8px;flex-wrap:wrap;margin:12px 0"><button class="btn-sec" onclick="wrmAI.statusPane()">${esc(t('ai_status_refresh'))}</button>${currentUser && currentUser.is_admin ? `<button onclick="closeModal('modal-settings'); adminTab='ai'; openAdminPanel()">${esc(t('ai_status_setup'))}</button>` : ''}</div>` + prefsHTML();
      prefsProv();
      const wins = winObjects.filter(wo => aiEligible(wo,wo.mode));
      if (wins.length) pane.innerHTML += `<h4>${esc(t('ai_status_connections'))}</h4>` + wins.map(wo => `<button class="btn-sec" style="margin:4px" onclick="closeModal('modal-settings'); openAI(${wo.id})">${esc(connLabel(winConn(wo)))}</button>`).join('');
    } catch(e) { pane.innerHTML = `<div class="hint-box bad">${esc(e.message)}</div><button onclick="wrmAI.statusPane()">${esc(t('ai_status_refresh'))}</button>`; }
  }
  // ── personal AI settings (stored per user in /api/me/ui/ai) ──
  function prefsHTML() {
    if (!cfg || !cfg.enabled) return '';
    const pr = cfg.prefs || {}, provs = cfg.providers || [], oc = cfg.opencode;
    const own = provs.filter(p => p.scope === 'user');
    const mcpOn = !!(currentUser && currentUser.policy && currentUser.policy.mcp_enabled);
    return `<div class="aip"><h4>👤 ${esc(t('aip_title'))}</h4><div class="ai-see">${esc(t('aip_d'))}</div>
      <div class="ai-row">${oc ? `<div><label>${esc(t('aip_engine'))}</label><select id="aip-eng"><option value="">✦ ${esc(t('ai_engine_wrm'))}</option><option value="opencode" ${pr.engine === 'opencode' ? 'selected' : ''} ${oc.available ? '' : 'disabled'}>⬛ ${esc(t('ai_engine_oc'))}</option></select></div>` : ''}
        <div><label>${esc(t('aip_mode'))}</label><select id="aip-mode"><option value="">${esc(t('aip_mode_conn'))}</option>${(cfg.modes || []).filter(m => m !== 'auto').map(m => `<option value="${m}" ${pr.mode === m ? 'selected' : ''}>${esc(t('ai_mode_' + m))}</option>`).join('')}</select></div></div>
      <div class="ai-row"><div><label>${esc(t('aip_provider'))}</label><select id="aip-prov" onchange="wrmAI.prefsProv()"><option value="0">${esc(t('aip_provider_first'))}</option>${provs.map(p => `<option value="${p.id}" ${pr.provider_id === p.id ? 'selected' : ''}>${esc(p.name)}${p.scope === 'user' ? ' (🔑)' : ''}</option>`).join('')}</select></div>
        <div><label>${esc(t('aip_model'))}</label><select id="aip-model"></select></div></div>
      <label class="ai-check"><input type="checkbox" id="aip-think" ${pr.thinking === false ? '' : 'checked'} ${cfg.show_thinking ? '' : 'disabled'}> ${esc(t('aip_thinking'))}</label>${cfg.show_thinking ? '' : `<div class="ai-see">${esc(t('aip_thinking_off'))}</div>`}
      <div style="margin:8px 0 14px;"><button class="btn-sm" onclick="wrmAI.savePrefs()">💾 ${esc(t('aip_save'))}</button></div>
      <h4>🔑 ${esc(t('aip_accounts'))}</h4>
      ${cfg.personal_keys ? `<div class="ai-see">${esc(t('aip_accounts_d'))}</div><div class="ai-see"><b>${own.length ? own.map(p => esc(p.name) + ' (' + esc(t('ai_kind_' + p.kind)) + ')').join(', ') : esc(t('aip_accounts_none'))}</b></div>
        <div style="display:flex;gap:8px;flex-wrap:wrap;margin:6px 0 14px;"><button class="btn-sm" onclick="wrmAI.myWizard(0)">🧭 ${esc(t('aiw_wizard'))}</button><button class="btn-sec btn-sm" onclick="wrmAI.myKeys(0)">🔑 ${esc(t('aip_manage'))}</button></div>`
        : `<div class="hint-box">${esc(t('aip_accounts_off'))}</div>`}
      <h4>🤖 ${esc(t('aip_apps'))}</h4><div class="ai-see">${esc(t('aip_apps_d'))}</div>
      <div style="margin:6px 0 14px;">${mcpOn ? `<button class="btn-sec btn-sm" onclick="showSettingsTab('st-mcp')">🔌 ${esc(t('aip_apps_btn'))}</button>` : `<span class="ai-see">${esc(t('aip_apps_off'))}</span>`}</div>
      <h4>🧠 ${esc(t('aip_sessions'))}</h4><div class="ai-see">${esc(t('aip_sessions_d'))}</div>
      <div style="margin:6px 0 4px;"><button class="btn-sec btn-sm" onclick="closeModal('modal-settings'); if (window.wrmLayout) wrmLayout.activate('aisess', {reveal: true})">🧠 ${esc(t('aip_sessions_btn'))}</button></div></div>`;
  }
  function prefsProv() {
    const ps = $('aip-prov'), ms = $('aip-model');
    if (!ps || !ms) return;
    const p = (cfg.providers || []).find(x => String(x.id) === ps.value);
    const cur = (cfg.prefs || {}).model || '';
    ms.innerHTML = `<option value="">${esc(t('aip_model_prov'))}</option>` + (p ? p.models.map(m => `<option ${m === cur ? 'selected' : ''}>${esc(m)}</option>`).join('') : '');
    ms.disabled = !p;
  }
  async function savePrefs() {
    const v = {provider_id: Number(($('aip-prov') || {}).value) || 0, model: ($('aip-model') || {}).value || '', mode: ($('aip-mode') || {}).value || '',
      engine: ($('aip-eng') || {}).value || '', thinking: !!($('aip-think') || {}).checked};
    if (!v.provider_id) v.model = '';
    try { await api('PUT', '/api/me/ui/ai', {value: v}); } catch (e) { showToast(e.message, 'error'); return; }
    if (cfg) cfg.prefs = v;
    showToast(t('aip_saved'), 'success');
  }
  async function policyChanged() {
    await loadCfg().catch(() => {});
    if (typeof refreshUserPolicy === 'function') await refreshUserPolicy();
    if ($('ai-status-pane') && $('st-ai') && $('st-ai').classList.contains('active')) statusPane();
  }

  // setup: provider, model, mode, notes, automatic limits, what the AI sees
  async function renderSetup(wo) {
    const id = wo.id, body = $('aib-' + id);
    $('aif-' + id).style.display = 'none';
    $('aipk-' + id).style.display = 'none';
    $('aipm-' + id).style.display = 'none';
    body.innerHTML = `<div class="fm-loading">${esc(t('loading'))}</div>`;
    let pol;
    try { await loadCfg(); pol = await api('GET', `/api/ai/connections/${wo.connId}${shareQ(wo)}`); }
    catch (e) { body.innerHTML = `<div class="fm-error">${esc(e.message)}</div>`; return; }
    S[id] = Object.assign(S[id] || {}, {pol});
    if (!cfg.enabled) { body.innerHTML = `<div class="hint-box warn">${esc(t('ai_disabled'))}: ${esc(cfg.reason || '')}</div>`; return; }
    if (!pol.allowed_modes.length) { body.innerHTML = `<div class="hint-box warn">${esc(t('ai_disabled'))}</div>`; return; }
    const resume = (cfg.active || []).filter(s => s.conn_id === wo.connId && !Object.values(S).some(x => x.sid === s.id) && !s.purpose);
    let recent = [];
    try { recent = (await api('GET', '/api/ai/sessions?purpose=terminal')).filter(s => s.conn_id === wo.connId && !s.live && s.transport === 'panel' && s.requests > 0).slice(0, 4); } catch (_) {}
    const provs = cfg.providers || [];
    const own = cfg.personal_keys ? `<button class="btn-sec btn-sm" onclick="wrmAI.myKeys(${id})">🔑 ${esc(t('ai_my_keys'))}</button>` : '';
    if (!provs.length) {
      body.innerHTML = `<div class="hint-box">${esc(t('ai_no_providers'))}</div>${cfg.personal_keys ? `<button class="btn-sm" onclick="wrmAI.myWizard(${id})">🧭 ${esc(t('aiw_wizard'))}</button> <button class="btn-sec btn-sm" onclick="wrmAI.myKeys(${id})">🔑 ${esc(t('ai_add_own_key'))}</button>` : ''}
        <div class="hint-box">${esc(t('ai_subscriptions_note'))}</div>`;
      return;
    }
    const shared = !!wo.shareToken;
    body.innerHTML = `<div class="ai-setup">
      ${resume.map(s => `<button class="btn-sec btn-sm" onclick="wrmAI.attach(${id}, ${s.id})">↪ ${esc(t('ai_continue'))} #${s.id} · ${esc(s.title || t('ai_mode_' + s.mode))} · ${esc(s.model)}</button>`).join('')}
      ${recent.length ? `<div class="ai-see">${esc(t('ai_recent'))}</div>` + recent.map(s => `<button class="btn-sec btn-sm" style="text-align:left" onclick="wrmAI.resume(${id}, ${s.id})">↪ ${esc(s.title || '#' + s.id)} · ${esc(fmtTime(s.started_at))}</button>`).join('') : ''}
      ${engineHTML(id)}
      <div class="ai-row"><div><label>${esc(t('ai_provider'))}</label><select id="ai-prov-${id}" onchange="wrmAI.models(${id})">${provs.map(p => `<option value="${p.id}">${esc(p.name)}${p.scope === 'user' ? ' (🔑)' : ''}</option>`).join('')}</select></div>
        <div><label>${esc(t('ai_model'))}</label><select id="ai-model-${id}"></select></div></div>
      <div><label>${esc(t('ai_mode'))}</label><select id="ai-mode-${id}" onchange="wrmAI.modeHelp(${id})">${pol.allowed_modes.map(m => `<option value="${m}" ${m === pol.default_mode ? 'selected' : ''}>${esc(t('ai_mode_' + m))}</option>`).join('')}</select>
        <div class="ai-see" id="ai-mode-d-${id}" style="margin-top:4px;"></div></div>
      <div id="ai-auto-${id}" style="display:none;">${autoForm('ai-a-' + id)}</div>
      ${pol.has_notes && pol.share_notes_allowed && !shared ? `<label class="ai-check"><input type="checkbox" id="ai-notes-${id}"> ${esc(t('ai_share_notes'))}</label>` : ''}
      <details><summary class="ai-see">👁 ${esc(t('ai_see'))}</summary><div id="ai-see-${id}">${seeHTML(pol.sees, provs[0] && provs[0].name, false)}</div></details>
      ${delegSetupHTML(id)}
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;"><button onclick="wrmAI.start(${id})">✦ ${esc(t('ai_start'))}</button>${own}</div>
      <div class="ai-see">${esc(t('ai_subscriptions_note'))}</div></div>`;
    models(id); modeHelp(id); engine(id);
    applyPrefs(id, pol);
  }
  // applyPrefs preselects the user's personal defaults (Settings → AI assistant).
  function applyPrefs(id, pol) {
    const pr = (cfg && cfg.prefs) || {};
    const eng = $('ai-eng-' + id);
    if (pr.engine === 'opencode' && eng && cfg.opencode && cfg.opencode.available) { eng.value = 'opencode'; engine(id); }
    const ps = $('ai-prov-' + id);
    const o = ps && pr.provider_id ? [...ps.options].find(x => x.value === String(pr.provider_id)) : null;
    if (o && !o.disabled) { ps.value = o.value; models(id); }
    const m = $('ai-model-' + id);
    if (o && m && pr.model && [...m.options].some(x => x.value === pr.model)) m.value = pr.model;
    const md = $('ai-mode-' + id);
    if (md && pr.mode && pr.mode !== 'auto' && (pol.allowed_modes || []).includes(pr.mode)) { md.value = pr.mode; modeHelp(id); }
  }

  // engine: the WRM assistant or an OpenCode agent (cfg.opencode says whether it can run)
  function engineHTML(id) {
    const oc = cfg && cfg.opencode;
    if (!oc) return '';
    return `<div class="ai-row"><div><label>${esc(t('ai_engine'))}</label><select id="ai-eng-${id}" onchange="wrmAI.engine(${id})">
        <option value="wrm">✦ ${esc(t('ai_engine_wrm'))}</option>
        <option value="opencode" ${oc.available ? '' : 'disabled'} title="${esc(oc.reason || '')}">⬛ ${esc(t('ai_engine_oc'))}${oc.available ? '' : ' (' + esc(t('ai_oc_unavail')) + ')'}</option></select></div>
      <div id="ai-agent-w-${id}" style="display:none;"><label>${esc(t('ai_oc_agent'))}</label><select id="ai-agent-${id}">${(oc.agents || ['build']).map(a => `<option value="${esc(a)}">${esc(t('ai_oc_agent_' + a))}</option>`).join('')}</select></div></div>
      ${oc.sandbox && oc.sandbox.offered ? `<div id="ai-sb-w-${id}" style="display:none;"><label class="ai-check" title="${esc(oc.sandbox.reason || '')}"><input type="checkbox" id="ai-sb-${id}" ${oc.sandbox.available ? '' : 'disabled'}> 🐳 ${esc(t('ai_oc_sandbox'))}${oc.sandbox.available ? '' : ' (' + esc(t('ai_oc_sandbox_off')) + ': ' + esc(oc.sandbox.reason || '') + ')'}</label>
        <div class="ai-see">${esc(tf('ai_oc_sandbox_d', {image: oc.sandbox.image || ''}))}</div></div>` : ''}
      <div class="ai-see" id="ai-eng-d-${id}"></div>`;
  }
  function engine(id) {
    const sel = $('ai-eng-' + id), oc = cfg && cfg.opencode;
    if (!sel || !oc) return;
    const on = sel.value === 'opencode', kinds = oc.kinds || [];
    $('ai-agent-w-' + id).style.display = on ? '' : 'none';
    if ($('ai-sb-w-' + id)) $('ai-sb-w-' + id).style.display = on ? '' : 'none';
    const d = $('ai-eng-d-' + id);
    d.textContent = on ? t('ai_engine_oc_d') : t('ai_engine_wrm_d') + (oc.available ? '' : ' · ' + t('ai_oc_unavail') + ': ' + (oc.reason || ''));
    const ps = $('ai-prov-' + id);
    if (!ps) return;
    let first = null;
    [...ps.options].forEach(o => {
      const p = (cfg.providers || []).find(x => String(x.id) === o.value);
      const bad = on && p && !kinds.includes(p.kind);
      o.disabled = bad;
      o.textContent = p.name + (p.scope === 'user' ? ' (🔑)' : '') + (bad ? ' · ' + t('ai_oc_kind_off') : '');
      if (!bad && first === null) first = o.value;
    });
    if (ps.selectedOptions[0] && ps.selectedOptions[0].disabled && first !== null) { ps.value = first; models(id); }
  }

  function autoForm(p) {
    const mx = cfg ? cfg.auto_max_minutes : 60, ma = cfg ? cfg.auto_max_actions : 50;
    const defAllow = "*";
    const defDeny = "rm -rf*\nrmdir*\nmkfs*\ndd *\n> /dev/*\n* /etc/ssh/*\n*passwd*\nreboot\nshutdown";
    return `<div class="hint-box"><b>${esc(t('ai_mode_auto_setup'))}</b>
      <div style="margin-top:6px;"><label>${esc(t('ai_auto_allow'))}</label><textarea id="${p}-allow" placeholder="*">${defAllow}</textarea></div>
      <div><label>${esc(t('ai_auto_deny'))}</label><textarea id="${p}-deny">${defDeny}</textarea></div>
      <div><label>${esc(t('ai_auto_path_allow'))}</label><textarea id="${p}-pallow" placeholder="/etc/nginx/**"></textarea></div>
      <div><label>${esc(t('ai_auto_path_deny'))}</label><textarea id="${p}-pdeny"></textarea></div>
      <div class="ai-row"><div><label>${esc(t('ai_auto_minutes'))}</label><input type="number" id="${p}-min" min="1" max="${mx}" value="${Math.min(30, mx)}"></div>
        <div><label>${esc(t('ai_auto_actions'))}</label><input type="number" id="${p}-act" min="1" max="${ma}" value="${Math.min(20, ma)}"></div></div>
      <label class="ai-check" style="margin-top:6px;"><input type="checkbox" id="${p}-ok" checked> ${esc(t('ai_auto_confirm'))}</label></div>`;
  }
  function readAuto(p) {
    const lines = x => ($(p + '-' + x).value || '').split('\n').map(s => s.trim()).filter(Boolean);
    return {auto: {allow: lines('allow'), deny: lines('deny'), path_allow: lines('pallow'), path_deny: lines('pdeny'),
      minutes: Number($(p + '-min').value) || 0, max_actions: Number($(p + '-act').value) || 0}, ok: $(p + '-ok').checked};
  }

  function seeHTML(sees, provider, notes) {
    if (!sees) return '';
    return `<ul class="ai-see">
      <li>${esc(t('ai_see_tools'))}: ${sees.read_tools.map(esc).join(', ')}</li>
      <li>${esc(t('ai_see_cmd'))}</li>
      <li>${esc(t('ai_see_write'))}: ${sees.write_tools.map(esc).join(', ')}</li>
      <li>${esc(sees.redacted ? t('ai_see_redacted') : t('ai_see_not_redacted'))}</li>
      <li>${esc(tf('ai_see_limit', {kb: sees.read_max_kb}))}</li>
      <li>${esc(tf('ai_see_denied', {n: sees.denied_paths}))}</li>
      <li>${esc(t('ai_see_never'))}</li>
      ${provider ? `<li>${esc(tf('ai_see_provider', {provider}))}</li>` : ''}
      ${notes ? `<li>${esc(t('ai_see_notes_on'))}</li>` : ''}</ul>`;
  }

  function models(id) {
    const sel = $('ai-prov-' + id), m = $('ai-model-' + id);
    const p = (cfg.providers || []).find(x => String(x.id) === sel.value);
    if (!p) return;
    m.innerHTML = p.models.map(x => `<option ${x === p.default_model ? 'selected' : ''}>${esc(x)}</option>`).join('');
    const see = $('ai-see-' + id);
    if (see && S[id] && S[id].pol) see.innerHTML = seeHTML(S[id].pol.sees, p.name, false);
  }
  function modeHelp(id) {
    const m = $('ai-mode-' + id).value;
    $('ai-mode-d-' + id).textContent = t('ai_mode_' + m + '_d');
    $('ai-auto-' + id).style.display = m === 'auto' ? '' : 'none';
  }

  async function start(id) {
    if (window.wrmAskNotifications) wrmAskNotifications();
    const wo = winOf(id);
    const mode = $('ai-mode-' + id).value;
    const body = {conn_id: wo.connId, provider_id: Number($('ai-prov-' + id).value), model: $('ai-model-' + id).value, mode,
      share_notes: !!($('ai-notes-' + id) && $('ai-notes-' + id).checked)};
    const eng = $('ai-eng-' + id);
    if (eng && eng.value === 'opencode') { body.engine = 'opencode'; body.agent = $('ai-agent-' + id).value; body.sandbox = !!($('ai-sb-' + id) && $('ai-sb-' + id).checked); showToast(t('ai_oc_starting'), 'info', 3000); }
    if (mode === 'auto') {
      const a = readAuto('ai-a-' + id);
      if (!a.ok) { showToast(t('ai_auto_need_confirm'), 'warning', 4000); return; }
      body.auto = a.auto; body.confirm_auto = true;
    }
    try {
      const v = await api('POST', '/api/ai/sessions' + shareQ(wo), body);
      attach(id, v.id);
    } catch (e) { showToast(e.message, 'error', 6000); }
  }

  // chat view bound to a session; the event stream replays everything from the start
  function attach(id, sid) {
    const wo = winOf(id);
    if (!wo) return;
    const old = S[id] || {};
    if (old.es) old.es.close();
    S[id] = {pol: old.pol, sid, seq: 0, ended: false, busy: false, cards: {}, pending: {}, cur: null};
    const body = $('aib-' + id);
    body.innerHTML = `<div class="ai-bar" id="ai-bar-${id}"></div><div class="ai-pend" id="ai-pend-${id}" style="display:none;"></div>`;
    $('aipk-' + id).style.display = '';
    const foot = $('aif-' + id);
    foot.style.display = '';
    foot.innerHTML = `<textarea id="ai-in-${id}" rows="2" placeholder="${esc(t('ai_placeholder'))}" onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();wrmAI.send(${id});}"></textarea>
      <button id="ai-send-${id}" onclick="wrmAI.send(${id})">${esc(t('ai_send'))}</button>
      <button id="ai-stop-${id}" class="btn-sec" style="display:none;" onclick="wrmAI.stop(${id})">■ ${esc(t('ai_stop'))}</button>`;
    if(wo.pendingAIContext){$('ai-in-'+id).value=wo.pendingAIContext;wo.pendingAIContext='';}
    const es = new EventSource(`/api/ai/sessions/${sid}/events?since=0`);
    S[id].es = es;
    ['session', 'user', 'assistant_start', 'thinking', 'text', 'assistant_done', 'tool_call', 'approval_required', 'approval_resolved', 'tool_result', 'mode', 'usage', 'busy', 'error', 'notice', 'ended', 'title', 'history']
      .forEach(ty => es.addEventListener(ty, ev => { try { onEvent(id, JSON.parse(ev.data)); } catch (e) { console.warn(e); } }));
    es.onerror = () => { if (S[id] && S[id].ended) es.close(); };
    indicators();
    if (!S[id].timer) S[id].timer = setInterval(() => tick(id), 1000);
  }

  function scrollEnd(id) { const b = $('aib-' + id); if (b && b.scrollHeight - b.scrollTop - b.clientHeight < 160) b.scrollTop = b.scrollHeight; }
  function add(id, html) { const b = $('aib-' + id); if (!b) return null; b.insertAdjacentHTML('beforeend', html); const el = b.lastElementChild; scrollEnd(id); return el; }

  function bar(id) {
    const st = S[id], v = st.view, el = $('ai-bar-' + id);
    if (!el || !v) return;
    const allowed = (st.pol && st.pol.allowed_modes) || [v.mode];
    const auto = v.auto ? ` · ${esc(tf('ai_auto_until', {time: new Date(v.auto.until).toLocaleTimeString(), used: v.auto.used, max: v.auto.max_actions}))}` : '';
    const oc = v.transport === 'opencode';
    el.innerHTML = `<select onchange="wrmAI.setMode(${id}, this.value)" ${st.ended || oc ? 'disabled' : ''} ${oc ? `title="${esc(t('ai_oc_mode_fixed'))}"` : ''}>${MODES.filter(m => allowed.includes(m) || m === v.mode).map(m => `<option value="${m}" ${m === v.mode ? 'selected' : ''}>${esc(t('ai_mode_' + m))}</option>`).join('')}</select>
      ${v.title || st.title ? `<b class="ai-title" title="${esc(t('ai_rename'))}" style="cursor:pointer;max-width:16em;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" onclick="wrmAI.rename(${id})">${esc(v.title || st.title)}</b>` : ''}
      <span>${oc ? `⬛ OpenCode · ${esc(v.agent || 'build')}${v.sandbox ? ' · 🐳 ' + esc(t('ai_oc_sb_on')) : ''} · ` : ''}${esc(v.provider)} · ${esc(v.model)}${auto}</span><span style="margin-left:auto;" id="ai-use-${id}"></span>`;
    usage(id);
    const pm = $('aipm-' + id);
    pm.style.display = ''; pm.textContent = t('ai_mode_' + v.mode);
  }
  function usage(id) {
    const v = S[id].view, el = $('ai-use-' + id);
    if (el && v) el.textContent = tf('ai_usage', {in: v.tokens_in || 0, out: v.tokens_out || 0}) + (v.cost_usd ? ' · $' + Number(v.cost_usd).toFixed(4) : '');
  }

  // the server's reasons for ending a session, translated where known
  function reasonText(r) {
    r = String(r || '');
    const map = {'ended by the user': 'ai_r_user', 'all sessions ended by the user': 'ai_r_user', 'killed by an administrator': 'ai_r_admin',
      'killed by an administrator (all sessions of the user)': 'ai_r_admin', 'kill switch: every AI session ended by an administrator': 'ai_r_killall',
      'kill switch turned on by an administrator': 'ai_r_switch', 'the AI assistant was turned off': 'ai_r_off',
      'the AI assistant was turned off for the user (kill switch)': 'ai_r_blocked', 'the policy no longer allows the AI assistant on this connection': 'ai_r_policy'};
    if (map[r]) return t(map[r]);
    if (r.startsWith('idle')) return t('ai_r_idle');
    return r;
  }

  function decisionPill(d) {
    const cls = {allow: 'ok', approve: 'warn', deny: 'bad'}[d.decision] || '';
    const lbl = d.decision === 'allow' ? (d.rule === 'auto' ? t('ai_d_auto') : d.read_only ? t('ai_d_readonly') : t('ai_d_allow')) : t('ai_d_' + d.decision);
    return `<span class="pill ${cls}">${esc(lbl)}</span>`;
  }

  function workingUI(id, busy) {
    if (!busy) { workLogDone(id); return; }
    workLog(id);
  }

  // The work log: one collapsible line per request that shows what the
  // assistant is doing right now (tool by tool), open while it works and
  // collapsed once the answer is there. It never duplicates the tool cards
  // below it; it is the overview you can ignore.
  function workLog(id) {
    const b = $('aib-' + id);
    if (!b || typeof b.querySelector !== 'function') return null;
    let el = b.querySelector('[data-worklog]:not([data-done])');
    if (!el) {
      b.insertAdjacentHTML('beforeend', `<details class="ai-worklog" data-worklog open style="margin:6px 0;border:1px solid var(--border);border-radius:8px;padding:6px 10px;background:var(--bg2);">
        <summary style="cursor:pointer;font-size:11.5px;color:var(--text2);">⏳ <span data-sum>${esc(t('ai_worklog_working'))}</span></summary>
        <div data-steps style="font-size:11px;color:var(--text2);margin-top:4px;display:flex;flex-direction:column;gap:2px;"></div></details>`);
      el = b.lastElementChild;
    }
    scrollEnd(id);
    return el;
  }

  // workLogStep adds one line; it returns the text element so a live step
  // (the model's thinking) can grow in place.
  function workLogStep(id, text) {
    const el = workLog(id);
    if (!el) return null;
    const box = el.querySelector('[data-steps]');
    const n = box.children.length + 1;
    box.insertAdjacentHTML('beforeend', `<div style="display:flex;gap:6px;"><span style="color:var(--text3);">${n}.</span><span style="white-space:pre-wrap;overflow-wrap:anywhere;">${esc(text)}</span></div>`);
    el.querySelector('[data-sum]').textContent = `${t('ai_worklog_working')} · ${tf('ai_worklog_steps', {n})}`;
    scrollEnd(id);
    return box.lastElementChild.lastElementChild;
  }

  function workLogDone(id) {
    const b = $('aib-' + id);
    if (!b || typeof b.querySelector !== 'function') return;
    const el = b.querySelector('[data-worklog]:not([data-done])');
    if (!el) return;
    const n = el.querySelectorAll('[data-steps] div').length;
    if (!n) { el.remove(); return; } // nothing happened (the question was only sent): no empty line
    el.querySelector('[data-sum]').textContent = t('ai_worklog') + (n ? ' · ' + tf('ai_worklog_steps', {n}) : '');
    el.open = false;
    el.dataset.done = '1';
  }

  function onEvent(id, ev) {
    const st = S[id];
    if (!st || ev.seq <= st.seq) return;
    st.seq = ev.seq;
    const d = ev.data || {};
    switch (ev.type) {
      case 'session': st.view = d; st.ended = d.ended; bar(id); (d.pending || []).forEach(a => approvalUI(id, a)); break;
      case 'user': workLogDone(id); add(id, `<div class="ai-msg user">${esc(d.text)}</div>`); break;
      case 'assistant_start': st.cur = null; st.think = null; workLog(id); break;
      case 'thinking': {
        // the model's summarized reasoning, live in the work log
        // the whole text stays: a long thought scrolls inside its own line instead of losing its start
        if (!st.think) { st.think = workLogStep(id, '💭 ', true); st.thinkText = ''; if (st.think) st.think.classList.add('ai-think'); }
        st.thinkText += d.delta || '';
        if (st.think) { const near = st.think.scrollHeight - st.think.scrollTop - st.think.clientHeight < 40; st.think.textContent = '💭 ' + st.thinkText; if (near) st.think.scrollTop = st.think.scrollHeight; }
        scrollEnd(id); break;
      }
      case 'title': st.title = d.title; if (st.view) st.view.title = d.title; bar(id); break;
      case 'history': {
        const msgs = d.messages || [];
        add(id, `<details class="ai-worklog" style="margin:6px 0;border:1px dashed var(--border);border-radius:8px;padding:6px 10px;"><summary style="cursor:pointer;font-size:11.5px;color:var(--text2);">↪ ${esc(tf('ai_resumed_from', {n: d.from}))} · ${msgs.some(m => m.kind === 'user') ? msgs.filter(m => m.kind === 'user').length + ' ' + esc(t('ai_resumed_msgs')) : esc(tf('ai_resumed_steps', {n: msgs.length}))}</summary>
          ${msgs.map(m => m.kind === 'user' ? `<div class="ai-msg user" style="opacity:.75">${esc(m.text)}</div>` : m.kind === 'assistant' ? `<div class="ai-msg assistant" style="opacity:.75">${md(m.text)}</div>`
            : `<div class="ai-note" style="opacity:.7">${m.kind === 'tool_call' ? '🛠' : m.kind === 'error' ? '⚠' : '📄'} ${esc(String(m.text || '').slice(0, 300))}</div>`).join('')}</details>`);
        break;
      }
      case 'text':
        if (!st.cur) { st.cur = add(id, `<div class="ai-msg assistant"></div>`); st.curText = ''; }
        st.curText += d.delta; st.cur.textContent = st.curText; scrollEnd(id); break;
      case 'assistant_done':
        if (d.text) { if (!st.cur) st.cur = add(id, `<div class="ai-msg assistant"></div>`); st.cur.innerHTML = md(d.text); }
        else if (st.cur && d.text === '') st.cur.remove(); // parsed tool JSON has no final assistant text
        if (d.text && d.stop === 'tool_use') workLogStep(id, '💬 ' + String(d.text).replace(/\s+/g, ' ').slice(0, 300));
        st.cur = null; st.think = null;
        if (d.stop !== 'tool_use') { workLogDone(id); if (window.wrmSound) wrmSound('reply'); }
        break;      case 'tool_call': {
        workLogStep(id, d.tool ? d.tool + (d.display ? ' · ' + String(d.display) : '') : String(d.display || ''));
        if (st.cards[d.call_id]) break; // an approval already made the card
        const el = add(id, `<div class="ai-card ${d.decision === 'deny' ? 'deny' : ''}" id="aic-${id}-${esc(d.call_id)}">
          <div class="ai-ch"><span class="ai-tool">${esc(d.tool)}</span>${decisionPill(d)}</div>
          <div class="ai-cmd">${esc(d.display)}</div>
          ${d.reason ? `<div class="ai-why">${esc(t('ai_reason'))}: ${esc(d.reason)}</div>` : ''}
          ${d.why && d.decision !== 'allow' ? `<div class="ai-why">⚑ ${esc(d.why)}</div>` : ''}
          ${d.diff && d.decision !== 'approve' ? diffHTML(d.diff) : ''}
          <div class="ai-appr"></div><div class="ai-res"></div></div>`);
        st.cards[d.call_id] = el; break;
      }
      case 'approval_required': approvalUI(id, d.session_id ? Object.assign({}, d.approval, {session_id: d.session_id}) : d.approval); break;
      case 'approval_resolved': {
        if (d.approval_id && window.wrmAiAlertDone) wrmAiAlertDone('approval-' + d.approval_id);
        if (st.pending && d.approval_id) delete st.pending[d.approval_id];
        if (st.pending && d.call_id) {
          for (const [aid, a] of Object.entries(st.pending)) if (a.call_id === d.call_id) delete st.pending[aid];
        }
        pendingBar(id);
        const card = st.cards[d.call_id];
        if (card) {
          card.classList.remove('wait');
          const a = card.querySelector('.ai-appr');
          const cls = d.outcome === 'approved' ? 'ok' : 'bad';
          a.innerHTML = `<div class="ai-ch"><span class="pill ${cls}">${esc(t('ai_o_' + d.outcome))}${d.edited ? ' · ' + esc(t('ai_edited')) : ''}</span>${d.by ? `<span class="ai-why">${esc(d.by)}</span>` : ''}</div>`;
        }
        indicators(); break;
      }
      case 'tool_result': {
        const card = st.cards[d.call_id];
        const head = d.error ? `<div class="ai-why" style="color:var(--red-t);">${esc(d.error)}</div>`
          : `<div class="ai-ch"><span class="pill ${d.exit_code === 0 ? '' : 'warn'}">${esc(tf('ai_exit', {code: d.exit_code}))}</span>${d.redactions ? `<span class="pill info">🔒 ${esc(tf('ai_redactions', {n: d.redactions}))}</span>` : ''}</div>`;
        const out = d.output != null ? `<details ${String(d.output).length < 600 ? 'open' : ''}><summary>${esc(t('ai_output'))} (${String(d.output).length} B)</summary><pre>${esc(d.output)}</pre></details>` : '';
        workLogStep(id, (d.error ? '✗ ' : d.exit_code === 0 ? '✓ ' : '⚠ ') + (d.tool || '') + (d.error ? ' · ' + String(d.error).slice(0, 160) : ' · ' + tf('ai_exit', {code: d.exit_code}) + (d.output != null ? ' · ' + String(d.output).length + ' B' : '')));
        if (card) card.querySelector('.ai-res').innerHTML = head + out;
        else add(id, `<div class="ai-card ${d.error ? 'deny' : ''}"><div class="ai-ch"><span class="ai-tool">${esc(d.tool || '')}</span>${d.error ? `<span class="pill bad">${esc(t('ai_d_deny'))}</span>` : ''}</div>${head}${out}</div>`);
        scrollEnd(id); break;
      }
      case 'mode':
        if (st.view) { st.view.mode = d.mode; st.view.auto = d.auto; }
        add(id, `<div class="ai-note">${esc(tf('ai_mode_changed', {mode: t('ai_mode_' + d.mode)}))} · ${esc(tf('ai_by', {who: d.by}))}${d.why ? ' · ' + esc(d.why) : ''}</div>`);
        bar(id); indicators(); break;
      case 'usage': if (st.view) Object.assign(st.view, d); usage(id); break;
      case 'busy': {
        st.busy = d.busy;
        const s1 = $('ai-send-' + id), s2 = $('ai-stop-' + id);
        if (s1) { s1.style.display = d.busy ? 'none' : ''; s1.disabled = !!d.busy; } if (s2) s2.style.display = d.busy ? '' : 'none';
        workingUI(id, !!d.busy);
        break;
      }
      case 'error': add(id, `<div class="ai-note err">⚠ ${esc(d.error)}</div>`); break;
      case 'notice': add(id, `<div class="ai-note">${esc(t('ai_n_' + d.text))}</div>`); break;
      case 'ended':
        st.ended = true;
        st.pending = {};
        pendingBar(id);
        workingUI(id, false);
        if (st.es) st.es.close();
        add(id, `<div class="ai-note err">⏻ ${esc(t('ai_ended'))}: ${esc(reasonText(d.reason))}</div><div style="display:flex;gap:6px;justify-content:center;flex-wrap:wrap;">${st.view && st.view.transport === 'opencode' ? '' : `<button class="btn-sm" onclick="wrmAI.resume(${id})">↪ ${esc(t('ai_continue_chat'))}</button>`}<button class="btn-sec btn-sm" onclick="wrmAI.newSession(${id})">✦ ${esc(t('ai_new'))}</button></div>`);
        const f = $('aif-' + id); if (f) f.style.display = 'none';
        $('aipk-' + id).style.display = 'none';
        if (st.view) st.view.ended = true;
        bar(id); indicators(); break;
    }
  }

  function approvalUI(id, a) {
    const st = S[id];
    if (!st) return;
    st.pending = st.pending || {};
    st.pending[a.id] = a;
    if (a.session_id) (st.apprSess = st.apprSess || {})[a.id] = a.session_id;
    let card = st.cards[a.call_id];
    if (!card) {
      // The approval can arrive before the tool card (a replayed session or a fast
      // model): make its own card, so there is always something to approve.
      card = add(id, `<div class="ai-card wait" id="aic-${id}-${esc(a.call_id || a.id)}">
        <div class="ai-ch"><span class="ai-tool">${esc(a.tool || '')}</span><span class="pill warn">${esc(t('ai_d_approve'))}</span></div>
        <div class="ai-cmd">${esc(a.command || a.path || a.transfer || '')}</div>
        <div class="ai-appr"></div><div class="ai-res"></div></div>`);
      if (card && a.call_id) st.cards[a.call_id] = card;
    }
    card.classList.add('wait');
    const box = card.querySelector('.ai-appr');
    const isWrite = !!a.path;
    box.innerHTML = `<div class="ai-why"><b>⏳ ${esc(t('ai_wait_approval'))}</b> · <span data-exp="${new Date(a.expires).getTime()}"></span></div>
      ${a.no_edit ? (isWrite ? diffHTML(a.diff) : `<pre class="ai-cmd">${esc(a.command)}</pre>`) + `<div class="ai-why">🐳 ${esc(t('ai_oc_noedit'))}</div>`
        : isWrite ? diffHTML(a.diff) + `<details><summary>✏️ ${esc(t('ai_edit'))}</summary><textarea id="ai-ed-${id}-${esc(a.id)}">${esc(a.content)}</textarea></details>`
        : `<textarea id="ai-ed-${id}-${esc(a.id)}" spellcheck="false">${esc(a.command)}</textarea>`}
      <div class="ai-acts"><input type="text" id="ai-nt-${id}-${esc(a.id)}" placeholder="${esc(t('ai_note_ph'))}">
        <button class="btn-sm" onclick="wrmAI.decide(${id}, '${esc(a.id)}', true, ${isWrite})">✓ ${esc(t('ai_approve'))}</button>
        <button class="btn-sec btn-sm" onclick="wrmAI.decide(${id}, '${esc(a.id)}', false, ${isWrite})">✕ ${esc(t('ai_deny'))}</button></div>`;
    tick(id); scrollEnd(id); pendingBar(id); indicators();
    if (window.wrmAiAlert) wrmAiAlert('approval', (a.tool || '') + ' · ' + (a.command || a.path || a.transfer || ''), 'approval-' + a.id);
  }

  // pendingBar keeps the newest approval at the top of the chat, even when the
  // tool card scrolled away. It is the "approve it in the chat" path.
  function pendingBar(id) {
    const st = S[id], el = $('ai-pend-' + id);
    if (!st || !el) return;
    const list = Object.values(st.pending || {});
    if (!list.length) { el.style.display = 'none'; el.innerHTML = ''; return; }
    const a = list[0], isWrite = !!a.path;
    const what = a.command || a.path || a.transfer || a.tool || '';
    el.style.display = '';
    el.innerHTML = `<b>⏳ ${esc(t('ai_pending'))}</b>
      <span class="ai-pend-w" title="${esc(what)}">${esc(a.tool || '')} · ${esc(what)}</span>
      ${list.length > 1 ? `<span class="pill">${esc(tf('ai_pending_more', {n: list.length - 1}))}</span>` : ''}
      <button class="btn-sm" onclick="wrmAI.decide(${id}, '${esc(a.id)}', true, ${isWrite})">✓ ${esc(t('ai_approve'))}</button>
      <button class="btn-sec btn-sm" onclick="wrmAI.decide(${id}, '${esc(a.id)}', false, ${isWrite})">✕ ${esc(t('ai_deny'))}</button>
      <button class="btn-sec btn-sm" onclick="wrmAI.pendingDetails(${id}, '${esc(a.id)}')">${esc(t('ai_pending_details'))}</button>`;
  }

  function pendingDetails(id, aid) {
    const st = S[id];
    const a = st && st.pending && st.pending[aid];
    if (!a) return;
    const card = st.cards[a.call_id];
    if (card) card.scrollIntoView({block: 'center', behavior: 'smooth'});
  }

  function tick(id) {
    const b = $('aib-' + id);
    if (!b) { const st = S[id]; if (st && st.timer) { clearInterval(st.timer); st.timer = null; } return; }
    b.querySelectorAll('[data-exp]').forEach(el => {
      const s = Math.max(0, Math.round((Number(el.dataset.exp) - Date.now()) / 1000));
      el.textContent = tf('ai_expires', {s});
    });
  }

  function sound(id) {
    const nowOn = !wrmSoundOn();
    try { localStorage.setItem('wrm_sound', nowOn ? '1' : '0'); } catch (_) {}
    const b = $('aisnd-' + id);
    if (b) {
      b.textContent = nowOn ? '🔊' : '🔇';
      b.title = t(nowOn ? 'ai_sound_on' : 'ai_sound_off');
    }
    showToast(t(nowOn ? 'ai_sound_on_toast' : 'ai_sound_off_toast'), 'info', 2000);
    if (nowOn) wrmSound('reply');
  }

  async function decide(id, aid, ok, isWrite) {
    const wo = winOf(id), st = S[id];
    const ed = $(`ai-ed-${id}-${aid}`), note = $(`ai-nt-${id}-${aid}`);
    const body = {decision: ok ? 'approve' : 'deny', note: note ? note.value : ''};
    if (ok && ed) { if (isWrite) body.content = ed.value; else body.command = ed.value; }
    const sid = (st.apprSess && st.apprSess[aid]) || st.sid;
    const r = await wrmAISessions.decide(`/api/ai/sessions/${sid}/approvals/${encodeURIComponent(aid)}${shareQ(wo)}`, body);
    if (r.decided) {
      // decided elsewhere (the AI app, another tab) or no longer waiting: show it as such
      const a = st.pending && st.pending[aid];
      if (st.pending) delete st.pending[aid];
      pendingBar(id);
      const card = a && st.cards[a.call_id];
      if (card) { card.classList.remove('wait'); card.querySelector('.ai-appr').innerHTML = `<div class="ai-why">${esc(wrmAISessions.decidedText(r.decided))}</div>`; }
      indicators();
    }
  }

  async function send(id) {
    const wo = winOf(id), st = S[id], inp = $('ai-in-' + id), btn = $('ai-send-' + id);
    let text = (inp.value || '').trim();
    if (/^\/wrm(?:\s+server)?$/i.test(text)) { inp.value='';openWRMCommand();return; }
    if (!text || !st || !st.sid || st.busy) return;
    if (wo.mode === 'sftp' && wo.path) text = `[${t('ai_fm_ctx')}: ${wo.path}]\n` + text;
    
    workingUI(id, true);
    if (btn) btn.disabled = true;

    try { 
      inp.value = ''; 
      await api('POST', `/api/ai/sessions/${st.sid}/prompt${shareQ(wo)}`, {text}); 
    }
    catch (e) { 
      workingUI(id, false);
      showToast(e.message, 'error', 5000); 
    }
    finally {
      if (btn) btn.disabled = !!st.busy;
    }
  }
  async function stop(id) { const st = S[id]; if (st && st.sid) api('POST', `/api/ai/sessions/${st.sid}/stop${shareQ(winOf(id))}`).catch(e => showToast(e.message, 'error')); }
  async function end(id) {
    const st = S[id];
    if (!st || !st.sid || !(await uiConfirm(t('ai_end_q'), {okText: t('ai_end'), danger: true}))) return;
    api('POST', `/api/ai/sessions/${st.sid}/kill${shareQ(winOf(id))}`).catch(e => showToast(e.message, 'error'));
  }
  function newSession(id) { const st = S[id]; if (st && st.es) st.es.close(); S[id] = {pol: st && st.pol}; renderSetup(winOf(id)); indicators(); }
  // resume continues an ended session (idle, stopped, WRM restarted) in a new
  // one that gets the conversation back; sid is an ended session from the list.
  async function resume(id, sid) {
    const wo = winOf(id), st = S[id] || {};
    sid = sid || st.sid;
    if (!wo || !sid) return;
    try {
      const v = await api('POST', '/api/ai/sessions' + shareQ(wo), {resume_from: sid, conn_id: wo.connId});
      closeAIModal();
      attach(id, v.id);
    } catch (e) { showToast(e.message, 'error', 6000); }
  }
  async function rename(id, sid) {
    const st = S[id] || {};
    sid = sid || st.sid;
    const cur = (st.view && st.view.title) || st.title || '';
    const v = await uiPrompt(t('ai_rename'), cur);
    if (v == null || !v.trim()) return;
    try { const r = await api('POST', `/api/ai/sessions/${sid}/title`, {title: v}); if (st.sid === sid) { st.title = r.title; if (st.view) st.view.title = r.title; bar(id); } }
    catch (e) { showToast(e.message, 'error'); }
  }

  async function setMode(id, mode) {
    const wo = winOf(id), st = S[id];
    const body = {mode};
    if (mode === 'auto') {
      const r = await autoDialog();
      if (!r) { bar(id); return; }
      body.auto = r; body.confirm = true;
    }
    try { st.view = await api('POST', `/api/ai/sessions/${st.sid}/mode${shareQ(wo)}`, body); bar(id); }
    catch (e) { showToast(e.message, 'error', 6000); bar(id); }
  }

  // a small modal for the automatic mode opt-in during a session
  function modal(html) {
    let m = $('ai-modal');
    if (!m) { m = document.createElement('div'); m.id = 'ai-modal'; m.className = 'modal-overlay ai-modal'; m.style.zIndex = 9600; document.body.appendChild(m); }
    m.innerHTML = `<div class="modal">${html}</div>`;
    m.classList.add('open');
    return m;
  }
  function closeAIModal() { const m = $('ai-modal'); if (m) m.classList.remove('open'); }
  function autoDialog() {
    return new Promise(resolve => {
      modal(`<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">✦ ${esc(t('ai_mode_auto'))}</h3><button class="icon-btn" id="ai-ad-x">✕</button></div>
        <div class="ai-see" style="margin:8px 0;">${esc(t('ai_mode_auto_d'))}</div><div class="ai-setup">${autoForm('ai-ad')}</div>
        <div class="modal-footer"><button class="btn-sec" id="ai-ad-c">${esc(t('cancel'))}</button><button id="ai-ad-ok">${esc(t('ai_start'))}</button></div>`);
      const done = v => { closeAIModal(); resolve(v); };
      $('ai-ad-x').onclick = $('ai-ad-c').onclick = () => done(null);
      $('ai-ad-ok').onclick = () => { const a = readAuto('ai-ad'); if (!a.ok) { showToast(t('ai_auto_need_confirm'), 'warning'); return; } done(a.auto); };
    });
  }

  function see(id) {
    const st = S[id] || {};
    const prov = st.view ? st.view.provider : '';
    modal(`<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">👁 ${esc(t('ai_see'))}</h3><button class="icon-btn" onclick="wrmAI.closeModal()">✕</button></div>
      ${st.view ? `<div class="hint-box"><b>${esc(t('ai_mode_' + st.view.mode))}</b>: ${esc(t('ai_mode_' + st.view.mode + '_d'))}</div>` : ''}
      ${seeHTML(st.pol && st.pol.sees, prov, st.view && st.view.share_notes)}<div class="hint-box">${esc(t('ai_subscriptions_note'))}</div>`);
  }

  let histWin = 0;
  async function history(id) {
    histWin = id;
    let list = [];
    try { list = await api('GET', '/api/ai/sessions'); } catch (e) { showToast(e.message, 'error'); return; }
    modal(`<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">🕘 ${esc(t('ai_history'))}</h3><button class="icon-btn" onclick="wrmAI.closeModal()">✕</button></div>
      ${sessionsTable(list, false)}`);
  }

  function sessionsTable(list, admin) {
    if (!list.length) return `<div class="chat-empty">${esc(t('none'))}</div>`;
    return `<div class="tbl-scroll"><table class="admin-table"><thead><tr><th>${esc(t('started'))}</th>${admin ? `<th>${esc(t('user'))}</th>` : ''}<th>${esc(t('server'))}</th><th>${esc(t('ai_model'))}</th><th>${esc(t('status'))}</th><th>${esc(t('ai_tokens'))}</th><th></th></tr></thead><tbody>` +
      list.map(s => `<tr><td style="white-space:nowrap;">${s.title ? `<b>${esc(s.title)}</b><div class="sub">` : ''}${esc(fmtTime(s.started_at))}${s.purpose ? ' · ' + esc(t('ai_purpose_' + s.purpose)) : ''}${s.title ? '</div>' : ''}</td>${admin ? `<td>${esc(s.user)}</td>` : ''}
        <td>${esc(s.connection)}<div class="sub mono">${esc(s.host)}</div></td><td>${esc(s.model)}<div class="sub">${esc(s.provider)} · ${esc(t('ai_mode_' + s.mode))}</div></td>
        <td><span class="pill ${s.live ? 'ok' : s.status === 'killed' ? 'warn' : ''}">${esc(s.live ? t('ai_live') : s.status)}</span>${s.end_reason ? `<div class="sub">${esc(reasonText(s.end_reason))}</div>` : ''}</td>
        <td class="mono">${s.tokens_in}/${s.tokens_out}${s.cost_usd ? `<div class="sub">$${Number(s.cost_usd).toFixed(4)}</div>` : ''}<div class="sub">${s.tool_calls} ${esc(t('ai_tool_calls'))}</div></td>
        <td><div style="display:flex;gap:4px;flex-wrap:wrap;"><button class="btn-sec btn-sm" onclick="wrmAI.transcript(${s.id}, ${admin})">${esc(t('ai_transcript'))}</button>
          ${s.recording_session ? `<button class="btn-sec btn-sm" onclick="wrmAI.closeModal(); openReplay(${s.recording_session})">▶</button>` : ''}
          ${admin && s.live ? `<button class="btn-sm danger" onclick="wrmAI.adminKill(${s.id})">${esc(t('ai_kill'))}</button>` : ''}
          ${!admin && !s.live && histWin && s.transport === 'panel' && !s.purpose && winOf(histWin) && winOf(histWin).connId === s.conn_id ? `<button class="btn-sm" onclick="wrmAI.resume(${histWin}, ${s.id})">↪ ${esc(t('ai_continue_chat'))}</button>` : ''}</div></td></tr>`).join('') + '</tbody></table></div>';
  }

  async function transcript(sid, admin) {
    let v;
    try { v = await api('GET', (admin ? '/api/admin/ai/sessions/' : '/api/ai/sessions/') + sid); } catch (e) { showToast(e.message, 'error'); return; }
    const s = v.session || {};
    const icon = {user: '🧑', assistant: '✦', tool_call: '🛠', tool_result: '📄', approval: '⏳', decision: '⚖', mode: '⚙', error: '⚠', system: 'ℹ', end: '⏻'};
    modal(`<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">${esc(t('ai_transcript'))} #${sid}</h3><button class="icon-btn" onclick="wrmAI.closeModal()">✕</button></div>
      <div class="ai-see" style="margin:6px 0 10px;">${esc(s.user || '')} · ${esc(s.connection || '')} (${esc(s.host || '')}) · ${esc(s.provider || '')} / ${esc(s.model || '')} · ${esc(fmtTime(s.started_at))} · ${s.tokens_in || 0}/${s.tokens_out || 0} · $${Number(s.cost_usd || 0).toFixed(4)}</div>
      <div class="ai-tr">${(v.messages || []).map(m => `<div class="ai-card"><div class="ai-ch"><span>${icon[m.kind] || '·'}</span><span class="k">${esc(m.kind)}</span><span class="ai-why">${esc(m.actor)} · ${esc(fmtTime(m.ts))}</span></div>
        ${m.kind === 'assistant' ? `<div class="ai-msg assistant">${md(m.content)}</div>` : `<pre>${esc(m.content)}</pre>`}</div>`).join('')}</div>`);
  }

  // ── indicators on the windows ─────────────────────────
  function indicators() {
    winObjects.forEach(wo => {
      const st = S[wo.id], b = $('wai-' + wo.id), btn = $('wb-ai-' + wo.id);
      const on = !!(st && st.sid && !st.ended);
      if (b) {
        b.style.display = on ? '' : 'none';
        if (on && st.view) b.title = tf('ai_active_on', {mode: t('ai_mode_' + st.view.mode)});
      }
      if (btn) btn.classList.toggle('on', on || wo.el.classList.contains('ai-open'));
    });
  }

  function onWinClose(wo) {
    const st = S[wo.id];
    if (st && st.sid && !st.ended) api('POST', `/api/ai/sessions/${st.sid}/kill${shareQ(wo)}`).catch(() => {});
    if (st && st.es) st.es.close();
    if (st && st.timer) clearInterval(st.timer);
    delete S[wo.id]; delete MAX[wo.id];
  }

  async function refresh() {
    // sessions ended elsewhere (admin kill, policy): the event stream reports it; nothing else to poll
    indicators();
  }

  // ── providers (personal and organisation) ─────────────
  const KINDS = ['anthropic', 'openai', 'azure', 'bedrock', 'vertex', 'openai_compatible', 'opencode', 'opencode_go'];
  const MODEL_HINT = {anthropic: 'claude-opus-5-5, claude-sonnet-5-5, claude-haiku-5-5', bedrock: 'anthropic.claude-opus-5-5', vertex: 'claude-opus-5-5', opencode: 'big-pickle', opencode_go: 'kimi-k3, glm-5.3, deepseek-v4.1-flash, claude-haiku-5-5'};

  function provForm(p, scope) {
    p = p || {kind: 'anthropic', enabled: true, refusal_fallback: true};
    const kinds = scope === 'user' && cfg ? KINDS.filter(k => (cfg.provider_kinds || KINDS).includes(k)) : KINDS;
    const f = (k, lbl, val, type, ph) => `<div class="field" data-k="${k}"><label>${esc(lbl)}</label><input type="${type || 'text'}" id="aipf-${k}" value="${esc(val == null ? '' : val)}" placeholder="${esc(ph || '')}" autocomplete="off"></div>`;
    const sec = (k, lbl, has) => f(k, lbl, '', 'password', has ? t('ai_prov_key_keep') : '');
    return `<div class="ai-form">
      <div class="ai-row"><div class="field"><label>${esc(t('ai_prov_name'))}</label><input type="text" id="aipf-name" value="${esc(p.name || '')}"></div>
        <div class="field"><label>${esc(t('ai_prov_kind'))}</label><select id="aipf-kind" onchange="wrmAI.formKind()" ${p.id ? 'disabled' : ''}>${kinds.map(k => `<option value="${k}" ${k === p.kind ? 'selected' : ''}>${esc(t('ai_kind_' + k))}</option>`).join('')}</select></div></div>
      ${f('models', t('ai_prov_models'), p.models, 'text', MODEL_HINT[p.kind] || '')}
      ${f('default_model', t('ai_prov_default'), p.default_model)}
      <div class="kind-anthropic kind-openai kind-azure kind-bedrock kind-openai_compatible kind-opencode kind-opencode_go">${sec('api_key', t('ai_prov_key') + ' / ' + t('ai_prov_bkey'), p.has_api_key)}</div>
      <div class="kind-anthropic kind-openai kind-bedrock kind-vertex kind-opencode kind-opencode_go">${f('base_url', t('ai_prov_base'), p.base_url)}</div>
      <div class="kind-azure kind-openai_compatible">${f('base_url2', p.kind === 'azure' ? t('ai_prov_endpoint') : t('ai_prov_base_req'), p.base_url)}</div>
      <div class="kind-azure ai-row">${f('deployment', t('ai_prov_deployment'), p.deployment)}${f('api_version', t('ai_prov_apiver'), p.api_version, 'text', '2024-10-21')}</div>
      <div class="kind-bedrock kind-vertex ai-row">${f('region', t('ai_prov_region'), p.region, 'text', 'eu-central-1 / global')}<div class="field"><label>${esc(t('ai_prov_api'))}</label><select id="aipf-api_mode"></select></div></div>
      <div class="kind-vertex">${f('project', t('ai_prov_project'), p.project)}<div class="field"><label>${esc(t('ai_prov_sa'))}</label><textarea id="aipf-service_account" placeholder="${esc(p.has_service_account ? t('ai_prov_key_keep') : '{ "type": "service_account", … }')}"></textarea></div></div>
      <div class="kind-bedrock">${sec('access_key_id', t('ai_prov_ak'), p.has_access_key)}${sec('secret_access_key', t('ai_prov_sk'), p.has_access_key)}${sec('session_token', t('ai_prov_st'), false)}</div>
      <div class="ai-row">${f('price_in', t('ai_prov_price'), p.price_in || '', 'number')}${f('price_out', '​', p.price_out || '', 'number')}</div>
      ${f('max_tokens', t('ai_prov_maxtok'), p.max_tokens || 0, 'number')}
      <label class="ai-check kind-anthropic"><input type="checkbox" id="aipf-fallback" ${p.refusal_fallback !== false ? 'checked' : ''}> ${esc(t('ai_prov_fallback'))}</label>
      <label class="ai-check"><input type="checkbox" id="aipf-enabled" ${p.enabled !== false ? 'checked' : ''}> ${esc(t('ai_prov_enabled'))}</label></div>`;
  }
  function formKind(p) {
    const k = $('aipf-kind').value;
    document.querySelectorAll('.ai-form [class*="kind-"]').forEach(el => { el.style.display = el.classList.contains('kind-' + k) ? '' : 'none'; });
    const am = $('aipf-api_mode'), cur = (p && p.api_mode) || am.value;
    const opts = k === 'bedrock' ? ['converse', 'messages'] : k === 'vertex' ? ['anthropic', 'openai'] : [];
    am.innerHTML = opts.map(o => `<option ${o === cur ? 'selected' : ''}>${o}</option>`).join('');
    const ml = $('aipf-models');
    if (ml && !ml.value) ml.placeholder = MODEL_HINT[k] || '';
  }
  function readProvForm(p) {
    const v = id => { const el = $('aipf-' + id); return el ? el.value.trim() : ''; };
    const k = $('aipf-kind').value;
    const out = {name: v('name'), kind: k, models: v('models'), default_model: v('default_model'), price_in: Number(v('price_in')) || 0, price_out: Number(v('price_out')) || 0,
      max_tokens: Number(v('max_tokens')) || 0, enabled: $('aipf-enabled').checked, refusal_fallback: $('aipf-fallback').checked,
      base_url: (k === 'azure' || k === 'openai_compatible') ? v('base_url2') : v('base_url'), deployment: v('deployment'), api_version: v('api_version'),
      region: v('region'), project: v('project'), api_mode: v('api_mode')};
    for (const s of ['api_key', 'access_key_id', 'secret_access_key', 'session_token', 'service_account']) { const x = v(s); if (x) out[s] = x; }
    if (!out.models && !p) out.models = (MODEL_HINT[k] || '');
    return out;
  }

  async function providersDialog(scope, after) {
    const base = scope === 'org' ? '/api/admin/ai/providers' : '/api/ai/providers';
    let list = [];
    try { list = await api('GET', base); } catch (e) { showToast(e.message, 'error'); return; }
    const html = `<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">🔑 ${esc(t(scope === 'org' ? 'ai_providers_org' : 'ai_my_keys'))}</h3><button class="icon-btn" onclick="wrmAI.closeModal()">✕</button></div>
      <div class="hint-box">${esc(t('ai_subscriptions_note'))}</div>${provList(list, scope)}<div style="margin-top:10px;display:flex;gap:8px;flex-wrap:wrap;"><button class="btn-sm" onclick="wrmAI.awOpen('${scope}')">🧭 ${esc(t('aiw_wizard'))}</button><button class="btn-sec btn-sm" onclick="wrmAI.editProv('${scope}', 0)">＋ ${esc(t('ai_prov_add'))}</button></div>`;
    modal(html);
    provAfter = after;
  }
  let provAfter = null, provCache = [];
  function provList(list, scope) {
    provCache = list;
    if (!list.length) return `<div class="chat-empty">${esc(t('ai_prov_none'))}</div>`;
    return `<div class="tbl-scroll"><table class="admin-table"><thead><tr><th>${esc(t('ai_prov_name'))}</th><th>${esc(t('ai_prov_kind'))}</th><th>${esc(t('ai_models_col'))}</th><th>${esc(t('ai_prov_secrets'))}</th><th></th></tr></thead><tbody>` +
      list.map(p => `<tr><td>${esc(p.name)}${p.enabled ? '' : ` <span class="pill">${esc(t('off'))}</span>`}</td><td>${esc(t('ai_kind_' + p.kind))}${p.region ? `<div class="sub">${esc(p.region)}</div>` : ''}</td>
        <td class="mono" style="font-size:11px;">${esc(p.models)}</td><td>${p.has_api_key || p.has_access_key || p.has_service_account ? '🔒' : '—'}</td>
        <td><div style="display:flex;gap:4px;flex-wrap:wrap;"><button class="btn-sec btn-sm" onclick="wrmAI.testProv('${scope}', ${p.id})">${esc(t('ai_prov_test'))}</button>
          <button class="btn-sec btn-sm" onclick="wrmAI.editProv('${scope}', ${p.id})">✏️</button><button class="btn-sec btn-sm" onclick="wrmAI.delProv('${scope}', ${p.id})">🗑</button></div></td></tr>`).join('') + '</tbody></table></div>';
  }
  function editProv(scope, pid) {
    const p = provCache.find(x => x.id === pid);
    modal(`<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">${esc(t(p ? 'ai_prov_edit' : 'ai_prov_add'))}</h3><button class="icon-btn" onclick="wrmAI.closeModal()">✕</button></div>
      ${provForm(p, scope)}<div class="modal-footer"><button class="btn-sec" onclick="wrmAI.providers('${scope}')">${esc(t('cancel'))}</button><button onclick="wrmAI.saveProv('${scope}', ${pid})">${esc(t('save'))}</button></div>`);
    formKind(p);
  }
  async function saveProv(scope, pid) {
    const base = scope === 'org' ? '/api/admin/ai/providers' : '/api/ai/providers';
    const p = provCache.find(x => x.id === pid);
    const body = readProvForm(p);
    try { await api(pid ? 'PUT' : 'POST', pid ? `${base}/${pid}` : base, body); showToast(t('saved'), 'success', 1200); }
    catch (e) { showToast(e.message, 'error', 6000); return; }
    if (scope === 'org' && adminTab === 'ai') { closeAIModal(); adminAI(); } else providersDialog(scope, provAfter);
    if (provAfter) provAfter();
  }
  async function testProv(scope, pid) {
    const base = scope === 'org' ? '/api/admin/ai/providers' : '/api/ai/providers';
    showToast(t('loading'), 'info', 1500);
    try { const r = await api('POST', `${base}/${pid}/test`, {}); r.ok ? showToast(t('ai_prov_test_ok') + (r.reply ? ': ' + r.reply : ''), 'success', 5000) : showToast(r.error, 'error', 8000); }
    catch (e) { showToast(e.message, 'error', 8000); }
  }
  async function delProv(scope, pid) {
    if (!(await uiConfirm(t('ai_prov_del_q'), {okText: t('delete'), danger: true}))) return;
    const base = scope === 'org' ? '/api/admin/ai/providers' : '/api/ai/providers';
    try { await api('DELETE', `${base}/${pid}`); } catch (e) { showToast(e.message, 'error'); return; }
    if (scope === 'org' && adminTab === 'ai') adminAI(); else providersDialog(scope, provAfter);
  }

  // ── provider wizard: pick → key and model → test → done ──
  // Uses the same endpoints, policies and audit as the full provider form.
  // Model names are only suggestions: the field is free text (several models separated by commas,
  // the first is the default) and the last step can load the provider's own model list.
  const AIW_PRESETS = [
    {id: 'anthropic', kind: 'anthropic', model: 'claude-sonnet-5-5, claude-opus-5-5, claude-haiku-4-5', key: true},
    {id: 'openai', kind: 'openai', model: 'gpt-5, gpt-5-mini, gpt-4.1, gpt-4.1-mini', key: true},
    {id: 'opencode', kind: 'opencode', model: 'big-pickle', key: false},
    {id: 'opencode_go', kind: 'opencode_go', model: 'kimi-k3', key: true},
    {id: 'gemini', kind: 'openai_compatible', base: 'https://generativelanguage.googleapis.com/v1beta/openai', model: 'gemini-2.5-flash, gemini-2.5-pro, gemini-2.5-flash-lite', key: true},
    {id: 'groq', kind: 'openai_compatible', base: 'https://api.groq.com/openai/v1', model: 'llama-3.3-70b-versatile, openai/gpt-oss-120b, openai/gpt-oss-20b', key: true},
    {id: 'openrouter', kind: 'openai_compatible', base: 'https://openrouter.ai/api/v1', model: 'anthropic/claude-sonnet-4.5, openai/gpt-5, google/gemini-2.5-pro', key: true},
    {id: 'mistral', kind: 'openai_compatible', base: 'https://api.mistral.ai/v1', model: 'mistral-large-latest, mistral-medium-latest, codestral-latest', key: true},
    {id: 'xai', kind: 'openai_compatible', base: 'https://api.x.ai/v1', model: 'grok-4, grok-code-fast-1', key: true},
    {id: 'deepseek', kind: 'openai_compatible', base: 'https://api.deepseek.com/v1', model: 'deepseek-chat, deepseek-reasoner', key: true},
    {id: 'together', kind: 'openai_compatible', base: 'https://api.together.xyz/v1', model: 'meta-llama/Llama-3.3-70B-Instruct-Turbo, Qwen/Qwen2.5-Coder-32B-Instruct', key: true},
    {id: 'fireworks', kind: 'openai_compatible', base: 'https://api.fireworks.ai/inference/v1', model: 'accounts/fireworks/models/llama-v3p3-70b-instruct', key: true},
    {id: 'cerebras', kind: 'openai_compatible', base: 'https://api.cerebras.ai/v1', model: 'llama-3.3-70b', key: true},
    {id: 'cohere', kind: 'openai_compatible', base: 'https://api.cohere.ai/compatibility/v1', model: 'command-a-03-2025, command-r-plus-08-2024', key: true},
    {id: 'perplexity', kind: 'openai_compatible', base: 'https://api.perplexity.ai', model: 'sonar-pro, sonar', key: true},
    {id: 'ollama', kind: 'openai_compatible', base: 'http://127.0.0.1:11434/v1', model: 'qwen2.5-coder:7b', key: false},
    {id: 'custom', kind: 'openai_compatible', base: '', model: '', key: false},
  ];
  let AW = null;
  // The Ollama preset has one extra step: detect, install, start and pull.
  function awSteps() { return AW && AW.preset === 'ollama' ? ['aiw_s_pick', 'aiw_s_ollama', 'aiw_s_key', 'aiw_s_test'] : ['aiw_s_pick', 'aiw_s_key', 'aiw_s_test']; }
  function awStage() { return awSteps()[AW.step - 1]; }
  function awOpen(scope) {
    AW = {scope, step: 1, preset: null, pid: 0, result: null, oll: null, run: null, runs: [], avail: null};
    awRender();
  }
  const awCopy = async i => {
    const s = AW && AW.oll && AW.oll.steps[i];
    try { await navigator.clipboard.writeText(s.command); showToast(t('aiw_o_copied'), 'success', 1500); } catch (e) { showToast(s.command, 'info', 8000); }
  };
  function awOllamaBody() {
    const o = AW.oll;
    if (!o) return `<div class="ai-see">${esc(t('aiw_o_checking'))}</div>`;
    if (o.err) return `<div class="hint-box warn">${esc(o.err)}</div>`;
    const run = AW.run;
    const can = o.can_manage && !(run && run.status === 'running');
    const stepRow = (i, st) => {
      const act = o.can_manage ? `<button class="btn-sm" ${can ? '' : 'disabled'} onclick="wrmAI.awOllAct('${st.id}')">${esc(t('aiw_o_do_' + st.id))}</button>` : '';
      const pull = st.id === 'pull' ? `<input type="text" id="aiw-ollmodel" value="${esc(AW.pullModel != null ? AW.pullModel : 'qwen2.5-coder:7b')}" placeholder="${esc(t('aiw_o_model_ph'))}" oninput="wrmAI.awPullModel(this.value)" autocomplete="off" style="max-width:260px;">` : '';
      return `<div style="margin:8px 0;"><div><b>${esc(t('aiw_o_s_' + st.id))}</b></div>
        <div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;"><code class="mono" style="flex:1;min-width:200px;overflow-x:auto;white-space:nowrap;">${esc(st.command)}</code>
        <button class="btn-sec btn-sm" onclick="wrmAI.awCopy(${i})">${esc(t('aiw_o_copy'))}</button></div>
        ${pull || act ? `<div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin-top:4px;">${pull}${act}</div>` : ''}</div>`;
    };
    let h = `<div class="hint-box ${o.running ? '' : 'warn'}">${o.running ? '✅ ' + esc(tf('aiw_o_running', {v: o.version, url: o.url})) : '⚠ ' + esc(tf('aiw_o_notrunning', {url: o.url}))}</div>`;
    if (o.running) {
      h += `<div class="ai-see"><b>${esc(t('aiw_o_models'))}</b></div>` + (o.models.length
        ? `<div class="ai-wgrid">${o.models.map((m, i) => `<button class="btn-sec" onclick="wrmAI.awOllUse(${i})">${esc(t('aiw_o_use_model'))}: ${esc(m.name)}</button>`).join('')}</div>`
        : `<div class="ai-see">${esc(t('aiw_o_nomodels'))}</div>`);
    }
    h += `<div class="ai-see">${esc(tf('aiw_o_os', {os: o.os}))}</div><div class="ai-see">${esc(t('aiw_o_manual'))}</div>${o.steps.map((s, i) => stepRow(i, s)).join('')}`;
    if (!o.auto_supported) h += `<div class="ai-see">${esc(t('aiw_o_nosupport'))}</div>`;
    else if (!o.is_admin) h += `<div class="ai-see">${esc(t('aiw_o_noadmin'))}</div>`;
    else if (!o.policy_enabled) h += `<div class="ai-see">${esc(t('aiw_o_nopolicy'))}</div>`;
    if (run) {
      h += `<div class="hint-box ${run.status === 'failed' ? 'warn' : ''}"><b>${esc(run.action)} ${esc(run.target || '')}: ${esc(t('aiw_o_run_' + run.status))}</b>${run.output ? `<pre class="mono" style="white-space:pre-wrap;max-height:160px;overflow:auto;margin:6px 0 0;">${esc(run.output)}</pre>` : ''}</div>`;
    }
    return h;
  }
  async function awOllLoad() {
    try {
      AW.oll = await api('GET', '/api/ai/ollama/status');
      if (AW.oll.can_manage) AW.runs = await api('GET', '/api/ai/ollama/runs').catch(() => []);
    } catch (e) { AW.oll = {err: e.message}; }
    if (AW && AW.preset === 'ollama' && awStage() === 'aiw_s_ollama') awRender();
  }
  function awPullModel(v) { AW.pullModel = v; }
  function awOllUse(i) { AW.vals = Object.assign({}, AW.vals || {}, {model: AW.oll.models[i].name}); AW.step = AW.step + 1; AW.result = null; awRender(); }
  async function awOllAct(action) {
    const o = AW.oll;
    let model = '';
    if (action === 'pull') {
      model = (AW.pullModel != null ? AW.pullModel : ($('aiw-ollmodel') || {}).value || '').trim();
      if (!model) { showToast(t('aiw_need'), 'error'); return; }
    }
    const cmd = action === 'pull' ? 'ollama pull ' + model : o.steps.find(s => s.id === action).command;
    if (!(await uiConfirm(tf('aiw_o_confirm', {cmd}), {okText: t('aiw_o_run'), danger: true}))) return;
    try {
      const r = await api('POST', '/api/ai/ollama/action', {action, model, confirm: true, url: o.url});
      AW.run = {id: r.run_id, action, target: model, status: 'running', output: ''};
    } catch (e) { showToast(e.message, 'error', 8000); return; }
    awRender();
    const id = AW.run.id;
    for (let i = 0; i < 1500 && AW && AW.run && AW.run.id === id; i++) {
      await new Promise(res => setTimeout(res, 2000));
      if (!AW || !AW.run || AW.run.id !== id) return;
      try {
        const r = await api('GET', `/api/ai/ollama/runs/${id}`);
        AW.run = {id, action, target: model, status: r.status, output: r.output};
        if (r.status !== 'running') break;
      } catch (e) { break; }
    }
    if (AW && awStage() === 'aiw_s_ollama') { awRender(); awOllLoad(); }
  }
  function awRender() {
    const steps = awSteps();
    const p = AIW_PRESETS.find(x => x.id === AW.preset);
    const stage = AW.step > 0 ? awStage() : '';
    const bar = `<div class="ai-wsteps">${steps.map((s, i) => `<span class="${i + 1 === AW.step ? 'on' : i + 1 < AW.step ? 'done' : ''}">${i + 1}. ${esc(t(s))}</span>`).join('')}</div>`;
    let body = '', nav = '';
    if (stage === 'aiw_s_pick') {
      const kinds = AW.scope === 'user' && cfg ? (cfg.provider_kinds || KINDS) : KINDS;
      const list = AIW_PRESETS.filter(x => kinds.includes(x.kind));
      body = `<div class="ai-see">${esc(t('aiw_pick'))}</div><div class="ai-wgrid">${list.map(x => `<button class="btn-sec" onclick="wrmAI.awPick('${x.id}')">${esc(t('aiw_p_' + x.id))}</button>`).join('')}
        <button class="btn-sec" onclick="wrmAI.editProv('${AW.scope}', 0)">${esc(t('aiw_p_advanced'))}</button></div>`;
    } else if (stage === 'aiw_s_ollama') {
      if (!AW.oll) awOllLoad();
      body = `<div class="ai-see">${esc(t('aiw_o_intro'))}</div>${awOllamaBody()}`;
      nav = `<button class="btn-sec" onclick="wrmAI.awGo(-1)">← ${esc(t('aiw_back'))}</button><button class="btn-sec" onclick="wrmAI.awOllRefresh()">${esc(t('aiw_o_refresh'))}</button><button onclick="wrmAI.awGo(1)">${esc(t('aiw_o_next'))} →</button>`;
    } else if (stage === 'aiw_s_key') {
      const v = AW.vals || {};
      const field = (id, lbl, val, type, ph) => `<div class="field"><label>${esc(lbl)}</label><input type="${type || 'text'}" id="aiw-${id}" value="${esc(val || '')}" placeholder="${esc(ph || '')}" autocomplete="off"></div>`;
      const defBase = p.id === 'ollama' && AW.oll && AW.oll.openai_base ? AW.oll.openai_base : p.base;
      const defModel = p.id === 'ollama' && AW.oll && AW.oll.models && AW.oll.models.length ? AW.oll.models[0].name : p.model;
      body = `<div class="ai-form">${field('name', t('aiw_name'), v.name != null ? v.name : t('aiw_p_' + p.id))}
        ${p.kind === 'openai_compatible' ? field('base', t('aiw_endpoint'), v.base != null ? v.base : defBase, 'text', 'https://…/v1') : ''}
        ${p.key || p.id === 'custom' || p.kind === 'opencode' ? field('key', t(p.key ? 'aiw_key' : 'aiw_key_opt'), '', 'password') : ''}
        ${field('model', t('aiw_model'), v.model != null ? v.model : defModel, 'text', tf('aiw_model_ph', {m: (p.model || 'model-name').split(',')[0]}))}
        <div class="ai-see">${esc(t('aiw_key_note'))} ${esc(t('aiw_model_note'))}${t('aiw_n_' + p.id) !== 'aiw_n_' + p.id ? ' ' + esc(t('aiw_n_' + p.id)) : ''}</div></div>`;
      nav = `<button class="btn-sec" onclick="wrmAI.awGo(-1)">← ${esc(t('aiw_back'))}</button><button onclick="wrmAI.awTest()">${esc(t('aiw_next'))} →</button>`;
    } else {
      const r = AW.result;
      body = !r ? `<div class="ai-see">${esc(t('aiw_testing'))}</div>` : r.ok
        ? `<div class="hint-box">✅ ${esc(tf('aiw_ok', {reply: r.reply ? ': ' + r.reply : ''}))}</div><div class="ai-see">${esc(t('aiw_use'))}</div>${availabilityHTML(AW.availability)}
          <div style="margin:8px 0;"><button class="btn-sec btn-sm" onclick="wrmAI.awLoadModels()">${esc(t('aiw_load_models'))}</button></div>${awAvailHTML()}`
        : `<div class="hint-box warn">❌ ${esc(tf('aiw_fail', {err: r.error}))}</div><div class="ai-see">${esc(t('aiw_fix'))}</div>`;
      nav = r && !r.ok ? `<button class="btn-sec" onclick="wrmAI.awGo(-1)">← ${esc(t('aiw_back'))}</button>` : '';
      nav += r && r.ok ? `<button onclick="wrmAI.awFinish()">✓ ${esc(t('aiw_finish'))}</button>` : '';
    }
    modal(`<div style="display:flex;align-items:center;gap:10px;"><h3 style="margin:0;flex:1;">🧭 ${esc(t('aiw_title'))}</h3><button class="icon-btn" onclick="wrmAI.awFinish()">✕</button></div>${bar}${body}${nav ? `<div class="modal-footer">${nav}</div>` : ''}`);
  }
  function awAvailHTML() {
    if (!AW.avail) return '';
    if (AW.avail.error) return `<div class="hint-box warn">${esc(AW.avail.error)}</div>`;
    if (!AW.avail.models.length) return `<div class="ai-see">${esc(t('aiw_models_none'))}</div>`;
    return `<div class="ai-see">${esc(t('aiw_models_loaded'))}</div><div style="display:flex;gap:6px;flex-wrap:wrap;max-height:160px;overflow:auto;">${AW.avail.models.map((m, i) => `<button class="btn-sec btn-sm" onclick="wrmAI.awAddModel(${i})">${esc(m)}${(AW.avail.free || []).includes(m) ? ` <span class="pill">${esc(t('aiw_free'))}</span>` : ''}</button>`).join('')}</div>`;
  }
  function awProvBase() { return AW.scope === 'org' ? '/api/admin/ai/providers' : '/api/ai/providers'; }
  async function awLoadModels() {
    try {
      const r = await api('GET', `${awProvBase()}/${AW.pid}/models`);
      AW.avail = r.ok ? {models: r.models, free: r.free || []} : {error: r.error};
    } catch (e) { AW.avail = {error: e.message}; }
    awRender();
  }
  async function awAddModel(i) {
    const name = AW.avail.models[i];
    const base = awProvBase();
    try {
      const cur = await api('GET', `${base}/${AW.pid}`);
      const list = (cur.models || '').split(/[\s,]+/).filter(Boolean);
      if (!list.includes(name)) list.push(name);
      await api('PUT', `${base}/${AW.pid}`, {models: list.join(','), default_model: cur.default_model || list[0]});
      showToast(t('aiw_models_added'), 'success', 1500);
    } catch (e) { showToast(e.message, 'error'); }
  }
  function awOllRefresh() { AW.oll = null; awRender(); }
  function awPick(id) { AW.preset = id; AW.vals = null; AW.step = 2; AW.oll = null; AW.run = null; AW.avail = null; awRender(); }
  function awGo(d) {
    if (awStage() === 'aiw_s_key') AW.vals = awRead();
    AW.step += d; AW.result = null; awRender();
  }
  function awRead() {
    const v = id => { const el = $('aiw-' + id); return el ? el.value.trim() : ''; };
    return {name: v('name'), base: v('base').replace(/\/+$/, ''), key: ($('aiw-key') || {}).value || '', model: v('model')};
  }
  async function awTest() {
    const p = AIW_PRESETS.find(x => x.id === AW.preset);
    const v = awRead();
    AW.vals = Object.assign({}, v, {key: ''});
    if (!v.name || !v.model || (p.kind === 'openai_compatible' && !v.base) || (p.key && !v.key && !AW.pid)) { showToast(t('aiw_need'), 'error'); return; }
    AW.step = awSteps().length; AW.result = null; awRender();
    const base = AW.scope === 'org' ? '/api/admin/ai/providers' : '/api/ai/providers';
    const models = v.model.split(/[\s,]+/).filter(Boolean).join(',');
    const body = {name: v.name, kind: p.kind, models, default_model: models.split(',')[0], enabled: true, refusal_fallback: true, base_url: p.kind === 'openai_compatible' || p.kind === 'openai' ? v.base : ''};
    if (v.key) body.api_key = v.key;
    try {
      const saved = await api(AW.pid ? 'PUT' : 'POST', AW.pid ? `${base}/${AW.pid}` : base, body);
      AW.pid = AW.pid || (saved && (saved.id || (saved.provider && saved.provider.id))) || 0;
      if (!AW.pid) {
        const list = await api('GET', base);
        const mine = list.filter(x => x.name === v.name).sort((a, b) => b.id - a.id)[0];
        AW.pid = mine ? mine.id : 0;
      }
      AW.result = AW.pid ? await api('POST', `${base}/${AW.pid}/test`, {}) : {ok: false, error: t('aiw_need')};
    } catch (e) { AW.result = {ok: false, error: e.message}; }
    if (AW && AW.step === awSteps().length) { AW.availability = await loadCfg().catch(() => null); if (typeof refreshUserPolicy === 'function') await refreshUserPolicy(); awRender(); }
  }
  function awFinish() {
    closeAIModal();
    const scope = AW && AW.scope;
    AW = null;
    if (scope === 'org' && typeof adminTab !== 'undefined' && adminTab === 'ai') adminAI();
    if (provAfter) provAfter();
  }

  // ── admin tab ─────────────────────────────────────────
  const AI_POLICY_KEYS = ['ai_assistant', 'ai_kill_switch', 'ai_modes', 'ai_default_mode', 'ai_mode_rules', 'ai_personal_keys', 'ai_provider_kinds', 'ai_models', 'ai_ollama_manage',
    'ai_redact_output', 'ai_show_thinking', 'ai_auto_titles', 'ai_share_notes', 'ai_allow_power', 'ai_sensitive_reads', 'ai_blocked_commands', 'ai_read_deny_paths', 'ai_approval_timeout_seconds', 'ai_auto_max_minutes',
    'ai_auto_max_actions', 'ai_auto_until_grant', 'ai_provider_header_seconds', 'ai_provider_request_seconds', 'ai_provider_stream_idle_seconds', 'ai_command_timeout_seconds', 'ai_output_max_kb', 'ai_max_steps', 'ai_idle_minutes', 'ai_transcript_retention_days', 'ai_keep_saved_sessions'];

  async function adminAI() {
    const body = $('admin-body');
    let sum, provs, sessions, settings, ocs;
    try {
      [sum, provs, sessions, settings, ocs] = await Promise.all([api('GET', '/api/admin/ai/summary'), api('GET', '/api/admin/ai/providers'),
        api('GET', '/api/admin/ai/sessions?limit=100'), api('GET', '/api/admin/settings'), api('GET', '/api/admin/opencode').catch(e => ({error: e.message, log: []}))]);
    } catch (e) { body.innerHTML = `<div class="fm-error">${esc(e.message)}</div>`; return; }
    adminSettingsCache = settings;
    const ks = sum.kill_switch;
    body.innerHTML = `
      <div class="adm-h">⏻ ${esc(t('adm_ai_kill'))}</div>
      <div class="hint-box ${ks ? 'warn' : ''}" style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;"><span style="flex:1;min-width:200px;">${esc(ks ? t('ai_kill_switch_on') : t('ai_kill_switch_off'))} · ${sum.active.length} ${esc(t('ai_live'))}</span>
        <button class="btn-sm danger" onclick="wrmAI.killAll()">⏻ ${esc(t('ai_kill_all'))}</button></div>
      <div class="adm-h">🔑 ${esc(t('ai_providers_org'))}</div>
      <div class="hint-box">${esc(t('ai_subscriptions_note'))}</div>
      ${provList(provs, 'org')}<div style="margin:8px 0 4px;"><button class="btn-sm" onclick="wrmAI.editProv('org', 0)">＋ ${esc(t('ai_prov_add'))}</button></div>
      ${delegateHTML(provs)}
      <div class="adm-h">⬛ ${esc(t('ai_oc_h'))}</div><div class="hint-box">${esc(t('ai_oc_help'))}</div><div id="ai-oc-box">${ocBoxHTML(ocs)}</div>
      <div class="policy-group">${OC_KEYS.map(k => settingRowHTML(adminSettingsCache.find(x => x.key === k))).join('')}</div>
      <div class="adm-h">✦ ${esc(t('ai_sessions'))}</div>${sessionsTable(sessions, true)}
      <div class="adm-h">📊 ${esc(t('ai_usage_users'))}</div>
      <div class="tbl-scroll"><table class="admin-table"><thead><tr><th>${esc(t('user'))}</th><th>${esc(t('ai_sessions'))}</th><th>${esc(t('ai_tokens'))}</th><th>${esc(t('ai_cost'))}</th><th></th></tr></thead><tbody>` +
      sum.usage.map(u => `<tr><td>${esc(u.user)}${u.blocked ? ` <span class="pill bad">${esc(t('ai_blocked'))}</span>` : ''}</td><td>${u.sessions}${u.active ? ` · <span class="pill ok">${u.active} ${esc(t('ai_live'))}</span>` : ''}</td>
        <td class="mono">${u.tokens_in}/${u.tokens_out}</td><td class="mono">$${Number(u.cost_usd).toFixed(4)}</td>
        <td><div style="display:flex;gap:4px;flex-wrap:wrap;">${u.active ? `<button class="btn-sm danger" onclick="wrmAI.killUser(${u.user_id})">${esc(t('ai_kill'))}</button>` : ''}
          <button class="btn-sec btn-sm" onclick="wrmAI.blockUser(${u.user_id}, ${!u.blocked})">${esc(t(u.blocked ? 'ai_unblock' : 'ai_block'))}</button></div></td></tr>`).join('') + `</tbody></table></div>
      <div class="adm-h">⚙ ${esc(t('ai_policies'))}</div><div class="hint-box">${esc(t('ai_mode_rules_help'))}</div>
      <div class="policy-group">${AI_POLICY_KEYS.map(k => settingRowHTML(adminSettingsCache.find(x => x.key === k))).join('')}</div>
      <div class="adm-h">⛔ ${esc(t('ai_destructive'))}</div><ul class="ai-see">${sum.destructive_rules.map(r => `<li><span class="mono">${esc(r.id)}</span> — ${esc(r.what)}</li>`).join('')}</ul>`;
  }
  // OpenCode on the WRM host: state, one-click install (with consent), its log
  const OC_KEYS = ['ai_opencode', 'ai_opencode_version', 'ai_opencode_registry', 'ai_opencode_path', 'ai_opencode_max_sessions', 'ai_opencode_sandbox', 'ai_opencode_sandbox_image'];
  let ocState = null, ocTimer = null;
  function ocBoxHTML(s) {
    ocState = s || {};
    s = ocState;
    const ok = !!s.binary, custom = s.source === 'path';
    const state = custom ? tf('ai_oc_custom', {p: s.binary}) : s.installed ? tf('ai_oc_installed', {v: s.version}) + (s.installed_by ? ' · ' + tf('ai_oc_by', {u: s.installed_by, t: s.installed_at ? new Date(s.installed_at).toLocaleString() : ''}) : '')
      : s.supported === false ? tf('ai_oc_unsupported', {os: s.os || ''}) : t('ai_oc_missing');
    const busy = !!s.installing;
    const btn = s.supported === false ? '' : `<button class="btn-sm" ${busy ? 'disabled' : ''} onclick="wrmAI.ocInstall()">⬇ ${esc(busy ? t('ai_oc_installing') : s.installed ? t('ai_oc_reinstall') : t('ai_oc_install'))}</button>`;
    const log = (s.log || []).length ? `<details ${busy || s.error ? 'open' : ''}><summary class="ai-see">${esc(t('ai_oc_log'))}</summary><pre class="mono" style="max-height:220px;overflow:auto;white-space:pre-wrap;font-size:11px;">${esc(s.log.join('\n'))}</pre></details>` : '';
    return `<div class="hint-box ${ok ? '' : 'warn'}" style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;"><span style="flex:1;min-width:200px;">${ok ? '✅' : '⚪'} ${esc(state)}${ok ? ' · ' + esc(tf('ai_oc_sessions', {n: s.sessions || 0, max: s.max_sessions || 0})) : ''}</span>
      ${btn}<button class="btn-sec btn-sm" onclick="wrmAI.ocRefresh()">↻ ${esc(t('ai_oc_refresh'))}</button></div>
      ${s.sandbox ? `<div class="hint-box ${s.sandbox.error ? 'warn' : ''}">🐳 ${esc(s.sandbox.error ? tf('ai_oc_sb_err', {e: s.sandbox.error}) : tf('ai_oc_sb_ok', {docker: s.sandbox.docker || '', image: s.sandbox.image || ''}))}</div>` : ''}
      ${s.error ? `<div class="hint-box warn">⚠ ${esc(t('ai_oc_failed'))}: ${esc(s.error)}</div>` : ''}${log}`;
  }
  async function ocRefresh() {
    let s;
    try { s = await api('GET', '/api/admin/opencode'); } catch (e) { s = {error: e.message, log: []}; }
    const box = $('ai-oc-box');
    if (box) box.innerHTML = ocBoxHTML(s);
    if (ocTimer) { clearTimeout(ocTimer); ocTimer = null; }
    if (s.installing && box) ocTimer = setTimeout(ocRefresh, 1500);
    return s;
  }
  async function ocInstall() {
    const s = ocState || {};
    const st = k => (adminSettingsCache.find(x => x.key === k) || {}).value || '';
    const q = tf('ai_oc_install_q', {v: st('ai_opencode_version') || s.version_wanted || '', pkg: s.package || '', reg: st('ai_opencode_registry') || 'https://registry.npmjs.org', root: s.root || ''});
    if (!(await uiConfirm(q, {okText: t('ai_oc_install')}))) return;
    try { await api('POST', '/api/admin/opencode/install', {confirm: true}); } catch (e) { showToast(e.message, 'error', 6000); }
    await ocRefresh();
  }

  // what the assistant may hand to helper models, and the user's own training choice
  const DATA_USE_ICON = {zero_retention: '🛡', trains: '⚠', unknown: '❔', paid: '💳', provider: '🔑'};
  function dataUseTag(m) { return `<span class="pill ${m.data_use === 'trains' || m.data_use === 'unknown' ? 'warn' : ''}" title="${esc(t('ai_du_' + m.data_use + '_d'))}">${DATA_USE_ICON[m.data_use] || ''} ${esc(t('ai_du_' + m.data_use))}</span>`; }
  function delegSetupHTML(id) {
    const d = cfg && cfg.delegate;
    if (!d || !d.enabled) return '';
    const on = (d.models || []).filter(m => m.allowed), off = (d.models || []).filter(m => !m.allowed);
    const toggle = d.training_policy === 'user' ? `<label class="ai-check"><input type="checkbox" ${d.training ? 'checked' : ''} onchange="wrmAI.delegTraining(${id}, this.checked)"> ${esc(t('ai_deleg_training_me'))}</label>`
      : `<div class="ai-see">${esc(t(d.training_policy === 'all' ? 'ai_deleg_training_all' : 'ai_deleg_training_off'))}</div>`;
    const row = m => `<div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;font-size:12px;margin:2px 0;"><span class="mono">${esc(m.id)}</span>${m.free ? ` <span class="pill">${esc(t('aiw_free'))}</span>` : ''} ${dataUseTag(m)}</div>`;
    return `<details><summary class="ai-see">🤝 ${esc(tf('ai_deleg_setup', {p: d.provider, n: on.length}))}</summary><div class="ai-see">${esc(t('ai_deleg_setup_d'))}</div>
      ${toggle}${on.map(row).join('')}${off.length ? `<div class="ai-see" style="margin-top:6px;">${esc(t('ai_deleg_excluded'))}</div>${off.map(row).join('')}` : ''}</details>`;
  }
  async function delegTraining(id, on) {
    try { await api('PUT', '/api/ai/delegate', {training: on}); } catch (e) { showToast(e.message, 'error'); }
    const wo = winOf(id); if (wo && !(S[id] && S[id].sid)) renderSetup(wo);
  }

  // delegation to helper models: provider, models, limits (settings ai_delegate*)
  function delegateHTML(provs) {
    const st = k => adminSettingsCache.find(x => x.key === k) || {value: ''};
    const on = st('ai_delegate').value === '1', pid = Number(st('ai_delegate_provider').value) || 0, models = st('ai_delegate_models').value;
    const prov = provs.find(p => p.id === pid);
    const state = !on ? t('ai_deleg_off') : !prov ? t('ai_deleg_noprov') : !prov.enabled ? t('ai_deleg_disabled') : tf('ai_deleg_on', {p: prov.name, m: models || prov.models});
    const locked = st('ai_delegate_provider').locked || st('ai_delegate_models').locked ? 'disabled' : '';
    return `<div class="adm-h">🤝 ${esc(t('ai_deleg_h'))}</div><div class="hint-box">${esc(t('ai_deleg_help'))}</div>
      <div class="hint-box ${on && prov && prov.enabled ? '' : 'warn'}" id="ai-deleg-state">${on && prov && prov.enabled ? '✅' : '⚪'} ${esc(state)}</div>
      <div class="policy-group">${settingRowHTML(st('ai_delegate').key ? st('ai_delegate') : null)}
        <div class="setting-row"><div><div class="setting-label">${esc(t('p_ai_delegate_provider'))}</div><div class="setting-desc">${esc(t('p_ai_delegate_provider_d'))}</div></div>
          <div class="setting-control"><select class="setting-select" id="ai-deleg-prov" ${locked} onchange="wrmAI.delegProv(this)"><option value="0">—</option>${provs.map(p => `<option value="${p.id}" ${p.id === pid ? 'selected' : ''}>${esc(p.name)} (${esc(t('ai_kind_' + p.kind))})</option>`).join('')}</select></div></div>
        <div class="setting-row" style="display:block;"><div class="setting-label">${esc(t('p_ai_delegate_models'))}</div><div class="setting-desc">${esc(t('p_ai_delegate_models_d'))}</div>
          <div style="display:flex;gap:6px;flex-wrap:wrap;margin-top:6px;"><input type="text" id="ai-deleg-models" style="flex:1;min-width:220px;" value="${esc(models)}" placeholder="${esc(prov ? prov.models : '')}" ${locked} onchange="saveSetting('ai_delegate_models', this.value.trim(), this).then(() => wrmAI.adminTab())">
            <button class="btn-sec btn-sm" ${pid ? '' : 'disabled'} onclick="wrmAI.delegLoad(${pid})">${esc(t('ai_deleg_load'))}</button></div><div id="ai-deleg-list" style="display:flex;gap:6px;flex-wrap:wrap;margin-top:6px;"></div></div>
        ${settingRowHTML(st('ai_delegate_training').key ? st('ai_delegate_training') : null)}
        ${settingRowHTML(st('ai_delegate_max_kb').key ? st('ai_delegate_max_kb') : null)}</div>`;
  }
  async function delegProv(el) { await saveSetting('ai_delegate_provider', el.value, el); adminAI(); }
  async function delegLoad(pid) {
    const box = $('ai-deleg-list');
    box.innerHTML = `<span class="ai-see">${esc(t('loading'))}</span>`;
    let r;
    try { r = await api('GET', `/api/admin/ai/providers/${pid}/models`); } catch (e) { box.innerHTML = `<div class="hint-box warn">${esc(e.message)}</div>`; return; }
    if (!r.ok) { box.innerHTML = `<div class="hint-box warn">${esc(r.error)}</div>`; return; }
    const free = r.free || [], du = r.data_use || {};
    const list = free.length ? r.models.filter(m => free.includes(m)).concat(r.models.filter(m => !free.includes(m))) : r.models;
    box.innerHTML = list.length ? list.map(m => `<button class="btn-sec btn-sm" onclick="wrmAI.delegAdd(this.dataset.m)" data-m="${esc(m)}">＋ ${esc(m)}${free.includes(m) ? ` <span class="pill">${esc(t('aiw_free'))}</span>` : ''}${du[m] ? ' ' + dataUseTag({data_use: du[m]}) : ''}</button>`).join('') : `<span class="ai-see">${esc(t('aiw_models_none'))}</span>`;
  }
  async function delegAdd(m) {
    const el = $('ai-deleg-models');
    const list = el.value.split(/[\s,]+/).filter(Boolean);
    if (!list.includes(m)) list.push(m);
    el.value = list.join(',');
    await saveSetting('ai_delegate_models', el.value, el);
    showToast(t('saved'), 'success', 1200);
  }
  async function killAll() {
    if (!(await uiConfirm(t('ai_kill_all_q'), {okText: t('ai_kill_all'), danger: true}))) return;
    try { const r = await api('POST', '/api/admin/ai/kill-all'); showToast(tf('ai_killed_n', {n: r.ended}), 'success'); } catch (e) { showToast(e.message, 'error'); }
    adminAI();
  }
  async function killUser(uid) {
    try { const r = await api('POST', `/api/admin/ai/users/${uid}/kill`); showToast(tf('ai_killed_n', {n: r.ended}), 'success'); } catch (e) { showToast(e.message, 'error'); }
    adminAI();
  }
  async function blockUser(uid, blocked) {
    try { await api('PUT', `/api/admin/ai/users/${uid}`, {blocked}); } catch (e) { showToast(e.message, 'error'); }
    adminAI();
  }
  async function adminKill(sid) {
    try { await api('POST', `/api/admin/ai/sessions/${sid}/kill`); } catch (e) { showToast(e.message, 'error'); }
    closeAIModal(); if (adminTab === 'ai') adminAI();
  }

  window.wrmAI = {
    md, diffHTML, reasonText, loadCfg,
    queueContext, toggle, dock, expand, start, attach, models, modeHelp, send, stop, end, decide, setMode, see, history, transcript, newSession, resume, rename, sound, pendingDetails,
    myKeys: id => providersDialog('user', () => { const wo = winOf(id); if (wo && !(S[id] && S[id].sid)) renderSetup(wo); else if (!wo) statusPane(); }),
    providers: scope => providersDialog(scope, provAfter), editProv, saveProv, testProv, delProv, formKind: () => formKind(),
    awOpen, awPick, awGo, awTest, awFinish, awCopy, awOllAct, awOllUse, awOllRefresh, awPullModel, awLoadModels, awAddModel,
    myWizard: id => { provAfter = () => { const wo = winOf(id); if (wo && !(S[id] && S[id].sid)) loadCfg().then(() => renderSetup(wo)); else if (!wo) statusPane(); }; awOpen('user'); },
    openPanel, busyIn, focusSession, prefsProv, savePrefs,
    statusPane, policyChanged, closeModal: closeAIModal, adminTab: adminAI, engine, ocInstall, ocRefresh, delegProv, delegLoad, delegAdd, delegTraining, killAll, killUser, blockUser, adminKill, onWinClose, refresh, indicators,
  };
})();
