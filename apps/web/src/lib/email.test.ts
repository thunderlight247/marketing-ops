import { afterEach, describe, expect, it, vi } from "vitest";
import {
  acknowledgeDeliveryAttention,
  clearEmailWebhook,
  disconnectEmail,
  fetchAcknowledgedEmailProblems,
  fetchEmailDeliveries,
  fetchEmailSettings,
  fetchLeadEmailDeliveries,
  formatDeliveryKind,
  formatDeliveryState,
  formatDeliveryStatus,
  formatEmailConnectionStatus,
  canRetryDelivery,
  looksLikeEmailAddress,
  retryEmailDelivery,
  saveEmailSettings,
  saveEmailWebhook,
  sendLeadReply,
  sendTestEmail,
  validateEmailSettingsInput,
  validateWebhookSecret,
  type EmailDelivery,
} from "./email";

function jsonRes(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as unknown as Response;
}

const settings = {
  provider: "resend",
  fromName: "Rosewood Heating",
  fromEmail: "hello@rosewood.example",
  notifyEmail: "team@rosewood.example",
  status: "connected",
  hasApiKey: true,
  lastVerifiedAt: "2026-10-31T09:00:00Z",
  updatedAt: "2026-10-31T09:00:00Z",
};

const delivery: EmailDelivery = {
  id: "delivery-1",
  kind: "lead_reply",
  leadId: "lead-1",
  to: "jane@example.com",
  subject: "Re: furnace check",
  status: "sent",
  deliveryState: "unknown",
  deliveryStateAt: null,
  deliveryDetail: "",
  attempts: 1,
  lastError: "",
  providerMessageId: "msg_1",
  createdAt: "2026-10-31T09:00:00Z",
  sentAt: "2026-10-31T09:00:02Z",
};

describe("email settings", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("reads an unconfigured workspace as a normal state", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonRes(200, { configured: false, settings: null })),
    );
    const state = await fetchEmailSettings("", "ws-1");
    expect(state).toEqual({ kind: "loaded", configured: false, settings: null });
  });

  it("parses a configured connection without the API key", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonRes(200, { configured: true, settings }));
    vi.stubGlobal("fetch", fetchMock);
    const state = await fetchEmailSettings("http://api", "ws-1");
    expect(state.kind).toBe("loaded");
    if (state.kind !== "loaded" || !state.settings) throw new Error("expected settings");
    expect(state.settings.fromEmail).toBe("hello@rosewood.example");
    expect(state.settings.hasApiKey).toBe(true);
    expect(fetchMock.mock.calls[0][0]).toBe("http://api/api/v1/workspaces/ws-1/email-settings");
  });

  it("rejects a settings payload with an unknown status", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonRes(200, { configured: true, settings: { ...settings, status: "fine" } }),
        ),
    );
    const state = await fetchEmailSettings("", "ws-1");
    expect(state.kind).toBe("unavailable");
  });

  it("sends the key only when one was typed", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonRes(200, { configured: true, settings }));
    vi.stubGlobal("fetch", fetchMock);

    await saveEmailSettings("", "ws-1", {
      fromName: "Rosewood Heating",
      fromEmail: " hello@rosewood.example ",
      notifyEmail: "team@rosewood.example",
      apiKey: "re_workspace_key",
    });
    expect(fetchMock.mock.calls[0][1].method).toBe("PUT");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      fromName: "Rosewood Heating",
      fromEmail: "hello@rosewood.example",
      notifyEmail: "team@rosewood.example",
      apiKey: "re_workspace_key",
    });

    await saveEmailSettings("", "ws-1", { fromEmail: "hello@rosewood.example", apiKey: "" });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body).apiKey).toBeUndefined();
  });

  it("surfaces the server's rejection reasons", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonRes(400, {
          error: "the email settings are not valid",
          problems: ["from email must be one valid email address"],
        }),
      ),
    );
    const result = await saveEmailSettings("", "ws-1", { fromEmail: "nope" });
    expect(result).toEqual({
      kind: "rejected",
      problems: ["from email must be one valid email address"],
    });
  });

  it("disconnects", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonRes(204, null)));
    await expect(disconnectEmail("", "ws-1")).resolves.toEqual({ kind: "disconnected" });
  });
});

