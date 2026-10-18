import { describe, expect, it, vi } from "vitest";
import {
  checkIntegrationCapabilities,
  createWorkspaceIntegration,
  deleteIntegration,
  fetchAdapters,
  fetchWorkspaceIntegrations,
  formatCapabilityName,
  formatCapabilityStatus,
  formatLifecycleStatus,
  saveIntegrationCredentials,
  validateIntegrationInput,
} from "./integrations";

describe("validateIntegrationInput", () => {
  it("rejects empty name and adapterKind", () => {
    const problems = validateIntegrationInput({});
    expect(problems).toContain("name is required");
    expect(problems).toContain("adapterKind is required");
  });

  it("rejects name longer than 120 characters", () => {
    const problems = validateIntegrationInput({
      name: "a".repeat(121),
      adapterKind: "sanity",
    });
    expect(problems).toContain("name is too long (max 120 characters)");
  });

  it("accepts valid input", () => {
    const problems = validateIntegrationInput({
      name: "Nutrition Guide Blog",
      adapterKind: "sanity",
    });
    expect(problems).toEqual([]);
  });
});

describe("format helpers", () => {
  it("formats capability names", () => {
    expect(formatCapabilityName("read_content")).toBe("Read content");
    expect(formatCapabilityName("create_draft")).toBe("Create draft");
    expect(formatCapabilityName("publish")).toBe("Publish content");
    expect(formatCapabilityName("schedule")).toBe("Schedule publishing");
  });

  it("formats lifecycle statuses", () => {
    expect(formatLifecycleStatus("connected")).toBe("Connected");
    expect(formatLifecycleStatus("limited")).toBe("Limited");
    expect(formatLifecycleStatus("unconfigured")).toBe("Unconfigured");
    expect(formatLifecycleStatus("error")).toBe("Error");
  });

  it("formats capability statuses", () => {
    expect(formatCapabilityStatus("supported")).toBe("Supported");
    expect(formatCapabilityStatus("unsupported")).toBe("Unsupported");
    expect(formatCapabilityStatus("unverified")).toBe("Unverified");
    expect(formatCapabilityStatus("failed")).toBe("Failed");
  });
});

describe("fetchAdapters", () => {
  it("returns descriptors on success", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        adapters: [{ kind: "sanity", displayName: "Sanity CMS", description: "Sanity pilot" }],
      }),
    });
    const adapters = await fetchAdapters("http://api.local", mockFetch as unknown as typeof fetch);
    expect(adapters.length).toBe(1);
    expect(adapters[0].kind).toBe("sanity");
  });

  it("returns empty array on error", async () => {
    const mockFetch = vi.fn().mockRejectedValue(new Error("network error"));
    const adapters = await fetchAdapters("http://api.local", mockFetch as unknown as typeof fetch);
    expect(adapters).toEqual([]);
  });
});

describe("fetchWorkspaceIntegrations", () => {
  const wsId = "ws-123";

  it("handles 404 missing workspace", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ status: 404, ok: false });
    const res = await fetchWorkspaceIntegrations(
      "http://api.local",
      wsId,
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("missing");
  });

  it("parses loaded integrations and capabilities", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        integrations: [
          {
            id: "in-1",
            workspaceId: wsId,
            adapterKind: "sanity",
            name: "Nutrition Guide",
            status: "connected",
            config: { projectId: "y97t5m4h" },
            hasCredentials: true,
            capabilities: [{ capability: "read_content", status: "supported", details: "Read ok" }],
            createdAt: "2026-03-01T00:00:00Z",
            updatedAt: "2026-03-01T00:00:00Z",
          },
        ],
      }),
    });
    const res = await fetchWorkspaceIntegrations(
      "http://api.local",
      wsId,
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("loaded");
    if (res.kind === "loaded") {
      expect(res.integrations.length).toBe(1);
      expect(res.integrations[0].hasCredentials).toBe(true);
      expect(res.integrations[0].capabilities.length).toBe(1);
    }
  });
});

describe("createWorkspaceIntegration", () => {
  const wsId = "ws-123";

  it("rejects invalid input without network call", async () => {
    const mockFetch = vi.fn();
    const res = await createWorkspaceIntegration(
      "http://api.local",
      wsId,
      {
        name: "",
        adapterKind: "sanity",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("rejected");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("handles 409 conflict", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({ error: "name taken" }),
    });
    const res = await createWorkspaceIntegration(
      "http://api.local",
      wsId,
      {
        name: "Existing Name",
        adapterKind: "sanity",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("conflict");
  });

  it("handles 201 created", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 201,
      ok: true,
      json: async () => ({
        id: "in-new",
        workspaceId: wsId,
        adapterKind: "sanity",
        name: "New Integration",
        status: "unconfigured",
        config: {},
        hasCredentials: false,
        capabilities: [],
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-03-01T00:00:00Z",
      }),
    });
    const res = await createWorkspaceIntegration(
      "http://api.local",
      wsId,
      {
        name: "New Integration",
        adapterKind: "sanity",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("created");
  });
});

describe("saveIntegrationCredentials and checkCapabilities", () => {
  const wsId = "ws-123";
  const inId = "in-456";

  it("saves credentials and returns updated integration", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: inId,
        workspaceId: wsId,
        adapterKind: "sanity",
        name: "Sanity",
        status: "connected",
        config: {},
        hasCredentials: true,
        capabilities: [],
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-03-01T00:00:00Z",
      }),
    });
    const res = await saveIntegrationCredentials(
      "http://api.local",
      wsId,
      inId,
      { token: "secret" },
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("saved");
    if (res.kind === "saved") {
      expect(res.integration.hasCredentials).toBe(true);
    }
  });

  it("checks capabilities and returns check results", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        lifecycleStatus: "connected",
        diagnostic: "All checks passed",
        checkedAt: "2026-03-01T00:00:00Z",
        capabilities: [{ capability: "read_content", status: "supported", details: "Ready" }],
      }),
    });
    const res = await checkIntegrationCapabilities(
      "http://api.local",
      wsId,
      inId,
      mockFetch as unknown as typeof fetch,
    );
    expect(res.kind).toBe("checked");
    if (res.kind === "checked") {
      expect(res.lifecycleStatus).toBe("connected");
      expect(res.capabilities.length).toBe(1);
    }
  });

  it("deletes integration successfully", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ status: 204, ok: true });
    const res = await deleteIntegration(
      "http://api.local",
      wsId,
      inId,
      mockFetch as unknown as typeof fetch,
    );
    expect(res.success).toBe(true);
  });
});
