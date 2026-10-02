import {
  $, api, json, announce, errorText, startSocket, makeTextureControls,
  saveDialog, download,
} from '../ui.js';
import {
  Renderer, CameraClock, localGraph, generateTemplate, createMetadata,
} from '../shared/gpu.js';
import { createPlayerCodeSession } from './code-session.js';
import { createRenderParameterTimeline } from '../shared/render-timeline.js';
import { createAuthSession } from '../auth-session.js';

const PLAYER_STORE = 'shaderflow.player';
const PHASE_LABELS = {
  idle: '尚未开放', registration: '报名中', graph: '正在准备渲染图', running: '游戏进行中',
};


let state = null;
let renderer = null;
let nodeKey = '';
let activeCode = '';
let localFallback = false;
let version = 0;
let compileTimer = 0;
let compiling = false;
let rerunCompile = false;
let renderStopped = false;
let paused = false;
let parameterTimeline = createRenderParameterTimeline({metadata:null});
const pendingInputs = new Map();
let cameraModule = null;
let cameraController = null;
let cameraControllerKey = '';
let useServerTrajectory = false;
let socketStop = null;
let lastFrameError = '';
const clock = new CameraClock();
const textures = new Map();
const authSession=createAuthSession({request:api,onLoggedOut:()=>{socketStop?.();socketStop=null;teardownRenderer();state=null;nodeKey='';activeCode='';editor.value='';$('#logoutButton').classList.add('hidden');showLoggedOut();}});

const editor = $('#codeEditor');
const errors = $('#errors');
const codeStatus = $('#codeStatus');
const versionStatus = $('#versionStatus');
const fatalError = $('#fatalError');

boot();

async function boot() {
  bindControls();
  try {
    cameraModule = await import('../camera/index.js');
  } catch (err) {
    announce(`相机脚本模块不可用，将保留手动相机：${errorText(err)}`, 'error');
  }
  try {
    const me = await authApi('/api/auth/me');
    $('#identity').textContent = `${me.name} · ${me.role}`;
    if (me.role !== 'player') {
      throw new Error('当前浏览器登录的是管理员；请选择测试玩家登录或使用玩家 OIDC 登录');
    }
    $('#loginButton').classList.add('hidden');
    $('#logoutButton').classList.remove('hidden');
    await refreshState();
    connectSocket();
  } catch (err) {
    showLoggedOut(err);
  }
  requestAnimationFrame(renderLoop);
}

async function authApi(path, options = {}) { return api(path, options); }

async function detectTestLogin() {
  try {
    const capabilities = await api('/api/auth/capabilities');
    $('#testLoginButton').classList.toggle('hidden', !capabilities.testPlayerLogin);
    $('#loginButton').classList.toggle('hidden', !capabilities.oidc);
    if (!capabilities.oidc && !capabilities.testPlayerLogin) fatalError.textContent = '服务端未配置玩家登录方式';
  } catch {
    $('#testLoginButton').classList.add('hidden');
  }
}

function showLoggedOut(err) {
  void detectTestLogin();
  $('#identity').textContent = '未登录';
  $('#phase').textContent = '需要登录';
  $('#roundDetails').textContent = '请使用活动身份系统登录后参加。';
  $('#loginButton').classList.remove('hidden');
  $('#logoutButton').classList.add('hidden');
  $('#registerButton').disabled = true;
  if (!String(err?.message || '').startsWith('401:') && err) {
    fatalError.textContent = errorText(err);
  }
}

function connectSocket() {
  socketStop?.();
  socketStop = startSocket(async message => {
    if (message.state) await applyState(message.state);
    else await refreshState();
  }, status => { $('#socketStatus').textContent = status; });
}

async function refreshState() {
  await applyState(await authApi('/api/state'));
}

async function applyState(next) {
  if (!next || typeof next !== 'object') return;
  const oldKey = nodeKey;
  state = next;
  $('#phase').textContent = PHASE_LABELS[state.phase] || state.phase || '未知';
  const settings = state.settings || {};
  const registered = Array.isArray(state.registrations) && state.registrations.length > 0;
  $('#registerButton').disabled = registered || !['registration', 'graph'].includes(state.phase);
  $('#registerButton').textContent = registered ? '已报名' : '报名';
  $('#roundDetails').textContent = describeRound(settings, registered);
  const node = state.ownNode;
  nodeKey = node ? `${node.id}:${node.inputs?.length || 0}:${node.outputs?.length || 0}:${settings.width}x${settings.height}:${settings.format}:${settings.blockSize}` : '';
  $('#assignment').textContent = node
    ? `节点 ${node.id} · ${node.inputs.length} 输入 / ${node.outputs.length} 输出`
    : registered ? '已报名，等待管理员开始并分配节点' : '尚未报名或分配节点';
  $('#workspace').classList.toggle('hidden', !node);
  updateTrajectoryButton();
  if (node && oldKey !== nodeKey) await setupNode(node, settings);
  if (!node && renderer) teardownRenderer();
}

