// Campaign and lead client library (Milestone 11, first increment).
// Network layer mirrors lib/integrations.ts: explicit result kinds, strict
// response parsing, and no thrown errors on clean failures.

export type CampaignStatus = "planned" | "active" | "paused" | "completed";
export type LeadStatus = "new" | "contacted" | "qualified" | "won" | "lost";
export type LeadSourceKind = "manual" | "form_api" | "import" | "unknown";

export interface Campaign {
  id: string;
  workspaceId: string;
  name: string;
  status: CampaignStatus;
  utmCampaign: string;
  notes: string;
  createdAt: string;
  updatedAt: string;
}

export interface Lead {
  id: string;
  workspaceId: string;
  campaignId: string | null;
  sourceKind: LeadSourceKind;
  sourceDetail: string;
  status: LeadStatus;
  contactName: string | null;
  contactEmail: string | null;
  contactPhone: string | null;
  message: string | null;
  notes: string;
  utmSource: string | null;
  utmMedium: string | null;
  utmCampaign: string | null;
  followUpAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export type CampaignsLeadsState =
  | { kind: "loading" }
  | { kind: "loaded"; campaigns: Campaign[]; leads: Lead[] }
  | { kind: "unavailable"; message: string }
  | { kind: "missing"; message: string };

export type CampaignInput = { name: string; status?: string; utmCampaign?: string; notes?: string };

export type CreateCampaignResult =
  | { kind: "created"; campaign: Campaign }
  | { kind: "conflict"; message: string }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export type CreateLeadResult =
  | { kind: "created"; lead: Lead }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export type UpdateLeadResult =
  | { kind: "saved"; lead: Lead }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export function formatCampaignStatus(status: CampaignStatus): string {
  const labels: Record<CampaignStatus, string> = {
    planned: "Planned",
    active: "Active",
    paused: "Paused",
    completed: "Completed",
  };
  return labels[status] ?? status;
}

export function formatLeadStatus(status: LeadStatus): string {
  const labels: Record<LeadStatus, string> = {
    new: "New",
    contacted: "Contacted",
    qualified: "Qualified",
    won: "Won",
    lost: "Lost",
  };
  return labels[status] ?? status;
}

export function formatLeadSource(kind: LeadSourceKind): string {
  const labels: Record<LeadSourceKind, string> = {
    manual: "Recorded manually",
    form_api: "Form / API",
    import: "Imported",
    unknown: "Source unknown",
  };
  return labels[kind] ?? kind;
}

export function validateCampaignInput(input: CampaignInput): string[] {
  const problems: string[] = [];
  const name = (input.name ?? "").trim();
  if (!name) {
    problems.push("campaign name is required");
  } else if (Array.from(name).length > 120) {
    problems.push("campaign name is too long (max 120 characters)");
  }
  if (input.status !== undefined && input.status !== "" && !isCampaignStatus(input.status)) {
    problems.push("status must be planned, active, paused, or completed");
  }
  return problems;
}

export function isCampaignStatus(value: string): value is CampaignStatus {
  return value === "planned" || value === "active" || value === "paused" || value === "completed";
}

export function isLeadStatus(value: string): value is LeadStatus {
  return (
    value === "new" ||
    value === "contacted" ||
    value === "qualified" ||
    value === "won" ||
    value === "lost"
  );
}

export function isLeadSource(value: string): value is LeadSourceKind {
  return value === "manual" || value === "form_api" || value === "import" || value === "unknown";
}

// validateLeadInput only checks what the UI actually collects. The source is
// a select over traceable origins plus "unknown", never a free text that can
// pretend to be measured data.
export function validateLeadInput(input: {
  sourceKind: string;
  contactName?: string;
  contactEmail?: string;
  campaignId?: string;
  followUpAt?: string;
}): string[] {
  const problems: string[] = [];
  if (!isLeadSource(input.sourceKind)) {
    problems.push("sourceKind must be form/api, import, manual, or unknown");
  }
  if (
    input.contactEmail &&
    input.contactEmail.trim() !== "" &&
    !/^[^\s@]+@[^\s@]+$/.test(input.contactEmail.trim())
  ) {
    problems.push("email does not look valid");
  }
  if (
    input.followUpAt &&
    input.followUpAt.trim() !== "" &&
    Number.isNaN(Date.parse(input.followUpAt))
  ) {
    problems.push("follow-up date is not a valid date");
  }
  return problems;
}

function parseCampaign(raw: unknown): Campaign | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (
    typeof obj.id !== "string" ||
    typeof obj.workspaceId !== "string" ||
    typeof obj.name !== "string" ||
    typeof obj.status !== "string" ||
    !isCampaignStatus(obj.status)
  ) {
    return null;
  }
  return {
    id: obj.id,
    workspaceId: obj.workspaceId,
    name: obj.name,
    status: obj.status,
    utmCampaign: typeof obj.utmCampaign === "string" ? obj.utmCampaign : "",
    notes: typeof obj.notes === "string" ? obj.notes : "",
    createdAt: typeof obj.createdAt === "string" ? obj.createdAt : "",
    updatedAt: typeof obj.updatedAt === "string" ? obj.updatedAt : "",
  };
}

