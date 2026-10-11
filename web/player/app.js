import { DEFAULT_EDITABLE, WallRenderer } from '../shared/wall.js';
import { playUploadGenie } from './upload-genie.js';
import { $, api, json, announce, errorText, startSocket } from '../ui.js';

let state=null, renderer=null, socketStop=null, version=0, source=DEFAULT_EDITABLE, rendering=false;
const preview=$('#preview');

async function refresh() { await applyState(await api('/api/state')); }
async function applyState(next) {
  state=next;
  const accepted=state.ownCode, candidate=state.ownCandidate;
  version=Math.max(version,accepted?.version||0,candidate?.version||0);
  if (!$('#editor').value) { source=candidate?.source || accepted?.source || DEFAULT_EDITABLE;$('#editor').value=source; }
  $('#version').textContent=`本地版本基数 ${version}`;
  $('#tileInfo').textContent=state.ownTile ? `现场格子：${state.ownTile.size}px（第 ${state.ownTile.column+1} 列，第 ${state.ownTile.row+1} 行）` : '尚未成为现场在线玩家';
  $('#status').textContent=candidate ? `候选 v${candidate.version} 等待管理大屏确认` : accepted ? `现场使用 v${accepted.version}` : '编辑并提交你的第一版 shader';
  await setupRenderer();
}
async function setupRenderer() {
  if (renderer) return;
  try { renderer=await new WallRenderer().init();renderer.setTarget('local',preview,Math.min(640,Math.max(200,state?.ownTile?.size||480)));const result=await renderer.setShader('local',$('#editor').value||DEFAULT_EDITABLE);if(!result.ok)throw new Error(result.errors.join('\n')); }
  catch(error) { $('#errors').textContent=`WebGPU 初始化失败：${errorText(error)}`;renderer=null; }
}
function connect() { if(socketStop)return;socketStop=startSocket(async message=>{if(message.state)await applyState(message.state);else await refresh();},(status,tone)=>{const node=$('#socket');node.textContent=status;node.dataset.tone=tone;node.style.color=tone==='success'?'#a3be8c':tone==='error'?'#bf616a':'#ebcb8b';}); }
async function boot() {
  try { const me=await api('/api/auth/me');if(me.role!=='player')throw new Error('管理员账号不能作为玩家提交代码');$('#identity').textContent=`${me.name}（由 oauth2-proxy 提供身份）`;await refresh();connect(); }
  catch(error) { $('#identity').textContent='未收到可信的 oauth2-proxy 身份 Header';$('#status').textContent='无法进入玩家端';$('#errors').textContent=errorText(error); }
  requestAnimationFrame(draw);
}
async function draw() { requestAnimationFrame(draw);if(rendering||!renderer)return;rendering=true;try{renderer.draw('local',Math.floor(performance.now()/10));}catch{}finally{rendering=false;} }
$('#submit').onclick=async()=>{
  const editable=$('#editor').value;$('#errors').textContent='';$('#submit').disabled=true;
  try { await setupRenderer();if(!renderer)throw new Error('WebGPU 不可用');const checked=await renderer.setShader('local',editable);if(!checked.ok)throw new Error(checked.errors.join('\n'));renderer.draw('local',Math.floor(performance.now()/10));await renderer.device.queue.onSubmittedWorkDone();playUploadGenie(preview,$('#submit'));const next=Math.max(version+1,Date.now());const response=await api('/api/candidate',{method:'POST',body:json({version:next,source:editable})});source=editable;version=next;$('#editor').value=source;await applyState(response);announce('候选代码已提交，等待管理大屏验证','success');}
  catch(error){$('#errors').textContent=errorText(error);announce('本地验证失败','error');}
  finally{$('#submit').disabled=false;}
};
$('#reset').onclick=()=>{$('#editor').value=DEFAULT_EDITABLE;source=DEFAULT_EDITABLE;$('#errors').textContent='';};
$('#editor').addEventListener('input',()=>{source=$('#editor').value;});
window.addEventListener('beforeunload',()=>socketStop?.());
boot();
