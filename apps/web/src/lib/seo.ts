/**
 * SEO metadata checks.
 *
 * These are pure ADVISORY recommendations computed from the draft itself:
 * title/slug/description quality, body depth, and image alt text. They are
 * deliberately kept separate from any measured analytics (search data) so a
 * recommendation can never be confused with observed performance.
 */

import { parseImageMarker } from "./markers";

export type SeoCheckStatus = "good" | "warn" | "fail";

export interface SeoCheck {
  id: string;
  label: string;
  status: SeoCheckStatus;
  detail: string;
}

export interface SeoInput {
  title: string;
  slug?: string | null;
  excerpt?: string | null;
  body: string;
  kind?: string;
}

export interface SeoReport {
  checks: SeoCheck[];
  good: number;
  warn: number;
  fail: number;
  /** Percentage of checks rated good; 0 when nothing is checked. */
  score: number;
}

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$/;

const wordCount = (text: string): number => text.trim().split(/\s+/).filter(Boolean).length;

const runeCount = (text: string): number => Array.from(text).length;

export function evaluateSeo(input: SeoInput): SeoReport {
  const isBlog = (input.kind ?? "blog_article") === "blog_article";
  const checks: SeoCheck[] = [];

  // ---- title -----------------------------------------------------------------
  const title = (input.title ?? "").trim().replace(/^#+\s*/, "");
  const titleLen = runeCount(title);
  if (titleLen === 0) {
    checks.push({ id: "title", label: "Page title", status: "fail", detail: "Set a title first." });
  } else if (titleLen < 15) {
    checks.push({
      id: "title",
      label: "Page title",
      status: "warn",
      detail: `${titleLen} characters — shorter than the recommended 15–60.`,
    });
  } else if (titleLen <= 60) {
    checks.push({
      id: "title",
      label: "Page title",
      status: "good",
      detail: `${titleLen} characters — a good length for search results.`,
    });
  } else if (titleLen <= 70) {
    checks.push({
      id: "title",
      label: "Page title",
      status: "warn",
      detail: `${titleLen} characters — may be cut off in search results; consider trimming toward 60.`,
    });
  } else {
    checks.push({
      id: "title",
      label: "Page title",
      status: "fail",
      detail: `${titleLen} characters — very likely truncated in search results.`,
    });
  }

  // ---- slug -----------------------------------------------------------------
  const slug = (input.slug ?? "").trim();
  if (!slug) {
    checks.push({
      id: "slug",
      label: "URL slug",
      status: "warn",
      detail:
        "No slug yet — one will be generated from the title for publishing, but a shorter explicit slug usually reads better.",
    });
  } else if (!SLUG_PATTERN.test(slug)) {
    checks.push({
      id: "slug",
      label: "URL slug",
      status: "fail",
      detail: "Use lowercase letters, digits, and hyphens only.",
    });
  } else if (runeCount(slug) > 60) {
    checks.push({
      id: "slug",
      label: "URL slug",
      status: "warn",
      detail: `${runeCount(slug)} characters — long slugs are harder to read and share.`,
    });
  } else {
    checks.push({ id: "slug", label: "URL slug", status: "good", detail: "Valid and readable." });
  }

  // ---- description / excerpt --------------------------------------------------
  const excerpt = (input.excerpt ?? "").trim();
  const excerptLen = runeCount(excerpt);
  if (excerptLen === 0) {
    checks.push({
      id: "description",
      label: "Meta description (excerpt)",
      status: "warn",
      detail: "Empty — search engines may invent a snippet; write a 70–160 character summary.",
    });
  } else if (excerptLen < 70) {
    checks.push({
      id: "description",
      label: "Meta description (excerpt)",
      status: "warn",
      detail: `${excerptLen} characters — under the recommended 70–160.`,
    });
  } else if (excerptLen <= 160) {
    checks.push({
      id: "description",
      label: "Meta description (excerpt)",
      status: "good",
      detail: `${excerptLen} characters — fits typical search-result snippets.`,
    });
  } else {
    checks.push({
      id: "description",
      label: "Meta description (excerpt)",
      status: "warn",
      detail: `${excerptLen} characters — will be cut off; trim toward 160.`,
    });
  }

  // ---- body depth ---------------------------------------------------------------
  const words = wordCount(input.body);
  const bodyTarget = isBlog ? 300 : 50;
  const bodyHint = isBlog ? "300+" : "50+";
  if (words >= bodyTarget) {
    checks.push({
      id: "depth",
      label: "Content depth",
      status: "good",
      detail: `${words} words — meets the minimum for a ${isBlog ? "blog article" : "short post"}.`,
    });
  } else if (words >= Math.round(bodyTarget / 2)) {
    checks.push({
      id: "depth",
      label: "Content depth",
      status: "warn",
      detail: `${words} words — thin; aim for ${bodyHint} on a ${isBlog ? "blog article" : "short post"}.`,
    });
  } else {
    checks.push({
      id: "depth",
      label: "Content depth",
      status: "fail",
      detail: `${words} words — far below the recommended ${bodyHint}.`,
    });
  }

  // ---- headings (blog articles) -------------------------------------------------
  if (isBlog) {
    const hasH2 = input.body.split("\n").some((line) => /^##\s/.test(line.trim()));
    checks.push({
      id: "headings",
      label: "Section headings",
      status: hasH2 ? "good" : "warn",
      detail: hasH2
        ? "The body has at least one “## ” section heading."
        : "Add “## ” section headings to give the article structure.",
    });
  }

  // ---- image alt text -------------------------------------------------------------
  const imageLines = input.body
    .split("\n")
    .filter((line) => line.trim().startsWith("[media-image"));
  const missingAlt = imageLines
    .map((line) => parseImageMarker(line))
    .filter((image) => image !== null && !image.alt);
  if (imageLines.length === 0) {
    // No images: nothing to check; neutral pass keeps the score honest.
    checks.push({
      id: "alt-text",
      label: "Image alt text",
      status: "good",
      detail: "No body images yet; when you add one, give it alt text.",
    });
  } else if (missingAlt.length === 0) {
    checks.push({
      id: "alt-text",
      label: "Image alt text",
      status: "good",
      detail: `All ${imageLines.length} body image(s) have alt text.`,
    });
  } else {
    checks.push({
      id: "alt-text",
      label: "Image alt text",
      status: "warn",
      detail: `${missingAlt.length} of ${imageLines.length} body image(s) are missing alt text; the main-image field has its own alt input.`,
    });
  }

  const good = checks.filter((c) => c.status === "good").length;
  const warn = checks.filter((c) => c.status === "warn").length;
  const fail = checks.filter((c) => c.status === "fail").length;
  return {
    checks,
    good,
    warn,
    fail,
    score: checks.length === 0 ? 0 : Math.round((good / checks.length) * 100),
  };
}
