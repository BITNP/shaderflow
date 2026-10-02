import test from 'node:test';import assert from 'node:assert/strict';import {readFile} from 'node:fs/promises';
const html=await readFile(new URL('./index.html',import.meta.url),'utf8'),source=await readFile(new URL('./app.js',import.meta.url),'utf8');
test('admin exposes four task modes and makes Final Output the Live stage',()=>{for(const mode of ['live','setup','replay','diagnostics'])assert.match(html,new RegExp(`data-mode="${mode}"`));assert.match(html,/class="live-stage card"/);assert.match(html,/id="finalCanvas"/);assert.match(html,/id="fullscreenStage"/);});
test('admin centralizes Round command and offers guarded logout',()=>{assert.match(html,/id="roundCommand"/);assert.match(html,/id="roundPrimary"/);assert.match(html,/id="logoutButton"/);assert.match(source,/停止权威大屏渲染/);assert.match(source,/authSession\.logout/);});
test('admin states direct GPU presentation rather than texture readback',()=>{assert.doesNotMatch(html,/全纹理读回/);assert.match(html,/无实时 CPU 读回/);});
