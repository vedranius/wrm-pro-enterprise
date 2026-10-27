#!/usr/bin/env node

/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const html = fs.readFileSync(path.join(__dirname,'..','remote-manager','static','index.html'),'utf8');
const slice = (from,to) => html.slice(html.indexOf(from),html.indexOf(to));

test('nested folder labels disambiguate equal connection names and include descendants', () => {
  const ctx = vm.createContext({folders:[{id:1,name:'Client-A'},{id:2,name:'Site-1',parent_id:1},{id:3,name:'Client-B'},{id:4,name:'Site-1',parent_id:3}]});
  vm.runInContext(slice('function folderPathLabel(', 'function updateFolderSelects('),ctx);
  assert.equal(ctx.connLabel({name:'Backup',folder_id:2}),'Client-A / Site-1 / Backup');
  assert.equal(ctx.connLabel({name:'Backup',folder_id:4}),'Client-B / Site-1 / Backup');
  assert.deepEqual([...ctx.folderDescendants(1)],[1,2]);
  assert.equal(ctx.connLabel({name:'Shared backup',folder_id:9,folder_path:'Other / Site'}),'Other / Site / Shared backup');
});

test('terminal path selection rejects multiline output and preserves a filename as data', () => {
  const ctx = vm.createContext({});
  vm.runInContext(slice('function selectedTerminalPath(', 'function showTerminalPathMenu('),ctx);
  const selected = text => ctx.selectedTerminalPath({term:{getSelection:()=>text}});
  assert.equal(selected("'some file.mp4'"),'some file.mp4');
  assert.equal(selected('line one\nline two'),'');
  assert.equal(selected('  /opt/clip.mp4  '),'/opt/clip.mp4');
  assert.equal(selected('$(touch /tmp/never-run).mp4'),'$(touch /tmp/never-run).mp4');
});

test('relative selected paths use the terminal directory and retain share authorization', async () => {
  const wo={id:1,connId:20,_selectedPath:'clip.mp4',cwd:'/opt/media'};
  const requests=[],downloads=[],previews=[];
  const ctx=vm.createContext({winObjects:[wo],closeAllCtx(){},guardWin:()=>true,joinPath:(a,b)=>a.replace(/\/$/,'')+'/'+b,
    selectedTerminalPath:()=>'',showToast(){},t:k=>k,connQS:()=> 'id=20&share_token=demo',termAskCwd:async()=>({path:'/fallback'}),
    openRemotePreview:(id,p)=>previews.push([id,p]),downloadFile:(id,p)=>downloads.push([id,p]),
    fetch:async url=>{requests.push(url);return {ok:true,json:async()=>({path:'/home/demo'})}},
  });
  vm.runInContext(slice('async function terminalPathAction(', '// ══ WINDOW MANAGEMENT'),ctx);
  await ctx.terminalPathAction(1,'preview'); assert.deepEqual(previews,[[1,'/opt/media/clip.mp4']]);
  wo._selectedPath='~/sound.m4a';await ctx.terminalPathAction(1,'download');
  assert.match(requests[0],/share_token=demo/);assert.deepEqual(downloads,[[1,'/home/demo/sound.m4a']]);
});

test('shared windows resolve their connection from the correct share', () => {
  const ctx=vm.createContext({isCollabMode:false,conns:[],collabConns:[],sharedGroups:[{share:{token:'s1'},connections:[{id:7,name:'Shared'}]}]});
  vm.runInContext(slice('function winConn(', 'function isFtpConn('),ctx);
  assert.equal(ctx.winConn({connId:7,shareToken:'s1'}).name,'Shared');
  assert.equal(ctx.winConn({connId:7,shareToken:'wrong'}),undefined);
});

test('switching folder tabs focuses a window from the selected client/site', () => {
  const a={id:1,conn:{folder_id:2},_lastFocus:10,el:{style:{display:'none'}}},b={id:2,conn:{folder_id:4},_lastFocus:20,el:{style:{display:'flex'}}};
  let focused=0;
  const ctx=vm.createContext({winObjects:[a,b],winConn:wo=>wo.conn,scheduleFit(){},focusWindow:id=>{focused=id},renderTabs(){},winLayoutChanged(){}});
  vm.runInContext(slice('let tabFolderFilter = null;', 'function renderTabs()'),ctx);
  ctx.setTabFolder('2');assert.equal(focused,1);assert.equal(a.el.style.display,'flex');
  ctx.setTabFolder('4');assert.equal(focused,2);
});

test('window headers show the connection name, the folder path follows renames in the tooltip', () => {
  const title={textContent:'',title:''},conn={name:'Backup',protocol:'SSH',folder_id:2};
  const ctx=vm.createContext({folders:[{id:1,name:'Client-A'},{id:2,name:'Site-1',parent_id:1}],winConn:()=>conn,document:{getElementById:()=>title},t:k=>k});
  vm.runInContext(slice('function folderPathLabel(', 'function updateFolderSelects('),ctx);
  ctx.refreshWindowConnectionTitle({id:1,mode:'ssh'});assert.equal(title.textContent,'SSH — Backup');assert.equal(title.title,'Client-A / Site-1 / Backup');
  ctx.folders[0].name='Client-B';conn.name='Backup-2';ctx.refreshWindowConnectionTitle({id:1,mode:'ssh'});assert.equal(title.textContent,'SSH — Backup-2');assert.equal(title.title,'Client-B / Site-1 / Backup-2');
});
