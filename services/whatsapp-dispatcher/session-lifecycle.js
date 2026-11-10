import fs from 'node:fs';
import path from 'node:path';

export async function cleanupFailedClient(state, client, clientKey, logger = console) {
  if (state.client !== client) return false;

  // Keep start/pair routes in their existing non-retryable state until Chromium
  // has closed its profile. Otherwise a quick retry can hit LocalAuth's lock.
  state.status = 'initializing';
  state.failedClient = client;
  try {
    await client.destroy();
  } catch (err) {
    logger.error(`[${clientKey}] Client cleanup error:`, err?.message || err);
  }

  // destroy() can emit a disconnect event; don't overwrite a newer session
  // created by another lifecycle transition while cleanup was in progress.
  if (state.client !== client) return false;

  state.client = null;
  state.failedClient = null;
  state.status = 'error';
  state.qrDataUrl = null;
  state.phone = '';
  state.pairingPhone = '';
  state.pairingCode = null;
  state.pairingRequested = false;
  state.pairingError = null;
  return true;
}

const BROWSER_LOCK_FILES = ['SingletonLock', 'SingletonSocket', 'SingletonCookie'];

function existsNoFollow(target) {
  try {
    fs.lstatSync(target);
    return true;
  } catch (_) {
    return false;
  }
}

function defaultIsProcessAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    return err?.code === 'EPERM';
  }
}

// Chromium records its owning process as a `<hostname>-<pid>` symlink. Only a
// killed process leaves it behind; a live one must keep it.
function lockOwnerPid(lockPath) {
  try {
    const target = fs.readlinkSync(lockPath);
    const pid = Number(String(target).split('-').pop());
    return Number.isInteger(pid) && pid > 0 ? pid : null;
  } catch (_) {
    return null;
  }
}

// A hard stop (crash, OOM kill, host reboot) can leave Chromium's profile
// locks in place. Chromium then refuses to start with "The browser is already
// running for ...", which forced a fresh QR scan even though the WhatsApp
// pairing was still on disk. Remove locks only when their owner is gone.
export function clearStaleAuthLocks(sessionDir, { isProcessAlive = defaultIsProcessAlive, logger = console } = {}) {
  let cleared = 0;
  for (const name of BROWSER_LOCK_FILES) {
    const lockPath = path.join(sessionDir, name);
    // lstat, not existsSync: a stale SingletonLock is a symlink to a dead pid
    // and therefore a broken link, which existsSync would report as missing.
    if (!existsNoFollow(lockPath)) continue;

    if (name === 'SingletonLock') {
      const owner = lockOwnerPid(lockPath);
      if (owner !== null && isProcessAlive(owner)) {
        logger.warn(`[${sessionDir}] Chromium profile is in use by pid ${owner}; leaving its lock files alone.`);
        return 0;
      }
    }

    try {
      fs.rmSync(lockPath, { force: true });
      cleared++;
    } catch (err) {
      logger.error(`[${sessionDir}] could not remove stale ${name}:`, err?.message || err);
    }
  }
  return cleared;
}
