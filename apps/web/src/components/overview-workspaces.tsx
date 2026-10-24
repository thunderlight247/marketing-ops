"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type WorkspacesState } from "@/lib/workspaces";

export function OverviewWorkspaces() {
  const [state, setState] = useState<WorkspacesState>({ kind: "loading" });
  const apiBaseUrl = getApiBaseUrl();

  useEffect(() => {
    void fetchWorkspaces(apiBaseUrl).then(setState);
  }, [apiBaseUrl]);

  if (state.kind === "loading") {
    return (
      <section className="rounded-lg border border-slate-200 bg-white p-5" aria-live="polite">
        <p className="text-sm text-slate-600">Loading your businesses…</p>
      </section>
    );
  }

  if (state.kind === "unavailable") {
    return (
      <section className="rounded-lg border border-slate-200 bg-white p-5">
        <h2 className="text-base font-semibold text-slate-900">Your businesses</h2>
        <p className="mt-2 text-sm text-slate-600">
          Business workspaces are not available until the API and database are running.
        </p>
        <p className="mt-1 text-xs text-slate-500">{state.reason}</p>
      </section>
    );
  }

  if (state.workspaces.length === 0) {
    return (
      <section className="rounded-lg border border-teal-200 bg-teal-50 p-5 sm:p-6">
        <div className="max-w-2xl">
          <p className="text-xs font-semibold uppercase tracking-wide text-teal-800">
            Getting started
          </p>
          <h2 className="mt-1 text-lg font-semibold text-slate-900">
            Add the first business you manage
          </h2>
          <p className="mt-2 text-sm leading-relaxed text-slate-700">
            There are no business workspaces yet. Create one per business; each keeps its brand,
            integrations, drafts, approvals, and publishing history separate.
          </p>
          <Link
            href="/workspaces"
            className="mt-4 inline-flex items-center rounded-md bg-teal-800 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
          >
            Add a business
          </Link>
        </div>
      </section>
    );
  }

  return (
    <section
      aria-labelledby="overview-businesses-heading"
      className="rounded-lg border border-slate-200 bg-white"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-5 py-4">
        <div>
          <h2 id="overview-businesses-heading" className="text-base font-semibold text-slate-900">
            Your businesses <span className="text-slate-500">({state.workspaces.length})</span>
          </h2>
          <p className="mt-0.5 text-xs text-slate-600">
            Each business has isolated content and publishing connections.
          </p>
        </div>
        <Link
          href="/workspaces"
          className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-800 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
        >
          Manage businesses
        </Link>
      </div>
      <ul className="divide-y divide-slate-100 px-5">
        {state.workspaces.map((workspace) => (
          <li key={workspace.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
            <div className="min-w-0">
              <Link
                href={`/workspaces/${encodeURIComponent(workspace.id)}`}
                className="font-medium text-teal-900 underline-offset-2 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
              >
                {workspace.name}
              </Link>
              <p className="mt-0.5 text-xs text-slate-500">
                {workspace.slug.startsWith("demo-") ? (
                  <span className="mr-1.5 inline-flex rounded bg-amber-100 px-1.5 py-0.5 font-semibold uppercase tracking-wide text-amber-900">
                    Demo
                  </span>
                ) : null}
                {workspace.websiteUrl ?? `/${workspace.slug}`} · Configure publishing from this
                business&apos;s workspace.
              </p>
            </div>
            <Link
              href={`/workspaces/${encodeURIComponent(workspace.id)}`}
              className="text-sm font-medium text-teal-800 hover:underline"
            >
              Open workspace <span aria-hidden="true">→</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
