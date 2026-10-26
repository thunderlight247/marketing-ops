import { withAuth } from "@/lib/config";
// Agency Agents client library (first specialist: SEO advisor).
// Mirrors lib/campaigns.ts: explicit result kinds, strict response parsing,
// and no thrown errors on clean failures.

export interface AgentRecommendation {
  priority: string; // high | medium | low (server normalizes)
  area: string;
  page?: string;
  issue: string;
  evidence: string;
  action: string;
}

export interface AdvisorReport {
  agentId: string;
  agentVersion: string;
  provider: string;
  model: string;
  dataSource: "search_console" | "none";
  generatedAt: string;
  cannibalizationChecked: boolean;
  summary: string;
  gaps: string[];
  recommendations: AgentRecommendation[];
}

export interface SeoAdvisoryEnvelope {
  report: AdvisorReport;
  dataNote: string;
  durationMs: number;
}

export type SeoAdvisoryState =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done"; report: AdvisorReport; dataNote: string; durationMs: number }
  | { kind: "unavailable"; message: string };

export function formatRecommendationPriority(priority: string): string {
  const labels: Record<string, string> = {
    high: "High",
    medium: "Medium",
    low: "Low",
  };
  return labels[priority] ?? priority;
}

export function formatDataSource(dataSource: string): string {
  if (dataSource === "search_console") return "Search Console page+query data";
  return "No connected Search Console data (advisory only)";
}

// agentRunUrl points at the single generic endpoint every registered
// specialist answers; the agent id selects the persona, not the route.
function agentRunUrl(baseUrl: string, workspaceId: string, agentId: string): string {
  const root = baseUrl.replace(/\/+$/, "");
  return `${root}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/agents/${encodeURIComponent(agentId)}/run`;
}

// requestSeoAdvisory is the network call for the advisory report. A report
// the server could not validate arrives as "rejected" with the safe message.
const seoAgentId = "seo_specialist";
const creatorAgentId = "content_creator";
export async function requestSeoAdvisory(
  baseUrl: string,
  workspaceId: string,
  focus: string,
  fetchImpl: typeof fetch = fetch,
): Promise<SeoAdvisoryState> {
  try {
    const res = await fetchImpl(agentRunUrl(baseUrl, workspaceId, seoAgentId), {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify(focus.trim() === "" ? {} : { focus: focus.trim() }),
    });
    if (!res.ok) {
      const detail = await res.json().catch(() => null);
      const message =
        detail && typeof detail.error === "string"
          ? detail.error
          : `request failed (${res.status})`;
      return { kind: "unavailable", message };
    }
    const body = parseEnvelope(await res.json().catch(() => null));
    if (!body) {
      return { kind: "unavailable", message: "the advisory response had an unexpected shape" };
    }
    if (!isValidReport(body.report)) {
      return { kind: "unavailable", message: "the advisory response was not a valid report" };
    }
    return {
      kind: "done",
      report: body.report,
      dataNote: typeof body.dataNote === "string" ? body.dataNote : "",
      durationMs: typeof body.durationMs === "number" ? body.durationMs : 0,
    };
  } catch {
    return { kind: "unavailable", message: "the API could not be reached" };
  }
}

function parseEnvelope(
  value: unknown,
): { report: unknown; dataNote: unknown; durationMs: unknown } | null {
  if (!isRecord(value)) return null;
  if (!isRecord(value.report)) return null;
  return { report: value.report, dataNote: value.dataNote, durationMs: value.durationMs };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

// isValidReport is a client-side shape check only; the server already
// validated the reply and enforced the data-honesty rules.
function isValidReport(value: unknown): value is AdvisorReport {
  if (!isRecord(value)) return false;
  const report = value as Record<string, unknown>;
  const recommendations = report.recommendations;
  if (!Array.isArray(recommendations)) return false;
  for (const rec of recommendations) {
    if (!isRecord(rec) || typeof rec.issue !== "string" || typeof rec.action !== "string") {
      return false;
    }
  }
  return (
    typeof report.agentId === "string" &&
    typeof report.dataSource === "string" &&
    typeof report.summary === "string" &&
    (report.gaps === undefined || Array.isArray(report.gaps))
  );
}

// ---- Registry: what agents exist -------------------------------------------
// GET /api/v1/agents lists every enabled specialist's dashboard-facing
// descriptor. The dashboard shows these capabilities; only the server's own
// gating keeps an agent from doing more than it lists here.

export interface AgentDescriptor {
  id: string;
  version: string;
  displayName: string;
  description: string;
  outputContract: string;
  dataNeeds: string[];
  actions: string[];
}

export type AgentsRegistryState =
  | { kind: "loading" }
  | { kind: "available"; agents: AgentDescriptor[] }
  | { kind: "unavailable"; message: string };

export function formatAgentContract(contract: string): string {
  const labels: Record<string, string> = {
    advisory: "Advisory only — returns a report, never edits",
    drafting: "Drafts into the human approval workflow",
  };
  return labels[contract] ?? contract;
}

export function formatAgentDataNeed(need: string): string {
  const labels: Record<string, string> = {
    search_console: "Search Console data",
  };
  return labels[need] ?? need;
}

export function formatAgentAction(action: string): string {
  const labels: Record<string, string> = {
    file_draft: "May file drafts in the approval workflow",
  };
  return labels[action] ?? action;
}

// fetchAgentRegistry reads the enabled-agents list. A response the client
// could not trust maps to "unavailable" with a safe message, never a
// partially parsed descriptor.
export async function fetchAgentRegistry(
  baseUrl: string,
  fetchImpl: typeof fetch = fetch,
): Promise<AgentsRegistryState> {
  const root = baseUrl.replace(/\/+$/, "");
  try {
    const res = await fetchImpl(`${root}/api/v1/agents`, {
      headers: withAuth({ Accept: "application/json" }),
    });
    if (!res.ok) {
      const detail = await res.json().catch(() => null);
      const message =
        detail && typeof detail.error === "string"
          ? detail.error
          : `request failed (${res.status})`;
      return { kind: "unavailable", message };
    }
    const body = await res.json().catch(() => null);
    if (!isRecord(body) || !Array.isArray(body.agents)) {
      return { kind: "unavailable", message: "the agents response had an unexpected shape" };
    }
    const agents: AgentDescriptor[] = [];
    for (const entry of body.agents) {
      const agent = parseDescriptor(entry);
      if (!agent) {
        return { kind: "unavailable", message: "the agents response had an unexpected shape" };
      }
      agents.push(agent);
    }
    return { kind: "available", agents };
  } catch {
    return { kind: "unavailable", message: "the API could not be reached" };
  }
}

function parseDescriptor(value: unknown): AgentDescriptor | null {
  if (!isRecord(value)) return null;
  const id = typeof value.id === "string" ? value.id : "";
  const version = typeof value.version === "string" ? value.version : "";
  if (id === "" || version === "") return null;
  return {
    id,
    version,
    displayName: typeof value.displayName === "string" ? value.displayName : id,
    description: typeof value.description === "string" ? value.description : "",
    outputContract: typeof value.outputContract === "string" ? value.outputContract : "",
    dataNeeds: stringList(value.dataNeeds),
    actions: stringList(value.actions),
  };
}

function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string");
}

