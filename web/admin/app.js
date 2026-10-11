import { WallRenderer } from '../shared/wall.js';
import { $, el, api, json, announce, errorText, startSocket } from '../ui.js';

const model = { state:null, renderer:null, socketStop:null, canvases:new Map(), sources:new Map(), attempts:new Map(), candidatePreviews:new Map(), drawing:false };

async function request(path, options={}) { return api(path, options); }
function candidateKey(candidate) { return `${candidate.sub}:${candidate.code.version}`; }

async function refresh() {
  try {
    await acceptState(await request('/api/state'));
    $('#login').classList.add('hidden'); $('#app').classList.remove('hidden');
    connect();
  } catch (error) {
    if (!String(error.message).startsWith('401:')) announce(errorText(error), 'error');
  }
}
function connect() {
  if (model.socketStop) return;
  model.socketStop = startSocket(async message => {
    if (message.state) await acceptState(message.state);
    else await refreshState();
  }, status => { $('#status').textContent = status; });
}
async function refreshState() { await acceptState(await request('/api/state')); }

async function acceptState(state) {
  model.state = state;
  $('#width').value = state.settings.width; $('#height').value = state.settings.height;
  const layout = state.layout || {};
  $('#layoutInfo').textContent = layout.tileSize ? `${layout.columns} × ${layout.rows} · 每格 ${layout.tileSize}px · 在线 ${state.players?.length || 0}` : '暂无已确认在线玩家';
  renderWall(); renderPlayers(); renderCandidates();
  await ensureRenderer();
  await processCandidates();
}
async function ensureRenderer() {
  if (model.renderer) return model.renderer;
  try { model.renderer = await new WallRenderer().init(); $('#status').textContent = 'WebGPU 大屏已就绪'; return model.renderer; }
  catch (error) { $('#status').textContent = `WebGPU 不可用：${errorText(error)}`; $('#status').className='bad'; throw error; }
}
function renderWall() {
  const inner = $('#wallInner');
  if (!inner) return;
  const {settings, players=[]} = model.state;
  inner.style.aspectRatio = `${settings.width} / ${settings.height}`;
  const alive = new Set(players.map(player => player.sub));
  for (const [sub, canvas] of model.canvases) if (!alive.has(sub)) { canvas.remove(); model.canvases.delete(sub); model.sources.delete(sub); }
  for (const player of players) {
    let canvas = model.canvases.get(player.sub);
    if (!canvas) { canvas=el('canvas',{className:'wall-tile',title:player.name});model.canvases.set(player.sub,canvas);inner.append(canvas); }
    const tile=player.tile;
    canvas.style.left=`${tile.x / settings.width * 100}%`;canvas.style.top=`${tile.y / settings.height * 100}%`;
    canvas.style.width=`${tile.size / settings.width * 100}%`;canvas.style.height=`${tile.size / settings.height * 100}%`;
    canvas.dataset.size=String(tile.size); canvas.dataset.version=String(player.code.version);
    model.sources.set(player.sub,player.code.source);
  }
}
function renderPlayers() {
  const root=$('#players');root.replaceChildren();
  const players=model.state.players || [];
  if (!players.length) root.append(el('p',{className:'hint',text:'等待玩家提交并通过验证的代码。'}));
  for (const player of players) root.append(el('div',{className:'player-row'},el('strong',{text:player.name}),el('span',{className:'hint',text:`v${player.code.version} · 格子 ${player.tile.column + 1}, ${player.tile.row + 1} · ${player.tile.size}px`})));
}
function renderCandidates() {
  const root=$('#candidates');root.replaceChildren();
  model.candidatePreviews.clear();
  const candidates=model.state.candidates || [];
  if (!candidates.length) root.append(el('p',{className:'hint',text:'没有待确认代码。'}));
  for (const candidate of candidates) {
    const key=candidateKey(candidate),attempt=model.attempts.get(key),preview=el('canvas',{className:'candidate-preview',width:160,height:160});
    const status=el('span',{className:attempt?.ok?'good':attempt?.error?'bad':'hint',text:attempt?.ok?'验证完成，正在确认':attempt?.error?`验证失败：${attempt.error}`:'等待大屏验证'});
    const retry=el('button',{type:'button',text:'重新验证',onclick:()=>{model.attempts.delete(key);renderCandidates();processCandidates();}});
    root.append(el('article',{className:'candidate'},el('div',{className:'row'},el('strong',{text:candidate.name}),el('span',{className:'hint',text:`v${candidate.code.version}`}),status,retry),preview,el('pre',{className:'candidate-code',text:candidate.code.source})));
    model.candidatePreviews.set(key,preview);
  }
}
async function processCandidates() {
  if (!model.renderer || !model.state) return;
  for (const candidate of model.state.candidates || []) {
    const key=candidateKey(candidate); if (model.attempts.has(key)) continue;
    model.attempts.set(key,{pending:true}); renderCandidates();
    const preview=model.candidatePreviews.get(key);
    try {
      const id=`candidate:${candidate.sub}:${candidate.code.version}`;
      const result=await model.renderer.setShader(id,candidate.code.source);
      if (!result.ok) throw new Error(result.errors.join('\n'));
      model.renderer.setTarget(id,preview,160);model.renderer.draw(id,0);await model.renderer.device.queue.onSubmittedWorkDone();
      model.attempts.set(key,{ok:true});renderCandidates();
      await request(`/api/admin/candidates/${encodeURIComponent(candidate.sub)}/promote`,{method:'POST',body:json({version:candidate.code.version})});
      announce(`${candidate.name} 的 v${candidate.code.version} 已通过权威验证`, 'success');
      await refreshState();
    } catch (error) { model.attempts.set(key,{error:errorText(error)});renderCandidates(); }
  }
}
async function draw() {
  requestAnimationFrame(draw);
  if (model.drawing || !model.renderer || !model.state) return;
  model.drawing=true;
  try {
    const frame=Math.floor(performance.now()/10);
    for (const player of model.state.players || []) {
      const canvas=model.canvases.get(player.sub),source=model.sources.get(player.sub);
      if (!canvas || !source) continue;
      const id=player.sub;
      if (canvas.dataset.renderVersion !== canvas.dataset.version) {
        const result=await model.renderer.setShader(id,source);if (!result.ok) throw new Error(`${player.name}: ${result.errors.join('\n')}`);
        model.renderer.setTarget(id,canvas,Number(canvas.dataset.size));canvas.dataset.renderVersion=canvas.dataset.version;
      }
      model.renderer.draw(id,frame);
    }
  } catch (error) { $('#status').textContent=`渲染错误：${errorText(error)}`;$('#status').className='bad'; }
  finally { model.drawing=false; }
}

$('#loginForm').onsubmit=async event => { event.preventDefault();$('#loginError').textContent='';try { await api('/api/admin/login',{method:'POST',body:json({password:$('#password').value})});await refresh(); } catch (error) { $('#loginError').textContent=errorText(error); } };
$('#saveSettings').onclick=async()=>{try { const current=model.state.settings;await request('/api/admin/settings',{method:'PUT',body:json({width:Number($('#width').value),height:Number($('#height').value),maxCodeBytes:current.maxCodeBytes})});await refreshState(); } catch(error) {announce(errorText(error),'error');}};
$('#logout').onclick=async()=>{await request('/api/auth/logout',{method:'POST'}).catch(()=>{});model.socketStop?.();model.socketStop=null;model.renderer?.destroy();model.renderer=null;model.state=null;$('#app').classList.add('hidden');$('#login').classList.remove('hidden');};
requestAnimationFrame(draw);refresh();
