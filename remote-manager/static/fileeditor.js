/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

/* File editor (v13.5.0): the editor of the file manager. Syntax highlighting
 * (Prism, behind a transparent textarea), line numbers, find and replace,
 * go to line, JSON check and format, a review of the changes before saving,
 * a check that nobody changed the file on the server since it was opened, and
 * an AI assistant panel (static/aichat.js, purpose "file") that reviews,
 * explains and edits the open file. Loaded on demand by openEditor(). */
Object.assign(LANGS.en, {
  fe_find: 'Find',
  fe_replace: 'Replace',
  fe_replace_with: 'Replace with…',
  fe_replace_one: 'Replace',
  fe_replace_all: 'Replace all',
  fe_replaced: 'Replaced: {n}',
  fe_no_match: 'No match',
  fe_goto: 'Go to line',
  fe_goto_prompt: 'Line number (1–{n})',
  fe_wrap: 'Wrap',
  fe_highlight: 'Highlight',
  fe_hl_off_big: 'Highlighting is off for files over 300 KB.',
  fe_json_ok: 'Valid JSON',
  fe_json_bad: 'Invalid JSON: {e}',
  fe_json_format: 'Format JSON',
  fe_json_check: 'Check JSON',
  fe_json_save_anyway: 'The file is not valid JSON ({e}). Save it anyway?',
  fe_changes: 'Changes',
  fe_changes_title: 'Your changes',
  fe_no_changes: 'No changes.',
  fe_review_save: 'Review before saving',
  fe_review_save_desc: 'Show the changes before every save.',
  fe_back: 'Back to editing',
  fe_save_these: 'Save these changes',
  fe_conflict: 'The file changed on the server after you opened it (someone else, a service or the AI assistant wrote it). Overwrite it with your version?',
  fe_overwrite: 'Overwrite',
  fe_show_server: 'Show differences',
  fe_server_vs_mine: 'Server version (−) and your version (+)',
  fe_reload: 'Reload from server',
  fe_reload_dirty: 'Reload the file from the server? Your unsaved changes are lost.',
  fe_reloaded: 'Reloaded from the server.',
  fe_ai: 'AI assistant',
  fe_ask_sel: 'Ask AI about the selection',
  fe_sel_prompt: 'About lines {a}–{b} of the file:',
  fe_ln_col: 'Ln {l}, Col {c}',
  fe_lang: 'Language',
  fe_plain: 'Plain text',
  fe_keys: 'Ctrl+S save · Ctrl+F find · Ctrl+H replace · Ctrl+G line · Esc close',
  fe_ai_changed_dirty: 'The assistant changed the file on the server, but you have unsaved changes.',
  fe_too_long_diff: 'The files differ in too many lines to show every change; showing the whole old and new text.',
});
Object.assign(LANGS.hr, {
  fe_find: 'Traži',
  fe_replace: 'Zamijeni',
  fe_replace_with: 'Zamijeni s…',
  fe_replace_one: 'Zamijeni',
  fe_replace_all: 'Zamijeni sve',
  fe_replaced: 'Zamijenjeno: {n}',
  fe_no_match: 'Nema pogodaka',
  fe_goto: 'Idi na liniju',
  fe_goto_prompt: 'Broj linije (1–{n})',
  fe_wrap: 'Prelamanje',
  fe_highlight: 'Isticanje',
  fe_hl_off_big: 'Isticanje je isključeno za fileove veće od 300 KB.',
  fe_json_ok: 'JSON je ispravan',
  fe_json_bad: 'Neispravan JSON: {e}',
  fe_json_format: 'Formatiraj JSON',
  fe_json_check: 'Provjeri JSON',
  fe_json_save_anyway: 'File nije ispravan JSON ({e}). Ipak spremiti?',
  fe_changes: 'Izmjene',
  fe_changes_title: 'Vaše izmjene',
  fe_no_changes: 'Nema izmjena.',
  fe_review_save: 'Pregled prije spremanja',
  fe_review_save_desc: 'Prikaži izmjene prije svakog spremanja.',
  fe_back: 'Natrag na uređivanje',
  fe_save_these: 'Spremi ove izmjene',
  fe_conflict: 'File se promijenio na serveru nakon što ste ga otvorili (netko drugi, neki servis ili AI asistent ga je zapisao). Prepisati ga vašom verzijom?',
  fe_overwrite: 'Prepiši',
  fe_show_server: 'Prikaži razlike',
  fe_server_vs_mine: 'Verzija na serveru (−) i vaša verzija (+)',
  fe_reload: 'Ponovno učitaj sa servera',
  fe_reload_dirty: 'Ponovno učitati file sa servera? Nespremljene izmjene će se izgubiti.',
  fe_reloaded: 'Ponovno učitano sa servera.',
  fe_ai: 'AI asistent',
  fe_ask_sel: 'Pitaj AI o označenom',
  fe_sel_prompt: 'O linijama {a}–{b} filea:',
  fe_ln_col: 'Lin {l}, Stup {c}',
  fe_lang: 'Jezik',
  fe_plain: 'Običan tekst',
  fe_keys: 'Ctrl+S spremi · Ctrl+F traži · Ctrl+H zamijeni · Ctrl+G linija · Esc zatvori',
  fe_ai_changed_dirty: 'Asistent je promijenio file na serveru, ali vi imate nespremljene izmjene.',
  fe_too_long_diff: 'Fileovi se razlikuju u previše linija da bi se prikazala svaka izmjena; prikazuje se cijeli stari i novi tekst.',
});