// ---- Second specialist: content creator ------------------------------------

export interface CreatorDraftVersion {
  versionNumber: number;
  excerpt: string | null;
  body: string | null;
  callToAction: string | null;
}

export interface CreatorDraftEnvelope {
  item: {
    id: string;
    kind: string;
    status: string;
    title: string;
    slug: string | null;
  };
  version: CreatorDraftVersion;
  provider: string;
  model: string;
  agentId: string;
  keyPoints: string[];
  durationMs: number;
  slugSuggestion: string | null;
}

export type CreatorDraftState =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done"; draft: CreatorDraftEnvelope }
  | { kind: "unavailable"; message: string };

export const contentKinds: ReadonlyArray<{ value: string; label: string }> = [
  { value: "blog_article", label: "Blog article" },
  { value: "social_post", label: "Social post" },
  { value: "snippet", label: "Snippet" },
];

// requestContentDraft files the creator's plan as a draft content item in the
// existing approval workflow. Brand rules guide the prompt; human review
// remains required. The client checks the response shape.
export async function requestContentDraft(
  baseUrl: string,
  workspaceId: string,
  kind: string,
  focus: string,
  fetchImpl: typeof fetch = fetch,
): Promise<CreatorDraftState> {
  try {
    const res = await fetchImpl(agentRunUrl(baseUrl, workspaceId, creatorAgentId), {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify({
        kind,
        ...(focus.trim() === "" ? {} : { focus: focus.trim() }),
      }),
    });
    if (!res.ok) {
      const detail = await res.json().catch(() => null);
      const message =
        detail && typeof detail.error === "string"
          ? detail.error
          : `request failed (${res.status})`;
      return { kind: "unavailable", message };
    }
    const draft = parseCreatorEnvelope(await res.json().catch(() => null));
    if (!draft) {
      return { kind: "unavailable", message: "the draft response had an unexpected shape" };
    }
    return { kind: "done", draft };
  } catch {
    return { kind: "unavailable", message: "the API could not be reached" };
  }
}

function parseCreatorEnvelope(value: unknown): CreatorDraftEnvelope | null {
  if (!isRecord(value) || !isRecord(value.item) || !isRecord(value.version)) return null;
  if (!Array.isArray(value.keyPoints)) return null;
  const item = value.item;
  const version = value.version;
  if (typeof item.id !== "string" || typeof item.title !== "string") return null;
  if (typeof version.versionNumber !== "number") return null;
  return {
    item: {
      id: item.id,
      kind: typeof item.kind === "string" ? item.kind : "",
      status: typeof item.status === "string" ? item.status : "",
      title: item.title,
      slug: typeof item.slug === "string" ? item.slug : null,
    },
    version: {
      versionNumber: version.versionNumber,
      excerpt: typeof version.excerpt === "string" ? version.excerpt : null,
      body: typeof version.body === "string" ? version.body : null,
      callToAction: typeof version.callToAction === "string" ? version.callToAction : null,
    },
    provider: typeof value.provider === "string" ? value.provider : "",
    model: typeof value.model === "string" ? value.model : "",
    agentId: typeof value.agentId === "string" ? value.agentId : "content_creator",
    keyPoints: value.keyPoints.filter((p): p is string => typeof p === "string"),
    durationMs: typeof value.durationMs === "number" ? value.durationMs : 0,
    slugSuggestion:
      typeof value.slugSuggestion === "string" && value.slugSuggestion !== ""
        ? value.slugSuggestion
        : null,
  };
}
