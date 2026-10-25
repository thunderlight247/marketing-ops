"use client";

import { useState } from "react";
import Link from "next/link";
import { getApiBaseUrl } from "@/lib/config";
import { contentKinds, requestContentDraft, type CreatorDraftState } from "@/lib/agents";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

export function WorkspaceContentCreatorSection({
  workspaceId,
  onCreated,
}: Readonly<{ workspaceId: string; onCreated: () => void }>) {
  const [state, setState] = useState<CreatorDraftState>({ kind: "idle" });
  const [kind, setKind] = useState("blog_article");
  const [focus, setFocus] = useState("");

  const apiBaseUrl = getApiBaseUrl();

  async function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState({ kind: "running" });
    const result = await requestContentDraft(apiBaseUrl, workspaceId, kind, focus);
    setState(result);
    if (result.kind === "done") onCreated();
  }

  const running = state.kind === "running";

  return (
    <section
      aria-labelledby="content-creator-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <h2 id="content-creator-heading" className="text-xs font-semibold uppercase tracking-widest text-slate-500">
          Content creator (agent draft)
        </h2>
      </div>
      <div className="px-4 py-5 sm:px-5">
      <p className="max-w-prose text-sm text-slate-600">
        The content-creator specialist writes one original draft from this workspace&apos;s brand
        profile. It lands as a draft in the content section above and follows the ordinary approval
        workflow: nothing is published without human review.
      </p>

      <form onSubmit={(e) => void handleCreate(e)} className="mt-4 grid gap-3 sm:max-w-md">
        <label htmlFor="creator-kind" className="text-sm font-medium text-slate-700">
          Content kind
        </label>
        <select
          id="creator-kind"
          value={kind}
          onChange={(event) => setKind(event.target.value)}
          className={inputClasses}
        >
          {contentKinds.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <label htmlFor="creator-focus" className="text-sm font-medium text-slate-700">
          Brief (optional)
        </label>
        <textarea
          id="creator-focus"
          value={focus}
          onChange={(event) => setFocus(event.target.value)}
          placeholder="e.g. a hiring-season post about our winter maintenance plan"
          rows={3}
          maxLength={2000}
          className={inputClasses}
        />
        <div>
          <button type="submit" disabled={running} className={PRIMARY_BUTTON}>
            {running ? "Creator is writing…" : "Create agent draft"}
          </button>
        </div>
      </form>

      {state.kind === "unavailable" && (
        <p role="alert" className="mt-4 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {state.message}
        </p>
      )}

      {state.kind === "done" && <DraftView state={state} workspaceId={workspaceId} />}

      {state.kind === "idle" && (
        <div className="mt-4 rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-600">
          No agent draft has been created yet.
        </div>
      )}
      </div>
    </section>
  );
}

function DraftView({
  state,
  workspaceId,
}: {
  state: Extract<CreatorDraftState, { kind: "done" }>;
  workspaceId: string;
}) {
  const { draft } = state;
  return (
    <div className="mt-4 rounded-xl border border-indigo-200 bg-indigo-50/40 p-4 text-sm text-slate-800">
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-600">
        <span className="rounded bg-white px-2 py-0.5 ring-1 ring-inset ring-slate-300">
          {draft.item.kind.replace(/_/g, " ")} · {draft.item.status}
        </span>
        <span className="rounded bg-white px-2 py-0.5 ring-1 ring-inset ring-slate-300">
          {draft.agentId} · {draft.provider}/{draft.model}
        </span>
        {draft.durationMs > 0 && (
          <span className="rounded bg-white px-2 py-0.5 ring-1 ring-inset ring-slate-300">
            {(draft.durationMs / 1000).toFixed(1)}s
          </span>
        )}
      </div>

      <p className="mt-3 font-medium text-slate-900">
        {draft.item.title}
        <span className="ml-2 text-xs font-normal text-slate-500">
          version {draft.version.versionNumber}
        </span>
      </p>

      {draft.version.excerpt !== null && draft.version.excerpt !== "" && (
        <p className="mt-1 text-slate-700">{draft.version.excerpt}</p>
      )}

      {draft.keyPoints.length > 0 && (
        <ul className="mt-2 list-disc pl-5 text-slate-700">
          {draft.keyPoints.map((point, index) => (
            <li key={index}>{point}</li>
          ))}
        </ul>
      )}

      {draft.version.body !== null && (
        <details className="mt-3">
          <summary className="cursor-pointer text-xs font-semibold uppercase tracking-wide text-slate-500">
            Draft body
          </summary>
          <pre className="mt-2 max-h-72 overflow-auto whitespace-pre-wrap rounded-md bg-white px-3 py-2 text-xs text-slate-700 ring-1 ring-inset ring-slate-200">
            {draft.version.body}
          </pre>
        </details>
      )}

      <p className="mt-3 text-xs text-slate-500">
        Saved as draft.{" "}
        <Link
          className="underline"
          href={`/content/${encodeURIComponent(workspaceId)}/${encodeURIComponent(draft.item.id)}`}
        >
          Review and approve draft
        </Link>
        .
      </p>
    </div>
  );
}
