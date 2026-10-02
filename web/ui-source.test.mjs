import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
test('texture previews open the shared fullscreen image modal on click',async()=>{const source=await readFile(new URL('./ui.js',import.meta.url),'utf8');assert.match(source,/canvas\.addEventListener\('click',\(\)=>fullscreenImage\(canvas,id\)\)/);});
