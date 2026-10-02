import test from 'node:test';
import assert from 'node:assert/strict';
import { createRenderParameterTimeline } from './render-timeline.js';

test('live updates immediately replace the active render parameters', () => {
  const timeline = createRenderParameterTimeline({camera:{fov:60},input:'v1'});
  timeline.update({camera:{fov:75},input:'v2'});
  assert.deepEqual(timeline.snapshot(), {camera:{fov:75},input:'v2'});
});

test('pause freezes active parameters while retaining only the latest pending edit', () => {
  const timeline = createRenderParameterTimeline({camera:{fov:60},input:'v1'});
  timeline.pause();
  timeline.update({camera:{fov:70},input:'v2'});
  timeline.update({camera:{fov:80},input:'v3'});
  assert.deepEqual(timeline.snapshot(), {camera:{fov:60},input:'v1'});
  assert.deepEqual(timeline.pending(), {camera:{fov:80},input:'v3'});
});

test('resume atomically promotes the latest pending state', () => {
  const timeline = createRenderParameterTimeline({camera:{fov:60},input:'v1'});
  const frozen = timeline.pause();
  frozen.camera.fov = 1;
  timeline.update({camera:{fov:90},input:'v4'});
  assert.deepEqual(timeline.resume(), {camera:{fov:90},input:'v4'});
  assert.equal(timeline.paused, false);
});
