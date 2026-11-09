import { describe, expect, it } from "vitest";
import { parseAnalyticsBreakdown, parseAnalyticsSummary } from "./analytics";

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

describe("parseAnalyticsBreakdown", () => {
  const breakdown = {
    from: "2026-10-09T00:00:00Z",
    to: "2026-11-09T00:00:00Z",
    trend: [{ day: "2026-11-07", crm: 2, engine: 5 }],
    utm: {
      source: { google: 3, "(none)": 4 },
      medium: { cpc: 3, "(none)": 4 },
      campaign: { spring: 3, "(none)": 4 },
    },
  };

  it("accepts a well-formed breakdown", () => {
    const parsed = parseAnalyticsBreakdown(breakdown);
    expect(parsed?.trend).toEqual([{ day: "2026-11-07", crm: 2, engine: 5 }]);
    expect(parsed?.utm.source).toEqual({ google: 3, "(none)": 4 });
  });

  it("rejects a missing UTM map or a negative count", () => {
    expect(parseAnalyticsBreakdown({ ...breakdown, utm: { source: {}, medium: {} } })).toBeNull();
    expect(
      parseAnalyticsBreakdown({ ...breakdown, trend: [{ day: "2026-11-07", crm: -1, engine: 0 }] }),
    ).toBeNull();
  });
});

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
