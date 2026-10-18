"use client";

import { useCallback, useEffect, useState } from "react";
import { EmptyState } from "@/components/empty-state";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchWorkspaceDetail,
  saveBrandProfile,
  type SaveBrandResult,
  type WorkspaceDetailState,
} from "@/lib/workspaces";

type FieldKey = "businessDescription" | "audience" | "services" | "tone";

const FIELD_LABELS: Record<FieldKey, { label: string; hint: string; textarea?: boolean }> = {
  businessDescription: {
    label: "Business description",
    hint: "What the business does, for whom, and what makes it different.",
    textarea: true,
  },
  audience: { label: "Audience", hint: "Who this business speaks to." },
  services: { label: "Services", hint: "Offered products or services." },
  tone: { label: "Brand tone", hint: "How content should sound." },
};

const inputClasses =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";
const labelClasses = "block text-sm font-medium text-slate-700";

function linesToTextarea(lines: string[] | null): string {
  return lines ? lines.join("\n") : "";
}

function EmptyBrandNotice() {
  return (
    <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15">
      This brand profile is empty. Fill in what you know; everything is optional but the more
      complete it is, the better the AI-assisted drafting (Milestone 4) works.
    </p>
  );
}

export function WorkspaceDetailPanel({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const [state, setState] = useState<WorkspaceDetailState>({ kind: "loading" });
  const [fields, setFields] = useState<Record<FieldKey, string>>({
    businessDescription: "",
    audience: "",
    services: "",
    tone: "",
  });
  const [prohibited, setProhibited] = useState("");
  const [ctas, setCtas] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    const loaded = await fetchWorkspaceDetail(apiBaseUrl, workspaceId);
    if (loaded.kind === "loaded") {
      const brand = loaded.brand;
      setFields({
        businessDescription: brand?.businessDescription ?? "",
        audience: brand?.audience ?? "",
        services: brand?.services ?? "",
        tone: brand?.tone ?? "",
      });
      setProhibited(linesToTextarea(brand?.prohibitedClaims ?? null));
      setCtas(linesToTextarea(brand?.callsToAction ?? null));
      setSavedAt(brand ? brand.updatedAt : null);
    }
    setState(loaded);
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;
      const asLines = (text: string) =>
        text
          .split("\n")
          .map((line) => line.trim())
          .filter((line) => line.length > 0);
      const nullable = (text: string) => (text.trim().length === 0 ? null : text.trim());
      const input = {
        businessDescription: nullable(fields.businessDescription),
        audience: nullable(fields.audience),
        services: nullable(fields.services),
        tone: nullable(fields.tone),
        rules: state.brand?.rules ?? {},
        prohibitedClaims: asLines(prohibited),
        callsToAction: asLines(ctas),
      };
      setSaving(true);
      setSaveError(null);
      const result: SaveBrandResult = await saveBrandProfile(apiBaseUrl, workspaceId, input);
      setSaving(false);
      if (result.kind === "saved") {
        setSavedAt(result.brand.updatedAt);
        setSaveError(null);
        setState({ kind: "loaded", workspace: state.workspace, brand: result.brand });
      } else {
        setSaveError(result.kind === "rejected" ? result.message : result.reason);
      }
    },
    [apiBaseUrl, workspaceId, state, fields, prohibited, ctas],
  );

  if (state.kind === "loading") {
    return <p className="text-sm text-slate-600">Loading workspace…</p>;
  }
  if (state.kind === "missing") {
    return (
      <div>
        <EmptyState title="Workspace not found" body={state.reason} />
      </div>
    );
  }
  if (state.kind === "unavailable") {
    return (
      <div className="rounded-lg border border-slate-200 bg-white px-5 py-4">
        <p className="text-sm font-medium text-red-900">Could not load this workspace.</p>
        <p className="mt-2 text-sm text-slate-800">{state.reason}</p>
        <button
          type="button"
          onClick={() => void load()}
          className="mt-3 rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-800 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
        >
          Try again
        </button>
      </div>
    );
  }

  const { workspace } = state;
  return (
    <div className="grid gap-6">
      <section
        aria-labelledby="workspace-facts"
        className="rounded-lg border border-slate-200 bg-white px-5 py-4"
      >
        <h2 id="workspace-facts" className="text-base font-semibold">
          {workspace.name}
        </h2>
        <dl className="mt-3 grid gap-2 text-sm sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-4">
          <dt className="font-medium text-slate-700">Slug</dt>
          <dd>
            <code className="rounded bg-slate-100 px-1 font-mono text-xs">{workspace.slug}</code>
          </dd>
          <dt className="font-medium text-slate-700">Website</dt>
          <dd className="min-w-0">
            {workspace.websiteUrl ? (
              <a
                href={workspace.websiteUrl}
                className="underline underline-offset-2 hover:text-teal-900"
              >
                {workspace.websiteUrl}
              </a>
            ) : (
              <span className="text-slate-500">not set</span>
            )}
          </dd>
          <dt className="font-medium text-slate-700">Status</dt>
          <dd>
            <span className="rounded-full bg-emerald-50 px-2.5 py-0.5 text-sm font-medium text-emerald-900 ring-1 ring-inset ring-emerald-700/25">
              {workspace.status}
            </span>
          </dd>
        </dl>
      </section>

      <section
        aria-labelledby="brand-profile-heading"
        className="rounded-lg border border-slate-200 bg-white px-5 py-4"
      >
        <div className="flex flex-wrap items-baseline justify-between gap-3 border-b border-slate-200 pb-4">
          <h2 id="brand-profile-heading" className="text-base font-semibold">
            Brand profile
          </h2>
          <p className="text-xs text-slate-500" aria-live="polite">
            {savedAt ? `Last saved ${new Date(savedAt).toLocaleString()}` : "Not saved yet"}
          </p>
        </div>

        {state.brand === null ? (
          <div className="mt-4">
            <EmptyBrandNotice />
          </div>
        ) : null}

        <form onSubmit={(event) => void save(event)} className="mt-4 grid gap-5">
          {(Object.keys(FIELD_LABELS) as FieldKey[]).map((key) => {
            const meta = FIELD_LABELS[key];
            return (
              <div key={key}>
                <label htmlFor={`brand-${key}`} className={labelClasses}>
                  {meta.label}
                </label>
                {meta.textarea ? (
                  <textarea
                    id={`brand-${key}`}
                    rows={3}
                    value={fields[key]}
                    onChange={(event) => setFields((f) => ({ ...f, [key]: event.target.value }))}
                    className={`mt-1 ${inputClasses}`}
                  />
                ) : (
                  <input
                    id={`brand-${key}`}
                    value={fields[key]}
                    onChange={(event) => setFields((f) => ({ ...f, [key]: event.target.value }))}
                    className={`mt-1 ${inputClasses}`}
                  />
                )}
                <p className="mt-1 text-xs text-slate-500">{meta.hint}</p>
              </div>
            );
          })}

          <div>
            <label htmlFor="brand-prohibited" className={labelClasses}>
              Prohibited claims <span className="font-normal text-slate-500">(one per line)</span>
            </label>
            <textarea
              id="brand-prohibited"
              rows={2}
              value={prohibited}
              onChange={(event) => setProhibited(event.target.value)}
              placeholder={"no health guarantees\nno price claims"}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div>
            <label htmlFor="brand-ctas" className={labelClasses}>
              Calls to action <span className="font-normal text-slate-500">(one per line)</span>
            </label>
            <textarea
              id="brand-ctas"
              rows={2}
              value={ctas}
              onChange={(event) => setCtas(event.target.value)}
              placeholder={"Book a taste test"}
              className={`mt-1 ${inputClasses}`}
            />
          </div>

          {saveError ? (
            <p
              role="alert"
              className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
            >
              {saveError}
            </p>
          ) : null}

          <div>
            <button
              type="submit"
              disabled={saving}
              className="rounded-md bg-teal-800 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {saving ? "Saving…" : "Save brand profile"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
