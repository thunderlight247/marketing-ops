import { describe, expect, it, vi } from "vitest";
import {
  createWorkspaceContent,
  decideContentApproval,
  fetchContentHistory,
  fetchWorkspaceContent,
  generateWorkspaceContent,
  fetchWorkspaceDraft,
  formatContentKind,
  formatContentStatus,
  saveWorkspaceDraft,
  pushToSanity,
  transitionContentStatus,
  validateContentInput,
  validateDraftInput,
} from "./content";

describe("validateContentInput", () => {
  it("rejects empty kind and title", () => {
    const problems = validateContentInput({});
    expect(problems).toContain("kind is required");
    expect(problems).toContain("title is required");
  });

  it("rejects invalid kind", () => {
    const problems = validateContentInput({ kind: "invalid", title: "Valid title" });
    expect(problems).toContain("kind must be blog_article, social_post, or snippet");
  });

  it("rejects title longer than 200 characters", () => {
    const problems = validateContentInput({
      kind: "blog_article",
      title: "a".repeat(201),
    });
    expect(problems).toContain("title is too long (max 200 characters)");
  });

  it("rejects invalid slug format", () => {
    const problems = validateContentInput({
      kind: "blog_article",
      title: "Valid title",
      slug: "Invalid Slug!",
    });
    expect(problems).toContain("slug must be lowercase letters, digits, or hyphens");
  });

  it("accepts valid input without slug", () => {
    const problems = validateContentInput({
      kind: "blog_article",
      title: "10 Nutrition Tips",
    });
    expect(problems).toEqual([]);
  });

  it("accepts valid input with clean slug", () => {
    const problems = validateContentInput({
      kind: "social_post",
      title: "Quick Morning Tip",
      slug: "morning-tip",
    });
    expect(problems).toEqual([]);
  });
});

describe("format helpers", () => {
  it("formats content kinds properly", () => {
    expect(formatContentKind("blog_article")).toBe("Blog article");
    expect(formatContentKind("social_post")).toBe("Social post");
    expect(formatContentKind("snippet")).toBe("Snippet");
  });

  it("formats content statuses properly", () => {
    expect(formatContentStatus("draft")).toBe("Draft");
    expect(formatContentStatus("review")).toBe("In review");
    expect(formatContentStatus("approved")).toBe("Approved");
    expect(formatContentStatus("published")).toBe("Published");
  });
});

describe("fetchWorkspaceContent", () => {
  const wsId = "ws-123";

  it("returns missing when workspace 404s", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 404,
      ok: false,
    });
    const result = await fetchWorkspaceContent(
      "http://api.local",
      wsId,
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("missing");
  });

  it("returns unavailable when server fails", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 500,
      ok: false,
      json: async () => ({ error: "internal error" }),
    });
    const result = await fetchWorkspaceContent(
      "http://api.local",
      wsId,
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("unavailable");
    if (result.kind === "unavailable") {
      expect(result.message).toBe("internal error");
    }
  });

  it("returns loaded items on success", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        items: [
          {
            id: "ci-1",
            workspaceId: wsId,
            kind: "blog_article",
            status: "draft",
            title: "Article 1",
            slug: "article-1",
            createdAt: "2026-03-01T00:00:00Z",
            updatedAt: "2026-03-01T00:00:00Z",
          },
        ],
      }),
    });
    const result = await fetchWorkspaceContent(
      "http://api.local",
      wsId,
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("loaded");
    if (result.kind === "loaded") {
      expect(result.items.length).toBe(1);
      expect(result.items[0].title).toBe("Article 1");
    }
  });
});

