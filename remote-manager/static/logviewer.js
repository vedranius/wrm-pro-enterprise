/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

/* Log viewer (v13.5.0): the Logs tab of Monitoring.
 * File list with sorting and filters, search across many files on the server
 * (grep over SSH, results sorted by time with a timeline), and viewer tabs with
 * line numbers, level colours, smart highlighting, in-log find, level filters
 * and follow. Log discovery settings live here, apart from the Zabbix settings.
 * It uses the same connection permissions as the rest of Monitoring. */
Object.assign(LANGS.en, {
  lv_files: 'Log files',
  lv_filter: 'Filter files…',
  lv_sort: 'Sort',
  lv_sort_modified_desc: 'Newest first',
  lv_sort_modified_asc: 'Oldest first',
  lv_sort_name: 'Name',
  lv_sort_size: 'Largest first',
  lv_sort_service: 'Service',
  lv_sort_operator: 'Operator',
  lv_sort_path: 'Path',
  lv_all_operators: 'All operators',
  lv_all_services: 'All services',
  lv_select_all: 'Select all shown',
  lv_select_none: 'Clear selection',
  lv_selected: 'selected',
  lv_settings: 'Log settings',
  lv_settings_hint: 'Where WRM looks for logs on this server and how it labels them. These settings are separate from the Zabbix monitoring settings.',
  lv_scan_paths: 'Folders to scan (one per line)',
  lv_scan_paths_d: 'Discovery searches these folders for log files. Default: /var/log, /home, /srv, /opt.',
  lv_exclude: 'Folder names to skip (one per line)',
  lv_exclude_d: 'Folders with these names are never entered (source trees, virtual environments, caches).',
  lv_patterns: 'Label patterns (one per line)',
  lv_patterns_d: 'Placeholders {operator}, {service}, {logdir} (a folder named log, logs, …), * (one folder) and ** (any depth). The first pattern that matches a log folder gives its operator and service; logs that match none are still listed and named after their folder.',
  lv_extra: 'Extra file name patterns (one per line, e.g. *.trc)',
  lv_extra_d: 'Files are logs when they look like logs (*.log, *_log, syslog, rotated copies, files in log folders). Scripts, code, configuration and binary files are never listed.',
  lv_roots: 'Fixed log folders: path | service | operator (one per line, journal:unit for a systemd unit)',
  lv_max_depth: 'Maximum folder depth',
  lv_max_files: 'Maximum files',
  lv_max_age: 'Only files changed in the last N days (0 = all)',
  lv_time_budget: 'Time limit for discovery (seconds)',
  lv_rotated: 'Include rotated copies (app.log.1, app.log.2026-10-22)',
  lv_compressed: 'Include compressed copies (.gz, .xz, .zst) in the list',
  lv_skip_empty: 'Skip empty files',
  lv_journal: 'Add systemd journal units',
  lv_systemd: 'Read log paths from systemd units',
  lv_docker: 'Read log paths from Docker containers',
  lv_supervisor: 'Read log paths from Supervisor',
  lv_method: 'Discovery method',
  lv_method_auto: 'Automatic (fast SSH search, SFTP when the shell is limited)',
  lv_method_ssh: 'SSH commands only (find, stat)',
  lv_method_sftp: 'SFTP only (slower, no shell commands)',
  lv_timezone: 'Timezone of the log timestamps',
  lv_defaults: 'Restore defaults',
  lv_save_find: 'Save and find logs',
  lv_saved: 'Log settings saved',
  lv_readonly: 'Only the owner of this connection can change its log settings.',
  lv_method_used: 'Method',
  lv_took: 'took',
  lv_search_tab: 'Search',
  lv_search_q: 'Search in logs…',
  lv_regex: 'Regular expression',
  lv_case: 'Match case',
  lv_word: 'Whole words',
  lv_context: 'Context lines',
  lv_scope_selected: 'Selected files',
  lv_scope_shown: 'All shown files',
  lv_search: 'Search',
  lv_stop: 'Stop',
  lv_last: 'Last',
  lv_range_clear: 'Any time',
  lv_hits: 'hits',
  lv_hits_truncated: 'Only the first matches per file are shown. Narrow the search or the time range to see more.',
  lv_no_hits: 'No matches.',
  lv_search_intro: 'Type what to look for and press Enter. The search runs on the server across the chosen log files, so large and old logs are searched quickly. Results are sorted by their time.',
  lv_newest: 'Newest first',
  lv_oldest: 'Oldest first',
  lv_timeline: 'Matches over time (click a bar to zoom in)',
  lv_all_files: 'All files',
  lv_no_time: 'no time',
  lv_find: 'Find in this log…',
  lv_only_matching: 'Only matching lines',
  lv_wrap: 'Wrap lines',
  lv_numbers: 'Line numbers',
  lv_follow: 'Follow',
  lv_following: 'Following new lines',
  lv_older: 'Load older lines',
  lv_goto: 'Go to line',
  lv_copy: 'Copy shown lines',
  lv_download: 'Download shown lines',
  lv_terminal: 'Open in terminal',
  lv_ask_ai: 'Ask AI about this log',
  lv_ask_ai_logs: 'AI log assistant',
  lv_lines: 'lines',
  lv_of: 'of',
  lv_rotated_reset: 'The file was rotated or truncated; showing it again from the end.',
  lv_jump_time: 'Jump to time',
  lv_close_tab: 'Close',
  lv_levels: 'Levels',
  lv_lvl_error: 'Errors',
  lv_lvl_warn: 'Warnings',
  lv_lvl_info: 'Info',
  lv_lvl_debug: 'Debug',
  lv_lvl_other: 'Other',
  lv_pick_file: 'Choose a log on the left to open it, or search across many logs above.',
  lv_date_hint: 'Date and time as',
  lv_bad_date: 'Use the date format',
  lv_no_discovery: 'No logs found yet for this server. Click Find logs.',
  lv_keys: 'Keys: / find · n / N next or previous match · g go to line · f follow · w wrap',
  lv_folder: 'Folder',
  lv_all_folders: 'All folders',
  lv_view: 'View',
  lv_view_list: 'List',
  lv_view_tree: 'Folder tree',
  lv_place_logs: 'log folder',
  lv_place_app: 'app folder',
  lv_place_logs_d: 'Log folder: /var/log or a folder named log, logs, …',
  lv_place_app_d: 'Application or script folder (not a log folder)',
  lv_op_guess: 'Operator guessed from words that repeat in folder and service names',
  lv_main_folders: 'Main folders for the folder tree (one per line, e.g. /home/scripts)',
  lv_main_folders_d: 'Everything under a main folder is grouped under it in the folder tree and the Folder filter. The scan paths and the Git workspace server roots are main folders too.',
  lv_guess_operators: 'Guess operators from words that repeat in folder and service names (nova, novatv → one operator group)',
  lv_tree_journal: 'systemd journal',
  lv_select_folder: 'Select all shown logs in this folder',
});
Object.assign(LANGS.hr, {
  lv_files: 'Log fileovi',
  lv_filter: 'Filtriraj fileove…',
  lv_sort: 'Poredaj',
  lv_sort_modified_desc: 'Najnoviji prvi',
  lv_sort_modified_asc: 'Najstariji prvi',
  lv_sort_name: 'Naziv',
  lv_sort_size: 'Najveći prvi',
  lv_sort_service: 'Servis',
  lv_sort_operator: 'Operater',
  lv_sort_path: 'Putanja',
  lv_all_operators: 'Svi operateri',
  lv_all_services: 'Svi servisi',
  lv_select_all: 'Označi sve prikazane',
  lv_select_none: 'Poništi odabir',
  lv_selected: 'označeno',
  lv_settings: 'Postavke logova',
  lv_settings_hint: 'Gdje WRM traži logove na ovom serveru i kako ih označava. Ove postavke su odvojene od postavki Zabbix nadzora.',
  lv_scan_paths: 'Folderi za pretragu (jedan po liniji)',
  lv_scan_paths_d: 'Pretraga traži log fileove u ovim folderima. Zadano: /var/log, /home, /srv, /opt.',
  lv_exclude: 'Nazivi foldera koji se preskaču (jedan po liniji)',
  lv_exclude_d: 'U foldere s ovim nazivima se nikad ne ulazi (izvorni kod, virtualna okruženja, cache).',
  lv_patterns: 'Uzorci oznaka (jedan po liniji)',
  lv_patterns_d: 'Oznake {operator}, {service}, {logdir} (folder imena log, logs, …), * (jedan folder) i ** (bilo koja dubina). Prvi uzorak koji odgovara log folderu daje operatera i servis; logovi koji ne odgovaraju nijednom ipak se prikazuju i nazivaju po svom folderu.',
  lv_extra: 'Dodatni uzorci imena fileova (jedan po liniji, npr. *.trc)',
  lv_extra_d: 'File je log kad izgleda kao log (*.log, *_log, syslog, rotirane kopije, fileovi u log folderima). Skripte, kod, konfiguracija i binarni fileovi se nikad ne prikazuju.',
  lv_roots: 'Fiksni log folderi: putanja | servis | operater (jedan po liniji, journal:unit za systemd unit)',
  lv_max_depth: 'Najveća dubina foldera',
  lv_max_files: 'Najviše fileova',
  lv_max_age: 'Samo fileovi mijenjani u zadnjih N dana (0 = svi)',
  lv_time_budget: 'Vremensko ograničenje pretrage (sekunde)',
  lv_rotated: 'Uključi rotirane kopije (app.log.1, app.log.2026-10-22)',
  lv_compressed: 'Uključi komprimirane kopije (.gz, .xz, .zst) u popis',
  lv_skip_empty: 'Preskoči prazne fileove',
  lv_journal: 'Dodaj systemd journal unitove',
  lv_systemd: 'Čitaj putanje logova iz systemd unitova',
  lv_docker: 'Čitaj putanje logova iz Docker kontejnera',
  lv_supervisor: 'Čitaj putanje logova iz Supervisora',
  lv_method: 'Način pretrage',
  lv_method_auto: 'Automatski (brza SSH pretraga, SFTP kad je shell ograničen)',
  lv_method_ssh: 'Samo SSH naredbe (find, stat)',
  lv_method_sftp: 'Samo SFTP (sporije, bez shell naredbi)',
  lv_timezone: 'Vremenska zona vremena u logovima',
  lv_defaults: 'Vrati zadano',
  lv_save_find: 'Spremi i pronađi logove',
  lv_saved: 'Postavke logova spremljene',
  lv_readonly: 'Samo vlasnik ove konekcije može mijenjati njene postavke logova.',
  lv_method_used: 'Način',
  lv_took: 'trajalo',
  lv_search_tab: 'Pretraga',
  lv_search_q: 'Pretraži logove…',
  lv_regex: 'Regularni izraz',
  lv_case: 'Razlikuj velika i mala slova',
  lv_word: 'Cijele riječi',
  lv_context: 'Linije konteksta',
  lv_scope_selected: 'Označeni fileovi',
  lv_scope_shown: 'Svi prikazani fileovi',
  lv_search: 'Traži',
  lv_stop: 'Zaustavi',
  lv_last: 'Zadnjih',
  lv_range_clear: 'Bilo kada',
  lv_hits: 'pogodaka',
  lv_hits_truncated: 'Prikazani su samo prvi pogoci po fileu. Suzite pretragu ili vremenski raspon da vidite više.',
  lv_no_hits: 'Nema pogodaka.',
  lv_search_intro: 'Upišite što tražite i pritisnite Enter. Pretraga se izvodi na serveru kroz odabrane log fileove, pa se i veliki i stari logovi pretražuju brzo. Rezultati su poredani po vremenu.',
  lv_newest: 'Najnoviji prvi',
  lv_oldest: 'Najstariji prvi',
  lv_timeline: 'Pogoci kroz vrijeme (klik na stupac suzi raspon)',
  lv_all_files: 'Svi fileovi',
  lv_no_time: 'bez vremena',
  lv_find: 'Traži u ovom logu…',
  lv_only_matching: 'Samo linije s pogotkom',
  lv_wrap: 'Prelamaj linije',
  lv_numbers: 'Brojevi linija',
  lv_follow: 'Prati',
  lv_following: 'Pratim nove linije',
  lv_older: 'Učitaj starije linije',
  lv_goto: 'Idi na liniju',
  lv_copy: 'Kopiraj prikazane linije',
  lv_download: 'Preuzmi prikazane linije',
  lv_terminal: 'Otvori u terminalu',
  lv_ask_ai: 'Pitaj AI o ovom logu',
  lv_ask_ai_logs: 'AI asistent za logove',
  lv_lines: 'linija',
  lv_of: 'od',
  lv_rotated_reset: 'File je rotiran ili skraćen; ponovno ga prikazujem od kraja.',
  lv_jump_time: 'Skoči na vrijeme',
  lv_close_tab: 'Zatvori',
  lv_levels: 'Razine',
  lv_lvl_error: 'Greške',
  lv_lvl_warn: 'Upozorenja',
  lv_lvl_info: 'Info',
  lv_lvl_debug: 'Debug',
  lv_lvl_other: 'Ostalo',
  lv_pick_file: 'Odaberite log lijevo da ga otvorite ili pretražite više logova odjednom gore.',
  lv_date_hint: 'Datum i vrijeme kao',
  lv_bad_date: 'Koristite format datuma',
  lv_no_discovery: 'Za ovaj server još nema pronađenih logova. Kliknite Pronađi logove.',
  lv_keys: 'Tipke: / traži · n / N sljedeći ili prethodni pogodak · g idi na liniju · f prati · w prelamanje',
  lv_folder: 'Folder',
  lv_all_folders: 'Svi folderi',
  lv_view: 'Prikaz',
  lv_view_list: 'Popis',
  lv_view_tree: 'Stablo foldera',
  lv_place_logs: 'log folder',
  lv_place_app: 'folder aplikacije',
  lv_place_logs_d: 'Log folder: /var/log ili folder imena log, logs, …',
  lv_place_app_d: 'Folder aplikacije ili skripti (nije log folder)',
  lv_op_guess: 'Operater prepoznat iz riječi koje se ponavljaju u imenima foldera i servisa',
  lv_main_folders: 'Glavni folderi za stablo (jedan po liniji, npr. /home/skripte)',
  lv_main_folders_d: 'Sve ispod glavnog foldera grupira se pod njim u stablu foldera i u filtru Folder. Folderi za pretragu i serverski rootovi Git workspacea su također glavni folderi.',
  lv_guess_operators: 'Prepoznaj operatere iz riječi koje se ponavljaju u imenima foldera i servisa (nova, novatv → jedna grupa operatera)',
  lv_tree_journal: 'systemd journal',
  lv_select_folder: 'Označi sve prikazane logove u ovom folderu',
});

