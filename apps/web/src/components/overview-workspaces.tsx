"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type WorkspacesState } from "@/lib/workspaces";
import { EmptyState } from "@/components/empty-state";

function WorkspaceIcon() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
      <polyline points="9 22 9 12 15 12 15 22" />
    </svg>
  );
}

export function OverviewWorkspaces() {
  const [state, setState] = useState<WorkspacesState>({ kind: "loading" });
  const apiBaseUrl = getApiBaseUrl();

  useEffect(() => {
    void fetchWorkspaces(apiBaseUrl).then(setState);
  }, [apiBaseUrl]);

  if (state.kind === "loading") {
    return (
      <section
        className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
        aria-live="polite"
      >
        <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Workspaces
          </h2>
        </div>
        <div className="flex items-center gap-2 px-5 py-6">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
          <p className="text-sm text-slate-500">Loading workspaces…</p>
        </div>
      </section>
    );
  }

  if (state.kind === "unavailable") {
    return (
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Workspaces
          </h2>
        </div>
        <div className="px-5 py-6">
          <p className="text-sm text-slate-600">
            Workspaces could not be loaded. Check that the API is reachable and try refreshing.
          </p>
        </div>
      </section>
    );
  }

  if (state.workspaces.length === 0) {
    return (
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Workspaces
          </h2>
        </div>
        <div className="px-5 py-6">
          <EmptyState
            title="No workspaces yet"
            body="Add your first workspace to start managing content for a client. Each workspace keeps its brand, integrations, and publishing history separate."
            icon={<WorkspaceIcon />}
            action={
              <Link
                href="/workspaces"
                className="inline-flex items-center rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
              >
                Add a workspace
              </Link>
            }
          />
        </div>
      </section>
    );
  }

  return (
    <section
      aria-labelledby="overview-workspaces-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 bg-slate-50 px-5 py-3">
        <h2
          id="overview-workspaces-heading"
          className="text-xs font-semibold uppercase tracking-widest text-slate-500"
        >
          Workspaces{" "}
          <span className="ml-1 rounded-full bg-slate-200 px-2 py-0.5 text-[10px] font-bold text-slate-600">
            {state.workspaces.length}
          </span>
        </h2>
        <Link
          href="/workspaces"
          className="text-xs font-medium text-indigo-600 hover:text-indigo-800 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
        >
          Manage all →
        </Link>
      </div>
      <ul className="divide-y divide-slate-100">
        {state.workspaces.map((workspace) => (
          <li
            key={workspace.id}
            className="flex flex-wrap items-center justify-between gap-3 px-5 py-3.5 hover:bg-slate-50 transition-colors"
          >
            <div className="flex min-w-0 items-center gap-3">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-indigo-100 text-indigo-600 text-xs font-bold uppercase">
                {workspace.name.slice(0, 2)}
              </span>
              <div className="min-w-0">
                <Link
                  href={`/workspaces/${encodeURIComponent(workspace.id)}`}
                  className="text-sm font-semibold text-slate-900 hover:text-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
                >
                  {workspace.name}
                </Link>
                <p className="mt-0.5 text-xs text-slate-400">
                  {workspace.websiteUrl ?? `/${workspace.slug}`}
                </p>
              </div>
            </div>
            <Link
              href={`/workspaces/${encodeURIComponent(workspace.id)}`}
              className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:border-indigo-300 hover:text-indigo-700 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
            >
              Open →
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
