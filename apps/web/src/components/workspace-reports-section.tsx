"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchWorkspaceAssessment,
  fetchWorkspaceReport,
  formatLeadSourceLabel,
  formatStatusLabel,
  type AssessmentFinding,
  type AssessmentState,
  type ReportCounts,
  type ReportState,
  type WorkspaceReport,
} from "@/lib/reports";

const inputClasses =
  "rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";

// ─── Lead pipeline funnel ─────────────────────────────────────────────────────

const FUNNEL_STAGES = ["new", "contacted", "qualified", "won"] as const;
const FUNNEL_COLORS: Record<string, { bar: string; badge: string; text: string }> = {
  new: { bar: "bg-blue-400", badge: "bg-blue-50 text-blue-800", text: "text-blue-700" },
  contacted: { bar: "bg-amber-400", badge: "bg-amber-50 text-amber-800", text: "text-amber-700" },
  qualified: {
    bar: "bg-indigo-400",
    badge: "bg-indigo-50 text-indigo-800",
    text: "text-indigo-700",
  },
  won: { bar: "bg-emerald-500", badge: "bg-emerald-50 text-emerald-800", text: "text-emerald-700" },
  lost: { bar: "bg-slate-300", badge: "bg-slate-100 text-slate-600", text: "text-slate-500" },
};

const FUNNEL_LABELS: Record<string, string> = {
  new: "New",
  contacted: "Contacted",
  qualified: "Qualified",
  won: "Won",
};

function pct(num: number, denom: number): string {
  if (denom === 0) return "—";
  return `${Math.round((num / denom) * 100)}%`;
}

function LeadFunnel({ counts }: { counts: ReportCounts }) {
  const total = Object.values(counts).reduce((s, n) => s + n, 0);
  const topOfFunnel = counts["new"] ?? 0;
  const won = counts["won"] ?? 0;
  const lost = counts["lost"] ?? 0;

  if (total === 0) {
    return <p className="text-sm text-slate-500">No leads recorded yet.</p>;
  }

  return (
    <div className="space-y-3">
      {/* Visual funnel bars */}
      {FUNNEL_STAGES.map((stage, i) => {
        const n = counts[stage] ?? 0;
        const prev = i === 0 ? total : (counts[FUNNEL_STAGES[i - 1]] ?? 0);
        const widthPct = topOfFunnel > 0 ? Math.round((n / (topOfFunnel || 1)) * 100) : 0;
        const conversionFromPrev = pct(n, prev);
        const colors = FUNNEL_COLORS[stage];
        return (
          <div key={stage}>
            <div className="mb-1 flex items-baseline justify-between gap-2">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-slate-700">{FUNNEL_LABELS[stage]}</span>
                {i > 0 && (
                  <span className="text-[11px] text-slate-400">
                    ↳ {conversionFromPrev} from previous
                  </span>
                )}
              </div>
              <span className={`text-sm font-bold tabular-nums ${colors.text}`}>{n}</span>
            </div>
            <div className="h-3 w-full overflow-hidden rounded-full bg-slate-100">
              <div
                className={`h-3 rounded-full ${colors.bar} transition-all duration-500`}
                style={{ width: `${widthPct}%` }}
              />
            </div>
          </div>
        );
      })}

      {/* Summary row */}
      <div className="mt-4 flex flex-wrap gap-3 border-t border-slate-100 pt-3 text-xs">
        <span className="font-medium text-slate-600">
          Total: <span className="font-bold text-slate-900">{total}</span>
        </span>
        <span className="font-medium text-slate-600">
          Win rate: <span className="font-bold text-emerald-700">{pct(won, total - lost)}</span>
        </span>
        {lost > 0 && (
          <span className="font-medium text-slate-600">
            Lost: <span className="font-bold text-slate-500">{lost}</span>
          </span>
        )}
        <span className="font-medium text-slate-600">
          Top→Won:{" "}
          <span className="font-bold text-emerald-700">{pct(won, topOfFunnel || total)}</span>
        </span>
      </div>
    </div>
  );
}

// ─── Content pipeline ─────────────────────────────────────────────────────────

const CONTENT_STATUS_ORDER = [
  "draft",
  "review",
  "approved",
  "queued",
  "publishing",
  "published",
  "rejected",
  "failed",
];
const CONTENT_STATUS_COLOR: Record<string, string> = {
  draft: "bg-slate-300",
  review: "bg-amber-400",
  approved: "bg-sky-400",
  queued: "bg-indigo-300",
  publishing: "bg-indigo-400",
  published: "bg-emerald-500",
  rejected: "bg-red-300",
  failed: "bg-red-500",
};

