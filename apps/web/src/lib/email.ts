import { withAuth } from "@/lib/config";
// Outbound email client library. One provider account per workspace: the
// dashboard collects the workspace's own from-address and API key, proves the
// connection with a real test message, sends reviewed lead replies through it,
// and reads back what the provider actually did.
//
// The API key is write-only: it is sent to the server, encrypted there, and
// never returned, so it is never displayed back.

export type EmailConnectionStatus = "unverified" | "connected" | "error";

export interface EmailSettings {
  provider: string;
  fromName: string;
  fromEmail: string;
  notifyEmail: string;
  status: EmailConnectionStatus;
  hasApiKey: boolean;
  /** True once a provider signing secret is stored for delivery callbacks. */
  webhookConfigured: boolean;
  /** The URL path the provider must call, per workspace. */
  webhookPath: string;
  lastVerifiedAt: string | null;
  updatedAt: string;
}

export type EmailSettingsState =
  | { kind: "loading" }
  | { kind: "loaded"; configured: boolean; settings: EmailSettings | null }
  | { kind: "unavailable"; message: string };

export type EmailSettingsInput = {
  fromName?: string;
  fromEmail: string;
  notifyEmail?: string;
  /** Empty means "keep the stored key". */
  apiKey?: string;
};

export type SaveEmailSettingsResult =
  | { kind: "saved"; settings: EmailSettings }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export type TestEmailResult =
  | { kind: "verified"; sentTo: string; providerMessageId: string; verifiedAt: string }
  | { kind: "failed"; message: string }
  | { kind: "unavailable"; message: string };

export type EmailDeliveryKind = "lead_notification" | "lead_reply";
export type EmailDeliveryStatus = "queued" | "sending" | "sent" | "failed";
/** What the provider later reported about a message it accepted. */
export type EmailDeliveryState = "unknown" | "delivered" | "delayed" | "bounced" | "complained";

export interface EmailDelivery {
  id: string;
  kind: EmailDeliveryKind;
  leadId: string | null;
  to: string;
  subject: string;
  status: EmailDeliveryStatus;
  /** Separate from status: "sent" only means the provider accepted it. */
  deliveryState: EmailDeliveryState;
  deliveryStateAt: string | null;
  deliveryDetail: string;
  attempts: number;
  lastError: string;
  providerMessageId: string;
  createdAt: string;
  sentAt: string | null;
}

export type DeliveriesState =
  | { kind: "loading" }
  | { kind: "loaded"; deliveries: EmailDelivery[] }
  | { kind: "unavailable"; message: string };

export type SendLeadReplyResult =
  | { kind: "queued"; delivery: EmailDelivery }
  | { kind: "conflict"; message: string }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export type RetryDeliveryResult =
  | { kind: "queued"; delivery: EmailDelivery }
  | { kind: "conflict"; message: string }
  | { kind: "unavailable"; message: string };

export type AcknowledgeAttentionResult =
  { kind: "acknowledged"; acknowledgedAt: string } | { kind: "unavailable"; message: string };

export type AcknowledgedProblemsState =
  | { kind: "loading" }
  | { kind: "loaded"; acknowledgedAt: string | null; deliveries: EmailDelivery[] }
  | { kind: "unavailable"; message: string };

export type SaveWebhookResult =
  | { kind: "saved"; settings: EmailSettings }
  | { kind: "rejected"; problems: string[] }
  | { kind: "unavailable"; message: string };

export function formatEmailConnectionStatus(status: EmailConnectionStatus): string {
  switch (status) {
    case "connected":
      return "Connected — the provider accepted a test message";
    case "unverified":
      return "Not verified yet — send a test message to confirm it works";
    case "error":
      return "The provider rejected these credentials — save a working API key";
    default:
      return status;
  }
}

export function formatDeliveryKind(kind: EmailDeliveryKind): string {
  return kind === "lead_reply" ? "Lead reply" : "New-lead alert";
}

