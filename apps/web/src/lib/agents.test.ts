import { describe, expect, it, vi } from "vitest";
import { formatDataSource, formatRecommendationPriority, requestSeoAdvisory } from "./agents";
import { getApiBaseUrl } from "./config";

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
    expect(url).toContain("/api/v1/workspaces/ws-1/agents/seo/advisory");
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
