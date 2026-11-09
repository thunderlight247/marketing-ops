"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { getApiBaseUrl } from "@/lib/config";
import {
  deleteWorkspace,
  fetchWorkspaceDetail,
  saveBrandProfile,
  updateWorkspaceFacts,
  validateWorkspaceFacts,
  type DeleteWorkspaceResult,
  type SaveBrandResult,
  type WorkspaceDetailState,
  type WorkspaceFactsInput,
  type WorkspaceFactsResult,
} from "@/lib/workspaces";

type TabKey = "profile" | "brand" | "guardrails" | "attribution" | "channels" | "danger";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wider text-slate-600 mb-1";
const hintClasses = "mt-1.5 text-xs text-slate-500";

function linesOf(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

function nullable(text: string): string | null {
  const trimmed = text.trim();
  return trimmed.length === 0 ? null : trimmed;
}

function linesToTextarea(lines: unknown): string {
  if (Array.isArray(lines)) {
    return lines.map(String).join("\n");
  }
  if (typeof lines === "string") {
    return lines;
  }
  return "";
}

const PRIMARY_BUTTON =
  "inline-flex items-center justify-center gap-2 rounded-lg bg-indigo-600 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors";
const DANGER_BUTTON =
  "inline-flex items-center justify-center gap-2 rounded-lg bg-red-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-red-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-500 disabled:cursor-not-allowed disabled:opacity-50 transition-colors";

export function WorkspaceDetailPanel({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const router = useRouter();
  const [activeTab, setActiveTab] = useState<TabKey>("profile");
  const [state, setState] = useState<WorkspaceDetailState>({ kind: "loading" });

  // Workspace facts form state
  const [name, setName] = useState("");
  const [websiteUrl, setWebsiteUrl] = useState("");
  const [status, setStatus] = useState<"active" | "archived">("active");
  const [factsErrors, setFactsErrors] = useState<string[]>([]);
  const [factsSaving, setFactsSaving] = useState(false);
  const [factsSavedToast, setFactsSavedToast] = useState(false);

  // Client profile inside rules
  const [contactName, setContactName] = useState("");
  const [contactEmail, setContactEmail] = useState("");
  const [contactPhone, setContactPhone] = useState("");
  const [clientRole, setClientRole] = useState("");
  const [clientIndustry, setClientIndustry] = useState("");
  const [clientTimezone, setClientTimezone] = useState("UTC");

  // Core Brand Profile
  const [businessDescription, setBusinessDescription] = useState("");
  const [uvp, setUvp] = useState("");
  const [audience, setAudience] = useState("");
  const [services, setServices] = useState("");
  const [tone, setTone] = useState("");
  const [competitors, setCompetitors] = useState("");
  const [targetKeywords, setTargetKeywords] = useState("");

  // Guardrails & CTAs
  const [prohibited, setProhibited] = useState("");
  const [ctas, setCtas] = useState("");
  const [complianceNotes, setComplianceNotes] = useState("");

  // Attribution Defaults
  const [utmSource, setUtmSource] = useState("");
  const [utmMedium, setUtmMedium] = useState("");
  const [utmCampaignPrefix, setUtmCampaignPrefix] = useState("");
  const [defaultSenderName, setDefaultSenderName] = useState("");
  const [defaultSenderEmail, setDefaultSenderEmail] = useState("");
  const [primaryChannel, setPrimaryChannel] = useState<"whatsapp" | "email" | "omnichannel">("whatsapp");

  // Brand saving state
  const [brandSaving, setBrandSaving] = useState(false);
  const [brandError, setBrandError] = useState<string | null>(null);
  const [brandSavedAt, setBrandSavedAt] = useState<string | null>(null);
  const [brandSavedToast, setBrandSavedToast] = useState(false);

  // Danger zone state
  const [confirmSlug, setConfirmSlug] = useState("");
  const [deleting, setDeleting] = useState(false);

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    const loaded = await fetchWorkspaceDetail(apiBaseUrl, workspaceId);
    if (loaded.kind === "loaded") {
      const brand = loaded.brand;
      setName(loaded.workspace.name);
      setWebsiteUrl(loaded.workspace.websiteUrl ?? "");
      setStatus(loaded.workspace.status === "archived" ? "archived" : "active");

      // Extract standard fields
      setBusinessDescription(brand?.businessDescription ?? "");
      setAudience(brand?.audience ?? "");
      setServices(brand?.services ?? "");
      setTone(brand?.tone ?? "");
      setProhibited(linesToTextarea(brand?.prohibitedClaims ?? null));
      setCtas(linesToTextarea(brand?.callsToAction ?? null));
      setBrandSavedAt(brand ? brand.updatedAt : null);

      // Extract extended rules if available
      const rules = brand?.rules ?? {};
      const contact = (rules.clientContact as Record<string, unknown> | undefined) ?? {};
      setContactName(typeof contact.name === "string" ? contact.name : "");
      setContactEmail(typeof contact.email === "string" ? contact.email : "");
      setContactPhone(typeof contact.phone === "string" ? contact.phone : "");
      setClientRole(typeof contact.role === "string" ? contact.role : "");
      setClientIndustry(typeof contact.industry === "string" ? contact.industry : "");
      setClientTimezone(typeof contact.timezone === "string" ? contact.timezone : "UTC");

      const positioning = (rules.brandPositioning as Record<string, unknown> | undefined) ?? {};
      setUvp(typeof positioning.uvp === "string" ? positioning.uvp : "");
      setCompetitors(linesToTextarea(positioning.competitors));
      setTargetKeywords(linesToTextarea(positioning.targetKeywords));
      setComplianceNotes(typeof positioning.complianceNotes === "string" ? positioning.complianceNotes : "");

      const attribution = (rules.attributionDefaults as Record<string, unknown> | undefined) ?? {};
      setUtmSource(typeof attribution.utmSource === "string" ? attribution.utmSource : "");
      setUtmMedium(typeof attribution.utmMedium === "string" ? attribution.utmMedium : "");
      setUtmCampaignPrefix(typeof attribution.utmCampaignPrefix === "string" ? attribution.utmCampaignPrefix : "");
      setDefaultSenderName(typeof attribution.defaultSenderName === "string" ? attribution.defaultSenderName : "");
      setDefaultSenderEmail(typeof attribution.defaultSenderEmail === "string" ? attribution.defaultSenderEmail : "");
      if (
        attribution.primaryChannel === "whatsapp" ||
        attribution.primaryChannel === "email" ||
        attribution.primaryChannel === "omnichannel"
      ) {
        setPrimaryChannel(attribution.primaryChannel);
      }
    }
    setState(loaded);
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  const saveFacts = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;
      const input: WorkspaceFactsInput = {
        name,
        websiteUrl: websiteUrl.trim().length === 0 ? null : websiteUrl.trim(),
        status,
      };
      const problems = validateWorkspaceFacts(input);
      setFactsErrors(problems);
      if (problems.length > 0) return;

      setFactsSaving(true);
      const result: WorkspaceFactsResult = await updateWorkspaceFacts(
        apiBaseUrl,
        workspaceId,
        input,
      );
      setFactsSaving(false);
      if (result.kind === "saved") {
        setFactsErrors([]);
        setState({ kind: "loaded", workspace: result.workspace, brand: state.brand });
        setFactsSavedToast(true);
        setTimeout(() => setFactsSavedToast(false), 4000);
      } else if (result.kind === "rejected") {
        setFactsErrors([result.message]);
      } else {
        setFactsErrors([result.reason]);
      }
    },
    [apiBaseUrl, workspaceId, state, name, websiteUrl, status],
  );

  const saveBrand = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;

      const updatedRules: Record<string, unknown> = {
        ...(state.brand?.rules ?? {}),
        clientContact: {
          name: contactName.trim() || undefined,
          email: contactEmail.trim() || undefined,
          phone: contactPhone.trim() || undefined,
          role: clientRole.trim() || undefined,
          industry: clientIndustry.trim() || undefined,
          timezone: clientTimezone.trim() || undefined,
        },
        brandPositioning: {
          uvp: uvp.trim() || undefined,
          competitors: linesOf(competitors),
          targetKeywords: linesOf(targetKeywords),
          complianceNotes: complianceNotes.trim() || undefined,
        },
        attributionDefaults: {
          utmSource: utmSource.trim() || undefined,
          utmMedium: utmMedium.trim() || undefined,
          utmCampaignPrefix: utmCampaignPrefix.trim() || undefined,
          defaultSenderName: defaultSenderName.trim() || undefined,
          defaultSenderEmail: defaultSenderEmail.trim() || undefined,
          primaryChannel,
        },
      };

      const input = {
        businessDescription: nullable(businessDescription),
        audience: nullable(audience),
        services: nullable(services),
        tone: nullable(tone),
        rules: updatedRules,
        prohibitedClaims: linesOf(prohibited),
        callsToAction: linesOf(ctas),
      };

      setBrandSaving(true);
      setBrandError(null);
      const result: SaveBrandResult = await saveBrandProfile(apiBaseUrl, workspaceId, input);
      setBrandSaving(false);
      if (result.kind === "saved") {
        setBrandSavedAt(result.brand.updatedAt);
        setBrandError(null);
        setState({ kind: "loaded", workspace: state.workspace, brand: result.brand });
        setBrandSavedToast(true);
        setTimeout(() => setBrandSavedToast(false), 4000);
      } else {
        setBrandError(result.kind === "rejected" ? result.message : result.reason);
      }
    },
    [
      apiBaseUrl,
      workspaceId,
      state,
      businessDescription,
      audience,
      services,
      tone,
      prohibited,
      ctas,
      contactName,
      contactEmail,
      contactPhone,
      clientRole,
      clientIndustry,
      clientTimezone,
      uvp,
      competitors,
      targetKeywords,
      complianceNotes,
      utmSource,
      utmMedium,
      utmCampaignPrefix,
      defaultSenderName,
      defaultSenderEmail,
      primaryChannel,
    ],
  );

  const remove = useCallback(
    async (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (state.kind !== "loaded") return;
      setDeleting(true);
      const result: DeleteWorkspaceResult = await deleteWorkspace(
        apiBaseUrl,
        workspaceId,
        confirmSlug,
      );
      setDeleting(false);
      if (result.kind === "deleted") {
        router.push("/workspaces");
      } else {
        setFactsErrors([result.kind === "rejected" ? result.message : result.reason]);
      }
    },
    [apiBaseUrl, workspaceId, state, confirmSlug, router],
  );

  if (state.kind === "loading") {
    return (
      <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-8 text-sm text-slate-500 shadow-sm">
        <svg className="h-5 w-5 animate-spin text-indigo-600" viewBox="0 0 24 24" fill="none">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
        </svg>
        <span>Loading workspace profile and settings…</span>
      </div>
    );
  }

  if (state.kind === "missing") {
    return (
      <div className="rounded-xl border border-dashed border-slate-300 bg-white px-6 py-12 text-center shadow-sm">
        <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-slate-100 text-slate-400">
          <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 className="mt-3 text-base font-bold text-slate-900">Workspace not found</h3>
        <p className="mt-1 text-sm text-slate-500">{state.reason}</p>
        <button
          type="button"
          onClick={() => router.push("/workspaces")}
          className="mt-5 inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50 transition-colors"
        >
          ← Return to workspaces
        </button>
      </div>
    );
  }

  if (state.kind === "unavailable") {
    return (
      <div className="rounded-xl border border-red-200 bg-white p-6 shadow-sm">
        <div className="flex items-start gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-red-100 text-red-600">
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <div>
            <h3 className="text-sm font-bold text-slate-900">Unable to reach workspace service</h3>
            <p className="mt-1 text-sm text-slate-600">{state.reason}</p>
            <button
              type="button"
              onClick={() => void load()}
              className="mt-3 inline-flex items-center justify-center rounded-lg bg-indigo-600 px-3.5 py-2 text-xs font-semibold text-white shadow-sm hover:bg-indigo-700 transition-colors"
            >
              Retry Connection
            </button>
          </div>
        </div>
      </div>
    );
  }

  const { workspace } = state;
  const confirmMatches = confirmSlug.length > 0 && confirmSlug === workspace.slug;

  const tabs: { key: TabKey; label: string; icon: string }[] = [
    { key: "profile", label: "Client & Business", icon: "🏢" },
    { key: "brand", label: "Brand Voice & UVP", icon: "✨" },
    { key: "guardrails", label: "Guardrails & CTAs", icon: "🛡️" },
    { key: "attribution", label: "Attribution & Defaults", icon: "🎯" },
    { key: "channels", label: "Channels & Hubs", icon: "🔗" },
    { key: "danger", label: "Lifecycle & Danger", icon: "⚠️" },
  ];

  return (
    <div className="grid gap-6">
      {/* Workspace Summary Header Card */}
      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-4">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-indigo-700 text-lg font-bold text-white shadow-sm">
              {workspace.name.charAt(0).toUpperCase()}
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-lg font-bold text-slate-900">{workspace.name}</h2>
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold ${
                    workspace.status === "active"
                      ? "bg-emerald-50 text-emerald-700 border border-emerald-200"
                      : "bg-slate-100 text-slate-700 border border-slate-200"
                  }`}
                >
                  {workspace.status === "active" ? "Active Client" : "Archived"}
                </span>
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-slate-500">
                <span>
                  Slug: <code className="rounded bg-slate-100 px-1 py-0.5 font-mono text-slate-700">{workspace.slug}</code>
                </span>
                {workspace.websiteUrl && (
                  <>
                    <span>•</span>
                    <a
                      href={workspace.websiteUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-indigo-600 hover:underline flex items-center gap-1"
                    >
                      {workspace.websiteUrl}
                      <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                      </svg>
                    </a>
                  </>
                )}
                <span>•</span>
                <span>Created {new Date(workspace.createdAt).toLocaleDateString()}</span>
              </div>
            </div>
          </div>

          {/* Quick Hub Navigation */}
          <div className="flex items-center gap-2 pt-2 sm:pt-0 border-t border-slate-100 sm:border-0">
            <Link
              href={`/workspaces/${workspace.id}/lead-engine`}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-100 hover:text-slate-900 transition-colors"
            >
              <span>📱</span> Lead Engine
            </Link>
            <Link
              href={`/workspaces/${workspace.id}/email`}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-100 hover:text-slate-900 transition-colors"
            >
              <span>✉️</span> Email
            </Link>
            <Link
              href={`/workspaces/${workspace.id}/publishing`}
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs font-medium text-slate-700 hover:bg-slate-100 hover:text-slate-900 transition-colors"
            >
              <span>📣</span> Social
            </Link>
          </div>
        </div>
      </div>

      {/* Navigation Tabs */}
      <div className="flex flex-wrap gap-1 rounded-xl border border-slate-200 bg-white p-1.5 shadow-sm">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => setActiveTab(tab.key)}
            className={`inline-flex items-center gap-2 rounded-lg px-3.5 py-2 text-xs font-semibold transition-all ${
              activeTab === tab.key
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-600 hover:bg-slate-100 hover:text-slate-900"
            }`}
          >
            <span>{tab.icon}</span>
            <span>{tab.label}</span>
          </button>
        ))}
      </div>

      {/* TAB 1: CLIENT & BUSINESS PROFILE */}
      {activeTab === "profile" && (
        <div className="grid gap-6">
          <form
            onSubmit={(e) => void saveFacts(e)}
            className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
          >
            <div className="border-b border-slate-100 bg-slate-50/70 px-5 py-3 sm:px-6">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-700">
                Core Workspace & Entity Details
              </h3>
              <p className="mt-0.5 text-xs text-slate-500">
                Fundamental identifiers used in routing, reporting, and dashboard displays.
              </p>
            </div>
            <div className="grid gap-5 p-5 sm:p-6">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="workspace-name" className={labelClasses}>
                    Client / Business Name <span className="text-red-500">*</span>
                  </label>
                  <input
                    id="workspace-name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    maxLength={140}
                    placeholder="e.g. Acme Marketing"
                    required
                    className={inputClasses}
                  />
                  <p className={hintClasses}>Displayed in the sidebar, reports, and headers.</p>
                </div>
                <div>
                  <label htmlFor="workspace-slug" className={labelClasses}>
                    Permanent Workspace Slug
                  </label>
                  <input
                    id="workspace-slug"
                    value={workspace.slug}
                    disabled
                    className="w-full rounded-lg border border-slate-200 bg-slate-100 px-3 py-2.5 text-sm font-mono text-slate-500 cursor-not-allowed"
                  />
                  <p className={hintClasses}>Immutable identifier used in direct URLs and API endpoints.</p>
                </div>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="workspace-website" className={labelClasses}>
                    Official Website URL
                  </label>
                  <input
                    id="workspace-website"
                    type="url"
                    value={websiteUrl}
                    onChange={(e) => setWebsiteUrl(e.target.value)}
                    placeholder="https://example.com"
                    className={inputClasses}
                  />
                  <p className={hintClasses}>Used by AI agents for crawling and context synthesis.</p>
                </div>
                <div>
                  <label htmlFor="workspace-status" className={labelClasses}>
                    Workspace Operational Status
                  </label>
                  <select
                    id="workspace-status"
                    value={status}
                    onChange={(e) => setStatus(e.target.value === "archived" ? "archived" : "active")}
                    className={inputClasses}
                  >
                    <option value="active">Active (Production Operations)</option>
                    <option value="archived">Archived (Suspended Dispatch & Automations)</option>
                  </select>
                  <p className={hintClasses}>Archived workspaces pause all lead discovery and outbound messaging.</p>
                </div>
              </div>

              {factsErrors.length > 0 && (
                <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-xs text-red-800">
                  <ul className="list-disc pl-4 space-y-1">
                    {factsErrors.map((err, i) => (
                      <li key={i}>{err}</li>
                    ))}
                  </ul>
                </div>
              )}

              {factsSavedToast && (
                <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-3 text-xs font-medium text-emerald-800 flex items-center gap-2">
                  <span>✓</span> Workspace details saved successfully.
                </div>
              )}

              <div className="flex justify-end pt-2">
                <button type="submit" disabled={factsSaving} className={PRIMARY_BUTTON}>
                  {factsSaving ? "Saving details…" : "Save Workspace Details"}
                </button>
              </div>
            </div>
          </form>

          {/* Client Contact Details */}
          <form
            onSubmit={(e) => void saveBrand(e)}
            className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
          >
            <div className="border-b border-slate-100 bg-slate-50/70 px-5 py-3 sm:px-6">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-700">
                Client Contact & Stakeholder Information
              </h3>
              <p className="mt-0.5 text-xs text-slate-500">
                Primary client point of contact for campaign approvals, escalations, and billing inquiries.
              </p>
            </div>
            <div className="grid gap-5 p-5 sm:p-6">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="contact-name" className={labelClasses}>
                    Primary Contact Name
                  </label>
                  <input
                    id="contact-name"
                    value={contactName}
                    onChange={(e) => setContactName(e.target.value)}
                    placeholder="e.g. Sarah Connor"
                    className={inputClasses}
                  />
                </div>
                <div>
                  <label htmlFor="contact-email" className={labelClasses}>
                    Primary Contact Email
                  </label>
                  <input
                    id="contact-email"
                    type="email"
                    value={contactEmail}
                    onChange={(e) => setContactEmail(e.target.value)}
                    placeholder="sarah@clientbrand.com"
                    className={inputClasses}
                  />
                </div>
              </div>

              <div className="grid gap-4 sm:grid-cols-3">
                <div>
                  <label htmlFor="contact-phone" className={labelClasses}>
                    Contact Phone / WhatsApp
                  </label>
                  <input
                    id="contact-phone"
                    value={contactPhone}
                    onChange={(e) => setContactPhone(e.target.value)}
                    placeholder="+1 555-0192"
                    className={inputClasses}
                  />
                </div>
                <div>
                  <label htmlFor="client-role" className={labelClasses}>
                    Stakeholder Role / Title
                  </label>
                  <input
                    id="client-role"
                    value={clientRole}
                    onChange={(e) => setClientRole(e.target.value)}
                    placeholder="e.g. VP Marketing / Founder"
                    className={inputClasses}
                  />
                </div>
                <div>
                  <label htmlFor="client-industry" className={labelClasses}>
                    Industry / Market Sector
                  </label>
                  <input
                    id="client-industry"
                    value={clientIndustry}
                    onChange={(e) => setClientIndustry(e.target.value)}
                    placeholder="e.g. B2B SaaS, Dental, Real Estate"
                    className={inputClasses}
                  />
                </div>
              </div>

              <div>
                <label htmlFor="client-timezone" className={labelClasses}>
                  Client Operating Timezone
                </label>
                <select
                  id="client-timezone"
                  value={clientTimezone}
                  onChange={(e) => setClientTimezone(e.target.value)}
                  className={inputClasses}
                >
                  <option value="UTC">UTC (Universal Coordinated Time)</option>
                  <option value="America/New_York">Eastern Time (US / Canada) — America/New_York</option>
                  <option value="America/Chicago">Central Time (US / Canada) — America/Chicago</option>
                  <option value="America/Denver">Mountain Time (US / Canada) — America/Denver</option>
                  <option value="America/Los_Angeles">Pacific Time (US / Canada) — America/Los_Angeles</option>
                  <option value="Europe/London">London / GMT — Europe/London</option>
                  <option value="Europe/Paris">Central Europe — Europe/Paris</option>
                  <option value="Asia/Dubai">Gulf Standard Time — Asia/Dubai</option>
                  <option value="Asia/Singapore">Singapore / Hong Kong — Asia/Singapore</option>
                  <option value="Asia/Tokyo">Tokyo — Asia/Tokyo</option>
                  <option value="Australia/Sydney">Sydney — Australia/Sydney</option>
                </select>
                <p className={hintClasses}>Used to schedule outreach and autonomous WhatsApp responses within client business hours.</p>
              </div>

              {brandSavedToast && (
                <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-3 text-xs font-medium text-emerald-800 flex items-center gap-2">
                  <span>✓</span> Client contact profile saved successfully.
                </div>
              )}

              <div className="flex justify-end pt-2">
                <button type="submit" disabled={brandSaving} className={PRIMARY_BUTTON}>
                  {brandSaving ? "Saving contact…" : "Save Contact Info"}
                </button>
              </div>
            </div>
          </form>
        </div>
      )}

      {/* TAB 2: BRAND VOICE & POSITIONING */}
      {activeTab === "brand" && (
        <form
          onSubmit={(e) => void saveBrand(e)}
          className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
        >
          <div className="border-b border-slate-100 bg-slate-50/70 px-5 py-3 sm:px-6 flex flex-wrap items-baseline justify-between gap-2">
            <div>
              <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-700">
                Brand Persona & Strategic Positioning
              </h3>
              <p className="mt-0.5 text-xs text-slate-500">
                Ingested by the 6 AI specialist agents to generate tailored copy, pitch angles, and content.
              </p>
            </div>
            {brandSavedAt && (
              <span className="text-xs text-slate-400">
                Last updated: {new Date(brandSavedAt).toLocaleString()}
              </span>
            )}
          </div>

          <div className="grid gap-5 p-5 sm:p-6">
            <div>
              <label htmlFor="brand-uvp" className={labelClasses}>
                Unique Value Proposition (UVP) / Tagline
              </label>
              <input
                id="brand-uvp"
                value={uvp}
                onChange={(e) => setUvp(e.target.value)}
                placeholder="e.g. The fastest all-in-one marketing engine for modern home service providers."
                className={inputClasses}
              />
              <p className={hintClasses}>Single sentence defining what makes this brand uniquely compelling.</p>
            </div>

            <div>
              <label htmlFor="brand-desc" className={labelClasses}>
                Comprehensive Business & Product Description
              </label>
              <textarea
                id="brand-desc"
                rows={4}
                value={businessDescription}
                onChange={(e) => setBusinessDescription(e.target.value)}
                placeholder="Detail what the company sells, its market niche, pricing tiers, and main deliverables."
                className={inputClasses}
              />
              <p className={hintClasses}>Provides essential grounding context for outbound pitches and blog posts.</p>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor="brand-audience" className={labelClasses}>
                  Target Audience & ICP (Ideal Customer Profile)
                </label>
                <textarea
                  id="brand-audience"
                  rows={3}
                  value={audience}
                  onChange={(e) => setAudience(e.target.value)}
                  placeholder="e.g. Mid-sized dental clinic owners in California with 2-5 locations."
                  className={inputClasses}
                />
                <p className={hintClasses}>Demographics, pain points, and decision-maker roles.</p>
              </div>
              <div>
                <label htmlFor="brand-services" className={labelClasses}>
                  Core Products & Service Offerings
                </label>
                <textarea
                  id="brand-services"
                  rows={3}
                  value={services}
                  onChange={(e) => setServices(e.target.value)}
                  placeholder="e.g. Same-day emergency implants, Invisalign consulting, preventative hygiene."
                  className={inputClasses}
                />
                <p className={hintClasses}>List packages or primary services mentioned in campaigns.</p>
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-3">
              <div>
                <label htmlFor="brand-tone" className={labelClasses}>
                  Brand Tone of Voice
                </label>
                <input
                  id="brand-tone"
                  value={tone}
                  onChange={(e) => setTone(e.target.value)}
                  placeholder="e.g. Authoritative, witty, empathetic"
                  className={inputClasses}
                />
                <p className={hintClasses}>Adjectives guiding phrasing style and energy.</p>
              </div>
              <div>
                <label htmlFor="brand-competitors" className={labelClasses}>
                  Key Competitors <span className="font-normal text-slate-400">(one per line)</span>
                </label>
                <textarea
                  id="brand-competitors"
                  rows={3}
                  value={competitors}
                  onChange={(e) => setCompetitors(e.target.value)}
                  placeholder={"https://competitor-a.com\nhttps://competitor-b.com"}
                  className={inputClasses}
                />
                <p className={hintClasses}>Used by SEO Advisor and Strategist for differentiation.</p>
              </div>
              <div>
                <label htmlFor="brand-keywords" className={labelClasses}>
                  Focus Target Keywords <span className="font-normal text-slate-400">(one per line)</span>
                </label>
                <textarea
                  id="brand-keywords"
                  rows={3}
                  value={targetKeywords}
                  onChange={(e) => setTargetKeywords(e.target.value)}
                  placeholder={"marketing automation\nlocal seo services"}
                  className={inputClasses}
                />
                <p className={hintClasses}>Priority search queries for content gap and SEO analysis.</p>
              </div>
            </div>

            {brandError && (
              <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-xs text-red-800">
                {brandError}
              </div>
            )}

            {brandSavedToast && (
              <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-3 text-xs font-medium text-emerald-800 flex items-center gap-2">
                <span>✓</span> Brand voice and positioning saved successfully.
              </div>
            )}

            <div className="flex justify-end pt-2">
              <button type="submit" disabled={brandSaving} className={PRIMARY_BUTTON}>
                {brandSaving ? "Saving brand profile…" : "Save Brand Profile"}
              </button>
            </div>
          </div>
        </form>
      )}

      {/* TAB 3: GUARDRAILS & CTAS */}
      {activeTab === "guardrails" && (
        <form
          onSubmit={(e) => void saveBrand(e)}
          className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
        >
          <div className="border-b border-slate-100 bg-slate-50/70 px-5 py-3 sm:px-6">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-700">
              Content Guardrails, Compliance & Calls to Action
            </h3>
            <p className="mt-0.5 text-xs text-slate-500">
              Enforce strict rules on AI copy generation to prevent hallucinations, legal risks, or off-brand promises.
            </p>
          </div>

          <div className="grid gap-5 p-5 sm:p-6">
            <div>
              <label htmlFor="brand-prohibited" className={labelClasses}>
                Prohibited Claims, Topics & Words <span className="font-normal text-slate-400">(one per line)</span>
              </label>
              <textarea
                id="brand-prohibited"
                rows={4}
                value={prohibited}
                onChange={(e) => setProhibited(e.target.value)}
                placeholder={"No guarantee of #1 Google ranking\nNever mention pricing without custom quote\nDo not use the word 'cheap'"}
                className={inputClasses}
              />
              <p className={hintClasses}>
                The AI specialist agents check every drafted pitch or post against these prohibited phrases before returning output.
              </p>
            </div>

            <div>
              <label htmlFor="brand-ctas" className={labelClasses}>
                Approved Calls to Action (CTAs) <span className="font-normal text-slate-400">(one per line)</span>
              </label>
              <textarea
                id="brand-ctas"
                rows={4}
                value={ctas}
                onChange={(e) => setCtas(e.target.value)}
                placeholder={"Book your free 15-minute growth audit\nReply 'YES' to see our case study\nClaim your trial account today"}
                className={inputClasses}
              />
              <p className={hintClasses}>
                Standard concluding pitches inserted into outbound emails and WhatsApp pitches.
              </p>
            </div>

            <div>
              <label htmlFor="brand-compliance-notes" className={labelClasses}>
                Special Compliance, Industry Disclaimers & Regulatory Notes
              </label>
              <textarea
                id="brand-compliance-notes"
                rows={3}
                value={complianceNotes}
                onChange={(e) => setComplianceNotes(e.target.value)}
                placeholder="e.g. HIPAA compliance disclaimer required on all medical advice; Financial disclosures required for tax estimates."
                className={inputClasses}
              />
              <p className={hintClasses}>Additional background instructions injected into the system prompts.</p>
            </div>

            {brandError && (
              <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-xs text-red-800">
                {brandError}
              </div>
            )}

            {brandSavedToast && (
              <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-3 text-xs font-medium text-emerald-800 flex items-center gap-2">
                <span>✓</span> Guardrails and CTAs saved successfully.
              </div>
            )}

            <div className="flex justify-end pt-2">
              <button type="submit" disabled={brandSaving} className={PRIMARY_BUTTON}>
                {brandSaving ? "Saving guardrails…" : "Save Guardrails & CTAs"}
              </button>
            </div>
          </div>
        </form>
      )}

      {/* TAB 4: ATTRIBUTION & DEFAULTS */}
      {activeTab === "attribution" && (
        <form
          onSubmit={(e) => void saveBrand(e)}
          className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm"
        >
          <div className="border-b border-slate-100 bg-slate-50/70 px-5 py-3 sm:px-6">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-700">
              Campaign Attribution & Outreach Defaults
            </h3>
            <p className="mt-0.5 text-xs text-slate-500">
              Configure UTM tracking parameters, sender addresses, and outbound routing for this client workspace.
            </p>
          </div>

          <div className="grid gap-5 p-5 sm:p-6">
            <div className="grid gap-4 sm:grid-cols-3">
              <div>
                <label htmlFor="utm-source" className={labelClasses}>
                  Default UTM Source
                </label>
                <input
                  id="utm-source"
                  value={utmSource}
                  onChange={(e) => setUtmSource(e.target.value)}
                  placeholder="e.g. whatsapp_outreach"
                  className={inputClasses}
                />
                <p className={hintClasses}>Appended to links: <code className="text-xs">utm_source=...</code></p>
              </div>
              <div>
                <label htmlFor="utm-medium" className={labelClasses}>
                  Default UTM Medium
                </label>
                <input
                  id="utm-medium"
                  value={utmMedium}
                  onChange={(e) => setUtmMedium(e.target.value)}
                  placeholder="e.g. direct_message"
                  className={inputClasses}
                />
                <p className={hintClasses}>Appended to links: <code className="text-xs">utm_medium=...</code></p>
              </div>
              <div>
                <label htmlFor="utm-campaign" className={labelClasses}>
                  Campaign Prefix
                </label>
                <input
                  id="utm-campaign"
                  value={utmCampaignPrefix}
                  onChange={(e) => setUtmCampaignPrefix(e.target.value)}
                  placeholder="e.g. q4-growth-"
                  className={inputClasses}
                />
                <p className={hintClasses}>Prefix added before generated campaign slugs.</p>
              </div>
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor="sender-name" className={labelClasses}>
                  Default Outbound Sender Name
                </label>
                <input
                  id="sender-name"
                  value={defaultSenderName}
                  onChange={(e) => setDefaultSenderName(e.target.value)}
                  placeholder="e.g. Growth Team at Acme"
                  className={inputClasses}
                />
                <p className={hintClasses}>Appears as the From name on outbound pitch emails.</p>
              </div>
              <div>
                <label htmlFor="sender-email" className={labelClasses}>
                  Default Outbound Sender Email
                </label>
                <input
                  id="sender-email"
                  type="email"
                  value={defaultSenderEmail}
                  onChange={(e) => setDefaultSenderEmail(e.target.value)}
                  placeholder="outreach@acmemarketing.com"
                  className={inputClasses}
                />
                <p className={hintClasses}>Must belong to a verified domain in your Resend account.</p>
              </div>
            </div>

            <div>
              <label htmlFor="primary-channel" className={labelClasses}>
                Primary Outreach Channel
              </label>
              <select
                id="primary-channel"
                value={primaryChannel}
                onChange={(e) => setPrimaryChannel(e.target.value as "whatsapp" | "email" | "omnichannel")}
                className={inputClasses}
              >
                <option value="whatsapp">WhatsApp Direct Dispatch (High Open Rates)</option>
                <option value="email">Cold Email Dispatch (Resend Engine)</option>
                <option value="omnichannel">Omnichannel (WhatsApp first, fallback to Email)</option>
              </select>
              <p className={hintClasses}>
                Defines which dispatch engine is selected by default when running lead discovery batches.
              </p>
            </div>

            {brandSavedToast && (
              <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-3 text-xs font-medium text-emerald-800 flex items-center gap-2">
                <span>✓</span> Attribution and campaign defaults saved successfully.
              </div>
            )}

            <div className="flex justify-end pt-2">
              <button type="submit" disabled={brandSaving} className={PRIMARY_BUTTON}>
                {brandSaving ? "Saving attribution…" : "Save Campaign Defaults"}
              </button>
            </div>
          </div>
        </form>
      )}

      {/* TAB 5: CHANNELS & HUBS */}
      {activeTab === "channels" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between">
                <span className="text-2xl">📱</span>
                <span className="rounded-full bg-emerald-50 border border-emerald-200 px-2 py-0.5 text-xs font-medium text-emerald-700">
                  Ready
                </span>
              </div>
              <h4 className="mt-3 text-sm font-bold text-slate-900">WhatsApp Lead Engine</h4>
              <p className="mt-1 text-xs text-slate-500">
                Google Places discovery, QR pairing, automated pitch dispatching, and 2-way AI conversational auto-replies.
              </p>
            </div>
            <Link
              href={`/workspaces/${workspace.id}/lead-engine`}
              className="mt-4 inline-flex items-center justify-center gap-2 rounded-lg bg-emerald-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-emerald-700 transition-colors"
            >
              Open WhatsApp Lead Engine →
            </Link>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between">
                <span className="text-2xl">✉️</span>
                <span className="rounded-full bg-indigo-50 border border-indigo-200 px-2 py-0.5 text-xs font-medium text-indigo-700">
                  Resend API
                </span>
              </div>
              <h4 className="mt-3 text-sm font-bold text-slate-900">Outbound Email Engine</h4>
              <p className="mt-1 text-xs text-slate-500">
                Audience segmentation, drip campaign dispatching, bounce management, and delivery log analytics.
              </p>
            </div>
            <Link
              href={`/workspaces/${workspace.id}/email`}
              className="mt-4 inline-flex items-center justify-center gap-2 rounded-lg bg-indigo-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-700 transition-colors"
            >
              Open Email Campaigns →
            </Link>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between">
                <span className="text-2xl">📣</span>
                <span className="rounded-full bg-amber-50 border border-amber-200 px-2 py-0.5 text-xs font-medium text-amber-700">
                  Multi-Channel
                </span>
              </div>
              <h4 className="mt-3 text-sm font-bold text-slate-900">Social Publishing & Content</h4>
              <p className="mt-1 text-xs text-slate-500">
                Drafting, scheduling, media uploads, and post calendar for Facebook, Instagram, X (Twitter), and YouTube.
              </p>
            </div>
            <Link
              href={`/workspaces/${workspace.id}/publishing`}
              className="mt-4 inline-flex items-center justify-center gap-2 rounded-lg bg-amber-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-amber-700 transition-colors"
            >
              Open Publishing Hub →
            </Link>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between">
                <span className="text-2xl">🤖</span>
                <span className="rounded-full bg-purple-50 border border-purple-200 px-2 py-0.5 text-xs font-medium text-purple-700">
                  6 Specialists
                </span>
              </div>
              <h4 className="mt-3 text-sm font-bold text-slate-900">AI Specialist Marketing Agents</h4>
              <p className="mt-1 text-xs text-slate-500">
                On-demand prompts for SEO audits, search gap articles, high-converting channel copy, and strategy plans.
              </p>
            </div>
            <Link
              href={`/workspaces/${workspace.id}/agents`}
              className="mt-4 inline-flex items-center justify-center gap-2 rounded-lg bg-purple-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-purple-700 transition-colors"
            >
              Open Marketing Agents →
            </Link>
          </div>
        </div>
      )}

      {/* TAB 6: DANGER ZONE */}
      {activeTab === "danger" && (
        <div className="grid gap-6">
          {/* Archive Notice */}
          <div className="rounded-xl border border-amber-200 bg-amber-50/70 p-5 shadow-sm">
            <h4 className="text-sm font-bold text-amber-900">Need to pause operations temporarily?</h4>
            <p className="mt-1 text-xs text-amber-800 leading-relaxed">
              Instead of permanently deleting this workspace, you can switch its status to <strong>Archived</strong> under the{" "}
              <button
                type="button"
                onClick={() => setActiveTab("profile")}
                className="font-semibold underline hover:text-amber-950"
              >
                Client & Business tab
              </button>
              . Archiving halts all outbound queues, lead discovery, and auto-replies while keeping historical campaign data intact.
            </p>
          </div>

          {/* Permanent Deletion Card */}
          <section
            aria-labelledby="danger-zone-heading"
            className="overflow-hidden rounded-xl border border-red-200 bg-white shadow-sm"
          >
            <div className="border-b border-red-100 bg-red-50/70 px-5 py-3 sm:px-6">
              <h3
                id="danger-zone-heading"
                className="text-xs font-semibold uppercase tracking-wider text-red-700 flex items-center gap-2"
              >
                <span>⚠️</span> Irreversible Workspace Deletion
              </h3>
            </div>
            <div className="p-5 sm:p-6">
              <p className="max-w-prose text-sm text-red-900 leading-relaxed">
                Deleting <strong>{workspace.name}</strong> permanently purges all linked brand profiles, leads, WhatsApp chat histories, publishing jobs, delivery logs, and uploaded media from the database.
              </p>

              <form onSubmit={(event) => void remove(event)} className="mt-5 grid gap-4 sm:max-w-md">
                <div>
                  <label htmlFor="delete-confirm" className={labelClasses}>
                    Type <code className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs text-red-700">{workspace.slug}</code> to confirm:
                  </label>
                  <input
                    id="delete-confirm"
                    value={confirmSlug}
                    onChange={(event) => setConfirmSlug(event.target.value)}
                    placeholder={workspace.slug}
                    autoComplete="off"
                    className="w-full rounded-lg border border-red-200 bg-red-50/40 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-red-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-500 transition-colors"
                  />
                </div>

                <div>
                  <button
                    type="submit"
                    disabled={deleting || !confirmMatches}
                    className={DANGER_BUTTON}
                  >
                    {deleting ? "Deleting permanently…" : "Delete workspace forever"}
                  </button>
                </div>
              </form>
            </div>
          </section>
        </div>
      )}
    </div>
  );
}
