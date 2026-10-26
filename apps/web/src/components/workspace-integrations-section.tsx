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
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";
const PRIMARY_BUTTON =
  "rounded-md bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";
const SECONDARY_BUTTON =
  "rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60";

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
  const [sanityDocType, setSanityDocType] = useState("post");
  const [sanityPublicUrlPrefix, setSanityPublicUrlPrefix] = useState("/posts/");
  const [genericBaseUrl, setGenericBaseUrl] = useState("");
  const [searchConsoleProperty, setSearchConsoleProperty] = useState("");
  const [metaChannel, setMetaChannel] = useState("facebook");
  const [metaTargetId, setMetaTargetId] = useState("");
  const [metaApiVersion, setMetaApiVersion] = useState("");
  const [metaPostLinkPrefix, setMetaPostLinkPrefix] = useState("");
  const [xPostLinkPrefix, setXPostLinkPrefix] = useState("");
  const [youtubeChannelId, setYoutubeChannelId] = useState("");
  const [youtubePrivacy, setYoutubePrivacy] = useState("public");
  const [youtubeCategoryId, setYoutubeCategoryId] = useState("");
  const [youtubeClientId, setYoutubeClientId] = useState("");
  const [youtubeClientSecret, setYoutubeClientSecret] = useState("");
  const [youtubeRefreshToken, setYoutubeRefreshToken] = useState("");
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
      config.docType = sanityDocType.trim() || "post";
      config.publicUrlPrefix = sanityPublicUrlPrefix.trim() || "/posts/";
      config.docType = sanityDocType.trim() || "post";
      config.publicUrlPrefix = sanityPublicUrlPrefix.trim() || "/posts/";
    } else if (adapterKind === "generic_api") {
      if (!genericBaseUrl.trim()) {
        setAddErrors(["Base API URL is required"]);
        return;
      }
      config.baseUrl = genericBaseUrl.trim();
    } else if (adapterKind === "search_console") {
      if (!searchConsoleProperty.trim()) {
        setAddErrors([
          "Search Console property is required (for example https://example.com/ or sc-domain:example.com)",
        ]);
        return;
      }
      config.property = searchConsoleProperty.trim();
    } else if (adapterKind === "meta") {
      if (!metaTargetId.trim()) {
        setAddErrors([
          "Target ID is required (the Facebook Page ID or the Instagram business account ID)",
        ]);
        return;
      }
      config.channel = metaChannel;
      config.targetId = metaTargetId.trim();
      if (metaApiVersion.trim()) {
        config.apiVersion = metaApiVersion.trim();
      }
      if (metaPostLinkPrefix.trim()) {
        config.postLinkPrefix = metaPostLinkPrefix.trim();
      }
    } else if (adapterKind === "youtube") {
      if (!youtubeChannelId.trim()) {
        setAddErrors(["YouTube channel ID is required (the UC… id of the authorized channel)"]);
        return;
      }
      config.channelId = youtubeChannelId.trim();
      if (youtubePrivacy) {
        config.privacyStatus = youtubePrivacy;
      }
      if (youtubeCategoryId.trim()) {
        config.categoryId = youtubeCategoryId.trim();
      }
    } else if (adapterKind === "x") {
      if (xPostLinkPrefix.trim()) {
        config.postLinkPrefix = xPostLinkPrefix.trim();
      }
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
      setSanityDataset("production");
      setSanityDocType("post");
      setSanityPublicUrlPrefix("/posts/");
      setGenericBaseUrl("");
      setSearchConsoleProperty("");
      setMetaChannel("facebook");
      setMetaTargetId("");
      setMetaApiVersion("");
      setMetaPostLinkPrefix("");
      setYoutubeChannelId("");
      setYoutubePrivacy("public");
      setYoutubeCategoryId("");
      setXPostLinkPrefix("");
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
    } else if (kind === "search_console") {
      payload.serviceAccount = credSecret.trim();
    } else if (kind === "meta") {
      payload.accessToken = credSecret.trim();
    } else if (kind === "x") {
      payload.accessToken = credSecret.trim();
    } else if (kind === "youtube") {
      if (!youtubeClientId.trim() || !youtubeClientSecret.trim() || !youtubeRefreshToken.trim()) {
        setCredSaving(false);
        setCredError("Client ID, client secret, and refresh token are all required.");
        return;
      }
      payload.clientId = youtubeClientId.trim();
      payload.clientSecret = youtubeClientSecret.trim();
      payload.refreshToken = youtubeRefreshToken.trim();
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
      setYoutubeClientId("");
      setYoutubeClientSecret("");
      setYoutubeRefreshToken("");
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
          <h2
            id="workspace-integrations-heading"
            className="text-xs font-semibold uppercase tracking-widest text-slate-500"
          >
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
          <h3 className="text-sm font-semibold text-slate-800">
            Connect this workspace&apos;s publishing destination
          </h3>
          <p className="mt-1 text-xs text-slate-600">
            Enter the business&apos;s Sanity project details below. The API token is saved
            separately as an encrypted server-side credential; never paste it into the project ID or
            dataset fields.
          </p>
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
                placeholder="e.g. Main website blog"
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
              <div>
                <label htmlFor="sanity-doctype" className={labelClasses}>
                  Article document type
                </label>
                <input
                  id="sanity-doctype"
                  type="text"
                  value={sanityDocType}
                  onChange={(e) => setSanityDocType(e.target.value)}
                  placeholder="post"
                  className={`mt-1 ${inputClasses}`}
                />
                <p className="mt-1 text-xs text-slate-500">
                  Must match the document type in this project&apos;s Sanity schema.
                </p>
              </div>
              <div>
                <label htmlFor="sanity-url-prefix" className={labelClasses}>
                  Public article path prefix
                </label>
                <input
                  id="sanity-url-prefix"
                  type="text"
                  value={sanityPublicUrlPrefix}
                  onChange={(e) => setSanityPublicUrlPrefix(e.target.value)}
                  placeholder="/blog/"
                  className={`mt-1 ${inputClasses}`}
                />
                <p className="mt-1 text-xs text-slate-500">
                  Used to verify websiteUrl + path prefix + article slug after publishing.
                </p>
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

          {adapterKind === "meta" && (
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label htmlFor="meta-channel" className={labelClasses}>
                  Channel
                </label>
                <select
                  id="meta-channel"
                  value={metaChannel}
                  onChange={(e) => setMetaChannel(e.target.value)}
                  className={`mt-1 ${inputClasses}`}
                >
                  <option value="facebook">Facebook Page post</option>
                  <option value="instagram">Instagram business post</option>
                </select>
                <p className="mt-1 text-xs text-slate-500">
                  One integration posts to exactly one channel. Connect a second one for the other
                  channel.
                </p>
              </div>
              <div>
                <label htmlFor="meta-target" className={labelClasses}>
                  Target ID
                </label>
                <input
                  id="meta-target"
                  type="text"
                  value={metaTargetId}
                  onChange={(e) => setMetaTargetId(e.target.value)}
                  placeholder={
                    metaChannel === "facebook"
                      ? "e.g. 123456789012345 (Page ID)"
                      : "e.g. 17841400000000000 (IG user ID)"
                  }
                  required
                  className={`mt-1 ${inputClasses}`}
                />
              </div>
              <div>
                <label htmlFor="meta-link-prefix" className={labelClasses}>
                  Article link prefix (optional, Facebook only)
                </label>
                <input
                  id="meta-link-prefix"
                  type="url"
                  value={metaPostLinkPrefix}
                  onChange={(e) => setMetaPostLinkPrefix(e.target.value)}
                  placeholder="https://example.com/blog"
                  className={`mt-1 ${inputClasses}`}
                />
                <p className="mt-1 text-xs text-slate-500">
                  When set, Facebook posts link to prefix + the article slug, and a repeated publish
                  reuses the earlier post instead of duplicating it.
                </p>
              </div>
              <div>
                <label htmlFor="meta-api-version" className={labelClasses}>
                  Graph API version (optional)
                </label>
                <input
                  id="meta-api-version"
                  type="text"
                  value={metaApiVersion}
                  onChange={(e) => setMetaApiVersion(e.target.value)}
                  placeholder="v21.0"
                  className={`mt-1 ${inputClasses}`}
                />
              </div>
              <p className="sm:col-span-2 text-xs text-slate-500">
                Official Graph API only, with an access token that carries the channel&apos;s
                manage-posts permission. Instagram posts additionally require a public image on the
                approved version.
              </p>
            </div>
          )}

          {adapterKind === "youtube" && (
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label htmlFor="youtube-channel" className={labelClasses}>
                  Channel ID
                </label>
                <input
                  id="youtube-channel"
                  type="text"
                  value={youtubeChannelId}
                  onChange={(e) => setYoutubeChannelId(e.target.value)}
                  placeholder="e.g. UCxxxxxxxxxxxxxxxxxxxxxx"
                  required
                  className={`mt-1 ${inputClasses}`}
                />
                <p className="mt-1 text-xs text-slate-500">
                  The UC… id of the channel the OAuth authorization belongs to (shown on YouTube
                  under account → advanced settings).
                </p>
              </div>
              <div>
                <label htmlFor="youtube-privacy" className={labelClasses}>
                  Privacy status
                </label>
                <select
                  id="youtube-privacy"
                  value={youtubePrivacy}
                  onChange={(e) => setYoutubePrivacy(e.target.value)}
                  className={`mt-1 ${inputClasses}`}
                >
                  <option value="public">Public</option>
                  <option value="unlisted">Unlisted</option>
                  <option value="private">Private</option>
                </select>
              </div>
              <div>
                <label htmlFor="youtube-category" className={labelClasses}>
                  Category ID (optional)
                </label>
                <input
                  id="youtube-category"
                  type="text"
                  value={youtubeCategoryId}
                  onChange={(e) => setYoutubeCategoryId(e.target.value)}
                  placeholder="22 (People &amp; Blogs)"
                  className={`mt-1 ${inputClasses}`}
                />
                <p className="mt-1 text-xs text-slate-500">
                  Numeric YouTube video category; 22 (People &amp; Blogs) is the default when left
                  empty.
                </p>
              </div>
              <p className="sm:col-span-2 text-xs text-slate-500">
                Official YouTube Data API only, with an OAuth refresh token minted with the
                youtube.upload scope. Uploads also require a public video file URL on the approved
                version (metadata key&nbsp;
                <code className="rounded bg-slate-200 px-1">videoUrl</code>) and each upload costs a
                large slice of the project&apos;s daily API quota.
              </p>
            </div>
          )}

          {adapterKind === "x" && (
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label htmlFor="x-link-prefix" className={labelClasses}>
                  Article link prefix (optional)
                </label>
                <input
                  id="x-link-prefix"
                  type="url"
                  value={xPostLinkPrefix}
                  onChange={(e) => setXPostLinkPrefix(e.target.value)}
                  placeholder="https://example.com/blog"
                  className={`mt-1 ${inputClasses}`}
                />
                <p className="mt-1 text-xs text-slate-500">
                  When set, each tweet tags the article link (prefix + slug) after the post text,
                  and a repeated publish reuses the earlier tweet instead of duplicating it.
                </p>
              </div>
              <p className="self-end text-xs text-slate-500 sm:col-span-1">
                Official X API v2 only, with an OAuth 2.0 user access token carrying the tweet.write
                scope. Every publish is still approval-gated here; scheduled tweets run through this
                platform&apos;s worker.
              </p>
            </div>
          )}

          {adapterKind === "search_console" && (
            <div className="mt-3">
              <label htmlFor="search-console-property" className={labelClasses}>
                Search Console property
              </label>
              <input
                id="search-console-property"
                type="text"
                value={searchConsoleProperty}
                onChange={(e) => setSearchConsoleProperty(e.target.value)}
                placeholder="https://example.com/ or sc-domain:example.com"
                required
                className={`mt-1 ${inputClasses}`}
              />
              <p className="mt-1 text-xs text-slate-500">
                URL-prefix or domain property. Read-only: the service account only needs read
                access; the check never verifies or modifies anything.
              </p>
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
                      setYoutubeClientId("");
                      setYoutubeClientSecret("");
                      setYoutubeRefreshToken("");
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

              {(inItem.status === "error" || inItem.status === "disconnected") && (
                <div
                  role="status"
                  data-testid="reconnect-banner"
                  className="mt-3 rounded-md bg-red-50 px-3 py-2 text-xs ring-1 ring-inset ring-red-700/15"
                >
                  <p className="font-medium text-red-900">Needs reconnection</p>
                  <p className="mt-1 text-red-800">
                    The saved credentials were rejected or expired — for example an access token
                    that was revoked, or an OAuth refresh token whose authorization ended.
                    Publishing stays blocked until you save new credentials and the check passes
                    again.
                  </p>
                  <button
                    type="button"
                    onClick={() => setCredIntegrationId(inItem.id)}
                    className="mt-2 rounded-md bg-red-700 px-2 py-1 text-xs font-semibold text-white hover:bg-red-800"
                  >
                    Reconnect now
                  </button>
                </div>
              )}
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
                  {inItem.adapterKind === "youtube" && (
                    <div className="grid gap-3 sm:grid-cols-3">
                      <div>
                        <label htmlFor={`cred-yt-id-${inItem.id}`} className={labelClasses}>
                          OAuth client ID
                        </label>
                        <input
                          id={`cred-yt-id-${inItem.id}`}
                          type="password"
                          value={youtubeClientId}
                          onChange={(e) => setYoutubeClientId(e.target.value)}
                          className={`mt-1 ${inputClasses}`}
                          required
                        />
                      </div>
                      <div>
                        <label htmlFor={`cred-yt-secret-${inItem.id}`} className={labelClasses}>
                          OAuth client secret
                        </label>
                        <input
                          id={`cred-yt-secret-${inItem.id}`}
                          type="password"
                          value={youtubeClientSecret}
                          onChange={(e) => setYoutubeClientSecret(e.target.value)}
                          className={`mt-1 ${inputClasses}`}
                          required
                        />
                      </div>
                      <div>
                        <label htmlFor={`cred-yt-refresh-${inItem.id}`} className={labelClasses}>
                          Refresh token (youtube.upload scope)
                        </label>
                        <input
                          id={`cred-yt-refresh-${inItem.id}`}
                          type="password"
                          value={youtubeRefreshToken}
                          onChange={(e) => setYoutubeRefreshToken(e.target.value)}
                          className={`mt-1 ${inputClasses}`}
                          required
                        />
                      </div>
                      <p className="sm:col-span-3 text-xs text-slate-500">
                        Mint the refresh token offline for this Google Cloud project with the
                        youtube.upload scope; the adapter exchanges it server-side for short-lived
                        access tokens. All three values are encrypted at rest with AES-256-GCM,
                        never returned to the browser, and never logged.
                      </p>
                    </div>
                  )}
                  {inItem.adapterKind !== "youtube" && (
                    <label htmlFor={`cred-${inItem.id}`} className={labelClasses}>
                      {inItem.adapterKind === "sanity"
                        ? "Sanity API write token"
                        : inItem.adapterKind === "x"
                          ? "X OAuth 2.0 user access token"
                          : inItem.adapterKind === "search_console"
                            ? "Google service-account key (JSON)"
                            : inItem.adapterKind === "meta"
                              ? "Meta Graph access token"
                              : "API secret key / token"}
                    </label>
                  )}
                  {inItem.adapterKind !== "youtube" && (
                    <>
                      <p className="text-xs text-slate-500">
                        {inItem.adapterKind === "search_console"
                          ? "Paste the full JSON of a Google service-account key. It is encrypted at rest with AES-256-GCM, never returned to the browser or logged, and is used read-only (webmasters.readonly) to verify property access and read search analytics."
                          : inItem.adapterKind === "meta"
                            ? "Paste the access token for this channel's target: a Page access token, or the Instagram-linked Page token. It is encrypted at rest with AES-256-GCM, never returned to the browser or logged, and needs only the channel's manage-posts permission (pages_manage_posts or instagram_content_publish)."
                            : inItem.adapterKind === "x"
                              ? "Paste an OAuth 2.0 user access token minted with the tweet.write scope for the posting account. It is encrypted at rest with AES-256-GCM, never returned to the browser or logged, and is verified against /users/me read-only when saved."
                              : "This is the Sanity API token for this workspace's project. It is encrypted at rest with AES-256-GCM, never returned to the browser or logged, and should have only the write permissions needed for this site."}
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
                    </>
                  )}
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
