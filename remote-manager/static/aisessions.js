/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// WRM PRO AI sessions (v13.8.0).
// One panel lists every AI session of the signed-in user, next to the saved Sessions: the
// assistant in a terminal, the log and file assistants, OpenCode agents and AI apps over MCP.
// A session can be searched and filtered, opened (transcript; the live view while it runs),
// continued in the WRM assistant (an AI app's work becomes the earlier context), saved so
// that it outlives the transcript retention, renamed and exported as Markdown or JSON.
// Each user only ever sees their own sessions (/api/ai/sessions?view=all).
// It also holds the shared text for a decision on an approval that no longer waits.
(function () {
  'use strict';

  Object.assign(LANGS.en, {
    aiss_panel: 'AI sessions', aiss_search: 'Search AI sessions…', aiss_k_all: 'All', aiss_k_assistant: 'Assistant', aiss_k_logs: 'Log assistant',
    aiss_k_file: 'File assistant', aiss_k_opencode: 'OpenCode', aiss_k_mcp: 'AI apps (MCP)', aiss_k_saved: '⭐ Saved', aiss_k_live: '● Running',
    aiss_none: 'No AI sessions yet. Start the assistant in a terminal (✦) or connect an AI app in Settings → AI connections.',
    aiss_none_q: 'No AI session matches.', aiss_more: 'Older sessions', aiss_live: 'running', aiss_refresh: 'Refresh',
    aiss_save: 'Save', aiss_saved: 'Saved', aiss_unsave: 'Remove from saved', aiss_saved_t: 'Saved sessions are kept beyond the transcript retention ({days} days).',
    aiss_saved_t_off: 'The administrator does not keep saved sessions beyond the transcript retention ({days} days).',
    aiss_continue: 'Continue', aiss_continue_t: 'Continue this conversation with the WRM assistant in a terminal on the same server',
    aiss_continue_mcp_t: 'Continue in the WRM assistant: what the AI app did on the server becomes its earlier context',
    aiss_open_live: 'Open live view', aiss_open_chat: 'Go to the chat', aiss_rename: 'Rename', aiss_export_md: 'Export Markdown', aiss_export_json: 'Export JSON',
    aiss_transcript: 'Transcript', aiss_empty_tr: 'Nothing was recorded in this session.', aiss_conn_missing: 'The connection of this session no longer exists or is not yours.',
    aiss_tokens: '{in} in / {out} out', aiss_tools: '{n} tool calls', aiss_appr: '{a} approved · {d} refused', aiss_resumed: 'continues #{n}',
    aiss_by_app: 'AI app', aiss_untitled: 'Untitled session', aiss_hint: 'Only you see your AI sessions. Administrators see every session under Admin → AI.',
    aiss_open: 'Open',
    // decisions on approvals that no longer wait (WRM assistant, live view, 🤖 menu)
    ai_dec_done: 'Already {outcome} by {by} {where}{time}.', ai_dec_where_app: 'in the AI app', ai_dec_where_wrm: 'in WRM',
    ai_dec_closed: 'The AI stopped waiting for this request and it was closed. Ask the AI to repeat it.',
    ai_dec_gone: 'This request is no longer waiting (the AI session ended or it expired). Ask the AI to repeat it.',
    ai_dec_o_approved: 'approved', ai_dec_o_denied: 'denied',
  });
  Object.assign(LANGS.hr, {
    aiss_panel: 'AI sesije', aiss_search: 'Traži AI sesije…', aiss_k_all: 'Sve', aiss_k_assistant: 'Asistent', aiss_k_logs: 'Asistent za logove',
    aiss_k_file: 'Asistent za datoteke', aiss_k_opencode: 'OpenCode', aiss_k_mcp: 'AI aplikacije (MCP)', aiss_k_saved: '⭐ Spremljene', aiss_k_live: '● Aktivne',
    aiss_none: 'Još nema AI sesija. Pokreni asistenta u terminalu (✦) ili poveži AI aplikaciju u Postavke → AI veze.',
    aiss_none_q: 'Nijedna AI sesija ne odgovara.', aiss_more: 'Starije sesije', aiss_live: 'aktivna', aiss_refresh: 'Osvježi',
    aiss_save: 'Spremi', aiss_saved: 'Spremljeno', aiss_unsave: 'Makni iz spremljenih', aiss_saved_t: 'Spremljene sesije čuvaju se i nakon roka čuvanja transkripata ({days} dana).',
    aiss_saved_t_off: 'Administrator ne čuva spremljene sesije nakon roka čuvanja transkripata ({days} dana).',
    aiss_continue: 'Nastavi', aiss_continue_t: 'Nastavi ovaj razgovor s WRM asistentom u terminalu na istom poslužitelju',
    aiss_continue_mcp_t: 'Nastavi u WRM asistentu: ono što je AI aplikacija napravila na poslužitelju postaje njegov raniji kontekst',
    aiss_open_live: 'Otvori live prikaz', aiss_open_chat: 'Idi na chat', aiss_rename: 'Preimenuj', aiss_export_md: 'Izvezi Markdown', aiss_export_json: 'Izvezi JSON',
    aiss_transcript: 'Transkript', aiss_empty_tr: 'U ovoj sesiji ništa nije zabilježeno.', aiss_conn_missing: 'Veza ove sesije više ne postoji ili nije tvoja.',
    aiss_tokens: '{in} ulaz / {out} izlaz', aiss_tools: '{n} poziva alata', aiss_appr: '{a} odobreno · {d} odbijeno', aiss_resumed: 'nastavak #{n}',
    aiss_by_app: 'AI aplikacija', aiss_untitled: 'Sesija bez naslova', aiss_hint: 'Svoje AI sesije vidiš samo ti. Administratori vide sve sesije u Admin → AI.',
    aiss_open: 'Otvori',
    ai_dec_done: '{by} je već {outcome} ovaj zahtjev {where}{time}.', ai_dec_where_app: 'u AI aplikaciji', ai_dec_where_wrm: 'u WRM-u',
    ai_dec_closed: 'AI je prestao čekati ovaj zahtjev i on je zatvoren. Zamoli AI da ga ponovi.',
    ai_dec_gone: 'Ovaj zahtjev više ne čeka (AI sesija je završila ili je istekao). Zamoli AI da ga ponovi.',
    ai_dec_o_approved: 'odobrio', ai_dec_o_denied: 'odbio',
  });

  const $ = id => document.getElementById(id);
  const tf = (k, v) => { let s = t(k); for (const n in (v || {})) s = s.split('{' + n + '}').join(String(v[n])); return s; };
  const KINDS = ['', 'assistant', 'logs', 'file', 'opencode', 'mcp', 'saved', 'live'];
  const ICON = {assistant: '✦', logs: '📜', file: '📝', opencode: '⬛', mcp: '🤖'};
  const st = {kind: '', q: '', list: [], more: false, loaded: false, meta: {}, qTimer: 0, seq: 0};
  try { st.kind = localStorage.getItem('wrm_aiss_kind') || ''; if (!KINDS.includes(st.kind)) st.kind = ''; } catch (_) {}

  async function api(method, url, body) {
    const r = await fetch(url, {method, headers: body ? {'Content-Type': 'application/json'} : {}, body: body ? JSON.stringify(body) : undefined});
    if (!r.ok) throw new Error(await apiError(r));
    return r.json().catch(() => ({}));
  }

  // decidedText explains why a decision on an approval did not apply (409 with "decided").
  function decidedText(info) {
    info = info || {};
    if (info.outcome === 'approved' || info.outcome === 'denied') {
      const where = String(info.via || '').startsWith('mcp') ? t('ai_dec_where_app') : t('ai_dec_where_wrm');
      return tf('ai_dec_done', {outcome: t('ai_dec_o_' + info.outcome), by: info.by || 'WRM', where, time: info.at ? ' · ' + fmtTime(info.at) : ''});
    }
    if (info.outcome === 'timeout' || info.outcome === 'cancelled') return t('ai_dec_closed');
    return t('ai_dec_gone');
  }
  // decide posts an approval decision; a request decided elsewhere is reported, not an error.
  async function decide(url, body) {
    const r = await fetch(url, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});
    if (r.ok) return {ok: true};
    const txt = await r.text().catch(() => '');
    let j = null;
    try { j = JSON.parse(txt); } catch (_) {}
    if (r.status === 409 && j && j.decided) { showToast(decidedText(j.decided), 'info', 7000); return {ok: false, decided: j.decided}; }
    showToast((j && j.error) || txt || ('HTTP ' + r.status), 'error', 6000);
    return {ok: false};
  }

  function root() { return $('pnl-aisess'); }
  function ensureDOM() {
    const el = root();
    if (!el || el.dataset.ready) return el;
    el.dataset.ready = '1';
    el.innerHTML = `<div class="pnl-pad aiss-tools"><input type="text" id="aiss-q" placeholder="${esc(t('aiss_search'))}" autocomplete="off">
        <div class="aiss-row"><select id="aiss-kind" title="${esc(t('aiss_panel'))}">${KINDS.map(k => `<option value="${k}">${esc(t('aiss_k_' + (k || 'all')))}</option>`).join('')}</select>
        <button class="btn-sm btn-sec" id="aiss-ref" title="${esc(t('aiss_refresh'))}">⟳</button></div></div>
      <div id="aiss-list"></div>`;
    $('aiss-kind').value = st.kind;
    $('aiss-kind').onchange = e => { st.kind = e.target.value; try { localStorage.setItem('wrm_aiss_kind', st.kind); } catch (_) {} load(); };
    $('aiss-q').oninput = e => { clearTimeout(st.qTimer); st.qTimer = setTimeout(() => { st.q = e.target.value.trim(); load(); }, 250); };
    $('aiss-ref').onclick = () => load();
    return el;
  }

  async function load(more) {
    if (typeof currentUser === 'undefined' || !currentUser) return;
    if (!ensureDOM()) return;
    const seq = ++st.seq;
    const qs = new URLSearchParams({view: 'all'});
    if (st.kind) qs.set('kind', st.kind);
    if (st.q) qs.set('q', st.q);
    if (more && st.list.length) qs.set('before', st.list[st.list.length - 1].id);
    let r;
    try { r = await api('GET', '/api/ai/sessions?' + qs); } catch (e) { if (seq === st.seq) $('aiss-list').innerHTML = `<div class="fm-error">${esc(e.message)}</div>`; return; }
    if (seq !== st.seq) return;
    st.list = more ? st.list.concat(r.sessions || []) : (r.sessions || []);
    st.more = !!r.more;
    st.meta = {keep: r.keep_saved, days: r.retention_days};
    st.loaded = true;
    render();
  }

  function titleOf(s) { return s.title || s.preview || t('aiss_untitled'); }
  function render() {
    const box = $('aiss-list');
    if (!box) return;
    if (!st.list.length) { box.innerHTML = `<div class="aiss-empty">${esc(st.q || st.kind ? t('aiss_none_q') : t('aiss_none'))}</div>`; return; }
    box.innerHTML = st.list.map(s => `<div class="sess-item aiss-item${s.live ? ' live' : ''}" data-id="${s.id}" title="${esc(titleOf(s))}">
        <div class="sess-iname"><span class="aiss-ico">${ICON[s.kind] || '✦'}</span><span class="aiss-tt">${esc(titleOf(s))}</span>
          <button class="aiss-star${s.saved ? ' on' : ''}" data-star="${s.id}" title="${esc(s.saved ? t('aiss_unsave') : t('aiss_save'))}">${s.saved ? '★' : '☆'}</button></div>
        <div class="sess-imeta">${s.live ? `<span class="aiss-dot">●</span> ` : ''}${esc(s.connection || '')} · ${esc(wrmFmt(s.started_at, 'datetime', {seconds: false}))}${s.kind === 'mcp' ? ' · ' + esc(s.provider || t('aiss_by_app')) : ''}</div></div>`).join('') +
      (st.more ? `<button class="btn-sec btn-sm aiss-more" id="aiss-more">${esc(t('aiss_more'))}</button>` : '') + `<div class="aiss-hint">${esc(t('aiss_hint'))}</div>`;
    box.querySelectorAll('.aiss-item').forEach(el => { el.onclick = e => { if (e.target.closest('[data-star]')) return; open(Number(el.dataset.id)); }; });
    box.querySelectorAll('[data-star]').forEach(b => { b.onclick = e => { e.stopPropagation(); const s = st.list.find(x => x.id === Number(b.dataset.star)); if (s) save(s.id, !s.saved); }; });
    const m = $('aiss-more'); if (m) m.onclick = () => load(true);
  }

  async function save(id, on) {
    try { await api('POST', `/api/ai/sessions/${id}/save`, {saved: on}); } catch (e) { showToast(e.message, 'error'); return; }
    const s = st.list.find(x => x.id === id); if (s) s.saved = on;
    render();
    const m = $('aiss-modal');
    if (m && m.classList.contains('open') && m.dataset.sid === String(id)) open(id);
    showToast(on ? t('aiss_saved') + ' · ' + tf(st.meta.keep ? 'aiss_saved_t' : 'aiss_saved_t_off', {days: st.meta.days}) : t('aiss_unsave'), on ? 'success' : 'info', on ? 5000 : 2500);
  }

  function modal(html, sid) {
    let m = $('aiss-modal');
    if (!m) { m = document.createElement('div'); m.id = 'aiss-modal'; m.className = 'modal-overlay'; m.style.zIndex = 9600; m.onclick = e => { if (e.target === m) close(); }; document.body.appendChild(m); }
    m.dataset.sid = String(sid || '');
    m.innerHTML = `<div class="modal aiss-modal">${html}</div>`;
    m.classList.add('open');
  }
  function close() { const m = $('aiss-modal'); if (m) m.classList.remove('open'); }

  function md(s) { return window.wrmAI && wrmAI.md ? wrmAI.md(s) : esc(s).replace(/\n/g, '<br>'); }

  async function open(id) {
    let v;
    try { v = await api('GET', '/api/ai/sessions/' + id); } catch (e) { showToast(e.message, 'error'); return; }
    const s = Object.assign({}, st.list.find(x => x.id === id) || {}, v.session || {});
    const kind = s.kind || (s.transport === 'mcp' ? 'mcp' : s.transport === 'opencode' ? 'opencode' : s.purpose || 'assistant');
    const saved = (st.list.find(x => x.id === id) || {}).saved;
    const icon = {user: '🧑', assistant: '✦', thinking: '💭', tool_call: '🛠', tool_result: '📄', approval: '⏳', decision: '⚖', mode: '⚙', error: '⚠', system: 'ℹ', end: '⏻'};
    const msgs = (v.messages || []).filter(m => m.kind !== 'system' || (v.messages || []).length < 3);
    const live = !!s.live;
    const acts = [
      live && kind === 'mcp' ? `<button class="btn-sm" onclick="wrmAISessions.live(${id})">👁 ${esc(t('aiss_open_live'))}</button>` : '',
      live && kind !== 'mcp' ? `<button class="btn-sm" onclick="wrmAISessions.goto(${id})">↗ ${esc(t('aiss_open_chat'))}</button>` : '',
      !live && s.conn_id ? `<button class="btn-sm" title="${esc(t(kind === 'mcp' || kind === 'opencode' ? 'aiss_continue_mcp_t' : 'aiss_continue_t'))}" onclick="wrmAISessions.resume(${id})">↪ ${esc(t('aiss_continue'))}</button>` : '',
      `<button class="btn-sec btn-sm" onclick="wrmAISessions.save(${id}, ${!saved})">${saved ? '★ ' + esc(t('aiss_unsave')) : '☆ ' + esc(t('aiss_save'))}</button>`,
      `<button class="btn-sec btn-sm" onclick="wrmAISessions.rename(${id})">✎ ${esc(t('aiss_rename'))}</button>`,
      `<a class="btn-sec btn-sm aiss-a" href="/api/ai/sessions/${id}/export?format=md" download>⬇ ${esc(t('aiss_export_md'))}</a>`,
      `<a class="btn-sec btn-sm aiss-a" href="/api/ai/sessions/${id}/export?format=json" download>⬇ ${esc(t('aiss_export_json'))}</a>`,
    ].filter(Boolean).join('');
    modal(`<div class="aiss-mh"><h3>${ICON[kind] || '✦'} ${esc(s.title || t('aiss_untitled'))}</h3><button class="icon-btn" onclick="wrmAISessions.close()">✕</button></div>
      <div class="aiss-sub">${live ? `<span class="pill ok">● ${esc(t('aiss_live'))}</span> ` : `<span class="pill">${esc(s.status || '')}</span> `}${esc(t('aiss_k_' + kind))} · ${esc(s.connection || '')} <span class="mono">(${esc(s.host || '')})</span> · ${esc(s.provider || '')}${s.model && s.model !== s.provider ? ' / ' + esc(s.model) : ''}
        <br>${esc(fmtTime(s.started_at))}${s.ended_at ? ' → ' + esc(fmtTime(s.ended_at)) : ''} · ${esc(tf('aiss_tools', {n: s.tool_calls || 0}))} · ${esc(tf('aiss_appr', {a: s.approvals || 0, d: s.denials || 0}))}${s.tokens_in || s.tokens_out ? ' · ' + esc(tf('aiss_tokens', {in: s.tokens_in || 0, out: s.tokens_out || 0})) : ''}${s.cost_usd ? ' · $' + Number(s.cost_usd).toFixed(4) : ''}${s.resumed_from ? ' · <a href="#" onclick="wrmAISessions.open(' + Number(s.resumed_from) + ');return false;">' + esc(tf('aiss_resumed', {n: s.resumed_from})) + '</a>' : ''}</div>
      <div class="aiss-acts">${acts}</div>
      <div class="aiss-tr">${msgs.length ? msgs.map(m => `<div class="ai-card"><div class="ai-ch"><span>${icon[m.kind] || '·'}</span><span class="k">${esc(m.kind)}</span><span class="ai-why">${esc(m.actor)} · ${esc(fmtTime(m.ts))}</span></div>
        ${m.kind === 'assistant' ? `<div class="ai-msg assistant">${md(m.content)}</div>` : m.kind === 'user' ? `<div class="ai-msg user">${esc(m.content)}</div>` : `<pre>${esc(m.content)}</pre>`}</div>`).join('') : `<div class="aiss-empty">${esc(t('aiss_empty_tr'))}</div>`}</div>`, id);
  }

  async function rename(id) {
    const s = st.list.find(x => x.id === id) || {};
    const v = await uiPrompt(t('aiss_rename'), s.title || '');
    if (v == null || !v.trim()) return;
    try { await api('POST', `/api/ai/sessions/${id}/title`, {title: v}); } catch (e) { showToast(e.message, 'error'); return; }
    await load();
    if ($('aiss-modal') && $('aiss-modal').classList.contains('open')) open(id);
  }

  function live(id) {
    close();
    (typeof loadMCP === 'function' ? loadMCP() : Promise.resolve()).then(() => window.wrmMCP && wrmMCP.watch(id, true)).catch(() => {});
  }
  function goto(id) {
    close();
    if (!(typeof loadAI === 'function')) return;
    loadAI().then(() => { if (!(wrmAI.focusSession && wrmAI.focusSession(id))) open(id); }).catch(() => {});
  }

  // resume continues an ended session in the WRM assistant of a terminal on its server:
  // an open terminal on that connection is reused, otherwise a new one is opened.
  async function resume(id) {
    const s = st.list.find(x => x.id === id) || (await api('GET', '/api/ai/sessions/' + id).then(v => v.session).catch(() => null));
    if (!s) return;
    const c = (typeof conns !== 'undefined' ? conns : []).find(x => x.id === s.conn_id);
    if (!c) { showToast(t('aiss_conn_missing'), 'warning', 6000); return; }
    close();
    let wo = winObjects.find(w => w.connId === c.id && w.mode === 'ssh' && !w.shareToken && !(window.wrmAI && wrmAI.busyIn && wrmAI.busyIn(w.id)));
    if (!wo) { wo = addWindow(); wo.connId = c.id; setWinMode(wo.id, 'ssh'); }
    if (typeof focusWindow === 'function') focusWindow(wo.id);
    try { await loadAI(); } catch (_) { showToast(t('ai_load_failed'), 'error'); return; }
    await wrmAI.openPanel(wo.id);
    await wrmAI.resume(wo.id, id);
    load();
  }

  const css = `
#pnl-aisess { display: flex; flex-direction: column; min-height: 0; height: 100%; }
.aiss-tools { display: flex; flex-direction: column; gap: 4px; }
.aiss-tools input { width: 100%; box-sizing: border-box; font-size: 11.5px; padding: 5px 7px; }
.aiss-row { display: flex; gap: 4px; align-items: center; } .aiss-row select { flex: 1; min-width: 0; font-size: 11px; padding: 3px 4px; }
#aiss-list { flex: 1; overflow-y: auto; padding: 5px; min-height: 0; }
.aiss-item .aiss-ico { flex: 0 0 auto; }
.aiss-item .aiss-tt { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.aiss-star { background: none; border: 0; padding: 0 2px; color: var(--text3); cursor: pointer; font-size: 13px; line-height: 1; flex: 0 0 auto; }
.aiss-star.on { color: var(--yellow, #f59e0b); }
.aiss-item.live { border-left: 2px solid var(--green, #22c55e); }
.aiss-dot { color: var(--green, #22c55e); }
.aiss-empty, .aiss-hint { font-size: 11px; color: var(--text3); padding: 8px 6px; line-height: 1.45; }
.aiss-more { display: block; margin: 6px auto; }
.aiss-modal { width: 820px; max-width: calc(100vw - 24px); max-height: calc(100vh - 32px); display: flex; flex-direction: column; }
.aiss-mh { display: flex; align-items: center; gap: 10px; } .aiss-mh h3 { margin: 0; flex: 1; min-width: 0; overflow-wrap: anywhere; }
.aiss-sub { font-size: 11.5px; color: var(--text2); margin: 6px 0; line-height: 1.6; }
.aiss-acts { display: flex; gap: 6px; flex-wrap: wrap; margin: 4px 0 10px; }
.aiss-a { text-decoration: none; display: inline-flex; align-items: center; }
.aiss-tr { overflow-y: auto; min-height: 0; flex: 1; display: flex; flex-direction: column; gap: 6px; }
.aiss-tr > * { flex-shrink: 0; }
.aiss-tr pre, .aiss-tr .ai-card pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 320px; overflow: auto; font: 11.5px/1.45 'JetBrains Mono', monospace; margin: 4px 0 0; }
.aiss-tr .ai-card { border: 1px solid var(--border2); border-radius: 9px; background: var(--bg); padding: 7px 8px; font-size: 12px; }
.aiss-tr .ai-ch { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; } .aiss-tr .ai-ch .k { font-weight: 700; font-size: 11px; color: var(--text2); text-transform: uppercase; }
.aiss-tr .ai-why { font-size: 11px; color: var(--text3); margin-left: auto; }
.aiss-tr .ai-msg { border-radius: 9px; padding: 7px 9px; font-size: 12.5px; line-height: 1.5; overflow-wrap: anywhere; margin-top: 4px; }
.aiss-tr .ai-msg.user { background: var(--accent-d); white-space: pre-wrap; } .aiss-tr .ai-msg.assistant { background: var(--bg3); }
`;
  const style = document.createElement('style'); style.textContent = css; document.head.appendChild(style);

  let refreshT = 0;
  function refresh() { if (!st.loaded) return; clearTimeout(refreshT); refreshT = setTimeout(() => load(), 400); }
  function shown() { ensureDOM(); if (!st.loaded) load(); else render(); }

  window.wrmAISessions = {load, refresh, shown, open, close, save, rename, resume, live, goto, decidedText, decide, langChanged: () => { const el = root(); if (el) { delete el.dataset.ready; el.innerHTML = ''; } if (st.loaded) { ensureDOM(); render(); } }};
})();
