import { createCaptureSession } from './shared/capture-session.js';

const captureSessions = new WeakMap();
export const $ = (selector, root = document) => root.querySelector(selector);
export const el = (tag, attributes = {}, ...children) => {
  const node = document.createElement(tag);
  for (const [name, value] of Object.entries(attributes)) {
    if (name === 'className') node.className = value;
    else if (name === 'text') node.textContent = String(value);
    else if (name.startsWith('on') && typeof value === 'function') node.addEventListener(name.slice(2).toLowerCase(), value);
    else if (value !== undefined && value !== null) node.setAttribute(name, String(value));
  }
  for (const child of children.flat()) if (child !== null && child !== undefined) node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  return node;
};
export async function api(path, options = {}) {
  const response = await fetch(path, { credentials: 'same-origin', ...options, headers: { ...(options.body ? {'Content-Type': 'application/json'} : {}), ...options.headers } });
  if (!response.ok) {
    const text = await response.text();
    let message = text;
    try { const data = JSON.parse(text); message = data.error || data.message || text; } catch {}
    throw new Error(`${response.status}: ${message || response.statusText}`);
  }
  if (response.status === 204) return null;
  const text = await response.text();
  return text ? JSON.parse(text) : null;
}
export const json = (body) => JSON.stringify(body);
export const errorText = (error) => error?.message || String(error);
export function announce(message, kind = 'info') {
  const area = $('#notices'); if (!area) return;
  const item = el('div', { className: `notice ${kind}`, text: message }); area.prepend(item);
  setTimeout(() => item.remove(), 11000);
}
export function openModal(title, content) {
  const backdrop = el('div', { className: 'modal-backdrop' });
  const dialog = el('div', { className: 'modal' }, el('div', { className: 'modal-heading' }, el('h2', { text: title }), el('button', { type:'button', text:'关闭', onclick: () => backdrop.remove() })), content);
  backdrop.append(dialog);
  backdrop.addEventListener('click', (event) => { if (event.target === backdrop) backdrop.remove(); });
  document.body.append(backdrop);
  return backdrop;
}
export function fullscreenImage(source, title='图像预览') {
  const image = source instanceof HTMLCanvasElement ? source.toDataURL() : source;
  openModal(title, el('img', { src:image, className:'full-image', alt:title }));
}
export function download(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = el('a', { href:url, download:filename }); document.body.append(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}
export function startSocket(onMessage, onStatus=()=>{}) {
  let last = 0, socket, stopped = false, delay = 500;
  const connect = () => {
    if (stopped) return;
    const url = new URL('/api/ws', location.href);
    url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    url.searchParams.set('since',String(last));
    socket = new WebSocket(url);
    socket.onopen = () => { delay=500; onStatus('已连接'); };
    socket.onmessage = async e => { try { const msg=JSON.parse(e.data); await onMessage(msg); if (Number.isFinite(msg.seq)) last=Math.max(last,msg.seq); } catch (err) { announce(`消息处理失败：${errorText(err)}`,'error'); socket.close(); } };
    socket.onerror = () => onStatus('连接异常');
    socket.onclose = () => { onStatus('连接中断，正在重连'); if(!stopped) { setTimeout(connect,delay); delay=Math.min(12000,Math.round(delay*1.7)); } };
  };
  connect();
  return () => { stopped=true; socket?.close(); };
}
export async function showPaint(initialWidth, initialHeight) {
  const width=Math.max(1,Math.floor(initialWidth)), height=Math.max(1,Math.floor(initialHeight));
  return new Promise(resolve => {
    const canvas=el('canvas',{width,height,className:'paint-canvas'}); const ctx=canvas.getContext('2d');
    ctx.fillStyle='white';ctx.fillRect(0,0,width,height);
    const color=el('input',{type:'color',value:'#ff4766',title:'颜色'}), size=el('input',{type:'range',min:1,max:96,value:12,title:'线宽'});
    let drawing=false, previous=null, history=[], future=[], erasing=false;
    const save=()=>{if(history.length>20)history.shift();history.push(ctx.getImageData(0,0,width,height));future=[];};
    const point=e=>{const r=canvas.getBoundingClientRect();return [(e.clientX-r.left)*width/r.width,(e.clientY-r.top)*height/r.height];};
    canvas.addEventListener('pointerdown',e=>{e.preventDefault();drawing=true;canvas.setPointerCapture(e.pointerId);save();previous=point(e);});
    canvas.addEventListener('pointermove',e=>{if(!drawing)return;const now=point(e);ctx.save();ctx.beginPath();ctx.moveTo(...previous);ctx.lineTo(...now);ctx.globalCompositeOperation=erasing?'destination-out':'source-over';ctx.strokeStyle=color.value;ctx.lineWidth=Number(size.value);ctx.lineCap='round';ctx.lineJoin='round';ctx.stroke();ctx.restore();previous=now;});
    canvas.addEventListener('pointerup',()=>{drawing=false;});
    const controls=el('div',{className:'row'},color,size,
      el('button',{type:'button',text:'橡皮',onclick:()=>{erasing=!erasing;}}),
      el('button',{type:'button',text:'撤销',onclick:()=>{if(history.length){future.push(ctx.getImageData(0,0,width,height));ctx.putImageData(history.pop(),0,0);}}}),
      el('button',{type:'button',text:'重做',onclick:()=>{if(future.length){history.push(ctx.getImageData(0,0,width,height));ctx.putImageData(future.pop(),0,0);}}}),
      el('button',{type:'button',text:'清空',onclick:()=>{save();ctx.fillStyle='white';ctx.fillRect(0,0,width,height);}}),
      el('button',{type:'button',text:'导出PNG',onclick:()=>canvas.toBlob(blob=>download(blob,'paint.png'))}),
      el('button',{type:'button',text:'使用此图',onclick:()=>{const copy=document.createElement('canvas');copy.width=width;copy.height=height;copy.getContext('2d').drawImage(canvas,0,0);modal.remove();resolve(copy);}}),
      el('button',{type:'button',text:'取消',onclick:()=>{modal.remove();resolve(null);}}));
    const modal=openModal(`画板 · ${width} × ${height}`,el('div',{},controls,canvas));
  });
}
export function imageFile(file) {
  return new Promise((resolve,reject)=>{
    if (!file || !['image/png','image/jpeg'].includes(file.type)) return reject(new Error('仅支持 PNG/JPEG'));
    const img=new Image(), url=URL.createObjectURL(file);
    img.onload=()=>{URL.revokeObjectURL(url);if(img.naturalWidth>4096||img.naturalHeight>4096){reject(new Error('图像不得超过 4096 × 4096'));return;}resolve(img);};
    img.onerror=()=>{URL.revokeObjectURL(url);reject(new Error('图像无法读取'));};img.src=url;
  });
}
export function makeTextureControls({id,width,height,onImage,onSave}) {
  const canvas=el('canvas',{width:Math.min(width,480),height:Math.min(height,270),className:'texture-canvas',title:'实时 GPU 预览'});
  canvas.addEventListener('click',()=>fullscreenImage(canvas,id));
  const file=el('input',{type:'file',accept:'image/png,image/jpeg',hidden:''});
  const mode=el('select',{},...['stretch','crop','contain-black','contain-transparent'].map((value,i)=>el('option',{value,text:['拉伸','裁剪填满','留黑边','留透明边'][i]})));
  file.addEventListener('change',async()=>{try{if(file.files[0])await onImage(await imageFile(file.files[0]),mode.value);}catch(err){announce(errorText(err),'error');}file.value='';});
  return {
    canvas,
    element: el(
      'div',
      {className: 'texture-card'},
      el('strong', {text: id}),
      canvas,
      el(
        'div',
        {className: 'row'},
        mode,
        el('button', {type: 'button', text: '上传', onclick: () => file.click()}),
        el('button', {
          type: 'button',
          text: '画板',
          onclick: async () => {
            const image = await showPaint(width, height);
            if (image) {
              try { await onImage(image, mode.value); }
              catch (err) { announce(errorText(err), 'error'); }
            }
          },
        }),
        el('button', {type: 'button', text: '保存', onclick: () => onSave?.(id)}),
        file,
      ),
    ),
  };
}
export async function saveDialog(renderer,id,width,height,manifest={},onCaptured) {
  const w=el('input',{type:'number',min:1,max:width,value:width}),h=el('input',{type:'number',min:1,max:height,value:height});
  const format=el('select',{},el('option',{value:'png',text:'PNG'}),el('option',{value:'jpeg',text:'JPEG'}));
  const status=el('span',{text:'导出使用当前参数快照'});
  const box=el('div',{className:'stack'},el('label',{},'宽度 ',w),el('label',{},'高度 ',h),el('label',{},'格式 ',format),status);
  const modal=openModal(`保存纹理 · ${id}`,box);
  box.append(el('button',{type:'button',text:'渲染并保存',onclick:async()=>{
    const sw=Number(w.value),sh=Number(h.value);
    if(!Number.isInteger(sw)||!Number.isInteger(sh)||sw<1||sh<1||sw>width||sh>height){status.textContent='尺寸必须大于零且不超过实时尺寸';return;}
    try{status.textContent='正在冻结状态并隔离渲染…';let session=captureSessions.get(renderer);if(!session){session=createCaptureSession(renderer,{onCaptured});captureSessions.set(renderer,session);}const blob=await session.capture({textureId:id,size:{width:sw,height:sh},format:format.value,manifest}); download(blob,`shaderflow-${id}.${format.value==='jpeg'?'jpg':'png'}`);modal.remove();}
    catch(err){status.textContent=errorText(err);}
  }}));
}
