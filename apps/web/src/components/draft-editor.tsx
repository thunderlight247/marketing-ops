"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Image from "next/image";
import { getApiBaseUrl } from "@/lib/config";
import {
  cancelContentJob,
  decideContentApproval,
  fetchContentHistory,
  fetchContentJobs,
  fetchWorkspaceDraft,
  formatContentKind,
  formatContentStatus,
  formatPublishJobStatus,
  pushToSanity,
  retryContentJob,
  saveWorkspaceDraft,
  transitionContentStatus,
  uploadIntegrationImage,
  type ApprovalDecision,
  type ContentHistoryState,
  type ContentJobsState,
  type DraftState,
  type SanityPublishMode,
  type SanityPublishResult,
  type SaveDraftResult,
} from "@/lib/content";
import { fetchWorkspaceIntegrations, type Integration } from "@/lib/integrations";
import { buildImageMarker, buildYoutubeMarker, youtubeEmbedUrl } from "@/lib/markers";
import { inlineSegments, parseDraftBody } from "@/lib/preview";
import { evaluateSeo, type SeoCheckStatus } from "@/lib/seo";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";
const PRIMARY_BUTTON =
  "inline-flex items-center justify-center rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors";

function isoToLocalInput(iso: string): string {
  return iso.slice(0, 16); // "YYYY-MM-DDTHH:mm" for datetime-local
}

function localInputToIso(value: string): string {
  if (!value) return "";
  return `${value}:00Z`; // the form works in UTC; label it as such
}

// Inline renders plain text segments; **bold** markers become <strong>. Text
// stays in React text nodes — nothing is injected as raw HTML, so no crafted
// body text can execute script in the preview.
function Inline({ text }: Readonly<{ text: string }>) {
  const segments = inlineSegments(text);
  return (
    <>
      {segments.map((segment, index) =>
        segment.bold ? (
          <strong key={index} className="font-semibold text-slate-900">
            {segment.text}
          </strong>
        ) : (
          <span key={index}>{segment.text}</span>
        ),
      )}
    </>
  );
}

// PreviewBody renders the parsed draft body with hand-rolled typography (no
// markdown library): headings, paragraphs, lists, rules, inline bold.
function PreviewBody({ body }: Readonly<{ body: string }>) {
  const blocks = parseDraftBody(body);
  if (blocks.length === 0) {
    return <p className="text-sm text-slate-500">Nothing to preview yet.</p>;
  }
  return (
    <div className="space-y-3">
      {blocks.map((block, index) => {
        switch (block.type) {
          case "h1":
            return (
              <h2 key={index} className="pt-1 text-xl font-semibold tracking-tight text-slate-900">
                <Inline text={block.text} />
              </h2>
            );
          case "h2":
            return (
              <h3 key={index} className="pt-1 text-lg font-semibold text-slate-900">
                <Inline text={block.text} />
              </h3>
            );
          case "h3":
            return (
              <h4 key={index} className="pt-1 text-base font-semibold text-slate-900">
                <Inline text={block.text} />
              </h4>
            );
          case "ul":
            return (
              <ul
                key={index}
                className="list-disc space-y-1 pl-5 text-sm leading-relaxed text-slate-800"
              >
                {block.items.map((entry, i) => (
                  <li key={i}>
                    <Inline text={entry} />
                  </li>
                ))}
              </ul>
            );
          case "ol":
            return (
              <ol
                key={index}
                className="list-decimal space-y-1 pl-5 text-sm leading-relaxed text-slate-800"
              >
                {block.items.map((entry, i) => (
                  <li key={i}>
                    <Inline text={entry} />
                  </li>
                ))}
              </ol>
            );
          case "hr":
            return <hr key={index} className="border-slate-200" />;
          case "image":
            return (
              <Image
                key={index}
                src={block.src}
                alt={block.alt ?? ""}
                width={1024}
                height={576}
                unoptimized
                className="h-auto max-w-full rounded-md border border-slate-200"
              />
            );
          case "youtube":
            return (
              <iframe
                key={index}
                src={block.embedUrl}
                title="YouTube video"
                className="aspect-video w-full rounded-md border border-slate-200"
                allow="accelerometer; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
                loading="lazy"
                referrerPolicy="strict-origin-when-cross-origin"
              />
            );
          default:
            return (
              <p key={index} className="text-sm leading-relaxed text-slate-800">
                <Inline text={block.text} />
              </p>
            );
        }
      })}
    </div>
  );
}

function seoStatusClass(status: SeoCheckStatus): string {
  switch (status) {
    case "good":
      return "bg-emerald-50 text-emerald-800";
    case "warn":
      return "bg-amber-50 text-amber-800";
    case "fail":
      return "bg-red-50 text-red-800";
  }
}

function seoStatusLabel(status: SeoCheckStatus): string {
  switch (status) {
    case "good":
      return "Good";
    case "warn":
      return "Improve";
    case "fail":
      return "Fix";
  }
}

