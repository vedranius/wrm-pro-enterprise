#!/usr/bin/env node

/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

'use strict';
// Exercise the real panel event handlers: completion replaces streamed tool JSON,
// and the processing indicator follows the model turn rather than POST acceptance.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const dir = path.join(__dirname, '..', 'remote-manager', 'static');
const source = fs.readFileSync(path.join(dir, 'ai.js'), 'utf8');
const html = fs.readFileSync(path.join(dir, 'index.html'), 'utf8');

function fixture() {
  const nodes = new Map(), messages = [];
  const body = {scrollTop: 0, scrollHeight: 0, clientHeight: 0,
    insertAdjacentHTML(position, markup) {
      const id = (markup.match(/id="([^"]*)"/) || [])[1];
      const node = {innerHTML: markup, textContent: '', style: {},
        remove() { const i = messages.indexOf(node); if (i >= 0) messages.splice(i, 1); if (id) nodes.delete(id); },
      };
      messages.push(node); if (id) nodes.set(id, node);
    },
    get lastElementChild() { return messages.at(-1); },
  };
  nodes.set('aib-1', body);
  nodes.set('ai-style', {});
  for (const id of ['ai-send-1', 'ai-stop-1', 'ai-in-1']) nodes.set(id, {style: {}, value: '', disabled: false});
  const ctx = vm.createContext({LANGS: {en: {}, hr: {}}, currentUser: {id: 1},
    winObjects: [{id: 1, mode: 'ssh', connId: 2}], document: {getElementById: id => nodes.get(id) || null},
    showToast() {}, fetch: async () => ({ok: true, json: async () => ({})}),
  });
  ctx.window = ctx; ctx.t = k => ctx.LANGS.en[k] || k;
  vm.runInContext(html.slice(html.indexOf('function esc(s)'), html.indexOf('function showToast(')), ctx);
  vm.runInContext(source.replace('  window.wrmAI = {', '  window.__fixture = {S, onEvent, send};\n  window.wrmAI = {'), ctx);
  const state = ctx.__fixture;
  state.S[1] = {seq: 0, sid: 2, busy: false, cards: {}, cur: null};
  let seq = 0;
  return {ctx, state, nodes, messages, event(type, data) { state.onEvent(1, {seq: ++seq, type, data}); }};
}

test('an empty cleaned completion removes streamed fallback tool JSON', () => {
  const f = fixture();
  f.event('assistant_start', {}); f.event('text', {delta: '{"name":"system_info","arguments":{}}'});
  assert.equal(f.messages.length, 1);
  f.event('assistant_done', {text: '', stop: 'stop'}); assert.equal(f.messages.length, 0);
  f.event('assistant_start', {}); f.event('text', {delta: 'Server date: 2026-10-13'});
  f.event('assistant_done', {text: 'Server date: 2026-10-13', stop: 'stop'});
  assert.equal(f.messages.length, 1); assert.equal(f.messages[0].innerHTML, 'Server date: 2026-10-13');
});

test('cleaned narrative replaces streamed JSON while preserving ordinary text and escaping HTML', () => {
  const f = fixture();
  f.event('assistant_start', {}); f.event('text', {delta: 'Checking. {"name":"system_info","arguments":{}}'});
  f.event('assistant_done', {text: 'Checking. <script>untrusted</script>'});
  assert.equal(f.messages.length, 1);
  assert.equal(f.messages[0].innerHTML, 'Checking. &lt;script&gt;untrusted&lt;/script&gt;');
});

test('processing indicator persists after POST acceptance until the backend ends the turn', async () => {
  const f = fixture(); f.nodes.get('ai-in-1').value = 'Check the date';
  f.ctx.fetch = async () => {
    f.event('busy', {busy: true});
    return {ok: true, json: async () => ({ok: true})};
  };
  await f.state.send(1);
  assert.equal(f.nodes.get('ai-send-1').disabled, true);
  f.nodes.get('ai-in-1').value = 'unsent draft'; await f.state.send(1);
  assert.equal(f.nodes.get('ai-in-1').value, 'unsent draft');
  f.event('busy', {busy: false});
  assert.equal(f.nodes.get('ai-send-1').disabled, false);
});

test('rejected prompt clears the processing indicator and allows a retry', async () => {
  const f = fixture(); f.nodes.get('ai-in-1').value = 'Check the date';
  f.ctx.fetch = async () => ({ok: false, json: async () => ({error: 'session ended'})});
  await f.state.send(1);
  assert.equal(f.nodes.has('ai-wait-1'), false); assert.equal(f.nodes.get('ai-send-1').disabled, false);
});
