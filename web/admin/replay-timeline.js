export function createReplayTimeline(events,{maxDelayMs=5000}={}) {
  const ordered=[...events].filter(item=>Number.isSafeInteger(item?.seq)).sort((a,b)=>a.seq-b.seq);
  let index=ordered.length?0:-1;
  const current=()=>index<0?null:ordered[index];
  return {
    current,
    next(){if(index>=0&&index<ordered.length-1)index++;return current();},
    previous(){if(index>0)index--;return current();},
    seek(seq){if(!ordered.length)return null;let found=0;for(let i=0;i<ordered.length&&ordered[i].seq<=seq;i++)found=i;index=found;return current();},
    delayToNext(speed=1){if(index<0||index>=ordered.length-1)return null;const from=Date.parse(ordered[index].event?.at||''),to=Date.parse(ordered[index+1].event?.at||'');if(!Number.isFinite(from)||!Number.isFinite(to))return 0;return Math.min(maxDelayMs,Math.max(0,(to-from)/Math.max(.01,Number(speed)||1)));},
    events:ordered,
  };
}