function jobStatusClass(status: string): string {
  switch (status) {
    case "succeeded":
      return "bg-emerald-50 text-emerald-800";
    case "failed":
      return "bg-red-50 text-red-800";
    case "running":
      return "bg-sky-50 text-sky-800";
    case "pending":
      return "bg-amber-50 text-amber-800";
    default:
      return "bg-slate-100 text-slate-700";
  }
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

  // Media state. The main image rides in the version metadata (its asset
  // reference is what a publisher needs); body media is inserted as single
  // [media-...] marker lines in the body text itself.
  const [loadedMetadata, setLoadedMetadata] = useState<Record<string, unknown>>({});
  const [mainImage, setMainImage] = useState<{ ref: string; src: string; alt: string }>({
    ref: "",
    src: "",
    alt: "",
  });
  const [mediaBusy, setMediaBusy] = useState(false);
  const [mediaError, setMediaError] = useState<string | null>(null);
  const [youtubeInput, setYoutubeInput] = useState("");
  const mainImageFileRef = useRef<HTMLInputElement | null>(null);
  const insertFileRef = useRef<HTMLInputElement | null>(null);

  const [formErrors, setFormErrors] = useState<string[]>([]);
  const [serverError, setServerError] = useState<string | null>(null);
  const [conflict, setConflict] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  // Approval workflow state
  const [history, setHistory] = useState<ContentHistoryState>({ kind: "loading" });
  const [invalidated, setInvalidated] = useState(false);
  const [deciding, setDeciding] = useState(false);
  const [decisionNote, setDecisionNote] = useState("");
  const [decisionError, setDecisionError] = useState<string | null>(null);

  // Preview + manual status state
  const [previewing, setPreviewing] = useState(false);
  const [moving, setMoving] = useState(false);
  const [moveError, setMoveError] = useState<string | null>(null);

  // Sanity publishing state
  const [sanityIntegrations, setSanityIntegrations] = useState<Integration[]>([]);
  const [integrationsLoaded, setIntegrationsLoaded] = useState(false);
  const [selectedIntegrationId, setSelectedIntegrationId] = useState("");
  const [sanityBusy, setSanityBusy] = useState<SanityPublishMode | null>(null);
  const [sanityResult, setSanityResult] = useState<SanityPublishResult | null>(null);

  // Social publishing: one picker over
  // this workspace's social integrations; publishes reuse the same
  // approval-gated endpoint as Sanity — the backend dispatches by adapter
  // kind — and a future scheduledAt queues the job for the worker instead of
  // publishing inline.
  const [socialIntegrations, setSocialIntegrations] = useState<Integration[]>([]);
  const [socialSelectedId, setSocialSelectedId] = useState("");
  const [socialBusy, setSocialBusy] = useState(false);
  const [socialResult, setSocialResult] = useState<SanityPublishResult | null>(null);
  const [socialScheduleInput, setSocialScheduleInput] = useState("");
  const [socialScheduleError, setSocialScheduleError] = useState<string | null>(null);

  // Scheduled publish + publishing jobs. The schedule input
  // works in UTC like the suggested publish slot; a future time queues the
  // job for the background worker instead of publishing inline.
  const [scheduleInput, setScheduleInput] = useState("");
  const [jobsState, setJobsState] = useState<ContentJobsState>({ kind: "loading" });
  const [jobBusyId, setJobBusyId] = useState<string | null>(null);
  const [jobActionError, setJobActionError] = useState<string | null>(null);

  const loadSanityIntegrations = useCallback(async () => {
    const result = await fetchWorkspaceIntegrations(apiBaseUrl, workspaceId);
    if (result.kind === "loaded") {
      const sanity = result.integrations.filter(
        (integration) => integration.adapterKind === "sanity",
      );
      setSanityIntegrations(sanity);
      setSelectedIntegrationId((current) => {
        if (current && sanity.some((integration) => integration.id === current)) {
          return current;
        }
        return sanity[0]?.id ?? "";
      });
      const social = result.integrations.filter(
        (integration) =>
          integration.adapterKind === "meta" ||
          integration.adapterKind === "youtube" ||
          integration.adapterKind === "x",
      );
      setSocialIntegrations(social);
      setSocialSelectedId((current) => {
        if (current && social.some((integration) => integration.id === current)) {
          return current;
        }
        return social[0]?.id ?? "";
      });
    }
    setIntegrationsLoaded(true);
  }, [apiBaseUrl, workspaceId]);

  const loadHistory = useCallback(async () => {
    const result = await fetchContentHistory(apiBaseUrl, workspaceId, itemId);
    setHistory(result);
  }, [apiBaseUrl, workspaceId, itemId]);

  const loadJobs = useCallback(async () => {
    const result = await fetchContentJobs(apiBaseUrl, workspaceId, itemId);
    setJobsState(result);
  }, [apiBaseUrl, workspaceId, itemId]);

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
      const metadata = version?.metadata ?? {};
      setLoadedMetadata(metadata);
      const rawMain = metadata.mainImage;
      if (
        rawMain &&
        typeof rawMain === "object" &&
        typeof (rawMain as { ref?: unknown }).ref === "string" &&
        ((rawMain as { ref: string }).ref as string) !== ""
      ) {
        const ref = (rawMain as { ref: string }).ref;
        const src =
          typeof (rawMain as { src?: unknown }).src === "string"
            ? String((rawMain as { src?: unknown }).src)
            : "";
        const alt =
          typeof (rawMain as { alt?: unknown }).alt === "string"
            ? String((rawMain as { alt?: unknown }).alt)
            : "";
        setMainImage({ ref, src, alt });
      } else {
        setMainImage({ ref: "", src: "", alt: "" });
      }
    }
    void loadHistory();
  }, [apiBaseUrl, workspaceId, itemId, loadHistory]);

  useEffect(() => {
    void load();
    void loadSanityIntegrations();
    void loadJobs();
  }, [load, loadSanityIntegrations, loadJobs]);

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

    // Main image: the asset reference (plus preview URL and alt) travels in
    // the version metadata; other metadata keys are preserved as loaded.
    const metadata: Record<string, unknown> = { ...loadedMetadata };
    if (mainImage.ref) {
      metadata.mainImage = { ref: mainImage.ref, src: mainImage.src, alt: mainImage.alt };
    } else {
      delete metadata.mainImage;
    }

    const result: SaveDraftResult = await saveWorkspaceDraft(apiBaseUrl, workspaceId, itemId, {
      title,
      slug,
      plannedPublishAt: localInputToIso(plannedPublishAt),
      excerpt,
      body,
      callToAction,
      channelCopy,
      metadata,
    });

    setSaving(false);
    if (result.kind === "saved") {
      setFormErrors([]);
      // Editing approved content invalidates the approval; the server already
      // reset the status to draft — surface that once here.
      if (state.kind === "loaded" && state.item.status === "approved") {
        setInvalidated(true);
      }
      void load();
    } else if (result.kind === "rejected") {
      setFormErrors(result.problems);
    } else if (result.kind === "conflict") {
      setConflict(result.message);
    } else {
      setServerError(result.message);
    }
  }

  async function handleTransition(to: "review" | "draft") {
    setMoveError(null);
    setMoving(true);
    const result = await transitionContentStatus(apiBaseUrl, workspaceId, itemId, to);
    setMoving(false);
    if (result.kind === "moved") {
      setPreviewing(false);
      void load();
    } else if (result.kind === "rejected") {
      setMoveError(result.problems.join("; "));
    } else {
      setMoveError(result.message);
    }
  }

  async function handleDecide(decision: ApprovalDecision) {
    setDecisionError(null);
    setDeciding(true);
    const result = await decideContentApproval(
      apiBaseUrl,
      workspaceId,
      itemId,
      decision,
      decisionNote || undefined,
    );
    setDeciding(false);
    if (result.kind === "decided") {
      setDecisionNote("");
      void load();
    } else if (result.kind === "rejected") {
      setDecisionError(result.problems.join("; "));
    } else {
      setDecisionError(result.message);
    }
  }

  async function handleSanity(mode: SanityPublishMode, scheduledAt?: string) {
    if (mode === "draft") setSanityResult(null);
    setSanityBusy(mode);
    const result = await pushToSanity(
      apiBaseUrl,
      workspaceId,
      itemId,
      selectedIntegrationId,
      mode,
      scheduledAt,
    );
    setSanityBusy(null);
    setSanityResult(result);
    if (mode === "publish") {
      void loadJobs();
      if (!scheduledAt) void load();
    }
    if (mode === "draft") void load();
  }

  async function handleSocialPublish(scheduledAt?: string) {
    setSocialResult(null);
    setSocialBusy(true);
    const result = await pushToSanity(
      apiBaseUrl,
      workspaceId,
      itemId,
      socialSelectedId,
      "publish",
      scheduledAt,
    );
    setSocialBusy(false);
    setSocialResult(result);
    void loadJobs();
    if (!scheduledAt) void load();
  }

  async function handleSocialSchedule() {
    setSocialScheduleError(null);
    const iso = localInputToIso(socialScheduleInput);
    if (!iso) {
      setSocialScheduleError("Choose a date and time first.");
      return;
    }
    if (new Date(iso).getTime() <= Date.now()) {
      setSocialScheduleError("The schedule must be in the future.");
      return;
    }
    setSocialScheduleInput("");
    await handleSocialPublish(iso);
  }

  async function handleSchedule() {
    setJobActionError(null);
    const iso = localInputToIso(scheduleInput);
    if (!iso) {
      setJobActionError("Choose a date and time first.");
      return;
    }
    if (new Date(iso).getTime() <= Date.now()) {
      setJobActionError("The schedule must be in the future.");
      return;
    }
    setScheduleInput("");
    await handleSanity("publish", iso);
  }

  async function handleRetryJob(jobId: string) {
    setJobActionError(null);
    setJobBusyId(jobId);
    const result = await retryContentJob(apiBaseUrl, workspaceId, itemId, jobId);
    setJobBusyId(null);
    setJobActionError(result.kind === "requeued" ? null : result.message);
    void loadJobs();
  }

  async function handleCancelJob(jobId: string) {
    setJobActionError(null);
    setJobBusyId(jobId);
    const result = await cancelContentJob(apiBaseUrl, workspaceId, itemId, jobId);
    setJobBusyId(null);
    setJobActionError(result.kind === "cancelled" ? null : result.message);
    void loadJobs();
  }

  // ---- media handling ----------------------------------------------------------

  const hasUploadIntegration =
    integrationsLoaded && sanityIntegrations.some((integration) => integration.hasCredentials);

  // Advisory SEO recommendations: recomputed live from the
  // current form fields. Purely derived advice — never measured analytics.
  const itemKind = state.kind === "loaded" ? state.item.kind : "blog_article";
  const seoReport = useMemo(
    () => evaluateSeo({ title, slug, excerpt, body, kind: itemKind }),
    [title, slug, excerpt, body, itemKind],
  );

  function appendToBody(prev: string, marker: string): string {
    const base = prev.trimEnd();
    return base ? `${base}\n\n${marker}` : marker;
  }

  async function uploadFile(file: File): Promise<{ ref: string; src: string } | null> {
    if (!selectedIntegrationId) {
      setMediaError("Connect a Sanity integration first.");
      return null;
    }
    setMediaError(null);
    setMediaBusy(true);
    const result = await uploadIntegrationImage(
      apiBaseUrl,
      workspaceId,
      selectedIntegrationId,
      file,
    );
    setMediaBusy(false);
    if (result.kind === "rejected") {
      setMediaError(result.problems.join("; "));
      return null;
    }
    if (result.kind === "unavailable") {
      setMediaError(result.message);
      return null;
    }
    return { ref: result.ref, src: result.url };
  }

  async function handleMainImageFile(file: File) {
    const uploaded = await uploadFile(file);
    if (!uploaded) return;
    setMainImage((current) => ({ ref: uploaded.ref, src: uploaded.src, alt: current.alt }));
  }

  async function handleInsertImageFile(file: File) {
    const uploaded = await uploadFile(file);
    if (!uploaded) return;
    setBody((prev) =>
      appendToBody(prev, buildImageMarker({ ref: uploaded.ref, src: uploaded.src })),
    );
  }

  function handleInsertYoutube() {
    const raw = youtubeInput.trim();
    const embed = youtubeEmbedUrl(raw);
    if (!embed) {
      setMediaError("Paste a YouTube video link (youtube.com or youtu.be).");
      return;
    }
    setMediaError(null);
    setYoutubeInput("");
    setBody((prev) => appendToBody(prev, buildYoutubeMarker(raw)));
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
              className="font-medium text-indigo-600 underline-offset-2 hover:text-indigo-700 hover:underline"
            >
              Content
            </Link>
            {" / "}
            <Link
              href={`/workspaces/${workspaceId}`}
              className="font-medium text-indigo-600 underline-offset-2 hover:text-indigo-700 hover:underline"
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
            {version && (
              <div className="ml-auto flex overflow-hidden rounded-md border border-slate-300">
                <button
                  type="button"
                  onClick={() => setPreviewing(false)}
                  aria-pressed={!previewing}
                  className={`px-3 py-1 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 ${
                    previewing
                      ? "bg-white text-slate-600 hover:bg-slate-50"
                      : "bg-indigo-600 text-white"
                  }
                  `}
                >
                  Edit
                </button>
                <button
                  type="button"
                  onClick={() => setPreviewing(true)}
                  aria-pressed={previewing}
                  className={`px-3 py-1 text-xs font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 ${
                    previewing
                      ? "bg-indigo-600 text-white"
                      : "bg-white text-slate-600 hover:bg-slate-50"
                  }`}
                >
                  Preview
                </button>
              </div>
            )}
          </div>
        </div>
        <p className="max-w-sm rounded-md bg-emerald-50 px-3 py-2 text-xs font-medium text-emerald-900 ring-1 ring-inset ring-emerald-700/15">
          Draft only. Publishing requires your explicit approval later — nothing here goes out
          automatically.
        </p>
      </div>

      {invalidated && (
        <p
          role="status"
          className="mb-4 rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15"
        >
          Your edits invalidated the approval. The draft must be approved again before it can
          publish.
        </p>
      )}

      {item.status === "approved" && (
        <p
          role="status"
          className="mb-4 rounded-md bg-emerald-50 px-4 py-3 text-sm font-medium text-emerald-900 ring-1 ring-inset ring-emerald-700/15"
        >
          This version is approved. Editing it now would reset the draft for re-approval.
        </p>
      )}

      {(item.status === "draft" || item.status === "review") && version && (
        <div className="mb-4 rounded-lg border border-slate-200 bg-white px-5 py-4">
          <h3 className="text-sm font-semibold text-slate-800">Review flow</h3>
          <p className="mt-1 text-xs text-slate-600">
            {item.status === "draft"
              ? "Submit this draft for review, or decide on it right away — version " +
                version.versionNumber +
                "."
              : "In review. Decide on it, or send it back to draft for more work — version " +
                version.versionNumber +
                "."}{" "}
            Nothing publishes without an explicit approval decision.
          </p>
          {item.status === "draft" && (
            <button
              type="button"
              disabled={moving}
              onClick={() => void handleTransition("review")}
              className="mt-3 rounded-md border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {moving ? "Submitting…" : "Submit for review"}
            </button>
          )}
          {item.status === "review" && (
            <button
              type="button"
              disabled={moving}
              onClick={() => void handleTransition("draft")}
              className="mt-3 rounded-md border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {moving ? "Moving…" : "Back to draft"}
            </button>
          )}
          {moveError && (
            <p
              role="alert"
              className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
            >
              {moveError}
            </p>
          )}
          <input
            type="text"
            value={decisionNote}
            onChange={(e) => setDecisionNote(e.target.value)}
            maxLength={2000}
            placeholder="Optional decision note (e.g. why, or rework instructions)"
            className={`mt-4 ${inputClasses}`}
          />
          {decisionError && (
            <p
              role="alert"
              className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
            >
              {decisionError}
            </p>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              disabled={deciding}
              onClick={() => void handleDecide("approve")}
              className="rounded-md bg-emerald-800 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {deciding ? "Recording…" : "Approve version"}
            </button>
            <button
              type="button"
              disabled={deciding}
              onClick={() => void handleDecide("reject")}
              className="rounded-md border border-red-300 px-4 py-2 text-sm font-semibold text-red-800 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60"
            >
              Reject version
            </button>
          </div>
        </div>
      )}

      {version && integrationsLoaded && (
        <div className="mb-4 rounded-lg border border-slate-200 bg-white px-5 py-4">
          <h3 className="text-sm font-semibold text-slate-800">Sanity publishing</h3>
          {sanityIntegrations.length === 0 ? (
            <p className="mt-1 text-xs text-slate-600">
              This workspace has no Sanity integration yet — connect one under
              <Link
                href={`/workspaces/${workspaceId}`}
                className="ml-1 font-medium text-indigo-600 underline-offset-2 hover:underline"
              >
                workspace integrations
              </Link>
              first.
            </p>
          ) : (
            <>
              <div className="mt-2 sm:max-w-sm">
                <label
                  htmlFor="sanity-integration"
                  className="block text-xs font-medium text-slate-600"
                >
                  Sanity integration
                </label>
                <select
                  id="sanity-integration"
                  value={selectedIntegrationId}
                  onChange={(e) => setSelectedIntegrationId(e.target.value)}
                  className={`mt-1 ${inputClasses}`}
                >
                  {sanityIntegrations.map((integration) => (
                    <option key={integration.id} value={integration.id}>
                      {integration.name}
                      {integration.hasCredentials ? "" : " (no write token saved)"}
                    </option>
                  ))}
                </select>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                <button
                  type="button"
                  disabled={sanityBusy !== null}
                  onClick={() => void handleSanity("draft")}
                  className="rounded-md border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {sanityBusy === "draft" ? "Pushing…" : "Push draft to Sanity"}
                </button>
                <button
                  type="button"
                  disabled={sanityBusy !== null || item.status !== "approved"}
                  title={
                    item.status === "approved"
                      ? undefined
                      : "Approve the newest version first — the server refuses unapproved publishes."
                  }
                  onClick={() => void handleSanity("publish")}
                  className="rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {sanityBusy === "publish" ? "Publishing…" : "Publish to Sanity"}
                </button>
              </div>
              {item.status === "approved" && (
                <div className="mt-3 flex flex-wrap items-end gap-2">
                  <div>
                    <label
                      htmlFor="draft-schedule"
                      className="block text-xs font-medium text-slate-600"
                    >
                      Or schedule the publish (UTC)
                    </label>
                    <input
                      id="draft-schedule"
                      type="datetime-local"
                      step={60}
                      value={scheduleInput}
                      onChange={(e) => setScheduleInput(e.target.value)}
                      disabled={sanityBusy !== null}
                      className={`max-w-xs ${inputClasses}`}
                    />
                  </div>
                  <button
                    type="button"
                    disabled={sanityBusy !== null || !scheduleInput}
                    onClick={() => void handleSchedule()}
                    className="rounded-md border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {sanityBusy === "publish" ? "Scheduling…" : "Queue scheduled publish"}
                  </button>
                </div>
              )}
              <p className="mt-2 text-xs text-slate-500">
                “Push draft” writes or replaces this item&apos;s invisible Sanity draft with version{" "}
                {version.versionNumber} — safe on any status. “Publish” refreshes that same Sanity
                draft from the newest approved version, then publishes it.
              </p>
              {sanityResult?.kind === "pushed" && (
                <div
                  role="status"
                  className="mt-3 rounded-md bg-emerald-50 px-3 py-2 text-sm text-emerald-900 ring-1 ring-inset ring-emerald-700/15"
                >
                  <p className="font-medium">{sanityResult.success.message}</p>
                  {sanityResult.success.mode === "draft" && sanityResult.success.draftId && (
                    <p className="mt-1 text-xs">
                      Sanity draft{" "}
                      <code className="rounded bg-emerald-100/60 px-1">
                        {sanityResult.success.draftId}
                      </code>
                      {sanityResult.success.transactionId
                        ? ` · transaction ${sanityResult.success.transactionId}`
                        : ""}
                    </p>
                  )}
                  {sanityResult.success.mode === "publish" && sanityResult.success.publicUrl && (
                    <p className="mt-1 text-xs">
                      <a
                        href={sanityResult.success.publicUrl}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="font-medium underline underline-offset-2"
                      >
                        {sanityResult.success.publicUrl}
                      </a>{" "}
                      {sanityResult.success.urlVerified
                        ? "— verified reachable (HTTP 200)."
                        : "— could not be verified from here; open it to confirm."}
                    </p>
                  )}
                </div>
              )}
              {sanityResult?.kind === "conflict" && (
                <p
                  role="alert"
                  className="mt-3 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15"
                >
                  {sanityResult.message}
                </p>
              )}
              {sanityResult?.kind === "failed" && (
                <p
                  role="alert"
                  className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
                >
                  {sanityResult.message}
                </p>
              )}
              {sanityResult?.kind === "rejected" && (
                <ul role="alert" className="mt-3 list-disc pl-5 text-xs text-red-700">
                  {sanityResult.problems.map((problem) => (
                    <li key={problem}>{problem}</li>
                  ))}
                </ul>
              )}
              {sanityResult?.kind === "unavailable" && (
                <p
                  role="alert"
                  className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
                >
                  {sanityResult.message}
                </p>
              )}
            </>
          )}
        </div>
      )}

      {version && integrationsLoaded && (
        <div className="mb-4 rounded-lg border border-slate-200 bg-white px-5 py-4">
          <h3 className="text-sm font-semibold text-slate-800">
            Social publishing (Meta & YouTube)
          </h3>
          {socialIntegrations.length === 0 ? (
            <p className="mt-1 text-xs text-slate-600">
              No social channel is connected yet — connect a Facebook Page post, Instagram business
              post, or YouTube channel integration under
              <Link
                href={`/workspaces/${workspaceId}`}
                className="ml-1 font-medium text-indigo-600 underline-offset-2 hover:underline"
              >
                workspace integrations
              </Link>
              first.
            </p>
          ) : (
            <>
              <div className="mt-2 sm:max-w-sm">
                <label
                  htmlFor="meta-integration"
                  className="block text-xs font-medium text-slate-600"
                >
                  Social destination
                </label>
                <select
                  id="meta-integration"
                  value={socialSelectedId}
                  onChange={(e) => setSocialSelectedId(e.target.value)}
                  className={`mt-1 ${inputClasses}`}
                >
                  {socialIntegrations.map((integration) => (
                    <option key={integration.id} value={integration.id}>
                      {integration.name}
                      {integration.hasCredentials ? "" : " (no credentials saved)"}
                    </option>
                  ))}
                </select>
              </div>
              <div className="mt-3">
                <button
                  type="button"
                  disabled={socialBusy || item.status !== "approved"}
                  title={
                    item.status === "approved"
                      ? undefined
                      : "Approve the newest version first — the server refuses unapproved publishes."
                  }
                  onClick={() => void handleSocialPublish()}
                  className={PRIMARY_BUTTON}
                >
                  {socialBusy ? "Posting…" : "Publish post now"}
                </button>
              </div>
              {item.status === "approved" && (
                <div className="mt-3 flex flex-wrap items-end gap-2">
                  <div>
                    <label
                      htmlFor="meta-schedule"
                      className="block text-xs font-medium text-slate-600"
                    >
                      Or schedule the post (UTC)
                    </label>
                    <input
                      id="meta-schedule"
                      type="datetime-local"
                      step={60}
                      value={socialScheduleInput}
                      onChange={(e) => setSocialScheduleInput(e.target.value)}
                      disabled={socialBusy}
                      className={`max-w-xs ${inputClasses}`}
                    />
                  </div>
                  <button
                    type="button"
                    disabled={socialBusy || !socialScheduleInput}
                    onClick={() => void handleSocialSchedule()}
                    className="rounded-md border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {socialBusy ? "Scheduling…" : "Queue scheduled post"}
                  </button>
                </div>
              )}
              <p className="mt-2 text-xs text-slate-500">
                The post goes out only after the newest version is approved. There is no
                destination-side draft on social destinations; Instagram additionally needs a public
                image on the approved version, and YouTube needs a public video file URL (metadata
                key&nbsp;
                <code className="rounded bg-slate-200 px-1">videoUrl</code>).
              </p>
              {socialScheduleError && (
                <p role="alert" className="mt-2 text-xs text-red-700">
                  {socialScheduleError}
                </p>
              )}
              {socialResult?.kind === "pushed" && (
                <div
                  role="status"
                  className="mt-3 rounded-md bg-emerald-50 px-3 py-2 text-sm text-emerald-900 ring-1 ring-inset ring-emerald-700/15"
                >
                  <p className="font-medium">{socialResult.success.message}</p>
                  {socialResult.success.publicUrl && (
                    <p className="mt-1 text-xs">
                      <a
                        href={socialResult.success.publicUrl}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="font-medium underline underline-offset-2"
                      >
                        {socialResult.success.publicUrl}
                      </a>
                    </p>
                  )}
                </div>
              )}
              {socialResult?.kind === "conflict" && (
                <p
                  role="alert"
                  className="mt-3 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15"
                >
                  {socialResult.message}
                </p>
              )}
              {socialResult?.kind === "failed" && (
                <div
                  role="alert"
                  className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
                >
                  <p>{socialResult.message}</p>
                  <p className="mt-1 text-xs">
                    If this failure says the saved credentials were rejected or expired, reconnect
                    the integration on the{" "}
                    <Link
                      href={`/workspaces/${workspaceId}`}
                      className="font-medium underline underline-offset-2"
                    >
                      workspace integrations page
                    </Link>
                    .
                  </p>
                </div>
              )}
              {socialResult?.kind === "rejected" && (
                <ul role="alert" className="mt-3 list-disc pl-5 text-xs text-red-700">
                  {socialResult.problems.map((problem) => (
                    <li key={problem}>{problem}</li>
                  ))}
                </ul>
              )}
              {socialResult?.kind === "unavailable" && (
                <div
                  role="alert"
                  className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
                >
                  <p>{socialResult.message}</p>
                  <p className="mt-1 text-xs">
                    If this failure says the saved credentials were rejected or expired, reconnect
                    the integration on the{" "}
                    <Link
                      href={`/workspaces/${workspaceId}`}
                      className="font-medium underline underline-offset-2"
                    >
                      workspace integrations page
                    </Link>
                    .
                  </p>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {version && (
        <div className="mb-4 rounded-lg border border-slate-200 bg-white px-5 py-4">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-slate-800">Publishing jobs</h3>
            <button
              type="button"
              onClick={() => void loadJobs()}
              className="rounded-md border border-slate-300 px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100"
            >
              Refresh
            </button>
          </div>
          {jobsState.kind === "loading" && (
            <p className="mt-2 text-xs text-slate-500">Loading publishing history…</p>
          )}
          {jobsState.kind === "missing" && (
            <p className="mt-2 text-xs text-slate-500">{jobsState.message}</p>
          )}
          {jobsState.kind === "unavailable" && (
            <p role="alert" className="mt-2 text-xs text-red-700">
              {jobsState.message}
            </p>
          )}
          {jobsState.kind === "loaded" && jobsState.jobs.length === 0 && (
            <p className="mt-2 text-xs text-slate-500">No publishing attempts yet for this item.</p>
          )}
          {jobsState.kind === "loaded" && jobsState.jobs.length > 0 && (
            <ul className="mt-2 space-y-2">
              {jobsState.jobs.map((job) => (
                <li
                  key={job.id}
                  className="rounded-md border border-slate-100 bg-slate-50 px-3 py-2"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span
                      className={`rounded px-2 py-0.5 text-xs font-medium ${jobStatusClass(job.status)}`}
                    >
                      {formatPublishJobStatus(job.status)}
                    </span>
                    <span className="text-xs text-slate-600">
                      attempt {job.attempts} · {new Date(job.updatedAt).toLocaleString()}
                    </span>
                    {job.scheduledAt && (
                      <span className="text-xs text-slate-600">
                        scheduled {new Date(job.scheduledAt).toLocaleString()}
                      </span>
                    )}
                    {job.nextAttemptAt && (
                      <span className="text-xs text-slate-600">
                        next attempt {new Date(job.nextAttemptAt).toLocaleString()}
                      </span>
                    )}
                    {(job.status === "failed" || job.status === "pending") && (
                      <div className="ml-auto flex gap-2">
                        {job.status === "failed" && (
                          <button
                            type="button"
                            disabled={jobBusyId === job.id}
                            onClick={() => void handleRetryJob(job.id)}
                            className="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
                          >
                            {jobBusyId === job.id ? "Retrying…" : "Retry"}
                          </button>
                        )}
                        <button
                          type="button"
                          disabled={jobBusyId === job.id}
                          onClick={() => void handleCancelJob(job.id)}
                          className="rounded-md px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60"
                        >
                          Cancel
                        </button>
                      </div>
                    )}
                  </div>
                  {job.lastError && <p className="mt-1 text-xs text-red-700">{job.lastError}</p>}
                </li>
              ))}
            </ul>
          )}
          {jobActionError && (
            <p role="alert" className="mt-2 text-xs text-red-700">
              {jobActionError}
            </p>
          )}
        </div>
      )}

      {seoReport.checks.length > 0 && (
        <div className="mb-4 rounded-lg border border-slate-200 bg-white px-5 py-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h3 className="text-sm font-semibold text-slate-800">SEO advice</h3>
              <p className="mt-0.5 text-xs text-slate-500">
                Recommendations from this draft&apos;s own content — not measured search data.
              </p>
            </div>
            <span
              className={`rounded px-2 py-0.5 text-xs font-semibold ${
                seoReport.score >= 90
                  ? "bg-emerald-50 text-emerald-800"
                  : seoReport.score >= 60
                    ? "bg-amber-50 text-amber-800"
                    : "bg-red-50 text-red-800"
              }`}
            >
              {seoReport.score}%
            </span>
          </div>
          <ul className="mt-3 grid gap-2 sm:grid-cols-2">
            {seoReport.checks.map((check) => (
              <li
                key={check.id}
                className="rounded-md border border-slate-100 bg-slate-50 px-3 py-2"
              >
                <div className="flex items-center gap-2">
                  <span
                    className={`rounded px-2 py-0.5 text-xs font-medium ${seoStatusClass(check.status)}`}
                  >
                    {seoStatusLabel(check.status)}
                  </span>
                  <span className="text-xs font-medium text-slate-800">{check.label}</span>
                </div>
                <p className="mt-1 text-xs text-slate-600">{check.detail}</p>
              </li>
            ))}
          </ul>
        </div>
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

      {!previewing ? (
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

            <div className="rounded-md border border-slate-200 bg-slate-50 p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className={labelClasses}>Main image</span>
                {mainImage.ref ? (
                  <button
                    type="button"
                    disabled={!editable}
                    onClick={() => setMainImage({ ref: "", src: "", alt: "" })}
                    className="rounded-md px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    Remove main image
                  </button>
                ) : null}
              </div>
              {mainImage.src ? (
                <Image
                  src={mainImage.src}
                  alt={mainImage.alt ?? ""}
                  width={320}
                  height={256}
                  unoptimized
                  className="mt-2 h-32 w-auto rounded-md border border-slate-200 object-contain"
                />
              ) : null}
              <input
                ref={mainImageFileRef}
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                disabled={!editable || mediaBusy}
                className="hidden"
                onChange={(e) => {
                  const file = e.currentTarget.files?.[0];
                  e.currentTarget.value = "";
                  if (file) void handleMainImageFile(file);
                }}
              />
              <button
                type="button"
                disabled={!editable || mediaBusy}
                onClick={() => mainImageFileRef.current?.click()}
                className="mt-2 rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {mediaBusy ? "Uploading…" : mainImage.ref ? "Replace image" : "Upload image"}
              </button>
              <input
                type="text"
                value={mainImage.alt}
                onChange={(e) => setMainImage((current) => ({ ...current, alt: e.target.value }))}
                placeholder="Image alt text (optional)"
                disabled={!editable}
                className={`mt-2 ${inputClasses}`}
              />
              <p className="mt-1 text-xs text-slate-500">
                {hasUploadIntegration
                  ? "Uploaded to the workspace's Sanity project; the image reference travels with the version."
                  : "Uploading needs a Sanity integration with a saved write token."}
              </p>
            </div>

            <div className="rounded-md border border-slate-200 bg-slate-50 p-4">
              <span className={labelClasses}>Add media to the body</span>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <input
                  ref={insertFileRef}
                  type="file"
                  accept="image/jpeg,image/png,image/webp,image/gif"
                  disabled={!editable || mediaBusy}
                  className="hidden"
                  onChange={(e) => {
                    const file = e.currentTarget.files?.[0];
                    e.currentTarget.value = "";
                    if (file) void handleInsertImageFile(file);
                  }}
                />
                <button
                  type="button"
                  disabled={!editable || mediaBusy}
                  onClick={() => insertFileRef.current?.click()}
                  className="rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  + Insert image
                </button>
                <input
                  type="url"
                  value={youtubeInput}
                  onChange={(e) => setYoutubeInput(e.target.value)}
                  placeholder="https://youtu.be/…"
                  disabled={!editable}
                  className={`${inputClasses} max-w-xs`}
                />
                <button
                  type="button"
                  disabled={!editable || mediaBusy}
                  onClick={handleInsertYoutube}
                  className="rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  + Insert YouTube video
                </button>
              </div>
              {mediaError && (
                <p role="alert" className="mt-2 text-xs text-red-700">
                  {mediaError}
                </p>
              )}
              <p className="mt-1 text-xs text-slate-500">
                Media is appended at the end of the body in “Edit” mode and appears in the preview;
                move it by editing the [media-…] lines in the body text.
              </p>
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
              Every save stores a new immutable version — full history is right below, with each
              version&apos;s decision.
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
      ) : (
        <section aria-label="Draft preview" className="rounded-lg border border-slate-200 bg-white">
          <div className="border-b border-slate-200 px-5 py-4">
            <h3 className="text-base font-semibold text-slate-900">{title || item.title}</h3>
            {slug && (
              <p className="mt-1 text-xs text-slate-500">
                <code className="rounded bg-slate-100 px-1 py-0.5">{slug}</code>
                {plannedPublishAt ? ` · planned ${localInputToIso(plannedPublishAt)}` : ""}
              </p>
            )}
          </div>
          <div className="px-5 py-5">
            {excerpt && (
              <p className="mb-4 rounded-md bg-slate-50 px-3 py-2 text-sm italic text-slate-700">
                {excerpt}
              </p>
            )}
            <PreviewBody body={body} />
            {callToAction && (
              <p className="mt-5 inline-flex rounded-md bg-indigo-50 px-3 py-1.5 text-sm font-semibold text-indigo-900 ring-1 ring-inset ring-indigo-700/20">
                {callToAction}
              </p>
            )}
            {channels.filter((channel) => channel.name.trim() && channel.copy.trim()).length >
              0 && (
              <div className="mt-5">
                <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                  Channel copy
                </h4>
                <dl className="mt-2 space-y-2">
                  {channels
                    .filter((channel) => channel.name.trim() && channel.copy.trim())
                    .map((channel, index) => (
                      <div
                        key={`${channel.name}-${index}`}
                        className="rounded-md bg-slate-50 px-3 py-2"
                      >
                        <dt className="text-xs font-medium uppercase text-slate-500">
                          {channel.name.trim().toLowerCase()}
                        </dt>
                        <dd className="mt-0.5 text-sm text-slate-800">{channel.copy}</dd>
                      </div>
                    ))}
                </dl>
              </div>
            )}
          </div>
        </section>
      )}

      <section
        aria-labelledby="history-heading"
        className="mt-6 rounded-lg border border-slate-200 bg-white"
      >
        <div className="border-b border-slate-200 px-5 py-4">
          <h3 id="history-heading" className="text-base font-semibold">
            Version history
          </h3>
          <p className="mt-0.5 text-xs text-slate-500">
            Every save is an immutable snapshot; older versions keep their context for comparison.
          </p>
        </div>
        <div className="px-5">
          {history.kind === "loading" && (
            <p className="py-4 text-sm text-slate-500">Loading history…</p>
          )}
          {history.kind === "missing" && (
            <p className="py-4 text-xs text-amber-800">{history.message}</p>
          )}
          {history.kind === "unavailable" && (
            <p className="py-4 text-xs text-red-800">{history.message}</p>
          )}
          {history.kind === "loaded" && history.history.versions.length === 0 && (
            <p className="py-4 text-xs text-slate-500">No versions yet — save the draft once.</p>
          )}
          {history.kind === "loaded" && history.history.versions.length > 0 && (
            <ul className="divide-y divide-slate-100">
              {[...history.history.versions].reverse().map((entry) => {
                const decision = history.history.approvals[entry.id];
                return (
                  <li key={entry.id} className="py-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <p className="text-sm font-medium text-slate-900">
                          Version {entry.versionNumber}
                          {entry.title !== item.title ? ` — ${entry.title}` : ""}
                        </p>
                        <p className="mt-0.5 text-xs text-slate-500">
                          {new Date(entry.createdAt).toLocaleString()} ·{" "}
                          {entry.body.length.toLocaleString()} characters
                        </p>
                      </div>
                      {decision ? (
                        <span
                          className={`inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${
                            decision.decision === "approved"
                              ? "bg-emerald-50 text-emerald-900 ring-emerald-700/25"
                              : "bg-red-50 text-red-900 ring-red-700/25"
                          }`}
                        >
                          {decision.decision === "approved" ? "Approved" : "Rejected"}
                        </span>
                      ) : (
                        <span className="rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-600">
                          No decision yet
                        </span>
                      )}
                    </div>
                    {decision?.note && (
                      <p className="mt-1 text-xs text-slate-600">Note: {decision.note}</p>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </section>
    </div>
  );
}
