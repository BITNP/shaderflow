import test from 'node:test';
import assert from 'node:assert/strict';
import { createVisualReplay } from './visual-replay.js';

test('visual replay restores shaders inputs metadata and one isolated frame', async () => {
  const calls=[];
  class FakeRenderer {
    async init(){calls.push('init');return this}
    async configure(graph,size,format,block){calls.push(['configure',graph,size,format,block])}
    async setShader(id,source){calls.push(['shader',id,source]);return {ok:true,errors:[]}}
    async setInput(id,blob,mode){calls.push(['input',id,await blob.text(),mode])}
    async setPresentTargets(targets){calls.push(['targets',[...targets.keys()]])}
    async frame(metadata){calls.push(['frame',metadata])}
    async destroy(){calls.push('destroy')}
  }
  const replay=createVisualReplay({RendererClass:FakeRenderer,fetchBlob:async ref=>new Blob([ref.sha256])});
  const state={graph:{inputs:['input0'],nodes:[{id:'n0',inputs:['input0'],outputs:['n0:0']}],output:'n0:0'},settings:{width:320,height:180,format:'rgba16float',blockSize:8},codes:{n0:{code:'shader'}},externalInputs:{input0:{sha256:'blob',resizeMode:'contain'}},trajectory:null,timeControl:{action:'pause',timeMs:42}};
  const canvas={};
  await replay.render(state,canvas);
  assert.deepEqual(calls[1],['configure',state.graph,{width:320,height:180},'rgba16float',8]);
  assert.deepEqual(calls[2],['shader','n0','shader']);
  assert.deepEqual(calls[3],['input','input0','blob','contain']);
  assert.deepEqual(calls[4],['targets',['n0:0']]);
  assert.equal(calls[5][0],'frame');
  assert.equal(calls[5][1].byteLength,160);
});

test('visual replay reuses the isolated Renderer across adjacent event boundaries', async () => {
  const calls=[];
  class FakeRenderer {
    async init(){calls.push('init');return this} async configure(){calls.push('configure')}
    async setShader(){calls.push('shader');return {ok:true,errors:[]}} async setInput(){calls.push('input')}
    async setPresentTargets(){calls.push('targets')} async frame(){calls.push('frame')} async destroy(){calls.push('destroy')}
  }
  const state={graph:{inputs:['input0'],nodes:[{id:'n0',inputs:['input0'],outputs:['n0:0']}],output:'n0:0'},settings:{width:10,height:10,format:'rgba8unorm',blockSize:8},codes:{n0:{version:1,code:'shader'}},externalInputs:{input0:{version:1,sha256:'blob',resizeMode:'stretch'}}};
  const replay=createVisualReplay({RendererClass:FakeRenderer,fetchBlob:async()=>new Blob()});
  await replay.render(state,{});await replay.render(structuredClone(state),{});
  assert.equal(calls.filter(x=>x==='init').length,1);assert.equal(calls.filter(x=>x==='configure').length,1);
  assert.equal(calls.filter(x=>x==='shader').length,1);assert.equal(calls.filter(x=>x==='input').length,1);assert.equal(calls.filter(x=>x==='frame').length,2);
});

test('visual replay rejects incomplete historical facts instead of rendering fallbacks', async () => {
  class FakeRenderer { async init(){return this} async configure(){} async destroy(){} }
  const replay=createVisualReplay({RendererClass:FakeRenderer,fetchBlob:async()=>new Blob()});
  const base={graph:{inputs:[],nodes:[{id:'n0',inputs:[],outputs:['n0:0']}],output:'n0:0'},settings:{width:10,height:10,format:'rgba8unorm',blockSize:8},codes:{}};
  await assert.rejects(()=>replay.render(base,{}),/缺少 Accepted Shader Version.*n0/);
});

test('visual replay uses only the final output presentation target', async () => {
  const calls=[];
  class FakeRenderer {
    async init(){return this} async configure(){} async setShader(){return {ok:true,errors:[]}}
    async setPresentTargets(targets){calls.push([...targets.keys()])} async frame(){} async destroy(){}
  }
  const state={graph:{inputs:[],nodes:[{id:'n0',inputs:[],outputs:['n0:0','n0:1']}],output:'n0:1'},settings:{width:10,height:10,format:'rgba8unorm',blockSize:8},codes:{n0:{code:'shader'}}};
  await createVisualReplay({RendererClass:FakeRenderer,fetchBlob:async()=>new Blob()}).render(state,{});
  assert.deepEqual(calls,[['n0:1']]);
});
