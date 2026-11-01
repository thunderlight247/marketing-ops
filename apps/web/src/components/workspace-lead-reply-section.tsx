"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { requestLeadReply, type LeadReplyState } from "@/lib/agents";
import { fetchWorkspaceCampaignsLeads, type Lead } from "@/lib/campaigns";
import {
  fetchEmailSettings,
  fetchLeadEmailDeliveries,
  formatDeliveryKind,
  formatDeliveryState,
  formatDeliveryStatus,
  sendLeadReply,
  type DeliveriesState,
  type EmailSettingsState,
} from "@/lib/email";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

// statusTone keeps the send lifecycle and the provider's delivery report
// visually distinct: a message can be accepted (`sent`) and still bounced.
function statusTone(value: string): string {
  if (value === "connected" || value === "sent" || value === "delivered") {
    return "bg-emerald-100 text-emerald-800";
  }
  if (value === "error" || value === "failed" || value === "bounced" || value === "complained") {
    return "bg-red-100 text-red-800";
  }
  return "bg-amber-100 text-amber-800";
}

function leadLabel(lead: Lead): string {
  const who = lead.contactName ?? lead.contactEmail ?? "Unnamed lead";
  return `${who} · ${lead.status} · ${lead.sourceKind}`;
}

type SendState =
  | { kind: "idle" }
  | { kind: "sending" }
  | { kind: "queued"; to: string }
  | { kind: "blocked"; message: string };

