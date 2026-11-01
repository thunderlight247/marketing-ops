"use client";

import { useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type Workspace } from "@/lib/workspaces";
import { fetchWorkspaceCampaignsLeads } from "@/lib/campaigns";
import { fetchWorkspaceContent } from "@/lib/content";

// ─── Calendar helpers ─────────────────────────────────────────────────────────

function daysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate();
}

function firstDayOfWeek(year: number, month: number): number {
  // 0=Sun … 6=Sat
  return new Date(year, month, 1).getDay();
}

const MONTH_NAMES = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];
const DAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// ─── Event types ──────────────────────────────────────────────────────────────

interface CalEvent {
  date: string; // YYYY-MM-DD
  label: string;
  kind: "follow_up" | "content";
  workspaceName: string;
  color: string;
}

// ─── Data loading ─────────────────────────────────────────────────────────────

async function loadAllEvents(apiBaseUrl: string, workspaces: Workspace[]): Promise<CalEvent[]> {
  const events: CalEvent[] = [];

  await Promise.all(
    workspaces.map(async (ws) => {
      const [campaignsResult, contentResult] = await Promise.allSettled([
        fetchWorkspaceCampaignsLeads(apiBaseUrl, ws.id),
        fetchWorkspaceContent(apiBaseUrl, ws.id),
      ]);

      // Follow-up leads
      if (campaignsResult.status === "fulfilled" && campaignsResult.value.kind === "loaded") {
        for (const lead of campaignsResult.value.leads) {
          if (lead.followUpAt) {
            events.push({
              date: lead.followUpAt.slice(0, 10),
              label: `Follow up: ${lead.contactName || lead.contactEmail || "Lead"}`,
              kind: "follow_up",
              workspaceName: ws.name,
              color: "bg-amber-100 text-amber-800",
            });
          }
        }
      }

      // Planned content
      if (contentResult.status === "fulfilled" && contentResult.value.kind === "loaded") {
        for (const item of contentResult.value.items) {
          if (item.plannedPublishAt && item.status !== "published") {
            events.push({
              date: item.plannedPublishAt.slice(0, 10),
              label: item.title || "Untitled",
              kind: "content",
              workspaceName: ws.name,
              color: "bg-indigo-100 text-indigo-800",
            });
          }
        }
      }
    }),
  );

  return events;
}

// ─── Calendar grid ────────────────────────────────────────────────────────────

