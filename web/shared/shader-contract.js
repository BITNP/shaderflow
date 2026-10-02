const FORMATS = new Set(['rgba32float','rgba16float','rgba8unorm']);
const METADATA_BODY = 'time:f32,pos:vec3f,canvasSize:vec2i,rot:mat3x3f,invRot:mat3x3f,depth:f32';

export function describeNodeABI({inputs, outputs, format, blockSize}) {
  if (!Number.isInteger(inputs) || inputs < 1 || !Number.isInteger(outputs) || outputs < 1) throw new Error('inputs/outputs must be positive integers');
  if (!FORMATS.has(format)) throw new Error(`Unsupported format: ${format}`);
  if (![4,8,16].includes(blockSize)) throw new Error('Invalid block size');
  const canonical = `v1/${inputs}/${outputs}/${format}/${blockSize}/${METADATA_BODY}`;
  return {version:1, inputs, outputs, format, blockSize, hash:`shaderflow-abi-v1-${fnv1a(canonical)}`};
}

export function validateShaderContract(source, abi) {
  const code = stripCommentsAndStrings(String(source || ''));
  const errors = [];
  for (let i=0;i<abi.inputs;i++) {
    const expected = new RegExp(`@group\\s*\\(\\s*0\\s*\\)\\s*@binding\\s*\\(\\s*${i}\\s*\\)\\s*var\\s+_input${i}\\s*:\\s*texture_storage_2d\\s*<\\s*${abi.format}\\s*,\\s*read\\s*>\\s*;`, 'g');
    if (count(code, expected) !== 1) errors.push(`input${i} binding must be exactly @group(0) @binding(${i}) texture_storage_2d<${abi.format}, read>`);
  }
  for (let i=0;i<abi.outputs;i++) {
    const binding=abi.inputs+i;
    const expected = new RegExp(`@group\\s*\\(\\s*0\\s*\\)\\s*@binding\\s*\\(\\s*${binding}\\s*\\)\\s*var\\s+_output${i}\\s*:\\s*texture_storage_2d\\s*<\\s*${abi.format}\\s*,\\s*write\\s*>\\s*;`, 'g');
    if (count(code, expected) !== 1) errors.push(`output${i} binding must be exactly @group(0) @binding(${binding}) texture_storage_2d<${abi.format}, write>`);
    const helper = new RegExp(`fn\\s+write${i}\\s*\\(\\s*coord\\s*:\\s*vec2i\\s*,\\s*value\\s*:\\s*vec4f\\s*\\)[^{]*\\{[^}]*textureStore\\s*\\(\\s*_output${i}\\s*,`, 's');
    if (!helper.test(code)) errors.push(`output${i} canonical write helper is missing`);
    const write = new RegExp(`write${i}\\s*\\(\\s*coord\\s*,\\s*output${i}\\s*\\)\\s*;`);
    if (!write.test(code)) errors.push(`output${i} must be written through write${i}(coord, output${i})`);
  }
  const metadata = /struct\s+_metadata\s*\{([^}]*)\}/s.exec(code);
  if (!metadata || normalizeFields(metadata[1]) !== METADATA_BODY) errors.push('metadata layout does not match ShaderFlow ABI v1');
  if (!/@group\s*\(\s*1\s*\)\s*@binding\s*\(\s*0\s*\)\s*var\s*<\s*uniform\s*>\s*metadata\s*:\s*_metadata\s*;/.test(code)) errors.push('metadata uniform binding must be @group(1) @binding(0)');
  const workgroup = new RegExp(`@compute\\s*@workgroup_size\\s*\\(\\s*${abi.blockSize}\\s*,\\s*${abi.blockSize}\\s*\\)\\s*fn\\s+main\\s*\\(`);
  if (!workgroup.test(code)) errors.push(`main workgroup size must be ${abi.blockSize} x ${abi.blockSize}`);
  const resources = [...code.matchAll(/@group\s*\(\s*(\d+)\s*\)\s*@binding\s*\(\s*(\d+)\s*\)\s*var(?:\s*<[^>]+>)?\s+([A-Za-z_]\w*)/g)];
  const expectedResources = abi.inputs + abi.outputs + 1;
  if (resources.length !== expectedResources) errors.push(`unexpected resource binding: expected ${expectedResources}, found ${resources.length}`);
  return {ok:errors.length===0, errors, abi};
}

function stripCommentsAndStrings(source) {
  let out='', mode='code';
  for(let i=0;i<source.length;i++){
    const c=source[i], n=source[i+1];
    if(mode==='line'){if(c==='\n'){out+='\n';mode='code';}else out+=' ';continue;}
    if(mode==='block'){if(c==='*'&&n==='/'){out+='  ';i++;mode='code';}else out+=c==='\n'?'\n':' ';continue;}
    if(mode==='string'){if(c==='\\'&&n){out+='  ';i++;continue;}if(c==='"'){out+=' ';mode='code';}else out+=c==='\n'?'\n':' ';continue;}
    if(c==='/'&&n==='/'){out+='  ';i++;mode='line';continue;}
    if(c==='/'&&n==='*'){out+='  ';i++;mode='block';continue;}
    if(c==='"'){out+=' ';mode='string';continue;}
    out+=c;
  }
  return out;
}
function normalizeFields(body){return body.replace(/\s+/g,'').replace(/,+$/,'');}
function count(source, regex){return [...source.matchAll(regex)].length;}
function fnv1a(text){let h=0x811c9dc5;for(let i=0;i<text.length;i++){h^=text.charCodeAt(i);h=Math.imul(h,0x01000193);}return (h>>>0).toString(16).padStart(8,'0');}
