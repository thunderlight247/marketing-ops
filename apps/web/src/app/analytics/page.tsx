"use client";

import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type Workspace } from "@/lib/workspaces";
import { fetchWorkspaceCampaignsLeads, type Lead, type Campaign } from "@/lib/campaigns";
import { listLeadEngineLeads, type LeadEngineLead } from "@/lib/leadEngine";
import { fetchEmailDeliveries, type EmailDelivery } from "@/lib/email";

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

function MiniBar({ label, value, max, color }: { label: string; value: number; max: number; color: string }) {
  const pct = max > 0 ? Math.round((value / max) * 100) : 0;
  return (
    <div>
      <div className="flex items-baseline justify-between gap-2 mb-1">
        <span className="text-xs text-slate-600 truncate">{label}</span>
        <span className="text-xs font-semibold text-slate-800 tabular-nums shrink-0">{value}</span>
      </div>
      <div className="h-2 w-full rounded-full bg-slate-100">
        <div
          className={`h-2 rounded-full ${color} transition-all`}
          style={{ width: `${pct}%` }}
        />
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

// ─── Per-workspace data aggregator ───────────────────────────────────────────

interface WorkspaceStats {
  workspace: Workspace;
  leads: Lead[];
  campaigns: Campaign[];
  engineLeads: LeadEngineLead[];
  deliveries: EmailDelivery[];
}

async function fetchWorkspaceStats(apiBaseUrl: string, workspace: Workspace): Promise<WorkspaceStats> {
  const [campaignsResult, engineLeads, deliveriesResult] = await Promise.allSettled([
    fetchWorkspaceCampaignsLeads(apiBaseUrl, workspace.id),
    listLeadEngineLeads(apiBaseUrl, workspace.id),
    fetchEmailDeliveries(apiBaseUrl, workspace.id),
  ]);

  const campaignsData =
    campaignsResult.status === "fulfilled" && campaignsResult.value.kind === "loaded"
      ? campaignsResult.value
      : { campaigns: [], leads: [] };

  return {
    workspace,
    leads: campaignsData.leads,
    campaigns: campaignsData.campaigns,
    engineLeads: engineLeads.status === "fulfilled" ? engineLeads.value : [],
    deliveries:
      deliveriesResult.status === "fulfilled" && deliveriesResult.value.kind === "loaded"
        ? deliveriesResult.value.deliveries
        : [],
  };
}

// ─── Main analytics page ──────────────────────────────────────────────────────

export default function AnalyticsPage() {
  const apiBaseUrl = getApiBaseUrl();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [stats, setStats] = useState<WorkspaceStats[]>([]);

  useEffect(() => {
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        const wsState = await fetchWorkspaces(apiBaseUrl);
        if (wsState.kind !== "loaded") {
          setError("Could not load workspaces.");
          return;
        }
        const all = await Promise.all(
          wsState.workspaces.map((ws) => fetchWorkspaceStats(apiBaseUrl, ws)),
        );
        setStats(all);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load analytics.");
      } finally {
        setLoading(false);
      }
    })();
  }, [apiBaseUrl]);

  // Aggregate totals
  const totalLeads = stats.reduce((n, s) => n + s.leads.length, 0);
  const totalEngineLeads = stats.reduce((n, s) => n + s.engineLeads.length, 0);
  const totalDeliveries = stats.reduce((n, s) => n + s.deliveries.length, 0);
  const totalCampaigns = stats.reduce((n, s) => n + s.campaigns.length, 0);

  const wonLeads = stats.reduce((n, s) => n + s.leads.filter((l) => l.status === "won").length, 0);
  const engineSent = stats.reduce(
    (n, s) =>
      n +
      s.engineLeads.filter((l) =>
        ["sent", "delivered", "replied", "demo_sent"].includes(l.status),
      ).length,
    0,
  );
  const engineReplied = stats.reduce(
    (n, s) => n + s.engineLeads.filter((l) => l.status === "replied" || l.status === "demo_sent").length,
    0,
  );
  const deliveredEmails = stats.reduce(
    (n, s) => n + s.deliveries.filter((d) => d.deliveryState === "delivered").length,
    0,
  );

  // Lead status breakdown (across all workspaces)
  const leadStatusCounts: Record<string, number> = {};
  stats.forEach((s) =>
    s.leads.forEach((l) => {
      leadStatusCounts[l.status] = (leadStatusCounts[l.status] ?? 0) + 1;
    }),
  );
  const maxLeadStatus = Math.max(...Object.values(leadStatusCounts), 1);

  // Engine lead status breakdown
  const engineStatusCounts: Record<string, number> = {};
  stats.forEach((s) =>
    s.engineLeads.forEach((l) => {
      engineStatusCounts[l.status] = (engineStatusCounts[l.status] ?? 0) + 1;
    }),
  );
  const maxEngineStatus = Math.max(...Object.values(engineStatusCounts), 1);

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

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900">Analytics</h1>
        <p className="mt-1 text-sm text-slate-500">
          Live overview of leads, campaigns, outreach, and email across all workspaces.
        </p>
      </div>

      {loading && (
        <div className="flex items-center gap-2 text-sm text-slate-500 py-8">
          <span className="h-2 w-2 animate-pulse rounded-full bg-slate-400" />
          Loading data…
        </div>
      )}

      {error && (
        <p role="alert" className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800">
          {error}
        </p>
      )}

      {!loading && !error && (
        <>
          {/* Top-level stat cards */}
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <StatCard label="CRM Leads" value={totalLeads} sub={`${wonLeads} won`} color="text-indigo-700" />
            <StatCard label="Campaigns" value={totalCampaigns} color="text-slate-700" />
            <StatCard
              label="Engine Leads"
              value={totalEngineLeads}
              sub={`${engineSent} sent · ${engineReplied} replied`}
              color="text-amber-700"
            />
            <StatCard
              label="Emails Sent"
              value={totalDeliveries}
              sub={`${deliveredEmails} delivered`}
              color="text-emerald-700"
            />
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            {/* CRM lead status breakdown */}
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <SectionHeader title="CRM Leads by Status" />
              <div className="px-5 py-4 grid gap-3">
                {totalLeads === 0 ? (
                  <p className="text-sm text-slate-500">No CRM leads recorded yet.</p>
                ) : (
                  Object.entries(leadStatusCounts)
                    .sort(([, a], [, b]) => b - a)
                    .map(([status, count]) => (
                      <MiniBar
                        key={status}
                        label={status.charAt(0).toUpperCase() + status.slice(1)}
                        value={count}
                        max={maxLeadStatus}
                        color={LEAD_STATUS_COLOR[status] ?? "bg-slate-400"}
                      />
                    ))
                )}
              </div>
            </div>

            {/* Engine lead status breakdown */}
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <SectionHeader title="Engine Outreach by Status" />
              <div className="px-5 py-4 grid gap-3">
                {totalEngineLeads === 0 ? (
                  <p className="text-sm text-slate-500">No engine leads yet. Run discovery from a workspace.</p>
                ) : (
                  Object.entries(engineStatusCounts)
                    .sort(([, a], [, b]) => b - a)
                    .map(([status, count]) => (
                      <MiniBar
                        key={status}
                        label={status.replace("_", " ").charAt(0).toUpperCase() + status.replace("_", " ").slice(1)}
                        value={count}
                        max={maxEngineStatus}
                        color={ENGINE_STATUS_COLOR[status] ?? "bg-slate-400"}
                      />
                    ))
                )}
              </div>
            </div>
          </div>

          {/* Per-workspace table */}
          {stats.length > 0 && (
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
                        Emails
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.map((s) => (
                      <tr key={s.workspace.id} className="border-b border-slate-100 last:border-0 hover:bg-slate-50 transition-colors">
                        <td className="px-5 py-3">
                          <a
                            href={`/workspaces/${encodeURIComponent(s.workspace.id)}`}
                            className="font-medium text-slate-900 hover:text-indigo-700"
                          >
                            {s.workspace.name}
                          </a>
                          <p className="text-xs text-slate-400">{s.workspace.websiteUrl ?? `/${s.workspace.slug}`}</p>
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums text-slate-700">{s.campaigns.length}</td>
                        <td className="px-4 py-3 text-right tabular-nums text-slate-700">{s.leads.length}</td>
                        <td className="px-4 py-3 text-right tabular-nums text-emerald-700 hidden sm:table-cell">
                          {s.leads.filter((l) => l.status === "won").length}
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums text-slate-700 hidden md:table-cell">
                          {s.engineLeads.length}
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums text-slate-700 hidden md:table-cell">
                          {s.deliveries.length}
                        </td>
                      </tr>
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
