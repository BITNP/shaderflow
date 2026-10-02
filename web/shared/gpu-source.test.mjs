import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = await readFile(new URL('./gpu.js', import.meta.url), 'utf8');

test('preview WGSL does not use nonexistent isNan builtin', () => {
  assert.equal(source.includes('isNan('), false);
  assert.match(source, /let finite= c == c;/);
});

test('read-only storage texture feature name is not a hard rejection', () => {
  assert.doesNotMatch(source, /features\.has\([^)]*\)\)throw new Error\([^)]*read-only/);
  assert.match(source, /probeReadOnlyStorageTexture/);
});

test('texture uploads use the four-argument WebGPU signature', () => {
  assert.match(source, /queue\.writeTexture\(\{texture:[^}]+\},upload,\{bytesPerRow:/);
  assert.doesNotMatch(source, /writeTexture\(\{texture:[^}]+\},\{data:/);
});

test('reconfiguration waits for active GPU operations before destroying textures', () => {
  assert.match(source, /async configure/);
  assert.match(source, /await this\.waitForIdle\(\);await this\.device\.queue\.onSubmittedWorkDone\(\);this\.destroyTextures\(\)/);
  assert.match(source, /this\.beginOperation\(\)/);
  assert.match(source, /this\.endOperation\(\)/);
});

test('readback pipelines are cached by texture format', () => {
  assert.match(source, /readbackPipelines=new Map\(\)/);
  assert.match(source, /if\(this\.readbackPipelines\.has\(format\)\)return this\.readbackPipelines\.get\(format\)/);
  assert.match(source, /this\.readbackPipelines\.set\(format,pipeline\)/);
});

test('live previews use full-screen triangle GPU canvas presentation', () => {
  assert.match(source, /GPUTextureUsage\.TEXTURE_BINDING/);
  assert.match(source, /fn buildScreenCover/);
  assert.match(source, /pass\.draw\(3\)/);
  assert.match(source, /getContext\('webgpu'\)/);
  assert.match(source, /encodePresentPasses\(encoder\)/);
});

test('capture rerenders an isolated graph at the requested size', () => {
  assert.match(source, /async capture\(/);
  assert.match(source, /const isolated=new Renderer\(\{device:this\.device\}\)/);
  assert.match(source, /isolated\.configure\(this\.graph,target,this\.format,this\.block\)/);
  assert.match(source, /new Int32Array\(packed\)\[8\]=target\.width/);
  assert.match(source, /isolated\.frame\(packed/);
  assert.doesNotMatch(source, /ctx\.drawImage\(original/);
});