export function formatDeliveryStatus(status: EmailDeliveryStatus): string {
  switch (status) {
    case "queued":
      return "Queued";
    case "sending":
      return "Sending";
    case "sent":
      return "Sent";
    case "failed":
      return "Not sent";
    default:
      return status;
  }
}

export function formatDeliveryState(state: EmailDeliveryState): string {
  switch (state) {
    case "delivered":
      return "Delivered";
    case "delayed":
      return "Delayed";
    case "bounced":
      return "Bounced";
    case "complained":
      return "Marked as spam";
    case "unknown":
    default:
      return "No delivery report yet";
  }
}

// The provider shows its signing secret as "whsec_<base64>". Requiring that
// shape catches a pasted URL or the API key before it silently breaks every
// callback.
export function looksLikeWebhookSecret(value: string): boolean {
  const secret = value.trim().replace(/^whsec_/, "");
  if (secret === "" || secret.length > 256) return false;
  return /^[A-Za-z0-9+/]+={0,2}$/.test(secret);
}

export function validateWebhookSecret(value: string): string[] {
  const trimmed = value.trim();
  if (trimmed === "") return ["paste the signing secret from the provider's webhook page"];
  if (!looksLikeWebhookSecret(trimmed)) {
    return ["that does not look like a signing secret (expected whsec_…)"];
  }
  return [];
}

const EMAIL_PATTERN = /^[^\s@<>,;"']+@[^\s@<>,;"']+\.[^\s@<>,;"']+$/;

export function looksLikeEmailAddress(value: string): boolean {
  const trimmed = value.trim();
  return trimmed.length <= 320 && EMAIL_PATTERN.test(trimmed);
}

// validateEmailSettingsInput mirrors the server's checks so an obvious mistake
// is caught before a round trip. The server stays the authority.
export function validateEmailSettingsInput(
  input: EmailSettingsInput,
  options: { hasStoredKey: boolean },
): string[] {
  const problems: string[] = [];
  const fromEmail = (input.fromEmail ?? "").trim();
  if (!looksLikeEmailAddress(fromEmail)) {
    problems.push("from email must be one valid email address");
  }
  const notifyEmail = (input.notifyEmail ?? "").trim();
  if (notifyEmail !== "" && !looksLikeEmailAddress(notifyEmail)) {
    problems.push("notification email must be one valid email address");
  }
  if (Array.from((input.fromName ?? "").trim()).length > 120) {
    problems.push("from name must be 120 characters or fewer");
  }
  const apiKey = (input.apiKey ?? "").trim();
  if (apiKey !== "" && (apiKey.length < 8 || apiKey.length > 256)) {
    problems.push("API key looks wrong (8 to 256 characters expected)");
  }
  if (apiKey === "" && !options.hasStoredKey) {
    problems.push("an API key is required to connect email");
  }
  return problems;
}

function apiRoot(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, "");
}

function workspacePath(baseUrl: string, workspaceId: string, suffix: string): string {
  return `${apiRoot(baseUrl)}/api/v1/workspaces/${encodeURIComponent(workspaceId)}/${suffix}`;
}

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

function parseSettings(raw: unknown): EmailSettings | null {
  if (!raw || typeof raw !== "object") return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.fromEmail !== "string" || typeof obj.status !== "string") return null;
  if (obj.status !== "unverified" && obj.status !== "connected" && obj.status !== "error") {
    return null;
  }
  return {
    provider: typeof obj.provider === "string" ? obj.provider : "resend",
    fromName: typeof obj.fromName === "string" ? obj.fromName : "",
    fromEmail: obj.fromEmail,
    notifyEmail: typeof obj.notifyEmail === "string" ? obj.notifyEmail : "",
    status: obj.status,
    hasApiKey: obj.hasApiKey === true,
    webhookConfigured: obj.webhookConfigured === true,
    webhookPath: typeof obj.webhookPath === "string" ? obj.webhookPath : "",
    lastVerifiedAt: typeof obj.lastVerifiedAt === "string" ? obj.lastVerifiedAt : null,
    updatedAt: typeof obj.updatedAt === "string" ? obj.updatedAt : "",
  };
}

