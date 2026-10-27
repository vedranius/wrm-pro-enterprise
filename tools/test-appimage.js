#!/usr/bin/env node

/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

for (const explicit of [false, true]) {
  test(`AppImage launcher preserves arguments and ${explicit ? 'explicit relative' : 'persistent default'} database`, () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'wrm-apprun-'));
    try {
      const app = path.join(root, 'app with spaces');
      fs.mkdirSync(path.join(app, 'usr/bin'), {recursive: true});
      fs.copyFileSync(path.join(__dirname, '../packaging/appimage/AppRun'), path.join(app, 'AppRun'));
      fs.writeFileSync(path.join(app, 'usr/bin/wrm'), '#!/bin/sh\nprintf "%s\\n" "$PWD" "$DB_PATH" "$LISTEN_ADDR" "$WRM_APPIMAGE" "$WRM_NO_SERVICE_PROMPT" "$@"\n', {mode: 0o755});
      const data = path.join(root, 'data $ literal');
      const env = {...process.env, XDG_DATA_HOME: data, PORT: '18082'};
      delete env.DB_PATH;
      delete env.LISTEN_ADDR;
      if (explicit) { env.DB_PATH = 'custom database.db'; env.LISTEN_ADDR = '127.0.0.1:18083'; }
      const r = spawnSync('/bin/sh', [path.join(app, 'AppRun'), '-env-file', 'file with spaces.env'], {cwd: root, env, encoding: 'utf8'});
      assert.equal(r.status, 0, r.stderr);
      assert.deepEqual(r.stdout.trim().split('\n'), [
        path.join(data, 'wrm-pro'),
        explicit ? path.join(root, 'custom database.db') : path.join(data, 'wrm-pro/remote_manager.db'),
        explicit ? '127.0.0.1:18083' : '127.0.0.1:18082', '1', '1', '-env-file', 'file with spaces.env',
      ]);
    } finally { fs.rmSync(root, {recursive: true, force: true}); }
  });
}