describe("test send", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("reports provider acceptance with the address that was used", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonRes(200, {
          verified: true,
          sentTo: "team@rosewood.example",
          providerMessageId: "msg_9",
          verifiedAt: "2026-10-31T09:00:00Z",
        }),
      ),
    );
    const result = await sendTestEmail("", "ws-1");
    expect(result).toEqual({
      kind: "verified",
      sentTo: "team@rosewood.example",
      providerMessageId: "msg_9",
      verifiedAt: "2026-10-31T09:00:00Z",
    });
  });

  it("treats a provider refusal as a failed connection, not a crash", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonRes(502, { error: "the email provider rejected this workspace's API key" }),
        ),
    );
    const result = await sendTestEmail("", "ws-1");
    expect(result).toEqual({
      kind: "failed",
      message: "the email provider rejected this workspace's API key",
    });
  });
});

describe("sending a lead reply", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("queues the reply and returns the delivery", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonRes(202, { delivery }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await sendLeadReply("http://api", "ws-1", "lead-1", {
      subject: "Re: furnace check",
      body: "Thursday at 9am works.",
    });
    expect(result).toEqual({
      kind: "queued",
      delivery: expect.objectContaining({ id: "delivery-1" }),
    });
    expect(fetchMock.mock.calls[0][0]).toBe(
      "http://api/api/v1/workspaces/ws-1/leads/lead-1/reply-send",
    );
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      subject: "Re: furnace check",
      body: "Thursday at 9am works.",
    });
  });

  it("reports a duplicate send as a conflict instead of a success", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonRes(409, { error: "a reply to this lead is already queued or was already sent" }),
        ),
    );
    const result = await sendLeadReply("", "ws-1", "lead-1");
    expect(result).toEqual({
      kind: "conflict",
      message: "a reply to this lead is already queued or was already sent",
    });
  });

  it("sends nothing without a lead id", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await sendLeadReply("", "ws-1", "  ");
    expect(result.kind).toBe("rejected");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses an unreadable queued delivery", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonRes(202, { delivery: { id: "x" } })));
    const result = await sendLeadReply("", "ws-1", "lead-1");
    expect(result.kind).toBe("unavailable");
  });
});

describe("delivery log", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("lists deliveries and drops unreadable rows", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonRes(200, { deliveries: [delivery, { id: "broken" }] })),
    );
    const state = await fetchEmailDeliveries("", "ws-1");
    expect(state).toEqual({
      kind: "loaded",
      deliveries: [expect.objectContaining({ id: "delivery-1", status: "sent" })],
    });
  });
});

describe("validation and formatting", () => {
  it("requires an address and a key on first connect", () => {
    const problems = validateEmailSettingsInput({ fromEmail: "" }, { hasStoredKey: false });
    expect(problems).toContain("from email must be one valid email address");
    expect(problems).toContain("an API key is required to connect email");
  });

  it("allows an empty key when one is already stored", () => {
    expect(
      validateEmailSettingsInput(
        { fromEmail: "hello@rosewood.example", notifyEmail: "", apiKey: "" },
        { hasStoredKey: true },
      ),
    ).toEqual([]);
  });

  it("rejects a second address in one field", () => {
    expect(looksLikeEmailAddress("a@example.com, b@example.com")).toBe(false);
    expect(looksLikeEmailAddress("  hello@rosewood.example ")).toBe(true);
    expect(
      validateEmailSettingsInput(
        { fromEmail: "hello@rosewood.example", notifyEmail: "not-an-address" },
        { hasStoredKey: true },
      ),
    ).toContain("notification email must be one valid email address");
  });

  it("labels states for the operator", () => {
    expect(formatEmailConnectionStatus("connected")).toMatch(/accepted a test message/);
    expect(formatEmailConnectionStatus("error")).toMatch(/rejected/);
    expect(formatDeliveryKind("lead_notification")).toBe("New-lead alert");
    expect(formatDeliveryStatus("failed")).toBe("Not sent");
  });
});

