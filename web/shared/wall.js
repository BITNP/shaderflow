export const DEFAULT_EDITABLE = `// 入门：x、y 是位置，范围 0.0～1.0；t 是不断增加的时间。
// 返回值保持在 -1.0～1.0：正数、负数分别使用下面两种颜色。
// // 后面是注释，不会执行。小数写成 1.0；每条语句以 ; 结尾。

// 颜色：vec3f(红, 绿, 蓝)，每个分量为 0.0～1.0。
const positiveColor = vec3f(1.0, 0.99, 0.95);
const negativeColor = vec3f(1.0, 0.63, 0.33);

// 常用函数速查：
// sin(a)、cos(a)      波动，结果 -1～1；a 用弧度，一圈约 6.28。
// abs(a)             绝对值：abs(-0.5) = 0.5。
// min(a, b)          取较小值；max(a, b) 取较大值。
// clamp(a, lo, hi)    把 a 限制在 lo～hi。
// floor(a)           向下取整：floor(2.7) = 2.0。
// fract(a)           a - floor(a)，用于重复图案。
// step(edge, a)      a < edge 时为 0.0，否则为 1.0。
// smoothstep(lo, hi, a)  从 lo 到 hi 平滑过渡为 0～1，要求 lo < hi。
// mix(a, b, k)       混合：k=0 得到 a，k=1 得到 b。
// length(vec2f(a, b)) 求长度，可用来计算到圆心的距离。

fn userfn(x: f32, y: f32, t: f32) -> f32 {
    // 默认：从左到右渐变。
    return 2.0 * x - 1.0;

    // 玩法：用下面任意一行替换上面的 return，每次只保留一个。
    // 波纹：增大 20.0 会更密，增大 t 前的系数会更快。
    // return sin(x * 20.0 + t * 2.0);

    // 重复渐变：把画面分成 8 段。
    // return fract(x * 8.0) * 2.0 - 1.0;

    // 双色分区：左边负值，右边正值。
    // return step(0.5, x) * 2.0 - 1.0;

    // 圆形波纹：先用 let 保存到画面中心的距离。
    // let d = length(vec2f(x - 0.5, y - 0.5));
    // return cos(d * 30.0 - t * 2.0);
}
// 参考文献:
// [1] https://tixy.land/
// [2] https://www.w3.org/TR/WGSL/
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
  constructor({onGpuTiming}={}) { this.device=null;this.format=null;this.pipelines=new Map();this.targets=new Map();this.uniform=null;this.bindGroups=new Map();this.onGpuTiming=onGpuTiming||null;this.gpuTimingSupported=false;this.timestampPeriod=0;this.timingRings=new Map(); }
  async init() {
    if (!navigator.gpu) throw new Error('浏览器不支持 WebGPU；请使用 HTTPS 和支持 WebGPU 的浏览器');
    const adapter=await navigator.gpu.requestAdapter();if (!adapter) throw new Error('没有可用的 WebGPU 适配器');
    const wantsTimestamps=adapter.features?.has?.('timestamp-query');
    try { this.device=await adapter.requestDevice(wantsTimestamps?{requiredFeatures:['timestamp-query']}:{}); }
    catch (error) { if (!wantsTimestamps) throw error;this.device=await adapter.requestDevice(); }
    this.format=navigator.gpu.getPreferredCanvasFormat();
    this.uniform=this.device.createBuffer({size:16,usage:GPUBufferUsage.UNIFORM|GPUBufferUsage.COPY_DST});
    if (this.onGpuTiming && this.device.features?.has?.('timestamp-query') && typeof this.device.queue.getTimestampPeriod==='function') {
      const period=this.device.queue.getTimestampPeriod();
      if (Number.isFinite(period) && period>0) { this.timestampPeriod=period;this.gpuTimingSupported=true; }
    }
    return this;
  }
  assertReady() { if (!this.device) throw new Error('渲染器尚未初始化'); }
  async compile(editable) {
    this.assertReady();const source=buildShaderSource(editable);const module=this.device.createShaderModule({code:source});const errors=compilationErrors(await module.getCompilationInfo());if(errors.length)return {ok:false,errors};
    try { this.device.pushErrorScope('validation');const pipeline=await this.device.createRenderPipelineAsync({layout:'auto',vertex:{module,entryPoint:'vs'},fragment:{module,entryPoint:'fs',targets:[{format:this.format}]},primitive:{topology:'triangle-list'}});const error=await this.device.popErrorScope();if(error)return {ok:false,errors:[error.message]};return {ok:true,pipeline,source,errors:[]}; }
    catch(error) { await this.device.popErrorScope().catch(()=>{});return {ok:false,errors:[String(error?.message||error)]}; }
  }
  async setShader(id,editable) { const result=await this.compile(editable);if(result.ok){this.pipelines.set(id,result.pipeline);this.bindGroups.delete(id);}return result; }
  setTarget(id,canvas,size) { this.assertReady();const side=Math.max(1,Math.floor(size));canvas.width=side;canvas.height=side;const context=canvas.getContext('webgpu');if(!context)throw new Error('无法创建 WebGPU Canvas 上下文');context.configure({device:this.device,format:this.format,alphaMode:'opaque'});this.targets.set(id,{canvas,context}); }
  nextTimingSlot(id) {
    if (!this.gpuTimingSupported) return null;
    let ring=this.timingRings.get(id);
    if (!ring) { ring={next:0,slots:Array.from({length:3},()=>({query:this.device.createQuerySet({type:'timestamp',count:2}),resolve:this.device.createBuffer({size:16,usage:GPUBufferUsage.QUERY_RESOLVE|GPUBufferUsage.COPY_SRC}),read:this.device.createBuffer({size:16,usage:GPUBufferUsage.COPY_DST|GPUBufferUsage.MAP_READ}),pending:false}))};this.timingRings.set(id,ring); }
    for(let attempt=0;attempt<ring.slots.length;attempt++){const slot=ring.slots[ring.next];ring.next=(ring.next+1)%ring.slots.length;if(!slot.pending){slot.pending=true;return slot;}}
    return null;
  }
  readTiming(id,slot) {
    slot.read.mapAsync(GPUMapMode.READ).then(()=>{const raw=new BigUint64Array(slot.read.getMappedRange().slice(0));slot.read.unmap();slot.pending=false;const milliseconds=Number(raw[1]-raw[0])*this.timestampPeriod/1e6;if(Number.isFinite(milliseconds)&&milliseconds>=0)this.onGpuTiming?.(id,milliseconds);}).catch(()=>{slot.pending=false;});
  }
  draw(id,frame) {
    this.assertReady();const pipeline=this.pipelines.get(id),target=this.targets.get(id);if(!pipeline||!target)throw new Error('shader 或画布尚未准备好');let bindGroup=this.bindGroups.get(id);if(!bindGroup){bindGroup=this.device.createBindGroup({layout:pipeline.getBindGroupLayout(0),entries:[{binding:0,resource:{buffer:this.uniform}}]});this.bindGroups.set(id,bindGroup);}
    this.device.queue.writeBuffer(this.uniform,0,new Uint32Array([frame>>>0]));const encoder=this.device.createCommandEncoder(),timing=this.nextTimingSlot(id);if(timing)encoder.writeTimestamp(timing.query,0);const pass=encoder.beginRenderPass({colorAttachments:[{view:target.context.getCurrentTexture().createView(),clearValue:{r:0,g:0,b:0,a:1},loadOp:'clear',storeOp:'store'}]});pass.setPipeline(pipeline);pass.setBindGroup(0,bindGroup);pass.draw(3);pass.end();if(timing){encoder.writeTimestamp(timing.query,1);encoder.resolveQuerySet(timing.query,0,2,timing.resolve,0);encoder.copyBufferToBuffer(timing.resolve,0,timing.read,0,16);}this.device.queue.submit([encoder.finish()]);if(timing)this.readTiming(id,timing);
  }
  destroy() { this.uniform?.destroy();for(const ring of this.timingRings.values())for(const slot of ring.slots){slot.query.destroy?.();slot.resolve.destroy();slot.read.destroy();}this.timingRings.clear();this.pipelines.clear();this.targets.clear();this.bindGroups.clear(); }
}
