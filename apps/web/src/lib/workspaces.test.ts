import { describe, expect, it } from "vitest";
import {
  createWorkspace,
  fetchWorkspaceDetail,
  fetchWorkspaces,
  parseWorkspace,
  parseWorkspaces,
  saveBrandProfile,
  validateWorkspaceInput,
  type Workspace,
} from "./workspaces";

const validWorkspace: Workspace = {
  id: "22222222-2222-2222-2222-222222222222",
  slug: "demo-studio",
  name: "Demo Studio",
  websiteUrl: null,
  status: "active",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("parseWorkspace", () => {
  it("accepts a well-formed workspace with and without a website", () => {
    expect(parseWorkspace({ ...validWorkspace })).toEqual(validWorkspace);
    expect(parseWorkspace({ ...validWorkspace, websiteUrl: "https://demo-cafe.example" })).toEqual({
      ...validWorkspace,
      websiteUrl: "https://demo-cafe.example",
    });
  });

  it("rejects malformed data", () => {
    expect(parseWorkspace(null)).toBeNull();
    expect(parseWorkspace("workspace")).toBeNull();
    expect(parseWorkspace([])).toBeNull();
    expect(parseWorkspace({ ...validWorkspace, id: 7 })).toBeNull();
    expect(parseWorkspace({ ...validWorkspace, slug: "" })).toBeNull();
    expect(parseWorkspace({ ...validWorkspace, websiteUrl: 5 })).toBeNull();
    expect(parseWorkspace({ ...validWorkspace, status: undefined })).toBeNull();
    expect(parseWorkspace({ ...validWorkspace, createdAt: 1 })).toBeNull();
  });
});

describe("parseWorkspaces", () => {
  it("accepts a well-formed list", () => {
    expect(parseWorkspaces({ workspaces: [validWorkspace] })).toEqual([validWorkspace]);
    expect(parseWorkspaces({ workspaces: [] })).toEqual([]);
  });

  it("rejects a non-object body or non-list / malformed list", () => {
    expect(parseWorkspaces(null)).toBeNull();
    expect(parseWorkspaces([validWorkspace])).toBeNull();
    expect(parseWorkspaces({ workspaces: "nope" })).toBeNull();
    expect(parseWorkspaces({ workspaces: [{ id: "x" }] })).toBeNull();
  });
});

describe("validateWorkspaceInput", () => {
  it("accepts a normal workspace form entry", () => {
    expect(
      validateWorkspaceInput({
        slug: "demo-cafe",
        name: "Demo Cafe",
        websiteUrl: "https://x.example",
      }),
    ).toEqual([]);
  });

  it("reports empty or malformed values", () => {
    expect(validateWorkspaceInput({ slug: "", name: "" }).length).toBeGreaterThan(0);
    expect(validateWorkspaceInput({ slug: "Nope!", name: "x" })).toContain(
      "Slugs use lowercase letters, digits, or hyphens.",
    );
    expect(validateWorkspaceInput({ slug: "ok", name: "   " })).toContain("A name is required.");
    expect(validateWorkspaceInput({ slug: "ok", name: "a".repeat(121) }).length).toBe(1);
  });
});

describe("fetchWorkspaces", () => {
  it("returns a parsed list on success", async () => {
    const fetchImpl = async () => jsonResponse(200, { workspaces: [validWorkspace] });
    const state = await fetchWorkspaces("http://api.test", fetchImpl);
    expect(state).toEqual({ kind: "loaded", workspaces: [validWorkspace] });
  });

  it("surfaces API errors, malformed bodies, and network failures as unavailable", async () => {
    const http409 = await fetchWorkspaces("http://api.test", async () =>
      jsonResponse(409, { error: "a workspace with that slug already exists" }),
    );
    expect(http409).toEqual({
      kind: "unavailable",
      reason: "a workspace with that slug already exists",
    });

    const httpPlain = await fetchWorkspaces(
      "http://api.test",
      async () => new Response("boom", { status: 500 }),
    );
    expect(http409).not.toEqual(
      expect.objectContaining({ reason: expect.stringContaining("boom") }),
    );
    expect(httpPlain.kind).toBe("unavailable");

    const malformed = await fetchWorkspaces("http://api.test", async () =>
      jsonResponse(200, { workspaces: [{ id: 4 }] }),
    );
    expect(malformed).toEqual({
      kind: "unavailable",
      reason: "The API answered, but the response was not a workspace list.",
    });

    const network = await fetchWorkspaces("http://api.test", async () => {
      throw new TypeError("connection refused");
    });
    expect(network).toEqual({
      kind: "unavailable",
      reason: "Could not reach the API at http://api.test.",
    });
  });
});

const validBrand = {
  businessDescription: "Coffee, baked.",
  audience: "Locals",
  services: null,
  tone: "Warm",
  rules: { voice: "warm" },
  prohibitedClaims: ["no price claims"],
  callsToAction: ["Book now"],
  updatedAt: "2026-02-03T04:05:06Z",
};

describe("fetchWorkspaceDetail", () => {
  it("returns a parsed workspace with a null brand", async () => {
    const fetchImpl = async () => jsonResponse(200, { workspace: validWorkspace, brand: null });
    const state = await fetchWorkspaceDetail("http://api.test", "abc", fetchImpl);
    expect(state).toEqual({ kind: "loaded", workspace: validWorkspace, brand: null });
  });

  it("accepts a well-formed brand profile", async () => {
    const fetchImpl = async () =>
      jsonResponse(200, { workspace: validWorkspace, brand: validBrand });
    const state = await fetchWorkspaceDetail("http://api.test", "abc", fetchImpl);
    expect(state).toEqual({ kind: "loaded", workspace: validWorkspace, brand: validBrand });
  });

  it("maps 404 to missing and other errors to unavailable", async () => {
    const missing = await fetchWorkspaceDetail("http://api.test", "abc", async () =>
      jsonResponse(404, { error: "no such workspace" }),
    );
    expect(missing).toEqual({ kind: "missing", reason: "no such workspace" });

    const broken = await fetchWorkspaceDetail(
      "http://api.test",
      "abc",
      async () => new Response("oops", { status: 500 }),
    );
    expect(broken.kind).toBe("unavailable");

    const malformed = await fetchWorkspaceDetail("http://api.test", "abc", async () =>
      jsonResponse(200, { workspace: null, brand: null }),
    );
    expect(malformed).toEqual({
      kind: "unavailable",
      reason: "The API answered, but the body was not a workspace detail.",
    });

    const badBrand = await fetchWorkspaceDetail("http://api.test", "abc", async () =>
      jsonResponse(200, {
        workspace: validWorkspace,
        brand: { ...validBrand, prohibitedClaims: "nope" },
      }),
    );
    expect(badBrand).toEqual({
      kind: "unavailable",
      reason: "The API answered, but the brand profile was malformed.",
    });
  });
});

describe("saveBrandProfile", () => {
  const input = {
    businessDescription: "Coffee",
    audience: null,
    services: null,
    tone: "Warm",
    rules: {},
    prohibitedClaims: [],
    callsToAction: [],
  };

  it("PUTs JSON and returns the saved brand", async () => {
    let captured: RequestInit | undefined;
    const fetchImpl = async (_url: unknown, init?: RequestInit) => {
      captured = init;
      return jsonResponse(200, validBrand);
    };
    const result = await saveBrandProfile("http://api.test", "abc", input, fetchImpl);
    expect(result).toEqual({ kind: "saved", brand: validBrand });
    expect(captured?.method).toBe("PUT");
    expect(JSON.parse(String(captured?.body))).toEqual(input);
  });

  it("maps validation rejections and network failures", async () => {
    const rejected = await saveBrandProfile("http://api.test", "abc", input, async () =>
      jsonResponse(400, { error: "businessDescription is too long" }),
    );
    expect(rejected).toEqual({ kind: "rejected", message: "businessDescription is too long" });

    const offline = await saveBrandProfile("http://api.test", "abc", input, async () => {
      throw new TypeError("offline");
    });
    expect(offline).toEqual({
      kind: "unavailable",
      reason: "Could not reach the API at http://api.test.",
    });
  });
});

describe("createWorkspace", () => {
  const input = {
    slug: "demo-cafe",
    name: "Demo Cafe",
    websiteUrl: " https://demo-cafe.example/ ",
  };

  it("posts a trimmed payload and returns the created workspace", async () => {
    let captured: RequestInit | undefined;
    const fetchImpl = async (_url: unknown, init?: RequestInit) => {
      captured = init;
      return jsonResponse(201, { ...validWorkspace, slug: "demo-cafe", name: "Demo Cafe" });
    };
    const result = await createWorkspace("http://api.test", input, fetchImpl);
    expect(result).toEqual({
      kind: "created",
      workspace: { ...validWorkspace, slug: "demo-cafe", name: "Demo Cafe" },
    });
    expect(captured?.method).toBe("POST");
    const body = JSON.parse(String(captured?.body)) as Record<string, string>;
    expect(body.slug).toBe("demo-cafe");
    // Whitespace is trimmed client-side; the server trims the trailing slash.
    expect(body.websiteUrl).toBe("https://demo-cafe.example/");
  });

  it("maps 400/409 responses to rejected with the API message", async () => {
    const badRequest = await createWorkspace("http://api.test", input, async () =>
      jsonResponse(400, { error: "slug must be lowercase letters, digits, or hyphens" }),
    );
    expect(badRequest).toEqual({
      kind: "rejected",
      message: "slug must be lowercase letters, digits, or hyphens",
    });
    const conflict = await createWorkspace("http://api.test", input, async () =>
      jsonResponse(409, { error: "a workspace with that slug already exists" }),
    );
    expect(conflict).toEqual({
      kind: "rejected",
      message: "a workspace with that slug already exists",
    });
  });

  it("maps network failures to unavailable", async () => {
    const result = await createWorkspace("http://api.test", input, async () => {
      throw new TypeError("offline");
    });
    expect(result).toEqual({
      kind: "unavailable",
      reason: "Could not reach the API at http://api.test.",
    });
  });
});
