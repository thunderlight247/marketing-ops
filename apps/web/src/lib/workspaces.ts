import { withAuth } from "@/lib/config";
export interface Workspace {
  id: string;
  slug: string;
  name: string;
  websiteUrl: string | null;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface BrandProfile {
  businessDescription: string | null;
  audience: string | null;
  services: string | null;
  tone: string | null;
  rules: Record<string, unknown>;
  prohibitedClaims: string[];
  callsToAction: string[];
  updatedAt: string;
}

export type WorkspaceDetailState =
  | { kind: "loading" }
  | { kind: "missing"; reason: string }
  | { kind: "unavailable"; reason: string }
  | { kind: "loaded"; workspace: Workspace; brand: BrandProfile | null };

export type SaveBrandInput = {
  businessDescription: string | null;
  audience: string | null;
  services: string | null;
  tone: string | null;
  rules: Record<string, unknown>;
  prohibitedClaims: string[];
  callsToAction: string[];
};

export type SaveBrandResult =
  | { kind: "saved"; brand: BrandProfile }
  | { kind: "rejected"; message: string }
  | { kind: "unavailable"; reason: string };

export type WorkspaceFactsInput = {
  name: string;
  websiteUrl: string | null;
  status: "active" | "archived";
};

export type WorkspaceFactsResult =
  | { kind: "saved"; workspace: Workspace }
  | { kind: "rejected"; message: string }
  | { kind: "unavailable"; reason: string }
  | { kind: "missing"; reason: string };

export type DeleteWorkspaceResult =
  | { kind: "deleted" }
  | { kind: "rejected"; message: string }
  | { kind: "unavailable"; reason: string }
  | { kind: "missing"; reason: string };

/** Client-side facts validation mirroring the API's rules. */
export function validateWorkspaceFacts(input: WorkspaceFactsInput): string[] {
  const problems: string[] = [];
  if (input.name.trim().length === 0) problems.push("A name is required.");
  else if (input.name.trim().length > 120) problems.push("Names are at most 120 characters.");
  if (!"active/archived".split("/").includes(input.status)) {
    problems.push("The status must be active or archived.");
  }
  return problems;
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

function parseBrand(data: unknown): BrandProfile | null {
  if (!isRecord(data)) return null;
  const strings = ["businessDescription", "audience", "services", "tone"] as const;
  for (const key of strings) {
    const value = data[key];
    if (value !== null && typeof value !== "string") return null;
  }
  if (!isRecord(data.rules)) return null;
  if (data.prohibitedClaims !== null && !Array.isArray(data.prohibitedClaims)) return null;
  if (data.callsToAction !== null && !Array.isArray(data.callsToAction)) return null;
  for (const key of ["prohibitedClaims", "callsToAction"] as const) {
    if (data[key] === null) return null;
    if (!Array.isArray(data[key]) || !data[key].every((item) => typeof item === "string"))
      return null;
  }
  if (typeof data.updatedAt !== "string" || data.updatedAt.length === 0) return null;
  return {
    businessDescription: data.businessDescription as string | null,
    audience: data.audience as string | null,
    services: data.services as string | null,
    tone: data.tone as string | null,
    rules: data.rules as Record<string, unknown>,
    prohibitedClaims: data.prohibitedClaims as string[],
    callsToAction: data.callsToAction as string[],
    updatedAt: data.updatedAt as string,
  };
}

/**
 * Fetches GET {baseUrl}/api/v1/workspaces/{id}. 404s become a "missing"
 * state; other failures become "unavailable"; malformed bodies too.
 */
export async function fetchWorkspaceDetail(
  baseUrl: string,
  id: string,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 4000,
): Promise<WorkspaceDetailState> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${baseUrl}/api/v1/workspaces/${encodeURIComponent(id)}`, {
      signal: controller.signal,
      headers: withAuth(),
      cache: "no-store",
    });
    if (response.status === 404) {
      return { kind: "missing", reason: await readErrorResponse(response) };
    }
    if (!response.ok) {
      return { kind: "unavailable", reason: await readErrorResponse(response) };
    }
    const body = (await response.json()) as unknown;
    if (!isRecord(body)) {
      return {
        kind: "unavailable",
        reason: "The API answered, but the body was not a workspace detail.",
      };
    }
    const workspace = parseWorkspace(body.workspace);
    if (!workspace) {
      return {
        kind: "unavailable",
        reason: "The API answered, but the body was not a workspace detail.",
      };
    }
    const brand = body.brand === null ? null : parseBrand(body.brand);
    if (body.brand !== null && !brand) {
      return {
        kind: "unavailable",
        reason: "The API answered, but the brand profile was malformed.",
      };
    }
    return { kind: "loaded", workspace, brand };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
}

function urlencodeId(id: string): string {
  return encodeURIComponent(id);
}

/** PATCHes the editable facts of one workspace. */
export async function updateWorkspaceFacts(
  baseUrl: string,
  id: string,
  input: WorkspaceFactsInput,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 6000,
): Promise<WorkspaceFactsResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${baseUrl}/api/v1/workspaces/${urlencodeId(id)}`, {
      method: "PATCH",
      headers: withAuth({ "Content-Type": "application/json" }),
      signal: controller.signal,
      cache: "no-store",
      body: JSON.stringify({
        name: input.name.trim(),
        websiteUrl: input.websiteUrl?.trim() || null,
        status: input.status,
      }),
    });
    if (response.status === 404) {
      return { kind: "missing", reason: await readErrorResponse(response) };
    }
    if (!response.ok) {
      return { kind: "rejected", message: await readErrorResponse(response) };
    }
    const workspace = parseWorkspace(await response.json());
    if (!workspace) {
      return { kind: "unavailable", reason: "The API answered, but the body was not a workspace." };
    }
    return { kind: "saved", workspace };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
}

