import test from 'node:test';
import assert from 'node:assert/strict';
import { cleanupFailedClient } from './session-lifecycle.js';

test('failed initialization destroys and detaches its client before allowing retry', async () => {
  const calls = [];
  const client = {
    async destroy() {
      calls.push('destroy');
    },
  };
  const state = {
    client,
    status: 'initializing',
    failedClient: null,
    qrDataUrl: 'qr-data',
    phone: '+233241234567',
    pairingPhone: '233241234567',
    pairingCode: 'ABCD1234',
    pairingRequested: true,
    pairingError: 'previous error',
  };
  const logs = [];

  const cleaned = await cleanupFailedClient(state, client, 'workspace-1', {
    error: (...args) => logs.push(args),
  });

  assert.equal(cleaned, true);
  assert.deepEqual(calls, ['destroy']);
  assert.equal(state.client, null);
  assert.equal(state.failedClient, null);
  assert.equal(state.status, 'error');
  assert.equal(state.qrDataUrl, null);
  assert.equal(state.phone, '');
  assert.equal(state.pairingPhone, '');
  assert.equal(state.pairingCode, null);
  assert.equal(state.pairingRequested, false);
  assert.equal(state.pairingError, null);
  assert.deepEqual(logs, []);
});

test('failed cleanup logs destroy errors but still releases the failed client', async () => {
  const client = {
    async destroy() {
      throw new Error('profile close failed');
    },
  };
  const state = { client, status: 'initializing', failedClient: null };
  const logs = [];

  const cleaned = await cleanupFailedClient(state, client, 'workspace-2', {
    error: (...args) => logs.push(args),
  });

  assert.equal(cleaned, true);
  assert.equal(state.client, null);
  assert.equal(state.status, 'error');
  assert.equal(logs.length, 1);
  assert.match(logs[0][0], /workspace-2/);
  assert.match(logs[0][1], /profile close failed/);
});

test('stale initialization failure cannot destroy or replace a newer client', async () => {
  let destroyCalls = 0;
  const staleClient = { async destroy() { destroyCalls++; } };
  const currentClient = { async destroy() { destroyCalls++; } };
  const state = { client: currentClient, status: 'connected' };

  const cleaned = await cleanupFailedClient(state, staleClient, 'workspace-3');

  assert.equal(cleaned, false);
  assert.equal(destroyCalls, 0);
  assert.equal(state.client, currentClient);
  assert.equal(state.status, 'connected');
});
