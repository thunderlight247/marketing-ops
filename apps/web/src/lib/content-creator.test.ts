import { describe, expect, it, vi } from "vitest";
import { requestContentDraft } from "./agents";

const envelope = {
  item: { id: "item-1", title: "Winter care", kind: "snippet", status: "draft", slug: null },
  version: { versionNumber: 1, body: "Review maintenance.", excerpt: null, callToAction: null },
  provider: "stub",
  model: "stub-v1",
  agentId: "content_creator",
  keyPoints: ["Maintenance"],
  durationMs: 12,
  slugSuggestion: "winter-care",
};

describe("requestContentDraft", () => {
  it("posts a trimmed brief and accepts the server's item envelope", async () => {
    const transport = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response(JSON.stringify(envelope), { status: 201 }));
    const result = await requestContentDraft(
      "http://localhost:8081/",
      "workspace/one",
      "snippet",
      "  Winter care  ",
      transport,
    );
    expect(transport).toHaveBeenCalledWith(
      "http://localhost:8081/api/v1/workspaces/workspace%2Fone/agents/content_creator/run",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ kind: "snippet", focus: "Winter care" }),
      }),
    );
    expect(result).toEqual({ kind: "done", draft: envelope });
  });
  it("omits an empty focus", async () => {
    const transport = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response(JSON.stringify(envelope), { status: 201 }));
    await requestContentDraft("http://localhost", "ws", "snippet", "  ", transport);
    expect(JSON.parse(String(transport.mock.calls[0][1]?.body))).toEqual({ kind: "snippet" });
  });
  it("reports server errors", async () => {
    const transport = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify({ error: "specialist unavailable" }), { status: 502 }),
      );
    expect(await requestContentDraft("http://localhost", "ws", "snippet", "", transport)).toEqual({
      kind: "unavailable",
      message: "specialist unavailable",
    });
  });
  it("rejects malformed success payloads", async () => {
    const transport = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response(JSON.stringify({ id: "wrong-envelope" })));
    expect(
      (await requestContentDraft("http://localhost", "ws", "snippet", "", transport)).kind,
    ).toBe("unavailable");
  });
  it("handles network failure", async () => {
    const transport = vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"));
    expect(await requestContentDraft("http://localhost", "ws", "snippet", "", transport)).toEqual({
      kind: "unavailable",
      message: "the API could not be reached",
    });
  });
});
