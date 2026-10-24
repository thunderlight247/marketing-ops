"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { EmptyState } from "@/components/empty-state";
import { getApiBaseUrl } from "@/lib/config";
import {
  createWorkspace,
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

function WorkspaceList({ workspaces }: { workspaces: Workspace[] }) {
  return (
    <ul className="divide-y divide-slate-100">
      {workspaces.map((workspace) => (
        <li
          key={workspace.id}
          className="flex flex-wrap items-center justify-between gap-3 px-5 py-3.5 hover:bg-slate-50 transition-colors"
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
              <p className="mt-0.5 flex items-center gap-1.5 text-xs text-slate-400">
                <code className="rounded bg-slate-100 px-1 font-mono">{workspace.slug}</code>
                {workspace.websiteUrl ? (
                  <>
                    <span>·</span>
                    <a
                      href={workspace.websiteUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="truncate max-w-[14rem] hover:underline hover:text-indigo-600"
                    >
                      {workspace.websiteUrl}
                    </a>
                  </>
                ) : null}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span
              className={`rounded-full px-2.5 py-0.5 text-xs font-medium ring-1 ring-inset capitalize ${STATUS_STYLES[workspace.status] ?? STATUS_STYLES.active}`}
            >
              {workspace.status}
            </span>
            <Link
              href={`/workspaces/${encodeURIComponent(workspace.id)}`}
              className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:border-indigo-300 hover:text-indigo-700 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
            >
              Open →
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
  const apiBaseUrl = getApiBaseUrl();

  const refresh = useCallback(async () => {
    setState({ kind: "loading" });
    setState(await fetchWorkspaces(apiBaseUrl));
  }, [apiBaseUrl]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

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
          <h2 id="workspaces-list-heading" className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Client workspaces
            {state.kind === "loaded" && (
              <span className="ml-2 rounded-full bg-slate-200 px-2 py-0.5 text-[10px] font-bold text-slate-600">
                {state.workspaces.length}
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
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                      <polyline points="9 22 9 12 15 12 15 22" />
                    </svg>
                  }
                />
              </div>
            ) : (
              <WorkspaceList workspaces={state.workspaces} />
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
          <h2 id="workspaces-create-heading" className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            New workspace
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
              <p className="mt-1 text-[11px] text-slate-400">Lowercase letters, numbers, and hyphens only.</p>
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
              <ul role="alert" className="rounded-lg bg-red-50 px-4 py-3 text-xs text-red-700 space-y-1 list-disc pl-7">
                {formErrors.map((message) => (
                  <li key={message}>{message}</li>
                ))}
              </ul>
            ) : null}

            {successMsg ? (
              <p role="status" className="rounded-lg bg-emerald-50 px-3 py-2 text-xs font-medium text-emerald-700">
                ✓ {successMsg}
              </p>
            ) : null}

            <button
              type="submit"
              disabled={submitting || state.kind === "loading"}
              className="rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors"
            >
              {submitting ? "Creating…" : "Create workspace"}
            </button>
          </form>
        </div>
      </section>
    </div>
  );
}
