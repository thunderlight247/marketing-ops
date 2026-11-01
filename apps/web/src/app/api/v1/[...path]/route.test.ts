import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { GET, POST } from "./route";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("API proxy", () => {
  it("fails closed when the API key is missing", async () => {
    vi.stubEnv("API_BASE_URL", "http://api.example");
    vi.stubEnv("API_KEY", "");
    vi.stubEnv("WEB_AUTH_DISABLED", "false");
    expect((await GET(new NextRequest("http://localhost/api/v1/workspaces"))).status).toBe(503);
  });

  it("forwards the request with server-side credentials, not browser credentials", async () => {
    vi.stubEnv("API_BASE_URL", "http://api.example");
    vi.stubEnv("API_KEY", "server-secret");
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response('{"ok":true}', {
          status: 201,
          headers: { "content-type": "application/json", "set-cookie": "secret=value" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);
    const request = new NextRequest("https://dashboard.example/api/v1/workspaces?x=1", {
      method: "POST",
      headers: { authorization: "Basic browser-secret", "content-type": "application/json" },
      body: '{"name":"test"}',
    });
    const result = await POST(request);
    expect(result.status).toBe(201);
    expect(result.headers.get("set-cookie")).toBeNull();
    const [target, options] = fetchMock.mock.calls[0];
    expect(target.toString()).toBe("http://api.example/api/v1/workspaces?x=1");
    expect(options.headers.get("authorization")).toBe("Bearer server-secret");
    expect(new TextDecoder().decode(options.body)).toBe('{"name":"test"}');
    expect(options.redirect).toBe("manual");
  });

  it("handles unreachable API without exposing errors", async () => {
    vi.stubEnv("API_BASE_URL", "http://api.example");
    vi.stubEnv("API_KEY", "server-secret");
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("secret")));
    const result = await GET(new NextRequest("http://localhost/api/v1/workspaces"));
    expect(result.status).toBe(502);
    expect(await result.text()).not.toContain("secret");
  });
});
