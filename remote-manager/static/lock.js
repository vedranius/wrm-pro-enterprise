/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// Screen lock (#115, v13.7). After a period without activity, when the window is hidden, when
// the computer locks or sleeps, on a keyboard shortcut or on "Lock now" the interface is
// covered by a lock screen that asks for the signed-in account's password (or a personal PIN).
// Sessions, tunnels and AI runs keep going; the lock only detaches the view. The server
// remembers the lock per sign-in session (/api/auth/lock), so reloading the page or opening a
// new tab shows the lock again, and every unlock attempt is checked and audited
// (/api/auth/lock/verify). Preferences live in Settings → Security and follow the user
// (/api/me/ui/lock); the administrator can cap the idle time and turn the PIN off.
'use strict';

Object.assign(LANGS.en, {
  lock_title: 'WRM PRO is locked',
  lock_hint: 'Enter your password to continue. Your sessions keep running in the background.',
  lock_password: 'Password',
  lock_pin: 'PIN',
  lock_unlock: 'Unlock',
  lock_now: 'Lock now',
  lock_after: 'Lock after inactivity',
  lock_off: 'Off',
  lock_minutes: '{n} min',
  lock_wrong: 'The password is not correct',
  lock_wrong_pin: 'The PIN is not correct',
  lock_throttled: 'Too many attempts; wait a minute',
  lock_signed_out: 'Too many wrong attempts: you were signed out.',
  lock_note: 'This is a screen lock, not a new sign-in: the browser session stays valid.',
  lock_running: '{n} open windows keep running',
  lock_sign_out: 'Sign out instead',
  lock_sso_hint: 'Your account signs in through a directory provider and has no password here. Set an unlock PIN in Settings → Security, or sign out and sign in again.',
  lock_warn: 'WRM locks in {n} s. Move the mouse or press a key to stay.',
  lock_queued: '{n} notifications arrived while WRM was locked',
  lock_notify_hidden: 'New notification (WRM is locked)',
  lock_title_tab: 'WRM PRO (locked)',
  // Settings → Security
  lk_h: 'Screen lock',
  lk_intro: 'The lock covers WRM and asks for your password. Everything keeps running in the background: terminals, file transfers, tunnels, AI and MCP sessions. Only the view is hidden.',
  lk_now_d: 'Lock WRM right away, for example before you leave the desk.',
  lk_idle: 'Lock after inactivity',
  lk_idle_d: 'No mouse or keyboard activity in any WRM tab for this long.',
  lk_idle_max: 'Your administrator allows at most {n} min.',
  lk_custom: 'Custom…',
  lk_custom_q: 'Minutes without activity (1–1440):',
  lk_hidden: 'Lock when WRM is not visible',
  lk_hidden_d: 'Another application or browser tab is in front, or the window is minimised (another visible WRM tab counts as visible).',
  lk_hidden_now: 'Immediately',
  lk_hidden_after: 'After {n} min',
  lk_os: 'Lock when the computer locks',
  lk_os_d: 'Uses the browser\'s idle detection (Chrome, Edge). The browser asks for permission once.',
  lk_os_na: 'This browser does not support it.',
  lk_os_denied: 'The browser did not allow idle detection.',
  lk_sleep: 'Lock after the computer sleeps',
  lk_sleep_d: 'When the computer wakes up after sleep or hibernation.',
  lk_start: 'Lock when WRM is opened',
  lk_start_d: 'Every new tab and every reload starts locked.',
  lk_move: 'Mouse movement counts as activity',
  lk_move_d: 'Off: only clicks, scrolling and keys keep WRM unlocked.',
  lk_warn: 'Warn before locking',
  lk_warn_d: 'A countdown before the inactivity lock.',
  lk_warn_s: '{n} s',
  lk_shortcut: 'Keyboard shortcut',
  lk_shortcut_d: 'Locks WRM from anywhere in the page, terminals included.',
  lk_look: 'Lock screen',
  lk_bg: 'Background',
  lk_bg_blur: 'Blurred',
  lk_bg_dim: 'Dimmed',
  lk_bg_solid: 'Solid (nothing visible)',
  lk_clock: 'Show the clock',
  lk_user: 'Show my name',
  lk_count: 'Show how many windows keep running',
  lk_tab_title: 'Hide the page title',
  lk_tab_title_d: 'The browser tab shows "WRM PRO (locked)" instead of the server name.',
  lk_message: 'Message on the lock screen',
  lk_message_ph: 'For example: back at 14:00, call 555-0100',
  lk_hide_toasts: 'Hold notifications while locked',
  lk_hide_toasts_d: 'Messages inside WRM wait until you unlock; desktop notifications do not show their text.',
  lk_pin_h: 'Unlock PIN',
  lk_pin_d: 'A short number (4–12 digits) to unlock instead of the password. Signing in still needs the password.',
  lk_pin_set: 'Set PIN',
  lk_pin_change: 'Change PIN',
  lk_pin_remove: 'Remove PIN',
  lk_pin_off: 'Your administrator turned the PIN off.',
  lk_pin_new: 'New PIN (4–12 digits)',
  lk_pin_again: 'PIN again',
  lk_pin_pass: 'Your current password',
  lk_pin_mismatch: 'The PINs do not match',
  lk_pin_saved: 'Unlock PIN saved',
  lk_pin_removed: 'Unlock PIN removed',
  lk_admin: 'Set by your administrator: {what}',
  lk_mode_server: 'while locked, this browser cannot reach WRM data until it is unlocked',
  lk_mode_remember: 'the lock survives a reload',
  lk_mode_browser: 'the lock lives only in this page',
  lk_saved: 'Saved',
  // Admin → Policies
  pg_screen_lock: 'Screen lock',
  p_screen_lock_mode: 'How strict the screen lock is',
  p_screen_lock_mode_d: 'Server: while locked, the browser cannot reach WRM data and a reload stays locked. Remember: a reload stays locked, the page keeps refreshing underneath. Browser: only the page covers itself. Running sessions keep going in every mode.',
  po_screen_lock_mode_server: 'Server (strict)',
  po_screen_lock_mode_remember: 'Remember the lock',
  po_screen_lock_mode_browser: 'Browser only',
  p_screen_lock_max_minutes: 'Longest idle time before the lock (minutes)',
  p_screen_lock_max_minutes_d: 'Users cannot turn the idle lock off or choose a longer time. 0 = the user decides.',
  p_screen_lock_pin: 'Allow unlocking with a PIN',
  p_screen_lock_pin_d: 'Users may set a personal 4–12 digit PIN that unlocks the screen (never signs in).',
  p_screen_lock_max_failures: 'Sign out after wrong unlock attempts',
  p_screen_lock_max_failures_d: 'After this many wrong passwords or PINs on the lock screen the browser session ends. 0 = never.',
});

