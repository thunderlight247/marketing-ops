import { describe, expect, it } from "vitest";
import { NAV_ITEMS, findNavItem, isActive } from "./navigation";

describe("NAV_ITEMS", () => {
  it("includes every required section", () => {
    const labels = NAV_ITEMS.map((item) => item.label);
    for (const required of [
      "Workspaces",
      "Content",
      "Calendar",
      "Integrations",
      "Analytics",
      "Settings",
    ]) {
      expect(labels).toContain(required);
    }
  });

  it("has unique keys and hrefs", () => {
    expect(new Set(NAV_ITEMS.map((i) => i.key)).size).toBe(NAV_ITEMS.length);
    expect(new Set(NAV_ITEMS.map((i) => i.href)).size).toBe(NAV_ITEMS.length);
  });

  it("gives every placeholder section an empty state", () => {
    for (const item of NAV_ITEMS.filter((i) => i.key !== "overview")) {
      expect(item.emptyTitle).not.toBe("");
      expect(item.emptyBody).not.toBe("");
    }
  });
});

describe("isActive", () => {
  it("matches the home page only exactly", () => {
    expect(isActive("/", "/")).toBe(true);
    expect(isActive("/content", "/")).toBe(false);
  });

  it("matches a section and its nested pages, but not lookalike paths", () => {
    expect(isActive("/content", "/content")).toBe(true);
    expect(isActive("/content/42", "/content")).toBe(true);
    expect(isActive("/contentious", "/content")).toBe(false);
  });
});

describe("findNavItem", () => {
  it("returns the item for a key", () => {
    expect(findNavItem("settings").href).toBe("/settings");
  });
});
