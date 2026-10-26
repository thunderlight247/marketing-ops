import { expect, test } from "@playwright/test";

test("campaign strategist files a planned proposal and the campaigns panel reflects it", async ({
  page,
}) => {
  const now = "2026-10-23T12:00:00Z";
  const workspace = {
    id: "strategist-workspace",
    slug: "strategist-demo",
    name: "Strategist demo",
    websiteUrl: null,
    status: "active",
    createdAt: now,
    updatedAt: now,
  };
  const campaign = {
    id: "strategist-campaign",
    workspaceId: workspace.id,
    name: "Winter service reminder campaign",
    status: "planned",
    utmCampaign: "winter-service-2026",
    notes: "Rationale: the brief asks for a slow-season push.",
    createdAt: now,
    updatedAt: now,
  };
  let proposed = false;
  let campaignReads = 0;
  const agentDescriptors = [
    {
      id: "campaign_strategist",
      version: "1",
      displayName: "Campaign strategist",
      description: "Proposes one planned campaign for operator review.",
      outputContract: "proposal",
      dataNeeds: [],
      actions: ["create_campaign"],
    },
  ];

  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let json: unknown;
    if (path.endsWith("/agents/campaign_strategist/run")) {
      expect(route.request().method()).toBe("POST");
      expect(route.request().postDataJSON()).toEqual({ focus: "slow-season reminder push" });
      proposed = true;
      json = {
        campaign: {
          id: campaign.id,
          name: campaign.name,
          status: campaign.status,
          utmCampaign: campaign.utmCampaign,
          notes: campaign.notes,
        },
        provider: "stub",
        model: "stub-v1",
        agentId: "campaign_strategist",
        keyPoints: ["Slow-season push", "No performance claims"],
        durationMs: 12,
      };
    } else if (path.endsWith("/campaigns")) {
      campaignReads += 1;
      json = proposed ? { campaigns: [campaign] } : { campaigns: [] };
    } else if (path.endsWith("/leads")) json = { leads: [] };
    else if (path.endsWith("/content")) json = { items: [] };
    else if (path.endsWith("/reports")) {
      // The workspace report is optional evidence; a 404 renders "missing"
      // without failing the page.
      await route.fulfill({ status: 404, json: { error: "No report for this workspace." } });
      return;
    } else if (path.endsWith("/integrations")) json = { integrations: [] };
    else if (path.endsWith("/adapters")) json = { adapters: [] };
    else if (path.endsWith("/agents")) json = { agents: agentDescriptors };
    else if (path.endsWith(`/workspaces/${workspace.id}`)) json = { workspace, brand: null };
    else throw new Error(`Unexpected API request: ${path}`);
    await route.fulfill({ status: path.endsWith("/run") ? 201 : 200, json });
  });
  await page.goto(`/workspaces/${workspace.id}`);

  const strategist = page.getByRole("region", { name: "Campaign proposal (agent)" });
  await expect(strategist).toBeVisible();
  await expect(page.getByText("No campaigns yet.", { exact: true })).toBeVisible();

  await strategist.getByLabel("Focus (optional)").fill("slow-season reminder push");
  await strategist.getByRole("button", { name: "Propose a campaign (agent)" }).click();

  await expect(strategist.getByText(`Proposed: ${campaign.name}`)).toBeVisible();
  await expect(strategist.getByText(`utm: ${campaign.utmCampaign}`)).toBeVisible();
  await expect(strategist.getByText(/Filed as a planned campaign/)).toBeVisible();

  // The proposal is operator-visible in the campaigns panel too, listed by the
  // refreshed campaigns query.
  const campaignsPanel = page.getByRole("region", { name: "Campaigns & leads" });
  await expect(campaignsPanel.getByText(campaign.name, { exact: true })).toBeVisible();
  await expect(campaignsPanel.getByText(`utm: ${campaign.utmCampaign}`)).toBeVisible();
  expect(campaignReads).toBeGreaterThanOrEqual(2);
});
