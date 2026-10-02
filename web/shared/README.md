# Shared WebGPU renderer

`gpu.js` is a browser ES module with no framework dependencies. It supports the three activity formats (`rgba32float`, `rgba16float`, `rgba8unorm`), one-node player pipelines and topologically ordered admin DAGs.

## GPU observability and recovery

`Renderer.diagnostics()` reports selected adapter/device capabilities, node and texture counts, render size/format, estimated texture allocation, and device-lost details. Texture views and per-node texture/metadata bind groups are cached after configuration so steady-state frames do not recreate them.

The admin uses rolling frame telemetry for submission FPS, average CPU encode/submit duration, capacity, and consecutive advisory `passTimeoutMs` overruns. This budget does not claim to interrupt a running GPU dispatch. It attempts one device-lost recovery by rebuilding the Renderer from authoritative graph, Accepted Shader Versions, External Inputs and presentation targets; a second loss remains a visible failure.

## Core API

```js
import { Renderer, localGraph, generateTemplate, createMetadata,
  cameraMetadata, CameraClock, FORMATS } from './gpu.js';

const renderer = await new Renderer().init();
renderer.configure(localGraph(2, 1), { width: 960, height: 540 }, 'rgba32float', 8);
const source = renderer.template(2, 1);
const checked = await renderer.validate(source, 2, 1);
await renderer.setShader('local', source);
await renderer.setInput('input0', imageBlobOrBitmap, 'stretch');
await renderer.frame(createMetadata({ canvasSize: [960, 540] }));
await renderer.preview('output0', canvas);
const png = await renderer.export('output0', { width: 480, height: 270 }, 'png');
```

### `Renderer`

- `new Renderer({device?})`: accepts an injected `GPUDevice` for tests; otherwise call `await init()`.
- `await init()`: requests WebGPU. If the adapter reports the optional `readonly_and_readwrite_storage_textures` feature name, it is explicitly requested. Browsers that expose read-only storage textures as core behavior may omit that name, so `init()` also compiles a minimal read-only storage-texture pipeline and only rejects when the real pipeline probe fails.
- `configure(graph, size, format, block)`: allocates all textures and orders the graph. `graph` is `{ inputs: string[], nodes: [{id, inputs: string[], outputs: string[]}], output?: string }`. Every texture is the same `size` and `format`; block is 4, 8, or 16.
- `setInput(id, source, mode)`: uploads `Blob`, `ImageBitmap`, HTML image/video, or canvas sources. Modes: `stretch`, `cover`/`crop`, `contain`/`letterbox`, and `transparent`; PNG/JPEG inputs are sRGB-linearized before upload and resized with bilinear canvas filtering.
- `setShader(nodeId, source)`: validates ShaderFlow ABI v1, then compiles and installs a node pipeline. Returns `{ok, errors, abi}`. A node without a valid shader uses a generated fallback writing `(0, 0, 0, 1)`.
- `validate(source, p, q, format, block)`: first validates protected bindings, metadata layout, entry/workgroup contract, resource count and canonical output writes, then compiles and dispatches a small one-frame local validation. Returns `{ok, errors, abi}`.
- `frame(metadata)`: dispatches nodes strictly in topological order. `metadata` can be the `ArrayBuffer` from `createMetadata` or its object form.
- `setPresentTargets(targets)`: configures WebGPU canvas targets for live preview. `frame()` presents all configured textures with full-screen triangles in the same command submission as the compute graph, avoiding GPU→CPU readback.
- `preview(textureId, canvas)`: explicit async readback for non-live use. It clamps HDR/negative values, converts NaN to zero, applies linear-to-sRGB display mapping, and paints a 2D canvas.
- `capture({textureId,size,format,metadata})`: freezes the request, creates an isolated renderer using the current graph, Accepted Shader Versions and External Inputs, rewrites metadata canvas size, executes one frame at the requested resolution, and encodes PNG/JPEG. Live rendering updates cannot mutate the in-progress capture.
- `export(textureId, size, format)`: compatibility wrapper around `capture()`. Requested size cannot exceed live resolution. EXR is intentionally deferred.
- `destroy()`: releases textures and buffers.

### Templates and metadata

`generateTemplate(p, q, format, block)` creates WGSL with storage bindings, `_metadata`, `readN` (coordinate-clamped reads), `writeN` (out-of-range writes discarded), and `vec4f` player variables. `Renderer.template(...)` is the same function.

`shader-contract.js` defines ShaderFlow ABI v1. Full-text WGSL editing remains available, but protected resource bindings, the exact metadata layout, `main` workgroup size, resource count, output helper shape and final `writeN(coord, outputN)` calls must remain valid. Comments and strings are removed before structural checks so they cannot spoof protected declarations.

`createMetadata({time, pos, canvasSize, rot, invRot, depth})` returns the exact 160-byte uniform layout: time @0, pos @16, canvasSize @32, rot @48, invRot @96, depth @144. `cameraMetadata({time,width,height,pos,euler,fov})` computes depth from the fixed short-edge FOV and yaw/pitch/roll matrices. `CameraClock` implements performance-time origin, reset, pause, and resume.

`localGraph(p, q)` returns a player graph with `input0..inputN` and `output0..outputN` texture ids.
