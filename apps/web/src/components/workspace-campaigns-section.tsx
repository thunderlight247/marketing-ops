"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  createCampaign,
  createLead,
  createLeadConnection,
  deleteLeadConnection,
  listLeadConnections,
  type LeadConnection,
  fetchWorkspaceCampaignsLeads,
  formatCampaignStatus,
  formatLeadSource,
  formatLeadStatus,
  updateLead,
  validateLeadInput,
  type Campaign,
  type CampaignsLeadsState,
  type Lead,
} from "@/lib/campaigns";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

function statusChipClass(value: string): string {
  switch (value) {
    case "active":
    case "won":
      return "bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-700/20";
    case "paused":
    case "contacted":
    case "qualified":
      return "bg-amber-50 text-amber-800 ring-1 ring-inset ring-amber-700/20";
    case "lost":
    case "completed":
      return "bg-slate-100 text-slate-600 ring-1 ring-inset ring-slate-500/20";
    default:
      return "bg-blue-50 text-blue-800 ring-1 ring-inset ring-blue-700/15";
  }
}

export function WorkspaceCampaignsSection({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const [state, setState] = useState<CampaignsLeadsState>({ kind: "loading" });
  const [connections, setConnections] = useState<LeadConnection[]>([]);
  const [connectionName, setConnectionName] = useState("");
  const [connectionSecret, setConnectionSecret] = useState<{
    connection: LeadConnection;
    secret: string;
  } | null>(null);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [connectionBusy, setConnectionBusy] = useState(false);
  const [connectionDetailsId, setConnectionDetailsId] = useState<string | null>(null);

  // Add-campaign form state
  const [showCampaignForm, setShowCampaignForm] = useState(false);
  const [campaignName, setCampaignName] = useState("");
  const [campaignUtm, setCampaignUtm] = useState("");
  const [campaignNotes, setCampaignNotes] = useState("");
  const [campaignErrors, setCampaignErrors] = useState<string[]>([]);
  const [campaignAdding, setCampaignAdding] = useState(false);

  // Add-lead form state
  const [showLeadForm, setShowLeadForm] = useState(false);
  const [leadSource, setLeadSource] = useState("manual" as string);
  const [leadSourceDetail, setLeadSourceDetail] = useState("");
  const [leadName, setLeadName] = useState("");
  const [leadEmail, setLeadEmail] = useState("");
  const [leadMessage, setLeadMessage] = useState("");
  const [leadNotes, setLeadNotes] = useState("");
  const [leadCampaignId, setLeadCampaignId] = useState("");
  const [leadFollowUp, setLeadFollowUp] = useState("");
  const [leadErrors, setLeadErrors] = useState<string[]>([]);
  const [leadAdding, setLeadAdding] = useState(false);

  // Lead action state (status changes)
  const [busyLeadId, setBusyLeadId] = useState<string | null>(null);
  const [leadActionError, setLeadActionError] = useState<string | null>(null);
  const [visibleLeads, setVisibleLeads] = useState(20);

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    setState(await fetchWorkspaceCampaignsLeads(apiBaseUrl, workspaceId));
  }, [apiBaseUrl, workspaceId]);

  const loadConnections = useCallback(async () => {
    try {
      setConnections(await listLeadConnections(apiBaseUrl, workspaceId));
    } catch (err) {
      setConnectionError(err instanceof Error ? err.message : "Could not load lead connections.");
    }
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
    void loadConnections();
  }, [load, loadConnections]);

  async function handleCreateConnection(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (connectionSecret) return;
    setConnectionBusy(true);
    setConnectionError(null);
    try {
      const created = await createLeadConnection(apiBaseUrl, workspaceId, connectionName);
      setConnectionSecret(created);
      setConnectionDetailsId(created.connection.id);
      setConnectionName("");
      await loadConnections();
    } catch (err) {
      setConnectionError(err instanceof Error ? err.message : "Could not create connection.");
    } finally {
      setConnectionBusy(false);
    }
  }

  async function handleDeleteConnection(connection: LeadConnection) {
    if (
      !window.confirm(
        `Revoke ${connection.name}? Its website will no longer be able to send leads.`,
      )
    )
      return;
    setConnectionBusy(true);
    setConnectionError(null);
    try {
      await deleteLeadConnection(apiBaseUrl, workspaceId, connection.id);
      if (connectionSecret?.connection.id === connection.id) setConnectionSecret(null);
      if (connectionDetailsId === connection.id) setConnectionDetailsId(null);
      await loadConnections();
    } catch (err) {
      setConnectionError(err instanceof Error ? err.message : "Could not revoke connection.");
    } finally {
      setConnectionBusy(false);
    }
  }

  async function handleAddCampaign(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setCampaignErrors([]);
    setCampaignAdding(true);
    const res = await createCampaign(apiBaseUrl, workspaceId, {
      name: campaignName,
      status: "active",
      utmCampaign: campaignUtm,
      notes: campaignNotes,
    });
    setCampaignAdding(false);
    if (res.kind === "created") {
      setCampaignName("");
      setCampaignUtm("");
      setCampaignNotes("");
      setShowCampaignForm(false);
      void load();
    } else if (res.kind === "conflict") {
      setCampaignErrors([res.message]);
    } else if (res.kind === "rejected") {
      setCampaignErrors(res.problems);
    } else {
      setCampaignErrors([res.message]);
    }
  }

  async function handleAddLead(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setLeadErrors([]);
    const problems = validateLeadInput({
      sourceKind: leadSource,
      contactName: leadName,
      contactEmail: leadEmail,
      campaignId: leadCampaignId,
      followUpAt: leadFollowUp,
    });
    if (problems.length > 0) {
      setLeadErrors(problems);
      return;
    }
    setLeadAdding(true);
    const res = await createLead(apiBaseUrl, workspaceId, {
      sourceKind: leadSource,
      sourceDetail: leadSourceDetail,
      contactName: leadName,
      contactEmail: leadEmail,
      message: leadMessage,
      notes: leadNotes,
      campaignId: leadCampaignId,
      followUpAt: leadFollowUp,
    });
    setLeadAdding(false);
    if (res.kind === "created") {
      setLeadSourceDetail("");
      setLeadName("");
      setLeadEmail("");
      setLeadMessage("");
      setLeadNotes("");
      setLeadCampaignId("");
      setLeadFollowUp("");
      setShowLeadForm(false);
      void load();
    } else if (res.kind === "rejected") {
      setLeadErrors(res.problems);
    } else {
      setLeadErrors([res.message]);
    }
  }

  async function handleLeadStatus(lead: Lead, status: string) {
    setBusyLeadId(lead.id);
    setLeadActionError(null);
    const res = await updateLead(apiBaseUrl, workspaceId, lead.id, { status });
    setBusyLeadId(null);
    if (res.kind === "saved") {
      void load();
    } else if (res.kind === "rejected") {
      setLeadActionError(res.problems.join("; "));
    } else {
      setLeadActionError(res.message);
    }
  }

  return (
    <section
      aria-labelledby="workspace-campaigns-heading"
      className="rounded-lg border border-slate-200 bg-white px-5 py-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="workspace-campaigns-heading" className="text-base font-semibold text-slate-900">
          Campaigns &amp; leads
        </h2>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => {
              setShowCampaignForm((v) => !v);
              setShowLeadForm(false);
            }}
            className="rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50"
          >
            {showCampaignForm ? "Close campaign form" : "New campaign"}
          </button>
          <button
            type="button"
            onClick={() => {
              setShowLeadForm((v) => !v);
              setShowCampaignForm(false);
            }}
            className={PRIMARY_BUTTON}
          >
            {showLeadForm ? "Close lead form" : "Record a lead"}
          </button>
        </div>
      </div>

      <div className="mt-4 rounded-md border border-slate-200 bg-slate-50 p-3">
        <h3 className="text-sm font-semibold text-slate-800">Website lead connections</h3>
        <p className="mt-1 text-xs text-slate-600">
          Connect each business website separately. Creating a connection provides credentials; it
          does not connect the website automatically. A developer must configure the website server
          using the instructions below. CRM and no-backend websites need a separate integration.
        </p>
        <form
          onSubmit={(e) => void handleCreateConnection(e)}
          className="mt-3 flex flex-wrap items-end gap-2"
        >
          <label className={labelClasses}>
            Website or form name
            <input
              value={connectionName}
              onChange={(e) => setConnectionName(e.target.value)}
              required
              maxLength={120}
              className={`mt-1 ${inputClasses}`}
            />
          </label>
          <button
            type="submit"
            disabled={connectionBusy || connectionSecret !== null}
            className={PRIMARY_BUTTON}
          >
            Create connection
          </button>
        </form>
        {connectionError && (
          <p role="alert" className="mt-2 text-xs text-red-700">
            {connectionError}
          </p>
        )}
        {connectionSecret && (
          <div className="mt-3 rounded-md bg-amber-50 p-3 text-xs text-amber-900" role="status">
            <p className="font-semibold">Copy this secret now. It cannot be shown again.</p>
            <p className="mt-1">
              Save it in the website server&apos;s secret store before hiding it or creating another
              connection.
            </p>
            <p className="mt-1 break-all">
              Secret: <code>{connectionSecret.secret}</code>
            </p>
            <p className="mt-1 break-all">
              API path:{" "}
              <code>{`/api/v1/workspaces/${workspaceId}/lead-connections/${connectionSecret.connection.id}/submissions`}</code>
            </p>
            <p className="mt-1">Use the setup instructions below for this website.</p>
            <button
              type="button"
              className="mt-2 underline"
              onClick={() => setConnectionSecret(null)}
            >
              Hide secret
            </button>
          </div>
        )}
        <ul className="mt-3 space-y-3">
          {connections.map((connection) => {
            const path = `/api/v1/workspaces/${workspaceId}/lead-connections/${connection.id}`;
            return (
              <li
                key={connection.id}
                className="rounded border border-slate-200 bg-white p-3 text-xs text-slate-700"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="font-semibold">
                    {connection.name} · Connection created; website delivery not verified
                  </span>
                  <div className="flex gap-3">
                    <button
                      type="button"
                      onClick={() =>
                        setConnectionDetailsId(
                          connectionDetailsId === connection.id ? null : connection.id,
                        )
                      }
                      className="text-indigo-700 underline"
                    >
                      {connectionDetailsId === connection.id
                        ? "Hide instructions"
                        : "Setup instructions"}
                    </button>
                    <button
                      type="button"
                      disabled={connectionBusy}
                      onClick={() => void handleDeleteConnection(connection)}
                      className="text-red-700 underline disabled:opacity-50"
                    >
                      Revoke
                    </button>
                  </div>
                </div>
                {connectionDetailsId === connection.id && (
                  <div className="mt-3 space-y-2">
                    <p className="break-all">
                      Submission path: <code>{path}/submissions</code>
                    </p>
                    <p className="break-all">
                      Read-only check path: <code>{path}/verify</code>
                    </p>
                    <ol className="list-decimal space-y-1 pl-4">
                      <li>
                        Configure an externally reachable HTTPS API origin that forwards only these
                        intake routes to the Go API. Do not use the operator dashboard URL unless
                        your deployment explicitly routes the intake paths separately.
                      </li>
                      <li>
                        Store the full submission URL and the one-time connection secret in this
                        website server&apos;s private configuration; never put the secret in browser
                        code or share it with another business.
                      </li>
                      <li>
                        From the website server, POST to the check path with{" "}
                        <code>Authorization: Bearer &lt;secret&gt;</code>. HTTP 200 verifies the
                        credentials only; it does not test the form.
                      </li>
                      <li>
                        Configure the website form backend to POST enquiries to the submission path.
                        Send JSON with a unique <code>submissionId</code> and{" "}
                        <code>contactEmail</code> or <code>contactPhone</code>; optional fields
                        include <code>contactName</code>, <code>message</code>, and UTM values.
                      </li>
                      <li>
                        Preserve the exact JSON and ID on retries. Test with authorized sample data,
                        confirm it appears in this workspace, and arrange privacy notices, spam
                        protection and delivery monitoring.
                      </li>
                    </ol>
                    <p>
                      For full production setup, failure handling and examples, see{" "}
                      <code>docs/local-development.md</code> → Website lead intake. If the one-time
                      secret is lost, revoke this connection and create another.
                    </p>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      </div>

      {state.kind === "loading" && <p className="mt-4 text-sm text-slate-500">Loading…</p>}
      {state.kind === "unavailable" && (
        <p role="alert" className="mt-4 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {state.message}
        </p>
      )}
      {state.kind === "missing" && (
        <p className="mt-4 text-sm text-slate-500">This workspace no longer exists.</p>
      )}

      {showCampaignForm && (
        <form
          onSubmit={(e) => void handleAddCampaign(e)}
          className="mt-4 grid gap-3 border border-slate-200 bg-slate-50 p-3 rounded-md sm:max-w-md"
        >
          <div>
            <label htmlFor="new-campaign-name" className={labelClasses}>
              Campaign name
            </label>
            <input
              id="new-campaign-name"
              value={campaignName}
              onChange={(e) => setCampaignName(e.target.value)}
              placeholder="Spring newsletter"
              required
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div>
            <label htmlFor="new-campaign-utm" className={labelClasses}>
              UTM campaign value <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="new-campaign-utm"
              value={campaignUtm}
              onChange={(e) => setCampaignUtm(e.target.value)}
              placeholder="spring-newsletter"
              className={`mt-1 ${inputClasses}`}
            />
            <p className="mt-1 text-xs text-slate-500">
              Recorded only where real tracking uses it; the platform never invents attribution.
            </p>
          </div>
          <div>
            <label htmlFor="new-campaign-notes" className={labelClasses}>
              Notes <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="new-campaign-notes"
              value={campaignNotes}
              onChange={(e) => setCampaignNotes(e.target.value)}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          {campaignErrors.length > 0 && (
            <ul
              role="alert"
              className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800 list-disc pl-5 space-y-1"
            >
              {campaignErrors.map((err) => (
                <li key={err}>{err}</li>
              ))}
            </ul>
          )}
          <div>
            <button type="submit" disabled={campaignAdding} className={PRIMARY_BUTTON}>
              {campaignAdding ? "Creating…" : "Create campaign"}
            </button>
          </div>
        </form>
      )}

      {showLeadForm && (
        <form
          onSubmit={(e) => void handleAddLead(e)}
          className="mt-4 grid gap-3 border border-slate-200 bg-slate-50 p-3 rounded-md sm:max-w-md"
        >
          <div>
            <label htmlFor="new-lead-source" className={labelClasses}>
              Source
            </label>
            <select
              id="new-lead-source"
              value={leadSource}
              onChange={(e) => setLeadSource(e.target.value)}
              className={`mt-1 ${inputClasses}`}
            >
              <option value="manual">Recorded manually</option>
              <option value="form_api">Form / API</option>
              <option value="import">Imported</option>
              <option value="unknown">Source unknown</option>
            </select>
            <p className="mt-1 text-xs text-slate-500">
              A lead either has a traceable source or is explicitly marked unknown; the source is
              fixed once recorded.
            </p>
          </div>
          <div>
            <label htmlFor="new-lead-source-detail" className={labelClasses}>
              Source detail <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="new-lead-source-detail"
              value={leadSourceDetail}
              onChange={(e) => setLeadSourceDetail(e.target.value)}
              placeholder="contact form on nutritionguide.xyz"
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <label htmlFor="new-lead-name" className={labelClasses}>
                Contact name
              </label>
              <input
                id="new-lead-name"
                value={leadName}
                onChange={(e) => setLeadName(e.target.value)}
                className={`mt-1 ${inputClasses}`}
              />
            </div>
            <div>
              <label htmlFor="new-lead-email" className={labelClasses}>
                Contact email
              </label>
              <input
                id="new-lead-email"
                type="email"
                value={leadEmail}
                onChange={(e) => setLeadEmail(e.target.value)}
                className={`mt-1 ${inputClasses}`}
              />
            </div>
          </div>
          <div>
            <label htmlFor="new-lead-message" className={labelClasses}>
              Message <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <textarea
              id="new-lead-message"
              rows={2}
              value={leadMessage}
              onChange={(e) => setLeadMessage(e.target.value)}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <label htmlFor="new-lead-campaign" className={labelClasses}>
                Campaign <span className="font-normal text-slate-500">(optional)</span>
              </label>
              <select
                id="new-lead-campaign"
                value={leadCampaignId}
                onChange={(e) => setLeadCampaignId(e.target.value)}
                className={`mt-1 ${inputClasses}`}
              >
                <option value="">No campaign</option>
                {state.kind === "loaded" &&
                  state.campaigns.map((c: Campaign) => (
                    <option key={c.id} value={c.id}>
                      {c.name}
                    </option>
                  ))}
              </select>
            </div>
            <div>
              <label htmlFor="new-lead-followup" className={labelClasses}>
                Follow up on <span className="font-normal text-slate-500">(optional)</span>
              </label>
              <input
                id="new-lead-followup"
                type="date"
                value={leadFollowUp}
                onChange={(e) => setLeadFollowUp(e.target.value)}
                className={`mt-1 ${inputClasses}`}
              />
            </div>
          </div>
          <div>
            <label htmlFor="new-lead-notes" className={labelClasses}>
              Notes <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="new-lead-notes"
              value={leadNotes}
              onChange={(e) => setLeadNotes(e.target.value)}
              className={`mt-1 ${inputClasses}`}
            />
          </div>
          {leadErrors.length > 0 && (
            <ul
              role="alert"
              className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800 list-disc pl-5 space-y-1"
            >
              {leadErrors.map((err) => (
                <li key={err}>{err}</li>
              ))}
            </ul>
          )}
          <div>
            <button type="submit" disabled={leadAdding} className={PRIMARY_BUTTON}>
              {leadAdding ? "Recording…" : "Record lead"}
            </button>
          </div>
        </form>
      )}

      {leadActionError && (
        <p role="alert" className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
          {leadActionError}
        </p>
      )}

      {state.kind === "loaded" && (
        <>
          <div className="mt-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">
              Campaigns ({state.campaigns.length})
            </h3>
            {state.campaigns.length === 0 ? (
              <p className="mt-2 text-sm text-slate-500">No campaigns yet.</p>
            ) : (
              <ul className="mt-2 space-y-2">
                {state.campaigns.map((c) => (
                  <li
                    key={c.id}
                    className="flex flex-wrap items-center gap-2 rounded border border-slate-100 bg-slate-50 px-3 py-2 text-sm"
                  >
                    <span className="font-medium text-slate-800">{c.name}</span>
                    <span
                      className={`rounded px-2 py-0.5 text-xs font-medium ${statusChipClass(c.status)}`}
                    >
                      {formatCampaignStatus(c.status)}
                    </span>
                    {c.utmCampaign && (
                      <span className="rounded bg-white px-2 py-0.5 text-xs font-mono text-slate-600 ring-1 ring-inset ring-slate-200">
                        utm: {c.utmCampaign}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="mt-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">
              Leads ({state.leads.length})
            </h3>
            {state.leads.length === 0 ? (
              <p className="mt-2 text-sm text-slate-500">No leads recorded yet.</p>
            ) : (
              <ul className="mt-2 space-y-2">
                {state.leads.slice(0, visibleLeads).map((l) => {
                  const campaign = state.campaigns.find((c) => c.id === l.campaignId);
                  return (
                    <li
                      key={l.id}
                      className="rounded border border-slate-100 bg-slate-50 px-3 py-2 text-sm"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-slate-800">
                          {l.contactName || "(no name captured)"}
                        </span>
                        <span
                          className={`rounded px-2 py-0.5 text-xs font-medium ${statusChipClass(l.status)}`}
                        >
                          {formatLeadStatus(l.status)}
                        </span>
                        <span className="text-xs text-slate-500">
                          {formatLeadSource(l.sourceKind)}
                        </span>
                        {campaign && (
                          <span className="text-xs text-slate-500">· {campaign.name}</span>
                        )}
                        {l.followUpAt && (
                          <span className="rounded bg-white px-2 py-0.5 text-xs text-slate-700 ring-1 ring-inset ring-slate-200">
                            follow-up {l.followUpAt.slice(0, 10)}
                          </span>
                        )}
                        {l.replySentAt ? (
                          <span className="rounded bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-800 ring-1 ring-inset ring-indigo-200">
                            reply sent {l.replySentAt.slice(0, 10)}
                          </span>
                        ) : l.replyDraftBody ? (
                          <span className="rounded bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-800 ring-1 ring-inset ring-emerald-200">
                            reply draft ready (not sent)
                          </span>
                        ) : null}
                      </div>
                      {l.contactEmail && (
                        <p className="mt-1 text-xs text-slate-600">{l.contactEmail}</p>
                      )}
                      {l.sourceDetail && (
                        <p className="mt-1 text-xs text-slate-500">Source: {l.sourceDetail}</p>
                      )}
                      {l.message && (
                        <p className="mt-1 text-xs text-slate-600" title={l.message}>
                          {l.message.length > 140 ? `${l.message.slice(0, 140)}…` : l.message}
                        </p>
                      )}
                      {l.replyDraftBody && (
                        <details className="mt-1 text-xs text-slate-600">
                          <summary className="cursor-pointer text-emerald-800">
                            Agent reply draft: {l.replyDraftSubject ?? "(no subject)"}
                          </summary>
                          <p className="mt-1 whitespace-pre-line text-slate-700">
                            {l.replyDraftBody}
                          </p>
                        </details>
                      )}
                      <div className="mt-2 flex flex-wrap gap-1">
                        {(["new", "contacted", "qualified", "won", "lost"] as const).map((s) => (
                          <button
                            key={s}
                            type="button"
                            disabled={busyLeadId === l.id || l.status === s}
                            onClick={() => void handleLeadStatus(l, s)}
                            className={`rounded px-2 py-0.5 text-xs ${
                              l.status === s
                                ? "bg-slate-200 text-slate-500"
                                : "bg-white text-slate-700 ring-1 ring-inset ring-slate-300 hover:bg-slate-100"
                            }`}
                          >
                            {formatLeadStatus(s)}
                          </button>
                        ))}
                      </div>
                    </li>
                  );
                })}
                {state.leads.length > visibleLeads && (
                  <li className="pt-1 text-center">
                    <button
                      type="button"
                      onClick={() => setVisibleLeads((n) => n + 20)}
                      className="text-xs font-semibold text-indigo-600 hover:text-indigo-800"
                    >
                      Show more ({state.leads.length - visibleLeads} remaining)
                    </button>
                  </li>
                )}
              </ul>
            )}
          </div>
        </>
      )}
    </section>
  );
}
