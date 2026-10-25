"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { getApiBaseUrl } from "@/lib/config";
import { WorkspaceContentSection } from "@/components/workspace-content-section";
import { WorkspaceIntegrationsSection } from "@/components/workspace-integrations-section";
import { WorkspaceCampaignsSection } from "@/components/workspace-campaigns-section";
import { WorkspaceReportsSection } from "@/components/workspace-reports-section";
import { WorkspaceSeoAdvisorSection } from "@/components/workspace-seo-advisor-section";
import { WorkspaceContentCreatorSection } from "@/components/workspace-content-creator-section";
import {
  deleteWorkspace,
  fetchWorkspaceDetail,
  saveBrandProfile,
  updateWorkspaceFacts,
  validateWorkspaceFacts,
  type DeleteWorkspaceResult,
  type SaveBrandResult,
  type WorkspaceDetailState,
  type WorkspaceFactsInput,
  type WorkspaceFactsResult,
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
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";

function linesOf(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

function nullable(text: string): string | null {
  const trimmed = text.trim();
  return trimmed.length === 0 ? null : trimmed;
}

function linesToTextarea(lines: string[] | null): string {
  return lines ? lines.join("\n") : "";
}

function EmptyBrandNotice() {
  return (
    <p className="rounded-xl bg-amber-50 border border-amber-100 px-4 py-3 text-sm text-amber-800">
      This brand profile is empty. Fill in what you know — everything is optional but the more
      complete it is, the better AI-assisted drafting works.
    </p>
  );
}

const PRIMARY_BUTTON =
  "inline-flex items-center justify-center rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors";
const DANGER_BUTTON =
  "inline-flex items-center justify-center rounded-lg bg-red-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-red-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors";

export function WorkspaceDetailPanel({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const router = useRouter();
  const [contentRevision, setContentRevision] = useState(0);
  const [state, setState] = useState<WorkspaceDetailState>({ kind: "loading" });

  // Facts form state.
  const [name, setName] = useState("");
  const [websiteUrl, setWebsiteUrl] = useState("");
  const [status, setStatus] = useState<"active" | "archived">("active");
  const [factsErrors, setFactsErrors] = useState<string[]>([]);
  const [factsSaving, setFactsSaving] = useState(false);

  // Brand form state.
  const [fields, setFields] = useState<Record<FieldKey, string>>({
    businessDescription: "",
    audience: "",
    services: "",
    tone: "",
  });
  const [prohibited, setProhibited] = useState("");
  const [ctas, setCtas] = useState("");
  const [brandSaving, setBrandSaving] = useState(false);
  const [brandError, setBrandError] = useState<string | null>(null);
  const [brandSavedAt, setBrandSavedAt] = useState<string | null>(null);

  // Danger zone state.
  const [confirmSlug, setConfirmSlug] = useState("");
  const [deleting, setDeleting] = useState(false);

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    const loaded = await fetchWorkspaceDetail(apiBaseUrl, workspaceId);
    if (loaded.kind === "loaded") {
      const brand = loaded.brand;
      setName(loaded.workspace.name);
      setWebsiteUrl(loaded.workspace.websiteUrl ?? "");
      setStatus(loaded.workspace.status === "archived" ? "archived" : "active");
      setFields({
        businessDescription: brand?.businessDescription ?? "",
        audience: brand?.audience ?? "",
        services: brand?.services ?? "",
        tone: brand?.tone ?? "",
      });
      setProhibited(linesToTextarea(brand?.prohibitedClaims ?? null));
      setCtas(linesToTextarea(brand?.callsToAction ?? null));
      setBrandSavedAt(brand ? brand.updatedAt : null);
    }
    setState(loaded);
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  const saveFacts = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;
      const input: WorkspaceFactsInput = {
        name,
        websiteUrl: websiteUrl.trim().length === 0 ? null : websiteUrl.trim(),
        status,
      };
      const problems = validateWorkspaceFacts(input);
      setFactsErrors(problems);
      if (problems.length > 0) return;

      setFactsSaving(true);
      const result: WorkspaceFactsResult = await updateWorkspaceFacts(
        apiBaseUrl,
        workspaceId,
        input,
      );
      setFactsSaving(false);
      if (result.kind === "saved") {
        setFactsErrors([]);
        setState({ kind: "loaded", workspace: result.workspace, brand: state.brand });
        void load(); // refresh facts from the server's canonical row
      } else if (result.kind === "rejected") {
        setFactsErrors([result.message]);
      } else {
        setFactsErrors([result.kind === "missing" ? result.reason : result.reason]);
      }
    },
    [apiBaseUrl, workspaceId, state, name, websiteUrl, status, load],
  );

  const saveBrand = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;
      const input = {
        businessDescription: nullable(fields.businessDescription),
        audience: nullable(fields.audience),
        services: nullable(fields.services),
        tone: nullable(fields.tone),
        rules: state.brand?.rules ?? {},
        prohibitedClaims: linesOf(prohibited),
        callsToAction: linesOf(ctas),
      };
      setBrandSaving(true);
      setBrandError(null);
      const result: SaveBrandResult = await saveBrandProfile(apiBaseUrl, workspaceId, input);
      setBrandSaving(false);
      if (result.kind === "saved") {
        setBrandSavedAt(result.brand.updatedAt);
        setBrandError(null);
        setState({ kind: "loaded", workspace: state.workspace, brand: result.brand });
      } else {
        setBrandError(result.kind === "rejected" ? result.message : result.reason);
      }
    },
    [apiBaseUrl, workspaceId, state, fields, prohibited, ctas],
  );

  const remove = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;
      setDeleting(true);
      const result: DeleteWorkspaceResult = await deleteWorkspace(
        apiBaseUrl,
        workspaceId,
        confirmSlug,
      );
      setDeleting(false);
      if (result.kind === "deleted") {
        router.push("/workspaces");
      } else {
        setFactsErrors([result.kind === "rejected" ? result.message : result.reason]);
      }
    },
    [apiBaseUrl, workspaceId, state, confirmSlug, router],
  );

  if (state.kind === "loading") {
    return <p className="text-sm text-slate-600">Loading workspace…</p>;
  }
  if (state.kind === "missing") {
    return (
      <div>
        <p className="rounded-lg border border-dashed border-slate-300 bg-white px-6 py-10 text-center">
          <span className="block font-semibold text-slate-900">Workspace not found</span>
          <span className="mt-2 block text-sm text-slate-600">{state.reason}</span>
          <button
            type="button"
            onClick={() => router.push("/workspaces")}
            className="mt-4 inline-flex items-center justify-center rounded-lg border border-slate-200 px-4 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors"
          >
            Back to workspaces
          </button>
        </p>
      </div>
    );
  }
  if (state.kind === "unavailable") {
    return (
      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <p className="text-sm font-medium text-red-900">Could not load this workspace.</p>
        <p className="mt-2 text-sm text-slate-800">{state.reason}</p>
        <button
          type="button"
          onClick={() => void load()}
          className="mt-3 inline-flex items-center justify-center rounded-lg border border-slate-200 px-4 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors"
        >
          Try again
        </button>
      </div>
    );
  }

  const { workspace } = state;
  const confirmMatches = confirmSlug.length > 0 && confirmSlug === workspace.slug;

  return (
    <div className="grid gap-5">
      <section
        aria-labelledby="workspace-facts"
        className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
      >
        <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
          <h2 id="workspace-facts" className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Workspace details
          </h2>
        </div>
        <form onSubmit={(event) => void saveFacts(event)} className="grid gap-4 px-4 py-5 sm:px-5">
          <div>
            <label htmlFor="facts-name" className={labelClasses}>
              Business name
            </label>
            <input
              id="facts-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={140}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="facts-website" className={labelClasses}>
                Website URL
              </label>
              <input
                id="facts-website"
                type="url"
                value={websiteUrl}
                onChange={(event) => setWebsiteUrl(event.target.value)}
                className={`mt-1 ${inputClasses}`}
              />
            </div>
            <div>
              <label htmlFor="facts-status" className={labelClasses}>
                Status
              </label>
              <select
                id="facts-status"
                value={status}
                onChange={(event) =>
                  setStatus(event.target.value === "archived" ? "archived" : "active")
                }
                className={`mt-1 ${inputClasses}`}
              >
                <option value="active">Active</option>
                <option value="archived">Archived</option>
              </select>
            </div>
          </div>
          <p className="text-xs text-slate-500">
            The slug <code className="rounded bg-slate-100 px-1 font-mono">{workspace.slug}</code>{" "}
            is permanent and used in links.
          </p>
          {factsErrors.length > 0 ? (
            <ul role="alert" className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800 list-disc pl-5 space-y-1">
              {factsErrors.map((message) => (
                <li key={message}>{message}</li>
              ))}
            </ul>
          ) : null}
          <div className="flex">
            <button type="submit" disabled={factsSaving} className={`w-full sm:w-auto ${PRIMARY_BUTTON}`}>
              {factsSaving ? "Saving…" : "Save details"}
            </button>
          </div>
        </form>
      </section>

      <section
        aria-labelledby="brand-profile-heading"
        className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
      >
        <div className="flex flex-wrap items-baseline justify-between gap-3 border-b border-slate-200 pb-4">
          <h2 id="brand-profile-heading" className="text-base font-semibold">
            Brand profile
          </h2>
          <p className="text-xs text-slate-500" aria-live="polite">
            {brandSavedAt
              ? `Last saved ${new Date(brandSavedAt).toLocaleString()}`
              : "Not saved yet"}
          </p>
        </div>

        {state.brand === null ? (
          <div className="px-4 pt-4 sm:px-5">
            <EmptyBrandNotice />
          </div>
        ) : null}

        <form onSubmit={(event) => void saveBrand(event)} className="grid gap-5 px-4 py-5 sm:px-5">
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

          {brandError ? (
            <p
              role="alert"
              className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800"
            >
              {brandError}
            </p>
          ) : null}

          <div className="flex">
            <button type="submit" disabled={brandSaving} className={`w-full sm:w-auto ${PRIMARY_BUTTON}`}>
              {brandSaving ? "Saving…" : "Save brand profile"}
            </button>
          </div>
        </form>
      </section>

      <WorkspaceContentSection key={contentRevision} workspaceId={workspace.id} />

      <WorkspaceContentCreatorSection
        workspaceId={workspace.id}
        onCreated={() => setContentRevision((revision) => revision + 1)}
      />

      <WorkspaceIntegrationsSection workspaceId={workspace.id} />

      <WorkspaceCampaignsSection workspaceId={workspace.id} />

      <WorkspaceReportsSection workspaceId={workspace.id} />

      <WorkspaceSeoAdvisorSection workspaceId={workspace.id} />

      <section
        aria-labelledby="danger-zone-heading"
        className="overflow-hidden rounded-xl border border-red-100 bg-white shadow-sm"
      >
        <div className="border-b border-red-100 bg-red-50 px-4 py-3 sm:px-5">
          <h2 id="danger-zone-heading" className="text-xs font-semibold uppercase tracking-widest text-red-700">
            Danger zone
          </h2>
        </div>
        <div className="px-4 py-5 sm:px-5">
          <p className="max-w-prose text-sm text-red-800">
            Deleting <strong>{workspace.name}</strong> permanently removes its integrations, brand
            profile, content, and publishing history. Archived is usually the safer choice.
          </p>
          <form onSubmit={(event) => void remove(event)} className="mt-4 grid gap-3 sm:max-w-sm">
            <label htmlFor="delete-confirm" className={labelClasses}>
              Type <code className="rounded bg-slate-100 px-1 font-mono text-xs">{workspace.slug}</code>{" "}
              to confirm
            </label>
            <input
              id="delete-confirm"
              value={confirmSlug}
              onChange={(event) => setConfirmSlug(event.target.value)}
              autoComplete="off"
              className={inputClasses}
            />
            <div>
              <button type="submit" disabled={deleting || !confirmMatches} className={DANGER_BUTTON}>
                {deleting ? "Deleting…" : "Delete workspace forever"}
              </button>
            </div>
          </form>
        </div>
      </section>
    </div>
  );
}
