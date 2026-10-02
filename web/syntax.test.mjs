import {readFile} from 'node:fs/promises';
import {SourceTextModule} from 'node:vm';

const scripts = [
  'web/ui.js',
  'web/shared/gpu.js',
  'web/camera/index.js',
  'web/admin/app.js',
  'web/player/app.js',
];

let failures = 0;
for (const script of scripts) {
  try {
    const source = await readFile(new URL(`../${script}`, import.meta.url), 'utf8');
    new SourceTextModule(source, {identifier: script});
    console.log(`ok - ${script}`);
  } catch (error) {
    failures++;
    console.error(`not ok - ${script}`);
    console.error(error);
  }
}

if (failures) process.exitCode = 1;