function parseDelivery(raw: unknown): EmailDelivery | null {
  if (!raw || typeof raw !== "object") return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.id !== "string" || obj.id === "") return null;
  if (obj.kind !== "lead_notification" && obj.kind !== "lead_reply") return null;
  if (
    obj.status !== "queued" &&
    obj.status !== "sending" &&
    obj.status !== "sent" &&
    obj.status !== "failed"
  ) {
    return null;
  }
  const state = obj.deliveryState;
  const deliveryState: EmailDeliveryState =
    state === "delivered" ||
    state === "delayed" ||
    state === "bounced" ||
    state === "complained" ||
    state === "unknown"
      ? state
      : "unknown";
  return {
    id: obj.id,
    kind: obj.kind,
    leadId: typeof obj.leadId === "string" ? obj.leadId : null,
    to: typeof obj.to === "string" ? obj.to : "",
    subject: typeof obj.subject === "string" ? obj.subject : "",
    status: obj.status,
    deliveryState,
    deliveryStateAt: typeof obj.deliveryStateAt === "string" ? obj.deliveryStateAt : null,
    deliveryDetail: typeof obj.deliveryDetail === "string" ? obj.deliveryDetail : "",
    attempts: typeof obj.attempts === "number" ? obj.attempts : 0,
    lastError: typeof obj.lastError === "string" ? obj.lastError : "",
    providerMessageId: typeof obj.providerMessageId === "string" ? obj.providerMessageId : "",
    createdAt: typeof obj.createdAt === "string" ? obj.createdAt : "",
    sentAt: typeof obj.sentAt === "string" ? obj.sentAt : null,
  };
}

// fetchEmailSettings reads the workspace's connection. "configured: false" is
// a normal state (nothing connected yet), not an error.
export async function fetchEmailSettings(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<EmailSettingsState> {
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-settings"), {
      headers: withAuth({ Accept: "application/json" }),
    });
    if (!res.ok) {
      const detail = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: detail?.error ?? `The API answered with status ${res.status}.`,
      };
    }
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (!body) {
      return { kind: "unavailable", message: "The email settings response was empty." };
    }
    if (body.configured !== true) {
      return { kind: "loaded", configured: false, settings: null };
    }
    const settings = parseSettings(body.settings);
    if (!settings) {
      return {
        kind: "unavailable",
        message: "The email settings response had an unexpected shape.",
      };
    }
    return { kind: "loaded", configured: true, settings };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Could not reach the API.") };
  }
}

export async function saveEmailSettings(
  baseUrl: string,
  workspaceId: string,
  input: EmailSettingsInput,
  fetchImpl: typeof fetch = fetch,
): Promise<SaveEmailSettingsResult> {
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-settings"), {
      method: "PUT",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify({
        fromName: (input.fromName ?? "").trim(),
        fromEmail: input.fromEmail.trim(),
        notifyEmail: (input.notifyEmail ?? "").trim(),
        ...(input.apiKey && input.apiKey.trim() !== "" ? { apiKey: input.apiKey.trim() } : {}),
      }),
    });
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (res.status === 400) {
      const problems = Array.isArray(body?.problems)
        ? (body?.problems as unknown[]).filter((p): p is string => typeof p === "string")
        : [];
      return {
        kind: "rejected",
        problems:
          problems.length > 0
            ? problems
            : [typeof body?.error === "string" ? body.error : "The email settings were refused."],
      };
    }
    if (!res.ok) {
      return {
        kind: "unavailable",
        message:
          typeof body?.error === "string"
            ? body.error
            : `The server answered with status ${res.status}.`,
      };
    }
    const settings = parseSettings(body?.settings);
    if (!settings) {
      return { kind: "unavailable", message: "The saved settings were not readable." };
    }
    return { kind: "saved", settings };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

