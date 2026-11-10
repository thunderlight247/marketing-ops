import test from 'node:test';
import assert from 'node:assert/strict';
import { createReconnectSupervisor } from './session-reconnect.js';

function fakeTimers() {
  const timers = new Map();
  let nextId = 1;
  return {
    setTimer(fn, ms) {
      const timer = { id: nextId++, fn, ms };
      timers.set(timer.id, timer);
      return timer;
    },
    clearTimer(timer) {
      timers.delete(timer.id);
    },
    get size() {
      return timers.size;
    },
    lastDelay() {
      return [...timers.values()].at(-1)?.ms;
    },
    async runNext() {
      const [timer] = timers.values();
      if (!timer) return false;
      timers.delete(timer.id);
      timer.fn();
      // Let the reconnect promise chain settle.
      for (let i = 0; i < 5; i++) await Promise.resolve();
      return true;
    },
  };
}

function settle() {
  return Promise.resolve().then(() => Promise.resolve());
}

test('schedules a saved session reconnect and stops when it is no longer eligible', async () => {
  const timers = fakeTimers();
  const reconnects = [];
  let eligible = true;
  const supervisor = createReconnectSupervisor({
    reconnect: (key) => reconnects.push(key),
    canReconnect: () => eligible,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  assert.equal(supervisor.schedule('workspace-1', 'init failed'), true);
  assert.equal(supervisor.isScheduled('workspace-1'), true);
  assert.equal(timers.lastDelay(), 5_000);

  await timers.runNext();
  assert.deepEqual(reconnects, ['workspace-1']);
  assert.equal(supervisor.isScheduled('workspace-1'), false);

  eligible = false;
  assert.equal(supervisor.schedule('workspace-1'), false);
  assert.equal(timers.size, 0);
});

test('backs off between failed attempts and resets after a confirmed connection', async () => {
  const timers = fakeTimers();
  const delays = [];
  const supervisor = createReconnectSupervisor({
    reconnect: async () => {
      throw new Error('still offline');
    },
    delays: [1_000, 4_000, 10_000],
    logger: { error: () => {} },
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  supervisor.schedule('workspace-2');
  assert.equal(timers.lastDelay(), 1_000);

  await timers.runNext();
  await settle();
  assert.equal(timers.lastDelay(), 4_000);
  assert.equal(supervisor.attemptOf('workspace-2'), 2);

  await timers.runNext();
  await settle();
  assert.equal(timers.lastDelay(), 10_000);

  // After the cap the delay stays at the largest step.
  await timers.runNext();
  await settle();
  assert.equal(timers.lastDelay(), 10_000);

  supervisor.reset('workspace-2');
  assert.equal(supervisor.attemptOf('workspace-2'), 0);
  await timers.runNext();
  assert.equal(timers.lastDelay(), 1_000);
});

test('cancel prevents a pending retry from running', async () => {
  const timers = fakeTimers();
  const reconnects = [];
  const supervisor = createReconnectSupervisor({
    reconnect: (key) => reconnects.push(key),
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  supervisor.schedule('workspace-3');
  assert.equal(supervisor.cancel('workspace-3'), true);
  assert.equal(supervisor.cancel('workspace-3'), false);
  assert.equal(timers.size, 0);
  assert.equal(await timers.runNext(), false);
  assert.deepEqual(reconnects, []);
});

test('snapshot exposes the queued retry for a dashboard countdown', () => {
  const timers = fakeTimers();
  const supervisor = createReconnectSupervisor({
    reconnect: () => {},
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  assert.equal(supervisor.snapshot('workspace-5'), null);
  supervisor.schedule('workspace-5', 'init failed: network down');

  const snapshot = supervisor.snapshot('workspace-5');
  assert.equal(snapshot.scheduled, true);
  assert.equal(snapshot.attempt, 1);
  assert.equal(snapshot.reason, 'init failed: network down');
  assert.ok(snapshot.retryInMs > 0 && snapshot.retryInMs <= 5_000);

  supervisor.cancel('workspace-5');
  assert.equal(supervisor.snapshot('workspace-5'), null);
});

test('a pending retry is not scheduled twice', () => {
  const timers = fakeTimers();
  const supervisor = createReconnectSupervisor({
    reconnect: () => {},
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });

  assert.equal(supervisor.schedule('workspace-4', 'disconnected'), true);
  assert.equal(supervisor.schedule('workspace-4', 'init failed'), false);
  assert.equal(timers.size, 1);
});
