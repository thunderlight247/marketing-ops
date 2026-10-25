// Workspace report client. Read-only fetch of
// the report endpoint with strict response parsing and explicit result kinds,
// mirroring lib/campaigns.ts. It does not compute anything itself: every
// figure comes straight from the server so numbers stay truthful.
//
// Time vocabulary, surfaced in the UI exactly as returned:
//   - "now" sections are current-state counts (the rows that exist today).
//   - "publishingRange" covers jobs created inside the requested range.

export interface ReportCounts {
  readonly [label: string]: number;
}

export interface PublishingFailure {
  jobId: string;
  itemId: string;
  integrationId: string;
  attempts: number;
  lastError: string;
  updatedAt: string;
}

export interface WorkspaceReport {
  workspaceId: string;
  generatedAt: string;
  range: { since: string | null; until: string | null };
  contentStatusNow: ReportCounts;
  campaignsNow: ReportCounts;
  leadsNow: ReportCounts;
  leadSourcesNow: ReportCounts;
  publishingRange: {
    jobsByStatus: ReportCounts;
    publishedCount: number;
    failedCount: number;
    recentFailures: PublishingFailure[];
  };
}

export type ReportState =
  | { kind: "loading" }
  | { kind: "loaded"; report: WorkspaceReport }
  | { kind: "unavailable"; message: string }
  | { kind: "missing"; message: string };

export type ReportQuery = { since?: string; until?: string };

// Human labels for statuses stored in the database. Unknown future statuses
// fall back to their raw stored value instead of being hidden.
const STATUS_LABELS: Record<string, string> = {
  // content_items
  draft: "Draft",
  review: "In review",
  approved: "Approved",
  rejected: "Rejected",
  queued: "Queued",
  publishing: "Publishing",
  published: "Published",
  failed: "Failed",
  // campaigns
  planned: "Planned",
  active: "Active",
  paused: "Paused",
  completed: "Completed",
  // leads (overlaps failed/published above only via statuses that differ)
  new: "New",
  contacted: "Contacted",
  qualified: "Qualified",
  won: "Won",
  lost: "Lost",
  // publishing jobs
  pending: "Pending",
  running: "Running",
  succeeded: "Succeeded",
  cancelled: "Cancelled",
};

export function formatStatusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status;
}

const SOURCE_LABELS: Record<string, string> = {
  manual: "Recorded manually",
  form_api: "Form / API",
  import: "Imported",
  unknown: "Source unknown",
};

export function formatLeadSourceLabel(kind: string): string {
  return SOURCE_LABELS[kind] ?? kind;
}

// validateReportQuery enforces the server-side contract before a bad request
// is ever sent.
export function validateReportQuery(query: ReportQuery): string[] {
  const problems: string[] = [];
  const since = query.since?.trim() ?? "";
  const until = query.until?.trim() ?? "";
  if (since && Number.isNaN(Date.parse(since))) {
    problems.push("from date is not a valid date");
  }
  if (until && Number.isNaN(Date.parse(until))) {
    problems.push("to date is not a valid date");
  }
  if (since && until && !Number.isNaN(Date.parse(since)) && !Number.isNaN(Date.parse(until))) {
    if (new Date(until).getTime() <= new Date(since).getTime()) {
      problems.push("to date must be after the from date");
    }
  }
  return problems;
}

function isCounts(value: unknown): value is ReportCounts {
  if (typeof value !== "object" || value === null) return false;
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    if (typeof v !== "number" || !Number.isFinite(v) || v < 0 || k === "") return false;
  }
  return true;
}

function optString(obj: Record<string, unknown>, key: string): string | null {
  const v = obj[key];
  return typeof v === "string" ? v : null;
}

function parseFailure(raw: unknown): PublishingFailure | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (
    typeof obj.jobId !== "string" ||
    typeof obj.itemId !== "string" ||
    typeof obj.integrationId !== "string" ||
    typeof obj.attempts !== "number" ||
    typeof obj.lastError !== "string"
  ) {
    return null;
  }
  return {
    jobId: obj.jobId,
    itemId: obj.itemId,
    integrationId: obj.integrationId,
    attempts: obj.attempts,
    lastError: obj.lastError,
    updatedAt: typeof obj.updatedAt === "string" ? obj.updatedAt : "",
  };
}

function parseReport(raw: unknown): WorkspaceReport | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (
    typeof obj.workspaceId !== "string" ||
    typeof obj.generatedAt !== "string" ||
    !isCounts(obj.contentStatusNow) ||
    !isCounts(obj.campaignsNow) ||
    !isCounts(obj.leadsNow) ||
    !isCounts(obj.leadSourcesNow)
  ) {
    return null;
  }
  const range = obj.range;
  if (typeof range !== "object" || range === null) return null;
  const rangeObj = range as Record<string, unknown>;

  const pr = obj.publishingRange;
  if (typeof pr !== "object" || pr === null) return null;
  const prObj = pr as Record<string, unknown>;
  if (
    !isCounts(prObj.jobsByStatus) ||
    typeof prObj.publishedCount !== "number" ||
    typeof prObj.failedCount !== "number"
  ) {
    return null;
  }
  const failuresRaw = prObj.recentFailures;
  if (!Array.isArray(failuresRaw)) return null;
  const failures: PublishingFailure[] = [];
  for (const raw2 of failuresRaw) {
    const parsed = parseFailure(raw2);
    if (parsed) failures.push(parsed);
  }

  return {
    workspaceId: obj.workspaceId,
    generatedAt: obj.generatedAt,
    range: {
      since: optString(rangeObj, "since"),
      until: optString(rangeObj, "until"),
    },
    contentStatusNow: obj.contentStatusNow,
    campaignsNow: obj.campaignsNow,
    leadsNow: obj.leadsNow,
    leadSourcesNow: obj.leadSourcesNow,
    publishingRange: {
      jobsByStatus: prObj.jobsByStatus,
      publishedCount: prObj.publishedCount,
      failedCount: prObj.failedCount,
      recentFailures: failures,
    },
  };
}

export async function fetchWorkspaceReport(
  baseUrl: string,
  workspaceId: string,
  query: ReportQuery = {},
  fetchImpl: typeof fetch = fetch,
): Promise<ReportState> {
  const problems = validateReportQuery(query);
  if (problems.length > 0) {
    return { kind: "unavailable", message: problems.join("; ") };
  }
  const root = baseUrl.replace(/\/+$/, "");
  const params = new URLSearchParams();
  if (query.since?.trim()) params.set("since", query.since.trim());
  if (query.until?.trim()) params.set("until", query.until.trim());
  const qs = params.toString();
  const url = `${root}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/reports${qs ? `?${qs}` : ""}`;

  try {
    const res = await fetchImpl(url, { headers: { Accept: "application/json" } });
    if (res.status === 404) {
      return { kind: "missing", message: "Workspace not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const parsed = parseReport(await res.json().catch(() => null));
    if (!parsed) {
      return { kind: "unavailable", message: "Server returned malformed report data." };
    }
    return { kind: "loaded", report: parsed };
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Network request failed.",
    };
  }
}
