import { withAuth } from "@/lib/config";
export type ContentKind = "blog_article" | "social_post" | "snippet";

export type ContentStatus =
  "draft" | "review" | "approved" | "rejected" | "queued" | "publishing" | "published" | "failed";

export interface ContentItem {
  id: string;
  workspaceId: string;
  kind: ContentKind;
  status: ContentStatus;
  title: string;
  slug: string | null;
  plannedPublishAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ContentVersion {
  id: string;
  contentItemId: string;
  versionNumber: number;
  title: string;
  excerpt: string | null;
  body: string;
  callToAction?: string | null;
  channelCopy?: Record<string, string>;
  metadata?: Record<string, unknown>;
  createdAt: string;
}

export type DraftState =
  | { kind: "loading" }
  | { kind: "missing"; message: string }
  | { kind: "unavailable"; message: string }
  | { kind: "loaded"; item: ContentItem; version: ContentVersion | null };

export type SaveDraftResult =
  | { kind: "saved"; item: ContentItem; version: ContentVersion }
  | { kind: "rejected"; problems: string[] }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

export interface DraftInput {
  title: string;
  slug?: string;
  plannedPublishAt?: string;
  excerpt?: string;
  body?: string;
  callToAction?: string;
  channelCopy?: Record<string, string>;
  /** Omitted keeps the previous version's metadata; provided replaces it. */
  metadata?: Record<string, unknown>;
}

export type UploadImageResult =
  | { kind: "uploaded"; ref: string; url: string }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export const IMAGE_MIME_TYPES = ["image/jpeg", "image/png", "image/webp", "image/gif"] as const;
export const MAX_IMAGE_BYTES = 8 * 1024 * 1024;

/**
 * Uploads one image through the API, which proxies it to the workspace's
 * Sanity project and returns the asset reference plus CDN URL. Never throws.
 */
export async function uploadIntegrationImage(
  baseUrl: string,
  workspaceId: string,
  integrationId: string,
  file: File,
  fetchImpl: typeof fetch = fetch,
): Promise<UploadImageResult> {
  const problems: string[] = [];
  if (!IMAGE_MIME_TYPES.includes(file.type as (typeof IMAGE_MIME_TYPES)[number])) {
    problems.push("only JPEG, PNG, WebP, or GIF images are supported");
  }
  if (file.size > MAX_IMAGE_BYTES) {
    problems.push("image is too large (max 8 MB)");
  }
  if (problems.length > 0) return { kind: "rejected", problems };

  const url =
    `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}` +
    `/integrations/${encodeURIComponent(integrationId)}/media`;
  const form = new FormData();
  form.append("file", file, file.name);

  try {
    const res = await fetchImpl(url, { method: "POST", headers: withAuth(), body: form });
    if (res.status === 413) {
      return { kind: "rejected", problems: ["image is too large (max 8 MB)"] };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `Upload failed (status ${res.status}).`,
      };
    }
    const data = (await res.json()) as Record<string, unknown>;
    if (typeof data.ref !== "string" || data.ref === "" || typeof data.url !== "string") {
      return { kind: "unavailable", message: "Server returned a malformed upload result." };
    }
    return { kind: "uploaded", ref: data.ref, url: data.url };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export type ContentListState =
  | { kind: "loading" }
  | { kind: "unavailable"; message: string }
  | { kind: "missing"; message: string }
  | { kind: "loaded"; items: ContentItem[] };

export type CreateContentResult =
  | { kind: "created"; item: ContentItem }
  | { kind: "rejected"; problems: string[] }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

export interface CreateContentInput {
  kind: ContentKind;
  title: string;
  slug?: string;
}

export type GenerateContentResult =
  | { kind: "created"; item: ContentItem; version: ContentVersion; provider: string; model: string }
  | { kind: "rejected"; problems: string[] }
  | { kind: "conflict"; message: string }
  | { kind: "failed"; message: string }
  | { kind: "unavailable"; message: string };

export interface GenerateContentInput {
  kind: ContentKind;
  topic: string;
  channel?: string;
  instructions?: string;
}

export const GENERATE_FORM_LIMITS = { topic: 2000, instructions: 2000 } as const;

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$|^[a-z0-9]$/;

function clearToUndefined(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

/** Validates the editable fields of a draft. Same rules as the API. */
export function validateDraftInput(input: {
  title?: string;
  slug?: string;
  plannedPublishAt?: string;
  excerpt?: string;
  body?: string;
  callToAction?: string;
  channelCopy?: Record<string, string>;
}): string[] {
  const problems: string[] = [];

  const title = (input.title ?? "").trim();
  if (!title) {
    problems.push("title cannot be blank");
  } else if (Array.from(title).length > 200) {
    problems.push("title is too long (max 200 characters)");
  }

  const slug = (input.slug ?? "").trim().toLowerCase();
  if (slug && !SLUG_PATTERN.test(slug)) {
    problems.push("slug must be lowercase letters, digits, or hyphens");
  }

  if (input.plannedPublishAt) {
    const when = new Date(input.plannedPublishAt);
    if (Number.isNaN(when.getTime())) {
      problems.push("plannedPublishAt must be a UTC ISO 8601 timestamp");
    }
  }

  for (const [name, max] of [
    ["excerpt", 5000],
    ["body", 100000],
    ["callToAction", 5000],
  ] as const) {
    const value = (input[name] ?? "").trim();
    if (Array.from(value).length > max) {
      problems.push(`${name} is too long (max ${max} characters)`);
    }
  }

  const channels = input.channelCopy ?? {};
  const names = Object.keys(channels);
  if (names.length > 32) {
    problems.push("channelCopy has too many channels (max 32)");
  }
  for (const name of names) {
    if (Array.from(channels[name]).length > 5000) {
      problems.push(`channelCopy for ${name} is too long (max 5000 characters)`);
    }
  }

  return problems;
}

export function validateContentInput(input: {
  kind?: string;
  title?: string;
  slug?: string;
}): string[] {
  const problems: string[] = [];
  const kind = (input.kind ?? "").trim();
  if (!kind) {
    problems.push("kind is required");
  } else if (kind !== "blog_article" && kind !== "social_post" && kind !== "snippet") {
    problems.push("kind must be blog_article, social_post, or snippet");
  }

  const title = (input.title ?? "").trim();
  if (!title) {
    problems.push("title is required");
  } else if (Array.from(title).length > 200) {
    problems.push("title is too long (max 200 characters)");
  }

  const slug = (input.slug ?? "").trim().toLowerCase();
  if (slug && !SLUG_PATTERN.test(slug)) {
    problems.push("slug must be lowercase letters, digits, or hyphens");
  }

  return problems;
}

export function formatContentKind(kind: ContentKind): string {
  switch (kind) {
    case "blog_article":
      return "Blog article";
    case "social_post":
      return "Social post";
    case "snippet":
      return "Snippet";
    default:
      return kind;
  }
}

export function formatContentStatus(status: ContentStatus): string {
  switch (status) {
    case "draft":
      return "Draft";
    case "review":
      return "In review";
    case "approved":
      return "Approved";
    case "rejected":
      return "Rejected";
    case "queued":
      return "Queued";
    case "publishing":
      return "Publishing";
    case "published":
      return "Published";
    case "failed":
      return "Failed";
    default:
      return status;
  }
}

function parseContentItem(raw: unknown): ContentItem | null {
  if (typeof raw !== "object" || raw === null) return null;
  const c = raw as Record<string, unknown>;
  if (
    typeof c.id !== "string" ||
    typeof c.workspaceId !== "string" ||
    typeof c.kind !== "string" ||
    typeof c.status !== "string" ||
    typeof c.title !== "string" ||
    (c.slug !== null && typeof c.slug !== "string") ||
    (c.plannedPublishAt !== null &&
      c.plannedPublishAt !== undefined &&
      typeof c.plannedPublishAt !== "string") ||
    typeof c.createdAt !== "string" ||
    typeof c.updatedAt !== "string"
  ) {
    return null;
  }

  return {
    id: c.id,
    workspaceId: c.workspaceId,
    kind: c.kind as ContentKind,
    status: c.status as ContentStatus,
    title: c.title,
    slug: c.slug,
    plannedPublishAt: (c.plannedPublishAt as string | null | undefined) ?? null,
    createdAt: c.createdAt,
    updatedAt: c.updatedAt,
  };
}

function parseContentVersion(raw: unknown): ContentVersion | null {
  if (typeof raw !== "object" || raw === null) return null;
  const v = raw as Record<string, unknown>;
  if (
    typeof v.id !== "string" ||
    typeof v.contentItemId !== "string" ||
    typeof v.versionNumber !== "number" ||
    typeof v.title !== "string" ||
    typeof v.body !== "string" ||
    (v.excerpt !== null && typeof v.excerpt !== "string") ||
    typeof v.createdAt !== "string"
  ) {
    return null;
  }
  const channels = v.channelCopy;
  if (channels !== undefined && (typeof channels !== "object" || channels === null)) {
    return null;
  }
  return {
    id: v.id,
    contentItemId: v.contentItemId,
    versionNumber: v.versionNumber,
    title: v.title,
    excerpt: v.excerpt as string | null,
    body: v.body,
    callToAction: (v.callToAction as string | null | undefined) ?? null,
    channelCopy: channels ? (channels as Record<string, string>) : undefined,
    metadata: v.metadata as Record<string, unknown> | undefined,
    createdAt: v.createdAt,
  };
}

export async function fetchWorkspaceContent(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ContentListState> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/content`;
  try {
    const res = await fetchImpl(url, { headers: withAuth({ Accept: "application/json" }) });
    if (res.status === 404) {
      return { kind: "missing", message: "Workspace not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as { items?: unknown[] };
    if (!Array.isArray(data.items)) {
      return { kind: "unavailable", message: "Server returned malformed content list." };
    }
    const items: ContentItem[] = [];
    for (const raw of data.items) {
      const parsed = parseContentItem(raw);
      if (parsed) items.push(parsed);
    }
    return { kind: "loaded", items };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export async function createWorkspaceContent(
  baseUrl: string,
  workspaceId: string,
  input: CreateContentInput,
  fetchImpl: typeof fetch = fetch,
): Promise<CreateContentResult> {
  const problems = validateContentInput(input);
  if (problems.length > 0) {
    return { kind: "rejected", problems };
  }

  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/content`;
  const bodyPayload: Record<string, string> = {
    kind: input.kind,
    title: input.title.trim(),
  };
  const trimmedSlug = (input.slug ?? "").trim().toLowerCase();
  if (trimmedSlug) {
    bodyPayload.slug = trimmedSlug;
  }

  try {
    const res = await fetchImpl(url, {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify(bodyPayload),
    });

    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "A content item with that slug already exists in this workspace.",
      };
    }

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `Failed to create content (status ${res.status}).`,
      };
    }

