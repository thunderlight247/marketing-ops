import { expect, test } from "@playwright/test";

test("content creator saves a draft, refreshes content and offers human review", async ({
  page,
}) => {
  const now = "2026-10-23T12:00:00Z";
  const workspace = {
    id: "creator-workspace",
    slug: "creator-demo",
    name: "Creator demo",
    websiteUrl: null,
    status: "active",
    createdAt: now,
    updatedAt: now,
  };
  const item = {
    id: "creator-item",
    workspaceId: workspace.id,
    kind: "snippet",
    status: "draft",
    title: "Winter care",
    slug: null,
    plannedPublishAt: null,
    createdAt: now,
    updatedAt: now,
  };
  let created = false;
  let contentReads = 0;
  const agentDescriptors = [
    {
      id: "search_gap_writer",
      version: "1",
      displayName: "Search-gap writer",
      description: "Drafts one piece aimed at a real Search Console demand gap, for review.",
      outputContract: "drafting",
      dataNeeds: ["search_console"],
      actions: ["file_draft"],
    },
    {
      id: "content_creator",
      version: "1",
      displayName: "Content creator",
      description: "Composes original drafts for review.",
      outputContract: "drafting",
      dataNeeds: [],
      actions: ["file_draft"],
    },
    {
      id: "seo_specialist",
      version: "1",
      displayName: "SEO advisor",
      description: "Advisory-only organic search review.",
      outputContract: "advisory",
      dataNeeds: ["search_console"],
      actions: [],
    },
  ];

  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let json: unknown;
    if (path.endsWith("/agents/content_creator/run")) {
      expect(route.request().method()).toBe("POST");
      expect(route.request().postDataJSON()).toEqual({ kind: "snippet", focus: "Winter care" });
      created = true;
      json = {
        item,
        version: {
          versionNumber: 1,
          body: "Review maintenance.",
          excerpt: null,
          callToAction: null,
        },
        provider: "stub",
        model: "stub-v1",
        agentId: "content_creator",
        keyPoints: ["Maintenance"],
        durationMs: 12,
        slugSuggestion: null,
      };
    } else if (path.endsWith("/content")) {
      contentReads += 1;
      json = { items: created ? [item] : [] };
    } else if (path.endsWith("/integrations")) json = { integrations: [] };
    else if (path.endsWith("/campaigns")) json = { campaigns: [] };
    else if (path.endsWith("/leads")) json = { leads: [] };
    else if (path.endsWith("/adapters")) json = { adapters: [] };
    else if (path.endsWith("/agents")) json = { agents: agentDescriptors };
    else if (path.endsWith(`/workspaces/${workspace.id}`)) json = { workspace, brand: null };
    else throw new Error(`Unexpected API request: ${path}`);
    await route.fulfill({ status: path.endsWith("/run") ? 201 : 200, json });
  });
  await page.goto(`/workspaces/${workspace.id}`);
  const creator = page.getByRole("region", { name: "Content creator (agent draft)" });
  await expect(creator).toBeVisible();
  await creator.getByLabel("Content kind").selectOption("snippet");
  await creator.getByLabel("Brief (optional)").fill("Winter care");
  await creator.getByRole("button", { name: "Create agent draft", exact: true }).click();
  await expect(creator.locator("p").filter({ hasText: "Winter care" })).toBeVisible();
  await creator.getByText("Draft body", { exact: true }).click();
  await expect(creator.getByText("Review maintenance.", { exact: true })).toBeVisible();
  await expect(creator.getByRole("link", { name: "Review and approve draft" })).toHaveAttribute(
    "href",
    `/content/${workspace.id}/${item.id}`,
  );
  await expect(page.getByRole("link", { name: "Winter care", exact: true })).toBeVisible();
  expect(contentReads).toBeGreaterThanOrEqual(2);
});
