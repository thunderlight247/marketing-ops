export type IntegrationStatus = "unconfigured" | "connected" | "limited" | "error" | "disconnected";

export type CapabilityStatus = "supported" | "unsupported" | "unverified" | "failed";

export interface CapabilityReport {
  capability: string;
  status: CapabilityStatus;
  details?: string;
  verifiedAt?: string;
}

export interface AdapterDescriptor {
  kind: string;
  displayName: string;
  description: string;
}

export interface Integration {
  id: string;
  workspaceId: string;
  adapterKind: string;
  name: string;
  status: IntegrationStatus;
  config: Record<string, unknown>;
  hasCredentials: boolean;
  capabilities: CapabilityReport[];
  createdAt: string;
  updatedAt: string;
}

export type IntegrationsState =
  | { kind: "loading" }
  | { kind: "loaded"; integrations: Integration[] }
  | { kind: "unavailable"; message: string }
  | { kind: "missing"; message: string };

export type CreateIntegrationResult =
  | { kind: "created"; integration: Integration }
  | { kind: "conflict"; message: string }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export type SaveCredentialsResult =
  { kind: "saved"; integration: Integration } | { kind: "unavailable"; message: string };

export type CheckCapabilitiesResult =
  | {
      kind: "checked";
      lifecycleStatus: IntegrationStatus;
      capabilities: CapabilityReport[];
      diagnostic: string;
      checkedAt: string;
    }
  | { kind: "unavailable"; message: string };

export function formatCapabilityName(cap: string): string {
  switch (cap) {
    case "read_content":
      return "Read content";
    case "create_draft":
      return "Create draft";
    case "publish":
      return "Publish content";
    case "update_content":
      return "Update content";
    case "schedule":
      return "Schedule publishing";
    case "analytics":
      return "Website analytics";
    case "lead_intake":
      return "Lead intake";
    default:
      return cap;
  }
}

export function formatLifecycleStatus(status: IntegrationStatus): string {
  switch (status) {
    case "connected":
      return "Connected";
    case "limited":
      return "Limited";
    case "unconfigured":
      return "Unconfigured";
    case "error":
      return "Error";
    case "disconnected":
      return "Disconnected";
    default:
      return status;
  }
}

export function formatCapabilityStatus(status: CapabilityStatus): string {
  switch (status) {
    case "supported":
      return "Supported";
    case "unsupported":
      return "Unsupported";
    case "unverified":
      return "Unverified";
    case "failed":
      return "Failed";
    default:
      return status;
  }
}

export function validateIntegrationInput(input: { name?: string; adapterKind?: string }): string[] {
  const problems: string[] = [];
  const name = (input.name ?? "").trim();
  if (!name) {
    problems.push("name is required");
  } else if (Array.from(name).length > 120) {
    problems.push("name is too long (max 120 characters)");
  }

  const adapterKind = (input.adapterKind ?? "").trim();
  if (!adapterKind) {
    problems.push("adapterKind is required");
  }

  return problems;
}

function parseIntegration(raw: unknown): Integration | null {
  if (typeof raw !== "object" || raw === null) return null;
  const obj = raw as Record<string, unknown>;
  if (
    typeof obj.id !== "string" ||
    typeof obj.workspaceId !== "string" ||
    typeof obj.adapterKind !== "string" ||
    typeof obj.name !== "string" ||
    typeof obj.status !== "string" ||
    typeof obj.createdAt !== "string" ||
    typeof obj.updatedAt !== "string"
  ) {
    return null;
  }

  const caps: CapabilityReport[] = [];
  if (Array.isArray(obj.capabilities)) {
    for (const c of obj.capabilities) {
      if (typeof c === "object" && c !== null) {
        const item = c as Record<string, unknown>;
        if (typeof item.capability === "string" && typeof item.status === "string") {
          caps.push({
            capability: item.capability,
            status: item.status as CapabilityStatus,
            details: typeof item.details === "string" ? item.details : undefined,
            verifiedAt: typeof item.verifiedAt === "string" ? item.verifiedAt : undefined,
          });
        }
      }
    }
  }

  return {
    id: obj.id,
    workspaceId: obj.workspaceId,
    adapterKind: obj.adapterKind,
    name: obj.name,
    status: obj.status as IntegrationStatus,
    config:
      typeof obj.config === "object" && obj.config !== null
        ? (obj.config as Record<string, unknown>)
        : {},
    hasCredentials: Boolean(obj.hasCredentials),
    capabilities: caps,
    createdAt: obj.createdAt,
    updatedAt: obj.updatedAt,
  };
}

