"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  checkIntegrationCapabilities,
  createWorkspaceIntegration,
  deleteIntegration,
  fetchAdapters,
  fetchWorkspaceIntegrations,
  formatCapabilityName,
  formatCapabilityStatus,
  formatLifecycleStatus,
  saveIntegrationCredentials,
  validateIntegrationInput,
  type AdapterDescriptor,
  type CapabilityReport,
  type CapabilityStatus,
  type CheckCapabilitiesResult,
  type CreateIntegrationResult,
  type Integration,
  type IntegrationStatus,
  type IntegrationsState,
  type SaveCredentialsResult,
} from "@/lib/integrations";

const inputClasses =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";
const labelClasses = "block text-sm font-medium text-slate-700";
const PRIMARY_BUTTON =
  "rounded-md bg-teal-800 px-3 py-1.5 text-xs font-semibold text-white hover:bg-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60";
const SECONDARY_BUTTON =
  "rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60";

function lifecycleBadgeClass(status: IntegrationStatus): string {
  switch (status) {
    case "connected":
      return "bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-700/20";
    case "limited":
      return "bg-amber-50 text-amber-800 ring-1 ring-inset ring-amber-700/20";
    case "error":
      return "bg-red-50 text-red-800 ring-1 ring-inset ring-red-700/20";
    case "disconnected":
      return "bg-slate-100 text-slate-700 ring-1 ring-inset ring-slate-600/20";
    default:
      return "bg-slate-100 text-slate-600 ring-1 ring-inset ring-slate-400/20";
  }
}

function capabilityBadgeClass(status: CapabilityStatus): string {
  switch (status) {
    case "supported":
      return "bg-emerald-50 text-emerald-800 ring-1 ring-inset ring-emerald-700/20";
    case "unsupported":
      return "bg-slate-100 text-slate-600 ring-1 ring-inset ring-slate-500/20";
    case "unverified":
      return "bg-amber-50 text-amber-800 ring-1 ring-inset ring-amber-700/20";
    case "failed":
      return "bg-red-50 text-red-800 ring-1 ring-inset ring-red-700/20";
    default:
      return "bg-slate-100 text-slate-600";
  }
}

