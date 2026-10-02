# Camera scripting API

Safe browser ES module; scripts are parsed/interpreted without `eval` or `Function`.

```js
import { createCameraController, computeCameraMetadata } from './index.js';
const camera = createCameraController({
  initSource: 'var phase = 0;',
  frameSource: 'pos[2] = -10 + sin(time / 1000); euler[1] = 10; fov = 60 + 5 * sin(time / 2000);'
});
const metadata = camera.evaluate( performance.now(), { width: 960, height: 540 });
```

`reset()` reruns initialization and is intended for time-origin reset. `evaluate(time, options)` runs the frame script. Defaults are position `(0,0,0)`, Euler `(0,0,0)`, and FOV `60°`. FOV is clamped to 1–179°. `depth = min(width,height)/(2*tan(fov*pi/360))`.

Supported syntax: declarations, assignments, arithmetic/comparison/logical operators, arrays (bounded indexing), `if`, ternary, `switch`, and whitelisted `Math` functions (`sin`, `cos`, `tan`, `abs`, `min`, `max`, `sqrt`, etc.; constants `PI`, `E`). No loops, user functions, objects, globals, or browser APIs. Scripts have a bounded step budget.

Euler angles are intrinsic yaw/pitch/roll (Y-P-R), in degrees. `fov` is also interpreted in degrees, with a default of `60` and a valid range of `1`–`179`; for example `fov = 90` means a 90-degree field of view. The script math functions (`sin`, `cos`, `tan`) use their normal JavaScript convention and receive radians. `rot` and `invRot` are 3×3 column-major arrays. Camera fields may be assigned as `pos[0..2]`, `euler[0..2]` (or `.x/.y/.z`, `.y/.p/.r` where applicable), and `fov`.
