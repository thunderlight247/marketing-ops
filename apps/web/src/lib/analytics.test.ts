import { describe, expect, it } from "vitest";
import { parseAnalyticsSummary } from "./analytics";

const valid = {
  generatedAt: "2026-11-07T12:00:00Z",
  totals: {
    workspaces: 2,
    campaigns: 3,
    crmLeads: 4,
    crmWon: 1,
    engineLeads: 121,
    engineSent: 80,
    engineReplied: 0,
    engineFailed: 6,
    engineBlacklisted: 35,
    emailsTotal: 9,
    emailsSent: 8,
    emailsDelivered: 7,
  },
  byWorkspace: [
    {
      workspaceId: "w1",
      slug: "redeem",
      name: "Redeem",
      campaigns: 2,
      crmLeads: 3,
      crmWon: 1,
      engineLeads: 85,
      engineSent: 47,
      engineReplied: 0,
      emailsTotal: 5,
      emailsSent: 4,
      emailsDelivered: 3,
    },
  ],
  crmLeadsByStatus: { new: 2, contacted: 1, won: 1 },
  engineLeadsByStatus: { sent: 47, blacklisted: 32, failed: 6 },
};

describe("parseAnalyticsSummary", () => {
  it("accepts the API payload", () => {
    const summary = parseAnalyticsSummary(valid);
    expect(summary?.totals.engineLeads).toBe(121);
    expect(summary?.byWorkspace).toHaveLength(1);
    expect(summary?.engineLeadsByStatus.blacklisted).toBe(32);
  });

  it("rejects a payload with a missing count", () => {
    const broken = {
      ...valid,
      totals: { ...valid.totals, engineLeads: undefined },
    };
    expect(parseAnalyticsSummary(broken)).toBeNull();
  });

  it("rejects negative or fractional counts", () => {
    expect(
      parseAnalyticsSummary({
        ...valid,
        totals: { ...valid.totals, emailsSent: -1 },
      }),
    ).toBeNull();
    expect(
      parseAnalyticsSummary({
        ...valid,
        totals: { ...valid.totals, emailsSent: 1.5 },
      }),
    ).toBeNull();
  });
});
