import { describe, expect, it } from "vitest";
import { NAV_ITEMS, findNavItem, isActive } from "./navigation";

describe("NAV_ITEMS", () => {
  it("only contains the global sections", () => {
    const labels = NAV_ITEMS.map((item) => item.label);
    expect(labels).toEqual(["Overview", "Workspaces", "Settings"]);
    for (const removed of ["Content", "Calendar", "Integrations", "Analytics"]) {
      expect(labels).not.toContain(removed);
    }
  });

  it("has unique keys and hrefs", () => {
    expect(new Set(NAV_ITEMS.map((i) => i.key)).size).toBe(NAV_ITEMS.length);
    expect(new Set(NAV_ITEMS.map((i) => i.href)).size).toBe(NAV_ITEMS.length);
  });

  it("gives every non-overview section an empty state", () => {
    for (const item of NAV_ITEMS.filter((i) => i.key !== "overview")) {
      expect(item.emptyTitle).not.toBe("");
      expect(item.emptyBody).not.toBe("");
    }
  });
});

describe("isActive", () => {
  it("matches the home page only exactly", () => {
    expect(isActive("/", "/")).toBe(true);
    expect(isActive("/workspaces", "/")).toBe(false);
  });

  it("matches a section and its nested pages, but not lookalike paths", () => {
    expect(isActive("/workspaces", "/workspaces")).toBe(true);
    expect(isActive("/workspaces/42", "/workspaces")).toBe(true);
    expect(isActive("/workspace", "/workspaces")).toBe(false);
  });
});

describe("findNavItem", () => {
  it("returns the item for a key", () => {
    expect(findNavItem("settings").href).toBe("/settings");
  });
});
