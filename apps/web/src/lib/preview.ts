/**
 * A deliberately minimal, dependency-free layout parser for draft previews.
 *
 * Drafts mostly arrive from the AI provider as simple markdown (headings,
 * paragraphs, lists, horizontal rules). Blocks are derived structurally so
 * the React renderer can build them from plain text nodes — nothing is ever
 * injected as raw HTML, so no crafted body text can execute script.
 */

export type PreviewBlock =
  | { type: "h1"; text: string }
  | { type: "h2"; text: string }
  | { type: "h3"; text: string }
  | { type: "p"; text: string }
  | { type: "ul"; items: string[] }
  | { type: "ol"; items: string[] }
  | { type: "hr" };

export interface InlineSegment {
  text: string;
  bold: boolean;
}

/** Splits "a **bold** b" into segments; unmatched markers stay literal. */
export function inlineSegments(text: string): InlineSegment[] {
  const segments: InlineSegment[] = [];
  const parts = text.split("**");
  const markers = parts.length - 1;
  if (markers === 0 || markers % 2 !== 0) {
    // No closed pairs: keep the text exactly as written.
    if (text) segments.push({ text, bold: false });
    return segments;
  }
  for (let i = 0; i < parts.length; i++) {
    if (parts[i] !== "") {
      segments.push({ text: parts[i], bold: i % 2 === 1 });
    }
  }
  return segments;
}

const headingLevel = (line: string): 1 | 2 | 3 | 0 => {
  const match = /^(#{1,6})\s/.exec(line.trim());
  if (!match) return 0;
  const level = Math.min(match[1].length, 3);
  return level as 1 | 2 | 3;
};

const isHr = (line: string): boolean => /^-{3,}$/.test(line.trim());
const isBullet = (line: string): string | null => {
  const match = /^[*-]\s+(.*)$/.exec(line.trim());
  return match ? match[1] : null;
};
const isOrdered = (line: string): string | null => {
  const match = /^\d+[.)]\s+(.*)$/.exec(line.trim());
  return match ? match[1] : null;
};

/** Parses the draft body into display blocks. */
export function parseDraftBody(body: string): PreviewBlock[] {
  const blocks: PreviewBlock[] = [];
  const lines = body.replace(/\r\n/g, "\n").split("\n");

  let paragraph: string[] = [];
  let listKind: "ul" | "ol" | null = null;
  let listItems: string[] = [];

  const flushParagraph = () => {
    if (paragraph.length > 0) {
      blocks.push({ type: "p", text: paragraph.join(" ").trim() });
      paragraph = [];
    }
  };
  const flushList = () => {
    if (listKind && listItems.length > 0) {
      blocks.push({ type: listKind, items: listItems });
    }
    listKind = null;
    listItems = [];
  };

  for (const line of lines) {
    const level = headingLevel(line);
    if (level > 0) {
      flushParagraph();
      flushList();
      blocks.push({
        type: level === 1 ? "h1" : level === 2 ? "h2" : "h3",
        text: line.replace(/^#+\s*/, "").trim(),
      });
      continue;
    }
    if (isHr(line)) {
      flushParagraph();
      flushList();
      blocks.push({ type: "hr" });
      continue;
    }
    const bullet = isBullet(line);
    const ordered = bullet === null ? isOrdered(line) : null;
    if (bullet !== null) {
      flushParagraph();
      if (listKind !== "ul") {
        flushList();
        listKind = "ul";
      }
      listItems.push(bullet.trim());
      continue;
    }
    if (ordered !== null) {
      flushParagraph();
      if (listKind !== "ol") {
        flushList();
        listKind = "ol";
      }
      listItems.push(ordered);
      continue;
    }
    flushList();
    if (line.trim() === "") {
      flushParagraph();
    } else {
      paragraph.push(line.trim());
    }
  }
  flushParagraph();
  flushList();
  return blocks;
}
