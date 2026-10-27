#!/usr/bin/env node

/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

// Regression tests for the real Git Simple handlers and request payloads, without browser dependencies.
// The DOM double models the controls these handlers use; rendering/layout needs a browser smoke check.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');

const staticDir = path.join(__dirname, '..', 'remote-manager', 'static');
const source = fs.readFileSync(path.join(staticDir, 'git.js'), 'utf8');
const html = fs.readFileSync(path.join(staticDir, 'index.html'), 'utf8');
const helpers = html.slice(html.indexOf('function esc(s)'), html.indexOf('function showToast('));
const decode = s => s.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');

function fixture() {
  const nodes = new Map();
  class Node {
    constructor(tag = 'div') { this.tag = tag; this.style = {}; this.dataset = {}; this.children = []; this.options = []; this.value = ''; this.disabled = false; this.checked = false; }
    set id(id) { this._id = id; nodes.set(id, this); }
    get id() { return this._id; }
    set innerHTML(value) {
      this.children.forEach(n => n.remove()); this.children = []; this._html = value;
      for (const match of value.matchAll(/<([a-z][\w-]*)\b([^>]*\bid="([^"]+)"[^>]*)>/g)) {
        const n = new Node(match[1]), attrs = match[2]; n.id = match[3];
        n.disabled = /\bdisabled\b/.test(attrs); n.checked = /\bchecked\b/.test(attrs);
        n.value = decode((attrs.match(/\bvalue="([^"]*)"/) || [0, ''])[1]);
        if (/display:none/.test(attrs)) n.style.display = 'none';
        if (n.tag === 'select') {
          const contents = value.slice(match.index + match[0].length).split('</select>')[0];
          n.options = [...contents.matchAll(/<option value="([^"]*)"[^>]*>([^<]*)<\/option>/g)].map(m => ({value: decode(m[1]), textContent: decode(m[2])}));
        }
        this.children.push(n);
      }
    }
    get innerHTML() { return this._html || ''; }
    appendChild(n) { this.children.push(n); return n; }
    remove() { this.children.forEach(n => n.remove()); if (nodes.get(this.id) === this) nodes.delete(this.id); }
    addEventListener() {}
    focus() {}
    querySelector() { return this.children.find(n => ['input', 'select', 'textarea'].includes(n.tag)) || null; }
  }
  const document = {
    createElement: tag => new Node(tag), getElementById: id => nodes.get(id) || null,
    head: new Node(), body: new Node(), addEventListener() {},
    querySelectorAll: () => [],
  };
  const ctx = vm.createContext({document, LANGS: {en: {}, hr: {}}, currentUser: {id: 1},
    localStorage: {getItem: () => 'simple', setItem() {}},
    setTimeout, clearTimeout, btoa: s => Buffer.from(s, 'binary').toString('base64'),
    fmtTime: s => s || '', apiError: async r => r.error || 'request failed',
    showToast: () => {}, uiConfirm: async () => true,
  });
  ctx.window = ctx; ctx.t = key => ctx.LANGS.en[key] || key;
  const requests = [];
  let reply = () => response({}, false);
  ctx.fetch = async (url, options = {}) => {
    requests.push({url, body: options.body ? JSON.parse(options.body) : null});
    return reply(url, options);
  };
  vm.runInContext(helpers, ctx);
  // Expose fixture state only in the in-memory test copy. Production exports are unchanged.
  vm.runInContext(source.replace('  window.gitWorkspace = {', '  window.__fixture = {G, E, S, sKey, envKey, paintSimple};\n  window.gitWorkspace = {'), ctx);
  const state = ctx.__fixture;
  state.G.state = {deploy: {update: true, rollback: true, restart: true}, settings: {}};
  state.E.ov = {services: [{app: 'demo', envs: [{name: 'prod', confirm: true, destinations: [{server: 'host-1', path: '/app'}, {server: 'host-2', path: '/app'}]}]}]};
  return {ctx, state, api: ctx.gitWorkspace, document, requests, node: id => document.getElementById(id),
    reply(fn) { reply = fn; },
    mount() { const body = new Node(); body.id = 'git-body'; document.body.appendChild(body); state.paintSimple(); },
  };
}
const response = (value, ok = true) => ({ok, json: async () => value});
function plan(options = {}) {
  return {id: 'plan-1', app: 'demo', env: 'prod', target: {commit: 'abcdef123456', version: 'v2'}, confirm: true, errors: [], excluded: {},
    dests: [0, 1].map(index => ({index, server: 'host-' + (index + 1), path: '/app', errors: [], counts: {changed: 1}, files: [{path: 'run.py', state: 'changed'}]})), ...options};
}
const history = () => ({dests: [0, 1].map(i => ({server: 'host-' + (i + 1), path: '/app', current: 'deploy-1'})),
  entries: [0, 1].map(dest => ({dest, action: 'DEPLOY', id: 'deploy-1', version: 'v2', at: '2026-10-13T10:00:00Z', previous: {version: 'v1.' + dest}}))});
const sentRuns = f => f.requests.filter(r => r.url === '/api/git/runs');
function type(f, id, text) { const n = f.node(id); n.value = text; n.oninput(); }
const tick = () => new Promise(resolve => setImmediate(resolve));

test('protected deploy requires both a destination and the current typed confirmation', async () => {
  const f = fixture(); f.reply(() => response(plan()));
  await f.api.simplePlan('demo', 'prod', 'update');
  f.api.simpleDestination(0, false); f.api.simpleDestination(1, false);
  type(f, 'gs-conf', 'prod');
  assert.equal(f.node('gs-start').disabled, true);
  await f.node('gs-start').onclick(); assert.equal(sentRuns(f).length, 0);
  f.api.simpleDestination(1, true); type(f, 'gs-conf', 'PROD');
  assert.equal(f.node('gs-start').disabled, false);
  type(f, 'gs-conf', 'wrong'); assert.equal(f.node('gs-start').disabled, true);
});

test('deploy sends selected destinations and file exclusions, prevents duplicate submission and recovers after failure', async () => {
  const f = fixture(); f.reply(url => response(plan(), url !== '/api/git/runs'));
  await f.api.simplePlan('demo', 'prod', 'update');
  f.api.simpleDestination(0, false); f.api.simpleExclude('run.py', false); type(f, 'gs-conf', 'prod');
  let accept; f.ctx.uiConfirm = () => new Promise(resolve => { accept = resolve; });
  const pending = f.node('gs-start').onclick();
  await f.node('gs-start').onclick(); assert.equal(f.node('gs-start').disabled, true);
  f.api.simpleDestination(0, true); f.api.simpleExclude('run.py', true);
  accept(true); await pending;
  assert.equal(sentRuns(f).length, 1);
  assert.deepEqual(sentRuns(f)[0].body.env.dests, [1]);
  assert.deepEqual(sentRuns(f)[0].body.env.exclude, ['run.py']);
  assert.equal(f.node('gs-start').disabled, false);
});

test('one failed destination leaves healthy destinations accessible from the card and selected by default', async () => {
  const f = fixture(), p = plan(); p.dests[0].errors = ['cannot connect'];
  f.reply(() => response(p));
  f.state.E.cells[f.state.envKey('demo', 'prod')] = {data: {target: p.target, dests: [{server: 'host-1', error: 'cannot connect'}, {server: 'host-2', path: '/app', exists: true, behind: 1}]}};
  f.mount(); assert.match(f.node('git-body').innerHTML, /gitWorkspace\.simplePlan/);
  await f.api.simplePlan('demo', 'prod', 'update');
  assert.deepEqual([...f.state.S.destinations], [1]);
  f.api.simpleDestination(0, true); assert.deepEqual([...f.state.S.destinations], [1]);
  type(f, 'gs-conf', 'prod'); assert.equal(f.node('gs-start').disabled, false);
});

test('rollback with no destinations remains disabled after typing confirmation; subset keeps per-server versions', async () => {
  const f = fixture(); f.reply(url => response(history(), url !== '/api/git/runs'));
  await f.api.simpleRollback('demo', 'prod');
  assert.match(f.node('git-modal').innerHTML, /v1\.0/); assert.match(f.node('git-modal').innerHTML, /v1\.1/);
  f.api.simpleRollbackDestination(0, false); f.api.simpleRollbackDestination(1, false);
  type(f, 'gs-rbconf', 'prod'); assert.equal(f.node('gs-rbgo').disabled, true);
  await f.node('gs-rbgo').onclick(); assert.equal(sentRuns(f).length, 0);
  f.api.simpleRollbackDestination(1, true); await f.node('gs-rbgo').onclick();
  assert.deepEqual(sentRuns(f)[0].body.env.dests, [1]); assert.equal(f.node('gs-rbgo').disabled, false);
});

test('late plan responses cannot replace another environment or reopen a canceled review', async () => {
  const f = fixture(), pending = [];
  f.reply(() => new Promise(resolve => pending.push(resolve)));
  const a = f.api.simplePlan('demo', 'prod', 'update'), b = f.api.simplePlan('other', 'test', 'deploy');
  pending[1](response(plan({id: 'other-plan', app: 'other', env: 'test', confirm: false}))); await b;
  pending[0](response(plan())); await a;
  assert.equal(f.state.S.ctx.app, 'other'); assert.equal(f.state.S.plan.id, 'other-plan');
  const c = f.api.simplePlan('demo', 'prod', 'update'); f.api.closeModal();
  pending[2](response(plan())); await c;
  assert.equal(f.node('git-modal'), null); assert.equal(f.state.S.plan, null);
});

test('canceled approval cannot submit after the review closes; network errors restore controls', async () => {
  const f = fixture(); f.reply(() => response(plan())); await f.api.simplePlan('demo', 'prod', 'update');
  type(f, 'gs-conf', 'prod'); let accept;
  f.ctx.uiConfirm = () => new Promise(resolve => { accept = resolve; });
  const pending = f.node('gs-start').onclick(); f.api.closeModal(); accept(true); await pending;
  assert.equal(sentRuns(f).length, 0);
  f.reply(url => { if (url === '/api/git/runs') throw new Error('offline'); return response(plan()); });
  await f.api.simplePlan('demo', 'prod', 'update'); type(f, 'gs-conf', 'prod'); f.ctx.uiConfirm = async () => true;
  await f.node('gs-start').onclick(); assert.equal(f.node('gs-start').disabled, false);
});

test('recompare preserves selected files and options; opening a fresh review resets them', async () => {
  const f = fixture(); f.reply(() => response(plan({excluded: {'saved.ini': {by: 'admin', at: '2026-10-13'}}})));
  await f.api.simplePlan('demo', 'prod', 'update'); f.api.simpleExclude('run.py', false); f.api.simpleDestination(0, false);
  f.node('gs-restart').checked = true;
  await f.api.simpleRecompare();
  assert.deepEqual([...f.state.S.destinations], [1]); assert.equal(f.state.S.excluded.has('run.py'), true); assert.equal(f.node('gs-restart').checked, true);
  await f.api.simplePlan('demo', 'prod', 'update');
  assert.deepEqual([...f.state.S.destinations], [0, 1]); assert.equal(f.state.S.excluded.has('run.py'), false); assert.equal(f.node('gs-restart').checked, false);
});

test('removed-only files are displayed as retained and have no misleading deploy checkbox', async () => {
  const f = fixture(), p = plan(); p.dests[0].files.push({path: 'old.py', state: 'removed'});
  f.reply(() => response(p)); await f.api.simplePlan('demo', 'prod', 'update');
  assert.match(f.node('git-modal').innerHTML, /data-path="old\.py" disabled/);
  assert.match(f.node('git-modal').innerHTML, /kept on the server/);
});

test('filters combine hostname, status, branch and version using stable connection keys', async () => {
  const f = fixture();
  f.reply(() => response({hosts: {'host-1': 'api.example.test', 'host-2': 'worker.example.test'}, target: {commit: 'abcdef123456', branch: 'main', version: 'v2'},
    dests: [{server: 'host-1', path: '/app', exists: true, behind: 1, version_md: 'v1', ref_md: 'release/stable', commit_md: '123456abcdef'}]}));
  f.mount(); await tick();
  const card = f.node(f.state.sKey('demo', 'prod'));
  f.api.simpleFilter('q', 'api.example.test'); f.api.simpleFilter('server', 'host-1');
  f.api.simpleFilter('status', 'update'); f.api.simpleFilter('branch', 'release'); f.api.simpleFilter('version', 'v1');
  assert.equal(card.style.display, '');
  assert.match(card.innerHTML, /Compared/);
  const option = f.node('gs-server-filter').options.find(o => o.value === 'host-1'); assert.equal(option.textContent, 'api.example.test');
  f.api.simpleFilter('status', 'ok'); assert.equal(card.style.display, 'none'); assert.equal(f.node('gs-filter-empty').style.display, '');
  f.api.simpleClearFilters(); assert.equal(card.style.display, '');
});

test('AI availability matches owned and shared connection permissions and accepts legacy lowercase SSH protocols', () => {
  const ctx = vm.createContext({currentUser: {policy: {ai: true}}, winConn: w => w.conn, isCollabMode: false, collab: {}, winObjects: []});
  vm.runInContext(html.slice(html.indexOf('function winPerms('), html.indexOf('function guardWin(')), ctx);
  vm.runInContext(html.slice(html.indexOf('function aiEligible('), html.indexOf('async function openAI(')), ctx);
  const owned = {conn: {protocol: 'SSH'}};
  assert.equal(ctx.aiAvailable(owned, 'ssh'), true); assert.equal(ctx.aiAvailable(owned, 'sftp'), true);
  assert.equal(ctx.aiAvailable({...owned, conn: {protocol: 'ssh'}}, 'ssh'), true);
  assert.equal(ctx.aiAvailable({...owned, conn: {protocol: 'SFTP'}}, 'sftp'), true);
  assert.equal(ctx.aiAvailable({...owned, shareToken: 'share', perms: {terminal: true, files_write: false}}, 'ssh'), false);
  assert.equal(ctx.aiAvailable({...owned, shareToken: 'share', perms: {terminal: true, files_write: true}}, 'ssh'), true);
  assert.equal(ctx.aiAvailable({...owned, console: true}, 'ssh'), false);
  assert.equal(ctx.aiAvailable({...owned, shareToken: 'share', perms: {terminal: false, files_write: true}}, 'ssh'), false);
  assert.equal(ctx.aiAvailable({...owned, conn: {protocol: 'RDP'}}, 'ssh'), false);
  ctx.currentUser.policy.ai = false; assert.equal(ctx.aiAvailable(owned, 'ssh'), false);
});

test('enabling and disabling AI refreshes already-open terminal buttons without reconnecting', async () => {
  const buttons = new Map([[1, {style: {display: 'none'}, classes: new Set(), classList: {toggle(name,on) { const item = [...buttons.values()].find(x => x.classList === this); if (on) item.classes.add(name); else item.classes.delete(name); }}}], [2, {style: {display: 'none'}, classes: new Set(), classList: {toggle(name,on) { const item = [...buttons.values()].find(x => x.classList === this); if (on) item.classes.add(name); else item.classes.delete(name); }}}]]);
  const terminal = {}, ctx = vm.createContext({currentUser: {id: 1, policy: {ai: false}}, isCollabMode: false, collab: {}, adminSettingsCache: [],
    winConn: w => w.conn, showToast() {}, t: k => k,
    document: {getElementById: id => buttons.get(Number(id.replace('wb-ai-', ''))) || null},
    winObjects: [{id: 1, mode: 'ssh', conn: {protocol: 'SSH'}, term: terminal}, {id: 2, mode: 'ssh', conn: {protocol: 'SSH'}, shareToken: 'share', perms: {terminal: true, files_write: false}}],
  });
  let enabled = true;
  ctx.fetch = async url => response(url === '/api/auth/me' ? {id: 1, policy: {ai: enabled}} : {changed: {ai_assistant: enabled ? 'all' : 'off'}});
  vm.runInContext(html.slice(html.indexOf('function winPerms('), html.indexOf('function guardWin(')), ctx);
  vm.runInContext(html.slice(html.indexOf('function aiEligible('), html.indexOf('async function openAI(')), ctx);
  vm.runInContext(html.slice(html.indexOf('function applyPolicyToUI('), html.indexOf('function showFolderCtx(')), ctx);
  vm.runInContext(html.slice(html.indexOf('async function saveSetting('), html.indexOf('async function adminHosts(')), ctx);
  await ctx.saveSetting('ai_assistant', 'all');
  assert.equal(buttons.get(1).style.display, ''); assert.equal(buttons.get(2).style.display, 'none');
  assert.equal(ctx.winObjects[0].term, terminal);
  assert.equal(buttons.get(1).classes.has('policy-off'), false);
  enabled = false; await ctx.saveSetting('ai_assistant', 'off'); assert.equal(buttons.get(1).style.display, '');
  assert.equal(buttons.get(1).classes.has('policy-off'), true); assert.equal(ctx.aiAvailable(ctx.winObjects[0], 'ssh'), false);
  let respond; ctx.fetch = () => new Promise(resolve => { respond = resolve; });
  const pending = ctx.refreshUserPolicy(); ctx.currentUser = null;
  respond(response({id: 1, policy: {ai: true}})); await pending;
  assert.equal(ctx.currentUser, null);
});