export async function fetchAdapters(
  baseUrl: string,
  fetchImpl: typeof fetch = fetch,
): Promise<AdapterDescriptor[]> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/integrations/adapters`;
  try {
    const res = await fetchImpl(url, { headers: { Accept: "application/json" } });
    if (!res.ok) return [];
    const data = (await res.json()) as { adapters?: unknown[] };
    if (!Array.isArray(data.adapters)) return [];
    const out: AdapterDescriptor[] = [];
    for (const item of data.adapters) {
      if (typeof item === "object" && item !== null) {
        const d = item as Record<string, unknown>;
        if (typeof d.kind === "string" && typeof d.displayName === "string") {
          out.push({
            kind: d.kind,
            displayName: d.displayName,
            description: typeof d.description === "string" ? d.description : "",
          });
        }
      }
    }
    return out;
  } catch {
    return [];
  }
}

export async function fetchWorkspaceIntegrations(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<IntegrationsState> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/integrations`;
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
    const data = (await res.json()) as { integrations?: unknown[] };
    if (!Array.isArray(data.integrations)) {
      return { kind: "unavailable", message: "Server returned malformed integration list." };
    }
    const list: Integration[] = [];
    for (const raw of data.integrations) {
      const parsed = parseIntegration(raw);
      if (parsed) list.push(parsed);
    }
    return { kind: "loaded", integrations: list };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export async function createWorkspaceIntegration(
  baseUrl: string,
  workspaceId: string,
  input: { name: string; adapterKind: string; config?: Record<string, unknown> },
  fetchImpl: typeof fetch = fetch,
): Promise<CreateIntegrationResult> {
  const problems = validateIntegrationInput(input);
  if (problems.length > 0) {
    return { kind: "rejected", problems };
  }

  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/integrations`;
  try {
    const res = await fetchImpl(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({
        name: input.name.trim(),
        adapterKind: input.adapterKind.trim(),
        config: input.config ?? {},
      }),
    });

    if (res.status === 409) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "conflict",
        message: body?.error ?? "An integration with that name already exists in this workspace.",
      };
    }

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `Failed to create integration (status ${res.status}).`,
      };
    }

    const data = await res.json();
    const created = parseIntegration(data);
    if (!created) {
      return { kind: "unavailable", message: "Server returned malformed created integration." };
    }
    return { kind: "created", integration: created };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export async function saveIntegrationCredentials(
  baseUrl: string,
  workspaceId: string,
  integrationId: string,
  credentials: Record<string, string>,
  fetchImpl: typeof fetch = fetch,
): Promise<SaveCredentialsResult> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/integrations/${encodeURIComponent(integrationId)}/credentials`;
  try {
    const res = await fetchImpl(url, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({ credentials }),
    });

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `Failed to save credentials (status ${res.status}).`,
      };
    }

    const data = await res.json();
    const updated = parseIntegration(data);
    if (!updated) {
      return { kind: "unavailable", message: "Server returned malformed integration." };
    }
    return { kind: "saved", integration: updated };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export async function checkIntegrationCapabilities(
  baseUrl: string,
  workspaceId: string,
  integrationId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<CheckCapabilitiesResult> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/integrations/${encodeURIComponent(integrationId)}/check`;
  try {
    const res = await fetchImpl(url, {
      method: "POST",
      headers: { Accept: "application/json" },
    });

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: body?.error ?? `Failed to check capabilities (status ${res.status}).`,
      };
    }

    const data = (await res.json()) as {
      lifecycleStatus: string;
      capabilities: unknown[];
      diagnostic: string;
      checkedAt: string;
    };

    const caps: CapabilityReport[] = [];
    if (Array.isArray(data.capabilities)) {
      for (const item of data.capabilities) {
        if (typeof item === "object" && item !== null) {
          const c = item as Record<string, unknown>;
          if (typeof c.capability === "string" && typeof c.status === "string") {
            caps.push({
              capability: c.capability,
              status: c.status as CapabilityStatus,
              details: typeof c.details === "string" ? c.details : undefined,
            });
          }
        }
      }
    }

    return {
      kind: "checked",
      lifecycleStatus: (data.lifecycleStatus as IntegrationStatus) || "unconfigured",
      capabilities: caps,
      diagnostic: data.diagnostic || "",
      checkedAt: data.checkedAt || new Date().toISOString(),
    };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { kind: "unavailable", message: `Could not reach the operations API (${msg}).` };
  }
}

export async function deleteIntegration(
  baseUrl: string,
  workspaceId: string,
  integrationId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<{ success: boolean; message?: string }> {
  const url = `${baseUrl.replace(/\/+$/, "")}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/integrations/${encodeURIComponent(integrationId)}`;
  try {
    const res = await fetchImpl(url, { method: "DELETE" });
    if (res.status === 204) return { success: true };
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return {
      success: false,
      message: body?.error ?? `Failed to delete integration (${res.status}).`,
    };
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Network error";
    return { success: false, message: `Could not reach the operations API (${msg}).` };
  }
}
