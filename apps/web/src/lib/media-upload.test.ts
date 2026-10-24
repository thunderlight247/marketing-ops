import { describe, expect, it, vi } from "vitest";
import { saveWorkspaceDraft, uploadIntegrationImage } from "./content";

function fakeFile(name: string, type: string, size: number): File {
  return new File(["x".repeat(size)], name, { type });
}

describe("uploadIntegrationImage", () => {
  it("rejects unsupported types and oversized files before any request", async () => {
    const send = vi.fn();
    await uploadIntegrationImage("http://x", "ws", "in", fakeFile("a.txt", "text/plain", 10), send);
    await uploadIntegrationImage(
      "http://x",
      "ws",
      "in",
      fakeFile("a.png", "image/png", 9 * 1024 * 1024),
      send,
    );
    expect(send).not.toHaveBeenCalled();
  });

  it("sends the file as multipart and returns the asset ref and url", async () => {
    let captured: RequestInit | undefined;
    const send = vi.fn(async (_url: string, init: RequestInit) => {
      captured = init;
      return new Response(
        JSON.stringify({ ref: "image-abc-800x600-jpg", url: "https://cdn.sanity.io/images/x.png" }),
        { status: 201 },
      );
    });
    const result = await uploadIntegrationImage(
      "http://x/",
      "ws-1",
      "in-1",
      fakeFile("logo.png", "image/png", 100),
      send as unknown as typeof fetch,
    );
    expect(result).toEqual({
      kind: "uploaded",
      ref: "image-abc-800x600-jpg",
      url: "https://cdn.sanity.io/images/x.png",
    });
    const url = (send.mock.calls[0]?.[0] as string) ?? "";
    expect(url).toBe("http://x/api/v1/workspaces/ws-1/integrations/in-1/media");
    expect(captured?.body instanceof FormData).toBe(true);
  });

  it("maps the server error body to an unavailable message", async () => {
    const send = vi.fn(
      async () => new Response(JSON.stringify({ error: "no token saved" }), { status: 409 }),
    );
    const result = await uploadIntegrationImage(
      "http://x",
      "ws",
      "in",
      fakeFile("a.png", "image/png", 10),
      send as unknown as typeof fetch,
    );
    expect(result).toEqual({ kind: "unavailable", message: "no token saved" });
  });
});

describe("saveWorkspaceDraft metadata", () => {
  it("sends metadata when provided", async () => {
    let body: string | undefined;
    const send = vi.fn(async (_url: string, init: RequestInit) => {
      body = String(init.body);
      return new Response(
        JSON.stringify({
          id: "i-1",
          workspaceId: "w-1",
          kind: "blog_article",
          status: "draft",
          title: "t",
          slug: null,
          plannedPublishAt: null,
          createdAt: "2026-01-01T00:00:00Z",
          updatedAt: "2026-01-01T00:00:00Z",
          version: {
            id: "v-2",
            contentItemId: "i-1",
            versionNumber: 2,
            title: "t",
            excerpt: null,
            body: "",
            createdAt: "2026-01-01T00:00:00Z",
          },
        }),
        { status: 200 },
      );
    });
    const result = await saveWorkspaceDraft(
      "http://x",
      "w-1",
      "i-1",
      { title: "t", metadata: { mainImage: { ref: "r" } } },
      send as unknown as typeof fetch,
    );
    expect(result.kind).toBe("saved");
    const parsed = JSON.parse(body ?? "{}") as Record<string, unknown>;
    expect(parsed.metadata).toEqual({ mainImage: { ref: "r" } });
  });

  it("rejects oversized metadata without a request", async () => {
    const send = vi.fn();
    const result = await saveWorkspaceDraft(
      "http://x",
      "w-1",
      "i-1",
      { title: "t", metadata: { blob: "x".repeat(20000) } },
      send as unknown as typeof fetch,
    );
    expect(result.kind).toBe("rejected");
    expect(send).not.toHaveBeenCalled();
  });
});
