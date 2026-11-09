"use client";

import { useEffect, useState, useCallback } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchHealth, type HealthState, describeDatabase } from "@/lib/health";
import { fetchOperators, createOperator, updateOperator, type Operator } from "@/lib/operators";
import {
  loadPlatformSettings,
  savePlatformSettings,
  type PlatformSettings,
  DEFAULT_PLATFORM_SETTINGS,
} from "@/lib/settings";

type TabKey = "overview" | "organization" | "team" | "ai" | "notifications" | "security";

export default function SettingsPage() {
  const apiBaseUrl = getApiBaseUrl();
  const [activeTab, setActiveTab] = useState<TabKey>("overview");
  const [healthState, setHealthState] = useState<HealthState>({ kind: "loading" });
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Platform Custom Settings State
  const [settings, setSettings] = useState<PlatformSettings>(DEFAULT_PLATFORM_SETTINGS);
  const [settingsSaved, setSettingsSaved] = useState(false);

  // Team RBAC state
  const [operators, setOperators] = useState<Operator[]>([]);
  const [loadingOperators, setLoadingOperators] = useState(true);
  const [teamError, setTeamError] = useState<string | null>(null);
  const [showInviteForm, setShowInviteForm] = useState(false);
  const [inviteName, setInviteName] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("editor");
  const [inviteError, setInviteError] = useState<string | null>(null);
  const [inviteSuccess, setInviteSuccess] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);

  const loadTeam = useCallback(async () => {
    setLoadingOperators(true);
    const state = await fetchOperators(apiBaseUrl);
    if (state.kind === "loaded") {
      setOperators(state.operators);
      setTeamError(null);
    } else {
      setOperators([]);
      setTeamError(state.message);
    }
    setLoadingOperators(false);
  }, [apiBaseUrl]);

  useEffect(() => {
    void fetchHealth(apiBaseUrl).then(setHealthState);
    void loadTeam();
    setSettings(loadPlatformSettings());
  }, [apiBaseUrl, loadTeam]);

  async function handleRefresh() {
    setHealthState({ kind: "loading" });
    setHealthState(await fetchHealth(apiBaseUrl));
    await loadTeam();
  }

  function handleSaveSettings(e?: React.FormEvent) {
    if (e) e.preventDefault();
    savePlatformSettings(settings);
    setSettingsSaved(true);
    setTimeout(() => setSettingsSaved(false), 3000);
  }

  async function handleInviteSubmit(e: React.FormEvent) {
    e.preventDefault();
    setInviteError(null);
    setInviteSuccess(null);
    setInviting(true);
    const res = await createOperator(apiBaseUrl, {
      name: inviteName,
      email: inviteEmail,
      role: inviteRole,
    });
    setInviting(false);
    if (!res.ok) {
      setInviteError(res.error ?? "Failed to invite member.");
    } else {
      setInviteSuccess(`Successfully added ${inviteName} as ${inviteRole}.`);
      setInviteName("");
      setInviteEmail("");
      setShowInviteForm(false);
      await loadTeam();
    }
  }

  async function handleRoleChange(opId: string, newRole: string) {
    const result = await updateOperator(apiBaseUrl, opId, { role: newRole });
    await loadTeam();
    if (!result.ok) setTeamError(result.error ?? "Could not update the role.");
  }

  async function handleStatusToggle(op: Operator) {
    if (
      op.status === "active" &&
      !window.confirm(`Deactivate ${op.name}? Their history is kept, but they are marked inactive.`)
    ) {
      return;
    }
    const nextStatus = op.status === "active" ? "inactive" : "active";
    const result = await updateOperator(apiBaseUrl, op.id, { status: nextStatus });
    await loadTeam();
    if (!result.ok) setTeamError(result.error ?? "Could not update the status.");
  }

  function copyText(key: string, text: string) {
    void navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  }

  const isLoaded = healthState.kind === "loaded";
  const health = isLoaded ? healthState.health : null;
  const dbInfo = health ? describeDatabase(health.database.status) : null;

  const tabs: { id: TabKey; label: string; icon: string }[] = [
    { id: "overview", label: "System Health & Ops", icon: "⚡" },
    { id: "organization", label: "Agency Profile", icon: "🏢" },
    { id: "team", label: "Team & RBAC", icon: "👥" },
    { id: "ai", label: "AI Engine & Models", icon: "🤖" },
    { id: "notifications", label: "Alerts & Webhooks", icon: "🔔" },
    { id: "security", label: "Security & Access", icon: "🛡️" },
  ];

  return (
    <div className="grid gap-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Platform Settings & Control Center</h1>
          <p className="mt-1 text-sm text-slate-500">
            Configure global agency parameters, manage operator access, AI models, and monitor infrastructure.
          </p>
        </div>
        {settingsSaved && (
          <div className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-50 border border-emerald-200 px-3 py-1.5 text-xs font-semibold text-emerald-800 animate-fade-in">
            ✓ Settings saved successfully
          </div>
        )}
      </div>

      {/* Tabs navigation */}
      <div className="border-b border-slate-200">
        <nav className="-mb-px flex space-x-2 sm:space-x-4 overflow-x-auto" aria-label="Settings Tabs">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 whitespace-nowrap border-b-2 py-3 px-3 sm:px-4 text-xs sm:text-sm font-semibold transition-colors ${
                activeTab === tab.id
                  ? "border-indigo-600 text-indigo-600"
                  : "border-transparent text-slate-500 hover:border-slate-300 hover:text-slate-700"
              }`}
            >
              <span>{tab.icon}</span>
              <span>{tab.label}</span>
            </button>
          ))}
        </nav>
      </div>

      {/* TAB 1: SYSTEM HEALTH & OPS */}
      {activeTab === "overview" && (
        <div className="grid gap-6">
          {/* Health Summary Card */}
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 bg-slate-50 px-5 py-3">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Backend API & Database Live Metrics
              </h2>
              <button
                type="button"
                onClick={() => void handleRefresh()}
                className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 transition-colors focus-visible:outline-2 focus-visible:outline-indigo-500"
              >
                ↻ Refresh Live Health
              </button>
            </div>

            <div className="p-5">
              {healthState.kind === "loading" ? (
                <div className="flex items-center gap-2 py-4 text-sm text-slate-500">
                  <span className="h-2 w-2 animate-pulse rounded-full bg-slate-400" />
                  Checking API and database latency…
                </div>
              ) : healthState.kind === "unavailable" ? (
                <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">
                  <p className="font-semibold">API Unreachable</p>
                  <p className="mt-1 text-xs">{healthState.reason}</p>
                </div>
              ) : health ? (
                <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                    <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                      Service Status
                    </span>
                    <div className="mt-1 flex items-center gap-2">
                      <span
                        className={`h-2.5 w-2.5 rounded-full ${
                          health.status === "ok" ? "bg-emerald-500" : "bg-amber-500"
                        }`}
                      />
                      <span className="text-sm font-bold uppercase text-slate-800">
                        {health.status}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-slate-500">{health.service}</p>
                  </div>

                  <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                    <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                      Environment
                    </span>
                    <p className="mt-1 text-sm font-bold text-slate-800 capitalize">
                      {health.environment}
                    </p>
                    <p className="mt-1 text-xs text-slate-500">Build: {health.version}</p>
                  </div>

                  <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                    <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                      PostgreSQL Connection
                    </span>
                    <div className="mt-1 flex items-center gap-2">
                      <span
                        className={`h-2.5 w-2.5 rounded-full ${
                          dbInfo?.tone === "good"
                            ? "bg-emerald-500"
                            : dbInfo?.tone === "warn"
                              ? "bg-amber-500"
                              : "bg-red-500"
                        }`}
                      />
                      <span className="text-sm font-bold text-slate-800">
                        {dbInfo?.label ?? health.database.status}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-slate-500">
                      {health.database.latencyMs !== undefined
                        ? `${health.database.latencyMs}ms roundtrip`
                        : health.database.message}
                    </p>
                  </div>

                  <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                    <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                      API Gateway
                    </span>
                    <p
                      className="mt-1 font-mono text-xs font-medium text-slate-800 truncate"
                      title={apiBaseUrl || "Same-origin reverse proxy"}
                    >
                      {apiBaseUrl || "Same-origin (Caddy/Next.js)"}
                    </p>
                    <p className="mt-1 text-[11px] text-slate-400">
                      Checked at {new Date(health.checkedAt).toLocaleTimeString()}
                    </p>
                  </div>
                </div>
              ) : null}
            </div>
          </section>

          {/* Automated Backup Status */}
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="flex items-center justify-between border-b border-slate-100 bg-slate-50 px-5 py-3">
              <div>
                <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                  Automated Database Backups & Disaster Recovery
                </h2>
                <p className="mt-0.5 text-xs text-slate-400">
                  Daily automated cron running at 02:00 AM UTC with gzip-9 compression and 7-day retention pruning.
                </p>
              </div>
              <span className="rounded-full bg-emerald-100 px-2.5 py-0.5 text-xs font-semibold text-emerald-800">
                Active Cron
              </span>
            </div>
            <div className="p-5 grid gap-4 sm:grid-cols-3">
              <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                <span className="text-xs font-semibold text-slate-600 block">Backup Schedule</span>
                <span className="mt-1 block font-mono text-xs font-bold text-slate-800">0 2 * * * (Daily)</span>
                <span className="mt-0.5 text-[11px] text-slate-400 block">Cron task in system crontab</span>
              </div>
              <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                <span className="text-xs font-semibold text-slate-600 block">Backup Directory</span>
                <span className="mt-1 block font-mono text-xs font-bold text-slate-800">backups/*.sql.gz</span>
                <span className="mt-0.5 text-[11px] text-slate-400 block">Integrity tested with gzip -tv</span>
              </div>
              <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                <span className="text-xs font-semibold text-slate-600 block">Disaster Recovery</span>
                <span className="mt-1 block font-mono text-xs font-bold text-slate-800">scripts/restore-db.sh</span>
                <span className="mt-0.5 text-[11px] text-slate-400 block">Single-transaction safe restoration</span>
              </div>
            </div>
          </section>

          {/* Systemd Services Quick Reference */}
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Operator Service Controls (CLI)
              </h2>
            </div>
            <div className="p-5 space-y-4">
              <p className="text-xs text-slate-600">
                The API and WhatsApp dispatcher run as continuous background user services with systemd lingering enabled.
              </p>
              <div className="grid gap-3">
                {[
                  {
                    id: "status",
                    title: "Check all service states",
                    cmd: "systemctl --user status marketing-ops-api marketing-ops-whatsapp",
                  },
                  {
                    id: "restart",
                    title: "Restart API & WhatsApp Dispatcher",
                    cmd: "systemctl --user restart marketing-ops-api marketing-ops-whatsapp",
                  },
                  {
                    id: "logs-api",
                    title: "Tail Live API Logs",
                    cmd: "journalctl --user -u marketing-ops-api -f -n 50",
                  },
                  {
                    id: "logs-wa",
                    title: "Tail WhatsApp Dispatcher Logs",
                    cmd: "journalctl --user -u marketing-ops-whatsapp -f -n 50",
                  },
                ].map(({ id, title, cmd }) => (
                  <div key={id} className="rounded-lg border border-slate-200 bg-slate-50 p-3">
                    <div className="flex items-center justify-between gap-2 mb-1.5">
                      <span className="text-xs font-semibold text-slate-700">{title}</span>
                      <button
                        type="button"
                        onClick={() => copyText(id, cmd)}
                        className="text-xs font-medium text-indigo-600 hover:text-indigo-800"
                      >
                        {copiedKey === id ? "Copied!" : "Copy command"}
                      </button>
                    </div>
                    <code className="block rounded bg-white px-3 py-2 font-mono text-xs text-slate-800 border border-slate-200 break-all">
                      {cmd}
                    </code>
                  </div>
                ))}
              </div>
            </div>
          </section>
        </div>
      )}

      {/* TAB 2: ORGANIZATION PROFILE */}
      {activeTab === "organization" && (
        <form onSubmit={(e) => handleSaveSettings(e)} className="grid gap-6">
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Agency Branding & Global Defaults
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">
                Default agency parameters applied across all managed client workspaces and outbound correspondence.
              </p>
            </div>
            <div className="p-5 grid gap-4 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Agency / Organization Name
                </label>
                <input
                  type="text"
                  required
                  value={settings.agency.agencyName}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      agency: { ...s.agency, agencyName: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Primary Operations Email
                </label>
                <input
                  type="email"
                  required
                  value={settings.agency.supportEmail}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      agency: { ...s.agency, supportEmail: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Agency Website URL
                </label>
                <input
                  type="url"
                  value={settings.agency.websiteUrl}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      agency: { ...s.agency, websiteUrl: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Default Platform Timezone
                </label>
                <select
                  value={settings.agency.defaultTimezone}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      agency: { ...s.agency, defaultTimezone: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                >
                  <option value="UTC">UTC (Coordinated Universal Time)</option>
                  <option value="GMT">GMT / Africa/Accra</option>
                  <option value="America/New_York">America/New_York (EST/EDT)</option>
                  <option value="America/Los_Angeles">America/Los_Angeles (PST/PDT)</option>
                  <option value="Europe/London">Europe/London (BST)</option>
                  <option value="Europe/Paris">Europe/Paris (CET)</option>
                  <option value="Asia/Dubai">Asia/Dubai (GST)</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Default Currency
                </label>
                <select
                  value={settings.agency.defaultCurrency}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      agency: { ...s.agency, defaultCurrency: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                >
                  <option value="USD">USD ($ - United States Dollar)</option>
                  <option value="EUR">EUR (€ - Euro)</option>
                  <option value="GBP">GBP (£ - British Pound)</option>
                  <option value="GHS">GHS (GH₵ - Ghanaian Cedi)</option>
                  <option value="NGN">NGN (₦ - Nigerian Naira)</option>
                </select>
              </div>

              <div className="sm:col-span-2">
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Agency Mission / Positioning Statement
                </label>
                <textarea
                  rows={2}
                  value={settings.agency.brandTagline}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      agency: { ...s.agency, brandTagline: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>
            </div>
            <div className="border-t border-slate-100 bg-slate-50/50 px-5 py-3 flex justify-end">
              <button
                type="submit"
                className="rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors"
              >
                Save Agency Profile
              </button>
            </div>
          </section>
        </form>
      )}

      {/* TAB 3: TEAM & RBAC */}
      {activeTab === "team" && (
        <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 bg-slate-50 px-5 py-3">
            <div>
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Team Members & Access Control (RBAC)
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">
                Control operator roles: <strong>Admin</strong> (full management), <strong>Editor</strong> (campaigns & content), and <strong>Viewer</strong> (read-only).
              </p>
            </div>
            <button
              type="button"
              onClick={() => {
                setShowInviteForm((v) => !v);
                setInviteError(null);
                setInviteSuccess(null);
              }}
              className="rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors"
            >
              {showInviteForm ? "Close invite" : "+ Add Team Member"}
            </button>
          </div>

          {inviteSuccess && (
            <div className="border-b border-emerald-100 bg-emerald-50 px-5 py-2.5 text-xs text-emerald-800">
              {inviteSuccess}
            </div>
          )}
          {inviteError && (
            <div className="border-b border-red-100 bg-red-50 px-5 py-2.5 text-xs text-red-800">
              {inviteError}
            </div>
          )}

          {showInviteForm && (
            <form
              onSubmit={(e) => void handleInviteSubmit(e)}
              className="border-b border-slate-200 bg-slate-50/70 p-5 grid gap-4 sm:grid-cols-3 sm:items-end"
            >
              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Full Name
                </label>
                <input
                  type="text"
                  required
                  value={inviteName}
                  onChange={(e) => setInviteName(e.target.value)}
                  placeholder="Alex Rivera"
                  className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-900 placeholder:text-slate-400 focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Email Address
                </label>
                <input
                  type="email"
                  required
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  placeholder="alex@agency.com"
                  className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-900 placeholder:text-slate-400 focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>
              <div className="flex gap-2">
                <div className="flex-1">
                  <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                    Assigned Role
                  </label>
                  <select
                    value={inviteRole}
                    onChange={(e) => setInviteRole(e.target.value)}
                    className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-900 focus:border-indigo-400 focus-visible:outline-indigo-500"
                  >
                    <option value="admin">Admin (Full Control)</option>
                    <option value="editor">Editor (Campaigns & Content)</option>
                    <option value="viewer">Viewer (Read-Only)</option>
                  </select>
                </div>
                <button
                  type="submit"
                  disabled={inviting}
                  className="self-end rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 disabled:opacity-50 transition-colors"
                >
                  {inviting ? "Adding…" : "Add Member"}
                </button>
              </div>
            </form>
          )}

          <div className="p-5">
            {loadingOperators ? (
              <div className="flex items-center gap-2 py-4 text-xs text-slate-500">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
                Loading operators…
              </div>
            ) : teamError ? (
              <div
                role="alert"
                className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2.5 text-xs text-red-800"
              >
                <span>{teamError}</span>
                <button
                  type="button"
                  onClick={() => void loadTeam()}
                  className="font-semibold underline hover:text-red-900"
                >
                  Retry
                </button>
              </div>
            ) : operators.length === 0 ? (
              <p className="text-xs text-slate-500">No operators registered yet.</p>
            ) : (
              <div className="overflow-x-auto rounded-lg border border-slate-200">
                <table className="min-w-full text-sm">
                  <thead>
                    <tr className="border-b border-slate-200 bg-slate-50 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
                      <th className="px-4 py-3">Member</th>
                      <th className="px-4 py-3">Email</th>
                      <th className="px-4 py-3">Role</th>
                      <th className="px-4 py-3">Status</th>
                      <th className="px-4 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 text-xs">
                    {operators.map((op) => (
                      <tr key={op.id} className="hover:bg-slate-50 transition-colors">
                        <td className="px-4 py-3 font-semibold text-slate-800 flex items-center gap-2.5">
                          <span className="h-7 w-7 rounded-full bg-indigo-100 text-indigo-700 font-bold flex items-center justify-center text-xs">
                            {op.name.charAt(0).toUpperCase()}
                          </span>
                          <span>{op.name}</span>
                        </td>
                        <td className="px-4 py-3 text-slate-600 font-mono text-[11px]">{op.email}</td>
                        <td className="px-4 py-3">
                          <select
                            value={op.role}
                            onChange={(e) => void handleRoleChange(op.id, e.target.value)}
                            className="rounded border border-slate-200 bg-white px-2 py-1 text-xs text-slate-700 capitalize focus:border-indigo-400 focus-visible:outline-indigo-500"
                          >
                            <option value="admin">Admin</option>
                            <option value="editor">Editor</option>
                            <option value="viewer">Viewer</option>
                          </select>
                        </td>
                        <td className="px-4 py-3">
                          <span
                            className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold capitalize ${
                              op.status === "active"
                                ? "bg-emerald-100 text-emerald-800"
                                : "bg-slate-100 text-slate-500"
                            }`}
                          >
                            {op.status}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-right">
                          <button
                            type="button"
                            onClick={() => void handleStatusToggle(op)}
                            className="text-[11px] font-medium text-slate-500 hover:text-indigo-600 underline"
                          >
                            {op.status === "active" ? "Deactivate" : "Activate"}
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </section>
      )}

      {/* TAB 4: AI ENGINE & MODELS */}
      {activeTab === "ai" && (
        <form onSubmit={(e) => handleSaveSettings(e)} className="grid gap-6">
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Global AI Provider & Specialist Orchestration
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">
                Configure primary LLM routing for content drafting, agency agents, and WhatsApp conversational auto-replies.
              </p>
            </div>
            <div className="p-5 grid gap-4 sm:grid-cols-2">
              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Primary AI Provider
                </label>
                <select
                  value={settings.ai.defaultProvider}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      ai: {
                        ...s.ai,
                        defaultProvider: e.target.value as "openrouter" | "openai" | "anthropic" | "stub",
                      },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                >
                  <option value="openrouter">OpenRouter (Multi-model routing)</option>
                  <option value="openai">OpenAI Direct</option>
                  <option value="anthropic">Anthropic Claude Direct</option>
                  <option value="stub">Offline Stub (Local testing)</option>
                </select>
                <p className="mt-1 text-[11px] text-slate-400">
                  Configured server key in <code>apps/api/.env</code>: OPENROUTER_API_KEY
                </p>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Primary Model Route
                </label>
                <input
                  type="text"
                  required
                  value={settings.ai.defaultModel}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      ai: { ...s.ai, defaultModel: e.target.value },
                    }))
                  }
                  placeholder="openai/gpt-4o or anthropic/claude-3.5-haiku"
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">
                  Recommended: <code>openai/gpt-4o</code>, <code>anthropic/claude-3.5-sonnet</code>, or <code>deepseek/deepseek-chat</code>
                </p>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Creativity / Temperature ({settings.ai.temperature})
                </label>
                <input
                  type="range"
                  min="0.1"
                  max="1.0"
                  step="0.05"
                  value={settings.ai.temperature}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      ai: { ...s.ai, temperature: parseFloat(e.target.value) },
                    }))
                  }
                  className="w-full accent-indigo-600"
                />
                <div className="flex justify-between text-[11px] text-slate-400 mt-1">
                  <span>0.1 (Strict & Factual)</span>
                  <span>0.7 (Balanced)</span>
                  <span>1.0 (Creative)</span>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Max Output Tokens
                </label>
                <input
                  type="number"
                  min="500"
                  max="16000"
                  step="500"
                  value={settings.ai.maxTokens}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      ai: { ...s.ai, maxTokens: parseInt(e.target.value) || 4000 },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
                <p className="mt-1 text-[11px] text-slate-400">Default: 4000 tokens per draft</p>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Fallback Model on Error
                </label>
                <input
                  type="text"
                  value={settings.ai.fallbackModel}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      ai: { ...s.ai, fallbackModel: e.target.value },
                    }))
                  }
                  placeholder="anthropic/claude-3.5-haiku"
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Monthly Spend Alert Threshold (USD)
                </label>
                <input
                  type="number"
                  value={settings.ai.monthlyBudgetUsd}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      ai: { ...s.ai, monthlyBudgetUsd: parseFloat(e.target.value) || 0 },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                />
              </div>
            </div>
            <div className="border-t border-slate-100 bg-slate-50/50 px-5 py-3 flex justify-end">
              <button
                type="submit"
                className="rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors"
              >
                Save AI Configuration
              </button>
            </div>
          </section>
        </form>
      )}

      {/* TAB 5: ALERTS & NOTIFICATIONS */}
      {activeTab === "notifications" && (
        <form onSubmit={(e) => handleSaveSettings(e)} className="grid gap-6">
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Webhook Notification Channels & Trigger Rules
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">
                Receive instant real-time alerts into your agency Slack or Discord server when critical events occur.
              </p>
            </div>
            <div className="p-5 grid gap-5">
              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Slack Incoming Webhook URL
                </label>
                <input
                  type="url"
                  placeholder="https://hooks.slack.com/services/T000/B000/XXXX"
                  value={settings.notifications.slackWebhookUrl}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      notifications: { ...s.notifications, slackWebhookUrl: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500 font-mono"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                  Discord Webhook URL
                </label>
                <input
                  type="url"
                  placeholder="https://discord.com/api/webhooks/XXXX/YYYY"
                  value={settings.notifications.discordWebhookUrl}
                  onChange={(e) =>
                    setSettings((s) => ({
                      ...s,
                      notifications: { ...s.notifications, discordWebhookUrl: e.target.value },
                    }))
                  }
                  className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500 font-mono"
                />
              </div>

              <div className="border-t border-slate-100 pt-4">
                <span className="text-xs font-semibold text-slate-700 block mb-3 uppercase tracking-wide">
                  Automated Event Triggers
                </span>
                <div className="space-y-3">
                  <label className="flex items-center gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={settings.notifications.notifyOnInboundLead}
                      onChange={(e) =>
                        setSettings((s) => ({
                          ...s,
                          notifications: {
                            ...s.notifications,
                            notifyOnInboundLead: e.target.checked,
                          },
                        }))
                      }
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 h-4 w-4"
                    />
                    <span className="text-xs text-slate-700">
                      <strong>New Inbound Lead Received</strong> (Form webhook submission or WhatsApp prospective client message)
                    </span>
                  </label>

                  <label className="flex items-center gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={settings.notifications.notifyOnWhatsAppDisconnect}
                      onChange={(e) =>
                        setSettings((s) => ({
                          ...s,
                          notifications: {
                            ...s.notifications,
                            notifyOnWhatsAppDisconnect: e.target.checked,
                          },
                        }))
                      }
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 h-4 w-4"
                    />
                    <span className="text-xs text-slate-700">
                      <strong>WhatsApp Session Disconnected</strong> (Account logout or network disconnect requiring QR re-authentication)
                    </span>
                  </label>

                  <label className="flex items-center gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={settings.notifications.notifyOnPublishFailure}
                      onChange={(e) =>
                        setSettings((s) => ({
                          ...s,
                          notifications: {
                            ...s.notifications,
                            notifyOnPublishFailure: e.target.checked,
                          },
                        }))
                      }
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 h-4 w-4"
                    />
                    <span className="text-xs text-slate-700">
                      <strong>Content Publishing Job Failed</strong> (Sanity, X, Meta, or YouTube upload failure after retry attempts)
                    </span>
                  </label>
                </div>
              </div>
            </div>
            <div className="border-t border-slate-100 bg-slate-50/50 px-5 py-3 flex justify-end">
              <button
                type="submit"
                className="rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors"
              >
                Save Notification Settings
              </button>
            </div>
          </section>
        </form>
      )}

      {/* TAB 6: SECURITY & ACCESS */}
      {activeTab === "security" && (
        <form onSubmit={(e) => handleSaveSettings(e)} className="grid gap-6">
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                Security, Tokens & Administrative Access
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">
                Platform API authentication tokens, webhook signing verification, and session lifecycle.
              </p>
            </div>
            <div className="p-5 grid gap-5">
              <div className="rounded-lg border border-slate-100 bg-slate-50 p-4">
                <span className="text-xs font-semibold text-slate-800 block">Operator API Key Authentication</span>
                <p className="mt-1 text-xs text-slate-600">
                  Every request from the web dashboard proxy or automation scripts carries a 256-bit cryptographic Bearer token.
                </p>
                <div className="mt-3 flex items-center gap-2">
                  <span className="rounded bg-emerald-100 px-2 py-0.5 text-[11px] font-semibold text-emerald-800">
                    Active
                  </span>
                  <span className="text-xs text-slate-500">
                    Managed via <code>apps/api/.env</code> and <code>apps/web/.env.local</code>
                  </span>
                </div>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                    Operator Session Expiration (Hours)
                  </label>
                  <input
                    type="number"
                    min="1"
                    max="720"
                    value={settings.security.sessionTimeoutHours}
                    onChange={(e) =>
                      setSettings((s) => ({
                        ...s,
                        security: {
                          ...s.security,
                          sessionTimeoutHours: parseInt(e.target.value) || 24,
                        },
                      }))
                    }
                    className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500"
                  />
                  <p className="mt-1 text-[11px] text-slate-400">Default: 24 hours</p>
                </div>

                <div>
                  <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                    Allowed IP Addresses / CIDRs (Optional)
                  </label>
                  <input
                    type="text"
                    placeholder="e.g. 192.168.1.0/24, 10.0.0.1"
                    value={settings.security.allowedIpRanges}
                    onChange={(e) =>
                      setSettings((s) => ({
                        ...s,
                        security: {
                          ...s.security,
                          allowedIpRanges: e.target.value,
                        },
                      }))
                    }
                    className="w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-900 focus:bg-white focus:border-indigo-400 focus-visible:outline-indigo-500 font-mono"
                  />
                  <p className="mt-1 text-[11px] text-slate-400">Leave blank to allow any origin</p>
                </div>
              </div>

              <div className="rounded-lg border border-indigo-100 bg-indigo-50/50 p-4">
                <span className="text-xs font-semibold text-indigo-900 block">Production Hardening Generator</span>
                <p className="mt-1 text-xs text-indigo-700">
                  To rotate or generate fresh 256-bit cryptographically secure keys for <code>API_KEY</code>, <code>WHATSAPP_DISPATCHER_SECRET</code>, and <code>CREDENTIAL_ENCRYPTION_KEY</code>:
                </p>
                <div className="mt-2 flex items-center justify-between gap-2 rounded bg-white p-2 border border-indigo-200">
                  <code className="font-mono text-xs text-slate-800">./scripts/setup-production-security.sh</code>
                  <button
                    type="button"
                    onClick={() => copyText("sec-script", "./scripts/setup-production-security.sh")}
                    className="text-xs font-semibold text-indigo-600 hover:text-indigo-800"
                  >
                    {copiedKey === "sec-script" ? "Copied!" : "Copy"}
                  </button>
                </div>
              </div>
            </div>
            <div className="border-t border-slate-100 bg-slate-50/50 px-5 py-3 flex justify-end">
              <button
                type="submit"
                className="rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors"
              >
                Save Security Settings
              </button>
            </div>
          </section>
        </form>
      )}
    </div>
  );
}
