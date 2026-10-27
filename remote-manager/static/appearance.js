/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// Appearance (v13.7): themes, a per-user theme editor and accessibility options.
//
// Every colour of the interface is a CSS custom property on :root (index.html). A theme is a
// set of those tokens plus a terminal palette; this file applies the chosen theme, the user's
// own changes on top of it, the colour-vision palette and the other accessibility options.
// Themes come from three places: the built-in list below, addons (ui.themes in addon.json, or
// the older single ui.theme stylesheet) and themes the user saved. The choice follows the user
// to every browser (/api/me/ui/appearance); a copy in localStorage paints the next load before
// the server answers. The administrator picks the default theme (appearance_default_theme) and
// whether users may add their own CSS (appearance_custom_css).
'use strict';

Object.assign(LANGS.en, {
  tab_appearance: 'Appearance',
  ap_general_row: 'Theme, colours and accessibility',
  ap_general_row_d: 'Themes, every colour and font, colour vision and other accessibility options',
  stin_appearance: 'Choose a theme, change any colour or font, and adjust WRM for colour vision deficiency, low vision or motion sensitivity. Your choice follows you to every browser.',
  ap_themes: 'Theme',
  ap_follow_os: 'Follow the system light / dark setting',
  ap_follow_os_d: 'Uses the dark theme or the light theme below, whichever your operating system shows.',
  ap_dark_theme: 'Dark theme',
  ap_light_theme: 'Light theme',
  ap_builtin: 'Built in',
  ap_from_addons: 'From addons',
  ap_mine: 'My themes',
  ap_default: 'default',
  ap_dark: 'dark',
  ap_light: 'light',
  ap_customize: 'Customise colours',
  ap_customize_d: 'Changes apply on top of the selected theme, right away. A field you clear returns to the theme\'s colour.',
  ap_g_surface: 'Backgrounds',
  ap_g_text: 'Text',
  ap_g_lines: 'Lines and borders',
  ap_g_accent: 'Accent',
  ap_g_status: 'Status colours',
  ap_g_tint: 'Status text',
  ap_g_term: 'Terminal',
  ap_reset_token: 'Back to the theme colour',
  ap_reset_all: 'Reset my changes',
  ap_reset_q: 'Remove all your colour, font and accessibility changes?',
  ap_save_as: 'Save as my theme',
  ap_save_name: 'Name of the theme:',
  ap_saved: 'Theme saved',
  ap_delete_theme: 'Delete this theme',
  ap_delete_q: 'Delete the theme {name}?',
  ap_export: 'Export',
  ap_import: 'Import',
  ap_import_bad: 'This is not a WRM theme file',
  ap_imported: 'Theme imported',
  ap_fonts: 'Fonts and size',
  ap_font_ui: 'Interface font',
  ap_font_mono: 'Monospace font (terminal, editor, logs)',
  ap_font_custom: 'Other (type a font name)…',
  ap_font_custom_q: 'Font family as in CSS, for example "Fira Sans", sans-serif. The font must be installed on this computer.',
  ap_ui_size: 'Interface size',
  ap_ui_size_d: 'Scales the whole interface. Terminal text has its own size in the Terminal tab.',
  ap_radius: 'Rounded corners',
  ap_density: 'Spacing',
  ap_density_compact: 'Compact',
  ap_density_normal: 'Normal',
  ap_density_roomy: 'Roomy (larger click targets)',
  ap_line_height: 'Line spacing',
  ap_letter: 'Letter spacing',
  ap_weight: 'Heavier text',
  ap_a11y: 'Accessibility',
  ap_cvd: 'Colour vision',
  ap_cvd_d: 'Replaces status colours (OK, warning, error, diff) with a palette you can tell apart. It works with every theme.',
  ap_cvd_none: 'Typical colour vision',
  ap_cvd_deutan: 'Deuteranopia / deuteranomaly (green-weak, most common)',
  ap_cvd_protan: 'Protanopia / protanomaly (red-weak)',
  ap_cvd_tritan: 'Tritanopia / tritanomaly (blue-yellow)',
  ap_cvd_achroma: 'Achromatopsia (no colour, light and dark only)',
  ap_cvd_term: 'Use the colour-vision palette in terminals too',
  ap_cvd_term_d: 'ANSI red, green, yellow, blue and magenta become colours you can tell apart. Programs\' colours change meaningfully only if they use the standard 16 colours.',
  ap_symbols: 'Do not rely on colour alone',
  ap_symbols_d: 'Status badges get ✓ ✕ ! signs, connection status dots get shapes (● up, ◆ slow, ■ down) and links are underlined.',
  ap_contrast: 'Contrast',
  ap_contrast_normal: 'Normal',
  ap_contrast_more: 'More (stronger text and borders)',
  ap_contrast_max: 'Maximum',
  ap_term_contrast: 'Minimum contrast in terminals',
  ap_term_contrast_d: 'The terminal lightens or darkens text whose colour is too close to its background.',
  ap_term_contrast_off: 'Off (as programs choose)',
  ap_motion: 'Animations',
  ap_motion_os: 'As the system says',
  ap_motion_reduce: 'Reduce',
  ap_motion_full: 'Full',
  ap_transparency: 'Reduce transparency and blur',
  ap_transparency_d: 'Menus, toasts and dialogs get solid backgrounds.',
  ap_focus: 'Strong keyboard focus outline',
  ap_focus_d: 'A thick, high-contrast outline around the element that has keyboard focus.',
  ap_underline: 'Underline links',
  ap_custom_css: 'Custom CSS',
  ap_custom_css_d: 'Advanced: CSS added to your own interface only, after the theme. Use the tokens (var(--bg), var(--accent) …) to stay in step with the theme.',
  ap_custom_css_off: 'Your administrator turned custom CSS off.',
  ap_preview: 'Preview',
  ap_pv_text: 'Main text',
  ap_pv_muted: 'Secondary text',
  ap_pv_faint: 'Faint text',
  ap_pv_link: 'a link',
  ap_pv_ok: 'Online',
  ap_pv_warn: 'Update',
  ap_pv_bad: 'Failed',
  ap_pv_add: '+ added line',
  ap_pv_del: '- removed line',
  ap_pv_button: 'Button',
  ap_pv_second: 'Secondary',
  ap_admin_default: 'Default for everyone',
  ap_admin_default_d: 'Administrator: make this theme the default for users who have not chosen one.',
  ap_admin_default_set: 'Default theme set to {name}',
  ap_unknown_theme: 'The theme {id} is not available any more; showing the default.',
  // built-in theme names
  th_graphite: 'Graphite',
  th_midnight: 'Midnight (OLED)',
  th_fjord: 'Fjord',
  th_solar_dark: 'Solar dark',
  th_forest: 'Forest',
  th_ember: 'Ember',
  th_daylight: 'Daylight',
  th_paper: 'Paper',
  th_solar_light: 'Solar light',
  th_hc_dark: 'High contrast dark',
  th_hc_light: 'High contrast light',
  // Admin → Policies
  pg_appearance: 'Appearance',
  p_appearance_default_theme: 'Default theme',
  p_appearance_default_theme_d: 'Theme id for users who have not chosen one: graphite, midnight, fjord, solar-dark, forest, ember, daylight, paper, solar-light, hc-dark, hc-light, or addon:<addon>:<theme>.',
  p_appearance_default_theme_ph: 'graphite',
  p_appearance_custom_css: 'Allow custom CSS',
  p_appearance_custom_css_d: 'Users may add CSS to their own interface (Settings → Appearance). It never reaches other users.',
});