function describeRound(settings, registered) {
  const slots = settings.players ? `人数 ${settings.players}` : '尚未配置人数';
  const size = settings.width && settings.height ? `${settings.width} × ${settings.height}` : '未知尺寸';
  return `${slots} · ${size} · ${settings.format || '未知格式'} · workgroup ${settings.blockSize || '?'}${registered ? ' · 您已报名' : ''}`;
}

async function setupNode(node, settings) {
  teardownRenderer();
  fatalError.textContent = '';
  textures.clear();
  const p = node.inputs.length;
  const q = node.outputs.length;
  const saved = loadNodeData(nodeKey);
  const session = createPlayerCodeSession({
    serverAccepted: state.ownCode,
    savedDraft: saved,
    template: generateTemplate(p, q, settings.format, settings.blockSize),
  });
  version = session.nextVersionFloor;
  activeCode = session.activeCode;
  localFallback = false;
  editor.value = session.draft;
  restoreCamera(saved.camera);
  versionStatus.textContent = `本地版本 ${version}`;
  codeStatus.textContent = '正在初始化 WebGPU…';
  try {
    renderer = await new Renderer().init();
    await renderer.configure(localGraph(p, q), {width: settings.width, height: settings.height}, settings.format, settings.blockSize);
    buildTextureUI(p, q, settings);
    await renderer.setPresentTargets(new Map([...textures].map(([id,canvas])=>[id,[canvas]])));
    if (activeCode) {
      const installed = await renderer.setShader('local', activeCode);
      if (!installed.ok) throw new Error(`服务端成功版本无法在当前设备编译：${installed.errors.join('\n')}`);
      codeStatus.textContent = `已恢复服务端成功版本 v${state.ownCode.version}`;
      codeStatus.className = 'good';
    } else {
      codeStatus.textContent = '模板仅作为草稿；编辑后将自动检查';
      codeStatus.className = 'hint';
    }
  } catch (err) {
    renderer = null;
    fatalError.textContent = `无法启动本地渲染：${errorText(err)}。代码仍可编辑和复制。`;
    codeStatus.textContent = 'WebGPU 不可用';
    codeStatus.className = 'bad';
  }
}

function teardownRenderer() {
  renderer?.destroy();
  renderer = null;
  textures.clear();
  $('#textureGrid').replaceChildren();
  clearTimeout(compileTimer);
}

function buildTextureUI(p, q, settings) {
  const grid = $('#textureGrid');
  grid.replaceChildren();
  for (let i = 0; i < p; i++) {
    const id = `input${i}`;
    const control = makeTextureControls({
      id, width: settings.width, height: settings.height,
      onImage: async (image, mode) => {
        const gpuMode = ({'contain-black':'contain', 'contain-transparent':'transparent'})[mode] || mode;
        if (paused) {
          pendingInputs.set(id, {image, mode:gpuMode});
          announce('已记录输入修改，恢复后应用', 'info');
          return;
        }
        await renderer.setInput(id, image, gpuMode);
      },
      onSave: () => saveDialog(renderer, id, settings.width, settings.height),
    });
    textures.set(id, control.canvas);
    grid.append(control.element);
  }
  for (let i = 0; i < q; i++) {
    const id = `output${i}`;
    const control = makeTextureControls({
      id, width: settings.width, height: settings.height,
      onImage: async () => { throw new Error('输出纹理不可上传图像'); },
      onSave: () => saveDialog(renderer, id, settings.width, settings.height),
    });
    control.element.querySelectorAll('select, input, button').forEach(element => {
      if (element.textContent === '保存') return;
      if (element.tagName !== 'CANVAS') element.classList.add('hidden');
    });
    textures.set(id, control.canvas);
    grid.append(control.element);
  }
}

