import { describe, expect, it, vi } from "vitest";
import {
  createWorkspaceContent,
  fetchWorkspaceContent,
  formatContentKind,
  formatContentStatus,
  validateContentInput,
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
