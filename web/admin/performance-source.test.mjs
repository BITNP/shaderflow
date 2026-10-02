import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = await readFile(new URL('./app.js', import.meta.url), 'utf8');

test('admin uses direct GPU presentation without an A/B selector', () => {
  assert.doesNotMatch(source, /previewMode/);
  assert.match(source, /setPresentTargets/);
  assert.match(source, /GPU 直接呈现/);
  assert.doesNotMatch(source, /previewAllTextures\(\)/);
});

test('admin displays GPU submission and direct presentation status', () => {
  assert.match(source, /GPU 提交 \$\{fps\.toFixed\(1\)\} FPS/);
  assert.match(source, /estimatedTextureBytes/);
  assert.match(source, /连续超预算/);
  assert.match(source, /GPU 直接呈现/);
});

test('admin attempts device-lost recovery only once', () => {
  assert.match(source, /gpuRecoveryCount>=1/);
  assert.match(source, /new Renderer\(\{onDeviceLost:recoverGPU\}\)/);
  assert.match(source, /WebGPU 已恢复/);
});