/** DELETEs a workspace; confirmSlug must match the workspace slug. */
export async function deleteWorkspace(
  baseUrl: string,
  id: string,
  confirmSlug: string,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 8000,
): Promise<DeleteWorkspaceResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(`${baseUrl}/api/v1/workspaces/${urlencodeId(id)}`, {
      method: "DELETE",
      headers: withAuth({ "Content-Type": "application/json" }),
      signal: controller.signal,
      cache: "no-store",
      body: JSON.stringify({ confirmSlug }),
    });
    if (response.status === 404) {
      return { kind: "missing", reason: await readErrorResponse(response) };
    }
    if (response.status === 204) {
      return { kind: "deleted" };
    }
    return { kind: "rejected", message: await readErrorResponse(response) };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
}

/** PUTs the brand profile for one workspace. */
export async function saveBrandProfile(
  baseUrl: string,
  id: string,
  input: SaveBrandInput,
  fetchImpl: typeof fetch = fetch,
  timeoutMs = 6000,
): Promise<SaveBrandResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(
      `${baseUrl}/api/v1/workspaces/${encodeURIComponent(id)}/brand`,
      {
        method: "PUT",
        headers: withAuth({ "Content-Type": "application/json" }),
        signal: controller.signal,
        cache: "no-store",
        body: JSON.stringify(input),
      },
    );
    if (!response.ok) {
      return { kind: "rejected", message: await readErrorResponse(response) };
    }
    const brand = parseBrand(await response.json());
    if (!brand) {
      return {
        kind: "unavailable",
        reason: "The API answered, but the body was not a brand profile.",
      };
    }
    return { kind: "saved", brand };
  } catch {
    return { kind: "unavailable", reason: `Could not reach the API at ${baseUrl}.` };
  } finally {
    clearTimeout(timer);
  }
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
      headers: withAuth(),
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
      headers: withAuth({ "Content-Type": "application/json" }),
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
