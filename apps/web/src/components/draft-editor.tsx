"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  fetchWorkspaceDraft,
  formatContentKind,
  formatContentStatus,
  saveWorkspaceDraft,
  type DraftState,
  type SaveDraftResult,
} from "@/lib/content";

const inputClasses =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";
const labelClasses = "block text-sm font-medium text-slate-700";
const PRIMARY_BUTTON =
  "rounded-md bg-teal-800 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60";

function isoToLocalInput(iso: string): string {
  return iso.slice(0, 16); // "YYYY-MM-DDTHH:mm" for datetime-local
}

function localInputToIso(value: string): string {
  if (!value) return "";
  return `${value}:00Z`; // the form works in UTC; label it as such
}

export function DraftEditor({
  workspaceId,
  itemId,
}: Readonly<{ workspaceId: string; itemId: string }>) {
  const apiBaseUrl = getApiBaseUrl();
  const [state, setState] = useState<DraftState>({ kind: "loading" });

  // Form fields
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [plannedPublishAt, setPlannedPublishAt] = useState("");
  const [excerpt, setExcerpt] = useState("");
  const [body, setBody] = useState("");
  const [callToAction, setCallToAction] = useState("");
  const [channels, setChannels] = useState<Array<{ name: string; copy: string }>>([]);

  const [formErrors, setFormErrors] = useState<string[]>([]);
  const [serverError, setServerError] = useState<string | null>(null);
  const [conflict, setConflict] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    const result = await fetchWorkspaceDraft(apiBaseUrl, workspaceId, itemId);
    setState(result);
    if (result.kind === "loaded") {
      const { item, version } = result;
      setTitle(version?.title ?? item.title);
      setSlug(item.slug ?? "");
      setPlannedPublishAt(item.plannedPublishAt ? isoToLocalInput(item.plannedPublishAt) : "");
      setExcerpt(version?.excerpt ?? "");
      setBody(version?.body ?? "");
      setCallToAction(version?.callToAction ?? "");
      setChannels(
        Object.entries(version?.channelCopy ?? {}).map(([name, copy]) => ({ name, copy })),
      );
    }
  }, [apiBaseUrl, workspaceId, itemId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleSave(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setServerError(null);
    setConflict(null);
    setFormErrors([]);
    setSaving(true);

    const channelCopy: Record<string, string> = {};
    for (const channel of channels) {
      const name = channel.name.trim().toLowerCase();
      if (name) channelCopy[name] = channel.copy;
    }

    const result: SaveDraftResult = await saveWorkspaceDraft(apiBaseUrl, workspaceId, itemId, {
      title,
      slug,
      plannedPublishAt: localInputToIso(plannedPublishAt),
      excerpt,
      body,
      callToAction,
      channelCopy,
    });

    setSaving(false);
    if (result.kind === "saved") {
      setFormErrors([]);
      void load();
    } else if (result.kind === "rejected") {
      setFormErrors(result.problems);
    } else if (result.kind === "conflict") {
      setConflict(result.message);
    } else {
      setServerError(result.message);
    }
  }

  if (state.kind === "loading") {
    return <p className="py-8 text-center text-sm text-slate-500">Loading draft…</p>;
  }

  if (state.kind === "missing") {
    return (
      <div className="rounded-md bg-amber-50 p-4 ring-1 ring-inset ring-amber-700/15">
        <p className="text-sm font-medium text-amber-900">{state.message}</p>
        <Link
          href="/content"
          className="mt-2 inline-block text-sm font-medium text-amber-900 underline underline-offset-2 hover:no-underline"
        >
          Back to content
        </Link>
      </div>
    );
  }

  if (state.kind === "unavailable") {
    return (
      <div className="rounded-md bg-red-50 p-4 ring-1 ring-inset ring-red-700/15">
        <p className="text-sm font-medium text-red-900">{state.message}</p>
        <button
          type="button"
          onClick={() => void load()}
          className="mt-3 text-xs font-semibold text-red-900 underline hover:no-underline"
        >
          Try again
        </button>
      </div>
    );
  }

  const { item, version } = state;
  const editable = item.status === "draft";

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm text-slate-600">
            <Link
              href="/content"
              className="font-medium text-teal-800 underline-offset-2 hover:text-teal-900 hover:underline"
            >
              Content
            </Link>
            {" / "}
            <Link
              href={`/workspaces/${workspaceId}`}
              className="font-medium text-teal-800 underline-offset-2 hover:text-teal-900 hover:underline"
            >
              workspace
            </Link>
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <span className="inline-flex items-center rounded bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-800 ring-1 ring-inset ring-sky-700/20">
              {formatContentKind(item.kind)}
            </span>
            <span className="inline-flex items-center rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700 ring-1 ring-inset ring-slate-600/20">
              {formatContentStatus(item.status)}
            </span>
            <span className="text-xs text-slate-500">
              {version
                ? `Version ${version.versionNumber} · saved ${new Date(version.createdAt).toLocaleString()}`
                : "Never edited yet"}
            </span>
          </div>
        </div>
        <p className="max-w-sm rounded-md bg-emerald-50 px-3 py-2 text-xs font-medium text-emerald-900 ring-1 ring-inset ring-emerald-700/15">
          Draft only. Publishing requires your explicit approval later — nothing here goes out
          automatically.
        </p>
      </div>

      {!editable && (
        <p className="mb-4 rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15">
          Approved or published content is frozen. Editing and re-approval arrive with the approval
          workflow (Milestone 5).
        </p>
      )}

      {formErrors.length > 0 && (
        <ul role="alert" className="mb-4 list-disc space-y-1 pl-5 text-sm text-red-700">
          {formErrors.map((problem) => (
            <li key={problem}>{problem}</li>
          ))}
        </ul>
      )}
      {conflict && (
        <p
          role="alert"
          className="mb-4 rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15"
        >
          {conflict}
        </p>
      )}
      {serverError && (
        <div
          role="alert"
          className="mb-4 rounded-md bg-red-50 p-4 ring-1 ring-inset ring-red-700/15"
        >
          <p className="text-sm font-medium text-red-900">{serverError}</p>
          <button
            type="button"
            onClick={() => void load()}
            className="mt-3 text-xs font-semibold text-red-900 underline hover:no-underline"
          >
            Reload draft
          </button>
        </div>
      )}

      <form
        onSubmit={(e) => void handleSave(e)}
        className="rounded-lg border border-slate-200 bg-white"
      >
        <div className="space-y-4 p-5">
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="sm:col-span-2">
              <label htmlFor="draft-title" className={labelClasses}>
                Title
              </label>
              <input
                id="draft-title"
                type="text"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                maxLength={200}
                required
                disabled={!editable}
                className={`mt-1 ${inputClasses}`}
              />
            </div>
            <div>
              <label htmlFor="draft-slug" className={labelClasses}>
                Slug
              </label>
              <input
                id="draft-slug"
                type="text"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                disabled={!editable}
                className={`mt-1 ${inputClasses}`}
              />
            </div>
          </div>

          <div>
            <label htmlFor="draft-planned" className={labelClasses}>
              Suggested publish time{" "}
              <span className="font-normal text-slate-500">(UTC, optional)</span>
            </label>
            <input
              id="draft-planned"
              type="datetime-local"
              step={60}
              value={plannedPublishAt}
              onChange={(e) => setPlannedPublishAt(e.target.value)}
              disabled={!editable}
              className={`mt-1 sm:max-w-xs ${inputClasses}`}
            />
            <p className="mt-1 text-xs text-slate-500">
              A planning hint for the calendar and scheduler, not a commitment to publish.
            </p>
          </div>

          <div>
            <label htmlFor="draft-excerpt" className={labelClasses}>
              Excerpt <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <textarea
              id="draft-excerpt"
              rows={2}
              value={excerpt}
              onChange={(e) => setExcerpt(e.target.value)}
              disabled={!editable}
              className={`mt-1 ${inputClasses}`}
            />
          </div>

          <div>
            <label htmlFor="draft-body" className={labelClasses}>
              Body
            </label>
            <textarea
              id="draft-body"
              rows={12}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              disabled={!editable}
              className={`mt-1 font-mono text-[13px] ${inputClasses}`}
            />
          </div>

          <div>
            <label htmlFor="draft-cta" className={labelClasses}>
              Call to action <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="draft-cta"
              type="text"
              value={callToAction}
              onChange={(e) => setCallToAction(e.target.value)}
              disabled={!editable}
              className={`mt-1 ${inputClasses}`}
            />
          </div>

          <div>
            <div className="flex items-center justify-between">
              <span className={labelClasses}>Channel-specific copy</span>
              <button
                type="button"
                disabled={!editable}
                onClick={() => setChannels((prev) => [...prev, { name: "", copy: "" }])}
                className="rounded-md border border-slate-300 px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
              >
                + Add channel
              </button>
            </div>
            {channels.length === 0 ? (
              <p className="mt-1 text-xs text-slate-500">
                Per-channel variants (for example facebook, instagram) if this content is reshaped
                per platform.
              </p>
            ) : (
              <div className="mt-2 space-y-2">
                {channels.map((channel, index) => (
                  <div
                    key={`${channel.name}-${index}`}
                    className="grid gap-2 sm:grid-cols-[14rem_minmax(0,1fr)_auto]"
                  >
                    <input
                      type="text"
                      value={channel.name}
                      onChange={(e) =>
                        setChannels((prev) =>
                          prev.map((c, i) => (i === index ? { ...c, name: e.target.value } : c)),
                        )
                      }
                      disabled={!editable}
                      placeholder="channel name"
                      className={inputClasses}
                    />
                    <input
                      type="text"
                      value={channel.copy}
                      onChange={(e) =>
                        setChannels((prev) =>
                          prev.map((c, i) => (i === index ? { ...c, copy: e.target.value } : c)),
                        )
                      }
                      disabled={!editable}
                      placeholder="copy for this channel"
                      className={inputClasses}
                    />
                    <button
                      type="button"
                      disabled={!editable}
                      onClick={() => setChannels((prev) => prev.filter((_, i) => i !== index))}
                      className="rounded-md border border-slate-300 px-2.5 py-1.5 text-xs font-medium text-red-800 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      Remove
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-slate-200 bg-slate-50 px-5 py-4">
          <p className="text-xs text-slate-500">
            Every save stores a new immutable version; earlier versions remain readable through the
            database while history tooling is added in Milestone 5.
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => void load()}
              className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
            >
              Reset
            </button>
            <button type="submit" disabled={saving || !editable} className={PRIMARY_BUTTON}>
              {saving ? "Saving…" : "Save draft"}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
