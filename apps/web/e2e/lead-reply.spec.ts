import { expect, test } from "@playwright/test";

test("lead responder drafts a reply, and the operator sends it through workspace email", async ({
  page,
}) => {
  const now = "2026-10-31T09:00:00Z";
  const workspace = {
    id: "reply-workspace",
    slug: "reply-demo",
    name: "Reply demo",
    websiteUrl: null,
    status: "active",
    createdAt: now,
    updatedAt: now,
  };
  const lead = {
    id: "reply-lead",
    workspaceId: workspace.id,
    campaignId: null,
    sourceKind: "form_api",
    sourceDetail: "contact form",
    status: "new",
    contactName: "Jane Doe",
    contactEmail: "jane@example.com",
    contactPhone: null,
    message: "Do you service the north side? I need a furnace check before winter.",
    notes: "",
    utmSource: null,
    utmMedium: null,
    utmCampaign: null,
    followUpAt: null,
    replyDraftSubject: null,
    replyDraftBody: null,
    replyDraftedAt: null,
    replySentAt: null,
    createdAt: now,
    updatedAt: now,
  };
  const subject = "Re: your furnace check enquiry";
  const body =
    "Hi Jane, thanks for the note about the north side. I'll confirm the visit window with you before anything is booked.";
  const delivery = {
    id: "reply-delivery",
    kind: "lead_reply",
    leadId: lead.id,
    to: "jane@example.com",
    subject,
    status: "queued",
    attempts: 0,
    lastError: "",
    providerMessageId: "",
    createdAt: now,
    sentAt: null,
  };
  // A reply the provider refused earlier: it is the row an operator retries.
  const failedDelivery = {
    ...delivery,
    id: "failed-delivery",
    status: "failed",
    attempts: 1,
    lastError: "the email provider rejected this workspace's API key",
    createdAt: now,
  };
  // A reply the provider accepted and the recipient's server then refused:
  // the bounce the operator resends.
  const bouncedDelivery = {
    ...delivery,
    id: "bounced-delivery",
    status: "sent",
    deliveryState: "bounced",
    deliveryStateAt: now,
    deliveryDetail: "mailbox full",
    createdAt: now,
  };
  let drafted = false;
  let sent = false;
  let retried = false;
  let resent = false;
  const agentDescriptors = [
    {
      id: "lead_responder",
      version: "1",
      displayName: "Lead responder",
      description: "Drafts a first reply to one recorded lead.",
      outputContract: "lead_reply",
      dataNeeds: [],
      actions: ["file_lead_reply"],
    },
  ];

  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    let json: unknown;
    let status = 200;
    if (path.endsWith("/agents/lead_responder/run")) {
      expect(method).toBe("POST");
      expect(route.request().postDataJSON()).toEqual({
        leadId: lead.id,
        focus: "mention we answer within one working day",
      });
      drafted = true;
      status = 201;
      json = {
        lead: { ...lead, replyDraftSubject: subject, replyDraftBody: body, replyDraftedAt: now },
        subject,
        body,
        provider: "stub",
        model: "stub-v1",
        agentId: "lead_responder",
        durationMs: 12,
      };
    } else if (path.endsWith(`/leads/${lead.id}/reply-send`)) {
      expect(method).toBe("POST");
      // The operator's own edited text is what gets sent and recorded.
      expect(route.request().postDataJSON()).toEqual({ subject, body });
      sent = true;
      status = 202;
      json = { delivery };
    } else if (path.endsWith("/email-settings")) {
      json = {
        configured: true,
        settings: {
          provider: "resend",
          fromName: "Rosewood Heating",
          fromEmail: "hello@rosewood.example",
          notifyEmail: "team@rosewood.example",
          status: "connected",
          hasApiKey: true,
          webhookConfigured: false,
          webhookPath: `/api/v1/email/webhooks/resend/${workspace.id}`,
          lastVerifiedAt: now,
          updatedAt: now,
        },
      };
    } else if (path.endsWith(`/email-deliveries/${failedDelivery.id}/retry`)) {
      expect(method).toBe("POST");
      retried = true;
      status = 202;
      json = { delivery: { ...failedDelivery, id: "retried-delivery", status: "queued" } };
    } else if (path.endsWith(`/email-deliveries/${bouncedDelivery.id}/retry`)) {
      expect(method).toBe("POST");
      resent = true;
      status = 202;
      json = {
        delivery: {
          ...bouncedDelivery,
          id: "resent-delivery",
          status: "queued",
          deliveryState: "unknown",
          deliveryStateAt: null,
          deliveryDetail: "",
        },
      };
    } else if (path.endsWith("/email-deliveries/acknowledged")) {
      // The workspace was acknowledged after the bounce, so that message is
      // what the dismissal covered and the panel still lists it.
      json = { acknowledgedAt: now, deliveries: [bouncedDelivery] };
    } else if (path.endsWith("/email-deliveries")) {
      json = {
        deliveries: [
          ...(sent ? [delivery] : []),
          ...(retried ? [] : [failedDelivery]),
          ...(resent ? [] : [bouncedDelivery]),
        ],
      };
    } else if (path.endsWith(`/leads/${lead.id}/email-deliveries`)) {
      json = { deliveries: [delivery, bouncedDelivery, failedDelivery] };
    } else if (path.endsWith("/leads")) {
      json = {
        leads: [
          {
            ...lead,
            ...(drafted ? { replyDraftSubject: subject, replyDraftBody: body } : {}),
            ...(sent ? { replySentAt: now, status: "contacted" } : {}),
          },
        ],
      };
    } else if (path.endsWith("/campaigns")) json = { campaigns: [] };
    else if (path.endsWith("/lead-connections")) json = { connections: [] };
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
    await route.fulfill({ status, json });
  });

  await page.goto(`/workspaces/${workspace.id}`);

  const section = page.getByRole("region", { name: "Lead replies (agent + email)" });
  await expect(section).toBeVisible();
  await expect(section.getByText("The lead responder drafts a first reply")).toBeVisible();
  // The workspace's own sending identity is shown before anything is sent.
  await expect(section.getByText("hello@rosewood.example")).toBeVisible();

  await section.getByLabel("Lead").selectOption(lead.id);
  // The picker shows the real enquiry the draft will answer.
  await expect(section.getByText("Do you service the north side?")).toBeVisible();

  // Every attempt for this one enquiry sits with the enquiry: the queued
  // reply, the bounce and the failed send, not scattered through the log.
  await expect(section.getByText("Sending history for this enquiry")).toBeVisible();
  await expect(section.getByText("Bounced", { exact: true })).toBeVisible();
  await expect(section.getByText("mailbox full")).toBeVisible();
  await expect(section.getByText("Not sent", { exact: true })).toBeVisible();

  await section.getByLabel("Focus (optional)").fill("mention we answer within one working day");
  await section.getByRole("button", { name: "Draft a reply (agent)" }).click();

  // The draft is reviewable text stored on the lead, with the status made
  // explicit: the agent itself sent nothing.
  await expect(section.getByText("Draft stored on the lead")).toBeVisible();
  await expect(section.getByText("status unchanged: new")).toBeVisible();
  await expect(section.locator("#lead-reply-subject")).toHaveValue(subject);
  await expect(section.locator("#lead-reply-body")).toHaveValue(body);
  await expect(section.getByText("Nothing is sent until you press send")).toBeVisible();

  // A person sends it: the request carries the reviewed text, the panel says
  // it is queued rather than sent, and the delivery log owns the real outcome.
  await section.getByRole("button", { name: "Send reply to jane@example.com" }).click();
  await expect(section.getByText("Queued to jane@example.com")).toBeVisible();
  await expect(section.getByText("The email worker sends it within seconds")).toBeVisible();

  // The lead now records the contact the send made, and the campaigns panel
  // shows it instead of a pending draft.
  const campaigns = page.getByRole("region", { name: "Campaigns & leads" });
  await expect(campaigns.getByText("reply sent 2026-10-31")).toBeVisible();
  await expect(section.getByText("A reply was sent to this lead on")).toBeVisible();

  // The email panel lists the queued message with its workspace-scoped target.
  const emailPanel = page.getByRole("region", { name: "Email sending (per workspace)" });
  await expect(emailPanel).toBeVisible();
  // Delivery reports are per workspace: the panel shows this client's own
  // callback URL, so the operator can paste the right one into the provider.
  await expect(emailPanel.getByText("Delivery reports (provider webhook)")).toBeVisible();
  await expect(emailPanel.getByTestId("email-webhook-url")).toHaveText(
    `/api/v1/email/webhooks/resend/${workspace.id}`,
  );
  await emailPanel.getByRole("button", { name: "Refresh" }).click();
  // Scope the log assertions to the log: the dismissed list below repeats the
  // same dates and statuses by design.
  const log = emailPanel.locator("#email-deliveries");
  await expect(log.getByText("Lead reply").first()).toBeVisible();
  await expect(log.getByText("Queued", { exact: true })).toBeVisible();

  // A failed reply can be retried from the log; the button sends the logged
  // delivery's id and the panel confirms the fresh queue, not a send.
  await expect(log.getByText("the email provider rejected")).toBeVisible();
  await log.getByRole("button", { name: "Retry send" }).click();
  await expect(emailPanel.getByText("Retry queued to jane@example.com")).toBeVisible();
  expect(retried).toBe(true);

  // A bounced reply never reached the person, so it can be resent from the log
  // too — with the bounce still visible as the record of what happened.
  await expect(log.getByText("Bounced", { exact: true })).toBeVisible();
  await expect(log.getByText("mailbox full")).toBeVisible();
  await log.getByRole("button", { name: "Resend" }).click();
  await expect(emailPanel.getByText("Resend queued to jane@example.com")).toBeVisible();
  expect(resent).toBe(true);

  // The dismissed problems are listed, not just remembered as a timestamp, so
  // an operator can see which message the cleared badge covered.
  const dismissed = emailPanel.getByTestId("acknowledged-problems");
  await expect(dismissed).toContainText("Problems you dismissed");
  await expect(dismissed).toContainText("Last acknowledged");
  await expect(dismissed.getByText("mailbox full")).toBeVisible();
  await expect(dismissed.getByText("jane@example.com")).toBeVisible();
});
