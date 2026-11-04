/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// Storages: S3-compatible object storage as a section of its own (docs/rfcs/0008).
// Loaded on demand by openStorages() in index.html; uses its helpers (t, esc, showToast).
'use strict';

Object.assign(LANGS.en, {
  stg_title: 'Storages', stg_add: 'Add storage', stg_none: 'No storages yet. Add an S3-compatible endpoint (Hetzner, DigitalOcean, AWS, MinIO).',
  stg_name: 'Name', stg_endpoint: 'Endpoint', stg_region: 'Region', stg_bucket: 'Bucket', stg_prefix: 'Prefix (folder inside the bucket)',
  stg_access: 'Access key', stg_secret: 'Secret key', stg_secret_keep: 'Leave empty to keep the current key',
  stg_pathstyle: 'Path-style addressing (Hetzner, DigitalOcean, MinIO)', stg_sse: 'Server-side encryption', stg_sse_none: 'None',
  stg_ca: 'TLS certificate fingerprint (only for a self-signed endpoint)', stg_write: 'Allow editing files', stg_upload: 'Allow adding new files',
  stg_delete: 'Allow deleting files', stg_delete_hint: 'Deleting is permanent and asks for a confirmation.',
  stg_test: 'Test connection', stg_testing: 'Testing…', stg_test_ok: 'The bucket answered.', stg_save: 'Save', stg_cancel: 'Cancel', stg_edit: 'Edit',
  stg_delete_storage: 'Remove storage', stg_delete_storage_q: 'Remove the storage {name}? The files in the bucket stay where they are.',
  stg_empty_bucket: 'This folder is empty.', stg_upload: 'Upload', stg_new_folder: 'New folder', stg_folder_name: 'Folder name',
  stg_new_name: 'New name', stg_rename: 'Rename', stg_download: 'Download', stg_delete_file: 'Delete', stg_delete_file_q: 'Delete {key}? This cannot be undone.',
  stg_edit_file: 'Edit', stg_play: 'Play', stg_saved: 'Saved', stg_conflict: 'The file changed in the meantime. Reload it, or overwrite anyway.',
  stg_reload: 'Reload', stg_overwrite: 'Overwrite', stg_too_big: 'This file is too large for the editor; download it instead.',
  stg_binary: 'This file is not text and cannot be edited here.', stg_json_format: 'Format JSON', stg_json_check: 'Check JSON',
  stg_json_ok: 'The JSON is valid.', stg_json_bad: 'JSON error: {e}', stg_subs_check: 'Check timings', stg_subs_shift: 'Shift by', stg_subs_renumber: 'Renumber',
  stg_subs_ms: 'ms', stg_subs_ok: 'Timings look consistent.', stg_subs_bad: '{n} timing problem(s).', stg_subs_shifted: 'Timings shifted by {n} ms.',
  stg_no_preview: 'No preview for this type; download it instead.', stg_readonly: 'Read only', stg_play_unsupported: 'The browser cannot play this video codec; download it instead.',
  stg_loading: 'Loading…', stg_failed: 'The storage request failed.', stg_storage: 'Storage', stg_files: 'Files',
  stg_upload_done: 'Uploaded.', stg_deleted: 'Deleted.', stg_moved: 'Renamed.',
  stg_provider_hetzner: 'Hetzner Object Storage', stg_provider_do: 'DigitalOcean Spaces', stg_provider_aws: 'Amazon S3', stg_provider_minio: 'MinIO',
  stg_share: 'Share', stg_share_t: 'Read-only link to a prefix', stg_share_prefix: 'Folder to share (relative to the storage prefix)',
  stg_share_hours: 'Valid for (hours)', stg_share_create: 'Create link', stg_share_link: 'Link (shown once)', stg_share_copy: 'Copy',
  stg_share_revoke: 'Revoke', stg_share_none: 'No links yet.', stg_share_q: 'Revoke this link? Everyone who has it loses access.',
  stg_share_expires: 'Expires', stg_share_hint: 'Anyone with the link can list and download this folder, read-only, until it expires.',
});
Object.assign(LANGS.hr, {
  stg_title: 'Storage', stg_add: 'Dodaj storage', stg_none: 'Nema storagea. Dodajte S3-kompatibilni endpoint (Hetzner, DigitalOcean, AWS, MinIO).',
  stg_name: 'Naziv', stg_endpoint: 'Endpoint', stg_region: 'Regija', stg_bucket: 'Bucket', stg_prefix: 'Prefiks (mapa unutar bucketa)',
  stg_access: 'Access key', stg_secret: 'Secret key', stg_secret_keep: 'Ostavite prazno da ostane postojeći ključ',
  stg_pathstyle: 'Path-style adresiranje (Hetzner, DigitalOcean, MinIO)', stg_sse: 'Šifriranje na strani poslužitelja', stg_sse_none: 'Bez',
  stg_ca: 'Fingerprint TLS certifikata (samo za self-signed endpoint)', stg_write: 'Dopusti uređivanje datoteka', stg_upload: 'Dopusti dodavanje novih datoteka',
  stg_delete: 'Dopusti brisanje datoteka', stg_delete_hint: 'Brisanje je trajno i traži potvrdu.',
  stg_test: 'Testiraj vezu', stg_testing: 'Testiram…', stg_test_ok: 'Bucket je odgovorio.', stg_save: 'Spremi', stg_cancel: 'Odustani', stg_edit: 'Uredi',
  stg_delete_storage: 'Ukloni storage', stg_delete_storage_q: 'Ukloniti storage {name}? Datoteke u bucketu ostaju gdje jesu.',
  stg_empty_bucket: 'Ova mapa je prazna.', stg_upload: 'Upload', stg_new_folder: 'Nova mapa', stg_folder_name: 'Naziv mape',
  stg_new_name: 'Novi naziv', stg_rename: 'Preimenuj', stg_download: 'Preuzmi', stg_delete_file: 'Obriši', stg_delete_file_q: 'Obrisati {key}? To se ne može poništiti.',
  stg_edit_file: 'Uredi', stg_play: 'Pusti', stg_saved: 'Spremljeno', stg_conflict: 'Datoteka je u međuvremenu promijenjena. Učitajte ponovno ili svejedno prepišite.',
  stg_reload: 'Učitaj ponovno', stg_overwrite: 'Prepiši', stg_too_big: 'Datoteka je prevelika za editor; preuzmite je.',
  stg_binary: 'Ovo nije tekstualna datoteka i ne može se ovdje uređivati.', stg_json_format: 'Formatiraj JSON', stg_json_check: 'Provjeri JSON',
  stg_json_ok: 'JSON je ispravan.', stg_json_bad: 'JSON greška: {e}', stg_subs_check: 'Provjeri vremena', stg_subs_shift: 'Pomakni za', stg_subs_renumber: 'Renumeriraj',
  stg_subs_ms: 'ms', stg_subs_ok: 'Vremena izgledaju ispravno.', stg_subs_bad: '{n} problem(a) s vremenima.', stg_subs_shifted: 'Vremena pomaknuta za {n} ms.',
  stg_no_preview: 'Nema pregleda za ovaj tip; preuzmite datoteku.', stg_readonly: 'Samo čitanje', stg_play_unsupported: 'Browser ne može pustiti ovaj video kodek; preuzmite datoteku.',
  stg_loading: 'Učitavam…', stg_failed: 'Zahtjev prema storageu nije uspio.', stg_storage: 'Storage', stg_files: 'Datoteke',
  stg_upload_done: 'Poslano.', stg_deleted: 'Obrisano.', stg_moved: 'Preimenovano.',
  stg_provider_hetzner: 'Hetzner Object Storage', stg_provider_do: 'DigitalOcean Spaces', stg_provider_aws: 'Amazon S3', stg_provider_minio: 'MinIO',
  stg_share: 'Dijeli', stg_share_t: 'Link samo za čitanje na mapu', stg_share_prefix: 'Mapa za dijeljenje (relativno na prefiks storagea)',
  stg_share_hours: 'Vrijedi (sati)', stg_share_create: 'Stvori link', stg_share_link: 'Link (prikazuje se jednom)', stg_share_copy: 'Kopiraj',
  stg_share_revoke: 'Ukini', stg_share_none: 'Nema linkova.', stg_share_q: 'Ukiniti ovaj link? Svi koji ga imaju izgubit će pristup.',
  stg_share_expires: 'Istječe', stg_share_hint: 'Svatko s linkom može popisati i preuzeti ovu mapu, samo čitanje, dok link ne istekne.',
});

