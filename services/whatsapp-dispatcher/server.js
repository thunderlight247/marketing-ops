/**
 * WhatsApp Dispatcher Service
 * ─────────────────────────────────────────────────────────────────────────────
 * Per-workspace WhatsApp session manager. Each workspace gets its own session
 * directory so QR scan is one-time per workspace (persisted via LocalAuth).
 *
 * Endpoints consumed by the Go API:
 *   POST /sessions/:workspaceId/start      → start/get session, return QR or connected status
 *   GET  /sessions/:workspaceId/status     → current session status
 *   POST /sessions/:workspaceId/send       → send a WhatsApp message
 *   POST /sessions/:workspaceId/disconnect → destroy session
 *
 * Run: node server.js
 * Port: 8083 (overridden by PORT env var)
 */
import 'dotenv/config';
import express from 'express';
import wwebjs from 'whatsapp-web.js';
const { Client, LocalAuth } = wwebjs;
import qrcode from 'qrcode';
import fs from 'node:fs';
import path from 'node:path';

// Node 24 kills the whole process on any unhandled promise rejection. The
// whatsapp-web.js library has one internally — its own 'framenavigated'
// listener (Client.js) rejects when the headless page reloads mid-session,
// which would take the dispatcher down mid-batch. Log and keep serving; the
// per-send error paths already recover or report cleanly.
process.on('unhandledRejection', (err) => {
  console.error('[unhandledRejection]', err?.message || err);
});
process.on('uncaughtException', (err) => {
  console.error('[uncaughtException]', err?.message || err);
});

const app = express();
app.use(express.json());

const PORT = Number(process.env.PORT) || 8083;
const EXEC_PATH = process.env.CHROMIUM_PATH || '/usr/bin/chromium';
const SESSIONS_DIR = './data/sessions';
const INTERNAL_SECRET = process.env.DISPATCHER_SECRET || '';
const API_BASE_URL = process.env.API_BASE_URL || 'http://localhost:8081';

// ─── per-workspace sessions ──────────────────────────────────────────────────
// Every workspace owns its own WhatsApp number, so sending starts a session
// keyed by the workspace id and the pairing is kept on disk (LocalAuth). That
// makes the QR scan a ONE-TIME step per workspace: after a successful scan the
// credentials are restored on every later dispatcher restart and the workspace
// reports "connected" without ever asking to connect again.
//
// The "shared" session is the account used before per-workspace numbers
// existed. It stays only as a fallback for a workspace that has not claimed a
// number of its own yet, so an in-flight batch keeps sending while each
// workspace moves to its own business number.
const SHARED_KEY = 'shared';
const OWN_MARKER = 'OWN_NUMBER';

function sessionDir(key) {
  return path.join(SESSIONS_DIR, `session-${key}`);
}

// LocalAuth stores the browser profile (and with it the WhatsApp credentials)
// under <sessionDir>/Default. If that exists the pairing can be restored with
// no QR scan.
function savedAuthExists(key) {
  return fs.existsSync(path.join(sessionDir(key), 'Default'));
}

// A marker means this workspace has paired its own number, so the shared
// fallback is abandoned for good.
function isClaimed(key) {
  return fs.existsSync(path.join(sessionDir(key), OWN_MARKER));
}

function claimOwnNumber(key, phone) {
  if (!key || key === SHARED_KEY) return;
  try {
    fs.mkdirSync(sessionDir(key), { recursive: true });
    fs.writeFileSync(path.join(sessionDir(key), OWN_MARKER), phone || '');
  } catch (err) {
    console.error(`[${key}] could not write own-number marker:`, err.message);
  }
}

function releaseOwnNumber(key) {
  try {
    fs.rmSync(path.join(sessionDir(key), OWN_MARKER), { force: true });
  } catch (_) {}
}

function isLive(state) {
  return !!state && ['connected', 'initializing', 'authenticated', 'qr_ready'].includes(state.status);
}

// Which on-disk session serves this workspace. Own number first (claimed or
// already connected), then the shared fallback, then its own (fresh QR).
function resolveKey(workspaceId) {
  if (!workspaceId || workspaceId === SHARED_KEY) return SHARED_KEY;
  if (isClaimed(workspaceId) || isLive(sessions.get(workspaceId))) return workspaceId;
  if (isLive(sessions.get(SHARED_KEY)) || savedAuthExists(SHARED_KEY)) return SHARED_KEY;
  return workspaceId;
}

