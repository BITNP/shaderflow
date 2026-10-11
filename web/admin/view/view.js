import { WallRenderer } from '../../shared/wall.js';
import { $, el, api, errorText, startSocket } from '../../ui.js';

const model={state:null,renderer:null,canvases:new Map(),sources:new Map(),socketStop:null,drawing:false,metrics:new Map(),lastMetricsAt:0,metricsSending:false};

async function ensureRenderer() {
  if (model.renderer) return model.renderer;
  model.renderer=await new WallRenderer({onGpuTiming:(id,milliseconds)=>recordGPU(id,milliseconds)}).init();
  return model.renderer;
}
function fitWall(settings){const inner=$('#wallInner'),ratio=settings.width/settings.height;inner.style.width=`${Math.floor(Math.min(window.innerWidth,window.innerHeight*ratio))}px`;}
async function syncViewport(){const width=Math.max(1,Math.round(window.innerWidth)),height=Math.max(1,Math.round(window.innerHeight));if(model.state?.settings?.width===width&&model.state?.settings?.height===height)return;const next=await api('/api/admin/viewport',{method:'PUT',body:JSON.stringify({width,height})});await acceptState(next);}
let viewportTimer=0;
function queueViewportSync(){if(model.state)fitWall(model.state.settings);clearTimeout(viewportTimer);viewportTimer=setTimeout(()=>syncViewport().catch(error=>{$('#viewStatus').textContent=`同步视口失败：${errorText(error)}`;}),150);}
function renderWall() {
  const {settings,players=[]}=model.state,inner=$('#wallInner');
  inner.style.aspectRatio=`${settings.width} / ${settings.height}`;fitWall(settings);
  const active=new Set(players.map(player=>player.sub));
  for(const [sub,canvas] of model.canvases) if(!active.has(sub)){canvas.remove();model.canvases.delete(sub);model.sources.delete(sub);}
  for(const player of players){
    let canvas=model.canvases.get(player.sub);
    if(!canvas){canvas=el('canvas',{className:'wall-tile',title:player.name});model.canvases.set(player.sub,canvas);inner.append(canvas);}
    const tile=player.tile;
    canvas.style.left=`${tile.x/settings.width*100}%`;canvas.style.top=`${tile.y/settings.height*100}%`;
    canvas.style.width=`${tile.size/settings.width*100}%`;canvas.style.height=`${tile.size/settings.height*100}%`;
    canvas.dataset.size=String(tile.size);canvas.dataset.version=String(player.code.version);model.sources.set(player.sub,player.code.source);
  }
}
function metricFor(id,version){let metric=model.metrics.get(id);if(!metric||metric.version!==version){metric={version,submit:0,submitSamples:0,gpu:null};model.metrics.set(id,metric);}return metric;}
function recordSubmit(id,version,milliseconds){const metric=metricFor(id,version);metric.submitSamples++;metric.submit=metric.submitSamples===1?milliseconds:metric.submit*.8+milliseconds*.2;}
function recordGPU(id,milliseconds){const player=(model.state?.players||[]).find(value=>value.sub===id);if(!player)return;const metric=metricFor(id,player.code.version);metric.gpu=metric.gpu===null?milliseconds:metric.gpu*.8+milliseconds*.2;}
async function reportMetrics(now){if(model.metricsSending||now-model.lastMetricsAt<1000)return;const reports=[];for(const player of model.state?.players||[]){const metric=model.metrics.get(player.sub);if(!metric||metric.version!==player.code.version||!metric.submitSamples)continue;reports.push({sub:player.sub,version:player.code.version,submitMs:metric.submit,gpuMs:metric.gpu});}if(!reports.length)return;model.metricsSending=true;model.lastMetricsAt=now;try{await api('/api/admin/metrics',{method:'PUT',body:JSON.stringify({metrics:reports})});}catch(error){$('#viewStatus').textContent=`性能指标上报失败：${errorText(error)}`;}finally{model.metricsSending=false;}}

async function acceptState(state){model.state=state;renderWall();await ensureRenderer();}
async function refresh(){await acceptState(await api('/api/state'));}

function connect(){model.socketStop?.();model.socketStop=startSocket(async message=>{if(message.state)await acceptState(message.state);else await refresh();},(status,tone)=>{const node=$('#viewStatus');node.textContent=status;node.dataset.tone=tone;node.style.color=tone==='success'?'#a3be8c':tone==='error'?'#bf616a':'#ebcb8b';});}
async function draw(){requestAnimationFrame(draw);if(model.drawing||!model.renderer||!model.state)return;model.drawing=true;try{const now=performance.now(),frame=Math.floor(now/10);for(const player of model.state.players||[]){const canvas=model.canvases.get(player.sub),source=model.sources.get(player.sub);if(!canvas||!source)continue;if(canvas.dataset.renderVersion!==canvas.dataset.version){const result=await model.renderer.setShader(player.sub,source);if(!result.ok)throw new Error(`${player.name}: ${result.errors.join('\n')}`);model.renderer.setTarget(player.sub,canvas,Number(canvas.dataset.size));canvas.dataset.renderVersion=canvas.dataset.version;metricFor(player.sub,player.code.version);}const started=performance.now();model.renderer.draw(player.sub,frame);recordSubmit(player.sub,player.code.version,performance.now()-started);}void reportMetrics(now);}catch(error){$('#viewStatus').textContent=`渲染错误：${errorText(error)}`;}finally{model.drawing=false;}}
async function boot(){try{const me=await api('/api/auth/me');if(me.role!=='admin')throw new Error('请先在管理控制台登录');await refresh();await syncViewport();connect();$('#viewStatus').textContent='大屏已就绪';}catch(error){$('#enterFullscreen').disabled=true;$('#viewStatus').textContent=`未获得管理员权限：${errorText(error)}`;}requestAnimationFrame(draw);}
$('#enterFullscreen').onclick=async()=>{try{if(document.fullscreenElement){await document.exitFullscreen();}else{await document.documentElement.requestFullscreen();}}catch(error){$('#viewStatus').textContent=`无法切换全屏：${errorText(error)}`;}};
document.addEventListener('fullscreenchange',()=>{const button=$('#enterFullscreen');button.textContent=document.fullscreenElement?'退出全屏':'进入全屏';queueViewportSync();});
window.addEventListener('resize',queueViewportSync);
window.addEventListener('beforeunload',()=>model.socketStop?.());
boot();
