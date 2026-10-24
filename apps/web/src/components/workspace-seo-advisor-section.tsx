"use client";

import { useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  formatDataSource,
  formatRecommendationPriority,
  requestSeoAdvisory,
  type SeoAdvisoryState,
} from "@/lib/agents";

const inputClasses =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

function priorityChipClass(priority: string): string {
  switch (priority) {
    case "high":
      return "bg-red-50 text-red-800 ring-1 ring-inset ring-red-700/20";
    case "medium":
      return "bg-amber-50 text-amber-800 ring-1 ring-inset ring-amber-700/20";
    default:
      return "bg-slate-100 text-slate-600 ring-1 ring-inset ring-slate-500/20";
  }
}

export function WorkspaceSeoAdvisorSection({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const [state, setState] = useState<SeoAdvisoryState>({ kind: "idle" });
  const [focus, setFocus] = useState("");

  const apiBaseUrl = getApiBaseUrl();

  async function handleRequest(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState({ kind: "running" });
    setState(await requestSeoAdvisory(apiBaseUrl, workspaceId, focus));
  }

  const running = state.kind === "running";

  return (
    <section
      aria-labelledby="seo-advisor-heading"
      className="rounded-lg border border-slate-200 bg-white px-5 py-4"
    >
      <h2 id="seo-advisor-heading" className="text-base font-semibold text-slate-900">
        SEO specialist (advisory)
      </h2>
      <p className="mt-1 max-w-prose text-sm text-slate-600">
        The first integrated specialist agent reviews this workspace and produces prioritized,
        evidence-based recommendations. It never edits content, publishes, or contacts anyone: human
        review decides any follow-up.
      </p>

      <form onSubmit={(e) => void handleRequest(e)} className="mt-4 grid gap-3 sm:max-w-md">
        <label htmlFor="seo-focus" className="text-sm font-medium text-slate-700">
          Focus (optional)
        </label>
        <input
          id="seo-focus"
          value={focus}
          onChange={(event) => setFocus(event.target.value)}
          placeholder="e.g. local search visibility"
          autoComplete="off"
          className={inputClasses}
        />
        <div>
          <button type="submit" disabled={running} className={PRIMARY_BUTTON}>
            {running ? "Advisor is working…" : "Generate advisory report"}
          </button>
        </div>
      </form>

      {state.kind === "unavailable" && (
        <p role="alert" className="mt-4 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {state.message}
        </p>
      )}

      {state.kind === "done" && <ReportView state={state} />}

      {state.kind === "idle" && (
        <div className="mt-4 rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-600">
          No report has been requested yet.
        </div>
      )}
    </section>
  );
}

function ReportView({ state }: { state: Extract<SeoAdvisoryState, { kind: "done" }> }) {
  const { report } = state;
  return (
    <div className="mt-4 rounded-md border border-indigo-300 bg-teal-50/50 px-4 py-3 text-sm text-slate-800">
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-600">
        <span className="rounded bg-white px-2 py-0.5 ring-1 ring-inset ring-slate-300">
          {formatDataSource(report.dataSource)}
        </span>
        <span className="rounded bg-white px-2 py-0.5 ring-1 ring-inset ring-slate-300">
          {report.agentId} v{report.agentVersion} · {report.provider}/{report.model}
        </span>
        {state.durationMs > 0 && (
          <span className="rounded bg-white px-2 py-0.5 ring-1 ring-inset ring-slate-300">
            {(state.durationMs / 1000).toFixed(1)}s
          </span>
        )}
      </div>

      {state.dataNote !== "" && <p className="mt-2 text-xs text-slate-500">{state.dataNote}</p>}

      {report.summary !== "" && <p className="mt-3 font-medium text-slate-900">{report.summary}</p>}

      {report.gaps.length > 0 && (
        <div className="mt-3">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">Gaps</h3>
          <ul className="mt-1 list-disc pl-5 text-slate-700">
            {report.gaps.map((gap, index) => (
              <li key={index}>{gap}</li>
            ))}
          </ul>
        </div>
      )}

      <div className="mt-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Recommendations ({report.recommendations.length})
        </h3>
        <ul className="mt-2 grid gap-2">
          {report.recommendations.map((rec, index) => (
            <li key={index} className="rounded-md border border-slate-200 bg-white px-3 py-2">
              <div className="flex flex-wrap items-center gap-2">
                <span
                  className={`rounded px-2 py-0.5 text-xs font-semibold ${priorityChipClass(rec.priority)}`}
                >
                  {formatRecommendationPriority(rec.priority)}
                </span>
                {rec.area !== "" && <span className="text-xs text-slate-500">{rec.area}</span>}
                {rec.page !== "" && (
                  <span className="font-mono text-xs text-slate-500">{rec.page}</span>
                )}
              </div>
              <p className="mt-1 text-slate-900">{rec.issue}</p>
              <p className="mt-1 text-xs text-slate-500">Evidence: {rec.evidence}</p>
              <p className="mt-1 text-slate-800">Action: {rec.action}</p>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
