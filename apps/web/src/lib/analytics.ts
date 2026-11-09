import { withAuth } from "@/lib/config";

// The analytics summary endpoint returns pre-aggregated counts. The dashboard
// never downloads lead rows just to count them, so its cost stays flat as the
// tables grow.

export interface AnalyticsTotals {
  workspaces: number;
  campaigns: number;
  crmLeads: number;
  crmWon: number;
  engineLeads: number;
  engineSent: number;
  engineReplied: number;
  engineFailed: number;
  engineBlacklisted: number;
  emailsTotal: number;
  emailsSent: number;
  emailsDelivered: number;
}

export interface WorkspaceAnalytics {
  workspaceId: string;
  slug: string;
  name: string;
  campaigns: number;
  crmLeads: number;
  crmWon: number;
  engineLeads: number;
  engineSent: number;
  engineReplied: number;
  emailsTotal: number;
  emailsSent: number;
  emailsDelivered: number;
}

export interface AnalyticsSummary {
  generatedAt: string;
  totals: AnalyticsTotals;
  byWorkspace: WorkspaceAnalytics[];
  crmLeadsByStatus: Record<string, number>;
  engineLeadsByStatus: Record<string, number>;
}

export type AnalyticsSummaryState =
  | { kind: "loading" }
  | { kind: "loaded"; summary: AnalyticsSummary }
  | { kind: "unavailable"; message: string };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function count(value: unknown): number | null {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : null;
}

function parseCounts(value: unknown): Record<string, number> | null {
  if (!isRecord(value)) return null;
  const out: Record<string, number> = {};
  for (const [label, raw] of Object.entries(value)) {
    const n = count(raw);
    if (n === null) return null;
    out[label] = n;
  }
  return out;
}

const TOTAL_KEYS = [
  "workspaces",
  "campaigns",
  "crmLeads",
  "crmWon",
  "engineLeads",
  "engineSent",
  "engineReplied",
  "engineFailed",
  "engineBlacklisted",
  "emailsTotal",
  "emailsSent",
  "emailsDelivered",
] as const;

const WORKSPACE_KEYS = [
  "campaigns",
  "crmLeads",
  "crmWon",
  "engineLeads",
  "engineSent",
  "engineReplied",
  "emailsTotal",
  "emailsSent",
  "emailsDelivered",
] as const;

function parseTotals(raw: unknown): AnalyticsTotals | null {
  if (!isRecord(raw)) return null;
  const out = {} as AnalyticsTotals;
  for (const key of TOTAL_KEYS) {
    const n = count(raw[key]);
    if (n === null) return null;
    out[key] = n;
  }
  return out;
}

function parseWorkspace(raw: unknown): WorkspaceAnalytics | null {
  if (!isRecord(raw)) return null;
  if (
    typeof raw.workspaceId !== "string" ||
    raw.workspaceId === "" ||
    typeof raw.slug !== "string" ||
    typeof raw.name !== "string"
  ) {
    return null;
  }
  const out: Partial<WorkspaceAnalytics> = {
    workspaceId: raw.workspaceId,
    slug: raw.slug,
    name: raw.name,
  };
  for (const key of WORKSPACE_KEYS) {
    const n = count(raw[key]);
    if (n === null) return null;
    out[key] = n;
  }
  return out as WorkspaceAnalytics;
}

/** Validates the API payload. Returns null when any required count is missing. */
export function parseAnalyticsSummary(raw: unknown): AnalyticsSummary | null {
  if (!isRecord(raw)) return null;
  if (typeof raw.generatedAt !== "string" || raw.generatedAt === "") return null;
  const totals = parseTotals(raw.totals);
  if (!totals) return null;
  if (!Array.isArray(raw.byWorkspace)) return null;
  const byWorkspace: WorkspaceAnalytics[] = [];
  for (const item of raw.byWorkspace) {
    const workspace = parseWorkspace(item);
    if (!workspace) return null;
    byWorkspace.push(workspace);
  }
  const crmLeadsByStatus = parseCounts(raw.crmLeadsByStatus);
  const engineLeadsByStatus = parseCounts(raw.engineLeadsByStatus);
  if (!crmLeadsByStatus || !engineLeadsByStatus) return null;
  return {
    generatedAt: raw.generatedAt,
    totals,
    byWorkspace,
    crmLeadsByStatus,
    engineLeadsByStatus,
  };
}