(function () {
  const $ = id => document.getElementById(id);
  const LEVELS = ['error', 'warn', 'info', 'debug', 'other'];
  const S = {
    host: null, ctx: null, conn: 0, files: [], meta: {}, filter: '', service: '', operator: '', folder: '', mains: [],
    view: lsGet('wrm_lv_view', 'list'), tree: {},
    sort: lsGet('wrm_lv_sort', 'modified_desc'), selected: new Set(), tabs: [], active: 'search', seq: 0,
    listW: +lsGet('wrm_lv_listw', '340') || 340,
    search: {q: '', regex: false, case: false, word: false, context: 0, from: '', to: '', scope: 'shown', hits: null, running: false, desc: true, file: '', levels: new Set(LEVELS), ms: 0, truncated: false, ctrl: null, error: ''},
    tz: '', settings: null, canEdit: true,
  };
  function lsGet(k, d) { try { return localStorage.getItem(k) || d; } catch (_) { return d; } }
  function lsSet(k, v) { try { localStorage.setItem(k, v); } catch (_) {} }

  const style = document.createElement('style');
  style.textContent = `
#lv{display:flex;flex-direction:column;height:100%;min-height:0;gap:8px}
#lv .lv-bar{display:flex;flex-wrap:wrap;gap:6px;align-items:center}
#lv .lv-bar input,#lv .lv-bar select{flex:0 1 auto;min-width:0}
#lv .lv-main{flex:1;min-height:0;display:flex;gap:0;border:1px solid var(--border);border-radius:8px;overflow:hidden;background:var(--bg)}
#lv .lv-list{flex:none;display:flex;flex-direction:column;min-width:200px;max-width:70%;border-right:1px solid var(--border)}
#lv .lv-split{flex:none;width:5px;cursor:col-resize;background:var(--bg3)}
#lv .lv-split:hover{background:var(--accent)}
#lv .lv-list-head{padding:6px;display:flex;flex-direction:column;gap:5px;border-bottom:1px solid var(--border)}
#lv .lv-list-head .lv-row{display:flex;gap:5px}
#lv .lv-list-head .lv-row>*{flex:1 1 0;min-width:0}
#lv .lv-files{flex:1;overflow:auto;min-height:0}
#lv .lv-file{display:flex;gap:6px;padding:6px 8px;border-bottom:1px solid var(--border);cursor:pointer;align-items:flex-start}
#lv .lv-file:hover{background:var(--bg3)}
#lv .lv-file.on{background:var(--bg4)}
#lv .lv-file input{width:auto;flex:none;margin-top:2px}
#lv .lv-file .lv-fn{font-weight:600;overflow-wrap:anywhere}
#lv .lv-file .lv-fp{font-size:11px;color:var(--text3);overflow-wrap:anywhere}
#lv .lv-pill{display:inline-block;font-size:10px;padding:0 5px;border-radius:8px;background:var(--bg3);border:1px solid var(--border2);margin-right:3px}
#lv .lv-pill.op{border-color:var(--purple)}
#lv .lv-pill.op.guess{border-style:dashed}
#lv .lv-pill.pl-logs{border-color:var(--green)}
#lv .lv-pill.pl-app{border-color:var(--yellow)}
#lv .lv-tdir{display:flex;gap:6px;align-items:center;padding:5px 8px;border-bottom:1px solid var(--border);cursor:pointer;background:var(--bg2);font-size:12px}
#lv .lv-tdir:hover{background:var(--bg3)}
#lv .lv-tdir.main{font-weight:700;background:var(--bg3)}
#lv .lv-tdir input{width:auto;flex:none;margin:0}
#lv .lv-tdir .tw{flex:none;width:1em;color:var(--text3)}
#lv .lv-tdir .tn{flex:1;min-width:0;overflow-wrap:anywhere}
#lv .lv-tdir .tc{flex:none;font-size:11px;color:var(--text3)}
#lv .lv-right{flex:1;min-width:0;display:flex;flex-direction:column}
#lv .lv-tabs{display:flex;gap:2px;overflow-x:auto;border-bottom:1px solid var(--border);background:var(--bg2);flex:none}
#lv .lv-tab{padding:6px 10px;cursor:pointer;white-space:nowrap;border:0;background:none;color:var(--text2);border-bottom:2px solid transparent;display:flex;gap:6px;align-items:center;border-radius:0}
#lv .lv-tab.on{color:var(--text);border-bottom-color:var(--accent);background:var(--bg)}
#lv .lv-tab .x{opacity:.6;padding:0 2px}
#lv .lv-tab .x:hover{opacity:1}
#lv .lv-pane{flex:1;min-height:0;display:flex;flex-direction:column}
#lv .lv-tools{display:flex;flex-wrap:wrap;gap:5px;padding:6px;border-bottom:1px solid var(--border);align-items:center}
#lv .lv-tools input[type=search]{flex:1 1 200px}
#lv .lv-tools label{font-size:12px;display:flex;gap:3px;align-items:center;white-space:nowrap}
#lv .lv-tools label input{width:auto}
#lv .lv-chip{font-size:11px;padding:2px 7px;border-radius:10px;border:1px solid var(--border2);background:var(--bg3);color:var(--text);cursor:pointer}
#lv .lv-chip.off{opacity:.4;text-decoration:line-through}
#lv .lv-chip.l-error{border-color:var(--red)}#lv .lv-chip.l-warn{border-color:var(--yellow)}#lv .lv-chip.l-info{border-color:var(--accent)}
#lv .lv-status{font-size:11px;color:var(--text3);padding:3px 8px;border-bottom:1px solid var(--border);overflow-wrap:anywhere}
#lv .lv-text{flex:1;min-height:0;overflow:auto;font:12px/1.45 var(--font-mono);background:var(--bg);user-select:text;outline:none}
#lv .lv-ln{display:flex;min-width:max-content;padding-right:8px}
#lv.wrap .lv-ln,#lv .wrap .lv-ln{min-width:0}
#lv .lv-ln .n{flex:none;width:7ch;text-align:right;padding-right:8px;color:var(--text3);user-select:none;border-right:1px solid var(--border);margin-right:8px;cursor:pointer}
#lv .lv-ln .c{white-space:pre}
#lv .wrap .lv-ln .c{white-space:pre-wrap;overflow-wrap:anywhere}
#lv .lv-ln.l-error{background:color-mix(in srgb,var(--red) 12%,transparent)}
#lv .lv-ln.l-warn{background:color-mix(in srgb,var(--yellow) 10%,transparent)}
#lv .lv-ln.l-debug{opacity:.72}
#lv .lv-ln.hit{box-shadow:inset 3px 0 var(--accent)}
#lv .lv-ln.ctx{opacity:.6}
#lv .lv-ln.cur{outline:1px solid var(--accent);outline-offset:-1px}
#lv .t-ts{color:#38bdf8}#lv .t-lvl-error{color:var(--red);font-weight:700}#lv .t-lvl-warn{color:var(--yellow);font-weight:700}#lv .t-lvl-info{color:var(--green)}#lv .t-lvl-debug{color:var(--text3)}
#lv .t-url{color:var(--purple);text-decoration:underline}#lv .t-ip{color:#f472b6}#lv .t-str{color:var(--yellow-t)}#lv .t-key{color:#22d3ee}#lv .t-exc{color:var(--red);text-decoration:underline dotted}#lv .t-num{color:#fb923c}#lv .t-uuid{color:var(--text2)}#lv .t-path{color:var(--green-t)}
#lv mark{background:#fde047;color:#111;border-radius:2px}
#lv mark.cur{background:#f97316;color:#fff}
#lv .lv-empty{padding:18px;color:var(--text3)}
#lv .lv-hit-file{font-size:11px;color:var(--text3);white-space:nowrap;cursor:pointer}
#lv .lv-hits .lv-ln{cursor:pointer}
#lv .lv-hits .lv-ln .n{width:20ch;text-align:left;white-space:nowrap}
#lv .lv-hits .lv-hit-file{flex:none;width:22ch;overflow:hidden;text-overflow:ellipsis;padding-right:8px;align-self:flex-start;line-height:1.75}
#lv .lv-tl{display:block;width:100%;height:46px;cursor:pointer}
#lv .lv-tl rect{fill:var(--accent)}#lv .lv-tl rect:hover{fill:var(--accent-h)}
#lv .lv-perfile{display:flex;gap:4px;flex-wrap:wrap;padding:4px 6px}
#lv-settings .lv-form{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,260px),1fr));gap:10px}
#lv-settings .lv-form label{display:block;font-size:12px;margin-bottom:3px}
#lv-settings .lv-form textarea{width:100%;box-sizing:border-box;min-height:84px;font:12px monospace}
#lv-settings .lv-form input:not([type=checkbox]),#lv-settings .lv-form select{width:100%;box-sizing:border-box}
#lv-settings .lv-form .wide{grid-column:1/-1}
#lv-settings .lv-checks label{display:flex;gap:6px;align-items:center;font-size:13px;margin:3px 0}
#lv-settings .lv-checks input{width:auto}
#lv-settings .d{font-size:11px;color:var(--text3);margin-top:2px}
@media (max-width:800px){#lv .lv-main{flex-direction:column}#lv .lv-list{max-width:none;width:auto!important;max-height:38vh;border-right:0;border-bottom:1px solid var(--border)}#lv .lv-split{display:none}}
`;
  document.head.appendChild(style);

  async function api(method, url, body, signal) {
    const r = await fetch(url, {method, signal, headers: body ? {'Content-Type': 'application/json'} : {}, body: body ? JSON.stringify(body) : undefined});
    const text = await r.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_) {}
    if (!r.ok) { const e = new Error((data && data.error) || text || r.statusText); e.status = r.status; throw e; }
    return data;
  }
  const base = p => String(p || '').split('/').pop();
  const dir = p => { const s = String(p || ''); const i = s.lastIndexOf('/'); return i > 0 ? s.slice(0, i) : '/'; };
  const isJournal = f => f && (f.kind === 'journal' || String(f.path).startsWith('journal:'));
  const fileName = f => isJournal(f) ? (f.unit || String(f.path).slice(8)) : base(f.path);
  const tz = () => S.tz || Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const fmt = (v, o) => { if (!v) return ''; const d = new Date(typeof v === 'number' ? v * 1000 : v); return isNaN(d) ? String(v) : wrmFmt(d, 'datetime', Object.assign({timeZone: tz()}, o)); };

  // ── Highlighting ────────────────────────────────────────────────
  const MON = 'Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec';
  const TOKENS = [
    ['ts', `\\b\\d{4}-\\d{2}-\\d{2}[T ]\\d{2}:\\d{2}:\\d{2}(?:[.,]\\d+)?(?:Z|[+-]\\d{2}:?\\d{2})?|\\b\\d{2}\\.\\d{2}\\.\\d{4}\\.? \\d{2}:\\d{2}:\\d{2}(?:[.,]\\d+)?|\\b\\d{4}/\\d{2}/\\d{2} \\d{2}:\\d{2}:\\d{2}(?:[.,]\\d+)?|\\[\\d{2}/(?:${MON})/\\d{4}:\\d{2}:\\d{2}:\\d{2} [+-]\\d{4}\\]|\\b(?:${MON}) [ \\d]\\d \\d{2}:\\d{2}:\\d{2}`],
    ['lvl-error', `\\b(?:FATAL|CRITICAL|CRIT|EMERG|ALERT|PANIC|SEVERE|ERROR|ERR|FAIL(?:ED|URE)?)\\b|\\[(?:error|crit|alert|emerg|fatal)\\]|\\blevel=(?:error|fatal|crit(?:ical)?|panic)\\b`],
    ['lvl-warn', `\\b(?:WARNING|WARN)\\b|\\[(?:warn|warning)\\]|\\blevel=warn(?:ing)?\\b`],
    ['lvl-info', `\\b(?:INFO|NOTICE)\\b|\\[(?:info|notice)\\]|\\blevel=(?:info|notice)\\b`],
    ['lvl-debug', `\\b(?:DEBUG|TRACE|VERBOSE)\\b|\\[(?:debug|trace)\\]|\\blevel=(?:debug|trace)\\b`],
    ['url', `\\b(?:https?|wss?|ftp)://[^\\s"'<>\\]\\)]+`],
    ['exc', `\\b(?:[A-Za-z_$][\\w$]*\\.)*[A-Z][\\w$]*(?:Exception|Error)\\b|\\bTraceback\\b|\\bpanic:`],
    ['uuid', `\\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\\b`],
    ['ip', `\\b(?:\\d{1,3}\\.){3}\\d{1,3}(?::\\d{1,5})?\\b`],
    ['str', `"(?:[^"\\\\]|\\\\.){0,400}"`],
    ['key', `\\b[A-Za-z_][\\w.-]{0,40}(?==[^=])`],
    ['path', `(?<![\\w/])/(?:[\\w.@-]+/)+[\\w.@-]+`],
    ['num', `(?<![\\w.])\\d+(?:\\.\\d+)?(?:ms|s|KB|MB|GB|%)?(?![\\w.])`],
  ];
  const TOKEN_RE = new RegExp(TOKENS.map(([k, r]) => `(?<${k.replace('-', '_')}>${r})`).join('|'), 'g');
  const LEVEL_RE = [
    ['error', /" 5\d\d \d|\b(?:FATAL|CRITICAL|CRIT|EMERG|ALERT|PANIC|SEVERE|ERROR|ERR)\b|\[(?:error|crit|alert|emerg|fatal)\]|\blevel=(?:error|fatal|crit|panic)|\b(?:Traceback|Exception)\b|"level":\s*"(?:error|fatal)"|<[0-3]>/],
    ['warn', /" 4\d\d \d|\b(?:WARNING|WARN)\b|\[(?:warn|warning)\]|\blevel=warn|"level":\s*"warn/],
    ['info', /\b(?:INFO|NOTICE)\b|\[(?:info|notice)\]|\blevel=info|"level":\s*"info/],
    ['debug', /\b(?:DEBUG|TRACE|VERBOSE)\b|\[(?:debug|trace)\]|\blevel=(?:debug|trace)|"level":\s*"(?:debug|trace)/],
  ];
  // A line's level; continuation lines (stack traces, wrapped messages that
  // start with whitespace or have no level) inherit the previous line's level.
  function lineLevel(text, prev) {
    const head = text.slice(0, 240);
    for (const [k, re] of LEVEL_RE) if (re.test(head)) return k;
    if (prev && prev !== 'other' && (/^\s/.test(text) || /^(?:at |Caused by|\.\.\.|File ")/.test(text.trim()))) return prev;
    return 'other';
  }
  function findRegex(q, regex, cs, word) {
    if (!q) return null;
    let src = regex ? q : q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    if (word) src = `\\b(?:${src})\\b`;
    try { return new RegExp(src, 'g' + (cs ? '' : 'i')); } catch (_) { return null; }
  }
  // highlight returns HTML for one line: smart tokens plus <mark> for matches.
  function highlight(text, re, curMatch) {
    if (text.length > 20000) text = text.slice(0, 20000) + ' …';
    const cls = new Array(text.length + 1).fill('');
    TOKEN_RE.lastIndex = 0;
    let m;
    while ((m = TOKEN_RE.exec(text))) {
      if (!m[0]) { TOKEN_RE.lastIndex++; continue; }
      const k = Object.keys(m.groups).find(g => m.groups[g] !== undefined);
      for (let i = m.index; i < m.index + m[0].length; i++) cls[i] = 't-' + k.replace('_', '-');
    }
    const mark = new Array(text.length).fill(0);
    if (re) {
      re.lastIndex = 0;
      let n = 0;
      while ((m = re.exec(text))) {
        if (!m[0]) { re.lastIndex++; continue; }
        n++;
        for (let i = m.index; i < m.index + m[0].length; i++) mark[i] = curMatch === n - 1 ? 2 : 1;
        if (n > 500) break;
      }
    }
    let out = '', i = 0;
    while (i < text.length) {
      let j = i + 1;
      while (j < text.length && cls[j] === cls[i] && mark[j] === mark[i]) j++;
      let seg = esc(text.slice(i, j));
      if (cls[i]) seg = `<span class="${cls[i]}">${seg}</span>`;
      if (mark[i]) seg = `<mark${mark[i] === 2 ? ' class="cur"' : ''}>${seg}</mark>`;
      out += seg;
      i = j;
    }
    return out || ' ';
  }

  // ── Mount and list ──────────────────────────────────────────────
  // mount(el, ctx): ctx = {servers: [{connection, sample}], server: id string, setServer(id), close()}
  async function mount(el, ctx) {
    S.host = el; S.ctx = ctx;
    const id = Number(ctx.server) || 0;
    if (id !== S.conn) {
      stopAll();
      Object.assign(S, {conn: id, files: [], meta: {}, selected: new Set(), tabs: [], active: 'search', settings: null, tz: '', folder: '', mains: [], tree: {}});
      S.search.hits = null; S.search.error = '';
      if (id) await loadFiles(false);
    }
    render();
  }
  function stopAll() {
    S.search.ctrl?.abort();
    for (const tab of S.tabs) stopFollow(tab);
    window.wrmChat?.closeLogAI();
  }
  async function loadSettings() {
    if (!S.conn) return null;
    try {
      const r = await api('GET', '/api/monitoring/logs/settings?id=' + S.conn);
      S.settings = r.settings; S.defaults = r.defaults; S.canEdit = r.can_edit !== false;
      S.tz = r.settings?.timezone || S.tz;
    } catch (_) {}
    return S.settings;
  }
  async function loadFiles(rerender = true) {
    const id = S.conn, seq = ++S.seq;
    if (!S.settings) await loadSettings();
    try {
      const r = await api('GET', '/api/monitoring/logs?id=' + id);
      if (seq !== S.seq) return;
      S.files = (r.files || []).map(f => ({...f, root_path: r.roots?.[f.root]?.path || ''}));
      S.mains = r.main_folders || [];
      S.meta = {method: r.method || '', at: r.discovered_at || '', discovered: !!r.discovered, warnings: r.warnings || [], incomplete: r.incomplete, services: r.services || []};
    } catch (e) {
      if (seq === S.seq) S.meta = {error: e.message};
    }
    if (rerender) render();
  }
  async function discover() {
    if (!S.conn || S.discovering) return;
    S.discovering = true; S.meta.error = ''; render();
    try {
      const d = await api('POST', '/api/monitoring/logs/discover?id=' + S.conn, {});
      await loadFiles(false);
      S.meta.ms = d.ms; S.meta.method = d.method || S.meta.method; S.meta.warnings = d.warnings || [];
      showToast(t('mon_discovery_done') + ': ' + (d.files ?? S.files.length), 'success');
    } catch (e) {
      S.meta.error = e.message;
    } finally {
      S.discovering = false; render();
    }
  }
  // Operators: "g:<group>" picks a group of similar operators (nova, novatv),
  // "o:<name>" one operator. Services are compared without case, so a script
  // folder and a log folder of the same service end up together.
  const opGroup = f => f.op_group || String(f.operator || '').toLowerCase();
  const opOk = f => !S.operator || (S.operator.startsWith('g:') ? opGroup(f) === S.operator.slice(2)
    : S.operator.startsWith('o:') ? String(f.operator || '').toLowerCase() === S.operator.slice(2) : f.operator === S.operator);
  const mainOf = f => isJournal(f) ? 'journal' : (f.main || dir(f.path));
  const inFolder = (f, p) => p === 'journal' ? isJournal(f) : !isJournal(f) && (dir(f.path) === p || f.path.startsWith(p.replace(/\/$/, '') + '/'));
  function shownFiles() {
    const q = S.filter.toLowerCase();
    const list = S.files.filter(f => (!S.service || String(f.service || '').toLowerCase() === S.service) && opOk(f) && (!S.folder || inFolder(f, S.folder)) &&
      (!q || [f.path, f.service, f.operator, f.unit].join(' ').toLowerCase().includes(q)));
    const by = S.sort, str = (a, b) => String(a || '￿').localeCompare(String(b || '￿'));
    list.sort((a, b) => by === 'modified_asc' ? (a.modified || 0) - (b.modified || 0)
      : by === 'name' ? str(fileName(a), fileName(b)) || str(a.path, b.path)
      : by === 'size' ? (b.size || 0) - (a.size || 0)
      : by === 'service' ? str(a.service, b.service) || (b.modified || 0) - (a.modified || 0)
      : by === 'operator' ? str(a.operator, b.operator) || str(a.service, b.service) || (b.modified || 0) - (a.modified || 0)
      : by === 'path' ? str(a.path, b.path)
      : (b.modified || 0) - (a.modified || 0));
    return list;
  }
  function opt(v, label, cur) { return `<option value="${esc(v)}"${String(v) === String(cur) ? ' selected' : ''}>${esc(label)}</option>`; }
  function render() {
    if (!S.host) return;
    const ctx = S.ctx, servers = ctx.servers || [];
    const focus = document.activeElement && S.host.contains(document.activeElement) ? {id: document.activeElement.id, pos: document.activeElement.selectionStart} : null;
    const listScroll = S.host.querySelector('.lv-files')?.scrollTop || 0;
    const shown = shownFiles();
    const opOpts = operatorOptions(), svcOpts = serviceOptions(), folderOpts = folderOptions();
    const meta = S.meta || {};
    const status = !S.conn ? t('mon_pick_server')
      : S.discovering ? '⏳ ' + t('mon_discovering')
      : [t('mon_log_count') + ': ' + S.files.length, meta.method ? t('lv_method_used') + ': ' + meta.method.toUpperCase() : '', meta.ms != null ? t('lv_took') + ' ' + (meta.ms / 1000).toFixed(1) + ' s' : '', meta.at && !String(meta.at).startsWith('0001') ? fmt(meta.at) : ''].filter(Boolean).join(' · ');
    S.host.innerHTML = `<div id="lv" class="${lsGet('wrm_lv_wrap', '') ? 'wrap' : ''}">
<div class="lv-bar">
 <select id="lv-server" aria-label="${esc(t('git_server'))}" onchange="wrmLogViewer.setServer(this.value)">${opt('', '— ' + t('git_server') + ' —', '')}${servers.map(x => opt(x.connection.id, (x.sample?.hostname || x.connection.host) + ' · ' + x.connection.name, ctx.server)).join('')}</select>
 <button onclick="wrmLogViewer.discover()" ${!S.conn || S.discovering ? 'disabled' : ''}>↻ ${esc(t('mon_discover'))}</button>
 <button class="btn-sec" onclick="wrmLogViewer.openSettings()" ${!S.conn ? 'disabled' : ''}>⚙ ${esc(t('lv_settings'))}</button>
 <button class="btn-sec" onclick="wrmLogViewer.askAI()" ${!S.conn ? 'disabled' : ''}>🤖 ${esc(t('lv_ask_ai_logs'))}</button>
 <span class="mon-sub" role="status" aria-busy="${!!S.discovering}">${esc(status)}</span>
</div>
${meta.error ? `<div class="hint-box warn">${esc(meta.error)}</div>` : ''}
${(meta.warnings || []).length ? `<details class="hint-box warn"><summary>${esc(t('mon_incomplete'))} (${meta.warnings.length})</summary>${meta.warnings.map(w => `<div>${esc(w)}</div>`).join('')}</details>` : ''}
<div class="lv-main">
 <div class="lv-list" style="width:${S.listW}px">
  <div class="lv-list-head">
   <input id="lv-filter" type="search" placeholder="${esc(t('lv_filter'))}" value="${esc(S.filter)}" oninput="wrmLogViewer.set('filter',this.value)">
   <div class="lv-row">
    <select id="lv-op" aria-label="${esc(t('mon_operator'))}" onchange="wrmLogViewer.set('operator',this.value)">${opt('', t('lv_all_operators'), S.operator)}${opOpts}</select>
    <select id="lv-svc" aria-label="${esc(t('mon_service'))}" onchange="wrmLogViewer.set('service',this.value)">${opt('', t('lv_all_services'), S.service)}${svcOpts}</select>
   </div>
   <div class="lv-row">
    <select id="lv-folder" aria-label="${esc(t('lv_folder'))}" onchange="wrmLogViewer.set('folder',this.value)">${opt('', t('lv_all_folders'), S.folder)}${folderOpts}</select>
   </div>
   <div class="lv-row">
    <select id="lv-sort" aria-label="${esc(t('lv_sort'))}" onchange="wrmLogViewer.set('sort',this.value)">${['modified_desc', 'modified_asc', 'name', 'size', 'service', 'operator', 'path'].map(k => opt(k, t('lv_sort') + ': ' + t('lv_sort_' + k), S.sort)).join('')}</select>
    <select id="lv-view" aria-label="${esc(t('lv_view'))}" onchange="wrmLogViewer.set('view',this.value)">${opt('list', t('lv_view') + ': ' + t('lv_view_list'), S.view)}${opt('tree', t('lv_view') + ': ' + t('lv_view_tree'), S.view)}</select>
   </div>
   <div class="mon-sub">${shown.length} / ${S.files.length} · ${S.selected.size} ${esc(t('lv_selected'))} · <a href="#" onclick="wrmLogViewer.selectShown(true);return false">${esc(t('lv_select_all'))}</a> · <a href="#" onclick="wrmLogViewer.selectShown(false);return false">${esc(t('lv_select_none'))}</a></div>
  </div>
  <div class="lv-files">${(S.view === 'tree' ? treeHtml(shown) : shown.slice(0, 3000).map(f => fileRow(f)).join('')) || `<div class="lv-empty">${esc(t(S.conn ? (S.discovering ? 'mon_discovering' : 'lv_no_discovery') : 'mon_pick_server'))}</div>`}</div>
 </div>
 <div class="lv-split" title="↔" onpointerdown="wrmLogViewer.resize(event)"></div>
 <div class="lv-right">
  <div class="lv-tabs" role="tablist">${tabButton('search', '🔎 ' + t('lv_search_tab') + (S.search.hits ? ` (${S.search.hits.filter(h => !h.context).length})` : ''))}${S.tabs.map(x => tabButton(x.id, (isJournal(x.file) ? '⚙ ' : '📄 ') + fileName(x.file), x.file.path)).join('')}</div>
  <div class="lv-pane" id="lv-pane"></div>
 </div>
</div></div>`;
    S.host.querySelector('.lv-files').scrollTop = listScroll;
    renderPane();
    window.wrmChat?.restoreLogAI();
    if (focus?.id && $(focus.id)) { $(focus.id).focus(); if (focus.pos != null) try { $(focus.id).setSelectionRange(focus.pos, focus.pos); } catch (_) {} }
  }
  function fileRow(f, depth) {
    const open = S.tabs.find(x => x.file.path === f.path);
    const sel = S.selected.has(f.path);
    return `<div class="lv-file${open && S.active === open.id ? ' on' : ''}"${depth ? ` style="padding-left:${8 + depth * 14}px"` : ''} onclick="if(event.target.tagName!=='INPUT')wrmLogViewer.openFile(${JSON.stringify(f.path).replace(/"/g, '&quot;')})">
<input type="checkbox" aria-label="${esc(f.path)}" ${sel ? 'checked' : ''} onchange="wrmLogViewer.select(${JSON.stringify(f.path).replace(/"/g, '&quot;')},this.checked)">
<div style="min-width:0;flex:1"><div class="lv-fn">${esc(fileName(f))}</div>
<div>${f.service ? `<span class="lv-pill">${esc(f.service)}</span>` : ''}${f.operator ? `<span class="lv-pill op${f.op_guess ? ' guess' : ''}"${f.op_guess ? ` title="${esc(t('lv_op_guess'))}"` : ''}>👤 ${f.op_guess ? '≈ ' : ''}${esc(f.operator)}</span>` : ''}${f.place === 'logs' || f.place === 'app' ? `<span class="lv-pill pl-${f.place}" title="${esc(t('lv_place_' + f.place + '_d'))}">${f.place === 'logs' ? '🗒' : '📜'} ${esc(t('lv_place_' + f.place))}</span>` : ''}</div>
<div class="lv-fp">${esc(isJournal(f) ? t('mon_journal') : dir(f.path))}</div>
${isJournal(f) ? '' : `<div class="lv-fp">${esc(fmtBytes(f.size || 0))} · ${esc(fmt(f.modified))}</div>`}</div></div>`;
  }
  function operatorOptions() {
    const groups = new Map();
    for (const f of S.files) {
      if (!f.operator) continue;
      const g = opGroup(f), m = String(f.operator).toLowerCase();
      let x = groups.get(g);
      if (!x) groups.set(g, x = {count: 0, members: new Map()});
      x.count++; x.members.set(m, (x.members.get(m) || 0) + 1);
    }
    return [...groups.keys()].sort().map(g => {
      const x = groups.get(g), members = [...x.members.keys()].sort();
      return opt('g:' + g, `${g} (${x.count})${members.length > 1 ? ' · ' + members.join(', ') : ''}`, S.operator) +
        (members.length > 1 ? members.map(m => opt('o:' + m, `\u00a0\u00a0↳ ${m} (${x.members.get(m)})`, S.operator)).join('') : '');
    }).join('');
  }
  function serviceOptions() {
    const by = new Map();
    for (const f of S.files) {
      if (!f.service || !opOk(f)) continue;
      const k = String(f.service).toLowerCase();
      let x = by.get(k);
      if (!x) by.set(k, x = {count: 0, names: new Map()});
      x.count++; x.names.set(f.service, (x.names.get(f.service) || 0) + 1);
    }
    return [...by.keys()].sort().map(k => {
      const x = by.get(k), name = [...x.names.entries()].sort((a, b) => b[1] - a[1])[0][0];
      return opt(k, `${name} (${x.count})`, S.service);
    }).join('');
  }
  // buildTree groups files under their main folder, then their subfolders.
  function buildTree(files) {
    const roots = new Map();
    const node = (p, name, main) => ({path: p, name, main, dirs: new Map(), files: [], count: 0});
    for (const f of files) {
      const m = mainOf(f);
      let n = roots.get(m);
      if (!n) roots.set(m, n = node(m, m === 'journal' ? t('lv_tree_journal') : m, m));
      n.count++;
      const d = isJournal(f) ? m : dir(f.path);
      if (d !== m && d.startsWith(m.replace(/\/$/, '') + '/')) {
        let p = m.replace(/\/$/, '');
        for (const seg of d.slice(p.length + 1).split('/')) {
          p += '/' + seg;
          let c = n.dirs.get(seg);
          if (!c) n.dirs.set(seg, c = node(p, seg, m));
          c.count++; n = c;
        }
      }
      n.files.push(f);
    }
    return [...roots.values()].sort((a, b) => a.main === 'journal' ? 1 : b.main === 'journal' ? -1 : a.path.localeCompare(b.path));
  }
  const byName = (a, b) => a.name.localeCompare(b.name, undefined, {sensitivity: 'base'});
  function folderOptions() {
    const out = [];
    const walk = (n, depth) => {
      if (out.length > 1500) return;
      out.push(opt(n.path, (depth ? '\u00a0\u00a0'.repeat(depth) + '└ ' : '') + n.name + ` (${n.count})`, S.folder));
      [...n.dirs.values()].sort(byName).forEach(c => walk(c, depth + 1));
    };
    buildTree(S.files).forEach(n => walk(n, 0));
    return out.join('');
  }
  const treeKey = n => n.main + '\u0000' + n.path;
  function treeOpen(n, depth) { const v = S.tree[treeKey(n)]; return v != null ? v : depth === 0 || !!S.filter; }
  function treeHtml(files) {
    let rows = 0;
    const q = v => JSON.stringify(v).replace(/"/g, '&quot;');
    const walk = (n, depth) => {
      if (rows > 3000) return '';
      rows++;
      const open = treeOpen(n, depth);
      const inNode = [], gather = x => { inNode.push(...x.files); x.dirs.forEach(gather); };
      gather(n);
      const all = inNode.length > 0 && inNode.every(f => S.selected.has(f.path));
      let h = `<div class="lv-tdir${depth ? '' : ' main'}" style="padding-left:${8 + depth * 14}px" role="button" aria-expanded="${open}" onclick="if(event.target.tagName!=='INPUT')wrmLogViewer.toggleDir(${q(n.main)},${q(n.path)},${depth})">
<span class="tw">${open ? '▾' : '▸'}</span><input type="checkbox" aria-label="${esc(t('lv_select_folder'))}" title="${esc(t('lv_select_folder'))}" ${all ? 'checked' : ''} onchange="wrmLogViewer.selectDir(${q(n.main)},${q(n.path)},this.checked)">
<span class="tn">${depth ? '📁' : '🗂'} ${esc(n.name)}</span><span class="tc">${n.count}</span></div>`;
      if (!open) return h;
      [...n.dirs.values()].sort(byName).forEach(c => { h += walk(c, depth + 1); });
      n.files.forEach(f => { if (rows++ <= 3000) h += fileRow(f, depth + 1); });
      return h;
    };
    return buildTree(files).map(n => walk(n, 0)).join('');
  }
  function toggleDir(main, p, depth) {
    const n = {main, path: p};
    S.tree[treeKey(n)] = !treeOpen(n, depth);
    render();
  }
  function selectDir(main, p, on) {
    shownFiles().filter(f => mainOf(f) === main && (p === main || inFolder(f, p))).forEach(f => on ? S.selected.add(f.path) : S.selected.delete(f.path));
    render();
  }
  function tabButton(id, label, title) {
    return `<button role="tab" class="lv-tab${S.active === id ? ' on' : ''}" aria-selected="${S.active === id}" title="${esc(title || label)}" onclick="wrmLogViewer.activate('${id}')">${esc(label)}${id !== 'search' ? `<span class="x" role="button" aria-label="${esc(t('lv_close_tab'))}" onclick="event.stopPropagation();wrmLogViewer.closeTab('${id}')">✕</span>` : ''}</button>`;
  }
  function set(k, v) {
    S[k] = v;
    if (k === 'operator') S.service = '';
    if (k === 'sort') lsSet('wrm_lv_sort', v);
    if (k === 'view') lsSet('wrm_lv_view', v);
    render();
  }
  function select(p, on) { if (on) S.selected.add(p); else S.selected.delete(p); render(); }
  function selectShown(on) { if (on) shownFiles().forEach(f => S.selected.add(f.path)); else S.selected.clear(); render(); }
  function setServer(v) { S.ctx.setServer?.(v); }
  function resize(e) {
    const list = S.host.querySelector('.lv-list'), x0 = e.clientX, w0 = list.offsetWidth;
    e.target.setPointerCapture(e.pointerId);
    e.target.onpointermove = ev => { S.listW = Math.max(200, Math.min(innerWidth * 0.7, w0 + ev.clientX - x0)); list.style.width = S.listW + 'px'; };
    e.target.onpointerup = () => { e.target.onpointermove = null; lsSet('wrm_lv_listw', String(Math.round(S.listW))); };
  }
  function activate(id) { S.active = id; render(); }

  // ── Search across files ─────────────────────────────────────────
  function renderPane() {
    const pane = $('lv-pane');
    if (!pane) return;
    if (S.active === 'search') return renderSearch(pane);
    const tab = S.tabs.find(x => x.id === S.active);
    if (!tab) { S.active = 'search'; return renderSearch(pane); }
    renderTab(pane, tab);
  }
  function rangeButtons() {
    return [['15m', 900], ['1h', 3600], ['6h', 21600], ['24h', 86400], ['7d', 604800], ['30d', 2592000]]
      .map(([l, s]) => `<button class="lv-chip" onclick="wrmLogViewer.quickRange(${s})">${esc(t('lv_last'))} ${l}</button>`).join('') +
      `<button class="lv-chip" onclick="wrmLogViewer.quickRange(0)">${esc(t('lv_range_clear'))}</button>`;
  }
  function renderSearch(pane) {
    const s = S.search, ph = wrmDatePlaceholder();
    const hits = s.hits || [];
    const files = [...new Set(hits.map(h => h.path))];
    let list = hits.filter(h => (!s.file || h.path === s.file));
    const re = findRegex(s.q, s.regex, s.case, s.word);
    // Levels per hit (context lines take the level of their hit).
    list.forEach(h => { if (!h.level) h.level = lineLevel(h.text); });
    const counts = {};
    list.filter(h => !h.context).forEach(h => { counts[h.level] = (counts[h.level] || 0) + 1; });
    list = list.filter(h => s.levels.has(h.level));
    const timed = list.filter(h => h.time), untimed = list.filter(h => !h.time);
    timed.sort((a, b) => (s.desc ? -1 : 1) * (Date.parse(a.time) - Date.parse(b.time)) || (a.path < b.path ? -1 : 1) || a.line - b.line);
    if (!s.desc) timed.forEach(() => {});
    const rows = timed.concat(untimed.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : a.line - b.line))).slice(0, 5000);
    const byPath = Object.fromEntries(S.files.map(f => [f.path, f]));
    pane.innerHTML = `<div class="lv-tools">
 <input id="lv-q" type="search" placeholder="${esc(t('lv_search_q'))}" value="${esc(s.q)}" onkeydown="if(event.key==='Enter')wrmLogViewer.search()" oninput="wrmLogViewer.searchSet('q',this.value,true)">
 <label title="${esc(t('lv_regex'))}"><input type="checkbox" ${s.regex ? 'checked' : ''} onchange="wrmLogViewer.searchSet('regex',this.checked)">.*</label>
 <label title="${esc(t('lv_case'))}"><input type="checkbox" ${s.case ? 'checked' : ''} onchange="wrmLogViewer.searchSet('case',this.checked)">Aa</label>
 <label title="${esc(t('lv_word'))}"><input type="checkbox" ${s.word ? 'checked' : ''} onchange="wrmLogViewer.searchSet('word',this.checked)">\\b</label>
 <label>${esc(t('lv_context'))}<select onchange="wrmLogViewer.searchSet('context',+this.value)">${[0, 1, 2, 3, 5].map(n => opt(n, n, s.context)).join('')}</select></label>
 <select aria-label="scope" onchange="wrmLogViewer.searchSet('scope',this.value)">${opt('shown', t('lv_scope_shown'), s.scope)}${opt('selected', t('lv_scope_selected') + ' (' + S.selected.size + ')', s.scope)}</select>
 ${s.running ? `<button onclick="wrmLogViewer.stopSearch()">${esc(t('lv_stop'))}</button>` : `<button onclick="wrmLogViewer.search()" ${!S.files.length ? 'disabled' : ''}>🔎 ${esc(t('lv_search'))}</button>`}
</div>
<div class="lv-tools">
 <label>${esc(t('mon_from'))}<input id="lv-from" placeholder="${esc(ph)}" title="${esc(t('lv_date_hint') + ' ' + ph)}" value="${esc(s.from)}" size="16" onchange="wrmLogViewer.searchSet('from',this.value)"></label>
 <label>${esc(t('mon_to'))}<input id="lv-to" placeholder="${esc(ph)}" title="${esc(t('lv_date_hint') + ' ' + ph)}" value="${esc(s.to)}" size="16" onchange="wrmLogViewer.searchSet('to',this.value)"></label>
 <label>${esc(t('mon_timezone'))}<input id="lv-tz" list="lv-tz-list" value="${esc(tz())}" size="14" onchange="wrmLogViewer.setTZ(this.value)"></label>
 <datalist id="lv-tz-list">${['UTC', 'Europe/Zagreb', 'Europe/London', 'Europe/Berlin', 'America/New_York', 'Asia/Tokyo', Intl.DateTimeFormat().resolvedOptions().timeZone].filter((x, i, a) => x && a.indexOf(x) === i).map(z => `<option value="${esc(z)}">`).join('')}</datalist>
 ${rangeButtons()}
</div>
${s.error ? `<div class="hint-box warn" style="margin:6px">${esc(s.error)}</div>` : ''}
${s.hits ? `<div class="lv-tools">
 <span class="mon-sub">${hits.filter(h => !h.context).length} ${esc(t('lv_hits'))} · ${files.length} ${esc(t('lv_files').toLowerCase())} · ${(s.ms / 1000).toFixed(1)} s</span>
 <button class="lv-chip" onclick="wrmLogViewer.searchSet('desc',${!s.desc})">${s.desc ? '↓ ' + esc(t('lv_newest')) : '↑ ' + esc(t('lv_oldest'))}</button>
 ${LEVELS.map(l => `<button class="lv-chip l-${l}${s.levels.has(l) ? '' : ' off'}" onclick="wrmLogViewer.searchLevel('${l}')">${esc(t('lv_lvl_' + l))} ${counts[l] || 0}</button>`).join('')}
 <select aria-label="file" onchange="wrmLogViewer.searchSet('file',this.value)">${opt('', t('lv_all_files'), s.file)}${files.map(p => opt(p, fileName(byPath[p] || {path: p}) + ' (' + hits.filter(h => h.path === p && !h.context).length + ')', s.file)).join('')}</select>
</div>${timeline(timed)}` : ''}
${s.truncated ? `<div class="hint-box warn" style="margin:6px">${esc(t('lv_hits_truncated'))}</div>` : ''}
<div class="lv-text lv-hits ${lsGet('wrm_lv_wrap', '') ? 'wrap' : ''}">${s.running ? `<div class="lv-empty">⏳ ${esc(t('mon_searching'))}</div>` : !s.hits ? `<div class="lv-empty">${esc(t('lv_search_intro'))}<br><br>${esc(t('lv_keys'))}</div>` : !rows.length ? `<div class="lv-empty">${esc(t('lv_no_hits'))}</div>` :
      rows.map(h => {
        const f = byPath[h.path] || {path: h.path};
        return `<div class="lv-ln l-${h.level}${h.context ? ' ctx' : ' hit'}" onclick="wrmLogViewer.openHit(${JSON.stringify(h.path).replace(/"/g, '&quot;')},${h.line})"><span class="n">${esc(h.time ? fmt(h.time) : t('lv_no_time'))}</span><span class="lv-hit-file" title="${esc(h.path)}:${h.line}">${esc(fileName(f))}:${h.line}</span><span class="c">${highlight(h.text, h.context ? null : re)}</span></div>`;
      }).join('')}</div>`;
  }
  function timeline(timed) {
    if (timed.length < 2) return '';
    const ts = timed.map(h => Date.parse(h.time)).filter(x => !isNaN(x));
    const lo = Math.min(...ts), hi = Math.max(...ts);
    if (hi <= lo) return '';
    const n = 60, w = (hi - lo) / n, b = new Array(n).fill(0);
    ts.forEach(x => b[Math.min(n - 1, Math.floor((x - lo) / w))]++);
    const top = Math.max(...b);
    S.search.tl = {lo, w};
    return `<svg class="lv-tl" viewBox="0 0 ${n * 10} 46" preserveAspectRatio="none" role="img" aria-label="${esc(t('lv_timeline'))}"><title>${esc(t('lv_timeline'))}</title>${b.map((c, i) => c ? `<rect x="${i * 10 + 1}" y="${44 - Math.max(2, 42 * c / top)}" width="8" height="${Math.max(2, 42 * c / top)}" onclick="wrmLogViewer.zoom(${i})"><title>${esc(fmt(new Date(lo + i * w)))} – ${esc(fmt(new Date(lo + (i + 1) * w)))}: ${c}</title></rect>` : '').join('')}</svg>`;
  }
  function searchSet(k, v, quiet) {
    S.search[k] = v;
    if (k === 'from' || k === 'to') {
      if (v && !wrmParseDateTime(v)) { showToast(t('lv_bad_date') + ' ' + wrmDatePlaceholder(), 'warning'); return; }
      if (S.search.q) return search();
    }
    if (!quiet) renderPane();
    if (k === 'scope' || k === 'context' || k === 'regex' || k === 'case' || k === 'word') if (S.search.q && S.search.hits) search();
  }
  function searchLevel(l) { const L = S.search.levels; if (L.has(l)) L.delete(l); else L.add(l); renderPane(); }
  function setTZ(v) {
    try { new Intl.DateTimeFormat('en', {timeZone: v}); } catch (_) { showToast(t('mon_timezone') + ': ' + v, 'warning'); return; }
    S.tz = v; render();
  }
  function quickRange(sec) {
    if (!sec) { S.search.from = ''; S.search.to = ''; }
    else { S.search.from = wrmFmt(new Date(Date.now() - sec * 1000), 'datetime', {timeZone: tz(), seconds: false}); S.search.to = ''; }
    if (S.search.q) search(); else renderPane();
  }
  function zoom(i) {
    const tl = S.search.tl;
    if (!tl) return;
    S.search.from = wrmFmt(new Date(tl.lo + i * tl.w), 'datetime', {timeZone: tz()});
    S.search.to = wrmFmt(new Date(tl.lo + (i + 1) * tl.w + 1000), 'datetime', {timeZone: tz()});
    search();
  }
  function stopSearch() { S.search.ctrl?.abort(); S.search.running = false; renderPane(); }
  async function search() {
    const s = S.search;
    s.q = ($('lv-q')?.value ?? s.q).trim();
    if (!s.q) { $('lv-q')?.focus(); return; }
    const from = wrmParseDateTime(s.from), to = wrmParseDateTime(s.to);
    if ((s.from && !from) || (s.to && !to)) { showToast(t('lv_bad_date') + ' ' + wrmDatePlaceholder(), 'warning'); return; }
    let files = s.scope === 'selected' ? [...S.selected] : shownFiles().map(f => f.path);
    files = files.slice(0, 2000);
    if (!files.length) { s.error = t('lv_no_discovery'); renderPane(); return; }
    s.ctrl?.abort();
    const ctrl = new AbortController();
    Object.assign(s, {ctrl, running: true, error: '', file: ''});
    S.active = 'search'; render();
    try {
      const r = await api('POST', '/api/monitoring/logs/grep?id=' + S.conn, {files, q: s.q, regex: s.regex, case: s.case, word: s.word, context: s.context, max: 500, from, to, timezone: tz()}, ctrl.signal);
      if (s.ctrl !== ctrl) return;
      Object.assign(s, {hits: r.hits || [], truncated: r.truncated || Object.values(r.per_file || {}).some(n => n >= 500), ms: r.ms || 0});
    } catch (e) {
      if (e.name !== 'AbortError') s.error = e.message;
    } finally {
      if (s.ctrl === ctrl) { s.running = false; s.ctrl = null; render(); }
    }
  }

  // ── Viewer tabs ─────────────────────────────────────────────────
  function fileByPath(p) { return S.files.find(f => f.path === p) || {path: p, kind: String(p).startsWith('journal:') ? 'journal' : '', unit: String(p).startsWith('journal:') ? p.slice(8) : ''}; }
  function newTab(p) {
    let tab = S.tabs.find(x => x.file.path === p);
    if (tab) return tab;
    if (S.tabs.length >= 12) closeTab(S.tabs[0].id);
    tab = {id: 'lt' + (++S.seq), file: fileByPath(p), lines: [], first: 1, total: 0, bytes: 0, cursor: '', find: '', findRegex: false, findCase: false, only: false,
      levels: new Set(LEVELS), match: -1, follow: false, timer: 0, loading: false, error: '', note: '', focusLine: 0, scroll: null};
    S.tabs.push(tab);
    return tab;
  }
  async function openFile(p) {
    const tab = newTab(p);
    S.active = tab.id;
    render();
    if (!tab.lines.length && !tab.loading) await loadTail(tab);
  }
  async function openHit(p, line) {
    const tab = newTab(p);
    S.active = tab.id;
    if (!tab.find && S.search.q) Object.assign(tab, {find: S.search.q, findRegex: S.search.regex, findCase: S.search.case});
    render();
    if (isJournal(tab.file)) { if (!tab.lines.length) await loadTail(tab); return; }
    stopFollow(tab);
    tab.focusLine = line; tab.scroll = null;
    if (tab.lines.length && line >= tab.first && line < tab.first + tab.lines.length) { renderPane(); return; }
    await load(tab, {mode: 'around', line, before: 300, after: 700});
  }
  async function load(tab, params, append) {
    tab.loading = true; tab.error = ''; renderPane();
    try {
      const q = new URLSearchParams({id: S.conn, path: tab.file.path, ...params});
      const r = await api('GET', '/api/monitoring/logs/lines?' + q);
      const lines = r.text ? r.text.split('\n') : [];
      if (append === 'before') { tab.lines = lines.concat(tab.lines).slice(0, 20000); tab.first = r.first; }
      else { tab.lines = lines; tab.first = r.first || 1; }
      tab.total = r.total || tab.total; tab.bytes = r.bytes || 0; tab.cursor = r.cursor || tab.cursor; tab.size = r.size;
      tab.levelsOf = null;
    } catch (e) {
      tab.error = e.message;
    } finally {
      tab.loading = false;
      if (S.tabs.includes(tab)) renderPane();
    }
  }
  function loadTail(tab) { tab.focusLine = 0; tab.scroll = 'end'; return load(tab, {mode: 'tail', n: 2000}); }
  async function older(tab) {
    if (isJournal(tab.file) || tab.first <= 1) return;
    const n = Math.min(2000, tab.first - 1), line = tab.first - n;
    const keep = tab.lines.length;
    await load(tab, {mode: 'around', line, before: 0, after: n - 1}, 'before');
    tab.scroll = {line: tab.first + (tab.lines.length - keep)};
    renderPane();
  }
  function stopFollow(tab) { tab.follow = false; clearTimeout(tab.timer); }
  function toggleFollow(tab) {
    if (tab.follow) { stopFollow(tab); renderPane(); return; }
    tab.follow = true; tab.focusLine = 0;
    const atTail = isJournal(tab.file) ? !!tab.cursor : tab.total && tab.first + tab.lines.length - 1 >= tab.total;
    (atTail ? Promise.resolve() : loadTail(tab)).then(() => { tab.scroll = 'end'; renderPane(); poll(tab); });
  }
  async function poll(tab) {
    if (!tab.follow || !S.tabs.includes(tab)) return;
    if (!document.hidden && S.host?.isConnected) {
      try {
        const journal = isJournal(tab.file);
        const params = journal ? (tab.cursor ? {mode: 'since', cursor: tab.cursor} : {mode: 'tail', n: 500}) : {mode: 'since', bytes: tab.bytes};
        const r = await api('GET', '/api/monitoring/logs/lines?' + new URLSearchParams({id: S.conn, path: tab.file.path, ...params}));
        if (r.reset) { tab.note = t('lv_rotated_reset'); await loadTail(tab); }
        else if (r.text) {
          const add = r.text.split('\n');
          const view = $('lv-text'), atEnd = view && view.scrollTop + view.clientHeight >= view.scrollHeight - 30;
          tab.lines = tab.lines.concat(add);
          if (tab.lines.length > 20000) { const drop = tab.lines.length - 20000; tab.lines = tab.lines.slice(drop); tab.first += drop; }
          if (!journal) { tab.total = tab.first + tab.lines.length - 1; tab.bytes = r.bytes; }
          tab.cursor = r.cursor || tab.cursor;
          tab.levelsOf = null;
          if (atEnd) tab.scroll = 'end';
          if (S.active === tab.id) renderPane();
        } else if (r.bytes != null && !journal) tab.bytes = r.bytes;
        tab.error = '';
      } catch (e) {
        tab.error = t('mon_reconnect') + ': ' + e.message;
        if ([401, 403, 404].includes(e.status)) { stopFollow(tab); }
        if (S.active === tab.id) renderPane();
      }
    }
    if (tab.follow) tab.timer = setTimeout(() => poll(tab), 2500);
  }
  function closeTab(id) {
    const tab = S.tabs.find(x => x.id === id);
    if (tab) stopFollow(tab);
    S.tabs = S.tabs.filter(x => x.id !== id);
    if (S.active === id) S.active = S.tabs.length ? S.tabs[S.tabs.length - 1].id : 'search';
    render();
  }
  function levelsFor(tab) {
    if (tab.levelsOf) return tab.levelsOf;
    let prev = '';
    tab.levelsOf = tab.lines.map(l => (prev = lineLevel(l, prev)));
    return tab.levelsOf;
  }
  function renderTab(pane, tab) {
    const prevScroll = $('lv-text')?.dataset.tab === tab.id ? $('lv-text').scrollTop : null;
    const re = findRegex(tab.find, tab.findRegex, tab.findCase, false);
    const lv = levelsFor(tab);
    const counts = {};
    lv.forEach(l => { counts[l] = (counts[l] || 0) + 1; });
    let html = '', matches = 0, shown = 0;
    const wrap = !!lsGet('wrm_lv_wrap', ''), nums = lsGet('wrm_lv_nums', '1') === '1';
    const journal = isJournal(tab.file);
    tab.matchLines = [];
    for (let i = 0; i < tab.lines.length; i++) {
      const text = tab.lines[i], n = tab.first + i;
      if (!tab.levels.has(lv[i])) continue;
      let hit = false;
      if (re) { re.lastIndex = 0; hit = re.test(text); }
      if (tab.only && re && !hit) continue;
      if (hit) { tab.matchLines.push(n); matches++; }
      shown++;
      html += `<div class="lv-ln l-${lv[i]}${hit ? ' hit' : ''}${n === tab.focusLine ? ' cur' : ''}" data-n="${n}">${nums ? `<span class="n" onclick="wrmLogViewer.pickLine(${n})">${journal ? '' : n}</span>` : ''}<span class="c">${highlight(text, re, hit && tab.matchLines[tab.match] === n ? 0 : -1)}</span></div>`;
    }
    if (tab.match >= tab.matchLines.length) tab.match = tab.matchLines.length - 1;
    const range = journal ? `${tab.lines.length} ${t('lv_lines')}` : tab.lines.length ? `${t('lv_lines')} ${tab.first}–${tab.first + tab.lines.length - 1} ${t('lv_of')} ${tab.total || '?'}` : '';
    pane.innerHTML = `<div class="lv-tools">
 <input id="lv-find" type="search" placeholder="${esc(t('lv_find'))}" value="${esc(tab.find)}" oninput="wrmLogViewer.tabSet('find',this.value)" onkeydown="if(event.key==='Enter'){event.preventDefault();wrmLogViewer.jump(event.shiftKey?-1:1)}">
 <label title="${esc(t('lv_regex'))}"><input type="checkbox" ${tab.findRegex ? 'checked' : ''} onchange="wrmLogViewer.tabSet('findRegex',this.checked)">.*</label>
 <label title="${esc(t('lv_case'))}"><input type="checkbox" ${tab.findCase ? 'checked' : ''} onchange="wrmLogViewer.tabSet('findCase',this.checked)">Aa</label>
 <button class="btn-sec btn-sm" title="${esc(t('mon_previous'))}" onclick="wrmLogViewer.jump(-1)">↑</button><button class="btn-sec btn-sm" title="${esc(t('mon_next'))}" onclick="wrmLogViewer.jump(1)">↓</button>
 <span class="mon-sub">${re ? (tab.match >= 0 ? tab.match + 1 + '/' : '') + matches + ' ' + esc(t('mon_matches').toLowerCase()) : ''}</span>
 <label><input type="checkbox" ${tab.only ? 'checked' : ''} onchange="wrmLogViewer.tabSet('only',this.checked)">${esc(t('lv_only_matching'))}</label>
</div>
<div class="lv-tools">
 ${LEVELS.map(l => `<button class="lv-chip l-${l}${tab.levels.has(l) ? '' : ' off'}" onclick="wrmLogViewer.tabLevel('${l}')">${esc(t('lv_lvl_' + l))} ${counts[l] || 0}</button>`).join('')}
 <span style="flex:1"></span>
 <button class="btn-sm ${tab.follow ? '' : 'btn-sec'}" onclick="wrmLogViewer.follow()" title="f">${tab.follow ? '⏸ ' + esc(t('lv_following')) : '▶ ' + esc(t('lv_follow'))}</button>
 ${journal ? '' : `<button class="btn-sec btn-sm" onclick="wrmLogViewer.older()" ${tab.first <= 1 || tab.loading ? 'disabled' : ''}>⇡ ${esc(t('lv_older'))}</button><button class="btn-sec btn-sm" onclick="wrmLogViewer.gotoLine()" title="g">↧ ${esc(t('lv_goto'))}</button>`}
 <label title="w"><input type="checkbox" ${wrap ? 'checked' : ''} onchange="wrmLogViewer.pref('wrm_lv_wrap',this.checked?'1':'')">${esc(t('lv_wrap'))}</label>
 <label><input type="checkbox" ${nums ? 'checked' : ''} onchange="wrmLogViewer.pref('wrm_lv_nums',this.checked?'1':'0')">${esc(t('lv_numbers'))}</label>
 <button class="btn-sec btn-sm" title="${esc(t('lv_copy'))}" onclick="wrmLogViewer.copy()">⧉</button>
 <button class="btn-sec btn-sm" title="${esc(t('lv_download'))}" onclick="wrmLogViewer.download()">⬇</button>
 ${journal ? '' : `<button class="btn-sec btn-sm" title="${esc(t('lv_terminal'))}" onclick="wrmLogViewer.terminal()">⌨</button>`}
 <button class="btn-sec btn-sm" title="${esc(t('lv_ask_ai'))}" onclick="wrmLogViewer.askAI(true)">🤖</button>
</div>
<div class="lv-status">${esc([journal ? t('mon_journal') + ' ' + (tab.file.unit || '') : tab.file.path, range, tab.size != null && !journal ? fmtBytes(tab.size) : '', tab.loading ? '⏳' : '', tab.note].filter(Boolean).join(' · '))}${tab.error ? ` · <span style="color:var(--red)">${esc(tab.error)}</span>` : ''}</div>
<div class="lv-text${wrap ? ' wrap' : ''}" id="lv-text" data-tab="${tab.id}" tabindex="0">${html || `<div class="lv-empty">${esc(tab.loading ? '⏳' : t('none'))}</div>`}</div>`;
    const view = $('lv-text');
    view.onkeydown = e => keys(e, tab);
    if (tab.scroll === 'end') view.scrollTop = view.scrollHeight;
    else if (tab.scroll && tab.scroll.line) view.querySelector(`[data-n="${tab.scroll.line}"]`)?.scrollIntoView({block: 'start'});
    else if (tab.focusLine && tab.scroll !== 'keep') view.querySelector(`[data-n="${tab.focusLine}"]`)?.scrollIntoView({block: 'center'});
    else if (prevScroll != null) view.scrollTop = prevScroll;
    if (!tab.loading) tab.scroll = tab.follow ? null : 'keep';
    if (tab.follow && prevScroll == null) view.scrollTop = view.scrollHeight;
  }
  const cur = () => S.tabs.find(x => x.id === S.active);
  function tabSet(k, v) {
    const tab = cur(); if (!tab) return;
    tab[k] = v; tab.match = -1;
    const pos = $('lv-find')?.selectionStart;
    renderPane();
    if (k === 'find') { const el = $('lv-find'); el.focus(); el.setSelectionRange(pos, pos); }
  }
  function tabLevel(l) { const tab = cur(); if (!tab) return; if (tab.levels.has(l)) tab.levels.delete(l); else tab.levels.add(l); tab.scroll = 'keep'; renderPane(); }
  function jump(d) {
    const tab = cur(); if (!tab || !tab.matchLines?.length) return;
    tab.match = (tab.match + d + tab.matchLines.length) % tab.matchLines.length;
    tab.focusLine = tab.matchLines[tab.match]; tab.scroll = null;
    const pos = $('lv-find')?.selectionStart, focused = document.activeElement?.id === 'lv-find';
    renderPane();
    if (focused) { $('lv-find').focus(); $('lv-find').setSelectionRange(pos, pos); }
  }
  function pickLine(n) { const tab = cur(); if (!tab) return; tab.focusLine = n; tab.scroll = 'keep'; renderPane(); }
  async function gotoLine() {
    const tab = cur(); if (!tab) return;
    const v = prompt(t('lv_goto') + (tab.total ? ' (1–' + tab.total + ')' : ''), tab.focusLine || '');
    const n = parseInt(v, 10);
    if (!n || n < 1) return;
    tab.focusLine = n; tab.scroll = null; stopFollow(tab);
    if (n >= tab.first && n < tab.first + tab.lines.length) renderPane();
    else await load(tab, {mode: 'around', line: n, before: 500, after: 1000});
  }
  function pref(k, v) { lsSet(k, v); const tab = cur(); if (tab) tab.scroll = 'keep'; render(); }
  function shownText() {
    const tab = cur(); if (!tab) return '';
    return [...($('lv-text')?.querySelectorAll('.lv-ln .c') || [])].map(x => x.textContent).join('\n');
  }
  function copy() { copyText(shownText()); }
  function download() {
    const tab = cur(); if (!tab) return;
    const url = URL.createObjectURL(new Blob([shownText() + '\n'], {type: 'text/plain'}));
    const a = document.createElement('a');
    a.href = url; a.download = (fileName(tab.file) || 'log') + '-excerpt.txt';
    document.body.appendChild(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 5000);
  }
  function terminal() {
    const tab = cur(); if (!tab) return;
    S.ctx.close?.();
    const wo = addWindow(); wo.connId = S.conn; wo.pendingCwd = dir(tab.file.path); wo.pendingLogPath = tab.file.path; setWinMode(wo.id, 'ssh');
  }
  function keys(e, tab) {
    if (e.target.tagName === 'INPUT') return;
    if (e.key === '/') { e.preventDefault(); $('lv-find')?.focus(); }
    else if (e.key === 'n') jump(1);
    else if (e.key === 'N') jump(-1);
    else if (e.key === 'g') gotoLine();
    else if (e.key === 'f') toggleFollow(tab);
    else if (e.key === 'w') pref('wrm_lv_wrap', lsGet('wrm_lv_wrap', '') ? '' : '1');
    else if (e.key === 'G' || e.key === 'End') { const v = $('lv-text'); v.scrollTop = v.scrollHeight; }
  }

  // ── Log settings (separate from Zabbix settings) ────────────────
  async function openSettings() {
    if (!S.conn) return;
    const s = await loadSettings();
    if (!s) return showToast(t('git_load_failed'), 'error');
    const lines = a => esc((a || []).join('\n'));
    const ro = S.canEdit ? '' : ' disabled';
    const check = (k, label) => `<label><input type="checkbox" id="lv-s-${k}" ${s[k] ? 'checked' : ''}${ro}> ${esc(t(label))}</label>`;
    const num = (k, label, lo, hi) => `<div><label for="lv-s-${k}">${esc(t(label))}</label><input id="lv-s-${k}" type="number" min="${lo}" max="${hi}" value="${esc(s[k])}"${ro}></div>`;
    $('lv-settings')?.remove();
    const el = document.createElement('div');
    el.id = 'lv-settings'; el.className = 'modal-overlay open'; el.style.zIndex = 8000;
    el.innerHTML = `<div class="modal" style="max-width:min(980px,96vw);width:96vw;max-height:92vh;overflow:auto" role="dialog" aria-modal="true" aria-labelledby="lv-s-title">
<div class="modal-header"><h3 id="lv-s-title">⚙ ${esc(t('lv_settings'))}</h3><button class="btn-sec btn-sm" onclick="document.getElementById('lv-settings').remove()">✕</button></div>
<p class="mon-sub">${esc(t('lv_settings_hint'))}</p>${S.canEdit ? '' : `<div class="hint-box warn">${esc(t('lv_readonly'))}</div>`}
<div class="lv-form">
 <div><label for="lv-s-scan_paths">${esc(t('lv_scan_paths'))}</label><textarea id="lv-s-scan_paths"${ro}>${lines(s.scan_paths)}</textarea><div class="d">${esc(t('lv_scan_paths_d'))}</div></div>
 <div><label for="lv-s-main_folders">${esc(t('lv_main_folders'))}</label><textarea id="lv-s-main_folders"${ro}>${lines(s.main_folders)}</textarea><div class="d">${esc(t('lv_main_folders_d'))}</div></div>
 <div><label for="lv-s-exclude">${esc(t('lv_exclude'))}</label><textarea id="lv-s-exclude"${ro}>${lines(s.exclude)}</textarea><div class="d">${esc(t('lv_exclude_d'))}</div></div>
 <div class="wide"><label for="lv-s-patterns">${esc(t('lv_patterns'))}</label><textarea id="lv-s-patterns"${ro}>${lines(s.patterns)}</textarea><div class="d">${esc(t('lv_patterns_d'))}</div></div>
 <div><label for="lv-s-roots">${esc(t('lv_roots'))}</label><textarea id="lv-s-roots"${ro}>${esc((s.roots || []).map(x => [x.kind === 'journal' ? 'journal:' + x.unit : x.path, x.service, x.operator].join(' | ')).join('\n'))}</textarea></div>
 <div><label for="lv-s-extra">${esc(t('lv_extra'))}</label><textarea id="lv-s-extra"${ro}>${lines(s.extra)}</textarea><div class="d">${esc(t('lv_extra_d'))}</div></div>
 ${num('max_depth', 'lv_max_depth', 1, 20)}${num('max_files', 'lv_max_files', 100, 50000)}${num('max_age_days', 'lv_max_age', 0, 3650)}${num('time_budget', 'lv_time_budget', 5, 300)}
 <div><label for="lv-s-method">${esc(t('lv_method'))}</label><select id="lv-s-method"${ro}>${['auto', 'ssh', 'sftp'].map(k => opt(k, t('lv_method_' + k), s.method || 'auto')).join('')}</select></div>
 <div><label for="lv-s-timezone">${esc(t('lv_timezone'))}</label><input id="lv-s-timezone" list="lv-tz-list" value="${esc(s.timezone || '')}" placeholder="${esc(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')}"${ro}></div>
 <div class="wide lv-checks">${check('guess_operators', 'lv_guess_operators')}${check('rotated', 'lv_rotated')}${check('compressed', 'lv_compressed')}${check('skip_empty', 'lv_skip_empty')}${check('journal', 'lv_journal')}${check('systemd', 'lv_systemd')}${check('docker', 'lv_docker')}${check('supervisor', 'lv_supervisor')}</div>
</div>
<div class="modal-footer">${S.canEdit ? `<button class="btn-sec" onclick="wrmLogViewer.settingsDefaults()">${esc(t('lv_defaults'))}</button><button class="btn-sec" onclick="wrmLogViewer.saveSettings(false)">${esc(t('save'))}</button><button onclick="wrmLogViewer.saveSettings(true)">${esc(t('lv_save_find'))}</button>` : ''}</div></div>`;
    document.body.appendChild(el);
    el.addEventListener('keydown', e => { if (e.key === 'Escape') el.remove(); });
    $('lv-s-scan_paths')?.focus();
  }
  function settingsDefaults() {
    const d = S.defaults || {};
    $('lv-s-scan_paths').value = (d.scan_paths || []).join('\n');
    $('lv-s-exclude').value = (d.exclude || []).join('\n');
    $('lv-s-patterns').value = (d.patterns || []).join('\n');
  }
  async function saveSettings(andFind) {
    const v = k => $('lv-s-' + k).value;
    const list = k => v(k).split('\n').map(x => x.trim()).filter(Boolean);
    const body = Object.assign({}, S.settings, {
      scan_paths: list('scan_paths'), main_folders: list('main_folders'), exclude: list('exclude'), patterns: list('patterns'), extra: list('extra'),
      roots: list('roots').map(x => { const [p, service = '', operator = ''] = x.split('|').map(y => y.trim()); return p.startsWith('journal:') ? {kind: 'journal', unit: p.slice(8), path: p, service, operator} : {path: p, service, operator}; }),
      max_depth: +v('max_depth'), max_files: +v('max_files'), max_age_days: +v('max_age_days'), time_budget: +v('time_budget'),
      method: v('method'), timezone: v('timezone').trim(),
    });
    for (const k of ['guess_operators', 'rotated', 'compressed', 'skip_empty', 'journal', 'systemd', 'docker', 'supervisor']) body[k] = $('lv-s-' + k).checked;
    try {
      const r = await api('PUT', '/api/monitoring/logs/settings?id=' + S.conn, body);
      S.settings = r.settings || body; S.tz = S.settings.timezone || '';
      $('lv-settings')?.remove();
      showToast(t('lv_saved'), 'success');
      if (andFind) discover(); else render();
    } catch (e) { showToast(e.message, 'error'); }
  }

  // ── AI ──────────────────────────────────────────────────────────
  function askAI(fromTab) {
    const tab = fromTab ? cur() : null;
    const server = (S.ctx.servers || []).find(x => String(x.connection.id) === String(S.conn));
    const ctx = {connId: S.conn, connName: server?.connection.name || '', file: tab ? tab.file.path : '', find: tab ? tab.find : S.search.q,
      excerpt: tab ? shownText().split('\n').slice(-200).join('\n') : '', timezone: tz()};
    if (typeof openLogAI === 'function') return openLogAI(ctx);
    if (!S.aiLoading) S.aiLoading = new Promise((ok, fail) => {
      const script = document.createElement('script');
      script.src = '/static/aichat.js'; script.onload = ok;
      script.onerror = () => { S.aiLoading = null; fail(new Error(t('git_load_failed'))); };
      document.head.appendChild(script);
    });
    S.aiLoading.then(() => openLogAI(ctx)).catch(e => showToast(e.message, 'error'));
  }
  function refreshTimes() { if (S.host?.isConnected) render(); }

  window.wrmLogViewer = {mount, render, discover, set, select, selectShown, toggleDir, selectDir, setServer, resize, activate, openFile, openHit, closeTab,
    search, stopSearch, searchSet, searchLevel, setTZ, quickRange, zoom,
    tabSet, tabLevel, jump, pickLine, gotoLine, pref, copy, download, terminal, older: () => cur() && older(cur()), follow: () => cur() && toggleFollow(cur()),
    openSettings, saveSettings, settingsDefaults, askAI, refreshTimes, stopAll,
    _test: {highlight, lineLevel, findRegex}};
})();
