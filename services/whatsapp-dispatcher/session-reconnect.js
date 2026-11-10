// Retries saved WhatsApp sessions that fail to start or drop unexpectedly.
// Pairing credentials stay on disk (LocalAuth); only the browser session is
// restarted, so a recovered session never asks the operator to scan again.
export const DEFAULT_RECONNECT_DELAYS = [5_000, 15_000, 30_000, 60_000, 120_000, 300_000];

export function createReconnectSupervisor({
  reconnect,
  canReconnect = () => true,
  delays = DEFAULT_RECONNECT_DELAYS,
  logger = console,
  setTimer = (fn, ms) => setTimeout(fn, ms),
  clearTimer = (timer) => clearTimeout(timer),
} = {}) {
  const pending = new Map();
  const attempts = new Map();

  function cancel(key) {
    const entry = pending.get(key);
    if (entry === undefined) return false;
    pending.delete(key);
    clearTimer(entry.timer);
    return true;
  }

  function cancelAll() {
    for (const key of [...pending.keys()]) cancel(key);
  }

  // Backoff goes back to the base delay once a session is really connected.
  function reset(key) {
    attempts.delete(key);
  }

  function isScheduled(key) {
    return pending.has(key);
  }

  function attemptOf(key) {
    return attempts.get(key) || 0;
  }

  // What the dashboard needs to show a countdown without guessing the backoff.
  function snapshot(key) {
    const entry = pending.get(key);
    if (!entry) return null;
    return {
      scheduled: true,
      attempt: entry.attempt,
      retryInMs: Math.max(0, entry.dueAt - Date.now()),
      reason: entry.reason,
    };
  }

  function schedule(key, reason = 'unknown') {
    if (!key || pending.has(key) || !canReconnect(key)) return false;

    const attempt = attemptOf(key);
    const delay = delays[Math.min(attempt, delays.length - 1)];
    attempts.set(key, attempt + 1);

    const timer = setTimer(() => {
      pending.delete(key);
      if (!canReconnect(key)) return;
      Promise.resolve()
        .then(() => reconnect(key, { attempt: attempt + 1, reason, delay }))
        .catch((err) => {
          logger.error(`[${key}] reconnect attempt failed:`, err?.message || err);
          schedule(key, reason);
        });
    }, delay);
    // A pending retry must not keep the process alive on shutdown.
    if (timer && typeof timer.unref === 'function') timer.unref();
    pending.set(key, { timer, dueAt: Date.now() + delay, attempt: attempt + 1, reason });
    return true;
  }

  return { schedule, cancel, cancelAll, reset, isScheduled, attemptOf, snapshot };
}
