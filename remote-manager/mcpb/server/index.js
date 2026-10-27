#!/usr/bin/env node

/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// WRM PRO stdio bridge for Claude (MCP bundle, .mcpb).
//
// Claude Desktop, Cowork and Claude Code start local MCP servers over stdio. This file is the
// server of the "WRM PRO" desktop extension that WRM generates in Settings → AI connections,
// and of the WRM PRO Claude plugin: it reads newline-delimited JSON-RPC messages from stdin,
// POSTs each one to WRM's /mcp endpoint with the bearer token, and writes every message of the
// answer (JSON or an event stream, including WRM's approval questions) to stdout, one per
// line. It mirrors `wrm -mcp-stdio` (mcp_stdio.go) so the user needs no WRM program on the
// computer: Claude Desktop ships the Node.js runtime this file needs. No dependencies; only
// protocol messages go to stdout, diagnostics go to stderr.
//
// Where the WRM address and token come from, first match wins:
//   1. WRM_MCP_URL and WRM_MCP_TOKEN (the extension's settings), optional WRM_MCP_CA_FILE
//      (PEM file to trust a private CA or a self-signed WRM certificate);
//   2. the connection file (WRM_MCP_CONFIG, default ~/.wrm/mcp.json: {"url", "token",
//      "ca_file"}), written by the wrm_connect tool or by WRM's setup script for all AI apps.
// With neither, or when WRM refuses the token, the bridge still starts and offers one tool,
// wrm_connect: given the WRM address it opens WRM's sign-in in the browser (OAuth with PKCE
// and a loopback redirect), stores the token it receives in the connection file and then
// forwards everything to WRM. The token never passes through the chat.
'use strict';

const http = require('http');
const https = require('https');
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const readline = require('readline');
const childProcess = require('child_process');

const VERSION = '0.0.0'; // replaced with the WRM version when the bundle is generated
const MAX_BODY = 32 << 20;
const CONNECT_WAIT_MS = Number(process.env.WRM_MCP_CONNECT_WAIT_MS) || 90000;
const LOGIN_TTL_MS = 15 * 60 * 1000;
const CONFIG_FILE = clean(process.env.WRM_MCP_CONFIG) || path.join(os.homedir(), '.wrm', 'mcp.json');

function clean(v) {
  v = String(v || '').trim();
  return v.startsWith('${') ? '' : v; // an optional setting the app left unfilled
}
function log(text) { process.stderr.write('wrm-mcp-bridge: ' + text + '\n'); }
function write(obj) { process.stdout.write((typeof obj === 'string' ? obj.trim() : JSON.stringify(obj)) + '\n'); }
function rpcError(id, message, code) { return {jsonrpc: '2.0', id: id === undefined ? null : id, error: {code: code || -32000, message}}; }
function rpcResult(id, result) { return {jsonrpc: '2.0', id, result}; }
function toolText(id, text, isError) { return rpcResult(id, {content: [{type: 'text', text}], isError: !!isError}); }

// ─── connection ──────────────────────────────────────

let url = '';
let token = '';
let target = null;
let lib = http;
let ca;
let source = '';
let lastError = '';

