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

function WorkspaceList({ workspaces }: { workspaces: Workspace[] }) {
  return (
    <ul className="divide-y divide-slate-200">
      {workspaces.map((workspace) => (
        <li key={workspace.id} className="flex flex-wrap items-baseline justify-between gap-2 py-3">
          <div className="min-w-0">
            <p className="truncate font-medium text-slate-900">
              <Link
                href={`/workspaces/${encodeURIComponent(workspace.id)}`}
                className="underline-offset-2 hover:text-teal-900 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
              >
                {workspace.name}
              </Link>
            </p>
            <p className="mt-0.5 text-sm text-slate-600">
              <code className="rounded bg-slate-100 px-1 font-mono text-xs">{workspace.slug}</code>
              {workspace.websiteUrl ? (
                <>
                  {" · "}
                  <a
                    href={workspace.websiteUrl}
                    className="underline underline-offset-2 hover:text-teal-900"
                  >
                    {workspace.websiteUrl}
                  </a>
                </>
              ) : null}
            </p>
          </div>
          <span className="rounded-full bg-emerald-50 px-2.5 py-0.5 text-sm font-medium text-emerald-900 ring-1 ring-inset ring-emerald-700/25">
            {workspace.status}
          </span>
        </li>
      ))}
    </ul>
  );
}

const inputClasses =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";
const labelClasses = "block text-sm font-medium text-slate-700";

export function WorkspacesPanel() {
  const [state, setState] = useState<WorkspacesState>({ kind: "loading" });
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [websiteUrl, setWebsiteUrl] = useState("");
  const [formErrors, setFormErrors] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
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
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <section
        aria-labelledby="workspaces-list-heading"
        className="rounded-lg border border-slate-200 bg-white"
      >
        <div className="flex items-center justify-between gap-4 border-b border-slate-200 px-5 py-4">
          <h2 id="workspaces-list-heading" className="text-base font-semibold">
            Your client workspaces
          </h2>
          <button
            type="button"
            onClick={() => void refresh()}
            disabled={state.kind === "loading"}
            className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-800 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60"
          >
            Refresh
          </button>
        </div>
        <div aria-live="polite" className="px-5">
          {state.kind === "loading" ? (
            <p className="py-6 text-sm text-slate-600">Loading workspaces…</p>
          ) : null}
          {state.kind === "unavailable" ? (
            <div className="py-5">
              <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                <span className="inline-flex items-center gap-1.5 rounded-full bg-red-50 px-2.5 py-0.5 text-sm font-medium text-red-900 ring-1 ring-inset ring-red-700/25">
                  <span aria-hidden="true" className="size-1.5 rounded-full bg-red-600" />
                  List unavailable
                </span>
              </p>
              <p className="mt-3 text-sm text-slate-800">{state.reason}</p>
            </div>
          ) : null}
          {state.kind === "loaded" ? (
            state.workspaces.length === 0 ? (
              <div className="py-6">
                <EmptyState
                  title="No workspaces yet"
                  body="Create a workspace for the first client business you manage. Nothing else can be scoped until a workspace exists."
                />
              </div>
            ) : (
              <WorkspaceList workspaces={state.workspaces} />
            )
          ) : null}
        </div>
      </section>

      <section
        aria-labelledby="workspaces-create-heading"
        className="rounded-lg border border-slate-200 bg-white px-5 py-4"
      >
        <h2 id="workspaces-create-heading" className="text-base font-semibold">
          Add a business
        </h2>
        <p className="mt-1 text-xs text-slate-600">
          A workspace is the private container for one business. Its ID is generated automatically;
          you only choose a display name, URL slug, and optional website.
        </p>
        <form onSubmit={(event) => void submit(event)} className="mt-4 grid gap-4">
          <div>
            <label htmlFor="workspace-slug" className={labelClasses}>
              Workspace URL slug
            </label>
            <input
              id="workspace-slug"
              value={slug}
              onChange={(event) => setSlug(event.target.value)}
              placeholder="demo-studio"
              autoComplete="off"
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div>
            <label htmlFor="workspace-name" className={labelClasses}>
              Business name
            </label>
            <input
              id="workspace-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Demo Studio"
              autoComplete="organization"
              maxLength={140}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div>
            <label htmlFor="workspace-website" className={labelClasses}>
              Website URL <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="workspace-website"
              type="url"
              value={websiteUrl}
              onChange={(event) => setWebsiteUrl(event.target.value)}
              placeholder="https://demo-studio.example"
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          {formErrors.length > 0 ? (
            <ul role="alert" className="list-disc space-y-1 pl-5 text-sm text-red-700">
              {formErrors.map((message) => (
                <li key={message}>{message}</li>
              ))}
            </ul>
          ) : null}
          <button
            type="submit"
            disabled={submitting || state.kind === "loading"}
            className="rounded-md bg-teal-800 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? "Creating…" : "Create workspace"}
          </button>
        </form>
        <p className="mt-3 text-xs text-slate-500">
          Add more businesses whenever you need; each business gets its own content and publishing
          settings.
        </p>
      </section>
    </div>
  );
}
