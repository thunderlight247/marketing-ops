"use client";

import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchAgentRegistry,
  formatAgentAction,
  formatAgentContract,
  formatAgentDataNeed,
  type AgentsRegistryState,
} from "@/lib/agents";

// WorkspaceAgentsSection shows which agency agents are enabled on the server
// and exactly what each one may do. It reads the registry's descriptor list,
// so a new specialist appears here without a dashboard change.
export function WorkspaceAgentsSection() {
  const [state, setState] = useState<AgentsRegistryState>({ kind: "loading" });

  const apiBaseUrl = getApiBaseUrl();

  useEffect(() => {
    let cancelled = false;
    void fetchAgentRegistry(apiBaseUrl).then((loaded) => {
      if (!cancelled) setState(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl]);

  if (state.kind === "available" && state.agents.length === 0) {
    return null;
  }

  return (
    <section
      aria-labelledby="agents-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <h2
          id="agents-heading"
          className="text-xs font-semibold uppercase tracking-widest text-slate-500"
        >
          Enabled agents (registry)
        </h2>
      </div>
      <div className="px-4 py-5 sm:px-5">
        <p className="max-w-prose text-sm text-slate-600">
          Agents are curated prompt profiles, not tools: they advise or draft within the bounds
          listed here, and every capability beyond that is a human decision through the ordinary
          flows.
        </p>

        {state.kind === "loading" ? (
          <p className="mt-4 text-sm text-slate-500">Loading agents…</p>
        ) : state.kind === "unavailable" ? (
          <p className="mt-4 text-sm text-slate-500">{state.message}</p>
        ) : (
          <ul className="mt-4 grid gap-3">
            {state.agents.map((agent) => (
              <li
                key={agent.id}
                className="rounded-xl border border-slate-100 bg-slate-50 px-4 py-4"
              >
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <span className="text-sm font-semibold text-slate-900">{agent.displayName}</span>
                  <span className="rounded bg-slate-200 px-1.5 py-0.5 font-mono text-[10px] text-slate-600">
                    {agent.id}
                  </span>
                  <span className="text-[11px] text-slate-500">v{agent.version}</span>
                </div>
                {agent.description ? (
                  <p className="mt-1 max-w-prose text-sm text-slate-600">{agent.description}</p>
                ) : null}
                <ul className="mt-2 grid gap-1 text-xs text-slate-500">
                  <li>{formatAgentContract(agent.outputContract)}</li>
                  {agent.dataNeeds.map((need) => (
                    <li key={need}>Reads: {formatAgentDataNeed(need)}</li>
                  ))}
                  {agent.actions.map((action) => (
                    <li key={action}>{formatAgentAction(action)}</li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
