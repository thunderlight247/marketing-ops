"use client";

import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  addNewsletterSubscriber,
  listNewsletterSubscribers,
  removeNewsletterSubscriber,
  type NewsletterSubscriber,
} from "@/lib/newsletter";

const inputClasses =
  "w-full rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-slate-900 placeholder:text-slate-400 focus:bg-white focus:border-indigo-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 transition-colors";
const labelClasses = "block text-xs font-semibold uppercase tracking-wide text-slate-500";
const PRIMARY_BUTTON =
  "rounded-lg bg-indigo-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 transition-colors";
const SECONDARY_BUTTON =
  "rounded-lg border border-slate-300 bg-white px-3.5 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 transition-colors";

// WorkspaceNewsletterSection keeps this workspace's newsletter subscriber list.
// Addresses are added here by the operator. An address that has unsubscribed
// is refused, so it cannot be re-added by accident.
export function WorkspaceNewsletterSection({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const apiBaseUrl = getApiBaseUrl();
  const [subscribers, setSubscribers] = useState<NewsletterSubscriber[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const rows = await listNewsletterSubscribers(apiBaseUrl, workspaceId);
        if (!cancelled) {
          setSubscribers(rows);
          setLoadError(null);
        }
      } catch (err) {
        if (!cancelled) setLoadError(err instanceof Error ? err.message : "Could not load subscribers.");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, workspaceId, reloadKey]);

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault();
    setFormError(null);
    setSaving(true);
    try {
      await addNewsletterSubscriber(apiBaseUrl, workspaceId, email.trim(), name.trim());
      setEmail("");
      setName("");
      setReloadKey((k) => k + 1);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Could not add subscriber.");
    } finally {
      setSaving(false);
    }
  }

  async function handleRemove(address: string) {
    setLoadError(null);
    try {
      await removeNewsletterSubscriber(apiBaseUrl, workspaceId, address);
      setReloadKey((k) => k + 1);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : "Could not remove subscriber.");
    }
  }

  return (
    <section aria-labelledby="newsletter-heading" className="grid gap-4">
      <div>
        <h2 id="newsletter-heading" className="text-lg font-semibold text-slate-900">
          Newsletter subscribers
        </h2>
        <p className="text-sm text-slate-500">
          The list of people who receive this workspace&apos;s newsletter. Anyone who unsubscribes
          from an email is kept off the list.
        </p>
      </div>

      <form onSubmit={handleAdd} className="flex flex-wrap items-end gap-3">
        <div className="grid gap-1 min-w-[14rem] flex-1">
          <label htmlFor="newsletter-email" className={labelClasses}>
            Email
          </label>
          <input
            id="newsletter-email"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className={inputClasses}
          />
        </div>
        <div className="grid gap-1 min-w-[10rem] flex-1">
          <label htmlFor="newsletter-name" className={labelClasses}>
            Name <span className="font-normal normal-case text-slate-400">(optional)</span>
          </label>
          <input
            id="newsletter-name"
            type="text"
            maxLength={120}
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={inputClasses}
          />
        </div>
        <button type="submit" disabled={saving} className={PRIMARY_BUTTON}>
          {saving ? "Adding…" : "Add subscriber"}
        </button>
      </form>
      {formError && (
        <p role="alert" className="text-sm text-red-700">
          {formError}
        </p>
      )}

      {loadError && (
        <p role="alert" className="text-sm text-red-700">
          {loadError}
        </p>
      )}

      {subscribers === null && !loadError && <p className="text-sm text-slate-500">Loading…</p>}

      {subscribers !== null && subscribers.length === 0 && (
        <p className="text-sm text-slate-500">No subscribers yet.</p>
      )}

      {subscribers !== null && subscribers.length > 0 && (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-left">
                <th className="px-5 py-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Email</th>
                <th className="px-4 py-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Name</th>
                <th className="px-4 py-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Added</th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {subscribers.map((s) => (
                <tr key={s.email} className="border-b border-slate-100 last:border-0">
                  <td className="px-5 py-2 text-slate-800 break-all">{s.email}</td>
                  <td className="px-4 py-2 text-slate-600">{s.name || "—"}</td>
                  <td className="px-4 py-2 text-slate-600 whitespace-nowrap">
                    {new Date(s.createdAt).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-2 text-right">
                    <button
                      type="button"
                      onClick={() => void handleRemove(s.email)}
                      className={SECONDARY_BUTTON}
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