export async function disconnectEmail(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<{ kind: "disconnected" } | { kind: "unavailable"; message: string }> {
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-settings"), {
      method: "DELETE",
      headers: withAuth({ Accept: "application/json" }),
    });
    if (res.status === 204 || res.ok) return { kind: "disconnected" };
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return {
      kind: "unavailable",
      message: body?.error ?? `The server answered with status ${res.status}.`,
    };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

// canRetryDelivery reports whether a logged message never reached the person:
// the send failed, or the provider later reported a bounce. A delivered or
// still-moving message is not a candidate, and new-lead alerts are sent once per
// enquiry by design.
export function canRetryDelivery(delivery: EmailDelivery): boolean {
  if (delivery.kind !== "lead_reply") return false;
  if (delivery.status === "failed") return true;
  return delivery.status === "sent" && delivery.deliveryState === "bounced";
}

// fetchAcknowledgedEmailProblems reads the failed sends and bounces an operator
// has already dismissed, so the workspace email panel can list what the badge
// no longer shows instead of only remembering a timestamp.
export async function fetchAcknowledgedEmailProblems(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<AcknowledgedProblemsState> {
  try {
    const res = await fetchImpl(
      workspacePath(baseUrl, workspaceId, "email-deliveries/acknowledged?limit=25"),
      { headers: withAuth({ Accept: "application/json" }) },
    );
    if (!res.ok) {
      const detail = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: detail?.error ?? `The API answered with status ${res.status}.`,
      };
    }
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (!body || !Array.isArray(body.deliveries)) {
      return { kind: "unavailable", message: "The dismissed problems had an unexpected shape." };
    }
    const acknowledgedAt =
      typeof body.acknowledgedAt === "string" && body.acknowledgedAt !== ""
        ? body.acknowledgedAt
        : null;
    const deliveries: EmailDelivery[] = [];
    for (const raw of body.deliveries) {
      const parsed = parseDelivery(raw);
      if (parsed) deliveries.push(parsed);
    }
    return { kind: "loaded", acknowledgedAt, deliveries };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Could not reach the API.") };
  }
}

