import test from 'node:test';
import assert from 'node:assert/strict';
import { createPlayerCodeSession } from './code-session.js';

test('reconnect restores the Accepted Shader Version without scheduling an upload', () => {
  const session = createPlayerCodeSession({
    serverAccepted: {version: 7, code: '// accepted'},
    savedDraft: {},
    template: '// generated template',
  });

  assert.equal(session.draft, '// accepted');
  assert.equal(session.activeCode, '// accepted');
  assert.equal(session.acceptedVersion, 7);
  assert.equal(session.shouldUpload, false);
});

test('a first assignment exposes a template draft but does not accept or upload it', () => {
  const session = createPlayerCodeSession({
    serverAccepted: null,
    savedDraft: {},
    template: '// generated template',
  });

  assert.equal(session.draft, '// generated template');
  assert.equal(session.activeCode, '');
  assert.equal(session.acceptedVersion, 0);
  assert.equal(session.shouldUpload, false);
});

test('a local draft remains editable without replacing the server accepted shader', () => {
  const session = createPlayerCodeSession({
    serverAccepted: {version: 7, code: '// accepted'},
    savedDraft: {version: 8, code: '// unfinished draft'},
    template: '// generated template',
  });

  assert.equal(session.draft, '// unfinished draft');
  assert.equal(session.activeCode, '// accepted');
  assert.equal(session.acceptedVersion, 7);
  assert.equal(session.nextVersionFloor, 8);
  assert.equal(session.shouldUpload, false);
});
