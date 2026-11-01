"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { acknowledgeDeliveryAttention } from "@/lib/email";
import {
  describeDeliveryAttention,
  fetchWorkspaces,
  notifyDeliveryAttentionChanged,
  type Workspace,
  type WorkspacesState,
} from "@/lib/workspaces";
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
  const [acknowledgingId, setAcknowledgingId] = useState<string | null>(null);
  const [ackError, setAckError] = useState<string | null>(null);
  const [ackNotice, setAckNotice] = useState<string | null>(null);
  const apiBaseUrl = getApiBaseUrl();

  useEffect(() => {
    void fetchWorkspaces(apiBaseUrl).then(setState);
  }, [apiBaseUrl]);

  // Acknowledging says "I have dealt with what the log showed". The badge clears
  // for that workspace, but anything that fails or bounces later raises it
  // again, so a dismiss can never hide a new problem.
  async function handleAcknowledge(workspace: Workspace) {
    setAcknowledgingId(workspace.id);
    setAckError(null);
    setAckNotice(null);
    const result = await acknowledgeDeliveryAttention(apiBaseUrl, workspace.id);
    setAcknowledgingId(null);
    if (result.kind === "unavailable") {
      setAckError(result.message);
      return;
    }
    setState((prev) =>
      prev.kind === "loaded"
        ? {
            kind: "loaded",
            workspaces: prev.workspaces.map((item) =>
              item.id === workspace.id
                ? {
                    ...item,
                    deliveryAttention: undefined,
                    deliveryAcknowledgedAt: result.acknowledgedAt,
                  }
                : item,
            ),
          }
        : prev,
    );
    // The status panel above counts the same problems; let it re-read them.
    notifyDeliveryAttentionChanged();
    setAckNotice(
      `Email problems for ${workspace.name} are marked as seen. A new failure or bounce will show here again.`,
    );
  }

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
      {ackError ? (
        <p
          role="alert"
          className="border-b border-rose-100 bg-rose-50 px-5 py-2.5 text-sm text-rose-800"
        >
          {ackError}
        </p>
      ) : null}
      {ackNotice ? (
        <p
          role="status"
          className="border-b border-emerald-100 bg-emerald-50 px-5 py-2.5 text-sm text-emerald-800"
        >
          {ackNotice}
        </p>
      ) : null}
      <ul className="divide-y divide-slate-100">
        {state.workspaces.map((workspace) => (
          <li
            key={workspace.id}
            className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-4 py-3 sm:px-5 sm:py-3.5 hover:bg-slate-50 transition-colors"
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
                {workspace.deliveryAttention ? (
                  <div className="mt-1.5 flex flex-wrap items-center gap-2">
                    <Link
                      href={`/workspaces/${encodeURIComponent(workspace.id)}#email-deliveries`}
                      data-testid={`delivery-attention-${workspace.id}`}
                      className="inline-flex items-center gap-1.5 rounded-full bg-rose-50 px-2 py-0.5 text-[11px] font-semibold text-rose-700 ring-1 ring-rose-200 hover:bg-rose-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-rose-500"
                    >
                      <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-rose-500" />
                      Email needs attention:{" "}
                      {describeDeliveryAttention(workspace.deliveryAttention)}
                    </Link>
                    <button
                      type="button"
                      onClick={() => void handleAcknowledge(workspace)}
                      disabled={acknowledgingId !== null}
                      aria-label={`Mark the email problems for ${workspace.name} as seen`}
                      title="Clear this from the overview once you have dealt with it. A new failure or bounce will show again."
                      className="rounded px-1.5 py-0.5 text-[11px] font-medium text-slate-500 underline-offset-2 hover:text-slate-700 hover:underline disabled:cursor-not-allowed disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
                    >
                      {acknowledgingId === workspace.id ? "Dismissing…" : "Dismiss"}
                    </button>
                  </div>
                ) : null}
                {workspace.deliveryAcknowledgedAt ? (
                  <p
                    data-testid={`delivery-acknowledged-${workspace.id}`}
                    className="mt-1 text-[11px] text-slate-400"
                  >
                    Last acknowledged {new Date(workspace.deliveryAcknowledgedAt).toLocaleString()}
                    {workspace.deliveryAttention
                      ? " — the badge shows what failed or bounced since."
                      : " — nothing new has failed or bounced since."}
                  </p>
                ) : null}
              </div>
            </div>
            <Link
              href={`/workspaces/${encodeURIComponent(workspace.id)}`}
              className="self-start sm:self-center rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:border-indigo-300 hover:text-indigo-700 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
            >
              Open →
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