// acknowledgeDeliveryAttention marks a workspace's current failed and bounced
// messages as seen, which clears the overview badge for them. It is not a
// deletion: the log keeps every message, and a problem recorded after the
// acknowledgement raises the badge again.
export async function acknowledgeDeliveryAttention(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<AcknowledgeAttentionResult> {
  try {
    const res = await fetchImpl(
      workspacePath(baseUrl, workspaceId, "delivery-attention/acknowledge"),
      {
        method: "POST",
        headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
        body: "{}",
      },
    );
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (!res.ok) {
      return {
        kind: "unavailable",
        message:
          typeof body?.error === "string"
            ? body.error
            : `The server answered with status ${res.status}.`,
      };
    }
    if (typeof body?.acknowledgedAt !== "string" || body.acknowledgedAt === "") {
      return { kind: "unavailable", message: "The acknowledgement was not readable." };
    }
    return { kind: "acknowledged", acknowledgedAt: body.acknowledgedAt };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

// retryEmailDelivery queues a fresh attempt at a reply that never arrived. The
// server resends the exact text that is logged and re-checks that the lead has
// no other live reply, so this cannot email a person twice.
export async function retryEmailDelivery(
  baseUrl: string,
  workspaceId: string,
  deliveryId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<RetryDeliveryResult> {
  const id = deliveryId.trim();
  if (id === "") {
    return { kind: "conflict", message: "pick a failed delivery first" };
  }
  try {
    const res = await fetchImpl(
      workspacePath(baseUrl, workspaceId, `email-deliveries/${encodeURIComponent(id)}/retry`),
      {
        method: "POST",
        headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
        body: "{}",
      },
    );
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (res.status === 409) {
      return {
        kind: "conflict",
        message: typeof body?.error === "string" ? body.error : "The retry was refused.",
      };
    }
    if (!res.ok) {
      return {
        kind: "unavailable",
        message:
          typeof body?.error === "string"
            ? body.error
            : `The server answered with status ${res.status}.`,
      };
    }
    const delivery = parseDelivery(body?.delivery);
    if (!delivery) {
      return { kind: "unavailable", message: "The queued retry was not readable." };
    }
    return { kind: "queued", delivery };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

// saveEmailWebhook stores the signing secret the provider uses for this
// workspace's delivery callbacks. Until it is set, a "sent" message can never
// be confirmed as delivered or bounced — the provider has nobody to tell.
export async function saveEmailWebhook(
  baseUrl: string,
  workspaceId: string,
  secret: string,
  fetchImpl: typeof fetch = fetch,
): Promise<SaveWebhookResult> {
  const problems = validateWebhookSecret(secret);
  if (problems.length > 0) return { kind: "rejected", problems };
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-settings/webhook"), {
      method: "PUT",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: JSON.stringify({ webhookSecret: secret.trim() }),
    });
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (res.status === 400) {
      return {
        kind: "rejected",
        problems: [
          typeof body?.error === "string" ? body.error : "The webhook secret was refused.",
        ],
      };
    }
    if (!res.ok) {
      return {
        kind: "unavailable",
        message:
          typeof body?.error === "string"
            ? body.error
            : `The server answered with status ${res.status}.`,
      };
    }
    const settings = parseSettings(body?.settings);
    if (!settings) {
      return { kind: "unavailable", message: "The saved webhook settings were not readable." };
    }
    return { kind: "saved", settings };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

// clearEmailWebhook forgets the callback secret; delivery states already
// recorded stay as they were.
export async function clearEmailWebhook(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<{ kind: "cleared" } | { kind: "unavailable"; message: string }> {
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-settings/webhook"), {
      method: "DELETE",
      headers: withAuth({ Accept: "application/json" }),
    });
    if (res.ok) return { kind: "cleared" };
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    return {
      kind: "unavailable",
      message: body?.error ?? `The server answered with status ${res.status}.`,
    };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

// sendTestEmail sends one real message. Only the provider's acceptance makes
// the connection verified, so this is the only way to reach "connected".
export async function sendTestEmail(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<TestEmailResult> {
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-settings/test"), {
      method: "POST",
      headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
      body: "{}",
    });
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (res.status === 502) {
      return {
        kind: "failed",
        message:
          typeof body?.error === "string"
            ? body.error
            : "The email provider refused the test message.",
      };
    }
    if (!res.ok) {
      return {
        kind: "unavailable",
        message:
          typeof body?.error === "string"
            ? body.error
            : `The server answered with status ${res.status}.`,
      };
    }
    if (typeof body?.sentTo !== "string" || body?.verified !== true) {
      return { kind: "unavailable", message: "The verification response was not readable." };
    }
    return {
      kind: "verified",
      sentTo: body.sentTo,
      providerMessageId: typeof body.providerMessageId === "string" ? body.providerMessageId : "",
      verifiedAt: typeof body.verifiedAt === "string" ? body.verifiedAt : "",
    };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}

export async function fetchEmailDeliveries(
  baseUrl: string,
  workspaceId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<DeliveriesState> {
  try {
    const res = await fetchImpl(workspacePath(baseUrl, workspaceId, "email-deliveries?limit=25"), {
      headers: withAuth({ Accept: "application/json" }),
    });
    if (!res.ok) {
      const detail = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: detail?.error ?? `The API answered with status ${res.status}.`,
      };
    }
    const body = (await res.json().catch(() => null)) as { deliveries?: unknown } | null;
    if (!body || !Array.isArray(body.deliveries)) {
      return { kind: "unavailable", message: "The delivery log had an unexpected shape." };
    }
    const deliveries: EmailDelivery[] = [];
    for (const raw of body.deliveries) {
      const parsed = parseDelivery(raw);
      if (parsed) deliveries.push(parsed);
    }
    return { kind: "loaded", deliveries };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Could not reach the API.") };
  }
}

// fetchLeadEmailDeliveries reads every message the platform tried to send about
// one lead — the new-enquiry alert and each reply attempt — so one enquiry's
// contact history reads together instead of scattered through the workspace
// log.
export async function fetchLeadEmailDeliveries(
  baseUrl: string,
  workspaceId: string,
  leadId: string,
  fetchImpl: typeof fetch = fetch,
): Promise<DeliveriesState> {
  const id = leadId.trim();
  if (id === "") return { kind: "loaded", deliveries: [] };
  try {
    const res = await fetchImpl(
      workspacePath(
        baseUrl,
        workspaceId,
        `leads/${encodeURIComponent(id)}/email-deliveries?limit=25`,
      ),
      { headers: withAuth({ Accept: "application/json" }) },
    );
    if (!res.ok) {
      const detail = (await res.json().catch(() => null)) as { error?: string } | null;
      return {
        kind: "unavailable",
        message: detail?.error ?? `The API answered with status ${res.status}.`,
      };
    }
    const body = (await res.json().catch(() => null)) as { deliveries?: unknown } | null;
    if (!body || !Array.isArray(body.deliveries)) {
      return { kind: "unavailable", message: "The send history had an unexpected shape." };
    }
    const deliveries: EmailDelivery[] = [];
    for (const raw of body.deliveries) {
      const parsed = parseDelivery(raw);
      if (parsed) deliveries.push(parsed);
    }
    return { kind: "loaded", deliveries };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Could not reach the API.") };
  }
}

// sendLeadReply queues the reviewed reply. The server stores exactly this text
// on the lead and sends it through the workspace's own account; a lead cannot
// receive two live replies, so a duplicate click is a conflict.
export async function sendLeadReply(
  baseUrl: string,
  workspaceId: string,
  leadId: string,
  input: { subject?: string; body?: string } = {},
  fetchImpl: typeof fetch = fetch,
): Promise<SendLeadReplyResult> {
  const id = leadId.trim();
  if (id === "") {
    return { kind: "rejected", problems: ["pick a lead first"] };
  }
  try {
    const res = await fetchImpl(
      workspacePath(baseUrl, workspaceId, `leads/${encodeURIComponent(id)}/reply-send`),
      {
        method: "POST",
        headers: withAuth({ "Content-Type": "application/json", Accept: "application/json" }),
        body: JSON.stringify({
          ...(input.subject ? { subject: input.subject } : {}),
          ...(input.body ? { body: input.body } : {}),
        }),
      },
    );
    const body = (await res.json().catch(() => null)) as Record<string, unknown> | null;
    if (res.status === 409 || res.status === 400) {
      const problems = Array.isArray(body?.problems)
        ? (body?.problems as unknown[]).filter((p): p is string => typeof p === "string")
        : [];
      const message =
        typeof body?.error === "string" ? body.error : "The reply could not be queued.";
      return problems.length > 0 ? { kind: "rejected", problems } : { kind: "conflict", message };
    }
    if (!res.ok) {
      return {
        kind: "unavailable",
        message:
          typeof body?.error === "string"
            ? body.error
            : `The server answered with status ${res.status}.`,
      };
    }
    const delivery = parseDelivery(body?.delivery);
    if (!delivery) {
      return { kind: "unavailable", message: "The queued delivery was not readable." };
    }
    return { kind: "queued", delivery };
  } catch (err) {
    return { kind: "unavailable", message: messageOf(err, "Network request failed.") };
  }
}