function CalendarGrid({
  year,
  month,
  events,
}: {
  year: number;
  month: number;
  events: CalEvent[];
}) {
  const days = daysInMonth(year, month);
  const firstDay = firstDayOfWeek(year, month);
  const today = new Date().toISOString().slice(0, 10);

  // Build a map from YYYY-MM-DD → events
  const eventMap: Record<string, CalEvent[]> = {};
  for (const ev of events) {
    eventMap[ev.date] = eventMap[ev.date] ?? [];
    eventMap[ev.date].push(ev);
  }

  // Cells: empty slots before first day + day cells
  const totalCells = Math.ceil((firstDay + days) / 7) * 7;
  const cells: (number | null)[] = [];
  for (let i = 0; i < firstDay; i++) cells.push(null);
  for (let d = 1; d <= days; d++) cells.push(d);
  while (cells.length < totalCells) cells.push(null);

  return (
    <div>
      {/* Day headers */}
      <div className="grid grid-cols-7 border-b border-slate-200 bg-slate-50">
        {DAY_LABELS.map((d) => (
          <div key={d} className="px-1 py-2 text-center text-[10px] font-semibold uppercase tracking-wide text-slate-400">
            {d}
          </div>
        ))}
      </div>

      {/* Day cells */}
      <div className="grid grid-cols-7 divide-x divide-y divide-slate-100">
        {cells.map((day, i) => {
          const dateStr =
            day !== null
              ? `${year}-${String(month + 1).padStart(2, "0")}-${String(day).padStart(2, "0")}`
              : null;
          const dayEvents = dateStr ? (eventMap[dateStr] ?? []) : [];
          const isToday = dateStr === today;
          return (
            <div
              key={i}
              className={`min-h-[72px] p-1 sm:p-1.5 ${day === null ? "bg-slate-50/50" : "bg-white"}`}
            >
              {day !== null && (
                <>
                  <span
                    className={`inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold ${
                      isToday
                        ? "bg-indigo-600 text-white"
                        : "text-slate-700"
                    }`}
                  >
                    {day}
                  </span>
                  <div className="mt-0.5 grid gap-0.5">
                    {dayEvents.slice(0, 3).map((ev, j) => (
                      <span
                        key={j}
                        title={`${ev.workspaceName}: ${ev.label}`}
                        className={`block truncate rounded px-1 text-[10px] font-medium ${ev.color}`}
                      >
                        {ev.label}
                      </span>
                    ))}
                    {dayEvents.length > 3 && (
                      <span className="text-[10px] text-slate-400">+{dayEvents.length - 3} more</span>
                    )}
                  </div>
                </>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─── Main calendar page ───────────────────────────────────────────────────────

export default function CalendarPage() {
  const apiBaseUrl = getApiBaseUrl();
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth());
  const [events, setEvents] = useState<CalEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      setLoading(true);
      setError(null);
      try {
        const wsState = await fetchWorkspaces(apiBaseUrl);
        if (wsState.kind !== "loaded") {
          setError("Could not load workspaces.");
          return;
        }
        const loaded = await loadAllEvents(apiBaseUrl, wsState.workspaces);
        setEvents(loaded);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load calendar.");
      } finally {
        setLoading(false);
      }
    })();
  }, [apiBaseUrl]);

  function prevMonth() {
    if (month === 0) { setYear((y) => y - 1); setMonth(11); }
    else setMonth((m) => m - 1);
  }
  function nextMonth() {
    if (month === 11) { setYear((y) => y + 1); setMonth(0); }
    else setMonth((m) => m + 1);
  }
  function goToday() { setYear(now.getFullYear()); setMonth(now.getMonth()); }

  // Events for the current month
  const prefix = `${year}-${String(month + 1).padStart(2, "0")}-`;
  const monthEvents = events.filter((e) => e.date.startsWith(prefix));

  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Calendar</h1>
          <p className="mt-0.5 text-sm text-slate-500">
            Scheduled content and lead follow-ups across all workspaces.
          </p>
        </div>
        <button
          type="button"
          onClick={goToday}
          className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 transition-colors"
        >
          Today
        </button>
      </div>

      {/* Month nav */}
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={prevMonth}
          aria-label="Previous month"
          className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-slate-600 hover:bg-slate-50 transition-colors"
        >
          ‹
        </button>
        <h2 className="min-w-[9rem] text-center text-sm font-semibold text-slate-900">
          {MONTH_NAMES[month]} {year}
        </h2>
        <button
          type="button"
          onClick={nextMonth}
          aria-label="Next month"
          className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-slate-600 hover:bg-slate-50 transition-colors"
        >
          ›
        </button>
      </div>

      {loading && (
        <div className="flex items-center gap-2 text-sm text-slate-500">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-slate-400" />
          Loading calendar…
        </div>
      )}
      {error && (
        <p role="alert" className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800">
          {error}
        </p>
      )}

      {!loading && !error && (
        <>
          <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <CalendarGrid year={year} month={month} events={events} />
          </div>

          {/* Legend */}
          <div className="flex flex-wrap gap-4 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded bg-amber-100 ring-1 ring-amber-200" />
              <span className="text-slate-600">Follow-up reminder</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded bg-indigo-100 ring-1 ring-indigo-200" />
              <span className="text-slate-600">Scheduled content</span>
            </span>
          </div>

          {/* Upcoming list */}
          {monthEvents.length > 0 && (
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <div className="border-b border-slate-100 bg-slate-50 px-5 py-3">
                <h2 className="text-xs font-semibold uppercase tracking-widest text-slate-500">
                  This month — {monthEvents.length} {monthEvents.length === 1 ? "event" : "events"}
                </h2>
              </div>
              <ul className="divide-y divide-slate-100">
                {monthEvents
                  .slice()
                  .sort((a, b) => a.date.localeCompare(b.date))
                  .map((ev, i) => (
                    <li key={i} className="flex flex-wrap items-start gap-3 px-5 py-3">
                      <span className="mt-0.5 min-w-[4.5rem] text-xs font-mono text-slate-400">
                        {ev.date.slice(5).replace("-", "/")}
                      </span>
                      <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${ev.color}`}>
                        {ev.kind === "follow_up" ? "Follow-up" : "Content"}
                      </span>
                      <span className="flex-1 min-w-0 text-sm text-slate-800 break-words">{ev.label}</span>
                      <span className="text-xs text-slate-400">{ev.workspaceName}</span>
                    </li>
                  ))}
              </ul>
            </div>
          )}

          {monthEvents.length === 0 && (
            <p className="text-sm text-slate-500">
              No follow-ups or scheduled content for {MONTH_NAMES[month]} {year}.
            </p>
          )}
        </>
      )}
    </div>
  );
}