// ─── internal auth middleware ─────────────────────────────────────────────────
// When DISPATCHER_SECRET is set, every /sessions/* request must supply the
// matching X-Internal-Secret header. The /health endpoint is always public.
function requireInternalAuth(req, res, next) {
  if (!INTERNAL_SECRET) return next(); // dev mode: no secret configured
  if (req.headers['x-internal-secret'] !== INTERNAL_SECRET) {
    return res.status(401).json({ error: 'unauthorized' });
  }
  next();
}

app.use('/sessions', requireInternalAuth);

// ─── health ───────────────────────────────────────────────────────────────────
app.get('/health', (_req, res) => {
  res.json({ status: 'ok', sessions: sessions.size });
});

// In-memory session map: workspaceId → SessionState
const sessions = new Map();

/** @typedef {{ client: Client|null, status: string, qrDataUrl: string|null, phone: string, pairingPhone: string, pairingCode: string|null, pairingRequested: boolean, pairingError: string|null }} SessionState */

function makeState() {
  return {
    client: null,
    status: 'disconnected',
    qrDataUrl: null,
    phone: '',
    pairingPhone: '',
    pairingCode: null,
    pairingRequested: false,
    pairingError: null,
  };
}

// WhatsApp accepts an international number with no symbols (for example
// 233241234567). Keep only digits so the UI can send +233… or spaces safely.
function normalizePairingPhone(value) {
  const digits = String(value || '').replace(/[^0-9]/g, '');
  return digits.length >= 8 && digits.length <= 15 ? digits : '';
}

// The library returns an 8-character code. Keep it raw here; the dashboard
// groups it as ABCD-EFGH for readability.
function cleanPairingCode(value) {
  const code = String(value || '').replace(/[^a-zA-Z0-9]/g, '').toUpperCase();
  return code || null;
}

// Request a pairing code on the SAME client that owns the QR. This is the
// alternate login method: scanning the QR and entering this code both complete
// the same persisted session, so whichever the operator uses wins.
async function requestPairingCodeForState(clientKey, client, state) {
  if (!state.pairingPhone || state.pairingRequested) return;
  state.pairingRequested = true;
  state.pairingError = null;
  try {
    const code = await client.requestPairingCode(state.pairingPhone);
    if (state.client !== client) return;
    const cleaned = cleanPairingCode(code);
    if (cleaned) state.pairingCode = cleaned;
    console.log(`[${clientKey}] Pairing code requested for ${state.pairingPhone}.`);
  } catch (err) {
    if (state.client !== client) return;
    // Let the next QR refresh try again; the current code was never produced.
    state.pairingRequested = false;
    state.pairingError = err?.message || 'Could not request a pairing code';
    console.error(`[${clientKey}] Pairing code error:`, state.pairingError);
  }
}

function sessionPayload(key, state) {
  const out = {
    status: state.status,
    phone: state.phone,
    source: key === SHARED_KEY ? 'shared' : 'own',
  };
  if (state.qrDataUrl) out.qr = state.qrDataUrl;
  if (state.pairingPhone) out.pairingPhone = state.pairingPhone;
  if (state.pairingCode) out.pairingCode = state.pairingCode;
  if (state.pairingError) out.pairingError = state.pairingError;
  return out;
}

function getStateOf(key) {
  if (!sessions.has(key)) sessions.set(key, makeState());
  return sessions.get(key);
}

/**
 * Create and wire up a WhatsApp client for a workspace.
 * If the session already has persisted auth (LocalAuth), it reconnects without a QR.
 */