function readConfigFile() {
  try { return JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8')) || {}; } catch (e) { return {}; }
}

// mcpURL turns what a user types ("192.168.1.5:8780", "https://wrm.example.com/", ".../mcp")
// into the /mcp address.
function mcpURL(addr) {
  addr = String(addr || '').trim().replace(/\/+$/, '');
  if (!addr) return '';
  if (!/^https?:\/\//i.test(addr)) addr = 'http://' + addr;
  if (!/\/mcp$/.test(addr)) addr += '/mcp';
  return addr;
}

function useConnection(u, t, caFile, from) {
  url = mcpURL(u);
  token = String(t || '').trim();
  source = from;
  try { target = new URL(url); } catch (e) { lastError = 'invalid WRM address: ' + url; target = null; return false; }
  if (target.protocol !== 'http:' && target.protocol !== 'https:') { lastError = 'invalid WRM address: ' + url; target = null; return false; }
  lib = target.protocol === 'https:' ? https : http;
  ca = undefined;
  if (caFile) {
    try { ca = fs.readFileSync(caFile); } catch (e) { lastError = 'cannot read the CA file: ' + e.message; target = null; return false; }
  }
  return true;
}

function loadConnection() {
  const envURL = clean(process.env.WRM_MCP_URL), envToken = clean(process.env.WRM_MCP_TOKEN);
  if (envURL && envToken) return useConnection(envURL, envToken, clean(process.env.WRM_MCP_CA_FILE), 'settings');
  const f = readConfigFile();
  if (f.url && f.token) return useConnection(f.url, f.token, clean(process.env.WRM_MCP_CA_FILE) || clean(f.ca_file), 'file');
  return false;
}

function saveConnection(u, t, expiresIn, caFile) {
  const old = readConfigFile();
  const data = {url: u, token: t, ca_file: caFile || old.ca_file || undefined, saved_at: new Date().toISOString(),
    expires_at: expiresIn > 0 ? new Date(Date.now() + expiresIn * 1000).toISOString() : undefined};
  fs.mkdirSync(path.dirname(CONFIG_FILE), {recursive: true, mode: 0o700});
  const tmp = CONFIG_FILE + '.' + process.pid + '.tmp';
  fs.writeFileSync(tmp, JSON.stringify(data, null, 2) + '\n', {mode: 0o600});
  fs.renameSync(tmp, CONFIG_FILE);
  try { fs.chmodSync(CONFIG_FILE, 0o600); } catch (e) { /* not on every file system */ }
}

// ─── HTTP to WRM ─────────────────────────────────────

let session = '';
let protocolVersion = '';

function request(method, body) {
  return new Promise((resolve, reject) => {
    const headers = {
      'Authorization': 'Bearer ' + token,
      'Accept': 'application/json, text/event-stream',
      'User-Agent': 'wrm-mcpb/' + VERSION,
    };
    if (body) { headers['Content-Type'] = 'application/json'; headers['Content-Length'] = Buffer.byteLength(body); }
    if (session) headers['Mcp-Session-Id'] = session;
    if (protocolVersion) headers['MCP-Protocol-Version'] = protocolVersion;
    const opts = {method, headers};
    if (ca) opts.ca = ca;
    const req = lib.request(target, opts, resolve);
    req.on('error', reject);
    if (body) req.write(body);
    req.end();
  });
}

// plainRequest is for the sign-in endpoints (metadata, registration, token).
function plainRequest(method, u, body, contentType) {
  return new Promise((resolve, reject) => {
    let t;
    try { t = new URL(u); } catch (e) { reject(new Error('invalid address ' + u)); return; }
    const l = t.protocol === 'https:' ? https : http;
    const headers = {'Accept': 'application/json', 'User-Agent': 'wrm-mcpb/' + VERSION};
    if (body) { headers['Content-Type'] = contentType; headers['Content-Length'] = Buffer.byteLength(body); }
    const opts = {method, headers, timeout: 20000};
    if (ca) opts.ca = ca;
    const req = l.request(t, opts, res => {
      readAll(res, 1 << 20).then(text => {
        let json = null;
        try { json = JSON.parse(text); } catch (e) { /* not JSON */ }
        resolve({status: res.statusCode, json, text});
      }, reject);
    });
    req.on('timeout', () => req.destroy(new Error('no answer within 20 seconds')));
    req.on('error', reject);
    if (body) req.write(body);
    req.end();
  });
}

function readAll(res, limit) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let n = 0;
    res.on('data', c => { n += c.length; if (n <= limit) chunks.push(c); });
    res.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    res.on('error', reject);
  });
}

// copyEvents writes the data of every server-sent event to stdout as it arrives (an
// approval question must reach the app while the tool call still waits).
function copyEvents(res, transform) {
  return new Promise(resolve => {
    res.setEncoding('utf8');
    let buf = '';
    let data = [];
    const flush = () => { if (data.length) { write(transform(data.join('\n'))); data = []; } };
    const line = l => {
      if (l.endsWith('\r')) l = l.slice(0, -1);
      if (l === '') flush();
      else if (l.startsWith('data:')) data.push(l.slice(5).replace(/^ /, ''));
    };
    res.on('data', chunk => {
      buf += chunk;
      let i;
      while ((i = buf.indexOf('\n')) >= 0) { line(buf.slice(0, i)); buf = buf.slice(i + 1); }
    });
    const end = () => { if (buf) line(buf); flush(); resolve(); };
    res.on('end', end);
    res.on('error', end);
  });
}

// firstResult reads an answer (JSON or an event stream) and returns the message with this id.
async function firstResult(res, id) {
  const ct = String(res.headers['content-type'] || '');
  const text = await readAll(res, MAX_BODY);
  const msgs = [];
  if (ct.startsWith('text/event-stream')) {
    for (const l of text.split('\n')) if (l.startsWith('data:')) { try { msgs.push(JSON.parse(l.slice(5))); } catch (e) { /* skip */ } }
  } else {
    try { msgs.push(JSON.parse(text)); } catch (e) { /* not JSON */ }
  }
  return msgs.find(m => m && m.id === id) || null;
}

