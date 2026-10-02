import test from 'node:test';
import assert from 'node:assert/strict';
import {createRoundCommandModel,normalizeAdminMode} from './control-surface.js';

test('running Round defaults to Live with a safe contextual command',()=>{
 const command=createRoundCommandModel({phase:'running',settings:{players:2},registrations:[{},{}],graph:{}});
 assert.equal(normalizeAdminMode('', 'running'),'live');
 assert.equal(command.phaseLabel,'运行中');
 assert.equal(command.primaryAction,null);
 assert.deepEqual(command.blockers,[]);
 assert.equal(command.canTerminate,true);
});

test('registration command explains why graph generation is blocked',()=>{
 const command=createRoundCommandModel({phase:'registration',settings:{players:3},registrations:[{}]});
 assert.equal(command.primaryAction.id,'generate');
 assert.equal(command.primaryAction.enabled,false);
 assert.deepEqual(command.blockers,['还需 2 名玩家报名']);
 assert.equal(command.registration.label,'1 / 3');
});

test('graph command promotes start only after a Draft Graph exists',()=>{
 const missing=createRoundCommandModel({phase:'graph',settings:{players:2},registrations:[{},{}]});
 assert.equal(missing.primaryAction.id,'start');assert.equal(missing.primaryAction.enabled,false);
 assert.deepEqual(missing.blockers,['Draft Graph 尚未生成或提交']);
 const ready=createRoundCommandModel({phase:'graph',settings:{players:2},registrations:[{},{}],graph:{nodes:[]}});
 assert.equal(ready.primaryAction.enabled,true);assert.deepEqual(ready.blockers,[]);
});