describe("email delivery reports", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("reads the webhook state of the connection", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonRes(200, {
          configured: true,
          settings: {
            ...settings,
            webhookConfigured: true,
            webhookPath: "/api/v1/email/webhooks/resend/ws-1",
          },
        }),
      ),
    );
    const state = await fetchEmailSettings("http://api", "ws-1");
    if (state.kind !== "loaded" || !state.settings) throw new Error("expected settings");
    expect(state.settings.webhookConfigured).toBe(true);
    expect(state.settings.webhookPath).toBe("/api/v1/email/webhooks/resend/ws-1");
  });

  it("parses a delivery state the provider reported", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonRes(200, {
          deliveries: [
            {
              ...delivery,
              deliveryState: "bounced",
              deliveryStateAt: "2026-10-31T09:05:00Z",
              deliveryDetail: "mailbox full",
            },
            { ...delivery, id: "delivery-2", deliveryState: "not-a-state" },
          ],
        }),
      ),
    );
    const state = await fetchEmailDeliveries("", "ws-1");
    if (state.kind !== "loaded") throw new Error("expected deliveries");
    expect(state.deliveries[0].deliveryState).toBe("bounced");
    expect(state.deliveries[0].deliveryDetail).toBe("mailbox full");
    expect(state.deliveries[1].deliveryState).toBe("unknown");
  });

  it("describes each delivery state in plain words", () => {
    expect(formatDeliveryState("delivered")).toBe("Delivered");
    expect(formatDeliveryState("bounced")).toBe("Bounced");
    expect(formatDeliveryState("delayed")).toBe("Delayed");
    expect(formatDeliveryState("complained")).toBe("Marked as spam");
    expect(formatDeliveryState("unknown")).toBe("No delivery report yet");
  });

  it("refuses a signing secret that is not shaped like one", () => {
    expect(validateWebhookSecret("")).toHaveLength(1);
    expect(validateWebhookSecret("re_workspace_key")).toHaveLength(1);
    expect(validateWebhookSecret("whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")).toEqual([]);
  });

  it("saves the signing secret to this workspace's endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonRes(200, {
        configured: true,
        settings: { ...settings, webhookConfigured: true, webhookPath: "/p" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const result = await saveEmailWebhook("", "ws-1", " whsec_abcdefgh ");
    expect(result.kind).toBe("saved");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/workspaces/ws-1/email-settings/webhook");
    expect(fetchMock.mock.calls[0][1].method).toBe("PUT");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      webhookSecret: "whsec_abcdefgh",
    });
  });

  it("never sends a secret it already knows is unusable", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await saveEmailWebhook("", "ws-1", "whsec_");
    expect(result.kind).toBe("rejected");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("removes the signing secret", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonRes(200, { configured: true }));
    vi.stubGlobal("fetch", fetchMock);
    expect(await clearEmailWebhook("", "ws-1")).toEqual({ kind: "cleared" });
    expect(fetchMock.mock.calls[0][1].method).toBe("DELETE");
  });
});

describe("email delivery retry", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("queues a retry of a failed reply", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonRes(202, { delivery: { ...delivery, status: "queued" } }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await retryEmailDelivery("", "ws-1", "delivery-1");
    expect(result.kind).toBe("queued");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/workspaces/ws-1/email-deliveries/delivery-1/retry",
    );
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
  });

  it("passes the server's refusal through", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonRes(409, { error: "a reply to this lead is already queued or was already sent" }),
        ),
    );
    const result = await retryEmailDelivery("", "ws-1", "delivery-1");
    expect(result).toEqual({
      kind: "conflict",
      message: "a reply to this lead is already queued or was already sent",
    });
  });

  it("offers a retry only for a reply that never arrived", () => {
    expect(canRetryDelivery({ ...delivery, status: "failed", deliveryState: "unknown" })).toBe(
      true,
    );
    expect(canRetryDelivery({ ...delivery, status: "sent", deliveryState: "bounced" })).toBe(true);
    expect(canRetryDelivery({ ...delivery, status: "sent", deliveryState: "delivered" })).toBe(
      false,
    );
    expect(canRetryDelivery({ ...delivery, status: "sent", deliveryState: "delayed" })).toBe(false);
    expect(canRetryDelivery({ ...delivery, status: "queued" })).toBe(false);
    expect(canRetryDelivery({ ...delivery, kind: "lead_notification", status: "failed" })).toBe(
      false,
    );
  });

  it("refuses to retry without a delivery id", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await retryEmailDelivery("", "ws-1", "  ");
    expect(result.kind).toBe("conflict");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("per-lead send history", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("asks for one lead's messages only", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonRes(200, {
        deliveries: [
          { ...delivery, kind: "lead_notification", status: "sent" },
          { ...delivery, deliveryState: "bounced", deliveryDetail: "mailbox full" },
        ],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const state = await fetchLeadEmailDeliveries("http://api", "ws-1", "lead-1");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "http://api/api/v1/workspaces/ws-1/leads/lead-1/email-deliveries?limit=25",
    );
    if (state.kind !== "loaded") throw new Error("expected history");
    expect(state.deliveries).toHaveLength(2);
    expect(state.deliveries[0].kind).toBe("lead_notification");
    expect(state.deliveries[1].deliveryState).toBe("bounced");
  });

  it("reads nothing for an unpicked lead instead of calling the API", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await fetchLeadEmailDeliveries("", "ws-1", "  ")).toEqual({
      kind: "loaded",
      deliveries: [],
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("surfaces a refused history lookup", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonRes(404, { error: "no such lead" })));
    const state = await fetchLeadEmailDeliveries("", "ws-1", "lead-1");
    expect(state).toEqual({ kind: "unavailable", message: "no such lead" });
  });
});

