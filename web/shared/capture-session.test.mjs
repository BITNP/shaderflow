import test from 'node:test';
import assert from 'node:assert/strict';
import { createCaptureSession } from './capture-session.js';

test('capture freezes request state before asynchronous isolated rendering', async () => {
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  let received;
  const renderer = {capture: async request => { received = request; await gate; return new Blob(['ok']); }};
  const session = createCaptureSession(renderer);
  const state = {textureId:'output0',size:{width:320,height:180},format:'png',manifest:{time:123,codeVersions:{n0:7}}};
  const pending = session.capture(state);
  state.size.width = 1;
  state.manifest.codeVersions.n0 = 99;
  release();
  await pending;
  assert.deepEqual(received, {textureId:'output0',size:{width:320,height:180},format:'png',manifest:{time:123,codeVersions:{n0:7}}});
});

test('capture publishes the same frozen request with the rendered blob', async () => {
  let published;
  const session=createCaptureSession({capture:async()=>new Blob(['png'])},{onCaptured:async value=>{published=value;}});
  const request={textureId:'out',size:{width:2,height:3},format:'png',manifest:{time:42}};
  await session.capture(request);request.manifest.time=99;
  assert.equal(published.request.manifest.time,42);
  assert.equal(await published.blob.text(),'png');
});

test('capture allows only one isolated render at a time', async () => {
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  const session = createCaptureSession({capture: async () => { await gate; return new Blob(['ok']); }});
  const first = session.capture({textureId:'output0',size:{width:1,height:1},format:'png'});
  await assert.rejects(session.capture({textureId:'output0',size:{width:1,height:1},format:'png'}), /already in progress/);
  release();
  await first;
});