(function () {
  const $ = id => document.getElementById(id);
  const HL_MAX = 300 * 1024;
  const tf = (k, vars) => { let s = t(k); for (const [a, b] of Object.entries(vars || {})) s = s.split('{' + a + '}').join(String(b)); return s; };
  const lsGet = (k, d) => { try { const v = localStorage.getItem(k); return v == null ? d : v; } catch (_) { return d; } };
  const lsSet = (k, v) => { try { localStorage.setItem(k, v); } catch (_) {} };
  let E = null; // the open editor
  let styled = false;

  const LANG_BY_EXT = {
    sh: 'bash', bash: 'bash', zsh: 'bash', ksh: 'bash', env: 'bash', crontab: 'bash',
    json: 'json', webmanifest: 'json', yml: 'yaml', yaml: 'yaml', toml: 'toml', ini: 'ini', cfg: 'ini', service: 'ini', timer: 'ini', socket: 'ini', mount: 'ini', properties: 'properties',
    xml: 'markup', html: 'markup', htm: 'markup', svg: 'markup', md: 'markdown', css: 'css', scss: 'scss', less: 'less',
    js: 'javascript', mjs: 'javascript', cjs: 'javascript', ts: 'typescript', py: 'python', go: 'go', rb: 'ruby', php: 'php', sql: 'sql',
    c: 'c', h: 'c', cpp: 'cpp', hpp: 'cpp', cc: 'cpp', rs: 'rust', java: 'java', cs: 'csharp', ps1: 'powershell', psm1: 'powershell', bat: 'batch', cmd: 'batch',
    diff: 'diff', patch: 'diff',
  };
  function langOf(path) {
    const name = path.split('/').pop(), low = name.toLowerCase();
    if (/^dockerfile/.test(low) || low.endsWith('.dockerfile')) return 'docker';
    if (low === 'makefile') return 'makefile';
    if (/^\.(bashrc|profile|bash_profile|zshrc|bash_aliases)$/.test(low)) return 'bash';
    if (low === '.htaccess') return 'apacheconf';
    const ext = low.includes('.') ? low.split('.').pop() : '';
    if (ext === 'conf') return /nginx/.test(path) ? 'nginx' : /apache|httpd/.test(path) ? 'apacheconf' : 'ini';
    return LANG_BY_EXT[ext] || '';
  }
  function hasGrammar(lang) { return !!(lang && window.Prism && Prism.languages[lang]); }

  function style() {
    if (styled) return; styled = true;
    const s = document.createElement('style');
    s.textContent = `
#modal-editor .modal.fe { width:min(1500px,98vw); height:min(92vh,1000px); padding:12px; gap:8px; }
.fe-head { display:flex; align-items:center; gap:10px; min-width:0; }
.fe-head h3 { margin:0 !important; font-size:14px !important; flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-family:var(--font-mono); }
.fe-meta { font-size:11px; color:var(--text3); white-space:nowrap; }
.fe-tools { display:flex; flex-wrap:wrap; gap:6px; align-items:center; }
.fe-tools label { display:flex; gap:4px; align-items:center; font-size:12px; color:var(--text2); white-space:nowrap; }
.fe-tools select { width:auto; padding:3px 6px; font-size:12px; background:var(--bg2); color:var(--text); border:1px solid var(--border2); border-radius:6px; }
.fe-find { display:flex; flex-wrap:wrap; gap:6px; align-items:center; padding:6px; border:1px solid var(--border); border-radius:8px; background:var(--bg2); }
.fe-find input[type=text] { width:min(260px,60vw); padding:4px 8px; font-size:12px; font-family:var(--font-mono); background:var(--bg1); color:var(--text); border:1px solid var(--border2); border-radius:6px; }
.fe-find .cnt { font-size:11px; color:var(--text3); min-width:70px; }
.fe-body { flex:1; min-height:0; display:flex; gap:8px; }
.fe-main { flex:1; min-width:0; min-height:0; display:flex; flex-direction:column; position:relative; }
.fe-ed { flex:1; min-height:0; display:flex; border:1px solid var(--border2); border-radius:8px; background:var(--bg0); overflow:hidden; }
.fe-ed:focus-within { border-color:var(--accent-b); }
.fe-gut { flex:none; overflow:hidden; padding:12px 8px 12px 10px; text-align:right; color:#4d5566; user-select:none; border-right:1px solid #1d222c; background:#0d1016; }
.fe-gut > div { will-change:transform; }
.fe-gut span { display:block; }
.fe-gut span.cur { color:#aab2bd; }
.fe-area { position:relative; flex:1; min-width:0; overflow:hidden; }
.fe-ed, .fe-ed pre, .fe-ta, .fe-gut { font-family:var(--font-mono); font-size:13px; line-height:20px; tab-size:4; }
.fe-area pre { position:absolute; left:0; top:0; margin:0; padding:12px; white-space:pre; pointer-events:none; color:var(--text); background:none; border:0; min-width:100%; will-change:transform; }
.fe-area pre code { font:inherit; color:inherit; background:none; padding:0; white-space:inherit; }
.fe-marks { color:transparent !important; }
.fe-marks mark { background:rgba(229,192,123,.28); color:transparent; border-radius:2px; }
.fe-marks mark.cur { background:rgba(255,153,0,.55); outline:1px solid #ff9900; }
.fe-ta { position:absolute; inset:0; width:100%; height:100%; margin:0; padding:12px; border:0; outline:none; resize:none; background:transparent; color:transparent; caret-color:var(--text); white-space:pre; overflow:auto; user-select:text; }
.fe-ta.plain { color:var(--text); }
.fe-ta::selection { background:rgba(97,175,239,.35); color:transparent; }
.fe-ta.plain::selection { color:#fff; }
.fe.wrap .fe-area pre, .fe.wrap .fe-ta { white-space:pre-wrap; overflow-wrap:anywhere; min-width:0; width:100%; }
.fe.wrap .fe-gut { display:none; }
.fe-area .token.comment, .fe-area .token.prolog, .fe-area .token.doctype, .fe-area .token.cdata { color:#7f8c98; font-style:italic; }
.fe-area .token.punctuation { color:#aab2bd; }
.fe-area .token.property, .fe-area .token.tag, .fe-area .token.boolean, .fe-area .token.number, .fe-area .token.constant, .fe-area .token.symbol, .fe-area .token.deleted { color:#e06c75; }
.fe-area .token.selector, .fe-area .token.attr-name, .fe-area .token.string, .fe-area .token.char, .fe-area .token.builtin, .fe-area .token.inserted { color:#98c379; }
.fe-area .token.operator, .fe-area .token.entity, .fe-area .token.url { color:#56b6c2; }
.fe-area .token.atrule, .fe-area .token.attr-value, .fe-area .token.keyword { color:#c678dd; }
.fe-area .token.function, .fe-area .token.class-name { color:#61afef; }
.fe-area .token.regex, .fe-area .token.important, .fe-area .token.variable, .fe-area .token.section { color:#e5c07b; }
.fe-area .token.key { color:#e06c75; }
.fe-ai { flex:none; width:min(430px,40%); min-height:0; display:flex; flex-direction:column; border:1px solid var(--border); border-radius:8px; overflow:hidden; }
.fe-note { font-size:12px; padding:6px 10px; border-radius:6px; background:var(--bg2); border:1px solid var(--border); display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
.fe-note.warn { border-color:rgba(229,192,123,.6); }
.fe-diff { position:absolute; inset:0; z-index:3; display:flex; flex-direction:column; gap:8px; background:var(--bg1); }
.fe-diff-body { flex:1; min-height:0; overflow:auto; margin:0; padding:8px 0; background:var(--bg0); border:1px solid var(--border2); border-radius:8px; font-family:var(--font-mono); font-size:12px; line-height:18px; user-select:text; }
.fe-diff-body div { white-space:pre; padding:0 10px; }
.fe-diff-body .a { background:rgba(152,195,121,.16); color:#b5e0a0; }
.fe-diff-body .d { background:rgba(224,108,117,.16); color:#f0a0a7; }
.fe-diff-body .h { color:#61afef; background:rgba(97,175,239,.08); margin-top:6px; }
.fe-diff-body .c { color:#8b93a1; }
.fe-diff-body .n { display:inline-block; width:5ch; color:#4d5566; text-align:right; margin-right:1ch; }
.fe-foot { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
.fe-foot .keys { margin-right:auto; font-size:11px; color:var(--text3); }
@media (max-width: 760px) {
  #modal-editor .modal.fe { width:100vw; height:100dvh; max-height:none; border-radius:0; padding:8px; }
  .fe-body { flex-direction:column; }
  .fe-ai { width:auto; height:45%; }
  .fe-foot .keys, .fe-meta .lc { display:none; }
}`;
    document.head.appendChild(s);
  }

  function shell() {
    const m = document.querySelector('#modal-editor .modal');
    m.className = 'modal fe' + (lsGet('wrm_fe_wrap', '') ? ' wrap' : '');
    m.innerHTML = `<div class="fe-head"><h3 id="fe-title"></h3><span class="fe-meta" id="fe-meta"></span></div>
<div class="fe-tools">
 <button class="btn-sec btn-sm" onclick="wrmFileEditor.find(false)" title="Ctrl+F">🔍 ${esc(t('fe_find'))}</button>
 <button class="btn-sec btn-sm" id="fe-repl-btn" onclick="wrmFileEditor.find(true)" title="Ctrl+H">⇄ ${esc(t('fe_replace'))}</button>
 <button class="btn-sec btn-sm" onclick="wrmFileEditor.gotoLine()" title="Ctrl+G">↧ ${esc(t('fe_goto'))}</button>
 <label title="${esc(t('fe_lang'))}"><select id="fe-lang" onchange="wrmFileEditor.setLang(this.value)"></select></label>
 <label><input type="checkbox" id="fe-wrap" onchange="wrmFileEditor.setWrap(this.checked)"> ${esc(t('fe_wrap'))}</label>
 <label title="${esc(t('fe_review_save_desc'))}"><input type="checkbox" id="fe-review" onchange="wrmFileEditor.setReview(this.checked)"> ${esc(t('fe_review_save'))}</label>
 <span id="fe-json" style="display:contents"></span>
 <button class="btn-sec btn-sm" onclick="wrmFileEditor.showChanges()">± ${esc(t('fe_changes'))}</button>
 <button class="btn-sec btn-sm" onclick="wrmFileEditor.reload()">⟲ ${esc(t('fe_reload'))}</button>
 <button class="btn-sec btn-sm" id="fe-ai-btn" onclick="wrmFileEditor.toggleAI()">🤖 ${esc(t('fe_ai'))}</button>
 <button class="btn-sec btn-sm" id="fe-sel-btn" style="display:none" onclick="wrmFileEditor.askSelection()">💬 ${esc(t('fe_ask_sel'))}</button>
</div>
<div class="fe-find" id="fe-find" style="display:none">
 <input type="text" id="fe-q" placeholder="${esc(t('fe_find'))}…" oninput="wrmFileEditor.search()" onkeydown="wrmFileEditor.findKey(event)">
 <label><input type="checkbox" id="fe-re" onchange="wrmFileEditor.search()"> .*</label>
 <label><input type="checkbox" id="fe-case" onchange="wrmFileEditor.search()"> Aa</label>
 <button class="btn-sec btn-sm" onclick="wrmFileEditor.next(-1)" title="Shift+Enter">↑</button>
 <button class="btn-sec btn-sm" onclick="wrmFileEditor.next(1)" title="Enter">↓</button>
 <span class="cnt" id="fe-cnt"></span>
 <span id="fe-repl" style="display:contents">
  <input type="text" id="fe-r" placeholder="${esc(t('fe_replace_with'))}" onkeydown="wrmFileEditor.findKey(event, true)">
  <button class="btn-sec btn-sm" onclick="wrmFileEditor.replace(false)">${esc(t('fe_replace_one'))}</button>
  <button class="btn-sec btn-sm" onclick="wrmFileEditor.replace(true)">${esc(t('fe_replace_all'))}</button>
 </span>
 <button class="btn-sec btn-sm" style="margin-left:auto" onclick="wrmFileEditor.closeFind()">✕</button>
</div>
<div id="fe-notes"></div>
<div class="fe-body">
 <div class="fe-main">
  <div class="fe-ed"><div class="fe-gut" id="fe-gut"><div id="fe-gut-in"></div></div>
   <div class="fe-area"><pre class="fe-marks" id="fe-marks" aria-hidden="true"></pre><pre id="fe-hl" aria-hidden="true"><code></code></pre>
    <textarea class="fe-ta" id="editor-text" spellcheck="false" autocapitalize="off" autocomplete="off" wrap="off"></textarea></div></div>
 </div>
 <div class="fe-ai" id="fe-ai" style="display:none"></div>
</div>
<div class="fe-foot"><span class="keys">${esc(t('fe_keys'))}</span>
 <button class="btn-sec" onclick="closeEditor()">${esc(t('close'))}</button>
 <button class="btn-sec" id="editor-save-btn" onclick="wrmFileEditor.save(false)">${esc(t('save'))}</button>
 <button id="fe-save-close" onclick="wrmFileEditor.save(true)">${esc(t('save_close'))}</button></div>`;
    const ta = $('editor-text');
    ta.addEventListener('input', onInput);
    ta.addEventListener('scroll', sync);
    ta.addEventListener('keydown', onKey);
    ta.addEventListener('select', onCaret);
    ta.addEventListener('keyup', onCaret);
    ta.addEventListener('mouseup', onCaret);
    $('fe-wrap').checked = !!lsGet('wrm_fe_wrap', '');
    $('fe-review').checked = lsGet('wrm_fe_review', '1') === '1';
  }

  // ── open / load ──
  async function fetchText(wo, path) {
    const res = await fetch(`/api/remote/download?${connQS(wo)}&path=${encodeURIComponent(path)}&inline=1`);
    if (!res.ok) throw new Error(await apiError(res));
    return new Uint8Array(await res.arrayBuffer());
  }
  const decode = buf => new TextDecoder('utf-8').decode(buf);
  const norm = s => s.replace(/\r\n/g, '\n');

  async function open(winId, path, size) {
    const wo = winObjects.find(w => w.id === winId);
    if (!wo) return;
    if (size > 5 * 1048576) { showToast(t('fm_too_big_edit'), 'warning'); return; }
    showToast(t('loading') + ' ' + path.split('/').pop(), 'info', 1200);
    let buf;
    try { buf = await fetchText(wo, path); }
    catch (e) { showToast(t('fm_open_failed') + ': ' + e.message, 'error', 5000); return; }
    if (buf.subarray(0, 8000).includes(0) && !(await uiConfirm(t('fm_binary_warn'), {title: t('fm_edit')}))) return;
    if (E && !(await close())) return;
    const text = decode(buf);
    style(); shell();
    E = {winId, path, wo, crlf: text.includes('\r\n'), original: norm(text), readOnly: !winCan(wo, 'files_write'),
      lang: langOf(path), matches: [], cur: -1, hlTimer: 0, chat: null, aiNote: false};
    $('fe-title').textContent = path; $('fe-title').title = path;
    $('editor-save-btn').style.display = E.readOnly ? 'none' : '';
    $('fe-save-close').style.display = E.readOnly ? 'none' : '';
    $('fe-repl-btn').style.display = E.readOnly ? 'none' : '';
    const ta = $('editor-text');
    ta.readOnly = E.readOnly;
    ta.value = E.original;
    langSelect();
    $('modal-editor').classList.add('open');
    refresh(true);
    setTimeout(() => { ta.focus(); ta.setSelectionRange(0, 0); ta.scrollTop = 0; sync(); }, 30);
  }

  function langSelect() {
    const langs = [...new Set(Object.values(LANG_BY_EXT).concat(['docker', 'makefile', 'nginx', 'apacheconf']))].filter(hasGrammar).sort();
    $('fe-lang').innerHTML = `<option value="">${esc(t('fe_plain'))}</option>` + langs.map(l => `<option value="${l}"${l === E.lang ? ' selected' : ''}>${l}</option>`).join('');
  }
  function setLang(l) { if (!E) return; E.lang = l; refresh(true); }
  function setWrap(on) { lsSet('wrm_fe_wrap', on ? '1' : ''); document.querySelector('#modal-editor .modal').classList.toggle('fe-wrap-on', on); document.querySelector('#modal-editor .modal').classList.toggle('wrap', on); $('editor-text').setAttribute('wrap', on ? 'soft' : 'off'); sync(); }
  function setReview(on) { lsSet('wrm_fe_review', on ? '1' : '0'); }

  // ── rendering: highlight layer, match marks, gutter ──
  function onInput() { if (!E) return; refresh(false); }
  function refresh(now) {
    if (!E) return;
    meta(); gutter(); jsonTools();
    if (E.find) search(true);
    clearTimeout(E.hlTimer);
    const v = $('editor-text').value;
    const delay = now ? 0 : v.length > 100000 ? 250 : 60;
    E.hlTimer = setTimeout(highlight, delay);
  }
  function highlight() {
    if (!E) return;
    const ta = $('editor-text'), v = ta.value, code = $('fe-hl').firstChild;
    const on = hasGrammar(E.lang) && v.length <= HL_MAX;
    ta.classList.toggle('plain', !on);
    $('fe-hl').style.display = on ? '' : 'none';
    if (on) {
      try { code.innerHTML = Prism.highlight(v, Prism.languages[E.lang], E.lang) + '\n'; }
      catch (_) { code.textContent = v + '\n'; }
    } else code.textContent = '';
    note('hl', hasGrammar(E.lang) && v.length > HL_MAX ? t('fe_hl_off_big') : '');
    sync();
  }
  function marks() {
    const pre = $('fe-marks'), v = $('editor-text').value;
    if (!E.find || !E.matches.length) { pre.textContent = ''; return; }
    let html = '', at = 0;
    E.matches.forEach((m, i) => {
      html += esc(v.slice(at, m[0])) + `<mark${i === E.cur ? ' class="cur"' : ''}>${esc(v.slice(m[0], m[1])) || ' '}</mark>`;
      at = m[1];
    });
    pre.innerHTML = html + esc(v.slice(at)) + '\n';
  }
  function gutter() {
    const n = $('editor-text').value.split('\n').length;
    if (E.gutN === n) return;
    E.gutN = n;
    const parts = new Array(n);
    for (let i = 0; i < n; i++) parts[i] = `<span>${i + 1}</span>`;
    $('fe-gut-in').innerHTML = parts.join('') + '<span>&nbsp;</span>';
    $('fe-gut').style.width = (String(n).length + 2.5) + 'ch';
  }
  function sync() {
    const ta = $('editor-text'); if (!ta) return;
    const tr = `translate(${-ta.scrollLeft}px,${-ta.scrollTop}px)`;
    $('fe-hl').style.transform = tr; $('fe-marks').style.transform = tr;
    $('fe-gut-in').style.transform = `translateY(${-ta.scrollTop}px)`;
    // keep the layers as wide as the text area's content so long lines line up
    const w = ta.scrollWidth + 'px';
    $('fe-hl').style.width = w; $('fe-marks').style.width = w;
  }
  function caret() {
    const ta = $('editor-text'), before = ta.value.slice(0, ta.selectionStart);
    const line = before.split('\n').length;
    return {line, col: ta.selectionStart - before.lastIndexOf('\n')};
  }
  function onCaret() {
    if (!E) return;
    meta();
    const ta = $('editor-text');
    $('fe-sel-btn').style.display = E.chat && ta.selectionEnd > ta.selectionStart ? '' : 'none';
    const c = caret().line;
    if (E.curLine !== c) {
      const g = $('fe-gut-in');
      g.children[E.curLine - 1]?.classList.remove('cur');
      g.children[c - 1]?.classList.add('cur');
      E.curLine = c;
    }
  }
  function dirty() { return E && $('editor-text').value !== E.original; }
  function meta() {
    const v = $('editor-text').value, c = caret();
    $('fe-meta').innerHTML = `<span class="lc">${esc(tf('fe_ln_col', {l: c.line, c: c.col}))} · </span>${v.split('\n').length} ${esc(t('lines'))} · ${E.crlf ? 'CRLF' : 'LF'} · UTF-8${E.readOnly ? ' · 🔒' : ''}${dirty() ? ' · ● ' + esc(t('modified')) : ''}`;
  }
  function note(key, html, cls) {
    let el = $('fe-note-' + key);
    if (!html) { el?.remove(); return; }
    if (!el) { el = document.createElement('div'); el.id = 'fe-note-' + key; $('fe-notes').appendChild(el); }
    el.className = 'fe-note' + (cls ? ' ' + cls : '');
    el.innerHTML = html;
  }

  // ── JSON ──
  function isJSON() { return E.lang === 'json'; }
  function jsonTools() {
    $('fe-json').innerHTML = isJSON() ? `<button class="btn-sec btn-sm" onclick="wrmFileEditor.checkJSON()">✓ ${esc(t('fe_json_check'))}</button>${E.readOnly ? '' : `<button class="btn-sec btn-sm" onclick="wrmFileEditor.formatJSON()">{ } ${esc(t('fe_json_format'))}</button>`}` : '';
  }
  function jsonError(v) {
    try { JSON.parse(v); return ''; } catch (e) {
      const m = /position (\d+)/.exec(e.message);
      if (m) { const before = v.slice(0, +m[1]); return e.message + ' (' + tf('fe_ln_col', {l: before.split('\n').length, c: +m[1] - before.lastIndexOf('\n')}) + ')'; }
      return e.message;
    }
  }
  function checkJSON() {
    const err = jsonError($('editor-text').value);
    showToast(err ? tf('fe_json_bad', {e: err}) : t('fe_json_ok'), err ? 'error' : 'success', err ? 7000 : 2500);
  }
  function formatJSON() {
    const ta = $('editor-text'), err = jsonError(ta.value);
    if (err) { showToast(tf('fe_json_bad', {e: err}), 'error', 7000); return; }
    const indent = /\n\t/.test(ta.value) ? '\t' : (/\n( +)/.exec(ta.value)?.[1].length || 2);
    setText(JSON.stringify(JSON.parse(ta.value), null, indent) + (ta.value.endsWith('\n') ? '\n' : ''));
  }
  // setText replaces the whole text so that Ctrl+Z can undo it
  function setText(v) {
    const ta = $('editor-text');
    ta.focus(); ta.select();
    if (!document.execCommand('insertText', false, v)) ta.value = v;
    refresh(true);
  }

  // ── keys ──
  function onKey(e) {
    const ta = e.target, k = e.key.toLowerCase(), mod = e.ctrlKey || e.metaKey;
    if (mod && k === 's') { e.preventDefault(); save(false); }
    else if (mod && k === 'f') { e.preventDefault(); find(false); }
    else if (mod && k === 'h') { e.preventDefault(); find(true); }
    else if (mod && k === 'g') { e.preventDefault(); gotoLine(); }
    else if (e.key === 'F3') { e.preventDefault(); next(e.shiftKey ? -1 : 1); }
    else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); if (E.find) closeFind(); else close(); }
    else if (ta.readOnly) return;
    else if (e.key === 'Tab' && !mod && !e.altKey) { e.preventDefault(); indent(ta, e.shiftKey); }
    else if (e.key === 'Enter' && !mod && !e.altKey && !e.shiftKey) {
      const s = ta.selectionStart, line = ta.value.slice(ta.value.lastIndexOf('\n', s - 1) + 1, s);
      const ws = /^[ \t]*/.exec(line)[0];
      const lead = ws + (/[{[(:]\s*$/.test(line) && E.lang && E.lang !== 'markdown' ? (ws.includes('\t') ? '\t' : '  ') : '');
      if (lead) { e.preventDefault(); document.execCommand('insertText', false, '\n' + lead); }
    }
  }
  function indent(ta, out) {
    const v = ta.value, s = ta.selectionStart, en = ta.selectionEnd;
    if (s === en && !out) { document.execCommand('insertText', false, '\t'); return; }
    const a = v.lastIndexOf('\n', s - 1) + 1, b = en > s && v[en - 1] === '\n' ? en - 1 : en;
    const block = v.slice(a, b), unit = /^ +/m.test(block) && !/^\t/m.test(block) ? '  ' : '\t';
    const lines = block.split('\n');
    const changed = lines.map(l => out ? l.replace(unit === '\t' ? /^(\t| {1,4})/ : /^ {1,2}/, '') : unit + l).join('\n');
    ta.setSelectionRange(a, b);
    document.execCommand('insertText', false, changed);
    ta.setSelectionRange(a, a + changed.length);
  }

  // ── find and replace ──
  function find(repl) {
    if (!E) return;
    if (repl && E.readOnly) repl = false;
    E.find = true;
    $('fe-find').style.display = '';
    $('fe-repl').style.display = repl ? 'contents' : 'none';
    const ta = $('editor-text'), sel = ta.value.slice(ta.selectionStart, ta.selectionEnd);
    if (sel && !sel.includes('\n') && sel.length < 200) $('fe-q').value = sel;
    (repl && $('fe-q').value ? $('fe-r') : $('fe-q')).focus();
    $('fe-q').select();
    search();
  }
  function closeFind() { if (!E) return; E.find = false; $('fe-find').style.display = 'none'; E.matches = []; marks(); $('editor-text').focus(); }
  function pattern() {
    const q = $('fe-q').value;
    if (!q) return null;
    const flags = 'g' + ($('fe-case').checked ? '' : 'i') + 'm';
    try { return new RegExp($('fe-re').checked ? q : q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), flags); }
    catch (_) { return undefined; }
  }
  function search(keep) {
    if (!E || !E.find) return;
    const re = pattern(), v = $('editor-text').value;
    E.matches = [];
    if (re) {
      let m;
      while ((m = re.exec(v)) && E.matches.length < 20000) {
        E.matches.push([m.index, m.index + m[0].length]);
        if (!m[0].length) re.lastIndex++;
      }
    }
    if (!keep || E.cur >= E.matches.length) {
      const at = $('editor-text').selectionStart;
      E.cur = E.matches.findIndex(m => m[0] >= at);
      if (E.cur < 0) E.cur = E.matches.length ? 0 : -1;
    }
    count(re);
    marks();
    if (!keep && E.cur >= 0) reveal(E.matches[E.cur], false);
  }
  function count(re) {
    $('fe-cnt').textContent = re === undefined ? '⚠ regex' : !re ? '' : E.matches.length ? `${E.cur + 1} / ${E.matches.length}` : t('fe_no_match');
  }
  function next(dir) {
    if (!E) return;
    if (!E.find) { find(false); return; }
    if (!E.matches.length) return;
    E.cur = (E.cur + dir + E.matches.length) % E.matches.length;
    count(pattern()); marks();
    reveal(E.matches[E.cur], true);
  }
  // reveal selects a range and scrolls it into the middle of the editor
  function reveal(m, select) {
    const ta = $('editor-text');
    if (select) ta.setSelectionRange(m[0], m[1]);
    const line = ta.value.slice(0, m[0]).split('\n').length;
    if (!document.querySelector('#modal-editor .modal').classList.contains('wrap')) {
      ta.scrollTop = Math.max(0, (line - 1) * 20 - ta.clientHeight / 2 + 30);
      const col = m[0] - ta.value.lastIndexOf('\n', m[0] - 1) - 1;
      const x = col * 7.8;
      if (x < ta.scrollLeft || x > ta.scrollLeft + ta.clientWidth - 60) ta.scrollLeft = Math.max(0, x - ta.clientWidth / 3);
    } else { ta.blur(); ta.setSelectionRange(m[0], m[1]); ta.focus(); }
    sync(); onCaret();
  }
  function findKey(e, inRepl) {
    e.stopPropagation();
    if (e.key === 'Enter') { e.preventDefault(); if (inRepl) replace(e.ctrlKey || e.metaKey); else next(e.shiftKey ? -1 : 1); }
    else if (e.key === 'Escape') { e.preventDefault(); closeFind(); }
    else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save(false); }
  }
  function replace(all) {
    if (!E || E.readOnly) return;
    const re = pattern(); if (!re) return;
    const ta = $('editor-text'), withText = $('fe-r').value, rx = $('fe-re').checked;
    if (all) {
      const n = E.matches.length; if (!n) return;
      setText(ta.value.replace(re, rx ? withText : () => withText));
      showToast(tf('fe_replaced', {n}), 'success', 2000);
      return;
    }
    if (E.cur < 0) return;
    const [a, b] = E.matches[E.cur];
    const one = new RegExp(re.source, re.flags.replace('g', ''));
    const out = rx ? ta.value.slice(a, b).replace(one, withText) : withText;
    ta.focus(); ta.setSelectionRange(a, b);
    document.execCommand('insertText', false, out);
    refresh(false);
    search(true);
    E.cur = E.matches.findIndex(m => m[0] >= a + out.length);
    if (E.cur < 0) E.cur = E.matches.length ? 0 : -1;
    count(re); marks();
    if (E.cur >= 0) reveal(E.matches[E.cur], false);
    $('fe-r').focus();
  }
  async function gotoLine() {
    if (!E) return;
    const ta = $('editor-text'), n = ta.value.split('\n').length;
    const v = await uiPrompt(tf('fe_goto_prompt', {n}), String(caret().line), {title: t('fe_goto')});
    const line = Math.min(n, Math.max(1, parseInt(v, 10) || 0));
    if (!v || !line) { ta.focus(); return; }
    const lines = ta.value.split('\n');
    let pos = 0;
    for (let i = 0; i < line - 1; i++) pos += lines[i].length + 1;
    ta.focus();
    reveal([pos, pos], true);
  }

  // ── diff ──
  // lineDiff returns [{op:' '|'-'|'+', text, a, b}] for two texts (LCS on lines
  // after trimming the common start and end); null when it would be too big.
  function lineDiff(oldText, newText) {
    const A = oldText.split('\n'), B = newText.split('\n');
    let s = 0;
    while (s < A.length && s < B.length && A[s] === B[s]) s++;
    let ea = A.length, eb = B.length;
    while (ea > s && eb > s && A[ea - 1] === B[eb - 1]) { ea--; eb--; }
    const a = A.slice(s, ea), b = B.slice(s, eb), n = a.length, m = b.length;
    const out = [];
    for (let i = 0; i < s; i++) out.push({op: ' ', text: A[i], a: i + 1, b: i + 1});
    if (n * m > 4e6) return null;
    const L = Array.from({length: n + 1}, () => new Uint32Array(m + 1));
    for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) L[i][j] = a[i] === b[j] ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1]);
    let i = 0, j = 0;
    while (i < n || j < m) {
      if (i < n && j < m && a[i] === b[j]) { out.push({op: ' ', text: a[i], a: s + i + 1, b: s + j + 1}); i++; j++; }
      else if (i < n && (j >= m || L[i + 1][j] >= L[i][j + 1])) { out.push({op: '-', text: a[i], a: s + i + 1}); i++; }
      else { out.push({op: '+', text: b[j], b: s + j + 1}); j++; }
    }
    for (let k = ea; k < A.length; k++) out.push({op: ' ', text: A[k], a: k + 1, b: eb + (k - ea) + 1});
    return out;
  }
  function diffHTML(oldText, newText) {
    const d = lineDiff(oldText, newText);
    const row = (cls, n, mark, text) => `<div class="${cls}"><span class="n">${n || ''}</span>${mark} ${esc(text)}</div>`;
    if (!d) return `<div class="h">${esc(t('fe_too_long_diff'))}</div>` + oldText.split('\n').map((l, i) => row('d', i + 1, '-', l)).join('') + newText.split('\n').map((l, i) => row('a', i + 1, '+', l)).join('');
    if (!d.some(x => x.op !== ' ')) return '';
    const keep = new Uint8Array(d.length);
    d.forEach((x, i) => { if (x.op !== ' ') for (let k = Math.max(0, i - 3); k <= Math.min(d.length - 1, i + 3); k++) keep[k] = 1; });
    let html = '', open = false;
    d.forEach((x, i) => {
      if (!keep[i]) { open = false; return; }
      if (!open) { html += `<div class="h">@@ ${x.a ? '−' + x.a : ''} ${x.b ? '+' + x.b : ''} @@</div>`; open = true; }
      html += x.op === '+' ? row('a', x.b, '+', x.text) : x.op === '-' ? row('d', x.a, '-', x.text) : row('c', x.b, ' ', x.text);
    });
    return html;
  }
  // diffView shows a diff over the editor; it resolves true when the user saves
  function diffView(title, html, saveLabel) {
    return new Promise(resolve => {
      const box = document.createElement('div');
      box.className = 'fe-diff'; box.id = 'fe-diff';
      box.innerHTML = `<div class="fe-note"><b>${esc(title)}</b></div><div class="fe-diff-body">${html || `<div class="c">${esc(t('fe_no_changes'))}</div>`}</div>
<div class="fe-foot"><button class="btn-sec" id="fe-diff-back">${esc(t('fe_back'))}</button>${saveLabel && html ? `<button id="fe-diff-save">${esc(saveLabel)}</button>` : ''}</div>`;
      document.querySelector('#modal-editor .fe-main').appendChild(box);
      const done = ok => { box.remove(); E && $('editor-text').focus(); resolve(ok); };
      box.querySelector('#fe-diff-back').onclick = () => done(false);
      box.querySelector('#fe-diff-save')?.addEventListener('click', () => done(true));
      box.addEventListener('keydown', e => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); done(false); } });
      (box.querySelector('#fe-diff-save') || box.querySelector('#fe-diff-back')).focus();
    });
  }
  function showChanges() { if (E && !$('fe-diff')) diffView(t('fe_changes_title'), diffHTML(E.original, $('editor-text').value)); }

  // ── save / reload / close ──
  async function save(closeAfter) {
    if (!E || $('fe-diff')) return;
    if (E.readOnly) { showToast(t('read_only_role'), 'warning'); return; }
    const ta = $('editor-text'), mine = ta.value;
    if (mine === E.original) { if (closeAfter) close(); else showToast(t('fe_no_changes'), 'info', 1500); return; }
    if (isJSON()) { const err = jsonError(mine); if (err && !(await uiConfirm(tf('fe_json_save_anyway', {e: err}), {title: t('save')}))) return; }
    if (lsGet('wrm_fe_review', '1') === '1' && !(await diffView(t('fe_changes_title'), diffHTML(E.original, mine), t('fe_save_these')))) return;
    const btn = $('editor-save-btn');
    btn.disabled = true;
    try {
      // nobody may have written the file since it was opened
      let server = null;
      try { server = norm(decode(await fetchText(E.wo, E.path))); } catch (_) { /* a deleted file is written again */ }
      if (server !== null && server !== E.original && server !== mine) {
        const pick = await uiDialog({title: t('save'), message: t('fe_conflict'), okText: t('fe_overwrite'), danger: true, altText: t('fe_show_server')});
        if (pick === 'alt') { if (!(await diffView(t('fe_server_vs_mine'), diffHTML(server, mine), t('fe_overwrite')))) return; }
        else if (!pick) return;
      }
      const name = E.path.split('/').pop(), dir = parentPath(E.path);
      const fd = new FormData();
      fd.append('file', new Blob([E.crlf ? mine.replace(/\n/g, '\r\n') : mine], {type: 'application/octet-stream'}), name);
      const res = await fetch(`/api/remote/upload?${connQS(E.wo)}&path=${encodeURIComponent(dir)}&overwrite=true&edit=1`, {method: 'POST', body: fd});
      const body = await res.json().catch(() => ({}));
      const r = (body.results || [])[0];
      if (!res.ok || !r || r.status !== 'ok') throw new Error((r && r.error) || body.error || res.statusText);
      E.original = mine;
      note('ai', '');
      meta();
      showToast('💾 ' + t('saved') + ': ' + name, 'success');
      if (E.wo.path === dir) loadFiles(E.wo.id, dir);
      if (closeAfter) close(true);
    } catch (e) {
      showToast(t('save_failed') + ': ' + e.message, 'error', 6000);
    } finally { if ($('editor-save-btn')) $('editor-save-btn').disabled = false; }
  }
  async function reload(silent) {
    if (!E) return;
    if (!silent && dirty() && !(await uiConfirm(t('fe_reload_dirty'), {title: t('fe_reload'), okText: t('fe_reload'), danger: true}))) return;
    try {
      const text = decode(await fetchText(E.wo, E.path));
      E.crlf = text.includes('\r\n'); E.original = norm(text);
      const ta = $('editor-text'), pos = ta.selectionStart, top = ta.scrollTop;
      ta.value = E.original;
      ta.setSelectionRange(Math.min(pos, ta.value.length), Math.min(pos, ta.value.length));
      ta.scrollTop = top;
      E.gutN = 0; note('ai', '');
      refresh(true);
      showToast(silent ? t('ch_reloaded') : t('fe_reloaded'), 'success', 2500);
    } catch (e) { showToast(t('fm_open_failed') + ': ' + e.message, 'error', 5000); }
  }
  async function close(force) {
    if (!E) return true;
    if (!force && dirty() && !(await uiConfirm(t('discard_changes'), {title: t('fm_edit'), okText: t('discard'), danger: true}))) return false;
    if (E.chat) window.wrmChat?.unmount(E.chat);
    $('modal-editor').classList.remove('open');
    E = null;
    return true;
  }

  // ── AI assistant ──
  function loadChat() {
    if (window.wrmChat) return Promise.resolve();
    return new Promise((ok, fail) => {
      const script = document.createElement('script');
      script.src = '/static/aichat.js'; script.onload = ok;
      script.onerror = () => fail(new Error(t('git_load_failed')));
      document.head.appendChild(script);
    });
  }
  async function toggleAI() {
    if (!E) return;
    const box = $('fe-ai');
    if (E.chat) { box.style.display = box.style.display === 'none' ? 'flex' : 'none'; return; }
    box.style.display = 'flex';
    try {
      await loadChat();
      const path = E.path, wo = E.wo;
      E.chat = await wrmChat.mount(box, {connId: wo.connId, purpose: 'file', filePath: path,
        qs: wo.shareToken ? '?share_token=' + encodeURIComponent(wo.shareToken) : '',
        onFileChanged: () => aiChanged(path)});
      onCaret();
    } catch (e) { box.style.display = 'none'; showToast(e.message, 'error'); }
  }
  // aiChanged runs after the assistant wrote the file (an approved edit_file or write_file)
  function aiChanged(path) {
    if (!E || E.path !== path) return;
    if (!dirty()) { reload(true); return; }
    note('ai', `⚠ ${esc(t('fe_ai_changed_dirty'))} <button class="btn-sec btn-sm" onclick="wrmFileEditor.reload(true)">⟲ ${esc(t('fe_reload'))}</button> <button class="btn-sec btn-sm" onclick="wrmFileEditor.showServerDiff()">± ${esc(t('fe_show_server'))}</button>`, 'warn');
  }
  async function showServerDiff() {
    if (!E) return;
    try { const server = norm(decode(await fetchText(E.wo, E.path))); diffView(t('fe_server_vs_mine'), diffHTML(server, $('editor-text').value)); }
    catch (e) { showToast(e.message, 'error'); }
  }
  async function askSelection() {
    if (!E) return;
    const ta = $('editor-text');
    if (ta.selectionEnd <= ta.selectionStart) return;
    if (!E.chat || $('fe-ai').style.display === 'none') await toggleAI();
    const v = ta.value, a = v.slice(0, ta.selectionStart).split('\n').length, b = v.slice(0, ta.selectionEnd).split('\n').length;
    let sel = v.slice(ta.selectionStart, ta.selectionEnd);
    if (sel.length > 6000) sel = sel.slice(0, 6000) + '\n…';
    wrmChat.prefill(E.chat, tf('fe_sel_prompt', {a, b}) + '\n```\n' + sel + '\n```\n');
  }

  window.wrmFileEditor = {open, close, save, reload, find, closeFind, search, next, findKey, replace, gotoLine, setLang, setWrap, setReview,
    checkJSON, formatJSON, showChanges, showServerDiff, toggleAI, askSelection, isOpen: () => !!E, lineDiff};
})();