Object.assign(LANGS.hr, {
  tab_appearance: 'Izgled',
  ap_general_row: 'Tema, boje i pristupačnost',
  ap_general_row_d: 'Teme, svaka boja i font, raspoznavanje boja i ostale opcije pristupačnosti',
  stin_appearance: 'Odaberite temu, promijenite bilo koju boju ili font i prilagodite WRM za daltonizam, slabiji vid ili osjetljivost na pokrete. Vaš izbor vas prati u svakom pregledniku.',
  ap_themes: 'Tema',
  ap_follow_os: 'Prati svijetli / tamni način sustava',
  ap_follow_os_d: 'Koristi tamnu ili svijetlu temu ispod, ovisno o tome što prikazuje operacijski sustav.',
  ap_dark_theme: 'Tamna tema',
  ap_light_theme: 'Svijetla tema',
  ap_builtin: 'Ugrađene',
  ap_from_addons: 'Iz addona',
  ap_mine: 'Moje teme',
  ap_default: 'zadana',
  ap_dark: 'tamna',
  ap_light: 'svijetla',
  ap_customize: 'Prilagodi boje',
  ap_customize_d: 'Promjene se primjenjuju na odabranu temu, odmah. Polje koje ispraznite vraća boju teme.',
  ap_g_surface: 'Pozadine',
  ap_g_text: 'Tekst',
  ap_g_lines: 'Linije i rubovi',
  ap_g_accent: 'Naglasak',
  ap_g_status: 'Boje statusa',
  ap_g_tint: 'Tekst statusa',
  ap_g_term: 'Terminal',
  ap_reset_token: 'Vrati boju teme',
  ap_reset_all: 'Poništi moje promjene',
  ap_reset_q: 'Ukloniti sve vaše promjene boja, fontova i pristupačnosti?',
  ap_save_as: 'Spremi kao moju temu',
  ap_save_name: 'Naziv teme:',
  ap_saved: 'Tema spremljena',
  ap_delete_theme: 'Obriši ovu temu',
  ap_delete_q: 'Obrisati temu {name}?',
  ap_export: 'Izvezi',
  ap_import: 'Uvezi',
  ap_import_bad: 'Ovo nije datoteka WRM teme',
  ap_imported: 'Tema uvezena',
  ap_fonts: 'Fontovi i veličina',
  ap_font_ui: 'Font sučelja',
  ap_font_mono: 'Font fiksne širine (terminal, uređivač, logovi)',
  ap_font_custom: 'Drugi (upišite naziv fonta)…',
  ap_font_custom_q: 'Obitelj fonta kao u CSS-u, na primjer "Fira Sans", sans-serif. Font mora biti instaliran na ovom računalu.',
  ap_ui_size: 'Veličina sučelja',
  ap_ui_size_d: 'Povećava ili smanjuje cijelo sučelje. Tekst terminala ima vlastitu veličinu u kartici Terminal.',
  ap_radius: 'Zaobljeni kutovi',
  ap_density: 'Razmaci',
  ap_density_compact: 'Zbijeno',
  ap_density_normal: 'Normalno',
  ap_density_roomy: 'Prozračno (veće površine za klik)',
  ap_line_height: 'Prored',
  ap_letter: 'Razmak između slova',
  ap_weight: 'Deblji tekst',
  ap_a11y: 'Pristupačnost',
  ap_cvd: 'Raspoznavanje boja',
  ap_cvd_d: 'Zamjenjuje boje statusa (u redu, upozorenje, greška, razlike) paletom koju možete razlikovati. Radi sa svakom temom.',
  ap_cvd_none: 'Uobičajeno raspoznavanje boja',
  ap_cvd_deutan: 'Deuteranopija / deuteranomalija (slabije zelene, najčešće)',
  ap_cvd_protan: 'Protanopija / protanomalija (slabije crvene)',
  ap_cvd_tritan: 'Tritanopija / tritanomalija (plavo-žuto)',
  ap_cvd_achroma: 'Akromatopsija (bez boja, samo svijetlo i tamno)',
  ap_cvd_term: 'Koristi tu paletu i u terminalima',
  ap_cvd_term_d: 'ANSI crvena, zelena, žuta, plava i ljubičasta postaju boje koje možete razlikovati. Boje programa se smisleno mijenjaju samo ako koriste standardnih 16 boja.',
  ap_symbols: 'Ne oslanjaj se samo na boju',
  ap_symbols_d: 'Oznake statusa dobivaju znakove ✓ ✕ !, točke statusa veza dobivaju oblike (● radi, ◆ sporo, ■ ne radi), a poveznice su podcrtane.',
  ap_contrast: 'Kontrast',
  ap_contrast_normal: 'Normalan',
  ap_contrast_more: 'Veći (jači tekst i rubovi)',
  ap_contrast_max: 'Najveći',
  ap_term_contrast: 'Najmanji kontrast u terminalima',
  ap_term_contrast_d: 'Terminal posvijetli ili potamni tekst čija je boja preblizu pozadini.',
  ap_term_contrast_off: 'Isključeno (kako programi odaberu)',
  ap_motion: 'Animacije',
  ap_motion_os: 'Kako kaže sustav',
  ap_motion_reduce: 'Smanji',
  ap_motion_full: 'Pune',
  ap_transparency: 'Smanji prozirnost i zamućenje',
  ap_transparency_d: 'Izbornici, obavijesti i dijalozi dobivaju punu pozadinu.',
  ap_focus: 'Jak obrub fokusa tipkovnice',
  ap_focus_d: 'Debeli obrub jakog kontrasta oko elementa koji ima fokus tipkovnice.',
  ap_underline: 'Podcrtaj poveznice',
  ap_custom_css: 'Vlastiti CSS',
  ap_custom_css_d: 'Napredno: CSS koji se dodaje samo vašem sučelju, nakon teme. Koristite tokene (var(--bg), var(--accent) …) da ostane usklađen s temom.',
  ap_custom_css_off: 'Administrator je isključio vlastiti CSS.',
  ap_preview: 'Pregled',
  ap_pv_text: 'Glavni tekst',
  ap_pv_muted: 'Sporedni tekst',
  ap_pv_faint: 'Blijedi tekst',
  ap_pv_link: 'poveznica',
  ap_pv_ok: 'Radi',
  ap_pv_warn: 'Ažuriranje',
  ap_pv_bad: 'Neuspjelo',
  ap_pv_add: '+ dodani redak',
  ap_pv_del: '- uklonjeni redak',
  ap_pv_button: 'Gumb',
  ap_pv_second: 'Sporedni',
  ap_admin_default: 'Zadana za sve',
  ap_admin_default_d: 'Administrator: postavi ovu temu kao zadanu za korisnike koji nisu odabrali svoju.',
  ap_admin_default_set: 'Zadana tema postavljena: {name}',
  ap_unknown_theme: 'Tema {id} više nije dostupna; prikazana je zadana.',
  th_graphite: 'Grafit',
  th_midnight: 'Ponoć (OLED)',
  th_fjord: 'Fjord',
  th_solar_dark: 'Solar tamna',
  th_forest: 'Šuma',
  th_ember: 'Žar',
  th_daylight: 'Dan',
  th_paper: 'Papir',
  th_solar_light: 'Solar svijetla',
  th_hc_dark: 'Visoki kontrast tamna',
  th_hc_light: 'Visoki kontrast svijetla',
  pg_appearance: 'Izgled',
  p_appearance_default_theme: 'Zadana tema',
  p_appearance_default_theme_d: 'Oznaka teme za korisnike koji nisu odabrali svoju: graphite, midnight, fjord, solar-dark, forest, ember, daylight, paper, solar-light, hc-dark, hc-light ili addon:<addon>:<tema>.',
  p_appearance_default_theme_ph: 'graphite',
  p_appearance_custom_css: 'Dopusti vlastiti CSS',
  p_appearance_custom_css_d: 'Korisnici mogu dodati CSS vlastitom sučelju (Postavke → Izgled). Nikad ne dolazi do drugih korisnika.',
});

