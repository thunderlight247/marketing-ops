import { describe, expect, it } from "vitest";
import { DEFAULT_API_BASE_URL, getApiBaseUrl, withAuth } from "./config";

describe("getApiBaseUrl", () => {
  it("uses the same-origin proxy by default", () => {
    expect(getApiBaseUrl()).toBe("");
    expect(getApiBaseUrl(" ")).toBe(DEFAULT_API_BASE_URL);
  });

  it("trims explicit base URLs", () => {
    expect(getApiBaseUrl(" https://api.example.com/ops-api/// ")).toBe(
      "https://api.example.com/ops-api",
    );
  });
});

describe("withAuth", () => {
  it("preserves headers without adding an API key", () => {
    expect(withAuth({ "Content-Type": "application/json" })).toEqual({
      "Content-Type": "application/json",
    });
  });
});
