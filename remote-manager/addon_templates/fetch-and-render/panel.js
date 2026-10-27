/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

(function () {
  'use strict';
  const ORIGIN = 'https://status.example.com';
  const $ = (id) => document.getElementById(id);
  let timer = null;
  let cfg = {path: '/api/status', every: 60};

  function cell(row, text, tag) {
    const c = document.createElement(tag || 'td');
    c.textContent = text;
    row.appendChild(c);
  }

  // Flattens an object or an array of objects into rows of key/value pairs.
  function render(data) {
    const t = $('out');
    t.textContent = '';
    const rows = Array.isArray(data) ? data : Object.entries(data || {}).map(([k, v]) => ({name: k, value: v}));
    if (!rows.length) return;
    const keys = Object.keys(typeof rows[0] === 'object' && rows[0] !== null ? rows[0] : {value: 0}).slice(0, 6);
    const head = t.insertRow();
    keys.forEach((k) => cell(head, k, 'th'));
    rows.slice(0, 200).forEach((r) => {
      const tr = t.insertRow();
      keys.forEach((k) => {
        const v = r && typeof r === 'object' ? r[k] : r;
        cell(tr, v !== null && typeof v === 'object' ? JSON.stringify(v) : String(v));
      });
    });
  }

  async function load() {
    const ctx = wrm.context;
    if (!ctx || !wrm.visible) return;
    if (ctx.http && !ctx.http.approved) {
      $('state').textContent = wrm.t('Waiting for an administrator to approve ', 'Čeka se da administrator odobri ') + ORIGIN;
      return;
    }
    const req = {method: 'GET', url: ORIGIN + cfg.path, headers: {Accept: 'application/json'}};
    if (ctx.http && ctx.http.ready && ctx.http.ready.api_token) req.secret = 'api_token';
    try {
      const res = await wrm.http(req);
      $('state').className = res.status < 400 ? 'muted' : 'bad';
      $('state').textContent = 'HTTP ' + res.status + ' · ' + new Date().toLocaleTimeString();
      if (res.encoding === 'text' && /json/.test(res.headers['Content-Type'] || '')) render(JSON.parse(res.body));
    } catch (e) {
      $('state').className = 'bad';
      $('state').textContent = (e.code || 'error') + ': ' + e.message;
    }
  }

  function schedule() {
    clearInterval(timer);
    timer = null;
    if (wrm.visible) {
      load();
      timer = setInterval(load, cfg.every * 1000);
    }
  }

  wrm.on('context', async () => {
    cfg = await wrm.storeGet('config', cfg);
    $('path').value = cfg.path;
    $('every').value = String(cfg.every);
    $('apply').textContent = wrm.t('Apply', 'Primijeni');
    schedule();
  });
  // Poll only while the window is visible.
  wrm.on('visibility', schedule);

  $('settings').addEventListener('submit', async (e) => {
    e.preventDefault();
    const p = $('path').value.trim();
    cfg = {path: p.startsWith('/') ? p : '/' + p, every: Number($('every').value) || 60};
    await wrm.storeSet('config', cfg).catch((err) => wrm.error(err.message));
    schedule();
  });

  wrm.ready();
})();
