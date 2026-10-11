import { WallRenderer } from '../shared/wall.js';
import { $, el, api, json, announce, errorText, startSocket } from '../ui.js';

const model = { state:null, renderer:null, socketStop:null, attempts:new Map() };

async function request(path, options={}) { return api(path, options); }

async function refresh() {
  try {
    const me = await request('/api/auth/me');
    if (me.role !== 'admin') {
      $('#app').classList.add('hidden'); $('#login').classList.remove('hidden');
      return;
    }
    await acceptState(await request('/api/state'));
    $('#login').classList.add('hidden'); $('#app').classList.remove('hidden');
    connect();
  } catch (error) {
    $('#app').classList.add('hidden'); $('#login').classList.remove('hidden');
    if (!String(error.message).startsWith('401:')) announce(errorText(error), 'error');
  }
}
function connect() {
  if (model.socketStop) return;
  model.socketStop = startSocket(async message => {
    if (message.state) await acceptState(message.state);
    else await refreshState();
  }, (status,tone) => { const node=$('#status');node.className='hint';node.textContent=status;node.dataset.tone=tone;node.style.color=tone==='success'?'#a3be8c':tone==='error'?'#bf616a':'#ebcb8b'; });
}
async function refreshState() { await acceptState(await request('/api/state')); }

async function acceptState(state) {
  model.state = state;
  renderPlayers();
  await ensureRenderer();
  await processCandidates();
}
async function ensureRenderer() {
  if (model.renderer) return model.renderer;
  try { model.renderer = await new WallRenderer().init(); $('#status').textContent = 'WebGPU 大屏已就绪'; return model.renderer; }
  catch (error) { const node=$('#status');node.textContent = `WebGPU 不可用：${errorText(error)}`; node.className='bad';delete node.dataset.tone;node.style.color=''; throw error; }
}
function metricLabel(metrics) {
  if (!metrics) return '性能采样中';
  const gpu = Number.isFinite(metrics.gpuMs) ? `${metrics.gpuMs.toFixed(2)} ms GPU` : 'GPU 计时不支持';
  return `${gpu} · ${Number(metrics.submitMs || 0).toFixed(2)} ms 提交`;
}
async function kickPlayer(player) {
  if (!window.confirm(`停止展示 ${player.name}？只有提交更高版本代码后才会自动重新加入。`)) return;
  try { await acceptState(await request(`/api/admin/players/${encodeURIComponent(player.sub)}/kick`, {method:'POST'})); }
  catch (error) { announce(errorText(error), 'error'); }
}
function renderPlayers() {
  const root=$('#players');root.replaceChildren();
  const players=model.state.players || [],layout=model.state.layout || {};
  root.classList.toggle('player-layout',Boolean(layout.rows && layout.columns));
  root.style.gridTemplateColumns=layout.columns ? `repeat(${layout.columns}, minmax(0, 1fr))` : '';
  root.style.gridTemplateRows=layout.rows ? `repeat(${layout.rows}, minmax(0, auto))` : '';
  if (!players.length) root.append(el('p',{className:'hint',text:'等待玩家提交并通过验证的代码。'}));
  for (const player of players) {
    const item=el('div',{className:'player-row'},el('div',{},el('strong',{text:player.name}),el('div',{className:'hint',text:`第 ${player.tile.row + 1} 行 · 第 ${player.tile.column + 1} 列 · ${player.tile.size}px`}),el('small',{className:'hint',text:metricLabel(player.metrics)})),el('button',{type:'button',text:'踢掉',onclick:()=>kickPlayer(player)}));
    item.style.gridColumn=String(player.tile.column + 1);item.style.gridRow=String(player.tile.row + 1);root.append(item);
  }
}
async function processCandidates() {
  if (!model.renderer || !model.state) return;
  for (const candidate of model.state.candidates || []) {
    const key=`${candidate.sub}:${candidate.code.version}`; if (model.attempts.has(key)) continue;
    model.attempts.set(key,{pending:true});
    const preview=document.createElement('canvas');
    try {
      const id=`candidate:${candidate.sub}:${candidate.code.version}`;
      const result=await model.renderer.setShader(id,candidate.code.source);
      if (!result.ok) throw new Error(result.errors.join('\n'));
      model.renderer.setTarget(id,preview,160);model.renderer.draw(id,0);await model.renderer.device.queue.onSubmittedWorkDone();
      model.attempts.set(key,{ok:true});
      await request(`/api/admin/candidates/${encodeURIComponent(candidate.sub)}/promote`,{method:'POST',body:json({version:candidate.code.version})});
      announce(`${candidate.name} 的 v${candidate.code.version} 已通过权威验证`, 'success');
      await refreshState();
    } catch (error) { model.attempts.set(key,{error:errorText(error)});announce(`${candidate.name} 的候选代码验证失败：${errorText(error)}`,'error'); }
  }
}

$('#loginForm').onsubmit=async event => { event.preventDefault();$('#loginError').textContent='';try { await api('/api/admin/login',{method:'POST',body:json({password:$('#password').value})});await refresh(); } catch (error) { $('#loginError').textContent=errorText(error); } };
$('#logout').onclick=async()=>{await request('/api/auth/logout',{method:'POST'}).catch(()=>{});model.socketStop?.();model.socketStop=null;model.renderer?.destroy();model.renderer=null;model.state=null;$('#app').classList.add('hidden');$('#login').classList.remove('hidden');};
refresh();