function optString(obj: Record<string, unknown>, key: string): string | null {
  const v = obj[key];
  return typeof v === "string" ? v : null;
}

function parseLead(raw: unknown): Lead | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (
    typeof obj.id !== "string" ||
    typeof obj.workspaceId !== "string" ||
    typeof obj.sourceKind !== "string" ||
    !isLeadSource(obj.sourceKind) ||
    typeof obj.status !== "string" ||
    !isLeadStatus(obj.status)
  ) {
    return null;
  }
  return {
    id: obj.id,
    workspaceId: obj.workspaceId,
    campaignId: optString(obj, "campaignId"),
    sourceKind: obj.sourceKind,
    sourceDetail: typeof obj.sourceDetail === "string" ? obj.sourceDetail : "",
    status: obj.status,
    contactName: optString(obj, "contactName"),
    contactEmail: optString(obj, "contactEmail"),
    contactPhone: optString(obj, "contactPhone"),
    message: optString(obj, "message"),
    notes: typeof obj.notes === "string" ? obj.notes : "",
    utmSource: optString(obj, "utmSource"),
    utmMedium: optString(obj, "utmMedium"),
    utmCampaign: optString(obj, "utmCampaign"),
    followUpAt: optString(obj, "followUpAt"),
    createdAt: typeof obj.createdAt === "string" ? obj.createdAt : "",
    updatedAt: typeof obj.updatedAt === "string" ? obj.updatedAt : "",
  };
}

export async function fetchWorkspaceCampaignsLeads(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<CampaignsLeadsState> {
  const root = baseUrl.replace(/\/+$/, "");
  try {
    const [campaignsRes, leadsRes] = await Promise.all([
      fetchImpl(`${root}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/campaigns`, {
        headers: { Accept: "application/json" },
      }),
      fetchImpl(`${root}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/leads`, {
        headers: { Accept: "application/json" },
      }),
    ]);
    if (campaignsRes.status === 404 || leadsRes.status === 404) {
      return { kind: "missing", message: "Workspace not found." };
    }
    if (!campaignsRes.ok || !leadsRes.ok) {
      const code = !campaignsRes.ok ? campaignsRes.status : leadsRes.status;
      return { kind: "unavailable", message: `The server answered with status ${code}.` };
    }
    const campaignData = (await campaignsRes.json()) as { campaigns?: unknown[] };
    const leadData = (await leadsRes.json()) as { leads?: unknown[] };
    if (!Array.isArray(campaignData.campaigns) || !Array.isArray(leadData.leads)) {
      return { kind: "unavailable", message: "Server returned malformed campaign/lead data." };
    }
    const campaigns: Campaign[] = [];
    for (const raw of campaignData.campaigns) {
      const parsed = parseCampaign(raw);
      if (parsed) campaigns.push(parsed);
    }
    const leads: Lead[] = [];
    for (const raw of leadData.leads) {
      const parsed = parseLead(raw);
      if (parsed) leads.push(parsed);
    }
    return { kind: "loaded", campaigns, leads };
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Network request failed.",
    };
  }
}

