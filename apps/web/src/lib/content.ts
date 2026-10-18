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
  createdAt: string;
  updatedAt: string;
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

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$|^[a-z0-9]$/;

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
    createdAt: c.createdAt,
    updatedAt: c.updatedAt,
  };
}

export async function fetchWorkspaceContent(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ContentListState> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/content`;
  try {
    const res = await fetchImpl(url, { headers: { Accept: "application/json" } });
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
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
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
