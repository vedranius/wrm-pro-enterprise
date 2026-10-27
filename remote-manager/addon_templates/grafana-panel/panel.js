/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

(function () {
  'use strict';
  // The Grafana this addon reads; it must equal http.origins in addon.json.
  const GRAFANA = 'https://grafana.example.com';
  const EVERY = 30;
  const $ = (id) => document.getElementById(id);
  let timer = null;
  let seen = null;

  async function get(path) {
    const res = await wrm.http({method: 'GET', url: GRAFANA + path, headers: {Accept: 'application/json'}, secret: 'grafana_token'});
    if (res.status >= 400) throw Object.assign(new Error('HTTP ' + res.status), {code: 'E_HTTP_STATUS'});
    return JSON.parse(res.body);
  }

  function row(t, cells, cls) {
    const tr = t.insertRow();
    cells.forEach((c) => {
      const td = tr.insertCell();
      td.textContent = c;
      if (cls) td.className = cls;
    });
  }

  async function load() {
    const ctx = wrm.context;
    if (!ctx || !wrm.visible) return;
    if (ctx.http && !ctx.http.approved) {
      $('state').textContent = wrm.t('Waiting for an administrator to approve ', 'Čeka se da administrator odobri ') + GRAFANA;
      return;
    }
    if (ctx.http && ctx.http.ready && !ctx.http.ready.grafana_token) {
      $('state').textContent = wrm.t('Enter the service account token in WRM: Settings → Addons → Instances → Secrets.',
        'Unesite token servisnog računa u WRM: Postavke → Dodaci → Instance → Tajne.');
      return;
    }
    try {
      const h = await get('/api/health');
      $('health').textContent = (h.version || '') + ' · ' + wrm.t('database ', 'baza ') + (h.database || '?');
      const alerts = (await get('/api/alertmanager/grafana/api/v2/alerts')).filter((a) => a.status && a.status.state === 'active');
      const t = $('alerts');
      t.textContent = '';
      if (!alerts.length) row(t, [wrm.t('No firing alerts', 'Nema aktivnih alarma')], 'ok');
      alerts.slice(0, 100).forEach((a) => row(t, [(a.labels && a.labels.alertname) || '?', (a.labels && a.labels.severity) || '', a.startsAt ? new Date(a.startsAt).toLocaleString() : ''], 'bad'));
      const names = new Set(alerts.map((a) => a.fingerprint));
      if (seen) {
        const fresh = alerts.filter((a) => !seen.has(a.fingerprint));
        if (fresh.length) wrm.notify(wrm.t('New alert: ', 'Novi alarm: ') + ((fresh[0].labels && fresh[0].labels.alertname) || '?'), 'warning');
      }
      seen = names;
      $('state').textContent = wrm.t('Updated ', 'Osvježeno ') + new Date().toLocaleTimeString();
      $('state').className = 'muted';
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
      timer = setInterval(load, EVERY * 1000);
    }
  }

  wrm.on('context', schedule);
  wrm.on('visibility', schedule);
  wrm.ready();
})();
