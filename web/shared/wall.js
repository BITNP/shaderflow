export const DEFAULT_EDITABLE = `// 预定义好的颜色，用户可编辑
const positiveColor = vec3f(255.,254.,244.)/256;
const negativeColor = vec3f(255.,160.,85.)/256.;

// 用户函数，其中 x y 为 0~1
fn userfn(x: f32, y: f32, t: f32) -> f32 {
  return sin(t + x * 6.283185 + y * 3.141593);
}
`;

function skipTrivia(source, index) {
  while (index < source.length) {
    if (/\s/.test(source[index])) { index++; continue; }
    if (source.startsWith('//', index)) { const end = source.indexOf('\n', index); index = end < 0 ? source.length : end + 1; continue; }
    if (source.startsWith('/*', index)) { const end = source.indexOf('*/', index + 2); if (end < 0) throw new Error('未闭合的块注释'); index = end + 2; continue; }
    break;
  }
  return index;
}
function matchAt(source, index, expression) { const m = expression.exec(source.slice(index)); return m ? [m, index + m[0].length] : [null, index]; }
function matchingBrace(source, start) {
  let depth = 0, mode = 'code';
  for (let i = start; i < source.length; i++) {
    const c = source[i], next = source[i + 1];
    if (mode === 'line') { if (c === '\n') mode = 'code'; continue; }
    if (mode === 'block') { if (c === '*' && next === '/') { mode = 'code'; i++; } continue; }
    if (mode === 'string') { if (c === '\\') { i++; continue; } if (c === '"') mode = 'code'; continue; }
    if (c === '/' && next === '/') { mode = 'line'; i++; continue; }
    if (c === '/' && next === '*') { mode = 'block'; i++; continue; }
    if (c === '"') { mode = 'string'; continue; }
    if (c === '{') depth++;
    if (c === '}' && --depth === 0) return i;
  }
  throw new Error('userfn 的大括号没有闭合');
}

