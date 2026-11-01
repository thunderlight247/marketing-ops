import { expect, test } from "@playwright/test";

test("the overview flags failing and bouncing email, and an operator can dismiss it", async ({
  page,
}) => {
  const now = "2026-10-31T09:00:00Z";
  const problem = "11111111-1111-1111-1111-111111111111";
  const clean = "22222222-2222-2222-2222-222222222222";
  let acknowledged = false;

  await page.route("**/api/v1/workspaces", async (route) => {
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
            // The badge disappears once the problems are acknowledged.
            ...(acknowledged ? {} : { deliveryAttention: { failed: 2, bounced: 1 } }),
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
  await page.route("**/api/v1/workspaces/*/delivery-attention/acknowledge", async (route) => {
    expect(route.request().method()).toBe("POST");
    acknowledged = true;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ acknowledgedAt: now }),
    });
  });

  await page.goto("/");

  // The status panel at the top counts the same problems.
  await expect(page.getByText("1 workspace needs attention")).toBeVisible();
  await expect(
    page.getByText("1 of 2 have a send that failed or a message the provider bounced."),
  ).toBeVisible();

  const badge = page.getByTestId(`delivery-attention-${problem}`);
  await expect(badge).toBeVisible();
  await expect(badge).toContainText("2 failed to send");
  await expect(badge).toContainText("1 bounced");
  // The badge takes the operator straight to the log they must act on.
  await expect(badge).toHaveAttribute("href", `/workspaces/${problem}#email-deliveries`);
  // A workspace with a clean log is not flagged.
  await expect(page.getByTestId(`delivery-attention-${clean}`)).toHaveCount(0);

  await page.getByRole("button", { name: /Mark the email problems for Rosewood Heating/ }).click();

  await expect(page.getByTestId(`delivery-attention-${problem}`)).toHaveCount(0);
  await expect(page.getByRole("status")).toContainText(
    "Email problems for Rosewood Heating are marked as seen",
  );
  // Dismissing says the problems were seen, not that email is fixed: the log
  // still holds them, and a new one would raise the badge again.
  await expect(page.getByRole("status")).toContainText(
    "A new failure or bounce will show here again",
  );
  // The status count follows the dismissal instead of staying stale.
  await expect(page.getByText("No delivery problems")).toBeVisible();
  await expect(page.getByText("1 workspace needs attention")).toHaveCount(0);
  // The row says when the acknowledgement happened, so the cleared badge is
  // still explainable.
  const acknowledgedLine = page.getByTestId(`delivery-acknowledged-${problem}`);
  await expect(acknowledgedLine).toContainText("Last acknowledged");
  await expect(acknowledgedLine).toContainText("nothing new has failed or bounced since");
});
