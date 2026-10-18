"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { NAV_ITEMS, isActive } from "@/lib/navigation";

const focusRing =
  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-700";

export function AppShell({ children }: Readonly<{ children: React.ReactNode }>) {
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="min-h-dvh md:grid md:grid-cols-[15rem_minmax(0,1fr)]">
      <a
        href="#main"
        className={`sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-white focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:shadow ${focusRing}`}
      >
        Skip to main content
      </a>

      <header className="sticky top-0 z-30 flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3 md:hidden">
        <span className="text-base font-semibold">Marketing Ops</span>
        <button
          type="button"
          onClick={() => setMenuOpen((open) => !open)}
          aria-expanded={menuOpen}
          aria-controls="primary-nav"
          className={`rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-800 hover:bg-slate-100 ${focusRing}`}
        >
          {menuOpen ? "Close menu" : "Menu"}
        </button>
      </header>

      <aside
        className={`${menuOpen ? "block" : "hidden"} border-b border-slate-200 bg-white md:sticky md:top-0 md:block md:h-dvh md:border-r md:border-b-0`}
      >
        <div className="hidden px-5 pt-6 pb-4 md:block">
          <p className="text-base font-semibold">Marketing Ops</p>
          <p className="text-sm text-slate-600">Operator dashboard</p>
        </div>
        <nav id="primary-nav" aria-label="Primary" className="px-3 py-2 md:py-0">
          <ul className="flex flex-col gap-1">
            {NAV_ITEMS.map((item) => {
              const active = isActive(pathname, item.href);
              return (
                <li key={item.key}>
                  <Link
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    onClick={() => setMenuOpen(false)}
                    className={`block rounded-md px-3 py-2 text-sm ${focusRing} ${
                      active
                        ? "bg-teal-50 font-semibold text-teal-900"
                        : "text-slate-700 hover:bg-slate-100 hover:text-slate-900"
                    }`}
                  >
                    {item.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>
      </aside>

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