function httpFailure(status, body) {
  let m = String(body || '').trim();
  try { m = JSON.parse(body).error.message || m; } catch (e) { /* plain text */ }
  if (status === 401) m += ' (the token is wrong, expired or revoked: call the wrm_connect tool to sign in again, or create a new token in WRM → Settings → AI connections)';
  return 'WRM answered HTTP ' + status + ': ' + m;
}

// ─── local tools (always offered) ────────────────────

function connectToolDef() {
  const state = mode === 'proxy' ? 'Connected to ' + url + '. Call it only to sign in again or to switch to another WRM.'
    : 'Not connected to WRM yet' + (lastError ? ' (' + lastError + ')' : '') + '. Ask the user for the WRM address (the one they open in the browser, for example http://192.168.1.10:8780 or https://wrm.example.com) and call this tool with it.';
  return {
    name: 'wrm_connect',
    title: 'Connect to WRM PRO',
    description: 'Sign in to a WRM PRO server: opens WRM in the browser, where the user signs in and approves which servers and permissions this AI app gets; the token goes straight to this computer, never into the chat. ' + state,
    inputSchema: {type: 'object', properties: {address: {type: 'string', description: 'WRM address, for example http://192.168.1.10:8780 or https://wrm.example.com (with or without /mcp). Empty: the last used address.'}}},
    annotations: {readOnlyHint: false, openWorldHint: true},
  };
}

function withConnectTool(raw, id) {
  let m;
  try { m = JSON.parse(raw); } catch (e) { return raw; }
  if (m && m.id === id && m.result && Array.isArray(m.result.tools) && !m.result.tools.some(t => t.name === 'wrm_connect')) {
    m.result.tools.push(connectToolDef());
    return JSON.stringify(m);
  }
  return raw;
}

// ─── sign-in (OAuth 2.1, PKCE, loopback redirect) ────

let login = null; // {base, link, done: Promise}

function b64url(buf) { return buf.toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, ''); }

function openBrowser(link) {
  if (process.env.WRM_MCP_NO_BROWSER === '1') return;
  try {
    const p = process.platform === 'win32' ? childProcess.spawn('rundll32', ['url.dll,FileProtocolHandler', link], {detached: true, stdio: 'ignore'})
      : childProcess.spawn(process.platform === 'darwin' ? 'open' : 'xdg-open', [link], {detached: true, stdio: 'ignore'});
    p.on('error', e => log('cannot open the browser: ' + e.message));
    p.unref();
  } catch (e) { log('cannot open the browser: ' + e.message); }
}

