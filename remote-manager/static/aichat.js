/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

/* AI chats (v13.5.0): the log assistant of the log viewer and the file
 * assistant of the editor. A compact chat bound to one connection and one
 * purpose: it lists the earlier chats of that purpose with their title and
 * date, continues an ended one, shows the work log (thinking, tool calls and
 * results) and asks for approvals like the terminal panel. The server decides
 * which tools a purpose may use (ai_chats.go). Needs static/ai.js (styles and
 * helpers), which it loads. */
Object.assign(LANGS.en, {
  ch_logs_title: 'AI log assistant',
  ch_file_title: 'AI file assistant',
  ch_new: 'New chat',
  ch_chats: 'Chats',
  ch_no_chats: 'No earlier chats yet.',
  ch_open: 'Open',
  ch_live: 'running',
  ch_logs_intro: 'Ask about this server\'s logs: what failed, since when, how often, which services and operators are affected. The assistant searches all discovered logs on the server and only reads; it never changes anything.',
  ch_file_intro: 'Ask the assistant to review, explain or change the open file. Every change is shown as a diff and waits for your approval; the editor reloads the file afterwards.',
  ch_logs_ph: 'e.g. Why did demo-api fail last night?',
  ch_file_ph: 'e.g. Check this configuration for mistakes',
  ch_ask_selection: 'Ask about the selection',
  ch_reloaded: 'The assistant changed the file; the editor reloaded it.',
  ch_changed_dirty: 'The assistant changed the file on the server, but you have unsaved changes. Reload to see its version (your changes are lost), or keep editing and save over it.',
  ch_reload: 'Reload',
  ch_context: 'Context from the log viewer',
  ch_mode_ro: 'Read-only: the assistant only reads logs.',
  ch_disabled: 'The AI assistant is not available for you or for this connection.',
  ch_no_providers: 'No AI provider is set up yet. Add one in Settings → AI (or your own key).',
  ch_rename: 'Rename',
});
Object.assign(LANGS.hr, {
  ch_logs_title: 'AI asistent za logove',
  ch_file_title: 'AI asistent za file',
  ch_new: 'Novi chat',
  ch_chats: 'Chatovi',
  ch_no_chats: 'Još nema ranijih chatova.',
  ch_open: 'Otvori',
  ch_live: 'radi',
  ch_logs_intro: 'Pitajte o logovima ovog servera: što je palo, od kada, koliko često, koji su servisi i operateri pogođeni. Asistent pretražuje sve pronađene logove na serveru i samo čita; nikad ništa ne mijenja.',
  ch_file_intro: 'Zamolite asistenta da pregleda, objasni ili izmijeni otvoreni file. Svaka izmjena prikazuje se kao diff i čeka vaše odobrenje; editor zatim ponovno učita file.',
  ch_logs_ph: 'npr. Zašto je demo-api pao sinoć?',
  ch_file_ph: 'npr. Provjeri ima li grešaka u ovoj konfiguraciji',
  ch_ask_selection: 'Pitaj o označenom',
  ch_reloaded: 'Asistent je izmijenio file; editor ga je ponovno učitao.',
  ch_changed_dirty: 'Asistent je izmijenio file na serveru, a vi imate nespremljene izmjene. Ponovno učitajte da vidite njegovu verziju (vaše izmjene se gube) ili nastavite uređivati i spremite preko nje.',
  ch_reload: 'Učitaj ponovno',
  ch_context: 'Kontekst iz preglednika logova',
  ch_mode_ro: 'Samo čitanje: asistent samo čita logove.',
  ch_disabled: 'AI asistent nije dostupan za vas ili za ovu konekciju.',
  ch_no_providers: 'Još nema postavljenog AI pružatelja. Dodajte ga u Postavke → AI (ili vlastiti ključ).',
  ch_rename: 'Preimenuj',
});

