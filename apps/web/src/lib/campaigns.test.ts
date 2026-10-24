import { describe, expect, it } from "vitest";
import {
  createCampaign,
  createLead,
  fetchWorkspaceCampaignsLeads,
  formatCampaignStatus,
  formatLeadSource,
  formatLeadStatus,
  isCampaignStatus,
  isLeadSource,
  isLeadStatus,
  updateLead,
  validateCampaignInput,
  validateLeadInput,
} from "./campaigns";

function jsonRes(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

describe("validation and formatting", () => {
  it("campaign validation requires a name within limits", () => {
    expect(validateCampaignInput({ name: "" })).toContain("campaign name is required");
    expect(validateCampaignInput({ name: "a".repeat(121) })).toContain(
      "campaign name is too long (max 120 characters)",
    );
    expect(validateCampaignInput({ name: "Summer push", status: "weird" })).toContain(
      "status must be planned, active, paused, or completed",
    );
    expect(validateCampaignInput({ name: "Summer push", status: "active" })).toEqual([]);
  });

  it("lead validation keeps sources traceable and emails plausible", () => {
    expect(validateLeadInput({ sourceKind: "scraped" })).toContain(
      "sourceKind must be form/api, import, manual, or unknown",
    );
    expect(validateLeadInput({ sourceKind: "unknown" })).toEqual([]);
    expect(validateLeadInput({ sourceKind: "manual", contactEmail: "nope" })).toContain(
      "email does not look valid",
    );
    expect(validateLeadInput({ sourceKind: "manual", followUpAt: "not-a-date" })).toContain(
      "follow-up date is not a valid date",
    );
  });

  it("status/source predicates cover the domain values only", () => {
    expect(isCampaignStatus("planned")).toBe(true);
    expect(isCampaignStatus("viral")).toBe(false);
    expect(isLeadStatus("won")).toBe(true);
    expect(isLeadStatus("pending")).toBe(false);
    expect(isLeadSource("form_api")).toBe(true);
    expect(isLeadSource("guess")).toBe(false);
  });

  it("formats use human labels", () => {
    expect(formatCampaignStatus("paused")).toBe("Paused");
    expect(formatLeadStatus("qualified")).toBe("Qualified");
    expect(formatLeadSource("unknown")).toBe("Source unknown");
    expect(formatLeadSource("manual")).toBe("Recorded manually");
  });
});

describe("fetchWorkspaceCampaignsLeads", () => {
  it("loads and parses both endpoint results", async () => {
    const base = "https://api.test";
    const calls: string[] = [];
    const impl: typeof fetch = async (input, init) => {
      void init;
      const url = String(input);
      calls.push(url);
      if (url.endsWith("/campaigns")) {
        return jsonRes(200, {
          campaigns: [{ id: "c1", workspaceId: "w", name: "C", status: "active" }],
        });
      }
      return jsonRes(200, {
        leads: [
          {
            id: "l1",
            workspaceId: "w",
            sourceKind: "form_api",
            sourceDetail: "site form",
            status: "new",
            contactEmail: "a@b.test",
            notes: "",
          },
        ],
      });
    };
    const state = await fetchWorkspaceCampaignsLeads(base, "w", impl);
    if (state.kind !== "loaded") {
      throw new Error(`expected loaded, got ${state.kind}`);
    }
    expect(state.campaigns).toHaveLength(1);
    expect(state.campaigns[0].status).toBe("active");
    expect(state.leads[0].sourceKind).toBe("form_api");
    expect(state.leads[0].contactEmail).toBe("a@b.test");
    expect(calls.some((u) => u.includes("/campaigns"))).toBe(true);
    expect(calls.some((u) => u.includes("/leads"))).toBe(true);
  });

  it("reports unavailable on network failure and missing on 404", async () => {
    const broken: typeof fetch = async () => {
      throw new Error("offline");
    };
    const state = await fetchWorkspaceCampaignsLeads("https://api.test", "w", broken);
    expect(state.kind).toBe("unavailable");

    const missing: typeof fetch = async () => jsonRes(404, {});
    expect((await fetchWorkspaceCampaignsLeads("https://api.test", "w", missing)).kind).toBe(
      "missing",
    );
  });
});

describe("createCampaign", () => {
  it("rejects invalid input locally without a request", async () => {
    let called = 0;
    const impl: typeof fetch = async () => {
      called += 1;
      return jsonRes(201, {});
    };
    const result = await createCampaign("https://api.test", "w", { name: "  " }, impl);
    if (result.kind !== "rejected") throw new Error(`expected rejected, got ${result.kind}`);
    expect(result.problems).toContain("campaign name is required");
    expect(called).toBe(0);
  });

  it("maps a name conflict to kind conflict", async () => {
    const impl: typeof fetch = async () =>
      jsonRes(409, { error: "a campaign with that name already exists in this workspace" });
    const result = await createCampaign("https://api.test", "w", { name: "Keep" }, impl);
    expect(result.kind).toBe("conflict");
  });

  it("trims utmCampaign and notes on the way out", async () => {
    let body: Record<string, unknown> = {};
    const impl: typeof fetch = async (_input, init) => {
      body = JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>;
      return jsonRes(201, { id: "c9", workspaceId: "w", name: body.name, status: "planned" });
    };
    const result = await createCampaign(
      "https://api.test",
      "w",
      { name: " C ", utmCampaign: "  spring  ", notes: " hi " },
      impl,
    );
    if (result.kind !== "created") throw new Error(`expected created, got ${result.kind}`);
    expect(body.utmCampaign).toBe("spring");
    expect(body.notes).toBe("hi");
  });
});

describe("createLead and updateLead", () => {
  it("sends a date input as an RFC3339 timestamp for the lead API", async () => {
    let body: Record<string, unknown> = {};
    const impl: typeof fetch = async (_input, init) => {
      body = JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>;
      return jsonRes(201, {
        id: "l10",
        workspaceId: "w",
        sourceKind: "manual",
        status: "new",
        followUpAt: body.followUpAt,
      });
    };
    const result = await createLead(
      "https://api.test",
      "w",
      { sourceKind: "manual", followUpAt: "2030-04-05" },
      impl,
    );
    if (result.kind !== "created") throw new Error(`expected created, got ${result.kind}`);
    expect(body.followUpAt).toBe("2030-04-05T00:00:00.000Z");
    expect(result.lead.followUpAt).toBe("2030-04-05T00:00:00.000Z");
  });

  it("omits blank optional fields from the payload", async () => {
    let body: Record<string, unknown> = {};
    const impl: typeof fetch = async (_input, init) => {
      body = JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>;
      return jsonRes(201, {
        id: "l9",
        workspaceId: "w",
        sourceKind: body.sourceKind,
        status: "new",
      });
    };
    const result = await createLead(
      "https://api.test",
      "w",
      { sourceKind: "manual", contactName: " Jane ", message: "  " },
      impl,
    );
    if (result.kind !== "created") throw new Error(`expected created, got ${result.kind}`);
    expect(body.sourceKind).toBe("manual");
    expect(body.contactName).toBe("Jane");
    expect("message" in body).toBe(false);
    expect("contactEmail" in body).toBe(false);
  });

  it("sends the patch through unchanged and parses the saved lead", async () => {
    let body: Record<string, unknown> = {};
    let method = "";
    const impl: typeof fetch = async (_input, init) => {
      body = JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>;
      method = String(init?.method ?? "");
      return jsonRes(200, {
        id: "l5",
        workspaceId: "w",
        sourceKind: "manual",
        status: body.status ?? "new",
      });
    };
    const result = await updateLead(
      "https://api.test",
      "w",
      "l5",
      { status: "contacted", clearFollowUp: true },
      impl,
    );
    if (result.kind !== "saved") throw new Error(`expected saved, got ${result.kind}`);
    expect(method).toBe("PATCH");
    expect(body.status).toBe("contacted");
    expect(body.clearFollowUp).toBe(true);
    expect(result.lead.status).toBe("contacted");
  });

  it("unavailable results carry the server message", async () => {
    const impl: typeof fetch = async () =>
      jsonRes(400, { error: "status must be new, contacted, qualified, won, or lost" });
    const result = await updateLead("https://api.test", "w", "l5", { status: "pending" }, impl);
    if (result.kind !== "unavailable") throw new Error(`expected unavailable, got ${result.kind}`);
    expect(result.message).toContain("status must be");
  });
});