export async function createCampaign(
  baseUrl: string,
  workspaceId: string,
  input: CampaignInput,
  fetchImpl: typeof fetch = fetch,
): Promise<CreateCampaignResult> {
  const problems = validateCampaignInput(input);
  if (problems.length > 0) {
    return { kind: "rejected", problems };
  }
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/campaigns`;
  let res: Response;
  try {
    res = await fetchImpl(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({
        name: input.name.trim(),
        status: input.status,
        utmCampaign: (input.utmCampaign ?? "").trim(),
        notes: (input.notes ?? "").trim(),
      }),
    });
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Network request failed.",
    };
  }
  if (res.status === 409) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return { kind: "conflict", message: body?.error ?? "That campaign name already exists." };
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return {
      kind: "unavailable",
      message: body?.error ?? `The server answered with status ${res.status}.`,
    };
  }
  const raw = await res.json().catch(() => null);
  const campaign = parseCampaign(raw);
  if (!campaign) {
    return { kind: "unavailable", message: "Server returned malformed campaign data." };
  }
  return { kind: "created", campaign };
}

export type LeadFormInput = {
  sourceKind: string;
  sourceDetail?: string;
  contactName?: string;
  contactEmail?: string;
  contactPhone?: string;
  message?: string;
  notes?: string;
  campaignId?: string;
  followUpAt?: string;
};

export async function createLead(
  baseUrl: string,
  workspaceId: string,
  input: LeadFormInput,
  fetchImpl: typeof fetch = fetch,
): Promise<CreateLeadResult> {
  const problems = validateLeadInput(input);
  if (problems.length > 0) {
    return { kind: "rejected", problems };
  }
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/leads`;
  let res: Response;
  try {
    res = await fetchImpl(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(cleanLeadPayload(input)),
    });
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Network request failed.",
    };
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return {
      kind: "unavailable",
      message: body?.error ?? `The server answered with status ${res.status}.`,
    };
  }
  const raw = await res.json().catch(() => null);
  const lead = parseLead(raw);
  if (!lead) {
    return { kind: "unavailable", message: "Server returned malformed lead data." };
  }
  return { kind: "created", lead };
}

export type LeadPatchInput = {
  status?: string;
  notes?: string;
  followUpAt?: string;
  clearFollowUp?: boolean;
  clearCampaign?: boolean;
};

export async function updateLead(
  baseUrl: string,
  workspaceId: string,
  leadId: string,
  input: LeadPatchInput,
  fetchImpl: typeof fetch = fetch,
): Promise<UpdateLeadResult> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/leads/${encodeURIComponent(leadId)}`;
  let res: Response;
  try {
    res = await fetchImpl(url, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(input),
    });
  } catch (err) {
    return {
      kind: "unavailable",
      message: err instanceof Error ? err.message : "Network request failed.",
    };
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return {
      kind: "unavailable",
      message: body?.error ?? `The server answered with status ${res.status}.`,
    };
  }
  const raw = await res.json().catch(() => null);
  const lead = parseLead(raw);
  if (!lead) {
    return { kind: "unavailable", message: "Server returned malformed lead data." };
  }
  return { kind: "saved", lead };
}

// cleanLeadPayload turns blank strings into omitted fields so the backend
// stores NULL, not empty values (data minimization).
function cleanLeadPayload(input: LeadFormInput): Record<string, unknown> {
  const payload: Record<string, unknown> = { sourceKind: input.sourceKind };
  const optional: [string, string | undefined][] = [
    ["sourceDetail", input.sourceDetail],
    ["contactName", input.contactName],
    ["contactEmail", input.contactEmail],
    ["contactPhone", input.contactPhone],
    ["message", input.message],
    ["notes", input.notes],
    ["campaignId", input.campaignId],
    ["followUpAt", input.followUpAt],
  ];
  for (const [key, value] of optional) {
    const trimmed = value?.trim();
    if (trimmed) {
      payload[key] =
        key === "followUpAt" && /^\d{4}-\d{2}-\d{2}$/.test(trimmed)
          ? `${trimmed}T00:00:00.000Z`
          : key === "followUpAt"
            ? value
            : trimmed;
    }
  }
  return payload;
}
