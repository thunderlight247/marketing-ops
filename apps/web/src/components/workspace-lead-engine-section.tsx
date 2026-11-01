"use client";

import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  addToBlacklist,
  connectWhatsApp,
  disconnectWhatsApp,
  getLeadEngineConfig,
  getWhatsAppStatus,
  listBlacklist,
  listLeadEngineLeads,
  listOutreachLogs,
  saveLeadEngineConfig,
  triggerDiscovery,
  triggerDispatch,
  type BlacklistEntry,
  type LeadEngineConfig,
  type LeadEngineLead,
  type OutreachLog,
} from "@/lib/leadEngine";
import { useWorkspaceEvents } from "@/lib/events";

// ─── Design tokens / shared classes ──────────────────────────────────────────

const INPUT =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const LABEL = "block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1";
const BTN_PRIMARY =
  "inline-flex items-center justify-center rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors";
const BTN_GHOST =
  "inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors";
const BTN_DANGER =
  "inline-flex items-center justify-center rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors";
const CARD = "overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm";
const CARD_HEADER = "border-b border-slate-100 bg-slate-50 px-4 py-3 sm:px-5";
const CARD_BODY = "px-4 py-5 sm:px-5";

// ─── helper components ────────────────────────────────────────────────────────

function Spinner({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      className="animate-spin"
      aria-hidden="true"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
      />
    </svg>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, string> = {
    discovered: "bg-slate-100 text-slate-700",
    pending: "bg-amber-100 text-amber-800",
    processing: "bg-blue-100 text-blue-800",
    sent: "bg-indigo-100 text-indigo-800",
    delivered: "bg-emerald-100 text-emerald-800",
    replied: "bg-green-100 text-green-800",
    demo_sent: "bg-purple-100 text-purple-800",
    failed: "bg-red-100 text-red-700",
    blacklisted: "bg-red-200 text-red-800",
    connected: "bg-emerald-100 text-emerald-800",
    qr_ready: "bg-amber-100 text-amber-800",
    disconnected: "bg-slate-100 text-slate-600",
    initializing: "bg-blue-100 text-blue-700",
    authenticated: "bg-cyan-100 text-cyan-700",
  };
  const cls = map[status] ?? "bg-slate-100 text-slate-700";
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>
      {status.replace("_", " ")}
    </span>
  );
}

function Alert({ kind, children }: { kind: "info" | "success" | "error" | "warn"; children: React.ReactNode }) {
  const styles: Record<typeof kind, string> = {
    info: "bg-blue-50 border-blue-100 text-blue-800",
    success: "bg-emerald-50 border-emerald-100 text-emerald-800",
    error: "bg-red-50 border-red-100 text-red-800",
    warn: "bg-amber-50 border-amber-100 text-amber-800",
  };
  return (
    <p role="alert" className={`rounded-lg border px-4 py-3 text-sm ${styles[kind]}`}>
      {children}
    </p>
  );
}

// ─── default Ghana lists (mirrors engine.go defaults) ─────────────────────────

const DEFAULT_CATEGORIES = [
  "school","academy","clinic","hospital","hotel","guesthouse","restaurant",
  "salon","barbershop","shop","boutique","store","church","ngo","law firm",
  "accounting","consulting",
];

const DEFAULT_CITIES = [
  "Accra","Kumasi","Tamale","Cape Coast","Takoradi","Sunyani","Koforidua",
  "Ho","Bolgatanga","Wa","Tema","Kasoa","Ashaiman",
];

// ─── tab navigation ───────────────────────────────────────────────────────────

type Tab = "setup" | "whatsapp" | "leads" | "blacklist";
const TABS: { id: Tab; label: string }[] = [
  { id: "setup", label: "Setup" },
  { id: "whatsapp", label: "WhatsApp" },
  { id: "leads", label: "Leads" },
  { id: "blacklist", label: "Blacklist" },
];