function createClient(clientKey, options = {}) {
  const client = new Client({
    authStrategy: new LocalAuth({
      clientId: clientKey,
      dataPath: SESSIONS_DIR,
    }),
    takeoverOnConflict: true,
    puppeteer: {
      executablePath: EXEC_PATH,
      headless: true,
      args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',
        '--disable-accelerated-2d-canvas',
        '--no-first-run',
        '--no-zygote',
        '--disable-gpu',
      ],
    },
  });

  const state = getStateOf(clientKey);
  state.client = client;
  state.status = 'initializing';
  if (options.pairingPhone) {
    state.pairingPhone = normalizePairingPhone(options.pairingPhone);
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
  } else {
    // A reconnect must never show or auto-request a code from an earlier
    // login attempt. Pairing starts only when the operator asks for it.
    state.pairingPhone = '';
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
  }

  client.on('qr', async (qr) => {
    try {
      state.qrDataUrl = await qrcode.toDataURL(qr);
      state.status = 'qr_ready';
      console.log(`[${clientKey}] QR ready.`);
      // If the operator chose the phone-number route, request the code now
      // that WhatsApp's auth store is ready. The QR remains displayed as the
      // alternate method, so either one can be used.
      if (state.pairingPhone && !state.pairingRequested) {
        void requestPairingCodeForState(clientKey, client, state);
      }
    } catch (err) {
      console.error(`[${clientKey}] QR encode error:`, err.message);
    }
  });

  client.on('authenticated', () => {
    state.status = 'authenticated';
    state.qrDataUrl = null;
    state.pairingPhone = '';
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
    console.log(`[${clientKey}] Authenticated.`);
  });

  // The library refreshes the code every three minutes and emits each one.
  // Keep the latest code available to the dashboard while it waits.
  client.on('code', (code) => {
    if (state.client !== client) return;
    const cleaned = cleanPairingCode(code);
    if (cleaned) state.pairingCode = cleaned;
  });

  client.on('ready', async () => {
    // "ready" can fire before WhatsApp's comms layer is primed — sends in
    // those first seconds fail with "[comms] sendIq called before
    // startComms". Wait for the app to settle, confirm the client really
    // reports a live session, and only then advertise "connected" to the
    // API/dashboard/engine.
    for (let attempt = 0; attempt < 4; attempt++) {
      await new Promise((r) => setTimeout(r, 3000));
      let live = null;
      try { live = await client.getState(); } catch (_) {}
      if (live === 'CONNECTED') break;
      console.log(`[${clientKey}] ready fired but state=${live} — waiting for comms to settle`);
    }
    // A 'disconnected' during the settle window put a new state in place;
    // never overwrite it with a status from a client that is already dead.
    if (state.client && state.client !== client) return;
    state.status = 'connected';
    state.pairingPhone = '';
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
    try {
      const info = client.info;
      state.phone = info?.wid?.user ? '+' + info.wid.user : '';
    } catch (_) {}
    // A workspace-keyed session that reaches "connected" is this workspace's
    // own number from now on — it never falls back to the shared account again.
    claimOwnNumber(clientKey, state.phone);
    console.log(`[${clientKey}] Connected. Phone: ${state.phone}`);
  });

  client.on('disconnected', (reason) => {
    state.status = 'disconnected';
    state.qrDataUrl = null;
    state.phone = '';
    state.client = null;
    state.pairingPhone = '';
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
    console.log(`[${clientKey}] Disconnected:`, reason);
  });

  client.on('auth_failure', (msg) => {
    state.status = 'auth_failure';
    state.pairingPhone = '';
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
    console.error(`[${clientKey}] Auth failure:`, msg);
  });

  client.on('message', async (msg) => {
    // Ignore outbound messages sent by us
    if (msg.fromMe) return;

    // Parse sender's phone number
    const rawNumber = (msg.from || '').replace('@c.us', '').replace(/[^0-9]/g, '');
    const phone = rawNumber ? '+' + rawNumber : '';
    const body = msg.body;
    if (!phone || !body) return;

    console.log(`[${clientKey}] Inbound WhatsApp message from ${phone}: ${body.slice(0, 60)}`);

    try {
      // In shared mode one number serves every workspace, so replies cannot be
      // attributed to a workspace by session alone — the API matches the lead
      // by phone across workspaces. Workspace-scoped dispatchers keep their
      // existing per-workspace webhook.
      const webhookUrl = SHARED_MODE
        ? `${API_BASE_URL}/api/v1/lead-engine/whatsapp/inbound`
        : `${API_BASE_URL}/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/inbound`;
      const headers = { 'Content-Type': 'application/json' };
      if (INTERNAL_SECRET) {
        headers['X-Internal-Secret'] = INTERNAL_SECRET;
      }
      await fetch(webhookUrl, {
        method: 'POST',
        headers,
        body: JSON.stringify({
          phone,
          body,
          messageId: msg.id?._serialized || '',
          timestamp: msg.timestamp || Math.floor(Date.now() / 1000)
        })
      });
    } catch (err) {
      console.error(`[${clientKey}] Failed to forward inbound message:`, err.message);
    }
  });

  client.initialize().catch((err) => {
    state.status = 'error';
    console.error(`[${clientKey}] Init error:`, err.message);
  });

  return client;
}

