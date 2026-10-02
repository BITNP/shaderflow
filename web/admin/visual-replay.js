import { Renderer, createMetadata } from '../shared/gpu.js';
import { createCameraController, computeCameraMetadata } from '../camera/index.js';

export function createVisualReplay({RendererClass=Renderer,fetchBlob}={}) {
  let renderer=null,configurationKey='',shaderVersions=new Map(),inputVersions=new Map();
  return {
    async render(state,canvas) {
      if(!state?.graph)throw new Error('回放状态缺少 Execution Graph');
      const settings=state.settings,key=JSON.stringify([state.graph,settings.width,settings.height,settings.format,settings.blockSize]);
      if(!renderer){renderer=await new RendererClass().init();}
      if(configurationKey!==key){await renderer.configure(state.graph,{width:settings.width,height:settings.height},settings.format,settings.blockSize);configurationKey=key;shaderVersions.clear();inputVersions.clear();}
      for(const node of state.graph.nodes){const code=state.codes?.[node.id];if(!code?.code)throw new Error(`回放状态缺少 Accepted Shader Version: ${node.id}`);const version=code.version??code.code;if(shaderVersions.get(node.id)!==version){const result=await renderer.setShader(node.id,code.code);if(!result.ok)throw new Error(`${node.id}: ${result.errors.join('\n')}`);shaderVersions.set(node.id,version);}}
      for(const id of state.graph.inputs){const input=state.externalInputs?.[id];if(!input)continue;const version=input.version??input.sha256;if(inputVersions.get(id)!==version){const blob=await fetchBlob(input);await renderer.setInput(id,blob,input.resizeMode);inputVersions.set(id,version);}}
      await renderer.setPresentTargets(new Map([[state.graph.output,[canvas]]]));
      await renderer.frame(replayMetadata(state));
      return renderer;
    },
    async destroy(){await renderer?.destroy?.();renderer=null;configurationKey='';shaderVersions.clear();inputVersions.clear();},
  };
}

function replayMetadata(state){
  const {width,height}=state.settings,timeControl=state.timeControl||{};
  const time=Number(timeControl.timeMs)||0;
  let camera={time,width,height,pos:[0,0,0],euler:[0,0,0],fov:60};
  if(state.trajectory?.active){const controller=createCameraController({initSource:state.trajectory.initSource,frameSource:state.trajectory.frameSource,mode:'script'});camera=controller.evaluate(time,{width,height});}
  const packed=camera.rot?camera:computeCameraMetadata(camera);
  return createMetadata({time,pos:packed.pos,canvasSize:[width,height],rot:packed.rot,invRot:packed.invRot,depth:packed.depth});
}
