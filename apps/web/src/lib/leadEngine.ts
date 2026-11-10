import { withAuth } from "@/lib/config";

// ─── types ────────────────────────────────────────────────────────────────────

export interface LeadEngineConfig {
  workspaceId: string;
  enabled: boolean;
  hasGooglePlacesKey: boolean;
  hasAiProviderKey: boolean;
  aiModel: string;
  senderName: string;
  businessOffering: string;
  targetAudience: string;
  portfolioUrl: string;
  targetCategories: string[];
  targetCities: string[];
  dailyLimit: number;
  minDelayMs: number;
  maxDelayMs: number;
  whatsappStatus:
    "disconnected" | "qr_ready" | "connected" | "initializing" | "authenticated" | string;
  whatsappPhone: string;
  pitchTemplate: string;
  enableAutoFollowup?: boolean;
  followup1Template?: string;
  followup1DelayDays?: number;
  followup2Template?: string;
  followup2DelayDays?: number;
}

export interface LeadEngineConfigInput {
  enabled: boolean;
  googlePlacesApiKey?: string;
  aiProviderKey?: string;
  aiModel?: string;
  senderName?: string;
  businessOffering?: string;
  targetAudience?: string;
  portfolioUrl?: string;
  targetCategories?: string[];
  targetCities?: string[];
  dailyLimit?: number;
  minDelayMs?: number;
  maxDelayMs?: number;
  pitchTemplate?: string;
  enableAutoFollowup?: boolean;
  followup1Template?: string;
  followup1DelayDays?: number;
  followup2Template?: string;
  followup2DelayDays?: number;
}

export interface LeadEngineLead {
  id: string;
  name: string;
  phone: string;
  category: string;
  city: string;
  address: string;
  /**
   * Null when the discovery source has no ratings at all (OpenStreetMap), which
   * is different from being rated zero. Render "not rated", never 0 stars.
   */
  rating: number | null;
  reviewCount: number;
  /**
   * Where this lead was discovered: "google_places", "openstreetmap" or
   * "csv_import". Shown so an operator can weigh how much to trust a row.
   */
  sourceDetail?: string;
  websiteUrl: string;
  pitch: string;
  status: string;
  placeId: string | null;
  sequenceStep?: number;
  nextFollowupAt?: string | null;
  createdAt: string;
  /** Why the last dispatch skipped this lead (already contacted, opt-out, safety check, daily cap). */
  skipReason?: string;
  /**
   * Outcome of the lead's latest outbound attempt, from the send log.
   * "sent" = the dispatcher accepted the message; "failed" = it did not go out
   * (see lastSendError); "sending" = the outcome was never confirmed.
   */
  lastSendStatus?: "sent" | "failed" | "sending" | string;
  lastSendAt?: string;
  lastSendError?: string;
}

export interface OutreachLog {
  id: string;
  workspaceId: string;
  leadId: string;
  direction: string;
  messageId: string;
  body: string;
  status: string;
  ack: number | null;
  error: string;
  createdAt: string;
}

// Daily outreach activity row: one log entry enriched with the lead name/phone.
export interface OutreachActivity {
  id: string;
  workspaceId: string;
  leadId: string;
  direction: string;
  messageId?: string;
  body: string;
  status: string;
  ack?: number | null;
  error?: string;
  createdAt: string;
  leadName: string;
  leadPhone: string;
}

export interface BlacklistEntry {
  id: string;
  workspaceId: string;
  phoneNumber: string;
  reason: string;
  createdAt: string;
}

// One paired WhatsApp session: a workspace's own number, or the shared
// fallback account that workspaces without their own number still use.
export interface WhatsAppSession {
  workspaceId: string;
  workspaceName: string;
  source: "own" | "shared" | string;
  status: string;
  phone: string;
  connected: boolean;
}

// A dispatched background reconnect for a session whose pairing is still on
// disk, so the dashboard can show a countdown instead of a bare error.
export interface WhatsAppRetry {
  scheduled: boolean;
  attempt: number;
  retryInMs: number;
  reason?: string;
}

export type WhatsAppConnectResult = {
  source?: "own" | "shared";
  retry?: WhatsAppRetry;
  /** The number a pairing code was requested for, when one is pending. */
  pairingPhone?: string;
  /** The 8-character code WhatsApp issued; the QR remains usable too. */
  pairingCode?: string;
  pairingError?: string;
} & (
  | { status: "connected"; phone: string }
  | { status: "qr_ready"; qr: string }
  | { status: string; phone?: string; qr?: string }
);

// ─── helpers ──────────────────────────────────────────────────────────────────

async function apiFetch<T>(base: string, path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(`${base}${path}`, {
    ...init,
    headers: withAuth({ ...(init?.headers as Record<string, string>) }),
  });
  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    throw new Error(`${resp.status}: ${body || resp.statusText}`);
  }
  return resp.json() as Promise<T>;
}

function jsonPost<T>(base: string, path: string, payload?: unknown): Promise<T> {
  return apiFetch<T>(base, path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: payload !== undefined ? JSON.stringify(payload) : undefined,
  });
}