describe("createWorkspaceContent", () => {
  const wsId = "ws-123";

  it("returns rejected for invalid input without network call", async () => {
    const mockFetch = vi.fn();
    const result = await createWorkspaceContent(
      "http://api.local",
      wsId,
      {
        kind: "blog_article",
        title: "",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("rejected");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns conflict on 409", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({ error: "slug taken" }),
    });
    const result = await createWorkspaceContent(
      "http://api.local",
      wsId,
      {
        kind: "blog_article",
        title: "Duplicate Article",
        slug: "duplicate",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("conflict");
  });

  it("returns created on 201", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 201,
      ok: true,
      json: async () => ({
        id: "ci-new",
        workspaceId: wsId,
        kind: "social_post",
        status: "draft",
        title: "New Post",
        slug: null,
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-03-01T00:00:00Z",
      }),
    });
    const result = await createWorkspaceContent(
      "http://api.local",
      wsId,
      {
        kind: "social_post",
        title: "New Post",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("created");
    if (result.kind === "created") {
      expect(result.item.title).toBe("New Post");
      expect(result.item.status).toBe("draft");
    }
  });
});

describe("validateDraftInput", () => {
  it("rejects a blank title", () => {
    expect(validateDraftInput({})).toContain("title cannot be blank");
  });

  it("rejects an invalid slug and publish timestamp", () => {
    const problems = validateDraftInput({
      title: "T",
      slug: "Nope!",
      plannedPublishAt: "yesterday",
    });
    expect(problems).toContain("slug must be lowercase letters, digits, or hyphens");
    expect(problems).toContain("plannedPublishAt must be a UTC ISO 8601 timestamp");
  });

  it("accepts a valid full form", () => {
    expect(
      validateDraftInput({
        title: "Recipe prep",
        slug: "recipe-prep",
        plannedPublishAt: "2026-06-01T09:00:00Z",
        excerpt: "Short line",
        body: "Long body",
        callToAction: "Order now",
        channelCopy: { instagram: "Hi!" },
      }),
    ).toEqual([]);
  });
});

describe("fetchWorkspaceDraft", () => {
  it("maps a 404 to missing", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ status: 404, ok: false });
    const result = await fetchWorkspaceDraft(
      "http://api.local/",
      "ws",
      "it",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("missing");
  });

  it("parses item with null version before the first save", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: "it",
        workspaceId: "ws",
        kind: "blog_article",
        status: "draft",
        title: "Fresh",
        slug: null,
        plannedPublishAt: null,
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-03-01T00:00:00Z",
        version: null,
      }),
    });
    const result = await fetchWorkspaceDraft(
      "http://api.local",
      "ws",
      "it",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("loaded");
    if (result.kind === "loaded") {
      expect(result.version).toBeNull();
      expect(result.item.plannedPublishAt).toBeNull();
    }
  });

  it("parses the newest version with channel copy", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: "it",
        workspaceId: "ws",
        kind: "social_post",
        status: "draft",
        title: "Edited",
        slug: "edited",
        plannedPublishAt: "2026-06-01T09:00:00Z",
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-04-01T00:00:00Z",
        version: {
          id: "v2",
          contentItemId: "it",
          versionNumber: 2,
          title: "Edited",
          body: "Body two",
          excerpt: null,
          callToAction: "Shop now",
          channelCopy: { facebook: "Post text" },
          createdAt: "2026-04-01T00:00:00Z",
        },
      }),
    });
    const result = await fetchWorkspaceDraft(
      "http://api.local",
      "ws",
      "it",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("loaded");
    if (result.kind === "loaded" && result.version) {
      expect(result.version.versionNumber).toBe(2);
      expect(result.version.channelCopy?.facebook).toBe("Post text");
    }
  });
});

