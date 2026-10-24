import { describe, expect, it } from "vitest";
import { evaluateSeo } from "./seo";

const goodBody = "## Start\n\n" + "word ".repeat(320);

describe("evaluateSeo", () => {
  it("rates a complete blog draft as fully good", () => {
    const report = evaluateSeo({
      title: "Five simple ways to plan content",
      slug: "five-ways-plan-content",
      excerpt: `${"This summary is deliberately long. ".repeat(2)}Ends right.`,
      body: goodBody,
      kind: "blog_article",
    });
    expect(report.fail).toBe(0);
    expect(report.score).toBe(100);
    const byId = Object.fromEntries(report.checks.map((c) => [c.id, c.status]));
    expect(byId.title).toBe("good");
    expect(byId.slug).toBe("good");
    expect(byId.description).toBe("good");
    expect(byId.depth).toBe("good");
    expect(byId.headings).toBe("good");
    expect(byId["alt-text"]).toBe("good");
  });

  it("warns when the title is too short and the description is missing", () => {
    const report = evaluateSeo({ title: "Hi", slug: "x", body: goodBody, kind: "blog_article" });
    const title = report.checks.find((c) => c.id === "title");
    const description = report.checks.find((c) => c.id === "description");
    expect(title?.status).toBe("warn");
    expect(title?.detail).toContain("shorter than");
    expect(description?.status).toBe("warn");
    expect(description?.detail).toContain("Empty");
  });

  it("fails a title that will be truncated and reports the length", () => {
    const report = evaluateSeo({ title: "a".repeat(80), slug: "x", body: goodBody });
    const title = report.checks.find((c) => c.id === "title");
    expect(title?.status).toBe("fail");
    expect(title?.detail).toContain("80 characters");
  });

  it("fails invalid slugs and warns about long slugs", () => {
    const bad = evaluateSeo({ title: "Valid title here", slug: "not valid!", body: goodBody });
    expect(bad.checks.find((c) => c.id === "slug")?.status).toBe("fail");

    const long = evaluateSeo({
      title: "Valid title here",
      slug: "x".repeat(61),
      body: goodBody,
    });
    expect(long.checks.find((c) => c.id === "slug")?.status).toBe("warn");
  });

  it("counts words in the body and flags thin content", () => {
    const thin = evaluateSeo({
      title: "A fine long title",
      slug: "t",
      body: "only five words here",
    });
    const depth = thin.checks.find((c) => c.id === "depth");
    expect(depth?.status).toBe("fail");
    expect(depth?.detail).toContain("4 words");

    const mid = evaluateSeo({ title: "A fine long title", slug: "t", body: "word ".repeat(180) });
    expect(mid.checks.find((c) => c.id === "depth")?.status).toBe("warn");
  });

  it("skips the heading check for non-blog kinds and relaxes the depth target", () => {
    const social = evaluateSeo({
      title: "Short punchy social",
      slug: "social",
      excerpt: "x".repeat(80),
      body: "word ".repeat(60),
      kind: "social_post",
    });
    expect(social.checks.some((c) => c.id === "headings")).toBe(false);
    expect(social.checks.find((c) => c.id === "depth")?.status).toBe("good");
  });

  it("checks alt text on body image markers", () => {
    const withAlt =
      '[media-image ref=image-a src=https://cdn.sanity.io/images/a.jpg alt="Team lunch"]\n' +
      '[media-image ref=image-b src=https://cdn.sanity.io/images/b.jpg alt="Office"]';
    const ok = evaluateSeo({
      title: "A fine long title",
      slug: "t",
      body: withAlt + "\n\n" + goodBody,
    });
    expect(ok.checks.find((c) => c.id === "alt-text")?.status).toBe("good");

    const missing = evaluateSeo({
      title: "A fine long title",
      slug: "t",
      body: "[media-image ref=image-a src=https://cdn.sanity.io/images/a.jpg]\n\n" + goodBody,
    });
    const alt = missing.checks.find((c) => c.id === "alt-text");
    expect(alt?.status).toBe("warn");
    expect(alt?.detail).toContain("1 of 1");
  });
});