function bindControls() {
  $('#loginButton').addEventListener('click', () => { location.href = '/auth/login'; });
  $('#logoutButton').addEventListener('click', async () => { await authSession.logout(); });
  $('#testLoginButton').addEventListener('click', async () => {
    const name = window.prompt('请输入测试玩家名称', '测试玩家');
    if (!name) return;
    try {
      await api('/api/test/player-login', {method: 'POST', body: json({name})});
      const me = await authApi('/api/auth/me');
      $('#identity').textContent = `${me.name} · ${me.role}`;
      $('#loginButton').classList.add('hidden');
      $('#testLoginButton').classList.add('hidden');
      $('#logoutButton').classList.remove('hidden');
      await refreshState();
      connectSocket();
    } catch (err) { announce(`测试登录失败：${errorText(err)}`, 'error'); }
  });
  $('#registerButton').addEventListener('click', async () => {
    try { await applyState(await authApi('/api/register', {method: 'POST', body: json({})})); announce('报名成功', 'success'); }
    catch (err) { announce(`报名失败：${errorText(err)}`, 'error'); }
  });
  editor.addEventListener('input', () => {
    persistNodeData();
    codeStatus.textContent = '等待自动检查…';
    codeStatus.className = 'hint';
    scheduleCompile();
  });
  $('#validateButton').addEventListener('click', () => scheduleCompile(0));
  $('#templateButton').addEventListener('click', () => {
    if (!state?.ownNode || !confirm('确定用生成模板替换当前编辑内容？')) return;
    const n = state.ownNode, s = state.settings;
    editor.value = generateTemplate(n.inputs.length, n.outputs.length, s.format, s.blockSize);
    persistNodeData();
    scheduleCompile(0);
  });
  $('#copyCodeButton').addEventListener('click', () => copyText(editor.value, '代码已复制'));
  $('#pauseButton').addEventListener('click', togglePause);
  $('#resetTimeButton').addEventListener('click', () => { clock.reset(); cameraController?.reset?.(); announce('本地 time 已重置', 'success'); });
  $('#cameraMode').addEventListener('change', cameraChanged);
  for (const id of ['fov','posX','posY','posZ','yaw','pitch','roll','cameraInit','cameraFrame']) $(id.startsWith('#') ? id : `#${id}`).addEventListener('input', cameraChanged);
  $('#serverTrajectoryButton').addEventListener('click', () => {
    useServerTrajectory = !useServerTrajectory;
    updateTrajectoryButton();
    rebuildCameraController();
  });
  $('#aiPromptButton').addEventListener('click', makePrompt);
  $('#copyPromptButton').addEventListener('click', () => copyText($('#aiPrompt').value, '提示词已复制'));
  $('#downloadPromptButton').addEventListener('click', () => download(new Blob([$('#aiPrompt').value], {type:'text/markdown;charset=utf-8'}), 'shaderflow-prompt.md'));
  window.addEventListener('beforeunload', () => { renderStopped = true; socketStop?.(); persistNodeData(); });
}

function scheduleCompile(delay = state?.settings?.compileDebounceMs ?? 500) {
  clearTimeout(compileTimer);
  compileTimer = setTimeout(validateAndUpload, Math.max(0, Number(delay) || 0));
}

async function validateAndUpload() {
  if (!renderer || !state?.ownNode) return;
  if (compiling) { rerunCompile = true; return; }
  compiling = true;
  $('#validateButton').disabled = true;
  errors.textContent = '';
  codeStatus.textContent = '正在编译并验证一帧…';
  codeStatus.className = 'warn';
  const source = editor.value;
  const node = state.ownNode;
  const settings = state.settings;
  try {
    const bytes = new TextEncoder().encode(source).length;
    if (!source.trim()) throw new Error('代码不能为空');
    if (bytes > settings.maxCodeBytes) throw new Error(`代码 ${bytes} 字节，超过限制 ${settings.maxCodeBytes}`);
    const result = await renderer.validate(source, node.inputs.length, node.outputs.length, settings.format, settings.blockSize);
    if (!result.ok) throw new Error(result.errors.join('\n'));
    const installed = await renderer.setShader('local', source);
    if (!installed.ok) throw new Error(installed.errors.join('\n'));
    await renderer.frame(currentMetadata());
    await renderer.device.queue.onSubmittedWorkDone();
    const nextVersion = Math.max(version + 1, Date.now());
    await authApi('/api/code', {method:'POST', body:json({version:nextVersion, code:source})});
    activeCode = source;
    localFallback = false;
    version = nextVersion;
    persistNodeData();
    versionStatus.textContent = `已提交版本 ${version}`;
    codeStatus.textContent = '编译、运行与上传成功';
    codeStatus.className = 'good';
    announce('代码验证成功并已上传', 'success');
  } catch (err) {
    errors.textContent = errorText(err);
    codeStatus.textContent = '检查失败：本地输出已切换为黑色；服务器仍使用上次成功版本';
    codeStatus.className = 'bad';
    renderer.useFallback('local');
    localFallback = true;
    await renderer.frame(currentMetadata()).catch(()=>{});
  } finally {
    compiling = false;
    $('#validateButton').disabled = false;
    if (rerunCompile) { rerunCompile = false; scheduleCompile(0); }
  }
}

