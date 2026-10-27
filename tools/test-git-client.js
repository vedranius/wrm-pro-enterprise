/* Copyright 2026 vedranius and WRM contributors
   SPDX-License-Identifier: Apache-2.0 */

'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'../remote-manager/static/git_client.js'),'utf8');
const start=source.indexOf(' async function boundedResponse(');
const end=source.indexOf('\n async function pump(',start);
assert.ok(start>=0&&end>start);
const boundedResponse=vm.runInNewContext(source.slice(start,end)+';boundedResponse',{Uint8Array,Error,t:()=> 'response limit'});

test('client relay reconstructs bounded response bytes',async()=>{
 const bytes=await boundedResponse(new Response(new Uint8Array([0,1,2,255])));
 assert.deepEqual(Array.from(bytes),[0,1,2,255]);
});
test('client relay cancels an announced oversize response',async()=>{
 let canceled=false;
 const body=new ReadableStream({cancel(){canceled=true}});
 await assert.rejects(boundedResponse(new Response(body,{headers:{'Content-Length':String(9*1024*1024)}})),{name:'RelayLimitError'});
 assert.equal(canceled,true);
});
test('client relay stops an unknown-length stream at its byte budget',async()=>{
 let reads=0,canceled=false;
 const body=new ReadableStream({pull(c){reads++;c.enqueue(new Uint8Array(1024*1024));if(reads===100)c.close()},cancel(){canceled=true}});
 await assert.rejects(boundedResponse(new Response(body)),{name:'RelayLimitError'});
 assert.equal(canceled,true);
 assert.ok(reads<=10,'continued downloading after the byte budget');
});