// ─── routes ───────────────────────────────────────────────────────────────────

/**
 * POST /sessions/:workspaceId/start
 * Starts a WhatsApp session (or returns existing). Responds with:
 *   { status: "qr_ready",  qr: "<data URL>" }   — scan required
 *   { status: "connected", phone: "+233..." }     — already logged in
 *   { status: "initializing" }                    — starting up
 */
app.post('/sessions/:workspaceId/start', async (req, res) => {
  const { workspaceId } = req.params;
  const key = resolveKey(workspaceId);
  const state = getStateOf(key);

  if (state.status === 'connected') {
    return res.json(sessionPayload(key, state));
  }
  if (state.status === 'qr_ready') {
    return res.json(sessionPayload(key, state));
  }
  if (state.status === 'initializing' || state.status === 'authenticated') {
    return res.json(sessionPayload(key, state));
  }

  // Not started — create a new client. With saved credentials this reconnects
  // silently (no QR); only a brand-new workspace gets a QR to scan once.
  createClient(key);

  // Wait briefly for QR or connected status to surface.
  await new Promise((r) => setTimeout(r, 3000));

  const fresh = getStateOf(key);
  return res.json(sessionPayload(key, fresh));
});

/**
 * POST /sessions/:workspaceId/own/start
 * Links THIS workspace's own WhatsApp number, even when it is currently
 * borrowing the shared account. Reuses an already saved pairing, otherwise
 * returns a QR to scan once.
 */
app.post('/sessions/:workspaceId/own/start', async (req, res) => {
  const { workspaceId } = req.params;
  const key = workspaceId;
  if (!key || key === SHARED_KEY) {
    return res.status(400).json({ success: false, error: 'a workspace id is required' });
  }
  const state = getStateOf(key);

  if (state.status === 'connected') {
    claimOwnNumber(key, state.phone);
    return res.json(sessionPayload(key, state));
  }
  if (state.status === 'qr_ready') {
    return res.json(sessionPayload(key, state));
  }
  if (state.status === 'initializing' || state.status === 'authenticated') {
    return res.json(sessionPayload(key, state));
  }

  createClient(key);
  await new Promise((r) => setTimeout(r, 3000));

  return res.json(sessionPayload(key, getStateOf(key)));
});

/**
 * Link with a phone-number pairing code instead of scanning the QR. The code
 * and the QR authenticate the same session; this endpoint just asks WhatsApp
 * to send/refresh a code for the supplied international number.
 *
 * POST /sessions/:workspaceId/pair      body: { phone: "233241234567" }
 * POST /sessions/:workspaceId/own/pair  body: { phone: "233241234567" }
 */
async function pairSession(req, res, own) {
  const { workspaceId } = req.params;
  const phone = normalizePairingPhone(req.body?.phone);
  if (!phone) {
    return res.status(400).json({
      success: false,
      error: 'Enter a phone number with its country code, digits only (8–15 digits).',
    });
  }
  if (own && (!workspaceId || workspaceId === SHARED_KEY)) {
    return res.status(400).json({ success: false, error: 'a workspace id is required' });
  }

  const key = own ? workspaceId : resolveKey(workspaceId);
  const state = getStateOf(key);

  if (state.status === 'connected') {
    if (own) claimOwnNumber(key, state.phone);
    return res.json(sessionPayload(key, state));
  }

  // Switching the requested number invalidates the previous pending request.
  if (state.pairingPhone !== phone || state.pairingError) {
    state.pairingPhone = phone;
    state.pairingCode = null;
    state.pairingRequested = false;
    state.pairingError = null;
  }

  if (state.client && state.status === 'qr_ready') {
    await requestPairingCodeForState(key, state.client, state);
    return res.json(sessionPayload(key, state));
  }
  if (state.client && (state.status === 'initializing' || state.status === 'authenticated')) {
    // The QR handler requests the code as soon as WhatsApp is ready.
    return res.json(sessionPayload(key, state));
  }

  if (state.client) {
    try { await state.client.destroy(); } catch (_) {}
    state.client = null;
  }
  createClient(key, { pairingPhone: phone });
  await new Promise((r) => setTimeout(r, 3000));
  return res.json(sessionPayload(key, getStateOf(key)));
}

