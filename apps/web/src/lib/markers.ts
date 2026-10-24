/**
 * Single-line media markers embedded in draft bodies.
 *
 * After an image upload succeeds, the editor appends one marker inside the
 * body text, e.g.:
 *
 *   [media-image ref=image-abc-800x600-jpg src=https://cdn.sanity.io/... alt="Alt"]
 *   [media-youtube src=https://youtu.be/ID]
 *
 * The API converts markers into Portable Text media blocks when publishing;
 * `parseDraftBody` turns them into preview blocks in the browser. Values are
 * `key=value` chips: unquoted values must not contain spaces; `alt` may be
 * quoted to allow spaces. Anything unparseable stays plain text.
 */

export interface MediaImageRef {
  ref: string;
  src?: string;
  alt?: string;
}

const IMAGE_MARKER = "[media-image ";
const YOUTUBE_MARKER = "[media-youtube ";

const TOKEN_PATTERN = /(?:[A-Za-z0-9_-]+)=(?:"(?:[^"\\]|\\.)*"|\S+)/g;

function parseMarkerTokens(inner: string): Map<string, string> {
  const fields = new Map<string, string>();
  const tokens = inner.match(TOKEN_PATTERN) ?? [];
  for (const token of tokens) {
    const eq = token.indexOf("=");
    if (eq <= 0) continue;
    const key = token.slice(0, eq);
    let value = token.slice(eq + 1);
    if (value.startsWith('"') && value.endsWith('"') && value.length >= 2) {
      value = value.slice(1, -1).replace(/\\(["])/g, "$1");
    }
    fields.set(key, value);
  }
  return fields;
}

function markerLine(trimmed: string, prefix: string): string | null {
  if (!trimmed.startsWith(prefix) || !trimmed.endsWith("]")) return null;
  return trimmed.slice(prefix.length, trimmed.length - 1);
}

/** Parses one line as an image marker; null when it is not a valid one. */
export function parseImageMarker(line: string): MediaImageRef | null {
  const inner = markerLine(line.trim(), IMAGE_MARKER);
  if (inner === null) return null;
  const fields = parseMarkerTokens(inner);
  const ref = fields.get("ref");
  if (!ref) return null;
  return { ref, src: fields.get("src") || undefined, alt: fields.get("alt") || undefined };
}

/** Parses one line as a YouTube marker, returning the pasted URL. */
export function parseYoutubeMarker(line: string): string | null {
  const inner = markerLine(line.trim(), YOUTUBE_MARKER);
  if (inner === null) return null;
  return parseMarkerTokens(inner).get("src") ?? null;
}

/** Builds the image marker inserted after an upload. */
export function buildImageMarker(image: { ref: string; src?: string; alt?: string }): string {
  const parts = [`ref=${image.ref}`];
  if (image.src) parts.push(`src=${image.src}`);
  if (image.alt) parts.push(`alt="${image.alt.replace(/["\\]/g, "\\$&")}"`);
  return `[media-image ${parts.join(" ")}]`;
}

/** Builds the YouTube marker for a pasted video URL. */
export function buildYoutubeMarker(url: string): string {
  return `[media-youtube src=${url}]`;
}

const YOUTUBE_HOSTS = new Set([
  "youtube.com",
  "www.youtube.com",
  "m.youtube.com",
  "youtu.be",
  "www.youtu.be",
]);

/**
 * Converts an https YouTube URL (watch, share, shorts, or embed form) into a
 * youtube-nocookie embed URL. Returns null for anything else, so only real
 * YouTube links are ever rendered as frames.
 */
export function youtubeEmbedUrl(raw: string): string | null {
  try {
    const url = new URL(raw);
    if (url.protocol !== "https:" || !YOUTUBE_HOSTS.has(url.hostname)) return null;
    let id = "";
    if (url.hostname.endsWith("youtu.be")) {
      id = url.pathname.slice(1);
    } else if (url.pathname.startsWith("/embed/") || url.pathname.startsWith("/shorts/")) {
      id = url.pathname.split("/")[2] ?? "";
    } else {
      id = url.searchParams.get("v") ?? "";
    }
    if (!/^[A-Za-z0-9_-]{6,}$/.test(id)) return null;
    return `https://www.youtube-nocookie.com/embed/${id}`;
  } catch {
    return null;
  }
}
