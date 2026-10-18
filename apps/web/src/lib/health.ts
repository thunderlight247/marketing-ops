export type ApiStatus = "ok" | "degraded";
export type DatabaseStatus = "ok" | "unavailable" | "unconfigured";

export interface ApiHealth {
  status: ApiStatus;
  service: string;
  version: string;
  environment: string;
  checkedAt: string;
  database: {
    status: DatabaseStatus;
    latencyMs?: number;
    message: string;
  };
}

export type HealthState =
  | { kind: "loading" }
  | { kind: "unavailable"; reason: string }
  | { kind: "loaded"; health: ApiHealth };

const API_STATUSES: readonly string[] = ["ok", "degraded"];
const DATABASE_STATUSES: readonly string[] = ["ok", "unavailable", "unconfigured"];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Validates an unknown JSON value as an API health report. Returns null if it does not match. */
export function parseHealth(data: unknown): ApiHealth | null {
  if (!isRecord(data) || !isRecord(data.database)) return null;
  const db = data.database;

  if (typeof data.status !== "string" || !API_STATUSES.includes(data.status)) return null;
  if (typeof db.status !== "string" || !DATABASE_STATUSES.includes(db.status)) return null;
  if (typeof db.message !== "string") return null;
  for (const key of ["service", "version", "environment", "checkedAt"] as const) {
    if (typeof data[key] !== "string") return null;
  }
  if (db.latencyMs !== undefined && typeof db.latencyMs !== "number") return null;

  return {
    status: data.status as ApiStatus,
    service: data.service as string,
    version: data.version as string,
    environment: data.environment as string,
    checkedAt: data.checkedAt as string,
    database: {
      status: db.status as DatabaseStatus,
      latencyMs: db.latencyMs as number | undefined,
      message: db.message,
    },
  };
}

/**
 * Fetches GET {baseUrl}/api/v1/health. Never throws: any network failure, timeout,
 * non-200 response, or malformed body becomes an "unavailable" state.
 */
export async function fetchHealth(
  baseUrl: string,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 4000,
): Promise<HealthState> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${baseUrl}/api/v1/health`, {
      signal: controller.signal,
      cache: "no-store",
    });
    if (!response.ok) {
      return { kind: "unavailable", reason: `The API answered with HTTP ${response.status}.` };
    }
    const health = parseHealth(await response.json());
    if (!health) {
      return {
        kind: "unavailable",
        reason: "The API answered, but the response was not a health report.",
      };
    }
    return { kind: "loaded", health };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
}

export type Tone = "good" | "warn" | "bad";

export interface Display {
  label: string;
  tone: Tone;
}

export function describeDatabase(status: DatabaseStatus): Display {
  switch (status) {
    case "ok":
      return { label: "Connected", tone: "good" };
    case "unconfigured":
      return { label: "Not configured", tone: "warn" };
    case "unavailable":
      return { label: "Unavailable", tone: "bad" };
  }
}
