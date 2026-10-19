import { describe, expect, it } from "vitest";
import { inlineSegments, parseDraftBody } from "./preview";

describe("inlineSegments", () => {
  it("splits bold markers into segments", () => {
    expect(inlineSegments("a **bold** b")).toEqual([
      { text: "a ", bold: false },
      { text: "bold", bold: true },
      { text: " b", bold: false },
    ]);
  });

  it("keeps unmatched markers literal", () => {
    expect(inlineSegments("just ** text")).toEqual([{ text: "just ** text", bold: false }]);
  });
});

describe("parseDraftBody", () => {
  it("parses headings, paragraphs, lists, and rules", () => {
    const body =
      "### The Story\n\nFirst paragraph.\n\n## Details\n\n- one\n- two\n\n1. a\n2. b\n\n---\n\nClosing.";
    const blocks = parseDraftBody(body);
    expect(blocks).toEqual([
      { type: "h3", text: "The Story" },
      { type: "p", text: "First paragraph." },
      { type: "h2", text: "Details" },
      { type: "ul", items: ["one", "two"] },
      { type: "ol", items: ["a", "b"] },
      { type: "hr" },
      { type: "p", text: "Closing." },
    ]);
  });

  it("joins soft-wrapped lines into one paragraph", () => {
    expect(parseDraftBody("line one\nline two")).toEqual([
      { type: "p", text: "line one line two" },
    ]);
  });

  it("returns an empty list for an empty body", () => {
    expect(parseDraftBody("")).toEqual([]);
  });
});