(function () {
  const $ = id => document.getElementById(id);
  const tf = (k, vars) => { let s = t(k); for (const [a, b] of Object.entries(vars || {})) s = s.split('{' + a + '}').join(String(b)); return s; };
  const C = {};
  let seq = 0;
  const EVENTS = ['session', 'user', 'assistant_start', 'thinking', 'text', 'assistant_done', 'tool_call', 'approval_required', 'approval_resolved', 'tool_result', 'usage', 'busy', 'error', 'notice', 'ended', 'title', 'history', 'mode'];

  const style = document.createElement('style');
  style.textContent = `
.wch{display:flex;flex-direction:column;min-height:0;height:100%;background:var(--bg2);color:var(--text)}
.wch-head{display:flex;gap:6px;align-items:center;padding:6px 8px;border-bottom:1px solid var(--border);flex:none}
.wch-head b{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.wch-body{flex:1;min-height:0;overflow:auto;padding:8px;display:flex;flex-direction:column;gap:6px}
.wch-foot{flex:none;display:flex;gap:6px;padding:6px 8px;border-top:1px solid var(--border);align-items:flex-end}
.wch-foot textarea{flex:1;min-height:38px;max-height:180px;resize:vertical;background:var(--bg3);color:var(--text);border:1px solid var(--border2);border-radius:6px;padding:6px;font:inherit}
.wch-list button{display:block;width:100%;text-align:left;margin:3px 0}
.wch-list .sub{font-size:11px;color:var(--text3)}
.wch .ai-card{border:1px solid var(--border2);border-radius:8px;padding:6px 8px;background:var(--bg)}
.wch .ai-card pre{max-height:220px;overflow:auto}
.wch .wch-wl{font-size:11px;color:var(--text2);border:1px solid var(--border);border-radius:8px;padding:5px 8px;background:var(--bg)}
.wch .wch-wl div{white-space:pre-wrap;overflow-wrap:anywhere}
.wch-setup{display:flex;flex-direction:column;gap:6px}
.wch-setup select{width:100%}
`;
  document.head.appendChild(style);

  async function api(method, url, body) {
    const r = await fetch(url, {method, headers: body ? {'Content-Type': 'application/json'} : {}, body: body ? JSON.stringify(body) : undefined});
    if (!r.ok) throw new Error(await apiError(r));
    return r.json().catch(() => ({}));
  }
  const fmt = v => (typeof wrmFmt === 'function' ? wrmFmt(v, 'datetime', {seconds: false}) : String(v || ''));

  // mount(el, opts): opts = {connId, purpose: 'logs'|'file', filePath, prefill, qs, onFileChanged(path)}
  async function mount(el, opts) {
    await loadAI();
    const id = 'c' + (++seq);
    const st = C[id] = {id, el, opts, sid: 0, es: null, seq: 0, cards: {}, pending: {}, cur: null, think: null, view: null};
    el.innerHTML = `<div class="wch" id="wch-${id}"><div class="wch-head"><b id="wch-t-${id}">${esc(t(opts.purpose === 'file' ? 'ch_file_title' : 'ch_logs_title'))}</b>
      <button class="btn-sec btn-sm" onclick="wrmChat.list('${id}')" title="${esc(t('ch_chats'))}">🕘</button>
      <button class="btn-sec btn-sm" onclick="wrmChat.fresh('${id}')" title="${esc(t('ch_new'))}">＋</button></div>
      <div class="wch-body" id="wch-b-${id}"></div>
      <div class="wch-foot" id="wch-f-${id}" style="display:none"><textarea id="wch-i-${id}" rows="2" placeholder="${esc(t(opts.purpose === 'file' ? 'ch_file_ph' : 'ch_logs_ph'))}" onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();wrmChat.send('${id}')}"></textarea>
      <button id="wch-s-${id}" onclick="wrmChat.send('${id}')">${esc(t('ai_send'))}</button><button id="wch-x-${id}" class="btn-sec" style="display:none" onclick="wrmChat.stop('${id}')">■</button></div></div>`;
    await setup(id);
    return id;
  }
  function unmount(id) { const st = C[id]; if (!st) return; st.es?.close(); delete C[id]; }

  async function chats(st) {
    return api('GET', `/api/ai/sessions?purpose=${st.opts.purpose}&conn_id=${st.opts.connId}`).catch(() => []);
  }
  function listHTML(id, list) {
    return `<div class="wch-list">${list.length ? list.slice(0, 30).map(s => `<button class="btn-sec btn-sm" onclick="wrmChat.${s.live ? 'attach' : 'resume'}('${id}', ${s.id})">
      <div>${esc(s.title || '#' + s.id)}${s.live ? ` <span class="pill ok">${esc(t('ch_live'))}</span>` : ''}</div>
      <div class="sub">${esc(fmt(s.started_at))}${s.file_path ? ' · ' + esc(s.file_path) : ''} · ${esc(s.model)}${s.live ? '' : ' · ↪ ' + esc(t('ai_continue_chat'))}</div></button>`).join('') : `<div class="muted-text">${esc(t('ch_no_chats'))}</div>`}</div>`;
  }
  async function setup(id) {
    const st = C[id]; if (!st) return;
    const body = $('wch-b-' + id);
    $('wch-f-' + id).style.display = 'none';
    body.innerHTML = `<div class="fm-loading">${esc(t('loading'))}</div>`;
    let cfg, pol;
    try { cfg = await wrmAI.loadCfg(); pol = await api('GET', `/api/ai/connections/${st.opts.connId}${st.opts.qs || ''}`); }
    catch (e) { body.innerHTML = `<div class="hint-box warn">${esc(e.message)}</div>`; return; }
    if (!cfg.enabled || !(pol.allowed_modes || []).length) { body.innerHTML = `<div class="hint-box warn">${esc(t('ch_disabled'))}${cfg.reason ? '<div>' + esc(cfg.reason) + '</div>' : ''}</div>`; return; }
    const provs = cfg.providers || [];
    if (!provs.length) { body.innerHTML = `<div class="hint-box">${esc(t('ch_no_providers'))}</div>`; return; }
    st.pol = pol;
    const list = (await chats(st)).filter(s => st.opts.purpose !== 'file' || !st.opts.filePath || s.file_path === st.opts.filePath || s.live);
    const modes = st.opts.purpose === 'logs' ? ['read_only'] : pol.allowed_modes.filter(m => m !== 'auto');
    body.innerHTML = `<div class="wch-setup"><div class="ai-see">${esc(t(st.opts.purpose === 'file' ? 'ch_file_intro' : 'ch_logs_intro'))}</div>
      <label>${esc(t('ai_provider'))}<select id="wch-p-${id}" onchange="wrmChat.models('${id}')">${provs.map(p => `<option value="${p.id}">${esc(p.name)}${p.scope === 'user' ? ' (🔑)' : ''}</option>`).join('')}</select></label>
      <label>${esc(t('ai_model'))}<select id="wch-m-${id}"></select></label>
      ${st.opts.purpose === 'file' ? `<label>${esc(t('ai_mode'))}<select id="wch-mo-${id}">${modes.map(m => `<option value="${m}" ${m === (modes.includes('ask') ? 'ask' : pol.default_mode) ? 'selected' : ''}>${esc(t('ai_mode_' + m))}</option>`).join('')}</select></label>` : `<div class="ai-see">🔒 ${esc(t('ch_mode_ro'))}</div>`}
      <button onclick="wrmChat.start('${id}')">✦ ${esc(t('ch_new'))}</button>
      ${list.length ? `<div class="ai-see" style="margin-top:6px">${esc(t('ch_chats'))}</div>${listHTML(id, list)}` : ''}</div>`;
    models(id);
  }
  function models(id) {
    const cfg = window.wrmAI && wrmAI._cfg;
    const sel = $('wch-m-' + id), p = $('wch-p-' + id);
    if (!sel || !p) return;
    wrmAI.loadCfg().then(c => {
      const prov = (c.providers || []).find(x => String(x.id) === p.value) || {};
      const list = prov.models || [];
      sel.innerHTML = list.map(m => `<option ${m === prov.default_model ? 'selected' : ''}>${esc(m)}</option>`).join('');
    });
    return cfg;
  }
  async function list(id) {
    const st = C[id]; if (!st) return;
    const all = await chats(st);
    const body = $('wch-b-' + id);
    body.insertAdjacentHTML('afterbegin', `<details open class="wch-wl"><summary>🕘 ${esc(t('ch_chats'))}</summary>${listHTML(id, all)}</details>`);
    body.scrollTop = 0;
  }
  function fresh(id) { const st = C[id]; if (!st) return; st.es?.close(); Object.assign(st, {sid: 0, es: null, view: null}); setup(id); }
  async function start(id) {
    const st = C[id]; if (!st) return;
    const b = {conn_id: st.opts.connId, provider_id: Number($('wch-p-' + id).value), model: $('wch-m-' + id).value, purpose: st.opts.purpose,
      mode: st.opts.purpose === 'file' ? ($('wch-mo-' + id)?.value || 'ask') : 'read_only'};
    if (st.opts.purpose === 'file') b.file_path = st.opts.filePath;
    try { const v = await api('POST', '/api/ai/sessions' + (st.opts.qs || ''), b); attach(id, v.id); }
    catch (e) { showToast(e.message, 'error', 6000); }
  }
  async function resume(id, sid) {
    const st = C[id]; if (!st) return;
    try { const v = await api('POST', '/api/ai/sessions' + (st.opts.qs || ''), {resume_from: sid, conn_id: st.opts.connId, file_path: st.opts.filePath || ''}); attach(id, v.id); }
    catch (e) { showToast(e.message, 'error', 6000); }
  }
  function attach(id, sid) {
    const st = C[id]; if (!st) return;
    st.es?.close();
    Object.assign(st, {sid, seq: 0, cards: {}, pending: {}, cur: null, think: null, ended: false, wl: null});
    $('wch-b-' + id).innerHTML = '';
    $('wch-f-' + id).style.display = '';
    const inp = $('wch-i-' + id);
    if (st.opts.prefill && !inp.value) { inp.value = st.opts.prefill; st.opts.prefill = ''; }
    const es = new EventSource(`/api/ai/sessions/${sid}/events?since=0`);
    st.es = es;
    EVENTS.forEach(ty => es.addEventListener(ty, ev => { try { onEvent(id, JSON.parse(ev.data)); } catch (e) { console.warn(e); } }));
    es.onerror = () => { if (st.ended) es.close(); };
    setTimeout(() => inp.focus(), 50);
  }
  function add(id, html) {
    const b = $('wch-b-' + id); if (!b) return null;
    const near = b.scrollHeight - b.scrollTop - b.clientHeight < 160;
    b.insertAdjacentHTML('beforeend', html);
    if (near) b.scrollTop = b.scrollHeight;
    return b.lastElementChild;
  }
  function wl(id) {
    const st = C[id];
    if (!st.wl || !st.wl.isConnected) st.wl = add(id, `<details class="wch-wl" open><summary>⏳ ${esc(t('ai_worklog'))}</summary></details>`);
    return st.wl;
  }
  function step(id, text) {
    const box = wl(id);
    box.insertAdjacentHTML('beforeend', `<div>${esc(text)}</div>`);
    const b = $('wch-b-' + id); if (b) b.scrollTop = b.scrollHeight;
    return box.lastElementChild;
  }
  function wlDone(id) { const st = C[id]; if (st.wl) { st.wl.open = false; st.wl.querySelector('summary').textContent = '✓ ' + t('ai_worklog'); st.wl = null; } }
  function onEvent(id, ev) {
    const st = C[id];
    if (!st || ev.seq <= st.seq) return;
    st.seq = ev.seq;
    const d = ev.data || {};
    switch (ev.type) {
      case 'session': st.view = d; if (d.title) $('wch-t-' + id).textContent = d.title; (d.pending || []).forEach(a => approval(id, a)); break;
      case 'title': $('wch-t-' + id).textContent = d.title; break;
      case 'history':
        add(id, `<details class="wch-wl"><summary>↪ ${esc(tf('ai_resumed_from', {n: d.from}))}</summary>${(d.messages || []).filter(m => m.kind === 'user' || m.kind === 'assistant')
          .map(m => `<div class="ai-msg ${m.kind}" style="opacity:.75;margin:4px 0">${m.kind === 'assistant' ? wrmAI.md(m.text) : esc(m.text)}</div>`).join('')}</details>`);
        break;
      case 'user': wlDone(id); add(id, `<div class="ai-msg user">${esc(d.text)}</div>`); break;
      case 'assistant_start': st.cur = null; st.think = null; wl(id); break;
      case 'thinking':
        if (!st.think) { st.think = step(id, '💭 '); st.thinkText = ''; }
        st.thinkText += d.delta || '';
        st.think.textContent = '💭 ' + st.thinkText.slice(-600);
        break;
      case 'text':
        if (!st.cur) { st.cur = add(id, `<div class="ai-msg assistant"></div>`); st.curText = ''; }
        st.curText += d.delta; st.cur.textContent = st.curText; break;
      case 'assistant_done':
        if (d.text) { if (!st.cur) st.cur = add(id, `<div class="ai-msg assistant"></div>`); st.cur.innerHTML = wrmAI.md(d.text); }
        else if (st.cur) st.cur.remove();
        if (d.text && d.stop === 'tool_use') step(id, '💬 ' + String(d.text).replace(/\s+/g, ' ').slice(0, 240));
        st.cur = null; st.think = null;
        if (d.stop !== 'tool_use') wlDone(id);
        break;
      case 'tool_call': {
        step(id, '🛠 ' + (d.tool || '') + (d.display ? ' · ' + d.display : ''));
        if (st.cards[d.call_id]) break;
        st.cards[d.call_id] = add(id, `<div class="ai-card"><div class="ai-ch"><span class="ai-tool">${esc(d.tool)}</span></div><div class="ai-cmd">${esc(d.display || '')}</div>${d.reason ? `<div class="ai-why">${esc(d.reason)}</div>` : ''}${d.diff && d.decision !== 'approve' ? wrmAI.diffHTML(d.diff) : ''}<div class="ai-appr"></div><div class="ai-res"></div></div>`);
        break;
      }
      case 'approval_required': approval(id, d.approval || d); break;
      case 'approval_resolved': {
        if (d.approval_id && window.wrmAiAlertDone) wrmAiAlertDone('approval-' + d.approval_id);
        const card = st.cards[d.call_id];
        if (card) card.querySelector('.ai-appr').innerHTML = `<span class="pill ${d.outcome === 'approved' ? 'ok' : 'bad'}">${esc(t('ai_o_' + d.outcome))}</span>`;
        break;
      }
      case 'tool_result': {
        step(id, (d.error ? '✗ ' : d.exit_code === 0 ? '✓ ' : '⚠ ') + (d.tool || '') + (d.error ? ' · ' + String(d.error).slice(0, 160) : ''));
        const card = st.cards[d.call_id];
        const out = d.error ? `<div class="ai-why" style="color:var(--red)">${esc(d.error)}</div>` : d.output != null ? `<details><summary>${esc(t('ai_output'))} (${String(d.output).length} B)</summary><pre>${esc(d.output)}</pre></details>` : '';
        if (card) card.querySelector('.ai-res').innerHTML = out;
        if (!d.error && d.exit_code === 0 && ['edit_file', 'write_file'].includes(d.tool) && st.opts.onFileChanged) st.opts.onFileChanged();
        break;
      }
      case 'busy':
        $('wch-s-' + id).style.display = d.busy ? 'none' : '';
        $('wch-x-' + id).style.display = d.busy ? '' : 'none';
        if (!d.busy) wlDone(id);
        break;
      case 'error': add(id, `<div class="ai-note err">⚠ ${esc(d.error)}</div>`); break;
      case 'notice': add(id, `<div class="ai-note">${esc(t('ai_n_' + d.text))}</div>`); break;
      case 'mode': add(id, `<div class="ai-note">${esc(tf('ai_mode_changed', {mode: t('ai_mode_' + d.mode)}))}</div>`); break;
      case 'ended':
        st.ended = true; st.es?.close(); wlDone(id);
        $('wch-f-' + id).style.display = 'none';
        add(id, `<div class="ai-note err">⏻ ${esc(t('ai_ended'))}: ${esc(wrmAI.reasonText(d.reason))}</div><div style="display:flex;gap:6px;justify-content:center"><button class="btn-sm" onclick="wrmChat.resume('${id}', ${st.sid})">↪ ${esc(t('ai_continue_chat'))}</button><button class="btn-sec btn-sm" onclick="wrmChat.fresh('${id}')">＋ ${esc(t('ch_new'))}</button></div>`);
        break;
    }
  }
  function approval(id, a) {
    const st = C[id];
    if (!a || !a.id || st.pending[a.id]) return;
    st.pending[a.id] = a;
    let card = st.cards[a.call_id];
    if (!card) card = st.cards[a.call_id] = add(id, `<div class="ai-card"><div class="ai-ch"><span class="ai-tool">${esc(a.tool || '')}</span></div><div class="ai-appr"></div><div class="ai-res"></div></div>`);
    card.querySelector('.ai-appr').innerHTML = `<div class="ai-why"><b>⏳ ${esc(t('ai_wait_approval'))}</b></div>${a.path ? wrmAI.diffHTML(a.diff) : `<pre class="ai-cmd">${esc(a.command || '')}</pre>`}
      <div class="ai-acts"><button class="btn-sm" onclick="wrmChat.decide('${id}','${esc(a.id)}',true)">✓ ${esc(t('ai_approve'))}</button><button class="btn-sec btn-sm" onclick="wrmChat.decide('${id}','${esc(a.id)}',false)">✕ ${esc(t('ai_deny'))}</button></div>`;
    if (window.wrmAiAlert) wrmAiAlert('approval', (a.tool || '') + ' · ' + (a.command || a.path || ''), 'approval-' + a.id);
  }
  async function decide(id, aid, ok) {
    const st = C[id]; if (!st) return;
    const r = await wrmAISessions.decide(`/api/ai/sessions/${st.sid}/approvals/${aid}${st.opts.qs || ''}`, {decision: ok ? 'approve' : 'deny'});
    if (r.ok || r.decided) delete st.pending[aid];
  }
  async function send(id, text) {
    const st = C[id]; if (!st || !st.sid) return;
    const inp = $('wch-i-' + id);
    text = (text ?? inp.value).trim();
    if (!text) return;
    try { await api('POST', `/api/ai/sessions/${st.sid}/prompt${st.opts.qs || ''}`, {text}); inp.value = ''; }
    catch (e) { showToast(e.message, 'error', 6000); }
  }
  function stop(id) { const st = C[id]; if (st && st.sid) api('POST', `/api/ai/sessions/${st.sid}/stop${st.opts.qs || ''}`).catch(() => {}); }
  // prefill puts text into the input of a chat (a running one, or the next one)
  function prefill(id, text) { const st = C[id]; if (!st) return; const inp = $('wch-i-' + id); if (inp && st.sid) { inp.value = text; inp.focus(); } else st.opts.prefill = text; }

  // ── the log viewer's assistant: a panel docked on the right of Logs ──
  let logChat = null, logBox = null;
  async function openLogAI(ctx) {
    let box = logBox;
    const host = document.querySelector('#lv .lv-main');
    if (!host) return;
    if (!box || logChat?.conn !== ctx.connId) {
      box?.remove();
      if (logChat) unmount(logChat.id);
      box = document.createElement('div');
      box.id = 'lv-ai';
      logBox = box;
      box.style.cssText = 'flex:none;width:min(420px,92vw);border-left:1px solid var(--border);min-height:0;display:flex;flex-direction:column';
      host.appendChild(box);
      const parts = [];
      if (ctx.file) parts.push((ctx.find ? `In ${ctx.file}, about "${ctx.find}": ` : `About the log ${ctx.file}: `));
      else if (ctx.find) parts.push(`About "${ctx.find}" in the logs: `);
      const id = await mount(box, {connId: ctx.connId, purpose: 'logs', prefill: parts.join('')});
      logChat = {id, conn: ctx.connId};
    } else {
      box.style.display = box.style.display === 'none' ? 'flex' : 'none';
      if (ctx.file) prefill(logChat.id, ctx.find ? `In ${ctx.file}, about "${ctx.find}": ` : `About the log ${ctx.file}: `);
    }
  }
  // The log viewer re-renders its layout; put the panel back where it was.
  function restoreLogAI() {
    const box = logBox, host = document.querySelector('#lv .lv-main');
    if (box && host && box.parentElement !== host) host.appendChild(box);
  }
  function closeLogAI() { if (logChat) unmount(logChat.id); logBox?.remove(); logChat = null; logBox = null; }

  window.openLogAI = openLogAI;
  window.wrmChat = {mount, unmount, setup, models, list, fresh, start, resume, attach, send, stop, decide, prefill, restoreLogAI, closeLogAI};
})();
