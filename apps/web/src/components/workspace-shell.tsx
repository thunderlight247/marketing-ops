"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type Workspace } from "@/lib/workspaces";

const TABS = [
  { key: "overview", label: "Overview", segment: "" },
  { key: "content", label: "Content", segment: "/content" },
  { key: "calendar", label: "Calendar", segment: "/calendar" },
  { key: "lead-engine", label: "Lead engine", segment: "/lead-engine" },
  { key: "email", label: "Email", segment: "/email" },
  { key: "integrations", label: "Integrations", segment: "/integrations" },
  { key: "analytics", label: "Analytics", segment: "/analytics" },
  { key: "settings", label: "Settings", segment: "/settings" },
] as const;

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "W";
  return (parts[0][0] + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toUpperCase();
}

export function WorkspaceShell({
  workspaceId,
  workspaceName,
  children,
}: Readonly<{
  workspaceId: string;
  workspaceName: string;
  children: React.ReactNode;
}>) {
  const pathname = usePathname();
  const router = useRouter();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [current, setCurrent] = useState<Workspace | null>(null);

  const base = `/workspaces/${encodeURIComponent(workspaceId)}`;

  useEffect(() => {
    let active = true;
    void fetchWorkspaces(getApiBaseUrl()).then((state) => {
      if (!active) return;
      if (state.kind === "loaded") {
        setWorkspaces(state.workspaces);
        setCurrent(state.workspaces.find((workspace) => workspace.id === workspaceId) ?? null);
      }
    });
    return () => {
      active = false;
    };
  }, [workspaceId]);

  const name = current?.name ?? workspaceName;
  const status = current?.status ?? "active";

  return (
    <div className="grid gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Breadcrumb" className="text-xs text-slate-500">
          <Link href="/workspaces" className="font-medium hover:text-indigo-700">
            Workspaces
          </Link>
          <span className="mx-1.5 text-slate-300" aria-hidden="true">
            /
          </span>
          <span className="font-semibold text-slate-700">{name}</span>
        </nav>
        {workspaces.length > 1 ? (
          <label className="flex items-center gap-2 text-xs font-medium text-slate-500">
            Switch workspace
            <select
              value={workspaceId}
              onChange={(event) =>
                router.push(`/workspaces/${encodeURIComponent(event.target.value)}`)
              }
              className="max-w-[14rem] rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-semibold text-slate-700 shadow-sm"
            >
              {workspaces.map((workspace) => (
                <option key={workspace.id} value={workspace.id}>
                  {workspace.name}
                </option>
              ))}
              {!workspaces.some((workspace) => workspace.id === workspaceId) ? (
                <option value={workspaceId}>{name}</option>
              ) : null}
            </select>
          </label>
        ) : null}
      </div>

      <header className="relative overflow-hidden rounded-2xl border border-indigo-100 bg-gradient-to-br from-indigo-600 via-indigo-600 to-violet-700 px-5 py-5 text-white shadow-sm sm:px-7 sm:py-6">
        <div
          className="pointer-events-none absolute -right-10 -top-16 h-44 w-44 rounded-full bg-white/10"
          aria-hidden="true"
        />
        <div className="relative flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-center gap-4">
            <span
              className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-white/15 text-lg font-bold tracking-wide ring-1 ring-inset ring-white/25"
              aria-hidden="true"
            >
              {initials(name)}
            </span>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="truncate text-2xl font-bold tracking-tight text-balance">{name}</h1>
                <span className="rounded-full bg-white/15 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide ring-1 ring-inset ring-white/20">
                  {status}
                </span>
              </div>
              <p className="mt-1 truncate text-sm text-indigo-100">
                {current?.websiteUrl ? (
                  <a
                    href={current.websiteUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="underline decoration-indigo-300 underline-offset-2 hover:text-white"
                  >
                    {current.websiteUrl}
                  </a>
                ) : (
                  <span className="font-mono text-xs text-indigo-200">
                    /{current?.slug ?? workspaceId}
                  </span>
                )}
              </p>
            </div>
          </div>
          <p className="max-w-xs text-xs leading-relaxed text-indigo-100">
            Everything in these pages is isolated to this workspace. Switch tabs without losing your
            place.
          </p>
        </div>
      </header>

      <nav
        aria-label="Workspace sections"
        className="-mx-1 overflow-x-auto border-b border-slate-200 pb-px"
      >
        <ul className="flex min-w-max gap-1 px-1">
          {TABS.map((tab) => {
            const href = `${base}${tab.segment}`;
            const active = tab.segment === "" ? pathname === base : pathname.startsWith(href);
            return (
              <li key={tab.key}>
                <Link
                  href={href}
                  aria-current={active ? "page" : undefined}
                  className={`inline-flex border-b-2 px-3 py-2.5 text-sm font-semibold transition-colors ${
                    active
                      ? "border-indigo-600 text-indigo-700"
                      : "border-transparent text-slate-500 hover:border-slate-300 hover:text-slate-900"
                  }`}
                >
                  {tab.label}
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>

      <div>{children}</div>
    </div>
  );
}