(function () {
  const $ = id => document.getElementById(id);
  const tf = (k, o) => t(k).replace(/\{(\w+)\}/g, (m, x) => (o && o[x] !== undefined ? o[x] : m));
  const CACHE = 'wrm_appearance';

  // ── built-in themes ───────────────────────────────
  // Only the base tokens are listed; the rest (rgb channels, hover and tint variants, the
  // terminal palette) are derived in tokensFor().
  const DARK_TINTS = {'red-t': '#fca5a5', 'green-t': '#86efac', 'yellow-t': '#fcd34d', 'accent-t': '#93c5fd', 'purple-t': '#c4b5fd', 'orange-t': '#fed7aa'};
  const LIGHT_TINTS = {'red-t': '#b91c1c', 'green-t': '#166534', 'yellow-t': '#92400e', 'accent-t': '#1d4ed8', 'purple-t': '#6d28d9', 'orange-t': '#c2410c'};
  const TERM_DARK = {black: '#1d222d', brightBlack: '#5a6376', red: '#ef4444', brightRed: '#f87171', green: '#22c55e', brightGreen: '#4ade80', yellow: '#f59e0b', brightYellow: '#fbbf24',
    blue: '#4f8cff', brightBlue: '#7aa8ff', magenta: '#a78bfa', brightMagenta: '#c4b5fd', cyan: '#06b6d4', brightCyan: '#22d3ee', white: '#e7e9ef', brightWhite: '#f8fafc'};
  const TERM_LIGHT = {black: '#24292f', brightBlack: '#57606a', red: '#cf222e', brightRed: '#a40e26', green: '#116329', brightGreen: '#1a7f37', yellow: '#7d4e00', brightYellow: '#633c01',
    blue: '#0969da', brightBlue: '#218bff', magenta: '#8250df', brightMagenta: '#a475f9', cyan: '#1b7c83', brightCyan: '#3192aa', white: '#6e7781', brightWhite: '#8c959f'};
  const TERM_SOLAR = {black: '#073642', brightBlack: '#586e75', red: '#dc322f', brightRed: '#cb4b16', green: '#859900', brightGreen: '#93a1a1', yellow: '#b58900', brightYellow: '#839496',
    blue: '#268bd2', brightBlue: '#839496', magenta: '#d33682', brightMagenta: '#6c71c4', cyan: '#2aa198', brightCyan: '#93a1a1', white: '#eee8d5', brightWhite: '#fdf6e3'};
  const TERM_FJORD = {black: '#3b4252', brightBlack: '#4c566a', red: '#bf616a', brightRed: '#d08770', green: '#a3be8c', brightGreen: '#b5d29e', yellow: '#ebcb8b', brightYellow: '#f0d9a8',
    blue: '#81a1c1', brightBlue: '#88c0d0', magenta: '#b48ead', brightMagenta: '#c8a9c2', cyan: '#8fbcbb', brightCyan: '#a3d4d3', white: '#e5e9f0', brightWhite: '#eceff4'};
  const TERM_HC_DARK = {black: '#000000', brightBlack: '#a0a0a0', red: '#ff5c5c', brightRed: '#ff8080', green: '#3dff7a', brightGreen: '#80ffa8', yellow: '#ffd400', brightYellow: '#ffe866',
    blue: '#6cb6ff', brightBlue: '#a3d0ff', magenta: '#e0a3ff', brightMagenta: '#f0ccff', cyan: '#40e0ff', brightCyan: '#99f0ff', white: '#ffffff', brightWhite: '#ffffff'};
  const TERM_HC_LIGHT = {black: '#000000', brightBlack: '#3d3d3d', red: '#b00000', brightRed: '#8a0000', green: '#006b2e', brightGreen: '#005224', yellow: '#6b4400', brightYellow: '#523400',
    blue: '#0040c0', brightBlue: '#00309a', magenta: '#6a00b8', brightMagenta: '#520090', cyan: '#005f6b', brightCyan: '#004852', white: '#3d3d3d', brightWhite: '#000000'};

  const BUILTIN = [
    {id: 'graphite', mode: 'dark', term: TERM_DARK, tokens: {bg0: '#0b0d12', bg: '#0a0c11', bg2: '#10131a', bg3: '#161a23', bg4: '#1d222d', bg5: '#252b38', border: '#1f2430', border2: '#2a3140',
      text: '#e7e9ef', text2: '#97a0b0', text3: '#5a6376', accent: '#4f8cff', green: '#22c55e', red: '#ef4444', yellow: '#f59e0b', purple: '#a78bfa', cyan: '#06b6d4'}},
    {id: 'midnight', mode: 'dark', term: Object.assign({}, TERM_DARK, {black: '#15181e'}), tokens: {bg0: '#000000', bg: '#000000', bg2: '#07080a', bg3: '#0e1014', bg4: '#15181e', bg5: '#1d2129', border: '#1a1d24', border2: '#262a33',
      text: '#f2f3f5', text2: '#a2a9b6', text3: '#646c7a', accent: '#5b9dff', green: '#22c55e', red: '#f05252', yellow: '#f5a623', purple: '#b197fc', cyan: '#22d3ee'}},
    {id: 'fjord', mode: 'dark', term: TERM_FJORD, tokens: {bg0: '#242933', bg: '#2e3440', bg2: '#353c4a', bg3: '#3b4252', bg4: '#434c5e', bg5: '#4c566a', border: '#434c5e', border2: '#56617a',
      text: '#eceff4', text2: '#c9d1de', text3: '#8f9bb3', accent: '#88c0d0', 'on-accent': '#2e3440', green: '#a3be8c', red: '#bf616a', yellow: '#ebcb8b', purple: '#b48ead', cyan: '#8fbcbb',
      'red-t': '#e3a6ad', 'green-t': '#c7dbb5', 'yellow-t': '#f3dfb6', 'accent-t': '#b6dce6', 'purple-t': '#d6bfd1', 'orange-t': '#e8b8a0'}},
    {id: 'solar-dark', mode: 'dark', term: Object.assign({}, TERM_SOLAR, {black: '#073642'}), tokens: {bg0: '#00212b', bg: '#002b36', bg2: '#073642', bg3: '#0b3d4a', bg4: '#124753', bg5: '#1a525f', border: '#0f4552', border2: '#215b6a',
      text: '#e6dfcc', text2: '#93a1a1', text3: '#657b83', accent: '#268bd2', green: '#859900', red: '#dc322f', yellow: '#b58900', purple: '#6c71c4', cyan: '#2aa198',
      'red-t': '#f2908d', 'green-t': '#bccd4f', 'yellow-t': '#e4bd4f', 'accent-t': '#80bdea', 'purple-t': '#a9ace3', 'orange-t': '#f0a37a'}},
    {id: 'forest', mode: 'dark', term: Object.assign({}, TERM_DARK, {blue: '#5aa9e6', brightBlue: '#8cc4f0'}), tokens: {bg0: '#090e0b', bg: '#0b110d', bg2: '#111914', bg3: '#17221b', bg4: '#1e2b23', bg5: '#26362c', border: '#1c2920', border2: '#2a3b30',
      text: '#e5ede7', text2: '#9badA0', text3: '#5c6e62', accent: '#3fbf7f', 'on-accent': '#04120a', green: '#4ade80', red: '#ef5350', yellow: '#f2b84b', purple: '#b39ddb', cyan: '#4dd0c8'}},
    {id: 'ember', mode: 'dark', term: TERM_DARK, tokens: {bg0: '#0d0b0a', bg: '#0e0c0b', bg2: '#151210', bg3: '#1c1815', bg4: '#241f1b', bg5: '#2e2722', border: '#2a231e', border2: '#3a3029',
      text: '#efe9e4', text2: '#aa9f96', text3: '#6b6058', accent: '#e8772e', green: '#22c55e', red: '#ef4444', yellow: '#f5b324', purple: '#c4a1ff', cyan: '#22c3d6', 'accent-t': '#fdba8c'}},
    {id: 'daylight', mode: 'light', term: TERM_LIGHT, tokens: {bg0: '#f6f8fa', bg: '#eef1f5', bg2: '#ffffff', bg3: '#f6f8fa', bg4: '#eceff3', bg5: '#e1e5eb', border: '#d8dde5', border2: '#c3cad5',
      text: '#1b1f24', text2: '#4c5562', text3: '#727c8b', accent: '#2563eb', green: '#15803d', red: '#dc2626', yellow: '#b45309', purple: '#7c3aed', cyan: '#0e7490', glass: 'rgba(255,255,255,.97)'}},
    {id: 'paper', mode: 'light', term: Object.assign({}, TERM_LIGHT, {background: '#fbf7ef'}), tokens: {bg0: '#f6f1e6', bg: '#f2ecdf', bg2: '#fbf7ef', bg3: '#f5efe3', bg4: '#ece4d4', bg5: '#e1d7c4', border: '#ddd2bd', border2: '#c8bba1',
      text: '#2b2620', text2: '#5c5346', text3: '#857a69', accent: '#9a5b13', green: '#4d7c0f', red: '#b42318', yellow: '#a15c07', purple: '#7a4fb0', cyan: '#0f6e78', glass: 'rgba(251,247,239,.97)',
      'accent-t': '#7a4510'}},
    {id: 'solar-light', mode: 'light', term: Object.assign({}, TERM_SOLAR, {background: '#fdf6e3', foreground: '#586e75', white: '#657b83', brightWhite: '#073642', black: '#eee8d5', brightBlack: '#93a1a1'}),
      tokens: {bg0: '#fdf6e3', bg: '#f7f0dc', bg2: '#fdf6e3', bg3: '#f5eedb', bg4: '#eee8d5', bg5: '#e3dbc3', border: '#e0d8c0', border2: '#cdc3a6',
        text: '#073642', text2: '#4f6369', text3: '#7d8d90', accent: '#268bd2', green: '#5f7000', red: '#c62d2a', yellow: '#8a6800', purple: '#6c71c4', cyan: '#1f7f78', glass: 'rgba(253,246,227,.97)'}},
    {id: 'hc-dark', mode: 'dark', term: TERM_HC_DARK, tokens: {bg0: '#000000', bg: '#000000', bg2: '#000000', bg3: '#0d0d0d', bg4: '#1a1a1a', bg5: '#2b2b2b', border: '#9a9a9a', border2: '#ffffff',
      text: '#ffffff', text2: '#f0f0f0', text3: '#cfcfcf', accent: '#ffd400', 'on-accent': '#000000', green: '#3dff7a', red: '#ff5c5c', yellow: '#ffd400', purple: '#e0a3ff', cyan: '#40e0ff', glass: '#000000',
      'red-t': '#ff9a9a', 'green-t': '#8dffb0', 'yellow-t': '#ffe866', 'accent-t': '#ffe866', 'purple-t': '#f0ccff', 'orange-t': '#ffc999'}},
    {id: 'hc-light', mode: 'light', term: TERM_HC_LIGHT, tokens: {bg0: '#ffffff', bg: '#ffffff', bg2: '#ffffff', bg3: '#f2f2f2', bg4: '#e6e6e6', bg5: '#d4d4d4', border: '#595959', border2: '#000000',
      text: '#000000', text2: '#141414', text3: '#3a3a3a', accent: '#0040c0', green: '#006b2e', red: '#b00000', yellow: '#7a4d00', purple: '#5b00b0', cyan: '#005f6b', glass: '#ffffff',
      'red-t': '#8a0000', 'green-t': '#005224', 'yellow-t': '#5c3a00', 'accent-t': '#00309a', 'purple-t': '#470090', 'orange-t': '#8a3a00'}},
  ];

  // The tokens the editor shows, grouped. Names without the leading "--".
  const GROUPS = [
    ['ap_g_surface', ['bg', 'bg0', 'bg2', 'bg3', 'bg4', 'bg5']],
    ['ap_g_text', ['text', 'text2', 'text3']],
    ['ap_g_lines', ['border', 'border2']],
    ['ap_g_accent', ['accent', 'on-accent']],
    ['ap_g_status', ['green', 'yellow', 'red', 'purple', 'cyan']],
    ['ap_g_tint', ['green-t', 'yellow-t', 'red-t', 'accent-t', 'purple-t', 'orange-t']],
    ['ap_g_term', ['term-background', 'term-foreground', 'term-red', 'term-green', 'term-yellow', 'term-blue', 'term-magenta', 'term-cyan']],
  ];
  const TOKEN_NAMES = {bg: 'Background', bg0: 'Deep (terminal, code)', bg2: 'Panels', bg3: 'Fields', bg4: 'Raised', bg5: 'Hover', text: 'Text', text2: 'Secondary', text3: 'Faint',
    border: 'Border', border2: 'Strong border', accent: 'Accent', 'on-accent': 'Text on accent', green: 'OK', yellow: 'Warning', red: 'Error', purple: 'Info', cyan: 'Cyan'};
  const TOKEN_NAMES_HR = {bg: 'Pozadina', bg0: 'Duboka (terminal, kod)', bg2: 'Ploče', bg3: 'Polja', bg4: 'Podignuto', bg5: 'Lebdenje', text: 'Tekst', text2: 'Sporedni', text3: 'Blijedi',
    border: 'Rub', border2: 'Jači rub', accent: 'Naglasak', 'on-accent': 'Tekst na naglasku', green: 'U redu', yellow: 'Upozorenje', red: 'Greška', purple: 'Info', cyan: 'Cijan'};

  // Colour-vision palettes (Okabe & Ito): distinguishable with every common deficiency.
  const CVD = {
    dark: {green: '#2ec4a0', 'green-t': '#8de3cd', red: '#ff7b2e', 'red-t': '#ffb889', yellow: '#f0e442', 'yellow-t': '#f5ec8a', purple: '#e08ec0', 'purple-t': '#f0bddb'},
    light: {green: '#00805d', 'green-t': '#006249', red: '#c24e00', 'red-t': '#9a3d00', yellow: '#8a7a00', 'yellow-t': '#6b5f00', purple: '#b0508a', 'purple-t': '#8c3a6c'},
    tritanDark: {yellow: '#ff9a3c', 'yellow-t': '#ffc999', purple: '#ff6fa8', 'purple-t': '#ffb3d1'},
    tritanLight: {yellow: '#b35400', 'yellow-t': '#8a4000', purple: '#c2185b', 'purple-t': '#9a1248'},
    achromaDark: {green: '#d9d9d9', 'green-t': '#e6e6e6', red: '#ffffff', 'red-t': '#ffffff', yellow: '#a6a6a6', 'yellow-t': '#c2c2c2', purple: '#bfbfbf', 'purple-t': '#d4d4d4', accent: '#e0e0e0', 'on-accent': '#000000'},
    achromaLight: {green: '#4d4d4d', 'green-t': '#3a3a3a', red: '#000000', 'red-t': '#000000', yellow: '#6e6e6e', 'yellow-t': '#555555', purple: '#5c5c5c', 'purple-t': '#454545', accent: '#262626', 'on-accent': '#ffffff'},
    termDark: {red: '#e66100', brightRed: '#ff8f3c', green: '#1fb58f', brightGreen: '#5fd8b5', yellow: '#f0e442', brightYellow: '#f7ef8a', blue: '#3d8fd6', brightBlue: '#7cc3f0', magenta: '#d17fb0', brightMagenta: '#e7acd0'},
    termLight: {red: '#b34700', brightRed: '#d55e00', green: '#00785a', brightGreen: '#009e73', yellow: '#7a6c00', brightYellow: '#968500', blue: '#0060a0', brightBlue: '#0072b2', magenta: '#a3527f', brightMagenta: '#cc79a7'},
  };

  const FONTS_UI = [
    ['', 'Inter (WRM)'],
    ["system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif", 'System'],
    ["'Atkinson Hyperlegible', Verdana, sans-serif", 'Atkinson Hyperlegible / Verdana (readable)'],
    ["Lexend, 'Segoe UI', sans-serif", 'Lexend (readable)'],
    ["OpenDyslexic, 'Comic Sans MS', sans-serif", 'OpenDyslexic (dyslexia)'],
    ["Verdana, Tahoma, sans-serif", 'Verdana'],
    ["Georgia, 'Times New Roman', serif", 'Georgia (serif)'],
  ];
  const FONTS_MONO = [
    ['', 'JetBrains Mono (WRM)'],
    ["'Cascadia Mono', 'Cascadia Code', Consolas, monospace", 'Cascadia Mono'],
    ["'Fira Code', 'Fira Mono', monospace", 'Fira Code'],
    ["'Source Code Pro', monospace", 'Source Code Pro'],
    ["'IBM Plex Mono', monospace", 'IBM Plex Mono'],
    ["Consolas, 'Courier New', monospace", 'Consolas'],
    ["Menlo, Monaco, monospace", 'Menlo'],
    ["'Atkinson Hyperlegible Mono', 'JetBrains Mono', monospace", 'Atkinson Hyperlegible Mono (readable)'],
  ];

  const DEFAULTS = {theme: '', follow_os: false, dark_theme: 'graphite', light_theme: 'daylight', overrides: {}, mine: [],
    font_ui: '', font_mono: '', ui_size: 100, radius: 10, density: 'normal', line_height: 0, letter: 0, weight: false,
    cvd: 'none', cvd_term: true, symbols: false, contrast: 'normal', term_contrast: 1, motion: 'os', solid: false, focus: false, underline: false, custom_css: ''};

  let prefs = loadLocal();
  let addonThemes = [];
  const mq = window.matchMedia ? window.matchMedia('(prefers-color-scheme: light)') : null;

  function store(v) { try { localStorage.setItem(CACHE, JSON.stringify(v)); } catch (_) { /* private mode */ } }
  function loadLocal() {
    let p = {};
    try { p = JSON.parse(localStorage.getItem(CACHE) || '{}') || {}; } catch (_) { p = {}; }
    return Object.assign({}, DEFAULTS, p);
  }
  function lang() { return typeof currentLang !== 'undefined' ? currentLang : 'en'; }
  function policy() { return (typeof currentUser !== 'undefined' && currentUser && currentUser.appearance) || {default_theme: 'graphite', custom_css: true}; }

  // ── colour helpers ────────────────────────────────
  function hexToRgb(h) {
    h = String(h || '').trim();
    let m = /^#([0-9a-f]{3})$/i.exec(h);
    if (m) h = '#' + m[1].split('').map(c => c + c).join('');
    m = /^#([0-9a-f]{6})([0-9a-f]{2})?$/i.exec(h);
    if (m) { const n = parseInt(m[1], 16); return [n >> 16 & 255, n >> 8 & 255, n & 255]; }
    m = /^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/i.exec(h);
    return m ? [+m[1], +m[2], +m[3]] : null;
  }
  function rgbToHex(r) { return '#' + r.map(v => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0')).join(''); }
  function mix(a, b, w) { const x = hexToRgb(a), y = hexToRgb(b); return x && y ? rgbToHex(x.map((v, i) => v * (1 - w) + y[i] * w)) : a; }
  function lum(h) { const c = hexToRgb(h); if (!c) return 0; const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }; return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2]); }
  function contrast(a, b) { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); }
  // A value an addon or an import may set: a colour or a short CSS value, never anything
  // that loads or runs something.
  function safeValue(v) { v = String(v == null ? '' : v); return v.length <= 200 && !/[;{}<>]|url\(|expression|@import|\\/i.test(v) ? v : ''; }

  // ── theme lookup ──────────────────────────────────
  function allThemes() {
    return BUILTIN.map(x => Object.assign({source: 'builtin'}, x))
      .concat(addonThemes.map(x => Object.assign({source: 'addon'}, x)))
      .concat((prefs.mine || []).map(x => Object.assign({source: 'mine'}, x)));
  }
  function themeById(id) { return allThemes().find(x => x.id === id); }
  function themeName(th) {
    if (!th) return '';
    if (th.source === 'builtin') return t('th_' + th.id.replace(/-/g, '_'));
    return (lang() === 'hr' && th.name_hr) || th.name || th.id;
  }
  function osLight() { return !!(mq && mq.matches); }
  function activeId() {
    if (prefs.follow_os) return osLight() ? prefs.light_theme : prefs.dark_theme;
    return prefs.theme || policy().default_theme || 'graphite';
  }
  function activeTheme() { return themeById(activeId()) || themeById(policy().default_theme) || BUILTIN[0]; }

  // tokensFor(): the full token map (with "--") and the terminal palette for a theme plus the
  // user's overrides and accessibility options.
  function tokensFor(th) {
    const light = th.mode === 'light';
    const base = th.base ? (themeById(th.base) || BUILTIN[0]) : null;
    const tk = Object.assign({}, light ? LIGHT_TINTS : DARK_TINTS, base ? base.tokens : {}, th.tokens || {});
    const ov = prefs.overrides || {};
    Object.keys(ov).forEach(k => { if (!k.startsWith('term-') && ov[k]) tk[k] = ov[k]; });
    if (prefs.cvd && prefs.cvd !== 'none') {
      const set = prefs.cvd === 'achroma' ? (light ? CVD.achromaLight : CVD.achromaDark)
        : prefs.cvd === 'tritan' ? Object.assign({}, light ? CVD.light : CVD.dark, light ? CVD.tritanLight : CVD.tritanDark) : (light ? CVD.light : CVD.dark);
      Object.assign(tk, set);
    }
    if (prefs.contrast === 'more' || prefs.contrast === 'max') {
      const w = prefs.contrast === 'max' ? 0.75 : 0.4;
      tk.text2 = mix(tk.text2, tk.text, w);
      tk.text3 = mix(tk.text3, tk.text, w);
      tk.border = mix(tk.border, tk.text3, w * 0.8);
      tk.border2 = mix(tk.border2, tk.text2, w * 0.8);
    }
    if (!tk['on-accent']) tk['on-accent'] = contrast(tk.accent, '#ffffff') >= contrast(tk.accent, '#000000') ? '#ffffff' : '#111111';
    if (!tk.glass) tk.glass = tk.bg2;
    const out = {};
    Object.keys(tk).forEach(k => { const v = safeValue(tk[k]); if (v) out['--' + k] = v; });
    out['--text1'] = tk.text;
    out['--accent-h'] = mix(tk.accent, light ? '#000000' : '#ffffff', 0.12);
    const rgb = k => { const c = hexToRgb(tk[k]); return c ? c.join(',') : null; };
    [['accent', 'accent-rgb'], ['red', 'red-rgb'], ['green', 'green-rgb'], ['yellow', 'yellow-rgb'], ['purple', 'purple-rgb'], ['cyan', 'cyan-rgb'], ['text2', 'muted-rgb']]
      .forEach(([k, n]) => { const v = rgb(k); if (v) out['--' + n] = v; });
    const a = rgb('accent');
    if (a) { out['--accent-d'] = `rgba(${a},.14)`; out['--accent-b'] = `rgba(${a},.38)`; }
    out['--fg-rgb'] = light ? '0,0,0' : '255,255,255';
    if (light) { out['--shadow-lg'] = '0 20px 60px rgba(0,0,0,.18), 0 2px 8px rgba(0,0,0,.12)'; out['--shadow-md'] = '0 8px 28px rgba(0,0,0,.14)'; }
    out['--radius'] = (prefs.radius | 0) + 'px';
    out['--radius-sm'] = Math.max(0, Math.round((prefs.radius | 0) * 0.7)) + 'px';
    if (prefs.font_ui) { const v = safeValue(prefs.font_ui); if (v) out['--font-ui'] = v; }
    if (prefs.font_mono) { const v = safeValue(prefs.font_mono); if (v) out['--font-mono'] = v; }
    // terminal
    const term = Object.assign({}, light ? TERM_LIGHT : TERM_DARK, base ? base.term : {}, th.term || {});
    term.background = term.background || tk.bg0;
    term.foreground = term.foreground || tk.text;
    term.cursor = tk.accent; term.cursorAccent = term.background;
    term.selectionBackground = a ? `rgba(${a},.35)` : 'rgba(79,140,255,.35)';
    if (prefs.cvd && prefs.cvd !== 'none' && prefs.cvd !== 'achroma' && prefs.cvd_term) Object.assign(term, light ? CVD.termLight : CVD.termDark);
    Object.keys(ov).forEach(k => { if (k.startsWith('term-') && ov[k]) term[k.slice(5)] = ov[k]; });
    return {vars: out, term, light};
  }

  // ── apply ─────────────────────────────────────────
  let applied = [];
  function apply() {
    const th = activeTheme();
    const {vars, term, light} = tokensFor(th);
    const root = document.documentElement;
    applied.forEach(k => { if (!(k in vars)) root.style.removeProperty(k); });
    Object.entries(vars).forEach(([k, v]) => root.style.setProperty(k, v));
    applied = Object.keys(vars);
    root.dataset.themeMode = light ? 'light' : 'dark';
    root.dataset.theme = th.id;
    root.style.colorScheme = light ? 'light' : 'dark';
    const cls = root.classList;
    cls.toggle('ap-symbols', !!prefs.symbols || prefs.cvd === 'achroma');
    cls.toggle('ap-underline', !!prefs.underline || !!prefs.symbols);
    cls.toggle('ap-focus', !!prefs.focus);
    cls.toggle('ap-solid', !!prefs.solid);
    cls.toggle('ap-weight', !!prefs.weight);
    cls.toggle('ap-reduce-motion', prefs.motion === 'reduce');
    cls.toggle('ap-full-motion', prefs.motion === 'full');
    ['compact', 'roomy'].forEach(d => cls.toggle('ap-' + d, prefs.density === d));
    root.style.setProperty('--ap-line', prefs.line_height ? String(prefs.line_height) : 'normal');
    root.style.setProperty('--ap-letter', prefs.letter ? prefs.letter + 'em' : 'normal');
    cls.toggle('ap-text-spacing', !!(prefs.line_height || prefs.letter));
    const size = Math.max(70, Math.min(160, parseInt(prefs.ui_size, 10) || 100));
    document.body && (document.body.style.zoom = size === 100 ? '' : String(size / 100));
    // brand marks drawn for a dark or a light background
    document.querySelectorAll('img[src*="-on-dark.svg"], img[src*="-on-light.svg"]').forEach(img => {
      const want = img.getAttribute('src').replace(/-on-(dark|light)\.svg$/, light ? '-on-light.svg' : '-on-dark.svg');
      if (img.getAttribute('src') !== want) img.setAttribute('src', want);
    });
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute('content', vars['--bg2'] || '#10131a');
    // addon stylesheet of the active theme only
    document.querySelectorAll('link[data-addon-theme]').forEach(l => { if (!(th.source === 'addon' && th.css && l.getAttribute('href') === th.css)) l.remove(); });
    if (th.source === 'addon' && th.css && !document.querySelector('link[data-addon-theme]')) {
      const l = document.createElement('link');
      l.rel = 'stylesheet'; l.dataset.addonTheme = '1'; l.href = th.css;
      document.head.appendChild(l);
    }
    let st = $('ap-custom-css');
    const css = policy().custom_css ? String(prefs.custom_css || '') : '';
    if (css) {
      if (!st) { st = document.createElement('style'); st.id = 'ap-custom-css'; document.head.appendChild(st); }
      st.textContent = css;
    } else if (st) st.remove();
    // terminals
    if (typeof TERM_THEME !== 'undefined') {
      Object.keys(TERM_THEME).forEach(k => delete TERM_THEME[k]);
      Object.assign(TERM_THEME, term);
    }
    try {
      (typeof winObjects !== 'undefined' ? winObjects : []).forEach(wo => {
        if (!wo.term) return;
        wo.term.options.theme = Object.assign({}, term);
        wo.term.options.fontFamily = termFont() || wo.term.options.fontFamily;
        wo.term.options.minimumContrastRatio = termContrast();
      });
    } catch (_) { /* a terminal that is closing */ }
    store(prefs);
    window.dispatchEvent(new CustomEvent('wrm:theme', {detail: {id: th.id, light}}));
  }

  function termFont() { return prefs.font_mono ? safeValue(prefs.font_mono) : ''; }
  function termContrast() { const v = parseFloat(prefs.term_contrast); return v >= 1 && v <= 21 ? v : 1; }

  // ── preferences ───────────────────────────────────
  let saveTimer = null;
  function save(p) {
    prefs = Object.assign({}, prefs, p);
    apply();
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      fetch('/api/me/ui/appearance', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({value: prefs})})
        .then(r => { if (!r.ok && r.status !== 423) r.json().then(d => showToast(d.error || 'Error', 'error')).catch(() => {}); }).catch(() => {});
    }, 400);
  }

  async function load() {
    try {
      const r = await fetch('/api/me/ui/appearance');
      if (r.ok) {
        const d = await r.json();
        if (d && d.value) prefs = Object.assign({}, DEFAULTS, d.value);
        else if (typeof settings !== 'undefined' && /^#[0-9a-f]{6}$/i.test(settings.accent || '') && settings.accent.toLowerCase() !== '#4f8cff' && !(prefs.overrides || {}).accent) {
          // The accent colour of Settings → General before v13.7 becomes an override once.
          prefs.overrides = Object.assign({}, prefs.overrides, {accent: settings.accent});
        }
      }
    } catch (_) { /* offline: the cached copy stays */ }
    apply();
  }

  // ── addon themes (addons.js hands the enabled list over) ──
  function setAddonThemes(list) {
    addonThemes = (list || []).filter(x => x && x.id).map(x => {
      const tokens = {};
      Object.entries(x.tokens || {}).forEach(([k, v]) => {
        const name = String(k).replace(/^--/, '');
        if (/^[a-z0-9-]{1,40}$/.test(name) && safeValue(v)) tokens[name] = safeValue(v);
      });
      return {id: x.id, name: x.name, name_hr: x.name_hr, mode: x.mode === 'light' ? 'light' : 'dark', tokens, css: x.css || '', addon: x.addon, personal: !!x.personal};
    });
    apply();
    const pane = $('appearance-pane');
    if (pane && pane.offsetParent) render();
  }

  // ── Settings → Appearance ─────────────────────────
  function swatches(th) {
    const {vars} = tokensFor(Object.assign({}, th, {}));
    const c = k => esc(vars['--' + k] || '#000');
    return `<div class="ap-sw" style="background:${c('bg')};border-color:${c('border2')};">
      <div style="background:${c('bg2')};border-bottom:1px solid ${c('border')};height:9px;"></div>
      <div style="display:flex;gap:3px;padding:5px;">
        <span style="flex:1;height:6px;border-radius:3px;background:${c('text')};opacity:.85;"></span>
        <span style="width:14px;height:6px;border-radius:3px;background:${c('accent')};"></span></div>
      <div style="display:flex;gap:3px;padding:0 5px 5px;">
        <i style="background:${c('green')}"></i><i style="background:${c('yellow')}"></i><i style="background:${c('red')}"></i><i style="background:${c('purple')}"></i></div></div>`;
  }

  function card(th, cur) {
    const def = th.id === policy().default_theme;
    return `<button type="button" class="ap-card ${th.id === cur ? 'active' : ''}" data-theme="${esc(th.id)}" aria-pressed="${th.id === cur}">
      ${swatches(th)}
      <span class="ap-card-n">${esc(themeName(th))}</span>
      <span class="ap-card-s">${esc(th.mode === 'light' ? t('ap_light') : t('ap_dark'))}${def ? ' · ' + esc(t('ap_default')) : ''}</span></button>`;
  }

  function option(list, val) {
    return list.map(o => `<option value="${esc(String(o[0]))}" ${String(o[0]) === String(val) ? 'selected' : ''}>${esc(o[1])}</option>`).join('');
  }
  function sw(id, on) { return `<label class="switch"><input type="checkbox" id="${id}" ${on ? 'checked' : ''}><span></span></label>`; }
  function row(label, desc, ctl) {
    return `<div class="setting-row"><div><div class="setting-label">${esc(label)}</div>${desc ? `<div class="setting-desc">${esc(desc)}</div>` : ''}</div><div class="setting-control">${ctl}</div></div>`;
  }

  function tokenRow(k, cur) {
    const names = lang() === 'hr' ? TOKEN_NAMES_HR : TOKEN_NAMES;
    const label = k.startsWith('term-') ? 'Terminal ' + k.slice(5) : k.endsWith('-t') ? (names[k.slice(0, -2)] || k) + ' (' + (lang() === 'hr' ? 'tekst' : 'text') + ')' : (names[k] || k);
    const own = (prefs.overrides || {})[k];
    const hex = rgbToHex(hexToRgb(cur) || [0, 0, 0]);
    return `<div class="ap-tok ${own ? 'own' : ''}"><input type="color" value="${esc(hex)}" data-tok="${esc(k)}" aria-label="${esc(label)}">
      <span class="ap-tok-n" title="--${esc(k)}">${esc(label)}</span>
      <input type="text" value="${esc(own || '')}" placeholder="${esc(hex)}" data-tok-t="${esc(k)}" spellcheck="false" aria-label="${esc(label)} (CSS)">
      ${own ? `<button type="button" class="icon-btn" data-tok-reset="${esc(k)}" title="${esc(t('ap_reset_token'))}" aria-label="${esc(t('ap_reset_token'))}">↺</button>` : '<span style="width:22px"></span>'}</div>`;
  }

  function render() {
    const pane = $('appearance-pane');
    if (!pane) return;
    const pol = policy();
    const cur = prefs.follow_os ? '' : activeId();
    const th = activeTheme();
    const {vars, term} = tokensFor(th);
    const val = k => k.startsWith('term-') ? term[k.slice(5)] : vars['--' + k];
    const groups = [['ap_builtin', allThemes().filter(x => x.source === 'builtin')], ['ap_from_addons', allThemes().filter(x => x.source === 'addon')], ['ap_mine', allThemes().filter(x => x.source === 'mine')]];
    const themeOpts = mode => allThemes().filter(x => x.mode === mode).map(x => [x.id, themeName(x)]);
    const isAdmin = typeof currentUser !== 'undefined' && currentUser && currentUser.is_admin;
    const fontOpts = (list, v) => option(list.concat(v && !list.some(o => o[0] === v) ? [[v, v]] : []).concat([['__custom', t('ap_font_custom')]]), v);
    pane.innerHTML = `
      <div class="ap-wrap">
      <div class="ap-main">
        <div class="adm-h">${esc(t('ap_themes'))}</div>
        ${row(t('ap_follow_os'), t('ap_follow_os_d'), sw('ap-follow', prefs.follow_os))}
        ${prefs.follow_os ? row(t('ap_dark_theme'), '', `<select class="setting-select" id="ap-dark">${option(themeOpts('dark'), prefs.dark_theme)}</select>`) +
          row(t('ap_light_theme'), '', `<select class="setting-select" id="ap-light">${option(themeOpts('light'), prefs.light_theme)}</select>`) : ''}
        ${groups.filter(g => g[1].length).map(g => `<div class="ap-gh">${esc(t(g[0]))}</div><div class="ap-grid">${g[1].map(x => card(x, cur)).join('')}</div>`).join('')}
        <div class="ap-actions">
          <button class="btn-sec btn-sm" id="ap-save-as">💾 ${esc(t('ap_save_as'))}</button>
          <button class="btn-sec btn-sm" id="ap-export">⬇ ${esc(t('ap_export'))}</button>
          <button class="btn-sec btn-sm" id="ap-import">⬆ ${esc(t('ap_import'))}</button>
          ${th.source === 'mine' ? `<button class="btn-sec btn-sm" id="ap-del">🗑 ${esc(t('ap_delete_theme'))}</button>` : ''}
          ${isAdmin && th.id !== pol.default_theme && th.source !== 'mine' && !th.personal ? `<button class="btn-sec btn-sm" id="ap-admin-default" title="${esc(t('ap_admin_default_d'))}">🛡 ${esc(t('ap_admin_default'))}</button>` : ''}
          <button class="btn-sec btn-sm" id="ap-reset">↺ ${esc(t('ap_reset_all'))}</button>
          <input type="file" id="ap-import-file" accept=".json,application/json" style="display:none">
        </div>

        <div class="adm-h">${esc(t('ap_a11y'))}</div>
        ${row(t('ap_cvd'), t('ap_cvd_d'), `<select class="setting-select" id="ap-cvd">${option([['none', t('ap_cvd_none')], ['deutan', t('ap_cvd_deutan')], ['protan', t('ap_cvd_protan')], ['tritan', t('ap_cvd_tritan')], ['achroma', t('ap_cvd_achroma')]], prefs.cvd)}</select>`)}
        ${prefs.cvd !== 'none' && prefs.cvd !== 'achroma' ? row(t('ap_cvd_term'), t('ap_cvd_term_d'), sw('ap-cvd-term', prefs.cvd_term)) : ''}
        ${row(t('ap_symbols'), t('ap_symbols_d'), sw('ap-symbols', prefs.symbols || prefs.cvd === 'achroma'))}
        ${row(t('ap_contrast'), '', `<select class="setting-select" id="ap-contrast">${option([['normal', t('ap_contrast_normal')], ['more', t('ap_contrast_more')], ['max', t('ap_contrast_max')]], prefs.contrast)}</select>`)}
        ${row(t('ap_term_contrast'), t('ap_term_contrast_d'), `<select class="setting-select" id="ap-term-contrast">${option([[1, t('ap_term_contrast_off')], [3, '3 : 1'], [4.5, '4.5 : 1 (WCAG AA)'], [7, '7 : 1 (WCAG AAA)']], prefs.term_contrast)}</select>`)}
        ${row(t('ap_motion'), '', `<select class="setting-select" id="ap-motion">${option([['os', t('ap_motion_os')], ['reduce', t('ap_motion_reduce')], ['full', t('ap_motion_full')]], prefs.motion)}</select>`)}
        ${row(t('ap_transparency'), t('ap_transparency_d'), sw('ap-solid', prefs.solid))}
        ${row(t('ap_focus'), t('ap_focus_d'), sw('ap-focus', prefs.focus))}
        ${row(t('ap_underline'), '', sw('ap-underline', prefs.underline))}

        <div class="adm-h">${esc(t('ap_fonts'))}</div>
        ${row(t('ap_font_ui'), '', `<select class="setting-select" id="ap-font-ui">${fontOpts(FONTS_UI, prefs.font_ui)}</select>`)}
        ${row(t('ap_font_mono'), '', `<select class="setting-select" id="ap-font-mono">${fontOpts(FONTS_MONO, prefs.font_mono)}</select>`)}
        ${row(t('ap_ui_size'), t('ap_ui_size_d'), `<input type="range" id="ap-size" min="70" max="160" step="5" value="${prefs.ui_size}" aria-label="${esc(t('ap_ui_size'))}"> <b id="ap-size-v" style="min-width:42px;display:inline-block;text-align:right;">${prefs.ui_size}%</b>`)}
        ${row(t('ap_density'), '', `<select class="setting-select" id="ap-density">${option([['compact', t('ap_density_compact')], ['normal', t('ap_density_normal')], ['roomy', t('ap_density_roomy')]], prefs.density)}</select>`)}
        ${row(t('ap_radius'), '', `<input type="range" id="ap-radius" min="0" max="18" step="1" value="${prefs.radius}" aria-label="${esc(t('ap_radius'))}"> <b id="ap-radius-v" style="min-width:42px;display:inline-block;text-align:right;">${prefs.radius}px</b>`)}
        ${row(t('ap_line_height'), '', `<select class="setting-select" id="ap-line">${option([[0, t('ap_contrast_normal')], [1.5, '1.5'], [1.75, '1.75'], [2, '2']], prefs.line_height)}</select>`)}
        ${row(t('ap_letter'), '', `<select class="setting-select" id="ap-letter">${option([[0, t('ap_contrast_normal')], [0.03, '+3 %'], [0.06, '+6 %'], [0.12, '+12 %']], prefs.letter)}</select>`)}
        ${row(t('ap_weight'), '', sw('ap-weight', prefs.weight))}

        <div class="adm-h">${esc(t('ap_customize'))}</div>
        <div class="setting-desc" style="margin-bottom:8px;">${esc(t('ap_customize_d'))}</div>
        ${GROUPS.map(g => `<div class="ap-gh">${esc(t(g[0]))}</div><div class="ap-toks">${g[1].map(k => tokenRow(k, val(k))).join('')}</div>`).join('')}

        <div class="adm-h">${esc(t('ap_custom_css'))}</div>
        ${pol.custom_css ? `<div class="setting-desc" style="margin-bottom:6px;">${esc(t('ap_custom_css_d'))}</div>
          <textarea id="ap-css" spellcheck="false" rows="6" style="width:100%;font-family:var(--font-mono);font-size:12px;" placeholder=".conn-name { font-weight: 600; }">${esc(prefs.custom_css || '')}</textarea>`
          : `<div class="setting-desc">${esc(t('ap_custom_css_off'))}</div>`}
      </div>
      <aside class="ap-preview" aria-label="${esc(t('ap_preview'))}">
        <div class="ap-gh">${esc(t('ap_preview'))}</div>
        <div class="ap-pv">
          <div style="color:var(--text);font-weight:600;">${esc(t('ap_pv_text'))}</div>
          <div style="color:var(--text2);">${esc(t('ap_pv_muted'))} · <a href="#" onclick="return false;">${esc(t('ap_pv_link'))}</a></div>
          <div style="color:var(--text3);">${esc(t('ap_pv_faint'))}</div>
          <div style="display:flex;gap:6px;flex-wrap:wrap;margin:8px 0;"><span class="pill ok">${esc(t('ap_pv_ok'))}</span><span class="pill warn">${esc(t('ap_pv_warn'))}</span><span class="pill bad">${esc(t('ap_pv_bad'))}</span><span class="pill info">Info</span></div>
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;"><span class="conn-st up"></span>web-01 <span class="conn-st slow"></span>db-01 <span class="conn-st down"></span>mail-01</div>
          <div style="font-family:var(--font-mono);font-size:11.5px;border:1px solid var(--border);border-radius:6px;overflow:hidden;">
            <div style="background:rgba(var(--green-rgb),.13);color:var(--green-t);padding:2px 6px;">${esc(t('ap_pv_add'))}</div>
            <div style="background:rgba(var(--red-rgb),.13);color:var(--red-t);padding:2px 6px;">${esc(t('ap_pv_del'))}</div></div>
          <div style="display:flex;gap:6px;margin:8px 0;"><button class="btn-sm" type="button">${esc(t('ap_pv_button'))}</button><button class="btn-sec btn-sm" type="button">${esc(t('ap_pv_second'))}</button><button class="btn-danger btn-sm" type="button">✕</button></div>
          <div class="ap-term" style="background:${esc(term.background)};color:${esc(term.foreground)};">
            <div>$ <span style="color:${esc(term.green)}">systemctl</span> status nginx</div>
            <div><span style="color:${esc(term.green)}">●</span> active (running) <span style="color:${esc(term.yellow)}">warn</span> <span style="color:${esc(term.red)}">error</span></div>
            <div><span style="color:${esc(term.blue)}">blue</span> <span style="color:${esc(term.magenta)}">magenta</span> <span style="color:${esc(term.cyan)}">cyan</span> <span style="color:${esc(term.brightBlack)}">dim</span></div></div>
        </div>
      </aside>
      </div>`;
    wire(pane);
  }

  function wire(pane) {
    const on = (id, ev, fn) => { const el = $(id); if (el) el.addEventListener(ev, fn); };
    pane.querySelectorAll('.ap-card').forEach(b => b.addEventListener('click', () => { save({theme: b.dataset.theme, follow_os: false}); render(); }));
    on('ap-follow', 'change', e => { save({follow_os: e.target.checked}); render(); });
    on('ap-dark', 'change', e => { save({dark_theme: e.target.value}); render(); });
    on('ap-light', 'change', e => { save({light_theme: e.target.value}); render(); });
    on('ap-cvd', 'change', e => { save({cvd: e.target.value}); render(); });
    on('ap-cvd-term', 'change', e => save({cvd_term: e.target.checked}));
    on('ap-symbols', 'change', e => save({symbols: e.target.checked}));
    on('ap-contrast', 'change', e => { save({contrast: e.target.value}); render(); });
    on('ap-term-contrast', 'change', e => save({term_contrast: parseFloat(e.target.value)}));
    on('ap-motion', 'change', e => save({motion: e.target.value}));
    on('ap-solid', 'change', e => save({solid: e.target.checked}));
    on('ap-focus', 'change', e => save({focus: e.target.checked}));
    on('ap-underline', 'change', e => save({underline: e.target.checked}));
    on('ap-weight', 'change', e => save({weight: e.target.checked}));
    on('ap-density', 'change', e => save({density: e.target.value}));
    on('ap-line', 'change', e => save({line_height: parseFloat(e.target.value) || 0}));
    on('ap-letter', 'change', e => save({letter: parseFloat(e.target.value) || 0}));
    on('ap-size', 'input', e => { $('ap-size-v').textContent = e.target.value + '%'; });
    on('ap-size', 'change', e => save({ui_size: parseInt(e.target.value, 10)}));
    on('ap-radius', 'input', e => { $('ap-radius-v').textContent = e.target.value + 'px'; save({radius: parseInt(e.target.value, 10)}); });
    const font = key => async e => {
      let v = e.target.value;
      if (v === '__custom') {
        const typed = await uiPrompt(t('ap_font_custom'), prefs[key] || '', {message: t('ap_font_custom_q')});
        if (typed === null || typed === undefined) { render(); return; }
        v = safeValue(typed.trim());
      }
      save({[key]: v}); render();
    };
    on('ap-font-ui', 'change', font('font_ui'));
    on('ap-font-mono', 'change', font('font_mono'));
    on('ap-css', 'change', e => save({custom_css: e.target.value.slice(0, 20000)}));
    const setTok = (k, v) => {
      const ov = Object.assign({}, prefs.overrides);
      v = safeValue(String(v || '').trim());
      if (v) ov[k] = v; else delete ov[k];
      save({overrides: ov});
    };
    pane.querySelectorAll('input[data-tok]').forEach(i => {
      i.addEventListener('input', () => setTok(i.dataset.tok, i.value));
      i.addEventListener('change', () => render());
    });
    pane.querySelectorAll('input[data-tok-t]').forEach(i => i.addEventListener('change', () => { setTok(i.dataset.tokT, i.value); render(); }));
    pane.querySelectorAll('[data-tok-reset]').forEach(b => b.addEventListener('click', () => { setTok(b.dataset.tokReset, ''); render(); }));
    on('ap-reset', 'click', async () => {
      if (!(await uiConfirm(t('ap_reset_q'), {okText: t('ap_reset_all')}))) return;
      save(Object.assign({}, DEFAULTS, {theme: prefs.theme, follow_os: prefs.follow_os, dark_theme: prefs.dark_theme, light_theme: prefs.light_theme, mine: prefs.mine}));
      render();
    });
    on('ap-save-as', 'click', async () => {
      const th = activeTheme();
      const name = ((await uiPrompt(t('ap_save_as'), themeName(th) + ' *', {message: t('ap_save_name')})) || '').trim().slice(0, 60);
      if (!name) return;
      const {vars, term} = tokensFor(Object.assign({}, th));
      const tokens = {};
      GROUPS.forEach(g => g[1].forEach(k => { if (!k.startsWith('term-') && vars['--' + k]) tokens[k] = vars['--' + k]; }));
      const termOut = {};
      Object.keys(TERM_DARK).concat(['background', 'foreground']).forEach(k => { if (term[k]) termOut[k] = term[k]; });
      const id = 'my-' + (Date.now().toString(36)).slice(-8);
      const mine = (prefs.mine || []).concat([{id, name, mode: th.mode, tokens, term: termOut}]).slice(-30);
      save({mine, theme: id, follow_os: false, overrides: {}});
      showToast(t('ap_saved'), 'success');
      render();
    });
    on('ap-del', 'click', async () => {
      const th = activeTheme();
      if (!(await uiConfirm(tf('ap_delete_q', {name: themeName(th)}), {okText: t('ap_delete_theme')}))) return;
      save({mine: (prefs.mine || []).filter(x => x.id !== th.id), theme: ''});
      render();
    });
    on('ap-export', 'click', () => {
      const th = activeTheme();
      const {vars, term} = tokensFor(Object.assign({}, th));
      const tokens = {};
      GROUPS.forEach(g => g[1].forEach(k => { if (!k.startsWith('term-') && vars['--' + k]) tokens['--' + k] = vars['--' + k]; }));
      const out = {wrm_theme: 1, id: th.id.replace(/^my-/, 'custom-'), name: themeName(th), mode: th.mode, tokens, term};
      const a = document.createElement('a');
      a.href = URL.createObjectURL(new Blob([JSON.stringify(out, null, 2)], {type: 'application/json'}));
      a.download = 'wrm-theme-' + out.id + '.json';
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 2000);
    });
    on('ap-import', 'click', () => $('ap-import-file').click());
    on('ap-import-file', 'change', async e => {
      const f = e.target.files && e.target.files[0];
      if (!f) return;
      try {
        const d = JSON.parse(await f.text());
        if (!d || !d.tokens || typeof d.tokens !== 'object') throw new Error();
        const tokens = {}, term = {};
        Object.entries(d.tokens).forEach(([k, v]) => { const n = String(k).replace(/^--/, ''); if (/^[a-z0-9-]{1,40}$/.test(n) && safeValue(v)) tokens[n] = safeValue(v); });
        Object.entries(d.term || {}).forEach(([k, v]) => { if (/^[a-zA-Z]{1,30}$/.test(k) && safeValue(v)) term[k] = safeValue(v); });
        const id = 'my-' + Date.now().toString(36).slice(-8);
        const mine = (prefs.mine || []).concat([{id, name: String(d.name || 'Imported').slice(0, 60), mode: d.mode === 'light' ? 'light' : 'dark', tokens, term}]).slice(-30);
        save({mine, theme: id, follow_os: false, overrides: {}});
        showToast(t('ap_imported'), 'success');
        render();
      } catch (_) { showToast(t('ap_import_bad'), 'error'); }
    });
    on('ap-admin-default', 'click', async () => {
      const th = activeTheme();
      const r = await fetch('/api/admin/settings', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({appearance_default_theme: th.id})});
      if (r.ok) {
        if (currentUser.appearance) currentUser.appearance.default_theme = th.id;
        showToast(tf('ap_admin_default_set', {name: themeName(th)}), 'success');
        render();
      } else showToast(await apiError(r), 'error');
    });
  }

  if (mq && mq.addEventListener) mq.addEventListener('change', () => { if (prefs.follow_os) { apply(); const p = $('appearance-pane'); if (p && p.offsetParent) render(); } });
  window.addEventListener('storage', e => { if (e.key === CACHE) { prefs = loadLocal(); apply(); } });

  window.wrmAppearance = {apply, render, load, setAddonThemes, termFont, termContrast, themes: () => allThemes().map(x => ({id: x.id, name: themeName(x), mode: x.mode, source: x.source})), prefs: () => Object.assign({}, prefs)};

  // Paint the cached choice now, then load the signed-in user's choice with the app.
  apply();
  if (typeof window.initApp === 'function' && !window.initApp.__appearance) {
    const orig = window.initApp;
    window.initApp = function () {
      const r = orig.apply(this, arguments);
      load();
      return r;
    };
    window.initApp.__appearance = true;
  }
})();
