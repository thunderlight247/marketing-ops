import { expect, test } from "@playwright/test";
import { randomUUID } from "node:crypto";

// Against the live API: the agents panel renders descriptors straight from
// GET /api/v1/agents, so this proves a new specialist appears without any
// dashboard change. All fixtures are removed in the cleanup step.
const apiBaseURL = process.env.PLAYWRIGHT_API_BASE_URL ?? "http://localhost:8081";

test("agents panel lists every registered specialist from the live registry", async ({ page }) => {
  const suffix = randomUUID().slice(0, 12);
  const workspaceSlug = `pw-agents-${suffix}`;
  let workspaceId: string | null = null;

  try {
    const created = await fetch(`${apiBaseURL}/api/v1/workspaces`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ slug: workspaceSlug, name: `Playwright agents ${suffix}` }),
    });
    expect(created.status).toBe(201);
    workspaceId = ((await created.json()) as { id: string }).id;

    await page.goto(`/workspaces/${workspaceId}`);
    const panel = page.getByRole("region", { name: "Enabled agents (registry)" });
    await expect(panel).toBeVisible();

    // The live registry currently enables six built-in specialists; each must
    // appear by its descriptor id chip, proving the panel is registry-driven.
    for (const id of [
      "content_creator",
      "search_gap_writer",
      "seo_specialist",
      "campaign_strategist",
      "channel_copy_writer",
      "lead_responder",
    ]) {
      await expect(panel.getByText(id, { exact: true })).toBeVisible();
    }
    await expect(panel.getByText("Search-gap writer", { exact: true })).toBeVisible();
    // The latest specialist's registry entry renders through the shared
    // labels: its contract and action lines must both be human-readable.
    await expect(
      panel.getByText("Writes per-channel copy into an existing draft item"),
    ).toBeVisible();
    await expect(
      panel.getByText(
        "May file per-channel copy into a draft item's version (never changes status or approval)",
      ),
    ).toBeVisible();
    await expect(
      panel.getByText(
        "May store a drafted reply on a lead (never sends it, never changes the lead's status)",
      ),
    ).toBeVisible();
    // The two Search Console specialists show the formatted data-need line;
    // the description of search_gap_writer also names Search Console, so the
    // count below deliberately anchors on the "Reads:" line instead.
    await expect(panel.getByText(/Reads: Search Console/)).toHaveCount(2);
  } finally {
    if (workspaceId) {
      const removed = await fetch(`${apiBaseURL}/api/v1/workspaces/${workspaceId}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ confirmSlug: workspaceSlug }),
      });
      expect(removed.status).toBe(204);
    }
  }
});