async function renderLoop() {
  if (renderStopped) return;
  if (renderer && activeCode && !compiling && !localFallback) {
    try {
      await renderer.frame(currentMetadata());
      lastFrameError = '';
    } catch (err) {
      const message = errorText(err);
      if (message !== lastFrameError) announce(`渲染失败：${message}`, 'error');
      lastFrameError = message;
      renderer?.useFallback?.('local');
      localFallback = true;
      if (renderer?.lost) await recoverDevice();
    }
  }
  requestAnimationFrame(renderLoop);
}

async function recoverDevice() {
  const source = activeCode || editor.value;
  const node = state?.ownNode;
  const settings = state?.settings;
  if (!node || !settings) return;
  try {
    renderer?.destroy();
    renderer = await new Renderer().init();
    await renderer.configure(localGraph(node.inputs.length, node.outputs.length), {width:settings.width,height:settings.height}, settings.format, settings.blockSize);
    await renderer.setPresentTargets(new Map([...textures].map(([id,canvas])=>[id,[canvas]])));
    if (source && !localFallback) {
      const result = await renderer.setShader('local', source);
      if (!result.ok) throw new Error(result.errors.join('\n'));
    } else {
      renderer.useFallback('local');
    }
    announce('WebGPU 设备已恢复；请重新设置本地输入图像', 'success');
  } catch (err) {
    fatalError.textContent = `WebGPU 设备恢复失败：${errorText(err)}。您仍可复制代码。`;
    renderer = null;
  }
}

function currentMetadata(usePendingControls=false) {
  const frozen = parameterTimeline.snapshot().metadata;
  if (paused && frozen && !usePendingControls) return frozen;
  const width = state?.settings?.width || 960;
  const height = state?.settings?.height || 540;
  const time = clock.time;
  let camera = manualCamera(time, width, height);
  if ($('#cameraMode').value === 'script' || useServerTrajectory) {
    try {
      const controller = getCameraController();
      if (controller) camera = controller.evaluate(time, {width, height});
      $('#cameraStatus').textContent = useServerTrajectory ? '正在临时使用管理员轨迹' : '本地相机脚本运行中';
    } catch (err) {
      $('#cameraStatus').textContent = `脚本错误，使用默认相机：${errorText(err)}`;
      camera = {time, width, height, pos:[0,0,0], euler:[0,0,0], fov:60};
    }
  } else $('#cameraStatus').textContent = '手动相机';
  const packed = cameraModule?.computeCameraMetadata?.(camera) || cameraMetadata(camera);
  const metadata = createMetadata({time, pos:packed.pos, canvasSize:[width,height], rot:packed.rot, invRot:packed.invRot, depth:packed.depth});
  parameterTimeline.update({metadata});
  return parameterTimeline.snapshot().metadata;
}

function manualCamera(time, width, height) {
  return {
    time, width, height,
    pos: [number('#posX'), number('#posY'), number('#posZ')],
    euler: [number('#yaw'), number('#pitch'), number('#roll')],
    fov: Math.min(179, Math.max(1, number('#fov') || 60)),
  };
}

function fallbackCamera({time, width, height, pos, fov}) {
  const angle = Math.min(179, Math.max(1, Number(fov) || 60));
  return {time, pos, rot:[1,0,0,0,1,0,0,0,1], invRot:[1,0,0,0,1,0,0,0,1], depth:Math.min(width,height)/(2*Math.tan(angle*Math.PI/360))};
}

function getTrajectory() {
  return state?.trajectory || state?.adminTrajectory || state?.cameraTrajectory || state?.settings?.trajectory || null;
}

function getCameraController() {
  if (!cameraModule?.createCameraController) return null;
  const trajectory = useServerTrajectory ? getTrajectory() : null;
  const initSource = trajectory?.initSource ?? trajectory?.init ?? $('#cameraInit').value;
  const frameSource = trajectory?.frameSource ?? trajectory?.frame ?? trajectory?.code ?? $('#cameraFrame').value;
  const key = `${useServerTrajectory}:${initSource}\u0000${frameSource}`;
  if (!cameraController || cameraControllerKey !== key) {
    cameraController = cameraModule.createCameraController({initSource, frameSource, mode:'script'});
    cameraControllerKey = key;
  }
  return cameraController;
}

