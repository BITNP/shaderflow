import test from 'node:test';
import assert from 'node:assert/strict';
import { estimateTextureBytes, createFrameTelemetry } from './gpu-observability.js';

test('capacity report estimates texture allocation by format', () => {
  assert.equal(estimateTextureBytes({width:1920,height:1080,textureCount:7,format:'rgba16float'}),116121600);
  assert.equal(estimateTextureBytes({width:10,height:10,textureCount:2,format:'rgba8unorm'}),800);
});

test('frame telemetry reports rolling FPS duration and advisory budget streak', () => {
  const telemetry=createFrameTelemetry({budgetMs:10,windowSize:3,overBudgetLimit:2});
  telemetry.record({durationMs:8,at:100});
  telemetry.record({durationMs:12,at:116});
  const report=telemetry.record({durationMs:15,at:132});
  assert.equal(report.samples,3);
  assert.equal(report.averageFrameMs,11.67);
  assert.equal(report.fps,62.5);
  assert.equal(report.overBudgetStreak,2);
  assert.equal(report.degraded,true);
});

test('successful budget frame clears advisory degradation streak', () => {
  const telemetry=createFrameTelemetry({budgetMs:10,overBudgetLimit:2});
  telemetry.record({durationMs:12,at:0});telemetry.record({durationMs:13,at:16});
  assert.equal(telemetry.snapshot().degraded,true);
  assert.equal(telemetry.record({durationMs:4,at:32}).degraded,false);
});
