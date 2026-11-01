"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { requestChannelCopy, type ChannelCopyState } from "@/lib/agents";
import { fetchWorkspaceContent, type ContentItem } from "@/lib/content";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

const channelLabels: Record<string, string> = {
  facebook: "Facebook",
  instagram: "Instagram",
  x: "X",
  youtube: "YouTube",
};

// WorkspaceChannelCopySection runs the channel-copy writer on one existing
// draft content item. The specialist adapts the item's own text into
// per-channel copy variants; the server files them into that item's newest
// version, merged over its previous channels. The item stays a draft and its
// approval state is untouched: review and approval stay with you.
export function WorkspaceChannelCopySection({
  workspaceId,
  onFiled,
}: Readonly<{ workspaceId: string; onFiled: () => void }>) {
  const [items, setItems] = useState<ContentItem[] | null>(null);
  const [itemsError, setItemsError] = useState<string | null>(null);
  const [itemId, setItemId] = useState("");
  const [focus, setFocus] = useState("");
  const [state, setState] = useState<ChannelCopyState>({ kind: "idle" });

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setItemsError(null);
    const list = await fetchWorkspaceContent(apiBaseUrl, workspaceId);
    if (list.kind === "loaded") {
      // Only drafts accept filed copy: the picker lists exactly those.
      const drafts = list.items.filter((item) => item.status === "draft");
      setItems(drafts);
      setItemId((current) =>
        current !== "" && drafts.some((item) => item.id === current) ? current : "",
      );
    } else if (list.kind === "unavailable" || list.kind === "missing") {
      setItems([]);
      setItemsError(list.message);
    }
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleRun(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState({ kind: "running" });
    const result = await requestChannelCopy(apiBaseUrl, workspaceId, itemId, focus);
    if (result.kind === "done") {
      setFocus("");
      onFiled();
    }
    setState(result);
  }

  const running = state.kind === "running";
  const drafts = items ?? [];
  const pickerUnavailable = itemsError ?? null;

  return (
    <section
      aria-labelledby="channel-copy-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <h2
          id="channel-copy-heading"
          className="text-xs font-semibold uppercase tracking-widest text-slate-500"
        >
          Channel copy (agent)
        </h2>
      </div>
      <div className="px-4 py-5 sm:px-5">
        <p className="max-w-prose text-sm text-slate-600">
          The channel-copy writer adapts one of your <strong>draft</strong> items into per-channel
          copy variants — Facebook, Instagram, X, YouTube — grounded in the item&apos;s own text and
          this workspace&apos;s brand profile. The variants are filed into that item as a new
          version, merged over its previous channel copy; the item stays a draft and nothing is
          published, scheduled, or approved.
        </p>

        {pickerUnavailable ? (
          <p role="alert" className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
            {pickerUnavailable}
          </p>
        ) : null}

        {drafts.length === 0 && !pickerUnavailable ? (
          <p className="mt-4 text-sm text-slate-500">
            No draft items yet. Create a draft in the content list above first — the writer only
            files copy into drafts.
          </p>
        ) : null}

        {drafts.length > 0 ? (
          <form onSubmit={(e) => void handleRun(e)} className="mt-4 grid gap-3 sm:max-w-md">
            <label htmlFor="channel-copy-item" className="text-sm font-medium text-slate-700">
              Draft item
            </label>
            <select
              id="channel-copy-item"
              value={itemId}
              onChange={(event) => setItemId(event.target.value)}
              className={inputClasses}
            >
              <option value="">Pick a draft item…</option>
              {drafts.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.title} ({item.kind})
                </option>
              ))}
            </select>
            <label htmlFor="channel-copy-focus" className="text-sm font-medium text-slate-700">
              Focus (optional)
            </label>
            <textarea
              id="channel-copy-focus"
              value={focus}
              onChange={(event) => setFocus(event.target.value)}
              placeholder="e.g. emphasize the booking deadline"
              rows={2}
              maxLength={2000}
              className={inputClasses}
            />
            <div>
              <button type="submit" disabled={running || itemId === ""} className={PRIMARY_BUTTON}>
                {running ? "Writing channel copy…" : "Write channel copy (agent)"}
              </button>
            </div>
          </form>
        ) : null}

        {state.kind === "done" ? (
          <div className="mt-4 rounded-xl border border-emerald-100 bg-emerald-50 px-4 py-3 text-sm">
            <p className="font-semibold text-emerald-900">
              Filed into: {state.copy.item.title}
              <span className="ml-2 rounded bg-white px-2 py-0.5 text-xs font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">
                version {state.copy.version.versionNumber}
              </span>
            </p>
            <dl className="mt-2 grid gap-2">
              {Object.entries(state.copy.channels).map(([channel, message]) => (
                <div key={channel}>
                  <dt className="text-xs font-semibold uppercase tracking-wide text-emerald-700">
                    {channelLabels[channel] ?? channel}
                  </dt>
                  <dd className="text-sm text-emerald-900">{message}</dd>
                </div>
              ))}
            </dl>
            <p className="mt-2 text-xs text-emerald-800" aria-live="polite">
              Saved as a new version, merged over previous channel copy. The item stays a draft —
              review and approve it in the content list ({state.copy.durationMs} ms,{" "}
              {state.copy.provider} {state.copy.model}).
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
