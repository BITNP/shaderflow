import test from 'node:test';
import assert from 'node:assert/strict';
import { DEFAULT_EDITABLE, buildShaderSource, parseEditable } from './wall.js';

test('locked editable document accepts only colors and userfn', () => {
  const parts = parseEditable(DEFAULT_EDITABLE);
  assert.match(parts.body, /return sin/);
  const full = buildShaderSource(DEFAULT_EDITABLE);
  assert.match(full, /@group\(0\) @binding\(0\) var<uniform> frameData/);
  assert.match(full, /@fragment fn fs/);
});

test('locked editable document rejects wrapper injection', () => {
  assert.throws(() => parseEditable(`${DEFAULT_EDITABLE}\n@fragment fn injected() {}`), /只能编辑/);
  assert.throws(() => parseEditable(DEFAULT_EDITABLE.replace('return sin', '@group(0) return sin')), /不能声明/);
});
