import { describe, expect, it, vi } from "vitest";
import {
  fetchAgentRegistry,
  formatAgentAction,
  formatAgentContract,
  formatAgentDataNeed,
  formatDataSource,
  formatRecommendationPriority,
  requestCampaignProposal,
  requestSeoAdvisory,
} from "./agents";
import { getApiBaseUrl } from "./config";

const descriptor = {
  id: "seo_specialist",
  version: "1",
  displayName: "SEO advisor",
  description: "Advisory-only organic search review.",
  outputContract: "advisory",
  dataNeeds: ["search_console"],
  actions: [],
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const validReport = {
  agentId: "seo_specialist",
  agentVersion: "1",
  provider: "stub",
  model: "stub-v1",
  dataSource: "none",
  generatedAt: "2026-10-22T00:00:00Z",
  cannibalizationChecked: false,
  summary: "No data connected yet.",
  gaps: ["insufficient connected GSC data"],
  recommendations: [
    {
      priority: "high",
      area: "metadata",
      page: "",
      issue: "Missing meta descriptions",
      evidence: "based on the provided workspace context only",
      action: "Add meta descriptions.",
    },
  ],
};

describe("requestSeoAdvisory", () => {
  const base = getApiBaseUrl("http://api.example");

  it("posts an empty body when no focus is given and returns a done state", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ report: validReport, dataNote: "n", durationMs: 12 }));
    const state = await requestSeoAdvisory(base, "ws-1", "   ", fetchImpl);
    expect(state.kind).toBe("done");
    const [url, init] = fetchImpl.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/workspaces/ws-1/agents/seo_specialist/run");
    expect(init.method).toBe("POST");
    expect(init.body).toBe("{}");
  });

  it("trims and forwards a provided focus", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ report: validReport }));
    await requestSeoAdvisory(base, "ws-1", "  local SEO  ", fetchImpl);
    const [, init] = fetchImpl.mock.calls[0] as [string, RequestInit];
    expect(init.body).toBe(JSON.stringify({ focus: "local SEO" }));
  });

  it("surfaces a server error message as unavailable", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ error: "agents are not configured" }, 501));
    const state = await requestSeoAdvisory(base, "ws-1", "", fetchImpl);
    expect(state).toEqual({
      kind: "unavailable",
      message: "agents are not configured",
    });
  });

  it("rejects a wrong report shape as unavailable", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ report: { agentId: "seo_specialist" } }));
    const state = await requestSeoAdvisory(base, "ws-1", "", fetchImpl);
    expect(state).toEqual({
      kind: "unavailable",
      message: "the advisory response was not a valid report",
    });
  });

  it("maps network failures to an unavailable state", async () => {
    const fetchImpl = vi.fn().mockRejectedValue(new Error("offline"));
    const state = await requestSeoAdvisory(base, "ws-1", "", fetchImpl);
    expect(state).toEqual({ kind: "unavailable", message: "the API could not be reached" });
  });
});

describe("format helpers", () => {
  it("formats priorities and data sources", () => {
    expect(formatRecommendationPriority("high")).toBe("High");
    expect(formatRecommendationPriority("weird")).toBe("weird");
    expect(formatDataSource("search_console")).toContain("Search Console");
    expect(formatDataSource("none")).toContain("No connected");
  });
});

describe("fetchAgentRegistry", () => {
  it("lists parsed agent descriptors", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ agents: [descriptor] }));
    const state = await fetchAgentRegistry("http://api.example", fetchImpl);
    expect(state).toEqual({ kind: "available", agents: [descriptor] });
    const [url, init] = fetchImpl.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://api.example/api/v1/agents");
    expect(init.headers).toBeDefined();
  });

  it("rejects a malformed descriptor entry as unavailable", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ agents: [{ id: "x" }] }));
    const state = await fetchAgentRegistry("http://api.example", fetchImpl);
    expect(state).toEqual({
      kind: "unavailable",
      message: "the agents response had an unexpected shape",
    });
  });

  it("treats a non-array agents field as unavailable", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse({ agents: {} }));
    expect(await fetchAgentRegistry("http://api.example", fetchImpl).then((s) => s.kind)).toBe(
      "unavailable",
    );
  });

  it("surfaces fixed server errors and network failures", async () => {
    const serverError = vi
      .fn()
      .mockResolvedValue(jsonResponse({ error: "registry unavailable" }, 503));
    expect(await fetchAgentRegistry("http://api", serverError)).toEqual({
      kind: "unavailable",
      message: "registry unavailable",
    });
    const offline = vi.fn().mockRejectedValue(new Error("down"));
    expect(await fetchAgentRegistry("http://api", offline)).toEqual({
      kind: "unavailable",
      message: "the API could not be reached",
    });
  });
});

describe("formatAgent helpers", () => {
  it("formats contracts, data needs, and actions with safe fallbacks", () => {
    expect(formatAgentContract("advisory")).toContain("Advisory");
    expect(formatAgentContract("drafting")).toContain("approval");
    expect(formatAgentContract("proposal")).toContain("campaign");
    expect(formatAgentContract("future_contract")).toBe("future_contract");
    expect(formatAgentDataNeed("search_console")).toContain("Search Console");
    expect(formatAgentDataNeed("campaigns")).toBe("campaigns");
    expect(formatAgentAction("file_draft")).toContain("file drafts");
    expect(formatAgentAction("create_campaign")).toContain("planned");
    expect(formatAgentAction("future_action")).toBe("future_action");
  });
});

describe("requestCampaignProposal", () => {
  const base = getApiBaseUrl("http://api.example");

  const validProposal = {
    campaign: {
      id: "c-1",
      name: "Winter service reminder",
      status: "planned",
      utmCampaign: "winter-service-2026",
      notes: "Reminder angle.",
    },
    provider: "stub",
    model: "stub-v1",
    agentId: "campaign_strategist",
    keyPoints: ["Reminder angle"],
    durationMs: 9,
  };

  it("posts to the strategist's generic run route and returns a done state", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse(validProposal, 201));
    const state = await requestCampaignProposal(base, "ws-1", "  slow season  ", fetchImpl);
    expect(state).toEqual({ kind: "done", proposal: validProposal });
    const [url, init] = fetchImpl.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/workspaces/ws-1/agents/campaign_strategist/run");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ focus: "slow season" }));
  });

  it("posts an empty body when no focus is given", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(jsonResponse(validProposal, 201));
    await requestCampaignProposal(base, "ws-1", "   ", fetchImpl);
    const [, init] = fetchImpl.mock.calls[0] as [string, RequestInit];
    expect(init.body).toBe("{}");
  });

  it("surfaces the duplicate-campaign conflict message", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ error: "a campaign with that name already exists in this workspace" }, 409),
      );
    const state = await requestCampaignProposal(base, "ws-1", "", fetchImpl);
    expect(state.kind).toBe("unavailable");
    expect(state.kind === "unavailable" && state.message).toContain("already exists");
  });

  it("rejects a response without a real campaign as unavailable", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(jsonResponse({ campaign: { id: "", name: "x" }, keyPoints: [] }));
    const state = await requestCampaignProposal(base, "ws-1", "", fetchImpl);
    expect(state.kind).toBe("unavailable");
  });

  it("maps network failures to an unavailable state", async () => {
    const fetchImpl = vi.fn().mockRejectedValue(new Error("offline"));
    const state = await requestCampaignProposal(base, "ws-1", "", fetchImpl);
    expect(state).toEqual({ kind: "unavailable", message: "the API could not be reached" });
  });
});