Object.assign(LANGS.hr, {
  lock_title: 'WRM PRO je zaključan',
  lock_hint: 'Upišite lozinku za nastavak. Sesije nastavljaju raditi u pozadini.',
  lock_password: 'Lozinka',
  lock_pin: 'PIN',
  lock_unlock: 'Otključaj',
  lock_now: 'Zaključaj sada',
  lock_after: 'Zaključaj nakon neaktivnosti',
  lock_off: 'Isključeno',
  lock_minutes: '{n} min',
  lock_wrong: 'Lozinka nije ispravna',
  lock_wrong_pin: 'PIN nije ispravan',
  lock_throttled: 'Previše pokušaja; pričekajte minutu',
  lock_signed_out: 'Previše pogrešnih pokušaja: odjavljeni ste.',
  lock_note: 'Ovo je zaključavanje zaslona, ne nova prijava: sesija preglednika ostaje valjana.',
  lock_running: 'Otvorenih prozora koji rade dalje: {n}',
  lock_sign_out: 'Radije se odjavi',
  lock_sso_hint: 'Vaš se račun prijavljuje preko imeničkog servisa i ovdje nema lozinku. Postavite PIN za otključavanje u Postavke → Sigurnost ili se odjavite i ponovno prijavite.',
  lock_warn: 'WRM se zaključava za {n} s. Pomaknite miš ili pritisnite tipku da ostane otključan.',
  lock_queued: 'Obavijesti pristigle dok je WRM bio zaključan: {n}',
  lock_notify_hidden: 'Nova obavijest (WRM je zaključan)',
  lock_title_tab: 'WRM PRO (zaključan)',
  lk_h: 'Zaključavanje zaslona',
  lk_intro: 'Zaključavanje prekrije WRM i traži vašu lozinku. Sve ostalo radi dalje u pozadini: terminali, prijenosi datoteka, tuneli, AI i MCP sesije. Skriven je samo prikaz.',
  lk_now_d: 'Odmah zaključaj WRM, na primjer prije nego što odete od stola.',
  lk_idle: 'Zaključaj nakon neaktivnosti',
  lk_idle_d: 'Toliko dugo bez miša i tipkovnice ni u jednoj WRM kartici.',
  lk_idle_max: 'Administrator dopušta najviše {n} min.',
  lk_custom: 'Prilagođeno…',
  lk_custom_q: 'Minuta bez aktivnosti (1–1440):',
  lk_hidden: 'Zaključaj kad WRM nije vidljiv',
  lk_hidden_d: 'Ispred je druga aplikacija ili kartica preglednika, ili je prozor smanjen (druga vidljiva WRM kartica se računa kao vidljiv WRM).',
  lk_hidden_now: 'Odmah',
  lk_hidden_after: 'Nakon {n} min',
  lk_os: 'Zaključaj kad se računalo zaključa',
  lk_os_d: 'Koristi prepoznavanje neaktivnosti preglednika (Chrome, Edge). Preglednik jednom traži dopuštenje.',
  lk_os_na: 'Ovaj preglednik to ne podržava.',
  lk_os_denied: 'Preglednik nije dopustio prepoznavanje neaktivnosti.',
  lk_sleep: 'Zaključaj nakon spavanja računala',
  lk_sleep_d: 'Kad se računalo probudi iz stanja spavanja ili hibernacije.',
  lk_start: 'Zaključaj pri otvaranju WRM-a',
  lk_start_d: 'Svaka nova kartica i svako osvježavanje počinje zaključano.',
  lk_move: 'Pomicanje miša se računa kao aktivnost',
  lk_move_d: 'Isključeno: WRM drže otključanim samo klikovi, pomicanje kotačićem i tipke.',
  lk_warn: 'Upozori prije zaključavanja',
  lk_warn_d: 'Odbrojavanje prije zaključavanja zbog neaktivnosti.',
  lk_warn_s: '{n} s',
  lk_shortcut: 'Prečac na tipkovnici',
  lk_shortcut_d: 'Zaključava WRM s bilo kojeg mjesta na stranici, uključujući terminale.',
  lk_look: 'Zaključani zaslon',
  lk_bg: 'Pozadina',
  lk_bg_blur: 'Zamućena',
  lk_bg_dim: 'Zatamnjena',
  lk_bg_solid: 'Puna (ništa se ne vidi)',
  lk_clock: 'Prikaži sat',
  lk_user: 'Prikaži moje ime',
  lk_count: 'Prikaži koliko prozora radi dalje',
  lk_tab_title: 'Sakrij naslov stranice',
  lk_tab_title_d: 'Kartica preglednika prikazuje „WRM PRO (zaključan)” umjesto imena servera.',
  lk_message: 'Poruka na zaključanom zaslonu',
  lk_message_ph: 'Na primjer: vraćam se u 14:00, zovite 555-0100',
  lk_hide_toasts: 'Zadrži obavijesti dok je zaključano',
  lk_hide_toasts_d: 'Poruke unutar WRM-a čekaju otključavanje; obavijesti na radnoj površini ne prikazuju tekst.',
  lk_pin_h: 'PIN za otključavanje',
  lk_pin_d: 'Kratki broj (4–12 znamenki) za otključavanje umjesto lozinke. Za prijavu i dalje treba lozinka.',
  lk_pin_set: 'Postavi PIN',
  lk_pin_change: 'Promijeni PIN',
  lk_pin_remove: 'Ukloni PIN',
  lk_pin_off: 'Administrator je isključio PIN.',
  lk_pin_new: 'Novi PIN (4–12 znamenki)',
  lk_pin_again: 'PIN ponovno',
  lk_pin_pass: 'Vaša trenutna lozinka',
  lk_pin_mismatch: 'PIN-ovi se ne podudaraju',
  lk_pin_saved: 'PIN za otključavanje spremljen',
  lk_pin_removed: 'PIN za otključavanje uklonjen',
  lk_admin: 'Postavio administrator: {what}',
  lk_mode_server: 'dok je zaključano, ovaj preglednik ne dohvaća WRM podatke dok se ne otključa',
  lk_mode_remember: 'zaključavanje ostaje i nakon osvježavanja',
  lk_mode_browser: 'zaključavanje postoji samo u ovoj stranici',
  lk_saved: 'Spremljeno',
  pg_screen_lock: 'Zaključavanje zaslona',
  p_screen_lock_mode: 'Koliko je strogo zaključavanje zaslona',
  p_screen_lock_mode_d: 'Server: dok je zaključano, preglednik ne dohvaća WRM podatke, a osvježavanje ostaje zaključano. Pamti: osvježavanje ostaje zaključano, stranica ispod nastavlja osvježavati podatke. Preglednik: samo se stranica prekrije. Pokrenute sesije rade dalje u svakom načinu.',
  po_screen_lock_mode_server: 'Server (strogo)',
  po_screen_lock_mode_remember: 'Pamti zaključavanje',
  po_screen_lock_mode_browser: 'Samo preglednik',
  p_screen_lock_max_minutes: 'Najdulja neaktivnost prije zaključavanja (minuta)',
  p_screen_lock_max_minutes_d: 'Korisnici ne mogu isključiti zaključavanje ni odabrati dulje vrijeme. 0 = odlučuje korisnik.',
  p_screen_lock_pin: 'Dopusti otključavanje PIN-om',
  p_screen_lock_pin_d: 'Korisnici mogu postaviti osobni PIN od 4–12 znamenki koji otključava zaslon (nikad ne prijavljuje).',
  p_screen_lock_max_failures: 'Odjava nakon pogrešnih pokušaja otključavanja',
  p_screen_lock_max_failures_d: 'Nakon toliko pogrešnih lozinki ili PIN-ova na zaključanom zaslonu sesija preglednika završava. 0 = nikad.',
});