function jsonPut<T>(base: string, path: string, payload: unknown): Promise<T> {
  return apiFetch<T>(base, path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

// ─── config ───────────────────────────────────────────────────────────────────

export function getLeadEngineConfig(base: string, workspaceId: string): Promise<LeadEngineConfig> {
  return apiFetch<LeadEngineConfig>(base, `/api/v1/workspaces/${workspaceId}/lead-engine/config`);
}

export function saveLeadEngineConfig(
  base: string,
  workspaceId: string,
  config: LeadEngineConfigInput,
): Promise<LeadEngineConfig> {
  return jsonPut<LeadEngineConfig>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/config`,
    config,
  );
}

// ─── WhatsApp ─────────────────────────────────────────────────────────────────

export function connectWhatsApp(base: string, workspaceId: string): Promise<WhatsAppConnectResult> {
  return jsonPost<WhatsAppConnectResult>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/connect`,
  );
}

// Links this workspace's OWN number (its own persistent session) instead of
// the shared account it may have been borrowing. One-time QR scan, restored on
// every later start.
export function connectOwnWhatsApp(
  base: string,
  workspaceId: string,
): Promise<WhatsAppConnectResult> {
  return jsonPost<WhatsAppConnectResult>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/connect?own=1`,
  );
}

// Pair via a code WhatsApp sends to the phone number instead of scanning the
// QR. The code and the QR authenticate the same saved session, so either can
// be used; the dashboard shows both while pairing.
export function pairOwnWhatsApp(
  base: string,
  workspaceId: string,
  phone: string,
): Promise<WhatsAppConnectResult> {
  return jsonPost<WhatsAppConnectResult>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/pair?own=1`,
    { phone },
  );
}

export function disconnectWhatsApp(base: string, workspaceId: string): Promise<unknown> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/disconnect`);
}

// Per-workspace outreach backlog, for the "which workspace is sending" rail.
export interface LeadEngineProgress {
  workspaceId: string;
  queued: number;
  inFlight: number;
  lastOutboundAt?: string;
}

export function listLeadEngineProgress(base: string): Promise<{ progress: LeadEngineProgress[] }> {
  return apiFetch(base, `/api/v1/lead-engine/progress`);
}

// Audit of every paired session: which workspace sends from which number.
export function listWhatsAppSessions(base: string): Promise<{ sessions: WhatsAppSession[] }> {
  return apiFetch(base, `/api/v1/lead-engine/whatsapp/sessions`);
}

export function getWhatsAppStatus(
  base: string,
  workspaceId: string,
): Promise<WhatsAppConnectResult> {
  return apiFetch<WhatsAppConnectResult>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/status`,
    // The QR rotates and a cached copy would pair an expired code.
    { cache: "no-store" },
  );
}

// ─── discovery / dispatch ─────────────────────────────────────────────────────

export function triggerDiscovery(base: string, workspaceId: string): Promise<{ status: string }> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/discover`);
}

export function triggerDispatch(base: string, workspaceId: string): Promise<{ status: string }> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/dispatch`);
}

// ─── follow-up ────────────────────────────────────────────────────────────────

// One contact this workspace has ALREADY messaged successfully — the
// selectable pool for follow-up rounds.
export interface MessagedContact {
  id: string;
  name: string;
  phone: string;
  category: string;
  city: string;
  lastSentAt: string;
  blacklisted: boolean;
}

export function listMessagedContacts(
  base: string,
  workspaceId: string,
): Promise<MessagedContact[]> {
  return apiFetch<MessagedContact[]>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/contacts`,
  );
}

/** Starts a background follow-up run for the selected contact IDs. */
export function sendFollowUp(
  base: string,
  workspaceId: string,
  message: string,
  leadIds: string[],
): Promise<{ status?: string; total?: number }> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/followup`, {
    message,
    leadIds,
  });
}

// ─── leads ────────────────────────────────────────────────────────────────────

export function listLeadEngineLeads(base: string, workspaceId: string): Promise<LeadEngineLead[]> {
  return apiFetch<LeadEngineLead[]>(base, `/api/v1/workspaces/${workspaceId}/lead-engine/leads`);
}

// ─── logs ─────────────────────────────────────────────────────────────────────

export function listOutreachLogs(
  base: string,
  workspaceId: string,
  leadId: string,
): Promise<OutreachLog[]> {
  return apiFetch<OutreachLog[]>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/leads/${leadId}/logs`,
  );
}

// ─── daily activity ───────────────────────────────────────────────────────

// days=0 → today only (each day starts empty); days=N → last N days.
export function listDailyOutreach(
  base: string,
  workspaceId: string,
  days = 0,
): Promise<OutreachActivity[]> {
  return apiFetch<OutreachActivity[]>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/outreach?days=${days}`,
  );
}

// ─── blacklist ────────────────────────────────────────────────────────────────

export function listBlacklist(base: string, workspaceId: string): Promise<BlacklistEntry[]> {
  return apiFetch<BlacklistEntry[]>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/blacklist`,
  );
}

export function addToBlacklist(
  base: string,
  workspaceId: string,
  phone: string,
  reason: string,
): Promise<unknown> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/blacklist`, {
    phone,
    reason,
  });
}

export interface ImportLeadsResult {
  total: number;
  imported: number;
  duplicates: number;
  failed: number;
}

export function importLeadEngineLeads(
  base: string,
  workspaceId: string,
  leads: Array<{
    name: string;
    phone: string;
    category?: string;
    city?: string;
    address?: string;
    websiteUrl?: string;
  }>,
): Promise<ImportLeadsResult> {
  return jsonPost<ImportLeadsResult>(base, `/api/v1/workspaces/${workspaceId}/lead-engine/import`, {
    leads,
  });
}
