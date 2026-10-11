import { WallRenderer } from '../../shared/wall.js';
import { $, el, api, errorText, startSocket } from '../../ui.js';

const model={state:null,renderer:null,canvases:new Map(),sources:new Map(),socketStop:null,drawing:false};

async function ensureRenderer() {
  if (model.renderer) return model.renderer;
  model.renderer=await new WallRenderer().init();
  return model.renderer;
}
function fitWall(settings){const inner=$('#wallInner'),ratio=settings.width/settings.height;inner.style.width=`${Math.floor(Math.min(window.innerWidth,window.innerHeight*ratio))}px`;}
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
async function acceptState(state){model.state=state;renderWall();await ensureRenderer();}
async function refresh(){await acceptState(await api('/api/state'));}
function connect(){model.socketStop?.();model.socketStop=startSocket(async message=>{if(message.state)await acceptState(message.state);else await refresh();},status=>{$('#viewStatus').textContent=status;});}
async function draw(){requestAnimationFrame(draw);if(model.drawing||!model.renderer||!model.state)return;model.drawing=true;try{const frame=Math.floor(performance.now()/10);for(const player of model.state.players||[]){const canvas=model.canvases.get(player.sub),source=model.sources.get(player.sub);if(!canvas||!source)continue;if(canvas.dataset.renderVersion!==canvas.dataset.version){const result=await model.renderer.setShader(player.sub,source);if(!result.ok)throw new Error(`${player.name}: ${result.errors.join('\n')}`);model.renderer.setTarget(player.sub,canvas,Number(canvas.dataset.size));canvas.dataset.renderVersion=canvas.dataset.version;}model.renderer.draw(player.sub,frame);}}catch(error){$('#viewStatus').textContent=`渲染错误：${errorText(error)}`;}finally{model.drawing=false;}}
function gate(show,message){$('#fullscreenGate').classList.toggle('hidden',!show);if(message)$('#gateMessage').textContent=message;}
async function boot(){try{const me=await api('/api/auth/me');if(me.role!=='admin')throw new Error('请先在管理控制台登录');await refresh();connect();gate(true,'进入大屏必须使用浏览器全屏模式。');}catch(error){gate(true,`无法打开大屏：${errorText(error)}`);$('#enterFullscreen').disabled=true;$('#viewStatus').textContent='未获得管理员权限';}requestAnimationFrame(draw);}
$('#enterFullscreen').onclick=async()=>{try{await document.documentElement.requestFullscreen();gate(false);$('#viewStatus').textContent='全屏大屏已开启';}catch(error){gate(true,`浏览器拒绝全屏：${errorText(error)}。请允许全屏后重试。`);}};
document.addEventListener('fullscreenchange',()=>{if(!document.fullscreenElement)gate(true,'已退出全屏。进入大屏前必须重新开启全屏模式。');});
window.addEventListener('resize',()=>{if(model.state)fitWall(model.state.settings);});
window.addEventListener('beforeunload',()=>model.socketStop?.());
boot();