// ─── Setup tab ────────────────────────────────────────────────────────────────

function SetupTab({
  config,
  onSaved,
}: {
  config: LeadEngineConfig;
  onSaved: (cfg: LeadEngineConfig) => void;
}) {
  const base = getApiBaseUrl();
  

  const [enabled, setEnabled] = useState(config.enabled);
  const [googleKey, setGoogleKey] = useState("");
  const [aiKey, setAiKey] = useState("");
  const [aiModel, setAiModel] = useState(config.aiModel || "meta-llama/llama-3.3-70b-instruct:free");
  const [senderName, setSenderName] = useState(config.senderName || "");
  const [businessOffering, setBusinessOffering] = useState(config.businessOffering || "");
  const [targetAudience, setTargetAudience] = useState(config.targetAudience || "");
  const [portfolioUrl, setPortfolioUrl] = useState(config.portfolioUrl || "");
  const [categoriesText, setCategoriesText] = useState(
    (config.targetCategories?.length ? config.targetCategories : DEFAULT_CATEGORIES).join("\n"),
  );
  const [citiesText, setCitiesText] = useState(
    (config.targetCities?.length ? config.targetCities : DEFAULT_CITIES).join("\n"),
  );
  const [dailyLimit, setDailyLimit] = useState(config.dailyLimit || 20);
  const [minDelay, setMinDelay] = useState(config.minDelayMs || 25000);
  const [maxDelay, setMaxDelay] = useState(config.maxDelayMs || 75000);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const toList = (text: string) =>
    text
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(false);
    if (minDelay >= maxDelay) {
      setError("Min delay must be less than max delay.");
      return;
    }
    setSaving(true);
    try {
      const saved = await saveLeadEngineConfig(base, config.workspaceId, {
        enabled,
        ...(googleKey ? { googlePlacesApiKey: googleKey } : {}),
        ...(aiKey ? { aiProviderKey: aiKey } : {}),
        aiModel,
        senderName,
        businessOffering,
        targetAudience,
        portfolioUrl,
        targetCategories: toList(categoriesText),
        targetCities: toList(citiesText),
        dailyLimit,
        minDelayMs: minDelay,
        maxDelayMs: maxDelay,
      });
      setGoogleKey("");
      setAiKey("");
      setSuccess(true);
      onSaved(saved);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <form onSubmit={(e) => void save(e)} className="grid gap-6">
      {/* enabled toggle */}
      <div className="flex items-center gap-3">
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          onClick={() => setEnabled((v) => !v)}
          className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 ${enabled ? "bg-indigo-600" : "bg-slate-200"}`}
        >
          <span
            className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ${enabled ? "translate-x-5" : "translate-x-0"}`}
          />
        </button>
        <span className="text-sm font-medium text-slate-700">
          {enabled ? "Lead engine enabled" : "Lead engine disabled"}
        </span>
      </div>

      {/* API keys */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label className={LABEL}>
            Google Places API Key{" "}
            {config.hasGooglePlacesKey && (
              <span className="font-normal normal-case text-emerald-600">✓ stored</span>
            )}
          </label>
          <input
            type="password"
            className={INPUT}
            placeholder={config.hasGooglePlacesKey ? "Leave blank to keep stored key" : "AIza…"}
            value={googleKey}
            onChange={(e) => setGoogleKey(e.target.value)}
            autoComplete="off"
          />
        </div>
        <div>
          <label className={LABEL}>
            AI Provider Key (OpenRouter){" "}
            {config.hasAiProviderKey && (
              <span className="font-normal normal-case text-emerald-600">✓ stored</span>
            )}
          </label>
          <input
            type="password"
            className={INPUT}
            placeholder={config.hasAiProviderKey ? "Leave blank to keep stored key" : "sk-or-…"}
            value={aiKey}
            onChange={(e) => setAiKey(e.target.value)}
            autoComplete="off"
          />
        </div>
      </div>

      {/* AI model + sender */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label className={LABEL}>AI Model</label>
          <input
            type="text"
            className={INPUT}
            value={aiModel}
            onChange={(e) => setAiModel(e.target.value)}
            placeholder="meta-llama/llama-3.3-70b-instruct:free"
          />
        </div>
        <div>
          <label className={LABEL}>Sender Name</label>
          <input
            type="text"
            className={INPUT}
            value={senderName}
            onChange={(e) => setSenderName(e.target.value)}
            placeholder="John"
          />
        </div>
      </div>

      {/* portfolio */}
      <div>
        <label className={LABEL}>Portfolio / Product URL</label>
        <input
          type="url"
          className={INPUT}
          value={portfolioUrl}
          onChange={(e) => setPortfolioUrl(e.target.value)}
          placeholder="https://example.com/services/"
        />
      </div>

      {/* business context */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label className={LABEL}>Business Offering</label>
          <textarea
            rows={3}
            className={INPUT}
            value={businessOffering}
            onChange={(e) => setBusinessOffering(e.target.value)}
            placeholder="We build websites and mobile apps for small businesses in Ghana."
          />
        </div>
        <div>
          <label className={LABEL}>Target Audience</label>
          <textarea
            rows={3}
            className={INPUT}
            value={targetAudience}
            onChange={(e) => setTargetAudience(e.target.value)}
            placeholder="Small and medium businesses without an online presence."
          />
        </div>
      </div>

      {/* categories + cities */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label className={LABEL}>
            Target Categories <span className="font-normal normal-case">(one per line)</span>
          </label>
          <textarea
            rows={6}
            className={INPUT}
            value={categoriesText}
            onChange={(e) => setCategoriesText(e.target.value)}
          />
        </div>
        <div>
          <label className={LABEL}>
            Target Cities <span className="font-normal normal-case">(one per line)</span>
          </label>
          <textarea
            rows={6}
            className={INPUT}
            value={citiesText}
            onChange={(e) => setCitiesText(e.target.value)}
          />
        </div>
      </div>

      {/* rate limits */}
      <fieldset className="grid gap-4 sm:grid-cols-3">
        <legend className={LABEL}>Rate Limits</legend>
        <div>
          <label className={LABEL}>Daily limit (messages)</label>
          <input
            type="number"
            min={1}
            max={500}
            className={INPUT}
            value={dailyLimit}
            onChange={(e) => setDailyLimit(Number(e.target.value))}
          />
        </div>
        <div>
          <label className={LABEL}>Min delay (ms)</label>
          <input
            type="number"
            min={5000}
            step={1000}
            className={INPUT}
            value={minDelay}
            onChange={(e) => setMinDelay(Number(e.target.value))}
          />
        </div>
        <div>
          <label className={LABEL}>Max delay (ms)</label>
          <input
            type="number"
            min={5000}
            step={1000}
            className={INPUT}
            value={maxDelay}
            onChange={(e) => setMaxDelay(Number(e.target.value))}
          />
        </div>
      </fieldset>

      {error && <Alert kind="error">{error}</Alert>}
      {success && <Alert kind="success">Configuration saved.</Alert>}

      <div>
        <button type="submit" disabled={saving} className={BTN_PRIMARY}>
          {saving ? (
            <>
              <Spinner size={14} />
              <span className="ml-2">Saving…</span>
            </>
          ) : (
            "Save configuration"
          )}
        </button>
      </div>
    </form>
  );
}

// ─── WhatsApp tab ─────────────────────────────────────────────────────────────

function WhatsAppTab({ config, onStatusChange }: { config: LeadEngineConfig; onStatusChange: (status: string, phone: string) => void }) {
  const base = getApiBaseUrl();
  

  const [status, setStatus] = useState(config.whatsappStatus);
  const [phone, setPhone] = useState(config.whatsappPhone);
  const [qr, setQr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Poll status when in a transitional state.
  useEffect(() => {
    if (status === "qr_ready" || status === "initializing" || status === "authenticated") {
      pollRef.current = setInterval(() => {
        void (async () => {
          try {
            const res = await getWhatsAppStatus(base, config.workspaceId);
            setStatus(res.status);
            if ("phone" in res && res.phone) setPhone(res.phone);
            if ("qr" in res && res.qr) setQr(res.qr);
            onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
            if (res.status === "connected") {
              if (pollRef.current) clearInterval(pollRef.current);
            }
          } catch {}
        })();
      }, 4000);
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [status, base, config.workspaceId, onStatusChange]);

  const connect = async () => {
    setError(null);
    setBusy(true);
    try {
      const res = await connectWhatsApp(base, config.workspaceId);
      setStatus(res.status);
      if ("qr" in res && res.qr) setQr(res.qr);
      if ("phone" in res && res.phone) setPhone(res.phone ?? "");
      onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Connection failed");
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    setError(null);
    setBusy(true);
    try {
      await disconnectWhatsApp(base, config.workspaceId);
      setStatus("disconnected");
      setPhone("");
      setQr(null);
      onStatusChange("disconnected", "");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Disconnect failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="grid gap-6">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm font-medium text-slate-700">Status:</span>
        <StatusBadge status={status} />
        {phone && <span className="text-sm text-slate-500">{phone}</span>}
      </div>

      {status === "connected" ? (
        <div className="grid gap-4">
          <Alert kind="success">
            WhatsApp is connected. Messages will be sent from <strong>{phone || "this number"}</strong>.
          </Alert>
          <div>
            <button onClick={() => void disconnect()} disabled={busy} className={BTN_DANGER}>
              {busy ? <><Spinner size={14} /><span className="ml-2">Disconnecting…</span></> : "Disconnect WhatsApp"}
            </button>
          </div>
        </div>
      ) : status === "qr_ready" && qr ? (
        <div className="grid gap-4">
          <Alert kind="warn">
            Scan the QR code below with WhatsApp on your phone. This is a one-time scan — the
            session will be saved automatically.
          </Alert>
          <div className="flex justify-center rounded-xl border border-slate-200 bg-white p-4">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={qr} alt="WhatsApp QR code" className="max-w-xs w-full" />
          </div>
          <p className="text-center text-xs text-slate-500 animate-pulse">
            Waiting for scan…
          </p>
        </div>
      ) : status === "initializing" || status === "authenticated" ? (
        <div className="flex items-center gap-3 text-sm text-slate-600">
          <Spinner />
          <span>Connecting to WhatsApp…</span>
        </div>
      ) : (
        <div className="grid gap-4">
          <Alert kind="info">
            Connect a WhatsApp number to enable automated outreach. The first connection requires a
            QR code scan. Subsequent startups will reconnect automatically.
          </Alert>
          <div>
            <button onClick={() => void connect()} disabled={busy} className={BTN_PRIMARY}>
              {busy ? <><Spinner size={14} /><span className="ml-2">Starting…</span></> : "Connect WhatsApp"}
            </button>
          </div>
        </div>
      )}

      {error && <Alert kind="error">{error}</Alert>}
    </div>
  );
}

// ─── Leads tab ────────────────────────────────────────────────────────────────

function LeadsTab({ config }: { config: LeadEngineConfig }) {
  const base = getApiBaseUrl();
  

  const [leads, setLeads] = useState<LeadEngineLead[]>([]);
  const [loading, setLoading] = useState(true);
  const [discovering, setDiscovering] = useState(false);
  const [dispatching, setDispatching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [selectedLead, setSelectedLead] = useState<string | null>(null);
  const [logs, setLogs] = useState<OutreachLog[]>([]);
  const [loadingLogs, setLoadingLogs] = useState(false);

  const loadLeads = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listLeadEngineLeads(base, config.workspaceId);
      setLeads(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load leads");
    } finally {
      setLoading(false);
    }
  }, [base, config.workspaceId]);

  useEffect(() => {
    void loadLeads();
  }, [loadLeads]);

  useWorkspaceEvents(config.workspaceId, (ev) => {
    if (
      ev.type === "lead_discovered" ||
      ev.type === "pitch_dispatched" ||
      ev.type === "whatsapp_inbound" ||
      ev.type === "whatsapp_replied" ||
      ev.type === "lead_created" ||
      ev.type === "lead_updated"
    ) {
      void loadLeads();
    }
  });

  const discover = async () => {
    setError(null);
    setNotice(null);
    setDiscovering(true);
    try {
      await triggerDiscovery(base, config.workspaceId);
      setNotice("Discovery started in background. Refresh in a moment to see new leads.");
      setTimeout(() => void loadLeads(), 5000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Discovery failed");
    } finally {
      setDiscovering(false);
    }
  };

  const dispatch = async () => {
    setError(null);
    setNotice(null);
    setDispatching(true);
    try {
      await triggerDispatch(base, config.workspaceId);
      setNotice("Dispatch started. Messages are being sent in the background.");
      setTimeout(() => void loadLeads(), 8000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Dispatch failed");
    } finally {
      setDispatching(false);
    }
  };

  const viewLogs = async (leadId: string) => {
    if (selectedLead === leadId) {
      setSelectedLead(null);
      return;
    }
    setSelectedLead(leadId);
    setLoadingLogs(true);
    try {
      const data = await listOutreachLogs(base, config.workspaceId, leadId);
      setLogs(data);
    } catch {
      setLogs([]);
    } finally {
      setLoadingLogs(false);
    }
  };

  const pending = leads.filter((l) => l.status === "discovered" || l.status === "pending").length;
  const sent = leads.filter((l) => ["sent", "delivered", "replied", "demo_sent"].includes(l.status)).length;
  const failed = leads.filter((l) => l.status === "failed").length;

  return (
    <div className="grid gap-5">
      {/* action bar */}
      <div className="flex flex-wrap gap-3">
        <button
          onClick={() => void discover()}
          disabled={discovering || !config.hasGooglePlacesKey}
          title={!config.hasGooglePlacesKey ? "Configure Google Places API key in Setup" : undefined}
          className={BTN_PRIMARY}
        >
          {discovering ? <><Spinner size={14} /><span className="ml-2">Discovering…</span></> : "🔍 Run Discovery"}
        </button>
        <button
          onClick={() => void dispatch()}
          disabled={dispatching || config.whatsappStatus !== "connected" || pending === 0}
          title={
            config.whatsappStatus !== "connected"
              ? "Connect WhatsApp first"
              : pending === 0
              ? "No pending leads"
              : undefined
          }
          className={BTN_PRIMARY}
        >
          {dispatching ? <><Spinner size={14} /><span className="ml-2">Sending…</span></> : `📤 Send Pending (${pending})`}
        </button>
        <button onClick={() => void loadLeads()} disabled={loading} className={BTN_GHOST}>
          {loading ? <Spinner size={14} /> : "↻ Refresh"}
        </button>
      </div>

      {/* stats */}
      {leads.length > 0 && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[
            { label: "Total", value: leads.length, color: "text-slate-700" },
            { label: "Pending", value: pending, color: "text-amber-700" },
            { label: "Sent", value: sent, color: "text-emerald-700" },
            { label: "Failed", value: failed, color: "text-red-600" },
          ].map(({ label, value, color }) => (
            <div key={label} className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-center">
              <p className={`text-2xl font-bold ${color}`}>{value}</p>
              <p className="text-xs text-slate-500">{label}</p>
            </div>
          ))}
        </div>
      )}

      {error && <Alert kind="error">{error}</Alert>}
      {notice && <Alert kind="info">{notice}</Alert>}

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-slate-500 py-6">
          <Spinner /> Loading leads…
        </div>
      ) : leads.length === 0 ? (
        <Alert kind="info">
          No leads discovered yet. Configure your Google Places API key in Setup, then click
          &quot;Run Discovery&quot; to find potential clients.
        </Alert>
      ) : (
        <>
          {/* Mobile card view */}
          <div className="grid gap-3 md:hidden">
            {leads.map((lead) => (
              <div
                key={`mobile-${lead.id}`}
                className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="font-semibold text-slate-900 break-words">{lead.name}</p>
                    {lead.phone && (
                      <a
                        href={`tel:${lead.phone}`}
                        className="mt-0.5 inline-block font-mono text-xs text-indigo-600 hover:underline"
                      >
                        {lead.phone}
                      </a>
                    )}
                  </div>
                  <StatusBadge status={lead.status} />
                </div>

                <div className="mt-2.5 flex flex-wrap items-center gap-2 text-xs text-slate-500">
                  <span className="rounded bg-slate-100 px-2 py-0.5 font-medium text-slate-700">
                    {lead.category}
                  </span>
                  {lead.city && <span>📍 {lead.city}</span>}
                  {lead.rating ? (
                    <span className="text-amber-600 font-medium">⭐ {lead.rating.toFixed(1)}</span>
                  ) : null}
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-slate-100 pt-2.5">
                  <button
                    type="button"
                    onClick={() => void viewLogs(lead.id)}
                    className="text-xs font-semibold text-indigo-600 hover:text-indigo-800"
                  >
                    {selectedLead === lead.id ? "Hide logs & pitch ▲" : "View logs & pitch ▼"}
                  </button>
                  <span className="text-[11px] text-slate-400">
                    {new Date(lead.createdAt).toLocaleDateString()}
                  </span>
                </div>

                {selectedLead === lead.id && (
                  <div className="mt-3 rounded-lg bg-slate-50 p-3 border border-slate-200 text-xs">
                    {lead.pitch && (
                      <div className="mb-3 rounded-md border border-slate-200 bg-white p-3">
                        <p className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
                          Pitch
                        </p>
                        <p className="text-slate-700 whitespace-pre-wrap">{lead.pitch}</p>
                      </div>
                    )}
                    {loadingLogs ? (
                      <div className="flex items-center gap-2 text-slate-500">
                        <Spinner size={14} /> Loading logs…
                      </div>
                    ) : logs.length === 0 ? (
                      <p className="text-slate-400">No outreach logs for this lead.</p>
                    ) : (
                      <div className="grid gap-2">
                        {logs.map((log) => (
                          <div
                            key={log.id}
                            className="rounded border border-slate-200 bg-white p-2.5"
                          >
                            <div className="flex flex-wrap items-center gap-1.5 text-slate-500 text-[11px]">
                              <span className="font-mono">{new Date(log.createdAt).toLocaleTimeString()}</span>
                              <span className="uppercase font-semibold">{log.direction}</span>
                              <StatusBadge status={log.status} />
                            </div>
                            {log.body && <p className="mt-1 text-slate-700">{log.body}</p>}
                            {log.error && <p className="mt-1 text-red-600 font-medium">Error: {log.error}</p>}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>

          {/* Desktop table view */}
          <div className="hidden md:block overflow-x-auto rounded-xl border border-slate-200">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left">
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Business</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Category</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 hidden sm:table-cell">City</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 hidden md:table-cell">Rating</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Status</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Actions</th>
                </tr>
              </thead>
              <tbody>
                {leads.map((lead) => (
                  <Fragment key={lead.id}>
                    <tr className="border-b border-slate-100 hover:bg-slate-50 transition-colors">
                      <td className="px-4 py-3">
                        <p className="font-medium text-slate-900">{lead.name}</p>
                        <p className="text-xs text-slate-400">{lead.phone}</p>
                      </td>
                      <td className="px-4 py-3 text-slate-600">{lead.category}</td>
                      <td className="px-4 py-3 text-slate-600 hidden sm:table-cell">{lead.city}</td>
                      <td className="px-4 py-3 hidden md:table-cell">
                        {lead.rating ? (
                          <span className="text-amber-600">⭐ {lead.rating.toFixed(1)}</span>
                        ) : (
                          <span className="text-slate-400">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge status={lead.status} />
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={() => void viewLogs(lead.id)}
                          className="text-xs text-indigo-600 hover:text-indigo-800 font-medium"
                        >
                          {selectedLead === lead.id ? "Hide logs" : "View logs"}
                        </button>
                      </td>
                    </tr>
                    {selectedLead === lead.id && (
                      <tr key={`${lead.id}-logs`}>
                        <td colSpan={6} className="bg-slate-50 px-4 py-3">
                          {lead.pitch && (
                            <div className="mb-3 rounded-lg border border-slate-200 bg-white p-3">
                              <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Pitch</p>
                              <p className="text-sm text-slate-700 whitespace-pre-wrap">{lead.pitch}</p>
                            </div>
                          )}
                          {loadingLogs ? (
                            <div className="flex items-center gap-2 text-sm text-slate-500">
                              <Spinner size={14} /> Loading logs…
                            </div>
                          ) : logs.length === 0 ? (
                            <p className="text-xs text-slate-400">No outreach logs for this lead.</p>
                          ) : (
                            <div className="grid gap-2">
                              {logs.map((log) => (
                                <div key={log.id} className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs">
                                  <div className="flex items-center gap-2 text-slate-500">
                                    <span className="font-mono">{new Date(log.createdAt).toLocaleString()}</span>
                                    <span className="uppercase">{log.direction}</span>
                                    <StatusBadge status={log.status} />
                                  </div>
                                  {log.body && <p className="mt-1 text-slate-700">{log.body}</p>}
                                  {log.error && <p className="mt-1 text-red-600">Error: {log.error}</p>}
                                </div>
                              ))}
                            </div>
                          )}
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}

// ─── Blacklist tab ────────────────────────────────────────────────────────────

function BlacklistTab({ workspaceId }: { workspaceId: string }) {
  const base = getApiBaseUrl();
  

  const [list, setList] = useState<BlacklistEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [phone, setPhone] = useState("");
  const [reason, setReason] = useState("");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setList(await listBlacklist(base, workspaceId));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load blacklist");
    } finally {
      setLoading(false);
    }
  }, [base, workspaceId]);

  useEffect(() => { void load(); }, [load]);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!phone.trim()) return;
    setError(null);
    setAdding(true);
    try {
      await addToBlacklist(base, workspaceId, phone.trim(), reason.trim());
      setPhone("");
      setReason("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add to blacklist");
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="grid gap-6">
      <form onSubmit={(e) => void add(e)} className="grid gap-4 sm:grid-cols-3 sm:items-end">
        <div>
          <label className={LABEL}>Phone Number</label>
          <input
            type="tel"
            className={INPUT}
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="+233241234567"
            required
          />
        </div>
        <div>
          <label className={LABEL}>Reason (optional)</label>
          <input
            type="text"
            className={INPUT}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Requested opt-out"
          />
        </div>
        <div>
          <button type="submit" disabled={adding || !phone.trim()} className={BTN_PRIMARY}>
            {adding ? <><Spinner size={14} /><span className="ml-2">Adding…</span></> : "Add to blacklist"}
          </button>
        </div>
      </form>

      {error && <Alert kind="error">{error}</Alert>}

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-slate-500"><Spinner /> Loading…</div>
      ) : list.length === 0 ? (
        <Alert kind="info">No suppressed numbers yet.</Alert>
      ) : (
        <>
          {/* Mobile card view */}
          <div className="grid gap-2 sm:hidden">
            {list.map((entry) => (
              <div key={`m-bl-${entry.id}`} className="rounded-lg border border-slate-200 bg-white p-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-sm font-semibold text-slate-800">{entry.phoneNumber}</span>
                  <span className="text-[11px] text-slate-400">
                    {new Date(entry.createdAt).toLocaleDateString()}
                  </span>
                </div>
                {entry.reason && <p className="mt-1 text-xs text-slate-600">{entry.reason}</p>}
              </div>
            ))}
          </div>

          {/* Desktop table view */}
          <div className="hidden sm:block overflow-x-auto rounded-xl border border-slate-200">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left">
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Phone</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Reason</th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Added</th>
                </tr>
              </thead>
              <tbody>
                {list.map((entry) => (
                  <tr key={entry.id} className="border-b border-slate-100 last:border-0">
                    <td className="px-4 py-3 font-mono text-slate-700">{entry.phoneNumber}</td>
                    <td className="px-4 py-3 text-slate-500">{entry.reason || "—"}</td>
                    <td className="px-4 py-3 text-slate-400 text-xs">
                      {new Date(entry.createdAt).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export function WorkspaceLeadEngineSection({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const base = getApiBaseUrl();
  

  const [activeTab, setActiveTab] = useState<Tab>("setup");
  const [config, setConfig] = useState<LeadEngineConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        setConfig(await getLeadEngineConfig(base, workspaceId));
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load lead engine config");
      } finally {
        setLoading(false);
      }
    })();
  }, [base, workspaceId]);

  const handleStatusChange = useCallback((status: string, phone: string) => {
    setConfig((prev) => prev ? { ...prev, whatsappStatus: status, whatsappPhone: phone } : prev);
  }, []);

  useWorkspaceEvents(workspaceId, (ev) => {
    if (ev.type === "whatsapp_status" && ev.message) {
      setConfig((prev) => prev ? { ...prev, whatsappStatus: ev.message || "disconnected" } : prev);
    }
  });

  return (
    <section
      aria-labelledby="lead-engine-heading"
      className={`${CARD} mt-6`}
    >
      <div className={CARD_HEADER}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2
              id="lead-engine-heading"
              className="text-xs font-semibold uppercase tracking-widest text-slate-600"
            >
              Lead Generation &amp; Outreach Engine
            </h2>
            <p className="mt-0.5 text-xs text-slate-400">
              Discover potential clients via Google Places and send tailored WhatsApp pitches.
            </p>
          </div>
          {config && (
            <div className="flex items-center gap-2">
              {config.whatsappStatus === "connected" && (
                <span className="flex h-2 w-2 rounded-full bg-emerald-500" />
              )}
              <StatusBadge status={config.whatsappStatus || "disconnected"} />
            </div>
          )}
        </div>
      </div>

      {/* tab bar */}
      <div className="border-b border-slate-200 bg-white px-4 sm:px-5">
        <nav className="-mb-px flex gap-0 overflow-x-auto" aria-label="Lead engine tabs">
          {TABS.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex-shrink-0 border-b-2 px-3 py-3 text-xs font-semibold uppercase tracking-wide transition-colors sm:px-4 ${
                activeTab === tab.id
                  ? "border-indigo-500 text-indigo-600"
                  : "border-transparent text-slate-500 hover:text-slate-700"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      <div className={CARD_BODY}>
        {loading ? (
          <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
            <Spinner /> Loading lead engine…
          </div>
        ) : error ? (
          <Alert kind="error">{error}</Alert>
        ) : config ? (
          <>
            {activeTab === "setup" && (
              <SetupTab config={config} onSaved={setConfig} />
            )}
            {activeTab === "whatsapp" && (
              <WhatsAppTab config={config} onStatusChange={handleStatusChange} />
            )}
            {activeTab === "leads" && (
              <LeadsTab config={config} />
            )}
            {activeTab === "blacklist" && (
              <BlacklistTab workspaceId={workspaceId} />
            )}
          </>
        ) : null}
      </div>
    </section>
  );
}
