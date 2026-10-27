/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// Git workspace of WRM PRO: compare services on servers with GitLab / GitHub or offline bundles.
// Loaded on demand by openGit() in index.html; uses its helpers (t, esc, showToast, apiError,
// uiConfirm, fmtTime, conns, folders, currentUser).
'use strict';

Object.assign(LANGS.en, { hf_runtime:'Runtime', hf_freshness:'Freshness', hf_stale:'Older than 30 minutes', hf_installation:'Installation', hf_problems:'Installation problems', hf_reset:'Reset filters', hf_matches:'Matching results',
  git_title: 'Git workspace', git_close: 'Close', git_tab_overview: 'Overview', git_tab_services: 'Services', git_tab_installs: 'Installations', git_tab_settings: 'Settings',
  git_check_now: 'Check now', git_checking: 'Checking…', git_last_check: 'Last check', git_never: 'never', git_no_checks: 'Checks are not allowed for your account.',
  git_st_ok: 'Up to date', git_st_update: 'Needs update', git_st_review: 'Review', git_st_error: 'Error', git_st_gone: 'Gone', git_st_unknown: 'Not checked',
  git_fs_ok: 'ok', git_fs_old: 'old', git_fs_modified: 'modified', git_fs_missing: 'missing', git_fs_extra: 'extra', git_fs_protected: 'protected',
  git_behind: '{n} behind', git_vmd_missing: 'missing', git_version_md: 'VERSION.md', git_target: 'Target', git_latest_tag: 'Latest tag',
  git_check_vmd: 'Also check VERSION.md', git_check_vmd_h: 'Off: the version is detected from the code only. On: VERSION.md is compared with the code as well.',
  git_detected: 'Detected version', git_detected_code: 'from the code', git_detected_mixed: 'Mixed / modified', git_detected_modified: 'Modified',
  git_detected_missing: 'Incomplete', git_detected_unknown: 'Unknown', git_vmd_ignored: 'VERSION.md is not used for the version check.',
  git_vmd_match: 'VERSION.md matches the code.', git_vmd_mismatch: 'VERSION.md says {v} ({c}); the code matches {dv} ({dc}).', git_vmd_no_version: 'VERSION.md cannot be matched to a single version ({s}).',
  git_vmd_none: 'No VERSION.md on the server.', git_code_error: 'Code check failed: {e}', git_approx: 'approx.',
  git_all_states: 'All states', git_all_envs: 'All environments', git_all_folders: 'All connection folders', git_all_dirs: 'All server folders', git_dir_filter_h: 'Folders on the servers: a folder also shows everything below it.', git_all_tags: 'All tags', git_search: 'Search…',
  git_ref_choice: 'Ref', git_ref_catalog: 'As in the catalog', git_ref_default: 'Default branch of each project', git_ref_branch: 'One branch for all',
  git_ref_override: 'Ref for this run', git_ref_override_ph: 'tag:latest, tag:v2 or a branch (empty = as chosen above)',
  git_server: 'Server', git_service: 'Service', git_path: 'Path', git_env: 'Environment', git_state: 'State', git_files: 'Files', git_checked: 'Checked', git_actions: 'Actions',
  git_empty_overview: 'Nothing to show yet. Add a source or import a bundle, fill in the catalog, choose the servers in Settings and run a check.',
  git_targets: 'Targets', git_refresh_targets: 'Refresh targets', git_source: 'Source', git_warnings: 'Warnings', git_no_target: 'no target yet',
  git_add_service: 'Add service', git_suggest: 'Suggest from repository', git_import_catalog: 'Import catalog', git_export_catalog: 'Export catalog',
  git_import_merge: 'Merge with the current catalog (OK) or replace it (Cancel)?', git_export_bundle: 'Export bundle', git_export_bundle_none: 'Select services with a target first.',
  git_from_bundle: 'From imported bundles (not in the catalog)', git_edit: 'Edit', git_delete: 'Delete', git_delete_q: 'Delete service {name} from the catalog?', git_save: 'Save', git_cancel: 'Cancel',
  git_f_name: 'Name', git_f_project: 'Project (group/name)', git_f_ref: 'Ref', git_f_ref_h: 'tag:latest, tag:v1 or a branch', git_f_branch: 'Branch (for tags)', git_f_tag_filter: 'Tag filter (regex)',
  git_f_subdir: 'Subdirectory = installation root', git_f_kind: 'Kind', git_kind_app: 'App', git_kind_library: 'Library', git_kind_tool: 'Tool',
  git_f_include: 'Include globs', git_f_exclude: 'Exclude globs', git_f_protected: 'Protected (never overwritten, per host)', git_f_fingerprint: 'Fingerprint files', git_f_hint: 'Install hints (directory names)',
  git_lines_h: 'One per line or comma-separated', git_suggest_project: 'Project', git_suggest_ref: 'Branch (empty = default)', git_suggest_go: 'Suggest', git_repo_files: 'Repository files',
  git_discover: 'Discover', git_add_install: 'Add by hand', git_details: 'Details', git_forget: 'Forget', git_forget_q: 'Forget this installation? Discovery may find it again.',
  git_units: 'Units', git_no_units: 'No systemd or supervisor unit mentions this directory.', git_diff: 'Diff', git_diff_title: 'Server → target', git_binary: 'Binary file',
  git_no_diff: 'No differences in the text (line endings or whitespace only).', git_capped: 'Only the first 5000 files were checked.', git_files_only: 'Show only changes',
  git_sources: 'Git sources', git_add_source: 'Add source', git_kind: 'Kind', git_url: 'Address', git_token: 'Read-only token', git_token_keep: 'leave empty to keep the stored token',
  git_test: 'Test', git_projects_found: '{n} projects', git_bundles: 'Offline bundles', git_import_bundle: 'Import bundle (.tar.gz)', git_bundle_age: 'created {d}',
  git_bundle_imported: 'Bundle imported: {n} files', git_source_mode: 'Targets from', git_mode_auto: 'Automatic (Git API, otherwise the newest bundle)', git_mode_live: 'Git API',
  git_mode_bundle: 'Offline bundle', git_newest_bundle: 'Newest bundle with the service', git_history_depth: 'History depth (earlier versions per file)',
  git_servers: 'Servers to check', git_servers_h: 'SSH connections. Checks only read files, nothing is changed on servers.', git_filter: 'Filter…', git_select_all: 'All', git_select_none: 'None',
  git_catalog_settings: 'Catalog settings', git_groups: 'Groups / organisations', git_fallback: 'Fallback branches', git_roots: 'Server roots (searched to depth 3)', git_ignore: 'Ignored directories (globs)',
  git_monitor: 'Periodic checks', git_monitor_on: 'Check my servers every {n} minutes while WRM runs and notify me', git_monitor_off: 'Periodic checks are turned off by the administrator.',
  git_monitor_h: 'Choose the channels for "Git" events in Settings → Notifications. Checks never update anything.', git_saved: 'Saved',
  git_n_installs: '{n} installations', git_none: '—', git_api_calls: '{n} API calls', git_from: 'from', git_bundle_only: 'bundle',
});
Object.assign(LANGS.hr, { hf_runtime:'Izvršavanje', hf_freshness:'Svježina', hf_stale:'Starije od 30 minuta', hf_installation:'Instalacija', hf_problems:'Problemi instalacije', hf_reset:'Poništi filtre', hf_matches:'Pronađeni rezultati',
  git_title: 'Git radni prostor', git_close: 'Zatvori', git_tab_overview: 'Pregled', git_tab_services: 'Servisi', git_tab_installs: 'Instalacije', git_tab_settings: 'Postavke',
  git_check_now: 'Provjeri sada', git_checking: 'Provjera…', git_last_check: 'Zadnja provjera', git_never: 'nikad', git_no_checks: 'Provjere nisu dopuštene vašem računu.',
  git_st_ok: 'Ažurno', git_st_update: 'Treba ažurirati', git_st_review: 'Pregledati', git_st_error: 'Greška', git_st_gone: 'Nema je', git_st_unknown: 'Nije provjereno',
  git_fs_ok: 'ok', git_fs_old: 'staro', git_fs_modified: 'izmijenjeno', git_fs_missing: 'nedostaje', git_fs_extra: 'višak', git_fs_protected: 'zaštićeno',
  git_behind: '{n} iza', git_vmd_missing: 'nema', git_version_md: 'VERSION.md', git_target: 'Cilj', git_latest_tag: 'Zadnji tag',
  git_check_vmd: 'Provjeri i VERSION.md', git_check_vmd_h: 'Isključeno: verzija se određuje samo iz koda. Uključeno: VERSION.md se uspoređuje s kodom.',
  git_detected: 'Detektirana verzija', git_detected_code: 'iz koda', git_detected_mixed: 'Miješano / izmijenjeno', git_detected_modified: 'Izmijenjeno',
  git_detected_missing: 'Nepotpuno', git_detected_unknown: 'Nepoznato', git_vmd_ignored: 'VERSION.md se ne koristi za provjeru verzije.',
  git_vmd_match: 'VERSION.md odgovara kodu.', git_vmd_mismatch: 'VERSION.md kaže {v} ({c}); kod odgovara {dv} ({dc}).', git_vmd_no_version: 'VERSION.md se ne može poistovjetiti s jednom verzijom ({s}).',
  git_vmd_none: 'Na serveru nema VERSION.md.', git_code_error: 'Provjera koda nije uspjela: {e}', git_approx: 'približno',
  git_all_states: 'Sva stanja', git_all_envs: 'Sva okruženja', git_all_folders: 'Sve mape veza', git_all_dirs: 'Svi folderi na serveru', git_dir_filter_h: 'Folderi na serverima: folder prikazuje i sve ispod sebe.', git_all_tags: 'Sve oznake', git_search: 'Traži…',
  git_ref_choice: 'Ref', git_ref_catalog: 'Kao u katalogu', git_ref_default: 'Zadana grana svakog projekta', git_ref_branch: 'Jedna grana za sve',
  git_ref_override: 'Ref za ovu provjeru', git_ref_override_ph: 'tag:latest, tag:v2 ili grana (prazno = kao gore)',
  git_server: 'Server', git_service: 'Servis', git_path: 'Putanja', git_env: 'Okruženje', git_state: 'Stanje', git_files: 'Datoteke', git_checked: 'Provjereno', git_actions: 'Akcije',
  git_empty_overview: 'Još nema ničega. Dodajte izvor ili uvezite bundle, popunite katalog, odaberite servere u Postavkama i pokrenite provjeru.',
  git_targets: 'Ciljevi', git_refresh_targets: 'Osvježi ciljeve', git_source: 'Izvor', git_warnings: 'Upozorenja', git_no_target: 'još nema cilja',
  git_add_service: 'Dodaj servis', git_suggest: 'Prijedlog iz repozitorija', git_import_catalog: 'Uvezi katalog', git_export_catalog: 'Izvezi katalog',
  git_import_merge: 'Spojiti s trenutnim katalogom (U redu) ili ga zamijeniti (Odustani)?', git_export_bundle: 'Izvezi bundle', git_export_bundle_none: 'Najprije odaberite servise koji imaju cilj.',
  git_from_bundle: 'Iz uvezenih bundleova (nisu u katalogu)', git_edit: 'Uredi', git_delete: 'Obriši', git_delete_q: 'Obrisati servis {name} iz kataloga?', git_save: 'Spremi', git_cancel: 'Odustani',
  git_f_name: 'Naziv', git_f_project: 'Projekt (grupa/naziv)', git_f_ref: 'Ref', git_f_ref_h: 'tag:latest, tag:v1 ili grana', git_f_branch: 'Grana (za tagove)', git_f_tag_filter: 'Filtar tagova (regex)',
  git_f_subdir: 'Poddirektorij = korijen instalacije', git_f_kind: 'Vrsta', git_kind_app: 'Aplikacija', git_kind_library: 'Biblioteka', git_kind_tool: 'Alat',
  git_f_include: 'Uključi (globovi)', git_f_exclude: 'Isključi (globovi)', git_f_protected: 'Zaštićeno (nikad se ne prepisuje, po hostu)', git_f_fingerprint: 'Datoteke za prepoznavanje', git_f_hint: 'Nazivi direktorija instalacije',
  git_lines_h: 'Jedno po retku ili odvojeno zarezom', git_suggest_project: 'Projekt', git_suggest_ref: 'Grana (prazno = zadana)', git_suggest_go: 'Predloži', git_repo_files: 'Datoteke repozitorija',
  git_discover: 'Pronađi', git_add_install: 'Dodaj ručno', git_details: 'Detalji', git_forget: 'Zaboravi', git_forget_q: 'Zaboraviti ovu instalaciju? Pretraga je može ponovno pronaći.',
  git_units: 'Jedinice', git_no_units: 'Nijedna systemd ili supervisor jedinica ne spominje ovaj direktorij.', git_diff: 'Razlike', git_diff_title: 'Server → cilj', git_binary: 'Binarna datoteka',
  git_no_diff: 'Nema razlika u tekstu (samo završeci redaka ili razmaci).', git_capped: 'Provjereno je samo prvih 5000 datoteka.', git_files_only: 'Prikaži samo promjene',
  git_sources: 'Git izvori', git_add_source: 'Dodaj izvor', git_kind: 'Vrsta', git_url: 'Adresa', git_token: 'Token samo za čitanje', git_token_keep: 'ostavite prazno za zadržavanje spremljenog tokena',
  git_test: 'Testiraj', git_projects_found: '{n} projekata', git_bundles: 'Offline bundleovi', git_import_bundle: 'Uvezi bundle (.tar.gz)', git_bundle_age: 'izrađen {d}',
  git_bundle_imported: 'Bundle uvezen: {n} datoteka', git_source_mode: 'Ciljevi iz', git_mode_auto: 'Automatski (Git API, inače najnoviji bundle)', git_mode_live: 'Git API',
  git_mode_bundle: 'Offline bundle', git_newest_bundle: 'Najnoviji bundle sa servisom', git_history_depth: 'Dubina povijesti (ranije verzije po datoteci)',
  git_servers: 'Serveri za provjeru', git_servers_h: 'SSH veze. Provjere samo čitaju datoteke, na serverima se ništa ne mijenja.', git_filter: 'Filtar…', git_select_all: 'Sve', git_select_none: 'Ništa',
  git_catalog_settings: 'Postavke kataloga', git_groups: 'Grupe / organizacije', git_fallback: 'Zamjenske grane', git_roots: 'Korijeni na serverima (pretraga do dubine 3)', git_ignore: 'Zanemareni direktoriji (globovi)',
  git_monitor: 'Periodične provjere', git_monitor_on: 'Provjeravaj moje servere svakih {n} minuta dok WRM radi i obavijesti me', git_monitor_off: 'Administrator je isključio periodične provjere.',
  git_monitor_h: 'Kanale za "Git" događaje odaberite u Postavke → Obavijesti. Provjere nikad ništa ne ažuriraju.', git_saved: 'Spremljeno',
  git_n_installs: '{n} instalacija', git_none: '—', git_api_calls: '{n} API poziva', git_from: 'iz', git_bundle_only: 'bundle',
});

Object.assign(LANGS.en, {
  git_tab_runs: 'Runs', git_update: 'Update', git_upgrade: 'Upgrade…', git_rollback: 'Rollback…', git_restart: 'Restart…', git_stamp: 'Stamp',
  git_select_installs: 'Select installations first.', git_restart_pending: 'restart pending', git_fs_changed: 'differs',
  git_k_update: 'Update', git_k_upgrade: 'Upgrade', git_k_rollback: 'Rollback', git_k_stamp: 'Stamp', git_k_restart: 'Restart',
  git_rs_scheduled: 'Scheduled', git_rs_starting: 'Starting', git_rs_running: 'Running', git_rs_done: 'Done', git_rs_failed: 'Failed', git_rs_cancelled: 'Cancelled',
  git_rs_skipped: 'Skipped', git_rs_interrupted: 'Interrupted', git_rs_pending: 'Waiting', git_rs_ok: 'OK', git_rs_rolled_back: 'Rolled back',
  git_plan_loading: 'Preparing…', git_upgrade_ref: 'Branch or tag to upgrade to', git_upgrade_ref_h: 'A branch (main) or a tag (tag:v2, tag:latest)', git_show_changes: 'Show changes',
  git_upgrade_warn: 'An upgrade switches the installations to another branch or version. Read the summary of the changes before you continue.',
  git_changes_summary: '{changed} to change, {missing} new, {local} changed locally, {removed} no longer in the target (they stay on the server)',
  git_prod: 'production', git_files_none: 'Nothing to change: the files are up to date.', git_show_all: 'Show all files', git_local: 'changed locally',
  git_local_warn: 'Ticked files with local changes are overwritten: the changes made on the server are lost (they stay in the backup).',
  git_check_cmd: 'Custom check for {app} (optional, runs in the installation directory; exit code 0 = OK)', git_ignore_imports: 'Skip the check for dangling Python imports',
  git_restart_choice: 'Service restart', git_rm_none: 'Do not restart (only show what needs a restart)', git_rm_now: 'Restart right after a successful update',
  git_rm_at: 'Restart at a chosen time', git_rm_delay: 'Restart after a delay (minutes)', git_restart_warn: 'These units will be restarted: {list}',
  git_no_units_restart: 'No systemd or supervisor unit is known for these installations (check them first).', git_when: 'When', git_run_now: 'Now', git_run_at: 'At a chosen time (maintenance window)',
  git_grace_h: 'A scheduled run is skipped when WRM was not running at its time for more than {n} minutes. A notification is sent before and after.',
  git_set_override: 'Follow this ref from now on (ref override for these services)', git_after_upgrade: 'Comparisons follow the service\'s ref (catalog or override) until you change it.',
  git_continue: 'Continue', git_back: 'Back', git_confirm_title: 'Confirm: {kind}', git_confirm_q: '{kind} of {n} installation(s) on {m} server(s):',
  git_type_host: 'Production: type the server name {name} to confirm', git_start: 'Start', git_schedule: 'Schedule', git_scheduled_ok: 'Scheduled for {t}',
  git_nothing_selected: 'No files are selected.', git_time_past: 'Choose a time in the future.', git_progress: 'Run {label}', git_stop: 'Stop after the current server',
  git_cancel_run: 'Cancel run', git_cancel_q: 'Cancel this run?', git_dangling: 'Dangling imports', git_health: 'Health check', git_backup: 'Backup',
  git_backups: 'Backups on the server', git_no_backups: 'No backups in .deploy-bak.', git_backup_files: '{n} files', git_backup_unknown: 'No record of this backup: only the files in it are restored.',
  git_force_stamp: 'Overwrite an existing VERSION.md', git_stamp_h: 'Stamp writes VERSION.md where an installation is up to date (and the file is missing). Tick the options below to overwrite an existing file or to stamp an installation as it is.',
  git_not_current: 'not up to date', git_has_vmd: 'VERSION.md exists', git_runs_empty: 'No runs yet.', git_created: 'Created', git_scheduled_for: 'Scheduled for',
  git_run: 'Run', git_kind_col: 'Action', git_items: 'Installations', git_written: '{n} written', git_deleted: '{n} deleted', git_waits_update: 'after the update',
  git_restore_hint: 'Restores the files of this backup; files that did not exist then are deleted. The current files are backed up first.', git_versions: 'Versions',
  git_refresh: 'Refresh', git_unit_list: 'Units', git_no_permission: 'Not allowed for your account.',
  git_step_connect: 'Log in', git_step_scan: 'Compare', git_step_prepare: 'Prepare', git_step_imports: 'Python imports', git_step_backups: 'Backups', git_step_backup: 'Backup',
  git_step_write: 'Write', git_step_checks: 'Checks', git_step_rollback: 'Rollback', git_step_version: 'VERSION.md', git_step_log: 'updates.jsonl', git_step_restart: 'Restart',
});
Object.assign(LANGS.hr, {
  git_tab_runs: 'Izvođenja', git_update: 'Ažuriraj', git_upgrade: 'Nadogradi…', git_rollback: 'Vrati…', git_restart: 'Ponovno pokreni…', git_stamp: 'Označi',
  git_select_installs: 'Najprije odaberite instalacije.', git_restart_pending: 'čeka ponovno pokretanje', git_fs_changed: 'razlikuje se',
  git_k_update: 'Ažuriranje', git_k_upgrade: 'Nadogradnja', git_k_rollback: 'Vraćanje', git_k_stamp: 'Označavanje', git_k_restart: 'Ponovno pokretanje',
  git_rs_scheduled: 'Zakazano', git_rs_starting: 'Pokreće se', git_rs_running: 'U tijeku', git_rs_done: 'Gotovo', git_rs_failed: 'Nije uspjelo', git_rs_cancelled: 'Otkazano',
  git_rs_skipped: 'Preskočeno', git_rs_interrupted: 'Prekinuto', git_rs_pending: 'Čeka', git_rs_ok: 'U redu', git_rs_rolled_back: 'Vraćeno',
  git_plan_loading: 'Priprema…', git_upgrade_ref: 'Grana ili tag za nadogradnju', git_upgrade_ref_h: 'Grana (main) ili tag (tag:v2, tag:latest)', git_show_changes: 'Prikaži promjene',
  git_upgrade_warn: 'Nadogradnja prebacuje instalacije na drugu granu ili verziju. Prije nastavka pročitajte sažetak promjena.',
  git_changes_summary: '{changed} za promjenu, {missing} novih, {local} lokalno izmijenjenih, {removed} više nije u cilju (ostaju na serveru)',
  git_prod: 'produkcija', git_files_none: 'Nema promjena: datoteke su ažurne.', git_show_all: 'Prikaži sve datoteke', git_local: 'lokalno izmijenjeno',
  git_local_warn: 'Označene datoteke s lokalnim izmjenama bit će prepisane: izmjene na serveru gube se (ostaju u sigurnosnoj kopiji).',
  git_check_cmd: 'Vlastita provjera za {app} (neobavezno, pokreće se u direktoriju instalacije; izlazni kod 0 = u redu)', git_ignore_imports: 'Preskoči provjeru visećih Python importa',
  git_restart_choice: 'Ponovno pokretanje servisa', git_rm_none: 'Ne pokreći ponovno (samo prikaži što treba ponovno pokrenuti)', git_rm_now: 'Ponovno pokreni odmah nakon uspješnog ažuriranja',
  git_rm_at: 'Ponovno pokreni u odabrano vrijeme', git_rm_delay: 'Ponovno pokreni nakon odgode (minute)', git_restart_warn: 'Ponovno će se pokrenuti ove jedinice: {list}',
  git_no_units_restart: 'Za ove instalacije nije poznata nijedna systemd ili supervisor jedinica (najprije ih provjerite).', git_when: 'Kada', git_run_now: 'Sada', git_run_at: 'U odabrano vrijeme (prozor održavanja)',
  git_grace_h: 'Zakazano izvođenje preskače se ako WRM u to vrijeme nije radio dulje od {n} minuta. Obavijest se šalje prije i poslije.',
  git_set_override: 'Ubuduće prati ovaj ref (zamjenski ref za ove servise)', git_after_upgrade: 'Usporedbe prate ref servisa (katalog ili zamjenski ref) dok ga ne promijenite.',
  git_continue: 'Nastavi', git_back: 'Natrag', git_confirm_title: 'Potvrda: {kind}', git_confirm_q: '{kind}: {n} instalacija na {m} servera:',
  git_type_host: 'Produkcija: upišite naziv servera {name} za potvrdu', git_start: 'Pokreni', git_schedule: 'Zakaži', git_scheduled_ok: 'Zakazano za {t}',
  git_nothing_selected: 'Nije odabrana nijedna datoteka.', git_time_past: 'Odaberite vrijeme u budućnosti.', git_progress: 'Izvođenje {label}', git_stop: 'Zaustavi nakon trenutnog servera',
  git_cancel_run: 'Otkaži izvođenje', git_cancel_q: 'Otkazati ovo izvođenje?', git_dangling: 'Viseći importi', git_health: 'Provjera stanja', git_backup: 'Sigurnosna kopija',
  git_backups: 'Sigurnosne kopije na serveru', git_no_backups: 'U .deploy-bak nema sigurnosnih kopija.', git_backup_files: '{n} datoteka', git_backup_unknown: 'Nema zapisa o ovoj kopiji: vraćaju se samo datoteke u njoj.',
  git_force_stamp: 'Prepiši postojeći VERSION.md', git_stamp_h: 'Označavanje upisuje VERSION.md gdje je instalacija ažurna (a datoteka nedostaje). Opcijama ispod možete prepisati postojeću datoteku ili označiti instalaciju kakva jest.',
  git_not_current: 'nije ažurno', git_has_vmd: 'VERSION.md postoji', git_runs_empty: 'Još nema izvođenja.', git_created: 'Izrađeno', git_scheduled_for: 'Zakazano za',
  git_run: 'Izvođenje', git_kind_col: 'Akcija', git_items: 'Instalacije', git_written: 'zapisano: {n}', git_deleted: 'obrisano: {n}', git_waits_update: 'nakon ažuriranja',
  git_restore_hint: 'Vraća datoteke iz ove kopije; datoteke koje tada nisu postojale brišu se. Trenutne datoteke najprije se spremaju u novu kopiju.', git_versions: 'Verzije',
  git_refresh: 'Osvježi', git_unit_list: 'Jedinice', git_no_permission: 'Nije dopušteno vašem računu.',
  git_step_connect: 'Prijava', git_step_scan: 'Usporedba', git_step_prepare: 'Priprema', git_step_imports: 'Python importi', git_step_backups: 'Sigurnosne kopije', git_step_backup: 'Sigurnosna kopija',
  git_step_write: 'Zapisivanje', git_step_checks: 'Provjere', git_step_rollback: 'Vraćanje', git_step_version: 'VERSION.md', git_step_log: 'updates.jsonl', git_step_restart: 'Ponovno pokretanje',
});

Object.assign(LANGS.en, {
  git_install: 'Install…', git_transfer: 'Transfer…', git_k_install: 'Install', git_k_transfer: 'Transfer', git_new_server: 'New server',
  git_w_server: 'Target server', git_w_services: 'Services', git_w_ref: 'Version', git_w_ref_target: 'Target ({v})', git_w_ref_other: 'Branch or tag…', git_w_ref_bundle: 'Bundle {b}',
  git_w_pick_service: 'Choose at least one service.', git_w_dirs: 'Target directories', git_w_roots: 'Server roots', git_w_root_missing: 'does not exist', git_w_root_ro: 'not writable',
  git_w_files: '{n} files, {kb} KB', git_w_check: 'Check the target', git_w_checks: 'Pre-checks', git_w_free: '{free} MB free, {need} MB needed', git_w_tools: 'Tools',
  git_w_missing_tool: 'missing', git_w_existing: 'An installation exists here ({n} files{v}).', git_w_overwrite: 'Replace it (the replaced files are backed up to .deploy-bak first)',
  git_w_protected: 'Per-host files', git_w_protected_h: 'Protected files are never taken from the repository. Fill them from a template, copy them from another installation or type them.',
  git_w_no_slots: 'The catalog lists no protected files for this service.', git_w_m_template: 'From a template', git_w_m_copy: 'Copy from', git_w_m_manual: 'Type the content', git_w_m_skip: 'Leave it out',
  git_w_tmpl_na: 'not available', git_w_in_repo: 'in the repository', git_w_typed: 'Typed value', git_w_vault_pw: '🔑 {n}: password', git_w_vault_user: '🔑 {n}: user name',
  git_w_vault_h: 'Values from the vault are read on the server side only and are not stored with the run.',
  git_w_unit: 'Create a service unit (optional)', git_w_unit_kind: 'Kind', git_w_unit_name: 'Name', git_w_unit_user: 'Run as user (empty = default)', git_w_unit_cmd: 'Command (absolute path)',
  git_w_unit_dir: 'Working directory', git_w_unit_enable: 'Start at boot (systemctl enable)',
  git_w_unit_warn: 'The unit file is written to the server as root (or with sudo -n) and is never written over an existing one. WRM does not test the command: check it.',
  git_w_start: 'Start the service after the install (restart of its units)', git_w_start_h: 'Units that mention the directory (also an existing or the new one) are started; their health is checked.',
  git_w_blocked: 'Fix the problems above first.', git_w_confirm: '{kind}: {n} service(s) on {server}', git_w_type: 'Type the server name {name} to confirm (production or overwrite)',
  git_w_source: 'Source', git_w_excluded: '{n} paths are not copied (logs, caches, backups, ignored directories)', git_w_tr_h: 'Copies the installation with its per-host configuration. Virtual environments and node_modules are not copied: recreate them on the target.',
  git_w_units_not_copied: 'Units of the source are not copied: {list}', git_w_same: 'Choose another server or directory than the source.', git_w_env: 'Environment',
  git_step_precheck: 'Pre-checks', git_step_fetch: 'Contents', git_step_receive: 'Copy to the server', git_step_register: 'Register', git_step_unit: 'Unit', git_step_source: 'Source',
});
Object.assign(LANGS.hr, {
  git_install: 'Instaliraj…', git_transfer: 'Prenesi…', git_k_install: 'Instalacija', git_k_transfer: 'Prijenos', git_new_server: 'Novi server',
  git_w_server: 'Ciljni server', git_w_services: 'Servisi', git_w_ref: 'Verzija', git_w_ref_target: 'Cilj ({v})', git_w_ref_other: 'Grana ili tag…', git_w_ref_bundle: 'Bundle {b}',
  git_w_pick_service: 'Odaberite barem jedan servis.', git_w_dirs: 'Ciljni direktoriji', git_w_roots: 'Korijeni na serveru', git_w_root_missing: 'ne postoji', git_w_root_ro: 'nije zapisiv',
  git_w_files: '{n} datoteka, {kb} KB', git_w_check: 'Provjeri cilj', git_w_checks: 'Provjere prije instalacije', git_w_free: 'slobodno {free} MB, potrebno {need} MB', git_w_tools: 'Alati',
  git_w_missing_tool: 'nema', git_w_existing: 'Ovdje postoji instalacija ({n} datoteka{v}).', git_w_overwrite: 'Zamijeni je (zamijenjene datoteke najprije se spremaju u .deploy-bak)',
  git_w_protected: 'Datoteke po hostu', git_w_protected_h: 'Zaštićene datoteke nikad se ne uzimaju iz repozitorija. Popunite ih iz predloška, kopirajte iz druge instalacije ili upišite.',
  git_w_no_slots: 'Katalog ne navodi zaštićene datoteke za ovaj servis.', git_w_m_template: 'Iz predloška', git_w_m_copy: 'Kopiraj iz', git_w_m_manual: 'Upiši sadržaj', git_w_m_skip: 'Izostavi',
  git_w_tmpl_na: 'nije dostupno', git_w_in_repo: 'u repozitoriju', git_w_typed: 'Upisana vrijednost', git_w_vault_pw: '🔑 {n}: lozinka', git_w_vault_user: '🔑 {n}: korisničko ime',
  git_w_vault_h: 'Vrijednosti iz trezora čitaju se samo na strani poslužitelja i ne spremaju se uz izvođenje.',
  git_w_unit: 'Izradi jedinicu servisa (neobavezno)', git_w_unit_kind: 'Vrsta', git_w_unit_name: 'Naziv', git_w_unit_user: 'Pokreni kao korisnik (prazno = zadano)', git_w_unit_cmd: 'Naredba (apsolutna putanja)',
  git_w_unit_dir: 'Radni direktorij', git_w_unit_enable: 'Pokreni pri podizanju sustava (systemctl enable)',
  git_w_unit_warn: 'Datoteka jedinice zapisuje se na server kao root (ili sa sudo -n) i nikad ne prepisuje postojeću. WRM ne testira naredbu: provjerite je.',
  git_w_start: 'Pokreni servis nakon instalacije (ponovno pokretanje njegovih jedinica)', git_w_start_h: 'Pokreću se jedinice koje spominju direktorij (i postojeće i nova); provjerava se njihovo stanje.',
  git_w_blocked: 'Najprije riješite gornje probleme.', git_w_confirm: '{kind}: {n} servis(a) na {server}', git_w_type: 'Upišite naziv servera {name} za potvrdu (produkcija ili zamjena)',
  git_w_source: 'Izvor', git_w_excluded: '{n} putanja se ne kopira (zapisi, priručne datoteke, sigurnosne kopije, zanemareni direktoriji)', git_w_tr_h: 'Kopira instalaciju s njezinom konfiguracijom po hostu. Virtualna okruženja i node_modules se ne kopiraju: izradite ih ponovno na cilju.',
  git_w_units_not_copied: 'Jedinice izvora se ne kopiraju: {list}', git_w_same: 'Odaberite drugi server ili direktorij od izvora.', git_w_env: 'Okruženje',
  git_step_precheck: 'Provjere', git_step_fetch: 'Sadržaj', git_step_receive: 'Kopiranje na server', git_step_register: 'Upis', git_step_unit: 'Jedinica', git_step_source: 'Izvor',
});

Object.assign(LANGS.en, {
  git_tab_gitignore: '.gitignore', gi_service: 'Service', gi_load: 'Load', gi_loading: 'Reading the repository and the latest checks…',
  gi_repo: 'Repository', gi_file: 'File', gi_branch: 'Branch', gi_current: 'Current .gitignore', gi_no_current: 'The repository has no .gitignore here yet.',
  gi_warnings: 'Committed files that should not be in Git (secrets, per-host configuration, logs)', gi_reason_secret: 'looks like a secret', gi_reason_log: 'log file',
  gi_warn_log: 'Log files do not belong in Git: keep the Logs patterns checked and remove it from the repository with git rm --cached {path}.', gi_reason_protected: 'matches a protected glob',
  gi_warn_advice: 'Commit a template {tmpl} instead (WRM fills it on install), add {path} to .gitignore and remove it from the repository with git rm --cached {path}.',
  gi_extra: 'Files only on servers (state extra, from the latest checks)', gi_extra_none: 'No extra files in the latest checks of this service.', gi_size: 'Size', gi_pattern: 'Pattern',
  gi_stacks: 'Standard patterns', gi_detected: 'found: {f}', gi_protected: 'Protected globs (per-host files)', gi_custom: 'More patterns', gi_custom_h: 'One per line, e.g. /data/ or *.local.ini',
  gi_lists: 'Catalog lists of this service', gi_lists_h: 'Exclude: never compared or deployed. Protected: per-host files, never overwritten.', gi_not_catalog: 'This service comes from a bundle: add it to the catalog to edit its lists.',
  gi_preview: 'Preview', gi_download: 'Download .gitignore', gi_mr: 'Create merge request…', gi_mr_gh: 'Create pull request…', gi_nothing: 'Nothing to add: the .gitignore already has these patterns.',
  gi_added: '{n} patterns added', gi_diff: 'Current → new', gi_mr_title: 'Merge request to {repo}', gi_mr_q: 'WRM creates the branch {branch}, commits {file} with these patterns and opens a merge request against {base} in:',
  gi_mr_type: 'Type the repository name {name} to confirm', gi_mr_go: 'Create', gi_mr_done: 'Created: {url}', gi_mr_default: 'The default output is a download; WRM writes to the Git server only when you choose this.',
  gi_note: 'Note',
  git_write_token: 'Write token (optional)', git_write_token_h: 'Only for .gitignore merge / pull requests. Leave empty to keep the stored one.',
  git_webhook: 'Webhook…', git_wh_title: 'Incoming webhook: {name}', git_wh_h: 'A push, tag or release in GitLab / GitHub, or a POST from a CI job, starts an immediate check of the affected services instead of waiting for the interval.',
  git_wh_off: 'The webhook is off.', git_wh_enable: 'Turn on', git_wh_renew: 'New secret', git_wh_disable: 'Turn off', git_wh_url: 'URL', git_wh_secret: 'Secret (shown only now: copy it)',
  git_wh_last: 'Last delivery', git_wh_gitlab: 'GitLab: Settings → Webhooks, the URL, the secret as "Secret token", events "Push" and "Tag push".',
  git_wh_github: 'GitHub: Settings → Webhooks, the URL, content type application/json, the secret, events "push" and "release".',
  git_wh_generic: 'Jenkins or any CI: POST with the header X-WRM-Token: <secret>, optionally a JSON body {"project": "group/name"} or {"services": ["name"]}.',
  git_feeds: 'Bundles from CI artifacts', git_feeds_h: 'Fetch the newest bundle from a URL (GitLab job artifacts API, Jenkins artifact URL or any HTTPS URL) and use it as the offline source.',
  git_add_feed: 'Add artifact URL', git_feed_url: 'URL of the bundle (.tar.gz)', git_feed_header: 'Auth header name', git_feed_value: 'Auth header value', git_feed_value_h: 'PRIVATE-TOKEN: <token> for GitLab, Authorization: Basic user:token for Jenkins. Leave empty to keep the stored value.',
  git_feed_interval: 'Fetch every (minutes, 0 = only by hand)', git_feed_fetch: 'Fetch now', git_feed_imported: 'Bundle imported', git_feed_unchanged: 'Unchanged since the last fetch', git_feed_last: 'Last fetched',
  git_deploy_method: 'Deploy method', git_dm_files: 'Files over SSH (WRM writes the files)', git_dm_jenkins: 'CI pipeline: Jenkins job', git_dm_gitlab: 'CI pipeline: GitLab pipeline',
  git_dm_h: 'For services built elsewhere: an update or upgrade triggers the job with the parameters server, install_path and version, and WRM follows its status.',
  git_dm_job: 'Job URL', git_dm_user: 'Jenkins user', git_dm_token: 'API token (or the job\'s trigger token without a user)', git_dm_gl_url: 'GitLab address (empty = the Git source)', git_dm_project: 'Project (empty = the service\'s project)',
  git_dm_ref: 'Branch to run (empty = the target\'s branch)', git_dm_trigger: 'Pipeline trigger token', git_dm_keep: 'leave empty to keep the stored token',
  git_ci_item: 'Deployed by CI ({kind}): WRM triggers the job with server, install_path and version and follows its status; no files are written.', git_ci_link: 'Open in CI', git_step_ci: 'CI pipeline',
});
Object.assign(LANGS.hr, {
  git_tab_gitignore: '.gitignore', gi_service: 'Servis', gi_load: 'Učitaj', gi_loading: 'Čitanje repozitorija i zadnjih provjera…',
  gi_repo: 'Repozitorij', gi_file: 'Datoteka', gi_branch: 'Grana', gi_current: 'Trenutni .gitignore', gi_no_current: 'Repozitorij ovdje još nema .gitignore.',
  gi_warnings: 'Datoteke u repozitoriju koje ne pripadaju Gitu (tajne, konfiguracija po hostu, logovi)', gi_reason_secret: 'izgleda kao tajna', gi_reason_log: 'log datoteka',
  gi_warn_log: 'Log datoteke ne pripadaju Gitu: ostavite uzorke Logs označenima i uklonite je iz repozitorija s git rm --cached {path}.', gi_reason_protected: 'odgovara zaštićenom globu',
  gi_warn_advice: 'Umjesto nje spremite predložak {tmpl} (WRM ga popunjava pri instalaciji), dodajte {path} u .gitignore i uklonite je iz repozitorija s git rm --cached {path}.',
  gi_extra: 'Datoteke samo na serverima (stanje višak, iz zadnjih provjera)', gi_extra_none: 'U zadnjim provjerama ovog servisa nema viška datoteka.', gi_size: 'Veličina', gi_pattern: 'Uzorak',
  gi_stacks: 'Standardni uzorci', gi_detected: 'pronađeno: {f}', gi_protected: 'Zaštićeni globovi (datoteke po hostu)', gi_custom: 'Dodatni uzorci', gi_custom_h: 'Jedan po retku, npr. /data/ ili *.local.ini',
  gi_lists: 'Popisi ovog servisa u katalogu', gi_lists_h: 'Isključi: nikad se ne uspoređuje ni ne isporučuje. Zaštićeno: datoteke po hostu, nikad se ne prepisuju.', gi_not_catalog: 'Ovaj servis dolazi iz bundlea: dodajte ga u katalog da biste uređivali njegove popise.',
  gi_preview: 'Pregled', gi_download: 'Preuzmi .gitignore', gi_mr: 'Izradi merge request…', gi_mr_gh: 'Izradi pull request…', gi_nothing: 'Nema ničega za dodati: .gitignore već ima ove uzorke.',
  gi_added: 'dodano uzoraka: {n}', gi_diff: 'Trenutno → novo', gi_mr_title: 'Merge request u {repo}', gi_mr_q: 'WRM izrađuje granu {branch}, sprema {file} s ovim uzorcima i otvara merge request prema {base} u:',
  gi_mr_type: 'Upišite naziv repozitorija {name} za potvrdu', gi_mr_go: 'Izradi', gi_mr_done: 'Izrađeno: {url}', gi_mr_default: 'Zadani izlaz je preuzimanje; WRM piše na Git server samo kad odaberete ovo.',
  gi_note: 'Napomena',
  git_write_token: 'Token za pisanje (neobavezno)', git_write_token_h: 'Samo za merge / pull requestove za .gitignore. Ostavite prazno za zadržavanje spremljenog.',
  git_webhook: 'Webhook…', git_wh_title: 'Dolazni webhook: {name}', git_wh_h: 'Push, tag ili release u GitLabu / GitHubu, ili POST iz CI posla, odmah pokreće provjeru zahvaćenih servisa umjesto čekanja intervala.',
  git_wh_off: 'Webhook je isključen.', git_wh_enable: 'Uključi', git_wh_renew: 'Nova tajna', git_wh_disable: 'Isključi', git_wh_url: 'URL', git_wh_secret: 'Tajna (prikazuje se samo sada: kopirajte je)',
  git_wh_last: 'Zadnja isporuka', git_wh_gitlab: 'GitLab: Settings → Webhooks, URL, tajna kao "Secret token", događaji "Push" i "Tag push".',
  git_wh_github: 'GitHub: Settings → Webhooks, URL, content type application/json, tajna, događaji "push" i "release".',
  git_wh_generic: 'Jenkins ili bilo koji CI: POST sa zaglavljem X-WRM-Token: <tajna>, neobavezno JSON tijelo {"project": "grupa/naziv"} ili {"services": ["naziv"]}.',
  git_feeds: 'Bundleovi iz CI artefakata', git_feeds_h: 'Dohvati najnoviji bundle s URL-a (GitLab API za artefakte posla, URL artefakta u Jenkinsu ili bilo koji HTTPS URL) i koristi ga kao offline izvor.',
  git_add_feed: 'Dodaj URL artefakta', git_feed_url: 'URL bundlea (.tar.gz)', git_feed_header: 'Naziv zaglavlja za prijavu', git_feed_value: 'Vrijednost zaglavlja za prijavu', git_feed_value_h: 'PRIVATE-TOKEN: <token> za GitLab, Authorization: Basic korisnik:token za Jenkins. Ostavite prazno za zadržavanje spremljene vrijednosti.',
  git_feed_interval: 'Dohvaćaj svakih (minuta, 0 = samo ručno)', git_feed_fetch: 'Dohvati sada', git_feed_imported: 'Bundle uvezen', git_feed_unchanged: 'Nepromijenjeno od zadnjeg dohvata', git_feed_last: 'Zadnji dohvat',
  git_deploy_method: 'Način isporuke', git_dm_files: 'Datoteke preko SSH-a (WRM zapisuje datoteke)', git_dm_jenkins: 'CI pipeline: Jenkins posao', git_dm_gitlab: 'CI pipeline: GitLab pipeline',
  git_dm_h: 'Za servise koji se grade drugdje: ažuriranje ili nadogradnja pokreće posao s parametrima server, install_path i version, a WRM prati njegovo stanje.',
  git_dm_job: 'URL posla', git_dm_user: 'Jenkins korisnik', git_dm_token: 'API token (ili token za okidanje posla bez korisnika)', git_dm_gl_url: 'Adresa GitLaba (prazno = Git izvor)', git_dm_project: 'Projekt (prazno = projekt servisa)',
  git_dm_ref: 'Grana za pokretanje (prazno = grana cilja)', git_dm_trigger: 'Token za okidanje pipelinea', git_dm_keep: 'ostavite prazno za zadržavanje spremljenog tokena',
  git_ci_item: 'Isporučuje CI ({kind}): WRM pokreće posao s parametrima server, install_path i version i prati njegovo stanje; datoteke se ne zapisuju.', git_ci_link: 'Otvori u CI-ju', git_step_ci: 'CI pipeline',
});

Object.assign(LANGS.en, {
  git_all_services: 'All services', git_all_servers: 'All servers', git_search_srv: 'Search server, host, path…',
  git_check_selected: 'Check selected ({n})', git_check_visible: 'Check visible ({n})', git_check_stale: 'Check only stale', git_check_cell: 'Check this cell',
  git_stale_never: 'never checked', git_stale_60: 'older than 1 hour', git_stale_1440: 'older than 24 hours', git_stale_10080: 'older than 7 days',
  git_sel_row: 'Select every installation of this server', git_sel_col: 'Select every installation of this service', git_sel_all: 'Select every visible installation',
  git_clear_sel: 'Clear selection', git_cancel_check: 'Cancel', git_job_progress: 'Checking {done}/{total}', git_job_targets: 'Refreshing targets…',
  git_job_done: 'Check finished: {n} installations.', git_job_cancelled: 'Check cancelled.', git_job_errors: '{n} could not be checked.', git_nothing_stale: 'No visible installation is that old.',
  git_q_queued: 'queued', git_q_running: 'checking…', git_checked_ago: 'checked {t}', git_ago_now: 'just now', git_ago_min: '{n} min ago', git_ago_h: '{n} h ago', git_ago_d: '{n} d ago',
  git_last_good: 'last good: {s}, {t}',
  git_files_all: 'Files in the folder and in Git', git_ff_all: 'All', git_ff_diff: 'Differing', git_ff_tracked: 'In the check', git_ff_untracked: 'Outside the check', git_files_search: 'Search files…',
  git_files_loading: 'Reading the folder on the server…', git_files_count: '{n} of {total}', git_files_truncated: 'The folder has more than {n} entries: only the first {n} are listed.',
  git_files_partial: 'Only the catalog\'s files of the repository are known: refresh the targets to see the other repository files too.',
  git_files_notcmp: '{n} repository files outside the catalog were not compared yet (Diff compares them).', git_files_more: 'Showing the first {n} rows: narrow the list with the filters.',
  git_files_failed: 'The folder could not be read: {e}', git_files_stored: 'Files of the last check', git_files_info: 'The check compares only the files the catalog tracks (and the protected ones). Files outside the check, such as logs or files not in the include list, are only listed: they are not compared and never change the state of the installation.',
  git_size: 'Size', git_mtime: 'Modified', git_reason: 'Why', git_open: 'Open',
  git_fs_same: 'same', git_fs_differs: 'differs', git_fs_unknown: 'not compared', git_fs_symlink: 'symlink', git_fs_unreadable: 'unreadable',
  git_r_tracked: 'tracked', git_r_excluded: 'excluded by the catalog', git_r_excluded_p: 'excluded by the catalog ({p})', git_r_not_included: 'not in the include list',
  git_r_protected: 'protected', git_r_protected_p: 'protected ({p})', git_r_extra: 'not in Git', git_r_tool: 'written by the deploy tool', git_r_compiled: 'compiled file',
  git_r_not_tracked: 'in Git, not tracked', git_r_logs: 'log file', git_fs_outside: 'outside the check',
  git_where_both: 'in Git and on the server', git_where_git: 'only in Git', git_where_server: 'only on the server', git_r_no_target: 'no target yet', git_link_outside: 'points out of the installation (not followed)', git_dir_unreadable: 'directory: permission denied',
  git_informational: 'Excluded by the catalog — informational, not updated.', git_info_untracked: 'Not tracked by the catalog — informational, not updated.',
  git_view_unified: 'Unified', git_view_split: 'Side by side', git_from_server: 'Server', git_from_git: 'Git',
  git_binary_same: 'Binary file: the same on the server and in Git (by hash).', git_binary_differs: 'Binary file: differs between the server and Git (by hash).',
  git_big_same: 'The file is too large to show: the same on the server and in Git (by hash).', git_big_differs: 'The file is too large to show: it differs between the server and Git (by hash).',
  git_missing_srv: 'The file is not on the server: everything below would be added.', git_view_binary: 'Binary file, {s}, SHA-256 {h}', git_view_big: 'The file is too large to show ({s}), SHA-256 {h}',
  git_crlf: 'Line endings are ignored (CRLF = LF).', git_view_side: 'Open from', git_target_err: 'Target: {e}',
  git_commits: 'Commits', git_commits_behind: 'Commits behind: {n}', git_commits_none: 'The server is at the target commit.', git_commits_loading: 'Loading commits…',
  git_commits_range: '{f} → {t}', git_commits_more: 'The newest {n} are shown.',
});
Object.assign(LANGS.hr, {
  git_all_services: 'Svi servisi', git_all_servers: 'Svi serveri', git_search_srv: 'Traži server, host, putanju…',
  git_check_selected: 'Provjeri odabrane ({n})', git_check_visible: 'Provjeri prikazane ({n})', git_check_stale: 'Provjeri samo zastarjele', git_check_cell: 'Provjeri ovu ćeliju',
  git_stale_never: 'nikad provjerene', git_stale_60: 'starije od 1 sata', git_stale_1440: 'starije od 24 sata', git_stale_10080: 'starije od 7 dana',
  git_sel_row: 'Odaberi sve instalacije ovog servera', git_sel_col: 'Odaberi sve instalacije ovog servisa', git_sel_all: 'Odaberi sve prikazane instalacije',
  git_clear_sel: 'Poništi odabir', git_cancel_check: 'Prekini', git_job_progress: 'Provjera {done}/{total}', git_job_targets: 'Osvježavanje ciljeva…',
  git_job_done: 'Provjera završena: {n} instalacija.', git_job_cancelled: 'Provjera prekinuta.', git_job_errors: 'Nije moguće provjeriti: {n}.', git_nothing_stale: 'Nijedna prikazana instalacija nije toliko stara.',
  git_q_queued: 'na čekanju', git_q_running: 'provjera…', git_checked_ago: 'provjereno {t}', git_ago_now: 'upravo', git_ago_min: 'prije {n} min', git_ago_h: 'prije {n} h', git_ago_d: 'prije {n} d',
  git_last_good: 'zadnje uspješno: {s}, {t}',
  git_files_all: 'Datoteke u direktoriju i u Gitu', git_ff_all: 'Sve', git_ff_diff: 'Različite', git_ff_tracked: 'U provjeri', git_ff_untracked: 'Izvan provjere', git_files_search: 'Traži datoteke…',
  git_files_loading: 'Čitanje direktorija na serveru…', git_files_count: '{n} od {total}', git_files_truncated: 'Direktorij ima više od {n} stavki: prikazano je samo prvih {n}.',
  git_files_partial: 'Poznate su samo datoteke repozitorija iz kataloga: osvježite ciljeve da biste vidjeli i ostale datoteke repozitorija.',
  git_files_notcmp: 'Datoteke repozitorija izvan kataloga koje još nisu uspoređene: {n} (usporedite ih gumbom Razlike).', git_files_more: 'Prikazano je prvih {n} redaka: suzite popis filtrima.',
  git_files_failed: 'Direktorij nije moguće pročitati: {e}', git_files_stored: 'Datoteke zadnje provjere', git_files_info: 'Provjera uspoređuje samo datoteke koje katalog prati (i zaštićene). Datoteke izvan provjere, poput logova ili datoteka koje nisu na popisu uključenih, samo se navode: ne uspoređuju se i nikad ne mijenjaju stanje instalacije.',
  git_size: 'Veličina', git_mtime: 'Izmijenjeno', git_reason: 'Zašto', git_open: 'Otvori',
  git_fs_same: 'isto', git_fs_differs: 'razlikuje se', git_fs_unknown: 'nije uspoređeno', git_fs_symlink: 'simbolička veza', git_fs_unreadable: 'nečitljivo',
  git_r_tracked: 'praćeno', git_r_excluded: 'isključeno katalogom', git_r_excluded_p: 'isključeno katalogom ({p})', git_r_not_included: 'nije na popisu uključenih',
  git_r_protected: 'zaštićeno', git_r_protected_p: 'zaštićeno ({p})', git_r_extra: 'nije u Gitu', git_r_tool: 'zapisuje alat za isporuku', git_r_compiled: 'prevedena datoteka',
  git_r_not_tracked: 'u Gitu, nije praćeno', git_r_logs: 'log datoteka', git_fs_outside: 'izvan provjere',
  git_where_both: 'u Gitu i na serveru', git_where_git: 'samo u Gitu', git_where_server: 'samo na serveru', git_r_no_target: 'još nema cilja', git_link_outside: 'vodi izvan instalacije (ne slijedi se)', git_dir_unreadable: 'direktorij: pristup odbijen',
  git_informational: 'Isključeno katalogom — informativno, ne ažurira se.', git_info_untracked: 'Katalog ovo ne prati — informativno, ne ažurira se.',
  git_view_unified: 'Objedinjeno', git_view_split: 'Usporedno', git_from_server: 'Server', git_from_git: 'Git',
  git_binary_same: 'Binarna datoteka: ista na serveru i u Gitu (po hashu).', git_binary_differs: 'Binarna datoteka: razlikuje se između servera i Gita (po hashu).',
  git_big_same: 'Datoteka je prevelika za prikaz: ista na serveru i u Gitu (po hashu).', git_big_differs: 'Datoteka je prevelika za prikaz: razlikuje se između servera i Gita (po hashu).',
  git_missing_srv: 'Datoteke nema na serveru: sve ispod bilo bi dodano.', git_view_binary: 'Binarna datoteka, {s}, SHA-256 {h}', git_view_big: 'Datoteka je prevelika za prikaz ({s}), SHA-256 {h}',
  git_crlf: 'Završeci redaka se zanemaruju (CRLF = LF).', git_view_side: 'Otvori iz', git_target_err: 'Cilj: {e}',
  git_commits: 'Commitovi', git_commits_behind: 'Commitova iza cilja: {n}', git_commits_none: 'Server je na commitu cilja.', git_commits_loading: 'Učitavanje commitova…',
  git_commits_range: '{f} → {t}', git_commits_more: 'Prikazano je najnovijih {n}.',
});

// v11.5.0: environments, deploy plans and safe deploys
Object.assign(LANGS.en, {
  git_tab_envs: 'Environments', genv_lazy_h: 'Every cell is read from its servers when it is shown.',
  genv_none: 'No service has environments yet. Add them with ✎ below: ordered destinations (server:/path) and the rules of each environment.',
  genv_services: 'Environments per service', genv_services_h: 'Services without environments keep working with Update, Upgrade and Rollback as before.',
  genv_edit: 'Environments', genv_deploy: 'Deploy', genv_history: 'History', genv_doctor: 'Doctor', genv_n_dests: '{n} destination(s)…',
  genv_drift: 'The servers of this environment differ', genv_not_deployed: 'not deployed yet', genv_no_state: 'no deploy state recorded yet',
  genv_locked_by: 'locked by {user} from {from}', genv_stale: 'stale',
  genv_edit_title: 'Environments of {app}', genv_edit_h: 'Destinations are deployed in this order; a rollback goes backwards. The server is the name of an SSH connection.',
  genv_none_app: 'No environments yet.', genv_add: 'Add environment', genv_remove: 'Remove', genv_name: 'Name', genv_keep: 'Backups to keep',
  genv_state_dir: 'State directory', genv_state_dir_ph: 'empty = <path>/.deploy-bak', genv_dests: 'Destinations (server:/path, in order)',
  genv_dests_h: 'One per line, e.g. app-01:/opt/demo-api', genv_allowed: 'Allowed refs (empty = any)', genv_ignore: 'Extra ignore patterns',
  genv_post: 'post_deploy command', genv_post_h: 'Runs in the app directory after the files, only when you tick it for a deploy or rollback.',
  genv_confirm: 'Ask to type the environment name before a deploy',
  genv_plan_title: 'Deploy {app} → {env}', genv_comparing: 'Comparing every destination with the repository…', genv_ref: 'Ref',
  genv_ref_ph: 'empty = the service target; tag:v2 or a branch', genv_compare: 'Compare',
  genv_fs_new: 'new', genv_fs_changed: 'changed', genv_fs_eol: 'same (CRLF/LF)', genv_fs_same: 'same', genv_fs_removed: 'removed from Git',
  genv_fs_extra: 'only on the server', genv_fs_protected: 'protected', genv_fs_ignored: 'ignored', genv_symlink: 'symlink on the server',
  genv_excluded_by: 'excluded by {by} · {at}', genv_include_h: 'Untick to leave this file out of this deploy (remembered for the next compare)',
  genv_excl_svc: 'service', genv_excl_env: 'environment', genv_excl_svc_h: 'Exclude permanently for the whole service (catalog exclude)',
  genv_excl_env_h: 'Exclude permanently in this environment only', genv_excl_q: 'Exclude {path} permanently ({scope})? An anchored pattern is written and the plan is compared again.',
  genv_excl_added: 'Pattern added: {p}', genv_excl_covered: 'Already covered by {p}', genv_current: 'current: {v} ({id}) · {by} · {at}',
  genv_dir_to_file: 'The branch turns the directory {path} into a file; these server files must be deleted:',
  genv_file_to_dir: 'The branch turns the file {path} into a directory; it must be deleted:',
  genv_unlock: 'Remove the stale lock',
  genv_unlock_q: 'Remove the lock of {user} from {from} (deploy {id})? Only do this when no deploy is running there. The server checks again that it is the same, stale lock.',
  genv_unlocked: 'The lock was removed', genv_options: 'Options', genv_n_excluded: '{n} file(s) left out of this deploy',
  genv_clear_excl: 'Clear remembered exclusions', genv_delete_removed: 'Delete files removed from the repository ({n})', genv_run_post: 'Run post_deploy:',
  genv_no_post: 'This environment has no post_deploy command.', genv_restart: 'Restart the units of each destination after its files (now)',
  genv_dry_run: 'Dry run', genv_deploy_go: 'Deploy…', genv_plan_used: 'This plan was applied already: compare again.',
  genv_confirm_title: 'Deploy {app} {v} to {env}?', genv_confirm_list: 'Destinations, in this order:', genv_type_env: 'Type the environment name {env} to confirm',
  genv_back: 'Back to the plan', genv_history_title: 'History of {app} · {env}', genv_when: 'When', genv_action: 'Action', genv_id: 'Deploy id', genv_who: 'Who',
  genv_version: 'Version', genv_previous: 'Previous', genv_servers: 'Destination', genv_files: 'Files', genv_history_empty: 'No deploys are recorded on these servers yet.',
  genv_l_new: 'New', genv_l_changed: 'Changed', genv_l_deleted: 'Deleted', genv_l_excluded: 'Excluded', genv_l_restored: 'Restored', genv_l_moved_aside: 'Moved aside',
  genv_l_skipped: 'Skipped (symlinks)', genv_l_moved: 'Arrived before the interruption', genv_rollback: 'Roll back…', genv_rb_title: 'Roll back deploy {id}',
  genv_rb_h: 'Destinations are rolled back in reverse order. Files the deploy added and the current versions are moved to backups/rollback-<id>/ in the state directory; nothing is deleted.',
  genv_rb_latest: 'latest deploy here', genv_rb_not_latest: 'not the latest (current: {id})', genv_rb_go: 'Roll back…', genv_rb_confirm: 'Roll back {id} on {n} destination(s)?',
  genv_rb_none: 'Choose at least one destination.', genv_doctor_title: 'Access doctor: {app} · {env}', genv_doctor_running: 'Checking every destination…',
  genv_chk_ssh: 'SSH', genv_chk_shell: 'Shell', genv_chk_user: 'User', genv_chk_path: 'App directory', genv_chk_state: 'State directory', genv_chk_unreadable: 'Subdirectories',
  genv_chk_method: 'Deploy method', genv_chk_webroot: 'Web root', genv_chk_sha256sum: 'sha256sum', genv_chk_rsync: 'rsync', genv_chk_tar: 'tar', genv_chk_lock: 'Lock',
  genv_dry: 'dry run', genv_partial: 'Interrupted transfer: {n} file(s) had arrived (removed again); nothing was changed.', genv_skipped: 'Skipped (symlinks on the server)',
  genv_result: 'Result', genv_blocked: 'Fix the errors above first.',
  git_step_lock: 'Lock', git_step_unlock: 'Unlock', git_step_plan: 'Plan', git_step_dry_run: 'Dry run', git_step_transfer: 'Transfer', git_step_state: 'State',
  git_step_history: 'History', git_step_post_deploy: 'post_deploy', git_step_restore: 'Restore',
});
Object.assign(LANGS.hr, {
  git_tab_envs: 'Okruženja', genv_lazy_h: 'Svaka se ćelija čita sa svojih servera kad se prikaže.',
  genv_none: 'Nijedan servis još nema okruženja. Dodajte ih gumbom ✎ ispod: poredana odredišta (server:/putanja) i pravila svakog okruženja.',
  genv_services: 'Okruženja po servisu', genv_services_h: 'Servisi bez okruženja i dalje rade s Ažuriraj, Nadogradi i Vrati kao prije.',
  genv_edit: 'Okruženja', genv_deploy: 'Isporuka', genv_history: 'Povijest', genv_doctor: 'Dijagnostika', genv_n_dests: 'odredišta: {n}…',
  genv_drift: 'Serveri ovog okruženja se razlikuju', genv_not_deployed: 'još nije isporučeno', genv_no_state: 'još nema zapisanog stanja isporuke',
  genv_locked_by: 'zaključao {user} s {from}', genv_stale: 'zastarjelo',
  genv_edit_title: 'Okruženja servisa {app}', genv_edit_h: 'Odredišta se isporučuju ovim redom; vraćanje ide unatrag. Server je naziv SSH veze.',
  genv_none_app: 'Još nema okruženja.', genv_add: 'Dodaj okruženje', genv_remove: 'Ukloni', genv_name: 'Naziv', genv_keep: 'Broj sigurnosnih kopija',
  genv_state_dir: 'Direktorij stanja', genv_state_dir_ph: 'prazno = <putanja>/.deploy-bak', genv_dests: 'Odredišta (server:/putanja, redom)',
  genv_dests_h: 'Jedno po retku, npr. app-01:/opt/demo-api', genv_allowed: 'Dopušteni refovi (prazno = svi)', genv_ignore: 'Dodatni uzorci za ignoriranje',
  genv_post: 'Naredba post_deploy', genv_post_h: 'Pokreće se u direktoriju aplikacije nakon datoteka, samo kad je označite za isporuku ili vraćanje.',
  genv_confirm: 'Traži upis naziva okruženja prije isporuke',
  genv_plan_title: 'Isporuka {app} → {env}', genv_comparing: 'Usporedba svakog odredišta s repozitorijem…', genv_ref: 'Ref',
  genv_ref_ph: 'prazno = cilj servisa; tag:v2 ili grana', genv_compare: 'Usporedi',
  genv_fs_new: 'nova', genv_fs_changed: 'izmijenjena', genv_fs_eol: 'ista (CRLF/LF)', genv_fs_same: 'ista', genv_fs_removed: 'uklonjena iz Gita',
  genv_fs_extra: 'samo na serveru', genv_fs_protected: 'zaštićena', genv_fs_ignored: 'ignorirana', genv_symlink: 'simbolička veza na serveru',
  genv_excluded_by: 'izostavio {by} · {at}', genv_include_h: 'Odznačite da datoteka ne ide u ovu isporuku (pamti se za sljedeću usporedbu)',
  genv_excl_svc: 'servis', genv_excl_env: 'okruženje', genv_excl_svc_h: 'Trajno izostavi za cijeli servis (exclude u katalogu)',
  genv_excl_env_h: 'Trajno izostavi samo u ovom okruženju', genv_excl_q: 'Trajno izostaviti {path} ({scope})? Upisuje se usidreni uzorak i plan se ponovno uspoređuje.',
  genv_excl_added: 'Dodan uzorak: {p}', genv_excl_covered: 'Već obuhvaćeno uzorkom {p}', genv_current: 'trenutno: {v} ({id}) · {by} · {at}',
  genv_dir_to_file: 'Grana pretvara direktorij {path} u datoteku; ove datoteke na serveru moraju se obrisati:',
  genv_file_to_dir: 'Grana pretvara datoteku {path} u direktorij; mora se obrisati:',
  genv_unlock: 'Ukloni zastarjelu bravu',
  genv_unlock_q: 'Ukloniti bravu korisnika {user} s {from} (isporuka {id})? Učinite to samo ako ondje ne teče nijedna isporuka. Server ponovno provjerava da je to ista, zastarjela brava.',
  genv_unlocked: 'Brava je uklonjena', genv_options: 'Opcije', genv_n_excluded: 'Izostavljenih datoteka u ovoj isporuci: {n}',
  genv_clear_excl: 'Očisti zapamćena izostavljanja', genv_delete_removed: 'Obriši datoteke uklonjene iz repozitorija ({n})', genv_run_post: 'Pokreni post_deploy:',
  genv_no_post: 'Ovo okruženje nema naredbu post_deploy.', genv_restart: 'Ponovno pokreni jedinice svakog odredišta nakon njegovih datoteka (odmah)',
  genv_dry_run: 'Probno izvođenje', genv_deploy_go: 'Isporuči…', genv_plan_used: 'Ovaj je plan već primijenjen: ponovno usporedite.',
  genv_confirm_title: 'Isporučiti {app} {v} u {env}?', genv_confirm_list: 'Odredišta, ovim redom:', genv_type_env: 'Za potvrdu upišite naziv okruženja {env}',
  genv_back: 'Natrag na plan', genv_history_title: 'Povijest: {app} · {env}', genv_when: 'Kada', genv_action: 'Radnja', genv_id: 'Oznaka isporuke', genv_who: 'Tko',
  genv_version: 'Verzija', genv_previous: 'Prethodno', genv_servers: 'Odredište', genv_files: 'Datoteke', genv_history_empty: 'Na ovim serverima još nema zabilježenih isporuka.',
  genv_l_new: 'Nove', genv_l_changed: 'Izmijenjene', genv_l_deleted: 'Obrisane', genv_l_excluded: 'Izostavljene', genv_l_restored: 'Vraćene', genv_l_moved_aside: 'Premještene u stranu',
  genv_l_skipped: 'Preskočene (simboličke veze)', genv_l_moved: 'Stigle prije prekida', genv_rollback: 'Vrati…', genv_rb_title: 'Vraćanje isporuke {id}',
  genv_rb_h: 'Odredišta se vraćaju obrnutim redom. Datoteke koje je isporuka dodala i trenutne verzije premještaju se u backups/rollback-<id>/ u direktoriju stanja; ništa se ne briše.',
  genv_rb_latest: 'ovdje zadnja isporuka', genv_rb_not_latest: 'nije zadnja (trenutna: {id})', genv_rb_go: 'Vrati…', genv_rb_confirm: 'Vratiti {id} na odredištima: {n}?',
  genv_rb_none: 'Odaberite barem jedno odredište.', genv_doctor_title: 'Dijagnostika pristupa: {app} · {env}', genv_doctor_running: 'Provjera svakog odredišta…',
  genv_chk_ssh: 'SSH', genv_chk_shell: 'Ljuska', genv_chk_user: 'Korisnik', genv_chk_path: 'Direktorij aplikacije', genv_chk_state: 'Direktorij stanja', genv_chk_unreadable: 'Poddirektoriji',
  genv_chk_method: 'Način isporuke', genv_chk_webroot: 'Web korijen', genv_chk_sha256sum: 'sha256sum', genv_chk_rsync: 'rsync', genv_chk_tar: 'tar', genv_chk_lock: 'Brava',
  genv_dry: 'probno', genv_partial: 'Prekinut prijenos: stiglo je datoteka: {n} (ponovno uklonjene); ništa nije promijenjeno.', genv_skipped: 'Preskočeno (simboličke veze na serveru)',
  genv_result: 'Rezultat', genv_blocked: 'Najprije ispravite pogreške iznad.',
  git_step_lock: 'Zaključavanje', git_step_unlock: 'Otključavanje', git_step_plan: 'Plan', git_step_dry_run: 'Probno izvođenje', git_step_transfer: 'Prijenos', git_step_state: 'Stanje',
  git_step_history: 'Povijest', git_step_post_deploy: 'post_deploy', git_step_restore: 'Vraćanje datoteka',
});

Object.assign(LANGS.en, {
  git_tab_activity: 'Activity', gact_v_git: 'Git activity', gact_v_history: 'Deploy history', gact_v_where: 'What is where',
  gact_v_git_h: 'Pushes and commits per branch and author, from the Git server.',
  gact_v_history_h: 'Deploys and rollbacks of every server, merged with the WRM runs of installations without environments.',
  gact_v_where_h: 'Branch, commit and version per environment and server, and how far each is behind its branch.',
  gact_csv_h: 'Download the current view with its filters as CSV', gact_from: 'From', gact_to: 'To', gact_tracked: 'Service and environment branches',
  gact_author_ph: 'Author (name, user, e-mail)', gact_loading: 'Reading the Git server…', gact_scope: '{p} on {src} · commits of: {b}',
  gact_fetched: 'read {t}', gact_cached: 'from the cache (↻ reads again)', gact_rate: 'The Git server\'s API rate limit was reached: the list is incomplete. Try again later.',
  gact_rate_until: 'The Git server\'s API rate limit was reached: the list is incomplete. Calls are possible again from {t}.',
  gact_truncated: 'Only the newest entries were read: narrow the dates or choose a branch.', gact_by_author: 'Per author', gact_by_branch: 'Per branch',
  gact_commits: 'Commits', gact_pushes: 'Pushes', gact_authors: 'Authors', gact_branches: 'Branches', gact_last: 'Last activity', gact_none: 'Nothing in this range.',
  gact_items: 'Pushes and commits ({n})', gact_kind: 'Kind', gact_branch: 'Branch', gact_author: 'Author', gact_change: 'Change',
  gact_k_commit: 'commit', gact_k_push: 'push', gact_n_commits: '{n} commit(s)', gact_more: '{n} more in the CSV export',
  gact_all_services: 'All services', gact_all_servers: 'All servers', gact_all_actions: 'All actions', gact_who_ph: 'Who', gact_branch_ph: 'Branch', gact_version_ph: 'Version or commit',
  gact_a_deploy: 'deploy', gact_a_rollback: 'rollback', gact_a_update: 'update', gact_a_upgrade: 'upgrade', gact_a_install: 'install', gact_a_transfer: 'transfer',
  gact_src_env: 'server history', gact_src_run: 'WRM run', gact_reading: 'Reading the servers: {n} of {m}',
  gact_no_checks: 'Checks are not allowed for your account: only what WRM stored is shown, not what the servers say.',
  gact_f_new: 'new', gact_f_changed: 'changed', gact_f_deleted: 'deleted', gact_f_excluded: 'excluded', gact_f_restored: 'restored', gact_f_moved_aside: 'moved aside',
  gact_f_skipped: 'skipped', gact_f_failed: 'failed', gact_f_moved: 'arrived before the break', gact_f_written: 'written',
  gact_at_head: 'at the head', gact_install: 'installation without environment', gact_drift: 'differs', gact_where_none: 'No environments or checked installations yet.',
  gact_env: 'Environment', gact_commit: 'Commit', gact_deployed: 'Deployed by', gact_behind: 'Behind the branch',
});
Object.assign(LANGS.hr, {
  git_tab_activity: 'Aktivnost', gact_v_git: 'Git aktivnost', gact_v_history: 'Povijest isporuka', gact_v_where: 'Što je gdje',
  gact_v_git_h: 'Push i commiti po granama i autorima, s Git servera.',
  gact_v_history_h: 'Isporuke i vraćanja svih servera, spojeni s WRM izvođenjima instalacija bez okruženja.',
  gact_v_where_h: 'Grana, commit i verzija po okruženju i serveru te koliko svaki zaostaje za svojom granom.',
  gact_csv_h: 'Preuzmi trenutni prikaz s njegovim filtrima kao CSV', gact_from: 'Od', gact_to: 'Do', gact_tracked: 'Grane servisa i okruženja',
  gact_author_ph: 'Autor (ime, korisnik, e-pošta)', gact_loading: 'Čitanje Git servera…', gact_scope: '{p} na {src} · commiti grana: {b}',
  gact_fetched: 'pročitano {t}', gact_cached: 'iz međuspremnika (↻ čita ponovno)', gact_rate: 'Dosegnuto je ograničenje API poziva Git servera: popis je nepotpun. Pokušajte kasnije.',
  gact_rate_until: 'Dosegnuto je ograničenje API poziva Git servera: popis je nepotpun. Pozivi su ponovno mogući od {t}.',
  gact_truncated: 'Pročitani su samo najnoviji zapisi: suzite datume ili odaberite granu.', gact_by_author: 'Po autoru', gact_by_branch: 'Po grani',
  gact_commits: 'Commiti', gact_pushes: 'Push', gact_authors: 'Autori', gact_branches: 'Grane', gact_last: 'Zadnja aktivnost', gact_none: 'Ništa u ovom razdoblju.',
  gact_items: 'Push i commiti ({n})', gact_kind: 'Vrsta', gact_branch: 'Grana', gact_author: 'Autor', gact_change: 'Promjena',
  gact_k_commit: 'commit', gact_k_push: 'push', gact_n_commits: 'commita: {n}', gact_more: 'još {n} u CSV izvozu',
  gact_all_services: 'Svi servisi', gact_all_servers: 'Svi serveri', gact_all_actions: 'Sve radnje', gact_who_ph: 'Tko', gact_branch_ph: 'Grana', gact_version_ph: 'Verzija ili commit',
  gact_a_deploy: 'isporuka', gact_a_rollback: 'vraćanje', gact_a_update: 'ažuriranje', gact_a_upgrade: 'nadogradnja', gact_a_install: 'instalacija', gact_a_transfer: 'prijenos',
  gact_src_env: 'povijest na serveru', gact_src_run: 'WRM izvođenje', gact_reading: 'Čitanje servera: {n} od {m}',
  gact_no_checks: 'Provjere nisu dopuštene za vaš račun: prikazano je samo ono što je WRM spremio, ne ono što kažu serveri.',
  gact_f_new: 'nove', gact_f_changed: 'izmijenjene', gact_f_deleted: 'obrisane', gact_f_excluded: 'izostavljene', gact_f_restored: 'vraćene', gact_f_moved_aside: 'premještene u stranu',
  gact_f_skipped: 'preskočene', gact_f_failed: 'neuspjele', gact_f_moved: 'stigle prije prekida', gact_f_written: 'zapisane',
  gact_at_head: 'na vrhu grane', gact_install: 'instalacija bez okruženja', gact_drift: 'razlikuje se', gact_where_none: 'Još nema okruženja ni provjerenih instalacija.',
  gact_env: 'Okruženje', gact_commit: 'Commit', gact_deployed: 'Isporučio', gact_behind: 'Zaostatak za granom',
});

// Simple mode of the Git workspace (v12.2.2)
Object.assign(LANGS.en, {
  gs_simple: 'Simple', gs_medium: 'Medium', gs_advanced: 'Advanced', gs_mode_h: 'Simple shows only deployment, versions, update, install and rollback. Advanced is the full Git workspace.',
  gs_intro: 'Pick an environment below. You see what runs on the server compared with the latest version on the branch, and you can install, update or roll back with one click. Everything else is in Advanced mode.',
  gs_none: 'Nothing found yet. Once a Git source is connected and your servers are reachable, services detected on the servers appear here automatically. Run a check to look for them.',
  gs_find: 'Find services on my servers',
  gs_auto_n: '{n} environment(s) were detected automatically from your servers.',
  gs_auto_h: 'They work as they are; nothing is deployed until you start it.',
  gs_auto_save: 'Save detected environments',
  gs_filter_service: 'All services', gs_filter_server: 'All servers', gs_filter_env: 'All environments', gs_filter_status: 'All statuses',
  gs_filter_branch: 'Branch contains…', gs_filter_version: 'Version or commit contains…', gs_filter_clear: 'Clear filters', gs_filter_none: 'No environments match these filters.',
  gs_checked: 'Compared', gs_files_preview: 'Files in this plan ({n})', gs_include_file: 'Include this file in the deployment', gs_file_status: 'Status by destination',
  gs_deploy_destinations: 'Deploy to selected servers', gs_destinations_selected: '{n} of {total} servers selected', gs_select_rollback: 'Select servers to roll back to this version',
  gs_removed_kept: 'kept on the server', gs_removed_h: 'Simple mode keeps files removed from the repository. Use Advanced mode to delete them after backup.',
  gs_auto_q: 'Save the detected environments to the catalog? Existing definitions are not changed.',
  gs_auto_done: '{n} environment(s) saved.',
  gs_on_server: 'On the server', gs_on_branch: 'Latest on the branch', gs_version: 'Version', gs_commit: 'Commit', gs_not_installed: 'Not installed here yet',
  gs_no_target: 'The latest version on the branch is not known yet. In Advanced mode open Overview and refresh the targets.',
  gs_status_ok: 'Up to date', gs_status_update: 'Update available', gs_status_missing: 'Not installed', gs_status_unknown: 'Cannot compare', gs_status_error: 'Problem', gs_status_review: 'Mixed / modified',
  gs_install: 'Install', gs_update: 'Update', gs_redeploy: 'Deploy again', gs_deploy: 'Deploy', gs_rollback: 'Roll back to the previous version', gs_refresh: 'Refresh',
  gs_differ: 'The servers in this environment run different versions.',
  gs_title_install: 'Install {app} on {env}', gs_title_update: 'Update {app} on {env}', gs_title_deploy: 'Deploy {app} to {env}',
  gs_from_to: 'Version on the server: {from} → will become: {to}', gs_files_n: 'Files to add: {n}, to change: {c}', gs_no_changes: 'The server already has exactly these files.',
  gs_backup_h: 'A backup is made before anything is changed, so you can roll back.', gs_other_version: 'Install a different version',
  gs_other_h: 'Leave empty for the latest. Type a tag (tag:v1.2) or a branch name.', gs_compare: 'Compare', gs_options: 'Options',
  gs_start_install: 'Install now', gs_start_update: 'Update now', gs_start_deploy: 'Deploy now', gs_confirm_q: '{action} {app} on {env} ({n} server(s))?',
  gs_blocked: 'This cannot be started until the problems above are solved (see Advanced mode).',
  gs_rb_title: 'Roll back {app} on {env}', gs_rb_none: 'There is nothing to roll back here: nothing was deployed through WRM yet, or the last deployment was already rolled back.',
  gs_rb_now: 'Now on the server', gs_rb_to: 'After the rollback', gs_rb_prev_unknown: 'the state before that deployment', gs_rb_go: 'Roll back now',
  gs_rb_q: 'Roll back {app} on {env} to {v}?', gs_rb_h: 'Files changed by the last deployment are restored from the backup.',
});
Object.assign(LANGS.hr, {
  gs_simple: 'Jednostavno', gs_medium: 'Srednje', gs_advanced: 'Napredno', gs_mode_h: 'Jednostavno prikazuje samo isporuku, verzije, nadogradnju, instalaciju i povratak. Napredno je cijeli Git radni prostor.',
  gs_intro: 'Odaberite okruženje ispod. Vidite što radi na serveru u usporedbi s najnovijom verzijom na grani i jednim klikom možete instalirati, nadograditi ili vratiti prethodnu verziju. Sve ostalo je u naprednom načinu.',
  gs_none: 'Još ništa nije pronađeno. Kad je Git izvor spojen i serveri dostupni, servisi pronađeni na serverima pojavljuju se ovdje automatski. Pokrenite provjeru da ih potražite.',
  gs_find: 'Pronađi servise na mojim serverima',
  gs_auto_n: '{n} okruženje(a) automatski je otkriveno na vašim serverima.',
  gs_auto_h: 'Rade kakva jesu; ništa se ne isporučuje dok to sami ne pokrenete.',
  gs_auto_save: 'Spremi otkrivena okruženja',
  gs_filter_service: 'Sve usluge', gs_filter_server: 'Svi serveri', gs_filter_env: 'Sva okruženja', gs_filter_status: 'Sva stanja',
  gs_filter_branch: 'Grana sadrži…', gs_filter_version: 'Verzija ili commit sadrži…', gs_filter_clear: 'Očisti filtre', gs_filter_none: 'Nijedno okruženje ne odgovara filtrima.',
  gs_checked: 'Uspoređeno', gs_files_preview: 'Datoteke u planu ({n})', gs_include_file: 'Uključi ovu datoteku u isporuku', gs_file_status: 'Stanje po odredištu',
  gs_deploy_destinations: 'Isporuči na odabrane servere', gs_destinations_selected: 'Odabrano servera: {n} od {total}', gs_select_rollback: 'Odaberite servere za povratak na ovu verziju',
  gs_removed_kept: 'ostaje na serveru', gs_removed_h: 'Simple način zadržava datoteke uklonjene iz repozitorija. Za njihovo brisanje uz backup koristite Advanced način.',
  gs_auto_q: 'Spremiti otkrivena okruženja u katalog? Postojeće definicije se ne mijenjaju.',
  gs_auto_done: '{n} okruženje(a) spremljeno.',
  gs_on_server: 'Na serveru', gs_on_branch: 'Najnovije na grani', gs_version: 'Verzija', gs_commit: 'Commit', gs_not_installed: 'Ovdje još nije instalirano',
  gs_no_target: 'Najnovija verzija na grani još nije poznata. U naprednom načinu otvorite Pregled i osvježite ciljeve.',
  gs_status_ok: 'Ažurno', gs_status_update: 'Dostupna nadogradnja', gs_status_missing: 'Nije instalirano', gs_status_unknown: 'Nije moguće usporediti', gs_status_error: 'Problem', gs_status_review: 'Miješano / izmijenjeno',
  gs_install: 'Instaliraj', gs_update: 'Nadogradi', gs_redeploy: 'Ponovno isporuči', gs_deploy: 'Isporuči', gs_rollback: 'Vrati prethodnu verziju', gs_refresh: 'Osvježi',
  gs_differ: 'Serveri u ovom okruženju imaju različite verzije.',
  gs_title_install: 'Instalacija {app} u {env}', gs_title_update: 'Nadogradnja {app} u {env}', gs_title_deploy: 'Isporuka {app} u {env}',
  gs_from_to: 'Verzija na serveru: {from} → postat će: {to}', gs_files_n: 'Datoteka za dodati: {n}, za izmijeniti: {c}', gs_no_changes: 'Server već ima upravo ove datoteke.',
  gs_backup_h: 'Prije ikakve promjene napravi se sigurnosna kopija, pa se možete vratiti.', gs_other_version: 'Instaliraj drugu verziju',
  gs_other_h: 'Ostavite prazno za najnoviju. Upišite oznaku (tag:v1.2) ili naziv grane.', gs_compare: 'Usporedi', gs_options: 'Opcije',
  gs_start_install: 'Instaliraj sada', gs_start_update: 'Nadogradi sada', gs_start_deploy: 'Isporuči sada', gs_confirm_q: '{action} {app} u {env} (servera: {n})?',
  gs_blocked: 'Ovo se ne može pokrenuti dok se gornji problemi ne riješe (vidi napredni način).',
  gs_rb_title: 'Povratak {app} u {env}', gs_rb_none: 'Ovdje nema što vratiti: preko WRM-a još ništa nije isporučeno ili je zadnja isporuka već vraćena.',
  gs_rb_now: 'Sada na serveru', gs_rb_to: 'Nakon povratka', gs_rb_prev_unknown: 'stanje prije te isporuke', gs_rb_go: 'Vrati sada',
  gs_rb_q: 'Vratiti {app} u {env} na {v}?', gs_rb_h: 'Datoteke koje je promijenila zadnja isporuka vraćaju se iz sigurnosne kopije.',
});

// Plain-language description under each tab of the Git workspace (v12.2.3)
Object.assign(LANGS.en, {
  gtd_overview: 'Compares what runs on each server with the latest version in the Git repository. Choose which version counts as the target, press "Check now" and read the colored states: up to date, needs update or review. Checking only reads files; nothing is changed.',
  gtd_services: 'The catalog of your applications: which Git project each one comes from, which branch or tag to follow and which files belong to it. Add a service by hand, suggest it from a repository, or import and export the whole catalog.',
  gtd_installs: 'Every copy of a service found on your servers. Discover finds them automatically, Add by hand registers one yourself, and Details shows the files that differ from the target. From here you can update, upgrade or roll back a single installation.',
  gtd_envs: 'Groups the same service into environments such as test and production, with a branch for each. Here you see what runs where and deploy or roll back a whole environment in one go.',
  gtd_runs: 'The list of every update, upgrade, rollback and restart started from WRM, with its state per server and the versions involved. Scheduled runs wait here until their time.',
  gtd_activity: 'What is happening in the repositories and on the servers: Git activity (pushes and commits), deploy history and a "what is where" table of branches, commits and versions.',
  gtd_gitignore: 'Shows which files a service keeps out of version control and helps you add missing patterns to its .gitignore, so temporary or secret files are not committed by accident.',
  gtd_settings: 'One-time setup: the Git servers WRM reads from, offline bundles for servers without Git access, where targets come from, which SSH servers to check and whether to check periodically.',
});
Object.assign(LANGS.hr, {
  gtd_overview: 'Uspoređuje što radi na svakom serveru s najnovijom verzijom u Git repozitoriju. Odaberite koja verzija je cilj, pritisnite „Provjeri sada” i čitajte obojena stanja: ažurno, treba nadogradnju ili pregled. Provjera samo čita datoteke; ništa se ne mijenja.',
  gtd_services: 'Katalog vaših aplikacija: iz kojeg Git projekta svaka dolazi, koju granu ili oznaku prati i koje datoteke joj pripadaju. Servis dodajte ručno, predložite ga iz repozitorija ili uvezite i izvezite cijeli katalog.',
  gtd_installs: 'Svaka kopija servisa pronađena na vašim serverima. „Otkrij” ih pronalazi automatski, „Dodaj ručno” upisuje jednu sami, a „Detalji” prikazuju datoteke koje se razlikuju od cilja. Odavde možete nadograditi, osvježiti ili vratiti pojedinu instalaciju.',
  gtd_envs: 'Grupira isti servis u okruženja poput testnog i produkcijskog, sa svojom granom za svako. Vidite što radi gdje i možete isporučiti ili vratiti cijelo okruženje odjednom.',
  gtd_runs: 'Popis svake nadogradnje, osvježavanja, povratka i ponovnog pokretanja započetog iz WRM-a, sa stanjem po serveru i verzijama. Zakazana izvođenja čekaju ovdje do svog vremena.',
  gtd_activity: 'Što se događa u repozitorijima i na serverima: Git aktivnost (push i commiti), povijest isporuka i tablica „što je gdje” s granama, commitima i verzijama.',
  gtd_gitignore: 'Prikazuje koje datoteke servis drži izvan verzioniranja i pomaže dodati nedostajuće obrasce u njegov .gitignore, da privremene ili tajne datoteke ne završe slučajno u repozitoriju.',
  gtd_settings: 'Jednokratno podešavanje: Git serveri s kojih WRM čita, offline paketi za servere bez Git pristupa, odakle dolaze ciljevi, koje SSH servere provjeravati i hoće li se provjeravati periodički.',
});

Object.assign(LANGS.en, {
 git_tab_functionality:'Functionality', gtd_functionality:'Service runtime, Supervisor workers, installation problems and configuration values on each server.',
 gf_intro:'Checks are read only and run with manual and periodic Git checks. HTTP/TCP probes are optional per service. Healthy means the configured checks passed; missing or inaccessible checks remain unverified.',
 gf_healthy:'Healthy', gf_warning:'Warning', gf_failed:'Failed', gf_unknown:'Unverified', gf_config:'Configuration', gf_no_config:'No configuration files found.', gf_config_hint:'Values are read now; secrets are masked. Comments describe settings where available. Lines that cannot be parsed are counted; source code is never executed.',
 gf_section:'Section', gf_variable:'Variable / function', gf_value:'Configured value', gf_description:'Description', gf_unparsed:'{n} lines could not be parsed as settings.',
 gf_no_install:'No installation registered. Discover services or add an installation.', gf_units:'Runtime units (one per line: systemd:name.service or supervisor:program)', gf_http:'HTTP health URL (optional)', gf_tcp:'TCP health address (optional, host:port)', gf_files:'Configuration files (optional relative paths; otherwise auto-discover)', gf_servers:'Server checks', gf_log:'Error log', gf_never:'Run a check to inspect functionality.', gf_source:'Repository', gf_checks:'Runtime checks', gf_all_workers:'All Supervisor workers and failed systemd services',
});
Object.assign(LANGS.hr, {
 git_tab_functionality:'Funkcionalnost', gtd_functionality:'Stanje servisa, Supervisor workeri, problemi instalacije i vrijednosti konfiguracije na svakom serveru.',
 gf_intro:'Provjere samo čitaju podatke i pokreću se pri ručnim i periodičnim Git provjerama. HTTP/TCP provjere su opcionalne po servisu. Uspješan status znači da su zadane provjere prošle; nedostupne ili nedostajuće provjere ostaju nepotvrđene.',
 gf_healthy:'Ispravno', gf_warning:'Upozorenje', gf_failed:'Neispravno', gf_unknown:'Nepotvrđeno', gf_config:'Konfiguracija', gf_no_config:'Nisu pronađene konfiguracijske datoteke.', gf_config_hint:'Vrijednosti se čitaju sada; tajne su maskirane. Postavke su opisane komentarima gdje postoje. Neprepoznati redci se broje; izvorni kod se ne izvršava.',
 gf_section:'Sekcija', gf_variable:'Varijabla / funkcija', gf_value:'Postavljena vrijednost', gf_description:'Opis', gf_unparsed:'{n} redaka nije prepoznato kao postavke.',
 gf_no_install:'Nema registrirane instalacije. Pronađite servise ili dodajte instalaciju.', gf_units:'Jedinice servisa (po retku: systemd:naziv.service ili supervisor:program)', gf_http:'HTTP URL provjere stanja (opcionalno)', gf_tcp:'TCP adresa provjere (opcionalno, host:port)', gf_files:'Konfiguracijske datoteke (opcionalne relativne putanje; inače automatsko pronalaženje)', gf_servers:'Provjere servera', gf_log:'Zapis greške', gf_never:'Pokrenite provjeru funkcionalnosti.', gf_source:'Repozitorij', gf_checks:'Provjere servisa', gf_all_workers:'Svi Supervisor workeri i neuspješne systemd jedinice',
});


Object.assign(LANGS.en, {
 gm_settings:'Original runtime settings', gm_settings_hint:'Stop Medium before saving. These are the original deploy-tool environment options: config Git repository/pull/check interval, commit identity, SSH options, command timeout and build user. Empty fields use the original defaults. Masked values are preserved.', gm_title:'Original deploy tool', gm_intro:'Medium runs the unchanged original deploy-tool interface and engine. Each account keeps private configuration, work files and SSH data. Windows uses a managed Linux runtime; no Windows rsync/Python setup is needed.',
 gm_prepare:'Prepare runtime / retry', gm_imported:'Tool ZIP imported; runtime needs preparation', gm_docker_setup:'Docker setup on Windows', gm_backend:'Runtime', gm_start:'Start', gm_stop:'Stop', gm_tool:'Import deploy-tool ZIP', gm_config:'Import deploy-config ZIP', gm_ssh:'Import SSH profile ZIP', gm_open:'Open full window', gm_running:'Running', gm_stopped:'Stopped', gm_tool_ready:'Tool ready', gm_tool_missing:'Tool not installed', gm_config_ready:'Config loaded', gm_config_missing:'Config not loaded', gm_missing:'Missing dependencies', gm_size:'ZIP exceeds 20 MiB', gm_trust:'This source/configuration can execute build and deployment commands on the server. Import only files you trust. Continue?', gm_stop_confirm:'Stop the original deploy tool? Wait for deployments to finish first.',
});
Object.assign(LANGS.hr, {
 gm_settings:'Postavke izvornog runtimea', gm_settings_hint:'Zaustavite Srednje prije spremanja. Ovo su izvorne deploy-tool opcije: Git repo konfiguracije/povlačenje/interval provjere, autor commitova, SSH opcije, timeout naredbi i build korisnik. Prazna polja koriste izvorne zadane vrijednosti. Maskirane vrijednosti se zadržavaju.', gm_title:'Izvorni deploy tool', gm_intro:'Srednje pokreće neizmijenjeno izvorno sučelje i deploy mehanizam. Svaki račun zadržava privatnu konfiguraciju, radne datoteke i SSH podatke. Windows koristi upravljani Linux runtime; ne treba zasebno postavljanje rsynca/Pythona.',
 gm_prepare:'Pripremi runtime / pokušaj ponovno', gm_imported:'Tool ZIP učitan; runtime treba pripremiti', gm_docker_setup:'Docker postavljanje na Windowsu', gm_backend:'Runtime', gm_start:'Pokreni', gm_stop:'Zaustavi', gm_tool:'Uvezi deploy-tool ZIP', gm_config:'Uvezi deploy-config ZIP', gm_ssh:'Uvezi SSH profil ZIP', gm_open:'Otvori cijeli prozor', gm_running:'Pokrenuto', gm_stopped:'Zaustavljeno', gm_tool_ready:'Tool spreman', gm_tool_missing:'Tool nije instaliran', gm_config_ready:'Config učitan', gm_config_missing:'Config nije učitan', gm_missing:'Nedostaju preduvjeti', gm_size:'ZIP prelazi 20 MiB', gm_trust:'Ovaj izvor/config može izvršavati build i deploy naredbe na serveru. Uvozite samo datoteke kojima vjerujete. Nastaviti?', gm_stop_confirm:'Zaustaviti izvorni deploy tool? Najprije pričekajte završetak deploya.',
});
Object.assign(LANGS.en, {
 mn_title:'Native engine (preview, read-only)', mn_intro:'WRM reads your deploy-config as it is, prepares the chosen branch or tag from a WRM Git source and compares it with each destination through the WRM connection linked to that server. Nothing is written to any server. Compare with the original tool runs the original comparison too and shows both change lists side by side.',
 mn_servers:'Servers and WRM connections', mn_server:'Server', mn_address:'Address in servers.yaml', mn_connection:'WRM connection', mn_auto:'Automatic', mn_none:'None', mn_unlinked:'not linked', mn_linked:'Connection saved',
 mn_plan:'Plan', mn_project:'Project', mn_env:'Environment', mn_ref:'Branch or tag', mn_preview:'Preview natively', mn_compare:'Compare with the original tool', mn_needs_running:'Start Medium (the original tool) first',
 mn_build_note:'This project has a build step. Choose where the build runs before previewing: on the WRM host, on a Linux build server over SSH, or not at all. The plan shows the build location and checks its platform and architecture against every destination.', mn_source_note:'Source: WRM Git source {0}', mn_no_source_note:'No WRM Git source serves this repository; add it under Git → Sources for the native preview.',
 mn_stage_configuration:'Reading the configuration…', mn_stage_original:'The original tool is comparing (fetch, export, build, rsync)…', mn_stage_build:'Building', mn_stage_source:'Preparing the source…', mn_stage_servers:'Comparing with the servers…',
 mn_same:'Same result as the original tool', mn_differs:'Differs from the original tool', mn_src_git_source:'WRM Git source', mn_src_original:'the original tool\'s prepared source', mn_files:'files',
 mn_new:'new', mn_changed:'changed', mn_deleted:'deleted', mn_no_changes:'No changes: the server matches the source.', mn_missing_dir:'The application directory does not exist yet; everything is new.',
 mn_only_ref:'Only in the original tool', mn_only_native:'Only in the native engine', mn_kind:'Type differs', mn_matched:'{0} of {1} changes match',
 mn_current:'On the server', mn_lock:'Locked by {0} since {1}', mn_history:'Recent deployments', mn_changes:'Changes', mn_excludes:'Exclusions', mn_post_deploy:'After deployment', mn_ref_error:'Original tool',
 mn_not_allowed:'not allowed for this environment', mn_confirm:'typing the environment name is required', mn_source_check:'Source check (WRM Git source vs. the original export)', mn_source_same:'identical', mn_source_missing:'missing in the Git source', mn_source_extra:'only in the Git source', mn_source_differs:'different content', mn_not_compared:'Not selected', mn_more:'… and {0} more',
});
Object.assign(LANGS.hr, {
 mn_title:'Nativni engine (pregled, samo čitanje)', mn_intro:'WRM čita vaš deploy-config kakav jest, priprema odabranu granu ili tag iz WRM Git izvora i uspoređuje ga sa svakim odredištem preko WRM veze povezane s tim serverom. Ništa se ne zapisuje ni na jedan server. Usporedba s izvornim alatom pokreće i izvornu usporedbu i prikazuje oba popisa promjena jedan uz drugi.',
 mn_servers:'Serveri i WRM veze', mn_server:'Server', mn_address:'Adresa u servers.yaml', mn_connection:'WRM veza', mn_auto:'Automatski', mn_none:'Nijedna', mn_unlinked:'nije povezan', mn_linked:'Veza spremljena',
 mn_plan:'Plan', mn_project:'Projekt', mn_env:'Okolina', mn_ref:'Grana ili tag', mn_preview:'Nativni pregled', mn_compare:'Usporedi s izvornim alatom', mn_needs_running:'Prvo pokrenite Srednje (izvorni alat)',
 mn_build_note:'Ovaj projekt ima build. Prije pregleda odaberite gdje se build izvodi: na WRM hostu, na Linux build serveru preko SSH-a ili nikako. Plan prikazuje mjesto builda i provjerava njegovu platformu i arhitekturu prema svakom odredištu.', mn_source_note:'Izvor: WRM Git izvor {0}', mn_no_source_note:'Nijedan WRM Git izvor ne poslužuje ovaj repozitorij; za nativni pregled dodajte ga pod Git → Izvori.',
 mn_stage_configuration:'Čitam konfiguraciju…', mn_stage_original:'Izvorni alat uspoređuje (fetch, izvoz, build, rsync)…', mn_stage_build:'Izvodim build', mn_stage_source:'Pripremam izvor…', mn_stage_servers:'Uspoređujem sa serverima…',
 mn_same:'Isti rezultat kao izvorni alat', mn_differs:'Razlikuje se od izvornog alata', mn_src_git_source:'WRM Git izvor', mn_src_original:'izvor koji je pripremio izvorni alat', mn_files:'datoteka',
 mn_new:'novo', mn_changed:'promijenjeno', mn_deleted:'obrisano', mn_no_changes:'Nema promjena: server odgovara izvoru.', mn_missing_dir:'Direktorij aplikacije još ne postoji; sve je novo.',
 mn_only_ref:'Samo u izvornom alatu', mn_only_native:'Samo u nativnom engineu', mn_kind:'Razlikuje se vrsta', mn_matched:'{0} od {1} promjena se poklapa',
 mn_current:'Na serveru', mn_lock:'Zaključao {0} od {1}', mn_history:'Zadnji deployevi', mn_changes:'Promjene', mn_excludes:'Izuzeća', mn_post_deploy:'Nakon deploya', mn_ref_error:'Izvorni alat',
 mn_not_allowed:'nije dopušten za ovu okolinu', mn_confirm:'potrebno je upisati ime okoline', mn_source_check:'Provjera izvora (WRM Git izvor naspram izvornog izvoza)', mn_source_same:'identično', mn_source_missing:'nedostaje u Git izvoru', mn_source_extra:'samo u Git izvoru', mn_source_differs:'drukčiji sadržaj', mn_not_compared:'Nije odabrano', mn_more:'… i još {0}',
});


Object.assign(LANGS.en, {
 gcp_title:'Configuration profiles', gcp_intro:'Import a reference config/INI file and inspect the corresponding file on each installation. Selected installation checkboxes limit the check; otherwise all installations of the chosen service are checked (up to 50).', gcp_add:'Add config/INI profile', gcp_settings:'settings', gcp_check:'Inspect servers', gcp_path:'Relative path on the server', gcp_operator:'Operator / client (optional)', gcp_file:'Reference file', gcp_import_hint:'The reference defines settings and expected values. It is parsed without executing scripts; secrets are masked before storage. The server file must be inside the installation.', gcp_file_limit:'Choose a text configuration file up to 256 KiB.', gcp_delete:'Delete this configuration profile?', gcp_values_hint:'Actual values are read from servers now. Script expressions are shown as source, never executed. Secret values are masked and are not compared. INI sections can identify operators.', gcp_expected:'Reference value', gcp_missing:'Missing', gcp_masked:'Masked', gcp_extra:'Additional setting', gcp_same:'Matches reference', gcp_different:'Different from reference',
});
Object.assign(LANGS.hr, {
 gcp_title:'Konfiguracijski profili', gcp_intro:'Uvezite referentni config/INI i pregledajte odgovarajuću datoteku na svakoj instalaciji. Označene instalacije ograničavaju provjeru; inače se provjeravaju sve instalacije odabranog servisa (do 50).', gcp_add:'Dodaj config/INI profil', gcp_settings:'postavki', gcp_check:'Pregledaj servere', gcp_path:'Relativna putanja na serveru', gcp_operator:'Operater / klijent (opcionalno)', gcp_file:'Referentna datoteka', gcp_import_hint:'Referenca definira postavke i očekivane vrijednosti. Parsira se bez izvršavanja skripti; tajne se maskiraju prije spremanja. Datoteka na serveru mora biti unutar instalacije.', gcp_file_limit:'Odaberite tekstualnu konfiguraciju do 256 KiB.', gcp_delete:'Obrisati ovaj konfiguracijski profil?', gcp_values_hint:'Stvarne vrijednosti čitaju se sada sa servera. Izrazi skripti prikazuju se kao izvor i ne izvršavaju se. Tajne su maskirane i ne uspoređuju se. INI sekcije mogu označavati operatere.', gcp_expected:'Referentna vrijednost', gcp_missing:'Nedostaje', gcp_masked:'Maskirano', gcp_extra:'Dodatna postavka', gcp_same:'Odgovara referenci', gcp_different:'Razlikuje se od reference',
});

Object.assign(LANGS.en, {
  gm_s2_run_other: 'In progress ({where}): {user} is preparing the shared runtime since {at}. Wait until it finishes; you can import your deploy-config meanwhile.',
  gm_hint_start_noout: 'The original tool stopped while starting. Its output is shown only to administrators: ask one to look at it in the activity log, fix the cause and press Start again.',
});
Object.assign(LANGS.hr, {
  gm_s2_run_other: 'U tijeku ({where}): {user} priprema zajedničko okruženje od {at}. Pričekaj da završi; u međuvremenu možeš uvesti svoj deploy-config.',
  gm_hint_start_noout: 'Izvorni alat je stao tijekom pokretanja. Njegov izlaz vide samo administratori: zamoli jednog da ga pogleda u dnevniku aktivnosti, ispravi uzrok i ponovno klikni Pokreni.',
});
Object.assign(LANGS.en, { gm_force_stop: 'Stop it anyway? A comparison or deployment that is still running is interrupted; the original tool\'s lock and history then show where it stopped.' });
Object.assign(LANGS.hr, { gm_force_stop: 'Ipak zaustaviti? Usporedba ili deploy koji još radi bit će prekinut; zaključavanje i povijest izvornog alata tada pokazuju gdje je stao.' });
Object.assign(LANGS.en, { git_back: 'Back', git_back_to: 'Back to “{name}”' });
Object.assign(LANGS.hr, { git_back: 'Natrag', git_back_to: 'Natrag na “{name}”' });
Object.assign(LANGS.en, { git_stamp_differ: '{n} file(s) differ from the target', git_stamp_more: '… and {n} more', git_stamp_partial: 'Also stamp installations that are not up to date (as they are)',
  git_stamp_partial_h: 'VERSION.md then records each file\'s actual version: older files their matching version, files changed by hand "(local)", and a note that the installation is partial. Nothing else is written.',
  git_stamp_none: 'None of these installations is up to date. Update them first, or tick “Also stamp installations that are not up to date”.', git_stamp_vmd_exists: 'VERSION.md already exists on every chosen installation. Tick “Overwrite an existing VERSION.md” to replace it.',
  git_nothing_usable: 'None of these installations can be used: see the message under each one (for example, check it first).' });
Object.assign(LANGS.hr, { git_stamp_differ: '{n} datoteka se razlikuje od cilja', git_stamp_more: '… i još {n}', git_stamp_partial: 'Označi i instalacije koje nisu ažurne (kakve jesu)',
  git_stamp_partial_h: 'VERSION.md tada bilježi stvarnu verziju svake datoteke: starije datoteke njihovu verziju, ručno mijenjane "(local)", uz napomenu da je instalacija djelomična. Ništa drugo se ne upisuje.',
  git_stamp_none: 'Nijedna od ovih instalacija nije ažurna. Najprije ih ažurirajte ili označite “Označi i instalacije koje nisu ažurne”.', git_stamp_vmd_exists: 'VERSION.md već postoji na svim odabranim instalacijama. Označite “Prepiši postojeći VERSION.md” da ga zamijenite.',
  git_nothing_usable: 'Nijedna od ovih instalacija se ne može koristiti: pogledajte poruku ispod svake (npr. najprije je provjerite).' });
Object.assign(LANGS.en, {"gm_quick": "Prepare and run trusted ZIPs", "gm_quick_hint": "Add deploy-tool and deploy-config ZIPs (SSH profile optional), then click once. WRM starts Docker, prepares dependencies, validates config and opens the original tool. Approved imports are retained if setup needs a retry. First Windows installation can show Docker/WSL/license prompts on the WRM host.", "gm_quick_files": "ZIPs are trusted executable/configuration imports. Leave a file empty to keep its existing import.", "gm_setup_docker": "Starting Docker / waiting for first-install prompts", "gm_setup_dependencies": "Preparing original dependencies", "gm_setup_configuration": "Checking configuration and SSH profile", "gm_setup_starting": "Starting original deploy tool", "gm_setup_ready": "Ready", "gm_advanced_setup": "Advanced setup / individual imports"});
Object.assign(LANGS.hr, {"gm_quick": "Pripremi i pokreni pouzdane ZIP-ove", "gm_quick_hint": "Dodajte deploy-tool i deploy-config ZIP (SSH profil po želji), zatim kliknite jednom. WRM pokreće Docker, priprema ovisnosti, provjerava config i otvara izvorni alat. Uvezeni ZIP-ovi ostaju spremljeni ako treba ponoviti pripremu. Prva Windows instalacija može prikazati Docker/WSL/licencne upite na WRM računalu.", "gm_quick_files": "ZIP-ovi su pouzdani uvozi izvršnog koda/postavki. Prazno polje zadržava postojeći uvoz.", "gm_setup_docker": "Pokretanje Dockera / čekanje upita prve instalacije", "gm_setup_dependencies": "Priprema ovisnosti izvornog alata", "gm_setup_configuration": "Provjera konfiguracije i SSH profila", "gm_setup_starting": "Pokretanje izvornog deploy alata", "gm_setup_ready": "Spremno", "gm_advanced_setup": "Napredne postavke / pojedinačni uvozi"});
// v12.6.5: Medium step by step, status lights, steps with times and the activity log
Object.assign(LANGS.en, {
  gm_two_title: 'Medium has two separate parts', gm_import: 'Import ZIP',
  gm_two_original: 'Your original deploy tool, run unchanged from its ZIP (in Docker or with Python on the WRM host). It is the only part that deploys and rolls back. Set it up with steps 1 to 5 below; once it runs, it opens below its card.',
  gm_two_preview: 'WRM\'s own engine. It only reads: it shows what a deployment would change on each server, through your WRM connections. It needs only your deploy-config, not Docker and not the original tool. Only "Compare with the original tool" needs the original tool running.',
  gm_s1: 'Deploy-tool ZIP (administrator, once for everyone)', gm_s2: 'Prepare the runtime (administrator)', gm_s3: 'Your deploy-config ZIP', gm_s4: 'Your SSH profile ZIP (optional)', gm_s5: 'Start the original deploy tool',
  gm_s1_ok: 'Imported (SHA-256 {sha}…).', gm_s1_todo_admin: 'Import the deploy-tool ZIP. WRM keeps it and prepares its runtime right after (step 2).', gm_s1_todo_user: 'Waiting for an administrator to import the deploy-tool ZIP.',
  gm_rt_docker: 'Docker (managed Linux container)', gm_rt_host: 'Python on the WRM host',
  gm_s2_run: 'In progress ({where}): {step}. The first time this downloads the Docker image and installs Python packages, which can take several minutes. You can import your deploy-config meanwhile.',
  gm_s2_ok: 'Ready ({where}).', gm_s2_after1: 'Follows step 1 ({where}).', gm_s2_missing: 'Missing on the WRM host for {where}: {deps}.', gm_s2_failed: 'The last preparation failed ({where}).',
  gm_s2_todo_admin: 'Not prepared yet ({where}): press "Prepare runtime / retry".', gm_s2_todo_user: 'Waiting for an administrator to prepare the runtime.',
  gm_s3_pending: 'Received and checked by WRM. The original tool checks and applies it at the next start (step 5).', gm_s3_ok: 'Loaded and checked by the original tool.', gm_s3_todo: 'Import your deploy-config ZIP (servers.yaml, environments.yaml, projects/). You can do this while the runtime is being prepared.', gm_s3_rejected: 'The original tool rejected your deploy-config.',
  gm_s4_pending: 'Received; applied at the next start.', gm_s4_text: 'Only if the servers need your own SSH keys or known_hosts. Without it the original tool uses WRM\'s SSH options.',
  gm_s5_running: 'Running since {since} ({took})', gm_s5_starting: 'Starting: {step}', gm_s5_failed: 'The last start failed.', gm_s5_exited: 'It stopped unexpectedly at {at} (exit code {code}).', gm_s5_ready: 'Ready to start. Start runs the original tool for you only (each user has their own); Stop ends it.', gm_s5_todo: 'Steps 1 to 3 are needed first.',
  gm_why_running: 'It is already running', gm_why_busy: 'Another Medium operation is running; wait for it to finish', gm_why_prepare_running: 'Stop every running original tool first',
  gm_now_mine: 'In progress: {op}, started at {at} ({took})', gm_now_other: 'In progress: {op} by {user}, started at {at} ({took})', gm_now_other_wait: 'Medium runs one setup or start at a time. Buttons that would collide are disabled until it finishes.', gm_now_other_short: 'another user\'s operation is running',
  gm_show_output: 'Show output', gm_hide_output: 'Hide output', gm_no_output: '(no output yet)', gm_output_policy: 'The administrator does not show command output to users (medium_log_output).',
  gm_last_failed: 'Last operation failed: {op}, started at {at}', gm_last_ok: 'Last operation: {op}, started at {at}', gm_last_more: 'The activity log below keeps every operation with its steps and output.',
  gm_conn_lost: 'Cannot reach WRM; retrying…',
  gm_l_running: 'Running', gm_l_working: 'Working: {op}', gm_l_other: 'Busy ({user})', gm_l_error: 'Needs attention', gm_l_stopped: 'Not running', gm_runtime_is: 'Runtime: {where}',
  gm_p_ready: 'Available', gm_p_needs_config: 'Needs deploy-config',
  gm_log_title: 'Activity log', gm_log_hint: 'Who started what in Medium, when, the result and how long it took. Click a row for its steps and output.', gm_log_empty: 'Nothing has run yet.',
  gm_log_when: 'When', gm_log_who: 'Who', gm_log_what: 'What', gm_log_result: 'Result', gm_log_took: 'Duration',
  gm_r_ok: 'Done', gm_r_error: 'Failed', gm_r_running: 'Running', gm_r_interrupted: 'Interrupted (WRM restarted)',
  gm_kept_config: 'deploy-config received and checked by WRM. The original tool checks and applies it at the next start.', gm_kept_ssh: 'SSH profile received; it is applied at the next start.',
  gm_imported_tool: 'Deploy tool imported; its runtime is being prepared (step 2).', gm_imported_config: 'deploy-config checked by the original tool and loaded.', gm_imported_ssh: 'SSH profile loaded.',
  gm_op_setup: 'Prepare and start', gm_op_prepare: 'Prepare runtime', gm_op_tool_import: 'Deploy-tool import', gm_op_start: 'Start', gm_op_config_import: 'deploy-config import', gm_op_ssh_import: 'SSH profile import', gm_op_stop: 'Stop', gm_op_settings: 'Runtime settings saved', gm_op_preview: 'WRM preview', gm_op_compare: 'Comparison with the original tool', gm_op_tool_request: 'Action in the original tool', gm_op_exited: 'The original tool stopped',
  gm_st_upload: 'Receiving and checking the ZIPs', gm_st_tool_unpack: 'Unpacking the deploy tool', gm_st_docker: 'Checking Docker (starting it if needed)', gm_st_image: 'Docker image and Python environment', gm_st_packages: 'Installing the tool\'s Python packages', gm_st_python: 'Checking Python on the WRM host', gm_st_venv: 'Creating the Python environment', gm_st_config_check: 'The original tool checks deploy-config', gm_st_ssh_apply: 'Applying the SSH profile', gm_st_start_checks: 'Checking what the start needs', gm_st_start_sync: 'Copying your files into the container', gm_st_start_process: 'Starting the process', gm_st_start_ready: 'Waiting for the tool to answer',
  gm_hint_docker: 'Start Docker Desktop on the WRM host and accept its prompts (WSL, licence) the first time. If it is not installed, install it once (link above). Then press "Prepare runtime / retry"; your ZIPs are kept.',
  gm_hint_image: 'Docker could not download or run its image. Check the WRM host\'s internet or registry access and free disk space, then retry.',
  gm_hint_packages_tls: 'A proxy or antivirus on this network inspects HTTPS, and pip in Docker does not trust its certificate. WRM passes this computer\'s trusted certificates to Docker by default (Administration → Policies → Git: "Medium trusts this computer\'s certificates"). If it still fails, export the proxy\'s root certificate as PEM, set its path in "Extra CA file for Medium" and press Prepare runtime again.',
  gm_hint_packages: 'pip could not install the tool\'s Python packages. Check access to pypi.org (or your mirror) from the WRM host and the requirements.lock in the deploy-tool ZIP; the output shows which package failed.',
  gm_hint_python: 'Install Python 3.12+ with git, ssh and rsync on the WRM host, or use the managed Docker runtime (WRM_MEDIUM_RUNTIME=docker).',
  gm_hint_config: 'Open the output in the activity log: it names the file and the line. Fix the deploy-config ZIP and import it again.',
  gm_hint_start: 'The last lines of the output show why the original tool stopped (often a missing package or a deploy-config error). Fix it and press Start again.',
  gm_hint_config_wrm: 'WRM\'s check names the file and the problem in the message above. Fix the deploy-config ZIP and import it again.',
  gm_hint_upload: 'Check the ZIP files (at most 20 MiB each, the expected files inside) and try again.'
});
Object.assign(LANGS.hr, {
  gm_two_title: 'Medium ima dva odvojena dijela', gm_import: 'Uvezi ZIP',
  gm_two_original: 'Tvoj izvorni deploy-tool, pokrenut neizmijenjen iz ZIP-a (u Dockeru ili s Pythonom na WRM računalu). Jedini dio koji deploya i radi rollback. Postavljaš ga koracima 1 do 5 ispod; kad radi, otvara se ispod svoje kartice.',
  gm_two_preview: 'WRM-ov vlastiti engine. Samo čita: pokazuje što bi deploy promijenio na svakom serveru, preko tvojih WRM veza. Treba mu samo tvoj deploy-config, ne treba Docker ni izvorni alat. Samo "Usporedi s izvornim alatom" traži da izvorni alat radi.',
  gm_s1: 'Deploy-tool ZIP (administrator, jednom za sve)', gm_s2: 'Priprema okruženja (administrator)', gm_s3: 'Tvoj deploy-config ZIP', gm_s4: 'Tvoj SSH profil ZIP (neobavezno)', gm_s5: 'Pokretanje izvornog deploy-toola',
  gm_s1_ok: 'Uvezen (SHA-256 {sha}…).', gm_s1_todo_admin: 'Uvezi deploy-tool ZIP. WRM ga sprema i odmah zatim priprema okruženje (korak 2).', gm_s1_todo_user: 'Čeka se da administrator uveze deploy-tool ZIP.',
  gm_rt_docker: 'Docker (upravljani Linux kontejner)', gm_rt_host: 'Python na WRM računalu',
  gm_s2_run: 'U tijeku ({where}): {step}. Prvi put se preuzima Docker slika i instaliraju Python paketi, što može trajati nekoliko minuta. U međuvremenu možeš uvesti svoj deploy-config.',
  gm_s2_ok: 'Spremno ({where}).', gm_s2_after1: 'Slijedi nakon koraka 1 ({where}).', gm_s2_missing: 'Na WRM računalu za {where} nedostaje: {deps}.', gm_s2_failed: 'Zadnja priprema nije uspjela ({where}).',
  gm_s2_todo_admin: 'Još nije pripremljeno ({where}): klikni "Pripremi okruženje / pokušaj ponovno".', gm_s2_todo_user: 'Čeka se da administrator pripremi okruženje.',
  gm_s3_pending: 'Primljen i provjeren u WRM-u. Izvorni alat ga provjerava i primjenjuje pri sljedećem pokretanju (korak 5).', gm_s3_ok: 'Učitan i provjeren u izvornom alatu.', gm_s3_todo: 'Uvezi svoj deploy-config ZIP (servers.yaml, environments.yaml, projects/). Možeš i dok se okruženje priprema.', gm_s3_rejected: 'Izvorni alat je odbio tvoj deploy-config.',
  gm_s4_pending: 'Primljen; primjenjuje se pri sljedećem pokretanju.', gm_s4_text: 'Samo ako serveri traže tvoje SSH ključeve ili known_hosts. Bez njega izvorni alat koristi WRM-ove SSH opcije.',
  gm_s5_running: 'Radi od {since} ({took})', gm_s5_starting: 'Pokreće se: {step}', gm_s5_failed: 'Zadnje pokretanje nije uspjelo.', gm_s5_exited: 'Neočekivano se zaustavio u {at} (izlazni kod {code}).', gm_s5_ready: 'Spremno za pokretanje. Pokreni pokreće izvorni alat samo za tebe (svaki korisnik ima svoj); Zaustavi ga gasi.', gm_s5_todo: 'Prvo trebaju koraci 1 do 3.',
  gm_why_running: 'Već radi', gm_why_busy: 'U tijeku je druga Medium radnja; pričekaj da završi', gm_why_prepare_running: 'Prvo zaustavi sve pokrenute izvorne alate',
  gm_now_mine: 'U tijeku: {op}, pokrenuto u {at} ({took})', gm_now_other: 'U tijeku: {op}, pokrenuo {user} u {at} ({took})', gm_now_other_wait: 'Medium radi jednu pripremu ili pokretanje odjednom. Gumbi koji bi se sudarili onemogućeni su dok ne završi.', gm_now_other_short: 'u tijeku je radnja drugog korisnika',
  gm_show_output: 'Prikaži izlaz', gm_hide_output: 'Sakrij izlaz', gm_no_output: '(još nema izlaza)', gm_output_policy: 'Administrator ne prikazuje izlaz naredbi korisnicima (medium_log_output).',
  gm_last_failed: 'Zadnja radnja nije uspjela: {op}, pokrenuta u {at}', gm_last_ok: 'Zadnja radnja: {op}, pokrenuta u {at}', gm_last_more: 'Dnevnik aktivnosti ispod čuva svaku radnju s koracima i izlazom.',
  gm_conn_lost: 'WRM trenutno nije dostupan; pokušavam ponovno…',
  gm_l_running: 'Radi', gm_l_working: 'Radi se: {op}', gm_l_other: 'Zauzeto ({user})', gm_l_error: 'Treba pažnju', gm_l_stopped: 'Ne radi', gm_runtime_is: 'Okruženje: {where}',
  gm_p_ready: 'Dostupno', gm_p_needs_config: 'Treba deploy-config',
  gm_log_title: 'Dnevnik aktivnosti', gm_log_hint: 'Tko je što pokrenuo u Mediumu, kada, s kojim rezultatom i koliko je trajalo. Klikni redak za korake i izlaz.', gm_log_empty: 'Još ništa nije pokrenuto.',
  gm_log_when: 'Kada', gm_log_who: 'Tko', gm_log_what: 'Što', gm_log_result: 'Rezultat', gm_log_took: 'Trajanje',
  gm_r_ok: 'Gotovo', gm_r_error: 'Neuspjelo', gm_r_running: 'U tijeku', gm_r_interrupted: 'Prekinuto (WRM ponovno pokrenut)',
  gm_kept_config: 'deploy-config je primljen i provjeren u WRM-u. Izvorni alat ga provjerava i primjenjuje pri sljedećem pokretanju.', gm_kept_ssh: 'SSH profil je primljen; primjenjuje se pri sljedećem pokretanju.',
  gm_imported_tool: 'Deploy-tool je uvezen; okruženje se priprema (korak 2).', gm_imported_config: 'Izvorni alat je provjerio i učitao deploy-config.', gm_imported_ssh: 'SSH profil je učitan.',
  gm_op_setup: 'Pripremi i pokreni', gm_op_prepare: 'Priprema okruženja', gm_op_tool_import: 'Uvoz deploy-toola', gm_op_start: 'Pokretanje', gm_op_config_import: 'Uvoz deploy-configa', gm_op_ssh_import: 'Uvoz SSH profila', gm_op_stop: 'Zaustavljanje', gm_op_settings: 'Spremljene postavke okruženja', gm_op_preview: 'WRM pregled', gm_op_compare: 'Usporedba s izvornim alatom', gm_op_tool_request: 'Radnja u izvornom alatu', gm_op_exited: 'Izvorni alat se zaustavio',
  gm_st_upload: 'Primanje i provjera ZIP-ova', gm_st_tool_unpack: 'Raspakiravanje deploy-toola', gm_st_docker: 'Provjera Dockera (pokretanje ako treba)', gm_st_image: 'Docker slika i Python okruženje', gm_st_packages: 'Instalacija Python paketa alata', gm_st_python: 'Provjera Pythona na WRM računalu', gm_st_venv: 'Izrada Python okruženja', gm_st_config_check: 'Izvorni alat provjerava deploy-config', gm_st_ssh_apply: 'Primjena SSH profila', gm_st_start_checks: 'Provjera što pokretanje treba', gm_st_start_sync: 'Kopiranje tvojih datoteka u kontejner', gm_st_start_process: 'Pokretanje procesa', gm_st_start_ready: 'Čekanje da se alat javi',
  gm_hint_docker: 'Pokreni Docker Desktop na WRM računalu i prvi put prihvati njegove upite (WSL, licenca). Ako nije instaliran, instaliraj ga jednom (poveznica gore). Zatim klikni "Pripremi okruženje / pokušaj ponovno"; tvoji ZIP-ovi ostaju spremljeni.',
  gm_hint_image: 'Docker nije mogao preuzeti ili pokrenuti sliku. Provjeri pristup internetu ili registryju s WRM računala i slobodan prostor na disku, pa pokušaj ponovno.',
  gm_hint_packages_tls: 'Proxy ili antivirus na ovoj mreži pregledava HTTPS, a pip u Dockeru ne vjeruje njegovom certifikatu. WRM po defaultu Dockeru predaje certifikate kojima ovo računalo vjeruje (Administracija → Pravila → Git: "Medium vjeruje certifikatima ovog računala"). Ako i dalje ne prolazi, izvezi root certifikat proxyja kao PEM, upiši njegovu putanju u "Dodatna CA datoteka za Medium" i ponovno pritisni Pripremi runtime.',
  gm_hint_packages: 'pip nije mogao instalirati Python pakete alata. Provjeri pristup pypi.org (ili tvom mirroru) s WRM računala i requirements.lock u deploy-tool ZIP-u; izlaz pokazuje koji paket nije prošao.',
  gm_hint_python: 'Instaliraj Python 3.12+ te git, ssh i rsync na WRM računalo ili koristi upravljani Docker (WRM_MEDIUM_RUNTIME=docker).',
  gm_hint_config: 'Otvori izlaz u dnevniku aktivnosti: navodi datoteku i redak. Ispravi deploy-config ZIP i uvezi ga ponovno.',
  gm_hint_start: 'Zadnji redovi izlaza pokazuju zašto je izvorni alat stao (često nedostaje paket ili je greška u deploy-configu). Ispravi to i ponovno klikni Pokreni.',
  gm_hint_config_wrm: 'WRM-ova provjera u poruci iznad navodi datoteku i problem. Ispravi deploy-config ZIP i uvezi ga ponovno.',
  gm_hint_upload: 'Provjeri ZIP datoteke (najviše 20 MiB svaka, s očekivanim datotekama unutra) i pokušaj ponovno.'
});
Object.assign(LANGS.en, {
  gm_op_docker: 'Docker installation', gm_dk_install: 'Install Docker',
  gm_st_docker_install: 'Installing Docker', gm_st_docker_download: 'Downloading Docker Desktop (docker.com)', gm_st_docker_verify: 'Checking the installer\'s signature (Docker Inc.)', gm_st_docker_start: 'Starting Docker', gm_st_docker_wait: 'Waiting for the Docker engine to answer',
  gm_s2_dk_consent: '{product} is not installed on the WRM host. WRM installs it itself ({how}), starts it and prepares the runtime, after an administrator allows it once with "Install Docker".',
  gm_s2_dk_wait_admin: '{product} is not installed on the WRM host yet. An administrator installs it from here once; then everything continues automatically.',
  gm_s2_dk_auto: '{product} is not installed yet. WRM installs it ({how}) automatically at the next preparation or tool import; allowed by {user} on {at}. Press "Install Docker" to do it now.',
  gm_s2_dk_off: '{product} is not installed, and automatic installation is switched off (Administration → Policies → Git: "Install Docker for Medium"). Switch it on, or install {product} on the WRM host yourself.',
  gm_s2_dk_manual: '{product} is not installed, and WRM cannot install it here: {why}.', gm_s2_dk_cmd: 'Run once on the WRM host as administrator; then press "Prepare runtime / retry" (or import the deploy-tool ZIP):',
  gm_s2_dk_run: 'In progress: {step}. Downloading and installing Docker can take several minutes; every step and its output are shown here and in the activity log.',
  gm_dk_license: 'Docker Desktop is free for personal use, education, non-commercial open source and small businesses (fewer than 250 employees and less than USD 10 million annual revenue); larger organisations need a paid Docker subscription.', gm_dk_license_link: 'Docker Subscription Service Agreement',
  gm_dk_confirm: 'Install {product} on the WRM host ({how})? WRM downloads and installs it, starts it and then prepares the runtime of the original deploy tool.',
  gm_dk_confirm_license: 'By continuing you accept the Docker Subscription Service Agreement ({url}) for your organisation. Your name and the time are recorded in the audit log.',
  gm_dk_ok_license: 'Accept the licence and install', gm_dk_ok: 'Install',
  gm_dk_how_desktop: 'official installer from docker.com, signature checked', gm_dk_how_pkg: 'packages of this Linux distribution: {method}',
  gm_hint_dk_error: 'Do what the error above says, then press "Prepare runtime / retry". Your ZIPs are kept.', gm_hint_dk_error_new: 'Do what the error above says, then press "Install Docker" again; after that import the deploy-tool ZIP (step 1).',
  gm_hint_docker_install: 'The output (Show output) says why the Docker installation failed; fix that and press "Install Docker" again. Your ZIPs are kept.',
  gm_hint_docker: 'Docker did not answer; the step\'s error above says why and what to do. When Docker is missing, an administrator presses "Install Docker" in step 2. Then press "Prepare runtime / retry"; your ZIPs are kept.'
});
Object.assign(LANGS.hr, {
  gm_op_docker: 'Instalacija Dockera', gm_dk_install: 'Instaliraj Docker',
  gm_st_docker_install: 'Instalacija Dockera', gm_st_docker_download: 'Preuzimanje Docker Desktopa (docker.com)', gm_st_docker_verify: 'Provjera potpisa instalacije (Docker Inc.)', gm_st_docker_start: 'Pokretanje Dockera', gm_st_docker_wait: 'Čekanje da se Docker engine javi',
  gm_s2_dk_consent: '{product} nije instaliran na WRM računalu. WRM ga sam instalira ({how}), pokreće i priprema okruženje kad to administrator jednom dopusti gumbom "Instaliraj Docker".',
  gm_s2_dk_wait_admin: '{product} još nije instaliran na WRM računalu. Administrator ga jednom instalira odavde; nakon toga sve ide automatski.',
  gm_s2_dk_auto: '{product} još nije instaliran. WRM ga instalira ({how}) automatski pri sljedećoj pripremi ili uvozu alata; dopustio {user}, {at}. Klikni "Instaliraj Docker" za instalaciju odmah.',
  gm_s2_dk_off: '{product} nije instaliran, a automatska instalacija je isključena (Administracija → Pravila → Git: "Instalacija Dockera za Medium"). Uključi je ili sam instaliraj {product} na WRM računalo.',
  gm_s2_dk_manual: '{product} nije instaliran, a WRM ga ovdje ne može instalirati: {why}.', gm_s2_dk_cmd: 'Pokreni jednom na WRM računalu kao administrator; zatim klikni "Pripremi runtime / pokušaj ponovno" (ili uvezi deploy-tool ZIP):',
  gm_s2_dk_run: 'U tijeku: {step}. Preuzimanje i instalacija Dockera mogu trajati nekoliko minuta; svaki korak i njegov izlaz vide se ovdje i u dnevniku aktivnosti.',
  gm_dk_license: 'Docker Desktop je besplatan za osobnu upotrebu, obrazovanje, nekomercijalni open source i male tvrtke (manje od 250 zaposlenih i manje od 10 milijuna USD godišnjeg prihoda); veće organizacije trebaju plaćenu Docker pretplatu.', gm_dk_license_link: 'Docker Subscription Service Agreement',
  gm_dk_confirm: 'Instalirati {product} na WRM računalo ({how})? WRM ga preuzima i instalira, pokreće i zatim priprema okruženje izvornog deploy-toola.',
  gm_dk_confirm_license: 'Nastavkom prihvaćaš Docker Subscription Service Agreement ({url}) za svoju organizaciju. Tvoje ime i vrijeme bilježe se u revizijski zapis.',
  gm_dk_ok_license: 'Prihvati licencu i instaliraj', gm_dk_ok: 'Instaliraj',
  gm_dk_how_desktop: 'službeni instalacijski program s docker.com, uz provjeru potpisa', gm_dk_how_pkg: 'paketi ove Linux distribucije: {method}',
  gm_hint_dk_error: 'Napravi ono što kaže greška iznad, zatim klikni "Pripremi runtime / pokušaj ponovno". Tvoji ZIP-ovi ostaju spremljeni.', gm_hint_dk_error_new: 'Napravi ono što kaže greška iznad, zatim ponovno klikni "Instaliraj Docker"; nakon toga uvezi deploy-tool ZIP (korak 1).',
  gm_hint_docker_install: 'Izlaz (Prikaži izlaz) pokazuje zašto instalacija Dockera nije uspjela; ispravi to i ponovno klikni "Instaliraj Docker". Tvoji ZIP-ovi ostaju spremljeni.',
  gm_hint_docker: 'Docker se nije javio; greška koraka iznad kaže zašto i što napraviti. Ako Docker nedostaje, administrator u koraku 2 klikne "Instaliraj Docker". Zatim klikni "Pripremi runtime / pokušaj ponovno"; tvoji ZIP-ovi ostaju spremljeni.'
});
Object.assign(LANGS.en, { mn_title: 'WRM preview (read-only)', mn_needs_running: 'Start the original deploy tool first (step 5)', gm_quick_title: 'Or everything at once: prepare and start in one click', gm_prepare: 'Prepare runtime / retry', gm_settings_hint: 'Stop the original deploy tool before saving; the settings apply at its next start. These are the original deploy-tool environment options: config Git repository/pull/check interval, commit identity, SSH options, command timeout and build user. Empty fields use the original defaults. Masked values are preserved.' });
Object.assign(LANGS.hr, { mn_title: 'WRM pregled (samo čitanje)', mn_needs_running: 'Prvo pokreni izvorni deploy-tool (korak 5)', gm_quick_title: 'Ili sve odjednom: pripremi i pokreni jednim klikom', gm_prepare: 'Pripremi okruženje / pokušaj ponovno', gm_title: 'Izvorni deploy-tool', gm_settings: 'Postavke izvornog alata', gm_settings_hint: 'Zaustavi izvorni deploy-tool prije spremanja; postavke vrijede od sljedećeg pokretanja. Ovo su izvorne deploy-tool opcije: Git repo konfiguracije/povlačenje/interval provjere, autor commitova, SSH opcije, timeout naredbi i build korisnik. Prazna polja koriste izvorne zadane vrijednosti. Maskirane vrijednosti se zadržavaju.', gm_quick: 'Pripremi i pokreni pouzdane ZIP-ove', gm_stop_confirm: 'Zaustaviti izvorni deploy-tool? Najprije pričekaj da deployi završe.' });
Object.assign(LANGS.en, {
  mn_title: 'WRM engine', gm_two_preview: 'WRM\'s own engine. It shows what a deployment would change on each server through your WRM connections. A project switched to the WRM native engine is deployed and rolled back by WRM itself, in the same server state the original tool uses. It needs only your deploy-config; only "Compare with the original tool" needs the original tool running.',
  mn_intro: 'WRM reads your deploy-config as it is, prepares the chosen branch or tag from a WRM Git source and compares it with each destination through the WRM connection linked to that server. A comparison and a dry run change nothing. A project switched to the WRM native engine is deployed and rolled back by WRM: lock, backup, upload, checksum check, post_deploy and history, written in the original tool\'s format, so either tool can continue after the other.',
  mn_build_note: 'This project has a build step. Choose where the build runs before previewing: on the WRM host, on a Linux build server over SSH, or not at all. The plan shows the build location and checks its platform and architecture against every destination.',
  mn_build_where: 'Build', mn_build_choose: 'Choose where to build', mn_build_host: 'WRM host', mn_build_server: 'Linux build server', mn_build_none: 'No build',
  mn_build_conn: 'Build server connection', mn_build_save: 'Save build location', mn_build_saved: 'Build location saved',
  mn_build_no_conn: 'Add an SSH connection for the build server under Servers.', mn_build_pick_conn: 'Choose a build server connection.', mn_build_needed: 'Choose where to build first.',
  mn_build_none_note: 'No build: the selected files come from the commit.', mn_build_ready_note: 'The plan shows the build location and checks its platform and architecture against every destination.',
  mn_build_log: 'Build output', mn_build_original: 'the original tool (it builds)', mn_build_unavailable: 'not available on this WRM host',
  mn_engine: 'Engine', mn_engine_docker: 'Original tool (Docker)', mn_engine_native: 'WRM native', mn_engine_hint: 'Who deploys this project. The server state is the same either way, so you can switch back at any time.',
  mn_engine_set: 'Engine saved', mn_engine_no_policy: 'Native deployments are not allowed for you (policy medium_native).',
  mn_exclude: 'Leave out', mn_exclude_hint: 'Tick changes to leave out of this deployment, then compare again.', mn_replan: 'Compare again without the ticked ones',
  mn_excluded_now: 'Left out of this deployment', mn_clear_excluded: 'Include everything again', mn_excluded_on: 'left out here',
  mn_dry_run: 'Dry run', mn_deploy: 'Deploy', mn_dry_hint: 'A dry run takes the lock, compares again under it and stops before changing anything.',
  mn_confirm_label: 'Type {0} to confirm', mn_deploy_needs_native: 'Switch the project to the WRM native engine first (Engine above).',
  mn_not_deployable: 'This comparison cannot be deployed natively: it needs a WRM Git source and every destination compared without errors.',
  mn_deploy_confirm: 'Deploy {0} to {1} ({2})?', mn_job_deploy: 'Deployment', mn_job_rollback: 'Rollback', mn_job_dry: 'dry run', mn_running: 'running: {0}',
  mn_cancel: 'Cancel', mn_cancel_hint: 'Stops before the next destination starts changing; a destination already being changed is finished.', mn_cancel_sent: 'Cancel requested',
  'mn_status_ok': 'done', 'mn_status_dry-run': 'dry run OK', mn_status_failed: 'failed', mn_status_mismatch: 'plan changed', mn_status_locked: 'locked', mn_status_skipped: 'skipped',
  mn_sent: '{0} uploaded, {1} deleted', mn_hist_title: 'History and rollback', mn_hist_load: 'Show history', mn_hist_none: 'No deployments yet.',
  mn_h_when: 'When', mn_h_action: 'Action', mn_h_version: 'Version', mn_h_by: 'By', mn_h_server: 'Server', mn_h_note: 'Note', mn_h_changes: 'Changes',
  mn_rb_target: 'A rollback undoes {0} ({1}, {2}) and restores {3}.', mn_rb_first: 'the state before the first deployment', mn_rb_none: 'Nothing to roll back',
  mn_rb_dry: 'Dry rollback', mn_rb_go: 'Roll back', mn_rb_on: 'on {0}', mn_rb_confirm: 'Roll back {0} on {1}?',
  mn_lock_stale: 'older than {0} min, may be released', mn_lock_young: 'may be released after {0} min', mn_lock_release: 'Release lock',
  mn_lock_release_confirm: 'Release the lock of {0} on {1}? Do it only when no deployment is running there.', mn_lock_released: 'Lock released',
});
Object.assign(LANGS.hr, {
  mn_title: 'WRM engine', gm_two_preview: 'WRM-ov vlastiti engine. Pokazuje što bi deploy promijenio na svakom serveru, preko tvojih WRM veza. Projekt prebačen na WRM nativni engine deploya i vraća sam WRM, u istom stanju na serveru koje koristi izvorni alat. Treba mu samo tvoj deploy-config; samo "Usporedi s izvornim alatom" traži da izvorni alat radi.',
  mn_intro: 'WRM čita tvoj deploy-config kakav jest, priprema odabranu granu ili tag iz WRM Git izvora i uspoređuje ga sa svakim odredištem preko WRM veze povezane s tim serverom. Usporedba i probni deploy ništa ne mijenjaju. Projekt prebačen na WRM nativni engine deploya i vraća WRM: lock, backup, prijenos, provjera kontrolnih zbrojeva, post_deploy i povijest, zapisani u formatu izvornog alata, pa svaki alat može nastaviti nakon drugoga.',
  mn_build_note: 'Ovaj projekt ima build. Prije pregleda odaberite gdje se build izvodi: na WRM hostu, na Linux build serveru preko SSH-a ili nikako. Plan prikazuje mjesto builda i provjerava njegovu platformu i arhitekturu prema svakom odredištu.',
  mn_build_where: 'Build', mn_build_choose: 'Odaberite gdje se build izvodi', mn_build_host: 'WRM host', mn_build_server: 'Linux build server', mn_build_none: 'Bez builda',
  mn_build_conn: 'Veza build servera', mn_build_save: 'Spremi mjesto builda', mn_build_saved: 'Mjesto builda spremljeno',
  mn_build_no_conn: 'Dodajte SSH vezu za build server pod Serveri.', mn_build_pick_conn: 'Odaberite vezu build servera.', mn_build_needed: 'Prvo odaberite gdje se build izvodi.',
  mn_build_none_note: 'Bez builda: odabrane datoteke dolaze iz commita.', mn_build_ready_note: 'Plan prikazuje mjesto builda i provjerava njegovu platformu i arhitekturu prema svakom odredištu.',
  mn_build_log: 'Izlaz builda', mn_build_original: 'izvorni alat (on builda)', mn_build_unavailable: 'nije dostupno na ovom WRM hostu',
  mn_engine: 'Engine', mn_engine_docker: 'Izvorni alat (Docker)', mn_engine_native: 'WRM nativno', mn_engine_hint: 'Tko deploya ovaj projekt. Stanje na serveru je isto u oba slučaja, pa se možeš vratiti bilo kad.',
  mn_engine_set: 'Engine spremljen', mn_engine_no_policy: 'Nativni deployevi ti nisu dopušteni (politika medium_native).',
  mn_exclude: 'Izostavi', mn_exclude_hint: 'Označi promjene koje želiš izostaviti iz ovog deploya, pa ponovno usporedi.', mn_replan: 'Ponovno usporedi bez označenih',
  mn_excluded_now: 'Izostavljeno iz ovog deploya', mn_clear_excluded: 'Ponovno uključi sve', mn_excluded_on: 'ovdje izostavljeno',
  mn_dry_run: 'Probni deploy', mn_deploy: 'Deployaj', mn_dry_hint: 'Probni deploy uzima lock, pod njim ponovno uspoređuje i staje prije bilo kakve promjene.',
  mn_confirm_label: 'Upiši {0} za potvrdu', mn_deploy_needs_native: 'Prvo prebaci projekt na WRM nativni engine (Engine iznad).',
  mn_not_deployable: 'Ova usporedba se ne može nativno deployati: treba WRM Git izvor i da se svako odredište usporedilo bez greške.',
  mn_deploy_confirm: 'Deployati {0} na {1} ({2})?', mn_job_deploy: 'Deploy', mn_job_rollback: 'Rollback', mn_job_dry: 'proba', mn_running: 'u tijeku: {0}',
  mn_cancel: 'Otkaži', mn_cancel_hint: 'Zaustavlja prije nego se počne mijenjati sljedeće odredište; odredište koje se upravo mijenja dovršava se.', mn_cancel_sent: 'Otkazivanje zatraženo',
  'mn_status_ok': 'gotovo', 'mn_status_dry-run': 'proba u redu', mn_status_failed: 'nije uspjelo', mn_status_mismatch: 'plan se promijenio', mn_status_locked: 'zaključano', mn_status_skipped: 'preskočeno',
  mn_sent: '{0} preneseno, {1} obrisano', mn_hist_title: 'Povijest i rollback', mn_hist_load: 'Prikaži povijest', mn_hist_none: 'Još nema deployeva.',
  mn_h_when: 'Kada', mn_h_action: 'Radnja', mn_h_version: 'Verzija', mn_h_by: 'Tko', mn_h_server: 'Server', mn_h_note: 'Napomena', mn_h_changes: 'Promjene',
  mn_rb_target: 'Rollback poništava {0} ({1}, {2}) i vraća {3}.', mn_rb_first: 'stanje prije prvog deploya', mn_rb_none: 'Nema ničega za rollback',
  mn_rb_dry: 'Probni rollback', mn_rb_go: 'Napravi rollback', mn_rb_on: 'na {0}', mn_rb_confirm: 'Napraviti rollback {0} na {1}?',
  mn_lock_stale: 'starije od {0} min, smije se otpustiti', mn_lock_young: 'smije se otpustiti nakon {0} min', mn_lock_release: 'Otpusti lock',
  mn_lock_release_confirm: 'Otpustiti lock korisnika {0} na {1}? Samo ako tamo ne radi nijedan deploy.', mn_lock_released: 'Lock otpušten',
});
Object.assign(LANGS.en, {
  mn_eol: 'line endings only', mn_eol_hint: 'Differs from the branch only in line endings (CRLF/LF). The deployment still sends it, so the difference disappears.', mn_eol_n: '{0} of them line endings only',
  mn_details: 'Details', mn_diff_title: 'What the deployment changes', mn_diff_server: 'on the server', mn_diff_source: 'in the branch', mn_diff_counts: '+{0} −{1} lines',
  mn_diff_missing_srv: 'The file is not on the server yet; the deployment adds it.', mn_diff_missing_src: 'The file is not in the branch; the deployment deletes it (it goes into the backup first).',
  mn_diff_binary: 'Binary file: there is no line comparison.', mn_diff_large: 'The file is larger than 512 KiB: there is no line comparison.', mn_diff_none: 'No differences in the lines.',
  mn_diff_eol: 'Only the line endings differ (CRLF on one side, LF on the other). The deployment sends the file and the difference disappears.', mn_diff_symlink: 'Symbolic link → {0}', mn_diff_dir: 'A directory is there.',
  mn_diff_trunc: 'The comparison is shortened to its first 5000 lines.', mn_diff_crlf: 'Line endings: server {0}, branch {1}.',
  mn_cfg_title: 'deploy-config', mn_cfg_open: 'Edit deploy-config', mn_cfg_intro: 'Edit servers.yaml, environments.yaml and the project files here. WRM saves a file only when the whole configuration still loads, and keeps your comments, layout and line endings as they are.',
  mn_cfg_file: 'File', mn_cfg_check: 'Check', mn_cfg_save: 'Save', mn_cfg_new: 'New project', mn_cfg_new_name: 'Name of the new project file (letters, digits, . _ -; without .yaml)',
  mn_cfg_delete: 'Delete project file', mn_cfg_delete_confirm: 'Delete {0}? The project disappears from deploy-config; the servers are not changed.', mn_cfg_ok: 'Valid: {0} servers, {1} environments, {2} projects.',
  mn_cfg_saved: 'Saved; the configuration is valid.', mn_cfg_deleted: 'Deleted.', mn_cfg_not_saved: 'Not saved, the configuration would not load:', mn_cfg_read_only: 'Read only: the policy medium_native does not allow editing deploy-config in WRM.',
  mn_cfg_discard: 'Discard the unsaved changes?', mn_cfg_invalid_now: 'The current configuration does not load:', mn_cfg_crlf: 'This file uses CRLF line endings; WRM keeps them when saving.', mn_cfg_new_note: 'A new project file: save it to create it.',
  gm_op_config_edit: 'deploy-config edit',
});
Object.assign(LANGS.hr, {
  mn_eol: 'samo završeci redaka', mn_eol_hint: 'Od grane se razlikuje samo u završecima redaka (CRLF/LF). Deploy je svejedno šalje, pa razlika nestaje.', mn_eol_n: 'od toga {0} samo završeci redaka',
  mn_details: 'Detalji', mn_diff_title: 'Što deploy mijenja', mn_diff_server: 'na serveru', mn_diff_source: 'u grani', mn_diff_counts: '+{0} −{1} redaka',
  mn_diff_missing_srv: 'Datoteke još nema na serveru; deploy je dodaje.', mn_diff_missing_src: 'Datoteke nema u grani; deploy je briše (prije toga ide u backup).',
  mn_diff_binary: 'Binarna datoteka: nema usporedbe po redcima.', mn_diff_large: 'Datoteka je veća od 512 KiB: nema usporedbe po redcima.', mn_diff_none: 'Nema razlika u redcima.',
  mn_diff_eol: 'Razlikuju se samo završeci redaka (na jednoj strani CRLF, na drugoj LF). Deploy šalje datoteku i razlika nestaje.', mn_diff_symlink: 'Simbolička poveznica → {0}', mn_diff_dir: 'Na tom mjestu je mapa.',
  mn_diff_trunc: 'Usporedba je skraćena na prvih 5000 redaka.', mn_diff_crlf: 'Završeci redaka: server {0}, grana {1}.',
  mn_cfg_title: 'deploy-config', mn_cfg_open: 'Uredi deploy-config', mn_cfg_intro: 'Ovdje uređuješ servers.yaml, environments.yaml i datoteke projekata. WRM sprema datoteku samo ako se cijela konfiguracija i dalje učitava, a komentare, raspored i završetke redaka ostavlja kakve si napisao.',
  mn_cfg_file: 'Datoteka', mn_cfg_check: 'Provjeri', mn_cfg_save: 'Spremi', mn_cfg_new: 'Novi projekt', mn_cfg_new_name: 'Ime nove datoteke projekta (slova, brojke, . _ -; bez .yaml)',
  mn_cfg_delete: 'Obriši datoteku projekta', mn_cfg_delete_confirm: 'Obrisati {0}? Projekt nestaje iz deploy-configa; serveri se ne mijenjaju.', mn_cfg_ok: 'Ispravno: servera {0}, okolina {1}, projekata {2}.',
  mn_cfg_saved: 'Spremljeno; konfiguracija je ispravna.', mn_cfg_deleted: 'Obrisano.', mn_cfg_not_saved: 'Nije spremljeno, konfiguracija se ne bi učitala:', mn_cfg_read_only: 'Samo čitanje: politika medium_native ne dopušta uređivanje deploy-configa u WRM-u.',
  mn_cfg_discard: 'Odbaciti nespremljene izmjene?', mn_cfg_invalid_now: 'Trenutna konfiguracija se ne učitava:', mn_cfg_crlf: 'Ova datoteka koristi CRLF završetke redaka; WRM ih zadržava pri spremanju.', mn_cfg_new_note: 'Nova datoteka projekta: spremi je da se stvori.',
  gm_op_config_edit: 'uređivanje deploy-configa',
});
Object.assign(LANGS.en, {
  gm_two_original: 'Your original deploy tool, run unchanged from its ZIP (in Docker or with Python on the WRM host). It is optional: projects on the WRM native engine are deployed by WRM itself. Set it up with steps 1 to 5 below when you still deploy with it or want to compare; once it runs, it opens below its card.',
  gm_optional: 'Optional: WRM deploys projects on the native engine by itself. You need the original tool only for projects you keep on it and for "Compare with the original tool".',
  gm_s3_native: 'Received and checked by WRM. The WRM engine uses it now; the original tool would apply it at its first start.',
  mn_cfg_start: 'Import your deploy-config ZIP (servers.yaml, environments.yaml and projects/), or write it here in the editor. With DEPLOY_CONFIG_REPO set in the Medium settings, the editor can pull it from Git.',
  mn_cfg_upload: 'Import deploy-config ZIP', mn_build_as: 'as {0}',
  mc_git_title: 'Git', mc_git_off: 'not connected', mc_git_off_hint: 'To edit deploy-config from its repository, set DEPLOY_CONFIG_REPO in the Medium settings and add a WRM Git source for that repository under Git → Sources (a write token there allows commit and push).',
  mc_git_source: 'WRM Git source {0}', mc_git_inactive: 'WRM uses the imported deploy-config. Pull branch {0} to edit WRM\'s own Git copy instead; the imported files stay as they are.',
  mc_git_use: 'Pull from Git and use it', mc_git_active: 'WRM uses its Git copy of branch {0} at', mc_git_pulled: 'pulled {0}', mc_git_behind: 'Git has newer commits ({0})', mc_git_current: 'up to date',
  mc_git_changes: 'Local changes ({0}):', mc_git_a_create: 'new', mc_git_a_update: 'changed', mc_git_a_delete: 'deleted', mc_git_clean: 'No local changes.',
  mc_git_message: 'Commit message', mc_git_message_ph: 'Describe the change (empty: the changed file names)', mc_git_push: 'Commit and push', mc_git_push_hint: 'One commit on the branch through the Git source\'s write token, with you ({0}) as its author when DEPLOY_USER_EMAIL is set in the Medium settings. WRM refuses when one of these files changed on Git meanwhile, and checks that the whole configuration still loads first.',
  mc_git_no_write: 'The Git source has no write token, so the changes cannot be pushed from WRM. Add one under Git → Sources, or discard the changes.',
  mc_git_discard_pull: 'Discard changes and pull', mc_git_pull: 'Pull', mc_git_stop: 'Stop using the Git copy',
  mc_git_discard_confirm: 'Discard the local changes ({0}) and replace them with the branch head?', mc_git_pulled_ok: 'Pulled; the configuration is valid.',
  mc_git_save_first: 'Save or discard the change in the editor first.', mc_git_push_confirm: 'Commit and push the local changes to branch {0}?', mc_git_pushed_ok: 'Committed and pushed.',
  mc_git_stop_confirm: 'Stop using the Git copy? WRM deletes its copy and goes back to the imported deploy-config.', mc_git_stop_confirm_changes: 'Stop using the Git copy? Its local changes are lost: WRM deletes the copy and goes back to the imported deploy-config.', mc_git_stopped: 'WRM uses the imported deploy-config again.',
  gm_op_config_pull: 'deploy-config pull', gm_op_config_push: 'deploy-config push',
});
Object.assign(LANGS.hr, {
  gm_two_original: 'Tvoj izvorni deploy-tool, pokrenut neizmijenjen iz ZIP-a (u Dockeru ili s Pythonom na WRM računalu). Neobavezan je: projekte na WRM nativnom engineu deploya sam WRM. Postavi ga koracima 1 do 5 ispod ako još deployaš s njim ili želiš usporedbu; kad radi, otvara se ispod svoje kartice.',
  gm_optional: 'Neobavezno: projekte na nativnom engineu WRM deploya sam. Izvorni alat trebaš samo za projekte koje držiš na njemu i za "Usporedi s izvornim alatom".',
  gm_s3_native: 'Primljen i provjeren u WRM-u. WRM engine ga već koristi; izvorni alat bi ga primijenio pri prvom pokretanju.',
  mn_cfg_start: 'Uvezi svoj deploy-config ZIP (servers.yaml, environments.yaml i projects/) ili ga napiši ovdje u uređivaču. Ako je u Medium postavkama postavljen DEPLOY_CONFIG_REPO, uređivač ga može povući iz Gita.',
  mn_cfg_upload: 'Uvezi deploy-config ZIP', mn_build_as: 'kao {0}',
  mc_git_title: 'Git', mc_git_off: 'nije povezan', mc_git_off_hint: 'Za uređivanje deploy-configa iz njegovog repozitorija postavi DEPLOY_CONFIG_REPO u Medium postavkama i dodaj WRM Git izvor za taj repozitorij pod Git → Izvori (token za pisanje ondje omogućuje commit i push).',
  mc_git_source: 'WRM Git izvor {0}', mc_git_inactive: 'WRM koristi uvezeni deploy-config. Povuci granu {0} da umjesto toga uređuješ WRM-ovu Git kopiju; uvezene datoteke ostaju kakve jesu.',
  mc_git_use: 'Povuci iz Gita i koristi', mc_git_active: 'WRM koristi svoju Git kopiju grane {0} na', mc_git_pulled: 'povučeno {0}', mc_git_behind: 'Git ima novije commitove ({0})', mc_git_current: 'ažurno',
  mc_git_changes: 'Lokalne izmjene ({0}):', mc_git_a_create: 'nova', mc_git_a_update: 'izmijenjena', mc_git_a_delete: 'obrisana', mc_git_clean: 'Nema lokalnih izmjena.',
  mc_git_message: 'Poruka commita', mc_git_message_ph: 'Opiši izmjenu (prazno: imena izmijenjenih datoteka)', mc_git_push: 'Commit i push', mc_git_push_hint: 'Jedan commit na grani preko tokena za pisanje Git izvora, s tobom ({0}) kao autorom kad je DEPLOY_USER_EMAIL postavljen u Medium postavkama. WRM odbija ako se neka od tih datoteka u međuvremenu promijenila u Gitu, a prije toga provjerava da se cijela konfiguracija i dalje učitava.',
  mc_git_no_write: 'Git izvor nema token za pisanje, pa se izmjene ne mogu pushati iz WRM-a. Dodaj ga pod Git → Izvori ili odbaci izmjene.',
  mc_git_discard_pull: 'Odbaci izmjene i povuci', mc_git_pull: 'Povuci', mc_git_stop: 'Prestani koristiti Git kopiju',
  mc_git_discard_confirm: 'Odbaciti lokalne izmjene ({0}) i zamijeniti ih vrhom grane?', mc_git_pulled_ok: 'Povučeno; konfiguracija je ispravna.',
  mc_git_save_first: 'Najprije spremi ili odbaci izmjenu u uređivaču.', mc_git_push_confirm: 'Commitati i pushati lokalne izmjene na granu {0}?', mc_git_pushed_ok: 'Commitano i pushano.',
  mc_git_stop_confirm: 'Prestati koristiti Git kopiju? WRM briše svoju kopiju i vraća se na uvezeni deploy-config.', mc_git_stop_confirm_changes: 'Prestati koristiti Git kopiju? Njezine lokalne izmjene se gube: WRM briše kopiju i vraća se na uvezeni deploy-config.', mc_git_stopped: 'WRM opet koristi uvezeni deploy-config.',
  gm_op_config_pull: 'povlačenje deploy-configa', gm_op_config_push: 'push deploy-configa',
});
(function () {
  const G = {state: null, tab: 'overview', f: {state: '', env: '', folder: '', dir: '', tag: '', q: '', app: '', conn: ''}, onlyChanges: true, busy: false, sel: new Set(),
    checkVMD: (() => { try { return localStorage.getItem('wrm_git_check_vmd') === '1'; } catch (_) { return false; } })()};
  const modeKey = () => 'wrm_git_mode:' + ((typeof currentUser !== 'undefined' && currentUser && (currentUser.id || currentUser.username)) || '');
  G.mode = (() => { try { return ['simple', 'medium'].includes(localStorage.getItem(modeKey())) ? localStorage.getItem(modeKey()) : 'advanced'; } catch (_) { return 'advanced'; } })();
  const $ = id => document.getElementById(id);
  const tf = (k, vars) => { let s = t(k); for (const [a, b] of Object.entries(vars || {})) s = s.split('{' + a + '}').join(b); return s; };
  const lines = s => String(s || '').split(/[\n,]/).map(x => x.trim()).filter(Boolean);
  const json = (method, body) => ({method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)});

  const css = `
#git-ws { position: fixed; inset: 0; z-index: 8800; background: var(--bg); color: var(--text); display: flex; flex-direction: column; font-size: 13px; }
#git-ws .g-head { display: flex; align-items: center; gap: 10px; padding: 10px 16px; border-bottom: 1px solid var(--border); background: var(--bg2); flex-wrap: wrap; }
#git-ws #medium-status {overflow-wrap:anywhere;color:var(--text2);margin:2px 0 8px;}
#git-ws .gm-two {display:grid;grid-template-columns:1fr 1fr;gap:12px;}
#git-ws .gm-two p {margin:4px 0 0;color:var(--text2);}
@media (max-width: 700px) { #git-ws .gm-two {grid-template-columns:1fr;} #git-ws .gm-log td:nth-child(5), #git-ws .gm-log th:nth-child(5) {display:none;} }
#git-ws .gm-head {display:flex;align-items:center;gap:10px;flex-wrap:wrap;}
#git-ws .gm-head h3 {margin:0;flex:1;}
#git-ws summary.gm-head {cursor:pointer;list-style:none;}
#git-ws summary.gm-head::-webkit-details-marker {display:none;}
#git-ws summary.gm-head::before {content:'▸';color:var(--text2);}
#git-ws details[open] > summary.gm-head::before {content:'▾';}
#git-ws .gm-light {display:inline-flex;align-items:center;gap:6px;padding:2px 10px;border-radius:999px;font-size:12px;font-weight:600;background:var(--bg3);white-space:nowrap;}
#git-ws .gm-light i {width:9px;height:9px;border-radius:50%;background:var(--text3);display:inline-block;}
#git-ws .gm-light.gm-ok i {background:var(--green);box-shadow:0 0 6px var(--green);} #git-ws .gm-light.gm-ok {color:var(--green);}
#git-ws .gm-light.gm-run i {background:var(--yellow);animation:gmPulse 1.2s infinite;} #git-ws .gm-light.gm-run {color:var(--yellow);}
#git-ws .gm-light.gm-bad i {background:var(--red);} #git-ws .gm-light.gm-bad {color:var(--red);}
@keyframes gmPulse {50% {opacity:.3;}}
#git-ws .gm-steps {list-style:none;margin:8px 0;padding:0;display:flex;flex-direction:column;gap:6px;}
#git-ws .gm-steps > li {border:1px solid var(--border);border-radius:8px;padding:8px 10px;background:var(--bg2);}
#git-ws .gm-steps > li.gm-ok {border-left:4px solid var(--green);} #git-ws .gm-steps > li.gm-run, #git-ws .gm-steps > li.gm-pend {border-left:4px solid var(--yellow);} #git-ws .gm-steps > li.gm-bad {border-left:4px solid var(--red);} #git-ws .gm-steps > li.gm-wait {border-left:4px solid var(--accent);} #git-ws .gm-steps > li.gm-opt {border-left:4px solid var(--border);}
#git-ws .gm-sh {display:flex;align-items:center;gap:8px;flex-wrap:wrap;}
#git-ws .gm-sh b {flex:1;min-width:150px;}
#git-ws .gm-ico {width:18px;text-align:center;font-weight:700;}
#git-ws .gm-act {display:flex;gap:6px;flex-wrap:wrap;}
#git-ws .gm-sd {color:var(--text2);margin:4px 0 0 26px;overflow-wrap:anywhere;}
#git-ws .gm-todo {margin-top:4px;color:var(--text);font-weight:500;}
#git-ws .gm-todo::before {content:'→ ';}
#git-ws .gm-tl {list-style:none;margin:6px 0;padding:0;}
#git-ws .gm-tl li {padding:2px 0;}
#git-ws .gm-tl li.gm-error {color:var(--red);} #git-ws .gm-tl li.gm-ok > span:first-child {color:var(--green);}
#git-ws .gm-err {color:var(--red);white-space:pre-wrap;overflow-wrap:anywhere;margin:2px 0 2px 18px;}
#git-ws .gm-bad {border-color:var(--red);}
#git-ws .gm-out {max-height:300px;overflow:auto;background:var(--bg);border:1px solid var(--border);border-radius:6px;padding:8px;font-size:11.5px;white-space:pre-wrap;overflow-wrap:anywhere;margin:6px 0 0;}
#git-ws .gm-cmd {background:var(--bg);border:1px solid var(--border);border-radius:6px;padding:6px 8px;font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere;margin:4px 0 0;user-select:all;}
#git-ws .gm-log {width:100%;border-collapse:collapse;}
#git-ws .gm-log th {text-align:left;color:var(--text3);font-weight:500;padding:4px 8px 4px 0;}
#git-ws .gm-log td {padding:5px 8px 5px 0;border-top:1px solid var(--border);vertical-align:top;overflow-wrap:anywhere;}
#git-ws .gm-log tr.gm-click {cursor:pointer;} #git-ws .gm-log tr.gm-click:hover td {background:var(--bg3);}
#git-ws .gm-quick > summary {cursor:pointer;font-weight:600;margin-top:6px;}
#git-ws #mn-card .s-enew { background: rgba(var(--green-rgb),.14); color: var(--green); }
#git-ws #mn-card .s-echanged, #git-ws #mn-card .s-pending { background: rgba(var(--yellow-rgb),.14); color: var(--yellow); }
#git-ws #mn-card .s-eremoved, #git-ws #mn-card .s-error { background: rgba(var(--red-rgb),.14); color: var(--red); white-space: normal; }
#git-ws #mn-card .s-differs { background: rgba(var(--purple-rgb),.16); color: var(--purple); }
#git-ws #mn-card table { width: auto; max-width: 100%; }
#git-ws #mn-card td { padding: 4px 12px 4px 0; vertical-align: top; overflow-wrap: anywhere; }
#git-ws #mn-card h4 { margin: 16px 0 6px; }
#git-ws #mn-card .mn-dest { display: inline-flex; gap: 6px; align-items: center; margin-right: 14px; }
#git-ws #mn-card > summary { cursor: pointer; }
#git-ws label.btn-sec {display:inline-flex;align-items:center;padding:6px 10px;border:1px solid var(--border);border-radius:6px;cursor:pointer;background:var(--bg3);}
#git-ws .g-head h2 { font-size: 15px; margin: 0; white-space: nowrap; }
#git-ws .g-tabs { display: flex; gap: 4px; flex-wrap: wrap; }
#git-ws .g-tabs button { background: transparent; border: 1px solid transparent; color: var(--text2); padding: 6px 12px; border-radius: 8px; cursor: pointer; }
#git-ws .g-tabs button.on { background: var(--accent-d); border-color: var(--accent); color: var(--text); }
#git-ws .g-sp { flex: 1; }
#git-ws .g-body { flex: 1; overflow: auto; padding: 14px 16px 40px; }
#git-ws .g-intro { padding: 8px 16px; font-size: 12px; line-height: 1.5; color: var(--text2); background: var(--bg2); border-bottom: 1px solid var(--border); overflow-wrap: anywhere; }
#git-ws .g-intro::before { content: 'ℹ '; color: var(--accent); }
#git-ws .g-bar:has(>input[type=search]), #git-ws .gs-filters, #git-modal .g-bar:has(>input[type=search]),
#git-ws .g-sticky { position:sticky;top:0;z-index:20;background:var(--bg2);padding:8px 0;max-height:40vh;overflow-y:auto; }
#git-ws .g-bar { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; margin: 0 0 12px; }
#git-ws .g-bar select, #git-ws .g-bar input { width: auto; min-width: 120px; flex: 0 1 auto; }
#git-ws .g-control-stack {position:sticky;top:0;z-index:25;background:var(--bg2);padding:8px;border-bottom:1px solid var(--border);max-height:48vh;overflow:auto;}
#git-ws .g-control-stack .g-bar {position:static!important;max-height:none;overflow:visible;margin-bottom:6px;}
#git-ws .g-card { background: var(--bg2); border: 1px solid var(--border); border-radius: 10px; padding: 12px 14px; margin-bottom: 14px; }
#git-ws .g-card h3 { margin: 0 0 10px; font-size: 13px; }
#git-ws .g-scroll { overflow: auto; max-width: 100%; }
#git-ws table { border-collapse: collapse; width: 100%; font-size: 12px; }
#git-ws th { text-align: left; font-size: 10px; text-transform: uppercase; letter-spacing: .05em; color: var(--text3); padding: 6px 8px; border-bottom: 1px solid var(--border); white-space: nowrap; }
#git-ws td { padding: 6px 8px; border-bottom: 1px solid var(--border); vertical-align: top; }
#git-ws .g-matrix td.g-cell { min-width: 150px; }
#git-ws .g-inst { display: block; border-radius: 7px; padding: 4px 7px; margin: 2px 0; cursor: pointer; border: 1px solid var(--border2); background: var(--bg3); }
#git-ws .g-inst:hover { border-color: var(--accent); }
#git-ws .g-st { display: inline-block; font-size: 10.5px; padding: 1px 7px; border-radius: 10px; font-weight: 600; white-space: nowrap; }
#git-ws .s-ok { background: rgba(var(--green-rgb),.14); color: var(--green); }
#git-ws .s-update, #git-ws .s-old, #git-ws .s-missing { background: rgba(var(--yellow-rgb),.14); color: var(--yellow); }
#git-ws .s-review, #git-ws .s-modified, #git-ws .s-error { background: rgba(var(--red-rgb),.14); color: var(--red); }
#git-ws .s-gone, #git-ws .s-unknown, #git-ws .s-extra, #git-ws .s-protected { background: var(--bg4); color: var(--text2); }
#git-ws .g-env { font-size: 10px; color: var(--purple); margin-left: 4px; }
#git-ws .g-mut { color: var(--text3); font-size: 11px; }
#git-ws .g-opt { display: inline-flex; gap: 5px; align-items: center; font-size: 11.5px; color: var(--text2); white-space: nowrap; }
#git-ws .g-opt input { width: auto; min-width: 0; }
#git-ws .g-mono { font-family: var(--font-mono); font-size: 11.5px; word-break: break-all; }
#git-ws .g-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 10px; }
#git-ws .g-grid label { display: block; font-size: 11px; color: var(--text2); margin-bottom: 3px; }
#git-ws textarea { width: 100%; min-height: 54px; font-family: inherit; }
#git-ws .g-conns { max-height: 260px; overflow: auto; border: 1px solid var(--border); border-radius: 8px; padding: 6px; }
#git-ws .g-conns label { display: flex; gap: 6px; align-items: center; padding: 2px 4px; }
#git-ws .g-conns input { width: auto; }
#git-modal { position: fixed; inset: 0; z-index: 8900; background: rgba(0,0,0,.55); display: flex; align-items: flex-start; justify-content: center; padding: 30px 12px; overflow: auto; }
#git-modal .g-dlg { min-width: 0; background: var(--bg2); border: 1px solid var(--border2); border-radius: 12px; width: min(980px, 100%); padding: 16px 18px; }
#git-modal .g-dlg h3 { margin: 0 0 10px; font-size: 14px; word-break: break-all; }
#git-modal .g-diff { font-family: var(--font-mono); font-size: 11.5px; background: var(--bg); border: 1px solid var(--border); border-radius: 8px; overflow: auto; max-height: 60vh; }
#git-modal .g-diff div { white-space: pre; padding: 0 8px; }
#git-modal .g-diff .d-add { background: rgba(var(--green-rgb),.13); color: #bbf7d0; }
#git-modal .g-diff .d-del { background: rgba(var(--red-rgb),.13); color: var(--red-t); }
#git-modal .g-diff .d-hunk { background: var(--accent-d); color: var(--text2); }
#git-modal .g-scroll { overflow:auto; max-width:100%; }
#git-modal table { border-collapse: collapse; width: 100%; font-size: 12px; }
#git-modal td, #git-modal th { padding: 4px 6px; border-bottom: 1px solid var(--border); text-align: left; }
#git-modal .g-st { display: inline-block; font-size: 10.5px; padding: 1px 7px; border-radius: 10px; font-weight: 600; }
#git-modal .s-ok { background: rgba(var(--green-rgb),.14); color: var(--green); }
#git-modal .s-old, #git-modal .s-missing, #git-modal .s-update { background: rgba(var(--yellow-rgb),.14); color: var(--yellow); }
#git-modal .s-modified, #git-modal .s-review, #git-modal .s-error { background: rgba(var(--red-rgb),.14); color: var(--red); }
#git-modal .s-extra, #git-modal .s-outside, #git-modal .s-protected, #git-modal .s-gone, #git-modal .s-unknown { background: var(--bg4); color: var(--text2); }
#git-modal .g-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 10px; }
#git-modal .g-grid label { display: block; font-size: 11px; color: var(--text2); margin-bottom: 3px; }
#git-modal textarea { width: 100%; min-height: 48px; font-family: inherit; }
#git-modal .g-btns { display: flex; gap: 8px; justify-content: flex-end; margin-top: 12px; flex-wrap: wrap; }
#git-modal .g-btns .g-back { margin-right: auto; }
#git-modal .g-backbar { display: flex; margin-bottom: 8px; }
#git-ws input:not([type=checkbox]), #git-ws select, #git-ws textarea, #git-modal input:not([type=checkbox]), #git-modal select, #git-modal textarea {
  background: var(--bg3); color: var(--text); border: 1px solid var(--border2); border-radius: 7px; padding: 6px 9px; font-size: 12.5px; box-sizing: border-box; outline: none; }
#git-ws .g-grid input, #git-ws .g-grid select, #git-modal .g-grid input, #git-modal .g-grid select { width: 100%; }
#git-ws input:focus, #git-ws select:focus, #git-ws textarea:focus, #git-modal input:focus, #git-modal select:focus, #git-modal textarea:focus { border-color: var(--accent); }
#git-modal .g-mut { color: var(--text3); font-size: 11px; }
#git-modal .g-mono { font-family: var(--font-mono); font-size: 11.5px; word-break: break-all; }
#git-modal .g-env { font-size: 10px; color: var(--purple); margin-left: 6px; }
#git-modal .g-card { border: 1px solid var(--border); border-radius: 9px; padding: 10px 12px; margin: 10px 0; }
#git-modal .g-card h3 { margin: 0 0 6px; font-size: 12.5px; }
#git-modal .g-card > label:not(.g-opt), #git-modal .g-conf label { display: block; font-size: 11px; color: var(--text2); margin-bottom: 3px; }
#git-modal .g-opt > span { flex: 1 1 0; min-width: 160px; }
#git-modal .g-conf { margin-top: 10px; }
#git-modal .g-conf input { width: min(320px, 100%); }
#git-ws .g-pend, #git-modal .g-pend { font-size: 10px; color: var(--yellow); margin-left: 4px; white-space: nowrap; }
#git-ws .g-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
#git-modal .g-item { border: 1px solid var(--border); border-radius: 9px; padding: 10px 12px; margin: 10px 0; background: var(--bg3); }
#git-modal .g-item h4 { margin: 0 0 6px; font-size: 12.5px; word-break: break-all; }
#git-modal .g-prod { background: rgba(var(--red-rgb),.16); color: var(--red); font-size: 10px; padding: 1px 6px; border-radius: 9px; font-weight: 700; text-transform: uppercase; margin-left: 4px; }
#git-modal .g-hideok .g-okrow { display: none; }
#git-modal .g-files { max-height: 32vh; overflow: auto; }
#git-modal .g-opt { display: flex; gap: 6px; align-items: center; margin: 4px 0; flex-wrap: wrap; }
#git-modal .g-opt input[type=radio], #git-modal .g-opt input[type=checkbox], #git-modal td input[type=checkbox], #git-modal td input[type=radio] { width: auto; }
#git-modal .g-log { font-size: 11.5px; margin: 6px 0 0; padding: 0; list-style: none; }
#git-modal .g-log li { padding: 2px 0; border-top: 1px dashed var(--border); word-break: break-word; white-space: pre-wrap; }
#git-modal .s-done, #git-modal .s-rs-ok { background: rgba(var(--green-rgb),.14); color: var(--green); }
#git-modal .s-failed, #git-modal .s-rolled_back { background: rgba(var(--red-rgb),.14); color: var(--red); }
#git-modal .s-scheduled, #git-modal .s-running, #git-modal .s-starting, #git-modal .s-pending, #git-modal .s-changed { background: rgba(var(--yellow-rgb),.14); color: var(--yellow); }
#git-modal .s-cancelled, #git-modal .s-skipped, #git-modal .s-interrupted { background: var(--bg4); color: var(--text2); }
#git-ws .s-done, #git-ws .s-rs-ok { background: rgba(var(--green-rgb),.14); color: var(--green); }
#git-ws .s-failed, #git-ws .s-rolled_back { background: rgba(var(--red-rgb),.14); color: var(--red); }
#git-ws .s-scheduled, #git-ws .s-running, #git-ws .s-starting, #git-ws .s-pending { background: rgba(var(--yellow-rgb),.14); color: var(--yellow); }
#git-ws .s-cancelled, #git-ws .s-skipped, #git-ws .s-interrupted { background: var(--bg4); color: var(--text2); }
#git-ws .g-diff { font-family: var(--font-mono); font-size: 11.5px; background: var(--bg); border: 1px solid var(--border); border-radius: 8px; overflow: auto; max-height: 50vh; }
#git-ws .g-diff div { white-space: pre; padding: 0 8px; }
#git-ws .g-diff .d-add { background: rgba(var(--green-rgb),.13); color: var(--green); }
#git-ws .g-diff .d-del { background: rgba(var(--red-rgb),.13); color: var(--red); }
#git-ws .g-diff .d-hunk { color: var(--text3); }
#git-ws .g-pats { display: flex; flex-wrap: wrap; gap: 4px 12px; }
#git-ws .g-pats label { display: inline-flex; gap: 5px; align-items: center; font-family: var(--font-mono); font-size: 11.5px; }
#git-ws .g-pats input, #git-ws td input[type=checkbox] { width: auto; }
#git-ws .g-warn { border-color: var(--red); }
#git-ws pre.g-pre { background: var(--bg); border: 1px solid var(--border); border-radius: 8px; padding: 8px; max-height: 30vh; overflow: auto; font-size: 11.5px; margin: 0; white-space: pre-wrap; word-break: break-all; }
@media (max-width: 640px) { #git-ws .g-head { padding: 8px 10px; } #git-ws .g-body { padding: 10px; } #git-ws .g-intro { padding: 6px 10px; } #git-ws .g-bar select, #git-ws .g-bar input { min-width: 0; flex: 1 1 140px; } }
#git-ws .g-cb, #git-modal .g-cb { width: auto; margin: 0 5px 0 0; vertical-align: middle; cursor: pointer; }
#git-ws .g-inst.sel { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent); }
#git-ws .g-inst .g-ago { font-size: 10.5px; color: var(--text3); }
#git-ws .g-cellbtn { background: transparent; border: 1px dashed var(--border2); color: var(--text2); border-radius: 6px; padding: 1px 7px; font-size: 11px; margin-top: 2px; cursor: pointer; }
#git-ws .g-cellbtn:hover { border-color: var(--accent); color: var(--text); }
#git-ws .g-cellbtn:disabled { opacity: .4; cursor: default; }
#git-ws .g-prog { display: inline-flex; gap: 6px; align-items: center; color: var(--text2); font-size: 12px; white-space: nowrap; }
.g-spin { display: inline-block; width: 10px; height: 10px; border: 2px solid var(--border2); border-top-color: var(--accent); border-radius: 50%; animation: g-spin .8s linear infinite; vertical-align: -1px; }
@keyframes g-spin { to { transform: rotate(360deg); } }
#git-ws .g-q { display: inline-flex; gap: 5px; align-items: center; font-size: 10.5px; color: var(--yellow); font-weight: 600; }
#git-ws .g-hostname { font-size: 10.5px; color: var(--text3); font-weight: 400; }
#git-ws .g-checkbar { padding: 6px 8px; border: 1px solid var(--border); border-radius: 9px; background: var(--bg2); }
#git-ws .g-checkbar select { min-width: 0; }
#git-modal .g-seg { display: inline-flex; border: 1px solid var(--border2); border-radius: 8px; overflow: hidden; flex-wrap: wrap; }
#git-modal .g-seg button { background: transparent; color: var(--text2); border: 0; border-right: 1px solid var(--border2); border-radius: 0; padding: 4px 10px; font-size: 12px; }
#git-modal .g-seg button:last-child { border-right: 0; }
#git-modal .g-seg button.on { background: var(--accent-d); color: var(--text); }
#git-modal .g-flist { max-height: 45vh; overflow: auto; border: 1px solid var(--border); border-radius: 8px; }
#git-modal .g-flist table td { vertical-align: top; }
#git-modal .g-flist .g-why { color: var(--text2); font-size: 11px; }
#git-modal .g-flist .g-pat { font-family: var(--font-mono); }
#git-modal .g-untracked td:first-child { opacity: .85; }
#git-modal .s-same { background: rgba(var(--green-rgb),.10); color: var(--green); }
#git-modal .s-differs { background: rgba(var(--purple-rgb),.16); color: var(--purple); }
#git-modal .s-symlink, #git-modal .s-unknown { background: var(--bg4); color: var(--text2); }
#git-modal .s-unreadable { background: rgba(var(--red-rgb),.10); color: var(--red); }
#git-modal .g-diff .d-add { color: var(--green); }
#git-modal .g-diff .d-del { color: var(--red); }
#git-modal .g-split { table-layout: fixed; width: 100%; border-collapse: collapse; font-family: var(--font-mono); font-size: 11.5px; }
#git-modal .g-split td { white-space: pre-wrap; word-break: break-all; padding: 0 6px; border: 0; vertical-align: top; }
#git-modal .g-split td.ln { width: 3.4em; color: var(--text3); text-align: right; user-select: none; padding: 0 4px; }
#git-modal .g-split td.d-del { background: rgba(var(--red-rgb),.13); color: var(--red); }
#git-modal .g-split td.d-add { background: rgba(var(--green-rgb),.13); color: var(--green); }
#git-modal .g-split td.d-none { background: var(--bg3); }
#git-modal .g-split tr.d-hunk td { background: var(--accent-d); color: var(--text2); }
#git-modal pre.g-code { background: var(--bg); border: 1px solid var(--border); border-radius: 8px; padding: 8px; max-height: 60vh; overflow: auto; font-size: 11.5px; margin: 0; white-space: pre; tab-size: 4; }
#git-modal .g-dhead { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin: 12px 0 6px; }
#git-modal .g-dhead h3 { margin: 0; flex: 1 1 200px; }
#git-modal .g-commits table td { white-space: normal; }
#git-modal .s-enew, #git-modal .s-echanged { background: rgba(var(--yellow-rgb),.14); color: var(--yellow); }
#git-modal .s-eeol { background: rgba(var(--purple-rgb),.16); color: var(--purple); }
#git-modal .s-esame { background: rgba(var(--green-rgb),.10); color: var(--green); }
#git-modal .s-eremoved { background: rgba(var(--red-rgb),.14); color: var(--red); }
#git-modal .s-eextra, #git-modal .s-eprotected, #git-modal .s-eignored { background: var(--bg4); color: var(--text2); }
#git-modal .g-cellbtn { background: transparent; border: 1px dashed var(--border2); color: var(--text2); border-radius: 6px; padding: 1px 7px; font-size: 11px; cursor: pointer; }
#git-modal .g-cellbtn:hover { border-color: var(--accent); color: var(--text); }
#git-modal .g-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
#git-modal .g-bar { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
#git-modal details summary { cursor: pointer; }
#git-ws .g-envcell .g-inst { cursor: default; }
#git-ws .g-seg { display: inline-flex; border: 1px solid var(--border2); border-radius: 8px; overflow: hidden; flex-wrap: wrap; }
#git-ws .g-seg button { background: transparent; color: var(--text2); border: 0; border-right: 1px solid var(--border2); border-radius: 0; padding: 5px 11px; font-size: 12px; cursor: pointer; }
#git-ws .g-seg button:last-child { border-right: 0; }
#git-ws .g-seg button.on { background: var(--accent-d); color: var(--text); }
#git-ws .g-wide { grid-template-columns: repeat(auto-fit, minmax(min(100%, 360px), 1fr)); }
#git-ws .g-wide .g-card { margin-bottom: 0; }
#git-ws tr.g-click { cursor: pointer; }
#git-ws tr.g-click:hover td { background: var(--bg3); }
#git-ws .s-kcommit { background: var(--accent-d); color: var(--text); }
#git-ws .s-kpush { background: rgba(var(--purple-rgb),.16); color: var(--purple); }
#git-ws tr.g-drift td { background: rgba(var(--red-rgb),.06); }
#git-ws tr.g-drift td:first-child { box-shadow: inset 3px 0 0 var(--red); }
#git-ws td a { color: var(--accent); text-decoration: none; }
#git-ws td a:hover { text-decoration: underline; }
#git-ws .g-bar input[type=date] { min-width: 0; }
#git-ws td.g-nw, #git-ws .g-nw { white-space: nowrap; word-break: normal; }
#git-ws td.g-path { min-width: 180px; }
#git-ws .g-date { display: inline-flex; gap: 6px; align-items: center; }
@media (max-width: 640px) { #git-ws .g-date { flex: 1 1 170px; } #git-ws .g-date input { flex: 1 1 auto; } }
#git-ws .gs-wrap { max-width: 980px; margin: 0 auto; }
#git-ws .gs-card { padding: 14px 16px; }
#git-ws .gs-card h3 { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: 16px; margin: 0 0 10px; }
#git-ws .gs-big { margin-left: auto; font-size: 12.5px; padding: 3px 12px; }
#git-ws .gs-cols { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 280px), 1fr)); gap: 10px; }
#git-ws .gs-box { border: 1px solid var(--border); border-radius: 8px; padding: 10px 12px; background: var(--bg); min-width: 0; overflow-wrap: anywhere; }
#git-ws .gs-box h4 { margin: 0 0 6px; font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: var(--text2); }
#git-ws .gs-ver { font-size: 18px; font-weight: 600; }
#git-ws .gs-acts { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 12px; }
#git-ws .gs-acts button, #git-modal .gs-go { font-size: 14px; padding: 9px 18px; }
#git-ws .gs-acts button.gs-primary { min-width: 160px; }
#git-modal .gs-destinations { border: 1px solid var(--border); border-radius: 8px; padding: 8px 10px; margin: 8px 0; }
#git-modal .gs-destinations > b { display: block; margin-bottom: 5px; }
#git-modal .gs-destinations .g-opt { margin: 2px 0; }
#git-modal .gs-file-review td:nth-child(2) { min-width: 140px; }
#git-modal .gs-from-to { font-size: 15px; margin: 8px 0; }
@media (max-width: 1280px) { #git-ws .g-tabs button { padding: 6px 8px; } #git-ws .g-head { gap: 8px; } }
@media (max-width: 640px) { #git-modal { padding: 8px 4px; } #git-modal .g-dlg { padding: 12px 10px; } #git-modal .g-hm { display: none; } #git-modal .g-flist { max-height: 55vh; } }
`;

  function stBadge(s) { return `<span class="g-st s-${esc(s)}">${esc(t('git_st_' + s))}</span>`; }
  function fsBadge(s) { return `<span class="g-st s-${esc(s)}">${esc(t('git_fs_' + s))}</span>`; }

  // ── folder filters (as in the log viewer): a tree of the folders of the given paths, each
  // with the number of items below it; a folder matches itself and everything under it ──
  const segsOf = p => String(p || '').split('/').filter(Boolean);
  function dirOptions(paths, cur, leafIsFolder) {
    const count = new Map();
    for (const p of paths) {
      const sg = segsOf(p), lead = String(p || '').startsWith('/') ? '/' : '';
      for (let k = 1; k <= (leafIsFolder ? sg.length : sg.length - 1); k++) { const key = lead + sg.slice(0, k).join('/'); count.set(key, (count.get(key) || 0) + 1); }
    }
    const keys = [...count.keys()].sort((a, b) => {
      const x = segsOf(a), y = segsOf(b);
      for (let k = 0; k < Math.min(x.length, y.length); k++) { const c = x[k].localeCompare(y[k], undefined, {sensitivity: 'base'}); if (c) return c; }
      return x.length - y.length;
    });
    return keys.slice(0, 1500).map(k => { const sg = segsOf(k), d = sg.length - 1;
      return `<option value="${esc(k)}" ${k === cur ? 'selected' : ''}>${d ? '\u00a0\u00a0'.repeat(d) + '└ ' + esc(sg[d]) : esc(k)} (${count.get(k)})</option>`; }).join('');
  }
  const inDir = (p, d) => !d || p === d || String(p || '').startsWith(d.replace(/\/$/, '') + '/');
  const dirSelect = (onchange, paths, cur, leafIsFolder, id) => `<select ${id ? `id="${id}" ` : ''}aria-label="${esc(t('git_all_dirs'))}" title="${esc(t('git_dir_filter_h'))}" onchange="${onchange}">
    <option value="">${esc(t('git_all_dirs'))}</option>${dirOptions(paths, cur, leafIsFolder)}</select>`;
  // connection folders: the folder tree of WRM, a folder matches its subfolders too
  const connFolderPath = id => { const out = []; let f = (folders || []).find(x => x.id === id); for (let n = 0; f && n < 50; n++) { out.unshift(f); f = folders.find(x => x.id === f.parent_id); } return out; };
  const inConnFolder = (fid, want) => !want || connFolderPath(fid).some(f => String(f.id) === String(want));
  function connFolderOptions(fids, cur) {
    const count = new Map(), byId = new Map();
    for (const id of fids) for (const f of connFolderPath(id)) { byId.set(f.id, f); count.set(f.id, (count.get(f.id) || 0) + 1); }
    const sortKey = id => connFolderPath(id).map(f => f.name);
    const ids = [...byId.keys()].sort((a, b) => { const x = sortKey(a), y = sortKey(b);
      for (let k = 0; k < Math.min(x.length, y.length); k++) { const c = x[k].localeCompare(y[k], undefined, {sensitivity: 'base'}); if (c) return c; } return x.length - y.length; });
    return ids.map(id => { const d = connFolderPath(id).length - 1;
      return `<option value="${id}" ${String(cur) === String(id) ? 'selected' : ''}>${d ? '\u00a0\u00a0'.repeat(d) + '└ ' : ''}${esc(byId.get(id).name)} (${count.get(id)})</option>`; }).join('');
  }

  async function load() {
    const r = await fetch('/api/git');
    if (!r.ok) { showToast(await apiError(r), 'error'); return false; }
    G.state = await r.json();
    return true;
  }

  function open() {
    if (!$('git-style')) { const s = document.createElement('style'); s.id = 'git-style'; s.textContent = css; document.head.appendChild(s); }
    let ws = $('git-ws');
    if (!ws) {
      ws = document.createElement('div');
      ws.id = 'git-ws';
      ws.innerHTML = `<div class="g-head"><h2>⎇ ${esc(t('git_title'))}</h2><div class="g-tabs" id="git-tabs"></div><span class="g-sp"></span>
        <span class="g-seg" id="git-mode" title="${esc(t('gs_mode_h'))}"></span><span class="g-mut" id="git-last"></span><button id="git-check" onclick="gitWorkspace.check()">${esc(t('git_check_now'))}</button>
        <button class="btn-sec" onclick="gitWorkspace.close()" title="${esc(t('git_close'))}">✕</button></div><div class="g-intro" id="git-intro"></div><div class="g-body" id="git-body"></div>`;
      document.body.appendChild(ws);
      document.addEventListener('keydown', e => { if (e.key === 'Escape' && $('git-ws') && !$('git-modal')) close(); });
    }
    ws.style.display = 'flex';
    load().then(ok => { if (ok) { render(); resumeJob(); } });
  }

  function close() { const ws = $('git-ws'); if (ws) ws.style.display = 'none'; closeModal(); }

  function render() {
    if(!$('git-ws')){paintFunctionality();return;}
    const s = G.state;
    $('git-mode').innerHTML = ['simple', 'medium', 'advanced'].map(m => `<button class="${G.mode === m ? 'on' : ''}" onclick="gitWorkspace.setGitMode('${m}')">${esc(t('gs_' + m))}</button>`).join('');
    if (G.mode === 'medium') {
      $('git-tabs').innerHTML = '';
      $('git-intro').style.display = 'none';
      $('git-last').style.display = $('git-check').style.display = 'none';
      paintMedium(); return;
    }
    if (G.mode === 'simple') {
      $('git-tabs').innerHTML = '';
      $('git-intro').style.display = 'none';
      $('git-last').style.display = $('git-check').style.display = 'none';
      paintSimple();
      return;
    }
    $('git-last').style.display = $('git-check').style.display = '';
    $('git-tabs').innerHTML = ['overview', 'services', 'installs', 'envs', 'runs', 'activity', 'gitignore', 'settings'].map(k =>
      `<button class="${G.tab === k ? 'on' : ''}" title="${esc(t('gtd_' + k))}" onclick="gitWorkspace.tab('${k}')">${esc(t('git_tab_' + k))}</button>`).join('');
    $('git-intro').style.display = '';
    $('git-intro').textContent = t('gtd_' + G.tab);
    $('git-last').textContent = `${t('git_last_check')}: ${s.last_check_at ? fmtTime(s.last_check_at) : t('git_never')}`;
    $('git-check').disabled = !s.can_check || G.busy || jobRunning();
    $('git-check').title = s.can_check ? '' : t('git_no_checks');
    $('git-check').textContent = G.busy || jobRunning() ? t('git_checking') : t('git_check_now');
    const body = $('git-body');
    if (G.tab === 'overview') body.innerHTML = overviewHTML();
    else if (G.tab === 'services') body.innerHTML = servicesHTML();
    else if (G.tab === 'installs') body.innerHTML = installsHTML();
    else if (G.tab === 'functionality') body.innerHTML = functionalityHTML();
    else if (G.tab === 'runs') body.innerHTML = runsHTML();
    else if (G.tab === 'envs') { body.innerHTML = envsHTML(); if (E.ov) envCells(); }
    else if (G.tab === 'activity') body.innerHTML = activityHTML();
    else if (G.tab === 'gitignore') body.innerHTML = ignoreHTML();
    else body.innerHTML = settingsHTML();
    if(document.getElementById('monitor-service-view'))paintFunctionality();
  }

  function setTab(k) { G.tab = k; render(); if (k === 'runs') loadRuns(); if (k === 'envs' && !E.ov) envLoad(); if (k === 'gitignore' && !I.info && !I.loading) ignoreLoad(); if (k === 'activity') actStart(); }

  // ── overview ──
  function filtered() {
    const f = G.f, q = f.q.toLowerCase();
    return G.state.installs.filter(i => (!f.state || i.state === f.state) && (!f.env || i.env === f.env) &&
      (!f.folder || inConnFolder(i.folder_id, f.folder)) && inDir(i.path, f.dir) && (!f.tag || (i.tags || []).includes(f.tag)) &&
      (!f.app || i.app === f.app) && (!f.conn || String(i.conn_id) === f.conn) &&
      (!q || (i.conn_name + ' ' + (i.host || '') + ' ' + i.app + ' ' + i.path + ' ' + (i.env || '')).toLowerCase().includes(q)));
  }

  function filterBar() {
    const s = G.state, f = G.f;
    const envs = [...new Set(s.installs.map(i => i.env).filter(Boolean))].sort();
    const fids = [...new Set(s.installs.map(i => i.folder_id).filter(Boolean))];
    const tags = [...new Set(s.installs.flatMap(i => i.tags || []))].sort();
    const appNames = [...new Set(s.installs.map(i => i.app))].sort();
    const servers = [...new Map(s.installs.map(i => [i.conn_id, i.conn_name || i.hostname || i.host || ''])).entries()].sort((a, b) => a[1].localeCompare(b[1]));
    const opt = (v, label, cur) => `<option value="${esc(v)}" ${String(cur) === String(v) ? 'selected' : ''}>${esc(label)}</option>`;
    return `<select onchange="gitWorkspace.filter('app', this.value)">${opt('', t('git_all_services'), f.app)}${appNames.map(x => opt(x, x, f.app)).join('')}</select>
      <select onchange="gitWorkspace.filter('conn', this.value)">${opt('', t('git_all_servers'), f.conn)}${servers.map(([id, n]) => opt(id, hn(n) || '#' + id, f.conn)).join('')}</select>
      <select onchange="gitWorkspace.filter('state', this.value)">${opt('', t('git_all_states'), f.state)}${['update', 'review', 'ok', 'error', 'gone', 'unknown'].map(x => opt(x, t('git_st_' + x), f.state)).join('')}</select>
      <select onchange="gitWorkspace.filter('env', this.value)">${opt('', t('git_all_envs'), f.env)}${envs.map(x => opt(x, x, f.env)).join('')}</select>
      ${dirSelect("gitWorkspace.filter('dir', this.value)", s.installs.map(i => i.path), f.dir, true)}
      <select onchange="gitWorkspace.filter('folder', this.value)">${opt('', t('git_all_folders'), f.folder)}${connFolderOptions(fids, f.folder)}</select>
      <select onchange="gitWorkspace.filter('tag', this.value)">${opt('', t('git_all_tags'), f.tag)}${tags.map(x => opt(x, x, f.tag)).join('')}</select>
      <input type="search" id="git-q" placeholder="${esc(t('git_search_srv'))}" value="${esc(f.q)}" oninput="gitWorkspace.filter('q', this.value, true)">`;
  }

  function refBar() {
    const st = G.state.settings;
    return `<label class="g-mut">${esc(t('git_ref_choice'))}</label>
      <select onchange="gitWorkspace.setRef(this.value)">${['catalog', 'default', 'branch'].map(m => `<option value="${m}" ${st.ref_mode === m ? 'selected' : ''}>${esc(t('git_ref_' + m))}</option>`).join('')}</select>
      ${st.ref_mode === 'branch' ? `<input id="git-ref-branch" value="${esc(st.ref_branch || '')}" placeholder="main" onchange="gitWorkspace.setRef('branch')" style="min-width:110px">` : ''}`;
  }

  function overviewHTML() {
    const s = G.state;
    const apps = [...s.catalog.apps.map(a => a.name), ...(s.bundle_apps || [])];
    const list = filtered();
    const targets = Object.fromEntries(s.targets.map(x => [x.app, x]));
    let html = `<div class="g-bar">${refBar()}<button class="btn-sec btn-sm" onclick="gitWorkspace.refresh()" ${s.can_check ? '' : 'disabled'}>↻ ${esc(t('git_refresh_targets'))}</button></div>`;
    html += `<div class="g-card"><h3>${esc(t('git_targets'))}</h3><div class="g-scroll"><table><tr><th>${esc(t('git_service'))}</th><th>${esc(t('git_target'))}</th><th>${esc(t('git_latest_tag'))}</th><th>${esc(t('git_source'))}</th><th>${esc(t('git_warnings'))}</th></tr>` +
      (apps.length ? apps.map(a => { const x = targets[a]; return `<tr><td><b>${esc(a)}</b></td><td>${x ? `<span class="g-mono">${esc(x.version || '—')}</span> <span class="g-mut">${esc(x.branch || '')} · ${esc(x.commit_date || '')}</span>` : `<span class="g-mut">${esc(t('git_no_target'))}</span>`}</td>
        <td class="g-mono">${esc((x && x.latest_tag) || '—')}</td><td class="g-mut">${esc(x ? x.source : '')}</td><td>${x && x.error ? `<span class="g-st s-error">${esc(x.error)}</span>` : esc(((x && x.warnings) || []).join('; '))}</td></tr>`; }).join('')
        : `<tr><td colspan="5" class="g-mut">${esc(t('git_empty_overview'))}</td></tr>`) + '</table></div></div>';
    html += `<div class="g-control-stack"><div class="g-bar">${filterBar()}</div>${checkBar(list)}</div>`;
    const servers = [...new Map(list.map(i => [i.conn_id, i])).entries()].sort((a, b) => a[1].conn_name.localeCompare(b[1].conn_name));
    if (!servers.length) return html + `<div class="hint-box">${esc(t('git_empty_overview'))}</div>`;
    const cols = [...apps, ...new Set(list.map(i => i.app))].filter((a, k, arr) => arr.indexOf(a) === k && list.some(i => i.app === a));
    const ids = l => l.map(i => i.id);
    const running = jobRunning();
    html += `<div class="g-card g-scroll"><table class="g-matrix"><tr><th>${selBox(ids(list), t('git_sel_all'))}${esc(t('git_server'))}</th>${cols.map(a => `<th>${selBox(ids(list.filter(i => i.app === a)), t('git_sel_col'))}${esc(a)}</th>`).join('')}</tr>` +
      servers.map(([cid, x]) => { const row = list.filter(i => i.conn_id === cid);
        return `<tr><td>${selBox(ids(row), t('git_sel_row'))}<b>${esc(hn(x.conn_name, x.hostname))}</b>${x.host ? `<div class="g-hostname g-mono">${esc(x.host)}</div>` : ''}</td>${cols.map(a => { const cell = row.filter(i => i.app === a);
          return `<td class="g-cell">${cell.map(instChip).join('')}${cell.length && G.state.can_check ? `<button class="g-cellbtn" title="${esc(t('git_check_cell'))}" ${running ? 'disabled' : ''} onclick="gitWorkspace.checkIds([${ids(cell).join(',')}])">↻ ${esc(t('git_check_cell'))}</button>` : ''}</td>`; }).join('')}</tr>`; }).join('') + '</table></div>';
    return html;
  }

  function detectedStateText(d) {
    return d.state === 'mixed' ? t('git_detected_mixed') : d.state === 'modified' ? t('git_detected_modified')
      : d.state === 'missing' ? t('git_detected_missing') : t('git_detected_unknown');
  }

  function codeVersion(i) {
    const d = i.detected || {};
    if (d.state === 'exact') return d.version || '—';
    if (d.state) return detectedStateText(d);
    return (i.version_md && i.version_md.version) || t('git_vmd_missing');
  }

  function vmdConflict(d, v) {
    const vv = String((v && v.version) || '').trim();
    if (!vv) return false;
    if (!d || !d.state) return false;
    if (d.state === 'unknown') return false;
    if (d.state !== 'exact') return true;
    if (vv.toLowerCase() === String(d.version || '').trim().toLowerCase()) return false;
    const vc = String((v && v.commit) || '').split(/\s+/)[0].slice(0, 10);
    const dc = String(d.commit || '').slice(0, 10);
    return !(vc.length >= 7 && dc.length >= 7 && (vc.startsWith(dc) || dc.startsWith(vc)));
  }

  function vmdLine(i, compact) {
    const v = i.version_md || {}, d = i.detected || {};
    if (!v.version) return `<span class="g-mut">${esc(t('git_vmd_none'))}</span>`;
    const conflict = vmdConflict(d, v);
    const head = `<span class="g-mono">${esc(v.version)}</span>${v.commit ? ` <span class="g-mono g-mut">${esc(v.commit)}</span>` : ''}`;
    if (compact) return `${head}${conflict ? ` <span class="g-st s-review">⚠</span>` : ''}`;
    let note;
    if (!d.state) note = t('git_vmd_ignored');
    else if (d.state !== 'exact') note = tf('git_vmd_no_version', {s: detectedStateText(d)});
    else if (conflict) note = tf('git_vmd_mismatch', {v: v.version, c: v.commit || '—', dv: codeVersion(i), dc: sh10(d.commit) || '—'});
    else note = t('git_vmd_match');
    return `${head}<div class="g-mut">${esc(note)}</div>`;
  }

  function vmdBox() {
    return `<label class="g-opt" title="${esc(t('git_check_vmd_h'))}"><input type="checkbox" ${G.checkVMD ? 'checked' : ''} onchange="gitWorkspace.setCheckVMD(this.checked)"><span>${esc(t('git_check_vmd'))}</span></label>`;
  }

  function instChip(i) {
    const vline = G.checkVMD ? `<div class="g-mut">${esc(t('git_version_md'))}: ${vmdLine(i, true)}</div>` : '';
    return `<span class="g-inst${G.sel.has(i.id) ? ' sel' : ''}" onclick="gitWorkspace.details(${i.id})" title="${esc(i.path)}"><input type="checkbox" class="g-cb" ${G.sel.has(i.id) ? 'checked' : ''} onclick="event.stopPropagation()" onchange="gitWorkspace.sel([${i.id}], this.checked)">${stateCell(i)}${i.env ? `<span class="g-env">${esc(i.env)}</span>` : ''}${i.restart_pending ? `<span class="g-pend" title="${esc(fmtTime(i.restart_pending))}">↻ ${esc(t('git_restart_pending'))}</span>` : ''}
      <div class="g-mut">${esc(t('git_detected'))}: <span class="g-mono">${esc(codeVersion(i))}</span>${i.detected && i.detected.approx ? ` <span class="g-mut">(${esc(t('git_approx'))})</span>` : ''}${i.behind ? ' · ' + esc(tf('git_behind', {n: i.behind})) : ''}</div>
      ${vline}
      <div class="g-ago" title="${esc(i.checked_at ? fmtTime(i.checked_at) : '')}">${lastGood(i)}${esc(tf('git_checked_ago', {t: ago(i.checked_at)}))}</div></span>`;
  }

  // ── selection and check jobs ──
  const J = {job: null, items: {}, seq: 0, timer: null, stale: 'never'};
  const jobRunning = () => !!(J.job && J.job.state === 'running');

  function ago(ts) {
    if (!ts) return t('git_never');
    const s = Math.max(0, (Date.now() - new Date(ts).getTime()) / 1000);
    if (s < 60) return t('git_ago_now');
    if (s < 3600) return tf('git_ago_min', {n: Math.floor(s / 60)});
    if (s < 86400) return tf('git_ago_h', {n: Math.floor(s / 3600)});
    return tf('git_ago_d', {n: Math.floor(s / 86400)});
  }

  function lastGood(i) { return i.state === 'error' && i.last_ok_state ? esc(tf('git_last_good', {s: t('git_st_' + i.last_ok_state), t: ago(i.last_ok_at)})) + ' · ' : ''; }

  // the state badge, or the job's spinner while the installation is queued or being checked
  function stateCell(i) {
    const it = jobRunning() && J.items[i.id];
    if (it && it.state === 'running') return `<span class="g-q"><span class="g-spin"></span>${esc(t('git_q_running'))}</span>`;
    if (it && it.state === 'queued') return `<span class="g-q">⏳ ${esc(t('git_q_queued'))}</span>`;
    return stBadge(i.state);
  }

  function selBox(ids, title) {
    const on = ids.length > 0 && ids.every(id => G.sel.has(id));
    return `<input type="checkbox" class="g-cb" title="${esc(title)}" ${on ? 'checked' : ''} ${ids.length ? '' : 'disabled'} onclick="event.stopPropagation()" onchange="gitWorkspace.sel([${ids.join(',')}], this.checked)">`;
  }

  function checkBar(list) {
    const running = jobRunning(), can = G.state.can_check, n = G.sel.size;
    const j = J.job || {};
    const stale = ['never', '60', '1440', '10080'];
    return `<div class="g-bar g-checkbar">
      <button class="btn-sm" onclick="gitWorkspace.checkSel()" ${can && !running && n ? '' : 'disabled'}>✓ ${esc(tf('git_check_selected', {n}))}</button>
      <button class="btn-sec btn-sm" onclick="gitWorkspace.checkVisible()" ${can && !running && list.length ? '' : 'disabled'}>${esc(tf('git_check_visible', {n: list.length}))}</button>
      <span style="display:inline-flex;gap:4px;align-items:center"><button class="btn-sec btn-sm" onclick="gitWorkspace.checkStale()" ${can && !running && list.length ? '' : 'disabled'}>${esc(t('git_check_stale'))}</button>
      <select id="git-stale" onchange="gitWorkspace.setStale(this.value)">${stale.map(x => `<option value="${x}" ${J.stale === x ? 'selected' : ''}>${esc(t('git_stale_' + x))}</option>`).join('')}</select></span>
      ${vmdBox()}
      ${n ? `<button class="btn-sec btn-sm" onclick="gitWorkspace.clearSel()">${esc(t('git_clear_sel'))}</button>` : ''}
      ${running ? `<span class="g-sp"></span><span class="g-prog"><span class="g-spin"></span>${esc(j.phase === 'targets' ? t('git_job_targets') : tf('git_job_progress', {done: j.done || 0, total: j.total || 0}))}</span>
        <button class="btn-danger btn-sm" onclick="gitWorkspace.cancelCheck()">✕ ${esc(t('git_cancel_check'))}</button>` : ''}</div>`;
  }

  // keepView re-renders the tab and keeps the focus, the caret and the scroll positions
  function keepView() {
    if (!$('git-ws') || $('git-ws').style.display === 'none') {paintFunctionality();return;}
    const ae = document.activeElement, id = ae && ae.id, pos = ae && typeof ae.selectionStart === 'number' ? ae.selectionStart : null;
    const scr = [...document.querySelectorAll('#git-body .g-scroll')].map(e => [e.scrollLeft, e.scrollTop]);
    const top = $('git-body') ? $('git-body').scrollTop : 0;
    render();
    document.querySelectorAll('#git-body .g-scroll').forEach((e, k) => { if (scr[k]) { e.scrollLeft = scr[k][0]; e.scrollTop = scr[k][1]; } });
    if ($('git-body')) $('git-body').scrollTop = top;
    if (id && $(id) && !$('git-modal')) { $(id).focus(); if (pos !== null) try { $(id).setSelectionRange(pos, pos); } catch (e) { /* not a text input */ } }
  }

  function sel(ids, on) { ids.forEach(id => on ? G.sel.add(id) : G.sel.delete(id)); keepView(); }
  function clearSel() { G.sel.clear(); keepView(); }
  function setStale(v) { J.stale = v; }
  function setCheckVMD(on) {
    G.checkVMD = !!on;
    try { localStorage.setItem('wrm_git_check_vmd', on ? '1' : '0'); } catch (_) {}
    if (G.mode === 'simple') paintSimple(); else keepView();
  }

  function mergeInstalls(list) {
    const by = new Map(list.map(i => [i.id, i]));
    G.state.installs = G.state.installs.map(i => by.has(i.id) ? by.get(i.id) : i);
    const have = new Set(G.state.installs.map(i => i.id));
    list.forEach(i => { if (!have.has(i.id)) G.state.installs.push(i); });
  }

  function setJob(j) {
    J.job = j;
    J.items = {};
    (j.items || []).forEach(it => { J.items[it.install_id] = it; });
    if (j.installs) mergeInstalls(j.installs);
    J.seq = j.seq || 0;
  }

  async function startJob(body) {
    if (jobRunning() || G.busy) return;
    if(body.refresh){G.busy=true;render();try{await gitClient.refreshConnected()}catch(e){showToast(e.message,'error');G.busy=false;render();return}G.busy=false;}
    const r = await fetch('/api/git/check/jobs', json('POST', body));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    J.seq = 0;
    setJob(await r.json());
    keepView();
    pollJob();
  }

  function pollJob() {
    clearTimeout(J.timer);
    J.timer = setTimeout(async () => {
      let j = null;
      try {
        const r = await fetch('/api/git/check/jobs/current?since=' + J.seq);
        if (r.ok) j = (await r.json()).job;
      } catch (e) { /* network: try again */ }
      if (!j) { if (jobRunning()) J.timer = setTimeout(pollJob, 3000); return; }
      setJob(j);
      if (j.state === 'running') { keepView(); pollJob(); return; }
      const errs = (j.items || []).filter(x => x.state === 'error').length;
      showToast((j.state === 'cancelled' ? t('git_job_cancelled') : tf('git_job_done', {n: (j.items || []).filter(x => x.state === 'done').length})) +
        (errs ? ' ' + tf('git_job_errors', {n: errs}) : ''), errs ? 'warning' : 'success', 5000);
      if (await load()) keepView();
    }, 900);
  }

  async function resumeJob() {
    try {
      const r = await fetch('/api/git/check/jobs/current');
      if (!r.ok) return;
      const j = (await r.json()).job;
      if (j && j.state === 'running') { J.seq = 0; setJob(j); keepView(); pollJob(); }
    } catch (e) { /* no job */ }
  }

  function checkIds(ids) { if (ids.length) startJob({install_ids: ids}); }
  function checkSel() { if (!G.sel.size) { showToast(t('git_select_installs'), 'warning'); return; } checkIds([...G.sel]); }
  function checkVisible() { checkIds(filtered().map(i => i.id)); }
  function checkStale() {
    const ids = filtered().map(i => i.id);
    if (!ids.length) return;
    const body = J.stale === 'never' ? {install_ids: ids, only_never: true} : {install_ids: ids, stale_minutes: Number(J.stale)};
    const cutoff = J.stale === 'never' ? 0 : Date.now() - Number(J.stale) * 60e3;
    if (!filtered().some(i => !i.checked_at || (cutoff && new Date(i.checked_at).getTime() <= cutoff))) { showToast(t('git_nothing_stale'), 'info'); return; }
    startJob(body);
  }

  async function cancelCheck() {
    const r = await fetch('/api/git/check/jobs/current/cancel', {method: 'POST'});
    if (!r.ok) showToast(await apiError(r), 'error');
  }

  // ── services ──
  function servicesHTML() {
    const s = G.state;
    const targets = Object.fromEntries(s.targets.map(x => [x.app, x]));
    const row = (a, fromBundle) => { const x = targets[a.name || a]; const name = a.name || a;
      return `<tr><td><input type="checkbox" class="git-sel" value="${esc(name)}" ${x && !x.error ? '' : 'disabled'} style="width:auto"></td><td><b>${esc(name)}</b>${fromBundle ? ` <span class="g-mut">(${esc(t('git_bundle_only'))})</span>` : ''}</td>
        <td class="g-mono">${esc(a.project || (x && x.project) || '')}</td><td class="g-mono">${esc(a.ref || (x && x.ref) || '')}${a.subdir ? ' · ' + esc(a.subdir) + '/' : ''}</td>
        <td>${x ? `<span class="g-mono">${esc(x.version)}</span> <span class="g-mut">${x.files} ${esc(t('git_files').toLowerCase())}</span>` : '<span class="g-mut">—</span>'}</td>
        <td style="white-space:nowrap">${fromBundle ? '' : `<button class="btn-sec btn-sm" onclick="gitWorkspace.editApp('${esc(name)}')">${esc(t('git_edit'))}</button> <button class="btn-danger btn-sm" onclick="gitWorkspace.delApp('${esc(name)}')">✕</button>`}</td></tr>`; };
    return `<div class="g-bar"><button onclick="gitWorkspace.editApp('')">＋ ${esc(t('git_add_service'))}</button>
      <button class="btn-sec" onclick="gitWorkspace.suggest()">✨ ${esc(t('git_suggest'))}</button>
      <button class="btn-sec" onclick="document.getElementById('git-cat-file').click()">⬆ ${esc(t('git_import_catalog'))}</button><input type="file" id="git-cat-file" accept=".json,application/json" style="display:none" onchange="gitWorkspace.importCatalog(this)">
      <button class="btn-sec" onclick="gitWorkspace.exportCatalog()">⬇ ${esc(t('git_export_catalog'))}</button>
      <button class="btn-sec" onclick="gitWorkspace.exportBundle()">📦 ${esc(t('git_export_bundle'))}</button>
      <button class="btn-sec" onclick="gitWorkspace.refresh()" ${s.can_check ? '' : 'disabled'}>↻ ${esc(t('git_refresh_targets'))}</button></div>
      <div class="g-card g-scroll"><table><tr><th></th><th>${esc(t('git_f_name'))}</th><th>${esc(t('git_f_project'))}</th><th>${esc(t('git_f_ref'))}</th><th>${esc(t('git_target'))}</th><th></th></tr>
      ${s.catalog.apps.map(a => row(a, false)).join('')}${(s.bundle_apps || []).map(a => row(a, true)).join('')}</table></div>`;
  }

  function appForm(a) {
    const f = (k, label, v, ph) => `<div><label>${esc(t(label))}</label><input id="ga-${k}" value="${esc(v || '')}" placeholder="${esc(ph || '')}"></div>`;
    const ta = (k, label, v) => `<div><label>${esc(t(label))}</label><textarea id="ga-${k}" placeholder="${esc(t('git_lines_h'))}">${esc((v || []).join('\n'))}</textarea></div>`;
    return `<div class="g-grid">${f('name', 'git_f_name', a.name)}${f('project', 'git_f_project', a.project, 'group/project')}${f('ref', 'git_f_ref', a.ref || 'tag:latest', t('git_f_ref_h'))}
      ${f('branch', 'git_f_branch', a.branch, 'main')}${f('tag_filter', 'git_f_tag_filter', a.tag_filter, '^v\\d')}${f('subdir', 'git_f_subdir', a.subdir, 'linux')}
      <div><label>${esc(t('git_f_kind'))}</label><select id="ga-kind">${['app', 'library', 'tool'].map(k => `<option value="${k}" ${a.kind === k ? 'selected' : ''}>${esc(t('git_kind_' + k))}</option>`).join('')}</select></div></div>
      <div class="g-grid" style="margin-top:10px">${ta('fingerprint', 'git_f_fingerprint', a.fingerprint)}${ta('protected', 'git_f_protected', a.protected)}${ta('install_hint', 'git_f_hint', a.install_hint)}
      ${ta('include', 'git_f_include', a.include)}${ta('exclude', 'git_f_exclude', a.exclude)}</div>
      <div class="g-card" style="margin-top:10px"><h3>${esc(t('git_tab_functionality'))}</h3><div class="g-grid">${f('health-http','gf_http',(a.functionality || {}).http)}${f('health-tcp','gf_tcp',(a.functionality || {}).tcp)}${ta('health-units','gf_units',((a.functionality || {}).units || []).map(u => u.kind + ':' + u.name))}${ta('health-files','gf_files',(a.functionality || {}).config_files)}</div></div>`;
  }

  function readAppForm() {
    const v = k => $('ga-' + k).value.trim();
    return {name: v('name'), project: v('project'), ref: v('ref'), branch: v('branch'), tag_filter: v('tag_filter'), subdir: v('subdir'), kind: v('kind'),
      functionality: {http:v('health-http'),tcp:v('health-tcp'),config_files:lines(v('health-files')),units:lines(v('health-units')).map(value => { const at=value.indexOf(':'); return {kind:value.slice(0,at),name:value.slice(at+1)}; })},
      fingerprint: lines(v('fingerprint')), protected: lines(v('protected')), install_hint: lines(v('install_hint')), include: lines(v('include')), exclude: lines(v('exclude'))};
  }

  function editApp(name, preset, files) {
    const a = preset || G.state.catalog.apps.find(x => x.name === name) || {kind: 'app', ref: 'tag:latest', protected: ['config*', '*.ini', '.env']};
    modal(`<h3>${esc(name || t('git_add_service'))}</h3>${appForm(a)}${name ? pipelineForm(name) : ''}${files ? `<details style="margin-top:10px"><summary class="g-mut">${esc(t('git_repo_files'))} (${files.length})</summary><div class="g-mono" style="max-height:200px;overflow:auto">${files.map(esc).join('<br>')}</div></details>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="ga-save">${esc(t('git_save'))}</button></div>`);
    $('ga-save').onclick = async () => {
      const app = readAppForm();
      if (a.environments) app.environments = a.environments;
      const r = await fetch('/api/git/catalog/apps/' + encodeURIComponent(name || app.name), json('PUT', app));
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      G.state = await r.json();
      if (name && !(await savePipeline(app.name))) return;
      closeModal(); render(); showToast(t('git_saved'), 'success');
    };
    if ($('gp-kind')) pipelineKind();
  }

  async function delApp(name) {
    if (!(await uiConfirm(tf('git_delete_q', {name}), {danger: true, okText: t('git_delete')}))) return;
    const r = await fetch('/api/git/catalog/apps/' + encodeURIComponent(name), {method: 'DELETE'});
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    G.state = await r.json(); render();
  }

  async function suggest() {
    const src = G.state.sources.find(x => x.kind !== 'bundle');
    let projects = [];
    if (src) { const r = await fetch(`/api/git/sources/${src.id}/test`, {method: 'POST'}); if (r.ok) projects = (await r.json()).projects || []; }
    modal(`<h3>✨ ${esc(t('git_suggest'))}</h3><div class="g-grid"><div><label>${esc(t('git_suggest_project'))}</label><input id="gs-project" list="gs-projects" placeholder="group/project">
      <datalist id="gs-projects">${projects.map(p => `<option value="${esc(p)}">`).join('')}</datalist></div>
      <div><label>${esc(t('git_suggest_ref'))}</label><input id="gs-ref"></div></div>
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gs-go">${esc(t('git_suggest_go'))}</button></div>`);
    $('gs-go').onclick = async () => {
      $('gs-go').disabled = true;
      const r = await fetch('/api/git/suggest', json('POST', {project: $('gs-project').value.trim(), ref: $('gs-ref').value.trim()}));
      $('gs-go').disabled = false;
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      const j = await r.json();
      editApp('', j.app, j.files);
    };
  }

  function importCatalog(input) {
    const file = input.files[0]; input.value = '';
    if (!file) return;
    file.text().then(async text => {
      let body;
      try { body = JSON.parse(text); } catch (e) { showToast(e.message, 'error'); return; }
      const merge = await uiConfirm(t('git_import_merge'), {okText: 'OK'});
      const r = await fetch('/api/git/catalog/import?mode=' + (merge ? 'merge' : 'replace'), json('POST', body));
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      G.state = await r.json(); render(); showToast(t('git_saved'), 'success');
    });
  }

  async function download(r, fallback) {
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    const m = /filename="([^"]+)"/.exec(r.headers.get('Content-Disposition') || '');
    const url = URL.createObjectURL(await r.blob());
    const a = document.createElement('a'); a.href = url; a.download = m ? m[1] : fallback; document.body.appendChild(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
  }

  function exportCatalog() { fetch('/api/git/catalog/export').then(r => download(r, 'catalog.json')); }

  function exportBundle() {
    const apps = [...document.querySelectorAll('.git-sel:checked')].map(x => x.value);
    if (!apps.length) { showToast(t('git_export_bundle_none'), 'warning'); return; }
    fetch('/api/git/bundles/export', json('POST', {apps})).then(r => download(r, 'bundle.tar.gz'));
  }

  // ── installations ──
  function installsHTML() {
    const list = filtered();
    const sshConns = (conns || []).filter(c => c.protocol === 'SSH').sort((a, b) => a.name.localeCompare(b.name));
    const apps = [...G.state.catalog.apps.map(a => a.name), ...(G.state.bundle_apps || [])];
    return `<div class="g-control-stack"><div class="g-bar">${filterBar()}<button class="btn-sec" onclick="gitWorkspace.check(true)" ${G.state.can_check && !jobRunning() ? '' : 'disabled'}>🔎 ${esc(t('git_discover'))}</button>
      <button class="btn-sm" onclick="gitWorkspace.install()" ${can('install') ? '' : `disabled title="${esc(t('git_no_permission'))}"`}>＋ ${esc(t('git_install'))}</button></div>
      ${checkBar(list)}<div class="g-bar g-actions">${actionButtons('')}</div></div>
      <div class="g-card g-scroll"><table><tr><th>${selBox(list.map(i => i.id), t('git_sel_all'))}</th><th>${esc(t('git_server'))}</th><th>${esc(t('git_service'))}</th><th>${esc(t('git_path'))}</th><th>${esc(t('git_env'))}</th><th>${esc(t('git_state'))}</th>
      <th>${esc(t('git_detected'))}</th><th>${esc(t('git_target'))}</th><th>${esc(t('git_checked'))}</th><th></th></tr>
      ${list.map(i => `<tr><td><input type="checkbox" class="git-isel g-cb" value="${i.id}" ${G.sel.has(i.id) ? 'checked' : ''} onchange="gitWorkspace.sel([${i.id}], this.checked)"></td><td>${esc(hn(i.conn_name, i.hostname))}${i.host ? `<div class="g-hostname g-mono">${esc(i.host)}</div>` : ''}</td><td><b>${esc(i.app)}</b>${i.restart_pending ? `<span class="g-pend" title="${esc(fmtTime(i.restart_pending))}">↻ ${esc(t('git_restart_pending'))}</span>` : ''}</td><td class="g-mono">${esc(i.path)}</td><td>${esc(i.env || '')}</td>
        <td>${stateCell(i)}${i.behind ? ` <span class="g-mut">${esc(tf('git_behind', {n: i.behind}))}</span>` : ''}${i.error ? `<div class="g-mut">${esc(i.error)}</div>` : ''}${i.state === 'error' && i.last_ok_state ? `<div class="g-mut">${lastGood(i).replace(/ · $/, '')}</div>` : ''}</td>
        <td class="g-mono">${esc(codeVersion(i))}${i.detected && i.detected.approx ? ` <span class="g-mut">(${esc(t('git_approx'))})</span>` : ''}${G.checkVMD ? `<div class="g-mut">${vmdLine(i, true)}</div>` : ''}</td><td class="g-mono">${esc(i.target || '—')}</td><td class="g-mut" title="${esc(i.checked_at ? fmtTime(i.checked_at) : '')}">${esc(ago(i.checked_at))}</td>
        <td style="white-space:nowrap">${G.state.can_check ? `<button class="btn-sec btn-sm" title="${esc(t('git_check_cell'))}" ${jobRunning() ? 'disabled' : ''} onclick="gitWorkspace.checkIds([${i.id}])">↻</button> ` : ''}<button class="btn-sec btn-sm" onclick="gitWorkspace.details(${i.id})">${esc(t('git_details'))}</button> <button class="btn-danger btn-sm" onclick="gitWorkspace.forget(${i.id})">✕</button></td></tr>`).join('')}</table></div>
      <div class="g-card"><h3>${esc(t('git_add_install'))}</h3><div class="g-bar"><select id="gi-conn">${sshConns.map(c => `<option value="${c.id}">${esc(c.name)}</option>`).join('')}</select>
      <select id="gi-app">${apps.map(a => `<option>${esc(a)}</option>`).join('')}</select><input id="gi-path" placeholder="/opt/service" style="min-width:200px"><input id="gi-env" placeholder="${esc(t('git_env'))}">
      <button class="btn-sec" onclick="gitWorkspace.addInstall()">＋</button></div></div>`;
  }

  async function addInstall() {
    const r = await fetch('/api/git/installs', json('POST', {conn_id: Number($('gi-conn').value), app: $('gi-app').value, path: $('gi-path').value.trim(), env: $('gi-env').value.trim()}));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    G.state = await r.json(); render();
  }

  async function forget(id) {
    if (!(await uiConfirm(t('git_forget_q'), {danger: true, okText: t('git_forget')}))) return;
    const r = await fetch('/api/git/installs/' + id, {method: 'DELETE'});
    if (r.ok) { G.state = await r.json(); render(); }
  }

  function functionalityBadge(h) {
    const state = h && h.state || 'unknown';
    return `<span class="pill ${state === 'healthy' ? 'ok' : state === 'failed' ? 'bad' : 'warn'}">${esc(t('gf_' + state))}</span>`;
  }
  function functionalityBody(h) {
    if (!h) return `<div class="g-mut">${esc(t('gf_never'))}</div>`;
    return `<div class="g-mut">${esc(fmtTime(h.checked_at))}</div>${(h.warnings || []).map(w => `<div class="hint-box warn" style="overflow-wrap:anywhere">⚠ ${esc(w)}</div>`).join('')}
      ${(h.checks || []).map(c => `<div class="g-card" style="margin-top:6px;overflow-wrap:anywhere">${functionalityBadge(c)} <b>${esc(c.kind)} · ${esc(c.name)}</b><pre style="white-space:pre-wrap;margin:4px 0">${esc(c.status)}</pre>${c.log ? `<details><summary>${esc(t('gf_log'))}</summary><pre style="white-space:pre-wrap;max-height:240px;overflow:auto">${esc(c.log)}</pre></details>` : ''}</div>`).join('')}`;
  }
  const HF = {q:'',server:'',app:'',env:'',dir:'',kind:'',health:'',age:'',problem:'',collapsed:new Set()};
  function functionalityHTML() {
    const installs=G.state.installs || [], apps=G.state.catalog.apps || [], servers=G.state.server_health || [];
    const groups=new Map();
    for (const i of installs) { if (!groups.has(i.conn_id)) groups.set(i.conn_id,{id:i.conn_id,name:i.conn_name,hostname:i.hostname,items:[],health:null}); groups.get(i.conn_id).items.push(i); }
    for (const h of servers) { if (!groups.has(h.conn_id)) groups.set(h.conn_id,{id:h.conn_id,name:h.name,hostname:h.hostname||'',folder:h.folder||'',items:[]}); groups.get(h.conn_id).health=h; }
    const option=(v,label,cur)=>`<option value="${esc(v)}" ${String(cur)===String(v)?'selected':''}>${esc(label)}</option>`;
    const choose=(key,label,values)=>`<select aria-label="${esc(label)}" onchange="gitWorkspace.functionalityFilter('${key}',this.value)">${option('',label,HF[key])}${values.map(([v,l])=>option(v,l,HF[key])).join('')}</select>`;
    const control=`<div class="g-bar g-sticky"><input id="health-q" type="search" placeholder="${esc(t('git_search_srv'))}" value="${esc(HF.q)}" oninput="gitWorkspace.functionalityFilter('q',this.value)">${choose('server',t('git_all_servers'),[...groups.values()].map(g=>[g.id,hn(g.name,g.hostname)+' · '+g.name+' #'+g.id]))}${choose('app',t('git_all_services'),apps.map(a=>[a.name,a.name]))}${choose('env',t('git_all_envs'),[...new Set(installs.map(i=>i.env).filter(Boolean))].map(x=>[x,x]))}${dirSelect("gitWorkspace.functionalityFilter('dir',this.value)",installs.map(i=>i.path),HF.dir,true)}${choose('kind',t('hf_runtime'),['systemd','supervisor','http','tcp','resource'].map(x=>[x,x]))}${choose('health',t('git_all_states'),['healthy','warning','failed','unknown'].map(x=>[x,t('gf_'+x)]))}${choose('age',t('hf_freshness'),[['never',t('gf_never')],['stale',t('hf_stale')]])}${choose('problem',t('hf_installation'),[['yes',t('hf_problems')]])}<button class="btn-sec btn-sm" onclick="gitWorkspace.functionalityReset()">${esc(t('hf_reset'))}</button><button class="btn-sec btn-sm" onclick="gitWorkspace.functionalityExpand(true)">＋</button><button class="btn-sec btn-sm" onclick="gitWorkspace.functionalityExpand(false)">−</button><button class="btn-sec" ${G.state.can_check&&!jobRunning()?'':'disabled'} onclick="gitWorkspace.check()">↻ ${esc(t('git_check_now'))}</button><button class="btn-sec" onclick="gitWorkspace.configProfiles()">${esc(t('gcp_title'))}</button></div>`;
    function matches(health,data) {
      if (HF.q&&!JSON.stringify(data).toLowerCase().includes(HF.q.toLowerCase())) return false;
      if (HF.health&&(health?.state || 'unknown')!==HF.health) return false;
      if (HF.kind&&!(health?.checks || []).some(x=>x.kind===HF.kind)) return false;
      if (HF.age==='never'&&health?.checked_at) return false;
      if (HF.age==='stale'&&(!health?.checked_at||Date.now()-new Date(health.checked_at).getTime()<30*60000)) return false;
      return true;
    }
    let count=0;
    const body=[...groups.values()].sort((a,b)=>hn(a.name,a.hostname).localeCompare(hn(b.name,b.hostname))).map(g=>{
      if (HF.server&&String(g.id)!==HF.server) return '';
      const items=g.items.filter(i=>(!HF.app||i.app===HF.app)&&(!HF.env||i.env===HF.env)&&inDir(i.path,HF.dir)&&(!HF.problem||i.error||['error','gone','unknown'].includes(i.state)||i.functionality?.warnings?.length)&&matches(i.functionality,{...i,repository:apps.find(a=>a.name===i.app)?.project}));
      const hostMatch=!HF.app&&!HF.env&&!HF.dir&&!HF.problem&&matches(g.health,g);
      if (!items.length&&!hostMatch) return '';count+=items.length+(hostMatch?1:0);
      const states=g.items.map(i=>i.functionality?.state || 'unknown');if(g.health)states.push(g.health.state);
      const totals=['healthy','warning','failed','unknown'].map(x=>`${t('gf_'+x)}: ${states.filter(s=>s===x).length}`).join(' · ');
      return `<details class="g-card hf-group" data-server="${g.id}" ${HF.collapsed.has(g.id)?'':'open'} ontoggle="gitWorkspace.functionalityToggle(${g.id},this.open)"><summary><b>${esc(hn(g.name,g.hostname))}</b> · ${esc(g.name)} #${g.id} · ${esc(g.folder||'')}<div class="g-mut">${esc(totals)}</div></summary>${hostMatch?functionalityBody(g.health):''}${items.map(i=>{const a=apps.find(a=>a.name===i.app)||{};return `<div class="g-card"><div class="g-bar">${functionalityBadge(i.functionality)}<b>${esc(i.app)}</b><span>${esc(i.env)}</span><span class="g-sp"></span><button class="btn-sec btn-sm" onclick="gitWorkspace.configuration(${i.id})">${esc(t('gf_config'))}</button><button class="btn-sec btn-sm" onclick="gitWorkspace.details(${i.id})">${esc(t('git_details'))}</button></div><div class="g-mono">${esc(i.path)}</div><div class="g-mut">${esc(t('gf_source'))}: ${esc(a.project||'—')}</div>${i.error?`<div class="hint-box bad">${esc(i.error)}</div>`:''}${functionalityBody(i.functionality)}</div>`}).join('')}</details>`;
    }).join('');
    return control+`<div class="g-mut">${esc(t('hf_matches'))}: ${count}</div>`+(body||`<div class="hint-box">${esc(t('none'))}</div>`)+apps.filter(a=>!installs.some(i=>i.app===a.name)).map(a=>`<div class="g-card"><b>${esc(a.name)}</b><div class="hint-box warn">${esc(t('gf_no_install'))}</div></div>`).join('');
  }
  function paintFunctionality() { const body=document.getElementById('monitor-service-view') || (G.tab==='functionality'&&$('git-body'));if(!body)return;const focus=document.activeElement?.id, pos=document.activeElement?.selectionStart, top=body.scrollTop;body.innerHTML=functionalityHTML();body.scrollTop=top;if(focus==='health-q'){const q=document.getElementById(focus);q.focus();if(pos!=null)q.setSelectionRange(pos,pos)} }
  function functionalityFilter(key,value) {HF[key]=value;paintFunctionality();}
  function functionalityReset(){for(const k of ['q','server','app','env','dir','kind','health','age','problem'])HF[k]='';paintFunctionality();}
  function functionalityToggle(id,open){if(open)HF.collapsed.delete(id);else HF.collapsed.add(id);}
  function functionalityExpand(open){if(open)HF.collapsed.clear();else for(const i of G.state.installs||[])HF.collapsed.add(i.conn_id);paintFunctionality();}
  async function monitoringMount(el){if(!$('git-style')){const s=document.createElement('style');s.id='git-style';s.textContent=css;document.head.appendChild(s)}if(!$('monitor-git-style')){const s=document.createElement('style');s.id='monitor-git-style';s.textContent=css.replaceAll('#git-ws','#monitor-service-view')+'\n#monitor-service-view {position:static;inset:auto;z-index:auto;display:block;background:inherit;font-size:13px;}';document.head.appendChild(s)}if(el&&await load()&&el.isConnected&&document.getElementById('monitor-service-view')===el)el.innerHTML=functionalityHTML();}

  const CP = {profiles:[], result:null, seq:0};
  async function configProfiles() {
    const seq = ++CP.seq;
    try {
      const r = await fetch('/api/git/config-profiles'); if (!r.ok) throw new Error(await apiError(r));
      CP.profiles = await r.json(); if (seq !== CP.seq) return;
      modal(`<h3>${esc(t('gcp_title'))}</h3><p>${esc(t('gcp_intro'))}</p><div class="g-bar"><button class="btn-sec" onclick="gitWorkspace.configProfileNew()">＋ ${esc(t('gcp_add'))}</button><span class="g-sp"></span><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>
      ${CP.profiles.map(p => `<div class="g-card"><b>${esc(p.name)}</b> · ${esc(p.app || t('git_all_services'))} · <span class="g-mono">${esc(p.path)}</span>${p.operator ? ' · ' + esc(p.operator) : ''}<div class="g-mut">${p.reference.entries.length} ${esc(t('gcp_settings'))}${p.reference.error ? ' · ' + esc(p.reference.error) : ''}${p.reference.unparsed ? ' · ' + esc(tf('gf_unparsed',{n:p.reference.unparsed})) : ''}</div><div class="g-bar"><button class="btn-sec" onclick="gitWorkspace.configProfileCheck(${p.id})">↻ ${esc(t('gcp_check'))}</button><button class="btn-sec" onclick="gitWorkspace.configProfileDelete(${p.id})">${esc(t('delete'))}</button></div></div>`).join('') || `<p>${esc(t('gf_no_config'))}</p>`}`);
    } catch (e) { showToast(String(e),'error'); }
  }
  function configProfileNew() {
    modal(`<h3>${esc(t('gcp_add'))}</h3><p>${esc(t('gcp_import_hint'))}</p><div class="g-grid"><div><label>${esc(t('name_lbl'))}</label><input id="cp-name" maxlength="120"></div><div><label>${esc(t('git_service'))}</label><select id="cp-app">${opt('',t('git_all_services'),'')}${G.state.catalog.apps.map(a=>opt(a.name,a.name,'')).join('')}</select></div><div><label>${esc(t('gcp_path'))}</label><input id="cp-path" placeholder="config.py"></div><div><label>${esc(t('gcp_operator'))}</label><input id="cp-operator" maxlength="120"></div><div><label>${esc(t('gcp_file'))}</label><input id="cp-file" type="file" accept=".ini,.conf,.cfg,.py,.json,.yaml,.yml,.env,text/*"></div></div><div class="g-bar" style="margin-top:12px"><button class="btn-sec" onclick="gitWorkspace.configProfileSave()">${esc(t('save'))}</button><button class="btn-sec" onclick="gitWorkspace.configProfiles()">${esc(t('cancel'))}</button></div>`);
    $('cp-file').onchange = () => { const f = $('cp-file').files[0]; if (f) { if (!$('cp-name').value) $('cp-name').value = f.name; if (!$('cp-path').value) $('cp-path').value = f.name; } };
  }
  async function configProfileSave() {
    const f = $('cp-file').files[0]; if (!f || f.size > 256 * 1024) { showToast(t('gcp_file_limit'),'error'); return; }
    const body = {name:$('cp-name').value,app:$('cp-app').value,path:$('cp-path').value,operator:$('cp-operator').value,content:await f.text()};
    try { const r = await fetch('/api/git/config-profiles',json('POST',body)); if (!r.ok) throw new Error(await apiError(r)); await configProfiles(); } catch (e) { showToast(String(e),'error'); }
  }
  async function configProfileDelete(id) {
    if (!confirm(t('gcp_delete'))) return;
    const r = await fetch('/api/git/config-profiles/'+id,{method:'DELETE'}); if (r.ok) configProfiles(); else showToast(await apiError(r),'error');
  }
  async function configProfileCheck(id) {
    const seq = ++CP.seq;
    modal(`<h3>${esc(t('gcp_title'))}</h3><p role="status">${esc(t('git_checking'))}</p><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button>`);
    try {
      const r = await fetch('/api/git/config-profiles/'+id+'/check',json('POST',{ids:[...G.sel]})); if (!r.ok) throw new Error(await apiError(r));
      const result = await r.json(); if (seq !== CP.seq || !$('git-modal')) return; CP.result = result;
      modal(`<h3>${esc(result.profile.name)}</h3><p>${esc(t('gcp_values_hint'))}</p><div class="g-bar" style="position:sticky;top:0;background:var(--bg2);z-index:1;padding:8px"><input type="search" aria-label="${esc(t('search'))}" placeholder="${esc(t('search'))}" oninput="gitWorkspace.configProfileFilter(this.value)"><span class="g-sp"></span><button class="btn-sec" onclick="gitWorkspace.configProfiles()">${esc(t('gcp_title'))}</button><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div><div id="cp-results"></div>`);
      configProfileFilter('');
    } catch (e) { if (seq === CP.seq && $('git-modal')) modal(`<h3>${esc(t('gcp_title'))}</h3><div class="hint-box bad">${esc(String(e))}</div><button class="btn-sec" onclick="gitWorkspace.configProfiles()">${esc(t('gcp_title'))}</button>`); }
  }
  function configProfileFilter(q) {
    if (!CP.result || !$('cp-results')) return; q = String(q).toLowerCase();
    const ref = new Map(CP.result.profile.reference.entries.map(e=>[e.section+'\u0000'+e.key,e]));
    let count = 0;
    const html = CP.result.rows.map(row => {
      const metadata = [row.server,row.connection,row.app,row.environment,row.operator,row.installation_path].join(' ');
      const actual = new Set(row.file.entries.map(e=>e.section+'\u0000'+e.key));
      const entries = row.file.entries.map(e=>({entry:e,missing:false})).concat([...ref].filter(([key])=>!actual.has(key)).map(([,entry])=>({entry,missing:true})));
      const filtered = entries.filter(({entry:e})=>!q || [metadata,e.section,e.key,e.value,e.description].join(' ').toLowerCase().includes(q));
      if (!filtered.length && q && ![metadata,row.file.error].join(' ').toLowerCase().includes(q)) return '';
      count += filtered.length;
      return `<section class="g-card"><h3>${esc(row.server)} <span class="g-mut">${esc(row.connection)} · ${esc(row.app)} · ${esc(row.environment)}${row.operator ? ' · '+esc(row.operator) : ''}</span></h3><div class="g-mono g-mut">${esc(row.installation_path)} / ${esc(row.file.path)} · ${esc(fmtTime(row.checked_at))}</div>${row.file.error ? `<div class="hint-box warn">${esc(row.file.error)}</div>` : ''}${row.file.unparsed ? `<div class="hint-box warn">${esc(tf('gf_unparsed',{n:row.file.unparsed}))}</div>` : ''}<div class="g-scroll"><table><thead><tr><th>${esc(t('gcp_operator'))} / ${esc(t('gf_section'))}</th><th>${esc(t('gf_variable'))}</th><th>${esc(t('gf_value'))}</th><th>${esc(t('gcp_expected'))}</th><th>${esc(t('status'))}</th></tr></thead><tbody>${filtered.map(({entry:e,missing})=> {const expected=ref.get(e.section+'\u0000'+e.key);const status=missing ? t('gcp_missing') : e.secret ? t('gcp_masked') : !expected ? t('gcp_extra') : e.value===expected.value ? t('gcp_same') : t('gcp_different'); return `<tr><td>${esc([row.operator,e.section].filter(Boolean).join(' / ') || row.environment || '—')}</td><td title="${esc(e.description || '')}">${esc(e.key)}</td><td style="white-space:pre-wrap;overflow-wrap:anywhere;max-width:420px">${missing?'—':esc(e.value)}</td><td style="white-space:pre-wrap;overflow-wrap:anywhere;max-width:420px">${expected?esc(expected.value):'—'}</td><td>${esc(status)}</td></tr>`;}).join('')}</tbody></table></div></section>`;
    }).join('');
    $('cp-results').innerHTML = `<p>${count} ${esc(t('gcp_settings'))}</p>` + (html || `<p>${esc(t('none'))}</p>`);
  }

  function showFunctionality() { openMonitoring('services'); }
  async function configuration(id) {
    modal(`<div class="g-bar"><h3 style="flex:1">${esc(t('gf_config'))}</h3><button class="btn-sec" onclick="gitWorkspace.closeModal()" aria-label="${esc(t('git_close'))}">✕</button></div><div class="hint-box">${esc(t('loading'))}</div>`);
    const r = await fetch('/api/git/installs/' + id + '/configuration');
    if (!r.ok) { modal(`<div class="g-bar"><h3 style="flex:1">${esc(t('gf_config'))}</h3><button class="btn-sec" onclick="gitWorkspace.closeModal()" aria-label="${esc(t('git_close'))}">✕</button></div><div class="hint-box warn">${esc(await apiError(r))}</div>`); return; }
    const data = await r.json();
    modal(`<div class="g-bar"><h3 style="flex:1">${esc(t('gf_config'))}</h3><button class="btn-sec" onclick="gitWorkspace.closeModal()" aria-label="${esc(t('git_close'))}">✕</button></div><div class="hint-box">${esc(t('gf_config_hint'))}</div><div class="g-mut">${esc(fmtTime(data.read_at))}</div>
      ${(data.files || []).map(f => `<div class="g-card"><h3 class="g-mono" style="overflow-wrap:anywhere">${esc(f.path)}</h3>${f.error ? `<div class="hint-box warn">${esc(f.error)}</div>` : ''}${f.unparsed ? `<div class="hint-box warn">${esc(tf('gf_unparsed',{n:f.unparsed}))}</div>` : ''}<div class="g-scroll"><table class="g-table"><thead><tr><th>${esc(t('gf_section'))}</th><th>${esc(t('gf_variable'))}</th><th>${esc(t('gf_value'))}</th><th>${esc(t('gf_description'))}</th></tr></thead><tbody>${(f.entries || []).map(e => `<tr><td>${esc(e.section)}</td><td class="g-mono">${esc(e.key)}${e.kind === 'function' ? '()' : ''}${e.line ? `<div class="g-mut">L${e.line}</div>` : ''}</td><td class="g-mono" style="white-space:pre-wrap;overflow-wrap:anywhere">${e.secret ? '🔒 ' : ''}${esc(e.value)}</td><td>${esc(e.description || '')}</td></tr>`).join('')}</tbody></table></div></div>`).join('') || `<div class="hint-box">${esc(t('gf_no_config'))}</div>`}`);
  }

  // ── installation details: every file, diff, view, commits ──
  const F = {id: 0, list: null, err: '', loading: false, filter: 'tracked', dir: '', q: '', diff: null, mode: window.innerWidth < 700 ? 'unified' : 'split'};

  async function details(id) {
    const r = await fetch('/api/git/installs/' + id);
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    const i = await r.json();
    G.detail = i;
    Object.assign(F, {id, list: null, err: '', loading: G.state.can_check, q: '', dir: '', diff: null});
    const d = i.detected || {}, v = i.version_md || {};
    modal(`<h3>${esc(hn(i.conn_name, i.hostname))} · ${esc(i.app)} · <span class="g-mono">${esc(i.path)}</span></h3>
      <div class="g-bar">${stBadge(i.state)}${i.env ? `<span class="g-env">${esc(i.env)}</span>` : ''}${Object.entries(i.counts || {}).map(([k, n]) => `${fsBadge(k)} ${n}`).join(' ')}
        <span class="g-mut" title="${esc(i.checked_at ? fmtTime(i.checked_at) : '')}">${lastGood(i)}${esc(tf('git_checked_ago', {t: ago(i.checked_at)}))}</span></div>
      <div class="g-bar g-actions">${actionButtons(i.id)}</div>
      ${i.error ? `<div class="hint-box bad">${esc(i.error)}</div>` : ''}${i.capped ? `<div class="hint-box warn">${esc(t('git_capped'))}</div>` : ''}
      <div class="g-grid"><div><label>${esc(t('git_detected'))}</label><span class="g-mono">${esc(codeVersion(i))}</span>
        ${d.commit ? `<div class="g-mut"><span class="g-mono">${esc(sh10(d.commit))}</span>${d.date ? ' · ' + esc(d.date) : ''}</div>` : ''}
        ${d.approx ? `<div class="g-mut">≈ ${esc(t('git_approx'))}</div>` : ''}
        ${d.state && d.state !== 'exact' ? `<div class="hint-box warn">${esc(detectedStateText(d))}</div>` : ''}</div>
        ${G.checkVMD ? `<div><label>${esc(t('git_version_md'))}</label>${vmdLine(i, false)}
          ${v.ref ? `<div class="g-mut">${esc(v.ref)}</div>` : ''}${v.updated ? `<div class="g-mut">${esc(v.updated)} ${esc(v.user || '')}${v.bundle ? ' · ' + esc(v.bundle) : ''}</div>` : ''}</div>` : ''}
        <div><label>${esc(t('git_target'))}</label><span class="g-mono">${esc(i.target || '—')}</span><div id="git-commits" class="g-commits"></div></div>
        <div><label>${esc(t('git_units'))}</label>${(i.units || []).length ? i.units.map(u => `<div class="g-mono">${esc(u.kind)}: ${esc(u.name)}</div>`).join('') : `<span class="g-mut">${esc(t('git_no_units'))}</span>`}</div></div>
      <h3 style="margin:14px 0 6px">${esc(t('git_files_all'))}</h3><div id="git-files">${filesHTML()}</div>
      <div id="git-diff"></div><div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);
    if (G.state.can_check) { loadFiles(id); loadCommits(i); }
  }

  async function loadFiles(id) {
    let list = null, err = '';
    try {
      const r = await fetch(`/api/git/installs/${id}/files`);
      if (r.ok) list = await r.json(); else err = await apiError(r);
    } catch (e) { err = e.message; }
    if (F.id !== id || !$('git-files')) return;
    Object.assign(F, {list, err, loading: false});
    paintFiles();
  }

  function paintFiles() {
    const el = $('git-files');
    if (!el) return;
    const ae = document.activeElement, focused = ae && ae.id === 'git-fq', pos = focused ? ae.selectionStart : 0;
    const box = el.querySelector('.g-flist'), top = box ? box.scrollTop : 0;
    el.innerHTML = filesHTML();
    const nb = el.querySelector('.g-flist');
    if (nb) nb.scrollTop = top;
    if (focused && $('git-fq')) { $('git-fq').focus(); $('git-fq').setSelectionRange(pos, pos); }
  }

  // the stored comparison of the last check (tracked and extra files)
  function storedHTML(i) {
    const files = (i.files || []).filter(f => !G.onlyChanges || (f.state !== 'ok' && f.state !== 'protected'));
    return `<label style="display:flex;gap:6px;align-items:center;margin:8px 0 6px"><input type="checkbox" style="width:auto" ${G.onlyChanges ? 'checked' : ''} onchange="gitWorkspace.toggleChanges(this.checked)">${esc(t('git_files_only'))}</label>
      <div class="g-flist"><table><tr><th>${esc(t('git_files_stored'))}</th><th>${esc(t('git_state'))}</th><th></th></tr>
      ${files.map(f => `<tr><td class="g-mono">${esc(f.path)}</td><td>${fsBadge(f.state)}${f.behind ? ` <span class="g-mut">${esc(tf('git_behind', {n: f.behind}))} (${esc(f.tag || '')} ${esc(f.date || '')})</span>` : ''}${f.note ? ` <span class="g-mut">${esc(f.note)}</span>` : ''}</td>
        <td>${f.diffable && f.state !== 'ok' && G.state.can_check ? `<button class="btn-sec btn-sm" onclick="gitWorkspace.diff(${i.id}, '${esc(encodeURIComponent(f.path))}')">${esc(t('git_diff'))}</button>` : ''}</td></tr>`).join('')}</table></div>`;
  }

  const DIFFERING = ['old', 'modified', 'missing', 'unknown'];

  function reasonText(f) {
    if (f.pattern && (f.reason === 'excluded' || f.reason === 'protected')) return tf('git_r_' + f.reason + '_p', {p: f.pattern});
    return t('git_r_' + f.reason);
  }

  function filesHTML() {
    const i = G.detail;
    if (!G.state.can_check) return storedHTML(i);
    if (F.loading) return `<div class="hint-box">⏳ ${esc(t('git_files_loading'))}</div>`;
    if (F.err || !F.list) return `<div class="hint-box bad">${esc(tf('git_files_failed', {e: F.err}))}</div>${storedHTML(i)}`;
    const L = F.list, q = F.q.toLowerCase();
    // in the check: tracked and protected files; every other file is outside the check
    const inCheck = f => f.tracked || f.reason === 'protected';
    const pass = f => (F.filter === 'all' || (F.filter === 'diff' && inCheck(f) && DIFFERING.includes(f.state)) || (F.filter === 'tracked' && inCheck(f)) || (F.filter === 'untracked' && !inCheck(f))) &&
      inDir(f.path, F.dir) && (!q || f.path.toLowerCase().includes(q));
    const rows = L.files.filter(pass), max = 1500;
    const seg = k => `<button class="${F.filter === k ? 'on' : ''}" onclick="gitWorkspace.fileFilter('${k}')">${esc(t('git_ff_' + k))}</button>`;
    const enc = p => esc(encodeURIComponent(p));
    const row = f => {
      const name = f.kind === 'symlink' ? `${esc(f.path)} <span class="g-mut">→ ${esc(f.link || '?')}</span>${f.outside ? `<div style="color:var(--red);font-family:inherit;font-size:11px">${esc(t('git_link_outside'))}</div>` : ''}`
        : f.kind === 'dir' ? `${esc(f.path)}/ <span class="g-mut">${esc(t('git_dir_unreadable'))}</span>` : esc(f.path);
      return `<tr class="${f.tracked || f.reason === 'protected' ? '' : 'g-untracked'}"><td class="g-mono">${name}</td>
        <td class="g-mut g-hm" style="white-space:nowrap">${f.on_server && f.kind === 'file' ? esc(fmtBytes(f.size)) : f.size ? esc(fmtBytes(f.size)) : ''}</td>
        <td class="g-mut g-hm" style="white-space:nowrap">${f.mtime ? esc(fmtTime(f.mtime)) : ''}</td>
        <td>${fsBadge(f.state)}${f.behind ? ` <span class="g-mut">${esc(tf('git_behind', {n: f.behind}))}${f.tag ? ' (' + esc(f.tag) + ')' : ''}</span>` : ''}</td>
        <td class="g-why">${esc(reasonText(f))}${f.state === 'outside' ? `<div class="g-mut">${esc(t(f.on_server && f.in_git ? 'git_where_both' : f.in_git ? 'git_where_git' : 'git_where_server'))}</div>` : ''}</td>
        <td style="white-space:nowrap">${f.viewable ? `<button class="btn-sec btn-sm" onclick="gitWorkspace.viewFile('${enc(f.path)}', '')">${esc(t('git_open'))}</button> ` : ''}${f.diffable && f.state !== 'ok' && f.state !== 'same' ? `<button class="btn-sec btn-sm" onclick="gitWorkspace.diff(${i.id}, '${enc(f.path)}')">${esc(t('git_diff'))}</button>` : ''}</td></tr>`;
    };
    return `${L.target_error ? `<div class="hint-box warn">${esc(tf('git_target_err', {e: L.target_error}))}</div>` : ''}
      ${L.truncated ? `<div class="hint-box warn">${esc(tf('git_files_truncated', {n: 10000}))}</div>` : ''}
      ${L.git_partial && L.target ? `<div class="hint-box">${esc(t('git_files_partial'))}</div>` : ''}
      <div class="g-bar" style="margin:6px 0"><span class="g-seg">${['tracked', 'diff', 'untracked', 'all'].map(seg).join('')}</span>
        ${dirSelect('gitWorkspace.fileDir(this.value)', L.files.map(f => f.path), F.dir, false)}
        <input type="search" id="git-fq" placeholder="${esc(t('git_files_search'))}" value="${esc(F.q)}" oninput="gitWorkspace.fileSearch(this.value)" style="flex:1 1 160px;min-width:0">
        <span class="g-mut">${esc(tf('git_files_count', {n: rows.length, total: L.files.length}))}</span></div>
      <div class="g-mut" style="margin-bottom:6px">${esc(t('git_files_info'))}</div>
      <div class="g-flist"><table><tr><th>${esc(t('git_path'))}</th><th class="g-hm">${esc(t('git_size'))}</th><th class="g-hm">${esc(t('git_mtime'))}</th><th>${esc(t('git_state'))}</th><th>${esc(t('git_reason'))}</th><th></th></tr>
      ${rows.slice(0, max).map(row).join('')}</table></div>${rows.length > max ? `<div class="g-mut">${esc(tf('git_files_more', {n: max}))}</div>` : ''}`;
  }

  let fqTimer = null;
  function fileFilter(k) { F.filter = k; paintFiles(); }
  function fileDir(v) { F.dir = v; paintFiles(); }
  function fileSearch(v) { F.q = v; clearTimeout(fqTimer); fqTimer = setTimeout(paintFiles, 200); }

  async function loadCommits(i) {
    const el = $('git-commits');
    const d = i.detected || {}, v = i.version_md || {};
    const allowVMD = G.checkVMD;
    if (!el || !i.target || !(allowVMD || d.commit)) return;
    el.innerHTML = `<div class="g-mut">⏳ ${esc(t('git_commits_loading'))}</div>`;
    let c = null, err = '';
    try {
      const r = await fetch(`/api/git/installs/${i.id}/commits${allowVMD ? '?md=1' : ''}`);
      if (r.ok) c = await r.json(); else err = await apiError(r);
    } catch (e) { err = e.message; }
    if (F.id !== i.id || !$('git-commits')) return;
    if (!c) { $('git-commits').innerHTML = `<div class="g-mut">${esc(t('git_commits'))}: ${esc(err)}</div>`; return; }
    $('git-commits').innerHTML = !c.count ? `<div class="g-mut">✓ ${esc(t('git_commits_none'))}</div>`
      : `<details><summary><b>${esc(tf('git_commits_behind', {n: c.count}))}</b> <span class="g-mut g-mono">${esc(tf('git_commits_range', {f: c.from, t: c.to}))}</span></summary>
        <div class="g-flist" style="max-height:30vh;margin-top:6px"><table>${c.commits.map(x => `<tr><td class="g-mono">${esc(x.sha.slice(0, 10))}</td><td>${esc(x.title)}</td><td class="g-mut g-hm">${esc(x.author)}</td><td class="g-mut g-hm" style="white-space:nowrap">${esc(x.date ? fmtTime(x.date) : '')}</td></tr>`).join('')}</table></div>
        ${c.commits.length < c.count ? `<div class="g-mut">${esc(tf('git_commits_more', {n: c.commits.length}))}</div>` : ''}</details>`;
  }

  async function diff(id, path) {
    const out = $('git-diff');
    out.innerHTML = `<div class="hint-box">⏳</div>`;
    out.scrollIntoView({block: 'nearest'});
    const r = await fetch(`/api/git/installs/${id}/diff?path=${path}`);
    if (!r.ok) { out.innerHTML = `<div class="hint-box bad">${esc(await apiError(r))}</div>`; return; }
    F.diff = await r.json();
    paintDiff();
  }

  function setMode(m) { F.mode = m; paintDiff(); }

  // splitRows pairs removed and added lines of a unified diff for the side-by-side view
  function splitRows(lines) {
    const rows = [];
    let del = [], add = [];
    const flush = () => { for (let k = 0; k < Math.max(del.length, add.length); k++) rows.push({l: del[k] || null, r: add[k] || null}); del = []; add = []; };
    for (const l of lines) {
      if (l.op === '-') del.push(l);
      else if (l.op === '+') add.push(l);
      else { flush(); rows.push(l.op === '@' ? {hunk: l} : {l, r: l, ctx: true}); }
    }
    flush();
    return rows;
  }

  function paintDiff() {
    const out = $('git-diff'), d = F.diff;
    if (!out || !d) return;
    const info = d.informational ? `<div class="hint-box warn">${esc(d.reason === 'excluded' || d.reason === 'not_included' ? t('git_informational') : t('git_info_untracked'))}${d.pattern ? ` <span class="g-mono">(${esc(d.pattern)})</span>` : ''}</div>` : '';
    const head = `<div class="g-dhead"><h3>${esc(t('git_diff_title'))}: <span class="g-mono">${esc(d.path)}</span> → ${esc(d.target || '')} ${fsBadge(d.state)}</h3>
      ${d.lines && d.lines.length ? `<span class="g-seg"><button class="${F.mode === 'unified' ? 'on' : ''}" onclick="gitWorkspace.diffMode('unified')">${esc(t('git_view_unified'))}</button><button class="${F.mode === 'split' ? 'on' : ''}" onclick="gitWorkspace.diffMode('split')">${esc(t('git_view_split'))}</button></span>` : ''}</div>`;
    let body;
    if (d.binary) body = `<div class="hint-box">${esc(t(d.same ? 'git_binary_same' : 'git_binary_differs'))}</div>`;
    else if (d.too_big) body = `<div class="hint-box">${esc(t(d.same ? 'git_big_same' : 'git_big_differs'))}</div>`;
    else if (!d.lines.length) body = `<div class="hint-box">${esc(t('git_no_diff'))}</div>`;
    else if (F.mode === 'split') {
      const cell = (l, side) => !l ? `<td class="ln"></td><td class="d-none"></td>` : `<td class="ln">${side === 'l' ? l.a || '' : l.b || ''}</td><td class="${l.op === '-' ? 'd-del' : l.op === '+' ? 'd-add' : ''}">${esc(l.s)}</td>`;
      body = `<div class="g-diff"><table class="g-split">${splitRows(d.lines).map(x => x.hunk ? `<tr class="d-hunk"><td class="ln"></td><td>@@ ${x.hunk.a}</td><td class="ln"></td><td>@@ ${x.hunk.b}</td></tr>` : `<tr>${cell(x.l, 'l')}${cell(x.r, 'r')}</tr>`).join('')}</table></div>`;
    } else {
      body = '<div class="g-diff">' + d.lines.map(l => l.op === '@' ? `<div class="d-hunk">@@ -${l.a} +${l.b} @@</div>` : `<div class="${l.op === '+' ? 'd-add' : l.op === '-' ? 'd-del' : ''}">${esc(l.op + ' ' + l.s)}</div>`).join('') + '</div>';
    }
    const notes = [d.missing_on_server ? t('git_missing_srv') : '', d.crlf_server !== d.crlf_target && d.lines && d.lines.length ? t('git_crlf') : ''].filter(Boolean);
    out.innerHTML = head + info + (notes.length ? `<div class="g-mut" style="margin-bottom:6px">${esc(notes.join(' '))}</div>` : '') + body;
    out.scrollIntoView({block: 'nearest'});
  }

  async function viewFile(path, side) {
    const out = $('git-diff'), id = F.id;
    if (!out) return;
    out.innerHTML = `<div class="hint-box">⏳</div>`;
    out.scrollIntoView({block: 'nearest'});
    const r = await fetch(`/api/git/installs/${id}/file?path=${path}${side ? '&side=' + side : ''}`);
    if (!r.ok) { out.innerHTML = `<div class="hint-box bad">${esc(await apiError(r))}</div>`; return; }
    const v = await r.json();
    const f = ((F.list || {}).files || []).find(x => x.path === v.path) || {};
    const other = v.side === 'server' ? (f.in_git && f.reason !== 'protected' ? 'git' : '') : (f.on_server ? 'server' : '');
    const head = `<div class="g-dhead"><h3>${esc(t('git_open'))}: <span class="g-mono">${esc(v.path)}</span> · ${esc(v.side === 'git' ? t('git_from_git') + (v.target ? ' ' + v.target : '') : t('git_from_server'))}</h3>
      ${other ? `<button class="btn-sec btn-sm" onclick="gitWorkspace.viewFile('${esc(encodeURIComponent(v.path))}', '${other}')">${esc(t('git_view_side'))} ${esc(t(other === 'git' ? 'git_from_git' : 'git_from_server'))}</button>` : ''}</div>`;
    let body;
    if (v.too_big) body = `<div class="hint-box">${esc(tf('git_view_big', {s: fmtBytes(v.size), h: v.hash || ''}))}</div>`;
    else if (v.binary) body = `<div class="hint-box">${esc(tf('git_view_binary', {s: fmtBytes(v.size), h: v.hash || ''}))}</div>`;
    else body = `<pre class="g-code">${esc(v.text)}</pre>`;
    out.innerHTML = head + body;
    out.scrollIntoView({block: 'nearest'});
  }

  // ── settings ──
  function settingsHTML() {
    const s = G.state, st = s.settings, cat = s.catalog;
    const apiSources = s.sources.filter(x => x.kind !== 'bundle'), bundles = s.sources.filter(x => x.kind === 'bundle');
    const sshConns = (conns || []).filter(c => c.protocol === 'SSH').sort((a, b) => a.name.localeCompare(b.name));
    const sel = new Set(st.conn_ids || []);
    return `<div class="g-card"><h3>${esc(t('git_sources'))}</h3>
      ${apiSources.length ? `<table>${apiSources.map(x => `<tr><td><b>${esc(x.name)}</b> <span class="g-mut">${esc(x.kind)}</span><div class="g-mono g-mut">${esc(x.url)}</div>${x.client_cache?`<div class="g-mut">${esc(t(x.client_cache.complete?'gc_cached':'gc_missing'))} · #${x.client_cache.published||0} · ${esc(x.client_cache.at?fmtTime(x.client_cache.at):'—')}</div>`:''}${x.last_error ? `<div class="g-st s-error">${esc(x.last_error)}</div>` : ''}</td>
        <td style="white-space:normal">${x.kind==='gitlab'?`<button class="btn-sec btn-sm" onclick="gitClient.open(${x.id})">${esc(t('gc_title'))}</button>`:''}<button class="btn-sec btn-sm" onclick="gitWorkspace.testSource(${x.id})">${esc(t('git_test'))}</button> <button class="btn-sec btn-sm" ${s.can_check ? '' : 'disabled'} ${x.owned&&!x.client_cache?'':'disabled'} onclick="gitWorkspace.webhook(${x.id})">${x.hook_id ? '🔔 ' : ''}${esc(t('git_webhook'))}</button> <button class="btn-sec btn-sm" ${x.owned?'':'disabled'} onclick="gitWorkspace.editSource(${x.id})">${esc(t('git_edit'))}</button> <button class="btn-danger btn-sm" ${x.owned?'':'disabled'} onclick="gitWorkspace.delSource(${x.id})">✕</button></td></tr>`).join('')}</table>` : ''}
      <div class="g-bar" style="margin-top:8px"><button class="btn-sec" onclick="gitWorkspace.editSource(0)">＋ ${esc(t('git_add_source'))}</button></div></div>
      <div class="g-card"><h3>${esc(t('git_bundles'))}</h3>
      ${bundles.length ? `<table>${bundles.map(x => `<tr><td><b>${esc(x.bundle_id || x.name)}</b> <span class="g-mut">${esc(tf('git_bundle_age', {d: x.created || '?'}))} · ${esc(x.created_by || '')}</span><div class="g-mono g-mut">${esc(x.origin || '')}</div><div class="g-mut">${esc((x.apps || []).join(', '))}</div></td>
        <td><button class="btn-danger btn-sm" onclick="gitWorkspace.delSource(${x.id})">✕</button></td></tr>`).join('')}</table>` : ''}
      <div class="g-bar" style="margin-top:8px"><button class="btn-sec" onclick="document.getElementById('git-bundle-file').click()">⬆ ${esc(t('git_import_bundle'))}</button><input type="file" id="git-bundle-file" accept=".gz,.tgz,application/gzip" style="display:none" onchange="gitWorkspace.importBundle(this)"></div></div>
      ${feedsHTML()}
      <div class="g-card"><h3>${esc(t('git_source_mode'))}</h3><div class="g-grid">
        <div><label>${esc(t('git_source_mode'))}</label><select id="gx-mode">${['auto', 'live', 'bundle'].map(m => `<option value="${m}" ${st.mode === m ? 'selected' : ''}>${esc(t('git_mode_' + m))}</option>`).join('')}</select></div>
        <div><label>${esc(t('git_mode_bundle'))}</label><select id="gx-bundle"><option value="0">${esc(t('git_newest_bundle'))}</option>${bundles.map(b => `<option value="${b.id}" ${st.bundle_id === b.id ? 'selected' : ''}>${esc(b.bundle_id || b.name)}</option>`).join('')}</select></div>
        ${apiSources.length ? `<div><label>${esc(t('git_source'))}</label><select id="gx-source">${apiSources.map(x => `<option value="${x.id}" ${st.source_id === x.id ? 'selected' : ''}>${esc(x.name)}</option>`).join('')}</select></div>` : ''}
        <div><label>${esc(t('git_history_depth'))}</label><input id="gx-depth" type="number" min="1" max="50" value="${st.history_depth}"></div></div></div>
      <div class="g-card"><h3>${esc(t('git_servers'))}</h3><div class="g-mut" style="margin-bottom:6px">${esc(t('git_servers_h'))}</div>
        <div class="g-bar"><input type="search" placeholder="${esc(t('git_filter'))}" oninput="gitWorkspace.filterConns(this.value)">
        <button class="btn-sec btn-sm" onclick="gitWorkspace.selConns(true)">${esc(t('git_select_all'))}</button><button class="btn-sec btn-sm" onclick="gitWorkspace.selConns(false)">${esc(t('git_select_none'))}</button></div>
        <div class="g-conns" id="gx-conns">${sshConns.map(c => `<label data-q="${esc((c.name + ' ' + c.host + ' ' + (c.tags || []).join(' ')).toLowerCase())}"><input type="checkbox" value="${c.id}" ${sel.has(c.id) ? 'checked' : ''}>${esc(c.name)} <span class="g-mut">${esc(c.host)}</span></label>`).join('')}</div></div>
      <div class="g-card"><h3>${esc(t('git_monitor'))}</h3>${s.interval_minutes > 0 ? `<label style="display:flex;gap:6px;align-items:center"><input type="checkbox" id="gx-monitor" style="width:auto" ${st.monitor ? 'checked' : ''}>${esc(tf('git_monitor_on', {n: s.interval_minutes}))}</label>` : `<div class="g-mut">${esc(t('git_monitor_off'))}</div>`}
        <div class="g-mut" style="margin-top:6px">${esc(t('git_monitor_h'))}</div></div>
      <div class="g-bar"><button onclick="gitWorkspace.saveSettings()">${esc(t('git_save'))}</button></div>
      <div class="g-card"><h3>${esc(t('git_catalog_settings'))}</h3><div class="g-grid">
        <div><label>${esc(t('git_url'))}</label><input id="gc-url" value="${esc(cat.gitlab.url || '')}" placeholder="https://git.example.com"></div>
        <div><label>${esc(t('git_groups'))}</label><textarea id="gc-groups">${esc((cat.gitlab.groups || []).join('\n'))}</textarea></div>
        <div><label>${esc(t('git_fallback'))}</label><textarea id="gc-fallback">${esc((cat.fallback_branches || []).join('\n'))}</textarea></div>
        <div><label>${esc(t('git_roots'))}</label><textarea id="gc-roots">${esc((cat.server_roots || []).join('\n'))}</textarea></div>
        <div><label>${esc(t('git_ignore'))}</label><textarea id="gc-ignore">${esc((cat.ignore_dirs || []).join('\n'))}</textarea></div></div>
        <div class="g-bar" style="margin-top:8px"><button onclick="gitWorkspace.saveCatalogSettings()">${esc(t('git_save'))}</button></div></div>`;
  }

  function filterConns(q) { q = q.toLowerCase(); document.querySelectorAll('#gx-conns label').forEach(l => { l.style.display = l.dataset.q.includes(q) ? '' : 'none'; }); }
  function selConns(on) { document.querySelectorAll('#gx-conns label').forEach(l => { if (l.style.display !== 'none') l.querySelector('input').checked = on; }); }

  async function putSettings(patch) {
    const body = Object.assign({}, G.state.settings, patch);
    const r = await fetch('/api/git/settings', json('PUT', body));
    if (!r.ok) { showToast(await apiError(r), 'error'); return false; }
    G.state = await r.json(); render(); return true;
  }

  function saveSettings() {
    const mon = $('gx-monitor');
    putSettings({mode: $('gx-mode').value, bundle_id: Number($('gx-bundle').value) || 0, source_id: Number(($('gx-source') || {}).value) || 0,
      history_depth: Number($('gx-depth').value) || 0, conn_ids: [...document.querySelectorAll('#gx-conns input:checked')].map(x => Number(x.value)),
      monitor: mon ? mon.checked : G.state.settings.monitor}).then(ok => ok && showToast(t('git_saved'), 'success'));
  }

  async function saveCatalogSettings() {
    const cat = Object.assign({}, G.state.catalog, {gitlab: {url: $('gc-url').value.trim(), groups: lines($('gc-groups').value)},
      fallback_branches: lines($('gc-fallback').value), server_roots: lines($('gc-roots').value), ignore_dirs: lines($('gc-ignore').value)});
    const r = await fetch('/api/git/catalog', json('PUT', cat));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    G.state = await r.json(); render(); showToast(t('git_saved'), 'success');
  }

  function setRef(mode) {
    const b = $('git-ref-branch');
    if (mode === 'branch' && !(b && b.value.trim())) { G.state.settings.ref_mode = 'branch'; render(); setTimeout(() => $('git-ref-branch') && $('git-ref-branch').focus(), 30); return; }
    putSettings({ref_mode: mode, ref_branch: b ? b.value.trim() : G.state.settings.ref_branch});
  }

  function editSource(id) {
    const x = G.state.sources.find(s => s.id === id) || {kind: 'gitlab', name: '', url: G.state.catalog.gitlab.url || ''};
    modal(`<h3>${esc(id ? x.name : t('git_add_source'))}</h3><div class="g-grid">
      <div><label>${esc(t('git_kind'))}</label><select id="gsrc-kind"><option value="gitlab" ${x.kind === 'gitlab' ? 'selected' : ''}>GitLab</option><option value="github" ${x.kind === 'github' ? 'selected' : ''}>GitHub</option><option value="gitea" ${x.kind === 'gitea' ? 'selected' : ''}>Gitea</option></select></div>
      <div><label>${esc(t('git_f_name'))}</label><input id="gsrc-name" value="${esc(x.name)}"></div>
      <div><label>${esc(t('git_url'))}</label><input id="gsrc-url" value="${esc(x.url)}" placeholder="https://git.example.com"></div>
      <div><label>${esc(t('git_token'))}</label><input id="gsrc-token" type="password" autocomplete="new-password" placeholder="${id && x.has_token ? esc(t('git_token_keep')) : ''}"></div>
      <div><label>${esc(t('git_write_token'))}</label><input id="gsrc-wtoken" type="password" autocomplete="new-password" placeholder="${id && x.has_write_token ? esc(t('git_token_keep')) : ''}"></div></div>
      <div class="g-mut" style="margin-top:6px">${esc(t('git_write_token_h'))}</div>
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gsrc-save">${esc(t('git_save'))}</button></div>`);
    $('gsrc-save').onclick = async () => {
      const body = {kind: $('gsrc-kind').value, name: $('gsrc-name').value.trim(), url: $('gsrc-url').value.trim()};
      const tok = $('gsrc-token').value;
      if (tok || !id) body.token = tok;
      if ($('gsrc-wtoken').value) body.write_token = $('gsrc-wtoken').value;
      const r = await fetch(id ? '/api/git/sources/' + id : '/api/git/sources', json(id ? 'PUT' : 'POST', body));
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      G.state = await r.json(); closeModal(); render();
    };
  }

  async function testSource(id) {
    const r = await fetch(`/api/git/sources/${id}/test`, {method: 'POST'});
    if (!r.ok) showToast(await apiError(r), 'error');
    else showToast(tf('git_projects_found', {n: ((await r.json()).projects || []).length}), 'success');
    load().then(ok => ok && render());
  }

  async function delSource(id) {
    if (!(await uiConfirm(t('git_delete') + '?', {danger: true, okText: t('git_delete')}))) return;
    const r = await fetch('/api/git/sources/' + id, {method: 'DELETE'});
    if (r.ok) { G.state = await r.json(); render(); }
  }

  async function importBundle(input) {
    const file = input.files[0]; input.value = '';
    if (!file) return;
    const r = await fetch('/api/git/bundles', {method: 'POST', headers: {'Content-Type': 'application/gzip'}, body: file});
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    const j = await r.json();
    G.state = j.state; render();
    showToast(tf('git_bundle_imported', {n: j.result.files}) + (j.result.warnings.length ? ' · ⚠ ' + j.result.warnings[0] : ''), j.result.warnings.length ? 'warning' : 'success', 5000);
  }

  // ── runs ──
  // "Check now" checks everything (discovery on the servers of the settings, targets first)
  function check(discoverOnly) {
    if (G.busy || jobRunning()) return;
    startJob({all: true, discover: true, refresh: !discoverOnly});
  }

  async function refresh() {
    if (G.busy) return;
    G.busy = true; render();
    try {
      await gitClient.refreshConnected();
      const r = await fetch('/api/git/targets/refresh', json('POST', {}));
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      G.state = (await r.json()).state;
    } catch(e) {showToast(e.message,'error');} finally { G.busy = false; render(); }
  }

  // ── deploy: update, upgrade, rollback, stamp, restart ──
  const D = {plan: null, kind: '', ref: '', items: [], body: null};
  const can = k => !!(G.state.deploy || {})[k];
  const kindName = k => t('git_k_' + k);
  function runBadge(s) { return `<span class="g-st s-${s === 'ok' ? 'rs-ok' : esc(s)}">${esc(t('git_rs_' + s))}</span>`; }
  function localISO(v) { const d = new Date(v); return isNaN(d) ? '' : d.toISOString(); }
  function defaultTime(h) { const d = new Date(Date.now() + h * 3600e3); d.setMinutes(0, 0, 0); const p = n => String(n).padStart(2, '0'); return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:00`; }
  function installById(id) { return (G.state.installs || []).find(x => x.id === id) || {}; }

  function actionButtons(id) {
    const a = id ? `[${id}]` : 'null';
    const b = (k, label, cls) => `<button class="${cls || 'btn-sec'} btn-sm" ${can(k === 'stamp' ? 'update' : k) ? '' : `disabled title="${esc(t('git_no_permission'))}"`} onclick="gitWorkspace.${k}(${a})">${esc(t(label))}</button>`;
    return b('update', 'git_update', 'btn-sm') + b('upgrade', 'git_upgrade') + (id ? b('rollback', 'git_rollback') : '') + b('restart', 'git_restart') + b('stamp', 'git_stamp') + b('transfer', 'git_transfer');
  }

  function pickIds(ids) {
    ids = ids || [...G.sel];
    if (!ids.length) showToast(t('git_select_installs'), 'warning');
    return ids;
  }

  async function plan(kind, ids, ref) {
    modal(`<h3>${esc(kindName(kind))}</h3><div class="hint-box">⏳ ${esc(t('git_plan_loading'))}</div>`);
    const r = await fetch('/api/git/plan', json('POST', {kind, ref: ref || '', install_ids: ids}));
    if (!r.ok) { modal(`<h3>${esc(kindName(kind))}</h3><div class="hint-box bad">${esc(await apiError(r))}</div><div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`); return null; }
    return (await r.json()).items;
  }

  function update(ids) { ids = pickIds(ids); if (ids.length) openDeploy('update', ids, ''); }

  function upgrade(ids) {
    ids = pickIds(ids);
    if (!ids.length) return;
    modal(`<h3>${esc(kindName('upgrade'))}</h3><div class="hint-box warn">${esc(t('git_upgrade_warn'))}</div>
      <div class="g-grid"><div><label>${esc(t('git_upgrade_ref'))}</label><input id="gu-ref" value="${esc(D.ref || '')}" placeholder="${esc(t('git_upgrade_ref_h'))}"></div></div>
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gu-go">${esc(t('git_show_changes'))}</button></div>`);
    const go = () => { const ref = $('gu-ref').value.trim(); if (ref) { D.ref = ref; openDeploy('upgrade', ids, ref); } };
    $('gu-go').onclick = go;
    $('gu-ref').onkeydown = e => { if (e.key === 'Enter') go(); };
  }

  async function openDeploy(kind, ids, ref) {
    const items = await plan(kind, ids, ref);
    if (!items) return;
    D.plan = items; D.kind = kind; D.ref = ref;
    const st = G.state.settings || {};
    const apps = [...new Set(items.map(i => i.app))];
    const units = items.filter(i => (i.units || []).length);
    const fileRow = (i, idx, f) => {
      const dis = f.state === 'ok' || f.state === 'protected';
      return `<tr class="${f.state === 'ok' ? 'g-okrow' : ''}"><td><input type="checkbox" class="gd-f" data-i="${idx}" data-p="${esc(f.path)}" data-local="${f.local ? 1 : 0}" ${f.selected ? 'checked' : ''} ${dis ? 'disabled' : ''} onchange="gitWorkspace.localWarn()"></td>
        <td class="g-mono">${esc(f.path)}</td><td>${fsBadge(f.state)}${f.local ? ` <span class="g-mut">${esc(t('git_local'))}</span>` : ''}${f.behind ? ` <span class="g-mut">${esc(tf('git_behind', {n: f.behind}))} (${esc(f.tag || '')} ${esc(f.date || '')})</span>` : ''}</td></tr>`;
    };
    const itemHTML = (i, idx) => {
      const changes = (i.files || []).filter(f => f.state !== 'ok' && f.state !== 'protected');
      const head = `<h4>${esc(hn(i.conn_name, i.hostname))} · ${esc(i.app)} · <span class="g-mono">${esc(i.path)}</span>${i.env ? `<span class="g-env">${esc(i.env)}</span>` : ''}${i.prod ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}</h4>`;
      if (i.error) return `<div class="g-item">${head}<div class="hint-box bad">${esc(i.error)}</div></div>`;
      const ver = `<div class="g-mut">${esc(t('git_version_md'))}: <span class="g-mono">${esc(i.from_version || t('git_vmd_missing'))}</span> → ${esc(t('git_target'))}: <span class="g-mono">${esc(i.target ? i.target.version : '—')}</span> ${i.target ? `<span class="g-mut">${esc(i.target.branch)} · ${esc(i.target.commit_date || '')}</span>` : ''}${kind === 'upgrade' && i.current ? ` <span class="g-mut">(${esc(t('git_from'))} ${esc(i.current.version)} · ${esc(i.current.branch)})</span>` : ''}</div>`;
      if (i.pipeline) return `<div class="g-item">${head}${ver}<div class="hint-box">🔧 ${esc(tf('git_ci_item', {kind: i.pipeline.kind === 'gitlab' ? 'GitLab' : 'Jenkins'}))}<div class="g-mono g-mut">${esc(i.pipeline.url)}</div></div></div>`;
      if (kind === 'stamp') {
        const differ = (i.files || []).filter(f => f.state !== 'ok' && f.state !== 'protected');
        const list = differ.length ? `<div class="g-files"><table>${differ.slice(0, 50).map(f => `<tr><td class="g-mono">${esc(f.path)}</td><td>${fsBadge(f.state)}</td></tr>`).join('')}</table>${differ.length > 50 ? `<div class="g-mut">${esc(tf('git_stamp_more', {n: differ.length - 50}))}</div>` : ''}</div>` : '';
        return `<div class="g-item">${head}${ver}<div>${i.eligible ? fsBadge('ok') : `<span class="g-st s-review">${esc(t('git_not_current'))}</span> <span class="g-mut">${esc(tf('git_stamp_differ', {n: differ.length}))}</span>`} ${i.has_version_md ? `<span class="g-mut">${esc(t('git_has_vmd'))}</span>` : ''}</div>${list}</div>`;
      }
      const c = i.counts || {};
      const sum = kind === 'upgrade' ? `<div class="hint-box warn">${esc(tf('git_changes_summary', {changed: (c.changed || 0) + (c.old || 0), missing: c.missing || 0, local: c.modified || 0, removed: (i.removed || []).length}))}${(i.removed || []).length ? `<div class="g-mono g-mut">${(i.removed || []).map(esc).join(', ')}</div>` : ''}</div>` : '';
      return `<div class="g-item">${head}${ver}${sum}${changes.length ? '' : `<div class="g-mut">${esc(t('git_files_none'))}</div>`}
        <div class="g-files"><table>${(i.files || []).map(f => fileRow(i, idx, f)).join('')}</table></div></div>`;
    };
    const restartHTML = () => {
      if (kind === 'stamp') return '';
      if (!units.length) return `<div class="g-card"><h3>${esc(t('git_restart_choice'))}</h3><div class="g-mut">${esc(t('git_no_units_restart'))}</div></div>`;
      const dis = can('restart') ? '' : 'disabled';
      return `<div class="g-card"><h3>${esc(t('git_restart_choice'))}</h3>
        <label class="g-opt"><input type="radio" name="gd-rm" value="none" checked onchange="gitWorkspace.restartWarn()"><span>${esc(t('git_rm_none'))}</span></label>
        <label class="g-opt"><input type="radio" name="gd-rm" value="now" ${dis} onchange="gitWorkspace.restartWarn()"><span>${esc(t('git_rm_now'))}</span></label>
        <label class="g-opt"><input type="radio" name="gd-rm" value="at" ${dis} onchange="gitWorkspace.restartWarn()"><span>${esc(t('git_rm_at'))}</span> <input type="datetime-local" id="gd-rat" value="${defaultTime(2)}"></label>
        <label class="g-opt"><input type="radio" name="gd-rm" value="delay" ${dis} onchange="gitWorkspace.restartWarn()"><span>${esc(t('git_rm_delay'))}</span> <input type="number" id="gd-rdelay" min="1" max="10080" value="15" style="width:90px"></label>
        <div id="gd-rwarn" class="hint-box warn" style="display:none">${esc(tf('git_restart_warn', {list: units.map(i => i.conn_name + ': ' + i.units.map(u => u.name).join(', ')).join('; ')}))}</div>
        ${!can('restart') ? `<div class="g-mut">${esc(t('git_no_permission'))}</div>` : ''}</div>`;
    };
    modal(`<h3>${esc(kindName(kind))}${kind === 'upgrade' ? ' → <span class="g-mono">' + esc(ref) + '</span>' : ''}</h3>
      ${kind === 'upgrade' ? `<div class="hint-box warn">${esc(t('git_upgrade_warn'))} ${esc(t('git_after_upgrade'))}</div>` : ''}
      ${kind === 'stamp' ? `<div class="g-mut">${esc(t('git_stamp_h'))}</div>` : `<label class="g-opt"><input type="checkbox" onchange="document.getElementById('gd-items').classList.toggle('g-hideok', !this.checked)"><span>${esc(t('git_show_all'))}</span></label>`}
      <div id="gd-items" class="g-hideok">${items.map(itemHTML).join('')}</div>
      <div id="gd-lwarn" class="hint-box bad" style="display:none">${esc(t('git_local_warn'))}</div>
      ${kind === 'stamp' ? `<label class="g-opt"><input type="checkbox" id="gd-force" ${items.some(i => !i.error && i.has_version_md) ? '' : 'disabled'}><span>${esc(t('git_force_stamp'))}</span></label>
        ${items.some(i => !i.error && !i.eligible) ? `<label class="g-opt"><input type="checkbox" id="gd-partial"><span>${esc(t('git_stamp_partial'))}</span></label><div class="g-mut">${esc(t('git_stamp_partial_h'))}</div>` : ''}` : `<div class="g-card"><div class="g-grid">${apps.map(a => `<div><label>${esc(tf('git_check_cmd', {app: a}))}</label><input class="gd-check" data-app="${esc(a)}" value="${esc((st.check_commands || {})[a] || '')}" placeholder="./run.py --selftest"></div>`).join('')}</div>
        <label class="g-opt"><input type="checkbox" id="gd-noimports"><span>${esc(t('git_ignore_imports'))}</span></label>
        ${kind === 'upgrade' ? `<label class="g-opt"><input type="checkbox" id="gd-override"><span>${esc(t('git_set_override'))}</span></label>` : ''}</div>`}
      ${restartHTML()}
      ${kind === 'stamp' ? '' : `<div class="g-card"><h3>${esc(t('git_when'))}</h3>
        <label class="g-opt"><input type="radio" name="gd-when" value="now" checked><span>${esc(t('git_run_now'))}</span></label>
        <label class="g-opt"><input type="radio" name="gd-when" value="at"><span>${esc(t('git_run_at'))}</span> <input type="datetime-local" id="gd-at" value="${defaultTime(1)}"></label>
        <div class="g-mut">${esc(tf('git_grace_h', {n: G.state.grace_minutes || 30}))}</div></div>`}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gd-next">${esc(t('git_continue'))}</button></div>`);
    $('gd-next').onclick = deployNext;
  }

  function localWarn() { $('gd-lwarn').style.display = [...document.querySelectorAll('.gd-f:checked')].some(x => x.dataset.local === '1') ? '' : 'none'; }
  function restartWarn() { const r = document.querySelector('input[name=gd-rm]:checked'); $('gd-rwarn').style.display = r && r.value !== 'none' ? '' : 'none'; }

  function deployNext() {
    const kind = D.kind;
    const items = [];
    D.plan.forEach((i, idx) => {
      if (i.error) return;
      if (kind === 'stamp') { if (i.eligible || ($('gd-partial') && $('gd-partial').checked)) items.push({install_id: i.install_id, review_id:i.review_id}); return; }
      if (i.pipeline) { items.push({install_id: i.install_id, review_id:i.review_id}); return; }
      const boxes = [...document.querySelectorAll(`.gd-f[data-i="${idx}"]:checked`)];
      if (!boxes.length) return;
      items.push({install_id: i.install_id, review_id:i.review_id, files: boxes.map(b => b.dataset.p), modified_ok: boxes.filter(b => b.dataset.local === '1').map(b => b.dataset.p)});
    });
    if (!items.length) {
      // say why nothing can run instead of a generic "no files" (stamp has no file list)
      const usable = D.plan.filter(i => !i.error);
      const msg = !usable.length ? 'git_nothing_usable' : kind === 'stamp' ? 'git_stamp_none' : 'git_nothing_selected';
      showToast(t(msg), 'warning', 6000); return;
    }
    const body = {kind, ref: D.ref || '', items};
    if (kind === 'stamp') {
      body.force = $('gd-force').checked;
      body.stamp_partial = !!($('gd-partial') && $('gd-partial').checked);
      const chosen = D.plan.filter(i => items.some(x => x.install_id === i.install_id));
      if (!body.force && chosen.every(i => i.has_version_md)) { showToast(t('git_stamp_vmd_exists'), 'warning', 6000); return; }
    }
    else {
      body.checks = Object.fromEntries([...document.querySelectorAll('.gd-check')].map(x => [x.dataset.app, x.value.trim()]));
      body.ignore_imports = $('gd-noimports').checked;
      if ($('gd-override')) body.set_ref_override = $('gd-override').checked;
      const rm = document.querySelector('input[name=gd-rm]:checked');
      body.restart = {mode: rm ? rm.value : 'none'};
      if (body.restart.mode === 'at') body.restart.at = localISO($('gd-rat').value);
      if (body.restart.mode === 'delay') body.restart.delay_minutes = Number($('gd-rdelay').value) || 0;
      if (document.querySelector('input[name=gd-when]:checked').value === 'at') {
        body.schedule_at = localISO($('gd-at').value);
        if (!body.schedule_at || new Date(body.schedule_at) < new Date()) { showToast(t('git_time_past'), 'warning'); return; }
      }
      if (body.restart.mode === 'at' && (!body.restart.at || new Date(body.restart.at) < new Date())) { showToast(t('git_time_past'), 'warning'); return; }
    }
    confirmRun(body, D.plan.filter(i => items.some(x => x.install_id === i.install_id)));
  }

  // second confirmation: a summary, and the server name typed for production installations
  function confirmRun(body, list) {
    D.body = body;
    const servers = [...new Set(list.map(i => i.conn_name))];
    const prod = [...new Map(list.filter(i => i.prod).map(i => [i.conn_name, i])).values()];
    const rm = (body.restart || {}).mode;
    const units = list.filter(i => (i.units || []).length);
    modal(`<h3>${esc(tf('git_confirm_title', {kind: kindName(body.kind)}))}</h3>
      <div>${esc(tf('git_confirm_q', {kind: kindName(body.kind), n: list.length, m: servers.length}))}</div>
      <ul>${list.map(i => `<li><b>${esc(i.app)}</b> @ ${esc(hn(i.conn_name, i.hostname))} <span class="g-mono">${esc(i.path)}</span>${i.prod ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}${body.backup_names ? ` ← <span class="g-mono">${esc(body.backup_names[i.install_id] || '')}</span>` : ''}</li>`).join('')}</ul>
      ${body.kind === 'upgrade' ? `<div class="hint-box warn">${esc(t('git_upgrade_warn'))} → <b class="g-mono">${esc(body.ref)}</b></div>` : ''}
      ${rm && rm !== 'none' && units.length ? `<div class="hint-box warn">${esc(tf('git_restart_warn', {list: units.map(i => i.conn_name + ': ' + i.units.map(u => u.name).join(', ')).join('; ')}))}${body.restart.at ? ' · ' + esc(fmtTime(body.restart.at)) : ''}${body.restart.delay_minutes ? ' · +' + body.restart.delay_minutes + ' min' : ''}</div>` : ''}
      ${body.schedule_at ? `<div class="hint-box">⏰ ${esc(tf('git_scheduled_ok', {t: fmtTime(body.schedule_at)}))}</div>` : ''}
      ${prod.map(i => `<div class="g-conf"><label>${esc(tf('git_type_host', {name: i.conn_name}))}</label><input class="gd-conf" data-name="${esc(i.conn_name)}" autocomplete="off" oninput="gitWorkspace.confirmCheck()"></div>`).join('')}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gd-go" class="${body.kind === 'stamp' ? '' : 'btn-danger'}" ${prod.length ? 'disabled' : ''}>${esc(t(body.schedule_at || (body.kind === 'restart' && rm !== 'now') ? 'git_schedule' : 'git_start'))}</button></div>`);
    D.items = list;
    $('gd-go').onclick = startRun;
  }

  function confirmCheck() { $('gd-go').disabled = ![...document.querySelectorAll('.gd-conf')].every(x => x.value.trim().toLowerCase() === x.dataset.name.trim().toLowerCase()); }

  async function startRun() {
    const body = Object.assign({}, D.body);
    delete body.backup_names;
    const typed = Object.fromEntries([...document.querySelectorAll('.gd-conf')].map(x => [x.dataset.name, x.value.trim()]));
    body.confirm = Object.fromEntries(D.items.filter(i => i.prod).map(i => [String(i.install_id), typed[i.conn_name] || '']));
    $('gd-go').disabled = true;
    const r = await fetch('/api/git/runs', json('POST', body));
    if (!r.ok) { $('gd-go').disabled = false; showToast(await apiError(r), 'error', 7000); return; }
    const v = await r.json();
    if (v.state === 'scheduled') {
      showToast(tf('git_scheduled_ok', {t: fmtTime(v.scheduled_at)}), 'success', 5000);
      closeModal(); G.tab = 'runs'; render(); loadRuns();
      return;
    }
    showRun(v.id);
  }

  function rollback(ids) {
    ids = pickIds(ids);
    if (!ids.length) return;
    const i = installById(ids[0]);
    modal(`<h3>${esc(kindName('rollback'))}: ${esc(i.conn_name || '')} · <span class="g-mono">${esc(i.path || '')}</span></h3><div class="hint-box">⏳ ${esc(t('git_plan_loading'))}</div>`);
    fetch(`/api/git/installs/${ids[0]}/backups`).then(async r => {
      if (!r.ok) { modal(`<h3>${esc(kindName('rollback'))}</h3><div class="hint-box bad">${esc(await apiError(r))}</div><div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`); return; }
      const j = await r.json();
      const list = j.backups || [];
      modal(`<h3>${esc(kindName('rollback'))}: ${esc(i.conn_name || '')} · <span class="g-mono">${esc(i.path || '')}</span></h3>
        <div class="g-mut">${esc(t('git_restore_hint'))}</div><h3 style="margin-top:10px">${esc(t('git_backups'))} <span class="g-mut g-mono">${esc(j.dir || '')}</span></h3>
        ${list.length ? `<div class="g-files"><table>${list.map((b, k) => `<tr><td><input type="radio" name="gr-b" value="${esc(b.name)}" ${k === 0 ? 'checked' : ''}></td>
          <td><span class="g-mono">${esc(b.name)}</span><div class="g-mut">${esc(b.time || '')}${b.user ? ' · ' + esc(b.user) : ''}${b.version ? ' · VERSION.md ' + esc(b.version) : ''}${b.from_version ? ' · ' + esc(b.from_version) + ' → ' + esc(b.to_version || '') : ''}</div>
          <details><summary class="g-mut">${esc(tf('git_backup_files', {n: (b.log_files || b.files || []).length}))}</summary><div class="g-mono g-mut">${(b.log_files || b.files || []).map(esc).join('<br>')}</div></details>
          ${b.log_files ? '' : `<div class="g-mut">⚠ ${esc(t('git_backup_unknown'))}</div>`}</td></tr>`).join('')}</table></div>` : `<div class="hint-box">${esc(t('git_no_backups'))}</div>`}
        <div class="g-card" style="margin-top:10px"><label>${esc(tf('git_check_cmd', {app: i.app}))}</label><input id="gr-check" value="${esc(((G.state.settings || {}).check_commands || {})[i.app] || '')}"></div>
        <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button>${list.length ? `<button id="gr-next">${esc(t('git_continue'))}</button>` : ''}</div>`);
      if (!list.length) return;
      $('gr-next').onclick = () => {
        const b = document.querySelector('input[name=gr-b]:checked').value;
        confirmRun({kind: 'rollback', items: [{install_id: i.id, backup: b}], checks: {[i.app]: $('gr-check').value.trim()}, backup_names: {[i.id]: b}},
          [{install_id: i.id, app: i.app, conn_name: i.conn_name, path: i.path, prod: isProd(i), units: i.units}]);
      };
    });
  }

  function isProd(i) {
    const p = ['prod', 'production', 'prd', 'live'];
    return p.includes(String(i.env || '').toLowerCase()) || (i.tags || []).some(x => p.includes(x.toLowerCase()) || (x.toLowerCase().startsWith('env:') && p.includes(x.toLowerCase().slice(4))));
  }

  function restart(ids) {
    ids = pickIds(ids);
    if (!ids.length) return;
    const list = ids.map(installById).filter(i => i.id);
    const withUnits = list.filter(i => (i.units || []).length);
    modal(`<h3>${esc(kindName('restart'))}</h3>
      ${withUnits.length ? `<table><tr><th>${esc(t('git_server'))}</th><th>${esc(t('git_path'))}</th><th>${esc(t('git_unit_list'))}</th></tr>${withUnits.map(i => `<tr><td>${esc(hn(i.conn_name, i.hostname))}${isProd(i) ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}</td><td class="g-mono">${esc(i.path)}</td><td class="g-mono">${i.units.map(u => esc(u.kind + ': ' + u.name)).join('<br>')}</td></tr>`).join('')}</table>`
        : `<div class="hint-box">${esc(t('git_no_units_restart'))}</div>`}
      ${withUnits.length ? `<div class="g-card" style="margin-top:10px"><label class="g-opt"><input type="radio" name="gx-rm" value="now" checked><span>${esc(t('git_run_now'))}</span></label>
        <label class="g-opt"><input type="radio" name="gx-rm" value="at"><span>${esc(t('git_rm_at'))}</span> <input type="datetime-local" id="gx-rat" value="${defaultTime(2)}"></label>
        <label class="g-opt"><input type="radio" name="gx-rm" value="delay"><span>${esc(t('git_rm_delay'))}</span> <input type="number" id="gx-rdelay" min="1" max="10080" value="15" style="width:90px"></label>
        <div class="g-mut">${esc(tf('git_grace_h', {n: G.state.grace_minutes || 30}))}</div></div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button>${withUnits.length ? `<button id="gx-next">${esc(t('git_continue'))}</button>` : ''}</div>`);
    if (!withUnits.length) return;
    $('gx-next').onclick = () => {
      const mode = document.querySelector('input[name=gx-rm]:checked').value;
      const body = {kind: 'restart', items: withUnits.map(i => ({install_id: i.id})), restart: {mode}};
      if (mode === 'at') { body.restart.at = localISO($('gx-rat').value); if (!body.restart.at || new Date(body.restart.at) < new Date()) { showToast(t('git_time_past'), 'warning'); return; } }
      if (mode === 'delay') body.restart.delay_minutes = Number($('gx-rdelay').value) || 0;
      confirmRun(body, withUnits.map(i => ({install_id: i.id, app: i.app, conn_name: i.conn_name, path: i.path, prod: isProd(i), units: i.units})));
    };
  }

  function stamp(ids) { ids = pickIds(ids); if (ids.length) openDeploy('stamp', ids, ''); }

  // ── new server: install and transfer wizards ──
  const W = {};
  const kb = n => Math.ceil((n || 0) / 1024);
  const sshConns = () => (conns || []).filter(c => c.protocol === 'SSH').sort((a, b) => a.name.localeCompare(b.name));
  const errBox = async r => `<div class="hint-box bad">${esc(await apiError(r))}</div>`;
  const btns = (next, label) => `<div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="${next}">${esc(t(label))}</button></div>`;

  function install() {
    if (!can('install')) return;
    const s = G.state;
    const apps = [...s.catalog.apps.map(a => a.name), ...(s.bundle_apps || [])];
    const targets = Object.fromEntries(s.targets.map(x => [x.app, x]));
    const bundles = s.sources.filter(x => x.kind === 'bundle');
    W.kind = 'install';
    const refSel = a => `<select class="gw-ref" data-app="${esc(a)}" onchange="gitWorkspace.wSvcRef(this)" style="width:auto">
        <option value="">${esc(tf('git_w_ref_target', {v: targets[a] ? targets[a].version : t('git_no_target')}))}</option>
        ${s.sources.some(x => x.kind !== 'bundle') ? `<option value="?">${esc(t('git_w_ref_other'))}</option>` : ''}
        ${bundles.filter(b => (b.apps || []).includes(a)).map(b => `<option value="bundle:${b.id}">${esc(tf('git_w_ref_bundle', {b: b.bundle_id || b.name}))}</option>`).join('')}</select>
        <input class="gw-refx" data-app="${esc(a)}" placeholder="tag:v2 / main" style="display:none;width:130px">`;
    modal(`<h3>${esc(t('git_k_install'))}: ${esc(t('git_new_server'))}</h3>
      <div class="g-grid"><div><label>${esc(t('git_w_server'))}</label><select id="gw-conn">${sshConns().map(c => `<option value="${c.id}">${esc(c.name)} (${esc(c.host)})</option>`).join('')}</select></div></div>
      <div class="g-card"><h3>${esc(t('git_w_services'))}</h3><div class="g-files"><table>${apps.map(a => `<tr><td><input type="checkbox" class="gw-app" value="${esc(a)}"></td><td><b>${esc(a)}</b></td><td>${refSel(a)}</td></tr>`).join('')}</table></div></div>
      ${btns('gw-next', 'git_continue')}`);
    $('gw-next').onclick = () => {
      const svcs = [...document.querySelectorAll('.gw-app:checked')].map(x => {
        const a = x.value, sel = document.querySelector(`.gw-ref[data-app="${CSS.escape(a)}"]`);
        const ref = sel.value === '?' ? document.querySelector(`.gw-refx[data-app="${CSS.escape(a)}"]`).value.trim() : sel.value;
        return {app: a, ref};
      });
      if (!svcs.length || !$('gw-conn').value) { showToast(t('git_w_pick_service'), 'warning'); return; }
      W.conn = Number($('gw-conn').value); W.svcs = svcs;
      installDirs();
    };
  }
  function wSvcRef(sel) { const x = document.querySelector(`.gw-refx[data-app="${CSS.escape(sel.dataset.app)}"]`); x.style.display = sel.value === '?' ? '' : 'none'; if (sel.value === '?') x.focus(); }

  async function prepare(body) {
    const r = await fetch('/api/git/provision/prepare', json('POST', body));
    if (!r.ok) { modal(`<h3>${esc(t('git_k_' + W.kind))}</h3>${await errBox(r)}<div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`); return null; }
    return r.json();
  }

  function rootsHTML(p) {
    return `<div class="g-mut">${esc(t('git_w_roots'))}: ${(p.roots || []).map(r => `<span class="g-mono">${esc(r.path)}</span> ${r.exists ? (r.writable ? '✅' : '⚠ ' + esc(t('git_w_root_ro'))) : '✗ ' + esc(t('git_w_root_missing'))}`).join(' · ') || '—'}</div>`;
  }

  async function installDirs() {
    modal(`<h3>${esc(t('git_k_install'))}</h3><div class="hint-box">⏳ ${esc(t('git_plan_loading'))}</div>`);
    const p = await prepare({kind: 'install', conn_id: W.conn, services: W.svcs});
    if (!p) return;
    const root = ((p.roots || []).find(r => r.exists && r.writable) || (p.roots || [])[0] || {}).path || '/opt';
    modal(`<h3>${esc(t('git_k_install'))} → ${esc(p.conn_name)}${p.prod ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}</h3>${rootsHTML(p)}
      <div class="g-card"><h3>${esc(t('git_w_dirs'))}</h3>${p.services.map((x, i) => `<div class="g-item"><h4>${esc(x.app)} ${x.target ? `<span class="g-mono">${esc(x.target.version)}</span> <span class="g-mut">${esc(x.target.branch)} · ${esc(x.target.source)} · ${esc(tf('git_w_files', {n: x.files, kb: kb(x.bytes)}))}</span>` : ''}</h4>
        ${x.error ? `<div class="hint-box bad">${esc(x.error)}</div>` : `<div class="g-grid"><div><label>${esc(t('git_path'))}</label><input class="gw-path" data-i="${i}" value="${esc(root.replace(/\/$/, '') + '/' + x.app)}"></div>
        <div><label>${esc(t('git_w_env'))}</label><input class="gw-env" data-i="${i}" placeholder="test / prod"></div></div>`}</div>`).join('')}</div>
      ${btns('gw-next', 'git_w_check')}`);
    if (p.services.some(x => x.error)) { $('gw-next').disabled = true; return; }
    $('gw-next').onclick = () => {
      W.svcs = W.svcs.map((x, i) => Object.assign({}, x, {path: document.querySelector(`.gw-path[data-i="${i}"]`).value.trim(), env: document.querySelector(`.gw-env[data-i="${i}"]`).value.trim()}));
      installCheck();
    };
  }

  function checkHTML(c, idx) {
    if (!c) return '';
    const tools = Object.entries(c.tools || {}).filter(([k]) => k !== 'tar').map(([k, v]) => `${esc(k)} <span class="g-mono">${esc(v)}</span>`);
    const errs = c.errors || [];
    return `<div class="g-mut">${esc(t('git_w_checks'))}: ${esc(c.parent)} · ${esc(tf('git_w_free', {free: Math.floor(c.free_kb / 1024), need: Math.ceil(c.need_kb / 1024)}))}${tools.length ? ' · ' + tools.join(', ') : ''}${(c.needs || []).filter(n => !(c.tools || {})[n]).map(n => ` · ⚠ ${esc(n)} ${esc(t('git_w_missing_tool'))}`).join('')}</div>
      ${errs.map(e => `<div class="hint-box bad">${esc(e)}</div>`).join('')}${(c.warnings || []).map(w => `<div class="hint-box warn">${esc(w)}</div>`).join('')}
      ${c.existing ? `<div class="hint-box warn">${esc(tf('git_w_existing', {n: c.files, v: c.version ? ', VERSION.md ' + c.version : ''}))}<label class="g-opt"><input type="checkbox" class="gw-over" data-i="${idx}"><span>${esc(t('git_w_overwrite'))}</span></label></div>` : ''}`;
  }
  const blocking = c => c && (c.errors || []).length > 0; // prepared with overwrite: an existing installation is a tick, not an error

  function slotHTML(i, k, sl) {
    const tmpls = sl.templates.filter(x => x.available);
    const mode = tmpls.length ? 'template' : sl.copies.length ? 'copy' : 'skip';
    const radio = (m, label, dis) => `<label class="g-opt"><input type="radio" name="gw-m-${i}-${k}" value="${m}" ${m === mode ? 'checked' : ''} ${dis ? 'disabled' : ''} onchange="gitWorkspace.wSlotMode(${i}, ${k})"><span>${esc(t(label))}</span></label>`;
    const vaultOpts = (W.creds || []).map(c => `<option value="${c.id}:password">${esc(tf('git_w_vault_pw', {n: c.name}))}</option><option value="${c.id}:username">${esc(tf('git_w_vault_user', {n: c.name}))}</option>`).join('');
    const fields = (tp, ti) => `<div class="g-grid gw-fields" data-t="${ti}" ${ti ? 'style="display:none"' : ''}>${tp.fields.map(f => `<div><label class="g-mono">${esc(f.name)}</label>
      <input class="gw-fv" data-id="${esc(f.id)}" data-def="${esc(f.default)}" value="${esc(f.default)}" type="${f.secret ? 'password' : 'text'}" autocomplete="off">
      ${vaultOpts ? `<select class="gw-fs" data-id="${esc(f.id)}" onchange="this.previousElementSibling.style.display = this.value ? 'none' : ''"><option value="">${esc(t('git_w_typed'))}</option>${vaultOpts}</select>` : ''}</div>`).join('')}</div>`;
    return `<div class="g-item gw-slot" data-i="${i}" data-k="${k}" data-path="${esc(sl.path)}"><h4><span class="g-mono">${esc(sl.path)}</span> ${sl.protected ? fsBadge('protected') : ''}</h4>
      <div class="g-bar">${radio('template', 'git_w_m_template', !tmpls.length)}${radio('copy', 'git_w_m_copy', !sl.copies.length)}${radio('manual', 'git_w_m_manual')}${radio('skip', 'git_w_m_skip')}</div>
      <div class="gw-p gw-p-template" ${mode === 'template' ? '' : 'style="display:none"'}>${tmpls.length > 1 ? `<select class="gw-tsel" onchange="this.parentElement.querySelectorAll('.gw-fields').forEach(x => x.style.display = x.dataset.t === this.value ? '' : 'none')">${tmpls.map((x, ti) => `<option value="${ti}">${esc(x.path)}</option>`).join('')}</select>` : ''}
        ${tmpls.map((tp, ti) => `<div class="g-mut gw-tname" data-t="${ti}" data-path="${esc(tp.path)}" ${ti ? 'style="display:none"' : ''}>${esc(tp.path)}${sl.in_repo && tp.path === sl.path ? ' (' + esc(t('git_w_in_repo')) + ')' : ''}</div>`).join('')}
        ${tmpls.map(fields).join('')}${sl.templates.filter(x => !x.available).map(x => `<div class="g-mut">${esc(x.path)}: ${esc(t('git_w_tmpl_na'))}</div>`).join('')}</div>
      <div class="gw-p gw-p-copy" ${mode === 'copy' ? '' : 'style="display:none"'}><select class="gw-copy">${sl.copies.map(c => `<option value="${c.install_id}">${esc(c.conn_name)} · ${esc(c.path)}</option>`).join('')}</select></div>
      <div class="gw-p gw-p-manual" style="display:none"><textarea class="gw-manual" rows="5" spellcheck="false"></textarea></div></div>`;
  }
  function wSlotMode(i, k) {
    const el = document.querySelector(`.gw-slot[data-i="${i}"][data-k="${k}"]`);
    const m = el.querySelector('input[type=radio]:checked').value;
    el.querySelectorAll('.gw-p').forEach(x => { x.style.display = x.classList.contains('gw-p-' + m) ? '' : 'none'; });
  }

  function unitHTML(i, app, dir) {
    return `<div class="g-card"><label class="g-opt"><input type="checkbox" class="gw-unit-on" data-i="${i}" onchange="gitWorkspace.wUnit(${i})"><span><b>${esc(t('git_w_unit'))}</b></span></label>
      <div class="gw-unit" data-i="${i}" style="display:none"><div class="hint-box warn">${esc(t('git_w_unit_warn'))}</div><div class="g-grid">
        <div><label>${esc(t('git_w_unit_kind'))}</label><select class="gw-uk"><option value="systemd">systemd</option><option value="supervisor">supervisor</option></select></div>
        <div><label>${esc(t('git_w_unit_name'))}</label><input class="gw-un" value="${esc(app)}"></div>
        <div><label>${esc(t('git_w_unit_user'))}</label><input class="gw-uu" placeholder="svc-${esc(app)}"></div>
        <div><label>${esc(t('git_w_unit_cmd'))}</label><input class="gw-uc" placeholder="/usr/bin/python3 ${esc(dir)}/run.py"></div>
        <div><label>${esc(t('git_w_unit_dir'))}</label><input class="gw-ud" value="${esc(dir)}"></div></div>
        <label class="g-opt"><input type="checkbox" class="gw-ue" checked><span>${esc(t('git_w_unit_enable'))}</span></label></div></div>`;
  }
  function wUnit(i) { document.querySelector(`.gw-unit[data-i="${i}"]`).style.display = document.querySelector(`.gw-unit-on[data-i="${i}"]`).checked ? '' : 'none'; }
  function unitOf(i) {
    if (!document.querySelector(`.gw-unit-on[data-i="${i}"]`).checked) return null;
    const el = document.querySelector(`.gw-unit[data-i="${i}"]`), v = c => el.querySelector(c).value.trim();
    return {kind: v('.gw-uk'), name: v('.gw-un'), user: v('.gw-uu'), command: v('.gw-uc'), work_dir: v('.gw-ud'), enable: el.querySelector('.gw-ue').checked};
  }
  const startHTML = () => `<div class="g-card"><label class="g-opt"><input type="checkbox" id="gw-start" ${can('restart') ? '' : 'disabled'}><span>${esc(t('git_w_start'))}</span></label>
    <div class="g-mut">${esc(can('restart') ? t('git_w_start_h') : t('git_no_permission'))}</div></div>`;

  async function installCheck() {
    modal(`<h3>${esc(t('git_k_install'))}</h3><div class="hint-box">⏳ ${esc(t('git_plan_loading'))}</div>`);
    if (!W.creds) { const r = await fetch('/api/credentials'); W.creds = r.ok ? (await r.json()).filter(c => c.has_password || c.username) : []; }
    const p = await prepare({kind: 'install', conn_id: W.conn, services: W.svcs, overwrite: true});
    if (!p) return;
    W.prep = p;
    const st = G.state.settings || {};
    modal(`<h3>${esc(t('git_k_install'))} → ${esc(p.conn_name)}${p.prod ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}</h3>
      ${p.services.map((x, i) => `<div class="g-card"><h3>${esc(x.app)} <span class="g-mono">${esc(x.target ? x.target.version : '')}</span> → <span class="g-mono">${esc(x.path || '')}</span></h3>
        ${x.error ? `<div class="hint-box bad">${esc(x.error)}</div>` : checkHTML(x.check, i)}
        <h3 style="margin-top:10px">${esc(t('git_w_protected'))}</h3><div class="g-mut">${esc(t('git_w_protected_h'))}${W.creds.length ? ' ' + esc(t('git_w_vault_h')) : ''}</div>
        ${x.slots.length ? x.slots.map((sl, k) => slotHTML(i, k, sl)).join('') : `<div class="g-mut">${esc(t('git_w_no_slots'))}</div>`}
        <div style="margin-top:8px"><label class="g-mut">${esc(tf('git_check_cmd', {app: x.app}))}</label><input class="gw-chk" data-app="${esc(x.app)}" value="${esc((st.check_commands || {})[x.app] || '')}" style="width:100%"></div>
        ${unitHTML(i, x.app, x.path || '')}</div>`).join('')}
      ${startHTML()}<div id="gw-block" class="hint-box bad" style="display:none">${esc(t('git_w_blocked'))}</div>${btns('gw-next', 'git_continue')}`);
    $('gw-next').onclick = installNext;
  }

  function slotSpec(el) {
    const m = el.querySelector('input[type=radio]:checked').value, f = {path: el.dataset.path, mode: m};
    if (m === 'copy') f.from_install = Number(el.querySelector('.gw-copy').value);
    if (m === 'manual') f.content = el.querySelector('.gw-manual').value;
    if (m === 'template') {
      const ti = el.querySelector('.gw-tsel') ? el.querySelector('.gw-tsel').value : '0';
      f.template = el.querySelector(`.gw-tname[data-t="${ti}"]`).dataset.path;
      f.values = {};
      el.querySelectorAll(`.gw-fields[data-t="${ti}"] .gw-fv`).forEach(inp => {
        const vs = inp.parentElement.querySelector('.gw-fs');
        if (vs && vs.value) { const [id, part] = vs.value.split(':'); f.values[inp.dataset.id] = {cred_id: Number(id), part}; }
        else if (inp.value !== inp.dataset.def) f.values[inp.dataset.id] = {value: inp.value};
      });
    }
    return f;
  }

  function installNext() {
    const p = W.prep;
    let blocked = false;
    const services = p.services.map((x, i) => {
      const c = x.check;
      const over = document.querySelector(`.gw-over[data-i="${i}"]`);
      if (x.error || !c || blocking(c) || (c.existing && !(over && over.checked))) blocked = true;
      return {app: x.app, ref: W.svcs[i].ref, path: x.path, env: W.svcs[i].env, files: [...document.querySelectorAll(`.gw-slot[data-i="${i}"]`)].map(slotSpec), unit: unitOf(i)};
    });
    $('gw-block').style.display = blocked ? '' : 'none';
    if (blocked) return;
    const overwrite = p.services.some(x => x.check && x.check.existing);
    const body = {kind: 'install', provision: {conn_id: W.conn, services, overwrite}, restart: {mode: $('gw-start').checked ? 'now' : 'none'},
      checks: Object.fromEntries([...document.querySelectorAll('.gw-chk')].map(x => [x.dataset.app, x.value.trim()]))};
    provisionConfirm(body, p, services.map(x => `<li><b>${esc(x.app)}</b> → <span class="g-mono">${esc(x.path)}</span>${x.unit ? ` · ${esc(x.unit.kind)} <span class="g-mono">${esc(x.unit.name)}</span>` : ''}</li>`).join(''), overwrite);
  }

  function provisionConfirm(body, p, list, overwrite) {
    W.body = body;
    const need = p.prod || overwrite;
    modal(`<h3>${esc(tf('git_confirm_title', {kind: kindName(body.kind)}))}</h3>
      <div>${esc(tf('git_w_confirm', {kind: kindName(body.kind), n: (body.provision.services || [1]).length, server: p.conn_name}))}${p.prod ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}</div><ul>${list}</ul>
      ${overwrite ? `<div class="hint-box warn">${esc(t('git_w_overwrite'))}</div>` : ''}
      ${body.restart.mode === 'now' ? `<div class="hint-box warn">${esc(t('git_w_start'))}</div>` : ''}
      ${need ? `<div class="g-conf"><label>${esc(tf('git_w_type', {name: p.conn_name}))}</label><input id="gw-conf" autocomplete="off" oninput="gitWorkspace.wCheckGo()"></div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gw-go" class="btn-danger" ${need ? 'disabled' : ''}>${esc(t('git_start'))}</button></div>`);
    W.confName = p.conn_name;
    $('gw-go').onclick = async () => {
      const b = JSON.parse(JSON.stringify(W.body));
      if ($('gw-conf')) b.provision.confirm = $('gw-conf').value.trim();
      $('gw-go').disabled = true;
      const r = await fetch('/api/git/runs', json('POST', b));
      if (!r.ok) { $('gw-go').disabled = false; showToast(await apiError(r), 'error', 7000); return; }
      showRun((await r.json()).id);
    };
  }
  function wCheckGo() { $('gw-go').disabled = $('gw-conf').value.trim().toLowerCase() !== String(W.confName).trim().toLowerCase(); }

  function transfer(ids) {
    ids = pickIds(ids);
    if (!ids.length || !can('transfer')) return;
    const src = installById(ids[0]);
    W.kind = 'transfer'; W.src = src;
    const others = sshConns();
    modal(`<h3>${esc(t('git_k_transfer'))}: ${esc(src.app)} · ${esc(src.conn_name)} · <span class="g-mono">${esc(src.path)}</span></h3>
      <div class="g-mut">${esc(t('git_w_tr_h'))}</div>
      <div class="g-grid" style="margin-top:8px"><div><label>${esc(t('git_w_server'))}</label><select id="gw-conn">${others.map(c => `<option value="${c.id}">${esc(c.name)} (${esc(c.host)})</option>`).join('')}</select></div>
        <div><label>${esc(t('git_path'))}</label><input id="gw-path" value="${esc(src.path)}"></div>
        <div><label>${esc(t('git_w_env'))}</label><input id="gw-env" value="${esc(src.env || '')}"></div></div>
      ${btns('gw-next', 'git_w_check')}`);
    const firstOther = others.find(c => c.id !== src.conn_id);
    if (firstOther) $('gw-conn').value = String(firstOther.id);
    $('gw-next').onclick = () => {
      W.conn = Number($('gw-conn').value); W.path = $('gw-path').value.trim(); W.env = $('gw-env').value.trim();
      if (W.conn === src.conn_id && W.path === src.path) { showToast(t('git_w_same'), 'warning'); return; }
      transferCheck();
    };
  }

  async function transferCheck() {
    modal(`<h3>${esc(t('git_k_transfer'))}</h3><div class="hint-box">⏳ ${esc(t('git_plan_loading'))}</div>`);
    const p = await prepare({kind: 'transfer', conn_id: W.conn, source_install: W.src.id, path: W.path, overwrite: true});
    if (!p) return;
    W.prep = p;
    const s = p.source, c = p.check;
    modal(`<h3>${esc(t('git_k_transfer'))}: ${esc(s.app)} → ${esc(p.conn_name)}${p.prod ? `<span class="g-prod">${esc(t('git_prod'))}</span>` : ''}</h3>
      <div class="g-card"><h3>${esc(t('git_w_source'))}: ${esc(s.conn_name)} · <span class="g-mono">${esc(s.path)}</span> ${s.version ? `<span class="g-mono">${esc(s.version)}</span>` : ''}</h3>
        <div class="g-mut">${esc(tf('git_w_files', {n: s.files, kb: kb(s.bytes)}))}</div>
        ${s.excluded ? `<details><summary class="g-mut">${esc(tf('git_w_excluded', {n: s.excluded}))}</summary><div class="g-mono g-mut">${s.excluded_list.map(x => esc(x.path) + ' <i>(' + esc(x.why) + ')</i>').join('<br>')}</div></details>` : ''}
        ${(s.units || []).length ? `<div class="g-mut">${esc(tf('git_w_units_not_copied', {list: s.units.map(u => u.name).join(', ')}))}</div>` : ''}</div>
      <div class="g-card"><h3>→ <span class="g-mono">${esc(W.path)}</span></h3>${checkHTML(c, 0)}</div>
      ${unitHTML(0, s.app, W.path)}${startHTML()}
      <div class="g-card"><label class="g-mut">${esc(tf('git_check_cmd', {app: s.app}))}</label><input class="gw-chk" data-app="${esc(s.app)}" value="${esc(((G.state.settings || {}).check_commands || {})[s.app] || '')}" style="width:100%"></div>
      <div id="gw-block" class="hint-box bad" style="display:none">${esc(t('git_w_blocked'))}</div>${btns('gw-next', 'git_continue')}`);
    $('gw-next').onclick = () => {
      const over = document.querySelector('.gw-over');
      const blocked = !c || blocking(c) || (c.existing && !(over && over.checked));
      $('gw-block').style.display = blocked ? '' : 'none';
      if (blocked) return;
      const body = {kind: 'transfer', provision: {conn_id: W.conn, source_install: W.src.id, path: W.path, env: W.env, unit: unitOf(0), overwrite: !!c.existing},
        restart: {mode: $('gw-start').checked ? 'now' : 'none'}, checks: {[s.app]: document.querySelector('.gw-chk').value.trim()}};
      provisionConfirm(body, p, `<li><b>${esc(s.app)}</b>: ${esc(s.conn_name)} <span class="g-mono">${esc(s.path)}</span> → ${esc(p.conn_name)} <span class="g-mono">${esc(W.path)}</span></li>`, !!c.existing);
    };
  }

  // ── .gitignore helper ──
  const I = {app: '', info: null, loading: false, preview: null, checked: null};
  const fmtSize = n => !n ? '—' : n < 1024 ? n + ' B' : n < 1048576 ? (n / 1024).toFixed(1) + ' KB' : (n / 1048576).toFixed(1) + ' MB';
  function allApps() { return [...G.state.catalog.apps.map(a => a.name), ...(G.state.bundle_apps || [])]; }

  async function ignoreLoad(app) {
    const apps = allApps();
    I.app = app || I.app || apps[0] || '';
    I.info = null; I.preview = null; I.checked = null;
    if (!I.app) { if (G.tab === 'gitignore') render(); return; }
    I.loading = true; if (G.tab === 'gitignore') render();
    try {
      const r = await fetch('/api/git/gitignore/' + encodeURIComponent(I.app));
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      I.info = await r.json();
      // pre-selected: the patterns of the stacks found in the repository and the protected globs
      I.checked = new Set([...I.info.stacks.filter(x => x.detected).flatMap(x => x.patterns), ...I.info.protected]);
    } finally { I.loading = false; if (G.tab === 'gitignore') render(); }
  }

  function ignoreHTML() {
    const apps = allApps();
    const bar = `<div class="g-bar"><label class="g-mut">${esc(t('gi_service'))}</label><select onchange="gitWorkspace.ignoreLoad(this.value)">${apps.map(a => `<option value="${esc(a)}" ${a === I.app ? 'selected' : ''}>${esc(a)}</option>`).join('')}</select>
      <button class="btn-sec btn-sm" onclick="gitWorkspace.ignoreLoad()">↻ ${esc(t('gi_load'))}</button></div>`;
    if (!apps.length) return bar + `<div class="hint-box">${esc(t('git_empty_overview'))}</div>`;
    if (I.loading || !I.info) return bar + `<div class="hint-box">⏳ ${esc(t('gi_loading'))}</div>`;
    const x = I.info, chk = v => I.checked && I.checked.has(v) ? 'checked' : '';
    const pat = v => `<label><input type="checkbox" class="gi-p" value="${esc(v)}" ${chk(v)} onchange="gitWorkspace.ignoreMark(this)">${esc(v)}</label>`;
    let h = bar + `<div class="g-card"><div class="g-grid"><div><label>${esc(t('gi_repo'))}</label><span class="g-mono">${esc(x.repo || '—')}</span></div>
      <div><label>${esc(t('gi_branch'))}</label><span class="g-mono">${esc(x.branch || '—')}</span></div><div><label>${esc(t('gi_file'))}</label><span class="g-mono">${esc(x.file)}</span></div></div>
      ${x.note ? `<div class="hint-box warn" style="margin-top:8px">${esc(t('gi_note'))}: ${esc(x.note)}</div>` : ''}</div>`;
    if (x.warnings.length) h += `<div class="g-card g-warn"><h3>⚠ ${esc(t('gi_warnings'))}</h3>${x.warnings.map(w => `<div style="margin:6px 0"><span class="g-mono"><b>${esc(w.path)}</b></span> <span class="g-st s-review">${esc(t('gi_reason_' + w.reason))}</span>
      <div class="g-mut">${esc(w.reason === 'log' ? tf('gi_warn_log', {path: w.path}) : tf('gi_warn_advice', {tmpl: w.template, path: w.path}))}</div></div>`).join('')}</div>`;
    h += `<div class="g-card"><h3>${esc(t('gi_extra'))}</h3>${x.extra.length ? `<div class="g-scroll" style="max-height:40vh"><table><tr><th></th><th>${esc(t('git_path'))}</th><th>${esc(t('gi_size'))}</th><th>${esc(t('git_server'))}</th><th>${esc(t('gi_pattern'))}</th></tr>
      ${x.extra.map(c => `<tr><td><input type="checkbox" class="gi-p" value="${esc(c.pattern)}" ${chk(c.pattern)} onchange="gitWorkspace.ignoreMark(this)"></td><td class="g-mono">${esc(c.path)}</td><td class="g-mut" style="white-space:nowrap">${esc(fmtSize(c.size))}</td>
        <td class="g-mut">${esc(c.servers.join(', '))}</td><td class="g-mono">${esc(c.pattern)}</td></tr>`).join('')}</table></div>` : `<div class="g-mut">${esc(t('gi_extra_none'))}</div>`}</div>`;
    h += `<div class="g-card"><h3>${esc(t('gi_stacks'))}</h3>${x.stacks.map(st => `<div style="margin:6px 0"><b>${esc(st.name)}</b>${st.detected ? ` <span class="g-st s-ok">${esc(tf('gi_detected', {f: st.reason}))}</span>` : ''}<div class="g-pats">${st.patterns.map(pat).join('')}</div></div>`).join('')}
      ${x.protected.length ? `<div style="margin:10px 0 6px"><b>${esc(t('gi_protected'))}</b><div class="g-pats">${x.protected.map(pat).join('')}</div></div>` : ''}
      <div class="g-grid" style="margin-top:8px"><div><label>${esc(t('gi_custom'))}</label><textarea id="gi-custom" placeholder="${esc(t('gi_custom_h'))}">${esc(I.custom || '')}</textarea></div></div></div>`;
    h += `<div class="g-card"><h3>${esc(t('gi_lists'))}</h3><div class="g-mut" style="margin-bottom:6px">${esc(t('gi_lists_h'))}</div>${x.in_catalog ? `<div class="g-grid">
      <div><label>${esc(t('git_f_exclude'))}</label><textarea id="gi-exclude">${esc(x.exclude.join('\n'))}</textarea></div>
      <div><label>${esc(t('git_f_protected'))}</label><textarea id="gi-protected">${esc(x.protected.join('\n'))}</textarea></div></div>
      <div class="g-bar" style="margin-top:8px"><button class="btn-sec btn-sm" onclick="gitWorkspace.ignoreSaveLists()">${esc(t('git_save'))}</button></div>` : `<div class="g-mut">${esc(t('gi_not_catalog'))}</div>`}</div>`;
    h += `<div class="g-card"><h3>${esc(t('gi_current'))}</h3>${x.has_current ? `<pre class="g-pre">${esc(x.current)}</pre>` : `<div class="g-mut">${esc(t('gi_no_current'))}</div>`}</div>`;
    const mrLabel = t(x.source_kind === 'github' ? 'gi_mr_gh' : 'gi_mr');
    h += `<div class="g-bar"><button onclick="gitWorkspace.ignorePreview()">${esc(t('gi_preview'))}</button><button class="btn-sec" onclick="gitWorkspace.ignoreDownload()">⬇ ${esc(t('gi_download'))}</button>
      <button class="btn-sec" ${x.can_mr ? '' : `disabled title="${esc(x.mr_note || '')}"`} onclick="gitWorkspace.ignoreMR()">${esc(mrLabel)}</button>${x.can_mr ? '' : ` <span class="g-mut">${esc(x.mr_note || '')}</span>`}</div>
      <div class="g-mut">${esc(t('gi_mr_default'))}</div><div id="gi-out">${I.preview ? previewHTML(I.preview) : ''}</div>`;
    return h;
  }

  function ignoreMark(el) { if (!I.checked) I.checked = new Set(); if (el.checked) I.checked.add(el.value); else I.checked.delete(el.value); document.querySelectorAll('.gi-p').forEach(b => { if (b.value === el.value) b.checked = el.checked; }); }
  function ignorePatterns() { I.custom = ($('gi-custom') || {}).value || ''; return [...new Set([...(I.checked || [])].concat(lines(I.custom.replace(/,/g, '\n'))))]; }

  function previewHTML(p) {
    if (!p.added.length) return `<div class="hint-box">${esc(t('gi_nothing'))}</div>`;
    return `<h3 style="margin:12px 0 6px">${esc(t('gi_diff'))} · ${esc(tf('gi_added', {n: p.added.length}))}</h3><div class="g-diff">` +
      p.lines.map(l => l.op === '@' ? `<div class="d-hunk">@@ -${l.a} +${l.b} @@</div>` : `<div class="${l.op === '+' ? 'd-add' : l.op === '-' ? 'd-del' : ''}">${esc(l.op + ' ' + l.s)}</div>`).join('') + '</div>';
  }

  async function ignorePreview() {
    const r = await fetch(`/api/git/gitignore/${encodeURIComponent(I.app)}/preview`, json('POST', {patterns: ignorePatterns()}));
    if (!r.ok) { showToast(await apiError(r), 'error'); return null; }
    I.preview = await r.json();
    $('gi-out').innerHTML = previewHTML(I.preview);
    $('gi-out').scrollIntoView({block: 'nearest'});
    return I.preview;
  }

  function ignoreDownload() { fetch(`/api/git/gitignore/${encodeURIComponent(I.app)}/download`, json('POST', {patterns: ignorePatterns()})).then(r => download(r, '.gitignore')); }

  async function ignoreSaveLists() {
    const a = G.state.catalog.apps.find(x => x.name === I.app);
    if (!a) return;
    const body = Object.assign({}, a, {exclude: lines($('gi-exclude').value), protected: lines($('gi-protected').value)});
    const r = await fetch('/api/git/catalog/apps/' + encodeURIComponent(I.app), json('PUT', body));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    G.state = await r.json(); showToast(t('git_saved'), 'success'); ignoreLoad(I.app);
  }

  async function ignoreMR() {
    const x = I.info, p = await ignorePreview();
    if (!p || !p.added.length) return;
    const gh = x.source_kind === 'github';
    modal(`<h3>${esc(gh ? t('gi_mr_gh').replace('…', '') : tf('gi_mr_title', {repo: x.repo}))}</h3>
      <div>${esc(tf('gi_mr_q', {branch: 'wrm/gitignore-' + I.app.toLowerCase() + '-…', file: x.file, base: x.branch}))}</div>
      <div class="hint-box warn" style="margin:8px 0"><b class="g-mono">${esc(x.repo)}</b></div>
      <div class="g-mono g-mut">${p.added.map(esc).join('<br>')}</div>
      <div class="g-conf"><label>${esc(tf('gi_mr_type', {name: x.project}))}</label><input id="gi-conf" autocomplete="off"></div>
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gi-go" disabled>${esc(t('gi_mr_go'))}</button></div>`);
    $('gi-conf').oninput = () => { $('gi-go').disabled = $('gi-conf').value.trim() !== x.project; };
    $('gi-go').onclick = async () => {
      $('gi-go').disabled = true;
      const r = await fetch(`/api/git/gitignore/${encodeURIComponent(I.app)}/mr`, json('POST', {patterns: ignorePatterns(), confirm: $('gi-conf').value.trim()}));
      if (!r.ok) { $('gi-go').disabled = false; showToast(await apiError(r), 'error', 7000); return; }
      const res = await r.json();
      modal(`<h3>✅ ${esc(tf('gi_mr_done', {url: ''}))}</h3><div><a href="${esc(res.url)}" target="_blank" rel="noopener noreferrer" class="g-mono">${esc(res.url)}</a></div><div class="g-mut g-mono">${esc(res.branch)}</div>
        <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);
    };
  }

  // ── CI: webhooks, artifact feeds, deploy method ──
  function hookURL(id) { return location.origin + '/api/hooks/git/' + id; }
  function webhook(id, secret) {
    const x = G.state.sources.find(s => s.id === id);
    if (!x) return;
    modal(`<h3>${esc(tf('git_wh_title', {name: x.name}))}</h3><div class="g-mut">${esc(t('git_wh_h'))}</div>
      ${x.hook_id ? `<div class="g-card"><label>${esc(t('git_wh_url'))}</label><input readonly value="${esc(hookURL(x.hook_id))}" onclick="this.select()" style="width:100%">
        ${secret ? `<label style="margin-top:8px">${esc(t('git_wh_secret'))}</label><input readonly value="${esc(secret)}" onclick="this.select()" style="width:100%">` : ''}
        <div class="g-mut" style="margin-top:6px">${esc(t('git_wh_last'))}: ${x.hook_at ? esc(fmtTime(x.hook_at)) : esc(t('git_never'))}</div></div>
        <ul class="g-mut"><li>${esc(t('git_wh_gitlab'))}</li><li>${esc(t('git_wh_github'))}</li><li>${esc(t('git_wh_generic'))}</li></ul>` : `<div class="hint-box">${esc(t('git_wh_off'))}</div>`}
      <div class="g-btns">${x.hook_id ? `<button class="btn-danger" onclick="gitWorkspace.hookSet(${id}, false)">${esc(t('git_wh_disable'))}</button><button class="btn-sec" onclick="gitWorkspace.hookSet(${id}, true)">${esc(t('git_wh_renew'))}</button>`
        : `<button onclick="gitWorkspace.hookSet(${id}, true)">${esc(t('git_wh_enable'))}</button>`}<button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);
  }
  async function hookSet(id, on) {
    const r = await fetch(`/api/git/sources/${id}/hook`, {method: on ? 'POST' : 'DELETE'});
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    const j = await r.json();
    G.state = on ? j.state : j; render();
    webhook(id, on ? j.secret : '');
  }

  function feedsHTML() {
    const feeds = G.state.feeds || [];
    return `<div class="g-card"><h3>${esc(t('git_feeds'))}</h3><div class="g-mut" style="margin-bottom:6px">${esc(t('git_feeds_h'))}</div>
      ${feeds.length ? `<table>${feeds.map(f => `<tr><td><b>${esc(f.name)}</b>${f.interval_minutes ? ` <span class="g-mut">⏱ ${f.interval_minutes} min</span>` : ''}<div class="g-mono g-mut">${esc(f.url)}</div>
        <div class="g-mut">${esc(t('git_feed_last'))}: ${f.last_ok_at ? esc(fmtTime(f.last_ok_at)) : esc(t('git_never'))}</div>${f.last_error ? `<div class="g-st s-error">${esc(f.last_error)}</div>` : ''}</td>
        <td style="white-space:nowrap"><button class="btn-sec btn-sm" onclick="gitWorkspace.feedFetch(${f.id})">${esc(t('git_feed_fetch'))}</button> <button class="btn-sec btn-sm" onclick="gitWorkspace.editFeed(${f.id})">${esc(t('git_edit'))}</button> <button class="btn-danger btn-sm" onclick="gitWorkspace.delFeed(${f.id})">✕</button></td></tr>`).join('')}</table>` : ''}
      <div class="g-bar" style="margin-top:8px"><button class="btn-sec" onclick="gitWorkspace.editFeed(0)">＋ ${esc(t('git_add_feed'))}</button></div></div>`;
  }
  function editFeed(id) {
    const f = (G.state.feeds || []).find(x => x.id === id) || {name: '', url: '', header_name: 'PRIVATE-TOKEN', interval_minutes: 0};
    modal(`<h3>${esc(id ? f.name : t('git_add_feed'))}</h3><div class="g-grid">
      <div><label>${esc(t('git_f_name'))}</label><input id="gf-name" value="${esc(f.name)}"></div>
      <div style="grid-column:1/-1"><label>${esc(t('git_feed_url'))}</label><input id="gf-url" value="${esc(f.url)}" placeholder="https://git.example.com/api/v4/projects/demo%2Fdemo-api/jobs/artifacts/main/raw/bundle.tar.gz?job=bundle"></div>
      <div><label>${esc(t('git_feed_header'))}</label><input id="gf-hname" value="${esc(f.header_name)}" placeholder="PRIVATE-TOKEN"></div>
      <div><label>${esc(t('git_feed_value'))}</label><input id="gf-hval" type="password" autocomplete="new-password" placeholder="${f.has_header ? esc(t('git_token_keep')) : ''}"></div>
      <div><label>${esc(t('git_feed_interval'))}</label><input id="gf-int" type="number" min="0" max="10080" value="${f.interval_minutes || 0}"></div></div>
      <div class="g-mut" style="margin-top:6px">${esc(t('git_feed_value_h'))}</div>
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gf-save">${esc(t('git_save'))}</button></div>`);
    $('gf-save').onclick = async () => {
      const body = {name: $('gf-name').value.trim(), url: $('gf-url').value.trim(), header_name: $('gf-hname').value.trim(), interval_minutes: Number($('gf-int').value) || 0};
      if ($('gf-hval').value || !id) body.header_value = $('gf-hval').value;
      const r = await fetch(id ? '/api/git/feeds/' + id : '/api/git/feeds', json(id ? 'PUT' : 'POST', body));
      if (!r.ok) { showToast(await apiError(r), 'error'); return; }
      G.state = await r.json(); closeModal(); render();
    };
  }
  async function feedFetch(id) {
    showToast('⏳ ' + t('git_feed_fetch'), 'info', 1500);
    const r = await fetch(`/api/git/feeds/${id}/fetch`, {method: 'POST'});
    if (!r.ok) { showToast(await apiError(r), 'error', 7000); load().then(ok => ok && render()); return; }
    const j = await r.json();
    G.state = j.state; render();
    showToast(t(j.status === 'imported' ? 'git_feed_imported' : 'git_feed_unchanged'), 'success');
  }
  async function delFeed(id) {
    if (!(await uiConfirm(t('git_delete') + '?', {danger: true, okText: t('git_delete')}))) return;
    const r = await fetch('/api/git/feeds/' + id, {method: 'DELETE'});
    if (r.ok) { G.state = await r.json(); render(); }
  }

  function pipelineForm(name) {
    if (!can('update')) return '';
    const p = (G.state.pipelines || []).find(x => x.app === name) || {kind: ''};
    const f = (k, label, v, ph, type) => `<div class="gp-f gp-${k}"><label>${esc(t(label))}</label><input id="gp-${k}" value="${esc(v || '')}" placeholder="${esc(ph || '')}" ${type ? `type="${type}" autocomplete="new-password"` : ''}></div>`;
    return `<div class="g-card"><h3>${esc(t('git_deploy_method'))}</h3><select id="gp-kind" onchange="gitWorkspace.pipelineKind()">
      <option value="">${esc(t('git_dm_files'))}</option><option value="jenkins" ${p.kind === 'jenkins' ? 'selected' : ''}>${esc(t('git_dm_jenkins'))}</option><option value="gitlab" ${p.kind === 'gitlab' ? 'selected' : ''}>${esc(t('git_dm_gitlab'))}</option></select>
      <div class="g-mut" style="margin:6px 0">${esc(t('git_dm_h'))}</div><div class="g-grid" id="gp-fields" data-had="${p.kind ? 1 : 0}">
      ${f('url', p.kind === 'gitlab' ? 'git_dm_gl_url' : 'git_dm_job', p.url, 'https://ci.example.com/job/deploy-demo')}${f('username', 'git_dm_user', p.username, 'ci-bot')}
      ${f('project', 'git_dm_project', p.project, 'group/project')}${f('ref', 'git_dm_ref', p.ref, 'main')}
      ${f('token', p.kind === 'gitlab' ? 'git_dm_trigger' : 'git_dm_token', '', p.has_token ? t('git_dm_keep') : '', 'password')}</div></div>`;
  }
  function pipelineKind() {
    const k = $('gp-kind').value;
    document.querySelectorAll('.gp-f').forEach(el => { el.style.display = !k ? 'none' : (el.classList.contains('gp-username') ? k === 'jenkins' : el.classList.contains('gp-project') || el.classList.contains('gp-ref') ? k === 'gitlab' : true) ? '' : 'none'; });
    if (k) { document.querySelector('.gp-url label').textContent = t(k === 'gitlab' ? 'git_dm_gl_url' : 'git_dm_job'); document.querySelector('.gp-token label').textContent = t(k === 'gitlab' ? 'git_dm_trigger' : 'git_dm_token'); }
  }
  async function savePipeline(name) {
    if (!$('gp-kind')) return true;
    const k = $('gp-kind').value, had = $('gp-fields').dataset.had === '1';
    if (!k) {
      if (!had) return true;
      const r = await fetch('/api/git/pipelines/' + encodeURIComponent(name), {method: 'DELETE'});
      if (r.ok) G.state = await r.json();
      return r.ok;
    }
    const body = {kind: k, url: $('gp-url').value.trim(), username: $('gp-username').value.trim(), project: $('gp-project').value.trim(), ref: $('gp-ref').value.trim()};
    if ($('gp-token').value || !had) body.token = $('gp-token').value;
    const r = await fetch('/api/git/pipelines/' + encodeURIComponent(name), json('PUT', body));
    if (!r.ok) { showToast(await apiError(r), 'error'); return false; }
    G.state = await r.json();
    return true;
  }

  // ── environments: overview, editor, plan and deploy, history and rollback, doctor ──
  const E = {cells: {}, ov: null, plan: null, excl: new Set(), showAll: false, opts: {}};
  const envKey = (app, env) => app + '\u0000' + env;
  const ENV_ACT = ['new', 'changed', 'eol', 'removed'];
  const envBadge = s => `<span class="g-st s-e${esc(s)}">${esc(t('genv_fs_' + s))}</span>`;
  const envEnc = (app, env) => `app=${encodeURIComponent(app)}&env=${encodeURIComponent(env)}`;
  const errModal = async (title, r) => modal(`<h3>${esc(title)}</h3><div class="hint-box bad">${esc(await apiError(r))}</div><div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);

  // servers are shown by their hostname; the connection name stays the key for filters and requests
  const hostMap = {};
  function hn(name, host) {
    if (host) return host;
    return hostMap[name] || ((G.state || {}).hosts || {})[name] || name;
  }
  const learnHosts = h => { if (h) Object.assign(hostMap, h); };

  async function envLoad() {
    const r = await fetch('/api/git/env/overview' + (G.mode === 'simple' ? '?auto=1' : ''));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    E.ov = await r.json();
    learnHosts(E.ov.hosts);
    if (G.mode === 'simple') { if ($('git-body')) paintSimple(); return; }
    if (G.tab === 'envs' && $('git-body')) { $('git-body').innerHTML = envsHTML(); envCells(); }
  }
  function envRefresh() { E.cells = {}; E.ov = null; if ($('git-body')) $('git-body').innerHTML = envsHTML(); envLoad(); }
  function envOf(app, env) { const s = ((E.ov || {}).services || []).find(x => x.app === app); return (s && s.envs.find(x => x.name === env)) || null; }

  const EF={q:'',app:'',env:'',server:'',dir:''};
  function envFilter(k,v){EF[k]=v;const body=$('git-body'),focus=document.activeElement?.id,pos=document.activeElement?.selectionStart;body.innerHTML=envsHTML();envCells();if(focus&&$(focus)){$(focus).focus();if(pos!=null)$(focus).setSelectionRange(pos,pos)}}
  function envsHTML() {
    const ov=E.ov;if(!ov)return `<div class="hint-box">⏳</div>`;
    const servers=[...new Set(ov.services.flatMap(s=>s.envs.flatMap(e=>e.destinations.map(d=>d.server))))].sort();
    const choose=(k,label,values)=>`<select aria-label="${esc(t(label))}" onchange="gitWorkspace.envFilter('${k}',this.value)">${opt('',t(label),EF[k])}${values.map(v=>opt(v,k==='server'?hn(v)+' · '+v:v,EF[k])).join('')}</select>`;
    const services=ov.services.filter(s=>!EF.app||s.app===EF.app).map(s=>({...s,envs:s.envs.filter(e=>(!EF.env||e.name===EF.env)&&(!EF.server||e.destinations.some(d=>d.server===EF.server))&&(!EF.dir||e.destinations.some(d=>inDir(d.path,EF.dir)))&&JSON.stringify([s.app,e.name,e.destinations.map(d=>[d.server,hn(d.server),d.path])]).toLowerCase().includes(EF.q.toLowerCase())).map(e=>({...e,destinations:e.destinations.filter(d=>(!EF.server||d.server===EF.server)&&inDir(d.path,EF.dir))}))})).filter(s=>s.envs.length);
    const envs=ov.envs.filter(n=>services.some(s=>s.envs.some(e=>e.name===n)));
    let html=`<div class="g-control-stack"><div class="g-bar"><input id="genv-q" type="search" placeholder="${esc(t('git_search_srv'))}" value="${esc(EF.q)}" oninput="gitWorkspace.envFilter('q',this.value)">${choose('app','git_all_services',ov.services.map(s=>s.app))}${choose('env','git_all_envs',ov.envs)}${choose('server','git_all_servers',servers)}${dirSelect("gitWorkspace.envFilter('dir',this.value)",ov.services.flatMap(s=>s.envs.flatMap(e=>e.destinations.map(d=>d.path))),EF.dir,true)}<button class="btn-sec btn-sm" onclick="gitWorkspace.envReset()">${esc(t('hf_reset'))}</button><button class="btn-sec btn-sm" onclick="gitWorkspace.envRefresh()">↻ ${esc(t('git_refresh'))}</button></div><div class="g-mut">${esc(t('genv_lazy_h'))} · ${services.length} / ${ov.services.length}</div></div>`;
    if(!services.length)html+=`<div class="hint-box">${esc(t('genv_none'))}</div>`;
    else html+=`<div class="g-card g-scroll"><table class="g-matrix"><tr><th>${esc(t('git_service'))}</th>${envs.map(n=>`<th>${esc(n)}</th>`).join('')}</tr>`+services.map(s=>`<tr><td><b>${esc(s.app)}</b><div><button class="g-cellbtn" onclick="gitWorkspace.envEdit('${esc(s.app)}')">✎ ${esc(t('genv_edit'))}</button></div></td>${envs.map(n=>{const e=s.envs.find(x=>x.name===n);return e?`<td class="g-cell g-envcell" data-app="${esc(s.app)}" data-env="${esc(n)}">${envCellHTML(s.app,e)}</td>`:'<td>—</td>'}).join('')}</tr>`).join('')+'</table></div>';
    html+=`<div class="g-card"><h3>${esc(t('genv_services'))}</h3><div class="g-actions">${G.state.catalog.apps.filter(a=>(!EF.app||a.name===EF.app)&&a.name.toLowerCase().includes(EF.q.toLowerCase())).map(a=>`<button class="btn-sec btn-sm" onclick="gitWorkspace.envEdit('${esc(a.name)}')">✎ ${esc(a.name)} (${(a.environments||[]).length})</button>`).join('')}</div></div>`;
    return html;
  }

  function envCellHTML(app, e) {
    const c = E.cells[envKey(app, e.name)];
    const a = `'${esc(app)}','${esc(e.name)}'`;
    const btns = `<div class="g-actions" style="margin-top:4px">${can('update') ? `<button class="g-cellbtn" onclick="gitWorkspace.envPlan(${a})">▶ ${esc(t('genv_deploy'))}</button>` : ''}
      <button class="g-cellbtn" onclick="gitWorkspace.envHistory(${a})">🕘 ${esc(t('genv_history'))}</button><button class="g-cellbtn" onclick="gitWorkspace.envDoctor(${a})">🩺 ${esc(t('genv_doctor'))}</button></div>`;
    if (!c || c.loading) return `<div class="g-mut"><span class="g-spin"></span> ${esc(tf('genv_n_dests', {n: e.destinations.length}))}</div>${btns}`;
    if (c.error) return `<div class="hint-box bad">${esc(c.error)}</div>${btns}`;
    const d = c.data;
    return (d.drift ? `<div><span class="g-st s-review">⚠ ${esc(t('genv_drift'))}</span></div>` : '') + d.dests.filter(x=>G.mode==='simple'||((!EF.server||x.server===EF.server)&&inDir(x.path,EF.dir))).map(x => {
      const cur = x.current;
      const ver = cur ? cur.version : x.version_md;
      const commit = cur ? String(cur.commit || '').slice(0, 10) : x.commit_md;
      const behind = x.behind > 0 ? ` <span class="g-st s-update">${esc(tf('git_behind', {n: x.behind}))}</span>` : x.behind === 0 ? ` <span class="g-st s-ok">${esc(t('git_st_ok'))}</span>` : '';
      return `<div class="g-inst"><b>${esc(hn(x.server))}</b> <span class="g-mono g-mut">${esc(x.path)}</span>
        ${x.error ? `<div><span class="g-st s-error">${esc(x.error)}</span></div>` : !x.exists ? `<div class="g-mut">${esc(t('genv_not_deployed'))}</div>`
          : `<div><span class="g-mono">${esc(ver || '—')}</span> <span class="g-mut g-mono">${esc(commit || '')}</span>${behind}</div><div class="g-ago">${cur ? esc((cur.by || '') + ' · ' + fmtTime(cur.at)) : esc(t('genv_no_state'))}</div>`}
        ${x.lock ? `<div class="g-q">🔒 ${esc(tf('genv_locked_by', {user: x.lock.user || '?', from: x.lock.from || '?'}))}${x.lock.stale ? ' · ' + esc(t('genv_stale')) : ''}</div>` : ''}</div>`;
    }).join('') + btns;
  }

  // one request per cell, three at a time
  function envCells() {
    const todo = [...document.querySelectorAll('#git-body .g-envcell')].filter(td => !E.cells[envKey(td.dataset.app, td.dataset.env)]);
    let i = 0;
    const next = async () => {
      if (i >= todo.length) return;
      const td = todo[i++], app = td.dataset.app, env = td.dataset.env;
      E.cells[envKey(app,env)]={loading:true};
      try {
        const r = await fetch('/api/git/env/cell?' + envEnc(app, env));
        E.cells[envKey(app, env)] = r.ok ? {data: await r.json()} : {error: await apiError(r)};
      } catch (err) { E.cells[envKey(app, env)] = {error: String(err)}; }
      const cell = document.querySelector(`#git-body .g-envcell[data-app="${CSS.escape(app)}"][data-env="${CSS.escape(env)}"]`), e = envOf(app, env);
      if (cell && e) cell.innerHTML = envCellHTML(app, e);
      return next();
    };
    for (let n = 0; n < 3; n++) next();
  }

  // ── environment editor ──
  function envEdit(app) {
    const a = G.state.catalog.apps.find(x => x.name === app);
    if (!a) return;
    E.edit = JSON.parse(JSON.stringify(a.environments || []));
    paintEnvEdit(app);
  }
  function envFormHTML(e, i) {
    const ta = (cls, label, v, ph) => `<div><label>${esc(t(label))}</label><textarea class="${cls}" placeholder="${esc(ph)}">${esc(v)}</textarea></div>`;
    return `<div class="g-item ge-env" data-i="${i}"><div class="g-grid">
      <div><label>${esc(t('genv_name'))}</label><input class="ge-name" value="${esc(e.name || '')}" placeholder="test"></div>
      <div><label>${esc(t('genv_keep'))}</label><input class="ge-keep" type="number" min="1" max="50" value="${esc(e.keep_backups || 5)}"></div>
      <div><label>${esc(t('genv_state_dir'))}</label><input class="ge-state" value="${esc(e.state_dir || '')}" placeholder="${esc(t('genv_state_dir_ph'))}"></div></div>
      <div class="g-grid" style="margin-top:8px"><div style="grid-column:1/-1"><label>${esc(t('genv_dests'))}</label><textarea class="ge-dests g-mono" rows="3" placeholder="app-01:/opt/demo-api">${esc((e.destinations || []).map(d => d.server + ':' + d.path).join('\n'))}</textarea><div class="g-mut">${esc(t('genv_dests_h'))}</div></div>
      ${ta('ge-refs', 'genv_allowed', (e.allowed_refs || []).join('\n'), 'refs/heads/main\nrefs/tags/v*')}${ta('ge-ignore', 'genv_ignore', (e.ignore || []).join('\n'), t('git_lines_h'))}</div>
      <div class="g-grid" style="margin-top:8px"><div><label>${esc(t('genv_post'))}</label><input class="ge-post" value="${esc(e.post_deploy || '')}" placeholder="./bin/migrate --yes"><div class="g-mut">${esc(t('genv_post_h'))}</div></div></div>
      <label class="g-opt"><input type="checkbox" class="ge-confirm" ${e.confirm ? 'checked' : ''}><span>${esc(t('genv_confirm'))}</span></label>
      <div class="g-btns" style="margin-top:4px"><button class="btn-danger btn-sm" onclick="gitWorkspace.envEditDel('${esc(e._app || '')}', ${i})">✕ ${esc(t('genv_remove'))}</button></div></div>`;
  }
  function paintEnvEdit(app) {
    E.edit.forEach(e => { e._app = app; });
    modal(`<h3>${esc(tf('genv_edit_title', {app}))}</h3><div class="g-mut">${esc(t('genv_edit_h'))}</div>
      <div id="ge-list">${E.edit.map(envFormHTML).join('') || `<div class="hint-box">${esc(t('genv_none_app'))}</div>`}</div>
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.envEditAdd('${esc(app)}')">＋ ${esc(t('genv_add'))}</button><span style="flex:1"></span>
      <button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="ge-save">${esc(t('git_save'))}</button></div>`);
    $('ge-save').onclick = () => envEditSave(app);
  }
  function readEnvForms() {
    return [...document.querySelectorAll('#ge-list .ge-env')].map(el => {
      const v = c => el.querySelector(c).value.trim();
      return {name: v('.ge-name'), keep_backups: Number(v('.ge-keep')) || 0, state_dir: v('.ge-state'), post_deploy: v('.ge-post'), confirm: el.querySelector('.ge-confirm').checked,
        allowed_refs: lines(v('.ge-refs')), ignore: v('.ge-ignore').split('\n').map(x => x.trim()).filter(Boolean),
        destinations: v('.ge-dests').split('\n').map(x => x.trim()).filter(Boolean).map(x => { const k = x.indexOf(':/'); return k > 0 ? {server: x.slice(0, k).trim(), path: x.slice(k + 1).trim()} : {server: x, path: ''}; })};
    });
  }
  function envEditAdd(app) { E.edit = readEnvForms(); E.edit.push({name: '', destinations: [], keep_backups: 5}); paintEnvEdit(app); }
  function envEditDel(app, i) { E.edit = readEnvForms(); E.edit.splice(i, 1); paintEnvEdit(app); }
  async function envEditSave(app) {
    const a = G.state.catalog.apps.find(x => x.name === app);
    const r = await fetch('/api/git/catalog/apps/' + encodeURIComponent(app), json('PUT', Object.assign({}, a, {environments: readEnvForms()})));
    if (!r.ok) { showToast(await apiError(r), 'error', 7000); return; }
    G.state = await r.json();
    closeModal(); showToast(t('git_saved'), 'success');
    if (G.tab === 'envs') envRefresh(); else render();
  }

  // ── plan, dry run and deploy ──
  async function envPlan(app, env, ref) {
    E.ctx = {app, env};
    modal(`<h3>${esc(tf('genv_plan_title', {app, env}))}</h3><div class="hint-box">⏳ ${esc(t('genv_comparing'))}</div>`);
    const r = await fetch('/api/git/env/plan', json('POST', {app, env, ref: ref || ''}));
    if (!r.ok) { await errModal(tf('genv_plan_title', {app, env}), r); return; }
    E.plan = await r.json();
    E.excl = new Set(Object.keys(E.plan.excluded || {}));
    E.opts = {del: false, post: false, restart: false, check: ((G.state.settings || {}).check_commands || {})[app] || ''};
    paintPlan();
  }
  function envRecompare() { const ref = $('ge-ref') ? $('ge-ref').value.trim() : (E.plan || {}).ref; envPlan(E.ctx.app, E.ctx.env, ref); }
  function envSaveOpts() {
    if (!$('ge-check')) return;
    E.opts = {del: !!($('ge-del') || {}).checked, post: !!($('ge-post') || {}).checked, restart: !!($('ge-restart') || {}).checked, check: $('ge-check').value.trim()};
  }
  function paintPlan() {
    const p = E.plan, o = E.opts, tg = p.target || {};
    const blocked = (p.errors || []).length > 0 || p.dests.some(d => (d.errors || []).length);
    const removed = Math.max(0, ...p.dests.map(d => (d.counts || {}).removed || 0));
    const destHTML = d => {
      const c = d.counts || {};
      const files = d.files.filter(f => E.showAll || ENV_ACT.includes(f.state) || E.excl.has(f.path));
      const rows = files.map(f => {
        const act = ENV_ACT.includes(f.state), x = (p.excluded || {})[f.path];
        const xb = scope => `<button class="g-cellbtn ge-x" data-p="${esc(f.path)}" data-scope="${scope}" title="${esc(t(scope === 'service' ? 'genv_excl_svc_h' : 'genv_excl_env_h'))}" onclick="gitWorkspace.envExclude(this)">⊘ ${esc(t(scope === 'service' ? 'genv_excl_svc' : 'genv_excl_env'))}</button>`;
        return `<tr><td>${act ? `<input type="checkbox" class="ge-f" data-p="${esc(f.path)}" ${E.excl.has(f.path) ? '' : 'checked'} title="${esc(t('genv_include_h'))}" onchange="gitWorkspace.envToggle(this)">` : ''}</td>
          <td class="g-mono">${esc(f.path)}</td><td>${envBadge(f.state)}${f.local ? ` <span class="g-mut">${esc(t('git_local'))}</span>` : ''}${f.behind ? ` <span class="g-mut">${esc(tf('git_behind', {n: f.behind}))} ${esc(f.tag || '')}</span>` : ''}${f.link ? ` <span class="g-mut">⤳ ${esc(t('genv_symlink'))}</span>` : ''}${f.pattern ? ` <span class="g-mut g-mono">${esc(f.pattern)}</span>` : ''}${x ? `<div class="g-mut">${esc(tf('genv_excluded_by', {by: x.by, at: fmtTime(x.at)}))}</div>` : ''}</td>
          <td style="white-space:nowrap">${act || f.state === 'extra' ? xb('service') + ' ' + xb('env') : ''}</td></tr>`;
      }).join('');
      const cur = d.current;
      return `<div class="g-item"><h4>${d.index + 1}. ${esc(hn(d.server))} · <span class="g-mono">${esc(d.path)}</span></h4>
        <div class="g-mut">${cur ? esc(tf('genv_current', {v: cur.version, id: cur.deploy_id, by: cur.by, at: fmtTime(cur.at)})) : esc(t(d.exists ? 'genv_no_state' : 'genv_not_deployed'))}${d.from_version ? ' · VERSION.md ' + esc(d.from_version) : ''}</div>
        <div class="g-actions" style="margin:5px 0">${['new', 'changed', 'eol', 'removed', 'extra', 'protected', 'ignored', 'same'].filter(k => c[k]).map(k => `${envBadge(k)} <b>${c[k]}</b>`).join(' ')}</div>
        ${(d.errors || []).map(x => `<div class="hint-box bad">${esc(x)}</div>`).join('')}${(d.warnings || []).map(x => `<div class="hint-box warn">${esc(x)}</div>`).join('')}
        ${(d.conflicts || []).map(x => `<div class="hint-box warn">${esc(tf(x.kind === 'dir_to_file' ? 'genv_dir_to_file' : 'genv_file_to_dir', {path: x.path}))} <span class="g-mono">${x.server.map(esc).join(', ')}</span></div>`).join('')}
        ${d.lock && d.lock.stale ? `<div class="g-btns" style="justify-content:flex-start;margin-top:4px"><button class="btn-danger btn-sm" data-dest="${d.index}" data-id="${esc(d.lock.id)}" data-user="${esc(d.lock.user)}" data-from="${esc(d.lock.from)}" onclick="gitWorkspace.envUnlock(this)">🔓 ${esc(t('genv_unlock'))}</button></div>` : ''}
        ${files.length ? `<div class="g-files"><table>${rows}</table></div>` : (d.errors || []).length ? '' : `<div class="g-mut">${esc(t('git_files_none'))}</div>`}</div>`;
    };
    modal(`<h3>${esc(tf('genv_plan_title', {app: p.app, env: p.env}))}</h3>
      <div class="g-bar" style="margin:0 0 6px"><label class="g-mut">${esc(t('genv_ref'))}</label><input id="ge-ref" value="${esc(p.ref || '')}" placeholder="${esc(t('genv_ref_ph'))}" style="flex:1 1 220px"><button class="btn-sec btn-sm" onclick="gitWorkspace.envRecompare()">↻ ${esc(t('genv_compare'))}</button></div>
      <div class="g-mut">${esc(t('git_target'))}: <span class="g-mono">${esc(tg.version || '—')}</span> · <span class="g-mono">${esc(p.ref_name || '')}</span> · <span class="g-mono">${esc(tg.commit || '')}</span> ${esc(tg.commit_date || '')}</div>
      ${(p.errors || []).map(x => `<div class="hint-box bad">${esc(x)}</div>`).join('')}${p.used ? `<div class="hint-box warn">${esc(t('genv_plan_used'))}</div>` : ''}
      <label class="g-opt"><input type="checkbox" ${E.showAll ? 'checked' : ''} onchange="gitWorkspace.envShowAll(this.checked)"><span>${esc(t('git_show_all'))}</span></label>
      ${p.dests.map(destHTML).join('')}
      <div class="g-card"><h3>${esc(t('genv_options'))}</h3>
        <div class="g-actions"><span class="g-mut" id="ge-nexcl">${esc(tf('genv_n_excluded', {n: E.excl.size}))}</span>${Object.keys(p.excluded || {}).length ? `<button class="btn-sec btn-sm" onclick="gitWorkspace.envClearExcl()">${esc(t('genv_clear_excl'))}</button>` : ''}</div>
        <label class="g-opt"><input type="checkbox" id="ge-del" ${o.del ? 'checked' : ''} ${removed ? '' : 'disabled'}><span>${esc(tf('genv_delete_removed', {n: removed}))}</span></label>
        ${p.post_deploy ? `<label class="g-opt"><input type="checkbox" id="ge-post" ${o.post ? 'checked' : ''}><span>${esc(t('genv_run_post'))} <code class="g-mono">${esc(p.post_deploy)}</code></span></label>` : `<div class="g-mut">${esc(t('genv_no_post'))}</div>`}
        <label class="g-opt"><input type="checkbox" id="ge-restart" ${o.restart ? 'checked' : ''} ${can('restart') ? '' : 'disabled'}><span>${esc(t('genv_restart'))}</span></label>
        <div style="margin-top:6px"><label class="g-mut">${esc(tf('git_check_cmd', {app: p.app}))}</label><input id="ge-check" value="${esc(o.check || '')}" placeholder="./run.py --selftest" style="width:100%"></div></div>
      ${blocked ? `<div class="hint-box bad">${esc(t('genv_blocked'))}</div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button>
        <button class="btn-sec" onclick="gitWorkspace.envGo(true)" ${blocked || p.used ? 'disabled' : ''}>🧪 ${esc(t('genv_dry_run'))}</button>
        <button class="btn-danger" onclick="gitWorkspace.envGo(false)" ${blocked || p.used ? 'disabled' : ''}>${esc(t('genv_deploy_go'))}</button></div>`);
  }
  function envShowAll(v) { envSaveOpts(); E.showAll = v; paintPlan(); }
  function envToggle(el) {
    const p = el.dataset.p;
    if (el.checked) E.excl.delete(p); else E.excl.add(p);
    document.querySelectorAll('.ge-f').forEach(x => { if (x.dataset.p === p) x.checked = el.checked; });
    $('ge-nexcl').textContent = tf('genv_n_excluded', {n: E.excl.size});
  }
  async function envExclude(btn) {
    const path = btn.dataset.p, scope = btn.dataset.scope, p = E.plan;
    if (!(await uiConfirm(tf('genv_excl_q', {path, scope: t(scope === 'service' ? 'genv_excl_svc' : 'genv_excl_env')}), {okText: '⊘'}))) return;
    const r = await fetch('/api/git/env/exclude', json('POST', {app: p.app, env: p.env, path, scope}));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    const res = await r.json();
    showToast(tf(res.added ? 'genv_excl_added' : 'genv_excl_covered', {p: res.pattern}), 'success', 5000);
    load(); envPlan(p.app, p.env, p.ref);
  }
  async function envClearExcl() {
    const p = E.plan;
    const r = await fetch('/api/git/env/exclusions?' + envEnc(p.app, p.env), {method: 'DELETE'});
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    envSaveOpts(); p.excluded = {}; E.excl.clear(); paintPlan();
  }
  async function envUnlock(btn) {
    const p = E.plan;
    if (!(await uiConfirm(tf('genv_unlock_q', {user: btn.dataset.user || '?', from: btn.dataset.from || '?', id: btn.dataset.id || '?'}), {danger: true, okText: t('genv_unlock')}))) return;
    const r = await fetch('/api/git/env/unlock', json('POST', {app: p.app, env: p.env, dest: Number(btn.dataset.dest), lock_id: btn.dataset.id}));
    if (!r.ok) { showToast(await apiError(r), 'error', 7000); return; }
    showToast(t('genv_unlocked'), 'success'); envRecompare();
  }
  function envGo(dry) {
    envSaveOpts();
    const p = E.plan, o = E.opts;
    const body = {kind: 'update', restart: {mode: o.restart ? 'now' : 'none'}, checks: {[p.app]: o.check},
      env: {app: p.app, env: p.env, plan_id: p.id, exclude: [...E.excl], delete_removed: o.del, post_deploy: o.post, dry_run: dry}};
    if (dry) { envStart(body); return; }
    modal(`<h3>${esc(tf('genv_confirm_title', {app: p.app, v: (p.target || {}).version || '', env: p.env}))}</h3>
      <div>${esc(t('genv_confirm_list'))}</div><ol>${p.dests.map(d => `<li><b>${esc(hn(d.server))}</b> <span class="g-mono">${esc(d.path)}</span> <span class="g-mut">${['new', 'changed', 'eol', 'removed'].filter(k => (d.counts || {})[k]).map(k => t('genv_fs_' + k) + ' ' + d.counts[k]).join(' · ')}</span></li>`).join('')}</ol>
      ${E.excl.size ? `<div class="g-mut">${esc(tf('genv_n_excluded', {n: E.excl.size}))}</div>` : ''}${o.del ? `<div class="hint-box warn">${esc(tf('genv_delete_removed', {n: Math.max(0, ...p.dests.map(d => (d.counts || {}).removed || 0))}))}</div>` : ''}
      ${o.post ? `<div class="hint-box warn">${esc(t('genv_run_post'))} <code class="g-mono">${esc(p.post_deploy)}</code></div>` : ''}${o.restart ? `<div class="hint-box warn">${esc(t('genv_restart'))}</div>` : ''}
      ${p.confirm ? `<div class="g-conf"><label>${esc(tf('genv_type_env', {env: p.env}))}</label><input id="ge-conf" autocomplete="off"></div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.envBack()">${esc(t('genv_back'))}</button><button id="ge-start" class="btn-danger" ${p.confirm ? 'disabled' : ''}>${esc(t('git_start'))}</button></div>`);
    if (p.confirm) $('ge-conf').oninput = () => { $('ge-start').disabled = $('ge-conf').value.trim().toLowerCase() !== p.env.toLowerCase(); };
    $('ge-start').onclick = () => { if (p.confirm) body.env.confirm = $('ge-conf').value.trim(); $('ge-start').disabled = true; envStart(body); };
  }
  async function envStart(body) {
    const r = await fetch('/api/git/runs', json('POST', body));
    if (!r.ok) { showToast(await apiError(r), 'error', 8000); if ($('ge-start')) $('ge-start').disabled = false; return; }
    const v = await r.json();
    if (!body.env.dry_run && body.kind === 'update' && E.plan) E.plan.used = true;
    E.cells = {};
    showRun(v.id);
  }
  function envBack() { if (E.plan) paintPlan(); }

  // ── history and rollback ──
  async function envHistory(app, env) {
    E.ctx = {app, env};
    modal(`<h3>${esc(tf('genv_history_title', {app, env}))}</h3><div class="hint-box">⏳</div>`);
    const r = await fetch('/api/git/env/history?' + envEnc(app, env));
    if (!r.ok) { await errModal(tf('genv_history_title', {app, env}), r); return; }
    const h = E.hist = await r.json();
    const cur = new Set(h.dests.map(d => d.current).filter(Boolean));
    const lists = x => ['new', 'changed', 'deleted', 'excluded', 'restored', 'moved_aside', 'skipped', 'moved'].filter(k => (x[k] || []).length).map(k =>
      `<div style="margin-top:4px"><b>${esc(t('genv_l_' + k))}</b> (${(x.counts || {})[k] != null ? x.counts[k] : x[k].length})<div class="g-mono g-mut">${x[k].map(esc).join('<br>')}</div></div>`).join('');
    const sum = x => { const c = x.counts || {}; return x.action === 'ROLLBACK' ? `↩ ${c.restored || 0} · ⇢ ${c.moved_aside || 0}${c.skipped ? ' · ⚠ ' + c.skipped : ''}` : x.result === 'partial' ? `⚠ ${c.moved || 0}/${c.total || 0}` : `+${c.new || 0} ~${c.changed || 0} −${c.deleted || 0}${c.eol ? ' · CRLF/LF ' + c.eol : ''}`; };
    const res = x => x.action === 'ROLLBACK' ? (x.complete === false ? 'failed' : 'done') : x.result === 'ok' ? 'done' : 'failed';
    modal(`<h3>${esc(tf('genv_history_title', {app, env}))}</h3>
      ${h.dests.filter(d => d.error).map(d => `<div class="hint-box bad">${esc(hn(d.server))}: ${esc(d.error)}</div>`).join('')}
      ${h.entries.length ? `<div class="g-scroll"><table><tr><th>${esc(t('genv_when'))}</th><th>${esc(t('genv_action'))}</th><th>${esc(t('genv_id'))}</th><th>${esc(t('genv_who'))}</th><th>${esc(t('genv_version'))}</th><th>${esc(t('genv_previous'))}</th><th>${esc(t('genv_servers'))}</th><th>${esc(t('genv_files'))}</th><th></th></tr>
        ${h.entries.map(x => `<tr><td class="g-mut">${esc(fmtTime(x.at))}</td><td><span class="g-st s-${res(x)}">${esc(x.action)}</span>${x.result && x.result !== 'ok' ? `<div class="g-mut">${esc(x.result)}</div>` : ''}${x.post_deploy ? `<div class="g-mut">post_deploy: ${esc(x.post_deploy)}</div>` : ''}</td>
          <td class="g-mono">${esc(x.id || '')}${x.rolled_back ? `<div class="g-mut">↩ ${esc(x.rolled_back)}</div>` : ''}</td><td>${esc(x.by || '')}</td>
          <td class="g-mono">${esc(x.version || '')}<div class="g-mut">${esc(String(x.commit || '').slice(0, 10))} ${esc(x.branch || '')}</div></td>
          <td class="g-mono g-mut">${x.previous ? esc(x.previous.version + ' · ' + x.previous.deploy_id) : x.action === 'DEPLOY' ? '—' : ''}</td>
          <td>${esc(x.server || '')}<div class="g-mono g-mut">${esc(x.path || '')}</div></td>
          <td style="min-width:150px"><details><summary class="g-mut">${esc(sum(x))}</summary>${lists(x)}${x.error ? `<div class="g-mut">${esc(x.error)}</div>` : ''}${x.note ? `<div class="g-mut">${esc(x.note)}</div>` : ''}</details></td>
          <td>${x.action === 'DEPLOY' && cur.has(x.id) && can('rollback') ? `<button class="btn-sec btn-sm" data-id="${esc(x.id)}" onclick="gitWorkspace.envRollback(this)">↩ ${esc(t('genv_rollback'))}</button>` : ''}</td></tr>`).join('')}</table></div>`
        : `<div class="hint-box">${esc(t('genv_history_empty'))}</div>`}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);
  }
  function envRollback(btn) {
    const id = btn.dataset.id, h = E.hist, e = envOf(h.app, h.env) || {};
    modal(`<h3>${esc(tf('genv_rb_title', {id}))}</h3><div class="g-mut">${esc(t('genv_rb_h'))}</div>
      <table style="margin-top:8px">${h.dests.map((d, i) => `<tr><td><input type="checkbox" class="ge-rbd" value="${i}" ${d.current === id ? 'checked' : 'disabled'}></td><td>${i + 1}. <b>${esc(hn(d.server))}</b></td><td class="g-mono">${esc(d.path)}</td>
        <td class="g-mut">${d.current === id ? esc(t('genv_rb_latest')) : esc(tf('genv_rb_not_latest', {id: d.current || '—'}))}</td></tr>`).join('')}</table>
      <div class="g-card">${e.post_deploy ? `<label class="g-opt"><input type="checkbox" id="ge-rbpost"><span>${esc(t('genv_run_post'))} <code class="g-mono">${esc(e.post_deploy)}</code></span></label>` : `<div class="g-mut">${esc(t('genv_no_post'))}</div>`}
        <label class="g-opt"><input type="checkbox" id="ge-rbrestart" ${can('restart') ? '' : 'disabled'}><span>${esc(t('genv_restart'))}</span></label></div>
      ${e.confirm ? `<div class="g-conf"><label>${esc(tf('genv_type_env', {env: h.env}))}</label><input id="ge-rbconf" autocomplete="off"></div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.envHistory('${esc(h.app)}','${esc(h.env)}')">${esc(t('git_cancel'))}</button>
        <button class="btn-sec" id="ge-rbdry">🧪 ${esc(t('genv_dry_run'))}</button><button class="btn-danger" id="ge-rbgo" ${e.confirm ? 'disabled' : ''}>${esc(t('genv_rb_go'))}</button></div>`);
    if (e.confirm) $('ge-rbconf').oninput = () => { $('ge-rbgo').disabled = $('ge-rbconf').value.trim().toLowerCase() !== h.env.toLowerCase(); };
    const go = async dry => {
      const dests = [...document.querySelectorAll('.ge-rbd:checked')].map(x => Number(x.value));
      if (!dests.length) { showToast(t('genv_rb_none'), 'warning'); return; }
      if (!dry && !(await uiConfirm(tf('genv_rb_confirm', {id, n: dests.length}), {danger: true, okText: t('genv_rb_go')}))) return;
      const body = {kind: 'rollback', restart: {mode: $('ge-rbrestart').checked ? 'now' : 'none'},
        env: {app: h.app, env: h.env, deploy_id: id, dests, post_deploy: !!($('ge-rbpost') || {}).checked, dry_run: dry, confirm: ($('ge-rbconf') || {}).value || ''}};
      const r = await fetch('/api/git/runs', json('POST', body));
      if (!r.ok) { showToast(await apiError(r), 'error', 8000); return; }
      E.cells = {};
      showRun((await r.json()).id);
    };
    $('ge-rbdry').onclick = () => go(true);
    $('ge-rbgo').onclick = () => go(false);
  }

  // ── access doctor ──
  async function envDoctor(app, env) {
    const title = `🩺 ${tf('genv_doctor_title', {app, env})}`;
    modal(`<h3>${esc(title)}</h3><div class="hint-box">⏳ ${esc(t('genv_doctor_running'))}</div>`);
    const r = await fetch('/api/git/env/doctor', json('POST', {app, env}));
    if (!r.ok) { await errModal(title, r); return; }
    const icon = {ok: '✅', warn: '⚠️', fail: '❌', info: 'ℹ️'};
    modal(`<h3>${esc(title)}</h3>${(await r.json()).dests.map((d, i) => `<div class="g-item"><h4>${i + 1}. ${esc(hn(d.server))} · <span class="g-mono">${esc(d.path)}</span></h4>
      <table>${d.checks.map(c => `<tr><td style="width:24px">${icon[c.status] || '•'}</td><td style="white-space:nowrap"><b>${esc(t('genv_chk_' + c.name))}</b></td><td>${esc(c.msg)}</td></tr>`).join('')}</table></div>`).join('')}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.envDoctor('${esc(app)}','${esc(env)}')">↻</button><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);
  }

  // ── progress and history ──
  let runTimer = null;
  async function showRun(id) {
    clearTimeout(runTimer);
    const r = await fetch('/api/git/runs/' + id);
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    const v = await r.json();
    const open = $('git-modal');
    if (open && open.dataset.run && open.dataset.run !== String(id)) return;
    const icon = s => ({ok: '✅', fail: '❌', skip: '⏭', info: 'ℹ'}[s] || '•');
    const ev = v.env;
    const html = `<h3>${esc(tf('git_progress', {label: v.label}))} · ${esc(kindName(v.kind))}${ev ? ` · ${esc(ev.app)} → ${esc(ev.env)}` : ''}${v.ref ? ' → <span class="g-mono">' + esc(v.ref) + '</span>' : ''} ${runBadge(v.state)}${ev && ev.dry_run ? ` <span class="g-st s-scheduled">🧪 ${esc(t('genv_dry'))}</span>` : ''}</h3>
      ${ev && ev.deploy_id ? `<div class="g-mut">${esc(t('genv_id'))}: <span class="g-mono">${esc(ev.deploy_id)}</span></div>` : ''}
      ${v.message ? `<div class="hint-box ${v.state === 'failed' ? 'bad' : ''}">${esc(v.message)}</div>` : ''}
      ${v.scheduled_at ? `<div class="g-mut">${esc(t('git_scheduled_for'))}: ${esc(fmtTime(v.scheduled_at))}</div>` : v.parent_id ? `<div class="g-mut">${esc(t('git_scheduled_for'))}: ${esc(t('git_waits_update'))}</div>` : ''}
      ${v.items.map(it => `<div class="g-item"><h4>${esc(hn(it.conn_name))} · ${esc(it.app)} · <span class="g-mono">${esc(it.path)}</span> ${runBadge(it.state)}${it.step ? ` <span class="g-mut">⏳ ${esc(t('git_step_' + it.step))}</span>` : ''}</h4>
        ${it.source_path ? `<div class="g-mut">${esc(t('git_w_source'))}: ${esc(it.source_conn_name)} · <span class="g-mono">${esc(it.source_path)}</span></div>` : ''}
        ${it.from_version || it.to_version ? `<div class="g-mut g-mono">${esc(it.from_version || '')} → ${esc(it.to_version || '')}</div>` : ''}
        ${it.via === 'ci' ? `<div>🔧 ${esc(t('git_step_ci'))} (${esc(it.ci_kind || '')}): <b>${esc(it.ci_status || '…')}</b>${it.ci_url ? ` · <a href="${esc(it.ci_url)}" target="_blank" rel="noopener noreferrer">${esc(t('git_ci_link'))}</a>` : ''}</div>` : ''}
        ${it.error ? `<div class="hint-box bad">${esc(it.error)}</div>` : ''}${it.note && it.note !== it.error ? `<div class="hint-box warn">${esc(it.note)}</div>` : ''}
        ${it.partial ? `<div class="hint-box warn">${esc(tf('genv_partial', {n: it.moved_count || 0}))}${(it.moved || []).length ? `<details><summary class="g-mut">${esc(t('genv_l_moved'))}</summary><div class="g-mono g-mut">${it.moved.map(esc).join('<br>')}</div></details>` : ''}</div>` : ''}
        ${(it.skipped || []).length ? `<div class="hint-box warn"><b>${esc(t('genv_skipped'))}</b><div class="g-mono">${it.skipped.map(esc).join('<br>')}</div></div>` : ''}
        ${it.post_deploy ? `<div class="g-mut">post_deploy: <b>${esc(it.post_deploy)}</b></div>` : ''}
        ${(it.dangling || []).length ? `<div class="hint-box bad"><b>${esc(t('git_dangling'))}</b><div class="g-mono">${it.dangling.map(esc).join('<br>')}</div></div>` : ''}
        ${(it.health || []).length ? `<div><b>${esc(t('git_health'))}:</b> ${it.health.map(h => `${h.ok ? '✅' : '❌'} <span class="g-mono">${esc(h.name)}</span> ${esc(h.status)}`).join(' · ')}</div>` : ''}
        ${it.made_backup ? `<div class="g-mut">${esc(t('git_backup'))}: <span class="g-mono">${esc(it.made_backup)}</span></div>` : ''}
        ${(it.written || []).length || (it.deleted || []).length ? `<div class="g-mut">${esc(tf('git_written', {n: (it.written || []).length}))}${(it.deleted || []).length ? ' · ' + esc(tf('git_deleted', {n: it.deleted.length})) : ''}</div>` : ''}
        <ul class="g-log">${(it.log || []).map(l => `<li>${icon(l.status)} <b>${esc(t('git_step_' + l.step))}</b> ${esc(l.msg || '')}</li>`).join('')}</ul></div>`).join('')}
      <div class="g-btns">${ev && ev.dry_run && !v.running && E.plan && E.plan.id === ev.plan_id && !E.plan.used ? `<button onclick="gitWorkspace.envBack()">${esc(t('genv_back'))}</button>` : ''}${v.running ? `<button class="btn-danger" onclick="gitWorkspace.cancelRun(${v.id})">${esc(t('git_stop'))}</button>` : v.state === 'scheduled' ? `<button class="btn-danger" onclick="gitWorkspace.cancelRun(${v.id})">${esc(t('git_cancel_run'))}</button>` : ''}
        <button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`;
    if (open && open.dataset.run === String(id)) open.querySelector('.g-dlg').innerHTML = html;
    else { modal(html); $('git-modal').dataset.run = String(id); }
    if (v.running || v.state === 'starting') runTimer = setTimeout(() => { if ($('git-modal') && $('git-modal').dataset.run === String(id)) showRun(id); }, 1000);
    else { load().then(ok => { if (ok && $('git-ws') && $('git-ws').style.display !== 'none') { render(); if (G.tab === 'runs') loadRuns(); } }); }
  }

  async function cancelRun(id) {
    if (!(await uiConfirm(t('git_cancel_q'), {danger: true, okText: t('git_cancel_run')}))) return;
    const r = await fetch(`/api/git/runs/${id}/cancel`, {method: 'POST'});
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    if ($('git-modal') && $('git-modal').dataset.run === String(id)) showRun(id);
    loadRuns();
  }

  async function loadRuns() {
    const r = await fetch('/api/git/runs');
    if (!r.ok) return;
    G.runs = (await r.json()).runs || [];
    if (G.tab === 'runs' && $('git-body')) $('git-body').innerHTML = runsHTML();
  }

  function runsHTML() {
    const list = G.runs;
    if (!list) return `<div class="hint-box">⏳</div>`;
    return `<div class="g-bar"><button class="btn-sec btn-sm" onclick="gitWorkspace.loadRuns()">↻ ${esc(t('git_refresh'))}</button></div>
      <div class="g-card g-scroll">${list.length ? `<table><tr><th>${esc(t('git_run'))}</th><th>${esc(t('git_kind_col'))}</th><th>${esc(t('git_state'))}</th><th>${esc(t('git_items'))}</th><th>${esc(t('git_versions'))}</th><th>${esc(t('git_created'))}</th><th>${esc(t('git_scheduled_for'))}</th><th></th></tr>
      ${list.map(v => `<tr><td class="g-mono">${esc(v.label)}<div class="g-mut">#${v.id}${v.parent_id ? ' ← #' + v.parent_id : ''}</div></td><td>${esc(kindName(v.kind))}${v.ref ? ` <span class="g-mono">${esc(v.ref)}</span>` : ''}${v.env ? `<div class="g-mut">${esc(v.env.app)} → ${esc(v.env.env)}${v.env.dry_run ? ' · 🧪 ' + esc(t('genv_dry')) : ''}</div>` : ''}</td>
        <td>${runBadge(v.state)}${v.message ? `<div class="g-mut">${esc(v.message)}</div>` : ''}</td>
        <td>${v.items.map(it => `<div>${runBadge(it.state)} ${esc(it.app)} @ ${esc(hn(it.conn_name))}</div>`).join('')}</td>
        <td class="g-mono">${Object.entries(v.versions || {}).map(([a, x]) => esc(a + ' ' + x)).join('<br>')}</td>
        <td class="g-mut">${esc(fmtTime(v.created_at))}</td><td class="g-mut">${v.scheduled_at ? esc(fmtTime(v.scheduled_at)) : v.parent_id && v.state === 'scheduled' ? esc(t('git_waits_update')) : '—'}</td>
        <td style="white-space:nowrap"><button class="btn-sec btn-sm" onclick="gitWorkspace.showRun(${v.id})">${esc(t('git_details'))}</button>${v.state === 'scheduled' || v.running ? ` <button class="btn-danger btn-sm" onclick="gitWorkspace.cancelRun(${v.id})">✕</button>` : ''}</td></tr>`).join('')}</table>` : `<div class="g-mut">${esc(t('git_runs_empty'))}</div>`}</div>`;
  }

  // ── activity: Git activity per branch and author, deploy history, what is where ──
  const A = {view: 'git', act: null, actErr: '', actBusy: false, af: {app: '', branch: '', author: '', since: '', until: ''},
    hf: {app: '', env: '', server: '', who: '', branch: '', version: '', action: '', since: '', until: ''}, hist: null, wf: {app: '', env: '', server: '', dir: ''}, where: null, seq: 0};
  const isoDay = d => { const p = n => String(n).padStart(2, '0'); return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`; };
  const qs = o => Object.entries(o).filter(([, v]) => v !== '' && v != null && v !== false).map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&');
  const ext = (url, html) => url ? `<a href="${esc(url)}" target="_blank" rel="noopener noreferrer">${html}</a>` : html;
  const short = s => String(s || '').slice(0, 10);
  const when = ts => ts ? esc(fmtTime(ts)) : '—';
  const opt = (v, label, cur) => `<option value="${esc(v)}" ${String(cur) === String(v) ? 'selected' : ''}>${esc(label)}</option>`;
  const envPairsOf = (app, env) => G.state.catalog.apps.filter(a => !app || a.name === app).flatMap(a => (a.environments || []).filter(e => !env || e.name === env).map(e => ({app: a.name, env: e.name, n: e.destinations.length})));
  const allEnvs = () => [...new Set(G.state.catalog.apps.flatMap(a => (a.environments || []).map(e => e.name)).concat((G.state.installs || []).map(i => i.env).filter(Boolean)))].sort();
  const allServers = () => [...new Set(G.state.catalog.apps.flatMap(a => (a.environments || []).flatMap(e => e.destinations.map(d => d.server))).concat((G.state.installs || []).map(i => i.conn_name)))].filter(Boolean).sort();
  const csvBtn = fn => `<button class="btn-sec btn-sm" onclick="gitWorkspace.${fn}()" title="${esc(t('gact_csv_h'))}">⬇ CSV</button>`;
  const dateInputs = (f, fn) => `<span class="g-date"><label class="g-mut">${esc(t('gact_from'))}</label><input type="date" value="${esc(f.since)}" onchange="gitWorkspace.${fn}('since', this.value)"></span>
    <span class="g-date"><label class="g-mut">${esc(t('gact_to'))}</label><input type="date" value="${esc(f.until)}" onchange="gitWorkspace.${fn}('until', this.value)"></span>`;

  function activityHTML() {
    const seg = ['git', 'history', 'where'].map(k => `<button class="${A.view === k ? 'on' : ''}" onclick="gitWorkspace.actView('${k}')">${esc(t('gact_v_' + k))}</button>`).join('');
    return `<div class="g-bar"><div class="g-seg">${seg}</div><span class="g-mut">${esc(t('gact_v_' + A.view + '_h'))}</span></div><div id="gact-body">${actBody()}</div>`;
  }
  function actBody() { return A.view === 'git' ? actGitHTML() : A.view === 'history' ? histHTML() : whereHTML(); }
  function actPaint() { const b = $('gact-body'); if (G.tab === 'activity' && b) b.innerHTML = actBody(); }
  function actView(v) { A.view = v; render(); actStart(); }
  function actStart() {
    if (A.view === 'git' && !A.act && !A.actBusy && !A.actErr) actLoad();
    else if (A.view === 'history' && !A.hist) histLoad();
    else if (A.view === 'where' && !A.where) whereLoad();
  }

  // Git activity
  function actDefaults() {
    if (!A.af.app) A.af.app = (G.state.catalog.apps[0] || {}).name || '';
    if (!A.af.until) A.af.until = isoDay(new Date());
    if (!A.af.since) A.af.since = isoDay(new Date(Date.now() - 30 * 864e5));
  }
  async function actLoad(refresh) {
    actDefaults();
    if (!A.af.app) { actPaint(); return; }
    const seq = ++A.seq;
    A.actBusy = true; A.actErr = ''; actPaint();
    try {
      const r = await fetch('/api/git/activity?' + qs(Object.assign({}, A.af, {refresh: refresh ? 1 : ''})));
      if (seq !== A.seq) return;
      if (r.ok) A.act = await r.json(); else { A.act = null; A.actErr = await apiError(r); }
    } catch (err) { A.actErr = String(err); }
    A.actBusy = false; actPaint();
  }
  function actFilter(k, v) { A.af[k] = v; if (k === 'app') A.af.branch = ''; A.act = null; actLoad(); }
  function actCSV() { fetch('/api/git/activity/export.csv?' + qs(A.af)).then(r => download(r, 'activity.csv')); }
  function actGitHTML() {
    actDefaults();
    const apps = G.state.catalog.apps;
    if (!apps.length) return `<div class="hint-box">${esc(t('git_empty_overview'))}</div>`;
    const x = A.act, f = A.af;
    const branches = x ? x.branches : (f.branch ? [f.branch] : []);
    let h = `<div class="g-bar"><select onchange="gitWorkspace.actFilter('app', this.value)">${apps.map(a => opt(a.name, a.name, f.app)).join('')}</select>
      <select onchange="gitWorkspace.actFilter('branch', this.value)">${opt('', t('gact_tracked'), f.branch)}${branches.map(b => opt(b, b, f.branch)).join('')}</select>
      <input type="search" placeholder="${esc(t('gact_author_ph'))}" value="${esc(f.author)}" onchange="gitWorkspace.actFilter('author', this.value.trim())">
      ${dateInputs(f, 'actFilter')}
      <button class="btn-sec btn-sm" onclick="gitWorkspace.actLoad(true)" ${A.actBusy ? 'disabled' : ''}>↻ ${esc(t('git_refresh'))}</button>${csvBtn('actCSV')}</div>`;
    if (A.actBusy) return h + `<div class="hint-box"><span class="g-spin"></span> ${esc(t('gact_loading'))}</div>`;
    if (A.actErr) return h + `<div class="hint-box bad">${esc(A.actErr)}</div>`;
    if (!x) return h;
    h += `<div class="g-mut" style="margin:-4px 0 8px">${esc(tf('gact_scope', {p: x.project, src: x.provider, b: (x.tracked || []).join(', ') || '—'}))} · ${esc(tf('gact_fetched', {t: fmtTime(x.fetched_at)}))}${x.cached ? ' · ' + esc(t('gact_cached')) : ''}</div>`;
    if (x.rate_limited) h += `<div class="hint-box warn">${esc(x.rate_reset ? tf('gact_rate_until', {t: fmtTime(x.rate_reset)}) : t('gact_rate'))}</div>`;
    else if (x.truncated) h += `<div class="hint-box warn">${esc(t('gact_truncated'))}</div>`;
    (x.notes || []).forEach(n => { h += `<div class="hint-box warn g-mono">${esc(n)}</div>`; });
    const sumRow = (s, k) => `<tr class="g-click" data-k="${esc(s.key)}" onclick="gitWorkspace.actFilter('${k}', this.dataset.k)"><td class="g-nw">${k === 'branch' ? `<span class="g-mono">${esc(s.key)}</span>` : esc(s.key)}</td><td>${s.commits}</td><td>${s.pushes}</td>
      <td>${k === 'branch' ? s.authors : `<span class="g-mono g-mut">${esc((s.branches || []).join(', '))}</span>`}</td><td class="g-mut">${when(s.last)}${k === 'branch' && s.url ? ` · ${ext(s.url, '↗')}` : ''}</td></tr>`;
    const sumTable = (list, k) => `<div class="g-card g-scroll"><h3>${esc(t(k === 'branch' ? 'gact_by_branch' : 'gact_by_author'))}</h3>${list.length ? `<table><tr><th>${esc(t(k === 'branch' ? 'gact_branch' : 'gact_author'))}</th><th>${esc(t('gact_commits'))}</th><th>${esc(t('gact_pushes'))}</th><th>${esc(t(k === 'branch' ? 'gact_authors' : 'gact_branches'))}</th><th>${esc(t('gact_last'))}</th></tr>${list.map(s => sumRow(s, k)).join('')}</table>` : `<div class="g-mut">${esc(t('gact_none'))}</div>`}</div>`;
    h += `<div class="g-grid g-wide">${sumTable(x.authors, 'author')}${sumTable(x.by_branch, 'branch')}</div>`;
    const items = x.items.slice(0, 500);
    h += `<div class="g-card g-scroll"><h3>${esc(tf('gact_items', {n: x.items.length}))}</h3>${items.length ? `<table><tr><th>${esc(t('genv_when'))}</th><th>${esc(t('gact_kind'))}</th><th>${esc(t('gact_branch'))}</th><th>${esc(t('gact_author'))}</th><th>${esc(t('gact_change'))}</th></tr>
      ${items.map(i => `<tr><td class="g-mut" style="white-space:nowrap">${when(i.at)}</td><td><span class="g-st s-k${esc(i.kind)}">${esc(t('gact_k_' + i.kind))}</span>${i.kind === 'push' && i.action && i.action !== 'pushed' && i.action !== 'push' ? `<div class="g-mut">${esc(i.action)}</div>` : ''}</td>
        <td class="g-mono g-nw">${esc(i.kind === 'commit' ? (i.branches || [i.branch]).join(', ') : i.ref_type === 'tag' ? '🏷 ' + i.ref : i.branch || i.ref || '')}</td><td>${esc(i.author || i.login || '')}</td>
        <td>${i.kind === 'push' ? `${i.count ? esc(tf('gact_n_commits', {n: i.count})) + ' · ' : ''}${ext(i.url, `<span class="g-mono">${esc(i.before && !/^0+$/.test(i.before) ? short(i.before) + '…' : '')}${esc(short(i.after))}</span>`)}${i.title ? ` <span class="g-mut">${esc(i.title)}</span>` : ''}`
          : `${ext(i.url, `<span class="g-mono">${esc(short(i.sha))}</span>`)} ${esc(i.title || '')}`}</td></tr>`).join('')}</table>${x.items.length > items.length ? `<div class="g-mut">${esc(tf('gact_more', {n: x.items.length - items.length}))}</div>` : ''}` : `<div class="g-mut">${esc(t('gact_none'))}</div>`}</div>`;
    return h;
  }

  // deploy history: the WRM runs, then one request per service and environment (three at a time)
  function histFilter(k, v) { A.hf[k] = v; histLoad(); }
  function histCSV() { fetch('/api/git/history/export.csv?' + qs(A.hf)).then(r => download(r, 'deploy-history.csv')); }
  function histSort(rows) { return rows.sort((a, b) => (Date.parse(b.at) || 0) - (Date.parse(a.at) || 0) || a.app.localeCompare(b.app) || String(a.env || '').localeCompare(String(b.env || '')) || a.server.localeCompare(b.server)); }
  async function histLoad(refresh) {
    const seq = ++A.seq, f = A.hf;
    const pairs = G.state.can_check ? envPairsOf(f.app, f.env) : [];
    const H = A.hist = {rows: [], errors: [], done: 0, total: pairs.length + 1};
    actPaint();
    const add = (rows, errs) => { if (seq !== A.seq) return false; H.rows = histSort(H.rows.concat(rows || [])); H.errors = H.errors.concat(errs || []); H.done++; actPaint(); return true; };
    try {
      const r = await fetch('/api/git/history/runs?' + qs(f));
      if (!add(r.ok ? (await r.json()).rows : [], r.ok ? [] : [await apiError(r)])) return;
    } catch (err) { if (!add([], [String(err)])) return; }
    let i = 0;
    const next = async () => {
      if (i >= pairs.length || seq !== A.seq) return;
      const p = pairs[i++];
      try {
        const r = await fetch('/api/git/history/cell?' + qs(Object.assign({}, f, {app: p.app, env: p.env, refresh: refresh ? 1 : ''})));
        if (r.ok) { const c = await r.json(); if (!add(c.rows, (c.errors || []).map(e => `${p.app} · ${p.env}: ${e}`))) return; }
        else if (!add([], [`${p.app} · ${p.env}: ${await apiError(r)}`])) return;
      } catch (err) { if (!add([], [String(err)])) return; }
      return next();
    };
    for (let n = 0; n < 3; n++) next();
  }
  function histFiles(x) {
    const keys = ['new', 'changed', 'deleted', 'excluded', 'restored', 'moved_aside', 'skipped', 'failed', 'moved', 'written'];
    const files = x.files || {}, c = x.counts || {};
    const present = keys.filter(k => (files[k] || []).length || c[k]);
    if (!present.length) return '<span class="g-mut">—</span>';
    const sum = present.map(k => `${t('gact_f_' + k)} ${c[k] != null ? c[k] : files[k].length}`).join(' · ');
    const lists = present.filter(k => (files[k] || []).length).map(k => `<div style="margin-top:4px"><b>${esc(t('gact_f_' + k))}</b><div class="g-mono g-mut">${files[k].map(esc).join('<br>')}</div></div>`).join('');
    return lists ? `<details><summary class="g-mut">${esc(sum)}</summary>${lists}</details>` : `<span class="g-mut">${esc(sum)}</span>`;
  }
  function histHTML() {
    const f = A.hf, H = A.hist;
    const apps = G.state.catalog.apps.map(a => a.name);
    let h = `<div class="g-bar"><select onchange="gitWorkspace.histFilter('app', this.value)">${opt('', t('gact_all_services'), f.app)}${apps.map(a => opt(a, a, f.app)).join('')}</select>
      <select onchange="gitWorkspace.histFilter('env', this.value)">${opt('', t('git_all_envs'), f.env)}${allEnvs().map(e => opt(e, e, f.env)).join('')}</select>
      <select onchange="gitWorkspace.histFilter('server', this.value)">${opt('', t('gact_all_servers'), f.server)}${allServers().map(s => opt(s, hn(s), f.server)).join('')}</select>
      <select onchange="gitWorkspace.histFilter('action', this.value)">${opt('', t('gact_all_actions'), f.action)}${['deploy', 'rollback', 'update', 'upgrade', 'install', 'transfer'].map(a => opt(a, t('gact_a_' + a), f.action)).join('')}</select>
      <input type="search" placeholder="${esc(t('gact_who_ph'))}" value="${esc(f.who)}" onchange="gitWorkspace.histFilter('who', this.value.trim())">
      <input type="search" placeholder="${esc(t('gact_branch_ph'))}" value="${esc(f.branch)}" onchange="gitWorkspace.histFilter('branch', this.value.trim())">
      <input type="search" placeholder="${esc(t('gact_version_ph'))}" value="${esc(f.version)}" onchange="gitWorkspace.histFilter('version', this.value.trim())">
      ${dateInputs(f, 'histFilter')}<button class="btn-sec btn-sm" onclick="gitWorkspace.histLoad(true)">↻ ${esc(t('git_refresh'))}</button>${csvBtn('histCSV')}</div>`;
    if (!G.state.can_check) h += `<div class="hint-box">${esc(t('gact_no_checks'))}</div>`;
    if (!H) return h;
    if (H.done < H.total) h += `<div class="g-mut" style="margin-bottom:6px"><span class="g-spin"></span> ${esc(tf('gact_reading', {n: H.done, m: H.total}))}</div>`;
    h += H.errors.map(e => `<div class="hint-box bad">${esc(e)}</div>`).join('');
    const res = x => x.result === 'ok' ? 'done' : x.result === 'partial' || x.result === 'post_deploy_failed' ? 'pending' : 'failed';
    h += `<div class="g-card g-scroll">${H.rows.length ? `<table><tr><th>${esc(t('genv_when'))}</th><th>${esc(t('genv_action'))}</th><th>${esc(t('git_service'))}</th><th>${esc(t('genv_servers'))}</th><th>${esc(t('genv_who'))}</th><th>${esc(t('gact_branch'))}</th><th>${esc(t('genv_version'))}</th><th>${esc(t('genv_files'))}</th></tr>
      ${H.rows.slice(0, 1000).map(x => `<tr><td class="g-mut" style="white-space:nowrap">${when(x.at)}</td>
        <td class="g-nw"><span class="g-st s-${res(x)}">${esc(t('gact_a_' + x.action))}</span>${x.result !== 'ok' ? `<div class="g-mut">${esc(x.result)}</div>` : ''}<div class="g-mut">${esc(t(x.source === 'run' ? 'gact_src_run' : 'gact_src_env'))}${x.run_id ? ' #' + x.run_id : ''}</div></td>
        <td class="g-nw"><b>${esc(x.app)}</b>${x.env ? `<span class="g-env">${esc(x.env)}</span>` : ''}${x.id ? `<div class="g-mono g-mut g-nw">${esc(x.id)}</div>` : ''}</td>
        <td class="g-path">${esc(hn(x.server))}<div class="g-mono g-mut">${esc(x.path)}</div></td><td>${esc(x.by || '')}</td><td class="g-mono g-nw">${esc(x.branch || '')}</td>
        <td class="g-mono g-nw">${esc(x.version || '')}${x.previous ? `<div class="g-mut">← ${esc(x.previous)}</div>` : ''}<div class="g-mut">${ext(x.commit_url, esc(short(x.commit)))}</div></td>
        <td style="min-width:150px">${histFiles(x)}${x.error ? `<div class="g-mut">${esc(x.error)}</div>` : ''}${x.note ? `<div class="g-mut">${esc(x.note)}</div>` : ''}</td></tr>`).join('')}</table>${H.rows.length > 1000 ? `<div class="g-mut">${esc(tf('gact_more', {n: H.rows.length - 1000}))}</div>` : ''}`
        : H.done < H.total ? '' : `<div class="g-mut">${esc(t('gact_none'))}</div>`}</div>`;
    return h;
  }

  // what is where: one request per service and environment and one per service for installations without environment
  function whereFilter(k, v) { A.wf[k] = v; if (k === 'server' || k === 'dir') actPaint(); else whereLoad(); }
  function whereCSV() { fetch('/api/git/where/export.csv?' + qs(A.wf)).then(r => download(r, 'what-is-where.csv')); }
  function whereLoad(refresh) {
    const seq = ++A.seq, f = A.wf;
    const cells = (G.state.can_check ? envPairsOf(f.app, f.env).map(p => Object.assign(p, {kind: 'env'})) : [])
      .concat(G.state.catalog.apps.filter(a => !f.app || a.name === f.app).filter(a => (G.state.installs || []).some(i => i.app === a.name)).map(a => ({app: a.name, env: '', kind: 'install'})));
    A.where = {cells, data: {}};
    actPaint();
    let i = 0;
    const next = async () => {
      if (i >= cells.length || seq !== A.seq) return;
      const c = cells[i++], key = c.kind + '\u0000' + c.app + '\u0000' + c.env;
      const url = c.kind === 'env' ? '/api/git/where/cell?' + qs({app: c.app, env: c.env, refresh: refresh ? 1 : ''}) : '/api/git/where/installs?' + qs({app: c.app, refresh: refresh ? 1 : ''});
      let d;
      try { const r = await fetch(url); d = r.ok ? await r.json() : {error: await apiError(r)}; } catch (err) { d = {error: String(err)}; }
      if (seq !== A.seq) return;
      A.where.data[key] = d;
      actPaint();
      return next();
    };
    for (let n = 0; n < 3; n++) next();
  }
  function whereRow(r) {
    const behind = r.behind > 0 ? ext(r.compare_url, `<span class="g-st s-update">${esc(tf('git_behind', {n: r.behind}))}</span>`) : r.behind === 0 ? `<span class="g-st s-ok">${esc(t('gact_at_head'))}</span>` : '<span class="g-mut">—</span>';
    return `<tr class="${r.drift ? 'g-drift' : ''}"><td class="g-nw"><b>${esc(r.app)}</b></td><td>${r.env ? `<span class="g-env">${esc(r.env)}</span>` : '<span class="g-mut">—</span>'}${r.kind === 'install' ? `<div class="g-mut">${esc(t('gact_install'))}</div>` : ''}</td>
      <td class="g-path">${esc(hn(r.server))}<div class="g-mono g-mut">${esc(r.path)}</div></td>
      ${r.error ? `<td colspan="5"><span class="g-st s-error">${esc(r.error)}</span></td>` : !r.exists ? `<td colspan="5" class="g-mut">${esc(t('genv_not_deployed'))}</td>`
        : `<td class="g-mono g-nw">${ext(r.branch_url, esc(r.branch || '—'))}</td><td class="g-mono g-nw">${ext(r.commit_url, esc(short(r.commit) || '—'))}</td>
      <td class="g-mono g-nw">${esc(r.version || '—')}${r.drift ? ` <span class="g-st s-review">⚠ ${esc(t('gact_drift'))}</span>` : ''}${r.state ? ` ${stBadge(r.state)}` : ''}${r.locked ? ' 🔒' : ''}</td>
      <td class="g-nw">${esc(r.by || '—')}<div class="g-mut">${r.at ? esc(/^\d{4}-\d\d-\d\dT/.test(r.at) ? fmtTime(r.at) : r.at) : ''}</div></td><td class="g-nw">${behind}</td>`}</tr>`;
  }
  function whereHTML() {
    const f = A.wf, W = A.where;
    const apps = G.state.catalog.apps.map(a => a.name);
    let h = `<div class="g-bar"><select onchange="gitWorkspace.whereFilter('app', this.value)">${opt('', t('gact_all_services'), f.app)}${apps.map(a => opt(a, a, f.app)).join('')}</select>
      <select onchange="gitWorkspace.whereFilter('env', this.value)">${opt('', t('git_all_envs'), f.env)}${allEnvs().map(e => opt(e, e, f.env)).join('')}</select>
      <select onchange="gitWorkspace.whereFilter('server', this.value)">${opt('', t('gact_all_servers'), f.server)}${allServers().map(s => opt(s, hn(s), f.server)).join('')}</select>
      ${dirSelect("gitWorkspace.whereFilter('dir', this.value)", (G.state.installs || []).map(i => i.path), f.dir, true)}
      <button class="btn-sec btn-sm" onclick="gitWorkspace.whereLoad(true)">↻ ${esc(t('git_refresh'))}</button>${csvBtn('whereCSV')}</div>`;
    if (!G.state.can_check) h += `<div class="hint-box">${esc(t('gact_no_checks'))}</div>`;
    if (!W) return h;
    if (!W.cells.length) return h + `<div class="hint-box">${esc(t('gact_where_none'))}</div>`;
    let rows = '';
    for (const c of W.cells) {
      const d = W.data[c.kind + '\u0000' + c.app + '\u0000' + c.env];
      if (!d) { rows += `<tr><td><b>${esc(c.app)}</b></td><td>${c.env ? `<span class="g-env">${esc(c.env)}</span>` : '—'}</td><td colspan="6" class="g-mut"><span class="g-spin"></span> ${esc(c.kind === 'env' ? tf('genv_n_dests', {n: c.n}) : t('gact_install'))}</td></tr>`; continue; }
      if (d.error) { rows += `<tr><td><b>${esc(c.app)}</b></td><td>${c.env ? `<span class="g-env">${esc(c.env)}</span>` : '—'}</td><td colspan="6"><span class="g-st s-error">${esc(d.error)}</span></td></tr>`; continue; }
      const list = (d.rows || []).filter(r => !f.server || r.server === f.server).filter(r => !f.env || r.env === f.env);
      if (d.note && list.length) rows += `<tr><td colspan="8" class="g-mut">${esc(c.app)}: ${esc(d.note)}</td></tr>`;
      rows += list.map(whereRow).join('');
    }
    return h + `<div class="g-card g-scroll"><table class="g-where"><tr><th>${esc(t('git_service'))}</th><th>${esc(t('gact_env'))}</th><th>${esc(t('genv_servers'))}</th><th>${esc(t('gact_branch'))}</th><th>${esc(t('gact_commit'))}</th><th>${esc(t('genv_version'))}</th><th>${esc(t('gact_deployed'))}</th><th>${esc(t('gact_behind'))}</th></tr>${rows}</table></div>`;
  }

  // ── Simple mode: one guided card per service environment, built on the environment endpoints ──
  const S = {pending: new Set(), loading: false, plan: null, ctx: null, submitting: false, excluded: new Set(), destinations: new Set(), rollbackDestinations: new Set(), filters: {app: '', server: '', dir: '', env: '', status: '', branch: '', version: '', q: ''}};
  const sKey = (app, env) => 'gs-' + btoa(unescape(encodeURIComponent(app + '\u0000' + env))).replace(/[^A-Za-z0-9]/g, '');
  const sh10 = c => String(c || '').slice(0, 10);

  function setGitMode(m) {
    G.mode = ['simple', 'medium'].includes(m) ? m : 'advanced';
    try { localStorage.setItem(modeKey(), G.mode); } catch (_) {}
    closeModal();
    if (G.mode === 'advanced') setTab(G.tab); else render();
  }


  // Medium (v12.6.5): a step-by-step guide for the original deploy tool with a status light
  // per part, the running operation's steps and output, and the activity log; WRM's
  // read-only preview is a separate card. The original tool itself opens below its card.
  let mediumTimer = null;
  const MS = {s: null, rows: [], openRow: null, showLog: false, logText: null, lastOp: ''};
  const mClock = iso => {
    if (!iso) return ''; const d = new Date(iso); if (isNaN(d)) return iso;
    const time = d.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false});
    return d.toDateString() === new Date().toDateString() ? time : d.toLocaleDateString([], {day: '2-digit', month: '2-digit'}) + ' ' + time;
  };
  const mDur = ms => { ms = Math.max(0, +ms || 0); if (ms < 1000) return '< 1 s'; const s = Math.round(ms / 1000); if (s < 60) return s + ' s'; const m = Math.floor(s / 60); return m < 60 ? m + ' min ' + (s % 60) + ' s' : Math.floor(m / 60) + ' h ' + (m % 60) + ' min'; };
  const mSince = iso => iso ? mDur(Date.now() - new Date(iso).getTime()) : '';
  const mLight = (kind, text) => `<span class="gm-light gm-${kind}"><i></i>${esc(text)}</span>`;
  const mOp = op => t('gm_op_' + op) !== 'gm_op_' + op ? t('gm_op_' + op) : op;
  const mStep = st => t('gm_st_' + st) !== 'gm_st_' + st ? t('gm_st_' + st) : st;
  async function paintMedium() {
    const body = $('git-body');
    if (!body || G.mode !== 'medium') return;
    if (!body.querySelector('#medium-status')) {
      const fileBtn = (kind, id) => `<label class="btn-sec btn-sm"${id ? ` id="${id}"` : ''} tabindex="0" role="button" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.querySelector('input').click()}">⇪ ${esc(t('gm_import'))}<input type="file" accept=".zip" hidden onchange="gitWorkspace.mediumUpload('${kind}',this)"></label>`;
      body.innerHTML = `<div class="g-card gm-overview"><h3>${esc(t('gm_two_title'))}</h3><div class="gm-two">
        <div><b>🛠 ${esc(t('gm_title'))}</b><p>${esc(t('gm_two_original'))}</p></div>
        <div><b>🔎 ${esc(t('mn_title'))}</b><p>${esc(t('gm_two_preview'))}</p></div></div></div>
      <details class="g-card" id="gm-original" open><summary class="gm-head"><h3>🛠 ${esc(t('gm_title'))}</h3><span id="gm-light"></span></summary>
        <p class="g-mut" id="gm-optional" hidden>${esc(t('gm_optional'))}</p>
        <div id="medium-status" role="status" aria-live="polite">${esc(t('loading'))}</div>
        <div id="gm-op"></div>
        <ol class="gm-steps" id="gm-steps">
          <li id="gm-s1"><div class="gm-sh"><span class="gm-ico"></span><b>1. ${esc(t('gm_s1'))}</b><span class="gm-act">${fileBtn('tool', 'medium-tool-label')}</span></div><div class="gm-sd"></div></li>
          <li id="gm-s2"><div class="gm-sh"><span class="gm-ico"></span><b>2. ${esc(t('gm_s2'))}</b><span class="gm-act"><button class="btn-sec btn-sm" id="medium-prepare" onclick="gitWorkspace.mediumAction('prepare')">${esc(t('gm_prepare'))}</button><button class="btn-sm" id="medium-docker-install" onclick="gitWorkspace.mediumDockerInstall()">🐳 ${esc(t('gm_dk_install'))}</button></span></div><div class="gm-sd"></div></li>
          <li id="gm-s3"><div class="gm-sh"><span class="gm-ico"></span><b>3. ${esc(t('gm_s3'))}</b><span class="gm-act">${fileBtn('config')}</span></div><div class="gm-sd"></div></li>
          <li id="gm-s4"><div class="gm-sh"><span class="gm-ico"></span><b>4. ${esc(t('gm_s4'))}</b><span class="gm-act">${fileBtn('ssh')}</span></div><div class="gm-sd"></div></li>
          <li id="gm-s5"><div class="gm-sh"><span class="gm-ico"></span><b>5. ${esc(t('gm_s5'))}</b><span class="gm-act"><button class="btn-sm" id="gm-start" onclick="gitWorkspace.mediumAction('start')">▶ ${esc(t('gm_start'))}</button><button class="btn-sec btn-sm" id="gm-stop" onclick="gitWorkspace.mediumAction('stop')">■ ${esc(t('gm_stop'))}</button><a class="btn-sec btn-sm" id="gm-open" href="/medium/" target="_blank" rel="noopener">↗ ${esc(t('gm_open'))}</a></span></div><div class="gm-sd"></div></li>
        </ol>
        <details class="gm-quick"><summary>${esc(t('gm_quick_title'))}</summary><p>${esc(t('gm_quick_hint'))}</p><p class="g-mut">${esc(t('gm_quick_files'))}</p><div class="g-grid" id="medium-quick-inputs"><label id="medium-quick-tool-label">${esc(t('gm_tool'))}<input id="medium-quick-tool" type="file" accept=".zip"></label><label>${esc(t('gm_config'))}<input id="medium-quick-config" type="file" accept=".zip"></label><label>${esc(t('gm_ssh'))}<input id="medium-quick-ssh" type="file" accept=".zip"></label></div><div class="g-bar"><button id="medium-quick-go" onclick="gitWorkspace.mediumQuickSetup()">▶ ${esc(t('gm_quick'))}</button></div></details>
        <div class="g-bar" style="margin-top:8px"><button class="btn-sec btn-sm" onclick="gitWorkspace.mediumRefresh()">↻ ${esc(t('git_refresh'))}</button><button class="btn-sec btn-sm" onclick="gitWorkspace.mediumSettings()">⚙ ${esc(t('gm_settings'))}</button></div>
      </details>
      <div id="medium-frame"></div>
      <details class="g-card" id="mn-card" ontoggle="if(this.open)gitWorkspace.nativeLoad()"><summary><b>🔎 ${esc(t('mn_title'))}</b> <span id="gm-preview-light"></span></summary><div id="mn-body"></div></details>
      <div class="g-card" id="gm-activity"><div class="gm-head"><h3>📜 ${esc(t('gm_log_title'))}</h3><button class="btn-sec btn-sm" onclick="gitWorkspace.mediumActivity(true)">↻</button></div><p class="g-mut">${esc(t('gm_log_hint'))}</p><div id="gm-rows"></div></div>`;
      MS.lastOp = ''; MS.arranged = false;
    }
    await mediumRefresh();
  }
  // what the user must do next, for the step that failed
  function mediumHint(step, err) {
    step = step || '';
    if (step === 'docker') return t('gm_hint_docker');
    if (['docker_install', 'docker_download', 'docker_verify'].includes(step)) return t('gm_hint_docker_install');
    if (step === 'docker_start' || step === 'docker_wait') return (MS.s || {}).tool_imported ? t('gm_hint_dk_error') : t('gm_hint_dk_error_new');
    if (step === 'image') return t('gm_hint_image');
    if (step === 'packages') return /certificate/i.test(String(err || '')) ? t('gm_hint_packages_tls') : t('gm_hint_packages');
    if (step === 'python' || step === 'venv') return t('gm_hint_python');
    if (step === 'config_check') return /rejected/.test(String(err || '')) ? t('gm_hint_config') : '';
    if (step === 'start_process' || step === 'start_ready') return /cancelled/.test(String(err || '')) ? '' : mOutHidden() ? t('gm_hint_start_noout') : t('gm_hint_start');
    if (step === 'start_sync') return /docker/i.test(String(err || '')) ? t('gm_hint_docker') : '';
    if (step === 'upload' || step === 'tool_unpack') return /deploy-config is not valid/.test(String(err || '')) ? t('gm_hint_config_wrm') : /ZIP|MiB|missing|expected/i.test(String(err || '')) ? t('gm_hint_upload') : '';
    return '';
  }
  // the administrator hides command output from this user (medium_log_output)
  function mOutHidden() { const s = MS.s || {}; return s.output_policy === 'none' || (s.output_policy === 'admins' && !s.admin); }
  function mediumStepsHTML(steps, live) {
    if (!steps || !steps.length) return '';
    return `<ul class="gm-tl">${steps.map(x => {
      const ico = x.status === 'ok' ? '✓' : x.status === 'error' ? '✗' : '⏳';
      const took = x.status === 'running' ? (live ? mSince(x.started) : '') : mDur(x.ms);
      return `<li class="gm-${x.status}"><span>${ico}</span> ${esc(mStep(x.step))} <span class="g-mut">${esc(mClock(x.started))}${took ? ' · ' + esc(took) : ''}</span>${x.error ? `<div class="gm-err">${esc(x.error)}</div>` : ''}</li>`;
    }).join('')}</ul>`;
  }
  function mediumPaintSteps(s) {
    const deps = Object.entries(s.dependencies || {}).filter(([, ok]) => !ok).map(([n]) => n);
    const runningOp = s.busy ? s.op : '';
    const failedStep = (s.steps || []).find(x => x.status === 'error');
    const set = (n, state, text, extra) => {
      const li = $('gm-s' + n); if (!li) return;
      li.className = 'gm-' + state;
      li.querySelector('.gm-ico').textContent = {ok: '✓', run: '⏳', pend: '◔', wait: '○', bad: '✗', opt: '–'}[state] || '○';
      li.querySelector('.gm-sd').innerHTML = esc(text) + (extra || '');
    };
    // 1. the deploy tool (an administrator imports it once for everyone)
    if (s.tool_imported) set(1, 'ok', tf('gm_s1_ok', {sha: (s.tool_digest || '').slice(0, 12)}));
    else set(1, 'wait', s.admin ? t('gm_s1_todo_admin') : t('gm_s1_todo_user'));
    // 2. its runtime (Docker or Python on the WRM host)
    const where = s.backend === 'docker' ? t('gm_rt_docker') : t('gm_rt_host');
    const holder = s.operation && !s.operation.mine ? s.operation : null;
    const rtSteps = ['docker', 'docker_install', 'docker_download', 'docker_verify', 'docker_start', 'docker_wait', 'image', 'packages', 'python', 'venv'];
    const dk = s.docker && !s.docker.installed ? s.docker : null;
    if (holder && ['prepare', 'tool_import', 'docker'].includes(holder.op)) set(2, 'run', tf('gm_s2_run_other', {where, user: holder.user, at: mClock(holder.started)}));
    else if (/^docker_/.test(s.stage || '') && s.busy) set(2, 'run', tf('gm_s2_dk_run', {step: mStep(s.stage)}));
    else if (['prepare', 'tool_import', 'docker'].includes(runningOp) || (runningOp === 'setup' && rtSteps.includes(s.stage))) set(2, 'run', tf('gm_s2_run', {where, step: mStep(s.stage)}));
    else if (s.tool) set(2, 'ok', tf('gm_s2_ok', {where}));
    else if (dk) mediumPaintDocker(dk, failedStep, set, s.admin);
    else if (failedStep && rtSteps.includes(failedStep.step)) set(2, 'bad', tf('gm_s2_failed', {where}), `<div class="gm-todo">${esc(mediumHint(failedStep.step, failedStep.error))}</div>`);
    else if (!s.tool_imported) set(2, 'wait', tf('gm_s2_after1', {where}));
    else if (deps.length) set(2, 'bad', tf('gm_s2_missing', {where, deps: deps.join(', ')}));
    else set(2, 'wait', s.admin ? tf('gm_s2_todo_admin', {where}) : t('gm_s2_todo_user'));
    // 3. deploy-config (each user's own)
    if (s.pending_config && s.default_engine === 'native' && !s.tool) set(3, 'ok', t('gm_s3_native'));
    else if (s.pending_config) set(3, 'pend', t('gm_s3_pending'));
    else if (s.config) set(3, 'ok', t('gm_s3_ok'));
    else set(3, 'wait', t('gm_s3_todo'));
    if (failedStep && failedStep.step === 'config_check' && !s.pending_config) set(3, 'bad', t('gm_s3_rejected'), `<div class="gm-todo">${esc(t('gm_hint_config'))}</div>`);
    // 4. SSH profile (optional)
    if (s.pending_ssh) set(4, 'pend', t('gm_s4_pending'));
    else set(4, 'opt', t('gm_s4_text'));
    // 5. start
    const ready = s.tool && (s.config || s.pending_config);
    if (s.running && s.ready) set(5, 'ok', tf('gm_s5_running', {since: mClock(s.running_since), took: mSince(s.running_since)}));
    else if (runningOp === 'start' || (runningOp === 'setup' && (s.stage || '').startsWith('start_'))) set(5, 'run', tf('gm_s5_starting', {step: mStep(s.stage)}));
    else if (failedStep && failedStep.step.startsWith('start_')) { const h = mediumHint(failedStep.step, failedStep.error); set(5, 'bad', t('gm_s5_failed'), h ? `<div class="gm-todo">${esc(h)}</div>` : ''); }
    else if (s.last_exit && !s.last_exit.expected) set(5, 'bad', tf('gm_s5_exited', {at: mClock(s.last_exit.at), code: s.last_exit.code}), `<div class="gm-todo">${esc(mOutHidden() ? t('gm_hint_start_noout') : t('gm_hint_start'))}</div>`);
    else if (ready) set(5, 'wait', t('gm_s5_ready'));
    else set(5, 'wait', t('gm_s5_todo'));
    // buttons: only what can work now; the reason is in the title
    const other = s.operation && !s.operation.mine;
    const busy = !!s.busy || !!s.operation;
    const startBtn = $('gm-start'), stopBtn = $('gm-stop');
    startBtn.disabled = busy || s.running || !ready;
    startBtn.title = s.running ? t('gm_why_running') : busy ? t('gm_why_busy') : !ready ? t('gm_s5_todo') : '';
    stopBtn.disabled = !s.running;
    $('gm-open').style.display = s.running && s.ready ? '' : 'none';
    $('medium-tool-label').style.display = s.admin ? '' : 'none';
    // a new deploy tool needs an idle Medium and no running tool (all users)
    const toolIn = $('medium-tool-label').querySelector('input');
    if (toolIn) toolIn.disabled = busy || !!s.running;
    $('medium-tool-label').title = busy ? t('gm_why_busy') : s.running ? t('gm_why_prepare_running') : '';
    $('medium-quick-tool-label').style.display = s.admin ? '' : 'none';
    $('medium-quick-go').disabled = busy || !!s.running;
    $('medium-quick-go').title = busy ? t('gm_why_busy') : s.running ? t('gm_why_running') : '';
    document.querySelectorAll('#medium-quick-inputs input').forEach(i => i.disabled = busy);
    $('medium-prepare').style.display = s.admin && s.tool_imported ? '' : 'none';
    $('medium-prepare').disabled = busy || !!s.running;
    $('medium-prepare').title = s.running ? t('gm_why_prepare_running') : busy ? t('gm_why_busy') : '';
    const dkBtn = $('medium-docker-install'), dki = s.docker || {};
    const dkFailed = failedStep && /^docker/.test(failedStep.step) && !s.tool_imported;
    dkBtn.style.display = s.admin && s.backend === 'docker' && dki.os && dki.allowed && ((!dki.installed && dki.auto) || (dkFailed && dki.consent)) ? '' : 'none';
    dkBtn.disabled = busy;
    dkBtn.title = busy ? t('gm_why_busy') : '';
    return other;
  }
  // step 2 while Docker is missing: what WRM will do, what it needs, or what to run by hand
  function mDockerHow(dk) { return dk.method === 'desktop' ? t('gm_dk_how_desktop') : tf('gm_dk_how_pkg', {method: dk.method || '?'}); }
  function mediumPaintDocker(dk, failedStep, set, admin) {
    const license = dk.needs_license ? `<div class="g-mut">${esc(t('gm_dk_license'))} <a href="${esc(dk.license_url)}" target="_blank" rel="noopener">${esc(t('gm_dk_license_link'))}</a></div>` : '';
    const note = dk.note ? `<div class="g-mut">${esc(dk.note)}</div>` : '';
    const failed = failedStep && /^docker/.test(failedStep.step) ? `<div class="gm-todo">${esc(mediumHint(failedStep.step, failedStep.error))}</div>` : '';
    if (!dk.allowed) set(2, 'bad', tf('gm_s2_dk_off', {product: dk.product}));
    else if (!dk.auto) set(2, 'bad', tf('gm_s2_dk_manual', {product: dk.product, why: dk.why || '?'}), dk.command ? `<div class="gm-todo">${esc(t('gm_s2_dk_cmd'))}<pre class="gm-cmd">${esc(dk.command)}</pre></div>` : '');
    else if (!dk.consent) set(2, failed ? 'bad' : 'wait', admin ? tf('gm_s2_dk_consent', {product: dk.product, how: mDockerHow(dk)}) : tf('gm_s2_dk_wait_admin', {product: dk.product}), (admin ? license + note : '') + failed);
    else set(2, failed ? 'bad' : 'wait', tf('gm_s2_dk_auto', {product: dk.product, how: mDockerHow(dk), user: dk.consent.user, at: mClock(dk.consent.at)}), note + failed);
  }
  async function mediumDockerInstall() {
    const dk = (MS.s || {}).docker || {};
    let msg = tf('gm_dk_confirm', {product: dk.product || 'Docker', how: mDockerHow(dk)});
    if (dk.note) msg += '\n\n' + dk.note;
    if (dk.needs_license) msg += '\n\n' + t('gm_dk_license') + '\n\n' + tf('gm_dk_confirm_license', {url: dk.license_url});
    if (!(await uiConfirm(msg, {okText: dk.needs_license ? t('gm_dk_ok_license') : t('gm_dk_ok')}))) return;
    try {
      const r = await fetch('/api/git/medium/docker', json('POST', {accept: true, accept_license: !!dk.needs_license}));
      if (!r.ok) throw new Error(await apiError(r));
      MS.showLog = true; MS.logText = null;
      await mediumRefresh(); mediumActivity(false);
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 12000); mediumRefresh(); }
  }
  // re-rendered output keeps its place; new output opens at its end, where errors are
  function mRender(box, html) {
    const was = [...box.querySelectorAll('pre.gm-out')].map(p => p.scrollTop + p.clientHeight >= p.scrollHeight - 8 ? -1 : p.scrollTop);
    const det = {};
    box.querySelectorAll('details[data-k]').forEach(d => { det[d.dataset.k] = d.open; });
    box.innerHTML = html;
    box.querySelectorAll('details[data-k]').forEach(d => { if (d.dataset.k in det) d.open = det[d.dataset.k]; });
    box.querySelectorAll('pre.gm-out').forEach((p, i) => { p.scrollTop = was[i] == null || was[i] < 0 ? p.scrollHeight : was[i]; });
  }
  const mOutEnd = `ontoggle="if(this.open){const p=this.querySelector('pre');if(p)p.scrollTop=p.scrollHeight}"`;
  function mediumPaintOp(s) {
    const box = $('gm-op'); if (!box) return;
    const op = s.operation;
    let html = '';
    if (op) {
      html += `<div class="hint-box gm-now">⏳ <b>${esc(tf(op.mine ? 'gm_now_mine' : 'gm_now_other', {op: mOp(op.op), user: op.user, at: mClock(op.started), took: mSince(op.started)}))}</b>${op.mine ? '' : `<div>${esc(t('gm_now_other_wait'))}</div>`}`;
      if (op.mine) html += mediumStepsHTML(s.steps, true);
      html += `<div class="g-bar"><button class="btn-sec btn-sm" onclick="gitWorkspace.mediumToggleLog()">${esc(MS.showLog ? t('gm_hide_output') : t('gm_show_output'))}</button></div>${MS.showLog ? `<pre class="gm-out" id="gm-out">${esc(MS.logText == null ? t('loading') : MS.logText || t('gm_no_output'))}</pre>` : ''}</div>`;
    } else if (s.op && s.steps && s.steps.length) {
      const failed = s.steps.find(x => x.status === 'error');
      // its output, from the activity log
      const row = MS.rows.find(x => x.op === s.op && x.at === s.op_started);
      const out = row && row.output != null ? `<details class="gm-lastout" data-k="${esc(s.op + '@' + s.op_started)}"${failed ? ' open' : ''} ${mOutEnd}><summary>${esc(t('gm_show_output'))}</summary><pre class="gm-out">${esc(row.output || t('gm_no_output'))}</pre></details>` : '';
      html += `<div class="hint-box ${failed ? 'gm-bad' : ''}"><b>${esc(tf(failed ? 'gm_last_failed' : 'gm_last_ok', {op: mOp(s.op), at: mClock(s.op_started)}))}</b>${mediumStepsHTML(s.steps, false)}${failed && mediumHint(failed.step, s.error || failed.error) ? `<div class="gm-todo">${esc(mediumHint(failed.step, s.error || failed.error))}</div>` : ''}${out}<div class="g-mut">${esc(t('gm_last_more'))}</div></div>`;
    } else if (s.error) {
      html += `<div class="hint-box gm-bad">${esc(s.error)}</div>`;
    }
    mRender(box, html);
  }
  // With the native engine as the default WRM deploys by itself: its card comes first and
  // opens, and the original tool's card folds away as optional (it stays usable).
  function mediumArrange(s) {
    const native = s.default_engine === 'native', orig = $('gm-original'), mn = $('mn-card');
    if (!orig || !mn) return;
    $('gm-optional').hidden = !native;
    if (MS.arranged === native) return;
    if (native) {
      orig.parentNode.insertBefore(mn, orig);
      if (!s.tool_imported && !s.running) orig.open = false;
      if (!mn.open) mn.open = true;
    }
    MS.arranged = native;
  }
  async function mediumRefresh() {
    clearTimeout(mediumTimer);
    if (G.mode !== 'medium' || !$('medium-status') || !$('git-ws') || $('git-ws').style.display === 'none') return;
    let s;
    try {
      const r = await fetch('/api/git/medium');
      if (!r.ok) { $('medium-status').textContent = await apiError(r); if ($('medium-frame')) $('medium-frame').replaceChildren(); mediumTimer = setTimeout(mediumRefresh, 10000); return; }
      s = await r.json();
    } catch (e) {
      // a network blip during a long background setup must not freeze the status
      if ($('medium-status')) $('medium-status').textContent = t('gm_conn_lost');
      mediumTimer = setTimeout(mediumRefresh, 5000);
      return;
    }
    MS.s = s;
    const other = mediumPaintSteps(s);
    const op = s.operation;
    let light;
    if (s.running && s.ready) light = mLight('ok', t('gm_l_running'));
    else if (op && op.mine) light = mLight('run', tf('gm_l_working', {op: mOp(op.op)}));
    else if (op) light = mLight('run', tf('gm_l_other', {user: op.user}));
    else if ((s.steps || []).some(x => x.status === 'error') || (s.last_exit && !s.last_exit.expected)) light = mLight('bad', t('gm_l_error'));
    else light = mLight('off', t('gm_l_stopped'));
    $('gm-light').innerHTML = light;
    const where = s.backend === 'docker' ? t('gm_rt_docker') : t('gm_rt_host');
    $('medium-status').innerHTML = esc([tf('gm_runtime_is', {where}), s.running ? tf('gm_s5_running', {since: mClock(s.running_since), took: mSince(s.running_since)}) : '', other ? t('gm_now_other_short') : ''].filter(Boolean).join(' · '));
    $('gm-preview-light').innerHTML = s.config || s.pending_config ? mLight('ok', t('gm_p_ready')) : mLight('off', t('gm_p_needs_config'));
    mediumArrange(s);
    mediumPaintOp(s);
    if (MS.showLog && op) mediumLoadLog();
    const host = $('medium-frame');
    if (s.running && s.ready && host && !host.firstChild) { const f = document.createElement('iframe'); f.title = t('gm_title'); f.src = '/medium/'; f.style.cssText = 'width:100%;height:calc(100vh - 250px);min-height:420px;border:1px solid var(--border);border-radius:8px;background:#fff'; host.appendChild(f); }
    if (!(s.running && s.ready) && host) host.replaceChildren();
    // the preview card's "Compare with the original tool" follows the tool's state
    const up = !!(s.running && s.ready);
    if (MN.cfg) MN.cfg.running = up;
    if ($('mn-compare') && $('mn-preview') && !$('mn-preview').disabled) { $('mn-compare').disabled = !up; $('mn-compare').title = up ? '' : t('mn_needs_running'); }
    const opKey = (op ? op.id + ':' + (s.stage || '') : '') + '|' + (s.op_started || '') + '|' + s.running + '|' + s.ready;
    if (opKey !== MS.lastOp) { MS.lastOp = opKey; mediumActivity(false).then(() => { if (MS.s === s && !s.operation) mediumPaintOp(s); }); }
    G.mediumBusy = !!op || !!s.busy;
    mediumTimer = setTimeout(mediumRefresh, G.mediumBusy ? 2000 : 10000);
  }
  async function mediumLoadLog() {
    try { const r = await fetch('/api/git/medium/log'); if (!r.ok) return; const v = await r.json(); MS.logText = v.operation || v.process || ''; if (v.policy === 'none' || (v.policy === 'admins' && !(MS.s && MS.s.admin))) MS.logText = t('gm_output_policy'); const pre = $('gm-out'); if (pre) { const end = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 8; pre.textContent = MS.logText || t('gm_no_output'); if (end) pre.scrollTop = pre.scrollHeight; } } catch (_) {}
  }
  function mediumToggleLog() { MS.showLog = !MS.showLog; MS.logText = null; if (MS.s) mediumPaintOp(MS.s); if (MS.showLog) mediumLoadLog(); }
  async function mediumActivity(force) {
    const box = $('gm-rows'); if (!box) return;
    try {
      const r = await fetch('/api/git/medium/activity?limit=50&output=1'); if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json(); MS.rows = v.rows || [];
    } catch (e) { if (force) box.textContent = String(e); return; }
    if (!MS.rows.length) { box.innerHTML = `<div class="g-mut">${esc(t('gm_log_empty'))}</div>`; return; }
    mRender(box, `<table class="gm-log"><thead><tr><th>${esc(t('gm_log_when'))}</th><th>${esc(t('gm_log_who'))}</th><th>${esc(t('gm_log_what'))}</th><th>${esc(t('gm_log_result'))}</th><th>${esc(t('gm_log_took'))}</th></tr></thead><tbody>${MS.rows.map(x => {
      const res = x.status === 'ok' ? mLight('ok', t('gm_r_ok')) : x.status === 'error' ? mLight('bad', t('gm_r_error')) : x.status === 'running' ? mLight('run', t('gm_r_running')) : mLight('off', t('gm_r_' + x.status) !== 'gm_r_' + x.status ? t('gm_r_' + x.status) : x.status);
      const open = MS.openRow === x.id;
      const more = x.detail || x.error || (x.steps && x.steps.length) || x.output != null;
      return `<tr class="${more ? 'gm-click' : ''}" ${more ? `onclick="gitWorkspace.mediumRow(${x.id})"` : ''}><td>${esc(mClock(x.at))}</td><td>${esc(x.user)}</td><td>${more ? (open ? '▾ ' : '▸ ') : ''}${esc(mOp(x.op))}${x.detail ? `<div class="g-mut">${esc(x.detail)}</div>` : ''}</td><td>${res}</td><td>${x.status === 'running' ? '' : esc(x.ms ? mDur(x.ms) : '–')}</td></tr>` + (open ? `<tr><td colspan="5">${x.error && !(x.steps || []).some(st => st.error) ? `<div class="gm-err">${esc(x.error)}</div>` : ''}${mediumStepsHTML(x.steps, false)}${x.output != null ? `<pre class="gm-out">${esc(x.output || t('gm_no_output'))}</pre>` : ''}</td></tr>` : '');
    }).join('')}</tbody></table>`);
  }
  function mediumRow(id) { MS.openRow = MS.openRow === id ? null : id; mediumActivity(false); }

  // Native engine preview (read-only): WRM reads deploy-config, prepares the source and
  // compares it with the servers through WRM connections; optionally side by side with the
  // original tool's own comparison.
  const MN = {cfg: null, timer: null, run: null, job: null, jobTimer: null, hist: null};
  const mnFmt = (k, ...a) => t(k).replace(/\{(\d)\}/g, (_, i) => a[+i] ?? '');
  async function nativeLoad() {
    const box = $('mn-body'); if (!box) return;
    box.textContent = t('loading');
    try {
      const r = await fetch('/api/git/medium/native/config'); if (!r.ok) throw new Error(await apiError(r));
      MN.cfg = await r.json(); nativePaint();
    } catch (e) { box.textContent = String(e); }
  }
  function nativePaint() {
    const c = MN.cfg, box = $('mn-body'); if (!box || !c) return;
    const refresh = `<button class="btn-sec" onclick="gitWorkspace.nativeLoad()">↻ ${esc(t('git_refresh'))}</button>`;
    if (!c.ok) { box.innerHTML = `<p><span class="g-st s-error">${esc(c.error)}</span></p><p class="g-mut">${esc(t('mn_cfg_start'))}</p><div class="g-bar">${refresh}<label class="btn-sec" tabindex="0" role="button" onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();this.querySelector('input').click()}">⇪ ${esc(t('mn_cfg_upload'))}<input type="file" accept=".zip" hidden onchange="gitWorkspace.mediumUpload('config',this)"></label><button class="btn-sec" onclick="gitWorkspace.nativeConfig()">📝 ${esc(t('mn_cfg_open'))}</button></div>`; return; }
    const conns = c.connections || [];
    const rows = c.config.servers.map(s => {
      const l = (c.links || {})[s.name];
      const opts = `<option value="0">${esc(l && l.auto ? t('mn_auto') + ': ' + l.conn_name : t('mn_none'))}</option>` + conns.map(x => `<option value="${x.id}" ${l && !l.auto && l.conn_id === x.id ? 'selected' : ''}>${esc(x.name)} (${esc(x.user + '@' + x.host)})</option>`).join('');
      return `<tr><td class="g-mono">${esc(s.name)}</td><td class="g-mono">${esc(s.user + '@' + s.host + ':' + s.port)}</td><td><select data-mn-server="${esc(s.name)}" aria-label="${esc(t('mn_connection') + ' ' + s.name)}" onchange="gitWorkspace.nativeLink(this)">${opts}</select>${l ? '' : ` <span class="g-st s-missing">${esc(t('mn_unlinked'))}</span>`}</td></tr>`;
    }).join('');
    const projects = c.projects.map(p => `<option value="${esc(p.name)}">${esc(p.name)}${p.build ? ' · build' : ''}</option>`).join('');
    box.innerHTML = `<p class="g-mut">${esc(t('mn_intro'))}</p>
      <h4>${esc(t('mn_servers'))}</h4><div class="g-scroll"><table><thead><tr><th>${esc(t('mn_server'))}</th><th>${esc(t('mn_address'))}</th><th>${esc(t('mn_connection'))}</th></tr></thead><tbody>${rows}</tbody></table></div>
      <h4>${esc(t('mn_plan'))}</h4>
      <div class="g-grid"><label>${esc(t('mn_project'))}<select id="mn-project" onchange="gitWorkspace.nativeProject()">${projects}</select></label><label>${esc(t('mn_env'))}<select id="mn-env" onchange="gitWorkspace.nativeEnv()"></select></label><label>${esc(t('mn_ref'))}<input id="mn-ref" list="mn-refs" placeholder="main" autocomplete="off"><datalist id="mn-refs"></datalist></label></div>
      <div id="mn-dests" class="g-bar"></div><div id="mn-engine" class="g-bar"></div><div id="mn-build" class="g-bar"></div><p id="mn-note" class="g-mut"></p>
      <div class="g-bar"><button id="mn-preview" onclick="gitWorkspace.nativePlan(false)">${esc(t('mn_preview'))}</button><button class="btn-sec" id="mn-compare" onclick="gitWorkspace.nativePlan(true)" ${c.running ? '' : 'disabled'} title="${esc(c.running ? '' : t('mn_needs_running'))}">${esc(t('mn_compare'))}</button>${refresh}</div>
      <div id="mn-result" role="status" aria-live="polite"></div><div id="mn-job" role="status" aria-live="polite"></div>
      <h4>${esc(t('mn_hist_title'))}</h4><div class="g-bar"><button class="btn-sec" onclick="gitWorkspace.nativeHistory()">${esc(t('mn_hist_load'))}</button></div><div id="mn-hist"></div>
      <h4>${esc(t('mn_cfg_title'))}</h4><p class="g-mut">${esc(t('mn_cfg_intro'))}</p><div class="g-bar"><button class="btn-sec" onclick="gitWorkspace.nativeConfig()">📝 ${esc(t('mn_cfg_open'))}</button></div>`;
    nativeProject();
    nativeResumeJob();
  }
  function mnProject() { const c = MN.cfg; const name = $('mn-project') && $('mn-project').value; return c && c.config.projects.find(p => p.name === name); }
  async function nativeProject() {
    const c = MN.cfg, p = mnProject(); if (!p) return;
    const view = c.projects.find(x => x.name === p.name) || {};
    $('mn-env').innerHTML = (view.environments || []).map(e => `<option value="${esc(e)}">${esc(e)}</option>`).join('');
    nativeBuild(view);
    const loc = view.build ? (view.build_location || '') : '';
    const hostOK = !!(c.host_build && c.host_build.supported);
    const ready = !view.build || loc === 'none' || (loc === 'host' && hostOK) || (loc === 'server' && view.build_conn_id);
    $('mn-note').textContent = view.build
      ? (!loc ? t('mn_build_note') : loc === 'none' ? t('mn_build_none_note') : t('mn_build_ready_note'))
      : (view.source ? mnFmt('mn_source_note', view.source) : t('mn_no_source_note'));
    $('mn-preview').disabled = !view.source || !ready;
    const eng = (c.engines || {})[p.name] || c.engine_default || 'docker';
    $('mn-engine').innerHTML = `<label>${esc(t('mn_engine'))} <select id="mn-engine-sel" onchange="gitWorkspace.nativeEngine(this.value)" ${c.can_deploy || eng === 'native' ? '' : 'disabled'}>${['docker', 'native'].map(e => `<option value="${e}" ${e === eng ? 'selected' : ''}>${esc(t('mn_engine_' + e))}</option>`).join('')}</select></label> <span class="g-mut">${esc(t(c.can_deploy ? 'mn_engine_hint' : 'mn_engine_no_policy'))}</span>`;
    nativeEnv();
    $('mn-refs').innerHTML = '';
    if (!view.source) return;
    try {
      const r = await fetch('/api/git/medium/native/refs?project=' + encodeURIComponent(p.name)); if (!r.ok) return;
      const v = await r.json(); if (mnProject() !== p) return;
      $('mn-refs').innerHTML = [...(v.branches || []), ...(v.tags || [])].map(x => `<option value="${esc(x)}">`).join('');
    } catch (_) {}
  }
  function nativeBuild(view, draftLoc) {
    const box = $('mn-build'); if (!box) return;
    if (!view || !view.build) { box.innerHTML = ''; return; }
    const c = MN.cfg, hb = c.host_build || {}, loc = draftLoc || view.build_location || '';
    const opt = (v, label, disabled) => `<option value="${v}" ${v === loc ? 'selected' : ''} ${disabled ? 'disabled' : ''}>${esc(label)}</option>`;
    let h = `<label>${esc(t('mn_build_where'))} <select id="mn-build-loc" onchange="gitWorkspace.nativeBuildDraft()">` +
      opt('', t('mn_build_choose')) +
      opt('host', t('mn_build_host') + (hb.platform ? ' (' + hb.platform + ')' : ''), !hb.supported && loc !== 'host') +
      opt('server', t('mn_build_server')) + opt('none', t('mn_build_none')) + `</select></label>`;
    if (loc === 'server') {
      const conns = c.connections || [];
      if (!conns.length) h += `<span class="g-st s-missing">${esc(t('mn_build_no_conn'))}</span>`;
      else h += `<label>${esc(t('mn_build_conn'))} <select id="mn-build-conn" onchange="gitWorkspace.nativeBuildSave()"><option value="0">${esc(t('mn_build_choose'))}</option>` +
        conns.map(x => `<option value="${x.id}" ${view.build_conn_id === x.id ? 'selected' : ''}>${esc(x.name)} (${esc(x.user + '@' + x.host)})</option>`).join('') + `</select></label>`;
    }
    if (loc === 'host' && !hb.supported) h += `<span class="g-st s-error">${esc(hb.reason || t('mn_build_unavailable'))}</span>`;
    h += ` <button class="btn-sec btn-sm" onclick="gitWorkspace.nativeBuildSave()">${esc(t('mn_build_save'))}</button>`;
    box.innerHTML = h;
  }
  function nativeBuildDraft() {
    const p = mnProject(); if (!p) return;
    const view = MN.cfg.projects.find(x => x.name === p.name) || {};
    const loc = $('mn-build-loc') ? $('mn-build-loc').value : '';
    nativeBuild(view, loc);
    if (loc !== 'server') nativeBuildSave();
  }
  async function nativeBuildSave() {
    const p = mnProject(); if (!p) return;
    const view = MN.cfg.projects.find(x => x.name === p.name) || {};
    const loc = $('mn-build-loc') ? $('mn-build-loc').value : (view.build_location || '');
    let conn = $('mn-build-conn') ? +$('mn-build-conn').value : 0;
    if (loc !== 'server') conn = 0;
    if (loc === 'server' && !conn) { showToast(t('mn_build_pick_conn'), 'error'); return; }
    try {
      const r = await fetch('/api/git/medium/native/build', json('POST', {project: p.name, location: loc, conn_id: conn}));
      if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json(), b = v.build || {};
      view.build_location = b.location || loc;
      view.build_conn_id = b.conn_id || 0;
      view.build_conn_name = b.conn_name || '';
      MN.run = null; const out = $('mn-result'); if (out) out.innerHTML = '';
      showToast(t('mn_build_saved'), 'success'); nativeProject();
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 8000); nativeProject(); }
  }
  function nativeEnv() {
    const p = mnProject(), env = $('mn-env') && $('mn-env').value;
    if ($('mn-hist')) $('mn-hist').innerHTML = '';
    const tg = p && (p.targets || []).find(x => x.name === env);
    $('mn-dests').innerHTML = tg ? tg.destinations.map(d => `<label class="mn-dest"><input type="checkbox" data-mn-dest="${esc(d.server)}" checked><span class="g-mono">${esc(d.server + ':' + d.path)}</span></label>`).join('') : '';
  }
  async function nativeLink(sel) {
    const r = await fetch('/api/git/medium/native/link', json('POST', {server: sel.dataset.mnServer, conn_id: +sel.value}));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    showToast(t('mn_linked'), 'success'); await nativeLoad();
  }
  async function nativeEngine(engine) {
    const p = mnProject(); if (!p) return;
    const r = await fetch('/api/git/medium/native/engine', json('POST', {project: p.name, engine}));
    if (!r.ok) { showToast(await apiError(r), 'error', 8000); nativeProject(); return; }
    MN.cfg.engines = Object.assign({}, MN.cfg.engines || {}); MN.cfg.engines[p.name] = engine;
    showToast(t('mn_engine_set'), 'success');
    if (MN.run && MN.run.result && MN.run.result.project === p.name) nativeRender(MN.run);
  }
  async function nativePlan(compare, excluded) {
    const p = mnProject(); if (!p) return;
    const view = MN.cfg.projects.find(x => x.name === p.name) || {};
    if (view.build && !view.build_location) { showToast(t('mn_build_needed'), 'error'); return; }
    if (view.build && view.build_location === 'server' && !view.build_conn_id) { showToast(t('mn_build_pick_conn'), 'error'); return; }
    const servers = [...document.querySelectorAll('[data-mn-dest]')].filter(x => x.checked).map(x => x.dataset.mnDest);
    const body = {project: p.name, environment: $('mn-env').value, ref: $('mn-ref').value.trim(), servers, compare, excluded: excluded || []};
    if (!body.ref || !servers.length) { showToast(t(!body.ref ? 'mn_ref' : 'mn_server') + '?', 'error'); return; }
    $('mn-preview').disabled = $('mn-compare').disabled = true;
    try {
      const r = await fetch('/api/git/medium/native/plan', json('POST', body)); if (!r.ok) throw new Error(await apiError(r));
      const {id} = await r.json(); nativePoll(id);
    } catch (e) { showToast(String(e), 'error', 8000); nativeProject(); $('mn-compare').disabled = !MN.cfg.running; }
  }
  async function nativePoll(id) {
    clearTimeout(MN.timer);
    const out = $('mn-result'); if (!out) return;
    try {
      const r = await fetch('/api/git/medium/native/plan/' + encodeURIComponent(id)); if (!r.ok) throw new Error(await apiError(r));
      const run = await r.json();
      if (!run.done) { out.innerHTML = `<p><span class="g-spin"></span> ${esc(t('mn_stage_' + run.stage) || run.stage)}</p>`; MN.timer = setTimeout(() => nativePoll(id), 1500); return; }
      run.id = id; MN.run = run; nativeRender(run);
    } catch (e) { out.textContent = String(e); }
    if ($('mn-compare')) { nativeProject(); $('mn-compare').disabled = !MN.cfg.running; }
  }
  function mnChanges(list, limit, pick, server) {
    const shown = list.slice(0, limit);
    const rows = shown.map(c => `<tr>${pick ? `<td><input type="checkbox" data-mn-ex="${esc(c.path)}" aria-label="${esc(t('mn_exclude') + ' ' + c.path)}" title="${esc(t('mn_exclude'))}"></td>` : ''}<td><span class="g-st s-e${c.action === 'deleted' ? 'removed' : c.action}">${esc(t('mn_' + c.action))}</span>${c.eol_only ? ` <span class="g-st s-pending" title="${esc(t('mn_eol_hint'))}">${esc(t('mn_eol'))}</span>` : ''}</td><td class="g-mono">${esc(c.path)}${c.link_target ? ' → ' + esc(c.link_target) : ''}</td><td class="g-mut">${esc(c.kind)}${c.server_kind && c.server_kind !== c.kind ? ' (' + esc(c.server_kind) + ')' : ''}</td>${server ? `<td>${c.kind === 'dir' || c.path.endsWith('/') ? '' : `<button class="btn-sec btn-sm" style="white-space:nowrap" data-mn-diff-s="${esc(server)}" data-mn-diff-p="${esc(c.path)}" onclick="gitWorkspace.nativeDiff(this)">${esc(t('mn_details'))}</button>`}</td>` : ''}</tr>`).join('');
    return `<div class="g-scroll"><table><tbody>${rows}</tbody></table></div>${list.length > limit ? `<p class="g-mut">${esc(mnFmt('mn_more', list.length - limit))}</p>` : ''}`;
  }
  function nativeRender(run) {
    const out = $('mn-result'); if (!out) return;
    if (!run.result) { out.innerHTML = `<p><span class="g-st s-error">${esc(run.error || 'error')}</span></p>`; return; }
    const r = run.result;
    const head = [`<b>${esc(r.project)}</b> · ${esc(r.environment)}`, `<span class="g-mono">${esc(r.ref)}</span> <span class="g-mono g-mut">${esc((r.commit || '').slice(0, 10))}</span>${r.version ? ' · ' + esc(r.version) : ''}`,
      `${esc(t('mn_src_' + r.source_from))}${r.source_name ? ' (' + esc(r.source_name) + ')' : ''} · ${r.files} ${esc(t('mn_files'))}`,
      !r.ref_allowed ? `<span class="g-st s-error">${esc(t('mn_not_allowed'))}</span>` : '', r.confirm ? `<span class="g-st s-pending">${esc(t('mn_confirm'))}</span>` : ''].filter(Boolean).join(' · ');
    let html = `<div class="g-card"><p>${head}</p>`;
    if (r.build) {
      let where = r.build_location === 'none' ? t('mn_build_none') : r.build_location === 'original' ? t('mn_build_original') : r.build_location === 'host' ? t('mn_build_host') : r.build_location === 'server' ? t('mn_build_server') + (r.build_server ? ' ' + r.build_server : '') : t('mn_build_choose');
      if (r.build_user) where += ' · ' + mnFmt('mn_build_as', r.build_user);
      if (r.build_platform) where += ' (' + r.build_platform + ')';
      html += `<p><b>${esc(t('mn_build_where'))}:</b> ${esc(where)} <span class="g-mono g-mut">${esc(r.build)}</span></p>`;
      if (r.build_log) html += `<details><summary>${esc(t('mn_build_log'))}</summary><pre class="gm-out">${esc(r.build_log)}</pre></details>`;
    }
    if (r.compared) html += `<p><span class="g-st ${r.same ? 's-ok' : 's-differs'}">${esc(t(r.same ? 'mn_same' : 'mn_differs'))}</span></p>`;
    if (r.warnings && r.warnings.length) html += `<ul>${r.warnings.map(w => `<li>${esc(w)}</li>`).join('')}</ul>`;
    if (r.source_check) {
      const sc = r.source_check;
      html += `<p><b>${esc(t('mn_source_check'))}:</b> ${sc.error ? `<span class="g-st s-error">${esc(sc.error)}</span>` : sc.same ? `<span class="g-st s-ok">${esc(t('mn_source_same'))}</span>` : [['missing', sc.missing], ['extra', sc.extra], ['differs', sc.differs]].filter(x => x[1].length).map(([k, l]) => `${esc(t('mn_source_' + k))}: <span class="g-mono">${esc(l.slice(0, 20).join(', '))}</span>`).join(' · ')}</p>`;
    }
    if (r.not_compared && r.not_compared.length) html += `<p class="g-mut">${esc(t('mn_not_compared'))}: ${esc(r.not_compared.join(', '))}</p>`;
    html += '</div>';
    for (const s of r.servers) {
      html += `<div class="g-card"><h4 class="g-mono">${esc(s.label)}</h4><p class="g-mut">${esc(t('mn_connection'))}: ${esc(s.connection || '-')}${s.platform ? ' · ' + esc(s.platform + '/' + s.arch) : ''}${s.post_deploy ? ` · ${esc(t('mn_post_deploy'))}: <span class="g-mono">${esc(s.post_deploy)}</span>` : ''}</p>`;
      if (s.error) html += `<p><span class="g-st s-error">${esc(s.error)}</span></p>`;
      else {
        html += `<p>${['new', 'changed', 'deleted'].map(a => `<span class="g-st s-e${a === 'deleted' ? 'removed' : a}">${s.counts[a]} ${esc(t('mn_' + a))}</span>`).join(' ')}${s.counts.eol ? ` <span class="g-mut" title="${esc(t('mn_eol_hint'))}">(${esc(mnFmt('mn_eol_n', s.counts.eol))})</span>` : ''}</p>`;
        if (!s.exists) html += `<p class="g-mut">${esc(t('mn_missing_dir'))}</p>`;
      }
      if (s.reference_error) html += `<p>${esc(t('mn_ref_error'))}: <span class="g-st s-error">${esc(s.reference_error)}</span></p>`;
      if (s.compare) {
        const cmp = s.compare;
        html += `<p><span class="g-st ${cmp.same ? 's-ok' : 's-differs'}">${esc(t(cmp.same ? 'mn_same' : 'mn_differs'))}</span> <span class="g-mut">${esc(mnFmt('mn_matched', cmp.matched, Math.max(s.reference_count, s.changes.length)))}</span></p>`;
        if (cmp.only_reference.length) html += `<p><b>${esc(t('mn_only_ref'))}</b></p>${mnChanges(cmp.only_reference, 100)}`;
        if (cmp.only_native.length) html += `<p><b>${esc(t('mn_only_native'))}</b></p>${mnChanges(cmp.only_native, 100)}`;
        if (cmp.kind_differs.length) html += `<p><b>${esc(t('mn_kind'))}:</b> <span class="g-mono">${esc(cmp.kind_differs.join(', '))}</span></p>`;
      }
      if (s.warnings && s.warnings.length) html += `<ul>${s.warnings.map(w => `<li>${esc(w)}</li>`).join('')}</ul>`;
      const st = s.state;
      if (st) {
        const cur = st.current;
        if (cur) html += `<p>${esc(t('mn_current'))}: <b class="g-mono">${esc(cur.version || cur.branch || '?')}</b> <span class="g-mut">${esc([cur.branch, (cur.commit || '').slice(0, 10), cur.deployed_at, cur.deployed_by].filter(Boolean).join(' · '))}</span></p>`;
        if (st.lock) html += `<p><span class="g-st s-pending">${esc(mnFmt('mn_lock', st.lock.user || '?', st.lock.since || '?'))}</span></p>`;
        if (st.history && st.history.length) html += `<details><summary>${esc(t('mn_history'))} (${st.history.length})</summary><div class="g-scroll"><table><tbody>${st.history.slice(0, 20).map(h => `<tr><td class="g-mono">${esc(h.deployed_at || '')}</td><td>${esc(h.action || 'DEPLOY')}</td><td class="g-mono">${esc(h.version || h.branch || '')}</td><td>${esc(h.deployed_by || '')}</td><td class="g-mut">${esc(h.server || '')}</td></tr>`).join('')}</tbody></table></div></details>`;
      }
      if (s.excluded_for && s.excluded_for.length) html += `<p class="g-mut">${esc(t('mn_excluded_on'))}: <span class="g-mono">${esc(s.excluded_for.join('  '))}</span></p>`;
      if (s.changes.length) html += `<details${s.changes.length <= 30 ? ' open' : ''}><summary>${esc(t('mn_changes'))} (${s.changes.length})</summary>${mnChanges(s.changes, 500, r.deployable, s.server)}</details>`;
      else if (!s.error) html += `<p class="g-mut">${esc(t('mn_no_changes'))}</p>`;
      html += `<details><summary>${esc(t('mn_excludes'))} (${s.excludes.length})</summary><p class="g-mono">${esc(s.excludes.join('  '))}</p></details></div>`;
    }
    html += mnDeployBar(r);
    out.innerHTML = html;
  }
  // The deploy bar under a finished comparison: leave changes out, dry run, deploy.
  function mnDeployBar(r) {
    if (!r.deployable) return `<p class="g-mut">${esc(t('mn_not_deployable'))}</p>`;
    const c = MN.cfg, native = ((c.engines || {})[r.project] || c.engine_default || 'docker') === 'native';
    const why = !native ? t('mn_deploy_needs_native') : !c.can_deploy ? t('mn_engine_no_policy') : !r.ref_allowed ? t('mn_not_allowed') : '';
    let h = `<div class="g-card" id="mn-deploy">`;
    if (r.excluded && r.excluded.length) h += `<p>${esc(t('mn_excluded_now'))}: <span class="g-mono">${esc(r.excluded.join('  '))}</span> <button class="btn-sec btn-sm" onclick="gitWorkspace.nativeReplan(true)">${esc(t('mn_clear_excluded'))}</button></p>`;
    h += `<div class="g-bar"><span class="g-mut">${esc(t('mn_exclude_hint'))}</span><button class="btn-sec" onclick="gitWorkspace.nativeReplan(false)">${esc(t('mn_replan'))}</button></div>`;
    if (r.confirm) h += `<div class="g-bar"><label>${esc(mnFmt('mn_confirm_label', r.environment))} <input id="mn-confirm" autocomplete="off" spellcheck="false"></label></div>`;
    h += `<div class="g-bar"><button class="btn-sec" id="mn-dry" onclick="gitWorkspace.nativeDeploy(true)" title="${esc(t('mn_dry_hint'))}">${esc(t('mn_dry_run'))}</button><button id="mn-go" onclick="gitWorkspace.nativeDeploy(false)" ${why ? `disabled title="${esc(why)}"` : ''}>🚀 ${esc(t('mn_deploy'))}</button>${why ? `<span class="g-mut">${esc(why)}</span>` : `<span class="g-mut">${esc(t('mn_dry_hint'))}</span>`}</div></div>`;
    return h;
  }
  function nativeReplan(clear) {
    const run = MN.run; if (!run || !run.result) return;
    const r = run.result;
    const ticked = [...document.querySelectorAll('[data-mn-ex]')].filter(x => x.checked).map(x => x.dataset.mnEx);
    const excluded = clear ? [] : [...new Set([...(r.excluded || []), ...ticked])];
    $('mn-ref').value = r.ref;
    nativePlan(!!r.compared, excluded);
  }
  async function nativeDeploy(dry) {
    const run = MN.run; if (!run || !run.result) return;
    const r = run.result, confirmText = $('mn-confirm') ? $('mn-confirm').value.trim() : '';
    if (!dry && !confirm(mnFmt('mn_deploy_confirm', r.ref, r.environment, r.servers.map(s => s.server).join(', ')))) return;
    const body = {run_id: run.id, revision: r.revision, confirm: confirmText, dry_run: dry};
    for (const b of ['mn-dry', 'mn-go']) if ($(b)) $(b).disabled = true;
    try {
      const resp = await fetch('/api/git/medium/native/deploy', json('POST', body)); if (!resp.ok) throw new Error(await apiError(resp));
      const {job_id} = await resp.json(); nativeJobStart(job_id);
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 10000); nativeRender(run); }
  }
  // ─── a running deployment or rollback: log and outcome per destination ───
  function nativeJobStart(id) { clearTimeout(MN.jobTimer); MN.job = {id, from: 0, lines: [], view: null}; nativeJobPoll(); }
  async function nativeResumeJob() {
    if (MN.job) { nativeJobPaint(); if (!(MN.job.view && MN.job.view.done)) nativeJobPoll(); return; }
    try {
      const r = await fetch('/api/git/medium/native/jobs'); if (!r.ok) return;
      const jobs = ((await r.json()).jobs || []).filter(j => !j.done);
      if (jobs.length) nativeJobStart(jobs[0].id);
    } catch (_) {}
  }
  async function nativeJobPoll() {
    clearTimeout(MN.jobTimer);
    const j = MN.job; if (!j || !$('mn-job')) return;
    try {
      const r = await fetch('/api/git/medium/native/jobs/' + encodeURIComponent(j.id) + '?from=' + j.from); if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json(); if (MN.job !== j) return;
      j.lines.push(...(v.lines || [])); j.from += (v.lines || []).length; j.view = v;
      nativeJobPaint();
      if (!v.done) { MN.jobTimer = setTimeout(nativeJobPoll, 1000); return; }
      if (v.kind === 'deploy' && v.dry_run && MN.run && MN.run.id === v.run_id) nativeRender(MN.run);
      if ($('mn-hist') && $('mn-hist').innerHTML) nativeHistory();
    } catch (e) { $('mn-job').innerHTML = `<p><span class="g-st s-error">${esc(String(e).replace(/^Error: /, ''))}</span></p>`; }
  }
  const mnLevel = {ok: 's-ok', warn: 's-pending', error: 's-error', cmd: 's-differs'};
  function nativeJobPaint() {
    const j = MN.job, box = $('mn-job'); if (!j || !j.view || !box) return;
    const v = j.view;
    const title = `${esc(t('mn_job_' + v.kind))}${v.dry_run ? ' (' + esc(t('mn_job_dry')) + ')' : ''} · <b>${esc(v.project)}</b> / ${esc(v.environment)}${v.ref ? ' · <span class="g-mono">' + esc(v.ref) + '</span>' : ''}${v.rolled_back ? ' · <span class="g-mono">' + esc(v.rolled_back) + '</span>' : ''} · <span class="g-mono g-mut">${esc(v.deploy_id)}</span>`;
    let h = `<div class="g-card"><p>${title}</p>`;
    if (!v.done) h += `<div class="g-bar"><span class="g-spin"></span> ${esc(mnFmt('mn_running', v.phase || '…'))} <button class="btn-sec btn-sm" onclick="gitWorkspace.nativeCancel()" title="${esc(t('mn_cancel_hint'))}">${esc(t('mn_cancel'))}</button></div>`;
    if (v.outcomes && v.outcomes.length) h += `<div class="g-scroll"><table><tbody>${v.outcomes.map(o => `<tr><td class="g-mono">${esc(o.label || o.server)}</td><td><span class="g-st ${o.status === 'ok' || o.status === 'dry-run' ? 's-ok' : o.status === 'skipped' ? 's-pending' : 's-error'}">${esc(t('mn_status_' + o.status) || o.status)}</span></td><td class="g-mut">${o.sent || o.deleted ? esc(mnFmt('mn_sent', o.sent, o.deleted)) : ''}</td><td>${esc(o.message || '')}</td></tr>`).join('')}</tbody></table></div>`;
    h += `<pre class="g-pre mn-log">${j.lines.map(l => `<span class="${l.level === 'out' ? 'g-mut' : ''}">${l.level === 'out' || l.level === 'info' ? '' : `<span class="g-st ${mnLevel[l.level] || ''}">${esc(l.level)}</span> `}${esc(l.text)}</span>`).join('\n')}</pre></div>`;
    const pre = box.querySelector('.mn-log'), stick = !pre || pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 8;
    box.innerHTML = h;
    const np = box.querySelector('.mn-log'); if (np && stick) np.scrollTop = np.scrollHeight;
  }
  async function nativeCancel() {
    const j = MN.job; if (!j) return;
    const r = await fetch('/api/git/medium/native/jobs/' + encodeURIComponent(j.id) + '/cancel', json('POST', {}));
    if (!r.ok) { showToast(await apiError(r), 'error'); return; }
    showToast(t('mn_cancel_sent'), 'success');
  }
  // ─── history, locks and rollback ───
  async function nativeHistory() {
    const p = mnProject(), env = $('mn-env') && $('mn-env').value, box = $('mn-hist'); if (!p || !env || !box) return;
    box.innerHTML = `<p><span class="g-spin"></span> ${esc(t('loading'))}</p>`;
    try {
      const r = await fetch('/api/git/medium/native/history?project=' + encodeURIComponent(p.name) + '&environment=' + encodeURIComponent(env)); if (!r.ok) throw new Error(await apiError(r));
      MN.hist = await r.json(); nativeHistoryPaint();
    } catch (e) { box.innerHTML = `<p><span class="g-st s-error">${esc(String(e).replace(/^Error: /, ''))}</span></p>`; }
  }
  function nativeHistoryPaint() {
    const hv = MN.hist, box = $('mn-hist'); if (!hv || !box) return;
    const native = hv.engine === 'native', groups = {};
    let h = '';
    hv.servers.forEach((s, i) => {
      h += `<div class="g-card"><h4 class="g-mono">${esc(s.label)}</h4>`;
      if (s.error) h += `<p><span class="g-st s-error">${esc(s.error)}</span></p>`;
      const cur = s.current;
      if (cur) h += `<p>${esc(t('mn_current'))}: <b class="g-mono">${esc(cur.version || cur.branch || '?')}</b> <span class="g-mut">${esc([cur.branch, (cur.commit || '').slice(0, 10), cur.deployed_at, cur.deployed_by].filter(Boolean).join(' · '))}</span></p>`;
      if (s.lock) {
        const age = s.lock.since ? (Date.now() - Date.parse(s.lock.since)) / 60000 : 0, stale = age >= hv.stale_lock_minutes;
        h += `<p><span class="g-st s-pending">${esc(mnFmt('mn_lock', s.lock.user || '?', s.lock.since || '?'))}</span> <span class="g-mut">${esc(mnFmt(stale ? 'mn_lock_stale' : 'mn_lock_young', hv.stale_lock_minutes))}</span>${hv.can_deploy ? ` <button class="btn-sec btn-sm" onclick="gitWorkspace.nativeLockRelease(${i})" ${stale ? '' : 'disabled'}>${esc(t('mn_lock_release'))}</button>` : ''}</p>`;
      }
      const rb = s.rollback;
      if (rb && rb.deploy_id) {
        h += `<p>${esc(mnFmt('mn_rb_target', rb.version || rb.deploy_id, rb.by || '?', rb.at || '?', rb.after === 'the state before the first deployment' ? t('mn_rb_first') : rb.after))}</p>`;
        (groups[rb.deploy_id] = groups[rb.deploy_id] || []).push(s.server);
      } else if (rb && !s.error) h += `<p class="g-mut">${esc(t('mn_rb_none'))}</p>`;
      if (s.entries.length) h += `<details><summary>${esc(t('mn_history'))} (${s.entries.length})</summary><div class="g-scroll"><table><thead><tr><th>${esc(t('mn_h_when'))}</th><th>${esc(t('mn_h_action'))}</th><th>${esc(t('mn_h_version'))}</th><th>${esc(t('mn_h_by'))}</th><th>${esc(t('mn_h_changes'))}</th><th>${esc(t('mn_h_note'))}</th></tr></thead><tbody>${s.entries.map(e => `<tr><td class="g-mono">${esc(e.deployed_at || '')}</td><td>${esc(e.action || 'DEPLOY')}${e.rolled_back ? ' <span class="g-mono g-mut">' + esc(e.rolled_back) + '</span>' : ''}</td><td class="g-mono">${esc(e.version || e.branch || '')}</td><td>${esc(e.deployed_by || '')}</td><td class="g-mut">${e.counts ? esc(['new', 'changed', 'deleted'].map(k => (e.counts[k] || 0) + ' ' + t('mn_' + k)).join(', ')) : ''}</td><td>${esc(e.note || '')}</td></tr>`).join('')}</tbody></table></div></details>`;
      else if (!s.error) h += `<p class="g-mut">${esc(t('mn_hist_none'))}</p>`;
      h += '</div>';
    });
    const ids = Object.keys(groups);
    if (ids.length) {
      const why = !native ? t('mn_deploy_needs_native') : !hv.can_deploy ? t('mn_engine_no_policy') : '';
      h += `<div class="g-card">${hv.confirm ? `<div class="g-bar"><label>${esc(mnFmt('mn_confirm_label', hv.environment))} <input id="mn-rb-confirm" autocomplete="off" spellcheck="false"></label></div>` : ''}`;
      ids.forEach((id, k) => { h += `<div class="g-bar"><span class="g-mono">${esc(id)}</span> <span class="g-mut">${esc(mnFmt('mn_rb_on', groups[id].join(', ')))}</span><button class="btn-sec" onclick="gitWorkspace.nativeRollback(${k}, true)">${esc(t('mn_rb_dry'))}</button><button onclick="gitWorkspace.nativeRollback(${k}, false)" ${why ? `disabled title="${esc(why)}"` : ''}>↩ ${esc(t('mn_rb_go'))}</button></div>`; });
      if (why) h += `<p class="g-mut">${esc(why)}</p>`;
      h += '</div>';
    }
    MN.rbGroups = groups; MN.rbIds = ids;
    box.innerHTML = h;
  }
  async function nativeRollback(k, dry) {
    const id = typeof k === 'number' ? (MN.rbIds || [])[k] : k;
    const hv = MN.hist, servers = (MN.rbGroups || {})[id]; if (!hv || !servers) return;
    if (!dry && !confirm(mnFmt('mn_rb_confirm', id, servers.join(', ')))) return;
    const body = {project: hv.project, environment: hv.environment, deploy_id: id, servers, confirm: $('mn-rb-confirm') ? $('mn-rb-confirm').value.trim() : '', dry_run: dry};
    try {
      const r = await fetch('/api/git/medium/native/rollback', json('POST', body)); if (!r.ok) throw new Error(await apiError(r));
      const {job_id} = await r.json(); nativeJobStart(job_id);
      $('mn-job').scrollIntoView({block: 'nearest'});
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 10000); }
  }
  // File details: the server's version against the branch, for one change of the plan.
  async function nativeDiff(btn) {
    const run = MN.run; if (!run || !run.id) return;
    const server = btn.dataset.mnDiffS, path = btn.dataset.mnDiffP;
    modal(`<h3>${esc(t('mn_diff_title'))}: <span class="g-mono">${esc(path)}</span></h3><div class="hint-box">⏳</div>`);
    let v;
    try {
      const r = await fetch('/api/git/medium/native/plan/' + encodeURIComponent(run.id) + '/diff?server=' + encodeURIComponent(server) + '&path=' + encodeURIComponent(path));
      if (!r.ok) throw new Error(await apiError(r));
      v = await r.json();
    } catch (e) { modal(`<h3>${esc(t('mn_diff_title'))}: <span class="g-mono">${esc(path)}</span></h3><div class="hint-box bad">${esc(String(e).replace(/^Error: /, ''))}</div><div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('close'))}</button></div>`); return; }
    const side = (x, label) => x.state === 'symlink' ? `${label}: ${esc(mnFmt('mn_diff_symlink', x.link || ''))}` : x.state === 'dir' ? `${label}: ${esc(t('mn_diff_dir'))}` : '';
    const notes = [side(v.server_side, esc(t('mn_diff_server'))), side(v.source_side, esc(t('mn_diff_source')))].filter(Boolean);
    if (v.server_side.state === 'missing') notes.push(esc(t('mn_diff_missing_srv')));
    if (v.source_side.state === 'missing') notes.push(esc(t('mn_diff_missing_src')));
    if (v.eol_only) notes.push(esc(t('mn_diff_eol')));
    else if (v.server_side.state === 'text' && v.source_side.state === 'text' && v.server_side.crlf !== v.source_side.crlf) notes.push(esc(mnFmt('mn_diff_crlf', v.server_side.crlf ? 'CRLF' : 'LF', v.source_side.crlf ? 'CRLF' : 'LF')));
    if (v.truncated) notes.push(esc(t('mn_diff_trunc')));
    let body;
    if ([v.server_side.state, v.source_side.state].includes('binary')) body = `<div class="hint-box">${esc(t('mn_diff_binary'))}</div>`;
    else if ([v.server_side.state, v.source_side.state].includes('too_large')) body = `<div class="hint-box">${esc(t('mn_diff_large'))}</div>`;
    else if (!v.lines.length) body = !v.eol_only && (v.server_side.state === 'text' || v.source_side.state === 'text') ? `<div class="hint-box">${esc(t('mn_diff_none'))}</div>` : '';
    else body = '<div class="g-diff">' + v.lines.map(l => l.op === '@' ? `<div class="d-hunk">@@ -${l.a} +${l.b} @@</div>` : `<div class="${l.op === '+' ? 'd-add' : l.op === '-' ? 'd-del' : ''}">${esc(l.op + ' ' + l.s)}</div>`).join('') + '</div>';
    modal(`<h3>${esc(t('mn_diff_title'))}: <span class="g-mono">${esc(v.path)}</span></h3>
      <p class="g-mut">${esc(v.server)} · <span class="g-st s-e${v.action === 'deleted' ? 'removed' : v.action}">${esc(t('mn_' + v.action))}</span> · ${esc(mnFmt('mn_diff_counts', v.added, v.removed))} · <span class="d-del">−</span> ${esc(t('mn_diff_server'))}, <span class="d-add">+</span> ${esc(t('mn_diff_source'))}</p>
      ${notes.length ? `<div class="hint-box">${notes.join('<br>')}</div>` : ''}${body}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('close'))}</button></div>`);
  }

  // deploy-config editor: raw YAML, saved only when the whole configuration loads.
  const MC = {files: [], name: '', hash: '', text: '', crlf: false, canEdit: false, isNew: false};
  const mcDirty = () => { const ta = $('mc-text'); return !!ta && ta.value !== MC.text; };
  async function nativeConfig(name) {
    try {
      const r = await fetch('/api/git/medium/native/config/files'); if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json(); MC.files = v.files || []; MC.canEdit = !!v.can_edit; MC.valid = v.valid; MC.error = v.error || '';
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 8000); return; }
    try { const g = await fetch('/api/git/medium/native/config/git'); MC.git = g.ok ? await g.json() : null; } catch (_) { MC.git = null; }
    const pick = name && MC.files.some(f => f.name === name) ? name : (MC.files.find(f => f.name === MC.name) ? MC.name : (MC.files[0] || {}).name);
    if (!pick) { MC.name = ''; MC.text = ''; MC.hash = ''; mcPaint(); return; }
    await mcOpen(pick, true);
  }
  async function mcOpen(name, force) {
    if (!force && mcDirty() && !confirm(t('mn_cfg_discard'))) { if ($('mc-file')) $('mc-file').value = MC.name; return; }
    try {
      const r = await fetch('/api/git/medium/native/config/file?name=' + encodeURIComponent(name)); if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json(); MC.name = v.name; MC.hash = v.hash; MC.text = (v.text || '').replace(/\r\n/g, '\n'); MC.crlf = !!v.crlf; MC.isNew = false;
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 8000); return; }
    mcPaint();
  }
  function mcPaint(msg) {
    const opts = MC.files.map(f => `<option value="${esc(f.name)}" ${f.name === MC.name ? 'selected' : ''}>${esc(f.name)}</option>`).join('') + (MC.isNew ? `<option value="${esc(MC.name)}" selected>${esc(MC.name)}</option>` : '');
    const ro = !MC.canEdit;
    modal(`<h3>📝 ${esc(t('mn_cfg_title'))}</h3><p class="g-mut">${esc(t('mn_cfg_intro'))}</p>
      ${MC.valid === false ? `<div class="hint-box bad">${esc(t('mn_cfg_invalid_now'))} ${esc(MC.error)}</div>` : ''}${ro ? `<div class="hint-box warn">${esc(t('mn_cfg_read_only'))}</div>` : ''}
      ${mcGitHTML()}
      <div class="g-bar"><label>${esc(t('mn_cfg_file'))} <select id="mc-file" onchange="gitWorkspace.mcOpen(this.value)">${opts}</select></label>${ro ? '' : `<button class="btn-sec btn-sm" onclick="gitWorkspace.mcNew()">＋ ${esc(t('mn_cfg_new'))}</button>`}</div>
      ${MC.crlf ? `<p class="g-mut">${esc(t('mn_cfg_crlf'))}</p>` : ''}${MC.isNew ? `<p class="g-mut">${esc(t('mn_cfg_new_note'))}</p>` : ''}
      <textarea id="mc-text" class="g-mono" spellcheck="false" autocomplete="off" aria-label="${esc(MC.name)}" style="min-height:50vh;font-size:12px;white-space:pre;tab-size:2" ${ro ? 'readonly' : ''}>${esc(MC.text)}</textarea>
      <div id="mc-msg" role="status" aria-live="polite">${msg || ''}</div>
      <div class="g-btns">${ro ? '' : `<button onclick="gitWorkspace.mcSave(false)">${esc(t('mn_cfg_save'))}</button><button class="btn-sec" onclick="gitWorkspace.mcSave(true)">${esc(t('mn_cfg_check'))}</button>${MC.name.startsWith('projects/') && !MC.isNew ? `<button class="btn-danger" onclick="gitWorkspace.mcDelete()">${esc(t('mn_cfg_delete'))}</button>` : ''}`}<button class="btn-sec" onclick="gitWorkspace.mcClose()">${esc(t('close'))}</button></div>`);
    const ta = $('mc-text');
    if (ta) ta.addEventListener('keydown', e => { if (e.key === 'Tab' && !e.shiftKey && !ta.readOnly) { e.preventDefault(); ta.setRangeText('  ', ta.selectionStart, ta.selectionEnd, 'end'); } });
  }
  // WRM's Git copy of deploy-config: pulled from DEPLOY_CONFIG_REPO through a WRM Git source,
  // edited here, committed and pushed back with the source's write token
  function mcGitHTML() {
    const g = MC.git; if (!g) return '';
    const short = c => esc((c || '').slice(0, 10));
    if (!g.available) return `<details class="g-card" id="mc-git"><summary>🔀 ${esc(t('mc_git_title'))}: ${esc(t('mc_git_off'))}</summary><p class="g-mut">${esc(g.error || '')}</p><p class="g-mut">${esc(t('mc_git_off_hint'))}</p></details>`;
    const ro = !MC.canEdit;
    let h = `<div class="g-card" id="mc-git"><b>🔀 ${esc(t('mc_git_title'))}</b> <span class="g-mono g-mut">${esc(g.repo)}</span> · ${esc(mnFmt('mc_git_source', g.source))}`;
    if (!g.active) {
      h += `<p class="g-mut">${esc(mnFmt('mc_git_inactive', g.branch || '?'))}${g.head ? ' <span class="g-mono">' + short(g.head) + '</span>' : ''}</p>`;
      if (g.head_error) h += `<div class="hint-box bad">${esc(g.head_error)}</div>`;
      if (!ro) h += `<div class="g-bar"><button class="btn-sm" onclick="gitWorkspace.mcGitPull(false)">⇩ ${esc(t('mc_git_use'))}</button></div>`;
      return h + '</div>';
    }
    const ch = g.changes || [];
    h += `<p>${esc(mnFmt('mc_git_active', g.branch))} <span class="g-mono">${short(g.commit)}</span> · ${esc(mnFmt('mc_git_pulled', typeof fmtTime === 'function' ? fmtTime(g.pulled_at) : g.pulled_at))}`
      + (g.behind ? ` <span class="g-st s-pending">${esc(mnFmt('mc_git_behind', short(g.head)))}</span>` : g.head ? ` <span class="g-st s-ok">${esc(t('mc_git_current'))}</span>` : '') + '</p>';
    if (g.head_error) h += `<div class="hint-box warn">${esc(g.head_error)}</div>`;
    if (g.error) h += `<div class="hint-box bad">${esc(g.error)}</div>`;
    h += ch.length ? `<p><b>${esc(mnFmt('mc_git_changes', ch.length))}</b> ${ch.map(c => `<span class="g-st ${c.action === 'delete' ? 's-error' : c.action === 'create' ? 's-ok' : 's-pending'}">${esc(t('mc_git_a_' + c.action))}</span> <span class="g-mono">${esc(c.name)}</span>`).join(' · ')}</p>` : `<p class="g-mut">${esc(t('mc_git_clean'))}</p>`;
    if (ro) return h + '</div>';
    if (ch.length && g.can_push) h += `<div class="g-bar"><label style="flex:1 1 240px;display:flex;gap:6px;align-items:center">${esc(t('mc_git_message'))} <input id="mc-git-msg" style="flex:1;min-width:0" maxlength="5000" placeholder="${esc(t('mc_git_message_ph'))}" autocomplete="off"></label><button class="btn-sm" onclick="gitWorkspace.mcGitPush()">⇧ ${esc(t('mc_git_push'))}</button></div><p class="g-mut">${esc(mnFmt('mc_git_push_hint', g.author || ''))}</p>`;
    else if (ch.length) h += `<p class="g-mut">${esc(t('mc_git_no_write'))}</p>`;
    h += `<div class="g-bar">${ch.length ? `<button class="btn-danger btn-sm" onclick="gitWorkspace.mcGitPull(true)">${esc(t('mc_git_discard_pull'))}</button>` : `<button class="btn-sec btn-sm" onclick="gitWorkspace.mcGitPull(false)">⇩ ${esc(t('mc_git_pull'))}</button>`}<button class="btn-sec btn-sm" onclick="gitWorkspace.mcGitStop()">${esc(t('mc_git_stop'))}</button></div>`;
    return h + '</div>';
  }
  async function mcGitAfter(r) {
    if (!r.ok) throw new Error(await apiError(r));
    const v = await r.json();
    MC.name = ''; await nativeConfig(); MN.run = null; nativeLoad();
    if (v.error) showToast(v.error, 'error', 12000);
    return v;
  }
  async function mcGitPull(discard) {
    if (mcDirty() && !confirm(t('mn_cfg_discard'))) return;
    if (discard && !confirm(mnFmt('mc_git_discard_confirm', ((MC.git || {}).changes || []).map(c => c.name).join(', ')))) return;
    try { await mcGitAfter(await fetch('/api/git/medium/native/config/git/pull', json('POST', {discard: !!discard}))); showToast(t('mc_git_pulled_ok'), 'success'); }
    catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 12000); }
  }
  async function mcGitPush() {
    if (mcDirty()) { showToast(t('mc_git_save_first'), 'error', 8000); return; }
    const m = ($('mc-git-msg') || {}).value || '';
    if (!confirm(mnFmt('mc_git_push_confirm', (MC.git || {}).branch || ''))) return;
    try { await mcGitAfter(await fetch('/api/git/medium/native/config/git/push', json('POST', {message: m.trim()}))); showToast(t('mc_git_pushed_ok'), 'success'); }
    catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 12000); }
  }
  async function mcGitStop() {
    const ch = ((MC.git || {}).changes || []).length;
    if (!confirm(t(ch ? 'mc_git_stop_confirm_changes' : 'mc_git_stop_confirm'))) return;
    try { await mcGitAfter(await fetch('/api/git/medium/native/config/git/stop', json('POST', {}))); showToast(t('mc_git_stopped'), 'success'); }
    catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 12000); }
  }
  function mcNew() {
    if (mcDirty() && !confirm(t('mn_cfg_discard'))) return;
    const n = (prompt(t('mn_cfg_new_name')) || '').trim().replace(/\.ya?ml$/i, '');
    if (!n) return;
    if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$/.test(n)) { showToast(t('mn_cfg_new_name'), 'error', 6000); return; }
    MC.name = 'projects/' + n + '.yaml'; MC.hash = ''; MC.crlf = false; MC.isNew = true;
    MC.text = `name: ${n}\nrepo: \ntargets:\n  dev: server:/path/${n}\n`;
    mcPaint();
  }
  async function mcSave(check) {
    const ta = $('mc-text'); if (!ta) return;
    const msg = $('mc-msg');
    try {
      const r = await fetch('/api/git/medium/native/config/files', json('POST', {name: MC.name, text: ta.value, base_hash: MC.hash, check_only: !!check}));
      if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json();
      if (!v.ok) { msg.innerHTML = `<div class="hint-box bad">${esc(t('mn_cfg_not_saved'))} ${esc(v.error)}</div>`; return; }
      const sum = v.summary || {}, ok = esc(mnFmt('mn_cfg_ok', sum.servers, sum.environments, sum.projects));
      if (check) { msg.innerHTML = `<div class="hint-box">${ok}</div>`; return; }
      MC.hash = v.hash; MC.text = ta.value; MC.isNew = false; MC.valid = true;
      showToast(t('mn_cfg_saved'), 'success');
      await nativeConfig(MC.name);
      if ($('mc-msg')) $('mc-msg').innerHTML = `<div class="hint-box">${esc(t('mn_cfg_saved'))} ${ok}</div>`;
      MN.run = null; nativeLoad();
    } catch (e) { msg.innerHTML = `<div class="hint-box bad">${esc(String(e).replace(/^Error: /, ''))}</div>`; }
  }
  async function mcDelete() {
    if (!confirm(mnFmt('mn_cfg_delete_confirm', MC.name))) return;
    try {
      const r = await fetch('/api/git/medium/native/config/files', json('POST', {name: MC.name, delete: true, base_hash: MC.hash}));
      if (!r.ok) throw new Error(await apiError(r));
      const v = await r.json();
      if (!v.ok) { $('mc-msg').innerHTML = `<div class="hint-box bad">${esc(t('mn_cfg_not_saved'))} ${esc(v.error)}</div>`; return; }
      showToast(t('mn_cfg_deleted'), 'success'); MC.name = ''; MC.text = '';
      await nativeConfig(); MN.run = null; nativeLoad();
    } catch (e) { $('mc-msg').innerHTML = `<div class="hint-box bad">${esc(String(e).replace(/^Error: /, ''))}</div>`; }
  }
  function mcClose() { if (mcDirty() && !confirm(t('mn_cfg_discard'))) return; closeModal(); }
  async function nativeLockRelease(i) {
    const hv = MN.hist, s = hv && hv.servers[i]; if (!s || !s.lock) return;
    if (!confirm(mnFmt('mn_lock_release_confirm', s.lock.user || '?', s.label))) return;
    const r = await fetch('/api/git/medium/native/lock/release', json('POST', {project: hv.project, environment: hv.environment, server: s.server, deploy_id: s.lock.deploy_id || ''}));
    if (!r.ok) { showToast(await apiError(r), 'error', 10000); return; }
    showToast(t('mn_lock_released'), 'success'); nativeHistory();
  }
  async function mediumQuickSetup(){
    const data=new FormData();for(const kind of ['tool','config','ssh']){const f=$('medium-quick-'+kind)?.files?.[0];if(f){if(f.size>20*1024*1024){showToast(t('gm_size'),'error');return}data.append(kind,f)}}
    const button=$('medium-quick-go');button.disabled=true;
    try{const r=await fetch('/api/git/medium/setup?confirm=1',{method:'POST',body:data});if(!r.ok)throw new Error(await apiError(r));for(const kind of ['tool','config','ssh'])$('medium-quick-'+kind).value='';await mediumRefresh();mediumActivity(false)}catch(e){showToast(String(e).replace(/^Error: /,''),'error',12000);button.disabled=false;mediumActivity(false)}
  }
  async function mediumSettings() {
    const r=await fetch('/api/git/medium/settings'); if (!r.ok) {showToast(await apiError(r),'error');return;}
    const values=await r.json();
    const keys=['DEPLOY_CONFIG_REPO','DEPLOY_CONFIG_PULL','DEPLOY_CONFIG_CHECK_MINUTES','DEPLOY_USER_EMAIL','DEPLOY_HOSTNAME','DEPLOY_SSH_OPTIONS','DEPLOY_TIMEOUT','DEPLOY_BUILD_USER','GIT_SSH_COMMAND'];
    modal(`<h3>${esc(t('gm_settings'))}</h3><p>${esc(t('gm_settings_hint'))}</p><div class="g-grid">${keys.map(k=>`<div><label>${esc(k)}</label><input data-medium-key="${k}" value="${esc(values[k] || '')}" autocomplete="off"></div>`).join('')}</div><div class="g-bar"><button class="btn-sec" onclick="gitWorkspace.mediumSaveSettings()">${esc(t('save'))}</button><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`);
  }
  async function mediumSaveSettings() {
    const values=Object.fromEntries([...document.querySelectorAll('[data-medium-key]')].map(e=>[e.dataset.mediumKey,e.value]));
    const r=await fetch('/api/git/medium/settings',json('POST',values)); if (!r.ok) {showToast(await apiError(r),'error');return;} closeModal();showToast(t('saved'),'success');
  }

  async function mediumUpload(kind, input) {
    const file = input.files && input.files[0]; if (!file) return;
    if (!confirm(t('gm_trust'))) { input.value = ''; return; }
    if (file.size > 20 * 1024 * 1024) { showToast(t('gm_size'), 'error'); input.value = ''; return; }
    const data = new FormData(); data.append('file', file);
    try {
      const r = await fetch('/api/git/medium/' + kind + '?confirm=1', {method:'POST',body:data}); if (!r.ok) throw new Error(await apiError(r));
      // 202: kept and checked by WRM; the original tool checks and applies it at the next start
      if (r.status === 202 && kind !== 'tool' && !(kind === 'config' && (MS.s || {}).default_engine === 'native')) showToast(t('gm_kept_' + kind), 'success', 9000);
      else showToast(t('gm_imported_' + kind), 'success');
      await mediumRefresh(); mediumActivity(false);
      if (kind === 'config' && $('mn-card') && $('mn-card').open) nativeLoad();
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 12000); mediumActivity(false); }
    input.value = '';
  }
  async function mediumAction(action) {
    if (action === 'stop' && !confirm(t('gm_stop_confirm'))) return;
    try {
      let r = await fetch('/api/git/medium/' + action, json('POST', {}));
      // the tool's state cannot be read (hung or crashed) or it reports work: stopping anyway needs a second confirmation
      if (!r.ok && action === 'stop' && r.status === 409) {
        const why = await apiError(r);
        if (!confirm(why + '\n\n' + t('gm_force_stop'))) return;
        r = await fetch('/api/git/medium/stop?force=1', json('POST', {}));
      }
      if (!r.ok) throw new Error(await apiError(r));
      await mediumRefresh(); mediumActivity(false);
    } catch (e) { showToast(String(e).replace(/^Error: /, ''), 'error', 12000); mediumRefresh(); }
  }

  function paintSimple() {
    const body = $('git-body');
    if (!body || G.mode !== 'simple') return;
    if (!E.ov && !S.loading) { S.loading = true; envLoad().finally(() => { S.loading = false; }); }
    body.innerHTML = simpleHTML();
    if (E.ov) { simpleApplyFilters(); simpleCells(); }
  }

  function simpleRefresh() { E.cells = {}; E.ov = null; S.pending.clear(); paintSimple(); }

  function simpleHTML() {
    const ov = E.ov;
    let html = `<div class="gs-wrap"><div class="hint-box">${esc(t('gs_intro'))}</div>
      <div class="g-bar" style="margin-bottom:10px"><button class="btn-sec btn-sm" onclick="gitWorkspace.simpleRefresh()">↻ ${esc(t('gs_refresh'))}</button><button class="btn-sec btn-sm" onclick="gitWorkspace.showFunctionality()">${esc(t('git_tab_functionality'))}</button>${vmdBox()}</div>`;
    if (!ov) return html + `<div class="hint-box">⏳</div></div>`;
    if (ov.services.length) html += simpleFilterHTML();
    const cards = ov.services.flatMap(s => s.envs.map(e => `<div class="g-card gs-card" id="${sKey(s.app, e.name)}">${simpleCardHTML(s.app, e)}</div>`));
    if ((ov.auto || 0) > 0) html += `<div class="hint-box"><div>${esc(tf('gs_auto_n', {n: ov.auto}))} ${esc(t('gs_auto_h'))}</div>${can('update') ? `<div class="g-btns" style="margin-top:6px"><button class="btn-sec btn-sm" onclick="gitWorkspace.simpleAdopt()">${esc(t('gs_auto_save'))}</button></div>` : ''}</div>`;
    const empty = `<div class="hint-box">${esc(t('gs_none'))}${(G.state || {}).can_check ? `<div class="g-btns" style="margin-top:6px"><button class="btn-sec btn-sm" onclick="gitWorkspace.check()">${esc(t('gs_find'))}</button></div>` : ''}</div>`;
    return html + (cards.join('') || empty) + (cards.length ? `<div id="gs-filter-empty" class="hint-box" style="display:none">${esc(t('gs_filter_none'))}</div>` : '') + '</div>';
  }

  function simpleFilterHTML() {
    const f = S.filters, services = [...new Set(E.ov.services.map(s => s.app))].sort();
    const envs = [...new Set(E.ov.services.flatMap(s => s.envs.map(e => e.name)))].sort();
    const servers = [...new Set(E.ov.services.flatMap(s => s.envs.flatMap(e => e.destinations.map(d => d.server))))].sort();
    const opt = (value, label, current) => `<option value="${esc(value)}" ${value === current ? 'selected' : ''}>${esc(label)}</option>`;
    const statuses = ['ok', 'update', 'review', 'missing', 'unknown', 'error'];
    return `<div class="g-bar gs-filters" aria-label="${esc(t('gs_intro'))}">
      <select aria-label="${esc(t('gs_filter_service'))}" onchange="gitWorkspace.simpleFilter('app',this.value)">${opt('', t('gs_filter_service'), f.app)}${services.map(x => opt(x, x, f.app)).join('')}</select>
      <select id="gs-server-filter" aria-label="${esc(t('gs_filter_server'))}" onchange="gitWorkspace.simpleFilter('server',this.value)">${opt('', t('gs_filter_server'), f.server)}${servers.map(x => opt(x, hn(x), f.server)).join('')}</select>
      ${dirSelect("gitWorkspace.simpleFilter('dir',this.value)", E.ov.services.flatMap(s => s.envs.flatMap(e => e.destinations.map(d => d.path))), f.dir, true)}
      <select aria-label="${esc(t('gs_filter_env'))}" onchange="gitWorkspace.simpleFilter('env',this.value)">${opt('', t('gs_filter_env'), f.env)}${envs.map(x => opt(x, x, f.env)).join('')}</select>
      <select aria-label="${esc(t('gs_filter_status'))}" onchange="gitWorkspace.simpleFilter('status',this.value)">${opt('', t('gs_filter_status'), f.status)}${statuses.map(x => opt(x, t('gs_status_' + x), f.status)).join('')}</select>
      <input type="search" aria-label="${esc(t('gs_filter_branch'))}" placeholder="${esc(t('gs_filter_branch'))}" value="${esc(f.branch)}" oninput="gitWorkspace.simpleFilter('branch',this.value)">
      <input type="search" aria-label="${esc(t('gs_filter_version'))}" placeholder="${esc(t('gs_filter_version'))}" value="${esc(f.version)}" oninput="gitWorkspace.simpleFilter('version',this.value)">
      <input type="search" aria-label="${esc(t('git_filter'))}" placeholder="${esc(t('git_filter'))}" value="${esc(f.q)}" oninput="gitWorkspace.simpleFilter('q',this.value)">
      <button class="btn-sec btn-sm" onclick="gitWorkspace.simpleClearFilters()">${esc(t('gs_filter_clear'))}</button></div>`;
  }

  function simpleFilter(k, v) { S.filters[k] = v || ''; simpleApplyFilters(); }
  function simpleClearFilters() {
    S.filters = {app: '', server: '', dir: '', env: '', status: '', branch: '', version: '', q: ''};
    document.querySelectorAll('#git-body .gs-filters select, #git-body .gs-filters input').forEach(el => { el.value = ''; });
    simpleApplyFilters();
  }
  function simpleApplyFilters() {
    const f = S.filters, q = f.q.toLowerCase(), branch = f.branch.toLowerCase(), version = f.version.toLowerCase();
    const serverFilter = $('gs-server-filter');
    if (serverFilter) [...serverFilter.options].forEach(o => { if (o.value) o.textContent = hn(o.value); });
    let visible = 0;
    for (const svc of (E.ov || {}).services || []) for (const e of svc.envs) {
      const el = $(sKey(svc.app, e.name)); if (!el) continue;
      const c = E.cells[envKey(svc.app, e.name)], d = c && c.data, tg = (d && d.target) || {};
      const dests = (d && d.dests) || e.destinations || [];
      const servers = dests.map(x => `${x.server || ''} ${hn(x.server || '', x.hostname || '')} ${x.path || ''}`);
      const versions = dests.flatMap(x => [x.current && x.current.version, x.version_md, (x.detected && x.detected.version), (x.detected && x.detected.commit), x.current && x.current.commit, x.commit_md]).filter(Boolean);
      const branches = [tg.branch, tg.ref_name, ...dests.map(x => (x.current && x.current.ref) || x.ref_md)].filter(Boolean);
      const status = c ? (c.error ? 'error' : d ? simpleStatus(d) : 'unknown') : 'unknown';
      const haystack = [svc.app, e.name, tg.version, tg.branch, tg.commit, ...servers, ...versions, ...branches].join(' ').toLowerCase();
      const show = (!f.app || svc.app === f.app) && (!f.env || e.name === f.env) &&
        (!f.server || dests.some(x => x.server === f.server)) && (!f.dir || dests.some(x => inDir(x.path, f.dir))) && (!f.status || status === f.status) &&
        (!branch || branches.join(' ').toLowerCase().includes(branch)) && (!version || versions.concat([tg.version, tg.commit]).filter(Boolean).join(' ').toLowerCase().includes(version)) &&
        (!q || haystack.includes(q));
      el.style.display = show ? '' : 'none'; if (show) visible++;
    }
    const empty = $('gs-filter-empty'); if (empty) empty.style.display = visible ? 'none' : '';
  }

  // stores the detected environments in the catalog (server side: permission check and audit)
  async function simpleAdopt() {
    if (!(await uiConfirm(t('gs_auto_q'), {okText: t('gs_auto_save')}))) return;
    const r = await fetch('/api/git/env/adopt', json('POST', {}));
    if (!r.ok) { showToast(await apiError(r), 'error', 8000); return; }
    const j = await r.json();
    showToast(tf('gs_auto_done', {n: j.environments || 0}), 'success');
    try { const s = await fetch('/api/git'); if (s.ok) G.state = await s.json(); } catch (_) {}
    simpleRefresh();
  }

  // up to date / update / mixed / missing / unknown / error, from the cell data of one environment
  function simpleStatus(d) {
    const tg = d.target || {};
    if (d.dests.some(x => x.error)) return 'error';
    if (d.dests.some(x => !x.exists)) return 'missing';
    if (d.dests.some(x => x.detected && ['mixed', 'modified', 'missing'].includes(x.detected.state))) return 'review';
    if (d.dests.some(x => x.detected && x.detected.state && x.detected.state !== 'exact')) return 'unknown';
    if (d.dests.some(x => x.behind > 0)) return 'update';
    if (d.dests.every(x => x.behind === 0)) return 'ok';
    if (!tg.commit || tg.error) return 'unknown';
    const commitOf = x => (x.detected && x.detected.state === 'exact') ? (x.detected.commit || '') : (x.current && x.current.commit) || x.commit_md || '';
    const same = x => { const c = String(commitOf(x)); return c.length >= 7 && (tg.commit.startsWith(c.slice(0, 7)) || c.startsWith(tg.commit.slice(0, 7))); };
    const known = d.dests.filter(x => commitOf(x));
    if (known.length !== d.dests.length) return 'unknown';
    return known.every(same) ? 'ok' : 'update';
  }

  function simpleCardHTML(app, e) {
    const c = E.cells[envKey(app, e.name)];
    const a = `'${esc(app)}','${esc(e.name)}'`;
    const head = badge => `<h3>${esc(app)} <span class="g-st s-ok" style="background:var(--bg4);color:var(--text2)">${esc(e.name)}</span>${badge || ''}</h3>`;
    if (!c) return head() + `<div class="g-mut"><span class="g-spin"></span> ${esc(tf('genv_n_dests', {n: e.destinations.length}))}</div>`;
    if (c.error) return head() + `<div class="hint-box bad">${esc(c.error)}</div>`;
    const d = c.data, tg = d.target || {}, st = simpleStatus(d);
    const cls = {ok: 'ok', update: 'update', review: 'review', missing: 'review', unknown: 'unknown', error: 'error'}[st];
    const badge = `<span class="g-st s-${cls} gs-big">${esc(t('gs_status_' + st))}</span>`;
    const dests = d.dests.map(x => {
      const cur = x.current, det = x.detected || {}, state = det.state;
      let ver = '', commit = '', stateBadge = '';
      if (state === 'exact') {
        ver = (det.approx ? '≈ ' : '') + (det.version || '');
        commit = sh10(det.commit);
      } else if (state) {
        stateBadge = `<span class="g-st s-${state === 'unknown' ? 'unknown' : 'review'}">${esc(detectedStateText(det))}</span>`;
      } else {
        ver = cur ? cur.version : x.version_md;
        commit = sh10(cur ? cur.commit : x.commit_md);
      }
      const vmd = G.checkVMD ? `<div class="g-ago">VERSION.md: ${esc(x.version_md || t('git_vmd_missing'))}${vmdConflict(det, {version: x.version_md, commit: x.commit_md}) ? ' <span class="g-st s-review">⚠</span>' : ''}</div>` : '';
      return `<div style="margin-bottom:6px"><b>${esc(hn(x.server))}</b> <span class="g-mono g-mut">${esc(x.path)}</span>` + (x.error ? `<div><span class="g-st s-error">${esc(x.error)}</span></div>`
        : !x.exists ? `<div class="g-mut">${esc(t('gs_not_installed'))}</div>`
        : `<div>${stateBadge}${ver ? `<span class="gs-ver g-mono">${esc(ver)}</span> ` : ''}<span class="g-mono g-mut">${esc(commit)}</span></div>${cur || x.updated_md ? `<div class="g-ago">${esc((cur ? cur.by || '' : x.user_md || '') + ' · ' + fmtTime(cur ? cur.at : x.updated_md))}</div>` : ''}${vmd}${x.code_error ? `<div class="g-mut">⚠ ${esc(tf('git_code_error', {e: x.code_error}))}</div>` : ''}`)
        + (x.lock ? `<div class="g-q">🔒 ${esc(tf('genv_locked_by', {user: x.lock.user || '?', from: x.lock.from || '?'}))}</div>` : '') + '</div>';
    }).join('');
    const branch = tg.version || tg.commit
      ? `<div><span class="gs-ver g-mono">${esc(tg.version || '—')}</span> <span class="g-mono g-mut">${esc(sh10(tg.commit))}</span></div><div class="g-ago">${esc([tg.branch, tg.commit_date].filter(Boolean).join(' · '))}</div>`
      : `<div class="g-mut">${esc(tg.error || t('gs_no_target'))}</div>`;
    const canGo = can('update') && d.dests.some(x => !x.error);
    const label = st === 'missing' ? 'install' : st === 'update' ? 'update' : st === 'ok' ? 'redeploy' : 'deploy';
    const anyExists = d.dests.some(x => x.exists);
    const checked = c.checkedAt ? `<div class="g-ago">${esc(t('gs_checked'))}: ${esc(fmtTime(new Date(c.checkedAt).toISOString()))}</div>` : '';
    return head(badge) + (checked ? `<div class="g-mut">${checked}</div>` : '') + (d.drift ? `<div class="hint-box warn">${esc(t('gs_differ'))}</div>` : '') +
      `<div class="gs-cols"><div class="gs-box"><h4>${esc(t('gs_on_server'))}</h4>${dests}</div><div class="gs-box"><h4>${esc(t('gs_on_branch'))}</h4>${branch}</div></div>
      <div class="gs-acts">${canGo ? `<button class="${st === 'ok' ? 'btn-sec' : 'gs-primary'}" onclick="gitWorkspace.simplePlan(${a},'${label}')">${esc(t('gs_' + label))}</button>` : ''}
        ${can('rollback') && anyExists ? `<button class="btn-sec" onclick="gitWorkspace.simpleRollback(${a})">↩ ${esc(t('gs_rollback'))}</button>` : ''}</div>`;
  }

  function simpleCells() {
    const cells = E.cells;
    const todo = ((E.ov || {}).services || []).flatMap(s => s.envs.map(e => [s.app, e.name])).filter(([app, env]) => !E.cells[envKey(app, env)] && !S.pending.has(envKey(app, env)));
    let i = 0;
    const next = async () => {
      if (i >= todo.length || E.cells !== cells || G.mode !== 'simple') return;
      const [app, env] = todo[i++], k = envKey(app, env);
      S.pending.add(k);
      try {
        const r = await fetch('/api/git/env/cell?' + envEnc(app, env));
        const result = r.ok ? {data: await r.json(), checkedAt: Date.now()} : {error: await apiError(r), checkedAt: Date.now()};
        if (E.cells !== cells) return;
        E.cells[k] = result;
        if (result.data) learnHosts(result.data.hosts);
      } catch (err) { if (E.cells !== cells) return; E.cells[k] = {error: String(err), checkedAt: Date.now()}; }
      S.pending.delete(k);
      const el = G.mode === 'simple' && $(sKey(app, env)), e = envOf(app, env);
      if (el && e) el.innerHTML = simpleCardHTML(app, e);
      if (G.mode === 'simple') simpleApplyFilters();
      return next();
    };
    for (let n = 0; n < 3; n++) next();
  }

  // install / update / deploy: the same plan and run endpoints as the advanced deploy dialog
  async function simplePlan(app, env, label, ref, preserve = false) {
    const same = preserve && S.ctx && S.ctx.app === app && S.ctx.env === env && S.plan && !S.plan.used;
    const previousExclusions = same ? new Set(S.excluded) : null;
    const previousDestinations = same ? new Set(S.destinations) : null;
    if (!same) S.opts = null;
    const ctx = S.ctx = {app, env, label};
    S.plan = null; S.submitting = false;
    const title = tf('gs_title_' + (label === 'redeploy' ? 'deploy' : label), {app, env});
    modal(`<h3>${esc(title)}</h3><div class="hint-box">⏳ ${esc(t('genv_comparing'))}</div>`);
    const pendingModal = $('git-modal'), active = () => S.ctx === ctx && $('git-modal') === pendingModal;
    try {
      const r = await fetch('/api/git/env/plan', json('POST', {app, env, ref: ref || ''}));
      if (!r.ok) { const error = await apiError(r); if (active()) modal(`<h3>${esc(title)}</h3><div class="hint-box bad">${esc(error)}</div>`); return; }
      const plan = await r.json();
      if (!active()) return;
      S.plan = plan;
      S.excluded = previousExclusions || new Set(Object.keys(plan.excluded || {}));
      const usable = plan.dests.filter(d => !(d.errors || []).length).map(d => d.index);
      S.destinations = previousDestinations ? new Set([...previousDestinations].filter(i => usable.includes(i))) : new Set(usable);
      paintSimplePlan();
    } catch (err) {
      if (active()) modal(`<h3>${esc(title)}</h3><div class="hint-box bad">${esc(String(err))}</div>`);
    }
  }

  function simpleRecompare() {
    if (S.submitting || !S.plan) return;
    const v = $('gs-ref').value.trim();
    S.opts = {post: !!($('gs-post') || {}).checked, restart: !!($('gs-restart') || {}).checked};
    return simplePlan(S.ctx.app, S.ctx.env, S.ctx.label, v, true);
  }

  function paintSimplePlan() {
    const p = S.plan, {app, env, label} = S.ctx, tg = p.target || {};
    const kind = label === 'redeploy' ? 'deploy' : label;
    const selectedDests = p.dests.filter(d => S.destinations.has(d.index));
    const blocked = (p.errors || []).length > 0 || selectedDests.length === 0 || selectedDests.some(d => (d.errors || []).length) || p.used;
    const fromV = [...new Set(p.dests.map(d => (d.current && d.current.version) || d.from_version).filter(Boolean))].join(', ') || '—';
    const o = S.opts || {};
    modal(`<h3>${esc(tf('gs_title_' + kind, {app, env}))}</h3>
      <div class="gs-from-to">${esc(tf('gs_from_to', {from: fromV, to: tg.version || p.ref_name || '—'}))} <span class="g-mono g-mut">${esc(sh10(tg.commit))}</span></div>
      ${(p.errors || []).map(x => `<div class="hint-box bad">${esc(x)}</div>`).join('')}
      <div id="gs-dest-count" class="g-mut">${esc(tf('gs_destinations_selected', {n: selectedDests.length, total: p.dests.length}))}</div>
      <div class="gs-destinations"><b>${esc(t('gs_deploy_destinations'))}</b>${p.dests.map(d => `<label class="g-opt"><input type="checkbox" data-index="${d.index}" ${S.destinations.has(d.index) ? 'checked' : ''} ${(d.errors || []).length ? 'disabled' : ''} onchange="gitWorkspace.simpleDestination(${d.index},this.checked)"><span>${esc(hn(d.server))} <span class="g-mono g-mut">${esc(d.path)}</span>${(d.errors || []).length ? ` · ${esc(t('gs_status_error'))}` : ''}</span></label>`).join('')}</div>
      ${p.dests.map(d => `<div class="g-item"><b>${esc(hn(d.server))}</b> <span class="g-mono g-mut">${esc(d.path)}</span>
        <div>${(d.errors || []).length ? '' : (d.counts || {}).new || changedOf(d) ? esc(tf('gs_files_n', {n: (d.counts || {}).new || 0, c: changedOf(d)})) : esc(t('gs_no_changes'))}</div>
        ${(d.errors || []).map(x => `<div class="hint-box bad">${esc(x)}</div>`).join('')}${(d.warnings || []).map(x => `<div class="hint-box warn">${esc(x)}</div>`).join('')}</div>`).join('')}
      ${simpleFilesHTML(p)}
      <div class="g-mut">${esc(t('gs_backup_h'))}</div>
      <details style="margin-top:8px"><summary class="g-mut">${esc(t('gs_other_version'))}</summary>
        <div class="g-bar" style="margin-top:6px"><input id="gs-ref" value="${esc(p.ref || '')}" placeholder="${esc(t('genv_ref_ph'))}" style="flex:1 1 200px"><button class="btn-sec btn-sm" onclick="gitWorkspace.simpleRecompare()">↻ ${esc(t('gs_compare'))}</button></div>
        <div class="g-mut">${esc(t('gs_other_h'))}</div></details>
      ${p.post_deploy || can('restart') ? `<div class="g-card"><h3>${esc(t('gs_options'))}</h3>
        ${p.post_deploy ? `<label class="g-opt"><input type="checkbox" id="gs-post" ${o.post ? 'checked' : ''}><span>${esc(t('genv_run_post'))} <code class="g-mono">${esc(p.post_deploy)}</code></span></label>` : ''}
        ${can('restart') ? `<label class="g-opt"><input type="checkbox" id="gs-restart" ${o.restart ? 'checked' : ''}><span>${esc(t('genv_restart'))}</span></label>` : ''}</div>` : ''}
      <div id="gs-blocked" class="hint-box bad" style="${blocked ? '' : 'display:none'}">${esc(t('gs_blocked'))}</div>
      ${p.confirm ? `<div class="g-conf"><label>${esc(tf('genv_type_env', {env}))}</label><input id="gs-conf" autocomplete="off"></div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button>
        <button id="gs-start" class="btn-danger gs-go" ${blocked || p.confirm ? 'disabled' : ''}>${esc(t('gs_start_' + kind))}</button></div>`);
    if (p.confirm) $('gs-conf').oninput = simpleUpdateStartButton;
    $('gs-start').onclick = () => simpleStart(kind);
  }
  function simpleFilesHTML(p) {
    const byPath = new Map();
    for (const d of p.dests || []) for (const f of d.files || []) {
      if (!ENV_ACT.includes(f.state)) continue;
      if (!byPath.has(f.path)) byPath.set(f.path, []);
      byPath.get(f.path).push({server: d.server, path: d.path, state: f.state});
    }
    const files = [...byPath.entries()].sort((a, b) => a[0].localeCompare(b[0]));
    if (!files.length) return '';
    const rows = files.map(([path, states]) => {
      const removedOnly = states.every(x => x.state === 'removed');
      return `<tr><td><input type="checkbox" data-path="${esc(path)}" ${removedOnly ? 'disabled' : S.excluded.has(path) ? '' : 'checked'} title="${esc(t(removedOnly ? 'gs_removed_h' : 'gs_include_file'))}" onchange="gitWorkspace.simpleExclude(this.dataset.path,this.checked)"></td>
        <td class="g-mono">${esc(path)}</td><td>${states.map(x => `${esc(hn(x.server))} <span class="g-mono g-mut">${esc(x.path)}</span>: ${esc(t('genv_fs_' + x.state))}${x.state === 'removed' ? ' · ' + esc(t('gs_removed_kept')) : ''}`).join('<br>')}${S.excluded.has(path) && (p.excluded || {})[path] ? `<div class="g-mut">${esc(tf('genv_excluded_by', {by: p.excluded[path].by, at: fmtTime(p.excluded[path].at)}))}</div>` : ''}</td></tr>`;
    }).join('');
    return `<details class="g-card" style="margin-top:8px"><summary><b>${esc(tf('gs_files_preview', {n: files.length}))}</b></summary><div class="g-mut" style="margin:6px 0">${esc(t('genv_include_h'))}</div>${files.some(([, states]) => states.some(x => x.state === 'removed')) ? `<div class="g-mut">${esc(t('gs_removed_h'))}</div>` : ''}<div class="g-files"><table class="gs-file-review"><tr><th></th><th>${esc(t('genv_files'))}</th><th>${esc(t('gs_file_status'))}</th></tr>${rows}</table></div><div id="gs-excluded-count" class="g-mut" style="margin-top:6px">${esc(tf('genv_n_excluded', {n: S.excluded.size}))}</div></details>`;
  }
  function simpleExclude(path, include) {
    if (!path || S.submitting) return;
    if (include) S.excluded.delete(path); else S.excluded.add(path);
    const count = $('gs-excluded-count'); if (count) count.textContent = tf('genv_n_excluded', {n: S.excluded.size});
  }
  function simpleDestination(index, selected) {
    index = Number(index);
    if (S.submitting || !S.plan || !S.plan.dests.some(d => d.index === index && !(d.errors || []).length)) return;
    if (selected) S.destinations.add(index); else S.destinations.delete(index);
    const p = S.plan, count = $('gs-dest-count');
    if (p && count) count.textContent = tf('gs_destinations_selected', {n: S.destinations.size, total: p.dests.length});
    simpleUpdateStartButton();
  }
  function simpleUpdateStartButton() {
    const start = $('gs-start'), p = S.plan;
    if (start && p) {
      const bad = (p.errors || []).length > 0 || p.used || S.destinations.size === 0 || p.dests.some(d => S.destinations.has(d.index) && (d.errors || []).length);
      const confirm = p.confirm && (!$('gs-conf') || $('gs-conf').value.trim().toLowerCase() !== S.ctx.env.toLowerCase());
      start.disabled = S.submitting || bad || !!confirm;
      const blocked = $('gs-blocked'); if (blocked) blocked.style.display = bad ? '' : 'none';
    }
  }
  const changedOf = d => ((d.counts || {}).changed || 0) + ((d.counts || {}).eol || 0);

  async function simpleStart(kind) {
    simpleUpdateStartButton();
    if (!$('gs-start') || $('gs-start').disabled) return;
    const p = S.plan, {app, env} = S.ctx;
    const ctx = S.ctx, review = $('git-modal');
    const opts = {post: !!($('gs-post') || {}).checked, restart: !!($('gs-restart') || {}).checked};
    S.opts = opts;
    const selected = p.dests.filter(d => S.destinations.has(d.index)).map(d => d.index);
    const body = {kind: 'update', restart: {mode: opts.restart ? 'now' : 'none'}, checks: {[app]: ((G.state.settings || {}).check_commands || {})[app] || ''},
      env: {app, env, plan_id: p.id, dests: selected, exclude: [...S.excluded], delete_removed: false, post_deploy: opts.post, dry_run: false, confirm: ($('gs-conf') || {}).value || ''}};
    S.submitting = true; simpleUpdateStartButton();
    try {
      if (!(await uiConfirm(tf('gs_confirm_q', {action: t('gs_' + (kind === 'deploy' ? 'deploy' : kind)), app, env, n: selected.length}), {danger: true, okText: t('gs_start_' + kind)}))) return;
      if (S.ctx !== ctx || S.plan !== p || $('git-modal') !== review) return;
      const r = await fetch('/api/git/runs', json('POST', body));
      if (!r.ok) { showToast(await apiError(r), 'error', 8000); return; }
      const id = (await r.json()).id;
      p.used = true;
      E.cells = {};
      if (S.ctx === ctx && $('git-modal') === review) { S.opts = null; showRun(id); }
    } catch (err) { showToast(String(err), 'error', 8000); }
    finally { if (S.ctx === ctx) { S.submitting = false; simpleUpdateStartButton(); } }
  }

  // rollback: the newest deployment that is still current on a server
  async function simpleRollback(app, env) {
    const ctx = S.ctx = {app, env};
    S.plan = null; S.submitting = false;
    const title = tf('gs_rb_title', {app, env});
    modal(`<h3>${esc(title)}</h3><div class="hint-box">⏳</div>`);
    const pendingModal = $('git-modal');
    let h;
    try {
      const r = await fetch('/api/git/env/history?' + envEnc(app, env));
      if (!r.ok) throw new Error(await apiError(r));
      h = await r.json();
    } catch (err) {
      if (S.ctx === ctx && $('git-modal') === pendingModal) modal(`<h3>${esc(title)}</h3><div class="hint-box bad">${esc(String(err))}</div>`);
      return;
    }
    if (S.ctx !== ctx || $('git-modal') !== pendingModal) return;
    const cur = new Set(h.dests.map(d => d.current).filter(Boolean));
    const x = h.entries.filter(y => y.action === 'DEPLOY' && cur.has(y.id)).sort((a, b) => String(b.at || '').localeCompare(String(a.at || '')))[0];
    const close = `<div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_close'))}</button></div>`;
    if (!x) { modal(`<h3>${esc(title)}</h3>${h.dests.filter(d => d.error).map(d => `<div class="hint-box bad">${esc(hn(d.server))}: ${esc(d.error)}</div>`).join('')}<div class="hint-box">${esc(t('gs_rb_none'))}</div>${close}`); return; }
    const dests = h.dests.map((d, i) => d.current === x.id ? i : -1).filter(i => i >= 0);
    S.rollbackDestinations = new Set(dests);
    const previous = i => {
      const row = h.entries.find(y => y.action === 'DEPLOY' && y.id === x.id && y.dest === i);
      return row && row.previous ? row.previous.version || t('gs_rb_prev_unknown') : t('gs_rb_prev_unknown');
    };
    const e = envOf(app, env) || {}, prev = [...new Set(dests.map(previous))].join(', ');
    modal(`<h3>${esc(title)}</h3>
      <div class="gs-cols"><div class="gs-box"><h4>${esc(t('gs_rb_now'))}</h4><div class="gs-ver g-mono">${esc(x.version || '—')}</div><div class="g-ago">${esc((x.by || '') + ' · ' + fmtTime(x.at))}</div></div>
        <div class="gs-box"><h4>${esc(t('gs_rb_to'))}</h4><div class="gs-ver g-mono">${esc(prev)}</div></div></div>
      <div class="g-mut" style="margin-top:6px">${esc(t('gs_rb_h'))}</div>
      <div class="g-mut" style="margin:8px 0">${esc(t('gs_select_rollback'))}</div><div class="gs-destinations">${dests.map(i => `<label class="g-opt"><input type="checkbox" data-index="${i}" checked onchange="gitWorkspace.simpleRollbackDestination(${i},this.checked)"><span>${esc(hn(h.dests[i].server))} <span class="g-mono g-mut">${esc(h.dests[i].path)}</span><div class="g-mut">${esc(tf('gs_from_to', {from: x.version || '—', to: previous(i)}))}</div></span></label>`).join('')}</div>
      ${e.post_deploy || can('restart') ? `<div class="g-card">${e.post_deploy ? `<label class="g-opt"><input type="checkbox" id="gs-rbpost"><span>${esc(t('genv_run_post'))} <code class="g-mono">${esc(e.post_deploy)}</code></span></label>` : ''}
        ${can('restart') ? `<label class="g-opt"><input type="checkbox" id="gs-rbrestart"><span>${esc(t('genv_restart'))}</span></label>` : ''}</div>` : ''}
      ${e.confirm ? `<div class="g-conf"><label>${esc(tf('genv_type_env', {env}))}</label><input id="gs-rbconf" autocomplete="off"></div>` : ''}
      <div class="g-btns"><button class="btn-sec" onclick="gitWorkspace.closeModal()">${esc(t('git_cancel'))}</button><button id="gs-rbgo" class="btn-danger gs-go" ${e.confirm || !dests.length ? 'disabled' : ''}>${esc(t('gs_rb_go'))}</button></div>`);
    if (e.confirm) $('gs-rbconf').oninput = simpleUpdateRollbackButton;
    $('gs-rbgo').onclick = async () => {
      simpleUpdateRollbackButton();
      if (!$('gs-rbgo') || $('gs-rbgo').disabled) return;
      const review = $('git-modal');
      const body = {kind: 'rollback', restart: {mode: ($('gs-rbrestart') || {}).checked ? 'now' : 'none'},
        env: {app, env, deploy_id: x.id, dests: dests.filter(i => S.rollbackDestinations.has(i)), post_deploy: !!($('gs-rbpost') || {}).checked, dry_run: false, confirm: ($('gs-rbconf') || {}).value || ''}};
      S.submitting = true; simpleUpdateRollbackButton();
      try {
        if (!(await uiConfirm(tf('gs_rb_q', {app, env, v: prev}), {danger: true, okText: t('gs_rb_go')}))) return;
        if (S.ctx !== ctx || $('git-modal') !== review) return;
        const r2 = await fetch('/api/git/runs', json('POST', body));
        if (!r2.ok) { showToast(await apiError(r2), 'error', 8000); return; }
        const id = (await r2.json()).id;
        E.cells = {};
        if (S.ctx === ctx && $('git-modal') === review) showRun(id);
      } catch (err) { showToast(String(err), 'error', 8000); }
      finally { if (S.ctx === ctx) { S.submitting = false; simpleUpdateRollbackButton(); } }
    };
  }
  function simpleRollbackDestination(index, selected) {
    index = Number(index);
    if (S.submitting || !Number.isInteger(index)) return;
    if (selected) S.rollbackDestinations.add(index); else S.rollbackDestinations.delete(index);
    simpleUpdateRollbackButton();
  }
  function simpleUpdateRollbackButton() {
    const go = $('gs-rbgo');
    if (!go) return;
    const needsConfirm = !!envOf(S.ctx.app, S.ctx.env)?.confirm;
    go.disabled = S.submitting || S.rollbackDestinations.size === 0 || (needsConfirm && (!$('gs-rbconf') || $('gs-rbconf').value.trim().toLowerCase() !== S.ctx.env.toLowerCase()));
  }

  // Dialog history: a dialog opened from another one keeps the previous dialog (its DOM, typed
  // values and handlers) so that "Back" returns to it. A new render of the same dialog (same
  // title) replaces it, and transient dialogs (loading, errors) are not kept.
  const modalTrail = [];
  const dlgTitle = d => { const h = d && d.querySelector('h3'); return h ? h.textContent.trim() : ''; };
  const dlgKeep = d => !!d.querySelector('input, select, textarea, table, pre, .g-item, .g-card, button:not(.btn-sec)');
  function modal(html) {
    const old = $('git-modal'), prev = old && old.querySelector('.g-dlg');
    clearTimeout(runTimer);
    const m = document.createElement('div');
    m.id = 'git-modal';
    m.innerHTML = `<div class="g-dlg">${html}</div>`;
    const dlg = m.firstElementChild;
    if (prev && dlgKeep(prev) && dlgTitle(prev) !== dlgTitle(dlg)) {
      prev.remove();
      modalTrail.push(prev);
      if (modalTrail.length > 20) modalTrail.shift();
    }
    if (old) old.remove();
    if (modalTrail.length) addBack(dlg);
    m.addEventListener('mousedown', e => { if (e.target === m) closeModal(); });
    m.addEventListener('keydown', e => {
      if (e.key === 'Escape') { e.stopPropagation(); closeModal(); }
      else if (e.altKey && e.key === 'ArrowLeft' && modalTrail.length) { e.preventDefault(); e.stopPropagation(); modalBack(); }
    });
    document.body.appendChild(m);
    const first = m.querySelector('input, select, textarea');
    if (first) first.focus();
  }
  // addBack puts a "Back" button first in the dialog's last button row (or above the dialog).
  function addBack(dlg) {
    if (dlg.querySelector('.g-back')) return;
    const b = document.createElement('button');
    b.type = 'button'; b.className = 'btn-sec g-back';
    b.textContent = '← ' + t('git_back');
    b.title = tf('git_back_to', {name: dlgTitle(modalTrail[modalTrail.length - 1]) || t('git_back')}) + ' (Alt+←)';
    b.onclick = modalBack;
    const rows = dlg.querySelectorAll('.g-btns');
    if (rows.length) rows[rows.length - 1].prepend(b);
    else { const bar = document.createElement('div'); bar.className = 'g-backbar'; bar.append(b); dlg.prepend(bar); }
  }
  function modalBack() {
    const prev = modalTrail.pop(), m = $('git-modal');
    if (!prev || !m) { closeModal(); return; }
    clearTimeout(runTimer);
    m.querySelector('.g-dlg').replaceWith(prev);
    const first = prev.querySelector('input, select, textarea, button');
    if (first) first.focus();
  }
  function closeModal() { const m = $('git-modal'); if (m) m.remove(); modalTrail.length = 0; clearTimeout(runTimer); }

  let qTimer = null;
  window.gitWorkspace = { monitoringMount, functionalityFilter, functionalityReset, functionalityToggle, functionalityExpand,
    open, close, configProfiles, configProfileCheck, configProfileDelete, configProfileNew, configProfileSave, configProfileFilter, mediumQuickSetup, mediumUpload, mediumAction, mediumDockerInstall, mediumActivity, mediumRow, mediumToggleLog, nativeLoad, nativeProject, nativeBuild, nativeBuildDraft, nativeBuildSave, nativeEnv, nativeLink, nativePlan, nativeEngine, nativeReplan, nativeDeploy, nativeCancel, nativeHistory, nativeRollback, nativeLockRelease, nativeDiff, nativeConfig, mcOpen, mcNew, mcSave, mcDelete, mcClose, mcGitPull, mcGitPush, mcGitStop, mediumRefresh, mediumSettings, mediumSaveSettings, showFunctionality, configuration, tab: setTab, setGitMode, setCheckVMD, simpleRefresh, simpleAdopt, simplePlan, simpleRecompare, simpleRollback, simpleFilter, simpleClearFilters, simpleExclude, simpleDestination, simpleRollbackDestination, check, refresh, details, diff, forget, sel, clearSel, setStale, checkIds, checkSel, checkVisible, checkStale, cancelCheck,
    viewFile, fileFilter, fileDir, fileSearch, diffMode: setMode, addInstall, editApp, delApp, suggest, importCatalog, exportCatalog, exportBundle,
    update, upgrade, rollback, restart, stamp, install, transfer,
    actView, actLoad, actFilter, actCSV, histLoad, histFilter, histCSV, whereLoad, whereFilter, whereCSV,
    envRefresh, envFilter, envReset(){for(const k of Object.keys(EF))EF[k]='';envFilter('q','')}, envEdit, envEditAdd, envEditDel, envPlan, envRecompare, envShowAll, envToggle, envExclude, envClearExcl, envUnlock, envGo, envBack, envHistory, envRollback, envDoctor, wSvcRef, wSlotMode, wUnit, wCheckGo, localWarn, restartWarn, confirmCheck, showRun, cancelRun, loadRuns,
    ignoreLoad, ignoreMark, ignorePreview, ignoreDownload, ignoreSaveLists, ignoreMR, webhook, hookSet, editFeed, feedFetch, delFeed, pipelineKind,
    editSource, testSource, delSource, importBundle, saveSettings, saveCatalogSettings, setRef, filterConns, selConns, closeModal, modalBack,
    filter(k, v, debounce) { G.f[k] = v; clearTimeout(qTimer); if (debounce) qTimer = setTimeout(() => { render(); const i = document.querySelector('#git-body input[type=search]'); if (i) { i.focus(); i.setSelectionRange(i.value.length, i.value.length); } }, 250); else render(); },
    toggleChanges(on) { G.onlyChanges = on; if (G.detail) details(G.detail.id); },
  };
})();