function rebuildCameraController() { cameraController = null; cameraControllerKey = ''; getCameraController(); }
function cameraChanged() { rebuildCameraController(); if (paused) currentMetadata(true); persistNodeData(); }

function updateTrajectoryButton() {
  const trajectory = getTrajectory();
  const button = $('#serverTrajectoryButton');
  button.disabled = !trajectory;
  if (!trajectory) useServerTrajectory = false;
  button.textContent = useServerTrajectory ? '停止使用管理员轨迹' : '临时使用管理员轨迹';
}

async function togglePause() {
  paused = !paused;
  if (paused) {
    const metadata = currentMetadata();
    parameterTimeline.update({metadata});
    parameterTimeline.pause();
    clock.pause();
  } else {
    parameterTimeline.resume();
    clock.resume();
    for (const [id, item] of pendingInputs) await renderer?.setInput(id, item.image, item.mode);
    pendingInputs.clear();
  }
  $('#pauseButton').textContent = paused ? '恢复' : '暂停';
  announce(paused ? '参数、相机和本地输入已冻结；编辑会在恢复时应用' : '已恢复，使用最新 pending 状态', 'success');
}

function makePrompt() {
  if (!state?.ownNode) return;
  const node = state.ownNode;
  const settings = state.settings;
  const prompt = `# ShaderFlow WGSL 编程任务\n\n请协助我编写一个 WebGPU compute shader。相机参数完全可以不使用；这个节点也可能只是全屏图像处理 pass。\n\n## 节点约束\n\n- 输入纹理数：${node.inputs.length}\n- 输出纹理数：${node.outputs.length}\n- 纹理尺寸：${settings.width} × ${settings.height}\n- storage texture 格式：\`${settings.format}\`（读写值均为 \`vec4f\`）\n- workgroup size：${settings.blockSize} × ${settings.blockSize}\n- dispatch 覆盖整张纹理\n- 预览会将 HDR/负值 clamp 到 [0,1]，NaN 显示为 0\n\n## metadata 布局\n\n\`\`\`wgsl\nstruct _metadata {\n  time: f32,          // 自本地重置起的毫秒数\n  pos: vec3f,         // 相机位置\n  canvasSize: vec2i,  // 纹理像素尺寸\n  rot: mat3x3f,       // 内禀 yaw-pitch-roll 旋转\n  invRot: mat3x3f,\n  depth: f32,         // min(width,height)/(2*tan(fov/2))\n};\n\`\`\`\n\n坐标系约定：相机初始朝向 -Z，+X 向右，+Y 向上，+Z 向后。读取函数会逐轴 clamp 越界坐标；写函数会丢弃越界写入。\n\n## 当前完整代码\n\n\`\`\`wgsl\n${editor.value}\n\`\`\`\n\n请返回可直接替换的完整 WGSL，并保留正确的 binding、metadata、入口函数和所有输出写入。`;
  $('#aiPrompt').value = prompt;
  $('#copyPromptButton').disabled = false;
  $('#downloadPromptButton').disabled = false;
}

function loadNodeData(key) {
  try {
    const all = JSON.parse(localStorage.getItem(PLAYER_STORE) || '{}');
    return all[key] || {};
  } catch { return {}; }
}

function persistNodeData() {
  if (!nodeKey) return;
  try {
    const all = JSON.parse(localStorage.getItem(PLAYER_STORE) || '{}');
    all[nodeKey] = {code:editor.value, version, camera:cameraForm()};
    localStorage.setItem(PLAYER_STORE, JSON.stringify(all));
  } catch (err) { console.warn('Unable to persist player state', err); }
}

function cameraForm() {
  return {
    mode:$('#cameraMode').value, fov:$('#fov').value,
    posX:$('#posX').value, posY:$('#posY').value, posZ:$('#posZ').value,
    yaw:$('#yaw').value, pitch:$('#pitch').value, roll:$('#roll').value,
    init:$('#cameraInit').value, frame:$('#cameraFrame').value,
  };
}

function restoreCamera(camera = {}) {
  $('#cameraMode').value = camera.mode || 'manual';
  $('#fov').value = camera.fov || 60;
  for (const id of ['posX','posY','posZ','yaw','pitch','roll']) $(`#${id}`).value = camera[id] ?? 0;
  $('#cameraInit').value = camera.init || '';
  $('#cameraFrame').value = camera.frame || '';
  rebuildCameraController();
}

function number(selector) { return Number($(selector).value) || 0; }
async function copyText(text, success) {
  try { await navigator.clipboard.writeText(text); announce(success, 'success'); }
  catch (err) { announce(`复制失败：${errorText(err)}`, 'error'); }
}
