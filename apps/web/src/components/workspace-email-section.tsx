"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  canRetryDelivery,
  clearEmailWebhook,
  disconnectEmail,
  fetchAcknowledgedEmailProblems,
  fetchEmailDeliveries,
  fetchEmailSettings,
  formatDeliveryKind,
  formatDeliveryState,
  formatDeliveryStatus,
  formatEmailConnectionStatus,
  retryEmailDelivery,
  saveEmailSettings,
  saveEmailWebhook,
  sendTestEmail,
  validateEmailSettingsInput,
  validateWebhookSecret,
  type AcknowledgedProblemsState,
  type DeliveriesState,
  type EmailDelivery,
  type EmailSettings,
  type EmailSettingsState,
} from "@/lib/email";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";
const PRIMARY_BUTTON =
  "rounded-lg bg-indigo-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 transition-colors";
const SECONDARY_BUTTON =
  "rounded-lg border border-slate-300 bg-white px-3.5 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 transition-colors";

function statusTone(status: string): string {
  if (status === "connected" || status === "sent" || status === "delivered") {
    return "bg-emerald-100 text-emerald-800";
  }
  if (
    status === "error" ||
    status === "failed" ||
    status === "bounced" ||
    status === "complained"
  ) {
    return "bg-red-100 text-red-800";
  }
  return "bg-amber-100 text-amber-800";
}

