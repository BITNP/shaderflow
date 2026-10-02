import { $, api, errorText } from '../ui.js';
import { createVisualReplay } from './visual-replay.js';
import { createReplayTimeline } from './replay-timeline.js';

export function wireReplay() {
  const list=$('#archiveList'),details=$('#replayDetails'),through=$('#replayThrough'),load=$('#loadReplay'),canvas=$('#replayCanvas'),previous=$('#replayPrevious'),next=$('#replayNext'),play=$('#replayPlay'),speed=$('#replaySpeed'),position=$('#replayPosition');
  const headers=()=>({}),request=(path)=>api(path);
  let timeline=createReplayTimeline([]),playing=false,timer=null,roundId='';
  const visual=createVisualReplay({fetchBlob:async ref=>{const query=new URLSearchParams({size:String(ref.size),contentType:ref.contentType});const response=await fetch(`/api/admin/archives/${encodeURIComponent(ref.roundId)}/blobs/${ref.sha256}?${query}`,{headers:headers()});if(!response.ok)throw new Error(await response.text());return response.blob();}});
  function stop(){playing=false;clearTimeout(timer);timer=null;play.textContent='播放';}
  function showEvent(item){if(!item){position.textContent='此 Round 没有事件';return;}through.value=String(item.seq);position.textContent=`#${item.seq} · ${item.event?.type||item.type||'unknown'} · ${item.event?.at||'无时间戳'}`;}
  async function render(seq){if(!roundId)return;details.textContent='正在重建只读状态和 WebGPU 画面…';const suffix=seq==null?'':`?through=${encodeURIComponent(seq)}`;const state=await request(`/api/admin/archives/${encodeURIComponent(roundId)}/state${suffix}`);await visual.render(state,canvas);details.textContent=JSON.stringify(state,null,2);}
  async function loadRound(){stop();roundId=list.value;if(!roundId)return;const archive=await request(`/api/admin/archives/${encodeURIComponent(roundId)}`);timeline=createReplayTimeline(archive.events);const requested=through.value?Number(through.value):Number.MAX_SAFE_INTEGER;const item=timeline.seek(requested);showEvent(item);await render(item?.seq);}
  async function step(direction){stop();const item=direction<0?timeline.previous():timeline.next();showEvent(item);if(item)await render(item.seq);}
  async function tick(){if(!playing)return;const delay=timeline.delayToNext(Number(speed.value));if(delay==null){stop();return;}timer=setTimeout(async()=>{try{const item=timeline.next();showEvent(item);if(item)await render(item.seq);await tick();}catch(e){details.textContent=errorText(e);stop();}},delay);}
  async function refresh(){try{const archives=await request('/api/admin/archives');list.replaceChildren(...archives.map(item=>{const option=document.createElement('option');option.value=item.roundId;option.textContent=`${item.roundId} · ${item.status}`;option.disabled=!item.schemaVersion;return option;}));}catch(e){details.textContent=errorText(e);}}
  load.onclick=()=>loadRound().catch(e=>details.textContent=errorText(e));previous.onclick=()=>step(-1).catch(e=>details.textContent=errorText(e));next.onclick=()=>step(1).catch(e=>details.textContent=errorText(e));play.onclick=()=>{if(playing){stop();return;}if(!timeline.current())return;playing=true;play.textContent='暂停';void tick();};list.onchange=stop;
  void refresh();
  return {refresh,destroy:async()=>{stop();await visual.destroy();}};
}