describe("saveWorkspaceDraft", () => {
  it("rejects client-side validation without calling the API", async () => {
    const mockFetch = vi.fn();
    const result = await saveWorkspaceDraft(
      "http://api.local",
      "ws",
      "it",
      { title: "  " },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("rejected");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("sends the editable fields and maps the new version", async () => {
    const mockFetch = vi.fn().mockImplementation(async (_url: string, init?: RequestInit) => {
      const payload = JSON.parse(String(init?.body)) as Record<string, unknown>;
      expect(payload.title).toBe("Renamed draft");
      expect(payload.slug).toBe("renamed-draft");
      expect(payload.body).toBe("Body one");
      expect(payload.plannedPublishAt).toBe("2026-06-01T09:00:00Z");
      return {
        status: 200,
        ok: true,
        json: async () => ({
          id: "it",
          workspaceId: "ws",
          kind: "blog_article",
          status: "draft",
          title: "Renamed draft",
          slug: "renamed-draft",
          plannedPublishAt: "2026-06-01T09:00:00Z",
          createdAt: "2026-03-01T00:00:00Z",
          updatedAt: "2026-04-01T00:00:00Z",
          version: {
            id: "v3",
            contentItemId: "it",
            versionNumber: 3,
            title: "Renamed draft",
            body: "Body one",
            excerpt: null,
            createdAt: "2026-04-01T00:00:00Z",
          },
        }),
      };
    });
    const result = await saveWorkspaceDraft(
      "http://api.local",
      "ws",
      "it",
      {
        title: "Renamed draft",
        slug: "renamed-draft",
        body: "Body one",
        plannedPublishAt: "2026-06-01T09:00:00Z",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("saved");
    if (result.kind === "saved") expect(result.version.versionNumber).toBe(3);
  });

  it("maps a slug conflict to a conflict result", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({
        error: "a content item with that slug already exists in this workspace",
      }),
    });
    const result = await saveWorkspaceDraft(
      "http://api.local",
      "ws",
      "it",
      { title: "T", slug: "taken" },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("conflict");
  });

  it("splits a 400 validation error into problems", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 400,
      ok: false,
      json: async () => ({ error: "title cannot be blank" }),
    });
    const result = await saveWorkspaceDraft(
      "http://api.local",
      "ws",
      "it",
      { title: "T" },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("rejected");
    if (result.kind === "rejected") {
      expect(result.problems).toContain("title cannot be blank");
    }
  });
});

describe("generateWorkspaceContent", () => {
  it("rejects a missing topic without calling the API", async () => {
    const mockFetch = vi.fn();
    const result = await generateWorkspaceContent(
      "http://api.local",
      "ws",
      { kind: "social_post", topic: "  " },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("rejected");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("sends the ask and returns the stored draft", async () => {
    const mockFetch = vi.fn().mockImplementation(async (_url: string, init?: RequestInit) => {
      const payload = JSON.parse(String(init?.body)) as Record<string, unknown>;
      expect(payload.kind).toBe("social_post");
      expect(payload.topic).toBe("Morning smoothie tips");
      expect(payload.channel).toBe("instagram");
      expect(payload.instructions).toBe("Keep casual");
      return {
        status: 201,
        ok: true,
        json: async () => ({
          id: "gen-1",
          workspaceId: "ws",
          kind: "social_post",
          status: "draft",
          title: "Smoothie tips",
          slug: "smoothie-tips",
          plannedPublishAt: null,
          createdAt: "2026-04-01T00:00:00Z",
          updatedAt: "2026-04-01T00:00:00Z",
          provider: "stub",
          model: "stub-v1",
          version: {
            id: "v1",
            contentItemId: "gen-1",
            versionNumber: 1,
            title: "Smoothie tips",
            body: "Generated body",
            excerpt: null,
            createdAt: "2026-04-01T00:00:00Z",
          },
        }),
      };
    });
    const result = await generateWorkspaceContent(
      "http://api.local",
      "ws",
      {
        kind: "social_post",
        topic: "Morning smoothie tips",
        channel: "Instagram",
        instructions: "Keep casual",
      },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("created");
    if (result.kind === "created") {
      expect(result.item.status).toBe("draft");
      expect(result.version.versionNumber).toBe(1);
      expect(result.provider).toBe("stub");
    }
  });

  it("only sends channel for social posts", async () => {
    const mockFetch = vi.fn().mockImplementation(async (_url: string, init?: RequestInit) => {
      const payload = JSON.parse(String(init?.body)) as Record<string, unknown>;
      expect(payload.channel).toBeUndefined();
      return { status: 201, ok: true, json: async () => generatedItemFixture };
    });
    await generateWorkspaceContent(
      "http://api.local",
      "ws",
      { kind: "blog_article", topic: "Anything" },
      mockFetch as unknown as typeof fetch,
    );
  });

  it("maps a provider failure (502) to a failed result with a safe message", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 502,
      ok: false,
      json: async () => ({ error: "the model did not answer in time" }),
    });
    const result = await generateWorkspaceContent(
      "http://api.local",
      "ws",
      { kind: "blog_article", topic: "x" },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("failed");
    if (result.kind === "failed") {
      expect(result.message).toBe("the model did not answer in time");
    }
  });

  it("maps a slug conflict to a conflict result", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({ error: "the generated slug already exists in this workspace" }),
    });
    const result = await generateWorkspaceContent(
      "http://api.local",
      "ws",
      { kind: "blog_article", topic: "x" },
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("conflict");
  });
});

const generatedItemFixture = {
  id: "gen-2",
  workspaceId: "ws",
  kind: "blog_article" as const,
  status: "draft" as const,
  title: "New draft",
  slug: null,
  plannedPublishAt: null,
  createdAt: "2026-04-01T00:00:00Z",
  updatedAt: "2026-04-01T00:00:00Z",
  provider: "stub",
  model: "stub-v1",
  version: {
    id: "v1",
    contentItemId: "gen-2",
    versionNumber: 1,
    title: "New draft",
    body: "Body",
    excerpt: null,
    createdAt: "2026-04-01T00:00:00Z",
  },
};

describe("fetchContentHistory", () => {
  it("maps 404 to missing", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ status: 404, ok: false });
    const result = await fetchContentHistory(
      "http://api.local",
      "ws",
      "it",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("missing");
  });

  it("pairs versions with their decisions", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        versions: [
          {
            id: "v1",
            contentItemId: "it",
            versionNumber: 1,
            title: "T",
            body: "one",
            excerpt: null,
            createdAt: "2026-04-01T00:00:00Z",
          },
          {
            id: "v2",
            contentItemId: "it",
            versionNumber: 2,
            title: "T",
            body: "two",
            excerpt: null,
            createdAt: "2026-04-02T00:00:00Z",
            decision: {
              id: "a1",
              contentItemId: "it",
              contentVersionId: "v2",
              decision: "approved",
              note: "ok",
              decidedAt: "2026-04-02T01:00:00Z",
            },
          },
        ],
      }),
    });
    const result = await fetchContentHistory(
      "http://api.local",
      "ws",
      "it",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("loaded");
    if (result.kind === "loaded") {
      expect(result.history.versions).toHaveLength(2);
      expect(result.history.approvals["v1"]).toBeUndefined();
      expect(result.history.approvals["v2"]?.decision).toBe("approved");
      expect(result.history.approvals["v2"]?.note).toBe("ok");
    }
  });
});