app.post('/sessions/:workspaceId/pair', (req, res) => void pairSession(req, res, false));
app.post('/sessions/:workspaceId/own/pair', (req, res) => void pairSession(req, res, true));

/**
 * GET /sessions/:workspaceId/status
 */
app.get('/sessions/:workspaceId/status', (req, res) => {
  const { workspaceId } = req.params;
  const key = resolveKey(workspaceId);
  const state = getStateOf(key);
  // A saved pairing revives itself instead of showing "disconnected": a
  // one-time scan must stay one-time, even after a dispatcher restart.
  if (!state.client && (state.status === 'disconnected' || state.status === 'error') && savedAuthExists(key)) {
    console.log(`[${key}] restoring saved session for status check (no QR needed)`);
    createClient(key);
  }
  res.json(sessionPayload(key, state));
});

/**
 * GET /sessions
 * Audit of every WhatsApp session the dispatcher knows about: which workspace
 * it belongs to, which number it is paired to, and whether it is live.
 */
app.get('/sessions', (_req, res) => {
  const out = [];
  const listed = new Set();
  for (const [key, state] of sessions.entries()) {
    listed.add(key);
    out.push({
      workspaceId: key === SHARED_KEY ? null : key,
      source: key === SHARED_KEY ? 'shared' : 'own',
      status: state.status,
      phone: state.phone,
      connected: state.status === 'connected',
    });
  }
  // Workspaces paired on disk but not currently running still count as paired.
  let entries = [];
  try {
    entries = fs.readdirSync(SESSIONS_DIR);
  } catch (_) {}
  for (const entry of entries) {
    if (!entry.startsWith('session-')) continue;
    const key = entry.slice('session-'.length);
    if (!key || key === SHARED_KEY || listed.has(key)) continue;
    if (!isClaimed(key)) continue;
    out.push({ workspaceId: key, source: 'own', status: 'disconnected', phone: '', connected: false });
  }
  res.json({ sessions: out });
});

/**
 * POST /sessions/:workspaceId/send
 * Body: { phone: "+233241234567", message: "..." }
 */
