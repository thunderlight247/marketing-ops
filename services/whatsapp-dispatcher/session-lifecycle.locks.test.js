import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { clearStaleAuthLocks } from './session-lifecycle.js';

function tempProfile() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'wa-session-'));
}

function existsNoFollow(target) {
  try {
    fs.lstatSync(target);
    return true;
  } catch (_) {
    return false;
  }
}

function writeLock(dir, target = null) {
  const lockPath = path.join(dir, 'SingletonLock');
  if (target) fs.symlinkSync(target, lockPath);
  else fs.writeFileSync(lockPath, '');
}

test('removes lock files left behind by a dead Chromium process', () => {
  const dir = tempProfile();
  writeLock(dir, 'light-999999');
  fs.writeFileSync(path.join(dir, 'SingletonSocket'), '');
  fs.writeFileSync(path.join(dir, 'SingletonCookie'), '');

  const cleared = clearStaleAuthLocks(dir, { isProcessAlive: () => false, logger: { warn() {}, error() {} } });

  assert.equal(cleared, 3);
  assert.equal(fs.existsSync(path.join(dir, 'SingletonLock')), false);
  assert.equal(fs.existsSync(path.join(dir, 'SingletonSocket')), false);
  assert.equal(fs.existsSync(path.join(dir, 'SingletonCookie')), false);
});

test('leaves the profile untouched when a live Chromium owns it', () => {
  const dir = tempProfile();
  writeLock(dir, 'light-4242');
  fs.writeFileSync(path.join(dir, 'SingletonSocket'), '');

  const warnings = [];
  const cleared = clearStaleAuthLocks(dir, {
    isProcessAlive: (pid) => pid === 4242,
    logger: { warn: (msg) => warnings.push(msg), error() {} },
  });

  assert.equal(cleared, 0);
  assert.equal(existsNoFollow(path.join(dir, 'SingletonLock')), true);
  assert.equal(fs.existsSync(path.join(dir, 'SingletonSocket')), true);
  assert.match(warnings[0], /pid 4242/);
});

test('does nothing when a saved profile has no lock files', () => {
  const dir = tempProfile();
  assert.equal(clearStaleAuthLocks(dir, { isProcessAlive: () => false }), 0);
});
