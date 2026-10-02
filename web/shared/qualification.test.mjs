import test from 'node:test';
import assert from 'node:assert/strict';
import {createQualificationRecord,qualificationMatrixStatus} from './qualification.js';

test('qualification record captures browser GPU and evidence without credentials',()=>{
 const record=createQualificationRecord({browser:{name:'Chrome',version:'1'},gpu:{vendor:'NVIDIA',architecture:'RTX'},profile:{nodes:5,width:1920,height:1080,format:'rgba16float'},result:{fps:60,averageFrameMs:16.7},evidence:['gpu.json']});
 assert.equal(record.schemaVersion,1);assert.equal(record.browser.name,'Chrome');assert.equal(record.result.fps,60);assert.equal(record.credentialsIncluded,false);assert.deepEqual(record.evidence,['gpu.json']);assert.equal(JSON.stringify(record).includes('token'),false);
});

test('qualification matrix reports missing browser and profile evidence',()=>{
 const records=[createQualificationRecord({browser:{name:'Chrome',version:'1'},gpu:{},profile:{nodes:1,width:960,height:540,format:'rgba8unorm'},result:{fps:60}})];
 const status=qualificationMatrixStatus(records,{browsers:['Chrome','Edge'],profiles:[{nodes:1,width:960,height:540,format:'rgba8unorm'}]});
 assert.equal(status.complete,false);assert.equal(status.completed.length,1);assert.equal(status.missing.length,1);assert.equal(status.missing[0].browser,'Edge');
});
