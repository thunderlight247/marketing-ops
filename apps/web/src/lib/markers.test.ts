import { describe, expect, it } from "vitest";
import { parseDraftBody } from "./preview";
import {
  buildImageMarker,
  buildYoutubeMarker,
  parseImageMarker,
  parseYoutubeMarker,
  youtubeEmbedUrl,
} from "./markers";

describe("markers", () => {
  it("round-trips an image marker with alt text", () => {
    const marker = buildImageMarker({
      ref: "image-abc-800x600-jpg",
      src: "https://cdn.sanity.io/images/p/d/abc.jpg",
      alt: `Team "lunch" party`,
    });
    expect(marker).toBe(
      '[media-image ref=image-abc-800x600-jpg src=https://cdn.sanity.io/images/p/d/abc.jpg alt="Team \\"lunch\\" party"]',
    );
    const parsed = parseImageMarker(marker);
    expect(parsed).toEqual({
      ref: "image-abc-800x600-jpg",
      src: "https://cdn.sanity.io/images/p/d/abc.jpg",
      alt: 'Team "lunch" party',
    });
  });

  it("rejects image markers without a ref and plain lines", () => {
    expect(parseImageMarker("[media-image src=https://example.com/a.png]")).toBeNull();
    expect(parseImageMarker("an ordinary paragraph")).toBeNull();
    expect(parseImageMarker("[media-image ref=x no closing")).toBeNull();
  });

  it("round-trips a youtube marker", () => {
    const marker = buildYoutubeMarker("https://youtu.be/dQw4w9WgXcQ");
    expect(marker).toBe("[media-youtube src=https://youtu.be/dQw4w9WgXcQ]");
    expect(parseYoutubeMarker(marker)).toBe("https://youtu.be/dQw4w9WgXcQ");
    expect(parseYoutubeMarker("watch this: [media-youtube src=...")).toBeNull();
  });

  it("converts supported YouTube URL forms into embed URLs", () => {
    expect(youtubeEmbedUrl("https://youtu.be/dQw4w9WgXcQ")).toBe(
      "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ",
    );
    expect(youtubeEmbedUrl("https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30s")).toBe(
      "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ",
    );
    expect(youtubeEmbedUrl("https://www.youtube.com/shorts/abc123xyz")).toBe(
      "https://www.youtube-nocookie.com/embed/abc123xyz",
    );
    expect(youtubeEmbedUrl("http://youtu.be/dQw4w9WgXcQ")).toBeNull();
    expect(youtubeEmbedUrl("https://evil.com/watch?v=dQw4w9WgXcQ")).toBeNull();
    expect(youtubeEmbedUrl("not a url")).toBeNull();
  });
});

describe("parseDraftBody media blocks", () => {
  it("turns media markers into image and youtube blocks", () => {
    const body =
      "Paragraph.\n\n" +
      '[media-image ref=image-abc-800x600-jpg src=https://cdn.sanity.io/images/p/d/abc.jpg alt="Team lunch"]\n\n' +
      "[media-youtube src=https://youtu.be/dQw4w9WgXcQ]\n\n" +
      "[media-youtube src=https://example.com/video]";
    expect(parseDraftBody(body)).toEqual([
      { type: "p", text: "Paragraph." },
      {
        type: "image",
        src: "https://cdn.sanity.io/images/p/d/abc.jpg",
        alt: "Team lunch",
      },
      {
        type: "youtube",
        embedUrl: "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ",
        raw: "https://youtu.be/dQw4w9WgXcQ",
      },
      // Unrecognized links stay plain text.
      { type: "p", text: "https://example.com/video" },
    ]);
  });

  it("drops image markers without a usable https preview URL", () => {
    expect(parseDraftBody("[media-image ref=image-abc-800x600-jpg]")).toEqual([
      { type: "p", text: "[media-image ref=image-abc-800x600-jpg]" },
    ]);
  });
});
