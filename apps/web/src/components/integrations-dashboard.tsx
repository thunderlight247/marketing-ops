"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type Workspace, type WorkspacesState } from "@/lib/workspaces";
import { WorkspaceIntegrationsSection } from "@/components/workspace-integrations-section";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";

export function IntegrationsDashboard() {
  const [workspacesState, setWorkspacesState] = useState<WorkspacesState>({ kind: "loading" });
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string>("");

  const apiBaseUrl = getApiBaseUrl();

  useEffect(() => {
    async function load() {
      const res = await fetchWorkspaces(apiBaseUrl);
      setWorkspacesState(res);
      if (res.kind === "loaded" && res.workspaces.length > 0) {
        setSelectedWorkspaceId(res.workspaces[0].id);
      }
    }
    void load();
  }, [apiBaseUrl]);

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">Integrations</h1>
        <p className="mt-1 text-sm text-slate-500">
          Connections to client websites, CMS platforms, and publishing channels.
        </p>
      </div>

      {workspacesState.kind === "loading" && (
        <div className="flex items-center gap-2 py-8">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
          <p className="text-sm text-slate-500">Loading workspaces…</p>
        </div>
      )}

      {workspacesState.kind === "unavailable" && (
        <div className="rounded-xl border border-red-100 bg-red-50 px-5 py-4">
          <p className="text-sm font-medium text-red-800">Could not load workspaces</p>
          <p className="mt-1 text-sm text-slate-600">
            Check that the API is reachable and try refreshing.
          </p>
        </div>
      )}

      {workspacesState.kind === "loaded" && workspacesState.workspaces.length === 0 && (
        <div className="rounded-lg border border-dashed border-slate-300 bg-white p-8 text-center">
          <h2 className="text-base font-semibold text-slate-800">No workspaces yet</h2>
          <p className="mt-1 text-sm text-slate-600">
            You need at least one client workspace before connecting websites or CMSs.
          </p>
          <div className="mt-4">
            <Link
              href="/workspaces"
              className="inline-flex rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
            >
              Go to Workspaces
            </Link>
          </div>
        </div>
      )}

      {workspacesState.kind === "loaded" && workspacesState.workspaces.length > 0 && (
        <div className="space-y-6">
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <label htmlFor="workspace-select" className="block text-sm font-medium text-slate-700">
              Active workspace
            </label>
            <div className="mt-1 sm:max-w-xs">
              <select
                id="workspace-select"
                value={selectedWorkspaceId}
                onChange={(e) => setSelectedWorkspaceId(e.target.value)}
                className={inputClasses}
              >
                {workspacesState.workspaces.map((ws: Workspace) => (
                  <option key={ws.id} value={ws.id}>
                    {ws.name} ({ws.slug})
                  </option>
                ))}
              </select>
            </div>
            <p className="mt-1 text-xs text-slate-500">
              Integrations and credentials belong strictly to the selected workspace.
            </p>
          </div>

          {selectedWorkspaceId && (
            <WorkspaceIntegrationsSection
              key={selectedWorkspaceId}
              workspaceId={selectedWorkspaceId}
            />
          )}
        </div>
      )}
    </div>
  );
}
