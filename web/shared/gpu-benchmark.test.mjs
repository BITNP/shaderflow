import test from 'node:test';
import assert from 'node:assert/strict';
import { createGpuBenchmarkSession, expectedBenchmarkProfiles } from './gpu-benchmark.js';

test('benchmark session aggregates a stable profile and device facts',()=>{
 const session=createGpuBenchmarkSession({warmupFrames:2,sampleFrames:3});
 session.begin({nodes:5,width:1920,height:1080,format:'rgba16float',diagnostics:{adapterInfo:{device:'RTX'},limits:{maxTextureDimension2D:16384}}});
 session.record({durationMs:20,at:0});session.record({durationMs:18,at:16});
 session.record({durationMs:10,at:32});session.record({durationMs:12,at:48});session.record({durationMs:11,at:64});
 const report=session.finish();assert.equal(report.profile.nodes,5);assert.equal(report.samples,3);assert.equal(report.averageFrameMs,11);assert.equal(report.minFrameMs,10);assert.equal(report.maxFrameMs,12);assert.equal(report.adapterInfo.device,'RTX');
});

test('benchmark matrix defines all 27 required profiles',()=>{
 const profiles=expectedBenchmarkProfiles();assert.equal(profiles.length,27);
 assert.deepEqual(new Set(profiles.map(x=>x.nodes)),new Set([1,5,10]));
 assert.deepEqual(new Set(profiles.map(x=>`${x.width}x${x.height}`)),new Set(['960x540','1920x1080','3840x2160']));
 assert.deepEqual(new Set(profiles.map(x=>x.format)),new Set(['rgba8unorm','rgba16float','rgba32float']));
});
