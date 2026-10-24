"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchWorkspaceReport,
  formatLeadSourceLabel,
  formatStatusLabel,
  type ReportCounts,
  type ReportState,
} from "@/lib/reports";

const inputClasses =
  "rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";

function CountRows({ counts }: { counts: ReportCounts }) {
  const entries = Object.entries(counts);
  if (entries.length === 0) {
    return <p className="text-sm text-slate-500">None recorded yet.</p>;
  }
  return (
    <ul className="mt-2 space-y-1">
      {entries
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([label, count]) => (
          <li key={label} className="flex items-center justify-between gap-3 text-sm">
            <span className="text-slate-700">{formatStatusLabel(label)}</span>
            <span
              className="rounded bg-slate-100 px-2 py-0.5 text-xs font-semibold tabular-nums text-slate-800"
              data-count={label}
            >
              {count}
            </span>
          </li>
        ))}
    </ul>
  );
}

function SourceRows({ counts }: { counts: ReportCounts }) {
  const entries = Object.entries(counts);
  if (entries.length === 0) {
    return <p className="text-sm text-slate-500">None recorded yet.</p>;
  }
  return (
    <ul className="mt-2 space-y-1">
      {entries
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([label, count]) => (
          <li key={label} className="flex items-center justify-between gap-3 text-sm">
            <span className="text-slate-700">{formatLeadSourceLabel(label)}</span>
            <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-semibold tabular-nums text-slate-800">
              {count}
            </span>
          </li>
        ))}
    </ul>
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

  return (
    <section
      aria-labelledby="workspace-reports-heading"
      className="rounded-lg border border-slate-200 bg-white px-5 py-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="workspace-reports-heading" className="text-base font-semibold text-slate-900">
          Report
        </h2>
        <form onSubmit={handleApply} className="flex flex-wrap items-center gap-2">
          <label htmlFor="report-since" className="text-xs font-medium text-slate-600">
            Publishing from
          </label>
          <input
            id="report-since"
            type="date"
            value={since}
            onChange={(e) => setSince(e.target.value)}
            className={inputClasses}
          />
          <label htmlFor="report-until" className="text-xs font-medium text-slate-600">
            to
          </label>
          <input
            id="report-until"
            type="date"
            value={until}
            onChange={(e) => setUntil(e.target.value)}
            className={inputClasses}
          />
          <button
            type="submit"
            disabled={applying}
            className="rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-60"
          >
            {applying ? "Updating…" : "Apply range"}
          </button>
        </form>
      </div>

      <p className="mt-1 max-w-prose text-xs text-slate-500">
        Current-state counts describe what is stored now; publishing counts follow the chosen range.
        Metrics that no verified integration supplies are never invented here.
      </p>

      {applyError && (
        <p role="alert" className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {applyError}
        </p>
      )}

      {state.kind === "loading" && <p className="mt-4 text-sm text-slate-500">Loading…</p>}
      {state.kind === "unavailable" && (
        <p role="alert" className="mt-4 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {state.message}
        </p>
      )}
      {state.kind === "missing" && (
        <p className="mt-4 text-sm text-slate-500">This workspace no longer exists.</p>
      )}

      {state.kind === "loaded" && (
        <>
          <p className="mt-3 text-xs text-slate-500">
            Generated{" "}
            {new Date(state.report.generatedAt).toISOString().replace("T", " ").slice(0, 16)} UTC
            {state.report.range.since || state.report.range.until
              ? " · publishing range applied"
              : " · full publishing history"}
          </p>

          <dl className="mt-3 grid gap-3 sm:grid-cols-2">
            <div className="rounded border border-slate-100 bg-slate-50 px-3 py-2">
              <dt className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                Publishing (range)
              </dt>
              <CountRows counts={state.report.publishingRange.jobsByStatus} />
            </div>
            <div className="rounded border border-slate-100 bg-slate-50 px-3 py-2">
              <dt className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                Content (now)
              </dt>
              <CountRows counts={state.report.contentStatusNow} />
            </div>
            <div className="rounded border border-slate-100 bg-slate-50 px-3 py-2">
              <dt className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                Campaigns (now)
              </dt>
              <CountRows counts={state.report.campaignsNow} />
            </div>
            <div className="rounded border border-slate-100 bg-slate-50 px-3 py-2">
              <dt className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                Leads by status (now)
              </dt>
              <CountRows counts={state.report.leadsNow} />
            </div>
            <div className="rounded border border-slate-100 bg-slate-50 px-3 py-2 sm:col-span-2">
              <dt className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                Leads by source (now)
              </dt>
              <SourceRows counts={state.report.leadSourcesNow} />
            </div>
          </dl>

          {state.report.publishingRange.recentFailures.length > 0 && (
            <div className="mt-3">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                Recent publishing failures (range)
              </h3>
              <ul className="mt-2 space-y-2">
                {state.report.publishingRange.recentFailures.map((f) => (
                  <li
                    key={f.jobId}
                    className="rounded border border-slate-100 bg-slate-50 px-3 py-2 text-sm"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-xs text-slate-600">
                        job {f.jobId.slice(0, 8)}
                      </span>
                      <span className="text-xs text-slate-500">
                        {f.attempts} attempt{f.attempts === 1 ? "" : "s"}
                      </span>
                      <span className="text-xs text-slate-500">
                        {new Date(f.updatedAt).toISOString().replace("T", " ").slice(0, 16)} UTC
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-red-700">{f.lastError}</p>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  );
}
