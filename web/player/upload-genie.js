let cancelActive = () => {};
let requestId = 0;

const clamp = (value, min = 0, max = 1) => Math.min(max, Math.max(min, value));
const lerp = (from, to, amount) => from + (to - from) * amount;
const easeOutCubic = value => 1 - (1 - value) ** 3;
const easeInOutCubic = value => value < .5 ? 4 * value ** 3 : 1 - (-2 * value + 2) ** 3 / 2;

function pulseTarget(target) {
  target.animate?.([
    { transform: 'scale(1)' },
    { transform: 'scale(1.06)', offset: .45 },
    { transform: 'scale(1)' },
  ], { duration: 180, easing: 'ease-out' });
}

async function captureCanvas(canvas) {
  if (typeof createImageBitmap === 'function') {
    try {
      const bitmap = await createImageBitmap(canvas);
      return { image: bitmap, dispose: () => bitmap.close() };
    } catch {
      // Some browsers do not allow ImageBitmap snapshots of a WebGPU canvas.
    }
  }

  const copy = document.createElement('canvas');
  copy.width = canvas.width;
  copy.height = canvas.height;
  const context = copy.getContext('2d');
  if (!context) throw new Error('无法创建预览快照');
  context.drawImage(canvas, 0, 0);
  return { image: copy, dispose: () => {} };
}

/**
 * Pull a still of the current preview into an upload target, like a minimized window.
 * This deliberately owns its errors: a decorative effect must never block submission.
 */
export function playUploadGenie(sourceCanvas, targetElement) {
  cancelActive();
  const currentRequest = ++requestId;

  if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
    pulseTarget(targetElement);
    return;
  }

  captureCanvas(sourceCanvas).then(({ image, dispose }) => {
    if (currentRequest !== requestId) {
      dispose();
      return;
    }
    const source = sourceCanvas.getBoundingClientRect();
    const target = targetElement.getBoundingClientRect();
    if (!source.width || !source.height || !target.width || !target.height) {
      dispose();
      pulseTarget(targetElement);
      return;
    }

    const overlay = document.createElement('canvas');
    overlay.className = 'upload-genie-overlay';
    const pixelRatio = window.devicePixelRatio || 1;
    overlay.width = Math.ceil(window.innerWidth * pixelRatio);
    overlay.height = Math.ceil(window.innerHeight * pixelRatio);
    const context = overlay.getContext('2d');
    if (!context) {
      dispose();
      return;
    }
    context.scale(pixelRatio, pixelRatio);
    document.body.append(overlay);

    const slices = 26;
    const start = performance.now();
    const duration = 780;
    const targetX = target.left + target.width / 2;
    const targetY = target.top + target.height / 2;
    let frame = 0;
    let finished = false;
    const cleanUp = () => {
      if (finished) return;
      finished = true;
      cancelAnimationFrame(frame);
      overlay.remove();
      dispose();
      if (cancelActive === cleanUp) cancelActive = () => {};
    };
    cancelActive = cleanUp;

    const draw = now => {
      if (finished) return;
      const progress = clamp((now - start) / duration);
      const globalProgress = easeInOutCubic(progress);
      context.clearRect(0, 0, window.innerWidth, window.innerHeight);

      for (let index = 0; index < slices; index += 1) {
        const top = index / slices;
        const bottom = (index + 1) / slices;
        // Lower scanlines leave first, forming the curved, tapering "genie" tail.
        const delay = (1 - bottom) * .36;
        const stripProgress = easeOutCubic(clamp((progress - delay) / (1 - delay)));
        const widthProgress = easeInOutCubic(clamp((globalProgress + stripProgress) / 2));
        const sourceY = top * image.height;
        const sourceHeight = (bottom - top) * image.height;
        const stripY = lerp(source.top + source.height * top, targetY, stripProgress);
        const stripHeight = lerp(source.height / slices + .8, Math.max(1, target.height / slices), stripProgress);
        const stripWidth = lerp(source.width, Math.max(2, target.width * .15), widthProgress);
        const stripX = lerp(source.left, targetX - stripWidth / 2, widthProgress);
        context.drawImage(image, 0, sourceY, image.width, sourceHeight, stripX, stripY, stripWidth, stripHeight);
      }

      if (progress < 1) frame = requestAnimationFrame(draw);
      else cleanUp();
    };

    targetElement.classList.add('upload-genie-target');
    window.setTimeout(() => targetElement.classList.remove('upload-genie-target'), duration);
    frame = requestAnimationFrame(draw);
  }).catch(() => pulseTarget(targetElement));
}
