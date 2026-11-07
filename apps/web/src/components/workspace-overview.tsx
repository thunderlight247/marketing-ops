"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchAnalyticsSummary, type AnalyticsSummary } from "@/lib/analytics";
import { fetchWorkspaceDetail, type WorkspaceDetailState } from "@/lib/workspaces";
import { WorkspaceSwitcher } from "@/components/workspace-switcher";

const PAGES = [
  {
    segment: "/content",
    label: "Content",
    description: "Drafts, AI-assisted creation, campaigns, and review.",
    accent: "from-sky-500 to-blue-600",
  },
  {
    segment: "/calendar",
    label: "Calendar",
    description: "Scheduled publishing, follow-ups, and overdue work.",
    accent: "from-amber-500 to-orange-600",
  },
  {
    segment: "/lead-engine",
    label: "Lead engine",
    description: "WhatsApp pairing, Google Places discovery, outreach, and replies.",
    accent: "from-emerald-500 to-teal-600",
  },
  {
    segment: "/email",
    label: "Email",
    description: "Provider connection, delivery log, bounces, and retries.",
    accent: "from-rose-500 to-pink-600",
  },
  {
    segment: "/integrations",
    label: "Integrations",
    description: "Publishing destinations and CMS connections for this workspace.",
    accent: "from-violet-500 to-purple-600",
  },
  {
    segment: "/analytics",
    label: "Analytics",
    description: "Lead, outreach, email, publishing, and SEO reporting.",
    accent: "from-indigo-500 to-blue-600",
  },
  {
    segment: "/settings",
    label: "Settings",
    description: "Workspace details, brand rules, and lifecycle.",
    accent: "from-slate-600 to-slate-800",
  },
] as const;

function Stat({ label, value, hint }: { label: string; value: number | string; hint: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <p className="text-xs font-semibold uppercase tracking-widest text-slate-400">{label}</p>
      <p className="mt-1 text-3xl font-bold tabular-nums text-slate-900">{value}</p>
      <p className="mt-0.5 text-xs text-slate-500">{hint}</p>
    </div>
  );
}

function brandCompletion(state: WorkspaceDetailState): number {
  if (state.kind !== "loaded" || !state.brand) return 0;
  const fields = [
    state.brand.businessDescription,
    state.brand.audience,
    state.brand.services,
    state.brand.tone,
  ];
  const filled = fields.filter((value) => value && value.trim() !== "").length;
  const extras =
    (state.brand.prohibitedClaims.length > 0 ? 1 : 0) +
    (state.brand.callsToAction.length > 0 ? 1 : 0);
  return Math.round(((filled + extras) / 6) * 100);
}

