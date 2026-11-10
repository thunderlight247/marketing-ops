"use client";

import Link from "next/link";
import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  addToBlacklist,
  connectOwnWhatsApp,
  connectWhatsApp,
  disconnectWhatsApp,
  getLeadEngineConfig,
  getWhatsAppStatus,
  importLeadEngineLeads,
  listBlacklist,
  listDailyOutreach,
  listLeadEngineLeads,
  listMessagedContacts,
  listOutreachLogs,
  listWhatsAppSessions,
  pairOwnWhatsApp,
  saveLeadEngineConfig,
  sendFollowUp,
  triggerDiscovery,
  triggerDispatch,
  type BlacklistEntry,
  type LeadEngineConfig,
  type LeadEngineLead,
  type MessagedContact,
  type OutreachActivity,
  type OutreachLog,
  type WhatsAppRetry,
  type WhatsAppSession,
} from "@/lib/leadEngine";
import { exportLeadEngineLeadsToCsv, parseLeadEngineCsv } from "@/lib/csv";
import { useWorkspaceEvents } from "@/lib/events";

// Where a lead came from. Google Maps is a curated commercial directory;
// OpenStreetMap is community-mapped and usually has no rating, so a row's
// provenance is worth showing next to its data.
const DISCOVERY_SOURCE_LABELS: Record<string, string> = {
  google_places: "Google Maps",
  openstreetmap: "OpenStreetMap",
  csv_import: "CSV import",
};

// ─── Design tokens / shared classes ──────────────────────────────────────────

const INPUT =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const LABEL = "block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1";
const BTN_PRIMARY =
  "inline-flex items-center justify-center rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors";
const BTN_GHOST =
  "inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors";
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
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
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
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}
    >
      {status.replace("_", " ")}
    </span>
  );
}

// A lead's status says where it sits in the pipeline; this line says what
// actually happened to its latest message. "Sent" is only shown once the
// dispatcher accepted it, and a failed attempt carries the reason it did not
// leave, so the operator never has to guess whether a message went out.
// Pull the human-readable part out of a stored dispatcher error, which is
// often a JSON body ({"success":false,"error":"..."}).
function sendFailureText(raw: string): string {
  const json = raw.match(/"error"\s*:\s*"([^"]+)"/);
  if (json) return json[1];
  return raw.replace(/^not_sent:\s*/, "").replace(/^dispatcher HTTP \d+:\s*/, "");
}

function SendConfirmation({ lead }: { lead: LeadEngineLead }) {
  if (lead.lastSendStatus === "sent") {
    return (
      <p className="mt-1 text-[11px] font-medium text-emerald-700">
        ✓ Sent
        {lead.lastSendAt ? ` ${new Date(lead.lastSendAt).toLocaleString()}` : ""}
      </p>
    );
  }
  if (lead.lastSendStatus === "failed") {
    const raw = lead.lastSendError || "";
    const refused =
      raw.startsWith("not_sent:") || /notOnWhatsApp|not registered on WhatsApp/i.test(raw);
    const detail = sendFailureText(raw) || "the dispatcher refused it";
    return (
      <p
        className={`mt-1 max-w-[240px] text-[11px] leading-snug ${
          refused ? "text-red-600" : "text-amber-700"
        }`}
        title={raw || detail}
      >
        {refused ? `Not sent — ${detail}` : `Not confirmed — ${detail}`}
      </p>
    );
  }
  if (lead.lastSendStatus === "sending") {
    return (
      <p className="mt-1 text-[11px] leading-snug text-blue-700">Sending… no confirmation yet</p>
    );
  }
  return null;
}

