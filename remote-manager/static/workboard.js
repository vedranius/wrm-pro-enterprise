/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// WRM PRO Workboard (v12.6.9, the shared canvas since v12.6.10).
// The window area is one canvas: terminals, file managers and desktops share it with Workboard
// addons and floating panels (static/layout.js). Every addon is a window the user adds from the
// catalog (nothing is on by default): server status, server load, notifications, clock, recent
// Git commits, running tunnels, quick connect, notes, links, saved sessions and a timer. Addon
// windows move, resize, snap, minimize, stack and pin like terminals; an addon can also be docked
// into a panel of the page layout and goes back onto the canvas when that panel is hidden.
// The ▤ Workboard menu holds the canvas tools: add addons, grid, tile and cascade, lock, saved
// layouts (addons, panels and optionally the open terminals), export and import.
// Everything is stored per user on the server (/api/me/ui/workboard) with a copy in this browser.
(function () {
  'use strict';

  Object.assign(LANGS.en, {
    wb_title: 'Workboard', wb_title_t: 'Workboard: your canvas. Add addons, arrange windows and panels, grid, saved layouts, export and import',
    wb_add: 'Add addon', wb_catalog: 'Addons', wb_catalog_hint: 'Add only what helps you. Every addon is a window on the canvas next to your terminals: move it, resize it, snap it, pin it, dock it as a panel, configure it and remove it again.',
    wb_added: 'Added: {name}', wb_max: 'The canvas holds at most {n} addons.',
    wb_t_settings: 'Settings', wb_t_remove: 'Remove', wb_t_refresh: 'Refresh now', wb_t_dock: 'Dock as a panel',
    wb_remove_q: 'Remove the addon {name}? Its settings are lost.', wb_removed: 'Removed: {name}',
    wb_locked: 'The layout is locked: unlock it in the ▤ Workboard menu to dock or move addons.',
    wb_set_title: 'Title', wb_set_title_ph: 'Default: the addon name',
    wb_set_accent: 'Colour', wb_acc_none: 'None', wb_acc_blue: 'Blue', wb_acc_green: 'Green', wb_acc_yellow: 'Yellow', wb_acc_red: 'Red', wb_acc_purple: 'Purple', wb_acc_gray: 'Grey',
    wb_set_scale: 'Text size', wb_sc_s: 'Small', wb_sc_m: 'Normal', wb_sc_l: 'Large', wb_sc_xl: 'Extra large',
    wb_set_conns: 'Servers', wb_set_filter: 'Filter…', wb_set_conns_all: 'None chosen: all servers with live status are shown.', wb_set_conns_need: 'Choose up to {n} servers.',
    wb_set_more: 'At most {n} servers.', wb_saved: 'Saved',
    wb_loading: 'Loading…', wb_updated: 'updated {when}', wb_none: 'Nothing to show.',
    // canvas menu
    wb_m_canvas: 'Canvas', wb_m_grid: 'Snap to the grid', wb_m_grid_t: 'Windows and panels on the canvas land on grid lines when moved or resized',
    wb_m_gsize: 'Grid size', wb_m_bg: 'Canvas background', wb_bg_dots: 'dots', wb_bg_lines: 'lines', wb_bg_none: 'plain', wb_bg_grid: 'only with the grid',
    wb_m_tile: 'Tile the windows', wb_m_cascade: 'Cascade the windows', wb_m_min_all: 'Minimize all windows', wb_m_show_all: 'Show all minimized windows',
    wb_m_hide_addons: 'Minimize the addons', wb_m_show_addons: 'Show the addons',
    wb_m_layouts: 'Saved layouts', wb_m_save: 'Save this layout…', wb_m_manage: 'Manage saved layouts…', wb_m_export: 'Export the layout (JSON)', wb_m_import: 'Import a layout…',
    wb_m_hint: 'The canvas is everything in the window area: terminals, files, desktops, addons and panels placed on it. Drag any window by its title bar, any panel by its tab. Right-click a title bar for pin, always on top and docking.',
    wb_sv_title: 'Saved layouts', wb_sv_name: 'Name', wb_sv_name_ph: 'e.g. Morning check, Deploy day', wb_sv_terms: 'Also the open terminals, file managers and desktops',
    wb_sv_terms_d: 'Without this the layout keeps the addons, panels and canvas settings; your terminals stay as they are.',
    wb_sv_save: 'Save', wb_sv_apply: 'Apply', wb_sv_delete: 'Delete', wb_sv_none: 'No saved layouts yet.', wb_sv_replace_q: 'A layout named {name} exists. Replace it?',
    wb_sv_saved: 'Layout saved: {name}', wb_sv_applied: 'Layout applied: {name}', wb_sv_delete_q: 'Delete the saved layout {name}?', wb_sv_max: 'At most {n} saved layouts. Delete one first.',
    wb_sv_what: '{a} addons, {p} panels on the canvas', wb_sv_what_w: '{a} addons, {w} windows', wb_sv_apply_q: 'Apply the layout {name}? Your addons and panel layout are replaced.',
    wb_sv_wins_q: 'This layout also has {n} terminal and file windows.', wb_sv_wins_replace: 'Replace my windows', wb_sv_wins_add: 'Add to my windows',
    wb_imp_bad: 'This file is not a WRM Workboard export.', wb_imp_q: 'Replace your addons and panel layout with the imported layout? Saved layouts from the file are added to yours.', wb_imp_ok: 'Layout imported',
    wb_too_big: 'The canvas could not be saved on the server: it is too large. Remove some saved layouts or shorten notes.',
    // addons
    wb_a_status: 'Server status', wb_a_status_d: 'Up, down and slow servers from the live status checks, with latency and since when.',
    wb_status_off: 'Live status checks are turned off. An administrator turns them on in Settings (status checks).',
    wb_status_problems: 'Only show servers with problems', wb_status_all_ok: 'All {n} servers are up.', wb_status_none: 'No server with live status yet. Live status is shown for connections with checks turned on.',
    wb_a_metrics: 'Server load', wb_a_metrics_d: 'CPU, RAM, load and disk usage of the servers you choose, refreshed every minute while visible.',
    wb_metrics_choose: 'Choose servers in ⚙ settings (SSH or SFTP connections).', wb_m_cpu: 'CPU', wb_m_ram: 'RAM', wb_m_load: 'Load', wb_m_up: 'up {d}',
    wb_m_src_zabbix: 'Monitoring (Zabbix agent)', wb_m_src_ssh: 'read over SSH', wb_m_more_disks: '+{n} more',
    wb_metrics_help: 'Values come from Monitoring when the server has it set up, otherwise WRM reads them over SSH (Linux: /proc and df), as far as the administrator allows it.',
    wb_a_events: 'Notifications', wb_a_events_d: 'What WRM told you: servers going down, expiring keys, new versions, Git jobs. Also without e-mail or chat channels.',
    wb_ev_read_all: 'Mark all read', wb_ev_clear: 'Clear', wb_ev_clear_q: 'Clear all notifications?', wb_ev_none: 'No notifications.', wb_ev_unread: '{n} unread',
    wb_a_clock: 'Clock', wb_a_clock_d: 'Your local time and the time zones you work with.',
    wb_clock_zones: 'Time zones', wb_clock_zones_ph: 'e.g. UTC, Europe/London, America/New_York', wb_clock_zones_bad: 'Unknown time zone: {z}', wb_clock_seconds: 'Show seconds',
    wb_a_commits: 'Recent commits', wb_a_commits_d: 'The last commits and pushes of a service from the Git workspace (GitLab, GitHub or Gitea).',
    wb_commits_service: 'Service (from the Git catalog)', wb_commits_branch: 'Branch', wb_commits_branch_ph: 'Empty: the branches the service tracks', wb_commits_count: 'How many',
    wb_commits_choose: 'Choose a service in ⚙ settings.', wb_commits_nogit: 'The Git workspace is not available to you.', wb_commits_noapps: 'The Git catalog has no services yet.', wb_commits_pushed: 'pushed', wb_commits_none: 'No commits in the last days.',
    wb_a_tunnels: 'Running tunnels', wb_a_tunnels_d: 'Tunnels that are running now, with their local address.', wb_tun_none: 'No tunnel is running.', wb_tun_open: 'Tunnels…',
    wb_a_quick: 'Quick connect', wb_a_quick_d: 'Buttons for the servers you open most often.', wb_quick_choose: 'Choose servers in ⚙ settings.',
    wb_a_notes: 'Notes', wb_a_notes_d: 'A short note to yourself, kept on the server.', wb_notes_ph: 'Write a note…',
    wb_a_links: 'Links', wb_a_links_d: 'Your bookmarks to web pages: dashboards, wikis, tickets, documentation.', wb_links_choose: 'Add links in ⚙ settings.',
    wb_links_field: 'Links, one per line: Name | https://address', wb_links_bad: 'Line {n}: only http:// and https:// addresses.',
    wb_a_sessions: 'Saved sessions', wb_a_sessions_d: 'Open a saved session (its terminals and file windows) with one click.', wb_sess_none: 'No saved sessions yet. Save one with + Save Session in the Sessions panel.', wb_sess_wins: '{n} windows',
    wb_a_timer: 'Timer', wb_a_timer_d: 'A countdown (for maintenance windows, waiting on a job) or a stopwatch.',
    wb_tm_mode: 'Mode', wb_tm_down: 'Countdown', wb_tm_up: 'Stopwatch', wb_tm_min: 'Minutes', wb_tm_start: 'Start', wb_tm_pause: 'Pause', wb_tm_reset: 'Reset', wb_tm_done: 'Time is up: {name}',
    wb_unavailable: 'Not available', wb_add_btn: 'Add',
  });
  Object.assign(LANGS.hr, {
    wb_title: 'Workboard', wb_title_t: 'Workboard: tvoj canvas. Dodaj addone, složi prozore i panele, mreža, spremljeni rasporedi, izvoz i uvoz',
    wb_add: 'Dodaj addon', wb_catalog: 'Addoni', wb_catalog_hint: 'Dodaj samo ono što ti pomaže. Svaki addon je prozor na canvasu pored tvojih terminala: premjesti ga, promijeni veličinu, poravnaj, prikvači, usidri kao panel, podesi i opet ukloni.',
    wb_added: 'Dodano: {name}', wb_max: 'Canvas ima najviše {n} addona.',
    wb_t_settings: 'Postavke', wb_t_remove: 'Ukloni', wb_t_refresh: 'Osvježi sada', wb_t_dock: 'Usidri kao panel',
    wb_remove_q: 'Ukloniti addon {name}? Njegove postavke se gube.', wb_removed: 'Uklonjeno: {name}',
    wb_locked: 'Raspored je zaključan: otključaj ga u izborniku ▤ Workboard da bi sidrio ili micao addone.',
    wb_set_title: 'Naslov', wb_set_title_ph: 'Zadano: naziv addona',
    wb_set_accent: 'Boja', wb_acc_none: 'Bez', wb_acc_blue: 'Plava', wb_acc_green: 'Zelena', wb_acc_yellow: 'Žuta', wb_acc_red: 'Crvena', wb_acc_purple: 'Ljubičasta', wb_acc_gray: 'Siva',
    wb_set_scale: 'Veličina teksta', wb_sc_s: 'Mala', wb_sc_m: 'Normalna', wb_sc_l: 'Velika', wb_sc_xl: 'Vrlo velika',
    wb_set_conns: 'Poslužitelji', wb_set_filter: 'Filtriraj…', wb_set_conns_all: 'Ništa odabrano: prikazuju se svi poslužitelji sa stanjem uživo.', wb_set_conns_need: 'Odaberi do {n} poslužitelja.',
    wb_set_more: 'Najviše {n} poslužitelja.', wb_saved: 'Spremljeno',
    wb_loading: 'Učitavam…', wb_updated: 'osvježeno {when}', wb_none: 'Nema ničega za prikaz.',
    wb_m_canvas: 'Canvas', wb_m_grid: 'Poravnaj po mreži', wb_m_grid_t: 'Prozori i paneli na canvasu sjedaju na linije mreže kad ih pomičeš ili im mijenjaš veličinu',
    wb_m_gsize: 'Veličina mreže', wb_m_bg: 'Pozadina canvasa', wb_bg_dots: 'točke', wb_bg_lines: 'linije', wb_bg_none: 'obična', wb_bg_grid: 'samo uz mrežu',
    wb_m_tile: 'Složi prozore jedan uz drugi', wb_m_cascade: 'Složi prozore kaskadno', wb_m_min_all: 'Smanji sve prozore', wb_m_show_all: 'Prikaži sve smanjene prozore',
    wb_m_hide_addons: 'Smanji addone', wb_m_show_addons: 'Prikaži addone',
    wb_m_layouts: 'Spremljeni rasporedi', wb_m_save: 'Spremi ovaj raspored…', wb_m_manage: 'Upravljaj spremljenim rasporedima…', wb_m_export: 'Izvezi raspored (JSON)', wb_m_import: 'Uvezi raspored…',
    wb_m_hint: 'Canvas je sve u području prozora: terminali, datoteke, udaljene radne površine, addoni i paneli postavljeni na njega. Prozor povuci za naslovnu traku, panel za karticu. Desni klik na naslovnu traku: prikvači, uvijek na vrhu, usidri.',
    wb_sv_title: 'Spremljeni rasporedi', wb_sv_name: 'Naziv', wb_sv_name_ph: 'npr. Jutarnja provjera, Dan deploya', wb_sv_terms: 'I otvoreni terminali, upravitelji datoteka i radne površine',
    wb_sv_terms_d: 'Bez ovoga raspored pamti addone, panele i postavke canvasa; tvoji terminali ostaju kakvi jesu.',
    wb_sv_save: 'Spremi', wb_sv_apply: 'Primijeni', wb_sv_delete: 'Obriši', wb_sv_none: 'Još nema spremljenih rasporeda.', wb_sv_replace_q: 'Raspored {name} već postoji. Zamijeniti ga?',
    wb_sv_saved: 'Raspored spremljen: {name}', wb_sv_applied: 'Raspored primijenjen: {name}', wb_sv_delete_q: 'Obrisati spremljeni raspored {name}?', wb_sv_max: 'Najviše {n} spremljenih rasporeda. Prvo obriši jedan.',
    wb_sv_what: '{a} addona, {p} panela na canvasu', wb_sv_what_w: '{a} addona, {w} prozora', wb_sv_apply_q: 'Primijeniti raspored {name}? Tvoji addoni i raspored panela se zamjenjuju.',
    wb_sv_wins_q: 'Ovaj raspored ima i {n} prozora s terminalima i datotekama.', wb_sv_wins_replace: 'Zamijeni moje prozore', wb_sv_wins_add: 'Dodaj mojim prozorima',
    wb_imp_bad: 'Ova datoteka nije izvoz WRM Workboarda.', wb_imp_q: 'Zamijeniti tvoje addone i raspored panela uvezenim rasporedom? Spremljeni rasporedi iz datoteke dodaju se tvojima.', wb_imp_ok: 'Raspored uvezen',
    wb_too_big: 'Canvas se nije mogao spremiti na poslužitelj: prevelik je. Obriši neke spremljene rasporede ili skrati bilješke.',
    wb_a_status: 'Stanje poslužitelja', wb_a_status_d: 'Poslužitelji koji rade, ne rade ili su spori, iz provjera stanja uživo, s latencijom i od kada.',
    wb_status_off: 'Provjere stanja uživo su isključene. Administrator ih uključuje u Postavkama (provjere stanja).',
    wb_status_problems: 'Prikaži samo poslužitelje s problemima', wb_status_all_ok: 'Svih {n} poslužitelja radi.', wb_status_none: 'Još nema poslužitelja sa stanjem uživo. Stanje se prikazuje za veze s uključenim provjerama.',
    wb_a_metrics: 'Opterećenje poslužitelja', wb_a_metrics_d: 'CPU, RAM, opterećenje i zauzeće diskova odabranih poslužitelja, osvježava se svake minute dok je vidljivo.',
    wb_metrics_choose: 'Odaberi poslužitelje u ⚙ postavkama (SSH ili SFTP veze).', wb_m_cpu: 'CPU', wb_m_ram: 'RAM', wb_m_load: 'Opterećenje', wb_m_up: 'radi {d}',
    wb_m_src_zabbix: 'Nadzor (Zabbix agent)', wb_m_src_ssh: 'pročitano preko SSH-a', wb_m_more_disks: '+{n} još',
    wb_metrics_help: 'Vrijednosti dolaze iz Nadzora kad ga poslužitelj ima postavljenog, inače ih WRM čita preko SSH-a (Linux: /proc i df), koliko administrator dopušta.',
    wb_a_events: 'Obavijesti', wb_a_events_d: 'Što ti je WRM javio: poslužitelji koji su pali, ključevi koji istječu, nove verzije, Git poslovi. I bez e-mail ili chat kanala.',
    wb_ev_read_all: 'Označi sve pročitanim', wb_ev_clear: 'Očisti', wb_ev_clear_q: 'Obrisati sve obavijesti?', wb_ev_none: 'Nema obavijesti.', wb_ev_unread: '{n} nepročitanih',
    wb_a_clock: 'Sat', wb_a_clock_d: 'Tvoje lokalno vrijeme i vremenske zone s kojima radiš.',
    wb_clock_zones: 'Vremenske zone', wb_clock_zones_ph: 'npr. UTC, Europe/London, America/New_York', wb_clock_zones_bad: 'Nepoznata vremenska zona: {z}', wb_clock_seconds: 'Prikaži sekunde',
    wb_a_commits: 'Zadnji commitovi', wb_a_commits_d: 'Zadnji commitovi i pushevi servisa iz Git radnog prostora (GitLab, GitHub ili Gitea).',
    wb_commits_service: 'Servis (iz Git kataloga)', wb_commits_branch: 'Grana', wb_commits_branch_ph: 'Prazno: grane koje servis prati', wb_commits_count: 'Koliko',
    wb_commits_choose: 'Odaberi servis u ⚙ postavkama.', wb_commits_nogit: 'Git radni prostor ti nije dostupan.', wb_commits_noapps: 'Git katalog još nema servisa.', wb_commits_pushed: 'push', wb_commits_none: 'Nema commitova u zadnjih nekoliko dana.',
    wb_a_tunnels: 'Aktivni tuneli', wb_a_tunnels_d: 'Tuneli koji sada rade, s lokalnom adresom.', wb_tun_none: 'Nijedan tunel ne radi.', wb_tun_open: 'Tuneli…',
    wb_a_quick: 'Brzo spajanje', wb_a_quick_d: 'Gumbi za poslužitelje koje najčešće otvaraš.', wb_quick_choose: 'Odaberi poslužitelje u ⚙ postavkama.',
    wb_a_notes: 'Bilješke', wb_a_notes_d: 'Kratka bilješka za tebe, sprema se na poslužitelj.', wb_notes_ph: 'Napiši bilješku…',
    wb_a_links: 'Poveznice', wb_a_links_d: 'Tvoje poveznice na web stranice: nadzorne ploče, wikiji, tiketi, dokumentacija.', wb_links_choose: 'Dodaj poveznice u ⚙ postavkama.',
    wb_links_field: 'Poveznice, jedna po retku: Naziv | https://adresa', wb_links_bad: 'Redak {n}: samo http:// i https:// adrese.',
    wb_a_sessions: 'Spremljene sesije', wb_a_sessions_d: 'Otvori spremljenu sesiju (njene terminale i prozore s datotekama) jednim klikom.', wb_sess_none: 'Još nema spremljenih sesija. Spremi jednu gumbom + Spremi sesiju u panelu Sesije.', wb_sess_wins: '{n} prozora',
    wb_a_timer: 'Mjerač vremena', wb_a_timer_d: 'Odbrojavanje (za servisne prozore, čekanje na posao) ili štoperica.',
    wb_tm_mode: 'Način', wb_tm_down: 'Odbrojavanje', wb_tm_up: 'Štoperica', wb_tm_min: 'Minute', wb_tm_start: 'Pokreni', wb_tm_pause: 'Pauza', wb_tm_reset: 'Ponovno', wb_tm_done: 'Vrijeme je isteklo: {name}',
    wb_unavailable: 'Nije dostupno', wb_add_btn: 'Dodaj',
  });

  const $ = id => document.getElementById(id);
  const tn = (k, v) => { let s = t(k); for (const n in (v || {})) s = s.split('{' + n + '}').join(v[n]); return s; };
  const MAX_TILES = 24, NOTE_MAX = 2000, METRIC_MAX = 8, QUICK_MAX = 12, STATUS_MAX = 60, LINKS_MAX = 30, SAVED_MAX = 10;
  const SNAPS = ['w', 'e', 'n', 's', 'nw', 'ne', 'sw', 'se', 'full'], GRIDS = [10, 20, 40], BGS = ['grid', 'dots', 'lines', 'none'];
  const ACCENTS = {blue: '#4f8cff', green: '#22c55e', yellow: '#f59e0b', red: '#ef4444', purple: '#a855f7', gray: '#94a3b8'};
  const SCALES = {s: 0.9, m: 1, l: 1.15, xl: 1.35};
  const user = () => (typeof currentUser !== 'undefined' && currentUser) || null;
  const pol = () => (user() && user().policy) || {};
  const allConns = () => (typeof conns !== 'undefined' && Array.isArray(conns) ? conns : []);
  const connById = id => allConns().find(c => c.id === id);
  const sshLike = c => c && (c.protocol === 'SSH' || c.protocol === 'SFTP');
  const compact = () => typeof isCompact === 'function' && isCompact();
  const visible = el => !!el && el.isConnected && el.getClientRects().length > 0 && document.visibilityState === 'visible';
  const pct = v => (v == null || !isFinite(v) ? null : Math.max(0, Math.min(100, Number(v))));
  const level = p => (p == null ? '' : p >= 90 ? 'crit' : p >= 75 ? 'warn' : 'ok');
  const httpURL = u => /^https?:\/\/[^\s]+$/i.test(String(u || ''));
  const locked = () => !!(window.wrmLayout && wrmLayout.locked());

  function ago(ts) {
    const d = new Date(ts).getTime();
    if (!d) return '';
    const s = Math.round((d - Date.now()) / 1000), a = Math.abs(s);
    let rtf;
    try { rtf = new Intl.RelativeTimeFormat(typeof currentLang !== 'undefined' && currentLang === 'hr' ? 'hr' : 'en', {numeric: 'auto'}); } catch (e) { return new Date(ts).toLocaleString(); }
    if (a < 60) return rtf.format(s, 'second');
    if (a < 3600) return rtf.format(Math.round(s / 60), 'minute');
    if (a < 86400) return rtf.format(Math.round(s / 3600), 'hour');
    return rtf.format(Math.round(s / 86400), 'day');
  }
  function dur(sec) {
    sec = Number(sec) || 0;
    const d = Math.floor(sec / 86400), h = Math.floor(sec % 86400 / 3600), m = Math.floor(sec % 3600 / 60);
    return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
  }
  function openConn(c) {
    if (!c) return;
    if (typeof closeDrawers === 'function') closeDrawers();
    if (typeof isWebConn === 'function' && isWebConn(c)) { openWebConn(c); return; }
    const wo = addWindow();
    wo.connId = c.id;
    setWinMode(wo.id, defaultWinMode(c));
  }
  function clockText(ms) {
    const neg = ms < 0, a = Math.floor(Math.abs(ms) / 1000), h = Math.floor(a / 3600), m = Math.floor(a % 3600 / 60), s = a % 60;
    const p = n => String(n).padStart(2, '0');
    return (neg ? '−' : '') + (h ? h + ':' + p(m) : p(m)) + ':' + p(s);
  }
  function timerDone(tile) {
    const msg = tn('wb_tm_done', {name: nameOf(tile)});
    showToast('⏱ ' + msg, 'warning', 10000);
    try { if ('Notification' in window && Notification.permission === 'granted') new Notification('WRM PRO', {body: msg}); } catch (e) { /* no notification */ }
  }
  function bar(label, p, side) {
    const v = pct(p);
    return `<div class="wb-bar ${level(v)}"><span class="l">${esc(label)}</span><span class="tr"><span class="fi" style="width:${v == null ? 0 : v.toFixed(1)}%"></span></span><span class="v">${v == null ? '—' : Math.round(v) + '%'}${side ? ' <span class="sd">' + esc(side) + '</span>' : ''}</span></div>`;
  }

  // ── addons ──
  // Each addon: ico, name/desc keys, avail() (with a reason when not), render(tile, el, ctx),
  // every (seconds between loads while visible), load(tile, ctx) and settings fields.
  const A = {
    status: {
      ico: '🟢', key: 'wb_a_status', desc: 'wb_a_status_d', fields: ['conns', 'problems'], connMax: STATUS_MAX, connFilter: () => true,
      sig: tile => JSON.stringify([typeof statusInfo !== 'undefined' && statusInfo.enabled, typeof connStatus !== 'undefined' ? connStatus : null, tile.cfg]),
      render(tile, el) {
        if (typeof statusInfo === 'undefined' || !statusInfo.enabled) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_status_off'))}</div>`; return; }
        const ids = (tile.cfg.ids || []).filter(connById);
        let list = (ids.length ? ids.map(connById) : allConns().filter(c => connStatus[c.id])).map(c => ({c, st: connStatus[c.id] || null}));
        const rank = x => (!x.st ? 3 : x.st.state === 'down' ? 0 : x.st.state === 'up' && x.st.latency_ms > 300 ? 1 : x.st.state === 'up' ? 4 : 2);
        list.sort((a, b) => rank(a) - rank(b) || a.c.name.localeCompare(b.c.name));
        const total = list.length;
        if (tile.cfg.problems) list = list.filter(x => rank(x) < 4);
        if (!total) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_status_none'))}</div>`; return; }
        if (!list.length) { el.innerHTML = `<div class="wb-ok">✓ ${esc(tn('wb_status_all_ok', {n: total}))}</div>`; return; }
        const up = list.filter(x => rank(x) === 4).length, down = list.filter(x => rank(x) === 0).length;
        el.innerHTML = `<div class="wb-sum">${down ? `<span class="b down">${down} ${esc(t('st_down'))}</span>` : ''}<span class="b up">${up} ${esc(t('st_up'))}</span></div>` +
          `<div class="wb-list">${list.slice(0, STATUS_MAX).map(x => {
            const cls = x.st ? statusClass(x.st) : 'unknown';
            const side = x.st && x.st.state === 'up' ? Number(x.st.latency_ms) + ' ms' : x.st && x.st.since ? ago(x.st.since) : '';
            return `<button type="button" class="wb-row" data-c="${x.c.id}" title="${esc(statusTitle(x.st))}"><span class="conn-st ${cls}"></span><span class="nm">${esc(x.c.name)}</span><span class="sd">${esc(side)}</span></button>`;
          }).join('')}</div>`;
        el.querySelectorAll('.wb-row').forEach(b => { b.onclick = () => openConn(connById(Number(b.dataset.c))); });
      },
    },
    metrics: {
      ico: '📈', key: 'wb_a_metrics', desc: 'wb_a_metrics_d', fields: ['conns'], connMax: METRIC_MAX, connFilter: sshLike, every: 60, refresh: true,
      async load(tile, ctx) {
        const ids = tile.cfg.ids || [];
        ctx.data = ctx.data || {};
        await Promise.all(ids.map(async id => {
          try {
            const r = await fetch('/api/workboard/metrics?id=' + id + (ctx.force ? '&refresh=1' : ''));
            const j = await r.json().catch(() => ({}));
            ctx.data[id] = r.ok ? j : {error: j.error || ('HTTP ' + r.status)};
          } catch (e) { ctx.data[id] = {error: String(e.message || e)}; }
        }));
      },
      render(tile, el, ctx) {
        const ids = (tile.cfg.ids || []).filter(id => sshLike(connById(id)));
        if (!ids.length) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_metrics_choose'))}</div><div class="wb-hint sm">${esc(t('wb_metrics_help'))}</div>`; return; }
        const data = ctx.data || {};
        el.innerHTML = `<div class="wb-srv-grid">${ids.map(id => {
          const c = connById(id), m = data[id];
          let body;
          if (!m) body = `<div class="wb-hint sm">${esc(t('wb_loading'))}</div>`;
          else if (m.error) body = `<div class="wb-err">⚠ ${esc(m.error)}</div>`;
          else {
            const disks = (m.disks || []).slice().sort((a, b) => b.percent - a.percent);
            const shown = disks.slice(0, tile.size === 'm' ? 6 : 3);
            const ld = m.load != null ? Number(m.load).toFixed(2) + (m.cpu_count ? ' / ' + m.cpu_count : '') : '';
            const loadPct = m.load != null && m.cpu_count ? (m.load / m.cpu_count) * 100 : null;
            body = bar(t('wb_m_cpu'), m.cpu_percent) +
              bar(t('wb_m_ram'), m.memory_percent, m.memory_total ? fmtBytes(m.memory_total) : '') +
              (m.load != null ? (loadPct != null ? bar(t('wb_m_load'), loadPct, ld) : `<div class="wb-kv"><span>${esc(t('wb_m_load'))}</span><b>${esc(ld)}</b></div>`) : '') +
              shown.map(d => bar(d.mount, d.percent, d.size_kb ? fmtBytes(d.size_kb * 1024) : '')).join('') +
              (disks.length > shown.length ? `<div class="wb-hint sm" title="${esc(disks.slice(shown.length).map(d => d.mount + ' ' + Math.round(d.percent) + '%').join('\n'))}">${esc(tn('wb_m_more_disks', {n: disks.length - shown.length}))}</div>` : '') +
              `<div class="wb-src">${esc(t(m.source === 'zabbix' ? 'wb_m_src_zabbix' : 'wb_m_src_ssh'))}${m.uptime ? ' · ' + esc(tn('wb_m_up', {d: dur(m.uptime)})) : ''}${m.at ? ' · ' + esc(ago(m.at)) : ''}</div>`;
          }
          const st = typeof connStatusOf === 'function' ? connStatusOf(c) : null;
          return `<div class="wb-srv"><button type="button" class="wb-srv-h" data-c="${id}"><span class="conn-st ${st ? statusClass(st) : 'unknown'}"></span><span class="nm">${esc(c.name)}</span></button>${body}</div>`;
        }).join('')}</div>`;
        el.querySelectorAll('.wb-srv-h').forEach(b => { b.onclick = () => openConn(connById(Number(b.dataset.c))); });
      },
    },
    events: {
      ico: '🔔', key: 'wb_a_events', desc: 'wb_a_events_d', fields: [], every: 300, refresh: true,
      async load(tile, ctx) {
        try {
          const r = await fetch('/api/me/events?limit=50');
          if (!r.ok) { ctx.error = 'HTTP ' + r.status; return; }
          const j = await r.json();
          ctx.events = j.events || []; ctx.unread = j.unread || 0; ctx.error = '';
          setUnread(ctx.unread);
        } catch (e) { ctx.error = String(e.message || e); }
      },
      render(tile, el, ctx) {
        if (ctx.error) { el.innerHTML = `<div class="wb-err">⚠ ${esc(ctx.error)}</div>`; return; }
        if (!ctx.events) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_loading'))}</div>`; return; }
        const ev = ctx.events;
        el.innerHTML = `<div class="wb-acts"><span class="wb-cnt">${ctx.unread ? esc(tn('wb_ev_unread', {n: ctx.unread})) : ''}</span>` +
          `<button type="button" class="btn-sec btn-sm" data-a="read" ${ctx.unread ? '' : 'disabled'}>${esc(t('wb_ev_read_all'))}</button><button type="button" class="btn-sec btn-sm" data-a="clear" ${ev.length ? '' : 'disabled'}>${esc(t('wb_ev_clear'))}</button></div>` +
          (ev.length ? `<div class="wb-list">${ev.map(e => `<div class="wb-ev${e.read ? '' : ' new'}" data-id="${e.id}"><div class="h"><span class="tt">${esc(e.title)}</span><span class="sd" title="${esc(fmtTime(e.at))}">${esc(ago(e.at))}</span></div>${e.body ? `<div class="bd">${esc(e.body)}</div>` : ''}</div>`).join('')}</div>`
            : `<div class="wb-hint">${esc(t('wb_ev_none'))}</div>`);
        el.querySelector('[data-a="read"]').onclick = () => markRead({all: true});
        el.querySelector('[data-a="clear"]').onclick = async () => {
          if (!await uiConfirm(t('wb_ev_clear_q'), {danger: true, okText: t('wb_ev_clear')})) return;
          await fetch('/api/me/events', {method: 'DELETE'}).catch(() => {});
          reloadType('events');
        };
        el.querySelectorAll('.wb-ev.new').forEach(d => { d.onclick = () => markRead({ids: [Number(d.dataset.id)]}); });
      },
    },
    clock: {
      ico: '🕒', key: 'wb_a_clock', desc: 'wb_a_clock_d', fields: ['zones', 'seconds'], tick: true,
      render(tile, el) {
        const sec = tile.cfg.seconds !== false, now = new Date(), loc = typeof currentLang !== 'undefined' && currentLang === 'hr' ? 'hr-HR' : undefined;
        const fmt = (tz, o) => { try { return new Intl.DateTimeFormat(loc, Object.assign({timeZone: tz || undefined}, o)).format(now); } catch (e) { return '—'; } };
        const tOpt = {hour: '2-digit', minute: '2-digit', second: sec ? '2-digit' : undefined, hour12: false};
        const zones = (tile.cfg.zones || []).filter(Boolean);
        const html = `<div class="wb-clock"><div class="big">${esc(fmt('', tOpt))}</div><div class="dt">${esc(fmt('', {weekday: 'long', day: 'numeric', month: 'long', year: 'numeric'}))}</div></div>` +
          (zones.length ? `<div class="wb-zones">${zones.map(z => `<div class="wb-kv"><span>${esc(z)}</span><b>${esc(fmt(z, {hour: '2-digit', minute: '2-digit', hour12: false, weekday: 'short'}))}</b></div>`).join('')}</div>` : '');
        if (el._last !== html) { el.innerHTML = html; el._last = html; }
      },
    },
    commits: {
      ico: '⎇', key: 'wb_a_commits', desc: 'wb_a_commits_d', fields: ['app', 'branch', 'count'], every: 300, refresh: true,
      avail: () => (pol().git === false ? t('wb_commits_nogit') : ''),
      async load(tile, ctx) {
        if (!tile.cfg.app) return;
        const q = new URLSearchParams({app: tile.cfg.app});
        if (tile.cfg.branch) q.set('branch', tile.cfg.branch);
        if (ctx.force) q.set('refresh', '1');
        try {
          const r = await fetch('/api/git/activity?' + q);
          const j = await r.json().catch(() => ({}));
          if (!r.ok) { ctx.error = j.error || ('HTTP ' + r.status); return; }
          ctx.items = (j.items || []).slice().sort((a, b) => String(b.at).localeCompare(String(a.at)));
          ctx.error = '';
        } catch (e) { ctx.error = String(e.message || e); }
      },
      render(tile, el, ctx) {
        if (!tile.cfg.app) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_commits_choose'))}</div>`; return; }
        const head = `<div class="wb-sub">⎇ ${esc(tile.cfg.app)}${tile.cfg.branch ? ' · ' + esc(tile.cfg.branch) : ''}</div>`;
        if (ctx.error) { el.innerHTML = head + `<div class="wb-err">⚠ ${esc(ctx.error)}</div>`; return; }
        if (!ctx.items) { el.innerHTML = head + `<div class="wb-hint">${esc(t('wb_loading'))}</div>`; return; }
        const items = ctx.items.slice(0, Number(tile.cfg.count) || 10);
        el.innerHTML = head + (items.length ? `<div class="wb-list">${items.map(i => {
          const title = i.kind === 'push' ? `${t('wb_commits_pushed')} ${i.ref || i.branch || ''}${i.count ? ' (' + i.count + ')' : ''}` : (i.title || '');
          const br = i.branch || (i.branches || []).join(', ') || (i.kind === 'push' ? '' : '');
          const sha = (i.sha || i.after || '').slice(0, 8);
          const inner = `<div class="h"><span class="tt">${i.kind === 'push' ? '⇪ ' : ''}${esc(title)}</span><span class="sd" title="${esc(fmtTime(i.at))}">${esc(ago(i.at))}</span></div><div class="mt">${esc(i.author || '')}${br ? ' · ' + esc(br) : ''}${sha ? ' · <code>' + esc(sha) + '</code>' : ''}</div>`;
          return /^https?:\/\//i.test(i.url || '') ? `<a class="wb-ev cm" href="${esc(i.url)}" target="_blank" rel="noopener noreferrer">${inner}</a>` : `<div class="wb-ev cm">${inner}</div>`;
        }).join('')}</div>` : `<div class="wb-hint">${esc(t('wb_commits_none'))}</div>`);
      },
    },
    tunnels: {
      ico: '🔀', key: 'wb_a_tunnels', desc: 'wb_a_tunnels_d', fields: [],
      sig: () => JSON.stringify(typeof tunnelState !== 'undefined' ? tunnelState.tunnels.filter(x => x.running).map(x => [x.id, x.listen, x.state, x.active]) : null),
      render(tile, el) {
        const run = (typeof tunnelState !== 'undefined' ? tunnelState.tunnels : []).filter(x => x.running);
        el.innerHTML = (run.length ? `<div class="wb-list">${run.map(v => {
          const c = connById(v.conn_id);
          return `<div class="wb-row ro"><span class="conn-st ${v.state === 'error' ? 'down' : 'up'}"></span><span class="nm">${esc(v.name || (c && c.name) || '')}</span><span class="sd"><code>${esc(v.listen || '')}</code></span></div>`;
        }).join('')}</div>` : `<div class="wb-hint">${esc(t('wb_tun_none'))}</div>`) +
          `<div class="wb-acts"><span class="wb-cnt"></span><button type="button" class="btn-sec btn-sm" data-a="tun">${esc(t('wb_tun_open'))}</button></div>`;
        el.querySelector('[data-a="tun"]').onclick = () => openTunnelsPanel();
      },
    },
    quick: {
      ico: '⚡', key: 'wb_a_quick', desc: 'wb_a_quick_d', fields: ['conns'], connMax: QUICK_MAX, connFilter: () => true,
      sig: tile => JSON.stringify([tile.cfg.ids, (tile.cfg.ids || []).map(id => { const c = connById(id); return c ? [c.name, connStatus[id] && connStatus[id].state] : null; })]),
      render(tile, el) {
        const list = (tile.cfg.ids || []).map(connById).filter(Boolean);
        if (!list.length) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_quick_choose'))}</div>`; return; }
        el.innerHTML = `<div class="wb-quick">${list.map(c => { const st = connStatusOf(c); return `<button type="button" class="btn-sec" data-c="${c.id}" title="${esc(c.host || '')}"><span class="conn-st ${st ? statusClass(st) : 'unknown'}"></span>${esc(c.name)}</button>`; }).join('')}</div>`;
        el.querySelectorAll('[data-c]').forEach(b => { b.onclick = () => openConn(connById(Number(b.dataset.c))); });
      },
    },
    notes: {
      ico: '📝', key: 'wb_a_notes', desc: 'wb_a_notes_d', fields: [],
      render(tile, el) {
        if (el.querySelector('textarea')) return;
        el.innerHTML = `<textarea class="wb-note" maxlength="${NOTE_MAX}" placeholder="${esc(t('wb_notes_ph'))}"></textarea>`;
        const ta = el.querySelector('textarea');
        ta.value = tile.cfg.text || '';
        ta.oninput = () => { tile.cfg.text = ta.value.slice(0, NOTE_MAX); saveSoon(); };
      },
    },
    links: {
      ico: '🔗', key: 'wb_a_links', desc: 'wb_a_links_d', fields: ['links'],
      render(tile, el) {
        const ls = (tile.cfg.links || []).filter(l => httpURL(l.url));
        if (!ls.length) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_links_choose'))}</div>`; return; }
        const host = u => { try { return new URL(u).host; } catch (e) { return ''; } };
        const html = `<div class="wb-list">${ls.map(l => `<a class="wb-row" href="${esc(l.url)}" target="_blank" rel="noopener noreferrer" title="${esc(l.url)}"><span class="ico">↗</span><span class="nm">${esc(l.name || host(l.url))}</span><span class="sd">${esc(host(l.url))}</span></a>`).join('')}</div>`;
        if (el._last !== html) { el.innerHTML = html; el._last = html; }
      },
    },
    sessions: {
      ico: '🗂', key: 'wb_a_sessions', desc: 'wb_a_sessions_d', fields: [],
      sig: () => JSON.stringify(typeof sessions !== 'undefined' ? sessions.map(s => [s.id, s.name, s.layout]) : null),
      render(tile, el) {
        const list = typeof sessions !== 'undefined' && Array.isArray(sessions) ? sessions : [];
        if (!list.length) { el.innerHTML = `<div class="wb-hint">${esc(t('wb_sess_none'))}</div>`; return; }
        el.innerHTML = `<div class="wb-list">${list.map(sv => {
          const n = typeof sessionLayout === 'function' ? sessionLayout(sv).windows.length : 0;
          return `<button type="button" class="wb-row" data-s="${sv.id}"><span class="ico">🗂</span><span class="nm">${esc(sv.name)}</span><span class="sd">${n ? esc(tn('wb_sess_wins', {n})) : ''}</span></button>`;
        }).join('')}</div>`;
        el.querySelectorAll('[data-s]').forEach(b => { b.onclick = () => openSession(Number(b.dataset.s)); });
      },
    },
    timer: {
      ico: '⏱', key: 'wb_a_timer', desc: 'wb_a_timer_d', fields: ['timer'], tick: true,
      render(tile, el) {
        const c = tile.cfg, down = c.mode !== 'up', total = (Number(c.min) || 25) * 60000, now = Date.now();
        let ms, running;
        if (down) { running = !!c.end; ms = running ? c.end - now : (c.left != null ? c.left : total); }
        else { running = !!c.start; ms = (c.acc || 0) + (running ? now - c.start : 0); }
        if (down && running && ms <= 0) { c.end = 0; c.left = 0; ms = 0; running = false; saveSoon(); timerDone(tile); }
        if (!el.querySelector('.wb-timer')) {
          el.innerHTML = `<div class="wb-timer"><div class="big"></div><div class="tr"><span class="fi"></span></div><div class="wb-acts c"><button type="button" class="btn-sm" data-a="go"></button><button type="button" class="btn-sec btn-sm" data-a="reset">${esc(t('wb_tm_reset'))}</button></div></div>`;
          el.querySelector('[data-a="go"]').onclick = () => {
            const x = tile.cfg, n = Date.now(), dn = x.mode !== 'up', tot = (Number(x.min) || 25) * 60000;
            if (dn) { if (x.end) { x.left = Math.max(0, x.end - n); x.end = 0; } else { const left = x.left != null && x.left > 0 ? x.left : tot; x.end = n + left; delete x.left; } }
            else if (x.start) { x.acc = (x.acc || 0) + n - x.start; x.start = 0; } else x.start = n;
            saveSoon(); paint(tile, true);
          };
          el.querySelector('[data-a="reset"]').onclick = () => { const x = tile.cfg; delete x.end; delete x.left; delete x.start; delete x.acc; saveSoon(); paint(tile, true); };
        }
        el.querySelector('.big').textContent = clockText(ms);
        el.querySelector('.big').classList.toggle('done', down && ms <= 0);
        el.querySelector('.tr').style.display = down ? '' : 'none';
        el.querySelector('.fi').style.width = down ? Math.max(0, Math.min(100, 100 * ms / total)).toFixed(1) + '%' : '0';
        const go = el.querySelector('[data-a="go"]'), lbl = t(running ? 'wb_tm_pause' : 'wb_tm_start');
        if (go.textContent !== lbl) go.textContent = lbl;
      },
    },
  };
  const ORDER = ['status', 'metrics', 'events', 'clock', 'commits', 'tunnels', 'quick', 'sessions', 'links', 'notes', 'timer'];
  const SIZES = {status: [340, 380], metrics: [560, 420], events: [380, 420], clock: [300, 220], commits: [480, 400], tunnels: [340, 240], quick: [380, 200], sessions: [320, 300], links: [320, 280], notes: [320, 260], timer: [280, 210]};

  // ── state and persistence ──
  let S = null, saveT = 0, ticker = 0, unread = 0, appsCache = null, applying = 0, warnedBig = false;
  const ctxs = {}, bodies = {}, panels = {}, registered = new Set(), wins = {};
  const lsKey = () => 'wrm_workboard_' + ((user() && user().id) || 0);
  const canSave = () => !!user() && !(typeof isCollabMode !== 'undefined' && isCollabMode);
  const newId = () => { let s = ''; const c = 'abcdefghijklmnopqrstuvwxyz0123456789'; for (let i = 0; i < 8; i++) s += c[Math.floor(Math.random() * c.length)]; return S && S.tiles.some(x => x.id === s) ? newId() : s; };
  const nameOf = tile => (tile.cfg.title || '').trim() || t(A[tile.type].key);
  const frac = v => { v = Number(v); return v === v && isFinite(v) ? Math.max(0, Math.min(1, Math.round(v * 10000) / 10000)) : null; };

  function sanCfg(c) {
    c = c && typeof c === 'object' ? c : {};
    const cfg = {};
    if (typeof c.title === 'string') cfg.title = c.title.slice(0, 60);
    if (Array.isArray(c.ids)) cfg.ids = c.ids.map(Number).filter(n => n > 0).slice(0, STATUS_MAX);
    if (c.problems) cfg.problems = true;
    if (Array.isArray(c.zones)) cfg.zones = c.zones.filter(z => typeof z === 'string').map(z => z.slice(0, 60)).slice(0, 8);
    if (c.seconds === false) cfg.seconds = false;
    if (typeof c.app === 'string') cfg.app = c.app.slice(0, 200);
    if (typeof c.branch === 'string') cfg.branch = c.branch.slice(0, 200);
    if ([5, 10, 20].includes(Number(c.count))) cfg.count = Number(c.count);
    if (typeof c.text === 'string') cfg.text = c.text.slice(0, NOTE_MAX);
    if (Array.isArray(c.links)) cfg.links = c.links.filter(l => l && httpURL(l.url)).map(l => ({name: String(l.name || '').slice(0, 80), url: String(l.url).slice(0, 1000)})).slice(0, LINKS_MAX);
    if (ACCENTS[c.accent]) cfg.accent = c.accent;
    if (SCALES[c.scale] && c.scale !== 'm') cfg.scale = c.scale;
    if (c.mode === 'up') cfg.mode = 'up';
    if (Number(c.min) >= 1 && Number(c.min) <= 1440) cfg.min = Math.round(Number(c.min));
    for (const k of ['end', 'start', 'acc', 'left']) if (Number(c[k]) > 0) cfg[k] = Math.round(Number(c[k]));
    return cfg;
  }
  function sanTiles(list) {
    const out = [], seen = new Set();
    for (const tl of Array.isArray(list) ? list : []) {
      if (!tl || !A[tl.type] || typeof tl.id !== 'string' || !/^[a-z0-9]{1,16}$/.test(tl.id) || seen.has(tl.id)) continue;
      seen.add(tl.id);
      const tile = {id: tl.id, type: tl.type, cfg: sanCfg(tl.cfg)};
      const g = tl.g;
      if (g && typeof g === 'object') {
        const n = {l: frac(g.l), t: frac(g.t), w: frac(g.w), h: frac(g.h)};
        if (n.l !== null && n.t !== null && n.w > 0.02 && n.h > 0.02) tile.g = n;
      }
      if (SNAPS.includes(tl.snap)) tile.snap = tl.snap;
      if (tl.min) tile.min = true;
      if (tl.pin) tile.pin = true;
      if (tl.top) tile.top = true;
      if (tl.size === 'm' && !tile.g) tile.wide = true; // v12.6.9 wide tiles open wider
      out.push(tile);
      if (out.length >= MAX_TILES) break;
    }
    return out;
  }
  function sanCanvas(c) {
    c = c && typeof c === 'object' ? c : {};
    return {grid: !!c.grid, gs: GRIDS.includes(Number(c.gs)) ? Number(c.gs) : 20, bg: BGS.includes(c.bg) ? c.bg : 'grid'};
  }
  function sanitize(x) {
    x = x && typeof x === 'object' ? x : {};
    const out = {v: 2, t: Number(x.t) || 0, tiles: sanTiles(x.tiles), canvas: sanCanvas(x.canvas), saved: []};
    for (const sv of Array.isArray(x.saved) ? x.saved : []) {
      if (!sv || typeof sv.name !== 'string' || !sv.name.trim()) continue;
      const n = {name: sv.name.trim().slice(0, 60), at: String(sv.at || '').slice(0, 40), tiles: sanTiles(sv.tiles), canvas: sanCanvas(sv.canvas)};
      if (sv.layout && typeof sv.layout === 'object') n.layout = sv.layout;
      if (sv.wins && Array.isArray(sv.wins.windows)) n.wins = {v: 1, windows: sv.wins.windows.slice(0, 40)};
      if (!out.saved.some(o => o.name === n.name)) out.saved.push(n);
      if (out.saved.length >= SAVED_MAX) break;
    }
    return out;
  }
  function saveSoon() {
    if (!S || !canSave()) return;
    S.t = Date.now();
    try { localStorage.setItem(lsKey(), JSON.stringify(S)); } catch (e) { /* only on the server then */ }
    clearTimeout(saveT);
    saveT = setTimeout(push, 800);
  }
  function push(keepalive) {
    saveT = 0;
    if (!S || !canSave()) return;
    fetch('/api/me/ui/workboard', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({value: S}), keepalive: !!keepalive})
      .then(r => { if (r.status === 400 && !warnedBig) { warnedBig = true; showToast(t('wb_too_big'), 'error', 8000); } else if (r.ok) warnedBig = false; }).catch(() => {});
  }
  window.addEventListener('pagehide', () => { if (saveT) { clearTimeout(saveT); push(true); } });

  async function init() {
    const hb = $('hint-add-addon');
    if (!canSave()) { const b = $('btn-workboard'); if (b) b.style.display = 'none'; if (hb) hb.style.display = 'none'; return; }
    if (hb) hb.style.display = '';
    let local = null;
    try { local = JSON.parse(localStorage.getItem(lsKey()) || 'null'); } catch (e) { /* none */ }
    S = sanitize(local || {});
    applyCanvas();
    whenCanvas(reconcile);
    if (!ticker) ticker = setInterval(tick, 1000);
    try {
      const r = await fetch('/api/me/ui/workboard');
      if (r.ok) {
        const j = await r.json(), v = j && j.value, vt = (v && Number(v.t)) || 0;
        if (v && vt > (S.t || 0)) {
          resetAll();
          S = sanitize(v);
          try { localStorage.setItem(lsKey(), JSON.stringify(S)); } catch (e) { /* not cached */ }
          applyCanvas();
          whenCanvas(reconcile);
        } else if (local && (S.t || 0) > vt) push();
      }
    } catch (e) { /* offline: the copy in this browser */ }
    const ev = S.tiles.find(x => x.type === 'events');
    if (ev) loadCtx(ev);
  }
  document.addEventListener('wrm-layout-loaded', () => { if (S) reconcile(); });
  // Addon windows need a measured canvas for their place.
  function whenCanvas(fn, n) {
    const c = $('win-container');
    if (c && c.clientWidth && c.clientHeight) { fn(); return; }
    if ((n || 0) < 100) setTimeout(() => whenCanvas(fn, (n || 0) + 1), 50); else fn();
  }

  // ── canvas: grid and background ──
  function applyCanvas() {
    const c = $('win-container');
    if (!c || !S) return;
    const cv = S.canvas, bg = cv.bg === 'grid' ? (cv.grid ? 'dots' : 'none') : cv.bg;
    c.classList.toggle('cv-dots', bg === 'dots');
    c.classList.toggle('cv-lines', bg === 'lines');
    c.style.setProperty('--cv-gs', cv.gs + 'px');
  }
  const grid = () => (S && S.canvas.grid && !compact() ? S.canvas.gs : 0);

  // ── tile bodies, addon windows and panels ──
  function bodyOf(tile) {
    let b = bodies[tile.id];
    if (!b) { b = document.createElement('div'); b.className = 'wb-body'; bodies[tile.id] = b; }
    b.style.zoom = SCALES[tile.cfg.scale] || 1;
    return b;
  }
  const ctxOf = tile => ctxs[tile.id] || (ctxs[tile.id] = {});
  const pid = tile => 'wb:' + tile.id;
  const tileById = id => S && S.tiles.find(x => x.id === id);
  const tileOfWin = wo => wo && tileById(wo.addon);
  const docked = tile => !compact() && !!window.wrmLayout && registered.has(tile.id) && wrmLayout.where(pid(tile)) !== 'hidden';

  // A spot on the canvas where a new addon window overlaps nothing (maximized windows aside),
  // scanning from the top right so addons gather beside the terminals.
  function freeSpot(w, h, self) {
    const c = $('win-container'), cw = c.clientWidth, ch = c.clientHeight, step = grid() || 20, gap = 8;
    const rects = [];
    winObjects.forEach(o => { if (o !== self && o.el && o.el.style.display !== 'none' && o.snap !== 'full' && !o.el.classList.contains('folder-hidden')) rects.push([o.el.offsetLeft, o.el.offsetTop, o.el.offsetWidth, o.el.offsetHeight]); });
    document.querySelectorAll('#win-container > .dk-float').forEach(f => rects.push([f.offsetLeft, f.offsetTop, f.offsetWidth, f.offsetHeight]));
    for (let y = 0; y + h <= ch; y += step) {
      for (let x = cw - w; x >= 0; x -= step) {
        if (!rects.some(([l, tp, rw, rh]) => x < l + rw + gap && x + w + gap > l && y < tp + rh + gap && y + h + gap > tp)) return {x, y};
      }
    }
    const n = Object.keys(wins).length % 8;
    return {x: Math.max(0, cw - w - 30 * n), y: Math.min(Math.max(0, ch - h), 30 * n)};
  }
  function applyGeo(tile, wo) {
    if (compact()) return;
    const el = wo.el, c = $('win-container'), cw = c.clientWidth || 1, ch = c.clientHeight || 1;
    if (tile.snap) { snapWindow(wo.id, tile.snap); return; }
    if (wo.snap) { wo.snap = null; wo.maximized = false; el.classList.remove('snapped-full', 'snapped'); }
    let g = tile.g;
    if (!g) {
      const sz = SIZES[tile.type] || [340, 300], w = Math.min(cw, tile.wide ? Math.max(sz[0], 560) : sz[0]), h = Math.min(ch, sz[1]);
      const p = freeSpot(w, h, wo);
      g = tile.g = {l: frac(p.x / cw), t: frac(p.y / ch), w: frac(w / cw), h: frac(h / ch)};
      delete tile.wide;
      saveSoon();
    }
    Object.assign(el.style, {left: Math.round(g.l * cw) + 'px', top: Math.round(g.t * ch) + 'px', width: Math.round(g.w * cw) + 'px', height: Math.round(g.h * ch) + 'px'});
    clampWinToBounds(el);
  }
  function decorate(tile, wo) {
    const def = A[tile.type], el = wo.el;
    const tt = $('wt-' + wo.id); if (tt) tt.textContent = def.ico + ' ' + nameOf(tile);
    el.classList.toggle('wb-acc', !!ACCENTS[tile.cfg.accent]);
    if (ACCENTS[tile.cfg.accent]) el.style.setProperty('--wb-accent', ACCENTS[tile.cfg.accent]); else el.style.removeProperty('--wb-accent');
    el.querySelectorAll('.win-btn.wb-a').forEach(b => {
      const a = b.dataset.a;
      b.title = t(a === 'refresh' ? 'wb_t_refresh' : a === 'set' ? 'wb_t_settings' : 'wb_t_dock');
      if (a === 'refresh') b.style.display = def.refresh ? '' : 'none';
      if (a === 'dock') b.style.display = compact() ? 'none' : '';
    });
    bodyOf(tile);
  }
  function openWin(tile) {
    if (wins[tile.id]) return wins[tile.id];
    if (typeof addWindow !== 'function') return null;
    applying++;
    let wo;
    try {
      wo = addWindow({mode: 'addon'});
      wo.addon = tile.id;
      wins[tile.id] = wo;
      const el = wo.el;
      el.classList.add('addon');
      el.dataset.addon = tile.type;
      const wc = $('wc-' + wo.id);
      wc.innerHTML = '';
      wc.classList.add('wb-wc');
      wc.appendChild(bodyOf(tile));
      const btns = el.querySelector('.win-btns'), minB = btns.querySelector('.win-btn.minimize');
      [['refresh', '↻'], ['set', '⚙'], ['dock', '⇥']].forEach(([a, txt]) => {
        const b = document.createElement('button');
        b.type = 'button'; b.className = 'win-btn wb-a'; b.dataset.a = a; b.textContent = txt;
        b.onclick = e => { e.stopPropagation(); act(wo.id, a, b); };
        btns.insertBefore(b, minB);
      });
      decorate(tile, wo);
      applyGeo(tile, wo);
      setWinPinned(wo, tile.pin);
      setWinOnTop(wo, tile.top);
      if (tile.min) el.style.display = 'none';
    } finally { applying--; }
    paint(tile, true);
    renderTabs();
    return wo;
  }
  // windowMoved keeps the addon's place: fractions of the canvas, so it survives other screens.
  function windowMoved(wo) {
    if (applying || !S) return;
    const tile = tileOfWin(wo);
    if (!tile) return;
    const el = wo.el;
    tile.min = el.style.display === 'none' || undefined;
    tile.pin = !!wo.pinned || undefined;
    tile.top = !!wo.onTop || undefined;
    if (!compact()) {
      tile.snap = wo.snap || undefined;
      if (!wo.snap) {
        const c = $('win-container'), cw = c.clientWidth || 1, ch = c.clientHeight || 1;
        const n = {l: frac((parseInt(el.style.left, 10) || 0) / cw), t: frac((parseInt(el.style.top, 10) || 0) / ch), w: frac((parseInt(el.style.width, 10) || el.offsetWidth) / cw), h: frac((parseInt(el.style.height, 10) || el.offsetHeight) / ch)};
        if (n.w > 0.02 && n.h > 0.02) tile.g = n;
      }
    }
    saveSoon();
  }
  function windowGone(wo) { if (wo && wins[wo.addon] === wo) delete wins[wo.addon]; }
  function closeWinOf(tile) {
    const wo = wins[tile.id];
    if (!wo) return;
    delete wins[tile.id];
    const b = bodies[tile.id];
    if (b && b.parentElement && b.parentElement.id === 'wc-' + wo.id) b.remove();
    closeWindow(wo.id);
  }

  function panelOf(tile) {
    let p = panels[tile.id];
    if (!p) {
      p = document.createElement('div');
      p.id = 'pnl-wb-' + tile.id; p.className = 'wb-pnl';
      p.innerHTML = `<div class="wb-pbar"><span class="wb-upd"></span><button type="button" class="wb-ib" data-a="refresh">↻</button><button type="button" class="wb-ib" data-a="set">⚙</button><button type="button" class="wb-ib" data-a="undock">⧉</button></div>`;
      p.querySelector('[data-a="set"]').onclick = () => { const tl = tileById(tile.id); if (tl) settings(tl); };
      p.querySelector('[data-a="refresh"]').onclick = () => { const tl = tileById(tile.id); if (tl) loadCtx(tl, true); };
      p.querySelector('[data-a="undock"]').onclick = () => { if (window.wrmLayout) wrmLayout.hide(pid(tile)); };
      panels[tile.id] = p;
    }
    p.querySelector('[data-a="refresh"]').style.display = A[tile.type].refresh ? '' : 'none';
    p.querySelector('[data-a="refresh"]').title = t('wb_t_refresh');
    p.querySelector('[data-a="set"]').title = t('wb_t_settings');
    p.querySelector('[data-a="undock"]').title = t('dk_wb_back');
    return p;
  }
  function registerPanel(tile, place) {
    const p = panelOf(tile), b = bodyOf(tile);
    if (b.parentElement !== p) p.appendChild(b);
    const ok = wrmLayout.register(pid(tile), {
      ico: A[tile.type].ico, el: p.id, node: p,
      label: () => { const tl = tileById(tile.id); return tl ? nameOf(tl) : ''; },
      shown: () => { const tl = tileById(tile.id); if (tl) paint(tl, true); },
      closed: () => { registered.delete(tile.id); reconcile(); },
    }, place);
    if (ok) registered.add(tile.id);
    return ok;
  }
  // reconcile matches the tiles with the canvas: a tile the layout places as a panel is a panel,
  // every other tile is a window; a placed panel id without a tile is dropped.
  function reconcile() {
    if (!S) return;
    if (window.wrmLayout && wrmLayout.ready()) {
      const placed = new Set(wrmLayout.placed());
      for (const id of placed) if (!S.tiles.some(x => pid(x) === id)) wrmLayout.remove(id);
      for (const tile of S.tiles) {
        if (placed.has(pid(tile)) && !compact()) { if (!registered.has(tile.id)) registerPanel(tile); }
        else if (registered.has(tile.id)) { registered.delete(tile.id); wrmLayout.remove(pid(tile)); }
      }
    }
    for (const tile of S.tiles) {
      if (registered.has(tile.id)) closeWinOf(tile);
      else if (!wins[tile.id]) openWin(tile);
    }
    for (const id of Object.keys(wins)) if (!tileById(id)) { const wo = wins[id]; delete wins[id]; closeWindow(wo.id); }
    for (const id of Object.keys(bodies)) if (!tileById(id)) { bodies[id].remove(); delete bodies[id]; delete ctxs[id]; if (panels[id]) { panels[id].remove(); delete panels[id]; } }
    updateBadge();
  }
  // resetAll takes every addon off the canvas (before another state replaces this one).
  function resetAll() {
    if (!S) return;
    for (const tile of S.tiles) {
      if (registered.has(tile.id)) { registered.delete(tile.id); if (window.wrmLayout) wrmLayout.remove(pid(tile)); }
      closeWinOf(tile);
    }
    for (const id of Object.keys(bodies)) { bodies[id].remove(); delete bodies[id]; delete ctxs[id]; }
    for (const id of Object.keys(panels)) { panels[id].remove(); delete panels[id]; }
  }

  // ── addon actions ──
  function act(winId, a, anchor) {
    const wo = winObjects.find(w => w.id === winId), tile = tileOfWin(wo);
    if (!tile) return;
    if (a === 'set') settings(tile);
    else if (a === 'refresh') loadCtx(tile, true);
    else if (a === 'dock') {
      if (locked()) { showToast(t('wb_locked'), 'warning', 3500); return; }
      wrmLayout.menu(anchor || wo.el.querySelector('.win-btn.wb-a[data-a="dock"]'), [{head: t('wb_t_dock')},
        ...[['right', '⇥', 'dk_move_right'], ['left', '⇤', 'dk_move_left'], ['bottom', '⤓', 'dk_move_bottom']].map(([z, ico, k]) => ({ico, label: t(k), do: () => dock(tile, z)}))]);
    } else if (a === 'remove') closeAddon(wo);
  }
  function dock(tile, zone) {
    if (!window.wrmLayout || compact()) return;
    if (locked()) { showToast(t('wb_locked'), 'warning', 3500); return; }
    const wo = wins[tile.id];
    if (wo) windowMoved(wo);
    if (!registerPanel(tile, zone)) return;
    closeWinOf(tile);
    wrmLayout.activate(pid(tile), {reveal: true});
    renderTabs();
  }
  function hasData(tile) {
    const c = tile.cfg;
    return !!((c.text || '').trim() || (c.ids || []).length || (c.links || []).length || c.app || c.end || c.start || c.acc);
  }
  async function closeAddon(wo) {
    const tile = tileOfWin(wo);
    if (!tile) { closeWindow(wo.id); return; }
    if (hasData(tile) && !await uiConfirm(tn('wb_remove_q', {name: nameOf(tile)}), {danger: true, okText: t('wb_t_remove')})) return;
    removeTile(tile);
    showToast(tn('wb_removed', {name: nameOf(tile)}), 'success', 2000);
  }
  function removeTile(tile) {
    if (registered.has(tile.id)) { registered.delete(tile.id); wrmLayout.remove(pid(tile)); }
    closeWinOf(tile);
    S.tiles = S.tiles.filter(x => x !== tile);
    saveSoon(); reconcile();
  }

  // ── painting and loading ──
  // A window counts as visible when part of it is not covered by another window.
  function seen(el) {
    if (!visible(el)) return false;
    const w = el.closest('.win');
    if (!w) return true;
    if (w.style.display === 'none' || w.classList.contains('folder-hidden')) return false;
    const r = w.getBoundingClientRect();
    const pts = [[0.5, 0.5], [0.15, 0.2], [0.85, 0.2], [0.15, 0.85], [0.85, 0.85]];
    return pts.some(([fx, fy]) => { const e = document.elementFromPoint(r.left + r.width * fx, r.top + r.height * fy); return !!e && w.contains(e); });
  }
  function paint(tile, force) {
    const def = A[tile.type], el = bodyOf(tile), ctx = ctxOf(tile);
    const why = def.avail ? def.avail() : '';
    if (why) { el.innerHTML = `<div class="wb-hint">${esc(why)}</div>`; el._last = ''; ctx.sig = ''; return; }
    if (def.sig) { const s = def.sig(tile); if (!force && s === ctx.sig && el.childElementCount) return; ctx.sig = s; }
    try { def.render(tile, el, ctx); } catch (e) { el.innerHTML = `<div class="wb-err">⚠ ${esc(String(e.message || e))}</div>`; }
    const upd = def.load && ctx.at ? tn('wb_updated', {when: ago(ctx.at)}) : '';
    const p = el.closest('.wb-pnl'), u = p && p.querySelector('.wb-upd');
    if (u) u.textContent = upd;
    const wo = wins[tile.id];
    if (wo) { const tt = $('wt-' + wo.id); if (tt) tt.title = upd; }
    if (def.load && !ctx.at && !ctx.busy) loadCtx(tile);
  }
  async function loadCtx(tile, force) {
    const def = A[tile.type], ctx = ctxOf(tile);
    if (!def.load || ctx.busy || (def.avail && def.avail())) return;
    // addons that name connections wait until the connection list has loaded
    if ((def.fields || []).includes('conns') && !allConns().length) return;
    ctx.busy = true; ctx.force = !!force;
    try { await def.load(tile, ctx); } finally { ctx.busy = false; ctx.force = false; ctx.at = new Date().toISOString(); }
    if (tileById(tile.id)) paint(tile, true);
  }
  function reloadType(type) { if (S) S.tiles.filter(x => x.type === type).forEach(x => loadCtx(x, true)); }
  async function markRead(body) {
    await fetch('/api/me/events/read', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)}).catch(() => {});
    reloadType('events');
  }
  function setUnread(n) { unread = n; updateBadge(); }
  function updateBadge() {
    const b = $('wb-badge');
    if (!b) return;
    const on = !!(S && S.tiles.some(x => x.type === 'events')) && unread > 0;
    b.style.display = on ? '' : 'none';
    b.textContent = on ? (unread > 99 ? '99+' : String(unread)) : '';
  }
  let secs = 0;
  function tick() {
    if (!S || !S.tiles.length) return;
    secs++;
    for (const tile of S.tiles) {
      const def = A[tile.type], el = bodies[tile.id];
      // a countdown ends on time even while its window is minimized or covered
      if (tile.type === 'timer' && tile.cfg.end && tile.cfg.mode !== 'up' && Date.now() >= tile.cfg.end) { paint(tile, true); continue; }
      if (!el || !seen(el)) continue;
      const ctx = ctxOf(tile);
      if (def.tick) paint(tile);
      else if (def.sig && secs % 2 === 0) paint(tile);
      if (def.load && def.every && !ctx.busy && (!ctx.at || Date.now() - new Date(ctx.at).getTime() > def.every * 1000)) loadCtx(tile);
      else if (def.load && secs % 30 === 0) paint(tile, true);
    }
  }

  // ── the ▤ Workboard menu: the canvas tools ──
  function menu(anchor, reopen) {
    if (!S || !window.wrmLayout) return;
    anchor = anchor || $('btn-workboard');
    const cv = S.canvas, lk = locked(), cp = compact();
    const anyMin = winObjects.some(w => w.el && w.el.style.display === 'none');
    const addons = winObjects.filter(w => w.mode === 'addon' && w.el);
    const addonsHidden = addons.length && addons.every(w => w.el.style.display === 'none');
    const again = () => menu(anchor, true);
    const items = [{head: '▤ ' + t('wb_title')},
      {ico: '＋', label: t('wb_add') + '…', do: () => setTimeout(catalog, 0)}];
    if (!cp) {
      items.push('-', {head: t('wb_m_canvas')},
        {check: cv.grid, label: t('wb_m_grid'), title: t('wb_m_grid_t'), do: () => { cv.grid = !cv.grid; saveSoon(); applyCanvas(); }, again},
        {ico: '▦', label: t('wb_m_gsize'), side: cv.gs + ' px', do: () => { cv.gs = GRIDS[(GRIDS.indexOf(cv.gs) + 1) % GRIDS.length]; saveSoon(); applyCanvas(); }, again},
        {ico: '░', label: t('wb_m_bg'), side: t('wb_bg_' + cv.bg), do: () => { cv.bg = BGS[(BGS.indexOf(cv.bg) + 1) % BGS.length]; saveSoon(); applyCanvas(); }, again},
        '-',
        {ico: '⊞', label: t('wb_m_tile'), off: lk, title: lk ? t('dk_locked_t') : '', do: () => arrangeWindows('tile')},
        {ico: '⧉', label: t('wb_m_cascade'), off: lk, title: lk ? t('dk_locked_t') : '', do: () => arrangeWindows('cascade')},
        {ico: '▁', label: t('wb_m_min_all'), do: () => arrangeWindows('min')});
      if (anyMin) items.push({ico: '▢', label: t('wb_m_show_all'), do: () => arrangeWindows('show')});
      if (addons.length) items.push({ico: '▤', label: t(addonsHidden ? 'wb_m_show_addons' : 'wb_m_hide_addons'), do: () => arrangeWindows(addonsHidden ? 'show' : 'min', w => w.mode === 'addon')});
      items.push({ico: lk ? '🔓' : '🔒', label: t(lk ? 'dk_unlock' : 'dk_lock'), do: () => wrmLayout.setLocked(!lk), again});
    }
    items.push('-', {head: t('wb_m_layouts')});
    S.saved.forEach(sv => items.push({ico: '▸', label: sv.name, side: sv.at ? fmtDay(sv.at) : '', do: () => applySaved(sv.name)}));
    items.push({ico: '💾', label: t('wb_m_save'), do: () => setTimeout(() => savedDialog(true), 0)});
    if (S.saved.length) items.push({ico: '☰', label: t('wb_m_manage'), do: () => setTimeout(() => savedDialog(false), 0)});
    items.push({ico: '⇩', label: t('wb_m_export'), do: exportLayout}, {ico: '⇧', label: t('wb_m_import'), do: importLayout});
    if (!cp) items.push('-', {ico: '▦', label: t('dk_panels') + '…', do: () => setTimeout(() => wrmLayout.openPanels(anchor), 0)});
    items.push({hint: t('wb_m_hint')});
    items.keep = !!reopen;
    wrmLayout.menu(anchor, items);
  }
  function fmtDay(at) { const d = new Date(at); return isNaN(d) ? '' : d.toLocaleDateString(typeof currentLang !== 'undefined' && currentLang === 'hr' ? 'hr-HR' : undefined, {day: 'numeric', month: 'short'}); }

  // ── saved layouts, export and import ──
  function snapshot(withWins) {
    Object.values(wins).forEach(windowMoved); // the places as they are on screen now
    const sv = {tiles: JSON.parse(JSON.stringify(S.tiles)), canvas: Object.assign({}, S.canvas)};
    const lay = window.wrmLayout && wrmLayout.snapshot();
    if (lay) sv.layout = lay;
    if (withWins && typeof captureLayout === 'function') { const w = captureLayout(); if (w.windows.length) sv.wins = w; }
    return sv;
  }
  async function applySnap(sv, name) {
    resetAll();
    S.tiles = sanTiles(sv.tiles);
    S.canvas = sanCanvas(sv.canvas);
    applyCanvas();
    if (sv.layout && window.wrmLayout) wrmLayout.apply(sv.layout); // reconciles through wrm-layout-loaded
    saveSoon();
    reconcile();
    const ws = sv.wins && Array.isArray(sv.wins.windows) ? sv.wins : null;
    if (ws && ws.windows.length) {
      const busy = winObjects.some(w => w.mode && w.mode !== 'addon');
      let how = 'replace';
      if (busy) {
        const r = await uiDialog({title: name, message: tn('wb_sv_wins_q', {n: ws.windows.length}), okText: t('wb_sv_wins_replace'), altText: t('wb_sv_wins_add')});
        if (!r) return;
        how = r === 'alt' ? 'add' : 'replace';
      }
      restoreWindowLayout(ws, how);
    }
  }
  async function applySaved(name) {
    const sv = S.saved.find(x => x.name === name);
    if (!sv) return;
    if (!await uiConfirm(tn('wb_sv_apply_q', {name}), {okText: t('wb_sv_apply')})) return;
    await applySnap(sv, name);
    showToast(tn('wb_sv_applied', {name}), 'success', 2500);
  }
  function savedDialog(focusSave) {
    const list = S.saved.map(sv => {
      const floats = sv.layout && Array.isArray(sv.layout.floats) ? sv.layout.floats.length : 0;
      const what = sv.wins ? tn('wb_sv_what_w', {a: sv.tiles.length, w: sv.wins.windows.length}) : tn('wb_sv_what', {a: sv.tiles.length, p: floats});
      return `<div class="wb-ci"><span class="ico">▤</span><div class="tx"><div class="nm">${esc(sv.name)}</div><div class="ds">${esc(what)}${sv.at ? ' · ' + esc(fmtTime(sv.at)) : ''}</div></div><button type="button" class="btn-sm" data-ap="${esc(sv.name)}">${esc(t('wb_sv_apply'))}</button><button type="button" class="btn-danger btn-sm" data-del="${esc(sv.name)}" title="${esc(t('wb_sv_delete'))}">✕</button></div>`;
    }).join('');
    const m = modal('modal-wb-saved', `<h3>▤ ${esc(t('wb_sv_title'))}</h3>` +
      `<div class="field"><label>${esc(t('wb_sv_name'))}</label><input type="text" id="wbsv-name" maxlength="60" placeholder="${esc(t('wb_sv_name_ph'))}"></div>` +
      `<label class="wb-chk"><input type="checkbox" id="wbsv-wins"> ${esc(t('wb_sv_terms'))}</label><div class="muted-text" style="line-height:1.5;margin:-4px 0 10px;">${esc(t('wb_sv_terms_d'))}</div>` +
      `<div style="display:flex;justify-content:flex-end;margin-bottom:12px;"><button type="button" data-a="save">💾 ${esc(t('wb_sv_save'))}</button></div>` +
      `<div class="wb-cat">${list || `<div class="wb-hint">${esc(t('wb_sv_none'))}</div>`}</div>` +
      `<div class="modal-footer"><button class="btn-sec" data-a="close">${esc(t('close'))}</button></div>`);
    const q = s => m.querySelector(s);
    q('[data-a="close"]').onclick = () => closeModal('modal-wb-saved');
    q('#wbsv-name').onkeydown = e => { if (e.key === 'Enter') { e.preventDefault(); q('[data-a="save"]').click(); } };
    q('[data-a="save"]').onclick = async () => {
      const name = q('#wbsv-name').value.trim().slice(0, 60);
      if (!name) { q('#wbsv-name').focus(); return; }
      const i = S.saved.findIndex(x => x.name === name);
      if (i < 0 && S.saved.length >= SAVED_MAX) { showToast(tn('wb_sv_max', {n: SAVED_MAX}), 'warning', 4000); return; }
      if (i >= 0 && !await uiConfirm(tn('wb_sv_replace_q', {name}), {okText: t('wb_sv_save')})) return;
      const sv = Object.assign({name, at: new Date().toISOString()}, snapshot(q('#wbsv-wins').checked));
      if (i >= 0) S.saved[i] = sv; else S.saved.push(sv);
      saveSoon();
      showToast(tn('wb_sv_saved', {name}), 'success', 2500);
      savedDialog(false);
    };
    m.querySelectorAll('[data-ap]').forEach(b => { b.onclick = async () => { closeModal('modal-wb-saved'); await applySaved(b.dataset.ap); }; });
    m.querySelectorAll('[data-del]').forEach(b => {
      b.onclick = async () => {
        const name = b.dataset.del;
        if (!await uiConfirm(tn('wb_sv_delete_q', {name}), {danger: true, okText: t('wb_sv_delete')})) return;
        S.saved = S.saved.filter(x => x.name !== name);
        saveSoon(); savedDialog(false);
      };
    });
    if (focusSave) setTimeout(() => q('#wbsv-name').focus(), 30);
  }
  function exportLayout() {
    const data = {format: 'wrm-workboard', v: 1, exported: new Date().toISOString(), app: typeof AppVersion !== 'undefined' ? AppVersion : undefined, workboard: Object.assign(snapshot(false), {saved: S.saved})};
    const blob = new Blob([JSON.stringify(data, null, 2)], {type: 'application/json'});
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'wrm-workboard-' + new Date().toISOString().slice(0, 10) + '.json';
    document.body.appendChild(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 2000);
  }
  function importLayout() {
    const inp = document.createElement('input');
    inp.type = 'file'; inp.accept = 'application/json,.json';
    inp.onchange = async () => {
      const f = inp.files && inp.files[0];
      if (!f) return;
      let data = null;
      try { if (f.size > 512 * 1024) throw new Error('size'); data = JSON.parse(await f.text()); } catch (e) { data = null; }
      if (!data || data.format !== 'wrm-workboard' || !data.workboard || typeof data.workboard !== 'object') { showToast(t('wb_imp_bad'), 'error', 5000); return; }
      if (!await uiConfirm(t('wb_imp_q'), {okText: t('wb_m_import')})) return;
      const wb = data.workboard;
      const extra = sanitize({saved: wb.saved}).saved.filter(sv => !S.saved.some(x => x.name === sv.name));
      S.saved = S.saved.concat(extra).slice(0, SAVED_MAX);
      await applySnap({tiles: wb.tiles, canvas: wb.canvas, layout: wb.layout}, f.name);
      showToast(t('wb_imp_ok'), 'success', 2500);
    };
    inp.click();
  }

  // ── catalog and settings dialogs ──
  function modal(id, html) {
    let m = $(id);
    if (!m) { m = document.createElement('div'); m.className = 'modal-overlay'; m.id = id; document.body.appendChild(m); m.addEventListener('mousedown', e => { if (e.target === m) closeModal(id); }); }
    m.innerHTML = `<div class="modal wb-modal">${html}</div>`;
    m.classList.add('open');
    return m;
  }
  function catalog() {
    if (!S) return;
    const m = modal('modal-wb-catalog', `<h3>▤ ${esc(t('wb_catalog'))}</h3><div class="muted-text" style="line-height:1.5;margin-bottom:10px;">${esc(t('wb_catalog_hint'))}</div><div class="wb-cat">${ORDER.map(k => {
      const d = A[k], why = d.avail ? d.avail() : '', n = S.tiles.filter(x => x.type === k).length;
      return `<div class="wb-ci"><span class="ico">${esc(d.ico)}</span><div class="tx"><div class="nm">${esc(t(d.key))}${n ? ` <span class="wb-n">×${n}</span>` : ''}</div><div class="ds">${esc(why || t(d.desc))}</div></div><button type="button" class="btn-sm" data-k="${k}" ${why ? 'disabled' : ''}>${esc(why ? t('wb_unavailable') : t('wb_add_btn'))}</button></div>`;
    }).join('')}</div><div class="modal-footer"><button class="btn-sec" data-a="close">${esc(t('close'))}</button></div>`);
    m.querySelector('[data-a="close"]').onclick = () => closeModal('modal-wb-catalog');
    m.querySelectorAll('[data-k]').forEach(b => { b.onclick = () => add(b.dataset.k); });
  }
  function add(type) {
    if (S.tiles.length >= MAX_TILES) { showToast(tn('wb_max', {n: MAX_TILES}), 'warning', 3500); return; }
    const tile = {id: newId(), type, cfg: {}};
    if (type === 'clock') tile.cfg.zones = ['UTC'];
    if (type === 'commits') tile.cfg.count = 10;
    if (type === 'timer') tile.cfg.min = 25;
    S.tiles.push(tile);
    saveSoon();
    closeModal('modal-wb-catalog');
    if (typeof closeDrawers === 'function') closeDrawers();
    const wo = openWin(tile);
    if (wo) focusWindow(wo.id);
    updateBadge();
    showToast(tn('wb_added', {name: t(A[type].key)}), 'success', 2200);
    if ((A[type].fields || []).some(f => f === 'conns' || f === 'app' || f === 'links') && type !== 'status') settings(tile);
  }
  async function settings(tile) {
    const def = A[tile.type], f = def.fields || [], cfg = tile.cfg;
    let html = `<h3>${esc(def.ico)} ${esc(t(def.key))}</h3>` +
      `<div class="field"><label>${esc(t('wb_set_title'))}</label><input type="text" id="wbs-title" maxlength="60" placeholder="${esc(t('wb_set_title_ph'))}" value="${esc(cfg.title || '')}"></div>` +
      `<div class="wb-2col"><div class="field"><label>${esc(t('wb_set_accent'))}</label><select id="wbs-accent"><option value="">${esc(t('wb_acc_none'))}</option>${Object.keys(ACCENTS).map(k => `<option value="${k}">${esc(t('wb_acc_' + k))}</option>`).join('')}</select></div>` +
      `<div class="field"><label>${esc(t('wb_set_scale'))}</label><select id="wbs-scale">${Object.keys(SCALES).map(k => `<option value="${k}">${esc(t('wb_sc_' + k))}</option>`).join('')}</select></div></div>`;
    if (f.includes('conns')) {
      const list = allConns().filter(def.connFilter).sort((a, b) => a.name.localeCompare(b.name));
      const sel = new Set(cfg.ids || []);
      html += `<div class="field"><label>${esc(t('wb_set_conns'))}</label><div class="muted-text" style="margin-bottom:6px;">${esc(tile.type === 'status' ? t('wb_set_conns_all') : tn('wb_set_conns_need', {n: def.connMax}))}</div>` +
        `<input type="text" id="wbs-filter" placeholder="${esc(t('wb_set_filter'))}"><div class="wb-pick" id="wbs-conns">${list.map(c => `<label class="wb-pi" data-q="${esc((c.name + ' ' + (c.host || '') + ' ' + (c.folder_path || '')).toLowerCase())}"><input type="checkbox" value="${c.id}" ${sel.has(c.id) ? 'checked' : ''}><span class="nm">${esc(c.name)}</span><span class="sd">${esc(c.protocol)} · ${esc(c.host || '')}</span></label>`).join('') || `<div class="wb-hint sm">${esc(t('wb_none'))}</div>`}</div></div>`;
      if (tile.type === 'metrics') html += `<div class="muted-text" style="line-height:1.5;">${esc(t('wb_metrics_help'))}</div>`;
    }
    if (f.includes('problems')) html += `<label class="wb-chk"><input type="checkbox" id="wbs-problems" ${cfg.problems ? 'checked' : ''}> ${esc(t('wb_status_problems'))}</label>`;
    if (f.includes('zones')) html += `<div class="field"><label>${esc(t('wb_clock_zones'))}</label><input type="text" id="wbs-zones" placeholder="${esc(t('wb_clock_zones_ph'))}" value="${esc((cfg.zones || []).join(', '))}"></div>`;
    if (f.includes('seconds')) html += `<label class="wb-chk"><input type="checkbox" id="wbs-seconds" ${cfg.seconds !== false ? 'checked' : ''}> ${esc(t('wb_clock_seconds'))}</label>`;
    if (f.includes('app')) html += `<div class="field"><label>${esc(t('wb_commits_service'))}</label><select id="wbs-app"><option value="">${esc(t('wb_loading'))}</option></select></div>` +
      `<div class="field"><label>${esc(t('wb_commits_branch'))}</label><input type="text" id="wbs-branch" maxlength="200" placeholder="${esc(t('wb_commits_branch_ph'))}" value="${esc(cfg.branch || '')}"></div>` +
      `<div class="field"><label>${esc(t('wb_commits_count'))}</label><select id="wbs-count">${[5, 10, 20].map(n => `<option value="${n}" ${Number(cfg.count || 10) === n ? 'selected' : ''}>${n}</option>`).join('')}</select></div>`;
    if (f.includes('links')) html += `<div class="field"><label>${esc(t('wb_links_field'))}</label><textarea id="wbs-links" rows="7" style="width:100%;font-family:var(--font-mono);font-size:12px;" placeholder="Wiki | https://wiki.example.com">${esc((cfg.links || []).map(l => (l.name ? l.name + ' | ' : '') + l.url).join('\n'))}</textarea></div>`;
    if (f.includes('timer')) html += `<div class="wb-2col"><div class="field"><label>${esc(t('wb_tm_mode'))}</label><select id="wbs-tmode"><option value="down">${esc(t('wb_tm_down'))}</option><option value="up">${esc(t('wb_tm_up'))}</option></select></div>` +
      `<div class="field"><label>${esc(t('wb_tm_min'))}</label><input type="number" id="wbs-tmin" min="1" max="1440" value="${Number(cfg.min) || 25}"></div></div>`;
    html += `<div class="modal-footer"><button class="btn-danger" data-a="rm" style="margin-right:auto;">${esc(t('wb_t_remove'))}</button><button class="btn-sec" data-a="cancel">${esc(t('cancel'))}</button><button data-a="save">${esc(t('save'))}</button></div>`;
    const m = modal('modal-wb-set', html);
    const q = s => m.querySelector(s);
    q('#wbs-accent').value = cfg.accent || '';
    q('#wbs-scale').value = cfg.scale || 'm';
    if (q('#wbs-tmode')) q('#wbs-tmode').value = cfg.mode === 'up' ? 'up' : 'down';
    if (q('#wbs-filter')) q('#wbs-filter').oninput = e => { const v = e.target.value.trim().toLowerCase(); m.querySelectorAll('.wb-pi').forEach(l => { l.style.display = !v || l.dataset.q.includes(v) ? '' : 'none'; }); };
    q('[data-a="cancel"]').onclick = () => closeModal('modal-wb-set');
    q('[data-a="rm"]').onclick = async () => {
      if (!await uiConfirm(tn('wb_remove_q', {name: nameOf(tile)}), {danger: true, okText: t('wb_t_remove')})) return;
      closeModal('modal-wb-set');
      removeTile(tile);
    };
    setTimeout(() => q('#wbs-title').focus(), 30);
    if (f.includes('app')) {
      const sel = q('#wbs-app');
      try {
        if (!appsCache) { const r = await fetch('/api/git'); const j = r.ok ? await r.json() : {}; appsCache = ((j.catalog && j.catalog.apps) || []).map(a => a.name).filter(Boolean).sort(); setTimeout(() => { appsCache = null; }, 60000); }
      } catch (e) { appsCache = []; }
      const apps = appsCache.slice();
      if (cfg.app && !apps.includes(cfg.app)) apps.unshift(cfg.app);
      sel.innerHTML = apps.length ? `<option value="">—</option>` + apps.map(a => `<option value="${esc(a)}">${esc(a)}</option>`).join('') : `<option value="">${esc(t('wb_commits_noapps'))}</option>`;
      sel.value = cfg.app || '';
    }
    q('[data-a="save"]').onclick = () => {
      const n = {};
      const title = q('#wbs-title').value.trim();
      if (title) n.title = title.slice(0, 60);
      if (q('#wbs-accent').value) n.accent = q('#wbs-accent').value;
      if (q('#wbs-scale').value !== 'm') n.scale = q('#wbs-scale').value;
      if (f.includes('conns')) {
        const ids = [...m.querySelectorAll('#wbs-conns input:checked')].map(i => Number(i.value));
        if (ids.length > def.connMax) { showToast(tn('wb_set_more', {n: def.connMax}), 'warning', 3500); return; }
        if (ids.length) n.ids = ids;
      }
      if (f.includes('problems') && q('#wbs-problems').checked) n.problems = true;
      if (f.includes('zones')) {
        const zs = q('#wbs-zones').value.split(/[,;\s]+/).map(z => z.trim()).filter(Boolean).slice(0, 8);
        for (const z of zs) { try { new Intl.DateTimeFormat('en', {timeZone: z}); } catch (e) { showToast(tn('wb_clock_zones_bad', {z}), 'error', 4000); return; } }
        n.zones = zs;
      }
      if (f.includes('seconds') && !q('#wbs-seconds').checked) n.seconds = false;
      if (f.includes('app')) { if (q('#wbs-app').value) n.app = q('#wbs-app').value; const br = q('#wbs-branch').value.trim(); if (br) n.branch = br; n.count = Number(q('#wbs-count').value) || 10; }
      if (f.includes('links')) {
        const links = [];
        const lines = q('#wbs-links').value.split('\n');
        for (let i = 0; i < lines.length; i++) {
          const ln = lines[i].trim();
          if (!ln) continue;
          const k = ln.lastIndexOf('|'), name = k >= 0 ? ln.slice(0, k).trim() : '', url = (k >= 0 ? ln.slice(k + 1) : ln).trim();
          if (!httpURL(url)) { showToast(tn('wb_links_bad', {n: i + 1}), 'error', 4000); return; }
          links.push({name: name.slice(0, 80), url: url.slice(0, 1000)});
        }
        if (links.length) n.links = links.slice(0, LINKS_MAX);
      }
      if (f.includes('timer')) {
        if (q('#wbs-tmode').value === 'up') n.mode = 'up';
        n.min = Math.max(1, Math.min(1440, Math.round(Number(q('#wbs-tmin').value) || 25)));
        // a running or paused timer keeps going unless its mode or length changed
        if ((n.mode || '') === (cfg.mode || '') && n.min === (Number(cfg.min) || 25)) for (const k of ['end', 'start', 'acc', 'left']) if (cfg[k]) n[k] = cfg[k];
      }
      if (cfg.text) n.text = cfg.text;
      const changedSrc = JSON.stringify([cfg.ids, cfg.app, cfg.branch]) !== JSON.stringify([n.ids, n.app, n.branch]);
      tile.cfg = n;
      if (changedSrc) delete ctxs[tile.id];
      const b = bodies[tile.id]; if (b) { b.innerHTML = ''; b._last = ''; }
      saveSoon();
      closeModal('modal-wb-set');
      const wo = wins[tile.id];
      if (wo) decorate(tile, wo);
      paint(tile, true);
      renderTabs();
      if (registered.has(tile.id)) wrmLayout.refresh();
      showToast(t('wb_saved'), 'success', 1600);
    };
  }

  // ── styles ──
  const css = document.createElement('style');
  css.textContent = `
.wb-upd { flex: 0 1000 auto; color: var(--text3); font-size: 10px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.wb-ib { background: transparent; border: 1px solid transparent; color: var(--text3); width: 24px; height: 24px; padding: 0; border-radius: 6px; font-size: 12px; line-height: 1; flex: none; cursor: pointer; }
.wb-ib:hover:not(:disabled) { background: var(--bg3); color: var(--text); border-color: var(--border2); }
.wb-ib:disabled { opacity: .35; cursor: default; }
.wb-body { flex: 1; min-height: 0; overflow: auto; padding: 10px 12px; font-size: 12px; }
.wb-pnl { display: flex; flex-direction: column; min-height: 0; }
.wb-pnl .wb-body { padding: 8px 10px; }
.wb-pbar { display: flex; align-items: center; gap: 4px; justify-content: flex-end; padding: 3px 6px 0; flex: none; }
.wb-pbar .wb-upd { flex: 1; text-align: left; padding-left: 4px; }
.wb-hint { color: var(--text3); line-height: 1.5; }
.wb-hint.sm { font-size: 11px; margin-top: 4px; }
.wb-err { color: var(--red); line-height: 1.45; word-break: break-word; }
.wb-ok { color: var(--green); font-weight: 600; }
.wb-sum { display: flex; gap: 6px; margin-bottom: 8px; flex-wrap: wrap; }
.wb-sum .b { font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 10px; background: var(--bg3); }
.wb-sum .b.down { color: var(--red); }
.wb-sum .b.up { color: var(--green); }
.wb-list { display: flex; flex-direction: column; gap: 2px; }
.wb-row { display: flex; align-items: center; gap: 8px; width: 100%; background: transparent; border: none; color: var(--text); padding: 5px 6px; border-radius: 6px; text-align: left; font-size: 12px; cursor: pointer; }
.wb-row:hover { background: var(--bg3); }
.wb-row.ro { cursor: default; }
.wb-row.ro:hover { background: transparent; }
.wb-row .nm, .wb-srv-h .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wb-row .sd, .wb-ev .sd { color: var(--text3); font-size: 11px; white-space: nowrap; }
.wb-row .conn-st, .wb-srv-h .conn-st, .wb-quick .conn-st { flex: none; position: static; }
.wb-srv-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 10px; }
.wb-srv { background: var(--bg3); border: 1px solid var(--border); border-radius: 9px; padding: 8px 10px; display: flex; flex-direction: column; gap: 5px; min-width: 0; }
.wb-srv-h { display: flex; align-items: center; gap: 7px; background: transparent; border: none; color: var(--text); padding: 0; font-weight: 700; font-size: 12px; cursor: pointer; text-align: left; }
.wb-srv-h:hover .nm { text-decoration: underline; }
.wb-bar { display: grid; grid-template-columns: minmax(42px, 30%) 1fr auto; align-items: center; gap: 7px; font-size: 11px; }
.wb-bar .l { color: var(--text2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-mono); }
.wb-bar .tr { height: 7px; background: var(--bg); border-radius: 4px; overflow: hidden; }
.wb-bar .fi { display: block; height: 100%; background: var(--green); border-radius: 4px; transition: width .4s; }
.wb-bar.warn .fi { background: var(--yellow); }
.wb-bar.crit .fi { background: var(--red); }
.wb-bar .v { font-weight: 700; white-space: nowrap; text-align: right; }
.wb-bar .sd { color: var(--text3); font-weight: 500; }
.wb-src { color: var(--text3); font-size: 10px; margin-top: 2px; }
.wb-kv { display: flex; justify-content: space-between; gap: 8px; font-size: 12px; padding: 2px 0; }
.wb-kv span { color: var(--text2); }
.wb-clock { text-align: center; padding: 6px 0 8px; }
.wb-clock .big { font: 700 34px var(--font-mono); letter-spacing: .02em; }
.wb-clock .dt { color: var(--text2); font-size: 12px; margin-top: 2px; }
.wb-zones { border-top: 1px solid var(--border); padding-top: 6px; }
.wb-acts { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.wb-acts .wb-cnt { flex: 1; color: var(--text2); font-size: 11px; font-weight: 600; }
.wb-ev { display: block; padding: 6px 8px; border-radius: 7px; border-left: 3px solid transparent; color: var(--text); text-decoration: none; }
.wb-ev + .wb-ev { margin-top: 2px; }
.wb-ev.new { background: var(--bg3); border-left-color: var(--accent); cursor: pointer; }
.wb-ev.cm:hover { background: var(--bg3); }
.wb-ev .h { display: flex; gap: 8px; align-items: baseline; }
.wb-ev .tt { flex: 1; min-width: 0; font-weight: 600; overflow-wrap: anywhere; }
.wb-ev .bd { color: var(--text2); font-size: 11px; margin-top: 2px; white-space: pre-wrap; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
.wb-ev .mt { color: var(--text3); font-size: 11px; margin-top: 2px; }
.wb-sub { color: var(--text2); font-size: 11px; font-weight: 600; margin-bottom: 6px; }
.wb-quick { display: flex; flex-wrap: wrap; gap: 6px; }
.wb-quick button { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; padding: 6px 10px; }
.wb-note { width: 100%; height: 100%; min-height: 110px; resize: none; background: var(--bg); color: var(--text); border: 1px solid var(--border); border-radius: 7px; padding: 8px; font-family: inherit; font-size: 12px; line-height: 1.5; box-sizing: border-box; }
.wb-modal { width: min(560px, calc(100vw - 24px)); max-height: calc(100vh - 32px); overflow: auto; }
.wb-cat { display: flex; flex-direction: column; gap: 6px; }
.wb-ci { display: flex; align-items: center; gap: 12px; padding: 9px 10px; background: var(--bg3); border: 1px solid var(--border); border-radius: 9px; }
.wb-ci .ico { font-size: 20px; width: 28px; text-align: center; flex: none; }
.wb-ci .tx { flex: 1; min-width: 0; }
.wb-ci .nm { font-weight: 700; font-size: 13px; }
.wb-ci .ds { color: var(--text2); font-size: 11.5px; line-height: 1.45; margin-top: 2px; }
.wb-n { color: var(--text3); font-weight: 600; font-size: 11px; }
.wb-pick { max-height: 220px; overflow: auto; border: 1px solid var(--border); border-radius: 7px; margin-top: 6px; padding: 4px; }
.field .wb-pi, .wb-pi { display: flex; align-items: center; gap: 8px; padding: 4px 6px; border-radius: 5px; cursor: pointer; font-size: 12px; text-transform: none; letter-spacing: 0; font-weight: 500; margin: 0; }
.wb-pi:hover { background: var(--bg3); }
.wb-pi input { width: auto; margin: 0; }
.wb-pi .nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); }
.wb-pi .sd { color: var(--text3); font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 45%; }
.wb-chk { display: flex; align-items: center; gap: 8px; font-size: 12px; margin: 6px 0 10px; cursor: pointer; }
.wb-chk input { width: auto; margin: 0; }
.wb-wc { overflow: hidden; }
.wb-wc > .wb-body { flex: 1; min-height: 0; overflow: auto; }
.win.addon .win-title { font-weight: 700; }
.win.addon.wb-acc .win-header { box-shadow: inset 0 3px 0 var(--wb-accent); }
.win.addon.wb-acc .win-dot { background: var(--wb-accent); }
.win-btn.wb-a { font-size: 12px; }
.wb-row .ico { flex: none; width: 16px; text-align: center; color: var(--text3); }
a.wb-row { text-decoration: none; }
.wb-2col { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.wb-timer { text-align: center; padding: 4px 0; }
.wb-timer .big { font: 700 36px var(--font-mono); letter-spacing: .02em; }
.wb-timer .big.done { color: var(--red); animation: wb-blink 1s steps(2, start) infinite; }
.wb-timer .tr { height: 6px; background: var(--bg3); border-radius: 3px; overflow: hidden; margin: 8px 6px 10px; }
.wb-timer .fi { display: block; height: 100%; background: var(--accent); transition: width .9s linear; }
.wb-acts.c { justify-content: center; }
@keyframes wb-blink { to { visibility: hidden; } }
#win-container.cv-dots { background-image: radial-gradient(circle, var(--border2) 1px, transparent 1.4px); background-size: var(--cv-gs, 20px) var(--cv-gs, 20px); }
#win-container.cv-lines { background-image: linear-gradient(to right, rgba(var(--muted-rgb),.08) 1px, transparent 1px), linear-gradient(to bottom, rgba(var(--muted-rgb),.08) 1px, transparent 1px); background-size: var(--cv-gs, 20px) var(--cv-gs, 20px); }
@media (max-width: 759px) { .wb-2col { grid-template-columns: 1fr; } }
`;
  document.head.appendChild(css);

  window.addEventListener('resize', (() => {
    let rt = 0, was = compact();
    return () => {
      clearTimeout(rt);
      rt = setTimeout(() => {
        if (!S || compact() === was) return;
        was = compact();
        reconcile();
        applying++;
        try { S.tiles.forEach(tile => { const wo = wins[tile.id]; if (wo) { applyGeo(tile, wo); decorate(tile, wo); } }); } finally { applying--; }
      }, 120);
    };
  })());

  window.wrmWorkboard = {
    init, menu, catalog, closeAddon, windowGone, windowMoved, grid,
    tabInfo(wo) { const tile = tileOfWin(wo); return tile ? {ico: A[tile.type].ico, title: nameOf(tile)} : {ico: '▤', title: t('wb_title')}; },
    // ctxItems adds the addon's own entries to the title bar / tab context menu.
    ctxItems(wo) {
      const tile = tileOfWin(wo);
      if (!tile) return '';
      const it = (a, ico, label) => `<div class="ctx-item" onclick="closeAllCtx(); wrmWorkboard.act(${wo.id}, '${a}')">${ico} ${esc(label)}</div>`;
      let h = it('set', '⚙', t('wb_t_settings'));
      if (A[tile.type].refresh) h += it('refresh', '↻', t('wb_t_refresh'));
      if (!compact()) h += it('dock-right', '⇥', t('dk_move_right')) + it('dock-left', '⇤', t('dk_move_left')) + it('dock-bottom', '⤓', t('dk_move_bottom'));
      return h + '<div class="ctx-hr"></div>';
    },
    act(winId, a) {
      if (a.startsWith('dock-')) { const tile = tileOfWin(winObjects.find(w => w.id === winId)); if (tile) dock(tile, a.slice(5)); return; }
      act(winId, a);
    },
    // bus is called by the page's event bus for every pushed message type.
    bus(type) {
      if (!S) return;
      if (type === 'events_changed') reloadType('events');
      if (type === 'sessions_changed' || type === 'status_changed' || type === 'tunnels_changed') setTimeout(() => S.tiles.forEach(x => { if (A[x.type].sig && bodies[x.id] && visible(bodies[x.id])) paint(x); }), 600);
    },
    langChanged() {
      if (!S) return;
      Object.values(bodies).forEach(b => { b._last = ''; if (b.querySelector('.wb-timer')) b.innerHTML = ''; });
      Object.values(ctxs).forEach(c => { c.sig = ''; });
      S.tiles.forEach(tile => { const wo = wins[tile.id]; if (wo) decorate(tile, wo); if (panels[tile.id]) panelOf(tile); paint(tile, true); });
      if (typeof renderTabs === 'function') renderTabs();
      if (window.wrmLayout) wrmLayout.refresh();
    },
    _state: () => S, _sanitize: sanitize,
  };
})();
