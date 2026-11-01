import { expect, test } from "@playwright/test";

test("channel-copy writer files per-channel copy into a draft item's version", async ({ page }) => {
  const now = "2026-10-26T12:00:00Z";
  const workspace = {
    id: "copy-writer-workspace",
    slug: "copy-writer-demo",
    name: "Copy writer demo",
    websiteUrl: null,
    status: "active",
    createdAt: now,
    updatedAt: now,
  };
  const draftItem = {
    id: "copy-item-1",
    workspaceId: workspace.id,
    kind: "social_post",
    status: "draft",
    title: "Winter service post",
    slug: null,
    plannedPublishAt: null,
    createdAt: now,
    updatedAt: now,
  };
  const version = {
    id: "copy-version-2",
    contentItemId: draftItem.id,
    versionNumber: 2,
    title: draftItem.title,
    excerpt: null,
    body: "Booking is now open for November furnace checks.",
    callToAction: null,
    channelCopy: {
      facebook: "Open all winter. Book November service now.",
      x: "Winter service. Book today.",
      youtube: "Existing channel description.",
    },
    createdAt: now,
  };
  let ran = false;
  const agentDescriptors = [
    {
      id: "channel_copy_writer",
      version: "1",
      displayName: "Channel copy writer",
      description: "Drafts per-channel copy variants for an existing draft content item.",
      outputContract: "channel_copy",
      dataNeeds: [],
      actions: ["file_channel_copy"],
    },
  ];

  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let json: unknown;
    if (path.endsWith("/agents/channel_copy_writer/run")) {
      expect(route.request().method()).toBe("POST");
      expect(route.request().postDataJSON()).toEqual({
        contentItemId: draftItem.id,
        focus: "emphasize the booking deadline",
      });
      ran = true;
      json = {
        item: {
          id: draftItem.id,
          kind: draftItem.kind,
          status: draftItem.status,
          title: draftItem.title,
        },
        version: { versionNumber: version.versionNumber },
        provider: "stub",
        model: "stub-v1",
        agentId: "channel_copy_writer",
        channels: {
          facebook: "Open all winter. Book November service now.",
          x: "Winter service. Book today.",
        },
        durationMs: 10,
      };
    } else if (path.endsWith("/agents")) json = { agents: agentDescriptors };
    else if (path.endsWith("/content")) json = { items: [draftItem] };
    else if (path.endsWith("/campaigns")) json = { campaigns: [] };
    else if (path.endsWith("/leads")) json = { leads: [] };
    else if (path.endsWith("/lead-connections")) json = { connections: [] };
    else if (path.endsWith("/email-settings")) json = { configured: false, settings: null };
    else if (path.endsWith("/email-deliveries/acknowledged"))
      json = { acknowledgedAt: null, deliveries: [] };
    else if (path.endsWith("/email-deliveries")) json = { deliveries: [] };
    else if (path.endsWith("/reports")) {
      await route.fulfill({ status: 404, json: { error: "No report for this workspace." } });
      return;
    } else if (path.endsWith("/integrations")) json = { integrations: [] };
    else if (path.endsWith("/adapters")) json = { adapters: [] };
    else if (path.endsWith(`/workspaces/${workspace.id}`)) json = { workspace, brand: null };
    else throw new Error(`Unexpected API request: ${path}`);
    await route.fulfill({ status: path.endsWith("/run") ? 201 : 200, json });
  });
  await page.goto(`/workspaces/${workspace.id}`);

  const section = page.getByRole("region", { name: "Channel copy (agent)" });
  await expect(section).toBeVisible();
  await expect(section.locator("#channel-copy-item")).toBeVisible();

  await section
    .locator("#channel-copy-item")
    .selectOption({ label: `${draftItem.title} (${draftItem.kind})` });
  await section.locator("#channel-copy-focus").fill("emphasize the booking deadline");
  await section.getByRole("button", { name: "Write channel copy (agent)" }).click();

  await expect(section.getByText(`Filed into: ${draftItem.title}`)).toBeVisible();
  await expect(section.getByText("version 2")).toBeVisible();
  await expect(section.getByText("Open all winter. Book November service now.")).toBeVisible();
  await expect(section.getByText("Winter service. Book today.")).toBeVisible();
  await expect(
    section.getByText(/Saved as a new version, merged over previous channel copy/),
  ).toBeVisible();
  expect(ran).toBe(true);
});
