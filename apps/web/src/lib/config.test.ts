import { describe, expect, it } from "vitest";
import { DEFAULT_API_BASE_URL, getApiBaseUrl } from "./config";

describe("getApiBaseUrl", () => {
  it("falls back to the default when unset or blank", () => {
    expect(getApiBaseUrl("")).toBe(DEFAULT_API_BASE_URL);
    expect(getApiBaseUrl("   ")).toBe(DEFAULT_API_BASE_URL);
  });

  it("trims whitespace and trailing slashes", () => {
    expect(getApiBaseUrl(" https://api.example.com/// ")).toBe("https://api.example.com");
  });

  it("keeps a path prefix", () => {
    expect(getApiBaseUrl("https://example.com/ops-api/")).toBe("https://example.com/ops-api");
  });
});