function ContentPipeline({ counts }: { counts: ReportCounts }) {
  const total = Object.values(counts).reduce((s, n) => s + n, 0);
  if (total === 0) {
    return <p className="text-sm text-slate-500">No content items yet.</p>;
  }
  const sorted = CONTENT_STATUS_ORDER.filter((s) => s in counts).concat(
    Object.keys(counts).filter((s) => !CONTENT_STATUS_ORDER.includes(s)),
  );
  return (
    <div className="space-y-2.5">
      {sorted.map((status) => {
        const n = counts[status] ?? 0;
        const widthPct = Math.round((n / total) * 100);
        const color = CONTENT_STATUS_COLOR[status] ?? "bg-slate-300";
        return (
          <div key={status}>
            <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
              <span className="text-slate-700">{formatStatusLabel(status)}</span>
              <span className="font-semibold tabular-nums text-slate-800">
                {n} <span className="font-normal text-slate-400">({widthPct}%)</span>
              </span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-slate-100">
              <div
                className={`h-2 rounded-full ${color} transition-all duration-500`}
                style={{ width: `${widthPct}%` }}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ─── Lead sources ─────────────────────────────────────────────────────────────

function LeadSources({ counts }: { counts: ReportCounts }) {
  const entries = Object.entries(counts);
  if (entries.length === 0) {
    return <p className="text-sm text-slate-500">No lead sources recorded yet.</p>;
  }
  const max = Math.max(...entries.map(([, n]) => n), 1);
  return (
    <div className="space-y-2.5">
      {entries
        .sort(([, a], [, b]) => b - a)
        .map(([label, count]) => (
          <div key={label}>
            <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
              <span className="text-slate-700">{formatLeadSourceLabel(label)}</span>
              <span className="font-semibold tabular-nums text-slate-800">{count}</span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-slate-100">
              <div
                className="h-2 rounded-full bg-sky-400 transition-all duration-500"
                style={{ width: `${Math.round((count / max) * 100)}%` }}
              />
            </div>
          </div>
        ))}
    </div>
  );
}

// ─── Publishing jobs section ──────────────────────────────────────────────────

function PublishingJobs({ report }: { report: WorkspaceReport }) {
  const { jobsByStatus, publishedCount, failedCount, recentFailures } = report.publishingRange;
  const totalJobs = Object.values(jobsByStatus).reduce((s, n) => s + n, 0);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-3">
        <div className="rounded-lg border border-slate-100 bg-slate-50 px-4 py-3 text-center">
          <p className="text-2xl font-bold text-emerald-700">{publishedCount}</p>
          <p className="text-xs text-slate-500">Published</p>
        </div>
        <div className="rounded-lg border border-slate-100 bg-slate-50 px-4 py-3 text-center">
          <p className="text-2xl font-bold text-red-600">{failedCount}</p>
          <p className="text-xs text-slate-500">Failed</p>
        </div>
        <div className="rounded-lg border border-slate-100 bg-slate-50 px-4 py-3 text-center">
          <p className="text-2xl font-bold text-slate-800">{totalJobs}</p>
          <p className="text-xs text-slate-500">Total jobs</p>
        </div>
        {totalJobs > 0 && (
          <div className="rounded-lg border border-slate-100 bg-slate-50 px-4 py-3 text-center">
            <p className="text-2xl font-bold text-indigo-700">{pct(publishedCount, totalJobs)}</p>
            <p className="text-xs text-slate-500">Success rate</p>
          </div>
        )}
      </div>

      {recentFailures.length > 0 && (
        <div>
          <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-red-600">
            Recent failures
          </p>
          <ul className="space-y-2">
            {recentFailures.map((f) => (
              <li
                key={f.jobId}
                className="rounded-lg border border-red-100 bg-red-50 px-3 py-2.5 text-sm"
              >
                <div className="flex flex-wrap items-center gap-2 text-xs">
                  <span className="font-mono text-slate-600">job {f.jobId.slice(0, 8)}</span>
                  <span className="text-slate-500">
                    {f.attempts} attempt{f.attempts === 1 ? "" : "s"}
                  </span>
                  <span className="text-slate-500">
                    {new Date(f.updatedAt).toISOString().replace("T", " ").slice(0, 16)} UTC
                  </span>
                </div>
                <p className="mt-1 text-xs text-red-700">{f.lastError}</p>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

// ─── Campaigns mini-chart ─────────────────────────────────────────────────────

const CAMPAIGN_COLOR: Record<string, string> = {
  planned: "bg-slate-300",
  active: "bg-emerald-400",
  paused: "bg-amber-400",
  completed: "bg-indigo-400",
};

function CampaignStatus({ counts }: { counts: ReportCounts }) {
  const entries = Object.entries(counts);
  if (entries.length === 0) return <p className="text-sm text-slate-500">No campaigns yet.</p>;
  const max = Math.max(...entries.map(([, n]) => n), 1);
  return (
    <div className="space-y-2.5">
      {entries
        .sort(([, a], [, b]) => b - a)
        .map(([status, n]) => (
          <div key={status}>
            <div className="mb-1 flex items-baseline justify-between gap-2 text-xs">
              <span className="text-slate-700">{formatStatusLabel(status)}</span>
              <span className="font-semibold tabular-nums text-slate-800">{n}</span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-slate-100">
              <div
                className={`h-2 rounded-full ${CAMPAIGN_COLOR[status] ?? "bg-slate-300"} transition-all duration-500`}
                style={{ width: `${Math.round((n / max) * 100)}%` }}
              />
            </div>
          </div>
        ))}
    </div>
  );
}

// ─── Main section ─────────────────────────────────────────────────────────────

const SOURCE_LABELS: Record<AssessmentFinding["source"], string> = {
  provided: "Provided",
  missing: "Missing",
  derived: "Derived",
};

function AssessmentPanel({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const [state, setState] = useState<AssessmentState>({ kind: "loading" });
  const apiBaseUrl = getApiBaseUrl();

  useEffect(() => {
    let active = true;
    void fetchWorkspaceAssessment(apiBaseUrl, workspaceId).then((next) => {
      if (active) setState(next);
    });
    return () => {
      active = false;
    };
  }, [apiBaseUrl, workspaceId]);

  return (
    <div className="mb-6 rounded-xl border border-slate-100 bg-white px-5 py-4 shadow-sm">
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-widest text-slate-500">
        Business Assessment
      </h3>
      <p className="mb-4 max-w-prose text-xs text-slate-500">
        Prioritized gaps from your stored brand profile and workspace data. Each item shows whether
        it was provided, is missing, or was derived from stored records. Nothing is estimated.
      </p>
      {state.kind === "loading" && <p className="text-sm text-slate-500">Assessing…</p>}
      {state.kind === "unavailable" && (
        <p role="alert" className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {state.message}
        </p>
      )}
      {state.kind === "missing" && (
        <p className="text-sm text-slate-500">This workspace no longer exists.</p>
      )}
      {state.kind === "loaded" && (
        <ul className="divide-y divide-slate-100">
          {state.findings.map((f) => (
            <li key={`${f.area}-${f.priority}`} className="py-3">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-medium text-slate-800">{f.area}</span>
                <span className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-600">
                  {SOURCE_LABELS[f.source]}
                </span>
                <span
                  className={`rounded px-1.5 py-0.5 text-xs ${
                    f.status === "attention"
                      ? "bg-amber-50 text-amber-800"
                      : f.status === "ok"
                        ? "bg-emerald-50 text-emerald-800"
                        : "bg-slate-100 text-slate-600"
                  }`}
                >
                  {f.status === "attention"
                    ? "Needs attention"
                    : f.status === "ok"
                      ? "OK"
                      : "Unknown"}
                </span>
              </div>
              <p className="mt-1 text-sm text-slate-600">{f.summary}</p>
              {f.recommendation && (
                <p className="mt-1 text-sm text-slate-800">Next: {f.recommendation}</p>
              )}
              {f.actionSegment && (
                <Link
                  href={`/workspaces/${workspaceId}${f.actionSegment}`}
                  className="mt-1 inline-block text-xs font-semibold text-indigo-700 hover:underline"
                >
                  Open {f.area.toLowerCase()} →
                </Link>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function WorkspaceReportsSection({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const [state, setState] = useState<ReportState>({ kind: "loading" });
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");
  const [applying, setApplying] = useState(false);
  const [applyError, setApplyError] = useState<string | null>(null);

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(
    async (query: { since?: string; until?: string }) => {
      setState({ kind: "loading" });
      setState(await fetchWorkspaceReport(apiBaseUrl, workspaceId, query));
    },
    [apiBaseUrl, workspaceId],
  );

  useEffect(() => {
    void load({});
  }, [load]);

  function handleApply(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setApplyError(null);
    const problems: string[] = [];
    if (since && Number.isNaN(Date.parse(since))) problems.push("From date is not valid");
    if (until && Number.isNaN(Date.parse(until))) problems.push("To date is not valid");
    if (problems.length === 0 && since && until && Date.parse(until) <= Date.parse(since)) {
      problems.push("To date must be after the from date");
    }
    if (problems.length > 0) {
      setApplyError(problems.join("; "));
      return;
    }
    setApplying(true);
    void load({ since: since || undefined, until: until || undefined }).finally(() =>
      setApplying(false),
    );
  }

  const report = state.kind === "loaded" ? state.report : null;

  return (
    <section
      aria-labelledby="workspace-reports-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      {/* Header + date range filter */}
      <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2
              id="workspace-reports-heading"
              className="text-xs font-semibold uppercase tracking-widest text-slate-500"
            >
              Reports &amp; Funnel
            </h2>
            {report && (
              <p className="mt-0.5 text-xs text-slate-400">
                Generated{" "}
                {new Date(report.generatedAt).toISOString().replace("T", " ").slice(0, 16)} UTC
                {report.range.since || report.range.until ? " · custom range" : " · full history"}
              </p>
            )}
          </div>
          <form onSubmit={handleApply} className="flex flex-wrap items-center gap-2">
            <label htmlFor="report-since" className="text-xs font-medium text-slate-600">
              From
            </label>
            <input
              id="report-since"
              type="date"
              value={since}
              onChange={(e) => setSince(e.target.value)}
              className={`${inputClasses} max-w-[9rem] text-xs`}
            />
            <label htmlFor="report-until" className="text-xs font-medium text-slate-600">
              to
            </label>
            <input
              id="report-until"
              type="date"
              value={until}
              onChange={(e) => setUntil(e.target.value)}
              className={`${inputClasses} max-w-[9rem] text-xs`}
            />
            <button
              type="submit"
              disabled={applying}
              className="rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-60"
            >
              {applying ? "Updating…" : "Apply"}
            </button>
            {(since || until) && (
              <button
                type="button"
                onClick={() => {
                  setSince("");
                  setUntil("");
                  void load({});
                }}
                className="text-xs text-slate-400 underline hover:text-slate-600"
              >
                Clear
              </button>
            )}
          </form>
        </div>
        {applyError && (
          <p role="alert" className="mt-2 text-xs text-red-700">
            {applyError}
          </p>
        )}
      </div>

      <div className="px-4 py-5 sm:px-5">
        <p className="mb-5 max-w-prose text-xs text-slate-500">
          Current-state counts describe what is stored now. Publishing counts follow the chosen date
          range. Figures are only as reliable as what your integrations report — no invented
          numbers.
        </p>

        {state.kind === "loading" && (
          <div className="flex items-center gap-2 py-6 text-sm text-slate-500">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
            Loading report…
          </div>
        )}
        {state.kind === "unavailable" && (
          <p role="alert" className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
            {state.message}
          </p>
        )}
        {state.kind === "missing" && (
          <p className="text-sm text-slate-500">This workspace no longer exists.</p>
        )}

        <AssessmentPanel workspaceId={workspaceId} />

        {report && (
          <div className="grid gap-6 lg:grid-cols-2">
            {/* Lead pipeline funnel */}
            <div className="rounded-xl border border-slate-100 bg-white px-5 py-4 shadow-sm">
              <h3 className="mb-4 text-xs font-semibold uppercase tracking-widest text-slate-500">
                Lead Pipeline Funnel
              </h3>
              <LeadFunnel counts={report.leadsNow} />
            </div>

            {/* Content pipeline */}
            <div className="rounded-xl border border-slate-100 bg-white px-5 py-4 shadow-sm">
              <h3 className="mb-4 text-xs font-semibold uppercase tracking-widest text-slate-500">
                Content Pipeline
              </h3>
              <ContentPipeline counts={report.contentStatusNow} />
            </div>

            {/* Lead sources */}
            <div className="rounded-xl border border-slate-100 bg-white px-5 py-4 shadow-sm">
              <h3 className="mb-4 text-xs font-semibold uppercase tracking-widest text-slate-500">
                Lead Sources
              </h3>
              <LeadSources counts={report.leadSourcesNow} />
            </div>

            {/* Campaigns */}
            <div className="rounded-xl border border-slate-100 bg-white px-5 py-4 shadow-sm">
              <h3 className="mb-4 text-xs font-semibold uppercase tracking-widest text-slate-500">
                Campaigns by Status
              </h3>
              <CampaignStatus counts={report.campaignsNow} />
            </div>

            {/* Publishing jobs */}
            <div className="rounded-xl border border-slate-100 bg-white px-5 py-4 shadow-sm lg:col-span-2">
              <h3 className="mb-4 text-xs font-semibold uppercase tracking-widest text-slate-500">
                Publishing Jobs{" "}
                <span className="font-normal text-slate-400">
                  {report.range.since || report.range.until ? "(selected range)" : "(all time)"}
                </span>
              </h3>
              <PublishingJobs report={report} />
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
