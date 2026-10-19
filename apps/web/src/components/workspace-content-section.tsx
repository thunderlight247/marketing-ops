"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import {
  createWorkspaceContent,
  fetchWorkspaceContent,
  formatContentKind,
  formatContentStatus,
  validateContentInput,
  type ContentItem,
  type ContentKind,
  type ContentListState,
  type CreateContentResult,
} from "@/lib/content";

const inputClasses =
  "w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";
const labelClasses = "block text-sm font-medium text-slate-700";
const PRIMARY_BUTTON =
  "rounded-md bg-teal-800 px-4 py-2 text-sm font-semibold text-white hover:bg-teal-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700 disabled:cursor-not-allowed disabled:opacity-60";

function kindBadgeClass(kind: ContentKind): string {
  switch (kind) {
    case "blog_article":
      return "bg-sky-50 text-sky-800 ring-1 ring-inset ring-sky-700/20";
    case "social_post":
      return "bg-purple-50 text-purple-800 ring-1 ring-inset ring-purple-700/20";
    case "snippet":
      return "bg-amber-50 text-amber-800 ring-1 ring-inset ring-amber-700/20";
    default:
      return "bg-slate-100 text-slate-700 ring-1 ring-inset ring-slate-600/20";
  }
}

export function WorkspaceContentSection({
  workspaceId,
}: Readonly<{
  workspaceId: string;
}>) {
  const [state, setState] = useState<ContentListState>({ kind: "loading" });
  const [showForm, setShowForm] = useState(false);

  // Form fields
  const [title, setTitle] = useState("");
  const [kind, setKind] = useState<ContentKind>("blog_article");
  const [slug, setSlug] = useState("");
  const [formErrors, setFormErrors] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  const apiBaseUrl = getApiBaseUrl();

  const load = useCallback(async () => {
    setState({ kind: "loading" });
    const res = await fetchWorkspaceContent(apiBaseUrl, workspaceId);
    setState(res);
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setServerError(null);

    const problems = validateContentInput({ kind, title, slug: slug || undefined });
    if (problems.length > 0) {
      setFormErrors(problems);
      return;
    }
    setFormErrors([]);
    setSubmitting(true);

    const res: CreateContentResult = await createWorkspaceContent(apiBaseUrl, workspaceId, {
      kind,
      title,
      slug: slug || undefined,
    });

    setSubmitting(false);

    if (res.kind === "created") {
      setTitle("");
      setSlug("");
      setKind("blog_article");
      setShowForm(false);
      void load();
    } else if (res.kind === "rejected") {
      setFormErrors(res.problems);
    } else if (res.kind === "conflict") {
      setServerError(res.message);
    } else {
      setServerError(res.message);
    }
  }

  return (
    <section
      aria-labelledby="workspace-content-heading"
      className="rounded-lg border border-slate-200 bg-white px-5 py-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-4">
        <div>
          <h2 id="workspace-content-heading" className="text-base font-semibold">
            Content items
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            Drafts and planning for this workspace. Content must be approved before publishing.
          </p>
        </div>
        <div>
          <button
            type="button"
            onClick={() => setShowForm((prev) => !prev)}
            className="rounded-md border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
          >
            {showForm ? "Cancel" : "+ New draft"}
          </button>
        </div>
      </div>

      {showForm && (
        <form
          onSubmit={(e) => void handleCreate(e)}
          className="mt-4 rounded-md border border-slate-200 bg-slate-50 p-4"
        >
          <h3 className="text-sm font-semibold text-slate-800">Create new draft</h3>
          <div className="mt-3 grid gap-3 sm:grid-cols-3">
            <div className="sm:col-span-2">
              <label htmlFor="content-title" className={labelClasses}>
                Title
              </label>
              <input
                id="content-title"
                type="text"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="e.g. 5 Simple Ways to Enjoy Ghanaian Foods"
                maxLength={200}
                required
                className={`mt-1 ${inputClasses}`}
              />
            </div>
            <div>
              <label htmlFor="content-kind" className={labelClasses}>
                Kind
              </label>
              <select
                id="content-kind"
                value={kind}
                onChange={(e) => setKind(e.target.value as ContentKind)}
                className={`mt-1 ${inputClasses}`}
              >
                <option value="blog_article">Blog article</option>
                <option value="social_post">Social post</option>
                <option value="snippet">Snippet</option>
              </select>
            </div>
          </div>

          <div className="mt-3">
            <label htmlFor="content-slug" className={labelClasses}>
              Slug <span className="font-normal text-slate-500">(optional)</span>
            </label>
            <input
              id="content-slug"
              type="text"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              placeholder="e.g. 5-ways-ghanaian-foods"
              className={`mt-1 ${inputClasses}`}
            />
            <p className="mt-1 text-xs text-slate-500">
              Lowercase letters, digits, and hyphens. Unique per workspace.
            </p>
          </div>

          {formErrors.length > 0 && (
            <ul role="alert" className="mt-3 list-disc space-y-1 pl-5 text-sm text-red-700">
              {formErrors.map((err) => (
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
            <button type="submit" disabled={submitting} className={PRIMARY_BUTTON}>
              {submitting ? "Creating…" : "Save draft"}
            </button>
            <button
              type="button"
              onClick={() => setShowForm(false)}
              className="rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="mt-4">
        {state.kind === "loading" && (
          <p className="py-6 text-center text-sm text-slate-500">Loading content items…</p>
        )}

        {state.kind === "missing" && (
          <p className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900 ring-1 ring-inset ring-amber-700/15">
            {state.message}
          </p>
        )}

        {state.kind === "unavailable" && (
          <div className="rounded-md bg-red-50 p-4 ring-1 ring-inset ring-red-700/15">
            <p className="text-sm font-medium text-red-900">Could not load content</p>
            <p className="mt-1 text-sm text-red-800">{state.message}</p>
            <button
              type="button"
              onClick={() => void load()}
              className="mt-3 text-xs font-semibold text-red-900 underline hover:no-underline"
            >
              Try again
            </button>
          </div>
        )}

        {state.kind === "loaded" && state.items.length === 0 && (
          <div className="rounded-md border border-dashed border-slate-200 py-8 text-center">
            <p className="text-sm font-medium text-slate-700">No content items yet</p>
            <p className="mt-1 text-xs text-slate-500">
              Create your first draft article, post, or snippet above.
            </p>
          </div>
        )}

        {state.kind === "loaded" && state.items.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-slate-200 text-xs font-medium uppercase tracking-wider text-slate-500">
                <tr>
                  <th scope="col" className="py-2.5 pr-4">
                    Title
                  </th>
                  <th scope="col" className="py-2.5 px-4">
                    Kind
                  </th>
                  <th scope="col" className="py-2.5 px-4">
                    Status
                  </th>
                  <th scope="col" className="py-2.5 px-4">
                    Slug
                  </th>
                  <th scope="col" className="py-2.5 pl-4 text-right">
                    Created
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {state.items.map((item: ContentItem) => (
                  <tr key={item.id} className="hover:bg-slate-50">
                    <td className="py-3 pr-4 font-medium text-slate-900">
                      <Link
                        href={`/content/${item.workspaceId}/${item.id}`}
                        className="font-medium text-slate-900 underline-offset-2 hover:text-teal-900 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700"
                      >
                        {item.title}
                      </Link>
                    </td>
                    <td className="py-3 px-4">
                      <span
                        className={`inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${kindBadgeClass(item.kind)}`}
                      >
                        {formatContentKind(item.kind)}
                      </span>
                    </td>
                    <td className="py-3 px-4">
                      <span className="inline-flex items-center rounded bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700 ring-1 ring-inset ring-slate-600/20">
                        {formatContentStatus(item.status)}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-xs text-slate-600 font-mono">
                      {item.slug ? (
                        <code className="rounded bg-slate-100 px-1 py-0.5">{item.slug}</code>
                      ) : (
                        <span className="text-slate-400">—</span>
                      )}
                    </td>
                    <td className="py-3 pl-4 text-right text-xs text-slate-500 whitespace-nowrap">
                      {new Date(item.createdAt).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}