export interface DailyLeadPoint {
  day: string;
  crm: number;
  engine: number;
}

export interface UTMCounts {
  source: Record<string, number>;
  medium: Record<string, number>;
  campaign: Record<string, number>;
}

export interface AnalyticsBreakdown {
  trend: DailyLeadPoint[];
  utm: UTMCounts;
}

export type AnalyticsBreakdownState =
  | { kind: "loading" }
  | { kind: "loaded"; breakdown: AnalyticsBreakdown }
  | { kind: "unavailable"; message: string };

/** Validates the breakdown payload. Returns null when any count is malformed. */
export function parseAnalyticsBreakdown(raw: unknown): AnalyticsBreakdown | null {
  if (!isRecord(raw) || !Array.isArray(raw.trend) || !isRecord(raw.utm)) return null;
  const trend: DailyLeadPoint[] = [];
  for (const item of raw.trend) {
    if (!isRecord(item) || typeof item.day !== "string") return null;
    const crm = count(item.crm);
    const engine = count(item.engine);
    if (crm === null || engine === null) return null;
    trend.push({ day: item.day, crm, engine });
  }
  const source = parseCounts(raw.utm.source);
  const medium = parseCounts(raw.utm.medium);
  const campaign = parseCounts(raw.utm.campaign);
  if (!source || !medium || !campaign) return null;
  return { trend, utm: { source, medium, campaign } };
}

/**
 * Fetches the daily trend and UTM breakdown for leads created between two
 * YYYY-MM-DD dates (inclusive). The API caps the range at 366 days.
 */
export async function fetchAnalyticsBreakdown(
  baseUrl: string,
  range: { from: string; to: string; workspaceId?: string },
  fetchImpl: typeof fetch = fetch,
): Promise<AnalyticsBreakdownState> {
  const root = baseUrl.replace(/\/+$/, "");
  const params = new URLSearchParams({ from: range.from, to: range.to });
  if (range.workspaceId) params.set("workspaceId", range.workspaceId);
  try {
    const res = await fetchImpl(`${root}/api/v1/analytics/breakdown?${params.toString()}`, {
      headers: withAuth({ Accept: "application/json" }),
    });
    const body = (await res.json().catch(() => null)) as unknown;
    if (!res.ok) {
      const message = isRecord(body) && typeof body.error === "string" ? body.error : null;
      return {
        kind: "unavailable",
        message: message ?? `The API answered with status ${res.status}.`,
      };
    }
    const breakdown = parseAnalyticsBreakdown(body);
    if (!breakdown) {
      return { kind: "unavailable", message: "The breakdown response had an unexpected shape." };
    }
    return { kind: "loaded", breakdown };
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Could not reach the API.",
    };
  }
}

/**
 * Fetches the summary in one request. Pass a workspace id to scope every count
 * to that workspace; omit it for the platform-wide view.
 */
export async function fetchAnalyticsSummary(
  baseUrl: string,
  workspaceId = "",
  fetchImpl: typeof fetch = fetch,
): Promise<AnalyticsSummaryState> {
  const root = baseUrl.replace(/\/+$/, "");
  const query = workspaceId ? `?workspaceId=${encodeURIComponent(workspaceId)}` : "";
  try {
    const res = await fetchImpl(`${root}/api/v1/analytics/summary${query}`, {
      headers: withAuth({ Accept: "application/json" }),
    });
    const body = (await res.json().catch(() => null)) as unknown;
    if (!res.ok) {
      const message = isRecord(body) && typeof body.error === "string" ? body.error : null;
      return {
        kind: "unavailable",
        message: message ?? `The API answered with status ${res.status}.`,
      };
    }
    const summary = parseAnalyticsSummary(body);
    if (!summary) {
      return { kind: "unavailable", message: "The analytics response had an unexpected shape." };
    }
    return { kind: "loaded", summary };
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Could not reach the API.",
    };
  }
}
