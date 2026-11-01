import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { middleware } from "./middleware";

afterEach(() => vi.unstubAllEnvs());

function configure() {
  vi.stubEnv("WEB_AUTH_DISABLED", "false");
  vi.stubEnv("OPERATOR_USERNAME", "operator");
  vi.stubEnv("OPERATOR_PASSWORD", "test-password");
  vi.stubEnv("WEB_ORIGIN", "https://dashboard.example");
}

const auth = `Basic ${btoa("operator:test-password")}`;

describe("operator access", () => {
  it("fails closed when credentials are missing", async () => {
    vi.stubEnv("WEB_AUTH_DISABLED", "false");
    vi.stubEnv("OPERATOR_USERNAME", "");
    expect((await middleware(new NextRequest("https://dashboard.example/"))).status).toBe(503);
  });

  it("rejects missing or incorrect credentials and accepts the operator", async () => {
    configure();
    expect((await middleware(new NextRequest("https://dashboard.example/"))).status).toBe(401);
    expect(
      (
        await middleware(
          new NextRequest("https://dashboard.example/", {
            headers: { authorization: "Basic invalid" },
          }),
        )
      ).status,
    ).toBe(401);
    expect(
      (
        await middleware(
          new NextRequest("https://dashboard.example/", { headers: { authorization: auth } }),
        )
      ).status,
    ).toBe(200);
  });

  it("rejects cross-origin and missing-origin writes", async () => {
    configure();
    for (const origin of [undefined, "https://evil.example"]) {
      const headers = new Headers({ authorization: auth });
      if (origin) headers.set("origin", origin);
      const request = new NextRequest("https://dashboard.example/api/v1/workspaces", {
        method: "POST",
        headers,
      });
      expect((await middleware(request)).status).toBe(403);
    }
    const request = new NextRequest("https://dashboard.example/api/v1/workspaces", {
      method: "POST",
      headers: { authorization: auth, origin: "https://dashboard.example" },
    });
    expect((await middleware(request)).status).toBe(200);
  });

  it("allows local use only when the bypass is explicitly enabled", async () => {
    vi.stubEnv("WEB_AUTH_DISABLED", "true");
    const request = new NextRequest("http://localhost:3001/api/v1/workspaces", {
      method: "POST",
      headers: { origin: "http://localhost:3001" },
    });
    expect((await middleware(request)).status).toBe(200);
  });
});