export function WorkspaceOverview({ workspaceId }: { workspaceId: string }) {
  const [detail, setDetail] = useState<WorkspaceDetailState>({ kind: "loading" });
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null);
  const [analyticsError, setAnalyticsError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const base = getApiBaseUrl();
    void Promise.all([
      fetchWorkspaceDetail(base, workspaceId).then((value) => {
        if (active) setDetail(value);
      }),
      fetchAnalyticsSummary(base, workspaceId).then((value) => {
        if (!active) return;
        if (value.kind === "loaded") setSummary(value.summary);
        else if (value.kind === "unavailable") setAnalyticsError(value.message);
      }),
    ]);
    return () => {
      active = false;
    };
  }, [workspaceId]);

  const workspace = detail.kind === "loaded" ? detail.workspace : null;
  const totals = summary?.byWorkspace[0];
  const completion = brandCompletion(detail);
  const base = `/workspaces/${encodeURIComponent(workspaceId)}`;
  const engineStatuses = summary?.engineLeadsByStatus ?? {};
  const maxEngine = Math.max(...Object.values(engineStatuses), 1);

  if (detail.kind === "loading") {
    return (
      <div className="grid gap-3" aria-busy="true">
        <div className="h-28 animate-pulse rounded-2xl bg-slate-200/70" />
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[0, 1, 2, 3].map((item) => (
            <div key={item} className="h-28 animate-pulse rounded-xl bg-slate-200/70" />
          ))}
        </div>
      </div>
    );
  }

  if (detail.kind === "missing" || detail.kind === "unavailable") {
    return (
      <div className="rounded-2xl border border-red-100 bg-red-50 p-6">
        <h2 className="text-base font-semibold text-red-900">Could not open this workspace</h2>
        <p className="mt-1 text-sm text-red-800">{detail.reason}</p>
        <Link
          href="/workspaces"
          className="mt-4 inline-flex rounded-lg bg-red-700 px-3.5 py-2 text-sm font-semibold text-white hover:bg-red-800"
        >
          Back to Workspaces
        </Link>
      </div>
    );
  }

  return (
    <div className="grid gap-6">
      {analyticsError ? (
        <p
          role="alert"
          className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
        >
          Some summary data could not be loaded: {analyticsError}
        </p>
      ) : null}

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat
          label="Engine Leads"
          value={totals?.engineLeads ?? 0}
          hint={`${totals?.engineSent ?? 0} contacted`}
        />
        <Stat label="CRM Leads" value={totals?.crmLeads ?? 0} hint={`${totals?.crmWon ?? 0} won`} />
        <Stat
          label="Emails Sent"
          value={totals?.emailsSent ?? 0}
          hint={`${totals?.emailsDelivered ?? 0} delivered`}
        />
        <Stat
          label="Brand Profile"
          value={`${completion}%`}
          hint={completion === 100 ? "Complete" : "Add missing context"}
        />
      </div>

      <section aria-labelledby="workspace-pages-heading">
        <div className="mb-3 flex items-end justify-between gap-3">
          <div>
            <h2 id="workspace-pages-heading" className="text-sm font-semibold text-slate-900">
              Workspace pages
            </h2>
            <p className="text-xs text-slate-500">
              Each page only reads and writes this workspace.
            </p>
          </div>
          {workspace?.websiteUrl ? (
            <a
              href={workspace.websiteUrl}
              target="_blank"
              rel="noreferrer"
              className="text-xs font-semibold text-indigo-700 hover:text-indigo-900"
            >
              Visit site ↗
            </a>
          ) : null}
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          {PAGES.map((page) => (
            <Link
              key={page.segment}
              href={`${base}${page.segment}`}
              className="group relative overflow-hidden rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition-all hover:-translate-y-0.5 hover:border-indigo-200 hover:shadow-md"
            >
              <span
                className={`absolute inset-x-0 top-0 h-1 bg-gradient-to-r ${page.accent}`}
                aria-hidden="true"
              />
              <p className="text-sm font-semibold text-slate-900 group-hover:text-indigo-700">
                {page.label} <span aria-hidden="true">→</span>
              </p>
              <p className="mt-1 text-xs leading-relaxed text-slate-500">{page.description}</p>
            </Link>
          ))}
        </div>
      </section>

      <section aria-labelledby="workspace-tools-heading">
        <div className="mb-3">
          <h2 id="workspace-tools-heading" className="text-sm font-semibold text-slate-900">
            Workspace tools
          </h2>
          <p className="text-xs text-slate-500">
            The original workspace functions are still here, grouped by what they do.
          </p>
        </div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {[
            {
              label: "WhatsApp lead engine",
              href: `${base}/lead-engine`,
              detail: "Google Places discovery, sending, limits, and pairing",
            },
            {
              label: "Email & delivery",
              href: `${base}/email`,
              detail: "Provider connection, test sends, bounces, and retries",
            },
            {
              label: "Integrations",
              href: `${base}/integrations`,
              detail: "CMS connections and publishing destinations",
            },
            {
              label: "Campaigns & leads",
              href: `${base}/content#campaigns`,
              detail: "Campaign records, CRM leads, and reply drafting",
            },
            {
              label: "Content studio",
              href: `${base}/content`,
              detail: "Drafts, AI generation, channel copy, and approvals",
            },
            {
              label: "Reports & SEO",
              href: `${base}/analytics#reports`,
              detail: "Publishing reports and Search Console advice",
            },
            {
              label: "Workspace settings",
              href: `${base}/settings`,
              detail: "Brand rules, workspace facts, and deletion",
            },
          ].map((tool) => (
            <Link
              key={tool.label}
              href={tool.href}
              className="group flex items-start justify-between gap-3 rounded-xl border border-slate-200 bg-white px-4 py-3 shadow-sm transition-colors hover:border-indigo-200 hover:bg-indigo-50/40"
            >
              <span className="min-w-0">
                <span className="block text-sm font-semibold text-slate-900 group-hover:text-indigo-700">
                  {tool.label}
                </span>
                <span className="mt-0.5 block text-xs leading-relaxed text-slate-500">
                  {tool.detail}
                </span>
              </span>
              <span className="text-slate-300 group-hover:text-indigo-500" aria-hidden="true">
                →
              </span>
            </Link>
          ))}
        </div>
      </section>

      <div className="grid gap-4 lg:grid-cols-2">
        <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Outreach status
          </h2>
          {Object.keys(engineStatuses).length === 0 ? (
            <p className="mt-3 text-sm text-slate-500">
              No engine leads yet. Connect WhatsApp and configure the lead engine on its own
              page, then run discovery.
            </p>
          ) : (
            <ul className="mt-4 grid gap-3">
              {Object.entries(engineStatuses)
                .sort(([, a], [, b]) => b - a)
                .map(([status, count]) => (
                  <li key={status}>
                    <div className="mb-1 flex items-center justify-between gap-3 text-xs">
                      <span className="capitalize text-slate-600">{status.replace(/_/g, " ")}</span>
                      <span className="font-semibold tabular-nums text-slate-800">{count}</span>
                    </div>
                    <div className="h-1.5 rounded-full bg-slate-100">
                      <div
                        className={`h-1.5 rounded-full ${
                          status === "failed" || status === "blacklisted"
                            ? "bg-red-400"
                            : status === "sent" || status === "delivered"
                              ? "bg-emerald-500"
                              : "bg-indigo-400"
                        }`}
                        style={{ width: `${Math.round((count / maxEngine) * 100)}%` }}
                      />
                    </div>
                  </li>
                ))}
            </ul>
          )}
          <Link
            href={`${base}/analytics`}
            className="mt-4 inline-flex text-xs font-semibold text-indigo-700 hover:text-indigo-900"
          >
            Open analytics →
          </Link>
        </section>

        <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Brand context
              </h2>
              <p className="mt-2 text-sm text-slate-600">
                {detail.brand
                  ? "AI drafting uses this profile to keep copy on-brand."
                  : "No brand profile yet. Add it so drafts and agents have context."}
              </p>
            </div>
            <span className="text-2xl font-bold tabular-nums text-indigo-700">{completion}%</span>
          </div>
          <div className="mt-4 h-2 rounded-full bg-slate-100">
            <div
              className="h-2 rounded-full bg-gradient-to-r from-indigo-500 to-violet-500"
              style={{ width: `${completion}%` }}
            />
          </div>
          <Link
            href={`${base}/settings`}
            className="mt-4 inline-flex rounded-lg border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-50"
          >
            {completion === 0 ? "Add brand profile" : "Edit brand profile"}
          </Link>
        </section>
      </div>

      <WorkspaceSwitcher currentId={workspaceId} />
    </div>
  );
}
