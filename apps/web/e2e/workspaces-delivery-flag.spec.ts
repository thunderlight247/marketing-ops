import { expect, test } from "@playwright/test";

test("the workspaces list flags failing and bouncing email, and links to the log", async ({
  page,
}) => {
  const now = "2026-10-31T09:00:00Z";
  const problem = "11111111-1111-1111-1111-111111111111";
  const clean = "22222222-2222-2222-2222-222222222222";

  await page.route("**/api/v1/workspaces", async (route) => {
    expect(route.request().method()).toBe("GET");
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        workspaces: [
          {
            id: problem,
            slug: "rosewood-heating",
            name: "Rosewood Heating",
            websiteUrl: "https://rosewood.example",
            status: "active",
            createdAt: now,
            updatedAt: now,
            deliveryAttention: { failed: 1, bounced: 3 },
            // Problems from before this moment were dismissed; the flag covers
            // only what happened after it.
            deliveryAcknowledgedAt: "2026-10-30T09:00:00Z",
          },
          {
            id: clean,
            slug: "quiet-co",
            name: "Quiet Co",
            websiteUrl: null,
            status: "active",
            createdAt: now,
            updatedAt: now,
          },
        ],
      }),
    });
  });

  await page.goto("/workspaces");

  const flag = page.getByTestId(`workspace-delivery-attention-${problem}`);
  await expect(flag).toBeVisible();
  await expect(flag).toContainText("Email needs attention");
  await expect(flag).toContainText("1 failed to send");
  await expect(flag).toContainText("3 bounced");
  await expect(flag).toHaveAttribute(
    "href",
    `/workspaces/${problem}/email#email-deliveries`,
  );
  // The acknowledgement tells the operator what the flag covers.
  const acknowledged = page.getByTestId(`workspace-delivery-acknowledged-${problem}`);
  await expect(acknowledged).toContainText("Last acknowledged");
  await expect(acknowledged).toContainText("the flag shows what failed or bounced since");
  // A workspace whose email is fine stays clean here too.
  await expect(page.getByTestId(`workspace-delivery-attention-${clean}`)).toHaveCount(0);
  await expect(page.getByTestId(`workspace-delivery-acknowledged-${clean}`)).toHaveCount(0);
});
