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
  whatsappStatus: "disconnected" | "qr_ready" | "connected" | "initializing" | "authenticated" | string;
  whatsappPhone: string;
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
}

export interface LeadEngineLead {
  id: string;
  name: string;
  phone: string;
  category: string;
  city: string;
  address: string;
  rating: number | null;
  reviewCount: number;
  websiteUrl: string;
  pitch: string;
  status: string;
  placeId: string | null;
  createdAt: string;
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

export interface BlacklistEntry {
  id: string;
  workspaceId: string;
  phoneNumber: string;
  reason: string;
  createdAt: string;
}

export type WhatsAppConnectResult =
  | { status: "connected"; phone: string }
  | { status: "qr_ready"; qr: string }
  | { status: string; phone?: string; qr?: string };

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

export function disconnectWhatsApp(base: string, workspaceId: string): Promise<unknown> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/disconnect`);
}

export function getWhatsAppStatus(
  base: string,
  workspaceId: string,
): Promise<WhatsAppConnectResult> {
  return apiFetch<WhatsAppConnectResult>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/whatsapp/status`,
  );
}

// ─── discovery / dispatch ─────────────────────────────────────────────────────

export function triggerDiscovery(base: string, workspaceId: string): Promise<{ status: string }> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/discover`);
}

export function triggerDispatch(base: string, workspaceId: string): Promise<{ status: string }> {
  return jsonPost(base, `/api/v1/workspaces/${workspaceId}/lead-engine/dispatch`);
}

// ─── leads ────────────────────────────────────────────────────────────────────

export function listLeadEngineLeads(
  base: string,
  workspaceId: string,
): Promise<LeadEngineLead[]> {
  return apiFetch<LeadEngineLead[]>(
    base,
    `/api/v1/workspaces/${workspaceId}/lead-engine/leads`,
  );
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

// ─── blacklist ────────────────────────────────────────────────────────────────

export function listBlacklist(base: string, workspaceId: string): Promise<BlacklistEntry[]> {
  return apiFetch<BlacklistEntry[]>(base, `/api/v1/workspaces/${workspaceId}/lead-engine/blacklist`);
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
