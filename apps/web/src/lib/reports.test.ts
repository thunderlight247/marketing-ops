import { afterEach, describe, expect, it, vi } from "vitest";
import {
  fetchWorkspaceReport,
  formatLeadSourceLabel,
  formatStatusLabel,
  validateReportQuery,
} from "@/lib/reports";

const BASE = "http://localhost:3000";
const WS = "11111111-1111-1111-1111-111111111111";

const validReport = {
  workspaceId: WS,
  generatedAt: "2026-10-24T12:00:00Z",
  range: { since: null, until: null },
  contentStatusNow: { draft: 2, published: 1 },
  campaignsNow: { active: 1 },
  leadsNow: { new: 3, won: 1 },
  leadSourcesNow: { manual: 4 },
  publishingRange: {
    jobsByStatus: { succeeded: 2, failed: 1 },
    publishedCount: 2,
    failedCount: 1,
    recentFailures: [
      {
        jobId: "job-1",
        itemId: "it-1",
        integrationId: "in-1",
        attempts: 2,
        lastError: "the destination refused the request",
        updatedAt: "2026-10-20T08:00:00Z",
      },
    ],
  },
};

function jsonResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

describe("report labels", () => {
  it("labels known statuses and keeps unknown raw values visible", () => {
    expect(formatStatusLabel("draft")).toBe("Draft");
    expect(formatStatusLabel("succeeded")).toBe("Succeeded");
    expect(formatStatusLabel("won")).toBe("Won");
    expect(formatStatusLabel("some_future_status")).toBe("some_future_status");
    expect(formatLeadSourceLabel("form_api")).toBe("Form / API");
    expect(formatLeadSourceLabel("mystery")).toBe("mystery");
  });
});

describe("validateReportQuery", () => {
  it("accepts valid or empty ranges", () => {
    expect(validateReportQuery({})).toEqual([]);
    expect(validateReportQuery({ since: "2026-10-01" })).toEqual([]);
    expect(validateReportQuery({ since: "2026-10-01", until: "2026-10-31" })).toEqual([]);
  });

  it("rejects invalid dates and inverted ranges", () => {
    expect(validateReportQuery({ since: "not a date" })).toContain("from date is not a valid date");
    expect(validateReportQuery({ until: "nope" })).toContain("to date is not a valid date");
    const problems = validateReportQuery({ since: "2026-11-01", until: "2026-10-01" });
    expect(problems).toContain("to date must be after the from date");
  });
});

describe("fetchWorkspaceReport", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns a loaded report for a valid response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, validReport));
    const state = await fetchWorkspaceReport(BASE, WS, {}, fetchMock);
    expect(state.kind).toBe("loaded");
    if (state.kind === "loaded") {
      expect(state.report.workspaceId).toBe(WS);
      expect(state.report.contentStatusNow.draft).toBe(2);
      expect(state.report.publishingRange.publishedCount).toBe(2);
      expect(state.report.publishingRange.recentFailures).toHaveLength(1);
      expect(state.report.publishingRange.recentFailures[0].lastError).toContain("refused");
    }
    expect(fetchMock).toHaveBeenCalledWith(
      `${BASE}/api/v1/workspaces/${WS}/reports`,
      expect.objectContaining({ headers: { Accept: "application/json" } }),
    );
  });

  it("sends the range as query params", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, validReport));
    await fetchWorkspaceReport(
      BASE,
      WS,
      { since: "2026-10-01T00:00:00Z", until: "2026-10-31T00:00:00Z" },
      fetchMock,
    );
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toContain("since=2026-10-01T00%3A00%3A00Z");
    expect(url).toContain("until=2026-10-31T00%3A00%3A00Z");
  });

  it("maps 404 to missing and other errors to unavailable", async () => {
    const missing = await fetchWorkspaceReport(
      BASE,
      WS,
      {},
      vi.fn().mockResolvedValue(jsonResponse(404, { error: "x" })),
    );
    expect(missing.kind).toBe("missing");

    const refused = await fetchWorkspaceReport(
      BASE,
      WS,
      {},
      vi.fn().mockResolvedValue(jsonResponse(500, { error: "boom" })),
    );
    expect(refused.kind).toBe("unavailable");
    if (refused.kind === "unavailable") {
      expect(refused.message).toBe("boom");
    }
  });

  it("rejects malformed reports instead of trusting them", async () => {
    const bad = {
      ...validReport,
      publishingRange: { ...validReport.publishingRange, publishedCount: "two" },
    };
    const state = await fetchWorkspaceReport(
      BASE,
      WS,
      {},
      vi.fn().mockResolvedValue(jsonResponse(200, bad)),
    );
    expect(state.kind).toBe("unavailable");
    if (state.kind === "unavailable") {
      expect(state.message).toContain("malformed");
    }
  });

  it("does not fetch at all for an already-invalid range", async () => {
    const fetchMock = vi.fn();
    const state = await fetchWorkspaceReport(
      BASE,
      WS,
      { since: "garbage" },
      fetchMock as unknown as typeof fetch,
    );
    expect(state.kind).toBe("unavailable");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("survives network failures as unavailable", async () => {
    const state = await fetchWorkspaceReport(
      BASE,
      WS,
      {},
      vi.fn().mockRejectedValue(new Error("offline")),
    );
    expect(state.kind).toBe("unavailable");
    if (state.kind === "unavailable") {
      expect(state.message).toBe("offline");
    }
  });
});
