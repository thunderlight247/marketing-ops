"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { describeDatabase, fetchHealth, type HealthState, type Tone } from "@/lib/health";

const toneClasses: Record<Tone, { badge: string; dot: string }> = {
  good: { badge: "bg-emerald-50 text-emerald-900 ring-emerald-700/25", dot: "bg-emerald-600" },
  warn: { badge: "bg-amber-50 text-amber-900 ring-amber-700/25", dot: "bg-amber-600" },
  bad: { badge: "bg-red-50 text-red-900 ring-red-700/25", dot: "bg-red-600" },
};

function Badge({ label, tone }: Readonly<{ label: string; tone: Tone }>) {
  const classes = toneClasses[tone];
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-sm font-medium ring-1 ring-inset ${classes.badge}`}
    >
      <span aria-hidden="true" className={`size-1.5 rounded-full ${classes.dot}`} />
      {label}
    </span>
  );
}

function Row({ term, children }: Readonly<{ term: string; children: React.ReactNode }>) {
  return (
    <div className="grid gap-1 py-3 sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-4">
      <dt className="text-sm font-medium text-slate-700">{term}</dt>
      <dd className="min-w-0 text-sm text-slate-800">{children}</dd>
    </div>
  );
}

export function StatusPanel() {
  const [state, setState] = useState<HealthState>({ kind: "loading" });
  const apiBaseUrl = getApiBaseUrl();

  const refresh = useCallback(async () => {
    setState({ kind: "loading" });
    setState(await fetchHealth(apiBaseUrl));
  }, [apiBaseUrl]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return (
    <section
      aria-labelledby="status-heading"
      className="rounded-lg border border-slate-200 bg-white"
    >
      <div className="flex items-center justify-between gap-4 border-b border-slate-200 px-5 py-4">
        <h2 id="status-heading" className="text-base font-semibold">
          System status
        </h2>
        <button
          type="button"
          onClick={() => void refresh()}
          disabled={state.kind === "loading"}
          className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-800 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60"
        >
          Check again
        </button>
      </div>

      <div aria-live="polite" className="px-5">
        {state.kind === "loading" ? (
          <p className="py-6 text-sm text-slate-600">Checking the API...</p>
        ) : null}

        {state.kind === "unavailable" ? (
          <div className="py-5">
            <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
              <Badge label="API unavailable" tone="bad" />
            </p>
            <p className="mt-3 text-sm text-slate-800">{state.reason}</p>
            <p className="mt-2 text-sm text-slate-600">
              Start the API with{" "}
              <code className="rounded bg-slate-100 px-1 font-mono">make api-run</code>, or check{" "}
              <code className="rounded bg-slate-100 px-1 font-mono">NEXT_PUBLIC_API_BASE_URL</code>{" "}
              in <code className="rounded bg-slate-100 px-1 font-mono">apps/web/.env.local</code>.
            </p>
          </div>
        ) : null}

        {state.kind === "loaded" ? (
          <dl className="divide-y divide-slate-200">
            <Row term="API">
              <Badge label="Running" tone="good" />
              <span className="mt-1 block text-slate-600 sm:ml-2 sm:inline">
                {state.health.service} {state.health.version}, {state.health.environment}
              </span>
            </Row>
            <Row term="Database">
              <Badge
                label={describeDatabase(state.health.database.status).label}
                tone={describeDatabase(state.health.database.status).tone}
              />
              <span className="mt-1 block text-slate-600 sm:ml-2 sm:inline">
                {state.health.database.message}
                {state.health.database.latencyMs !== undefined
                  ? ` (${state.health.database.latencyMs} ms)`
                  : ""}
              </span>
            </Row>
            <Row term="Last checked">
              <time dateTime={state.health.checkedAt}>
                {new Date(state.health.checkedAt).toLocaleString()}
              </time>
            </Row>
          </dl>
        ) : null}
      </div>
    </section>
  );
}