describe("decideContentApproval", () => {
  it("posts the decision and maps the result", async () => {
    const mockFetch = vi.fn().mockImplementation(async (_url: string, init?: RequestInit) => {
      const payload = JSON.parse(String(init?.body)) as Record<string, unknown>;
      expect(payload.decision).toBe("approve");
      expect(payload.note).toBe("ship it");
      return {
        status: 200,
        ok: true,
        json: async () => ({
          id: "it",
          workspaceId: "ws",
          kind: "blog_article",
          status: "approved",
          title: "T",
          slug: null,
          plannedPublishAt: null,
          createdAt: "2026-04-01T00:00:00Z",
          updatedAt: "2026-04-02T00:00:00Z",
          decision: {
            id: "a1",
            contentItemId: "it",
            contentVersionId: "v1",
            decision: "approved",
            note: "ship it",
            decidedAt: "2026-04-02T00:00:00Z",
          },
        }),
      };
    });
    const result = await decideContentApproval(
      "http://api.local",
      "ws",
      "it",
      "approve",
      "ship it",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("decided");
    if (result.kind === "decided") {
      expect(result.item.status).toBe("approved");
      expect(result.decision.decision).toBe("approved");
    }
  });

  it("rejects an oversized note client-side", async () => {
    const mockFetch = vi.fn();
    const result = await decideContentApproval(
      "http://api.local",
      "ws",
      "it",
      "reject",
      "x".repeat(2001),
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("rejected");
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("maps a 409 conflict with the server message", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({
        error: "the newest version already has a decision; edit the draft to submit it again",
      }),
    });
    const result = await decideContentApproval(
      "http://api.local",
      "ws",
      "it",
      "approve",
      undefined,
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("conflict");
    if (result.kind === "conflict") {
      expect(result.message).toContain("already has a decision");
    }
  });
});

describe("transitionContentStatus", () => {
  it("posts the manual move and maps the item", async () => {
    const mockFetch = vi.fn().mockImplementation(async (_url: string, init?: RequestInit) => {
      const payload = JSON.parse(String(init?.body)) as Record<string, unknown>;
      expect(payload.status).toBe("review");
      return {
        status: 200,
        ok: true,
        json: async () => ({
          id: "it",
          workspaceId: "ws",
          kind: "blog_article",
          status: "review",
          title: "T",
          slug: null,
          plannedPublishAt: null,
          createdAt: "2026-04-01T00:00:00Z",
          updatedAt: "2026-04-02T00:00:00Z",
        }),
      };
    });
    const result = await transitionContentStatus(
      "http://api.local",
      "ws",
      "it",
      "review",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("moved");
    if (result.kind === "moved") expect(result.item.status).toBe("review");
  });

  it("maps a 409 conflict", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({
        error: "that status change is not allowed from the item's current status",
      }),
    });
    const result = await transitionContentStatus(
      "http://api.local",
      "ws",
      "it",
      "review",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("conflict");
  });
});

describe("pushToSanity", () => {
  const wsId = "ws-123";
  const itemId = "ci-9";

  it("rejects invalid input without a network call", async () => {
    const mockFetch = vi.fn();
    for (const [integrationId, mode] of [
      ["", "draft"],
      ["   ", "publish"],
      ["int-1", "live"],
    ] as const) {
      const result = await pushToSanity(
        "http://api.local",
        wsId,
        itemId,
        integrationId,
        mode as "draft" | "publish",
        mockFetch as unknown as typeof fetch,
      );
      expect(result.kind).toBe("rejected");
    }
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("posts to the publish endpoint with the integration and mode", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 201,
      ok: true,
      json: async () => ({
        id: "ci-9",
        workspaceId: wsId,
        kind: "blog_article",
        status: "draft",
        title: "Pilot post",
        slug: "pilot-post",
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-03-01T00:00:00Z",
        mode: "draft",
        draftId: "drafts.mktops-ci-9-v1",
        transactionId: "tx-1",
        message: "ok",
      }),
    });
    const result = await pushToSanity(
      "http://api.local",
      wsId,
      itemId,
      "int-1",
      "draft",
      mockFetch as unknown as typeof fetch,
    );
    expect(mockFetch).toHaveBeenCalledWith(
      "http://api.local/api/v1/workspaces/ws-123/content/ci-9/publish",
      expect.objectContaining({ method: "POST" }),
    );
    const body = JSON.parse(mockFetch.mock.calls[0][1].body);
    expect(body).toEqual({ integrationId: "int-1", mode: "draft" });
    expect(result.kind).toBe("pushed");
    if (result.kind === "pushed") {
      expect(result.success.draftId).toBe("drafts.mktops-ci-9-v1");
      expect(result.success.publicUrl).toBeNull();
    }
  });

  it("surfaces Sanity-side 502 failures as failed", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 502,
      ok: false,
      json: async () => ({ error: "the Sanity API refused the request (HTTP 403)" }),
    });
    const result = await pushToSanity(
      "http://api.local",
      wsId,
      itemId,
      "int-1",
      "publish",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("failed");
    if (result.kind === "failed") {
      expect(result.message).toContain("403");
    }
  });

  it("surfaces the duplicate-publish conflict as conflict", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 409,
      ok: false,
      json: async () => ({ error: "this exact version was already published" }),
    });
    const result = await pushToSanity(
      "http://api.local",
      wsId,
      itemId,
      "int-1",
      "publish",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("conflict");
    expect(result.kind === "conflict" && result.message).toContain("already published");
    void result;
  });

  it("parses the published reply with a verified URL", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      json: async () => ({
        id: "ci-9",
        workspaceId: wsId,
        kind: "blog_article",
        status: "published",
        title: "Pilot post",
        slug: "pilot-post",
        createdAt: "2026-03-01T00:00:00Z",
        updatedAt: "2026-03-01T00:00:00Z",
        mode: "publish",
        jobId: "job-1",
        transactionId: "tx-2",
        publicUrl: "https://nutritionguide.xyz/posts/pilot-post",
        urlVerified: true,
        message: "Published",
      }),
    });
    const result = await pushToSanity(
      "http://api.local",
      wsId,
      itemId,
      "int-1",
      "publish",
      mockFetch as unknown as typeof fetch,
    );
    expect(result.kind).toBe("pushed");
    if (result.kind === "pushed") {
      expect(result.success.item.status).toBe("published");
      expect(result.success.publicUrl).toBe("https://nutritionguide.xyz/posts/pilot-post");
      expect(result.success.urlVerified).toBe(true);
      expect(result.success.jobId).toBe("job-1");
    }
  });
});