// The editable document is deliberately parsed as exactly two constants plus one
// function. The locked wrapper is never sent from the player or editable in UI.
export function parseEditable(value) {
  const source = String(value || '');
  let index = skipTrivia(source, 0), match;
  [match, index] = matchAt(source, index, /^const\s+positiveColor\s*=\s*([^;{}]+)\s*;/);
  if (!match) throw new Error('必须保留 const positiveColor = ...;');
  const positive = match[1].trim();
  index = skipTrivia(source, index);
  [match, index] = matchAt(source, index, /^const\s+negativeColor\s*=\s*([^;{}]+)\s*;/);
  if (!match) throw new Error('必须保留 const negativeColor = ...;');
  const negative = match[1].trim();
  index = skipTrivia(source, index);
  [match, index] = matchAt(source, index, /^fn\s+userfn\s*\(\s*x\s*:\s*f32\s*,\s*y\s*:\s*f32\s*,\s*t\s*:\s*f32\s*\)\s*->\s*f32\s*\{/);
  if (!match) throw new Error('必须保留 fn userfn(x: f32, y: f32, t: f32) -> f32 { ... }');
  const open = index - 1, close = matchingBrace(source, open);
  if (skipTrivia(source, close + 1) !== source.length) throw new Error('只能编辑颜色常量和 userfn 函数');
  const body = source.slice(open + 1, close);
  if (/@(group|binding|vertex|fragment|compute)|\b(fn\s+(fs|vs)|struct\s+(Frame|VertexOut)|frameData)\b/.test(body)) throw new Error('userfn 不能声明管线资源或覆盖锁定函数');
  return {positive, negative, body};
}

export function buildShaderSource(editable) {
  const {positive, negative, body} = parseEditable(editable);
  return `struct Frame { frame : u32, };
@group(0) @binding(0) var<uniform> frameData : Frame;

struct VertexOut {
  @builtin(position) position : vec4f,
  @location(0) uv : vec2f,
};

@vertex fn vs(@builtin(vertex_index) vertexIndex : u32) -> VertexOut {
  let pos = array(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
  let p = pos[vertexIndex];
  var out : VertexOut;
  out.position = vec4f(p, 0.0, 1.0);
  out.uv = p * 0.5 + 0.5;
  return out;
}

const positiveColor = ${positive};
const negativeColor = ${negative};

fn userfn(x: f32, y: f32, t: f32) -> f32 {${body}}

@fragment fn fs(in : VertexOut) -> @location(0) vec4f {
  let t = f32(frameData.frame) * 0.01;
  let uv = in.uv;
  let ret = clamp(userfn(uv.x, uv.y, t), -1.0, 1.0);
  return vec4f(select(negativeColor * -ret, positiveColor * ret, ret > 0.0), 1.0);
}`;
}

function compilationErrors(info) { return info.messages.filter(message => message.type === 'error').map(message => `${message.lineNum}:${message.linePos} ${message.message}`); }

export class WallRenderer {
  constructor() { this.device = null; this.format = null; this.pipelines = new Map(); this.targets = new Map(); this.uniform = null; this.bindGroups = new Map(); }
  async init() {
    if (!navigator.gpu) throw new Error('浏览器不支持 WebGPU；请使用 HTTPS 和支持 WebGPU 的浏览器');
    const adapter = await navigator.gpu.requestAdapter();
    if (!adapter) throw new Error('没有可用的 WebGPU 适配器');
    this.device = await adapter.requestDevice();
    this.format = navigator.gpu.getPreferredCanvasFormat();
    this.uniform = this.device.createBuffer({size:16, usage:GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST});
    return this;
  }
  assertReady() { if (!this.device) throw new Error('渲染器尚未初始化'); }
  async compile(editable) {
    this.assertReady();
    const source = buildShaderSource(editable);
    const module = this.device.createShaderModule({code:source});
    const errors = compilationErrors(await module.getCompilationInfo());
    if (errors.length) return {ok:false, errors};
    try {
      this.device.pushErrorScope('validation');
      const pipeline = await this.device.createRenderPipelineAsync({layout:'auto', vertex:{module,entryPoint:'vs'}, fragment:{module,entryPoint:'fs',targets:[{format:this.format}]}, primitive:{topology:'triangle-list'}});
      const error = await this.device.popErrorScope();
      if (error) return {ok:false,errors:[error.message]};
      return {ok:true,pipeline,source,errors:[]};
    } catch (error) {
      await this.device.popErrorScope().catch(() => {});
      return {ok:false,errors:[String(error?.message || error)]};
    }
  }
  async setShader(id, editable) { const result = await this.compile(editable); if (result.ok) { this.pipelines.set(id,result.pipeline); this.bindGroups.delete(id); } return result; }
  setTarget(id, canvas, size) {
    this.assertReady();
    const side = Math.max(1, Math.floor(size));
    canvas.width = side; canvas.height = side;
    const context = canvas.getContext('webgpu');
    if (!context) throw new Error('无法创建 WebGPU Canvas 上下文');
    context.configure({device:this.device,format:this.format,alphaMode:'opaque'});
    this.targets.set(id,{canvas,context});
  }
  draw(id, frame) {
    this.assertReady();
    const pipeline = this.pipelines.get(id), target = this.targets.get(id);
    if (!pipeline || !target) throw new Error('shader 或画布尚未准备好');
    let bindGroup = this.bindGroups.get(id);
    if (!bindGroup) { bindGroup = this.device.createBindGroup({layout:pipeline.getBindGroupLayout(0),entries:[{binding:0,resource:{buffer:this.uniform}}]});this.bindGroups.set(id,bindGroup); }
    this.device.queue.writeBuffer(this.uniform,0,new Uint32Array([frame >>> 0]));
    const encoder=this.device.createCommandEncoder();
    const pass=encoder.beginRenderPass({colorAttachments:[{view:target.context.getCurrentTexture().createView(),clearValue:{r:0,g:0,b:0,a:1},loadOp:'clear',storeOp:'store'}]});
    pass.setPipeline(pipeline);pass.setBindGroup(0,bindGroup);pass.draw(3);pass.end();this.device.queue.submit([encoder.finish()]);
  }
  destroy() { this.uniform?.destroy();this.pipelines.clear();this.targets.clear();this.bindGroups.clear(); }
}