app.post('/sessions/:workspaceId/send', async (req, res) => {
  const { workspaceId } = req.params;
  const { phone, message } = req.body;

  if (!phone || !message) {
    return res.status(400).json({ success: false, error: 'phone and message are required' });
  }

  const key = resolveKey(workspaceId);
  const state = getStateOf(key);
  if (!state.client && savedAuthExists(key)) {
    // Session dropped but the pairing is on disk: bring it back silently. The
    // API treats this 409 as "not sent" and retries next run.
    createClient(key);
  }
  if (state.status !== 'connected' || !state.client) {
    return res.status(409).json({ success: false, error: 'WhatsApp not connected for this workspace' });
  }

  try {
    // Normalize phone: digits only, then resolve to the account's real WhatsApp
    // id. WhatsApp's LID-era protocol rejects blind `<number>@c.us` sends with
    // "No LID for user"; getNumberId performs the same lookup the UI does and
    // returns null when the number is not registered on WhatsApp at all.
    const normalized = phone.replace(/[^\d]/g, '');
    const chatId = await state.client.getNumberId(normalized);
    if (!chatId) {
      return res.status(400).json({
        success: false,
        error: `Number ${normalized} is not registered on WhatsApp`,
        notOnWhatsApp: true,
      });
    }
    let result;
    try {
      result = await state.client.sendMessage(chatId._serialized, message);
    } catch (sendErr) {
      // A session that just connected throws "[comms] sendIq called before
      // startComms": the messaging layer is not primed yet and the message
      // was provably NOT delivered, so one retry after a settle wait is
      // safe (no duplicate risk).
      if (!/startComms/i.test(sendErr?.message || '')) throw sendErr;
      console.log(`[${key}] comms not ready for ${normalized}; settling 8s and retrying once`);
      await new Promise((r) => setTimeout(r, 8000));
      result = await state.client.sendMessage(chatId._serialized, message);
    }
    // NOTE: whatsapp-web.js can resolve sendMessage with undefined even when
    // the message was accepted; the message itself must never crash the
    // response with "Cannot read properties of undefined (reading 'id')" —
    // that made the API mark delivered leads as failed.
    return res.json({ success: true, messageId: result?.id?.id || result?.id?._serialized || null });
  } catch (err) {
    console.error(`[${key}] Send error:`, err?.message);
    // IMPORTANT: sendMessage failing does NOT mean the message was not sent.
    // If we cannot prove non-delivery, tell the API the outcome is unknown;
    // the message may still have gone out and must never be re-sent blindly.
    if (/Cannot read properties|reading 'id'|Timed out|timeout/i.test(err?.message || '')) {
      return res.status(503).json({ success: false, error: 'send outcome unknown — do not retry automatically', outcomeUnknown: true });
    }
    // A crashed headless Chromium surfaces as "detached Frame" / closed
    // target errors while state.status still says connected. The session is
    // dead: drop it and re-initialize from the persisted LocalAuth so the
    // next send (or schwarz status check) reconnects without a fresh QR scan.
    if (/detached frame|target closed|session closed|protocol error/i.test(err.message)) {
      state.status = 'disconnected';
      try { if (state.client) await state.client.destroy(); } catch (_) {}
      state.client = null;
      createClient(key);
      console.log(`[${key}] send failed with a dead browser; re-initializing session from saved auth.`);
    }
    return res.status(500).json({ success: false, error: err.message });
  }
});

/**
 * POST /sessions/:workspaceId/disconnect
 * Destroys the WhatsApp session and clears auth.
 */
app.post('/sessions/:workspaceId/disconnect', async (req, res) => {
  const { workspaceId } = req.params;
  const key = resolveKey(workspaceId);
  const state = getStateOf(key);
  if (state.client) {
    try {
      await state.client.destroy();
    } catch (_) {}
    state.client = null;
  }
  state.status = 'disconnected';
  state.qrDataUrl = null;
  state.phone = '';
  // An explicit disconnect of a workspace's own number clears the claim so it
  // can be paired again later; the shared fallback is untouched.
  if (key !== SHARED_KEY) releaseOwnNumber(key);
  res.json({ status: 'disconnected' });
});

// ─── graceful shutdown ────────────────────────────────────────────────────────
async function shutdown() {
  console.log('Shutting down gracefully — destroying active sessions…');
  for (const [id, state] of sessions.entries()) {
    if (state.client) {
      try { await state.client.destroy(); } catch (e) {
        console.error(`[${id}] destroy error on shutdown:`, e.message);
      }
    }
  }
  process.exit(0);
}
process.on('SIGTERM', () => void shutdown());
process.on('SIGINT',  () => void shutdown());

// ─── restore saved sessions on boot ──────────────────────────────────────────
// Restarting the dispatcher must never require a new QR scan. Previously saved
// sessions are started here: the shared account (if it was ever paired) and any
// workspace that already claimed its own number. Workspace dirs without a claim
// are legacy leftovers and stay dormant on purpose — starting several browsers
// on the same account would make them fight over it.
function restoreSavedSessions() {
  if (savedAuthExists(SHARED_KEY)) {
    console.log('[shared] restoring saved session');
    createClient(SHARED_KEY);
  }
  let entries = [];
  try {
    entries = fs.readdirSync(SESSIONS_DIR);
  } catch (_) {
    return;
  }
  for (const entry of entries) {
    if (!entry.startsWith('session-')) continue;
    const key = entry.slice('session-'.length);
    if (!key || key === SHARED_KEY) continue;
    if (!isClaimed(key) || !savedAuthExists(key) || sessions.has(key)) continue;
    console.log(`[${key}] restoring saved session`);
    createClient(key);
  }
}

// ─── start ────────────────────────────────────────────────────────────────────
app.listen(PORT, () => {
  console.log(`WhatsApp Dispatcher listening on port ${PORT}`);
  restoreSavedSessions();
});
