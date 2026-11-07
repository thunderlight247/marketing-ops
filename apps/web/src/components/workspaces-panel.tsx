"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { EmptyState } from "@/components/empty-state";
import { getApiBaseUrl } from "@/lib/config";
import { listWhatsAppSessions, type WhatsAppSession } from "@/lib/leadEngine";
import {
  createWorkspace,
  describeDeliveryAttention,
  fetchWorkspaces,
  validateWorkspaceInput,
  type CreateWorkspaceResult,
  type Workspace,
  type WorkspacesState,
} from "@/lib/workspaces";

const STATUS_STYLES: Record<string, string> = {
  active: "bg-emerald-50 text-emerald-700 ring-emerald-600/20",
  paused: "bg-amber-50 text-amber-700 ring-amber-600/20",
  archived: "bg-slate-100 text-slate-500 ring-slate-400/20",
};

function WorkspaceAvatar({ name }: { name: string }) {
  return (
    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-indigo-100 text-xs font-bold uppercase text-indigo-700">
      {name.slice(0, 2)}
    </span>
  );
}

function WorkspaceList({
  workspaces,
  sessions,
}: {
  workspaces: Workspace[];
  sessions: WhatsAppSession[] | null;
}) {
  return (
    <ul className="divide-y divide-slate-100">
      {workspaces.map((workspace) => (
        <li
          key={workspace.id}
          className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-4 py-3 sm:px-5 sm:py-3.5 hover:bg-slate-50 transition-colors"
        >
          <div className="flex min-w-0 items-center gap-3">
            <WorkspaceAvatar name={workspace.name} />
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-slate-900">
                <Link
                  href={`/workspaces/${encodeURIComponent(workspace.id)}`}
                  className="hover:text-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
                >
                  {workspace.name}
                </Link>
              </p>
              <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-xs text-slate-400">
                <code className="rounded bg-slate-100 px-1 font-mono">{workspace.slug}</code>
                {workspace.websiteUrl ? (
                  <>
                    <span>·</span>
                    <a
                      href={workspace.websiteUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="truncate max-w-[12rem] sm:max-w-[18rem] hover:underline hover:text-indigo-600"
                    >
                      {workspace.websiteUrl}
                    </a>
                  </>
                ) : null}
              </p>
              {sessions !== null ? (
                <p className="mt-1">
                  {(() => {
                    const session = sessions.find((s) => s.workspaceId === workspace.id);
                    if (!session) {
                      return (
                        <span className="inline-flex items-center gap-1.5 rounded-full bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-500">
                          <span className="h-1.5 w-1.5 rounded-full bg-slate-400" aria-hidden="true" />
                          No WhatsApp session
                        </span>
                      );
                    }
                    return (
                      <span
                        className={`inline-flex flex-wrap items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${
                          session.connected
                            ? "bg-emerald-50 text-emerald-800 ring-emerald-600/20"
                            : "bg-slate-100 text-slate-600 ring-slate-400/20"
                        }`}
                      >
                        <span
                          className={`h-1.5 w-1.5 rounded-full ${
                            session.connected ? "bg-emerald-500" : "bg-slate-400"
                          }`}
                          aria-hidden="true"
                        />
                        {session.source === "own" ? "Own number" : "Shared number"}
                        <span className="text-slate-400" aria-hidden="true">
                          ·
                        </span>
                        <span className="font-mono font-normal text-slate-500">
                          {session.phone || "not paired"}
                        </span>
                        <span className="text-slate-400">
                          · {session.connected ? "connected" : "offline"}
                        </span>
                      </span>
                    );
                  })()}
                </p>
              ) : null}
              {workspace.deliveryAttention ? (
                <p className="mt-1">
                  <Link
                    href={`/workspaces/${encodeURIComponent(workspace.id)}/email#email-deliveries`}
                    data-testid={`workspace-delivery-attention-${workspace.id}`}
                    className="inline-flex items-center gap-1.5 rounded-full bg-rose-50 px-2 py-0.5 text-[11px] font-semibold text-rose-700 ring-1 ring-rose-200 hover:bg-rose-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-rose-500"
                  >
                    <span aria-hidden className="h-1.5 w-1.5 rounded-full bg-rose-500" />
                    Email needs attention: {describeDeliveryAttention(workspace.deliveryAttention)}
                  </Link>
                </p>
              ) : null}
              {workspace.deliveryAcknowledgedAt ? (
                <p
                  data-testid={`workspace-delivery-acknowledged-${workspace.id}`}
                  className="mt-1 text-[11px] text-slate-400"
                >
                  Last acknowledged {new Date(workspace.deliveryAcknowledgedAt).toLocaleString()}
                  {workspace.deliveryAttention
                    ? " — the flag shows what failed or bounced since."
                    : " — nothing new has failed or bounced since."}
                </p>
              ) : null}
            </div>
          </div>
          <div className="flex items-center justify-between sm:justify-end gap-2 pt-1 sm:pt-0 border-t sm:border-t-0 border-slate-100">
            <span
              className={`rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ring-inset capitalize ${STATUS_STYLES[workspace.status] ?? STATUS_STYLES.active}`}
            >
              {workspace.status}
            </span>
            <Link
              href={`/workspaces/${encodeURIComponent(workspace.id)}`}
              target="_blank"
              rel="noreferrer"
              title="Open in a new tab — workspaces in other tabs keep running"
              className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:border-indigo-300 hover:text-indigo-700 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
            >
              Open ↗
            </Link>
          </div>
        </li>
      ))}
    </ul>
  );
}

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold text-slate-600 uppercase tracking-wide";

export function WorkspacesPanel() {
  const [state, setState] = useState<WorkspacesState>({ kind: "loading" });
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [websiteUrl, setWebsiteUrl] = useState("");
  const [formErrors, setFormErrors] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [successMsg, setSuccessMsg] = useState("");
  // null = pairing status unavailable (dispatcher down or still starting).
  const [sessions, setSessions] = useState<WhatsAppSession[] | null>(null);
  const apiBaseUrl = getApiBaseUrl();

  const loadSessions = useCallback(async () => {
    try {
      const res = await listWhatsAppSessions(apiBaseUrl);
      setSessions(res.sessions ?? []);
    } catch {
      setSessions(null);
    }
  }, [apiBaseUrl]);

  const refresh = useCallback(async () => {
    setState({ kind: "loading" });
    const [next] = await Promise.all([fetchWorkspaces(apiBaseUrl), loadSessions()]);
    setState(next);
  }, [apiBaseUrl, loadSessions]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const connectedCount = sessions?.filter((s) => s.connected).length ?? null;

  const submit = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      setSuccessMsg("");
      const input = { slug, name, websiteUrl };
      const problems = validateWorkspaceInput(input);
      setFormErrors(problems);
      if (problems.length > 0) return;

      setSubmitting(true);
      const result: CreateWorkspaceResult = await createWorkspace(apiBaseUrl, input);
      setSubmitting(false);
      if (result.kind === "created") {
        setSlug("");
        setName("");
        setWebsiteUrl("");
        setFormErrors([]);
        setSuccessMsg(`"${result.workspace.name}" workspace created.`);
        await refresh();
      } else if (result.kind === "rejected") {
        setFormErrors([result.message]);
      } else {
        setFormErrors([result.reason]);
      }
    },
    [apiBaseUrl, slug, name, websiteUrl, refresh],
  );

  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_20rem]">
      {/* Workspace list */}
      <section
        aria-labelledby="workspaces-list-heading"
        className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
      >
        <div className="flex items-center justify-between gap-4 border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2
            id="workspaces-list-heading"
            className="text-xs font-semibold uppercase tracking-widest text-slate-500"
          >
            Connected workspaces
            {state.kind === "loaded" && (
              <span className="ml-2 rounded-full bg-slate-200 px-2 py-0.5 text-[10px] font-bold text-slate-600">
                {state.workspaces.length}
              </span>
            )}
            {connectedCount !== null && connectedCount > 0 && (
              <span className="ml-2 inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-[10px] font-bold text-emerald-700">
                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" aria-hidden="true" />
                {connectedCount} WhatsApp live
              </span>
            )}
          </h2>
          <button
            type="button"
            onClick={() => void refresh()}
            disabled={state.kind === "loading"}
            className="text-xs font-medium text-slate-500 hover:text-slate-800 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
          >
            {state.kind === "loading" ? "Loading…" : "Refresh"}
          </button>
        </div>

        <div aria-live="polite">
          {state.kind === "loading" ? (
            <div className="flex items-center gap-2 px-5 py-6">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
              <p className="text-sm text-slate-500">Loading workspaces…</p>
            </div>
          ) : null}

          {state.kind === "unavailable" ? (
            <div className="px-5 py-5">
              <p className="text-sm text-red-700 font-medium">Could not load workspaces</p>
              <p className="mt-1 text-sm text-slate-600">
                The API is not reachable. Check that the API server is running and try refreshing.
              </p>
            </div>
          ) : null}

          {state.kind === "loaded" ? (
            state.workspaces.length === 0 ? (
              <div className="px-5 py-8">
                <EmptyState
                  title="No workspaces yet"
                  body="Create a workspace for each client you manage. Each workspace keeps its content, integrations, and publishing history completely separate."
                  icon={
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
                  }
                />
              </div>
            ) : (
              <WorkspaceList workspaces={state.workspaces} sessions={sessions} />
            )
          ) : null}
        </div>
      </section>

      {/* Create workspace form */}
      <section
        aria-labelledby="workspaces-create-heading"
        className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
      >
        <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2
            id="workspaces-create-heading"
            className="text-xs font-semibold uppercase tracking-widest text-slate-500"
          >
            Add another workspace
          </h2>
        </div>
        <div className="px-5 py-5">
          <p className="text-xs leading-relaxed text-slate-500">
            Each workspace is a private container for one client. Choose a display name, URL slug,
            and optional website — its unique ID is generated automatically.
          </p>
          <form onSubmit={(event) => void submit(event)} className="mt-5 grid gap-4">
            <div>
              <label htmlFor="workspace-name" className={labelClasses}>
                Business name
              </label>
              <input
                id="workspace-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Acme Nutrition"
                autoComplete="organization"
                maxLength={140}
                className={`mt-1.5 ${inputClasses}`}
              />
            </div>
            <div>
              <label htmlFor="workspace-slug" className={labelClasses}>
                URL slug
              </label>
              <input
                id="workspace-slug"
                value={slug}
                onChange={(event) => setSlug(event.target.value)}
                placeholder="acme-nutrition"
                autoComplete="off"
                className={`mt-1.5 ${inputClasses}`}
              />
              <p className="mt-1 text-[11px] text-slate-400">
                Lowercase letters, numbers, and hyphens only.
              </p>
            </div>
            <div>
              <label htmlFor="workspace-website" className={labelClasses}>
                Website <span className="font-normal normal-case text-slate-400">(optional)</span>
              </label>
              <input
                id="workspace-website"
                type="url"
                value={websiteUrl}
                onChange={(event) => setWebsiteUrl(event.target.value)}
                placeholder="https://acmenutrition.com"
                className={`mt-1.5 ${inputClasses}`}
              />
            </div>

            {formErrors.length > 0 ? (
              <ul
                role="alert"
                className="rounded-lg bg-red-50 px-4 py-3 text-xs text-red-700 space-y-1 list-disc pl-7"
              >
                {formErrors.map((message) => (
                  <li key={message}>{message}</li>
                ))}
              </ul>
            ) : null}

            {successMsg ? (
              <p
                role="status"
                className="rounded-lg bg-emerald-50 px-3 py-2 text-xs font-medium text-emerald-700"
              >
                ✓ {successMsg}
              </p>
            ) : null}

            <button
              type="submit"
              disabled={submitting || state.kind === "loading"}
              className="rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              {submitting ? "Adding…" : "Add workspace"}
            </button>
          </form>
        </div>
      </section>
    </div>
  );
}