// WorkspaceLeadReplySection runs the lead responder on one recorded enquiry,
// then lets the operator edit and send that reply through this workspace's own
// email account. The agent never sends anything; the send is an explicit
// operator action, recorded on the lead and in the delivery log.
export function WorkspaceLeadReplySection({
  workspaceId,
  onFiled,
}: Readonly<{ workspaceId: string; onFiled: () => void }>) {
  const [leads, setLeads] = useState<Lead[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [leadId, setLeadId] = useState("");
  const [focus, setFocus] = useState("");
  const [state, setState] = useState<LeadReplyState>({ kind: "idle" });
  const [email, setEmail] = useState<EmailSettingsState>({ kind: "loading" });
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [sendState, setSendState] = useState<SendState>({ kind: "idle" });
  const [history, setHistory] = useState<DeliveriesState>({ kind: "loading" });

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setLoadError(null);
    const result = await fetchWorkspaceCampaignsLeads(apiBaseUrl, workspaceId);
    if (result.kind === "loaded") {
      // A decided lead (won/lost) gets no first-reply draft from the server.
      const open = result.leads.filter((lead) => lead.status !== "won" && lead.status !== "lost");
      setLeads(open);
      setLeadId((current) =>
        current !== "" && open.some((lead) => lead.id === current) ? current : "",
      );
    } else if (result.kind === "unavailable" || result.kind === "missing") {
      setLeads([]);
      setLoadError(result.message);
    }
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
    void fetchEmailSettings(apiBaseUrl, workspaceId).then(setEmail);
  }, [load, apiBaseUrl, workspaceId]);

  // The sending history belongs to the picked enquiry, so it follows the
  // selection and is re-read after anything is queued.
  const loadHistory = useCallback(
    async (id: string) => {
      if (id === "") {
        setHistory({ kind: "loaded", deliveries: [] });
        return;
      }
      setHistory(await fetchLeadEmailDeliveries(apiBaseUrl, workspaceId, id));
    },
    [apiBaseUrl, workspaceId],
  );

  useEffect(() => {
    void loadHistory(leadId);
  }, [leadId, loadHistory]);

  async function handleRun(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState({ kind: "running" });
    setSendState({ kind: "idle" });
    const result = await requestLeadReply(apiBaseUrl, workspaceId, leadId, focus);
    if (result.kind === "done") {
      setFocus("");
      setSubject(result.reply.subject);
      setBody(result.reply.body);
      onFiled();
      await load();
    }
    setState(result);
  }

  async function handleSend() {
    setSendState({ kind: "sending" });
    const result = await sendLeadReply(apiBaseUrl, workspaceId, leadId, { subject, body });
    if (result.kind === "queued") {
      setSendState({ kind: "queued", to: result.delivery.to });
      onFiled();
      await load();
      await loadHistory(leadId);
      return;
    }
    if (result.kind === "conflict") {
      setSendState({ kind: "blocked", message: result.message });
      await load();
      return;
    }
    if (result.kind === "rejected") {
      setSendState({ kind: "blocked", message: result.problems.join("; ") });
      return;
    }
    setSendState({ kind: "blocked", message: result.message });
  }

  const running = state.kind === "running";
  const list = leads ?? [];
  const selected = list.find((lead) => lead.id === leadId) ?? null;
  const emailReady = email.kind === "loaded" && email.configured;
  const alreadySent = selected?.replySentAt ?? null;
  const sending = sendState.kind === "sending";

  // Choosing another lead clears the previous reply text and send result, so a
  // message can never be aimed at the wrong person.
  function selectLead(next: string) {
    setLeadId(next);
    setState({ kind: "idle" });
    setSendState({ kind: "idle" });
    const lead = list.find((item) => item.id === next) ?? null;
    setSubject(lead?.replyDraftSubject ?? "");
    setBody(lead?.replyDraftBody ?? "");
  }

  return (
    <section
      aria-labelledby="lead-reply-heading"
      className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5">
        <h2
          id="lead-reply-heading"
          className="text-xs font-semibold uppercase tracking-widest text-slate-500"
        >
          Lead replies (agent + email)
        </h2>
      </div>
      <div className="px-4 py-5 sm:px-5">
        <p className="max-w-prose text-sm text-slate-600">
          The lead responder drafts a first reply to one enquiry you already recorded, using only
          that lead&apos;s own words and this workspace&apos;s brand profile. Review and edit it,
          then send it through this workspace&apos;s email account. Nothing is sent until you press
          send, no contact details are invented, and the delivery log records what the provider
          answered.
        </p>

        {loadError ? (
          <p role="alert" className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
            {loadError}
          </p>
        ) : null}

        {email.kind === "loaded" && !email.configured ? (
          <p className="mt-4 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800">
            Email is not connected for this workspace yet, so replies can be drafted but not sent.
            Connect the account in the email panel above.
          </p>
        ) : email.kind === "loaded" && email.settings ? (
          <p className="mt-4 text-xs text-slate-500">
            Sending as {email.settings.fromName || email.settings.fromEmail} &lt;
            {email.settings.fromEmail}&gt;
          </p>
        ) : null}

        {list.length === 0 && !loadError ? (
          <p className="mt-4 text-sm text-slate-500">
            No open leads yet. Record a lead in the campaigns &amp; leads panel above, or connect a
            website so enquiries arrive on their own.
          </p>
        ) : null}

        {list.length > 0 ? (
          <form onSubmit={(e) => void handleRun(e)} className="mt-4 grid gap-3 sm:max-w-md">
            <label htmlFor="lead-reply-lead" className="text-sm font-medium text-slate-700">
              Lead
            </label>
            <select
              id="lead-reply-lead"
              value={leadId}
              onChange={(event) => selectLead(event.target.value)}
              className={inputClasses}
            >
              <option value="">Pick a lead…</option>
              {list.map((lead) => (
                <option key={lead.id} value={lead.id}>
                  {leadLabel(lead)}
                </option>
              ))}
            </select>
            {selected?.message ? (
              <p className="rounded-lg bg-slate-50 px-3 py-2 text-sm text-slate-600">
                <span className="font-medium text-slate-700">Their enquiry: </span>
                {selected.message}
              </p>
            ) : null}
            <label htmlFor="lead-reply-focus" className="text-sm font-medium text-slate-700">
              Focus (optional)
            </label>
            <textarea
              id="lead-reply-focus"
              value={focus}
              onChange={(event) => setFocus(event.target.value)}
              placeholder="e.g. mention we answer within one working day"
              rows={2}
              maxLength={2000}
              className={inputClasses}
            />
            <div>
              <button type="submit" disabled={running || leadId === ""} className={PRIMARY_BUTTON}>
                {running ? "Drafting a reply…" : "Draft a reply (agent)"}
              </button>
            </div>
          </form>
        ) : null}

        {state.kind === "done" ? (
          <div className="mt-4 rounded-xl border border-emerald-100 bg-emerald-50 px-4 py-3 text-sm">
            <p className="font-semibold text-emerald-900">
              Draft stored on the lead
              <span className="ml-2 rounded bg-white px-2 py-0.5 text-xs font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">
                status unchanged: {state.reply.leadStatus}
              </span>
            </p>
            <label
              htmlFor="lead-reply-subject"
              className="mt-3 block text-xs font-semibold uppercase tracking-wide text-emerald-700"
            >
              Subject
            </label>
            <input
              id="lead-reply-subject"
              value={subject}
              onChange={(event) => setSubject(event.target.value)}
              maxLength={200}
              className="mt-1 w-full rounded-lg border border-emerald-200 bg-white px-3 py-2 text-sm text-emerald-950"
            />
            <label
              htmlFor="lead-reply-body"
              className="mt-2 block text-xs font-semibold uppercase tracking-wide text-emerald-700"
            >
              Body
            </label>
            <textarea
              id="lead-reply-body"
              rows={7}
              value={body}
              onChange={(event) => setBody(event.target.value)}
              className="mt-1 w-full rounded-lg border border-emerald-200 bg-white px-3 py-2 text-sm text-emerald-950"
            />
            <p className="mt-2 text-xs text-emerald-800" aria-live="polite">
              Generated by {state.reply.provider} {state.reply.model} in {state.reply.durationMs}{" "}
              ms. Edit anything that needs a real detail before sending.
            </p>
          </div>
        ) : null}

        {state.kind === "unavailable" ? (
          <p role="alert" className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
            {state.message}
          </p>
        ) : null}

        {alreadySent ? (
          <p className="mt-4 rounded-lg bg-indigo-50 px-3 py-2 text-sm text-indigo-900">
            A reply was sent to this lead on {new Date(alreadySent).toLocaleString()}. Sending again
            is a separate decision — record it as a new contact from your own mailbox.
          </p>
        ) : null}

        {(state.kind === "done" || subject !== "" || body !== "") && selected && !alreadySent ? (
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => void handleSend()}
              disabled={sending || !emailReady || subject.trim() === "" || body.trim() === ""}
              className={PRIMARY_BUTTON}
            >
              {sending
                ? "Queueing the reply…"
                : `Send reply to ${selected.contactEmail ?? "the lead"}`}
            </button>
            {!emailReady ? (
              <span className="text-xs text-slate-500">Connect email to enable sending.</span>
            ) : null}
          </div>
        ) : null}

        {sendState.kind === "queued" ? (
          <p
            role="status"
            className="mt-3 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-800"
          >
            Queued to {sendState.to}. The email worker sends it within seconds; the delivery log in
            the email panel shows whether the provider accepted it.
          </p>
        ) : null}
        {sendState.kind === "blocked" ? (
          <p role="alert" className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-800">
            {sendState.message}
          </p>
        ) : null}

        {selected ? (
          <div className="mt-5 border-t border-slate-100 pt-4">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
                Sending history for this enquiry
              </h3>
              <button
                type="button"
                onClick={() => void loadHistory(leadId)}
                className="text-xs font-medium text-indigo-600 hover:text-indigo-800"
              >
                Refresh
              </button>
            </div>
            <p className="mt-1 max-w-prose text-xs text-slate-500">
              Every message the platform has tried to send about this lead, newest first: the alert
              it produced and each reply attempt, with what the provider answered.
            </p>
            {history.kind === "loading" ? (
              <p className="mt-2 text-sm text-slate-500">Loading…</p>
            ) : history.kind === "unavailable" ? (
              <p className="mt-2 text-sm text-slate-500">{history.message}</p>
            ) : history.deliveries.length === 0 ? (
              <p className="mt-2 text-sm text-slate-500">
                Nothing has been sent about this enquiry yet.
              </p>
            ) : (
              <ul className="mt-2 divide-y divide-slate-100 rounded-lg border border-slate-200">
                {history.deliveries.map((delivery) => (
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
                          className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${statusTone(delivery.deliveryState)}`}
                        >
                          {formatDeliveryState(delivery.deliveryState)}
                        </span>
                      ) : null}
                      <span className="text-[11px] text-slate-500">{delivery.to}</span>
                      <span className="text-[11px] text-slate-400">
                        {new Date(delivery.sentAt ?? delivery.createdAt).toLocaleString()}
                      </span>
                    </div>
                    <p className="mt-0.5 truncate text-sm text-slate-700">{delivery.subject}</p>
                    {delivery.deliveryDetail ? (
                      <p className="mt-0.5 text-xs text-slate-600">{delivery.deliveryDetail}</p>
                    ) : null}
                    {delivery.lastError ? (
                      <p className="mt-0.5 text-xs text-red-700">{delivery.lastError}</p>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
            <p className="mt-2 text-[11px] text-slate-500">
              Retrying or resending an attempt is done from the email panel&apos;s delivery log.
            </p>
          </div>
        ) : null}
      </div>
    </section>
  );
}