// WorkspaceEmailSection connects this workspace's own email account: its own
// from-address, its own provider API key, and the address that should hear
// about new enquiries. The key is write-only — the server stores it encrypted
// and never returns it. "Connected" is only ever set by a real test message the
// provider accepted, so the badge means something.
export function WorkspaceEmailSection({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const [state, setState] = useState<EmailSettingsState>({ kind: "loading" });
  const [deliveries, setDeliveries] = useState<DeliveriesState>({ kind: "loading" });
  const [acknowledgedProblems, setAcknowledgedProblems] = useState<AcknowledgedProblemsState>({
    kind: "loading",
  });
  const [fromName, setFromName] = useState("");
  const [fromEmail, setFromEmail] = useState("");
  const [notifyEmail, setNotifyEmail] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [busy, setBusy] = useState<null | "save" | "test" | "disconnect" | "webhook">(null);
  const [problems, setProblems] = useState<string[]>([]);
  const [webhookProblems, setWebhookProblems] = useState<string[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [webhookError, setWebhookError] = useState<string | null>(null);
  const [webhookNotice, setWebhookNotice] = useState<string | null>(null);
  const [webhookCopied, setWebhookCopied] = useState(false);
  const [retryingId, setRetryingId] = useState<string | null>(null);

  const apiBaseUrl = getApiBaseUrl();

  const applySettings = useCallback((settings: EmailSettings | null) => {
    setFromName(settings?.fromName ?? "");
    setFromEmail(settings?.fromEmail ?? "");
    setNotifyEmail(settings?.notifyEmail ?? "");
    setApiKey("");
    setWebhookSecret("");
  }, []);

  const loadDeliveries = useCallback(async () => {
    setDeliveries(await fetchEmailDeliveries(apiBaseUrl, workspaceId));
  }, [apiBaseUrl, workspaceId]);

  const loadAcknowledgedProblems = useCallback(async () => {
    setAcknowledgedProblems(await fetchAcknowledgedEmailProblems(apiBaseUrl, workspaceId));
  }, [apiBaseUrl, workspaceId]);

  const refreshLog = useCallback(async () => {
    await Promise.all([loadDeliveries(), loadAcknowledgedProblems()]);
  }, [loadDeliveries, loadAcknowledgedProblems]);

  const load = useCallback(async () => {
    const loaded = await fetchEmailSettings(apiBaseUrl, workspaceId);
    setState(loaded);
    if (loaded.kind === "loaded") {
      applySettings(loaded.settings);
    }
    await refreshLog();
  }, [apiBaseUrl, workspaceId, applySettings, refreshLog]);

  useEffect(() => {
    void load();
  }, [load]);

  const configured = state.kind === "loaded" && state.configured;
  const settings = state.kind === "loaded" ? state.settings : null;

  async function handleSave(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setProblems([]);
    setNotice(null);
    setError(null);
    const found = validateEmailSettingsInput(
      { fromName, fromEmail, notifyEmail, apiKey },
      { hasStoredKey: settings?.hasApiKey === true },
    );
    if (found.length > 0) {
      setProblems(found);
      return;
    }
    setBusy("save");
    const result = await saveEmailSettings(apiBaseUrl, workspaceId, {
      fromName,
      fromEmail,
      notifyEmail,
      apiKey,
    });
    setBusy(null);
    if (result.kind === "saved") {
      setNotice(
        result.settings.status === "connected"
          ? "Settings saved and the connection is still verified."
          : "Settings saved. Send a test message to confirm the account works.",
      );
      await load();
      return;
    }
    if (result.kind === "rejected") {
      setProblems(result.problems);
      return;
    }
    setError(result.message);
  }

  async function handleTest() {
    setBusy("test");
    setNotice(null);
    setError(null);
    const result = await sendTestEmail(apiBaseUrl, workspaceId);
    setBusy(null);
    if (result.kind === "verified") {
      setNotice(`The provider accepted a test message to ${result.sentTo}.`);
      await load();
      return;
    }
    if (result.kind === "failed") {
      setError(result.message);
      await load();
      return;
    }
    setError(result.message);
  }

  async function handleSaveWebhook(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setWebhookProblems([]);
    setWebhookNotice(null);
    setWebhookError(null);
    const found = validateWebhookSecret(webhookSecret);
    if (found.length > 0) {
      setWebhookProblems(found);
      return;
    }
    setBusy("webhook");
    const result = await saveEmailWebhook(apiBaseUrl, workspaceId, webhookSecret);
    setBusy(null);
    if (result.kind === "saved") {
      setWebhookNotice(
        "Signing secret saved. Delivery reports from the provider will now mark these messages delivered or bounced.",
      );
      setWebhookSecret("");
      await load();
      return;
    }
    if (result.kind === "rejected") {
      setWebhookProblems(result.problems);
      return;
    }
    setWebhookError(result.message);
  }

  async function handleClearWebhook() {
    setBusy("webhook");
    setWebhookNotice(null);
    setWebhookError(null);
    const result = await clearEmailWebhook(apiBaseUrl, workspaceId);
    setBusy(null);
    if (result.kind === "cleared") {
      setWebhookNotice(
        "Signing secret removed. Delivery states already recorded stay as they are.",
      );
      await load();
      return;
    }
    setWebhookError(result.message);
  }

  async function handleCopyWebhookUrl(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      setWebhookCopied(true);
    } catch {
      setWebhookCopied(false);
    }
  }

  // A reply that never arrived — a failed send or a bounce — can be tried
  // again from the log. The server resends the exact logged text and refuses
  // when another reply is already live, so this is a convenience, not a way
  // around the send rules.
  async function handleRetry(delivery: EmailDelivery) {
    const label = delivery.deliveryState === "bounced" ? "Resend" : "Retry";
    setRetryingId(delivery.id);
    setNotice(null);
    setError(null);
    const result = await retryEmailDelivery(apiBaseUrl, workspaceId, delivery.id);
    setRetryingId(null);
    if (result.kind === "queued") {
      setNotice(`${label} queued to ${result.delivery.to}. The worker sends it shortly.`);
      await loadDeliveries();
      return;
    }
    if (result.kind === "conflict") {
      setError(result.message);
      await loadDeliveries();
      return;
    }
    setError(result.message);
  }

  async function handleDisconnect() {
    setBusy("disconnect");
    setNotice(null);
    setError(null);
    const result = await disconnectEmail(apiBaseUrl, workspaceId);
    setBusy(null);
    if (result.kind === "disconnected") {
      setNotice("Email disconnected for this workspace. Nothing else was changed.");
      await load();
      return;
    }
    setError(result.message);
  }

  const list = deliveries.kind === "loaded" ? deliveries.deliveries : [];
  const acknowledgedList =
    acknowledgedProblems.kind === "loaded" ? acknowledgedProblems.deliveries : [];
  const acknowledgedAt =
    acknowledgedProblems.kind === "loaded" ? acknowledgedProblems.acknowledgedAt : null;
  const webhookUrl =
    settings?.webhookPath && settings.webhookPath !== ""
      ? `${apiBaseUrl.replace(/\/+$/, "")}${settings.webhookPath}`
      : "";

  return (
    <section
      aria-labelledby="email-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <h2
          id="email-heading"
          className="text-xs font-semibold uppercase tracking-widest text-slate-500"
        >
          Email sending (per workspace)
        </h2>
        {settings ? (
          <span
            className={`rounded px-2 py-0.5 text-[11px] font-semibold ${statusTone(settings.status)}`}
          >
            {settings.status === "connected"
              ? "Connected"
              : settings.status === "error"
                ? "Connection error"
                : "Not verified"}
          </span>
        ) : null}
      </div>
      <div className="px-4 py-5 sm:px-5">
        <p className="max-w-prose text-sm text-slate-600">
          This workspace sends through its own email account, so each client keeps their own sending
          domain and reputation. Use the provider API key for this client&apos;s account: the key is
          encrypted at rest, never shown again, and used only for this workspace&apos;s lead alerts
          and replies.
        </p>
        <details className="mt-3 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
          <summary className="cursor-pointer font-medium text-slate-700">
            What a client needs before this works in production
          </summary>
          <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs text-slate-600">
            <li>Create an account with the email provider (Resend) for the client.</li>
            <li>
              Add and verify the client&apos;s sending domain there, publishing the DNS records the
              provider shows (SPF and DKIM). Verification happens in DNS, outside this platform.
            </li>
            <li>
              Create an API key in that account and paste it below. <strong>From email</strong> must
              be an address on the verified domain.
            </li>
            <li>
              Save, then send the test message. Only a provider acceptance marks the connection
              verified.
            </li>
            <li>
              Set a notification address to hear about new enquiries. Leave it empty to disable
              alerts for this workspace.
            </li>
          </ol>
        </details>

        {state.kind === "loading" ? (
          <p className="mt-4 text-sm text-slate-500">Loading the email connection…</p>
        ) : state.kind === "unavailable" ? (
          <p role="alert" className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
            {state.message}
          </p>
        ) : null}

        {settings ? (
          <p className="mt-4 text-xs text-slate-500">
            {formatEmailConnectionStatus(settings.status)}
          </p>
        ) : null}

        <form onSubmit={(e) => void handleSave(e)} className="mt-4 grid gap-3 sm:max-w-lg">
          <div>
            <label htmlFor="email-from-name" className={labelClasses}>
              From name
            </label>
            <input
              id="email-from-name"
              value={fromName}
              onChange={(e) => setFromName(e.target.value)}
              placeholder="Rosewood Heating"
              maxLength={120}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div>
            <label htmlFor="email-from-address" className={labelClasses}>
              From email
            </label>
            <input
              id="email-from-address"
              value={fromEmail}
              onChange={(e) => setFromEmail(e.target.value)}
              placeholder="hello@rosewood.example"
              autoComplete="off"
              className={`mt-1 ${inputClasses}`}
            />
            <p className="mt-1 text-[11px] text-slate-500">
              Must be on a domain verified in the provider account. Replies go here if a client
              writes back.
            </p>
          </div>
          <div>
            <label htmlFor="email-notify" className={labelClasses}>
              New-lead alert address (optional)
            </label>
            <input
              id="email-notify"
              value={notifyEmail}
              onChange={(e) => setNotifyEmail(e.target.value)}
              placeholder="team@rosewood.example"
              autoComplete="off"
              className={`mt-1 ${inputClasses}`}
            />
            <p className="mt-1 text-[11px] text-slate-500">
              One alert per enquiry, sent when a lead is recorded or arrives from a connected
              website.
            </p>
          </div>
          <div>
            <label htmlFor="email-api-key" className={labelClasses}>
              Provider API key {settings?.hasApiKey ? "(leave empty to keep the stored key)" : ""}
            </label>
            <input
              id="email-api-key"
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={settings?.hasApiKey ? "••••••••" : "re_..."}
              autoComplete="new-password"
              className={`mt-1 ${inputClasses}`}
            />
          </div>

          {problems.length > 0 ? (
            <ul role="alert" className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
              {problems.map((problem) => (
                <li key={problem}>{problem}</li>
              ))}
            </ul>
          ) : null}
          {error ? (
            <p role="alert" className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
              {error}
            </p>
          ) : null}
          {notice ? (
            <p
              role="status"
              className="rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-800"
            >
              {notice}
            </p>
          ) : null}

          <div className="flex flex-wrap items-center gap-2">
            <button type="submit" disabled={busy !== null} className={PRIMARY_BUTTON}>
              {busy === "save" ? "Saving…" : configured ? "Save changes" : "Connect email"}
            </button>
            <button
              type="button"
              onClick={() => void handleTest()}
              disabled={busy !== null || !configured}
              className={SECONDARY_BUTTON}
            >
              {busy === "test" ? "Sending…" : "Send test message"}
            </button>
            {configured ? (
              <button
                type="button"
                onClick={() => void handleDisconnect()}
                disabled={busy !== null}
                className={SECONDARY_BUTTON}
              >
                {busy === "disconnect" ? "Disconnecting…" : "Disconnect"}
              </button>
            ) : null}
          </div>
        </form>

        <div className="mt-8 border-t border-slate-100 pt-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Delivery reports (provider webhook)
            </h3>
            {configured ? (
              <span
                className={`rounded px-2 py-0.5 text-[11px] font-semibold ${
                  settings?.webhookConfigured
                    ? "bg-emerald-100 text-emerald-800"
                    : "bg-amber-100 text-amber-800"
                }`}
              >
                {settings?.webhookConfigured ? "Receiving reports" : "Not connected"}
              </span>
            ) : null}
          </div>
          <p className="mt-2 max-w-prose text-sm text-slate-600">
            Sending only proves the provider accepted a message. Point the provider&apos;s webhook
            at this workspace to learn whether it was actually delivered, was delayed, bounced, or
            was marked as spam.
          </p>
          <details className="mt-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">
            <summary className="cursor-pointer font-medium text-slate-700">
              How to connect delivery reports in the provider
            </summary>
            <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs text-slate-600">
              <li>Open the client&apos;s account at the provider and go to Webhooks.</li>
              <li>
                Add an endpoint with the URL shown below, subscribing to delivered, delivery
                delayed, bounced, and complained events.
              </li>
              <li>
                Copy the endpoint&apos;s signing secret (it starts with <code>whsec_</code>) and
                paste it here, then save.
              </li>
              <li>
                Reports are accepted only when their signature verifies against this
                workspace&apos;s own secret, so one client&apos;s callbacks can never change
                another&apos;s log.
              </li>
            </ol>
          </details>

          {configured ? (
            <div className="mt-3 grid gap-3 sm:max-w-xl">
              <div>
                <span className={labelClasses}>Endpoint URL for this workspace</span>
                <div className="mt-1 flex flex-wrap items-center gap-2">
                  <code
                    data-testid="email-webhook-url"
                    className="min-w-0 flex-1 truncate rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-xs text-slate-700"
                  >
                    {webhookUrl}
                  </code>
                  <button
                    type="button"
                    onClick={() => void handleCopyWebhookUrl(webhookUrl)}
                    className={SECONDARY_BUTTON}
                  >
                    {webhookCopied ? "Copied" : "Copy"}
                  </button>
                </div>
              </div>
              {settings?.webhookConfigured ? (
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-xs text-slate-500">
                    A signing secret is stored for this workspace.
                  </p>
                  <button
                    type="button"
                    onClick={() => void handleClearWebhook()}
                    disabled={busy !== null}
                    className={SECONDARY_BUTTON}
                  >
                    {busy === "webhook" ? "Removing…" : "Remove secret"}
                  </button>
                </div>
              ) : (
                <form onSubmit={(e) => void handleSaveWebhook(e)} className="grid gap-2">
                  <label htmlFor="email-webhook-secret" className={labelClasses}>
                    Signing secret
                  </label>
                  <div className="flex flex-wrap items-center gap-2">
                    <input
                      id="email-webhook-secret"
                      type="password"
                      value={webhookSecret}
                      onChange={(e) => setWebhookSecret(e.target.value)}
                      placeholder="whsec_…"
                      autoComplete="off"
                      className={`min-w-0 flex-1 ${inputClasses}`}
                    />
                    <button type="submit" disabled={busy !== null} className={PRIMARY_BUTTON}>
                      {busy === "webhook" ? "Saving…" : "Save secret"}
                    </button>
                  </div>
                  {webhookProblems.length > 0 ? (
                    <ul
                      role="alert"
                      className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800"
                    >
                      {webhookProblems.map((problem) => (
                        <li key={problem}>{problem}</li>
                      ))}
                    </ul>
                  ) : null}
                </form>
              )}
              {webhookError ? (
                <p role="alert" className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
                  {webhookError}
                </p>
              ) : null}
              {webhookNotice ? (
                <p
                  role="status"
                  className="rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-800"
                >
                  {webhookNotice}
                </p>
              ) : null}
            </div>
          ) : (
            <p className="mt-2 text-xs text-slate-500">
              Connect the email account above first — the endpoint belongs to it.
            </p>
          )}
        </div>

        <div id="email-deliveries" className="mt-6 scroll-mt-6">
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Recent outbound email
            </h3>
            <button
              type="button"
              onClick={() => void refreshLog()}
              className="text-xs font-medium text-indigo-600 hover:text-indigo-800"
            >
              Refresh
            </button>
          </div>
          {deliveries.kind === "loading" ? (
            <p className="mt-2 text-sm text-slate-500">Loading…</p>
          ) : deliveries.kind === "unavailable" ? (
            <p className="mt-2 text-sm text-slate-500">{deliveries.message}</p>
          ) : list.length === 0 ? (
            <p className="mt-2 text-sm text-slate-500">
              Nothing sent yet. Replies you approve and new-lead alerts appear here with the
              provider&apos;s answer.
            </p>
          ) : (
            <ul className="mt-2 divide-y divide-slate-100 rounded-lg border border-slate-200">
              {list.map((delivery) => (
                <li key={delivery.id} className="px-3 py-2.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-xs font-semibold text-slate-700">
                      {formatDeliveryKind(delivery.kind)}
                    </span>
                    <span
                      className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${statusTone(delivery.status)}`}
                    >
                      {formatDeliveryStatus(delivery.status)}
                    </span>
                    {delivery.deliveryState !== "unknown" ? (
                      <span
                        data-testid="email-delivery-state"
                        className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${statusTone(delivery.deliveryState)}`}
                      >
                        {formatDeliveryState(delivery.deliveryState)}
                      </span>
                    ) : null}
                    <span className="text-[11px] text-slate-500">{delivery.to}</span>
                    {delivery.sentAt ? (
                      <span className="text-[11px] text-slate-400">
                        {new Date(delivery.sentAt).toLocaleString()}
                      </span>
                    ) : null}
                  </div>
                  <p className="mt-0.5 truncate text-sm text-slate-700">{delivery.subject}</p>
                  {delivery.deliveryDetail ? (
                    <p className="mt-1 text-xs text-slate-600">{delivery.deliveryDetail}</p>
                  ) : null}
                  {delivery.lastError ? (
                    <p className="mt-1 text-xs text-red-700">{delivery.lastError}</p>
                  ) : null}
                  {canRetryDelivery(delivery) ? (
                    <button
                      type="button"
                      onClick={() => void handleRetry(delivery)}
                      disabled={retryingId !== null || busy !== null}
                      className={`mt-1.5 ${SECONDARY_BUTTON}`}
                    >
                      {retryingId === delivery.id
                        ? "Queueing…"
                        : delivery.deliveryState === "bounced"
                          ? "Resend"
                          : "Retry send"}
                    </button>
                  ) : null}
                  {delivery.attempts > 1 ? (
                    <p className="text-[11px] text-slate-400">
                      {delivery.attempts} attempts
                      {delivery.providerMessageId
                        ? ` · provider id ${delivery.providerMessageId}`
                        : ""}
                    </p>
                  ) : delivery.providerMessageId ? (
                    <p className="text-[11px] text-slate-400">
                      provider id {delivery.providerMessageId}
                    </p>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="mt-6" data-testid="acknowledged-problems">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
            Problems you dismissed
          </h3>
          <p className="mt-1 text-xs text-slate-500">
            Acknowledging the overview badge clears it here; the messages themselves are never
            deleted, so this is what that acknowledgement covered.
          </p>
          {acknowledgedProblems.kind === "loading" ? (
            <p className="mt-2 text-sm text-slate-500">Loading…</p>
          ) : acknowledgedProblems.kind === "unavailable" ? (
            <p className="mt-2 text-sm text-slate-500">{acknowledgedProblems.message}</p>
          ) : acknowledgedAt === null ? (
            <p className="mt-2 text-sm text-slate-500">
              Nothing has been dismissed for this workspace yet.
            </p>
          ) : (
            <>
              <p className="mt-2 text-[11px] text-slate-400">
                Last acknowledged {new Date(acknowledgedAt).toLocaleString()}
              </p>
              {acknowledgedList.length === 0 ? (
                <p className="mt-2 text-sm text-slate-500">
                  That acknowledgement covered no failed or bounced message.
                </p>
              ) : (
                <ul className="mt-2 divide-y divide-slate-100 rounded-lg border border-slate-200">
                  {acknowledgedList.map((delivery) => (
                    <li key={delivery.id} className="px-3 py-2.5">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-xs font-semibold text-slate-700">
                          {formatDeliveryKind(delivery.kind)}
                        </span>
                        <span
                          className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${statusTone(delivery.status)}`}
                        >
                          {formatDeliveryStatus(delivery.status)}
                        </span>
                        {delivery.deliveryState !== "unknown" ? (
                          <span
                            data-testid="acknowledged-delivery-state"
                            className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${statusTone(delivery.deliveryState)}`}
                          >
                            {formatDeliveryState(delivery.deliveryState)}
                          </span>
                        ) : null}
                        <span className="text-[11px] text-slate-500">{delivery.to}</span>
                      </div>
                      <p className="mt-0.5 truncate text-sm text-slate-700">{delivery.subject}</p>
                      {delivery.deliveryDetail ? (
                        <p className="mt-1 text-xs text-slate-600">{delivery.deliveryDetail}</p>
                      ) : null}
                      {delivery.lastError ? (
                        <p className="mt-1 text-xs text-red-700">{delivery.lastError}</p>
                      ) : null}
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
      </div>
    </section>
  );
}