export function WorkspaceIntegrationsSection({
  workspaceId,
}: Readonly<{
  workspaceId: string;
}>) {
  const [state, setState] = useState<IntegrationsState>({ kind: "loading" });
  const [adapters, setAdapters] = useState<AdapterDescriptor[]>([]);
  const [showAddForm, setShowAddForm] = useState(false);

  // New integration form state
  const [name, setName] = useState("");
  const [adapterKind, setAdapterKind] = useState("sanity");
  const [sanityProjectId, setSanityProjectId] = useState("");
  const [sanityDataset, setSanityDataset] = useState("production");
  const [genericBaseUrl, setGenericBaseUrl] = useState("");
  const [addErrors, setAddErrors] = useState<string[]>([]);
  const [adding, setAdding] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  // Credentials drawer state
  const [credIntegrationId, setCredIntegrationId] = useState<string | null>(null);
  const [credSecret, setCredSecret] = useState("");
  const [credSaving, setCredSaving] = useState(false);
  const [credError, setCredError] = useState<string | null>(null);

  // Diagnostic notification per integration
  const [checkingId, setCheckingId] = useState<string | null>(null);
  const [diagnostic, setDiagnostic] = useState<{ id: string; message: string } | null>(null);

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    const res = await fetchWorkspaceIntegrations(apiBaseUrl, workspaceId);
    setState(res);
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
    async function loadAdapters() {
      const list = await fetchAdapters(apiBaseUrl);
      setAdapters(list);
    }
    void loadAdapters();
  }, [apiBaseUrl, load]);

  async function handleAdd(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setServerError(null);

    const problems = validateIntegrationInput({ name, adapterKind });
    if (problems.length > 0) {
      setAddErrors(problems);
      return;
    }

    const config: Record<string, unknown> = {};
    if (adapterKind === "sanity") {
      if (!sanityProjectId.trim()) {
        setAddErrors(["Sanity Project ID is required"]);
        return;
      }
      config.projectId = sanityProjectId.trim();
      config.dataset = sanityDataset.trim() || "production";
    } else if (adapterKind === "generic_api") {
      if (!genericBaseUrl.trim()) {
        setAddErrors(["Base API URL is required"]);
        return;
      }
      config.baseUrl = genericBaseUrl.trim();
    }

    setAddErrors([]);
    setAdding(true);

    const res: CreateIntegrationResult = await createWorkspaceIntegration(apiBaseUrl, workspaceId, {
      name,
      adapterKind,
      config,
    });

    setAdding(false);

    if (res.kind === "created") {
      setName("");
      setSanityProjectId("");
      setGenericBaseUrl("");
      setShowAddForm(false);
      void load();
    } else if (res.kind === "conflict") {
      setServerError(res.message);
    } else if (res.kind === "rejected") {
      setAddErrors(res.problems);
    } else {
      setServerError(res.message);
    }
  }

  async function handleSaveCredentials(
    e: React.FormEvent<HTMLFormElement>,
    inId: string,
    kind: string,
  ) {
    e.preventDefault();
    setCredError(null);
    if (!credSecret.trim()) {
      setCredError("Credential value cannot be empty.");
      return;
    }

    setCredSaving(true);
    const payload: Record<string, string> = {};
    if (kind === "sanity") {
      payload.token = credSecret.trim();
    } else {
      payload.apiKey = credSecret.trim();
    }

    const res: SaveCredentialsResult = await saveIntegrationCredentials(
      apiBaseUrl,
      workspaceId,
      inId,
      payload,
    );
    setCredSaving(false);

    if (res.kind === "saved") {
      setCredSecret("");
      setCredIntegrationId(null);
      void load();
    } else {
      setCredError(res.message);
    }
  }

  async function handleCheck(inId: string) {
    setCheckingId(inId);
    setDiagnostic(null);
    const res: CheckCapabilitiesResult = await checkIntegrationCapabilities(
      apiBaseUrl,
      workspaceId,
      inId,
    );
    setCheckingId(null);

    if (res.kind === "checked") {
      setDiagnostic({ id: inId, message: res.diagnostic || "Capability verification completed." });
      void load();
    } else {
      setDiagnostic({ id: inId, message: res.message });
    }
  }

  async function handleDelete(inId: string, inName: string) {
    if (!window.confirm(`Disconnect and remove integration "${inName}"?`)) return;
    const res = await deleteIntegration(apiBaseUrl, workspaceId, inId);
    if (res.success) {
      void load();
    } else {
      alert(res.message || "Failed to remove integration.");
    }
  }

  return (
    <section
      aria-labelledby="workspace-integrations-heading"
      className="rounded-lg border border-slate-200 bg-white px-5 py-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-4">
        <div>
          <h2 id="workspace-integrations-heading" className="text-base font-semibold">
            Integrations & Capabilities
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            Publishing destinations and CMS integrations. Capabilities are verified using read-only
            checks.
          </p>
        </div>
        <div>
          <button
            type="button"
            onClick={() => setShowAddForm((prev) => !prev)}
            className={SECONDARY_BUTTON}
          >
            {showAddForm ? "Cancel" : "+ Connect integration"}
          </button>
        </div>
      </div>

      {showAddForm && (
        <form
          onSubmit={(e) => void handleAdd(e)}
          className="mt-4 rounded-md border border-slate-200 bg-slate-50 p-4"
        >
          <h3 className="text-sm font-semibold text-slate-800">Connect new integration</h3>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <div>
              <label htmlFor="int-name" className={labelClasses}>
                Integration name
              </label>
              <input
                id="int-name"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Nutrition Guide Blog"
                maxLength={120}
                required
                className={`mt-1 ${inputClasses}`}
              />
            </div>
            <div>
              <label htmlFor="int-adapter" className={labelClasses}>
                Adapter type
              </label>
              <select
                id="int-adapter"
                value={adapterKind}
                onChange={(e) => setAdapterKind(e.target.value)}
                className={`mt-1 ${inputClasses}`}
              >
                {adapters.map((a) => (
                  <option key={a.kind} value={a.kind}>
                    {a.displayName}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {adapterKind === "sanity" && (
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label htmlFor="sanity-proj" className={labelClasses}>
                  Sanity Project ID
                </label>
                <input
                  id="sanity-proj"
                  type="text"
                  value={sanityProjectId}
                  onChange={(e) => setSanityProjectId(e.target.value)}
                  placeholder="e.g. y97t5m4h"
                  required
                  className={`mt-1 ${inputClasses}`}
                />
              </div>
              <div>
                <label htmlFor="sanity-ds" className={labelClasses}>
                  Dataset
                </label>
                <input
                  id="sanity-ds"
                  type="text"
                  value={sanityDataset}
                  onChange={(e) => setSanityDataset(e.target.value)}
                  placeholder="production"
                  className={`mt-1 ${inputClasses}`}
                />
              </div>
            </div>
          )}

          {adapterKind === "generic_api" && (
            <div className="mt-3">
              <label htmlFor="generic-url" className={labelClasses}>
                Base API URL
              </label>
              <input
                id="generic-url"
                type="url"
                value={genericBaseUrl}
                onChange={(e) => setGenericBaseUrl(e.target.value)}
                placeholder="https://api.client.example/v1"
                required
                className={`mt-1 ${inputClasses}`}
              />
            </div>
          )}

          {addErrors.length > 0 && (
            <ul role="alert" className="mt-3 list-disc space-y-1 pl-5 text-sm text-red-700">
              {addErrors.map((err) => (
                <li key={err}>{err}</li>
              ))}
            </ul>
          )}

          {serverError && (
            <p
              role="alert"
              className="mt-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800 ring-1 ring-inset ring-red-700/15"
            >
              {serverError}
            </p>
          )}

          <div className="mt-4 flex gap-2">
            <button type="submit" disabled={adding} className={PRIMARY_BUTTON}>
              {adding ? "Connecting…" : "Add integration"}
            </button>
            <button
              type="button"
              onClick={() => setShowAddForm(false)}
              className="rounded-md border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-100"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="mt-4 space-y-4">
        {state.kind === "loading" && (
          <p className="py-6 text-center text-sm text-slate-500">Loading integrations…</p>
        )}

        {state.kind === "unavailable" && (
          <div className="rounded-md bg-red-50 p-4 ring-1 ring-inset ring-red-700/15">
            <p className="text-sm font-medium text-red-900">Could not load integrations</p>
            <p className="mt-1 text-sm text-red-800">{state.message}</p>
          </div>
        )}

        {state.kind === "loaded" && state.integrations.length === 0 && (
          <div className="rounded-md border border-dashed border-slate-200 py-8 text-center">
            <p className="text-sm font-medium text-slate-700">No integrations connected yet</p>
            <p className="mt-1 text-xs text-slate-500">
              Connect a website, CMS, or manual workflow using &quot;Connect integration&quot;
              above.
            </p>
          </div>
        )}

        {state.kind === "loaded" &&
          state.integrations.map((inItem: Integration) => (
            <div
              key={inItem.id}
              className="rounded-md border border-slate-200 bg-white p-4 shadow-xs"
            >
              <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 pb-3">
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-semibold text-slate-900">{inItem.name}</h3>
                  <span className="rounded bg-slate-100 px-2 py-0.5 text-xs font-mono text-slate-600">
                    {inItem.adapterKind}
                  </span>
                  <span
                    className={`inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${lifecycleBadgeClass(
                      inItem.status,
                    )}`}
                  >
                    {formatLifecycleStatus(inItem.status)}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => void handleCheck(inItem.id)}
                    disabled={checkingId === inItem.id}
                    className={SECONDARY_BUTTON}
                  >
                    {checkingId === inItem.id ? "Checking…" : "Run check"}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setCredIntegrationId((curr) => (curr === inItem.id ? null : inItem.id));
                      setCredSecret("");
                      setCredError(null);
                    }}
                    className={SECONDARY_BUTTON}
                  >
                    {credIntegrationId === inItem.id
                      ? "Close credentials"
                      : inItem.hasCredentials
                        ? "Update credentials"
                        : "Set credentials"}
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleDelete(inItem.id, inItem.name)}
                    className="rounded-md px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50"
                  >
                    Disconnect
                  </button>
                </div>
              </div>

              {diagnostic && diagnostic.id === inItem.id && (
                <div className="mt-3 rounded-md bg-blue-50 px-3 py-2 text-xs text-blue-900 ring-1 ring-inset ring-blue-700/15">
                  <strong>Verification report:</strong> {diagnostic.message}
                </div>
              )}

              {credIntegrationId === inItem.id && (
                <form
                  onSubmit={(e) => void handleSaveCredentials(e, inItem.id, inItem.adapterKind)}
                  className="mt-3 rounded-md border border-slate-200 bg-slate-50 p-3"
                >
                  <label htmlFor={`cred-${inItem.id}`} className={labelClasses}>
                    {inItem.adapterKind === "sanity"
                      ? "Sanity API write token"
                      : "API secret key / token"}
                  </label>
                  <p className="text-xs text-slate-500">
                    Encrypted at rest using AES-256-GCM. Never returned to the browser or logged.
                  </p>
                  <input
                    id={`cred-${inItem.id}`}
                    type="password"
                    value={credSecret}
                    onChange={(e) => setCredSecret(e.target.value)}
                    placeholder="Enter secret token"
                    required
                    className={`mt-1 ${inputClasses}`}
                  />
                  {credError && (
                    <p role="alert" className="mt-2 text-xs text-red-700">
                      {credError}
                    </p>
                  )}
                  <div className="mt-3 flex gap-2">
                    <button type="submit" disabled={credSaving} className={PRIMARY_BUTTON}>
                      {credSaving ? "Encrypting & verifying…" : "Save & verify"}
                    </button>
                    <button
                      type="button"
                      onClick={() => setCredIntegrationId(null)}
                      className="rounded-md border border-slate-300 px-2 py-1 text-xs text-slate-700 hover:bg-slate-100"
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              )}

              <div className="mt-3">
                <h4 className="text-xs font-semibold uppercase tracking-wider text-slate-500">
                  Verified capabilities
                </h4>
                <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {inItem.capabilities.map((cap: CapabilityReport) => (
                    <div
                      key={cap.capability}
                      className="flex flex-col justify-between rounded border border-slate-100 bg-slate-50 p-2.5 text-xs"
                    >
                      <div className="flex items-center justify-between gap-1">
                        <span className="font-medium text-slate-800">
                          {formatCapabilityName(cap.capability)}
                        </span>
                        <span
                          className={`rounded px-1.5 py-0.5 font-medium ${capabilityBadgeClass(
                            cap.status,
                          )}`}
                        >
                          {formatCapabilityStatus(cap.status)}
                        </span>
                      </div>
                      {cap.details && (
                        <p className="mt-1 text-slate-500 line-clamp-2" title={cap.details}>
                          {cap.details}
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          ))}
      </div>
    </section>
  );
}
