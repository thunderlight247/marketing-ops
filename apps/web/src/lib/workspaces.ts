export interface Workspace {
  id: string;
  slug: string;
  name: string;
  websiteUrl: string | null;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export type WorkspacesState =
  | { kind: "loading" }
  | { kind: "unavailable"; reason: string }
  | { kind: "loaded"; workspaces: Workspace[] };

export interface CreateWorkspaceInput {
  slug: string;
  name: string;
  websiteUrl?: string;
}

export type CreateWorkspaceResult =
  | { kind: "created"; workspace: Workspace }
  | { kind: "rejected"; message: string }
  | { kind: "unavailable"; reason: string };

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$|^[a-z0-9]$/;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Validates unknown JSON as a workspace. Returns null if it does not match. */
export function parseWorkspace(data: unknown): Workspace | null {
  if (!isRecord(data)) return null;
  const stringsOk = (["id", "slug", "name", "status", "createdAt", "updatedAt"] as const).every(
    (key) => typeof data[key] === "string" && (data[key] as string).length > 0,
  );
  if (!stringsOk) return null;
  if (data.websiteUrl !== null && typeof data.websiteUrl !== "string") return null;
  return {
    id: data.id as string,
    slug: data.slug as string,
    name: data.name as string,
    websiteUrl: data.websiteUrl as string | null,
    status: data.status as string,
    createdAt: data.createdAt as string,
    updatedAt: data.updatedAt as string,
  };
}

/** Validates a workspaces list response body. Returns null if it is not well-formed. */
export function parseWorkspaces(data: unknown): Workspace[] | null {
  if (!isRecord(data) || !Array.isArray(data.workspaces)) return null;
  const out: Workspace[] = [];
  for (const item of data.workspaces) {
    const workspace = parseWorkspace(item);
    if (!workspace) return null;
    out.push(workspace);
  }
  return out;
}

/** Client-side check mirroring the API's validation, so obvious mistakes never leave the form. */
export function validateWorkspaceInput(input: CreateWorkspaceInput): string[] {
  const problems: string[] = [];
  const slug = input.slug.trim();
  if (slug === "") problems.push("A URL slug is required.");
  else if (!SLUG_PATTERN.test(slug))
    problems.push("Slugs use lowercase letters, digits, or hyphens.");
  if (input.name.trim().length === 0) problems.push("A name is required.");
  else if (input.name.trim().length > 120) problems.push("Names are at most 120 characters.");
  return problems;
}

async function readErrorResponse(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as unknown;
    if (isRecord(body) && typeof body.error === "string" && body.error.length > 0) {
      return body.error;
    }
  } catch {
    // fall through to the generic message
  }
  return `The API answered with HTTP ${response.status}.`;
}

/**
 * Fetches GET {baseUrl}/api/v1/workspaces. Never throws: failures become an
 * "unavailable" state, and malformed bodies are treated as unavailable too.
 */
export async function fetchWorkspaces(
  baseUrl: string,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 4000,
): Promise<WorkspacesState> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${baseUrl}/api/v1/workspaces`, {
      signal: controller.signal,
      cache: "no-store",
    });
    if (!response.ok) {
      return { kind: "unavailable", reason: await readErrorResponse(response) };
    }
    const workspaces = parseWorkspaces(await response.json());
    if (!workspaces) {
      return {
        kind: "unavailable",
        reason: "The API answered, but the response was not a workspace list.",
      };
    }
    return { kind: "loaded", workspaces };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
}

/** POSTs a new workspace. Network failures map to "unavailable"; 400/409 responses map to "rejected". */
export async function createWorkspace(
  baseUrl: string,
  input: CreateWorkspaceInput,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 6000,
): Promise<CreateWorkspaceResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${baseUrl}/api/v1/workspaces`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      signal: controller.signal,
      cache: "no-store",
      body: JSON.stringify({
        slug: input.slug.trim(),
        name: input.name.trim(),
        websiteUrl: input.websiteUrl?.trim() || undefined,
      }),
    });
    if (!response.ok) {
      return { kind: "rejected", message: await readErrorResponse(response) };
    }
    const workspace = parseWorkspace(await response.json());
    if (!workspace) {
      return { kind: "unavailable", reason: "The API answered, but the body was not a workspace." };
    }
    return { kind: "created", workspace };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
}
