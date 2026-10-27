#!/usr/bin/env node

/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// Checks the embedded web UI (remote-manager/static/index.html):
//   - JavaScript syntax of the main inline script
//   - every translation key used by t('…') and data-i18n* exists in English and Croatian
//   - both languages have the same keys
// Usage: node tools/check-ui.js [path/to/index.html]     (exit code 1 on problems)
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const file = process.argv[2] || path.join(__dirname, '..', 'remote-manager', 'static', 'index.html');
const html = fs.readFileSync(file, 'utf8');
const scripts = [...html.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g)].map(m => m[1]);
if (!scripts.length) { console.error('no inline script found'); process.exit(1); }
let js = scripts.reduce((a, b) => (b.length > a.length ? b : a));

let problems = 0;
try { new vm.Script(js, { filename: 'index.html <script>' }); }
catch (e) { console.error('JavaScript syntax error:', e.message); process.exit(1); }
// Scripts loaded on demand (static/git.js, static/ai.js, static/mcp.js): syntax, and their translations and keys are checked with the page.
for (const extra of ['git.js', 'ai.js', 'mcp.js', 'monitoring.js', 'git_client.js', 'layout.js', 'workboard.js', 'logviewer.js', 'addons.js', 'lock.js', 'appearance.js', 'aichat.js', 'fileeditor.js', 'aisessions.js']) {
  const f = path.join(path.dirname(file), extra);
  if (!fs.existsSync(f)) continue;
  const src = fs.readFileSync(f, 'utf8');
  try { new vm.Script(src, { filename: extra }); }
  catch (e) { console.error(`JavaScript syntax error in ${extra}:`, e.message); process.exit(1); }
  js += '\n' + src;
}

// Evaluate only the translation tables: the LANGS literal and the later Object.assign(LANGS.xx, {…}) blocks.
const start = js.indexOf('const LANGS = {');
const end = js.indexOf('function t(k)');
if (start < 0 || end < 0) { console.error('LANGS or t() not found'); process.exit(1); }
const ctx = { localStorage: { getItem() { return null; }, setItem() {} } };
vm.runInNewContext(js.slice(start, end).replace('const LANGS', 'var LANGS'), ctx);
const L = ctx.LANGS;
const re = /Object\.assign\(LANGS\.(en|hr), \{/g;
let m;
while ((m = re.exec(js))) {
  if (m.index < end) continue;
  let i = js.indexOf('{', m.index + 18), depth = 0, k = i;
  for (; k < js.length; k++) {
    const c = js[k];
    if (c === '{') depth++;
    else if (c === '}') { if (--depth === 0) break; }
    else if (c === '"' || c === "'" || c === '`') { const q = c; k++; while (js[k] !== q) { if (js[k] === '\\') k++; k++; } }
  }
  Object.assign(L[m[1]], vm.runInNewContext('(' + js.slice(i, k + 1) + ')'));
}

const used = new Set();
for (const mm of js.matchAll(/\b(?:t|tf)\(\s*(['"])([a-zA-Z0-9_]+)\1\s*[,)]/g)) used.add(mm[2]);
for (const mm of html.matchAll(/data-i18n(?:-[a-z]+)?="([a-zA-Z0-9_]+)"/g)) used.add(mm[1]);
for (const lang of ['en', 'hr']) {
  const missing = [...used].filter(k => !(k in L[lang]));
  if (missing.length) { problems++; console.error(`missing in ${lang}: ${missing.join(' ')}`); }
}
const onlyEn = Object.keys(L.en).filter(k => !(k in L.hr));
const onlyHr = Object.keys(L.hr).filter(k => !(k in L.en));
if (onlyEn.length) { problems++; console.error('only in en: ' + onlyEn.join(' ')); }
if (onlyHr.length) { problems++; console.error('only in hr: ' + onlyHr.join(' ')); }
console.log(`UI check: ${(js.length / 1024).toFixed(0)} KB script, ${Object.keys(L.en).length} keys per language, ${used.size} used — ${problems ? 'PROBLEMS' : 'ok'}`);
process.exit(problems ? 1 : 0);
