export function createCaptureSession(renderer, {onCaptured}={}) {
  let active = false;
  return {
    get active() { return active; },
    async capture(request) {
      if (active) throw new Error('Capture already in progress');
      active = true;
      const snapshot = structuredClone(request);
      try {
        const blob = await renderer.capture(snapshot);
        await onCaptured?.({request:snapshot,blob});
        return blob;
      } finally {
        active = false;
      }
    },
  };
}
