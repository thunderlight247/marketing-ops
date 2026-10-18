import { describe, expect, it } from "vitest";
import { describeDatabase, fetchHealth, parseHealth } from "./health";

const validReport = {
  status: "ok",
  service: "marketing-ops-api",
  version: "dev",
  environment: "development",
  checkedAt: "2026-01-02T03:04:05Z",
  database: { status: "ok", latencyMs: 3, message: "The database responded to a ping." },
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("parseHealth", () => {
  it("accepts a valid report", () => {
    expect(parseHealth(validReport)?.database.latencyMs).toBe(3);
  });

  it("accepts a report without latency", () => {
    const report = { ...validReport, database: { status: "unconfigured", message: "not set" } };
    expect(parseHealth(report)?.database.latencyMs).toBeUndefined();
  });

  it.each([
    ["null", null],
    ["a string", "ok"],
    ["an array", []],
    ["a missing database", { ...validReport, database: undefined }],
    ["an unknown api status", { ...validReport, status: "fine" }],
    ["an unknown database status", { ...validReport, database: { status: "weird", message: "x" } }],
    ["a non-string version", { ...validReport, version: 1 }],
    [
      "a non-numeric latency",
      { ...validReport, database: { ...validReport.database, latencyMs: "3" } },
    ],
  ])("rejects %s", (_name: string, value: unknown) => {
    expect(parseHealth(value)).toBeNull();
  });
});

describe("fetchHealth", () => {
  it("returns the report on success and calls the health path", async () => {
    let requested = "";
    const fakeFetch = (async (input: RequestInfo | URL) => {
      requested = String(input);
      return jsonResponse(validReport);
    }) as typeof fetch;

    const state = await fetchHealth("http://api.test", fakeFetch);
    expect(requested).toBe("http://api.test/api/v1/health");
    expect(state.kind).toBe("loaded");
  });

  it("reports unavailable for a non-200 response", async () => {
    const fakeFetch = (async () => jsonResponse({}, 500)) as typeof fetch;
    const state = await fetchHealth("http://api.test", fakeFetch);
    expect(state).toEqual({ kind: "unavailable", reason: "The API answered with HTTP 500." });
  });

  it("reports unavailable for a network error", async () => {
    const fakeFetch = (async () => {
      throw new TypeError("fetch failed");
    }) as typeof fetch;
    const state = await fetchHealth("http://api.test", fakeFetch);
    expect(state.kind).toBe("unavailable");
  });

  it("reports unavailable for a malformed body", async () => {
    const fakeFetch = (async () => jsonResponse({ hello: "world" })) as typeof fetch;
    const state = await fetchHealth("http://api.test", fakeFetch);
    expect(state.kind).toBe("unavailable");
  });

  it("reports unavailable when the body is not JSON", async () => {
    const fakeFetch = (async () =>
      new Response("<html>oops</html>", { status: 200 })) as typeof fetch;
    const state = await fetchHealth("http://api.test", fakeFetch);
    expect(state.kind).toBe("unavailable");
  });

  it("aborts a request that exceeds the timeout", async () => {
    const hangingFetch = ((_input: RequestInfo | URL, init?: RequestInit) =>
      new Promise((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
      })) as typeof fetch;

    const state = await fetchHealth("http://api.test", hangingFetch, 20);
    expect(state.kind).toBe("unavailable");
  });
});

describe("describeDatabase", () => {
  it("never presents a missing database as healthy", () => {
    expect(describeDatabase("ok").tone).toBe("good");
    expect(describeDatabase("unconfigured").tone).toBe("warn");
    expect(describeDatabase("unavailable").tone).toBe("bad");
  });
});
