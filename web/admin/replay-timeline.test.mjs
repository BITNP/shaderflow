import test from 'node:test';
import assert from 'node:assert/strict';
import { createReplayTimeline } from './replay-timeline.js';

test('timeline steps over archive event sequence boundaries', () => {
  const timeline=createReplayTimeline([{seq:4,event:{at:'2026-01-01T00:00:01Z',type:'code.updated'}},{seq:9,event:{at:'2026-01-01T00:00:03Z',type:'input.updated'}}]);
  assert.equal(timeline.current().seq,4);
  assert.equal(timeline.next().seq,9);
  assert.equal(timeline.next().seq,9);
  assert.equal(timeline.previous().seq,4);
  assert.equal(timeline.seek(8).seq,4);
  assert.equal(timeline.seek(9).seq,9);
});

test('timeline computes timestamp playback delay with speed and an upper bound', () => {
  const timeline=createReplayTimeline([{seq:1,event:{at:'2026-01-01T00:00:00Z'}},{seq:2,event:{at:'2026-01-01T00:00:10Z'}}],{maxDelayMs:2000});
  assert.equal(timeline.delayToNext(1),2000);
  assert.equal(timeline.delayToNext(4),2000);
  timeline.seek(2);
  assert.equal(timeline.delayToNext(1),null);
});

test('empty timeline exposes no current event', () => {
  const timeline=createReplayTimeline([]);
  assert.equal(timeline.current(),null);
  assert.equal(timeline.next(),null);
  assert.equal(timeline.previous(),null);
});
