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

const app = express();
app.use(express.json());

const PORT = Number(process.env.PORT) || 8083;
const EXEC_PATH = process.env.CHROMIUM_PATH || '/usr/bin/chromium';
const SESSIONS_DIR = './data/sessions';
const INTERNAL_SECRET = process.env.DISPATCHER_SECRET || '';
const API_BASE_URL = process.env.API_BASE_URL || 'http://localhost:8081';

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

/** @typedef {{ client: Client|null, status: string, qrDataUrl: string|null, phone: string }} SessionState */

function makeState() {
  return { client: null, status: 'disconnected', qrDataUrl: null, phone: '' };
}

function getState(workspaceId) {
  if (!sessions.has(workspaceId)) sessions.set(workspaceId, makeState());
  return sessions.get(workspaceId);
}

/**
 * Create and wire up a WhatsApp client for a workspace.
 * If the session already has persisted auth (LocalAuth), it reconnects without a QR.
 */
function createClient(workspaceId) {
  const client = new Client({
    authStrategy: new LocalAuth({
      clientId: workspaceId,
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

  const state = getState(workspaceId);
  state.client = client;
  state.status = 'initializing';

  client.on('qr', async (qr) => {
    try {
      state.qrDataUrl = await qrcode.toDataURL(qr);
      state.status = 'qr_ready';
      console.log(`[${workspaceId}] QR ready.`);
    } catch (err) {
      console.error(`[${workspaceId}] QR encode error:`, err.message);
    }
  });

  client.on('authenticated', () => {
    state.status = 'authenticated';
    state.qrDataUrl = null;
    console.log(`[${workspaceId}] Authenticated.`);
  });

  client.on('ready', async () => {
    state.status = 'connected';
    try {
      const info = client.info;
      state.phone = info?.wid?.user ? '+' + info.wid.user : '';
    } catch (_) {}
    console.log(`[${workspaceId}] Connected. Phone: ${state.phone}`);
  });

  client.on('disconnected', (reason) => {
    state.status = 'disconnected';
    state.qrDataUrl = null;
    state.phone = '';
    state.client = null;
    console.log(`[${workspaceId}] Disconnected:`, reason);
  });

  client.on('auth_failure', (msg) => {
    state.status = 'auth_failure';
    console.error(`[${workspaceId}] Auth failure:`, msg);
  });

  client.on('message', async (msg) => {
    // Ignore outbound messages sent by us
    if (msg.fromMe) return;

    // Parse sender's phone number
    const rawNumber = (msg.from || '').replace('@c.us', '').replace(/[^0-9]/g, '');
    const phone = rawNumber ? '+' + rawNumber : '';
    const body = msg.body;
    if (!phone || !body) return;

    console.log(`[${workspaceId}] Inbound WhatsApp message from ${phone}: ${body.slice(0, 60)}`);

    try {
      const webhookUrl = `${API_BASE_URL}/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/inbound`;
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
      console.error(`[${workspaceId}] Failed to forward inbound message:`, err.message);
    }
  });

  client.initialize().catch((err) => {
    state.status = 'error';
    console.error(`[${workspaceId}] Init error:`, err.message);
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
  const state = getState(workspaceId);

  if (state.status === 'connected') {
    return res.json({ status: 'connected', phone: state.phone });
  }
  if (state.status === 'qr_ready') {
    return res.json({ status: 'qr_ready', qr: state.qrDataUrl });
  }
  if (state.status === 'initializing' || state.status === 'authenticated') {
    return res.json({ status: state.status });
  }

  // Not started — create a new client.
  createClient(workspaceId);

  // Wait briefly for QR or connected status to surface.
  await new Promise((r) => setTimeout(r, 3000));

  const fresh = getState(workspaceId);
  if (fresh.status === 'connected') {
    return res.json({ status: 'connected', phone: fresh.phone });
  }
  if (fresh.status === 'qr_ready') {
    return res.json({ status: 'qr_ready', qr: fresh.qrDataUrl });
  }
  return res.json({ status: fresh.status });
});

/**
 * GET /sessions/:workspaceId/status
 */
app.get('/sessions/:workspaceId/status', (req, res) => {
  const { workspaceId } = req.params;
  const state = getState(workspaceId);
  const out = { status: state.status, phone: state.phone };
  if (state.status === 'qr_ready') out.qr = state.qrDataUrl;
  res.json(out);
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

  const state = getState(workspaceId);
  if (state.status !== 'connected' || !state.client) {
    return res.status(409).json({ success: false, error: 'WhatsApp not connected for this workspace' });
  }

  try {
    // Normalize phone: strip leading + and any spaces, append @c.us
    const normalized = phone.replace(/^\+/, '').replace(/\s/g, '');
    const chatId = `${normalized}@c.us`;
    const result = await state.client.sendMessage(chatId, message);
    return res.json({ success: true, messageId: result.id?.id || null });
  } catch (err) {
    console.error(`[${workspaceId}] Send error:`, err.message);
    return res.status(500).json({ success: false, error: err.message });
  }
});

/**
 * POST /sessions/:workspaceId/disconnect
 * Destroys the WhatsApp session and clears auth.
 */
app.post('/sessions/:workspaceId/disconnect', async (req, res) => {
  const { workspaceId } = req.params;
  const state = getState(workspaceId);
  if (state.client) {
    try {
      await state.client.destroy();
    } catch (_) {}
    state.client = null;
  }
  state.status = 'disconnected';
  state.qrDataUrl = null;
  state.phone = '';
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

// ─── start ────────────────────────────────────────────────────────────────────
app.listen(PORT, () => {
  console.log(`WhatsApp Dispatcher listening on port ${PORT}`);
});
