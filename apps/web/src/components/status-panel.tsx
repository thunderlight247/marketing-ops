"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { describeDatabase, fetchHealth, type HealthState, type Tone } from "@/lib/health";

const toneClasses: Record<Tone, { badge: string; dot: string }> = {
  good: { badge: "bg-emerald-50 text-emerald-800 ring-emerald-600/20", dot: "bg-emerald-500" },
  warn: { badge: "bg-amber-50 text-amber-800 ring-amber-600/20", dot: "bg-amber-500" },
  bad: { badge: "bg-red-50 text-red-800 ring-red-600/20", dot: "bg-red-500" },
};

function Badge({ label, tone }: Readonly<{ label: string; tone: Tone }>) {
  const classes = toneClasses[tone];
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ring-inset ${classes.badge}`}
    >
      <span aria-hidden="true" className={`size-1.5 rounded-full ${classes.dot}`} />
      {label}
    </span>
  );
}

function Row({ term, children }: Readonly<{ term: string; children: React.ReactNode }>) {
  return (
    <div className="grid gap-1 py-3 sm:grid-cols-[9rem_minmax(0,1fr)] sm:gap-4">
      <dt className="text-xs font-medium text-slate-500 sm:pt-0.5">{term}</dt>
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
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="flex items-center justify-between gap-4 border-b border-slate-100 bg-slate-50 px-5 py-3">
        <h2 id="status-heading" className="text-xs font-semibold uppercase tracking-widest text-slate-500">
          System status
        </h2>
        <button
          type="button"
          onClick={() => void refresh()}
          disabled={state.kind === "loading"}
          className="rounded-md px-2.5 py-1 text-xs font-medium text-slate-600 hover:bg-slate-200 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {state.kind === "loading" ? "Checking…" : "Refresh"}
        </button>
      </div>

      <div aria-live="polite" className="px-5">
        {state.kind === "loading" ? (
          <div className="flex items-center gap-2 py-5">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
            <p className="text-sm text-slate-500">Connecting to the API…</p>
          </div>
        ) : null}

        {state.kind === "unavailable" ? (
          <div className="py-5">
            <div className="flex items-center gap-2">
              <Badge label="API unavailable" tone="bad" />
            </div>
            <p className="mt-3 text-sm text-slate-600">
              The platform API is not reachable. Check that it is running and that the correct API
              address is configured.
            </p>
          </div>
        ) : null}

        {state.kind === "loaded" ? (
          <>
            <dl className="divide-y divide-slate-100">
              <Row term="API">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge label="Operational" tone="good" />
                  <span className="text-xs text-slate-500">
                    {state.health.service}{state.health.version ? ` ${state.health.version}` : ""}{" "}
                    · {state.health.environment}
                  </span>
                </div>
              </Row>
              <Row term="Database">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge
                    label={describeDatabase(state.health.database.status).label}
                    tone={describeDatabase(state.health.database.status).tone}
                  />
                  <span className="text-xs text-slate-500">
                    {state.health.database.message}
                    {state.health.database.latencyMs !== undefined
                      ? ` · ${state.health.database.latencyMs} ms`
                      : ""}
                  </span>
                </div>
              </Row>
              <Row term="Last checked">
                <time dateTime={state.health.checkedAt} className="text-xs text-slate-500">
                  {new Date(state.health.checkedAt).toLocaleString()}
                </time>
              </Row>
            </dl>
            {state.health.database.status === "unconfigured" ? (
              <div className="border-t border-amber-100 bg-amber-50 px-5 py-3 -mx-5">
                <p className="text-xs text-amber-800">
                  The API is running but no database is connected. Configure{" "}
                  <code className="rounded bg-amber-100 px-1 font-mono">DATABASE_URL</code> and
                  restart the API to enable data persistence.
                </p>
              </div>
            ) : null}
          </>
        ) : null}
      </div>
    </section>
  );
}
