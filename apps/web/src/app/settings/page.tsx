"use client";

import { useEffect, useState, useCallback } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchHealth, type HealthState, describeDatabase } from "@/lib/health";
import { fetchOperators, createOperator, updateOperator, type Operator } from "@/lib/operators";

export default function SettingsPage() {
  const apiBaseUrl = getApiBaseUrl();
  const [healthState, setHealthState] = useState<HealthState>({ kind: "loading" });
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Team RBAC state
  const [operators, setOperators] = useState<Operator[]>([]);
  const [loadingOperators, setLoadingOperators] = useState(true);
  const [showInviteForm, setShowInviteForm] = useState(false);
  const [inviteName, setInviteName] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState("editor");
  const [inviteError, setInviteError] = useState<string | null>(null);
  const [inviteSuccess, setInviteSuccess] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);

  const loadTeam = useCallback(async () => {
    setLoadingOperators(true);
    setOperators(await fetchOperators(apiBaseUrl));
    setLoadingOperators(false);
  }, [apiBaseUrl]);

  useEffect(() => {
    void fetchHealth(apiBaseUrl).then(setHealthState);
    void loadTeam();
  }, [apiBaseUrl, loadTeam]);

  async function handleRefresh() {
    setHealthState({ kind: "loading" });
    setHealthState(await fetchHealth(apiBaseUrl));
    await loadTeam();
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
    await updateOperator(apiBaseUrl, opId, { role: newRole });
    await loadTeam();
  }

  async function handleStatusToggle(op: Operator) {
    const nextStatus = op.status === "active" ? "inactive" : "active";
    await updateOperator(apiBaseUrl, op.id, { status: nextStatus });
    await loadTeam();
  }

  function copyText(key: string, text: string) {
    void navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  }

  const isLoaded = healthState.kind === "loaded";
  const health = isLoaded ? healthState.health : null;
  const dbInfo = health ? describeDatabase(health.database.status) : null;

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900">Platform Settings & Operations</h1>
        <p className="mt-1 text-sm text-slate-500">
          Monitor system services, database health, environment parameters, and production readiness.
        </p>
      </div>

      {/* System Health Card */}
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Backend API Health & Status
          </h2>
          <button
            type="button"
            onClick={() => void handleRefresh()}
            className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-50 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500"
          >
            ↻ Refresh Health
          </button>
        </div>

        <div className="p-5">
          {healthState.kind === "loading" ? (
            <div className="flex items-center gap-2 py-4 text-sm text-slate-500">
              <span className="h-2 w-2 animate-pulse rounded-full bg-slate-400" />
              Checking API health at <code className="font-mono text-xs">{apiBaseUrl}</code>…
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
                  <span className="text-sm font-bold uppercase text-slate-800">{health.status}</span>
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
                <p className="mt-1 text-xs text-slate-500">Version: {health.version}</p>
              </div>

              <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                  Database
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
                    ? `${health.database.latencyMs}ms latency`
                    : health.database.message}
                </p>
              </div>

              <div className="rounded-lg border border-slate-100 bg-slate-50 p-3.5">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                  API Base URL
                </span>
                <p className="mt-1 font-mono text-xs font-medium text-slate-800 truncate" title={apiBaseUrl}>
                  {apiBaseUrl}
                </p>
                <p className="mt-1 text-[11px] text-slate-400">
                  Checked: {new Date(health.checkedAt).toLocaleTimeString()}
                </p>
              </div>
            </div>
          ) : null}
        </div>
      </section>

      {/* Team Management & RBAC */}
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 bg-slate-50 px-5 py-3">
          <div>
            <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
              Team Members & Role-Based Access (RBAC)
            </h2>
            <p className="mt-0.5 text-xs text-slate-400">
              Manage operators, assign permissions (Admin, Editor, Viewer), and control platform access.
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
            {showInviteForm ? "Close invite" : "+ Add Member"}
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
          <form onSubmit={(e) => void handleInviteSubmit(e)} className="border-b border-slate-200 bg-slate-50/70 p-5 grid gap-4 sm:grid-cols-3 sm:items-end">
            <div>
              <label className="block text-xs font-semibold uppercase tracking-wide text-slate-500 mb-1">
                Full Name
              </label>
              <input
                type="text"
                required
                value={inviteName}
                onChange={(e) => setInviteName(e.target.value)}
                placeholder="Jane Doe"
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
                placeholder="jane@company.com"
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
                {inviting ? "Adding…" : "Add"}
              </button>
            </div>
          </form>
        )}

        <div className="p-5">
          {loadingOperators ? (
            <div className="flex items-center gap-2 py-4 text-xs text-slate-500">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
              Loading team members…
            </div>
          ) : operators.length === 0 ? (
            <p className="text-xs text-slate-500">No team members registered yet.</p>
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
                      <td className="px-4 py-3 font-semibold text-slate-800">{op.name}</td>
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

      {/* Production Readiness Checklist */}
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Production Readiness Checklist
          </h2>
        </div>
        <div className="divide-y divide-slate-100">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-5">
            <div>
              <div className="flex items-center gap-2">
                <span className="flex h-5 w-5 items-center justify-center rounded-full bg-emerald-100 text-emerald-700 text-xs font-bold">
                  ✓
                </span>
                <span className="text-sm font-semibold text-slate-900">
                  Persistent WhatsApp Sessions
                </span>
              </div>
              <p className="mt-1 text-xs text-slate-500 pl-7">
                LocalAuth storage preserves credentials across restarts in{" "}
                <code className="rounded bg-slate-100 px-1 py-0.5 text-[11px]">services/whatsapp-dispatcher/data/sessions/</code>.
                No repeated QR scanning required.
              </p>
            </div>
            <span className="self-start sm:self-auto rounded-full bg-emerald-100 px-2.5 py-0.5 text-xs font-semibold text-emerald-800">
              Active
            </span>
          </div>

          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-5">
            <div>
              <div className="flex items-center gap-2">
                <span className="flex h-5 w-5 items-center justify-center rounded-full bg-emerald-100 text-emerald-700 text-xs font-bold">
                  ✓
                </span>
                <span className="text-sm font-semibold text-slate-900">
                  Background Service Survival (Systemd Linger)
                </span>
              </div>
              <p className="mt-1 text-xs text-slate-500 pl-7">
                User lingering is enabled (<code className="rounded bg-slate-100 px-1 py-0.5 text-[11px]">loginctl enable-linger light</code>).
                All 3 systemd services (API, Web, WhatsApp Dispatcher) keep running even after SSH logout.
              </p>
            </div>
            <span className="self-start sm:self-auto rounded-full bg-emerald-100 px-2.5 py-0.5 text-xs font-semibold text-emerald-800">
              Configured
            </span>
          </div>

          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-5">
            <div>
              <div className="flex items-center gap-2">
                <span className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-100 text-indigo-700 text-xs font-bold">
                  ✓
                </span>
                <span className="text-sm font-semibold text-slate-900">
                  WhatsApp Dispatcher Internal Secret
                </span>
              </div>
              <p className="mt-1 text-xs text-slate-500 pl-7">
                Go API communicates with the dispatcher using <code className="rounded bg-slate-100 px-1 py-0.5 text-[11px]">X-Internal-Secret</code> header.
                Configure <code className="rounded bg-slate-100 px-1 py-0.5 text-[11px]">WHATSAPP_DISPATCHER_SECRET</code> in production to enforce auth.
              </p>
            </div>
            <span className="self-start sm:self-auto rounded-full bg-indigo-100 px-2.5 py-0.5 text-xs font-semibold text-indigo-800">
              Supported
            </span>
          </div>

          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-5">
            <div>
              <div className="flex items-center gap-2">
                <span className="flex h-5 w-5 items-center justify-center rounded-full bg-slate-100 text-slate-700 text-xs font-bold">
                  i
                </span>
                <span className="text-sm font-semibold text-slate-900">
                  Production Environment Switch
                </span>
              </div>
              <p className="mt-1 text-xs text-slate-500 pl-7">
                To enforce strict Bearer authentication and operator login on public networks, set{" "}
                <code className="rounded bg-slate-100 px-1 py-0.5 text-[11px]">APP_ENV=production</code> and{" "}
                <code className="rounded bg-slate-100 px-1 py-0.5 text-[11px]">API_KEY=&lt;secret&gt;</code> in both API and Web environments.
              </p>
            </div>
            <span className="self-start sm:self-auto rounded-full bg-slate-100 px-2.5 py-0.5 text-xs font-semibold text-slate-700">
              Guidance
            </span>
          </div>
        </div>
      </section>

      {/* Operator Service Commands */}
      <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
            Systemd Services Quick Reference
          </h2>
        </div>
        <div className="p-5 space-y-4">
          <p className="text-xs text-slate-600">
            Services run under user-level systemd. Use these commands in terminal to check status or restart:
          </p>

          <div className="grid gap-3">
            {[
              {
                id: "status",
                title: "Check Status of All Services",
                cmd: "systemctl --user status marketing-ops-api marketing-ops-web marketing-ops-whatsapp",
              },
              {
                id: "restart",
                title: "Restart All Services",
                cmd: "systemctl --user restart marketing-ops-api marketing-ops-web marketing-ops-whatsapp",
              },
              {
                id: "logs-api",
                title: "View API Logs",
                cmd: "journalctl --user -u marketing-ops-api -f -n 50",
              },
              {
                id: "logs-wa",
                title: "View WhatsApp Dispatcher Logs",
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
  );
}