function page(res, code, title, text) {
  const esc = s => String(s).replace(/[&<>"]/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'}[c]));
  res.writeHead(code, {'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store', 'Content-Security-Policy': "default-src 'none'; style-src 'unsafe-inline'"});
  res.end('<!doctype html><meta charset="utf-8"><title>WRM PRO</title><body style="font-family:system-ui,sans-serif;max-width:34em;margin:4em auto;padding:0 1em"><h2>' +
    esc(title) + '</h2><p>' + esc(text) + '</p></body>');
}

async function startLogin(base, onToken) {
  const meta = await plainRequest('GET', base + '/.well-known/oauth-authorization-server');
  if (meta.status !== 200 || !meta.json || !meta.json.authorization_endpoint || !meta.json.token_endpoint || !meta.json.registration_endpoint) {
    throw new Error(meta.status === 404 ? 'WRM at ' + base + ' does not offer sign-in for AI apps: an administrator must turn on AI connections (MCP) with OAuth sign-in in WRM → Settings → AI connections'
      : 'no WRM sign-in found at ' + base + ' (HTTP ' + meta.status + '); check the address');
  }
  const server = http.createServer();
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  const redirect = 'http://127.0.0.1:' + server.address().port + '/callback';
  let reg;
  try {
    reg = await plainRequest('POST', meta.json.registration_endpoint, JSON.stringify({
      client_name: 'WRM PRO plugin (' + (os.hostname() || 'this computer').slice(0, 60) + ')', redirect_uris: [redirect],
      grant_types: ['authorization_code'], response_types: ['code'], token_endpoint_auth_method: 'none'}), 'application/json');
  } catch (e) { server.close(); throw e; }
  if (reg.status >= 300 || !reg.json || !reg.json.client_id) {
    server.close();
    throw new Error('WRM refused to register this app: ' + ((reg.json && (reg.json.error_description || reg.json.error)) || 'HTTP ' + reg.status));
  }
  const verifier = b64url(crypto.randomBytes(32));
  const state = b64url(crypto.randomBytes(16));
  const q = new URLSearchParams({response_type: 'code', client_id: reg.json.client_id, redirect_uri: redirect,
    code_challenge: b64url(crypto.createHash('sha256').update(verifier).digest()), code_challenge_method: 'S256', state});
  const link = meta.json.authorization_endpoint + '?' + q.toString();
  let finish;
  const done = new Promise(resolve => { finish = resolve; });
  const timer = setTimeout(() => { server.close(); finish({ok: false, error: 'the sign-in was not finished within 15 minutes'}); }, LOGIN_TTL_MS);
  timer.unref();
  server.on('request', async (req, res) => {
    const u = new URL(req.url, redirect);
    if (u.pathname !== '/callback') { page(res, 404, 'Not found', ''); return; }
    if (u.searchParams.get('state') !== state) { page(res, 400, 'Wrong sign-in', 'This answer does not belong to the sign-in this computer started.'); return; }
    clearTimeout(timer);
    server.close();
    if (u.searchParams.get('error')) {
      const why = u.searchParams.get('error_description') || u.searchParams.get('error');
      page(res, 200, 'Not connected', 'WRM did not grant access: ' + why + '. You can close this tab.');
      finish({ok: false, error: 'WRM did not grant access: ' + why});
      return;
    }
    try {
      const tok = await plainRequest('POST', meta.json.token_endpoint, new URLSearchParams({grant_type: 'authorization_code',
        code: u.searchParams.get('code') || '', redirect_uri: redirect, client_id: reg.json.client_id, code_verifier: verifier}).toString(),
      'application/x-www-form-urlencoded');
      if (tok.status !== 200 || !tok.json || !tok.json.access_token) throw new Error((tok.json && (tok.json.error_description || tok.json.error)) || 'HTTP ' + tok.status);
      await onToken(base + '/mcp', tok.json.access_token, Number(tok.json.expires_in) || 0);
      page(res, 200, 'Connected', 'This AI app is now connected to WRM PRO. You can close this tab and go back to the chat.');
      finish({ok: true});
    } catch (e) {
      page(res, 200, 'Not connected', 'The sign-in failed: ' + e.message);
      finish({ok: false, error: 'the sign-in failed: ' + e.message});
    }
  });
  return {base, link, done};
}

async function connectTool(id, args) {
  const addr = String((args && args.address) || '').trim() || readConfigFile().url || (mode === 'proxy' ? url : '');
  if (!addr) {
    write(toolText(id, 'Ask the user for the WRM address (the address they open WRM with in the browser, for example http://192.168.1.10:8780), then call wrm_connect with it.', true));
    return;
  }
  const base = mcpURL(addr).replace(/\/mcp$/, '');
  try {
    if (!login || login.base !== base) {
      login = await startLogin(base, async (u, t, expiresIn) => { saveConnection(u, t, expiresIn); await switchTo(u, t); });
    }
  } catch (e) {
    login = null;
    write(toolText(id, 'Could not start the WRM sign-in: ' + e.message, true));
    return;
  }
  const current = login;
  openBrowser(current.link);
  const r = await Promise.race([current.done, new Promise(resolve => setTimeout(() => resolve(null), CONNECT_WAIT_MS))]);
  if (!r) {
    write(toolText(id, 'WRM sign-in is open in the browser. Ask the user to sign in to WRM there and approve the access. If no browser window opened, give the user this link: ' +
      current.link + ' — when they have approved, the WRM tools appear here by themselves; call wrm_connect again to check.'));
    return;
  }
  if (login === current) login = null;
  if (!r.ok) { write(toolText(id, 'Not connected: ' + r.error + '.', true)); return; }
  write(toolText(id, 'Connected to WRM PRO at ' + base + '. The WRM tools are available now: start with list_connections, then open_session. The connection is saved on this computer for the next sessions.'));
}

// ─── modes ───────────────────────────────────────────

let mode = 'local'; // 'proxy' forwards to WRM; 'local' answers here and offers wrm_connect
let clientInit = null;

function localInitialize(msg) {
  const p = (msg.params && msg.params.protocolVersion) || '2025-06-18';
  return rpcResult(msg.id, {protocolVersion: p, capabilities: {tools: {listChanged: true}},
    serverInfo: {name: 'wrm-pro', title: 'WRM PRO (not connected)', version: VERSION},
    instructions: 'WRM PRO is not connected on this computer yet' + (lastError ? ' (' + lastError + ')' : '') +
      '. Ask the user for the WRM address and call the wrm_connect tool; WRM then opens in the browser for sign-in and approval.'});
}

function local(raw, msg) {
  const isRequest = msg.id !== undefined && msg.id !== null && typeof msg.method === 'string';
  if (!isRequest) return; // notifications and answers have nobody to go to
  switch (msg.method) {
    case 'initialize': write(localInitialize(msg)); return;
    case 'ping': write(rpcResult(msg.id, {})); return;
    case 'tools/list': write(rpcResult(msg.id, {tools: [connectToolDef()]})); return;
    case 'prompts/list': write(rpcResult(msg.id, {prompts: []})); return;
    case 'resources/list': write(rpcResult(msg.id, {resources: []})); return;
    case 'tools/call':
      if (msg.params && msg.params.name === 'wrm_connect') return connectTool(msg.id, msg.params.arguments);
      write(toolText(msg.id, 'WRM PRO is not connected yet' + (lastError ? ' (' + lastError + ')' : '') + '. Call wrm_connect with the WRM address first.', true));
      return;
    default: write(rpcError(msg.id, 'method not available until WRM is connected: ' + msg.method, -32601));
  }
}

// openWRM starts a WRM session with the app's own initialize request; on success the bridge
// forwards everything from then on.
async function openWRM(initMsg) {
  const res = await request('POST', JSON.stringify(initMsg));
  if (res.statusCode !== 200) throw new Error(httpFailure(res.statusCode, await readAll(res, 4096)));
  session = res.headers['mcp-session-id'] || '';
  const m = await firstResult(res, initMsg.id);
  if (!m || !m.result) throw new Error((m && m.error && m.error.message) || 'WRM gave no initialize answer');
  protocolVersion = m.result.protocolVersion || '';
  return m;
}

async function closeWRM() {
  if (!session) return;
  try { const res = await request('DELETE'); res.resume(); } catch (e) { /* ending anyway */ }
  session = '';
}

// switchTo moves the running bridge to a freshly signed-in WRM and tells the app that the
// tool list changed.
async function switchTo(u, t) {
  if (mode === 'proxy') await closeWRM();
  if (!useConnection(u, t, clean(process.env.WRM_MCP_CA_FILE) || clean(readConfigFile().ca_file), 'file')) throw new Error(lastError);
  protocolVersion = '';
  const init = {jsonrpc: '2.0', id: 'wrm-bridge-init', method: 'initialize', params: (clientInit && clientInit.params) || {protocolVersion: '2025-06-18', capabilities: {}, clientInfo: {name: 'wrm-mcpb'}}};
  await openWRM(init);
  const n = await request('POST', JSON.stringify({jsonrpc: '2.0', method: 'notifications/initialized'}));
  n.resume();
  mode = 'proxy';
  lastError = '';
  write({jsonrpc: '2.0', method: 'notifications/tools/list_changed'});
}

async function initialize(raw, msg) {
  clientInit = msg;
  if (!target) { local(raw, msg); return; }
  await closeWRM();
  protocolVersion = '';
  try {
    const m = await openWRM(msg);
    if (m.result.capabilities) m.result.capabilities.tools = Object.assign({}, m.result.capabilities.tools, {listChanged: true});
    mode = 'proxy';
    write(m);
  } catch (e) {
    lastError = (e.code ? 'WRM is not reachable at ' + url + ': ' : '') + e.message;
    log(lastError);
    session = '';
    mode = 'local';
    local(raw, msg);
  }
}

async function forward(raw, msg) {
  if (mode !== 'proxy') { local(raw, msg); return; }
  if (msg.method === 'tools/call' && msg.params && msg.params.name === 'wrm_connect') { await connectTool(msg.id, msg.params.arguments); return; }
  const isRequest = msg.id !== undefined && msg.id !== null && typeof msg.method === 'string';
  const fail = text => { log(text); if (isRequest) write(rpcError(msg.id, text)); };
  const transform = msg.method === 'tools/list' ? s => withConnectTool(s, msg.id) : s => s;
  let res;
  try { res = await request('POST', raw); } catch (e) { fail('WRM is not reachable at ' + url + ': ' + e.message); return; }
  const ct = String(res.headers['content-type'] || '');
  if (res.statusCode === 202) { res.resume(); return; }
  if (ct.startsWith('text/event-stream')) { await copyEvents(res, transform); return; }
  const body = await readAll(res, ct.startsWith('application/json') ? MAX_BODY : 4096);
  if (res.statusCode !== 200 || !ct.startsWith('application/json')) { fail(httpFailure(res.statusCode, body)); return; }
  write(transform(body));
}

// ─── setup for every AI app on this computer ─────────
//
//   node wrm-mcp.js --setup --url <WRM address> [--apps a,b]
//
// WRM's "All AI apps on this computer" script writes this file to ~/.wrm/wrm-mcp.js and runs
// it with --setup: it signs in to WRM in the browser (or uses WRM_MCP_TOKEN), saves the
// connection file once, and registers this bridge as the "wrm" MCP server in every AI app it
// finds. The apps then share the one connection: a later wrm_connect in any of them renews it
// for all. Files it edits are copied to <file>.wrm-backup-<time> first.

function which(cmd) {
  const exts = process.platform === 'win32' ? (process.env.PATHEXT || '.EXE;.CMD;.BAT').split(';') : [''];
  for (const dir of String(process.env.PATH || '').split(path.delimiter)) {
    if (!dir) continue;
    for (const ext of exts) {
      const f = path.join(dir, cmd + ext.toLowerCase());
      const g = path.join(dir, cmd + ext);
      for (const c of [f, g]) { try { if (fs.statSync(c).isFile()) return c; } catch (e) { /* next */ } }
    }
  }
  return '';
}

// runCLI runs an app's command line tool. On Windows those are often .cmd shims, which only
// run through cmd.exe, so the arguments are quoted for it; they are fixed words and file paths,
// and anything cmd.exe would still interpret inside quotes is refused instead of escaped.
function runCLI(cmd, args) {
  const win = process.platform === 'win32';
  const q = a => {
    if (/["%!\r\n]/.test(a)) throw new Error('cannot pass ' + JSON.stringify(a) + ' safely to ' + path.basename(cmd));
    return /[\s&|<>^(),;=]/.test(a) ? '"' + a + '"' : a;
  };
  let r;
  try {
    r = childProcess.spawnSync(win ? q(cmd) : cmd, win ? args.map(q) : args, {encoding: 'utf8', shell: win, timeout: 180000, windowsHide: true});
  } catch (e) {
    return {ok: false, text: e.message};
  }
  const text = ((r.stdout || '') + (r.stderr || '')).trim().split('\n').filter(Boolean).pop() || '';
  return {ok: r.status === 0, text: r.error ? r.error.message : text};
}

function stripJSONComments(text) {
  let out = '', inStr = false, i = 0;
  while (i < text.length) {
    const c = text[i], n = text[i + 1];
    if (inStr) { out += c; if (c === '\\') { out += n || ''; i += 2; continue; } if (c === '"') inStr = false; i++; continue; }
    if (c === '"') { inStr = true; out += c; i++; continue; }
    if (c === '/' && n === '/') { while (i < text.length && text[i] !== '\n') i++; continue; }
    if (c === '/' && n === '*') { i += 2; while (i < text.length && !(text[i] === '*' && text[i + 1] === '/')) i++; i += 2; continue; }
    out += c; i++;
  }
  return out.replace(/,(\s*[}\]])/g, '$1');
}

// mergeJSON reads the file once (no separate existence check), keeps that content as the
// backup and writes the merged object back in place, so symlinked config files stay links.
function mergeJSON(file, change) {
  let obj = {};
  let raw = null;
  try { raw = fs.readFileSync(file); } catch (e) { if (e.code !== 'ENOENT') throw e; }
  if (raw === null) {
    fs.mkdirSync(path.dirname(file), {recursive: true});
  } else {
    const text = raw.toString('utf8').replace(/^\uFEFF/, '');
    if (text.trim()) {
      try { obj = JSON.parse(stripJSONComments(text)); } catch (e) { throw new Error(file + ' is not valid JSON; add WRM by hand'); }
    }
    if (!obj || typeof obj !== 'object' || Array.isArray(obj)) throw new Error(file + ' does not hold a JSON object; add WRM by hand');
    fs.writeFileSync(file + '.wrm-backup-' + new Date().toISOString().replace(/[:.]/g, '-'), raw, {flag: 'wx'});
  }
  change(obj);
  fs.writeFileSync(file, JSON.stringify(obj, null, 2) + '\n');
}

function isDir(d) { try { return fs.statSync(d).isDirectory(); } catch (e) { return false; } }

function setupApps(self) {
  const home = os.homedir(), win = process.platform === 'win32', mac = process.platform === 'darwin';
  const appData = process.env.APPDATA || path.join(home, 'AppData', 'Roaming');
  const nodeBin = process.execPath;
  const stdio = {command: nodeBin, args: [self]};
  const userConfig = name => win ? path.join(appData, name) : mac ? path.join(home, 'Library', 'Application Support', name) : path.join(home, '.config', name);
  const claudeDesktopDirs = () => {
    const dirs = [userConfig('Claude')];
    if (win && process.env.LOCALAPPDATA) {
      const pk = path.join(process.env.LOCALAPPDATA, 'Packages');
      try { for (const d of fs.readdirSync(pk)) if (/^Claude_/i.test(d)) dirs.push(path.join(pk, d, 'LocalCache', 'Roaming', 'Claude')); } catch (e) { /* no store apps */ }
    }
    return dirs.filter(isDir);
  };
  return [
    {id: 'claude-code', name: 'Claude Code', run() {
      const c = which('claude');
      if (!c) return null;
      let r = runCLI(c, ['plugin', 'install', 'wrm-pro', '--marketplace', 'vedranius/wrm-pro-enterprise']);
      if (r.ok) return 'WRM PRO plugin installed (skills, /wrm-pro:wrm and this connection)';
      runCLI(c, ['plugin', 'marketplace', 'add', 'vedranius/wrm-pro-enterprise']);
      r = runCLI(c, ['plugin', 'install', 'wrm-pro@wrm-pro']);
      if (r.ok) return 'WRM PRO plugin installed (skills, /wrm-pro:wrm and this connection)';
      runCLI(c, ['mcp', 'remove', '--scope', 'user', 'wrm']);
      r = runCLI(c, ['mcp', 'add', '--scope', 'user', 'wrm', '--', nodeBin, self]);
      if (!r.ok) throw new Error(r.text);
      return 'MCP server "wrm" added (the plugin could not be installed: ' + r.text + ')';
    }},
    {id: 'claude-desktop', name: 'Claude app (Desktop chat and Cowork)', run() {
      const dirs = claudeDesktopDirs();
      if (!dirs.length) return null;
      for (const d of dirs) mergeJSON(path.join(d, 'claude_desktop_config.json'), o => { o.mcpServers = Object.assign({}, o.mcpServers, {wrm: stdio}); });
      return 'MCP server "wrm" added to claude_desktop_config.json; quit and start the Claude app again';
    }},
    {id: 'codex', name: 'Codex CLI', run() {
      const c = which('codex');
      if (!c) return null;
      runCLI(c, ['mcp', 'remove', 'wrm']);
      const r = runCLI(c, ['mcp', 'add', 'wrm', '--', nodeBin, self]);
      if (!r.ok) throw new Error(r.text);
      return 'MCP server "wrm" added';
    }},
    {id: 'gemini', name: 'Gemini CLI', run() {
      const c = which('gemini');
      if (!c) return null;
      runCLI(c, ['mcp', 'remove', '--scope', 'user', 'wrm']);
      const r = runCLI(c, ['mcp', 'add', '--scope', 'user', 'wrm', nodeBin, self]);
      if (!r.ok) throw new Error(r.text);
      return 'MCP server "wrm" added';
    }},
    {id: 'vscode', name: 'VS Code', run() {
      const d = userConfig(path.join('Code', 'User'));
      if (!isDir(d)) return null;
      mergeJSON(path.join(d, 'mcp.json'), o => { o.servers = Object.assign({}, o.servers, {wrm: Object.assign({type: 'stdio'}, stdio)}); });
      return 'MCP server "wrm" added to the user mcp.json';
    }},
    {id: 'cursor', name: 'Cursor', run() {
      const d = path.join(home, '.cursor');
      if (!isDir(d)) return null;
      mergeJSON(path.join(d, 'mcp.json'), o => { o.mcpServers = Object.assign({}, o.mcpServers, {wrm: stdio}); });
      return 'MCP server "wrm" added to ~/.cursor/mcp.json';
    }},
    {id: 'windsurf', name: 'Windsurf', run() {
      const d = path.join(home, '.codeium', 'windsurf');
      if (!isDir(d)) return null;
      mergeJSON(path.join(d, 'mcp_config.json'), o => { o.mcpServers = Object.assign({}, o.mcpServers, {wrm: stdio}); });
      return 'MCP server "wrm" added to mcp_config.json';
    }},
    {id: 'opencode', name: 'OpenCode', run() {
      const d = path.join(process.env.XDG_CONFIG_HOME || path.join(home, '.config'), 'opencode');
      if (!isDir(d) && !which('opencode')) return null;
      const f = fs.existsSync(path.join(d, 'opencode.jsonc')) ? path.join(d, 'opencode.jsonc') : path.join(d, 'opencode.json');
      mergeJSON(f, o => {
        if (!o.$schema) o.$schema = 'https://opencode.ai/config.json';
        o.mcp = Object.assign({}, o.mcp, {wrm: {type: 'local', command: [nodeBin, self], enabled: true}});
      });
      return 'MCP server "wrm" added to ' + path.basename(f);
    }},
  ];
}

async function setup(argv) {
  const out = s => process.stdout.write(s + '\n');
  const opt = n => { const i = argv.indexOf('--' + n); return i >= 0 ? String(argv[i + 1] || '') : ''; };
  const only = opt('apps').split(',').filter(Boolean);
  const u = mcpURL(opt('url') || clean(process.env.WRM_MCP_URL));
  if (!u) throw new Error('give the WRM address with --url');
  const caFile = clean(process.env.WRM_MCP_CA_FILE);
  let t = clean(process.env.WRM_MCP_TOKEN);
  if (!t) {
    if (!useConnection(u, '', caFile, 'setup')) throw new Error(lastError);
    const base = url.replace(/\/mcp$/, '');
    let got = null;
    const l = await startLogin(base, async (u2, t2, exp) => { got = {u: u2, t: t2, exp}; });
    out('Sign in to WRM in the browser and approve the access for the AI apps on this computer.');
    out('If no browser window opens, open this link: ' + l.link);
    openBrowser(l.link);
    const r = await l.done;
    if (!r.ok || !got) throw new Error(r.error || 'not signed in');
    t = got.t;
  }
  if (!useConnection(u, t, caFile, 'setup')) throw new Error(lastError);
  out('Checking the connection to ' + url + ' ...');
  let m;
  try {
    m = await openWRM({jsonrpc: '2.0', id: 1, method: 'initialize', params: {protocolVersion: '2025-06-18', capabilities: {}, clientInfo: {name: 'wrm-setup', version: VERSION}}});
  } catch (e) { throw new Error(e.code ? 'WRM is not reachable at ' + url + ': ' + e.message : e.message); }
  await closeWRM();
  out('  OK: ' + ((m.result.serverInfo && m.result.serverInfo.title) || 'WRM PRO'));
  saveConnection(url, token, 0, caFile);
  out('Saved the connection in ' + CONFIG_FILE);
  out('');
  const self = path.resolve(process.argv[1]);
  let added = 0, failed = 0;
  for (const app of setupApps(self)) {
    if (only.length && !only.includes(app.id)) continue;
    try {
      const r = app.run();
      if (r === null) { out('  [--] ' + app.name + ': not found on this computer'); continue; }
      out('  [ok] ' + app.name + ': ' + r);
      added++;
    } catch (e) {
      out('  [!!] ' + app.name + ': ' + e.message);
      failed++;
    }
  }
  out('');
  out('Claude app plugin (skills and /wrm-pro:wrm in chat and Cowork): Customize > Plugins > Add marketplace > vedranius/wrm-pro-enterprise > WRM PRO, or Upload plugin with claude-plugin-wrm-pro.zip from the WRM release.');
  out('Start the AI apps again so they load WRM. In any of them, wrm_connect signs in again when the access expires; the new access then serves all of them.');
  return failed && !added ? 1 : 0;
}

if (process.argv[2] === '--setup') {
  setup(process.argv.slice(3)).then(code => process.exit(code), e => { process.stderr.write('WRM setup failed: ' + (e && e.message || e) + '\n'); process.exit(1); });
} else {
  serve();
}

function serve() {
  loadConnection();
  if (!target && lastError) log(lastError);

  const pending = new Set();
  let chain = Promise.resolve();
  const rl = readline.createInterface({input: process.stdin, crlfDelay: Infinity});
  rl.on('line', line => {
    const raw = line.trim();
    if (!raw) return;
    let msg;
    try { msg = JSON.parse(raw); } catch (e) { write({jsonrpc: '2.0', id: null, error: {code: -32700, message: 'parse error'}}); return; }
    if (msg.method === 'initialize') {
      // nothing else may run before the session exists
      chain = Promise.all([...pending]).then(() => initialize(raw, msg));
      track(chain);
      return;
    }
    track(chain.then(() => forward(raw, msg)));
  });
  function track(p) {
    const q = p.catch(e => log(String(e && e.message || e))).finally(() => pending.delete(q));
    pending.add(q);
  }
  rl.on('close', async () => {
    await Promise.all([...pending]);
    await closeWRM();
    process.exit(0);
  });
}
