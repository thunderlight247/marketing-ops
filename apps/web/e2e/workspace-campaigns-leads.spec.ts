import { expect, test, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";

const apiBaseURL = process.env.PLAYWRIGHT_API_BASE_URL ?? "http://localhost:8081";

type Workspace = { id: string; slug: string; name: string };
type Campaign = { id: string; name: string; status: string };
type Lead = {
  id: string;
  contactName: string | null;
  campaignId: string | null;
  sourceKind: string;
  sourceDetail: string;
  status: string;
  followUpAt: string | null;
};

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseURL}/api/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    throw new Error(
      `API fixture request failed: ${init?.method ?? "GET"} ${path} → ${response.status}`,
    );
  }
  return (await response.json()) as T;
}

async function clickLeadStatus(page: Page, contactName: string, status: string) {
  const row = page.locator("li").filter({ hasText: contactName });
  await row.getByRole("button", { name: status, exact: true }).click();
}

test("campaigns and leads can be created, linked, listed, and moved through statuses", async ({
  page,
}) => {
  test.setTimeout(60_000);
  const suffix = randomUUID().slice(0, 12);
  const workspaceSlug = `pw-m11-${suffix}`;
  const workspaceName = `Playwright M11 ${suffix}`;
  const campaignName = `E2E campaign ${suffix}`;
  const contactName = `E2E lead ${suffix}`;
  const contactEmail = `pw-m11-${suffix}@example.invalid`;
  const followUpDate = new Date(Date.now() + 10 * 24 * 60 * 60 * 1000).toISOString();
  let workspace: Workspace | null = null;
  let otherWorkspace: Workspace | null = null;

  try {
    // Create two isolated workspaces through the API; never add fixtures to a
    // demo/client workspace or require direct database credentials in tests.
    workspace = await api<Workspace>("/workspaces", {
      method: "POST",
      body: JSON.stringify({ slug: workspaceSlug, name: workspaceName }),
    });
    otherWorkspace = await api<Workspace>("/workspaces", {
      method: "POST",
      body: JSON.stringify({ slug: `${workspaceSlug}-other`, name: `${workspaceName} other` }),
    });
    expect(workspace.id).toMatch(/^[0-9a-f-]{36}$/i);
    expect(otherWorkspace.id).not.toBe(workspace.id);

    await page.goto(`/workspaces/${workspace.id}`);
    await expect(page.getByRole("heading", { name: "Workspace", exact: true })).toBeVisible();
    await expect(page.getByRole("region", { name: "Workspace details" })).toBeVisible();
    const section = page.getByRole("region", { name: "Campaigns & leads" });
    await expect(section).toBeVisible();
    await expect(section.getByText("No campaigns yet.")).toBeVisible();
    await expect(section.getByText("No leads recorded yet.")).toBeVisible();

    await section.getByRole("button", { name: "New campaign" }).click();
    await section.getByLabel("Campaign name").fill(campaignName);
    await section.getByLabel("UTM campaign value").fill(`utm-${suffix}`);
    await section.getByRole("button", { name: "Create campaign" }).click();

    await expect(section.getByText(campaignName, { exact: true })).toBeVisible();
    await expect(section.getByText(`utm: utm-${suffix}`)).toBeVisible();
    const createdCampaigns = await api<{ campaigns: Campaign[] }>(
      `/workspaces/${workspace.id}/campaigns`,
    );
    expect(createdCampaigns.campaigns).toHaveLength(1);
    const campaignId = createdCampaigns.campaigns[0].id;
    const patchedCampaign = await api<Campaign>(
      `/workspaces/${workspace.id}/campaigns/${campaignId}`,
      { method: "PATCH", body: JSON.stringify({ status: "paused", notes: "E2E pause check" }) },
    );
    expect(patchedCampaign.status).toBe("paused");
    await page.reload();
    const campaignRow = section
      .locator("ul")
      .first()
      .locator("li")
      .filter({ hasText: campaignName });
    await expect(campaignRow).toContainText("Paused");
    await section.getByRole("button", { name: "New campaign" }).click();
    await section.getByLabel("Campaign name").fill(campaignName);
    await section.getByRole("button", { name: "Create campaign" }).click();
    await expect(section.getByRole("alert")).toContainText("already exists in this workspace");
    await section.getByRole("button", { name: "Close campaign form" }).click();

    await section.getByRole("button", { name: "Record a lead" }).click();
    await section.getByLabel("Source", { exact: true }).selectOption("form_api");
    await section.getByLabel("Source detail").fill("Playwright contact form");
    await section.getByLabel("Contact name").fill(contactName);
    await section.getByLabel("Follow up on").fill(followUpDate.slice(0, 10));
    const emailInput = section.getByLabel("Contact email");
    await emailInput.fill("invalid-email");
    expect(
      await emailInput.evaluate((element) => (element as HTMLInputElement).validity.valid),
    ).toBe(false);
    await section.getByRole("button", { name: "Record lead" }).click();
    await expect(section.getByLabel("Contact name")).toHaveValue(contactName);
    await expect(section.locator("li").filter({ hasText: contactName })).toHaveCount(0);
    await emailInput.fill(contactEmail);
    await section.getByLabel("Campaign").selectOption({ label: campaignName });
    await section.getByLabel("Message").fill("Please follow up about the campaign.");
    await section.getByRole("button", { name: "Record lead" }).click();

    const leadRow = section.locator("li").filter({ hasText: contactName });
    await expect(leadRow).toBeVisible();
    await expect(leadRow).toContainText("Form / API");
    await expect(leadRow).toContainText("Source: Playwright contact form");
    await expect(leadRow).toContainText(campaignName);
    await expect(leadRow).toContainText(contactEmail);
    await expect(leadRow).toContainText(`follow-up ${followUpDate.slice(0, 10)}`);

    // Reload proves the source, campaign assignment, and follow-up were saved
    // by the live API rather than only reflected in transient form state.
    await page.reload();
    await expect(section.getByText(campaignName, { exact: true })).toBeVisible();
    await expect(leadRow).toContainText("Source: Playwright contact form");
    await expect(leadRow).toContainText(`follow-up ${followUpDate.slice(0, 10)}`);

    await clickLeadStatus(page, contactName, "Contacted");
    await expect(leadRow.getByText("Contacted", { exact: true }).first()).toBeVisible();
    await clickLeadStatus(page, contactName, "Qualified");
    await expect(leadRow.getByText("Qualified", { exact: true }).first()).toBeVisible();

    const [campaigns, leads] = await Promise.all([
      api<{ campaigns: Campaign[] }>(`/workspaces/${workspace.id}/campaigns`),
      api<{ leads: Lead[] }>(`/workspaces/${workspace.id}/leads`),
    ]);
    expect(campaigns.campaigns).toHaveLength(1);
    expect(campaigns.campaigns[0].name).toBe(campaignName);
    expect(campaigns.campaigns[0].status).toBe("paused");
    expect(leads.leads).toHaveLength(1);
    expect(leads.leads[0].contactName).toBe(contactName);
    expect(leads.leads[0].sourceKind).toBe("form_api");
    expect(leads.leads[0].sourceDetail).toBe("Playwright contact form");
    expect(leads.leads[0].campaignId).toBe(campaigns.campaigns[0].id);
    expect(leads.leads[0].status).toBe("qualified");
    expect(leads.leads[0].followUpAt?.slice(0, 10)).toBe(followUpDate.slice(0, 10));

    // Explicit clear flags remove optional follow-up and campaign values;
    // source attribution remains immutable even if extra fields are sent.
    const clearedLead = await api<Lead>(`/workspaces/${workspace.id}/leads/${leads.leads[0].id}`, {
      method: "PATCH",
      body: JSON.stringify({
        clearFollowUp: true,
        clearCampaign: true,
        sourceKind: "import",
        sourceDetail: "forged source",
      }),
    });
    expect(clearedLead.campaignId).toBeNull();
    expect(clearedLead.followUpAt).toBeNull();
    expect(clearedLead.sourceKind).toBe("form_api");
    expect(clearedLead.sourceDetail).toBe("Playwright contact form");
    await page.goto(`/workspaces/${workspace.id}`);
    await expect(page.getByRole("region", { name: "Workspace details" })).toBeVisible();
    const clearedSection = page.getByRole("region", { name: "Campaigns & leads" });
    const clearedRow = clearedSection.locator("li").filter({ hasText: contactName });
    await expect(clearedRow).toBeVisible();
    await expect(clearedRow).not.toContainText(`follow-up ${followUpDate.slice(0, 10)}`);
    await expect(clearedRow).not.toContainText(campaignName);

    // The same campaign name is valid in another workspace and remains
    // invisible from the first workspace's UI and API.
    const otherCampaign = await api<Campaign>(`/workspaces/${otherWorkspace.id}/campaigns`, {
      method: "POST",
      body: JSON.stringify({ name: campaignName, status: "active" }),
    });
    expect(otherCampaign.id).not.toBe(campaigns.campaigns[0].id);
    const [otherCampaigns, otherLeads] = await Promise.all([
      api<{ campaigns: Campaign[] }>(`/workspaces/${otherWorkspace.id}/campaigns`),
      api<{ leads: Lead[] }>(`/workspaces/${otherWorkspace.id}/leads`),
    ]);
    expect(otherCampaigns.campaigns).toHaveLength(1);
    expect(otherCampaigns.campaigns[0].name).toBe(campaignName);
    expect(otherLeads.leads).toEqual([]);
    await page.goto(`/workspaces/${otherWorkspace.id}`);
    await expect(page.getByRole("region", { name: "Workspace details" })).toBeVisible();
    const otherSection = page.getByRole("region", { name: "Campaigns & leads" });
    await expect(otherSection.getByText(campaignName, { exact: true })).toBeVisible();
    await expect(otherSection.getByText("No leads recorded yet.")).toBeVisible();
    await expect(otherSection.locator("li").filter({ hasText: contactName })).toHaveCount(0);
    const crossWorkspaceLead = await fetch(
      `${apiBaseURL}/api/v1/workspaces/${otherWorkspace.id}/leads`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          campaignId: campaigns.campaigns[0].id,
          sourceKind: "manual",
        }),
      },
    );
    expect(crossWorkspaceLead.status).toBe(400);
  } finally {
    // The confirmed slug is required for deletion; cascade removes only these
    // fixtures' campaigns, leads, and audit events.
    const cleanupErrors: string[] = [];
    for (const fixture of [otherWorkspace, workspace]) {
      if (!fixture) continue;
      try {
        const response = await fetch(`${apiBaseURL}/api/v1/workspaces/${fixture.id}`, {
          method: "DELETE",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ confirmSlug: fixture.slug }),
        });
        if (response.status !== 204) {
          cleanupErrors.push(`workspace ${fixture.id} deletion returned HTTP ${response.status}`);
          continue;
        }
        for (const resource of ["campaigns", "leads"]) {
          const check = await fetch(`${apiBaseURL}/api/v1/workspaces/${fixture.id}/${resource}`);
          if (check.status !== 404) {
            cleanupErrors.push(
              `workspace ${fixture.id} cascade cleanup for ${resource} returned HTTP ${check.status}`,
            );
          }
        }
      } catch (error) {
        cleanupErrors.push(`workspace ${fixture.id} cleanup failed: ${String(error)}`);
      }
    }
    if (cleanupErrors.length > 0) {
      throw new Error(`Playwright fixture cleanup failed: ${cleanupErrors.join("; ")}`);
    }
  }
});
