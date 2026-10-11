export const $ = (selector, root=document) => root.querySelector(selector);
export function el(tag, attributes={}, ...children) {
  const node=document.createElement(tag);
  for (const [name,value] of Object.entries(attributes)) {
    if (name==='className') node.className=value;
    else if (name==='text') node.textContent=String(value);
    else if (name.startsWith('on') && typeof value==='function') node.addEventListener(name.slice(2).toLowerCase(),value);
    else if (value!==undefined&&value!==null) node.setAttribute(name,String(value));
  }
  for (const child of children.flat()) if (child!==null&&child!==undefined) node.append(child instanceof Node?child:document.createTextNode(String(child)));
  return node;
}
export async function api(path,options={}) {
  const response=await fetch(path,{credentials:'same-origin',...options,headers:{...(options.body?{'Content-Type':'application/json'}:{}),...options.headers}});
  if (!response.ok) throw new Error(`${response.status}: ${(await response.text())||response.statusText}`);
  if (response.status===204) return null;
  const text=await response.text();return text?JSON.parse(text):null;
}
export const json=value=>JSON.stringify(value);
export const errorText=error=>error?.message||String(error);
export function announce(message,kind='info') { const area=$('#notices');if(!area)return;const item=el('div',{className:`notice ${kind}`,text:message});area.append(item);setTimeout(()=>item.remove(),8000); }
export function startSocket(onMessage,onStatus=()=>{}) {
  let last=0,socket,stopped=false,delay=500;
  const connect=()=>{if(stopped)return;const url=new URL('/api/ws',location.href);url.protocol=location.protocol==='https:'?'wss:':'ws:';url.searchParams.set('since',String(last));socket=new WebSocket(url);socket.onopen=()=>{delay=500;onStatus('已连接','success');};socket.onmessage=async event=>{try{const message=JSON.parse(event.data);await onMessage(message);if(Number.isFinite(message.seq))last=Math.max(last,message.seq);}catch(error){announce(`同步失败：${errorText(error)}`,'error');socket.close();}};socket.onclose=()=>{onStatus('连接中断，正在重连','warning');if(!stopped){setTimeout(connect,delay);delay=Math.min(12000,Math.round(delay*1.7));}};socket.onerror=()=>onStatus('连接异常','error');};
  connect();return()=>{stopped=true;socket?.close();};
}
