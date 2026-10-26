"use client";

import { useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { requestCampaignProposal, type CampaignProposalState } from "@/lib/agents";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

// WorkspaceCampaignStrategistSection asks the campaign_strategist specialist
// for one proposed campaign. The proposal is always filed "planned": the
// campaigns panel above lists it, and only the operator activates it.
export function WorkspaceCampaignStrategistSection({
  workspaceId,
  onProposed,
}: Readonly<{ workspaceId: string; onProposed: () => void }>) {
  const [state, setState] = useState<CampaignProposalState>({ kind: "idle" });
  const [focus, setFocus] = useState("");

  const apiBaseUrl = getApiBaseUrl();

  async function handlePropose(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState({ kind: "running" });
    const result = await requestCampaignProposal(apiBaseUrl, workspaceId, focus);
    if (result.kind === "done") {
      setFocus("");
      onProposed();
    }
    setState(result);
  }

  const running = state.kind === "running";

  return (
    <section
      aria-labelledby="campaign-strategist-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <h2
          id="campaign-strategist-heading"
          className="text-xs font-semibold uppercase tracking-widest text-slate-500"
        >
          Campaign proposal (agent)
        </h2>
      </div>
      <div className="px-4 py-5 sm:px-5">
        <p className="max-w-prose text-sm text-slate-600">
          The campaign-strategist specialist proposes one campaign — name, suggested UTM value, and
          rationale — grounded in this workspace&apos;s brand profile. It files the proposal as a
          <strong> planned</strong> campaign below: activating it, pausing it, or deleting it stays
          with you. The proposal never cites performance data it was not given.
        </p>

        <form onSubmit={(e) => void handlePropose(e)} className="mt-4 grid gap-3 sm:max-w-md">
          <label htmlFor="strategist-focus" className="text-sm font-medium text-slate-700">
            Focus (optional)
          </label>
          <textarea
            id="strategist-focus"
            value={focus}
            onChange={(event) => setFocus(event.target.value)}
            placeholder="e.g. a slow-season reminder push for our maintenance plan"
            rows={3}
            maxLength={2000}
            className={inputClasses}
          />
          <div>
            <button type="submit" disabled={running} className={PRIMARY_BUTTON}>
              {running ? "Strategist is planning…" : "Propose a campaign (agent)"}
            </button>
          </div>
        </form>

        {state.kind === "done" ? (
          <div className="mt-4 rounded-xl border border-emerald-100 bg-emerald-50 px-4 py-3 text-sm">
            <p className="font-semibold text-emerald-900">
              Proposed: {state.proposal.campaign.name}{" "}
              <span className="rounded bg-white px-2 py-0.5 text-xs font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">
                Planned
              </span>
            </p>
            {state.proposal.campaign.utmCampaign ? (
              <p className="mt-1 text-xs font-mono text-emerald-800">
                utm: {state.proposal.campaign.utmCampaign}
              </p>
            ) : null}
            <div
              className="mt-2 max-w-prose text-sm text-emerald-900"
              title={state.proposal.campaign.notes}
            >
              {state.proposal.campaign.notes.length > 400
                ? `${state.proposal.campaign.notes.slice(0, 400)}…`
                : state.proposal.campaign.notes}
            </div>
            {state.proposal.keyPoints.length > 0 ? (
              <ul className="mt-2 list-disc pl-5 text-xs text-emerald-800">
                {state.proposal.keyPoints.map((point) => (
                  <li key={point}>{point}</li>
                ))}
              </ul>
            ) : null}
            <p className="mt-2 text-xs text-emerald-800" aria-live="polite">
              Filed as a planned campaign in the campaigns list above ({state.proposal.durationMs}
              ms, {state.proposal.provider} {state.proposal.model}). Review it before activating.
            </p>
          </div>
        ) : null}

        {state.kind === "unavailable" ? (
          <p role="alert" className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
            {state.message}
          </p>
        ) : null}
      </div>
    </section>
  );
}