(function () {
  const state = { list: [], current: null, prefix: '', entries: [], next: '', editor: null, styleAdded: false };
  const VIDEO_EXT = ['.mp4', '.webm', '.m4v', '.mov', '.ogv'];
  const TEXT_EXT = ['.json', '.txt', '.md', '.yaml', '.yml', '.xml', '.ini', '.conf', '.cfg', '.env', '.sh', '.go', '.py', '.js', '.ts', '.css', '.html', '.srt', '.vtt', '.ass', '.ssa', '.log', '.csv'];
  const IMAGE_EXT = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.bmp', '.avif'];

  const $ = (sel) => document.querySelector(sel);
  const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; };
  const ext = (k) => { const i = k.lastIndexOf('.'); return i < 0 ? '' : k.slice(i).toLowerCase(); };

  function css() {
    if (state.styleAdded) return;
    state.styleAdded = true;
    const s = document.createElement('style');
    s.textContent = `
.stg-wrap { display:flex; width:min(1180px,96vw); height:86vh; padding:0; overflow:hidden; }
.stg-side { width:250px; min-width:200px; border-right:1px solid var(--border2); display:flex; flex-direction:column; }
.stg-side-head { padding:14px 14px 8px; display:flex; align-items:center; justify-content:space-between; gap:8px; }
.stg-side-head h3 { margin:0; font-size:14px; }
.stg-list { flex:1; overflow:auto; padding:6px; }
.stg-item { padding:9px 10px; border-radius:8px; cursor:pointer; border:1px solid transparent; margin-bottom:4px; }
.stg-item:hover { background:var(--bg3); }
.stg-item.sel { background:var(--bg3); border-color:var(--accent); }
.stg-item .n { font-size:12.5px; font-weight:600; }
.stg-item .m { font-size:10.5px; color:var(--text3); margin-top:2px; }
.stg-dot { display:inline-block; width:7px; height:7px; border-radius:50%; background:var(--text3); margin-right:7px; vertical-align:middle; opacity:.5; }
.stg-dot.ok { background:#3fb950; opacity:1; }
.stg-dot.bad { background:#f85149; opacity:1; }
.stg-main { flex:1; display:flex; flex-direction:column; min-width:0; }
.stg-head { padding:10px 14px; border-bottom:1px solid var(--border2); display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
.stg-crumbs { font-size:12px; color:var(--text2); flex:1; min-width:120px; }
.stg-crumbs a { color:var(--accent); cursor:pointer; text-decoration:none; }
.stg-crumbs span { color:var(--text3); }
.stg-rows { flex:1; overflow:auto; padding:6px 10px; }
.stg-row { display:flex; align-items:center; gap:10px; padding:7px 8px; border-radius:6px; font-size:12.5px; }
.stg-row:hover { background:var(--bg3); }
.stg-row .k { flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.stg-row .s { color:var(--text3); font-size:11px; width:80px; text-align:right; }
.stg-row .d { color:var(--text3); font-size:11px; width:150px; }
.stg-row button { padding:2px 7px; font-size:11px; }
.stg-empty { padding:30px 14px; text-align:center; color:var(--text3); font-size:12px; }
.stg-editor { position:absolute; inset:0; background:var(--bg2); display:flex; flex-direction:column; z-index:5; }
.stg-editor-head { padding:10px 14px; border-bottom:1px solid var(--border2); display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
.stg-editor textarea { flex:1; width:100%; border:0; border-radius:0; font-family:var(--font-mono); font-size:12px; resize:none; padding:12px 14px; background:var(--bg1); color:var(--text); }
.stg-editor-foot { padding:8px 14px; border-top:1px solid var(--border2); display:flex; gap:8px; align-items:center; font-size:11px; color:var(--text3); }
.stg-player { position:absolute; inset:0; background:#000; display:flex; align-items:center; justify-content:center; z-index:5; }
.stg-player video { max-width:100%; max-height:100%; }
.stg-player .close { position:absolute; top:8px; right:10px; }
.stg-modal { position:relative; }
.stg-badge { font-size:10px; padding:1px 6px; border-radius:10px; background:var(--bg3); color:var(--text2); margin-left:4px; }
.stg-form { width:min(620px,94vw); max-height:86vh; overflow:auto; }
.stg-form .row2 { display:flex; gap:10px; }
.stg-form .row2 .field { flex:1; }
.stg-hintx { font-size:10.5px; color:var(--text3); margin-top:3px; }
`;
    document.head.appendChild(s);
  }

  function api(method, path, body, raw) {
    const opts = { method, headers: { 'X-WRM-Request': '1' }, credentials: 'same-origin' };
    if (body !== undefined && body !== null) {
      if (raw) { opts.body = body; opts.headers['Content-Type'] = raw; }
      else { opts.body = JSON.stringify(body); opts.headers['Content-Type'] = 'application/json'; }
    }
    return fetch(path, opts).then(async (res) => {
      const text = await res.text();
      let data = null;
      try { data = text ? JSON.parse(text) : null; } catch (_) { data = null; }
      if (!res.ok) {
        const msg = (data && data.error) || t('stg_failed');
        const err = new Error(msg); err.status = res.status; throw err;
      }
      return data;
    });
  }

  function overlay() { return document.getElementById('stg-overlay'); }

  function shell() {
    let ov = overlay();
    if (ov) return ov;
    ov = el('div', 'modal-overlay');
    ov.id = 'stg-overlay';
    ov.innerHTML = `<div class="modal stg-wrap stg-modal">
      <div class="stg-side"><div class="stg-side-head"><h3>${esc(t('stg_title'))}</h3>
        <button class="btn-sec btn-sm" id="stg-add">＋ ${esc(t('stg_add'))}</button></div>
        <div class="stg-list" id="stg-list"></div></div>
      <div class="stg-main"><div class="stg-head">
          <div class="stg-crumbs" id="stg-crumbs"></div>
          <button class="btn-sec btn-sm" id="stg-upload" style="display:none;">⬆ ${esc(t('stg_upload'))}</button>
          <button class="btn-sec btn-sm" id="stg-newdir" style="display:none;">＋ ${esc(t('stg_new_folder'))}</button>
          <button class="btn-sec btn-sm" id="stg-share" style="display:none;">🔗 ${esc(t('stg_share'))}</button>
          <button class="btn-sec btn-sm" id="stg-refresh">↻</button>
          <button class="btn-sec btn-sm" id="stg-close">✕</button>
        </div>
        <div class="stg-rows" id="stg-rows"></div>
        <div class="stg-empty" id="stg-hint" style="display:none;"></div>
      </div></div>`;
    document.body.appendChild(ov);
    ov.addEventListener('mousedown', (e) => { if (e.target === ov) close(); });
    ov.querySelector('#stg-close').onclick = close;
    ov.querySelector('#stg-add').onclick = () => editStorage(null);
    ov.querySelector('#stg-refresh').onclick = () => loadPrefix(state.prefix);
    ov.querySelector('#stg-upload').onclick = uploadFile;
    ov.querySelector('#stg-newdir').onclick = newFolder;
    ov.querySelector('#stg-share').onclick = openShares;
    return ov;
  }

  function open() {
    css();
    shell().classList.add('open');
    loadList();
  }

  function close() {
    const ov = overlay();
    if (ov) ov.classList.remove('open');
  }

  function loadList() {
    api('GET', '/api/storages').then((list) => {
      state.list = list || [];
      renderList();
      if (state.list.length && !state.current) selectStorage(state.list[0].id);
      else if (!state.list.length) { state.current = null; renderBrowser(); }
    }).catch((e) => showToast(e.message, 'error'));
  }

  function renderList() {
    const box = $('#stg-list');
    if (!box) return;
    box.textContent = '';
    if (!state.list.length) {
      box.appendChild(el('div', 'stg-empty', t('stg_none')));
      return;
    }
    state.list.forEach((s) => {
      const item = el('div', 'stg-item' + (state.current && state.current.id === s.id ? ' sel' : ''));
      const n = el('div', 'n');
      const dot = el('span', 'stg-dot' + (s.status_ok === true ? ' ok' : s.status_ok === false ? ' bad' : ''));
      dot.title = s.status_ok === false ? (s.status_err || t('stg_failed')) : (s.status_at || '');
      n.appendChild(dot); n.appendChild(document.createTextNode(s.name));
      const m = el('div', 'm', `${s.bucket}${s.prefix ? '/' + s.prefix : ''}`);
      item.appendChild(n); item.appendChild(m);
      item.onclick = () => selectStorage(s.id);
      box.appendChild(item);
    });
  }

  function selectStorage(id) {
    state.current = state.list.find((x) => x.id === id) || null;
    state.prefix = '';
    renderList();
    loadPrefix('');
  }

  function can(cap) {
    const s = state.current;
    if (!s) return false;
    if (cap === 'read') return true;
    if (cap === 'write') return !!s.allow_write;
    if (cap === 'upload') return !!s.allow_upload;
    if (cap === 'delete') return !!s.allow_delete;
    return false;
  }

  function loadPrefix(prefix) {
    if (!state.current) { renderBrowser(); return; }
    const id = state.current.id;
    api('GET', `/api/storages/${id}/objects?prefix=${encodeURIComponent(prefix || '')}`).then((res) => {
      if (!state.current || state.current.id !== id) return;
      state.prefix = res.prefix || '';
      state.entries = (res.objects || []).slice();
      state.prefixes = res.prefixes || [];
      renderBrowser();
    }).catch((e) => { showToast(e.message, 'error'); renderBrowser(); });
  }

  function renderBrowser() {
    const rows = $('#stg-rows');
    const crumbs = $('#stg-crumbs');
    const hint = $('#stg-hint');
    const up = $('#stg-upload'), nd = $('#stg-newdir');
    if (!rows) return;
    rows.textContent = '';
    crumbs.textContent = '';
    hint.style.display = 'none';
    if (!state.current) {
      hint.textContent = t('stg_none');
      hint.style.display = 'block';
      up.style.display = 'none'; nd.style.display = 'none';
      const sh0 = document.getElementById('stg-share'); if (sh0) sh0.style.display = 'none';
      return;
    }
    up.style.display = can('upload') ? '' : 'none';
    nd.style.display = can('write') ? '' : 'none';
    const shb = document.getElementById('stg-share'); if (shb) shb.style.display = '';

    const root = el('a', null, state.current.name);
    root.onclick = () => loadPrefix('');
    crumbs.appendChild(root);
    let acc = '';
    state.prefix.split('/').filter(Boolean).forEach((part) => {
      acc += part + '/';
      const sep = el('span', null, ' / ');
      crumbs.appendChild(sep);
      const a = el('a', null, part);
      a.onclick = () => loadPrefix(acc);
      crumbs.appendChild(a);
    });

    (state.prefixes || []).forEach((p) => {
      const name = p.slice(state.prefix.length).replace(/\/$/, '');
      const row = el('div', 'stg-row');
      const k = el('div', 'k', '📁 ' + name);
      k.onclick = () => loadPrefix(p);
      k.style.cursor = 'pointer';
      row.appendChild(k);
      const s = el('div', 's', '');
      const d = el('div', 'd', '');
      row.appendChild(s); row.appendChild(d);

      rows.appendChild(row);
    });

    (state.entries || []).forEach((o) => {
      const name = o.key.slice(state.prefix.length);
      const row = el('div', 'stg-row');
      const k = el('div', 'k', '📄 ' + name);
      k.title = o.key;
      k.style.cursor = 'pointer';
      k.onclick = () => openObject(o.key);
      row.appendChild(k);
      row.appendChild(el('div', 's', fmtSize(o.size)));
      row.appendChild(el('div', 'd', (o.last_modified || '').replace('T', ' ').slice(0, 16)));

      const dl = el('button', 'btn-sec btn-sm', '⬇');
      dl.title = t('stg_download');
      dl.onclick = () => downloadObject(o.key);
      row.appendChild(dl);

      if (can('write')) {
        const ed = el('button', 'btn-sec btn-sm', '✏');
        ed.title = t('stg_edit_file');
        ed.onclick = () => openEditor(o.key);
        row.appendChild(ed);
        const rn = el('button', 'btn-sec btn-sm', '⇄');
        rn.title = t('stg_rename');
        rn.onclick = () => renameObject(o.key, false);
        row.appendChild(rn);
      }
      if (can('delete')) {
        const del = el('button', 'btn-sec btn-sm danger', '🗑');
        del.title = t('stg_delete_file');
        del.onclick = () => deleteObject(o.key);
        row.appendChild(del);
      }
      rows.appendChild(row);
    });

    if (!(state.prefixes || []).length && !(state.entries || []).length) {
      hint.textContent = t('stg_empty_bucket');
      hint.style.display = 'block';
    }
  }

  function fmtSize(n) {
    if (!n) return '0 B';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    return (n / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function objectURL(key, inline) {
    return `/api/storages/${state.current.id}/object?key=${encodeURIComponent(key)}${inline ? '&inline=1' : ''}`;
  }

  function openObject(key) {
    const e = ext(key);
    if (VIDEO_EXT.includes(e)) return openPlayer(key);
    if (IMAGE_EXT.includes(e)) return openImage(key);
    if (TEXT_EXT.includes(e)) return openEditor(key);
    downloadObject(key);
  }

  function downloadObject(key) {
    const a = document.createElement('a');
    a.href = objectURL(key, false);
    a.download = key.split('/').pop();
    document.body.appendChild(a);
    a.click();
    a.remove();
  }

  function openImage(key) {
    const ov = overlay();
    const pane = el('div', 'stg-player');
    const img = el('img');
    img.src = objectURL(key, true);
    img.alt = key;
    img.style.maxWidth = '100%'; img.style.maxHeight = '100%';
    const close = el('button', 'btn-sec close', '✕');
    close.onclick = () => pane.remove();
    pane.appendChild(img); pane.appendChild(close);
    ov.querySelector('.stg-main').appendChild(pane);
  }

  // ─── player and subtitles ───────────────────────────────────────────────────────────────

  function vttTime(v) { // SRT uses , for milliseconds; VTT uses .
    return v.replace(',', '.');
  }

  function srtToVtt(text) {
    const lines = text.replace(/\r\n/g, '\n').split('\n');
    const out = ['WEBVTT', ''];
    for (const line of lines) {
      if (/^\s*\d+\s*$/.test(line)) continue; // the SRT index is not used by VTT
      out.push(line.replace(/^\d\d:\d\d:\d\d,\d\d\d\s*-->\s*\d\d:\d\d:\d\d,\d\d\d/, (m) => vttTime(m)));
    }
    return out.join('\n');
  }

  function assToVtt(text) {
    const out = ['WEBVTT', ''];
    const time = (t) => {
      const m = t.trim().match(/^(\d+):(\d\d):(\d\d)\.(\d\d)$/);
      return m ? `${m[1]}:${m[2]}:${m[3]}.${m[4]}0` : '00:00:00.000';
    };
    for (const line of text.split(/\r?\n/)) {
      if (!/^Dialogue:/.test(line)) continue;
      const parts = line.split(',');
      if (parts.length < 10) continue;
      const from = time(parts[1]), to = time(parts[2]);
      const body = parts.slice(9).join(',').replace(/\{[^}]*\}/g, '').replace(/\\N/gi, '\n');
      out.push(`${from} --> ${to}`, body, '');
    }
    return out.join('\n');
  }

  function openPlayer(key) {
    const ov = overlay();
    const pane = el('div', 'stg-player');
    const video = document.createElement('video');
    video.controls = true;
    video.autoplay = true;
    video.src = objectURL(key, true);
    const note = el('div', 'stg-empty', '');
    note.style.color = '#ddd';
    note.style.display = 'none';
    video.onerror = () => {
      note.textContent = t('stg_play_unsupported');
      note.style.display = 'block';
    };
    const close = el('button', 'btn-sec close', '✕');
    close.onclick = () => pane.remove();
    pane.appendChild(video); pane.appendChild(note); pane.appendChild(close);
    ov.querySelector('.stg-main').appendChild(pane);
    attachSubtitle(video, key, pane);
  }

  // attachSubtitle looks for a sibling .vtt/.srt/.ass of the video and adds it as a track.
  // SRT and ASS are converted to VTT in the browser; the file in the bucket is never touched.
  function attachSubtitle(video, key, pane) {
    const base = key.replace(/\.[^./]+$/, '');
    const found = (state.entries || []).find((o) => {
      const k = o.key;
      return k === base + '.vtt' || k === base + '.srt' || k === base + '.ass';
    });
    if (!found) return;
    const subExt = ext(found.key);
    const done = (src) => {
      const track = document.createElement('track');
      track.kind = 'subtitles';
      track.label = found.key.split('/').pop();
      track.srclang = 'und';
      track.src = src;
      track.default = true;
      video.appendChild(track);
    };
    if (subExt === '.vtt') { done(objectURL(found.key, true) + '&vtt=1'); return; }
    api('GET', `/api/storages/${state.current.id}/text?key=${encodeURIComponent(found.key)}`).then((res) => {
      if (!res || res.binary) return;
      const vtt = subExt === '.ass' || subExt === '.ssa' ? assToVtt(res.content) : srtToVtt(res.content);
      done(URL.createObjectURL(new Blob([vtt], { type: 'text/vtt' })));
    }).catch(() => { /* a missing subtitle must not break playback */ });
  }

  // ─── editor ─────────────────────────────────────────────────────────────────────────────

  function openEditor(key) {
    const id = state.current.id;
    api('GET', `/api/storages/${id}/text?key=${encodeURIComponent(key)}`).then((res) => {
      if (res.binary) { showToast(t('stg_binary'), 'error'); return; }
      state.editor = { key, etag: res.etag, bom: res.bom, crlf: res.crlf, original: res.content, content: res.content };
      renderEditor();
    }).catch((e) => {
      if (e.status === 413) showToast(t('stg_too_big'), 'error');
      else showToast(e.message, 'error');
    });
  }

  function renderEditor() {
    const ov = overlay();
    const ed = state.editor;
    const pane = el('div', 'stg-editor');
    const head = el('div', 'stg-editor-head');
    const title = el('div', 'stg-crumbs', '📄 ' + ed.key);
    title.style.flex = '1';
    head.appendChild(title);

    const e = ext(ed.key);
    const isJSON = e === '.json';
    const isSubs = ['.srt', '.vtt', '.ass', '.ssa'].includes(e);

    if (isJSON) {
      const fmt = el('button', 'btn-sec btn-sm', t('stg_json_format'));
      fmt.onclick = () => { try { const v = JSON.parse(ta.value); ta.value = JSON.stringify(v, null, 2); } catch (err) { showToast(t('stg_json_bad').replace('{e}', err.message), 'error'); } };
      const chk = el('button', 'btn-sec btn-sm', t('stg_json_check'));
      chk.onclick = () => { try { JSON.parse(ta.value); showToast(t('stg_json_ok'), 'success'); } catch (err) { showToast(t('stg_json_bad').replace('{e}', err.message), 'error'); } };
      head.appendChild(fmt); head.appendChild(chk);
    }
    if (isSubs) head.appendChild(subtitleTools(() => ta.value, (v) => { ta.value = v; }));

    const save = el('button', 'btn-pri btn-sm', t('stg_save'));
    save.onclick = () => saveEditor(ta.value, false);
    const closeBtn = el('button', 'btn-sec btn-sm', '✕');
    closeBtn.onclick = () => pane.remove();
    if (can('write')) head.appendChild(save);
    head.appendChild(closeBtn);

    const ta = document.createElement('textarea');
    ta.spellcheck = false;
    ta.value = ed.content;
    const foot = el('div', 'stg-editor-foot', '');
    const st = () => { foot.textContent = `${ta.value.length} B · ${ed.bom ? 'BOM' : 'UTF-8'} · ${ed.crlf ? 'CRLF' : 'LF'}${can('write') ? '' : ' · ' + t('stg_readonly')}`; };
    ta.addEventListener('input', st);
    st();

    pane.appendChild(head); pane.appendChild(ta); pane.appendChild(foot);
    ov.querySelector('.stg-main').appendChild(pane);
    ta.focus();
    state.editor.el = pane;
    state.editor.ta = ta;
  }

  function subtitleTools(get, set) {
    const wrap = el('span');
    const check = el('button', 'btn-sec btn-sm', t('stg_subs_check'));
    check.onclick = () => {
      const problems = subtitleProblems(get());
      if (problems) showToast(t('stg_subs_bad').replace('{n}', problems), 'error');
      else showToast(t('stg_subs_ok'), 'success');
    };
    const renumber = el('button', 'btn-sec btn-sm', t('stg_subs_renumber'));
    renumber.onclick = () => { set(renumberSrt(get())); };
    const shift = el('button', 'btn-sec btn-sm', t('stg_subs_shift') + ' 100 ' + t('stg_subs_ms'));
    shift.onclick = () => { set(shiftTimings(get(), 100)); showToast(t('stg_subs_shifted').replace('{n}', 100), 'success'); };
    wrap.appendChild(check); wrap.appendChild(shift); wrap.appendChild(renumber);
    return wrap;
  }

  const TS_RE = /(\d\d):(\d\d):(\d\d)[,.](\d\d\d)/g;
  function tsToMs(h, m, s, ms) { return ((+h) * 3600 + (+m) * 60 + (+s)) * 1000 + (+ms); }
  function msToTs(ms, comma) {
    const h = Math.floor(ms / 3600000), m = Math.floor(ms % 3600000 / 60000), s = Math.floor(ms % 60000 / 1000), r = ms % 1000;
    const p = (n, w) => String(n).padStart(w, '0');
    return `${p(h, 2)}:${p(m, 2)}:${p(s, 2)}${comma ? ',' : '.'}${p(r, 3)}`;
  }

  function subtitleProblems(text) {
    let n = 0;
    let last = 0;
    const re = /(\d\d):(\d\d):(\d\d)[,.](\d\d\d)\s*-->\s*(\d\d):(\d\d):(\d\d)[,.](\d\d\d)/g;
    let m;
    while ((m = re.exec(text))) {
      const from = tsToMs(m[1], m[2], m[3], m[4]);
      const to = tsToMs(m[5], m[6], m[7], m[8]);
      if (from < last) n++;           // out of order
      if (to <= from) n++;            // empty or inverted cue
      last = to;
    }
    return n;
  }

  function shiftTimings(text, delta) {
    const comma = /\d,\d\d\d/.test(text);
    return text.replace(TS_RE, (all, h, m, s, ms) => {
      const v = Math.max(0, tsToMs(h, m, s, ms) + delta);
      return msToTs(v, all.includes(','));
    });
  }

  function renumberSrt(text) {
    let i = 1;
    return text.replace(/^\s*\d+\s*$/gm, () => String(i++));
  }

  function saveEditor(content, force) {
    const ed = state.editor;
    if (!ed) return;
    const body = { content, etag: force ? '' : ed.etag, bom: ed.bom, crlf: ed.crlf };
    api('PUT', `/api/storages/${state.current.id}/text?key=${encodeURIComponent(ed.key)}`, body).then(() => {
      showToast(t('stg_saved'), 'success');
      ed.el.remove();
      loadPrefix(state.prefix);
    }).catch((e) => {
      if (e.status === 409) {
        if (confirm(t('stg_conflict'))) saveEditor(content, true);
        else openEditor(ed.key);
        return;
      }
      showToast(e.message, 'error');
    });
  }

  // ─── file actions ──────────────────────────────────────────────────────────────────────

  function uploadFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.onchange = () => {
      const f = input.files && input.files[0];
      if (!f) return;
      const key = state.prefix + f.name;
      const limit = (state.current.max_object_mb || 16) * 1024 * 1024;
      if (f.size > limit) { showToast(t('stg_too_big'), 'error'); return; }
      api('PUT', `/api/storages/${state.current.id}/object?key=${encodeURIComponent(key)}`, f, f.type || 'application/octet-stream')
        .then(() => { showToast(t('stg_upload_done'), 'success'); loadPrefix(state.prefix); })
        .catch((e) => showToast(e.message, 'error'));
    };
    input.click();
  }

  function newFolder() {
    const name = prompt(t('stg_folder_name'));
    if (!name) return;
    const prefix = state.prefix + name.replace(/^\/+|\/+$/g, '');
    api('POST', `/api/storages/${state.current.id}/dir?prefix=${encodeURIComponent(prefix)}`)
      .then(() => loadPrefix(state.prefix))
      .catch((e) => showToast(e.message, 'error'));
  }

  function renameObject(key, isFolder) {
    const base = key.slice(state.prefix.length).replace(/\/$/, '');
    const name = prompt(t('stg_new_name'), base);
    if (!name || name === base) return;
    const to = state.prefix + name.replace(/^\/+|\/+$/g, '') + (isFolder ? '/' : '');
    api('POST', `/api/storages/${state.current.id}/move`, { from: key, to, overwrite: false })
      .then(() => { showToast(t('stg_moved'), 'success'); loadPrefix(state.prefix); })
      .catch((e) => showToast(e.message, 'error'));
  }

  function deleteObject(key) {
    if (!confirm(t('stg_delete_file_q').replace('{key}', key))) return;
    fetch(`/api/storages/${state.current.id}/object?key=${encodeURIComponent(key)}`, {
      method: 'DELETE', credentials: 'same-origin',
      headers: { 'X-WRM-Request': '1', 'X-WRM-Confirm': 'delete' },
    }).then(async (res) => {
      if (!res.ok) { const d = await res.json().catch(() => ({})); throw new Error(d.error || t('stg_failed')); }
      showToast(t('stg_deleted'), 'success');
      loadPrefix(state.prefix);
    }).catch((e) => showToast(e.message, 'error'));
  }

  // ─── share links ──────────────────────────────────────────────────────────────────────

  function openShares() {
    const s = state.current;
    if (!s) return;
    api('GET', `/api/storages/${s.id}/shares`).then((list) => renderShares(list || []))
      .catch((e) => showToast(e.message, 'error'));
  }

  function renderShares(list) {
    const s = state.current;
    document.querySelectorAll('.stg-share-modal').forEach((n) => n.remove());
    const ov = el('div', 'modal-overlay open stg-share-modal');
    const rows = list.map((sh) => `<li style="display:flex;gap:10px;align-items:center;padding:4px 0;">
        <span style="flex:1;">📁 ${esc(sh.prefix || '/')}</span>
        <span class="stg-hintx">${esc(t('stg_share_expires'))}: ${esc((sh.expires_at || '').slice(0, 16).replace('T', ' '))}</span>
        <button class="btn-sec btn-sm" data-share="${sh.id}">${esc(t('stg_share_revoke'))}</button></li>`).join('');
    ov.innerHTML = `<div class="modal" style="width:min(620px,94vw);">
      <h3>🔗 ${esc(t('stg_share_t'))}</h3>
      <p class="stg-hintx">${esc(t('stg_share_hint'))}</p>
      <div class="field"><label for="stg-sh-prefix">${esc(t('stg_share_prefix'))}</label>
        <input id="stg-sh-prefix" type="text" value="${esc(state.prefix.replace(/\/$/, ''))}"></div>
      <div class="field"><label for="stg-sh-hours">${esc(t('stg_share_hours'))}</label>
        <input id="stg-sh-hours" type="number" min="1" max="720" value="24"></div>
      <div id="stg-sh-link" style="display:none;margin:10px 0;">
        <div class="stg-hintx">${esc(t('stg_share_link'))}</div>
        <div style="display:flex;gap:8px;align-items:center;">
          <input id="stg-sh-url" type="text" readonly style="flex:1;" value="">
          <button class="btn-sec btn-sm" id="stg-sh-copy">${esc(t('stg_share_copy'))}</button>
        </div>
      </div>
      <h3 style="margin-top:16px;font-size:12px;">${esc(t('stg_share'))}</h3>
      <ul style="list-style:none;padding:0;margin:0;">${rows || `<li class="stg-hintx">${esc(t('stg_share_none'))}</li>`}</ul>
      <div class="modal-footer">
        <button class="btn-sec" id="stg-sh-close">${esc(t('stg_cancel'))}</button>
        <button class="btn-pri" id="stg-sh-create">${esc(t('stg_share_create'))}</button>
      </div></div>`;
    document.body.appendChild(ov);
    ov.addEventListener('mousedown', (e) => { if (e.target === ov) ov.remove(); });
    ov.querySelector('#stg-sh-close').onclick = () => ov.remove();
    ov.querySelectorAll('[data-share]').forEach((btn) => {
      btn.onclick = () => {
        if (!confirm(t('stg_share_q'))) return;
        api('DELETE', `/api/storages/${s.id}/shares/${btn.dataset.share}`)
          .then(() => api('GET', `/api/storages/${s.id}/shares`))
          .then((l) => renderShares(l || []))
          .catch((e) => showToast(e.message, 'error'));
      };
    });
    ov.querySelector('#stg-sh-create').onclick = () => {
      const prefix = ov.querySelector('#stg-sh-prefix').value.trim();
      const hours = parseInt(ov.querySelector('#stg-sh-hours').value, 10) || 24;
      api('POST', `/api/storages/${s.id}/shares`, { prefix, hours }).then((sh) => {
        const url = location.origin + sh.url;
        const box = ov.querySelector('#stg-sh-link');
        box.style.display = 'block';
        ov.querySelector('#stg-sh-url').value = url;
        ov.querySelector('#stg-sh-copy').onclick = () => copyText(url);
        return api('GET', `/api/storages/${s.id}/shares`).then((l) => {
          // Re-render, but keep the new link visible.
          renderShares(l || []);
          const again = document.querySelector('.stg-share-modal');
          if (again) {
            again.querySelector('#stg-sh-link').style.display = 'block';
            again.querySelector('#stg-sh-url').value = url;
            again.querySelector('#stg-sh-copy').onclick = () => copyText(url);
          }
        });
      }).catch((e) => showToast(e.message, 'error'));
    };
  }

  // ─── add / edit a storage ──────────────────────────────────────────────────────────────

  function editStorage(s) {
    css();
    const isNew = !s;
    const src = s || { name: '', endpoint: '', region: '', bucket: '', prefix: '', path_style: true, access_key: '', allow_write: true, allow_upload: false, allow_delete: false, max_object_mb: 16, sse: '' };
    const ov = el('div', 'modal-overlay open');
    ov.innerHTML = `<div class="modal stg-form">
      <h3>${isNew ? esc(t('stg_add')) : esc(t('stg_edit'))}</h3>
      <div class="field"><label for="stg-f-name">${esc(t('stg_name'))}</label><input id="stg-f-name" type="text" value="${esc(src.name)}"></div>
      <div class="row2">
        <div class="field"><label for="stg-f-endpoint">${esc(t('stg_endpoint'))}</label><input id="stg-f-endpoint" type="url" placeholder="https://fsn1.your-objectstorage.com" value="${esc(src.endpoint)}"></div>
        <div class="field"><label for="stg-f-region">${esc(t('stg_region'))}</label><input id="stg-f-region" type="text" placeholder="fsn1" value="${esc(src.region)}"></div>
      </div>
      <div class="row2">
        <div class="field"><label for="stg-f-bucket">${esc(t('stg_bucket'))}</label><input id="stg-f-bucket" type="text" value="${esc(src.bucket)}"></div>
        <div class="field"><label for="stg-f-prefix">${esc(t('stg_prefix'))}</label><input id="stg-f-prefix" type="text" placeholder="team-a/backups" value="${esc(src.prefix)}"></div>
      </div>
      <div class="row2">
        <div class="field"><label for="stg-f-access">${esc(t('stg_access'))}</label><input id="stg-f-access" type="text" value="${esc(src.access_key)}" autocomplete="off"></div>
        <div class="field"><label for="stg-f-secret">${esc(t('stg_secret'))}</label><input id="stg-f-secret" type="password" autocomplete="new-password" placeholder="${isNew ? '' : esc(t('stg_secret_keep'))}"></div>
      </div>
      <div class="field"><label for="stg-f-ca">${esc(t('stg_ca'))}</label><input id="stg-f-ca" type="text" value="${esc(src.ca_fingerprint || '')}" placeholder="sha256:..."></div>
      <div class="field field-check"><input id="stg-f-path" type="checkbox" ${src.path_style ? 'checked' : ''}><label for="stg-f-path" style="margin:0;text-transform:none;letter-spacing:0;font-size:11.5px;">${esc(t('stg_pathstyle'))}</label></div>
      <div class="field field-check"><input id="stg-f-write" type="checkbox" ${src.allow_write ? 'checked' : ''}><label for="stg-f-write" style="margin:0;text-transform:none;letter-spacing:0;font-size:11.5px;">${esc(t('stg_write'))}</label></div>
      <div class="field field-check"><input id="stg-f-upload" type="checkbox" ${src.allow_upload ? 'checked' : ''}><label for="stg-f-upload" style="margin:0;text-transform:none;letter-spacing:0;font-size:11.5px;">${esc(t('stg_upload'))}</label></div>
      <div class="field field-check"><input id="stg-f-delete" type="checkbox" ${src.allow_delete ? 'checked' : ''}><label for="stg-f-delete" style="margin:0;text-transform:none;letter-spacing:0;font-size:11.5px;">${esc(t('stg_delete'))}</label></div>
      <div class="stg-hintx">${esc(t('stg_delete_hint'))}</div>
      <div class="modal-footer">
        ${isNew ? '' : `<button class="btn-sec danger" id="stg-f-remove">${esc(t('stg_delete_storage'))}</button>`}
        <span style="flex:1"></span>
        <button class="btn-sec" id="stg-f-cancel">${esc(t('stg_cancel'))}</button>
        <button class="btn-sec" id="stg-f-test">${esc(t('stg_test'))}</button>
        <button class="btn-pri" id="stg-f-save">${esc(t('stg_save'))}</button>
      </div></div>`;
    document.body.appendChild(ov);
    const val = (id) => document.getElementById(id).value.trim();
    const body = () => ({
      name: val('stg-f-name'), endpoint: val('stg-f-endpoint'), region: val('stg-f-region'),
      bucket: val('stg-f-bucket'), prefix: val('stg-f-prefix'), access_key: val('stg-f-access'),
      secret: document.getElementById('stg-f-secret').value,
      ca_fingerprint: val('stg-f-ca'),
      path_style: document.getElementById('stg-f-path').checked,
      allow_write: document.getElementById('stg-f-write').checked,
      allow_upload: document.getElementById('stg-f-upload').checked,
      allow_delete: document.getElementById('stg-f-delete').checked,
      max_object_mb: src.max_object_mb || 16, sse: src.sse || '',
    });
    document.getElementById('stg-f-cancel').onclick = () => ov.remove();
    document.getElementById('stg-f-test').onclick = (ev) => {
      const btn = ev.currentTarget;
      if (isNew) { showToast(t('stg_test') + ': ' + t('stg_save'), 'info'); return; }
      btn.disabled = true; btn.textContent = t('stg_testing');
      api('POST', `/api/storages/${src.id}/test`).then(() => showToast(t('stg_test_ok'), 'success'))
        .catch((e) => showToast(e.message, 'error'))
        .finally(() => { btn.disabled = false; btn.textContent = t('stg_test'); });
    };
    document.getElementById('stg-f-save').onclick = () => {
      const b = body();
      if (isNew && !b.secret) { showToast(t('stg_secret'), 'error'); return; }
      const p = isNew ? api('POST', '/api/storages', b) : api('PUT', `/api/storages/${src.id}`, b);
      p.then(() => { ov.remove(); loadList(); }).catch((e) => showToast(e.message, 'error'));
    };
    const rm = document.getElementById('stg-f-remove');
    if (rm) rm.onclick = () => {
      if (!confirm(t('stg_delete_storage_q').replace('{name}', src.name))) return;
      api('DELETE', `/api/storages/${src.id}`).then(() => {
        ov.remove(); if (state.current && state.current.id === src.id) state.current = null;
        loadList();
      }).catch((e) => showToast(e.message, 'error'));
    };
  }

  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;
    const ed = document.querySelector('.stg-editor');
    if (ed) { ed.remove(); return; }
    const pl = document.querySelector('.stg-player');
    if (pl) { pl.remove(); return; }
    const ov = overlay();
    if (ov && ov.classList.contains('open')) close();
  });

  window.wrmStorages = {
    open,
    close,
    _state: state,
    reload: () => loadPrefix(state.prefix),
  };
})();
