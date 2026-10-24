"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { NAV_ITEMS, isActive } from "@/lib/navigation";

const focusRing =
  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500";

// SVG icons — inline for zero bundle overhead
function OverviewIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
    </svg>
  );
}
function WorkspacesIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
      <polyline points="9 22 9 12 15 12 15 22" />
    </svg>
  );
}
function ContentIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="16" y1="13" x2="8" y2="13" />
      <line x1="16" y1="17" x2="8" y2="17" />
      <polyline points="10 9 9 9 8 9" />
    </svg>
  );
}
function CalendarIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
      <line x1="16" y1="2" x2="16" y2="6" />
      <line x1="8" y1="2" x2="8" y2="6" />
      <line x1="3" y1="10" x2="21" y2="10" />
    </svg>
  );
}
function IntegrationsIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="18" cy="5" r="3" />
      <circle cx="6" cy="12" r="3" />
      <circle cx="18" cy="19" r="3" />
      <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
      <line x1="15.41" y1="6.51" x2="8.59" y2="10.49" />
    </svg>
  );
}
function AnalyticsIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <line x1="18" y1="20" x2="18" y2="10" />
      <line x1="12" y1="20" x2="12" y2="4" />
      <line x1="6" y1="20" x2="6" y2="14" />
    </svg>
  );
}
function SettingsIcon({ active }: { active: boolean }) {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={active ? 2.2 : 1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  );
}

const NAV_ICONS: Record<string, (props: { active: boolean }) => React.ReactElement> = {
  overview: OverviewIcon,
  workspaces: WorkspacesIcon,
  content: ContentIcon,
  calendar: CalendarIcon,
  integrations: IntegrationsIcon,
  analytics: AnalyticsIcon,
  settings: SettingsIcon,
};

function HamburgerIcon() {
  return (
    <svg
      aria-hidden="true"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
    >
      <line x1="3" y1="6" x2="21" y2="6" />
      <line x1="3" y1="12" x2="21" y2="12" />
      <line x1="3" y1="18" x2="21" y2="18" />
    </svg>
  );
}

function CloseIcon() {
  return (
    <svg
      aria-hidden="true"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

export function AppShell({ children }: Readonly<{ children: React.ReactNode }>) {
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="min-h-dvh bg-slate-50 md:grid md:grid-cols-[16rem_minmax(0,1fr)]">
      {/* Skip link */}
      <a
        href="#main"
        className={`sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-white focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:shadow-md ${focusRing}`}
      >
        Skip to main content
      </a>

      {/* Mobile top bar */}
      <header className="sticky top-0 z-30 flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3 shadow-sm md:hidden">
        <div className="flex items-center gap-2.5">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-indigo-600 text-white">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
            </svg>
          </span>
          <span className="text-sm font-semibold text-slate-900">Marketing Ops</span>
        </div>
        <button
          type="button"
          onClick={() => setMenuOpen((open) => !open)}
          aria-expanded={menuOpen}
          aria-controls="primary-nav"
          aria-label={menuOpen ? "Close menu" : "Open menu"}
          className={`rounded-md p-1.5 text-slate-600 hover:bg-slate-100 hover:text-slate-900 ${focusRing}`}
        >
          {menuOpen ? <CloseIcon /> : <HamburgerIcon />}
        </button>
      </header>

      {/* Sidebar */}
      <aside
        className={`${
          menuOpen ? "block" : "hidden"
        } border-b border-slate-200 bg-white md:sticky md:top-0 md:block md:h-dvh md:overflow-y-auto md:border-r md:border-b-0`}
      >
        {/* Logo / wordmark */}
        <div className="hidden px-5 pt-6 pb-5 md:block">
          <div className="flex items-center gap-2.5">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-indigo-600 text-white shadow-sm">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
              </svg>
            </span>
            <div>
              <p className="text-sm font-bold leading-tight text-slate-900">Marketing Ops</p>
              <p className="text-[11px] leading-tight text-slate-400 font-medium tracking-wide uppercase">Operator</p>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav id="primary-nav" aria-label="Primary" className="px-3 pb-4 md:pt-1">
          <p className="mb-1 px-3 pt-3 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
            Navigation
          </p>
          <ul className="flex flex-col gap-0.5">
            {NAV_ITEMS.map((item) => {
              const active = isActive(pathname, item.href);
              const Icon = NAV_ICONS[item.key];
              return (
                <li key={item.key}>
                  <Link
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    onClick={() => setMenuOpen(false)}
                    className={`flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors ${focusRing} ${
                      active
                        ? "bg-indigo-50 font-semibold text-indigo-700"
                        : "font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900"
                    }`}
                  >
                    <span
                      className={`shrink-0 ${active ? "text-indigo-600" : "text-slate-400"}`}
                    >
                      {Icon ? <Icon active={active} /> : null}
                    </span>
                    {item.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>

        {/* Footer */}
        <div className="hidden border-t border-slate-100 px-5 py-4 md:block">
          <p className="text-[11px] text-slate-400">Internal tool · Not indexed</p>
        </div>
      </aside>

      {/* Main content */}
      <main
        id="main"
        tabIndex={-1}
        className="min-w-0 px-4 py-6 focus:outline-none md:px-10 md:py-10"
      >
        <div className="mx-auto max-w-4xl">{children}</div>
      </main>
    </div>
  );
}
