/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// WRM PRO panel layout (v12.6.8).
// Every side panel of the page (Connections, Sessions, Snippets, Clipboard and the AI live
// view) is a tab in a group. A group sits in a dock zone (left, right or bottom) or in a
// floating window. Dragging a tab onto another tab bar stacks it there, onto the top or bottom
// part of a panel splits that column, onto an edge strip docks it, and anywhere else lets it
// float. The layout can be locked so nothing moves by accident. It is stored per user on the
// server (/api/me/ui/layout) with a copy in this browser for an instant first paint, so it
// follows the user to every browser. Phones keep the fixed layout with the two drawers.
// Workboard addons docked as panels (static/workboard.js) are dynamic panels with ids
// "wb:<tile>": they are registered at run time and go back onto the canvas when hidden.
// v12.6.10: a floating panel lives on the canvas (#win-container) next to the terminals and
// addon windows, shares their z-order, stays inside the canvas and follows the canvas grid.
(function () {
  'use strict';

  Object.assign(LANGS.en, {
    pn_conn: 'Connections', dk_panels: 'Panels', dk_panels_t: 'Panels: show, hide and move the parts of the page, lock the layout',
    dk_move_left: 'Dock on the left', dk_move_right: 'Dock on the right', dk_move_bottom: 'Dock at the bottom', dk_float: 'On the canvas (anywhere)',
    dk_hide: 'Hide {name}', dk_lock: 'Lock the layout', dk_unlock: 'Unlock the layout', dk_reset: 'Reset the layout',
    dk_reset_q: 'Put every panel back where it was at the start? Your current layout is replaced.',
    dk_locked_t: 'The layout is locked. Unlock it in the ▤ Workboard or ▦ Panels menu to move windows and panels.', dk_locked_on: 'Layout locked: windows and panels stay where they are', dk_unlocked: 'Layout unlocked',
    dk_drag_t: 'Drag the tab to move this panel', dk_menu_t: 'Panel options',
    dk_where_left: 'left', dk_where_right: 'right', dk_where_bottom: 'bottom', dk_where_float: 'on the canvas', dk_where_hidden: 'hidden',
    dk_hint_auto: 'appears while an AI app works on a server',
    dk_panels_hint: 'Drag a panel by its tab: onto another tab bar to stack them, onto the top or bottom part of a panel to split the column, onto an edge strip to dock it, anywhere else on the canvas to place it there next to your terminals. The layout follows you to every browser.',
    dk_locked_hint: 'Locked: windows and panels cannot be dragged, split or resized. Showing and hiding still works.',
    dk_drop_left: 'Left', dk_drop_right: 'Right', dk_drop_bottom: 'Bottom', dk_drop_float: 'On the canvas',
    dk_wb_back: 'Back onto the canvas as a window', dk_wb_head: 'Workboard addons',
  });
  Object.assign(LANGS.hr, {
    pn_conn: 'Veze', dk_panels: 'Paneli', dk_panels_t: 'Paneli: prikaži, sakrij i premjesti dijelove stranice, zaključaj raspored',
    dk_move_left: 'Usidri lijevo', dk_move_right: 'Usidri desno', dk_move_bottom: 'Usidri dolje', dk_float: 'Na canvas (bilo gdje)',
    dk_hide: 'Sakrij: {name}', dk_lock: 'Zaključaj raspored', dk_unlock: 'Otključaj raspored', dk_reset: 'Vrati početni raspored',
    dk_reset_q: 'Vratiti sve panele na početna mjesta? Trenutni raspored se zamjenjuje.',
    dk_locked_t: 'Raspored je zaključan. Otključaj ga u izborniku ▤ Workboard ili ▦ Paneli da bi micao prozore i panele.', dk_locked_on: 'Raspored zaključan: prozori i paneli ostaju gdje jesu', dk_unlocked: 'Raspored otključan',
    dk_drag_t: 'Povuci karticu da premjestiš ovaj panel', dk_menu_t: 'Opcije panela',
    dk_where_left: 'lijevo', dk_where_right: 'desno', dk_where_bottom: 'dolje', dk_where_float: 'na canvasu', dk_where_hidden: 'skriven',
    dk_hint_auto: 'pojavi se dok AI aplikacija radi na poslužitelju',
    dk_panels_hint: 'Panel povuci za karticu: na drugu traku kartica da ih složiš zajedno, na gornji ili donji dio panela da podijeliš stupac, na rubnu traku da ga usidriš, bilo gdje drugdje na canvasu da ga tamo postaviš uz terminale. Raspored te prati u svakom pregledniku.',
    dk_locked_hint: 'Zaključano: prozore i panele se ne može povlačiti, dijeliti ni mijenjati im veličinu. Prikazivanje i skrivanje i dalje radi.',
    dk_drop_left: 'Lijevo', dk_drop_right: 'Desno', dk_drop_bottom: 'Dolje', dk_drop_float: 'Na canvas',
    dk_wb_back: 'Vrati na canvas kao prozor', dk_wb_head: 'Workboard addoni',
  });

  const $ = id => document.getElementById(id);
  const tn = (k, v) => { let s = t(k); for (const n in (v || {})) s = s.split('{' + n + '}').join(v[n]); return s; };
  const ZONES = ['left', 'right', 'bottom'];
  const P = {
    conn: {ico: '🖥', key: 'pn_conn', el: 'pnl-conn'},
    sess: {ico: '🗂', key: 'sessions', el: 'pnl-sess'},
    // v13.8.0: every AI session of the user (static/aisessions.js)
    aisess: {ico: '🧠', key: 'aiss_panel', el: 'pnl-aisess', avail: () => typeof currentUser !== 'undefined' && !!currentUser, shown: () => { if (window.wrmAISessions) wrmAISessions.shown(); }},
    snip: {ico: '⚡', key: 'snippets', el: 'pnl-snip', avail: () => typeof currentUser !== 'undefined' && !!currentUser, shown: () => { if (typeof renderSnippets === 'function') renderSnippets(); }},
    ai: {ico: '🤖', key: 'rp_ai', el: 'rp-ai', avail: () => !!(window.wrmMCP && wrmMCP.dockAvailable && wrmMCP.dockAvailable()), badge: 'rp-ai-badge', auto: true},
    clip: {ico: '📋', key: 'clipboard', el: 'clip-panel'},
  };
  const IDS = Object.keys(P);
  const HOME = {conn: 'left', sess: 'right', aisess: 'right', snip: 'right', ai: 'right', clip: 'right'};
  // Dynamic panels: a stored layout keeps their place even before the Workboard has registered
  // them (they are just not shown until then).
  const DYN = /^wb:[a-z0-9]{1,16}$/;
  const known = id => typeof id === 'string' && (!!P[id] || DYN.test(id));
  const allIds = () => Object.keys(P);
  const nameOf = id => (P[id] ? (P[id].label ? P[id].label() : t(P[id].key)) : id);

  function def() {
    return {v: 1, locked: false, t: 0,
      zones: {left: {groups: [{p: ['conn'], a: 'conn', s: 1}], w: 240, wa: 400},
        right: {groups: [{p: ['sess', 'aisess', 'snip', 'ai'], a: 'sess', s: 3}, {p: ['clip'], a: 'clip', s: 1}], w: 210, wa: 400},
        bottom: {groups: [], h: 240}},
      floats: [], hidden: [], last: {}};
  }

  let S = null, compactL = null, sig = '', saveT = 0, suppressClick = false;
  const canvas = () => $('win-container');
  // Floating panels share the window z-order of the canvas (topZ in index.html).
  const raise = w => { if (typeof topZ !== 'undefined') { w.style.zIndex = ++topZ; if (typeof raiseOnTop === 'function') raiseOnTop(); } };
  const grid = () => (typeof canvasGrid === 'function' ? canvasGrid() : 0);
  const gsnap = v => { const g = grid(); return g ? Math.round(v / g) * g : v; };
  // Viewport point → canvas coordinates.
  const toCanvas = (x, y) => { const c = canvas(), r = c ? c.getBoundingClientRect() : {left: 0, top: 0}; return {x: x - r.left, y: y - r.top}; };
  const lastShown = {};

  const compact = () => typeof isCompact === 'function' && isCompact();
  const cur = () => (compact() ? (compactL || (compactL = sanitize(def()))) : S);
  const num = (v, lo, hi, d) => { v = Number(v); return v === v && v !== null && isFinite(v) ? Math.max(lo, Math.min(hi, v)) : d; };

  // sanitize makes any stored value a valid layout: every known panel exactly once (in a group,
  // a floating window or hidden), unknown ones dropped, and a panel added in a later version
  // placed where the default layout has it.
  function sanitize(x) {
    if (!x || typeof x !== 'object' || !x.zones || typeof x.zones !== 'object') return sanitize(def());
    const seen = new Set();
    const grp = g => {
      if (!g || !Array.isArray(g.p)) return null;
      const p = g.p.filter(id => known(id) && !seen.has(id) && seen.add(id));
      return p.length ? {p, a: p.includes(g.a) ? g.a : p[0], s: num(g.s, 0.05, 100, 1)} : null;
    };
    const z = n => (x.zones[n] && typeof x.zones[n] === 'object' ? x.zones[n] : {});
    const out = {v: 1, locked: !!x.locked, t: Number(x.t) || 0, zones: {}, floats: [], hidden: [], last: {}};
    for (const n of ZONES) out.zones[n] = {groups: (Array.isArray(z(n).groups) ? z(n).groups : []).map(grp).filter(Boolean)};
    for (const n of ['left', 'right']) {
      out.zones[n].w = num(z(n).w, 140, 520, n === 'left' ? 240 : 210);
      out.zones[n].wa = num(z(n).wa, 140, 1200, 400);
    }
    out.zones.bottom.h = num(z('bottom').h, 100, 2000, 240);
    for (const f of Array.isArray(x.floats) ? x.floats : []) {
      const g = grp(f);
      if (g) out.floats.push(Object.assign(g, {x: num(f.x, -4000, 10000, 80), y: num(f.y, 0, 10000, 80), w: num(f.w, 220, 4000, 380), h: num(f.h, 140, 4000, 420)}));
    }
    for (const id of Array.isArray(x.hidden) ? x.hidden : []) if (IDS.includes(id) && !seen.has(id)) { seen.add(id); out.hidden.push(id); }
    if (x.last && typeof x.last === 'object') for (const id of IDS) if (ZONES.includes(x.last[id])) out.last[id] = x.last[id];
    if (x.fsize && typeof x.fsize === 'object') for (const id of Object.keys(x.fsize)) {
      const f = x.fsize[id];
      if (known(id) && f && typeof f === 'object') (out.fsize = out.fsize || {})[id] = {w: num(f.w, 220, 4000, 380), h: num(f.h, 140, 4000, 420)};
    }
    for (const id of IDS) if (!seen.has(id)) addTo(out, id, HOME[id]);
    return out;
  }
  function addTo(L, id, z) {
    const gs = L.zones[z].groups;
    if (gs.length) { gs[0].p.push(id); gs[0].a = gs[0].a || id; } else gs.push({p: [id], a: id, s: 1});
  }
  function detach(L, id) {
    L.hidden = L.hidden.filter(x => x !== id);
    const strip = g => {
      const i = g.p.indexOf(id);
      if (i < 0) return;
      g.p.splice(i, 1);
      if (g.a === id) g.a = g.p[Math.max(0, i - 1)] || g.p[0];
    };
    ZONES.forEach(z => L.zones[z].groups.forEach(strip));
    L.floats.forEach(strip);
  }
  function prune(L) {
    ZONES.forEach(z => { L.zones[z].groups = L.zones[z].groups.filter(g => g.p.length); });
    L.floats = L.floats.filter(g => g.p.length);
  }
  function locate(L, id) {
    for (const z of ZONES) for (const g of L.zones[z].groups) if (g.p.includes(id)) return {z, g};
    for (const f of L.floats) if (f.p.includes(id)) return {f, g: f};
    return {hidden: true};
  }
  function where(id) {
    const L = cur();
    if (!L) return HOME[id] || 'hidden';
    const l = locate(L, id);
    return l.z || (l.f ? 'float' : 'hidden');
  }

  // The v12.6.7 settings (right tab, right panel widths, AI dock) become the first layout.
  function migrate() {
    const L = def();
    const uid = (typeof currentUser !== 'undefined' && currentUser && currentUser.id) || 0;
    try {
      const rt = localStorage.getItem('wrm_right_tab');
      L.zones.right.groups[0].a = rt === 'snippets' ? 'snip' : rt === 'ai' ? 'ai' : 'sess';
      const w = JSON.parse(localStorage.getItem('wrm_rp_widths') || '{}') || {};
      if (w.main) L.zones.right.w = w.main;
      if (w.ai) L.zones.right.wa = w.ai;
      const d = JSON.parse(localStorage.getItem('wrm_ai_dock_' + uid) || '{}') || {};
      if (d.mode === 'bottom') { detach(L, 'ai'); L.zones.bottom.groups.push({p: ['ai'], a: 'ai', s: 1}); if (d.bottomH) L.zones.bottom.h = d.bottomH; }
      else if (d.mode === 'float') { const f = d.float || {}; detach(L, 'ai'); L.floats.push({p: ['ai'], a: 'ai', x: f.x == null ? 80 : f.x, y: f.y == null ? 120 : f.y, w: f.w || 520, h: f.h || 380}); }
      if (d.min) { detach(L, 'ai'); L.hidden.push('ai'); L.last.ai = d.mode === 'bottom' ? 'bottom' : 'right'; }
    } catch (e) { /* storage unavailable: defaults */ }
    prune(L);
    return sanitize(L);
  }

  // ── persistence ──
  const lsKey = () => 'wrm_layout_' + ((typeof currentUser !== 'undefined' && currentUser && currentUser.id) || 0);
  const canSave = () => typeof currentUser !== 'undefined' && !!currentUser && !(typeof isCollabMode !== 'undefined' && isCollabMode);
  function saveSoon() {
    if (!S || !canSave()) return;
    S.t = Date.now();
    try { localStorage.setItem(lsKey(), JSON.stringify(S)); } catch (e) { /* only on the server then */ }
    clearTimeout(saveT);
    saveT = setTimeout(push, 700);
  }
  function push(keepalive) {
    saveT = 0;
    if (!S || !canSave()) return;
    fetch('/api/me/ui/layout', {method: 'PUT', headers: {'Content-Type': 'application/json', 'X-WRM-Request': '1'}, body: JSON.stringify({value: S}), keepalive: !!keepalive}).catch(() => {});
  }
  window.addEventListener('pagehide', () => { if (saveT) { clearTimeout(saveT); push(true); } });

  async function init() {
    let local = null;
    try { local = JSON.parse(localStorage.getItem(lsKey()) || 'null'); } catch (e) { /* none */ }
    S = local ? sanitize(local) : migrate();
    compactL = null; sig = '';
    render();
    if (!canSave()) return;
    try {
      const r = await fetch('/api/me/ui/layout');
      if (!r.ok) return;
      const j = await r.json(), v = j && j.value, vt = (v && Number(v.t)) || 0;
      if (v && vt > (S.t || 0)) {
        S = sanitize(v); sig = '';
        try { localStorage.setItem(lsKey(), JSON.stringify(S)); } catch (e) { /* not cached */ }
        render();
        document.dispatchEvent(new CustomEvent('wrm-layout-loaded'));
      } else if (local && (S.t || 0) > vt) push();
    } catch (e) { /* offline: the copy in this browser */ }
  }

  // ── rendering ──
  function avail(id) {
    const p = P[id];
    if (!p || !$(p.el)) return false;
    if (p.dyn && compact()) return false;
    if (p.auto && compact()) return false;
    return !p.avail || !!p.avail();
  }
  const vis = g => g.p.filter(avail);
  const activeOf = g => { const v = vis(g); return v.includes(g.a) ? g.a : v[0]; };
  const zoneHost = z => $(z === 'bottom' ? 'dz-bottom-g' : 'dz-' + z);
  function park() {
    let p = $('dk-park');
    if (!p) { p = document.createElement('div'); p.id = 'dk-park'; p.hidden = true; document.body.appendChild(p); }
    return p;
  }
  function signature(L) {
    const nm = g => vis(g).map(id => id + '|' + nameOf(id) + '|' + (P[id].ico || ''));
    return JSON.stringify([ZONES.map(z => L.zones[z].groups.map(nm)), L.floats.map(nm), compact(), typeof currentLang !== 'undefined' ? currentLang : '']);
  }

  function build(L) {
    const pk = park();
    allIds().forEach(id => { const e = $(P[id].el); if (e) { e.classList.add('dk-pnl'); e.classList.remove('dk-on'); pk.appendChild(e); } });
    document.querySelectorAll('.dk-float').forEach(e => e.remove());
    for (const z of ZONES) {
      const host = zoneHost(z);
      if (!host) continue;
      host.innerHTML = '';
      L.zones[z].groups.filter(g => vis(g).length).forEach((g, i) => {
        if (i) host.appendChild(splitter(z));
        host.appendChild(groupEl(g, {z}));
      });
    }
    if (compact()) return;
    L.floats.forEach(f => {
      if (!vis(f).length) return;
      const w = document.createElement('div');
      w.className = 'dk-float';
      w._f = f;
      Object.assign(w.style, {left: f.x + 'px', top: f.y + 'px', width: f.w + 'px', height: f.h + 'px'});
      raise(w);
      w.appendChild(groupEl(f, {f}));
      w.addEventListener('pointerdown', () => raise(w), true);
      (canvas() || document.body).appendChild(w);
      clampFloat(w);
      if (window.ResizeObserver) new ResizeObserver(() => {
        if (!w.isConnected || S.locked) return;
        const nw = Math.round(w.offsetWidth), nh = Math.round(w.offsetHeight);
        if (Math.abs(nw - f.w) < 2 && Math.abs(nh - f.h) < 2) return;
        f.w = nw; f.h = nh; saveSoon();
      }).observe(w);
    });
  }
  function groupEl(g, loc) {
    const el = document.createElement('div');
    el.className = 'dk-group';
    el._g = g; el._loc = loc;
    const tabs = document.createElement('div'), list = document.createElement('div');
    tabs.className = 'dk-tabs';
    list.className = 'dk-tl'; list.setAttribute('role', 'tablist');
    tabs.appendChild(list);
    vis(g).forEach(id => {
      const p = P[id], b = document.createElement('button');
      b.type = 'button'; b.className = 'dk-tab'; b.dataset.p = id; b.setAttribute('role', 'tab');
      b.title = t('dk_drag_t');
      b.innerHTML = `<span class="ico">${esc(p.ico)}</span><span class="nm">${esc(nameOf(id))}</span>${p.badge ? `<span class="rp-badge" id="${p.badge}"></span>` : ''}`;
      b.addEventListener('click', () => { if (suppressClick) return; activate(id); });
      b.addEventListener('pointerdown', e => tabDown(e, id, el));
      list.appendChild(b);
    });
    const fill = document.createElement('span');
    fill.className = 'dk-fill';
    if (loc.f) fill.addEventListener('pointerdown', e => floatMove(e, el));
    const mb = document.createElement('button');
    mb.type = 'button'; mb.className = 'dk-mbtn'; mb.textContent = '⋯';
    mb.title = t('dk_menu_t'); mb.setAttribute('aria-label', t('dk_menu_t')); mb.setAttribute('aria-haspopup', 'menu');
    mb.addEventListener('click', e => { e.stopPropagation(); groupMenu(mb, g); });
    list.appendChild(fill);
    tabs.appendChild(mb);
    const body = document.createElement('div');
    body.className = 'dk-body';
    vis(g).forEach(id => body.appendChild($(P[id].el)));
    el.append(tabs, body);
    return el;
  }
  function splitter(z) {
    const s = document.createElement('div');
    s.className = 'dk-split' + (z === 'bottom' ? ' h' : '');
    s._z = z;
    s.addEventListener('pointerdown', e => splitDown(e, s));
    return s;
  }

  function update(L) {
    document.querySelectorAll('.dk-group').forEach(el => {
      const g = el._g, a = activeOf(g);
      el.querySelectorAll('.dk-tab').forEach(b => { const on = b.dataset.p === a; b.classList.toggle('on', on); b.setAttribute('aria-selected', on ? 'true' : 'false'); });
      const on = el.querySelector('.dk-tab.on'), tl = el.querySelector('.dk-tl');
      if (on && tl && tl.scrollWidth > tl.clientWidth) {
        if (on.offsetLeft < tl.scrollLeft) tl.scrollLeft = on.offsetLeft;
        else if (on.offsetLeft + on.offsetWidth > tl.scrollLeft + tl.clientWidth) tl.scrollLeft = on.offsetLeft + on.offsetWidth - tl.clientWidth;
      }
      vis(g).forEach(id => $(P[id].el).classList.toggle('dk-on', id === a));
      if (!el._loc.f) el.style.flex = g.s + ' 1 0';
    });
    const has = z => L.zones[z].groups.some(g => vis(g).length);
    const sb = $('sidebar'), sp = $('sess-panel'), bt = $('dz-bottom');
    if (sb) sb.classList.toggle('dz-empty', !has('left'));
    if (sp) sp.classList.toggle('dz-empty', !has('right'));
    if (bt) bt.classList.toggle('on', has('bottom') && !compact());
    if (!compact()) {
      if (sb) sb.style.width = zoneW('left') + 'px';
      if (sp) sp.style.width = zoneW('right') + 'px';
      if (bt) bt.style.height = Math.min(L.zones.bottom.h, bottomMax()) + 'px';
    }
    allIds().forEach(id => {
      const s = isShown(id);
      if (s && !lastShown[id] && P[id].shown) P[id].shown();
      lastShown[id] = s;
    });
    document.body.classList.toggle('dk-locked', !!(S && S.locked) && !compact());
    const pb = $('btn-panels');
    if (pb) { const i = pb.querySelector('span'); if (i) i.textContent = S && S.locked ? '🔒' : '▦'; pb.classList.toggle('dk-is-locked', !!(S && S.locked)); }
    if (typeof rightTab !== 'undefined') {
      const a = ['snip', 'ai', 'sess'].find(isShown);
      if (a) rightTab = a === 'snip' ? 'snippets' : a === 'ai' ? 'ai' : 'sessions';
    }
    if (window.wrmMCP && wrmMCP.dockStyle) wrmMCP.dockStyle();
    if (typeof scheduleRelayout === 'function') scheduleRelayout();
  }
  function render() {
    if (!S) return;
    const L = cur(), s = signature(L);
    if (s !== sig) { sig = s; build(L); }
    update(L);
  }

  function shownIn(z) {
    const L = cur();
    return L ? L.zones[z].groups.map(activeOf).filter(Boolean) : [];
  }
  function zoneW(z) { const zn = S.zones[z]; return shownIn(z).includes('ai') ? zn.wa : zn.w; }
  function zoneMax(z) { return shownIn(z).includes('ai') ? Math.max(420, Math.min(1200, Math.round(window.innerWidth * 0.5))) : 520; }
  function bottomMax() { const c = $('center'); return Math.max(140, Math.round((c ? c.clientHeight : window.innerHeight) * 0.75)); }
  function isShown(id) { const e = P[id] && $(P[id].el); return !!e && e.classList.contains('dk-on') && !!e.closest('.dk-group'); }

  // ── actions ──
  function activate(id, o) {
    if (!S) return;
    const L = cur(), loc = locate(L, id);
    if (loc.g && loc.g.a !== id) { loc.g.a = id; if (L === S) saveSoon(); }
    render();
    // a panel on the canvas comes to the front, above the windows that cover it
    if (loc.f) { const e = P[id] && $(P[id].el), w = e && e.closest('.dk-float'); if (w) raise(w); }
    if (o && o.reveal && !compact()) {
      if (loc.z === 'left' && $('sidebar').classList.contains('collapsed') && typeof toggleSidebar === 'function') toggleSidebar();
      if (loc.z === 'right' && $('sess-panel').classList.contains('collapsed') && typeof toggleSessPanel === 'function') toggleSessPanel();
    }
  }
  function blocked() {
    if (compact()) return true;
    if (S.locked) { showToast(t('dk_locked_t'), 'warning', 3500); return true; }
    return false;
  }
  // floatAt places a panel on the canvas at canvas coordinates x, y.
  function floatAt(id, x, y) {
    const sz = (S.fsize && S.fsize[id]) || {};
    const w = sz.w || (id === 'ai' ? 520 : 340), h = sz.h || (id === 'ai' ? 380 : 420);
    S.floats.push({p: [id], a: id, x: Math.round(gsnap(x)), y: Math.round(gsnap(y)), w, h});
  }
  // The next free-ish spot for a panel put on the canvas from a menu.
  function canvasSpot(id) {
    const c = canvas(), n = S.floats.length % 6, sz = (S.fsize && S.fsize[id]) || {};
    const cw = c ? c.clientWidth : window.innerWidth;
    return {x: Math.max(0, Math.round(cw / 2 - (sz.w || 340) / 2) + 28 * n), y: 40 + 28 * n};
  }
  function rememberZone(id, from) { if (ZONES.includes(from)) S.last[id] = from; }
  function moveTo(id, z) {
    if (!S || blocked()) return;
    const from = where(id);
    if (z === from && z !== 'float') return;
    detach(S, id);
    if (z === 'float') { const p = canvasSpot(id); floatAt(id, p.x, p.y); rememberZone(id, from); }
    else if (ZONES.includes(z)) {
      const gs = S.zones[z].groups;
      if (gs.length) { gs[0].p.push(id); gs[0].a = id; } else gs.push({p: [id], a: id, s: 1});
    }
    prune(S); saveSoon(); render();
    if (ZONES.includes(z)) activate(id, {reveal: true});
  }
  // Showing and hiding change what is visible, not where things are, so they work while locked.
  function hide(id) {
    if (!S || compact()) return;
    if (P[id] && P[id].dyn) { const cb = P[id].closed; remove(id); if (cb) cb(); return; }
    const from = where(id);
    if (from === 'hidden') return;
    rememberZone(id, from);
    const l = locate(S, id);
    if (l.f) { S.fsize = S.fsize || {}; S.fsize[id] = {w: l.f.w, h: l.f.h}; }
    detach(S, id); S.hidden.push(id);
    prune(S); saveSoon(); render();
  }
  function show(id) {
    if (!S || compact() || where(id) !== 'hidden') return;
    detach(S, id);
    addTo(S, id, S.last[id] || HOME[id] || 'right');
    const l = locate(S, id);
    if (l.g) l.g.a = id;
    saveSoon(); render();
    activate(id, {reveal: true});
  }
  function setLocked(on) {
    if (!S) return;
    S.locked = !!on;
    saveSoon(); render();
    showToast(t(S.locked ? 'dk_locked_on' : 'dk_unlocked'), 'success', 2200);
    document.dispatchEvent(new CustomEvent('wrm-layout-locked'));
  }
  // snapshot / apply carry a whole layout for the Workboard's saved layouts, export and import.
  function snapshot() { return S ? JSON.parse(JSON.stringify(S)) : null; }
  function apply(x) {
    if (!x || typeof x !== 'object') return false;
    S = sanitize(x); sig = '';
    saveSoon(); render();
    document.dispatchEvent(new CustomEvent('wrm-layout-loaded'));
    return true;
  }
  async function reset() {
    if (!S || !await uiConfirm(t('dk_reset_q'), {okText: t('dk_reset')})) return;
    S = sanitize(def()); sig = '';
    saveSoon(); render();
    document.dispatchEvent(new CustomEvent('wrm-layout-loaded'));
  }
  // register adds a dynamic panel: def {ico, label(), el (its id), node (the element, when it
  // is not in the page yet), shown(), closed()}. With place it is
  // also put into the layout ('float' or a zone); without it, it only shows where the stored
  // layout already has it.
  function register(id, d, place) {
    if (!DYN.test(id) || !d || !d.el) return false;
    P[id] = Object.assign({}, d, {dyn: true});
    delete P[id].node;
    const e = d.node || $(d.el);
    if (e && !e.closest('.dk-group')) park().appendChild(e);
    if (place && S && where(id) === 'hidden') {
      if (place === 'float') { const p = canvasSpot(id); floatAt(id, p.x, p.y); }
      else addTo(S, id, ZONES.includes(place) ? place : 'right');
      const l = locate(S, id);
      if (l.g) l.g.a = id;
      saveSoon();
    }
    sig = ''; render();
    return true;
  }
  // remove takes a dynamic panel out of the layout (its element is the caller's to reuse).
  function remove(id) {
    if (!DYN.test(id)) return;
    const el = P[id] && $(P[id].el);
    if (S) {
      const l = locate(S, id);
      if (l.f) { S.fsize = S.fsize || {}; S.fsize[id] = {w: l.f.w, h: l.f.h}; }
      if (!l.hidden) { detach(S, id); prune(S); saveSoon(); }
    }
    delete P[id]; delete lastShown[id];
    if (el) { el.classList.remove('dk-on'); park().appendChild(el); }
    sig = ''; render();
  }
  // Dynamic panel ids the current layout places (registered or not).
  function placed() {
    if (!S) return [];
    const out = [];
    ZONES.forEach(z => S.zones[z].groups.forEach(g => g.p.forEach(id => { if (DYN.test(id)) out.push(id); })));
    S.floats.forEach(f => f.p.forEach(id => { if (DYN.test(id)) out.push(id); }));
    return out;
  }
  function zoneResized(z, px) {
    if (!S || compact() || !px) return;
    const zn = S.zones[z];
    if (shownIn(z).includes('ai')) zn.wa = Math.round(px); else zn.w = Math.round(px);
    saveSoon();
  }

  // ── drag and drop ──
  const inR = (r, x, y) => x >= r.left && x <= r.right && y >= r.top && y <= r.bottom;
  function strips() {
    const wc = $('win-container');
    if (!wc) return [];
    const r = wc.getBoundingClientRect(), W = 46;
    const spec = {left: [r.left + 4, r.top + 4, W, r.height - 8], right: [r.right - W - 4, r.top + 4, W, r.height - 8], bottom: [r.left + W + 10, r.bottom - W - 4, r.width - 2 * W - 20, W]};
    return ZONES.map(z => {
      let s = $('dk-strip-' + z);
      if (!s) { s = document.createElement('div'); s.id = 'dk-strip-' + z; s.className = 'dk-strip' + (z === 'bottom' ? '' : ' v'); document.body.appendChild(s); }
      s.textContent = (z === 'left' ? '⇤ ' : z === 'right' ? '⇥ ' : '⤓ ') + t('dk_drop_' + z);
      const [l, tp, w, h] = spec[z];
      Object.assign(s.style, {left: l + 'px', top: tp + 'px', width: Math.max(40, w) + 'px', height: Math.max(40, h) + 'px'});
      return {z, s};
    });
  }
  function hit(x, y, st) {
    for (const {z, s} of st.strips) if (inR(s.getBoundingClientRect(), x, y)) return {k: 'zone', z, el: s};
    const el = document.elementFromPoint(x, y), gel = el && el.closest('.dk-group');
    if (gel) {
      const g = gel._g, loc = gel._loc, tabs = gel.querySelector('.dk-tabs'), tr = tabs.getBoundingClientRect(), gr = gel.getBoundingClientRect();
      if (inR(tr, x, y) || loc.f) {
        const tbs = [...tabs.querySelectorAll('.dk-tab')];
        const before = tbs.find(b => { const r = b.getBoundingClientRect(); return x < r.left + r.width / 2; });
        return {k: 'stack', g, at: before ? before.dataset.p : null, r: gr};
      }
      const br = gel.querySelector('.dk-body').getBoundingClientRect(), horiz = loc.z === 'bottom';
      const rel = horiz ? (x - br.left) / br.width : (y - br.top) / br.height;
      if (rel < 0.3 || rel > 0.7) {
        const after = rel > 0.7;
        const r = horiz ? {left: after ? br.left + br.width / 2 : br.left, top: br.top, width: br.width / 2, height: br.height}
          : {left: br.left, top: after ? br.top + br.height / 2 : br.top, width: br.width, height: br.height / 2};
        return {k: 'split', g, z: loc.z, after, r};
      }
      return {k: 'stack', g, at: null, r: gr};
    }
    const sz = (S.fsize && S.fsize[st.id]) || {};
    return {k: 'float', x: x - 40, y: y - 14, r: {left: x - 40, top: y - 14, width: sz.w || 340, height: sz.h || 300}};
  }
  function indicate(st) {
    const tg = st.tgt;
    st.strips.forEach(({z, s}) => s.classList.toggle('hot', !!tg && tg.k === 'zone' && tg.z === z));
    let r = tg && tg.k !== 'zone' ? tg.r : null;
    if (tg && tg.k === 'split' && tg.g === st.g0 && st.g0.p.length === 1) r = null;
    if (tg && tg.k === 'stack' && tg.g === st.g0 && st.g0.p.length === 1) r = null;
    st.ind.style.display = r ? 'block' : 'none';
    if (r) Object.assign(st.ind.style, {left: r.left + 'px', top: r.top + 'px', width: r.width + 'px', height: r.height + 'px'});
    st.ind.classList.toggle('fl', !!tg && tg.k === 'float');
  }
  function tabDown(ev, id, gel) {
    if (ev.button !== 0 || compact() || !S) return;
    const st = {id, g0: gel._g, sx: ev.clientX, sy: ev.clientY, on: false};
    const end = () => {
      document.removeEventListener('pointermove', mv); document.removeEventListener('pointerup', up); document.removeEventListener('pointercancel', cancel);
      document.body.classList.remove('dk-dragging');
      if (st.ghost) st.ghost.remove();
      if (st.ind) st.ind.remove();
      (st.strips || []).forEach(({s}) => s.classList.remove('hot'));
    };
    const mv = e => {
      if (!st.on) {
        if (Math.abs(e.clientX - st.sx) + Math.abs(e.clientY - st.sy) < 7) return;
        if (S.locked) { end(); showToast(t('dk_locked_t'), 'warning', 3500); return; }
        st.on = true;
        document.body.classList.add('dk-dragging');
        st.strips = strips();
        st.ghost = document.createElement('div'); st.ghost.className = 'dk-ghost';
        st.ghost.textContent = P[id].ico + ' ' + nameOf(id);
        st.ind = document.createElement('div'); st.ind.className = 'dk-ind';
        document.body.append(st.ind, st.ghost);
      }
      e.preventDefault();
      st.ghost.style.left = (e.clientX + 12) + 'px'; st.ghost.style.top = (e.clientY + 10) + 'px';
      st.tgt = hit(e.clientX, e.clientY, st);
      indicate(st);
    };
    const up = () => {
      end();
      if (!st.on) return;
      suppressClick = true; setTimeout(() => { suppressClick = false; }, 0);
      drop(st);
    };
    const cancel = () => end();
    document.addEventListener('pointermove', mv); document.addEventListener('pointerup', up); document.addEventListener('pointercancel', cancel);
  }
  function drop(st) {
    const {id, tgt, g0} = st;
    if (!tgt) return;
    if ((tgt.k === 'stack' || tgt.k === 'split') && tgt.g === g0 && g0.p.length === 1) return;
    const from = where(id), fl = locate(S, id).f;
    if (fl) { S.fsize = S.fsize || {}; S.fsize[id] = {w: fl.w, h: fl.h}; }
    detach(S, id);
    if (tgt.k === 'stack') {
      const i = tgt.at && tgt.at !== id ? tgt.g.p.indexOf(tgt.at) : -1;
      if (i >= 0) tgt.g.p.splice(i, 0, id); else tgt.g.p.push(id);
      tgt.g.a = id;
    } else if (tgt.k === 'split') {
      const gs = S.zones[tgt.z].groups, s = tgt.g.s / 2;
      tgt.g.s = s;
      gs.splice(gs.indexOf(tgt.g) + (tgt.after ? 1 : 0), 0, {p: [id], a: id, s});
    } else if (tgt.k === 'zone') {
      const gs = S.zones[tgt.z].groups;
      gs.push({p: [id], a: id, s: gs.length ? gs.reduce((a, g) => a + g.s, 0) / gs.length : 1});
    } else {
      const p = toCanvas(tgt.x, tgt.y);
      floatAt(id, Math.max(0, p.x), Math.max(0, p.y));
      rememberZone(id, from);
    }
    prune(S); saveSoon(); render();
    if (tgt.k === 'zone' || tgt.k === 'split') activate(id, {reveal: true});
  }
  function splitDown(ev, sp) {
    if (ev.button !== 0 || compact() || !S || S.locked) return;
    const a = sp.previousElementSibling, b = sp.nextElementSibling;
    if (!a || !b || !a._g || !b._g) return;
    ev.preventDefault();
    const horiz = sp._z === 'bottom', ra = a.getBoundingClientRect(), rb = b.getBoundingClientRect();
    const pa = horiz ? ra.width : ra.height, pb = horiz ? rb.width : rb.height, tot = a._g.s + b._g.s, s0 = horiz ? ev.clientX : ev.clientY;
    document.body.classList.add('dk-resizing');
    const mv = e => {
      const na = Math.max(60, Math.min(pa + pb - 60, pa + (horiz ? e.clientX : e.clientY) - s0));
      a._g.s = tot * na / (pa + pb); b._g.s = tot - a._g.s;
      a.style.flex = a._g.s + ' 1 0'; b.style.flex = b._g.s + ' 1 0';
    };
    const up = () => {
      document.removeEventListener('pointermove', mv); document.removeEventListener('pointerup', up);
      document.body.classList.remove('dk-resizing');
      saveSoon();
      if (typeof scheduleRelayout === 'function') scheduleRelayout();
    };
    document.addEventListener('pointermove', mv); document.addEventListener('pointerup', up);
  }
  // A panel on the canvas stays completely inside it, like the windows (it shrinks to fit).
  function clampFloat(w) {
    const f = w._f, c = canvas();
    if (!c || !c.clientWidth || !c.clientHeight) return;
    const cw = c.clientWidth, ch = c.clientHeight;
    if (w.offsetWidth > cw) w.style.width = cw + 'px';
    if (w.offsetHeight > ch) w.style.height = ch + 'px';
    const x = Math.max(0, Math.min(cw - w.offsetWidth, parseInt(w.style.left, 10) || 0));
    const y = Math.max(0, Math.min(ch - w.offsetHeight, parseInt(w.style.top, 10) || 0));
    w.style.left = x + 'px'; w.style.top = y + 'px';
    if (f) { f.x = Math.round(x); f.y = Math.round(y); }
  }
  function clampFloats() { document.querySelectorAll('.dk-float').forEach(clampFloat); }
  function floatMove(ev, gel) {
    const w = gel.parentElement;
    if (ev.button !== 0 || !w || !w._f || compact()) return;
    if (S.locked) { showToast(t('dk_locked_t'), 'warning', 3500); return; }
    ev.preventDefault();
    const sx = ev.clientX - w.offsetLeft, sy = ev.clientY - w.offsetTop;
    document.body.classList.add('dk-dragging');
    const mv = e => { w.style.left = gsnap(e.clientX - sx) + 'px'; w.style.top = gsnap(e.clientY - sy) + 'px'; clampFloat(w); };
    const up = () => {
      document.removeEventListener('pointermove', mv); document.removeEventListener('pointerup', up);
      document.body.classList.remove('dk-dragging');
      clampFloat(w); saveSoon();
    };
    document.addEventListener('pointermove', mv); document.addEventListener('pointerup', up);
  }
  function bottomResize(ev) {
    if (ev.button !== 0 || compact() || !S || S.locked) return;
    const bt = $('dz-bottom');
    ev.preventDefault();
    const sy = ev.clientY, sh = bt.offsetHeight, max = bottomMax();
    document.body.classList.add('dk-resizing');
    const mv = e => { S.zones.bottom.h = Math.round(Math.max(100, Math.min(max, sh - (e.clientY - sy)))); bt.style.height = S.zones.bottom.h + 'px'; };
    const up = () => {
      document.removeEventListener('pointermove', mv); document.removeEventListener('pointerup', up);
      document.body.classList.remove('dk-resizing');
      saveSoon();
      if (typeof scheduleRelayout === 'function') scheduleRelayout();
    };
    document.addEventListener('pointermove', mv); document.addEventListener('pointerup', up);
  }

  // ── menus ──
  let menuAnchor = null;
  function menuEl() {
    let m = $('dk-menu');
    if (m) return m;
    m = document.createElement('div');
    m.id = 'dk-menu'; m.className = 'dk-menu'; m.setAttribute('role', 'menu');
    document.body.appendChild(m);
    document.addEventListener('pointerdown', e => { if (m.classList.contains('open') && !m.contains(e.target) && e.target !== menuAnchor && !(menuAnchor && menuAnchor.contains(e.target))) closeMenu(); }, true);
    document.addEventListener('keydown', e => { if (e.key === 'Escape' && m.classList.contains('open')) { closeMenu(); if (menuAnchor && menuAnchor.focus) menuAnchor.focus(); } });
    return m;
  }
  function closeMenu() { const m = $('dk-menu'); if (m) m.classList.remove('open'); }
  function openMenu(anchor, items) {
    const m = menuEl();
    if (m.classList.contains('open') && menuAnchor === anchor && !items.keep) { closeMenu(); return; }
    menuAnchor = anchor;
    m.innerHTML = '';
    items.forEach(it => {
      if (it === '-') { const d = document.createElement('div'); d.className = 'dk-msep'; m.appendChild(d); return; }
      if (it.head || it.hint) { const d = document.createElement('div'); d.className = it.head ? 'dk-mh' : 'dk-mhint'; d.textContent = it.head || it.hint; m.appendChild(d); return; }
      const b = document.createElement('button');
      b.type = 'button'; b.className = 'dk-mi'; b.setAttribute('role', it.check === undefined ? 'menuitem' : 'menuitemcheckbox');
      if (it.check !== undefined) b.setAttribute('aria-checked', it.check ? 'true' : 'false');
      b.disabled = !!it.off;
      b.innerHTML = `<span class="ico">${esc(it.check === undefined ? (it.ico || '') : it.check ? '☑' : '☐')}</span><span class="tx">${esc(it.label)}</span>${it.side ? `<span class="wh">${esc(it.side)}</span>` : ''}`;
      if (it.title) b.title = it.title;
      b.onclick = e => { e.stopPropagation(); if (!it.again) closeMenu(); it.do(); if (it.again) it.again(); };
      m.appendChild(b);
    });
    m.classList.add('open');
    // An anchor moved into the top bar's ⋯ menu has no box: open under ⋯ instead.
    let ar = anchor;
    if (!ar.getClientRects().length) ar = $('btn-more') && $('btn-more').getClientRects().length ? $('btn-more') : document.body;
    const r = ar === document.body ? {left: window.innerWidth / 2, right: window.innerWidth / 2 + 120, top: 60, bottom: 60} : ar.getBoundingClientRect(), mw = m.offsetWidth, mh = m.offsetHeight;
    let x = Math.min(window.innerWidth - mw - 8, Math.max(8, r.right - mw)), y = r.bottom + 4;
    if (y + mh > window.innerHeight - 8) y = Math.max(8, r.top - mh - 4);
    m.style.left = Math.max(8, x) + 'px'; m.style.top = y + 'px';
    const first = m.querySelector('button:not(:disabled)');
    if (first) first.focus();
  }
  function groupMenu(anchor, g) {
    if (!S) return;
    const id = activeOf(g) || g.a, w = where(id), lk = S.locked, name = nameOf(id), dyn = !!(P[id] && P[id].dyn);
    if (!P[id]) return;
    const items = [{head: P[id].ico + ' ' + name}];
    if (!compact()) {
      [['left', '⇤', 'dk_move_left'], ['right', '⇥', 'dk_move_right'], ['bottom', '⤓', 'dk_move_bottom'], ['float', '⧉', 'dk_float']].forEach(([z, ico, k]) =>
        items.push({ico, label: t(k), off: lk || (w === z && z !== 'float') || (w === 'float' && z === 'float'), title: lk ? t('dk_locked_t') : '', do: () => moveTo(id, z)}));
      items.push(dyn ? {ico: '▤', label: t('dk_wb_back'), do: () => hide(id)} : {ico: '–', label: tn('dk_hide', {name}), do: () => hide(id)});
      items.push('-');
    }
    items.push({ico: lk ? '🔓' : '🔒', label: t(lk ? 'dk_unlock' : 'dk_lock'), off: compact(), do: () => setLocked(!lk)});
    items.push({ico: '▦', label: t('dk_panels') + '…', do: () => setTimeout(() => openPanels(anchor), 0)});
    items.push({ico: '↺', label: t('dk_reset'), off: compact() || lk, do: reset});
    openMenu(anchor, items);
  }
  function openPanels(anchor) {
    if (!S) return;
    anchor = anchor || $('btn-panels');
    const items = [{head: '▦ ' + t('dk_panels')}];
    IDS.forEach(id => {
      const w = where(id), on = w !== 'hidden', ok = avail(id) || !on;
      items.push({check: on, label: P[id].ico + ' ' + t(P[id].key), side: !avail(id) && on && P[id].auto ? t('dk_hint_auto') : t('dk_where_' + w),
        off: compact() || (!ok && !P[id].auto), do: () => (on ? hide(id) : show(id)), again: () => openPanels(anchor)});
    });
    const dyn = allIds().filter(id => P[id].dyn && where(id) !== 'hidden');
    if (dyn.length) {
      items.push({head: t('dk_wb_head')});
      dyn.forEach(id => items.push({check: true, label: P[id].ico + ' ' + nameOf(id), side: t('dk_where_' + where(id)), title: t('dk_wb_back'),
        off: compact(), do: () => hide(id), again: () => openPanels(anchor)}));
    }
    items.push('-');
    items.push({ico: S.locked ? '🔓' : '🔒', label: t(S.locked ? 'dk_unlock' : 'dk_lock'), off: compact(), do: () => setLocked(!S.locked), again: () => openPanels(anchor)});
    items.push({ico: '↺', label: t('dk_reset'), off: compact() || S.locked, do: reset});
    items.push({hint: t(S.locked ? 'dk_locked_hint' : 'dk_panels_hint')});
    items.keep = $('dk-menu') && $('dk-menu').classList.contains('open') && menuAnchor === anchor;
    openMenu(anchor, items);
  }

  // ── styles ──
  const css = document.createElement('style');
  css.textContent = `
#dz-left, #dz-right { display: flex; flex-direction: column; flex: 1; min-height: 0; min-width: 0; overflow: hidden; height: 100%; }
#dz-bottom { display: none; flex: none; position: relative; min-height: 100px; border-top: 1px solid var(--border2); background: var(--bg2); }
#dz-bottom.on { display: flex; }
#dz-bottom-g { display: flex; flex: 1; min-width: 0; min-height: 0; }
#dz-bottom .dz-resize { position: absolute; left: 0; right: 0; top: -3px; height: 6px; cursor: ns-resize; z-index: 5; }
#dz-bottom .dz-resize:hover { background: var(--accent); }
#sidebar.dz-empty, #sess-panel.dz-empty { display: none !important; }
@media (min-width: 760px) { #sidebar, #sess-panel { max-width: none; } }
.dk-group { display: flex; flex-direction: column; min-height: 64px; min-width: 0; overflow: hidden; }
#dz-bottom-g .dk-group { min-width: 180px; min-height: 0; }
#dz-bottom-g .dk-group + .dk-split + .dk-group, #dz-bottom-g .dk-group { border: 0; }
.dk-tabs { display: flex; align-items: center; gap: 2px; padding: 5px 6px; border-bottom: 1px solid var(--border); flex: none; min-height: 32px; }
.dk-tl { position: relative; display: flex; align-items: center; gap: 2px; flex: 1; min-width: 0; overflow-x: auto; scrollbar-width: none; align-self: stretch; }
.dk-tl::-webkit-scrollbar { display: none; }
#dz-left > .dk-group:first-child > .dk-tabs { padding-right: 34px; }
#dz-right > .dk-group:first-child > .dk-tabs { padding-left: 34px; }
.dk-tab { background: transparent; border: none; color: var(--text3); font: 700 9.5px var(--font-mono); letter-spacing: .08em; text-transform: uppercase; padding: 4px 7px; border-radius: 5px; cursor: pointer; white-space: nowrap; display: inline-flex; align-items: center; gap: 4px; flex: none; touch-action: none; }
.dk-tab:hover { background: var(--bg3); color: var(--text2); }
.dk-tab.on { background: var(--bg3); color: var(--text); box-shadow: inset 0 -2px 0 var(--accent); }
.dk-tab .ico { font-size: 11px; letter-spacing: 0; }
.dk-tab .rp-badge { margin-left: 0; }
.dk-tab .rp-badge:empty { display: none; }
.dk-fill { flex: 1; align-self: stretch; min-width: 6px; }
.dk-float .dk-fill { cursor: move; }
.dk-mbtn { background: transparent; border: 1px solid transparent; color: var(--text3); width: 22px; height: 22px; padding: 0; border-radius: 5px; font-size: 13px; line-height: 1; flex: none; cursor: pointer; }
.dk-mbtn:hover { background: var(--bg3); color: var(--text); border-color: var(--border2); }
.dk-body { flex: 1; min-height: 0; min-width: 0; display: flex; flex-direction: column; overflow: hidden; }
.dk-body > .dk-pnl { display: none !important; }
.dk-body > .dk-pnl.dk-on { display: flex !important; flex: 1 1 auto; min-height: 0; min-width: 0; flex-direction: column; overflow: hidden; height: auto; max-height: none; opacity: 1; }
.dk-body > .clip-panel { border-top: 0; }
.dk-body > .clip-panel .clip-header { justify-content: flex-end; border-bottom: 0; padding: 4px 8px 0; }
.dk-body > .clip-panel .clip-header > span { display: none; }
.pnl-pad { padding: 8px 10px; border-bottom: 1px solid var(--border); flex: none; }
.pnl-pad:empty, .pnl-pad.off { display: none; }
.dk-split { flex: none; height: 5px; cursor: ns-resize; background: var(--border); transition: background .15s; touch-action: none; }
.dk-split.h { height: auto; width: 5px; cursor: ew-resize; }
.dk-split:hover { background: var(--accent); }
body.dk-locked .dk-split { cursor: default; pointer-events: none; }
body.dk-locked .sidebar-resize-handle, body.dk-locked .sess-resize-handle, body.dk-locked #dz-bottom .dz-resize { display: none; }
.dk-float { position: absolute; display: flex; flex-direction: column; background: var(--bg2); border: 1px solid var(--border2); border-radius: 10px; box-shadow: 0 16px 44px rgba(0,0,0,.55); resize: both; overflow: hidden; min-width: 220px; min-height: 140px; max-width: 100%; max-height: 100%; }
.dk-float > .dk-group { flex: 1; }
body.dk-locked .dk-float { resize: none; }
body.dk-locked .dk-float .dk-fill { cursor: default; }
.dk-ghost { position: fixed; z-index: 9800; pointer-events: none; padding: 5px 10px; background: var(--accent); color: #fff; border-radius: 6px; font-size: 11px; font-weight: 700; box-shadow: 0 6px 18px rgba(0,0,0,.45); white-space: nowrap; }
.dk-ind { position: fixed; z-index: 9790; pointer-events: none; display: none; border: 2px solid var(--accent); background: rgba(var(--accent-rgb),.18); border-radius: 8px; }
.dk-ind.fl { border-style: dashed; background: rgba(var(--accent-rgb),.08); }
.dk-strip { position: fixed; z-index: 9785; pointer-events: none; display: none; align-items: center; justify-content: center; border: 2px dashed var(--accent); background: rgba(var(--accent-rgb),.08); border-radius: 8px; color: var(--text); font-size: 11px; font-weight: 700; text-align: center; }
.dk-strip.v { writing-mode: vertical-rl; }
body.dk-dragging .dk-strip { display: flex; }
.dk-strip.hot { background: rgba(var(--accent-rgb),.32); }
body.dk-dragging, body.dk-dragging * { user-select: none !important; cursor: grabbing !important; }
body.dk-dragging iframe, body.dk-dragging canvas, body.dk-resizing iframe, body.dk-resizing canvas { pointer-events: none; }
body.dk-resizing, body.dk-resizing * { user-select: none !important; }
.dk-menu { position: fixed; z-index: 9700; display: none; min-width: 230px; max-width: min(340px, calc(100vw - 16px)); max-height: calc(100vh - 16px); overflow: auto; background: var(--bg2); border: 1px solid var(--border2); border-radius: 9px; box-shadow: 0 14px 40px rgba(0,0,0,.55); padding: 5px; }
.dk-menu.open { display: block; }
.dk-mi { display: flex; width: 100%; align-items: center; gap: 8px; background: transparent; border: none; color: var(--text); padding: 6px 9px; border-radius: 6px; font-size: 12px; font-weight: 500; text-align: left; cursor: pointer; }
.dk-mi:hover:not(:disabled), .dk-mi:focus-visible { background: var(--bg3); }
.dk-mi:disabled { opacity: .45; cursor: default; }
.dk-mi .ico { width: 18px; text-align: center; flex: none; }
.dk-mi .tx { flex: 1; min-width: 0; }
.dk-mi .wh { color: var(--text3); font-size: 11px; text-align: right; max-width: 48%; }
.dk-mh { padding: 6px 9px 4px; font-size: 10px; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--text3); }
.dk-msep { height: 1px; background: var(--border); margin: 4px 2px; }
.dk-mhint { padding: 6px 9px; font-size: 11px; color: var(--text3); line-height: 1.45; }
#btn-panels.dk-is-locked { color: var(--yellow); }
@media (max-width: 759px) { #btn-panels { display: none !important; } .dk-mbtn { display: none; } .dk-tab { touch-action: auto; } #dz-left > .dk-group:first-child > .dk-tabs, #dz-right > .dk-group:first-child > .dk-tabs { padding-left: 6px; padding-right: 6px; } }
`;
  document.head.appendChild(css);

  (function wire() {
    const h = document.querySelector('#dz-bottom .dz-resize');
    if (h) h.addEventListener('pointerdown', bottomResize);
    let wasCompact = compact(), rt = 0;
    window.addEventListener('resize', () => {
      clearTimeout(rt);
      rt = setTimeout(() => {
        if (!S) return;
        if (compact() !== wasCompact) { wasCompact = compact(); compactL = null; render(); return; }
        clampFloats();
        update(cur());
      }, 60);
    });
  })();

  window.wrmLayout = {
    init, refresh: render, activate, moveTo, hide, show, reset, openPanels, closeMenu, zoneResized, zoneMax, where, isShown,
    isHidden: id => where(id) === 'hidden', locked: () => !!(S && S.locked && !compact()), setLocked, ready: () => !!S,
    register, remove, placed, clampFloats, snapshot, apply, menu: openMenu,
    _state: () => S, _sanitize: sanitize,
  };
})();
