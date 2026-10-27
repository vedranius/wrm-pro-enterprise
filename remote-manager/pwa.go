// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
)

// ─── INSTALLABLE APP (PWA) ───────────────────────────
//
// WRM can be installed as an app (desktop and mobile): a web app manifest and a small
// service worker. The service worker only caches WRM's own static files (scripts, fonts,
// icons) per version, so the app window opens fast; everything else — the page itself, the
// API and the WebSockets — always goes to the server. Browsers install apps only over
// HTTPS (or on localhost).

func manifestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(jsonMarshal(map[string]interface{}{
		"name":             "WRM PRO — Web Remote Manager",
		"short_name":       "WRM",
		"description":      "SSH, SFTP, RDP and VNC to your servers in the browser",
		"id":               "/",
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"display_override": []string{"window-controls-overlay", "standalone"},
		"background_color": "#0b0f14",
		"theme_color":      "#0f1419",
		"categories":       []string{"developer", "utilities", "productivity"},
		"icons": []map[string]string{
			{"src": "/static/brand/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": "/static/brand/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
			{"src": "/static/brand/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
			{"src": "/static/brand/favicon.svg", "sizes": "any", "type": "image/svg+xml"},
		},
	}))
}

const serviceWorkerJS = `// WRM service worker: caches WRM's static files per version; pages, API and WebSockets always go to the server.
const CACHE = 'wrm-%VERSION%';
const PRECACHE = ['/static/vendor/xterm.js', '/static/vendor/xterm.css', '/static/vendor/fonts.css', '/static/brand/icon-192.png', '/static/brand/favicon.svg', '/static/brand/mark-on-dark.svg'];
self.addEventListener('install', e => {
  e.waitUntil(caches.open(CACHE).then(c => c.addAll(PRECACHE)).catch(() => {}).then(() => self.skipWaiting()));
});
self.addEventListener('activate', e => {
  e.waitUntil(caches.keys().then(keys => Promise.all(keys.filter(k => k.startsWith('wrm-') && k !== CACHE).map(k => caches.delete(k)))).then(() => self.clients.claim()));
});
self.addEventListener('fetch', e => {
  const req = e.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;
  if (url.pathname.startsWith('/static/')) {
    e.respondWith(caches.open(CACHE).then(c => c.match(req).then(hit => hit || fetch(req).then(res => { if (res.ok) c.put(req, res.clone()); return res; }))));
    return;
  }
  if (req.mode === 'navigate') {
    e.respondWith(fetch(req).catch(() => new Response('<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>WRM</title><body style="background:#0b0f14;color:#cbd5e1;font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;text-align:center"><div><h2>WRM is not reachable</h2><p>The app needs a connection to your WRM server.</p><button onclick="location.reload()">Try again</button></div>', {headers: {'Content-Type': 'text/html; charset=utf-8'}})));
  }
});
`

func serviceWorkerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(strings.ReplaceAll(serviceWorkerJS, "%VERSION%", AppVersion)))
}