(function () {
  const $ = id => document.getElementById(id);
  const tf = (k, o) => t(k).replace(/\{(\w+)\}/g, (m, x) => (o && o[x] !== undefined ? o[x] : m));
  const PREFS_KEY = 'wrm_lock_prefs';
  const LEGACY_KEY = 'wrm_lock_minutes'; // v13.2 kept only the idle minutes here
  const ACT_KEY = 'wrm_lock_last';
  const VIS_KEY = 'wrm_lock_visible';
  const DEFAULTS = {
    idle: 5, hidden: -1, os: false, sleep: true, start: false, move: true, warn: 30, shortcut: 'ctrl+alt+l',
    bg: 'blur', clock: true, user: true, count: true, tabTitle: true, message: '', holdToasts: true,
  };
  const SHORTCUTS = ['off', 'ctrl+alt+l', 'ctrl+shift+l', 'alt+shift+l', 'ctrl+alt+k'];
  let prefs = loadLocal();
  let last = Date.now(), lastTick = Date.now(), hiddenAt = 0, timer = null, overlay = null, busy = false;
  let warnEl = null, booted = false, deferredInit = null, savedTitle = '', queued = [], idleDetector = null;
  let chan = null;
  try { chan = new BroadcastChannel('wrm-lock'); } catch (_) { /* old browser: tabs lock on their own */ }

  function store(k, v) { try { localStorage.setItem(k, v); } catch (_) { /* private mode */ } }
  function read(k) { try { return localStorage.getItem(k); } catch (_) { return null; } }

  function loadLocal() {
    let p = {};
    try { p = JSON.parse(read(PREFS_KEY) || '{}') || {}; } catch (_) { p = {}; }
    const legacy = parseInt(read(LEGACY_KEY), 10);
    if (p.idle === undefined && Number.isFinite(legacy)) p.idle = legacy;
    return Object.assign({}, DEFAULTS, p);
  }

  function policy() { return (typeof currentUser !== 'undefined' && currentUser && currentUser.screen_lock) || {mode: 'server', max_minutes: 0, pin_allowed: true, has_pin: false}; }

  // minutes(): the idle lock after the administrator's cap.
  function minutes() {
    let m = parseInt(prefs.idle, 10);
    if (!Number.isFinite(m) || m < 0) m = DEFAULTS.idle;
    const max = policy().max_minutes || 0;
    if (max > 0 && (m === 0 || m > max)) m = max;
    return Math.min(m, 1440);
  }

  function signedIn() { return typeof currentUser !== 'undefined' && !!currentUser; }

  let lastWrite = 0;
  function touch(e) {
    if (e && e.type === 'mousemove' && !prefs.move) return;
    if (overlay) return;
    last = Date.now();
    if (last - lastWrite > 4000) { lastWrite = last; store(ACT_KEY, String(last)); }
    if (warnEl) { warnEl.remove(); warnEl = null; }
  }

  function lastActivity() {
    const shared = parseInt(read(ACT_KEY), 10);
    return Number.isFinite(shared) ? Math.max(last, shared) : last;
  }

  function otherTabVisible() {
    const v = parseInt(read(VIS_KEY), 10);
    return Number.isFinite(v) && Date.now() - v < 8000;
  }

  // ── the lock screen ───────────────────────────────
  function runningCount() {
    try { return (typeof winObjects !== 'undefined' && winObjects.length) || 0; } catch (_) { return 0; }
  }

  function clockText() {
    const d = new Date();
    const lang = typeof currentLang !== 'undefined' && currentLang === 'hr' ? 'hr-HR' : undefined;
    return {time: d.toLocaleTimeString(lang, {hour: '2-digit', minute: '2-digit'}), date: d.toLocaleDateString(lang, {weekday: 'long', day: 'numeric', month: 'long'})};
  }

  function render(why) {
    const pol = policy();
    const me = signedIn() ? currentUser : {};
    const usePin = pol.has_pin && pol.pin_allowed;
    const noPassword = me.sso_only && !me.directory_password;
    const bg = prefs.bg === 'solid' ? 'background:var(--bg,#0a0c11);' : prefs.bg === 'dim' ? 'background:rgba(4,6,10,.9);' : 'background:rgba(6,8,12,.82);backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);';
    overlay.className = 'lock-screen lock-bg-' + prefs.bg;
    overlay.style.cssText = 'position:fixed;inset:0;z-index:2147483000;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:24px;gap:18px;' + bg;
    const c = clockText();
    overlay.innerHTML = `
      ${prefs.clock ? `<div class="lock-clock" aria-hidden="true" style="text-align:center;color:#fff;text-shadow:0 2px 12px #0008;"><div id="lock-time" style="font-size:56px;font-weight:300;letter-spacing:1px;line-height:1;">${esc(c.time)}</div><div id="lock-date" style="font-size:14px;opacity:.85;margin-top:6px;">${esc(c.date)}</div></div>` : ''}
      <div role="dialog" aria-modal="true" aria-labelledby="lock-h" style="width:380px;max-width:100%;background:var(--bg2,#11151c);border:1px solid var(--border2,#2a3240);border-radius:12px;padding:22px;color:var(--text,#e5e7eb);box-shadow:0 20px 60px #000a;font:13px/1.45 var(--font-ui,system-ui,sans-serif);">
        <div style="display:flex;align-items:center;gap:10px;margin-bottom:8px;">
          <div style="font-size:24px;">🔒</div>
          <div style="flex:1;min-width:0;"><h3 id="lock-h" style="margin:0;font-size:16px;">${esc(t('lock_title'))}</h3>
            ${prefs.user && me.username ? `<div style="color:var(--text2,#97a0b0);font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${esc(me.display_name || me.username)}${me.display_name && me.display_name !== me.username ? ' (' + esc(me.username) + ')' : ''}</div>` : ''}</div>
        </div>
        ${prefs.message ? `<div style="background:var(--bg3,#161a23);border-left:3px solid var(--accent,#4f8cff);padding:8px 10px;border-radius:6px;margin-bottom:10px;white-space:pre-wrap;overflow-wrap:anywhere;">${esc(prefs.message)}</div>` : ''}
        <div style="color:var(--text2,#97a0b0);font-size:12px;margin-bottom:12px;">${esc(noPassword && !usePin ? t('lock_sso_hint') : t('lock_hint'))}</div>
        ${usePin && !noPassword ? `<div class="lock-tabs" role="tablist" style="display:flex;gap:4px;margin-bottom:8px;">
          <button type="button" class="btn-sec btn-sm" role="tab" id="lock-use-pin" aria-selected="true">${esc(t('lock_pin'))}</button>
          <button type="button" class="btn-sec btn-sm" role="tab" id="lock-use-pass" aria-selected="false">${esc(t('lock_password'))}</button></div>` : ''}
        ${noPassword && !usePin ? '' : `<label for="lock-pass" id="lock-pass-l" style="font-size:12px;color:var(--text2,#97a0b0);">${esc(usePin ? t('lock_pin') : t('lock_password'))}</label>
        <input id="lock-pass" type="password" ${usePin ? 'inputmode="numeric" autocomplete="off" data-pin="1"' : 'autocomplete="current-password"'} style="width:100%;margin:4px 0 10px;padding:9px;border-radius:6px;border:1px solid var(--border2,#2a3240);background:var(--bg3,#0b0f15);color:var(--text,#e5e7eb);font-size:14px;">
        <div id="lock-err" role="alert" style="color:var(--red-t,#fca5a5);font-size:12px;min-height:16px;margin-bottom:8px;"></div>
        <button id="lock-go" class="btn-primary" style="width:100%;padding:9px;">${esc(t('lock_unlock'))}</button>`}
        <button id="lock-out" class="btn-sec btn-sm" style="width:100%;margin-top:8px;">${esc(t('lock_sign_out'))}</button>
        ${prefs.count && runningCount() ? `<div style="margin-top:12px;font-size:12px;color:var(--text2,#97a0b0);">▶ ${esc(tf('lock_running', {n: runningCount()}))}</div>` : ''}
        <div style="margin-top:8px;font-size:11.5px;color:var(--text3,#5a6376);">${esc(t('lock_note'))}</div>
      </div>`;
    const inp = $('lock-pass');
    if (inp) {
      inp.focus();
      inp.addEventListener('keydown', e => { if (e.key === 'Enter') unlock(); });
      $('lock-go').addEventListener('click', unlock);
    }
    const pinTab = $('lock-use-pin'), passTab = $('lock-use-pass');
    const setMode = pin => {
      if (!inp) return;
      inp.value = '';
      inp.dataset.pin = pin ? '1' : '';
      inp.setAttribute('inputmode', pin ? 'numeric' : 'text');
      inp.setAttribute('autocomplete', pin ? 'off' : 'current-password');
      $('lock-pass-l').textContent = pin ? t('lock_pin') : t('lock_password');
      pinTab.setAttribute('aria-selected', pin ? 'true' : 'false');
      passTab.setAttribute('aria-selected', pin ? 'false' : 'true');
      pinTab.classList.toggle('active', pin); passTab.classList.toggle('active', !pin);
      inp.focus();
    };
    if (pinTab) { pinTab.classList.add('active'); pinTab.onclick = () => setMode(true); passTab.onclick = () => setMode(false); }
    $('lock-out').addEventListener('click', () => { if (typeof doLogout === 'function') doLogout(); else location.reload(); });
    overlay.dataset.why = why || '';
  }

  // show(): cover the page. post=true tells the server and the other tabs.
  function show(reason, post) {
    if (overlay || !signedIn()) return;
    if (warnEl) { warnEl.remove(); warnEl = null; }
    overlay = document.createElement('div');
    overlay.id = 'lock-screen';
    document.body.appendChild(overlay);
    render(reason);
    if (prefs.tabTitle) { savedTitle = document.title; document.title = t('lock_title_tab'); }
    window.wrmLock.locked = true;
    if (post) {
      fetch('/api/auth/lock', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({reason})}).catch(() => {});
      if (chan) chan.postMessage({t: 'lock'});
    }
  }

  function lock(reason) { show(typeof reason === 'string' ? reason : 'manual', true); }

  function hide() {
    if (overlay) overlay.remove();
    overlay = null;
    window.wrmLock.locked = false;
    if (savedTitle) { document.title = savedTitle; savedTitle = ''; }
    last = Date.now(); store(ACT_KEY, String(last));
    if (deferredInit) { const f = deferredInit; deferredInit = null; f(); }
    if (queued.length && typeof window.showToast === 'function') {
      const n = queued.length, list = queued.slice(-4);
      queued = [];
      list.forEach(q => origToast(q[0], q[1], q[2]));
      if (n > list.length) origToast(tf('lock_queued', {n}), 'info', 5000);
    }
  }

  async function unlock() {
    if (busy) return;
    const inp = $('lock-pass');
    const secret = (inp || {}).value || '';
    const pin = !!(inp && inp.dataset.pin);
    const err = $('lock-err'), btn = $('lock-go');
    if (!secret) { if (err) err.textContent = pin ? t('lock_pin') : t('lock_password'); return; }
    busy = true;
    if (btn) btn.disabled = true;
    try {
      const res = await fetch('/api/auth/lock/verify', {
        method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(pin ? {pin: secret} : {password: secret})
      });
      if (res.status === 401) {
        const d = await res.json().catch(() => ({}));
        if (d.signed_out) { if (err) err.textContent = t('lock_signed_out'); setTimeout(() => location.reload(), 1800); return; }
      }
      if (!res.ok) throw new Error(res.status === 429 ? t('lock_throttled') : pin ? t('lock_wrong_pin') : t('lock_wrong'));
      hide();
      if (chan) chan.postMessage({t: 'unlock'});
      arm();
    } catch (e) {
      if (err) err.textContent = e.message;
      if (inp) { inp.value = ''; inp.focus(); }
    } finally {
      busy = false;
      if (btn) btn.disabled = false;
    }
  }

  // ── triggers ──────────────────────────────────────
  function warn(left) {
    if (!warnEl) {
      warnEl = document.createElement('div');
      warnEl.id = 'lock-warn';
      warnEl.setAttribute('role', 'status');
      warnEl.style.cssText = 'position:fixed;left:50%;top:56px;transform:translateX(-50%);z-index:2147482000;background:var(--bg3,#161a23);color:var(--text,#e7e9ef);border:1px solid var(--yellow,#f59e0b);border-radius:8px;padding:8px 14px;box-shadow:0 8px 28px #0007;font-size:13px;max-width:calc(100vw - 32px);';
      document.body.appendChild(warnEl);
    }
    warnEl.textContent = '🔒 ' + tf('lock_warn', {n: Math.max(1, Math.ceil(left / 1000))});
  }

  function tick() {
    const now = Date.now();
    const gap = now - lastTick;
    lastTick = now;
    if (!signedIn()) return;
    if (overlay) {
      if (prefs.clock) { const c = clockText(), el = $('lock-time'), d = $('lock-date'); if (el) el.textContent = c.time; if (d) d.textContent = c.date; }
      return;
    }
    if (!document.hidden) store(VIS_KEY, String(now));
    // Timers stop while the computer sleeps: a long gap means it slept.
    if (prefs.sleep && gap > 180000) { lock('sleep'); return; }
    if (document.hidden && prefs.hidden >= 0 && hiddenAt && !otherTabVisible() && now - hiddenAt >= prefs.hidden * 60000) { lock('hidden'); return; }
    const m = minutes();
    if (!m) return;
    const left = lastActivity() + m * 60000 - now;
    if (left <= 0) { lock('idle'); return; }
    const w = (parseInt(prefs.warn, 10) || 0) * 1000;
    if (w && left <= w && !document.hidden) warn(left);
    else if (warnEl) { warnEl.remove(); warnEl = null; }
  }

  function arm() {
    clearInterval(timer);
    lastTick = Date.now();
    timer = setInterval(tick, 1000);
  }

  function matchShortcut(e) {
    const s = prefs.shortcut;
    if (!s || s === 'off' || !e.key || e.key.toLowerCase() !== s.slice(-1)) return false;
    const want = s.split('+');
    return e.ctrlKey === want.includes('ctrl') && e.altKey === want.includes('alt') && e.shiftKey === want.includes('shift') && !e.metaKey;
  }

  async function startOSDetector(ask) {
    if (idleDetector) { try { idleDetector.abort(); } catch (_) { /* gone */ } idleDetector = null; }
    if (!prefs.os || !('IdleDetector' in window)) return 'na';
    try {
      if (ask && (await IdleDetector.requestPermission()) !== 'granted') return 'denied';
      const ctl = new AbortController();
      const d = new IdleDetector();
      d.addEventListener('change', () => { if (d.screenState === 'locked') lock('os_locked'); });
      await d.start({threshold: 60000, signal: ctl.signal});
      idleDetector = ctl;
      return 'ok';
    } catch (_) { return 'denied'; }
  }

  // ── holding notifications ─────────────────────────
  let origToast = null;
  function wrapToast() {
    if (origToast || typeof window.showToast !== 'function') return;
    origToast = window.showToast;
    window.showToast = function (msg, type, dur) {
      if (overlay && prefs.holdToasts) { queued.push([msg, type, dur]); if (queued.length > 50) queued.shift(); return; }
      return origToast.apply(this, arguments);
    };
    try { showToast = window.showToast; } catch (_) { /* not a global binding */ }
    if ('Notification' in window && !window.Notification.__lock) {
      const N = window.Notification;
      const Wrapped = function (title, opts) {
        if (overlay && prefs.holdToasts) return new N('WRM PRO', {body: t('lock_notify_hidden'), icon: (opts || {}).icon, tag: 'wrm-locked'});
        return new N(title, opts);
      };
      Object.defineProperty(Wrapped, 'permission', {get: () => N.permission});
      Wrapped.requestPermission = N.requestPermission.bind(N);
      Wrapped.prototype = N.prototype;
      Wrapped.__lock = true;
      window.Notification = Wrapped;
    }
  }

  // ── preferences ───────────────────────────────────
  async function loadPrefs() {
    try {
      const r = await fetch('/api/me/ui/lock');
      if (r.ok) {
        const d = await r.json();
        if (d && d.value) { prefs = Object.assign({}, DEFAULTS, d.value); store(PREFS_KEY, JSON.stringify(prefs)); }
      }
    } catch (_) { /* offline: keep the cached copy */ }
  }

  let saveTimer = null;
  function savePrefs(p) {
    prefs = Object.assign({}, prefs, p);
    store(PREFS_KEY, JSON.stringify(prefs));
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      fetch('/api/me/ui/lock', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({value: prefs})}).catch(() => {});
    }, 400);
  }

  // ── Settings → Security ───────────────────────────
  function sel(id, opts, val) {
    return `<select class="setting-select" id="${id}">${opts.map(o => `<option value="${esc(String(o[0]))}" ${String(o[0]) === String(val) ? 'selected' : ''}>${esc(o[1])}</option>`).join('')}</select>`;
  }
  function sw(id, on) { return `<label class="switch"><input type="checkbox" id="${id}" ${on ? 'checked' : ''}><span></span></label>`; }
  function row(label, desc, ctl, extra) {
    return `<div class="setting-row"><div><div class="setting-label">${esc(label)}</div>${desc ? `<div class="setting-desc">${esc(desc)}</div>` : ''}${extra || ''}</div><div class="setting-control">${ctl}</div></div>`;
  }

  function renderSettings(box) {
    if (!box) return;
    const pol = policy();
    const max = pol.max_minutes || 0;
    const cur = minutes();
    let idleOpts = [0, 1, 2, 5, 10, 15, 30, 60, 120, 240].filter(n => !max || (n > 0 && n <= max));
    if (!idleOpts.includes(cur)) idleOpts.push(cur);
    idleOpts.sort((a, b) => a - b);
    const idleSel = sel('lk-idle', idleOpts.map(n => [n, n ? tf('lock_minutes', {n}) : t('lock_off')]).concat([['custom', t('lk_custom')]]), cur);
    const hiddenSel = sel('lk-hidden', [[-1, t('lock_off')], [0, t('lk_hidden_now')], [1, tf('lk_hidden_after', {n: 1})], [5, tf('lk_hidden_after', {n: 5})], [15, tf('lk_hidden_after', {n: 15})]], prefs.hidden);
    const warnSel = sel('lk-warn', [[0, t('lock_off')], [10, tf('lk_warn_s', {n: 10})], [30, tf('lk_warn_s', {n: 30})], [60, tf('lk_warn_s', {n: 60})]], prefs.warn);
    const keyName = s => s === 'off' ? t('lock_off') : s.split('+').map(k => k.length === 1 ? k.toUpperCase() : k[0].toUpperCase() + k.slice(1)).join(' + ');
    const scSel = sel('lk-shortcut', SHORTCUTS.map(s => [s, keyName(s)]), prefs.shortcut);
    const bgSel = sel('lk-bg', [['blur', t('lk_bg_blur')], ['dim', t('lk_bg_dim')], ['solid', t('lk_bg_solid')]], prefs.bg);
    const modeText = t('lk_mode_' + (pol.mode || 'server'));
    const adminNote = `<div class="setting-desc" style="margin-top:4px;">🛡 ${esc(tf('lk_admin', {what: modeText}))}${max ? ' · ' + esc(tf('lk_idle_max', {n: max})) : ''}</div>`;
    const osSupported = 'IdleDetector' in window;
    const me = signedIn() ? currentUser : {};
    const pinCtl = !pol.pin_allowed ? `<span class="muted-text">${esc(t('lk_pin_off'))}</span>` :
      `<button class="btn-sec btn-sm" id="lk-pin-set">${esc(pol.has_pin ? t('lk_pin_change') : t('lk_pin_set'))}</button>${pol.has_pin ? ` <button class="btn-sec btn-sm" id="lk-pin-del">${esc(t('lk_pin_remove'))}</button>` : ''}`;
    box.innerHTML = `
      <div class="adm-h" style="display:flex;align-items:center;gap:8px;">🔒 ${esc(t('lk_h'))}<span style="flex:1"></span><button class="btn-sm" id="lk-now">🔒 ${esc(t('lock_now'))}</button></div>
      <div class="hint-box" style="margin-bottom:8px;">${esc(t('lk_intro'))}${adminNote}</div>
      ${row(t('lk_idle'), t('lk_idle_d'), idleSel)}
      ${row(t('lk_warn'), t('lk_warn_d'), warnSel)}
      ${row(t('lk_hidden'), t('lk_hidden_d'), hiddenSel)}
      ${row(t('lk_os'), osSupported ? t('lk_os_d') : t('lk_os_na'), osSupported ? sw('lk-os', prefs.os) : '', '<div class="setting-desc" id="lk-os-msg"></div>')}
      ${row(t('lk_sleep'), t('lk_sleep_d'), sw('lk-sleep', prefs.sleep))}
      ${row(t('lk_start'), t('lk_start_d'), sw('lk-start', prefs.start))}
      ${row(t('lk_move'), t('lk_move_d'), sw('lk-move', prefs.move))}
      ${row(t('lk_shortcut'), t('lk_shortcut_d'), scSel)}
      ${row(t('lk_hide_toasts'), t('lk_hide_toasts_d'), sw('lk-hold', prefs.holdToasts))}
      ${row(t('lk_pin_h'), t('lk_pin_d'), pinCtl, `<div id="lk-pin-form"></div>`)}
      <div class="adm-h">${esc(t('lk_look'))}</div>
      ${row(t('lk_bg'), '', bgSel)}
      ${row(t('lk_clock'), '', sw('lk-clock', prefs.clock))}
      ${row(t('lk_user'), '', sw('lk-user', prefs.user))}
      ${row(t('lk_count'), '', sw('lk-count', prefs.count))}
      ${row(t('lk_tab_title'), t('lk_tab_title_d'), sw('lk-title', prefs.tabTitle))}
      <div class="setting-row" style="display:block;"><label class="setting-label" for="lk-message">${esc(t('lk_message'))}</label>
        <textarea id="lk-message" maxlength="300" rows="2" style="width:100%;margin-top:6px;" placeholder="${esc(t('lk_message_ph'))}">${esc(prefs.message || '')}</textarea></div>`;
    const on = (id, ev, fn) => { const el = $(id); if (el) el.addEventListener(ev, fn); };
    on('lk-now', 'click', () => lock('manual'));
    on('lk-idle', 'change', async e => {
      let v = e.target.value;
      if (v === 'custom') {
        const n = parseInt(await uiPrompt(t('lk_idle'), String(cur), {message: t('lk_custom_q')}), 10);
        if (!Number.isFinite(n) || n < 1 || n > 1440) { renderSettings(box); return; }
        v = max ? Math.min(n, max) : n;
      }
      savePrefs({idle: parseInt(v, 10)}); touch(); renderSettings(box);
    });
    on('lk-warn', 'change', e => savePrefs({warn: parseInt(e.target.value, 10)}));
    on('lk-hidden', 'change', e => savePrefs({hidden: parseInt(e.target.value, 10)}));
    on('lk-shortcut', 'change', e => savePrefs({shortcut: e.target.value}));
    on('lk-bg', 'change', e => savePrefs({bg: e.target.value}));
    on('lk-os', 'change', async e => {
      savePrefs({os: e.target.checked});
      const msg = $('lk-os-msg');
      if (e.target.checked) {
        const r = await startOSDetector(true);
        if (r !== 'ok') { e.target.checked = false; savePrefs({os: false}); if (msg) msg.textContent = r === 'na' ? t('lk_os_na') : t('lk_os_denied'); }
      } else startOSDetector(false);
    });
    [['lk-sleep', 'sleep'], ['lk-start', 'start'], ['lk-move', 'move'], ['lk-hold', 'holdToasts'], ['lk-clock', 'clock'], ['lk-user', 'user'], ['lk-count', 'count'], ['lk-title', 'tabTitle']]
      .forEach(([id, k]) => on(id, 'change', e => savePrefs({[k]: e.target.checked})));
    on('lk-message', 'change', e => savePrefs({message: e.target.value.slice(0, 300)}));
    on('lk-pin-set', 'click', () => pinForm(box, me));
    on('lk-pin-del', 'click', async () => {
      const r = await fetch('/api/auth/lock/pin', {method: 'DELETE'});
      if (r.ok) { showToast(t('lk_pin_removed'), 'success'); await refreshMe(); renderSettings(box); }
      else showToast(await apiError(r), 'error');
    });
  }

  function pinForm(box, me) {
    const f = $('lk-pin-form');
    if (!f) return;
    const needPass = !(me.sso_only && !me.directory_password);
    f.innerHTML = `<div style="display:grid;gap:6px;margin-top:8px;max-width:260px;">
      <input type="password" id="lk-pin1" inputmode="numeric" autocomplete="off" maxlength="12" placeholder="${esc(t('lk_pin_new'))}" aria-label="${esc(t('lk_pin_new'))}">
      <input type="password" id="lk-pin2" inputmode="numeric" autocomplete="off" maxlength="12" placeholder="${esc(t('lk_pin_again'))}" aria-label="${esc(t('lk_pin_again'))}">
      ${needPass ? `<input type="password" id="lk-pin-pw" autocomplete="current-password" placeholder="${esc(t('lk_pin_pass'))}" aria-label="${esc(t('lk_pin_pass'))}">` : ''}
      <button class="btn-sm" id="lk-pin-save">${esc(t('save'))}</button></div>`;
    $('lk-pin1').focus();
    $('lk-pin-save').addEventListener('click', async () => {
      const a = $('lk-pin1').value.trim(), b = $('lk-pin2').value.trim();
      if (a !== b) { showToast(t('lk_pin_mismatch'), 'error'); return; }
      const r = await fetch('/api/auth/lock/pin', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({pin: a, password: needPass ? $('lk-pin-pw').value : ''})});
      if (r.ok) { showToast(t('lk_pin_saved'), 'success'); await refreshMe(); renderSettings(box); }
      else showToast(await apiError(r), 'error');
    });
  }

  async function refreshMe() {
    try { const r = await fetch('/api/auth/me'); if (r.ok) currentUser = Object.assign(currentUser || {}, await r.json()); } catch (_) { /* keep */ }
  }

  // ── start ─────────────────────────────────────────
  function button() {
    let b = $('btn-lock');
    if (!b) {
      b = document.createElement('button');
      b.id = 'btn-lock';
      b.className = 'btn-top';
      b.innerHTML = '<span>🔒</span><span class="lbl"></span>';
      b.addEventListener('click', () => lock('manual'));
      if ($('btn-logout')) $('btn-logout').parentNode.insertBefore(b, $('btn-logout'));
    }
    b.title = t('lock_now');
    b.setAttribute('aria-label', t('lock_now'));
    const lbl = b.querySelector('.lbl');
    if (lbl) lbl.textContent = t('lock_now');
  }

  function listen() {
    if (booted) return;
    booted = true;
    ['pointerdown', 'keydown', 'wheel', 'touchstart'].forEach(ev => window.addEventListener(ev, touch, {passive: true}));
    let lastMove = 0;
    window.addEventListener('mousemove', e => { const n = Date.now(); if (n - lastMove > 1000) { lastMove = n; touch(e); } }, {passive: true});
    window.addEventListener('keydown', e => { if (!overlay && signedIn() && matchShortcut(e)) { e.preventDefault(); e.stopPropagation(); lock('shortcut'); } }, true);
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) {
        hiddenAt = Date.now();
        if (prefs.hidden === 0 && !overlay && signedIn()) setTimeout(() => { if (document.hidden && !otherTabVisible()) lock('hidden'); }, 1500);
      } else { hiddenAt = 0; touch(); if (!overlay) syncServer(); }
    });
    if (chan) chan.onmessage = e => {
      const m = e.data || {};
      if (m.t === 'lock') show('other_tab', false);
      if (m.t === 'unlock' && overlay) verifyUnlocked();
    };
    // A request refused with 423 means another tab (or the server) locked this session.
    if (!window.fetch.__lock) {
      const f = window.fetch;
      window.fetch = function () {
        return f.apply(this, arguments).then(res => { if (res.status === 423 && !overlay && signedIn()) show('other_tab', false); return res; });
      };
      window.fetch.__lock = true;
    }
  }

  async function verifyUnlocked() {
    if (policy().mode === 'browser') { hide(); arm(); return; }
    try {
      const r = await fetch('/api/auth/lock');
      if (r.ok && !(await r.json()).locked) { hide(); arm(); }
    } catch (_) { /* stay locked */ }
  }

  async function syncServer() {
    if (policy().mode === 'browser') return;
    try {
      const r = await fetch('/api/auth/lock');
      if (r.ok && (await r.json()).locked) show('other_tab', false);
    } catch (_) { /* offline */ }
  }

  function boot() {
    if (!signedIn()) return;
    button();
    listen();
    wrapToast();
    loadPrefs().then(() => { arm(); if (prefs.os) startOSDetector(false); });
    arm();
  }

  window.wrmLock = {lock, arm, minutes, renderSettings, locked: false, prefs: () => Object.assign({}, prefs)};

  // initApp waits while the session is locked (on the server, or "lock when WRM is opened"),
  // so a reload never shows the interface before the unlock.
  if (typeof window.initApp === 'function' && !window.initApp.__lock) {
    const orig = window.initApp;
    window.initApp = function () {
      const args = arguments, self = this;
      const run = () => { const r = orig.apply(self, args); boot(); return r; };
      const lockedNow = signedIn() && (currentUser.screen_locked || (prefs.start && !currentUser.restricted));
      if (!lockedNow) return run();
      listen();
      button();
      deferredInit = run;
      show(currentUser.screen_locked ? 'other_tab' : 'start', false);
      return undefined;
    };
    window.initApp.__lock = true;
  } else if (signedIn()) {
    boot();
  }
})();
