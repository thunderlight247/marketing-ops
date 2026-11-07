"use client";

import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchAnalyticsSummary,
  type AnalyticsSummary,
  type WorkspaceAnalytics,
} from "@/lib/analytics";

// ─── Shared mini-components ───────────────────────────────────────────────────

function StatCard({
  label,
  value,
  sub,
  color = "text-slate-800",
}: {
  label: string;
  value: number | string;
  sub?: string;
  color?: string;
}) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-4 py-4 shadow-sm">
      <p className="text-xs font-semibold uppercase tracking-widest text-slate-400">{label}</p>
      <p className={`mt-1 text-3xl font-bold ${color}`}>{value}</p>
      {sub && <p className="mt-0.5 text-xs text-slate-400">{sub}</p>}
    </div>
  );
}

function MiniBar({
  label,
  value,
  max,
  color,
}: {
  label: string;
  value: number;
  max: number;
  color: string;
}) {
  const pct = max > 0 ? Math.round((value / max) * 100) : 0;
  return (
    <div>
      <div className="flex items-baseline justify-between gap-2 mb-1">
        <span className="text-xs text-slate-600 truncate">{label}</span>
        <span className="text-xs font-semibold text-slate-800 tabular-nums shrink-0">{value}</span>
      </div>
      <div className="h-2 w-full rounded-full bg-slate-100">
        <div className={`h-2 rounded-full ${color} transition-all`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function SectionHeader({ title }: { title: string }) {
  return (
    <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
      <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">{title}</h2>
    </div>
  );
}

function humanStatus(status: string): string {
  const words = status.replace(/_/g, " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

const LEAD_STATUS_COLOR: Record<string, string> = {
  new: "bg-blue-400",
  contacted: "bg-amber-400",
  qualified: "bg-indigo-400",
  won: "bg-emerald-500",
  lost: "bg-slate-300",
};

const ENGINE_STATUS_COLOR: Record<string, string> = {
  discovered: "bg-slate-300",
  pending: "bg-amber-400",
  processing: "bg-blue-400",
  sent: "bg-indigo-400",
  delivered: "bg-emerald-400",
  replied: "bg-emerald-600",
  demo_sent: "bg-purple-400",
  failed: "bg-red-400",
  blacklisted: "bg-red-300",
};

function WorkspaceRow({ workspace }: { workspace: WorkspaceAnalytics }) {
  return (
    <tr className="border-b border-slate-100 last:border-0 hover:bg-slate-50 transition-colors">
      <td className="px-5 py-3">
        <a
          href={`/workspaces/${encodeURIComponent(workspace.workspaceId)}`}
          className="font-medium text-slate-900 hover:text-indigo-700"
        >
          {workspace.name}
        </a>
        <p className="text-xs text-slate-400">/{workspace.slug}</p>
      </td>
      <td className="px-4 py-3 text-right tabular-nums text-slate-700">{workspace.campaigns}</td>
      <td className="px-4 py-3 text-right tabular-nums text-slate-700">{workspace.crmLeads}</td>
      <td className="px-4 py-3 text-right tabular-nums text-emerald-700 hidden sm:table-cell">
        {workspace.crmWon}
      </td>
      <td className="px-4 py-3 text-right tabular-nums text-slate-700 hidden md:table-cell">
        {workspace.engineLeads}
      </td>
      <td className="px-4 py-3 text-right tabular-nums text-slate-700 hidden md:table-cell">
        {workspace.emailsSent}
      </td>
    </tr>
  );
}

// ─── Main analytics page ──────────────────────────────────────────────────────

export function WorkspaceAnalytics({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const apiBaseUrl = getApiBaseUrl();
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loadedAt, setLoadedAt] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setError(null);
      const state = await fetchAnalyticsSummary(apiBaseUrl, workspaceId ?? "");
      if (cancelled) return;
      if (state.kind === "loaded") {
        setSummary(state.summary);
        setLoadedAt(new Date().toLocaleTimeString([], { hour: "numeric", minute: "2-digit" }));
      } else if (state.kind === "unavailable") {
        setError(state.message);
      }
      setLoading(false);
    })();
    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, reloadKey, workspaceId]);

  const totals = summary?.totals;
  const workspaces = summary?.byWorkspace ?? [];
  const crmStatuses = summary?.crmLeadsByStatus ?? {};
  const engineStatuses = summary?.engineLeadsByStatus ?? {};
  const maxLeadStatus = Math.max(...Object.values(crmStatuses), 1);
  const maxEngineStatus = Math.max(...Object.values(engineStatuses), 1);

  return (
    <div className="grid gap-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Analytics</h1>
          <p className="mt-1 text-sm text-slate-500">
            {workspaceId
              ? "This workspace's leads, outreach, email, and campaign counts."
              : "Overview of leads, campaigns, outreach, and email across all workspaces."}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {loadedAt && <span className="text-xs text-slate-400">Updated {loadedAt}</span>}
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            disabled={loading}
            className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-50 transition-colors"
          >
            ↻ Refresh
          </button>
        </div>
      </div>

      {loading && (
        <div className="flex items-center gap-2 text-sm text-slate-500 py-8">
          <span className="h-2 w-2 animate-pulse rounded-full bg-slate-400" />
          Loading analytics…
        </div>
      )}

      {error && (
        <div
          role="alert"
          className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800"
        >
          <p>{error}</p>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            className="mt-2 rounded-lg border border-red-200 bg-white px-3 py-1 text-xs font-medium text-red-700 hover:bg-red-50"
          >
            Retry
          </button>
        </div>
      )}

      {!loading && !error && totals && (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <StatCard
              label="CRM Leads"
              value={totals.crmLeads}
              sub={`${totals.crmWon} won`}
              color="text-indigo-700"
            />
            <StatCard label="Campaigns" value={totals.campaigns} color="text-slate-700" />
            <StatCard
              label="Engine Leads"
              value={totals.engineLeads}
              sub={`${totals.engineSent} sent · ${totals.engineReplied} replied`}
              color="text-amber-700"
            />
            <StatCard
              label="Emails Sent"
              value={totals.emailsSent}
              sub={`${totals.emailsDelivered} delivered · ${totals.emailsTotal} logged`}
              color="text-emerald-700"
            />
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <SectionHeader title="CRM Leads by Status" />
              <div className="px-5 py-4 grid gap-3">
                {totals.crmLeads === 0 ? (
                  <p className="text-sm text-slate-500">No CRM leads recorded yet.</p>
                ) : (
                  Object.entries(crmStatuses)
                    .sort(([, a], [, b]) => b - a)
                    .map(([status, n]) => (
                      <MiniBar
                        key={status}
                        label={humanStatus(status)}
                        value={n}
                        max={maxLeadStatus}
                        color={LEAD_STATUS_COLOR[status] ?? "bg-slate-400"}
                      />
                    ))
                )}
              </div>
            </div>

            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <SectionHeader title="Engine Outreach by Status" />
              <div className="px-5 py-4 grid gap-3">
                {totals.engineLeads === 0 ? (
                  <p className="text-sm text-slate-500">
                    No engine leads yet. Run discovery from a workspace.
                  </p>
                ) : (
                  Object.entries(engineStatuses)
                    .sort(([, a], [, b]) => b - a)
                    .map(([status, n]) => (
                      <MiniBar
                        key={status}
                        label={humanStatus(status)}
                        value={n}
                        max={maxEngineStatus}
                        color={ENGINE_STATUS_COLOR[status] ?? "bg-slate-400"}
                      />
                    ))
                )}
              </div>
            </div>
          </div>

          {workspaces.length > 0 && (
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <SectionHeader title="Per-Workspace Summary" />
              <div className="overflow-x-auto">
                <table className="min-w-full text-sm">
                  <thead>
                    <tr className="border-b border-slate-200 bg-slate-50 text-left">
                      <th className="px-5 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                        Workspace
                      </th>
                      <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 text-right">
                        Campaigns
                      </th>
                      <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 text-right">
                        CRM Leads
                      </th>
                      <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 text-right hidden sm:table-cell">
                        Won
                      </th>
                      <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 text-right hidden md:table-cell">
                        Engine Leads
                      </th>
                      <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 text-right hidden md:table-cell">
                        Emails Sent
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {workspaces.map((workspace) => (
                      <WorkspaceRow key={workspace.workspaceId} workspace={workspace} />
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