function ModeChip({
  tone,
  children,
}: {
  tone: "emerald" | "indigo" | "slate";
  children: React.ReactNode;
}) {
  const tones = {
    emerald: "bg-emerald-100 text-emerald-800",
    indigo: "bg-indigo-100 text-indigo-700",
    slate: "bg-slate-100 text-slate-600",
  } as const;
  return (
    <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${tones[tone]}`}>
      {children}
    </span>
  );
}

function formatPairingCode(code: string): string {
  const cleaned = code.replace(/[^a-zA-Z0-9]/g, "").toUpperCase();
  return cleaned.length === 8 ? `${cleaned.slice(0, 4)}-${cleaned.slice(4)}` : cleaned;
}

function whatsAppStatusLabel(status: string): string {
  switch (status) {
    case "connected":
      return "Connected";
    case "qr_ready":
      return "Waiting for scan";
    case "initializing":
      return "Starting";
    case "authenticated":
      return "Authenticating";
    case "disconnected":
      return "Disconnected";
    default:
      return status ? status.replace(/_/g, " ") : "Unknown";
  }
}

function RefreshIcon() {
  return (
    <svg
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M20 11a8 8 0 1 0-2.3 6.3" />
      <path d="M20 4v7h-7" />
    </svg>
  );
}

function Alert({
  kind,
  children,
}: {
  kind: "info" | "success" | "error" | "warn";
  children: React.ReactNode;
}) {
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
  "school",
  "academy",
  "clinic",
  "hospital",
  "hotel",
  "guesthouse",
  "restaurant",
  "salon",
  "barbershop",
  "shop",
  "boutique",
  "store",
  "church",
  "ngo",
  "law firm",
  "accounting",
  "consulting",
];

const DEFAULT_CITIES = [
  "Accra",
  "Kumasi",
  "Tamale",
  "Cape Coast",
  "Takoradi",
  "Sunyani",
  "Koforidua",
  "Ho",
  "Bolgatanga",
  "Wa",
  "Tema",
  "Kasoa",
  "Ashaiman",
];

// ─── tab navigation ───────────────────────────────────────────────────────────

type Tab = "setup" | "whatsapp" | "leads" | "daily" | "followup" | "blacklist";
const TABS: { id: Tab; label: string }[] = [
  { id: "setup", label: "Setup" },
  { id: "whatsapp", label: "WhatsApp" },
  { id: "leads", label: "Leads" },
  { id: "daily", label: "Daily Activity" },
  { id: "followup", label: "Follow-up" },
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
  const [aiModel, setAiModel] = useState(
    config.aiModel || "meta-llama/llama-3.3-70b-instruct:free",
  );
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
  // Proven-safe defaults: 2–5 minutes between messages (same values the
  // legacy engine ran for months without an account restriction).
  const [minDelay, setMinDelay] = useState(config.minDelayMs || 120000);
  const [maxDelay, setMaxDelay] = useState(config.maxDelayMs || 300000);
  const [pitchTemplate, setPitchTemplate] = useState(config.pitchTemplate || "");
  const [enableAutoFollowup, setEnableAutoFollowup] = useState(Boolean(config.enableAutoFollowup));
  const [followup1Template, setFollowup1Template] = useState(config.followup1Template || "");
  const [followup1DelayDays, setFollowup1DelayDays] = useState(config.followup1DelayDays || 3);
  const [followup2Template, setFollowup2Template] = useState(config.followup2Template || "");
  const [followup2DelayDays, setFollowup2DelayDays] = useState(config.followup2DelayDays || 5);
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
        ...(googleKey.trim() ? { googlePlacesApiKey: googleKey.trim() } : {}),
        ...(aiKey.trim() ? { aiProviderKey: aiKey.trim() } : {}),
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
        pitchTemplate,
        enableAutoFollowup,
        followup1Template,
        followup1DelayDays,
        followup2Template,
        followup2DelayDays,
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

      {/* custom outreach message */}
      <div>
        <label className={LABEL}>
          Message to send{" "}
          <span className="font-normal normal-case">
            (optional — replaces the AI-written pitch)
          </span>
        </label>
        <textarea
          rows={6}
          className={INPUT}
          value={pitchTemplate}
          onChange={(e) => setPitchTemplate(e.target.value)}
          placeholder={
            "Leave blank to let the AI write a pitch from your business details.\n\n" +
            "When filled, this exact message is sent to every lead. Placeholders you can use:\n" +
            "{{business_name}} {{category}} {{city}} {{sender_name}} {{portfolio_url}} {{rating}}"
          }
        />
        <div className="mt-2 rounded-lg border border-indigo-100 bg-indigo-50/60 p-3 text-xs text-slate-700">
          <p className="font-semibold text-indigo-900">🛡️ Anti-Ban Spintax Protection:</p>
          <p className="mt-0.5 text-slate-600">
            Wrap alternatives in curly braces like <code>{"{Hi|Hello|Hey}"}</code> or{" "}
            <code>{"{quick question|checking in}"}</code>. The engine automatically randomizes
            outgoing text for each recipient to prevent spam filters from flagging identical message
            patterns.
          </p>
        </div>
        <p className="mt-1 text-xs text-slate-500">
          Placeholders: <code>{"{{business_name}}"}</code>, <code>{"{{category}}"}</code>,{" "}
          <code>{"{{city}}"}</code>, <code>{"{{sender_name}}"}</code>,{" "}
          <code>{"{{portfolio_url}}"}</code>, <code>{"{{rating}}"}</code>
        </p>
      </div>

      {/* Multi-step cadence follow-up */}
      <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-sm font-semibold text-slate-900">
              Multi-Step Automated Follow-Up Cadence
            </h3>
            <p className="mt-0.5 text-xs text-slate-500">
              Automatically re-engage leads who haven&apos;t replied. As soon as a lead replies or
              is marked won/lost/blacklisted, all scheduled follow-ups are immediately canceled.
            </p>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={enableAutoFollowup}
            onClick={() => setEnableAutoFollowup((v) => !v)}
            className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 ${enableAutoFollowup ? "bg-indigo-600" : "bg-slate-200"}`}
          >
            <span
              className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ${enableAutoFollowup ? "translate-x-5" : "translate-x-0"}`}
            />
          </button>
        </div>

        {enableAutoFollowup && (
          <div className="space-y-4 pt-3 border-t border-slate-100">
            {/* Step 2: Follow-up #1 */}
            <div className="rounded-lg border border-slate-200 bg-slate-50/70 p-3.5 space-y-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label className="text-xs font-semibold uppercase tracking-wide text-indigo-700">
                  Step 2: Follow-up #1
                </label>
                <div className="flex items-center gap-1.5 text-xs text-slate-600">
                  <span>Send</span>
                  <input
                    type="number"
                    min={1}
                    max={30}
                    className="w-14 rounded border border-slate-300 bg-white px-2 py-0.5 text-center text-xs font-semibold text-slate-800"
                    value={followup1DelayDays}
                    onChange={(e) => setFollowup1DelayDays(Math.max(1, Number(e.target.value)))}
                  />
                  <span>days after initial pitch</span>
                </div>
              </div>
              <textarea
                rows={3}
                className={INPUT}
                value={followup1Template}
                onChange={(e) => setFollowup1Template(e.target.value)}
                placeholder="{Hi|Hey} {{business_name}}, just following up on our note regarding {{category}} services. Did you get a chance to take a look?"
              />
            </div>

            {/* Step 3: Follow-up #2 */}
            <div className="rounded-lg border border-slate-200 bg-slate-50/70 p-3.5 space-y-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label className="text-xs font-semibold uppercase tracking-wide text-indigo-700">
                  Step 3: Follow-up #2 (Break-up / Final)
                </label>
                <div className="flex items-center gap-1.5 text-xs text-slate-600">
                  <span>Send</span>
                  <input
                    type="number"
                    min={1}
                    max={60}
                    className="w-14 rounded border border-slate-300 bg-white px-2 py-0.5 text-center text-xs font-semibold text-slate-800"
                    value={followup2DelayDays}
                    onChange={(e) => setFollowup2DelayDays(Math.max(1, Number(e.target.value)))}
                  />
                  <span>days after Follow-up #1</span>
                </div>
              </div>
              <textarea
                rows={3}
                className={INPUT}
                value={followup2Template}
                onChange={(e) => setFollowup2Template(e.target.value)}
                placeholder="{Hi|Hello} {{business_name}}, I assume you are busy or not in the market right now. I will leave our link at {{portfolio_url}} if you ever need us. Best wishes!"
              />
            </div>
          </div>
        )}
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

function WhatsAppTab({
  config,
  onStatusChange,
}: {
  config: LeadEngineConfig;
  onStatusChange: (status: string, phone: string) => void;
}) {
  const base = getApiBaseUrl();

  const [status, setStatus] = useState(config.whatsappStatus);
  const [phone, setPhone] = useState(config.whatsappPhone);
  const [qr, setQr] = useState<string | null>(null);
  const [pairingCode, setPairingCode] = useState<string | null>(null);
  const [pairingPhone, setPairingPhone] = useState("");
  const [pairingError, setPairingError] = useState<string | null>(null);
  const [pairPhone, setPairPhone] = useState("");
  const [pairingBusy, setPairingBusy] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [source, setSource] = useState<string>("");
  // A background reconnect the dispatcher has already queued, plus the local
  // deadline so the tab can count down between polls.
  const [retry, setRetry] = useState<WhatsAppRetry | null>(null);
  const [retryDueAt, setRetryDueAt] = useState<number | null>(null);
  const [retrySeconds, setRetrySeconds] = useState(0);
  const [sessions, setSessions] = useState<WhatsAppSession[]>([]);
  const [sessionsError, setSessionsError] = useState(false);
  // Which option the operator just started, so only that button shows a spinner.
  const [pendingMode, setPendingMode] = useState<"own" | "shared" | null>(null);
  const mode: "own" | "shared" | "" = source === "own" || source === "shared" ? source : "";
  const retrying = retry !== null;

  // Keep the countdown honest between status polls. Repeated polls for the
  // same attempt must not push the deadline back up; a new attempt (backoff
  // grew) legitimately replaces it.
  const retryDueRef = useRef<number | null>(null);
  const retryAttemptRef = useRef<number | null>(null);
  const trackRetry = useCallback((next?: WhatsAppRetry) => {
    if (!next?.scheduled) {
      retryDueRef.current = null;
      retryAttemptRef.current = null;
      setRetry(null);
      setRetryDueAt(null);
      return;
    }
    const incoming = Date.now() + next.retryInMs;
    const sameAttempt = retryAttemptRef.current === next.attempt;
    const deadline =
      sameAttempt && retryDueRef.current !== null
        ? Math.min(retryDueRef.current, incoming)
        : incoming;
    retryDueRef.current = deadline;
    retryAttemptRef.current = next.attempt;
    setRetry(next);
    setRetryDueAt(deadline);
    setRetrySeconds(Math.max(0, Math.ceil((deadline - Date.now()) / 1000)));
  }, []);

  useEffect(() => {
    if (retryDueAt === null) return;
    const update = () => setRetrySeconds(Math.max(0, Math.ceil((retryDueAt - Date.now()) / 1000)));
    update();
    const id = setInterval(update, 1000);
    return () => clearInterval(id);
  }, [retryDueAt]);

  const loadSessions = useCallback(async () => {
    try {
      const res = await listWhatsAppSessions(base);
      setSessions(res.sessions ?? []);
      setSessionsError(false);
    } catch {
      // The audit list is informational; never break the tab over it.
      setSessionsError(true);
    }
  }, [base]);

  // Which number this workspace sends from (its own, or the shared fallback)
  // plus every paired session, loaded up front so the tab is truthful before
  // any button is pressed.
  useEffect(() => {
    void (async () => {
      try {
        const res = await getWhatsAppStatus(base, config.workspaceId);
        setStatus(res.status);
        trackRetry(res.retry);
        // Show a QR that is already waiting, instead of making the operator
        // wait for the first poll tick (or miss it entirely).
        setQr("qr" in res && res.qr ? res.qr : null);
        if ("phone" in res && res.phone) setPhone(res.phone);
        if (res.source) setSource(res.source);
        setPairingCode(res.pairingCode ?? null);
        setPairingError(res.pairingError ?? null);
        if (res.pairingPhone) {
          setPairingPhone(res.pairingPhone);
          setPairPhone((current) => current || res.pairingPhone || "");
        }
        onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
      } catch {}
      await loadSessions();
    })();
  }, [base, config.workspaceId, loadSessions, onStatusChange, trackRetry]);

  // Poll status while the session is changing — including a queued retry, which
  // is otherwise an "error" status the tab would stop watching.
  useEffect(() => {
    if (
      status === "qr_ready" ||
      status === "initializing" ||
      status === "authenticated" ||
      retrying
    ) {
      pollRef.current = setInterval(
        () => {
          void (async () => {
            try {
              const res = await getWhatsAppStatus(base, config.workspaceId);
              setStatus(res.status);
              trackRetry(res.retry);
              if ("phone" in res && res.phone) setPhone(res.phone);
              if ("qr" in res && res.qr) setQr(res.qr);
              if (res.pairingCode) setPairingCode(res.pairingCode);
              if (res.pairingError) setPairingError(res.pairingError);
              if (res.pairingPhone) {
                setPairingPhone(res.pairingPhone);
                setPairPhone((current) => current || res.pairingPhone || "");
              }
              onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
              if (res.status === "connected") {
                setPendingMode(null);
                setPairingCode(null);
                setPairingError(null);
                if (pollRef.current) clearInterval(pollRef.current);
              }
            } catch {}
          })();
        },
        retrying ? 2000 : 4000,
      );
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [status, retrying, base, config.workspaceId, onStatusChange, trackRetry]);

  // Pair this workspace's own business number, so it stops sending from the
  // account another workspace may also be using.
  const connectOwn = async () => {
    setError(null);
    setBusy(true);
    setPendingMode("own");
    // Drop any previous QR so the panel cannot show a code from an old attempt.
    setQr(null);
    setPairingCode(null);
    setPairingError(null);
    try {
      const res = await connectOwnWhatsApp(base, config.workspaceId);
      setStatus(res.status);
      trackRetry(res.retry);
      if (res.status === "connected") setPendingMode(null);
      if ("qr" in res && res.qr) setQr(res.qr);
      if ("phone" in res && res.phone) setPhone(res.phone ?? "");
      if (res.source) setSource(res.source);
      if (res.pairingCode) setPairingCode(res.pairingCode);
      if (res.pairingPhone) {
        setPairingPhone(res.pairingPhone);
        setPairPhone((current) => current || res.pairingPhone || "");
      }
      setPairingError(res.pairingError ?? null);
      onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
      void loadSessions();
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
      trackRetry(undefined);
      setPhone("");
      setQr(null);
      setSource("");
      setPendingMode(null);
      setPairingCode(null);
      setPairingPhone("");
      setPairingError(null);
      onStatusChange("disconnected", "");
      void loadSessions();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Disconnect failed");
    } finally {
      setBusy(false);
    }
  };

  // Move this workspace from its own number back onto the shared account.
  // Disconnecting clears the own-number claim in the dispatcher, so the next
  // start resolves to the shared session that is already paired.
  const switchToSharedNumber = async () => {
    if (
      !window.confirm(
        "Switch this workspace to the shared WhatsApp number? Its own number is disconnected first, so any send in progress from it stops.",
      )
    ) {
      return;
    }
    setError(null);
    setBusy(true);
    setPendingMode("shared");
    try {
      try {
        await disconnectWhatsApp(base, config.workspaceId);
      } catch {
        // Not connected to its own number: there is nothing to release.
      }
      const res = await connectWhatsApp(base, config.workspaceId);
      setStatus(res.status);
      trackRetry(res.retry);
      if ("phone" in res && res.phone) setPhone(res.phone ?? "");
      if (res.source) setSource(res.source);
      setPairingCode(null);
      setPairingError(null);
      if (res.status === "connected") setPendingMode(null);
      onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
      void loadSessions();
    } catch (err) {
      setPendingMode(null);
      setError(err instanceof Error ? err.message : "Could not switch to the shared number");
    } finally {
      setBusy(false);
    }
  };

  // Ask WhatsApp for a pairing code on the same session the QR belongs to.
  // Only digits are sent; the country code is required by WhatsApp.
  const requestPairingCode = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setPairingError(null);
    const digits = pairPhone.replace(/[^0-9]/g, "");
    if (digits.length < 8 || digits.length > 15) {
      setPairingError("Enter the full phone number including its country code (8–15 digits).");
      return;
    }
    setPairingBusy(true);
    setPairingPhone(digits);
    setPairPhone(digits);
    try {
      const res = await pairOwnWhatsApp(base, config.workspaceId, digits);
      setStatus(res.status);
      trackRetry(res.retry);
      if (res.status === "connected") {
        setPendingMode(null);
        setPairingCode(null);
      }
      if ("qr" in res && res.qr) setQr(res.qr);
      if ("phone" in res && res.phone) setPhone(res.phone ?? "");
      if (res.source) setSource(res.source);
      if (res.pairingCode) setPairingCode(res.pairingCode);
      if (res.pairingPhone) setPairingPhone(res.pairingPhone);
      setPairingError(res.pairingError ?? null);
      onStatusChange(res.status, "phone" in res ? (res.phone ?? "") : "");
      void loadSessions();
    } catch (err) {
      setPairingError(err instanceof Error ? err.message : "Could not request a pairing code");
    } finally {
      setPairingBusy(false);
    }
  };

  return (
    <div className="grid gap-6">
      {/* Connection summary: one glance tells you which number sends and whether it is live. */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
        <div className="flex items-center gap-3">
          <span
            className={`h-2.5 w-2.5 rounded-full ${
              status === "connected"
                ? "bg-emerald-500"
                : retrying || status === "qr_ready"
                  ? "bg-amber-500"
                  : "bg-slate-300"
            } ${retrying ? "animate-pulse motion-reduce:animate-none" : ""}`}
            aria-hidden="true"
          />
          <div>
            <p className="text-sm font-semibold text-slate-900" role="status" aria-live="polite">
              {retrying
                ? "Reconnecting to WhatsApp"
                : status === "connected"
                  ? "Connected and ready to send"
                  : whatsAppStatusLabel(status)}
            </p>
            <p className="mt-0.5 text-xs text-slate-500">
              {phone ? <span className="font-mono">{phone}</span> : "No number paired"}
              {" · "}
              {mode === "own"
                ? "this workspace's own number"
                : mode === "shared"
                  ? "shared team account"
                  : "connection mode not set"}
            </p>
            {retrying ? (
              <p className="mt-1 text-xs font-semibold text-amber-800">
                Retry in {retrySeconds}s · attempt {retry?.attempt} of ∞ · saved pairing reused
              </p>
            ) : null}
          </div>
        </div>
        <StatusBadge status={status} />
      </div>

      {/* Two equally valid ways to send. Both can run at the same time across workspaces. */}
      <div className="grid gap-4 sm:grid-cols-2">
        <article
          aria-label="This workspace's own WhatsApp number"
          className={`flex flex-col rounded-xl border p-4 ${
            mode === "own"
              ? "border-emerald-300 bg-emerald-50/40 ring-1 ring-emerald-200"
              : "border-slate-200 bg-white"
          }`}
        >
          <div className="flex items-start justify-between gap-2">
            <h3 className="text-sm font-semibold text-slate-900">
              This workspace&apos;s own number
            </h3>
            {mode === "own" && status === "connected" ? (
              <ModeChip tone="emerald">Active</ModeChip>
            ) : (
              <ModeChip tone="slate">One-time pairing</ModeChip>
            )}
          </div>
          <p className="mt-2 flex-1 text-xs leading-relaxed text-slate-600">
            Pair the client&apos;s business number once by scanning a QR code or entering a code
            WhatsApp sends to the phone. It is saved on this server and reused on every restart.
            Only this workspace sends from it.
          </p>
          {mode === "own" && phone && (
            <p className="mt-3 font-mono text-xs text-slate-700">{phone}</p>
          )}
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => void connectOwn()}
              disabled={busy}
              className={mode === "own" ? BTN_GHOST : BTN_PRIMARY}
            >
              {busy && pendingMode === "own" ? (
                <>
                  <Spinner size={14} />
                  <span className="ml-2">Starting…</span>
                </>
              ) : mode === "own" ? (
                "Reconnect own number"
              ) : (
                "Connect own number"
              )}
            </button>
            {mode === "own" && (
              <button
                type="button"
                onClick={() => void disconnect()}
                disabled={busy}
                className="inline-flex items-center justify-center rounded-lg px-3 py-2 text-xs font-semibold text-red-700 hover:bg-red-50 disabled:opacity-50"
              >
                {busy ? "Disconnecting…" : "Disconnect"}
              </button>
            )}
          </div>
        </article>

        <article
          aria-label="Shared team WhatsApp number"
          className={`flex flex-col rounded-xl border p-4 ${
            mode === "shared"
              ? "border-indigo-300 bg-indigo-50/40 ring-1 ring-indigo-200"
              : "border-slate-200 bg-white"
          }`}
        >
          <div className="flex items-start justify-between gap-2">
            <h3 className="text-sm font-semibold text-slate-900">Shared team number</h3>
            {mode === "shared" && status === "connected" ? (
              <ModeChip tone="emerald">Active</ModeChip>
            ) : (
              <ModeChip tone="slate">Fallback</ModeChip>
            )}
          </div>
          <p className="mt-2 flex-1 text-xs leading-relaxed text-slate-600">
            The number already paired on this server. Workspaces without their own number send from
            it, and several can use it at the same time while they are being onboarded.
          </p>
          {mode === "shared" && phone && (
            <p className="mt-3 font-mono text-xs text-slate-700">{phone}</p>
          )}
          <div className="mt-4 flex flex-wrap items-center gap-2">
            {mode === "shared" ? (
              <p className="text-xs text-slate-500">
                Managed for the whole team — disconnecting it here would stop every workspace using
                it.
              </p>
            ) : (
              <button
                type="button"
                onClick={() => void switchToSharedNumber()}
                disabled={busy}
                className={BTN_GHOST}
              >
                {busy && pendingMode === "shared" ? (
                  <>
                    <Spinner size={14} />
                    <span className="ml-2">Switching…</span>
                  </>
                ) : mode === "own" ? (
                  "Switch to shared number"
                ) : (
                  "Use shared number"
                )}
              </button>
            )}
          </div>
        </article>
      </div>

      {status === "qr_ready" ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="grid content-start gap-3">
            <div>
              <h3 className="text-sm font-semibold text-slate-900">
                {mode === "own"
                  ? "This workspace's own number — scan to pair"
                  : mode === "shared"
                    ? "Shared team number — scan to pair"
                    : "Pair a WhatsApp number"}
              </h3>
              <p className="mt-1 text-xs leading-relaxed text-slate-600">
                {phone ? (
                  <>
                    Open WhatsApp on <span className="font-mono">{phone}</span> → Linked devices →
                    Link a device, then scan this code.
                  </>
                ) : (
                  <>
                    Open the phone that owns this number → WhatsApp → Linked devices → Link a
                    device, then scan this code.
                  </>
                )}
              </p>
            </div>
            <Alert kind="warn">
              Scan the QR code, or ask WhatsApp to send a code to the phone number. Either method
              pairs this same saved session.
            </Alert>
            {qr ? (
              <div className="flex justify-center rounded-xl border border-slate-200 bg-white p-4">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={qr} alt="WhatsApp QR code" className="max-w-xs w-full" />
              </div>
            ) : (
              <div className="flex items-center justify-center rounded-xl border border-slate-200 bg-white px-4 py-10 text-sm text-slate-500">
                Preparing the QR code…
              </div>
            )}
            <p className="text-center text-xs text-slate-500 animate-pulse motion-reduce:animate-none">
              Waiting for a scan or a pairing code…
            </p>
          </div>

          <form
            onSubmit={(e) => void requestPairingCode(e)}
            className="grid content-start gap-3 rounded-xl border border-slate-200 bg-white p-4"
          >
            <div>
              <h3 className="text-sm font-semibold text-slate-900">Pair with phone number</h3>
              <p className="mt-1 text-xs leading-relaxed text-slate-600">
                Enter the number that has WhatsApp, including the country code. WhatsApp sends an
                8-character code — open WhatsApp → Linked devices → Link with phone number and enter
                it. The QR code stays available as the other option.
              </p>
            </div>
            <div>
              <label htmlFor="whatsapp-pair-phone" className={LABEL}>
                Phone number
              </label>
              <input
                id="whatsapp-pair-phone"
                type="tel"
                inputMode="numeric"
                autoComplete="tel"
                value={pairPhone}
                onChange={(e) => setPairPhone(e.target.value)}
                placeholder="233241234567"
                className={INPUT}
              />
              <p className="mt-1 text-xs text-slate-500">
                Digits only, with country code. Example: 233241234567.
              </p>
            </div>
            <button
              type="submit"
              disabled={pairingBusy || !pairPhone.trim()}
              className={BTN_PRIMARY}
            >
              {pairingBusy ? (
                <>
                  <Spinner size={14} />
                  <span className="ml-2">Requesting code…</span>
                </>
              ) : (
                "Get pairing code"
              )}
            </button>

            {pairingCode ? (
              <div
                role="status"
                className="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3"
              >
                <p className="text-xs font-semibold uppercase tracking-wide text-emerald-800">
                  Enter this code in WhatsApp
                </p>
                <p className="mt-1 font-mono text-2xl font-bold tracking-[0.18em] text-emerald-950">
                  {formatPairingCode(pairingCode)}
                </p>
                {pairingPhone ? (
                  <p className="mt-1 font-mono text-xs text-emerald-800">+{pairingPhone}</p>
                ) : null}
                <p className="mt-2 text-xs text-emerald-800">
                  WhatsApp refreshes the code automatically. You can still scan the QR on the left.
                </p>
              </div>
            ) : pairingPhone && !pairingError ? (
              <p
                role="status"
                className="flex items-center gap-2 rounded-lg bg-slate-50 px-3 py-2 text-xs text-slate-600"
              >
                <Spinner size={14} />
                Requesting a code for +{pairingPhone}…
              </p>
            ) : null}

            {pairingError ? <Alert kind="error">{pairingError}</Alert> : null}
            <p className="text-xs text-slate-500">
              No code yet? Keep this panel open; it updates when WhatsApp responds.
            </p>
          </form>
        </div>
      ) : status === "initializing" || status === "authenticated" ? (
        <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-600">
          <Spinner />
          <span>Connecting to WhatsApp…</span>
        </div>
      ) : retrying ? (
        <div
          role="status"
          aria-live="polite"
          className="flex flex-wrap items-center gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
        >
          <Spinner />
          <span>
            Reconnecting in <span className="font-semibold">{retrySeconds}s</span> (attempt{" "}
            {retry?.attempt}). The saved pairing is reused — no scan needed unless WhatsApp revoked
            it.
          </span>
        </div>
      ) : status !== "connected" ? (
        <Alert kind="info">
          Choose a number above to enable automated outreach. Pair once by scanning a QR code or
          entering a code sent to the phone; every later startup reconnects on its own.
        </Alert>
      ) : null}

      {error && <Alert kind="error">{error}</Alert>}

      {/* Concurrency made visible: every paired workspace appears with its own number. */}
      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-4 py-3">
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-widest text-slate-600">
              Running now
            </h3>
            <p className="mt-0.5 text-xs text-slate-500">
              Every workspace keeps its own session. Open another workspace in a new tab and both
              keep running.
            </p>
          </div>
          <button
            type="button"
            onClick={() => void loadSessions()}
            className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-semibold text-slate-600 hover:bg-slate-50"
          >
            <RefreshIcon />
            Refresh
          </button>
        </div>
        {sessionsError ? (
          <p className="px-4 py-4 text-xs text-slate-500">
            Live pairing status is unavailable right now. The dispatcher may still be starting —
            this tab keeps working meanwhile.
          </p>
        ) : sessions.length === 0 ? (
          <p className="px-4 py-4 text-xs text-slate-500">
            No WhatsApp session is live yet. Pair a number above to start one.
          </p>
        ) : (
          <ul className="divide-y divide-slate-100">
            {sessions.map((s) => {
              const current = s.workspaceId === config.workspaceId;
              return (
                <li
                  key={s.workspaceId || s.source}
                  className="flex flex-wrap items-center justify-between gap-2 px-4 py-3"
                >
                  <div className="flex min-w-0 items-center gap-2.5">
                    <span
                      className={`h-2 w-2 shrink-0 rounded-full ${
                        s.connected ? "bg-emerald-500" : "bg-slate-300"
                      }`}
                      aria-hidden="true"
                    />
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-slate-800">
                        {s.workspaceName}
                        {current && (
                          <span className="ml-2 text-[11px] font-semibold uppercase tracking-wide text-indigo-600">
                            this workspace
                          </span>
                        )}
                      </p>
                      <p className="mt-0.5 text-xs text-slate-500">
                        <span className="font-mono">{s.phone || "no number"}</span>
                        {" · "}
                        {s.source === "own" ? "own number" : "shared number"}
                        {" · "}
                        {whatsAppStatusLabel(s.status)}
                      </p>
                    </div>
                  </div>
                  {!current && s.source === "own" && s.workspaceId && (
                    <Link
                      href={`/workspaces/${encodeURIComponent(s.workspaceId)}`}
                      target="_blank"
                      rel="noreferrer"
                      className="ml-auto rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-semibold text-slate-600 transition-colors hover:border-indigo-200 hover:text-indigo-700"
                    >
                      Open ↗
                    </Link>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

// ─── Leads tab ────────────────────────────────────────────────────────────────

// Local midnight for "today", so a day boundary is the operator's midnight,
// not UTC's.
function startOfLocalDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

// ─── Persisted list preferences ───────────────────────────────────────────────

// Which outcome filter and day range the operator last chose, kept per
// workspace so one workspace's review view never leaks into another's.
type LeadFilter = "all" | "sent" | "not_sent" | "skipped";

const leadFilterKey = (workspaceId: string) => `leadEngine.sendFilter:${workspaceId}`;
const showPreviousKey = (workspaceId: string) => `leadEngine.showPrevious:${workspaceId}`;

// These are read after mount, never during render: reading localStorage in a
// useState initializer yields different markup on the server and the client —
// exactly the hydration mismatch this page has to avoid.
function readLeadFilter(workspaceId: string): LeadFilter {
  try {
    const stored = window.localStorage.getItem(leadFilterKey(workspaceId));
    if (stored === "all" || stored === "sent" || stored === "not_sent" || stored === "skipped") {
      return stored;
    }
  } catch {
    // Storage disabled or private mode: fall back to the default view.
  }
  return "all";
}

function readShowPrevious(workspaceId: string): boolean {
  try {
    return window.localStorage.getItem(showPreviousKey(workspaceId)) === "1";
  } catch {
    return false;
  }
}

function storePreference(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // A saved preference is a convenience; full or disabled storage must not
    // break the page.
  }
}

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
  // Each day starts on a clean slate: the table shows leads found today plus
  // anything still waiting to be sent. Earlier failed/blacklisted rows stay in
  // the database (the send-side dedupe needs them) but are hidden by default.
  const [showPrevious, setShowPrevious] = useState(false);
  // Outcome filter: review only what the send run did — delivered, failed, or
  // skipped — instead of scrolling the whole day's list.
  const [sendFilter, setSendFilter] = useState<LeadFilter>("all");
  const [dayStart, setDayStart] = useState(() => startOfLocalDay(new Date()));

  // CSV Import state
  const [showImport, setShowImport] = useState(false);
  const [importing, setImporting] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [importResult, setImportResult] = useState<{
    total: number;
    imported: number;
    duplicates: number;
    failed: number;
  } | null>(null);
  const [parsedLeads, setParsedLeads] = useState<
    Array<{
      name: string;
      phone: string;
      category?: string;
      city?: string;
      address?: string;
      websiteUrl?: string;
    }>
  >([]);

  const handleCsvFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setImportError(null);
    setImportResult(null);

    const reader = new FileReader();
    reader.onload = (event) => {
      try {
        const text = String(event.target?.result ?? "");
        const rows = parseLeadEngineCsv(text);
        if (rows.length === 0) {
          setImportError(
            "No valid rows found in CSV. Ensure columns include headers like 'Name' and 'Phone'.",
          );
          setParsedLeads([]);
          return;
        }
        setParsedLeads(rows);
      } catch (err) {
        setImportError(err instanceof Error ? err.message : "Failed to parse CSV file");
        setParsedLeads([]);
      }
    };
    reader.onerror = () => {
      setImportError("Could not read file");
      setParsedLeads([]);
    };
    reader.readAsText(file);
    e.target.value = "";
  };

  const executeImport = async () => {
    if (parsedLeads.length === 0) return;
    setImporting(true);
    setImportError(null);
    try {
      const res = await importLeadEngineLeads(base, config.workspaceId, parsedLeads);
      setImportResult(res);
      setParsedLeads([]);
      void loadLeads();
    } catch (err) {
      setImportError(err instanceof Error ? err.message : "Import failed");
    } finally {
      setImporting(false);
    }
  };

  const handleExportCsv = () => {
    if (leads.length === 0) return;
    exportLeadEngineLeadsToCsv(leads, config.workspaceId);
  };

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

  // Restore the saved view once hydrated, so a refresh lands on the same filter
  // and day range instead of resetting to "today, all".
  useEffect(() => {
    setSendFilter(readLeadFilter(config.workspaceId));
    setShowPrevious(readShowPrevious(config.workspaceId));
  }, [config.workspaceId]);

  const chooseSendFilter = (value: LeadFilter) => {
    setSendFilter(value);
    storePreference(leadFilterKey(config.workspaceId), value);
  };

  const toggleShowPrevious = () => {
    const next = !showPrevious;
    setShowPrevious(next);
    storePreference(showPreviousKey(config.workspaceId), next ? "1" : "0");
  };

  // Roll the view over at local midnight and refetch, so a tab left open
  // overnight starts the new day empty on its own. This only reads — it does
  // not touch the sender, the dispatcher, or an in-flight batch.
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    const schedule = () => {
      const now = new Date();
      const nextMidnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1).getTime();
      timer = setTimeout(
        () => {
          setDayStart(startOfLocalDay(new Date()));
          setShowPrevious(false);
          storePreference(showPreviousKey(config.workspaceId), "0");
          void loadLeads();
          schedule();
        },
        Math.max(1000, nextMidnight - now.getTime() + 1000),
      );
    };
    schedule();
    return () => clearTimeout(timer);
  }, [loadLeads, config.workspaceId]);

  useWorkspaceEvents(config.workspaceId, (ev) => {
    // The run summary replaces "Dispatch started…" once the background batch
    // finishes, and explains anything the per-lead reasons cannot (daily cap).
    if (ev.type === "pitch_dispatched" && ev.message?.startsWith("Dispatch run ended with error")) {
      setError(ev.message);
    }
    if (ev.type === "pitch_dispatched" && ev.message?.startsWith("Outreach dispatch completed")) {
      setNotice(ev.message);
    }
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

  // Today's working set: leads recorded today, plus any lead still waiting to
  // be sent (an overnight queue must stay visible), never yesterday's failed
  // or blacklisted rows unless the operator explicitly asks for them.
  const visibleLeads = showPrevious
    ? leads
    : leads.filter(
        (lead) =>
          lead.status === "discovered" ||
          lead.status === "pending" ||
          new Date(lead.createdAt).getTime() >= dayStart,
      );
  const hiddenCount = leads.length - visibleLeads.length;

  const pending = visibleLeads.filter(
    (l) => l.status === "discovered" || l.status === "pending",
  ).length;
  // Count from the send log where it exists: a lead whose status update did
  // not stick is still counted as sent once the dispatcher accepted it.
  const wasSent = (l: LeadEngineLead) =>
    ["sent", "delivered", "replied", "demo_sent"].includes(l.status) || l.lastSendStatus === "sent";
  const sent = visibleLeads.filter(wasSent).length;
  const failed = visibleLeads.filter(
    (l) => !wasSent(l) && (l.status === "failed" || l.lastSendStatus === "failed"),
  ).length;
  const isSkipped = (l: LeadEngineLead) => Boolean(l.skipReason);
  // "Not sent" = an attempt was made but nothing went out (failed or never
  // confirmed), excluding leads the dispatcher skipped before trying.
  const notSent = (l: LeadEngineLead) =>
    !wasSent(l) &&
    !isSkipped(l) &&
    (l.status === "failed" || l.lastSendStatus === "failed" || l.lastSendStatus === "sending");
  const skippedCount = visibleLeads.filter(isSkipped).length;
  const notSentCount = visibleLeads.filter(notSent).length;
  const filteredLeads =
    sendFilter === "all"
      ? visibleLeads
      : sendFilter === "sent"
        ? visibleLeads.filter(wasSent)
        : sendFilter === "not_sent"
          ? visibleLeads.filter(notSent)
          : visibleLeads.filter(isSkipped);
  const filterLabel = { all: "All", sent: "Sent", not_sent: "Not sent", skipped: "Skipped" }[
    sendFilter
  ];

  return (
    <div className="grid gap-5">
      {/* action bar */}
      <div className="flex flex-wrap gap-3">
        <button
          onClick={() => void discover()}
          disabled={discovering || !config.hasGooglePlacesKey}
          title={
            !config.hasGooglePlacesKey ? "Configure Google Places API key in Setup" : undefined
          }
          className={BTN_PRIMARY}
        >
          {discovering ? (
            <>
              <Spinner size={14} />
              <span className="ml-2">Discovering…</span>
            </>
          ) : (
            "🔍 Run Discovery"
          )}
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
          {dispatching ? (
            <>
              <Spinner size={14} />
              <span className="ml-2">Sending…</span>
            </>
          ) : (
            `📤 Send Pending (${pending})`
          )}
        </button>
        <button
          type="button"
          onClick={() => {
            setShowImport((v) => !v);
            setImportError(null);
            setImportResult(null);
          }}
          className={BTN_GHOST}
        >
          {showImport ? "✕ Close Import" : "📥 Import CSV"}
        </button>
        <button
          type="button"
          onClick={handleExportCsv}
          disabled={leads.length === 0}
          title={leads.length === 0 ? "No leads to export" : "Export leads to CSV"}
          className={BTN_GHOST}
        >
          📤 Export CSV ({leads.length})
        </button>
        <button onClick={() => void loadLeads()} disabled={loading} className={BTN_GHOST}>
          {loading ? <Spinner size={14} /> : "↻ Refresh"}
        </button>
      </div>

      {/* CSV Import Dropdown Panel */}
      {showImport && (
        <div className="rounded-xl border border-indigo-200 bg-indigo-50/40 p-4 space-y-3">
          <div className="flex items-start justify-between gap-2">
            <div>
              <h4 className="text-sm font-semibold text-slate-900">Bulk Import Leads via CSV</h4>
              <p className="mt-0.5 text-xs text-slate-600">
                Upload a CSV with columns such as <code>Name</code>, <code>Phone</code>,{" "}
                <code>Category</code>, <code>City</code>, <code>Address</code>, and{" "}
                <code>Website</code>. Duplicates by phone number will be automatically skipped.
              </p>
            </div>
            <button
              type="button"
              onClick={() => setShowImport(false)}
              className="text-xs text-slate-400 hover:text-slate-600 font-semibold"
            >
              ✕ Close
            </button>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <label className="cursor-pointer rounded-lg bg-indigo-600 px-3.5 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors">
              Select CSV File
              <input
                type="file"
                accept=".csv,text/csv"
                onChange={handleCsvFileChange}
                disabled={importing}
                className="sr-only"
              />
            </label>
            <span className="text-xs text-slate-500">
              {parsedLeads.length > 0
                ? `${parsedLeads.length} leads parsed and ready to import`
                : "Supports UTF-8 CSV with Name and Phone headers"}
            </span>
          </div>

          {importError && <Alert kind="error">{importError}</Alert>}
          {importResult && (
            <Alert kind="success">
              Import complete: {importResult.imported} leads added, {importResult.duplicates}{" "}
              duplicates skipped, {importResult.failed} failed.
            </Alert>
          )}

          {parsedLeads.length > 0 && (
            <div className="space-y-3 pt-2">
              <div className="max-h-48 overflow-y-auto rounded-lg border border-slate-200 bg-white">
                <table className="min-w-full text-xs">
                  <thead>
                    <tr className="border-b border-slate-200 bg-slate-50 text-left text-slate-500 font-semibold">
                      <th className="px-3 py-2">Name</th>
                      <th className="px-3 py-2">Phone</th>
                      <th className="px-3 py-2">Category</th>
                      <th className="px-3 py-2">City</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {parsedLeads.slice(0, 10).map((row, idx) => (
                      <tr key={idx} className="hover:bg-slate-50">
                        <td className="px-3 py-1.5 font-medium text-slate-800">{row.name}</td>
                        <td className="px-3 py-1.5 font-mono text-slate-600">{row.phone}</td>
                        <td className="px-3 py-1.5 text-slate-500">{row.category || "—"}</td>
                        <td className="px-3 py-1.5 text-slate-500">{row.city || "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {parsedLeads.length > 10 && (
                <p className="text-[11px] text-slate-400">
                  Showing first 10 of {parsedLeads.length} rows preview.
                </p>
              )}

              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => void executeImport()}
                  disabled={importing}
                  className={BTN_PRIMARY}
                >
                  {importing ? (
                    <>
                      <Spinner size={14} />
                      <span className="ml-2">Importing…</span>
                    </>
                  ) : (
                    `Confirm & Import ${parsedLeads.length} Leads`
                  )}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setParsedLeads([]);
                    setImportError(null);
                  }}
                  disabled={importing}
                  className={BTN_GHOST}
                >
                  Clear
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* stats */}
      {visibleLeads.length > 0 && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[
            { label: "Today", value: visibleLeads.length, color: "text-slate-700" },
            { label: "Pending", value: pending, color: "text-amber-700" },
            { label: "Sent", value: sent, color: "text-emerald-700" },
            { label: "Failed", value: failed, color: "text-red-600" },
          ].map(({ label, value, color }) => (
            <div
              key={label}
              className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-center"
            >
              <p className={`text-2xl font-bold ${color}`}>{value}</p>
              <p className="text-xs text-slate-500">{label}</p>
            </div>
          ))}
        </div>
      )}

      {/* outcome filter */}
      {visibleLeads.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-semibold uppercase tracking-wide text-slate-400">Show</span>
          {(
            [
              { id: "all", label: "All", count: visibleLeads.length },
              { id: "sent", label: "Sent", count: sent },
              { id: "not_sent", label: "Not sent", count: notSentCount },
              { id: "skipped", label: "Skipped", count: skippedCount },
            ] as const
          ).map((option) => (
            <button
              key={option.id}
              type="button"
              onClick={() => chooseSendFilter(option.id)}
              aria-pressed={sendFilter === option.id}
              className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
                sendFilter === option.id
                  ? "bg-indigo-600 text-white"
                  : "border border-slate-200 bg-white text-slate-600 hover:bg-slate-50"
              }`}
            >
              {option.label} ({option.count})
            </button>
          ))}
        </div>
      )}

      {leads.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 bg-white px-4 py-2.5">
          <p className="text-xs text-slate-500">
            {showPrevious
              ? `Showing every lead recorded for this workspace (${leads.length}).`
              : hiddenCount > 0
                ? `Today's leads. ${hiddenCount} earlier failed/blacklisted row${hiddenCount === 1 ? " is" : "s are"} hidden.`
                : "Today's leads — this list clears itself each day."}
          </p>
          <button type="button" onClick={toggleShowPrevious} className={BTN_GHOST}>
            {showPrevious ? "Show today only" : `Show previous days (${leads.length})`}
          </button>
        </div>
      )}

      {error && <Alert kind="error">{error}</Alert>}
      {notice && <Alert kind="info">{notice}</Alert>}

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-slate-500 py-6">
          <Spinner /> Loading leads…
        </div>
      ) : filteredLeads.length === 0 ? (
        <Alert kind="info">
          {visibleLeads.length > 0
            ? `No leads match the “${filterLabel}” filter. Choose “All” to see every lead.`
            : hiddenCount > 0
              ? `Nothing new today. ${hiddenCount} row${hiddenCount === 1 ? "" : "s"} from previous days (sent, failed, blacklisted) ${hiddenCount === 1 ? "is" : "are"} hidden — use “Show previous days” to review them.`
              : 'No leads discovered yet. Click "Run Discovery" to scan your target categories and cities. If a run returns nothing, double-check your Google Places API key in Setup.'}
        </Alert>
      ) : (
        <>
          {/* Mobile card view */}
          <div className="grid gap-3 md:hidden">
            {filteredLeads.map((lead) => (
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
                  <div className="shrink-0 text-right">
                    <StatusBadge status={lead.status} />
                    <SendConfirmation lead={lead} />
                  </div>
                </div>

                <div className="mt-2.5 flex flex-wrap items-center gap-2 text-xs text-slate-500">
                  <span className="rounded bg-slate-100 px-2 py-0.5 font-medium text-slate-700">
                    {lead.category}
                  </span>
                  {lead.city && <span>📍 {lead.city}</span>}
                  {lead.rating ? (
                    <span className="text-amber-600 font-medium">⭐ {lead.rating.toFixed(1)}</span>
                  ) : null}
                  {lead.sourceDetail && (
                    <span
                      className="rounded border border-slate-200 px-1.5 py-0.5 text-[10px] font-semibold text-slate-500"
                      title="Where this lead was discovered"
                    >
                      {DISCOVERY_SOURCE_LABELS[lead.sourceDetail] ?? lead.sourceDetail}
                    </span>
                  )}
                  <span className="rounded bg-indigo-50 border border-indigo-200 px-1.5 py-0.5 text-[10px] font-semibold text-indigo-700">
                    {lead.sequenceStep === 2
                      ? "Step 2: Follow-up 1"
                      : lead.sequenceStep && lead.sequenceStep >= 3
                        ? "Step 3: Follow-up 2"
                        : "Step 1: Pitch"}
                  </span>{" "}
                  {lead.nextFollowupAt && ["sent", "delivered"].includes(lead.status) && (
                    <span className="text-[11px] text-indigo-600 font-medium">
                      Next: {new Date(lead.nextFollowupAt).toLocaleDateString()}
                    </span>
                  )}
                  {lead.skipReason && (
                    <span className="rounded border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700">
                      Skipped: {lead.skipReason}
                    </span>
                  )}
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
                              <span className="font-mono">
                                {new Date(log.createdAt).toLocaleTimeString()}
                              </span>
                              <span className="uppercase font-semibold">{log.direction}</span>
                              <StatusBadge status={log.status} />
                            </div>
                            {log.body && <p className="mt-1 text-slate-700">{log.body}</p>}
                            {log.error && (
                              <p className="mt-1 text-red-600 font-medium">Error: {log.error}</p>
                            )}
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
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Business
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Category
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 hidden sm:table-cell">
                    City
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500 hidden md:table-cell">
                    Rating
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Cadence
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Status
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {filteredLeads.map((lead) => (
                  <Fragment key={lead.id}>
                    <tr className="border-b border-slate-100 hover:bg-slate-50 transition-colors">
                      <td className="px-4 py-3">
                        <p className="font-medium text-slate-900">{lead.name}</p>
                        <p className="text-xs text-slate-400">{lead.phone}</p>
                        {lead.sourceDetail && (
                          <p className="text-[10px] text-slate-400">
                            {DISCOVERY_SOURCE_LABELS[lead.sourceDetail] ?? lead.sourceDetail}
                          </p>
                        )}
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
                        <span className="inline-flex items-center rounded bg-indigo-50 px-2 py-0.5 text-xs font-semibold text-indigo-700">
                          {lead.sequenceStep === 2
                            ? "Step 2: Follow-up 1"
                            : lead.sequenceStep && lead.sequenceStep >= 3
                              ? "Step 3: Follow-up 2"
                              : "Step 1: Pitch"}
                        </span>
                        {lead.nextFollowupAt && ["sent", "delivered"].includes(lead.status) && (
                          <p className="mt-0.5 text-[11px] text-slate-500">
                            Due: {new Date(lead.nextFollowupAt).toLocaleDateString()}
                          </p>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge status={lead.status} />
                        <SendConfirmation lead={lead} />
                        {lead.skipReason && (
                          <p
                            className="mt-1 max-w-[200px] text-[11px] leading-snug text-amber-700"
                            title={`Skipped: ${lead.skipReason}`}
                          >
                            Skipped: {lead.skipReason}
                          </p>
                        )}
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
                        <td colSpan={7} className="bg-slate-50 px-4 py-3">
                          {lead.pitch && (
                            <div className="mb-3 rounded-lg border border-slate-200 bg-white p-3">
                              <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">
                                Pitch
                              </p>
                              <p className="text-sm text-slate-700 whitespace-pre-wrap">
                                {lead.pitch}
                              </p>
                            </div>
                          )}
                          {loadingLogs ? (
                            <div className="flex items-center gap-2 text-sm text-slate-500">
                              <Spinner size={14} /> Loading logs…
                            </div>
                          ) : logs.length === 0 ? (
                            <p className="text-xs text-slate-400">
                              No outreach logs for this lead.
                            </p>
                          ) : (
                            <div className="grid gap-2">
                              {logs.map((log) => (
                                <div
                                  key={log.id}
                                  className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs"
                                >
                                  <div className="flex items-center gap-2 text-slate-500">
                                    <span className="font-mono">
                                      {new Date(log.createdAt).toLocaleString()}
                                    </span>
                                    <span className="uppercase">{log.direction}</span>
                                    <StatusBadge status={log.status} />
                                  </div>
                                  {log.body && <p className="mt-1 text-slate-700">{log.body}</p>}
                                  {log.error && (
                                    <p className="mt-1 text-red-600">Error: {log.error}</p>
                                  )}
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

  useEffect(() => {
    void load();
  }, [load]);

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
            {adding ? (
              <>
                <Spinner size={14} />
                <span className="ml-2">Adding…</span>
              </>
            ) : (
              "Add to blacklist"
            )}
          </button>
        </div>
      </form>

      {error && <Alert kind="error">{error}</Alert>}

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-slate-500">
          <Spinner /> Loading…
        </div>
      ) : list.length === 0 ? (
        <Alert kind="info">No suppressed numbers yet.</Alert>
      ) : (
        <>
          {/* Mobile card view */}
          <div className="grid gap-2 sm:hidden">
            {list.map((entry) => (
              <div
                key={`m-bl-${entry.id}`}
                className="rounded-lg border border-slate-200 bg-white p-3"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-sm font-semibold text-slate-800">
                    {entry.phoneNumber}
                  </span>
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
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Phone
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Reason
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                    Added
                  </th>
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

// ─── Follow-up tab ──────────────────────────────────────────────────────

// Compose one message, tick the contacts this workspace ALREADY messaged,
// and start the run. Sends go out in the background at the workspace's
// pacing (2–5 min apart), so the click starts the round instead of waiting
// for every send — progress lands in Daily Activity and the event feed.
function FollowUpTab({ workspaceId }: { workspaceId: string }) {
  const base = getApiBaseUrl();
  const [message, setMessage] = useState("");
  const [contacts, setContacts] = useState<MessagedContact[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const list = await listMessagedContacts(base, workspaceId);
      setContacts(list);
      // Drop selections that no longer exist (e.g. leads removed since).
      setSelected((prev) => {
        const ids = new Set(list.map((c) => c.id));
        const next = new Set<string>();
        prev.forEach((id) => {
          if (ids.has(id)) next.add(id);
        });
        return next;
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load messaged contacts");
    } finally {
      setLoading(false);
    }
  }, [base, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  const query = search.trim().toLowerCase();
  const filtered = query
    ? contacts.filter(
        (c) =>
          c.name.toLowerCase().includes(query) ||
          c.phone.toLowerCase().includes(query) ||
          c.category.toLowerCase().includes(query) ||
          c.city.toLowerCase().includes(query),
      )
    : contacts;
  const selectableIds = filtered.filter((c) => !c.blacklisted).map((c) => c.id);

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });

  const send = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!message.trim() || selected.size === 0 || sending) return;
    setSending(true);
    setError(null);
    setResult(null);
    try {
      const ids = Array.from(selected);
      await sendFollowUp(base, workspaceId, message, ids);
      setResult(
        `Run started for ${ids.length} contact${ids.length === 1 ? "" : "s"} — messages go out at this workspace's pacing (a few minutes apart). Watch the Daily Activity tab; the summary lands in the event feed when the round finishes.`,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to start follow-up run");
    } finally {
      setSending(false);
    }
  };

  // Listen for the run's completion event so the user sees the summary.
  // The hook manages its own subscription lifecycle off workspaceId.
  useWorkspaceEvents(workspaceId, (event) => {
    if (event.type === "pitch_dispatched" && event.message?.startsWith("Follow-up")) {
      setResult(event.message);
    }
  });

  return (
    <div className="grid gap-6">
      <form onSubmit={(e) => void send(e)} className="grid gap-4">
        <div>
          <label className={LABEL}>Follow-up Message</label>
          <textarea
            className={`${INPUT} min-h-[110px] resize-y`}
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder={"Hi {{business_name}}, just following up on my earlier message…"}
            required
          />
          <p className="mt-1 text-[11px] text-slate-400">
            Placeholders merge per contact: {"{{business_name}}"}, {"{{category}}"}, {"{{city}}"},{" "}
            {"{{sender_name}}"}, {"{{portfolio_url}}"}.
          </p>
        </div>
        <button
          type="submit"
          disabled={sending || !message.trim() || selected.size === 0}
          className={BTN_PRIMARY}
        >
          {sending ? (
            <>
              <Spinner size={14} />
              <span className="ml-2">Starting…</span>
            </>
          ) : (
            `Send Follow-up (${selected.size} contact${selected.size === 1 ? "" : "s"})`
          )}
        </button>
      </form>

      {result && <Alert kind="success">{result}</Alert>}
      {error && <Alert kind="error">{error}</Alert>}

      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Contacts already messaged ({contacts.length}) — selected {selected.size}
        </span>
        <div className="flex items-center gap-2">
          <input
            type="text"
            className="rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-xs text-slate-800 placeholder:text-slate-400 focus:border-indigo-400 focus:bg-white focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
            placeholder="Search name, phone, city…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <button
            type="button"
            disabled={selectableIds.length === 0}
            onClick={() =>
              setSelected((prev) => {
                const next = new Set(prev);
                selectableIds.forEach((id) => next.add(id));
                return next;
              })
            }
            className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-40"
          >
            Select all
          </button>
          <button
            type="button"
            disabled={selected.size === 0}
            onClick={() => setSelected(new Set())}
            className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-40"
          >
            Clear
          </button>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-slate-500">
          <Spinner /> Loading contacts…
        </div>
      ) : filtered.length === 0 ? (
        <Alert kind="info">
          No contacts yet. Send the first batch from the Leads tab, and everyone it reached shows up
          here for the follow-up round.
        </Alert>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="w-10 px-3 py-3" />
                <th className="px-3 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Contact
                </th>
                <th className="px-3 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Business
                </th>
                <th className="px-3 py-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Last sent
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((c) => (
                <tr
                  key={c.id}
                  onClick={c.blacklisted ? undefined : () => toggle(c.id)}
                  className={`border-b border-slate-100 last:border-0 ${c.blacklisted ? "text-slate-400" : "cursor-pointer hover:bg-indigo-50/50"}`}
                >
                  <td className="px-3 py-2.5">
                    <input
                      type="checkbox"
                      className="h-4 w-4 rounded border-slate-300 text-indigo-600 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
                      checked={selected.has(c.id)}
                      disabled={c.blacklisted}
                      onChange={() => toggle(c.id)}
                      onClick={(e) => e.stopPropagation()}
                    />
                  </td>
                  <td className="px-3 py-2.5">
                    <span className="font-medium text-slate-800">{c.name || "Unnamed"}</span>
                    <span className="ml-2 font-mono text-xs text-slate-500">{c.phone}</span>
                    {c.blacklisted && (
                      <span className="ml-2 rounded bg-rose-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-rose-700">
                        Dead number
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-2.5 text-xs text-slate-500">
                    {[c.category, c.city].filter(Boolean).join(" · ") || "—"}
                  </td>
                  <td className="px-3 py-2.5 text-xs text-slate-400">
                    {new Date(c.lastSentAt).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ─── Daily activity tab ────────────────────────────────────────────────────

// A send whose outcome was never confirmed. A provable refusal (recorded as
// `not_sent:` or a number WhatsApp says does not exist) is not unknown;
// anything else left failed/sending may have reached WhatsApp and is never
// retried automatically, so a human has to check it.
function isUnconfirmedOutcome(row: { status: string; error?: string }): boolean {
  if (row.status === "sending") return true;
  if (row.status !== "failed") return false;
  const error = row.error ?? "";
  return !error.startsWith("not_sent:") && !/notOnWhatsApp|not registered on WhatsApp/i.test(error);
}

// Shows what was SENT and what FAILED for the CURRENT day by default — a new
// day starts empty. Previous days only load when the user asks for them.
function DailyTab({ workspaceId }: { workspaceId: string }) {
  const base = getApiBaseUrl();
  const [rows, setRows] = useState<OutreachActivity[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showPrevious, setShowPrevious] = useState(false);
  const [needsReview, setNeedsReview] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRows(await listDailyOutreach(base, workspaceId, showPrevious ? 7 : 0));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load outreach activity");
    } finally {
      setLoading(false);
    }
  }, [base, workspaceId, showPrevious]);

  useEffect(() => {
    void load();
  }, [load]);

  const sentToday = rows.filter((r) => r.direction === "outbound" && r.status === "sent").length;
  const failedToday = rows.filter(
    (r) => r.direction === "outbound" && r.status === "failed",
  ).length;
  const inboundToday = rows.filter((r) => r.direction === "inbound").length;
  const unconfirmed = rows.filter((r) => r.direction === "outbound" && isUnconfirmedOutcome(r));
  const shownRows = needsReview ? unconfirmed : rows;

  const statusTone = (r: OutreachActivity) =>
    r.direction === "inbound"
      ? "bg-sky-100 text-sky-700"
      : r.status === "sent"
        ? "bg-emerald-100 text-emerald-700"
        : r.status === "failed"
          ? "bg-red-100 text-red-700"
          : "bg-slate-100 text-slate-600";

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-semibold text-emerald-700">
            {sentToday} sent
          </span>
          <span className="rounded-full bg-red-100 px-2.5 py-1 text-xs font-semibold text-red-700">
            {failedToday} failed
          </span>
          <span className="rounded-full bg-sky-100 px-2.5 py-1 text-xs font-semibold text-sky-700">
            {inboundToday} inbound
          </span>
          <button
            type="button"
            onClick={() => setNeedsReview((value) => !value)}
            disabled={unconfirmed.length === 0}
            aria-pressed={needsReview}
            title="Sends with no confirmed outcome — they are never retried automatically"
            className={`rounded-full px-2.5 py-1 text-xs font-semibold transition-colors ${
              needsReview
                ? "bg-amber-600 text-white"
                : unconfirmed.length > 0
                  ? "bg-amber-100 text-amber-800 hover:bg-amber-200"
                  : "bg-slate-100 text-slate-400"
            }`}
          >
            ⚠ {unconfirmed.length} unconfirmed
          </button>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className={showPrevious ? BTN_PRIMARY : BTN_GHOST}
            onClick={() => setShowPrevious((v) => !v)}
          >
            {showPrevious ? "Showing last 7 days" : "Show previous days (7)"}
          </button>
          <button
            type="button"
            className={BTN_GHOST}
            onClick={() => void load()}
            disabled={loading}
          >
            Refresh
          </button>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 py-8 text-sm text-slate-500">
          <Spinner /> Loading activity…
        </div>
      ) : error ? (
        <Alert kind="error">{error}</Alert>
      ) : rows.length === 0 ? (
        <p className="py-8 text-center text-sm text-slate-400">
          {showPrevious
            ? "No outreach activity in the last 7 days."
            : "No activity yet today — sends will appear here as they go out."}
        </p>
      ) : shownRows.length === 0 ? (
        <p className="py-8 text-center text-sm text-slate-400">
          No unconfirmed sends in this range — every attempt was confirmed or provably refused.
        </p>
      ) : (
        <div>
          {needsReview && (
            <p className="mt-4 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
              These sends were attempted but never confirmed. They are not retried automatically — a
              retry could duplicate a message — so check the contact on WhatsApp before resending.
            </p>
          )}
          <div className="mt-4 max-h-[420px] overflow-y-auto rounded-lg border border-slate-200">
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50 text-left">
                <tr>
                  <th className="px-3 py-2 text-xs font-semibold text-slate-500">Time</th>
                  <th className="px-3 py-2 text-xs font-semibold text-slate-500">Contact</th>
                  <th className="px-3 py-2 text-xs font-semibold text-slate-500">Direction</th>
                  <th className="px-3 py-2 text-xs font-semibold text-slate-500">Status</th>
                  <th className="px-3 py-2 text-xs font-semibold text-slate-500">Message</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {shownRows.map((r) => (
                  <tr key={r.id}>
                    <td className="whitespace-nowrap px-3 py-2 text-xs text-slate-500">
                      {new Date(r.createdAt).toLocaleString()}
                    </td>
                    <td className="px-3 py-2">
                      <div className="text-xs font-medium text-slate-800">
                        {r.leadName || "Unknown"}
                      </div>
                      <div className="text-xs text-slate-400">{r.leadPhone || "—"}</div>
                    </td>
                    <td className="px-3 py-2 text-xs text-slate-600">
                      {r.direction === "inbound" ? "← inbound" : "→ outbound"}
                    </td>
                    <td className="px-3 py-2">
                      <span
                        className={`inline-block rounded px-2 py-0.5 text-[11px] font-semibold ${statusTone(r)}`}
                      >
                        {r.status}
                        {r.error ? " ⚠" : ""}
                      </span>
                      {isUnconfirmedOutcome(r) && (
                        <div className="mt-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-700">
                          outcome unknown
                        </div>
                      )}
                      {r.error && (
                        <div className="mt-1 max-w-xs text-[11px] text-red-600" title={r.error}>
                          {r.error}
                        </div>
                      )}
                    </td>
                    <td className="max-w-sm px-3 py-2 text-xs text-slate-600">
                      <div className="line-clamp-2" title={r.body}>
                        {r.body}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
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
    setConfig((prev) => (prev ? { ...prev, whatsappStatus: status, whatsappPhone: phone } : prev));
  }, []);

  useWorkspaceEvents(workspaceId, (ev) => {
    if (ev.type === "whatsapp_status" && ev.message) {
      setConfig((prev) =>
        prev ? { ...prev, whatsappStatus: ev.message || "disconnected" } : prev,
      );
    }
  });

  return (
    <section
      id="lead-engine"
      aria-labelledby="lead-engine-heading"
      className={`${CARD} mt-6 scroll-mt-24`}
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
            {activeTab === "setup" && <SetupTab config={config} onSaved={setConfig} />}
            {activeTab === "whatsapp" && (
              <WhatsAppTab config={config} onStatusChange={handleStatusChange} />
            )}
            {activeTab === "leads" && <LeadsTab config={config} />}
            {activeTab === "daily" && <DailyTab workspaceId={workspaceId} />}
            {activeTab === "followup" && <FollowUpTab workspaceId={workspaceId} />}
            {activeTab === "blacklist" && <BlacklistTab workspaceId={workspaceId} />}
          </>
        ) : null}
      </div>
    </section>
  );
}
