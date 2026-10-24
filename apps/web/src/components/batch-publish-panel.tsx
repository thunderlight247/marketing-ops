"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchWorkspaceContent,
  pushToSanity,
  type ContentItem,
  type SanityPublishResult,
} from "@/lib/content";
import { fetchWorkspaceIntegrations, type Integration } from "@/lib/integrations";
import { fetchWorkspaces, type Workspace, type WorkspacesState } from "@/lib/workspaces";

interface BatchEntry {
  workspace: Workspace;
  item: ContentItem;
  sanityIntegrations: Integration[];
}

interface BatchResult {
  name: string;
  result: SanityPublishResult;
}

function entryKey(workspaceId: string, itemId: string): string {
  return `${workspaceId}:${itemId}`;
}

export function BatchPublishPanel() {
  const apiBaseUrl = getApiBaseUrl();
  const [workspaces, setWorkspaces] = useState<WorkspacesState>({ kind: "loading" });
  const [entries, setEntries] = useState<BatchEntry[]>([]);
  const [loadingEntries, setLoadingEntries] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [integrationByEntry, setIntegrationByEntry] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, BatchResult>>({});
  const [publishing, setPublishing] = useState(false);

  const load = useCallback(async () => {
    setLoadError(null);
    setLoadingEntries(true);
    const workspaceResult = await fetchWorkspaces(apiBaseUrl);
    setWorkspaces(workspaceResult);
    if (workspaceResult.kind !== "loaded") {
      setEntries([]);
      setLoadingEntries(false);
      return;
    }

    const workspaceResults = await Promise.all(
      workspaceResult.workspaces.map(async (workspace) => {
        const [content, integrations] = await Promise.all([
          fetchWorkspaceContent(apiBaseUrl, workspace.id),
          fetchWorkspaceIntegrations(apiBaseUrl, workspace.id),
        ]);
        if (content.kind !== "loaded") {
          const message =
            content.kind === "missing" || content.kind === "unavailable"
              ? content.message
              : "Content is still loading.";
          return { workspace, error: message, entries: [] as BatchEntry[] };
        }
        const sanityIntegrations =
          integrations.kind === "loaded"
            ? integrations.integrations.filter(
                (integration) =>
                  integration.adapterKind === "sanity" &&
                  integration.hasCredentials &&
                  integration.status !== "disconnected",
              )
            : [];
        return {
          workspace,
          error: null,
          entries: content.items.map((item) => ({ workspace, item, sanityIntegrations })),
        };
      }),
    );
    const failures = workspaceResults.filter((result) => result.error !== null);
    setLoadError(
      failures.length > 0
        ? `Could not load content for ${failures.map((result) => result.workspace.name).join(", ")}. Refresh to try again.`
        : null,
    );
    const loadedEntries = workspaceResults.flatMap((result) => result.entries);
    setEntries(loadedEntries);
    setIntegrationByEntry((current) => {
      const next: Record<string, string> = {};
      for (const entry of loadedEntries) {
        const key = entryKey(entry.workspace.id, entry.item.id);
        const options = entry.sanityIntegrations;
        const currentChoice = current[key];
        next[key] = options.some((integration) => integration.id === currentChoice)
          ? currentChoice
          : (options[0]?.id ?? "");
      }
      return next;
    });
    setSelected({});
    setResults({});
    setLoadingEntries(false);
  }, [apiBaseUrl]);

  useEffect(() => {
    void load();
  }, [load]);

  const approvedEntries = useMemo(
    () => entries.filter((entry) => entry.item.status === "approved"),
    [entries],
  );
  const selectedEntries = approvedEntries.filter((entry) => {
    const key = entryKey(entry.workspace.id, entry.item.id);
    return (
      selected[key] &&
      entry.sanityIntegrations.some((integration) => integration.id === integrationByEntry[key])
    );
  });

  async function publishSelected() {
    if (selectedEntries.length === 0 || publishing) return;
    const approvedForConfirmation = selectedEntries
      .map(({ workspace, item }) => `• ${workspace.name}: ${item.title}`)
      .join("\n");
    if (
      !window.confirm(
        `Publish these ${selectedEntries.length} approved post(s) to their selected business destinations?\n\n${approvedForConfirmation}\n\nThis starts one independent publish per post.`,
      )
    ) {
      return;
    }

    setPublishing(true);
    const outcomes = await Promise.all(
      selectedEntries.map(async ({ workspace, item }) => {
        const key = entryKey(workspace.id, item.id);
        const result = await pushToSanity(
          apiBaseUrl,
          workspace.id,
          item.id,
          integrationByEntry[key],
          "publish",
        );
        return [key, { name: `${workspace.name}: ${item.title}`, result }] as const;
      }),
    );
    setResults(Object.fromEntries(outcomes));
    setPublishing(false);
  }

  return (
    <section
      aria-labelledby="batch-publish-heading"
      className="mt-6 rounded-lg border border-slate-200 bg-white"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 px-5 py-4">
        <div>
          <h2 id="batch-publish-heading" className="text-base font-semibold text-slate-900">
            Publish approved posts across businesses
          </h2>
          <p className="mt-1 text-xs text-slate-600">
            Select distinct approved posts and their business-specific Sanity destinations. They
            start together, but each has its own result.
          </p>
        </div>
        <button
          type="button"
          onClick={() => void load()}
          disabled={loadingEntries || publishing}
          className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-800 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {loadingEntries ? "Refreshing…" : "Refresh posts"}
        </button>
      </div>

      <div className="px-5 py-4">
        {workspaces.kind === "loading" || loadingEntries ? (
          <p className="text-sm text-slate-500" aria-live="polite">
            Loading approved posts…
          </p>
        ) : null}
        {workspaces.kind === "unavailable" ? (
          <p className="text-sm text-red-800" role="alert">
            Could not load businesses: {workspaces.reason}
          </p>
        ) : null}
        {loadError ? (
          <p className="mb-3 text-sm text-red-800" role="alert">
            {loadError}
          </p>
        ) : null}
        {workspaces.kind === "loaded" && workspaces.workspaces.length === 0 ? (
          <div className="rounded-md border border-dashed border-slate-300 p-4 text-sm text-slate-600">
            Add a business before preparing posts.{" "}
            <Link className="font-medium text-teal-800 underline" href="/workspaces">
              Create a workspace
            </Link>
            .
          </div>
        ) : null}
        {!loadingEntries &&
        workspaces.kind === "loaded" &&
        workspaces.workspaces.length > 0 &&
        approvedEntries.length === 0 ? (
          <p className="rounded-md bg-slate-50 p-4 text-sm text-slate-600">
            No approved posts are ready. Create and approve content in a business workspace; drafts
            are never included here.
          </p>
        ) : null}

        {approvedEntries.length > 0 ? (
          <ul className="divide-y divide-slate-100">
            {approvedEntries.map(({ workspace, item, sanityIntegrations }) => {
              const key = entryKey(workspace.id, item.id);
              const result = results[key];
              const integrationId = integrationByEntry[key] ?? "";
              const canPublish = sanityIntegrations.length > 0;
              return (
                <li
                  key={key}
                  className="grid gap-3 py-4 sm:grid-cols-[auto_minmax(0,1fr)_minmax(13rem,17rem)] sm:items-start"
                >
                  <input
                    type="checkbox"
                    aria-label={`Select ${item.title} from ${workspace.name}`}
                    checked={Boolean(selected[key])}
                    disabled={!canPublish || publishing}
                    onChange={(event) =>
                      setSelected((current) => ({ ...current, [key]: event.target.checked }))
                    }
                    className="mt-1 size-4 rounded border-slate-300 text-teal-800 focus:ring-teal-700 disabled:opacity-50"
                  />
                  <div className="min-w-0">
                    <p className="font-medium text-slate-900">{item.title}</p>
                    <p className="mt-0.5 text-xs text-slate-600">
                      {workspace.name} · {item.kind.replaceAll("_", " ")} · approved ·{" "}
                      {item.slug ? `/${item.slug}` : "no slug"}
                    </p>
                    {!canPublish ? (
                      <p className="mt-1 text-xs text-amber-800">
                        No Sanity integration with saved credentials.{" "}
                        <Link
                          className="underline"
                          href={`/workspaces/${encodeURIComponent(workspace.id)}`}
                        >
                          Configure this business
                        </Link>
                        .
                      </p>
                    ) : null}
                    {result ? (
                      <p
                        className={`mt-2 text-xs ${result.result.kind === "pushed" ? "text-emerald-800" : "text-red-800"}`}
                        role="status"
                      >
                        {result.result.kind === "pushed"
                          ? `${result.name}: ${result.result.success.message}`
                          : result.result.kind === "conflict" ||
                              result.result.kind === "failed" ||
                              result.result.kind === "unavailable"
                            ? `${result.name}: ${result.result.message}`
                            : `${result.name}: ${result.result.kind === "rejected" ? result.result.problems.join("; ") : "Request was rejected."}`}
                      </p>
                    ) : null}
                  </div>
                  <div>
                    {canPublish ? (
                      <label className="block text-xs font-medium text-slate-600">
                        Publish through
                        <select
                          value={integrationId}
                          disabled={publishing}
                          onChange={(event) =>
                            setIntegrationByEntry((current) => ({
                              ...current,
                              [key]: event.target.value,
                            }))
                          }
                          className="mt-1 w-full rounded-md border border-slate-300 px-2 py-2 text-sm text-slate-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
                        >
                          {sanityIntegrations.map((integration) => (
                            <option key={integration.id} value={integration.id}>
                              {integration.name}
                            </option>
                          ))}
                        </select>
                      </label>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        ) : null}

        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 pt-4">
          <p className="text-xs text-slate-500">
            {selectedEntries.length} selected · publishing sends separate requests concurrently;
            there is no all-or-nothing transaction.
          </p>
          <button
            type="button"
            onClick={() => void publishSelected()}
            disabled={selectedEntries.length === 0 || publishing || loadingEntries}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {publishing
              ? `Publishing ${selectedEntries.length} posts…`
              : `Publish ${selectedEntries.length} selected posts`}
          </button>
        </div>
      </div>
    </section>
  );
}
