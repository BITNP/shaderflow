export function createRenderParameterTimeline(initial) {
  let active = clone(initial);
  let pending = clone(initial);
  let paused = false;

  return {
    get paused() { return paused; },
    snapshot() { return clone(active); },
    pending() { return clone(pending); },
    update(next) {
      pending = clone(next);
      if (!paused) active = clone(pending);
      return clone(active);
    },
    pause() {
      paused = true;
      pending = clone(active);
      return clone(active);
    },
    resume() {
      active = clone(pending);
      paused = false;
      return clone(active);
    },
  };
}

function clone(value) {
  if (value === undefined) return undefined;
  return structuredClone(value);
}
