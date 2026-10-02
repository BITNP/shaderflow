import test from 'node:test';
import assert from 'node:assert/strict';
import { generateTemplate } from './gpu.js';
import { describeNodeABI, validateShaderContract } from './shader-contract.js';

const abi = describeNodeABI({inputs:2, outputs:2, format:'rgba16float', blockSize:8});
const valid = generateTemplate(2, 2, 'rgba16float', 8);

test('canonical generated template satisfies the node ABI', () => {
  const result = validateShaderContract(valid, abi);
  assert.deepEqual(result.errors, []);
  assert.equal(result.ok, true);
  assert.match(abi.hash, /^shaderflow-abi-v1-/);
});

test('rejects changed input binding and storage access', () => {
  const source = valid.replace('@binding(1) var _input1: texture_storage_2d<rgba16float, read>', '@binding(7) var _input1: texture_storage_2d<rgba16float, write>');
  const result = validateShaderContract(source, abi);
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /input1 binding/);
});

test('rejects extra resource bindings', () => {
  const source = `${valid}\n@group(2) @binding(0) var extraSampler: sampler;`;
  const result = validateShaderContract(source, abi);
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /unexpected resource binding/);
});

test('rejects metadata layout or workgroup changes', () => {
  const changedMetadata = valid.replace('depth: f32,', 'depth: vec2f,');
  assert.match(validateShaderContract(changedMetadata, abi).errors.join('\n'), /metadata layout/);
  const changedBlock = valid.replace('@workgroup_size(8, 8)', '@workgroup_size(16, 8)');
  assert.match(validateShaderContract(changedBlock, abi).errors.join('\n'), /workgroup size/);
});

test('rejects missing canonical output write', () => {
  const source = valid.replace('write1(coord, output1);', '// deliberately omitted');
  const result = validateShaderContract(source, abi);
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /output1.*written/);
});

test('comments cannot spoof protected bindings or writes', () => {
  const source = valid
    .replace('@group(0) @binding(0) var _input0: texture_storage_2d<rgba16float, read>;', '// @group(0) @binding(0) var _input0: texture_storage_2d<rgba16float, read>;')
    .replace('write0(coord, output0);', '// write0(coord, output0);');
  const result = validateShaderContract(source, abi);
  assert.match(result.errors.join('\n'), /input0 binding/);
  assert.match(result.errors.join('\n'), /output0.*written/);
});