    const data = await res.json();
    const created = parseContentItem(data);
    if (!created) {
      return { kind: "unavailable", message: "Server returned malformed created item." };
    }
    return { kind: "created", item: created };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

function itemUrl(baseUrl: string, workspaceId: string, itemId: string): string {
  return `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/content/${encodeURIComponent(itemId)}`;
}

/**
 * Fetches GET /workspaces/{id}/content/{itemId}: the item plus its newest
 * brief version (or null before the first save). Never throws.
 */
export async function fetchWorkspaceDraft(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<DraftState> {
  try {
    const res = await fetchImpl(itemUrl(baseUrl, workspaceId, itemId), {
      headers: withAuth({ Accept: "application/json" }),
    });
    if (res.status === 404) {
      return { kind: "missing", message: "Workspace or content item not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as { version?: unknown };
    const item = parseContentItem(data);
    if (!item) {
      return { kind: "unavailable", message: "Server returned malformed draft detail." };
    }
    const version =
      data.version === null || data.version === undefined
        ? null
        : parseContentVersion(data.version);
    if (data.version !== null && data.version !== undefined && version === null) {
      return { kind: "unavailable", message: "Server returned malformed draft version." };
    }
    return { kind: "loaded", item, version };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

/**
 * PATCHes the editable draft fields. The API merges them onto the current
 * item and appends a new immutable version on every save. Never throws.
 */
export async function saveWorkspaceDraft(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  input: DraftInput,
  fetchImpl: typeof fetch = fetch,
): Promise<SaveDraftResult> {
  const problems = validateDraftInput(input);
  if (problems.length > 0) {
    return { kind: "rejected", problems };
  }

  const payload: Record<string, unknown> = {
    title: input.title.trim(),
    slug: (input.slug ?? "").trim().toLowerCase(),
    excerpt: clearToUndefined(input.excerpt) ?? "",
    body: input.body ?? "",
    callToAction: clearToUndefined(input.callToAction) ?? "",
    channelCopy: input.channelCopy ?? {},
  };
  if (input.metadata !== undefined) {
    if (JSON.stringify(input.metadata).length > 16384) {
      return { kind: "rejected", problems: ["metadata is too large (max 16384 bytes)"] };
    }
    payload.metadata = input.metadata;
  }
  const planned = clearToUndefined(input.plannedPublishAt);
  if (planned) {
    payload.plannedPublishAt = planned;
  }

  try {
    const res = await fetchImpl(itemUrl(baseUrl, workspaceId, itemId), {
      method: "PATCH",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify(payload),
    });
    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "The item cannot be saved in its current state.",
      };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      const error = body?.error;
      if (res.status === 400 && error) {
        return { kind: "rejected", problems: error.split("; ") };
      }
      return {
        kind: "unavailable",
        message: error ?? `Failed to save the draft (status ${res.status}).`,
      };
    }
    const data = (await res.json()) as { version?: unknown };
    const item = parseContentItem(data);
    const version = parseContentVersion(data.version);
    if (!item || !version) {
      return { kind: "unavailable", message: "Server returned malformed saved draft." };
    }
    return { kind: "saved", item, version };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

/**
 * POSTs the generation ask. The server asks a model for a structured draft
 * and stores it as a draft content item with a version; it never publishes.
 * Provider failures (timeout, malformed output, refused requests) surface as
 * a `failed` result with a safe message. Never throws.
 */
export async function generateWorkspaceContent(
  baseUrl: string,
  workspaceId: string,
  input: GenerateContentInput,
  fetchImpl: typeof fetch = fetch,
): Promise<GenerateContentResult> {
  const problems: string[] = [];
  const topic = (input.topic ?? "").trim();
  const instructions = (input.instructions ?? "").trim();
  if (input.kind !== "blog_article" && input.kind !== "social_post" && input.kind !== "snippet") {
    problems.push("kind must be blog_article, social_post, or snippet");
  }
  if (!topic) problems.push("topic is required");
  if (Array.from(topic).length > 2000) problems.push("topic is too long (max 2000 characters)");
  if (Array.from(instructions).length > 2000) {
    problems.push("instructions are too long (max 2000 characters)");
  }
  if (problems.length > 0) return { kind: "rejected", problems };

  const payload = {
    kind: input.kind,
    topic,
    ...(input.kind === "social_post" && (input.channel ?? "").trim()
      ? { channel: (input.channel ?? "").trim().toLowerCase() }
      : {}),
    ...(instructions ? { instructions } : {}),
  };

  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/content/generate`;
  try {
    const res = await fetchImpl(url, {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify(payload),
    });
    if (res.status === 400) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      if (body?.error) return { kind: "rejected", problems: body.error.split("; ") };
      return { kind: "unavailable", message: "The server refused the generation ask." };
    }
    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "The generated slug already exists in this workspace.",
      };
    }
    if (res.status === 502) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return { kind: "failed", message: body?.error ?? "Generation failed." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `Failed to generate content (status ${res.status}).`,
      };
    }
    const data = (await res.json()) as { version?: unknown; provider?: unknown; model?: unknown };
    const item = parseContentItem(data);
    const version = parseContentVersion(data.version);
    if (!item || !version) {
      return { kind: "unavailable", message: "Server returned a malformed generated draft." };
    }
    return {
      kind: "created",
      item,
      version,
      provider: typeof data.provider === "string" ? data.provider : "",
      model: typeof data.model === "string" ? data.model : "",
    };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export type ApprovalDecision = "approve" | "reject";

export interface ApprovalRecord {
  id: string;
  contentItemId: string;
  contentVersionId: string;
  decision: "approved" | "rejected";
  note: string | null;
  decidedAt: string;
}

export interface ContentHistory {
  versions: ContentVersion[];
  approvals: Record<string, ApprovalRecord>; // keyed by version id
}

export type ContentHistoryState =
  | { kind: "loading" }
  | { kind: "missing"; message: string }
  | { kind: "unavailable"; message: string }
  | { kind: "loaded"; history: ContentHistory };

export type DecideResult =
  | { kind: "decided"; item: ContentItem; decision: ApprovalRecord }
  | { kind: "rejected"; problems: string[] }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

function approvalUrl(baseUrl: string, workspaceId: string, itemId: string, suffix: string): string {
  return itemUrl(baseUrl, workspaceId, itemId) + suffix;
}

/**
 * GET /content/{itemId}/history: every version with its decision (if any).
 * Never throws.
 */
export async function fetchContentHistory(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ContentHistoryState> {
  try {
    const res = await fetchImpl(approvalUrl(baseUrl, workspaceId, itemId, "/history"), {
      headers: withAuth({ Accept: "application/json" }),
    });
    if (res.status === 404) {
      return { kind: "missing", message: "Workspace or content item not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as { versions?: unknown };
    if (!Array.isArray(data.versions)) {
      return { kind: "unavailable", message: "Server returned malformed history." };
    }
    const history: ContentHistory = { versions: [], approvals: {} };
    for (const raw of data.versions) {
      const praise = raw as Record<string, unknown>;
      const version = parseContentVersion(praise);
      if (!version) {
        return { kind: "unavailable", message: "Server returned malformed history entries." };
      }
      history.versions.push(version);
      if (
        praise.decision !== null &&
        typeof praise.decision === "object" &&
        praise.decision !== null
      ) {
        const d = praise.decision as Record<string, unknown>;
        const record: ApprovalRecord = {
          id: String(d.id ?? ""),
          contentItemId: String(d.contentItemId ?? ""),
          contentVersionId: String(d.contentVersionId ?? ""),
          decision: d.decision === "rejected" ? "rejected" : "approved",
          note: typeof d.note === "string" ? d.note : null,
          decidedAt: String(d.decidedAt ?? ""),
        };
        history.approvals[version.id] = record;
      }
    }
    return { kind: "loaded", history };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

/**
 * POSTs approve/reject for the item's newest version. The server records the
 * decision, moves the status, and audits it. Never throws.
 */
export async function decideContentApproval(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  decision: ApprovalDecision,
  note?: string,
  fetchImpl: typeof fetch = fetch,
): Promise<DecideResult> {
  const trimmedNote = (note ?? "").trim();
  if (trimmedNote && Array.from(trimmedNote).length > 2000) {
    return { kind: "rejected", problems: ["note is too long (max 2000 characters)"] };
  }
  const payload: Record<string, unknown> = { decision };
  if (trimmedNote) payload.note = trimmedNote;

  try {
    const res = await fetchImpl(approvalUrl(baseUrl, workspaceId, itemId, "/decision"), {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify(payload),
    });
    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "This decision is not allowed right now.",
      };
    }
    if (res.status === 400) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "rejected",
        problems: body?.error ? body.error.split("; ") : ["The decision was refused."],
      };
    }
    if (res.status === 404) {
      return { kind: "unavailable", message: "Workspace or content item not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as Record<string, unknown>;
    const item = parseContentItem(data);
    const rawDecision = data.decision as Record<string, unknown> | null;
    if (!item || !rawDecision || typeof rawDecision.id !== "string") {
      return { kind: "unavailable", message: "Server returned a malformed decision." };
    }
    const record: ApprovalRecord = {
      id: rawDecision.id,
      contentItemId: String(rawDecision.contentItemId ?? item.id),
      contentVersionId: String(rawDecision.contentVersionId ?? ""),
      decision: rawDecision.decision === "rejected" ? "rejected" : "approved",
      note: typeof rawDecision.note === "string" ? rawDecision.note : null,
      decidedAt: String(rawDecision.decidedAt ?? ""),
    };
    return { kind: "decided", item, decision: record };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export type ManualStatus = "review" | "draft";

export type TransitionStatusResult =
  | { kind: "moved"; item: ContentItem }
  | { kind: "rejected"; problems: string[] }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

/** POSTs a manual status move (submit for review / back to draft). Never throws. */
export async function transitionContentStatus(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  status: ManualStatus,
  fetchImpl: typeof fetch = fetch,
): Promise<TransitionStatusResult> {
  if (status !== "review" && status !== "draft") {
    return { kind: "rejected", problems: ["status must be review or draft"] };
  }
  try {
    const res = await fetchImpl(itemUrl(baseUrl, workspaceId, itemId) + "/status", {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify({ status }),
    });
    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return { kind: "conflict", message: body?.error ?? "That status change is not allowed." };
    }
    if (res.status === 400) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "rejected",
        problems: body?.error ? body.error.split("; ") : ["The status change was refused."],
      };
    }
    if (res.status === 404) {
      return { kind: "unavailable", message: "Workspace or content item not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const item = parseContentItem(await res.json());
    if (!item) {
      return { kind: "unavailable", message: "Server returned a malformed item." };
    }
    return { kind: "moved", item };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

/** Sanity publish modes. */
export type SanityPublishMode = "draft" | "publish";

export interface SanityPublishSuccess {
  mode: SanityPublishMode;
  item: ContentItem;
  jobId: string | null;
  draftId: string | null;
  transactionId: string | null;
  publicUrl: string | null;
  urlVerified: boolean | null;
  message: string;
  /** True when the server accepted a schedule and queued the job for the worker. */
  scheduled: boolean;
}

export type SanityPublishResult =
  | { kind: "pushed"; success: SanityPublishSuccess }
  | { kind: "rejected"; problems: string[] }
  | { kind: "conflict"; message: string }
  | { kind: "failed"; message: string } // Sanity-side write failure (502)
  | { kind: "unavailable"; message: string };

/**
 * POSTs /content/{itemId}/publish for one Sanity integration.
 * mode "draft" writes (or replaces) the invisible Sanity draft — no approval
 * needed. mode "publish" pushes the approved version live; the server enforces
 * the approval gate and records the job/artifact. Never throws.
 * A future scheduledAt queues the publish for the background worker instead of
 * running it inline and answers 202 with the queued job id.
 */
export async function pushToSanity(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  integrationId: string,
  mode: SanityPublishMode,
  scheduledAt?: string,
  fetchImpl: typeof fetch = fetch,
): Promise<SanityPublishResult> {
  const problems: string[] = [];
  const trimmedIntegration = (integrationId ?? "").trim();
  if (!trimmedIntegration) problems.push("integrationId is required");
  if (mode !== "draft" && mode !== "publish") {
    problems.push("mode must be draft or publish");
  }
  if (problems.length > 0) return { kind: "rejected", problems };

  const payload: Record<string, unknown> = { integrationId: trimmedIntegration, mode };
  if (scheduledAt) payload.scheduledAt = scheduledAt;

  const url = itemUrl(baseUrl, workspaceId, itemId) + "/publish";
  try {
    const res = await fetchImpl(url, {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify(payload),
    });
    if (res.status === 400) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "rejected",
        problems: body?.error ? body.error.split("; ") : ["The request was refused."],
      };
    }
    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "This publishing step is not allowed right now.",
      };
    }
    if (res.status === 502) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return { kind: "failed", message: body?.error ?? "The Sanity write failed." };
    }
    if (res.status === 404) {
      return {
        kind: "unavailable",
        message: "Workspace, content item, or integration not found.",
      };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as Record<string, unknown>;
    const item = parseContentItem(data.item);
    if (!item) {
      return { kind: "unavailable", message: "Server returned a malformed publish reply." };
    }
    return {
      kind: "pushed",
      success: {
        mode: data.mode === "publish" ? "publish" : "draft",
        item,
        jobId: typeof data.jobId === "string" ? data.jobId : null,
        draftId: typeof data.draftId === "string" ? data.draftId : null,
        transactionId: typeof data.transactionId === "string" ? data.transactionId : null,
        publicUrl:
          typeof data.publicUrl === "string" || data.publicUrl === null
            ? ((data.publicUrl as string | null) ?? null)
            : null,
        urlVerified: typeof data.urlVerified === "boolean" ? data.urlVerified : null,
        message: typeof data.message === "string" ? data.message : "",
        scheduled: res.status === 202,
      },
    };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

// ---- Publishing jobs ---------------------------------------------------------

export type PublishingJobStatus = "pending" | "running" | "succeeded" | "failed" | "cancelled";

export interface PublishingJob {
  id: string;
  contentItemId: string;
  contentVersionId: string;
  integrationId: string;
  status: PublishingJobStatus;
  scheduledAt: string | null;
  attempts: number;
  lastError: string | null;
  nextAttemptAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export type ContentJobsState =
  | { kind: "loading" }
  | { kind: "missing"; message: string }
  | { kind: "unavailable"; message: string }
  | { kind: "loaded"; jobs: PublishingJob[] };

export function formatPublishJobStatus(status: string): string {
  switch (status) {
    case "pending":
      return "Queued";
    case "running":
      return "Running";
    case "succeeded":
      return "Published";
    case "failed":
      return "Failed";
    case "cancelled":
      return "Cancelled";
    default:
      return status;
  }
}

function parsePublishingJob(raw: unknown): PublishingJob | null {
  if (typeof raw !== "object" || raw === null) return null;
  const j = raw as Record<string, unknown>;
  if (
    typeof j.id !== "string" ||
    typeof j.contentItemId !== "string" ||
    typeof j.contentVersionId !== "string" ||
    typeof j.integrationId !== "string" ||
    typeof j.status !== "string" ||
    typeof j.attempts !== "number" ||
    typeof j.createdAt !== "string" ||
    typeof j.updatedAt !== "string"
  ) {
    return null;
  }
  const optionalString = (v: unknown): string | null =>
    typeof v === "string" && v !== "" ? v : null;
  return {
    id: j.id,
    contentItemId: j.contentItemId,
    contentVersionId: j.contentVersionId,
    integrationId: j.integrationId,
    status: j.status as PublishingJobStatus,
    scheduledAt: optionalString(j.scheduledAt),
    attempts: j.attempts,
    lastError: optionalString(j.lastError),
    nextAttemptAt: optionalString(j.nextAttemptAt),
    createdAt: j.createdAt,
    updatedAt: j.updatedAt,
  };
}

/**
 * GET /content/{itemId}/jobs: this item's publishing history, newest first.
 * Never throws.
 */
export async function fetchContentJobs(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ContentJobsState> {
  try {
    const res = await fetchImpl(`${itemUrl(baseUrl, workspaceId, itemId)}/jobs`, {
      headers: withAuth({ Accept: "application/json" }),
    });
    if (res.status === 404) {
      return { kind: "missing", message: "Workspace or content item not found." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as { jobs?: unknown };
    if (!Array.isArray(data.jobs)) {
      return { kind: "unavailable", message: "Server returned malformed publishing jobs." };
    }
    const jobs: PublishingJob[] = [];
    for (const raw of data.jobs) {
      const parsed = parsePublishingJob(raw);
      if (!parsed) {
        return { kind: "unavailable", message: "Server returned malformed publishing jobs." };
      }
      jobs.push(parsed);
    }
    return { kind: "loaded", jobs };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export type CancelJobResult =
  | { kind: "cancelled"; job: PublishingJob }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

/** POSTs /jobs/{jobId}/cancel for one queued or failed job. Never throws. */
export async function cancelContentJob(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  jobId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<CancelJobResult> {
  if (!jobId) return { kind: "conflict", message: "A job id is required." };
  return postJobAction(
    baseUrl,
    workspaceId,
    itemId,
    jobId,
    "cancel",
    (data): CancelJobResult => {
      const job = parsePublishingJob(data.job);
      return job && job.status === "cancelled"
        ? { kind: "cancelled", job }
        : { kind: "unavailable", message: "Server returned a malformed cancel result." };
    },
    fetchImpl,
  );
}

export type RetryJobResult =
  | { kind: "requeued"; job: PublishingJob }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

/** POSTs /jobs/{jobId}/retry for one failed job. Never throws. */
export async function retryContentJob(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  jobId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<RetryJobResult> {
  if (!jobId) return { kind: "conflict", message: "A job id is required." };
  return postJobAction(
    baseUrl,
    workspaceId,
    itemId,
    jobId,
    "retry",
    (data): RetryJobResult => {
      const job = parsePublishingJob(data.job);
      return job && job.status === "pending"
        ? { kind: "requeued", job }
        : { kind: "unavailable", message: "Server returned a malformed retry result." };
    },
    fetchImpl,
  );
}

async function postJobAction<T>(
  baseUrl: string,
  workspaceId: string,
  itemId: string,
  jobId: string,
  action: "cancel" | "retry",
  build: (data: Record<string, unknown>) => T,
  fetchImpl: typeof fetch,
): Promise<T | { kind: "conflict"; message: string } | { kind: "unavailable"; message: string }> {
  const url = `${itemUrl(baseUrl, workspaceId, itemId)}/jobs/${encodeURIComponent(jobId)}/${action}`;
  try {
    const res = await fetchImpl(url, { method: "POST", headers: withAuth() });
    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "That job action is not allowed right now.",
      };
    }
    if (res.status === 404) {
      return { kind: "conflict", message: "That job no longer exists in this item's history." };
    }
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `The server answered with status ${res.status}.`,
      };
    }
    const data = (await res.json()) as Record<string, unknown>;
    return build(data);
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}