describe("acknowledging delivery problems", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("posts to this workspace's acknowledgement and returns when it took effect", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonRes(200, { acknowledgedAt: "2026-10-31T10:00:00Z" }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await acknowledgeDeliveryAttention("", "ws-1");
    expect(result).toEqual({ kind: "acknowledged", acknowledgedAt: "2026-10-31T10:00:00Z" });
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/workspaces/ws-1/delivery-attention/acknowledge",
    );
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
  });

  it("passes the server's refusal through", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonRes(404, { error: "no such workspace" })));
    expect(await acknowledgeDeliveryAttention("", "ws-1")).toEqual({
      kind: "unavailable",
      message: "no such workspace",
    });
  });

  it("does not claim success on an unreadable body", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonRes(200, {})));
    const result = await acknowledgeDeliveryAttention("", "ws-1");
    expect(result.kind).toBe("unavailable");
  });

  it("reports an unreachable API instead of throwing", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network down")));
    expect(await acknowledgeDeliveryAttention("", "ws-1")).toEqual({
      kind: "unavailable",
      message: "network down",
    });
  });
});

describe("fetching the dismissed problems", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("returns the acknowledgement time and the messages it covered", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonRes(200, {
        acknowledgedAt: "2026-10-31T10:00:00Z",
        deliveries: [
          {
            ...delivery,
            status: "sent",
            deliveryState: "bounced",
            deliveryDetail: "mailbox full",
          },
        ],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const state = await fetchAcknowledgedEmailProblems("", "ws-1");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/workspaces/ws-1/email-deliveries/acknowledged?limit=25",
    );
    if (state.kind !== "loaded") throw new Error("expected a loaded state");
    expect(state.acknowledgedAt).toBe("2026-10-31T10:00:00Z");
    expect(state.deliveries).toHaveLength(1);
    expect(state.deliveries[0].deliveryDetail).toBe("mailbox full");
  });

  it("treats an unacknowledged workspace as an empty, timestamp-less list", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonRes(200, { acknowledgedAt: null, deliveries: [] })),
    );
    expect(await fetchAcknowledgedEmailProblems("", "ws-1")).toEqual({
      kind: "loaded",
      acknowledgedAt: null,
      deliveries: [],
    });
  });

  it("reports a refusal or an unreadable body instead of inventing a list", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonRes(500, { error: "acknowledged problem listing failed" })),
    );
    expect(await fetchAcknowledgedEmailProblems("", "ws-1")).toEqual({
      kind: "unavailable",
      message: "acknowledged problem listing failed",
    });

    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonRes(200, { acknowledgedAt: "x" })));
    expect((await fetchAcknowledgedEmailProblems("", "ws-1")).kind).toBe("unavailable");
  });

  it("never throws when the API is unreachable", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("socket closed")));
    expect(await fetchAcknowledgedEmailProblems("", "ws-1")).toEqual({
      kind: "unavailable",
      message: "socket closed",
    });
  });
});
