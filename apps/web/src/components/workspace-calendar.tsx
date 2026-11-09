"use client";

import { useCallback, useEffect, useState } from "react";
import { getApiBaseUrl } from "@/lib/config";
import { fetchWorkspaces, type Workspace } from "@/lib/workspaces";
import { fetchWorkspaceCampaignsLeads } from "@/lib/campaigns";
import { fetchWorkspaceContent } from "@/lib/content";
import {
  buildCalendarICS,
  calendarEventDay,
  isOverdueFollowUp,
  localDayKey,
  type CalendarEvent,
} from "@/lib/calendar";

// ─── Platform icons ───────────────────────────────────────────────────────────

const PLATFORM_EMOJI: Record<string, string> = {
  instagram: "📸",
  facebook: "📘",
  linkedin: "💼",
  twitter: "🐦",
  tiktok: "🎵",
  youtube: "▶️",
  pinterest: "📌",
  threads: "🧵",
};

function PlatformChips({ platforms }: { platforms: string[] }) {
  if (!platforms.length) return null;
  return (
    <div className="flex flex-wrap gap-0.5 mt-0.5">
      {platforms.slice(0, 4).map((p) => (
        <span
          key={p}
          title={p.charAt(0).toUpperCase() + p.slice(1)}
          className="text-[9px] leading-none"
        >
          {PLATFORM_EMOJI[p] ?? "🔗"}
        </span>
      ))}
    </div>
  );
}

// ─── Calendar helpers ─────────────────────────────────────────────────────────

function daysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate();
}

function firstDayOfWeek(year: number, month: number): number {
  // 0=Sun … 6=Sat
  return new Date(year, month, 1).getDay();
}

const MONTH_NAMES = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];
const DAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// ─── Data loading ─────────────────────────────────────────────────────────────

interface LoadedCalendar {
  events: CalendarEvent[];
  failed: string[];
}

async function loadAllEvents(apiBaseUrl: string, workspaces: Workspace[]): Promise<LoadedCalendar> {
  const events: CalendarEvent[] = [];
  const failed: string[] = [];

  await Promise.all(
    workspaces.map(async (ws) => {
      const [campaignsResult, contentResult] = await Promise.allSettled([
        fetchWorkspaceCampaignsLeads(apiBaseUrl, ws.id),
        fetchWorkspaceContent(apiBaseUrl, ws.id),
      ]);
      const campaigns =
        campaignsResult.status === "fulfilled" && campaignsResult.value.kind === "loaded"
          ? campaignsResult.value
          : null;
      const content =
        contentResult.status === "fulfilled" && contentResult.value.kind === "loaded"
          ? contentResult.value
          : null;
      if (!campaigns || !content) failed.push(ws.name);

      // Follow-up leads
      if (campaigns) {
        for (const lead of campaigns.leads) {
          if (lead.followUpAt && lead.status !== "won" && lead.status !== "lost") {
            events.push({
              date: calendarEventDay(lead.followUpAt, "follow_up"),
              label: `Follow up: ${lead.contactName || lead.contactEmail || "Lead"}`,
              kind: "follow_up",
              workspaceId: ws.id,
              workspaceName: ws.name,
              href: `/workspaces/${encodeURIComponent(ws.id)}`,
              color: "bg-amber-100 text-amber-800",
            });
          }
        }
      }

      // Planned content
      if (content) {
        for (const item of content.items) {
          if (item.plannedPublishAt && item.status !== "published") {
            events.push({
              date: calendarEventDay(item.plannedPublishAt, "content"),
              label: item.title || "Untitled",
              kind: "content",
              workspaceId: ws.id,
              workspaceName: ws.name,
              href: `/content/${encodeURIComponent(ws.id)}/${encodeURIComponent(item.id)}`,
              color:
                item.kind === "social_post"
                  ? "bg-purple-100 text-purple-800"
                  : "bg-indigo-100 text-indigo-800",
              platforms: item.targetPlatforms ?? [],
            });
          }
        }
      }
    }),
  );

  return { events, failed };
}

// ─── Calendar grid ────────────────────────────────────────────────────────────

function CalendarGrid({
  year,
  month,
  events,
}: {
  year: number;
  month: number;
  events: CalendarEvent[];
}) {
  const days = daysInMonth(year, month);
  const firstDay = firstDayOfWeek(year, month);
  const today = localDayKey(new Date());

  // Build a map from YYYY-MM-DD → events
  const eventMap: Record<string, CalendarEvent[]> = {};
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
          <div
            key={d}
            className="px-1 py-2 text-center text-[10px] font-semibold uppercase tracking-wide text-slate-400"
          >
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
                      isToday ? "bg-indigo-600 text-white" : "text-slate-700"
                    }`}
                  >
                    {day}
                  </span>
                  <div className="mt-0.5 grid gap-0.5">
                    {dayEvents.slice(0, 3).map((ev, j) => (
                      <span
                        key={j}
                        title={`${ev.workspaceName}: ${ev.label}`}
                        className={`block truncate rounded px-1 text-[10px] font-medium ${
                          isOverdueFollowUp(ev, today) ? "bg-red-100 text-red-800" : ev.color
                        }`}
                      >
                        {ev.kind === "content" && ev.platforms && ev.platforms.length > 0 && (
                          <PlatformChips platforms={ev.platforms} />
                        )}
                        {ev.label}
                      </span>
                    ))}
                    {dayEvents.length > 3 && (
                      <span className="text-[10px] text-slate-400">
                        +{dayEvents.length - 3} more
                      </span>
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

export function WorkspaceCalendar({ workspaceId }: Readonly<{ workspaceId: string }>) {
  const apiBaseUrl = getApiBaseUrl();
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth());
  const [events, setEvents] = useState<CalendarEvent[]>([]);

  const [failedWorkspaces, setFailedWorkspaces] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loadedAt, setLoadedAt] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const wsState = await fetchWorkspaces(apiBaseUrl);
      if (wsState.kind !== "loaded") {
        setError("Could not load workspaces.");
        return;
      }
      const scoped = wsState.workspaces.filter((workspace) => workspace.id === workspaceId);
      if (scoped.length === 0) {
        setError("That workspace is not active or no longer exists.");
        return;
      }
      const loaded = await loadAllEvents(apiBaseUrl, scoped);
      setEvents(loaded.events);
      setFailedWorkspaces(loaded.failed);
      setLoadedAt(new Date().toLocaleTimeString([], { hour: "numeric", minute: "2-digit" }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load calendar.");
    } finally {
      setLoading(false);
    }
  }, [apiBaseUrl, workspaceId]);

  useEffect(() => {
    void load();
  }, [load, reloadKey]);

  function prevMonth() {
    if (month === 0) {
      setYear((y) => y - 1);
      setMonth(11);
    } else setMonth((m) => m - 1);
  }
  function nextMonth() {
    if (month === 11) {
      setYear((y) => y + 1);
      setMonth(0);
    } else setMonth((m) => m + 1);
  }
  function goToday() {
    setYear(now.getFullYear());
    setMonth(now.getMonth());
  }

  const today = localDayKey(now);
  const visibleEvents = events.filter((event) => event.workspaceId === workspaceId);
  const overdueEvents = visibleEvents
    .filter((event) => isOverdueFollowUp(event, today))
    .sort((a, b) => a.date.localeCompare(b.date));

  // Events for the current month
  const prefix = `${year}-${String(month + 1).padStart(2, "0")}-`;
  const monthEvents = visibleEvents.filter((e) => e.date.startsWith(prefix));

  function downloadICS() {
    const blob = new Blob([buildCalendarICS(visibleEvents)], {
      type: "text/calendar;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `marketing-calendar-${today}.ics`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900">Calendar</h1>
          <p className="mt-0.5 text-sm text-slate-500">
            Scheduled content and lead follow-ups for this workspace.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {loadedAt && <span className="text-xs text-slate-400">Updated {loadedAt}</span>}
          <button
            type="button"
            onClick={downloadICS}
            disabled={visibleEvents.length === 0}
            className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-50 transition-colors"
          >
            Export .ics
          </button>
          <button
            type="button"
            onClick={goToday}
            className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 transition-colors"
          >
            Today
          </button>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            disabled={loading}
            className="rounded-lg border border-slate-200 px-3 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50 disabled:opacity-50 transition-colors"
          >
            ↻ Refresh
          </button>
        </div>
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
        <p
          role="alert"
          className="rounded-xl bg-red-50 border border-red-100 px-4 py-3 text-sm text-red-800"
        >
          {error}
        </p>
      )}

      {failedWorkspaces.length > 0 && (
        <p
          role="alert"
          className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900"
        >
          Some workspaces could not be loaded: {failedWorkspaces.join(", ")}. Their follow-ups and
          scheduled content are missing from this view.
        </p>
      )}

      {!loading && !error && (
        <>
          <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <CalendarGrid year={year} month={month} events={visibleEvents} />
          </div>

          {/* Legend */}
          <div className="flex flex-wrap gap-4 text-xs">
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded bg-amber-100 ring-1 ring-amber-200" />
              <span className="text-slate-600">Follow-up reminder</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded bg-purple-100 ring-1 ring-purple-200" />
              <span className="text-slate-600">Social post</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded bg-indigo-100 ring-1 ring-indigo-200" />
              <span className="text-slate-600">Blog / snippet</span>
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded bg-red-100 ring-1 ring-red-200" />
              <span className="text-slate-600">Overdue</span>
            </span>
          </div>

          {/* Past-due follow-ups need attention regardless of the month shown. */}
          {overdueEvents.length > 0 && (
            <div className="overflow-hidden rounded-xl border border-red-200 bg-white shadow-sm">
              <div className="border-b border-red-100 bg-red-50 px-5 py-3">
                <h2 className="text-xs font-semibold uppercase tracking-widest text-red-700">
                  Overdue follow-ups — {overdueEvents.length}
                </h2>
              </div>
              <ul className="divide-y divide-red-50">
                {overdueEvents.map((ev, i) => (
                  <li key={i} className="flex flex-wrap items-start gap-3 px-5 py-3">
                    <span className="mt-0.5 min-w-[4.5rem] text-xs font-mono text-red-500">
                      {ev.date.slice(5).replace("-", "/")}
                    </span>
                    <a
                      href={ev.href}
                      className="flex-1 min-w-0 text-sm text-slate-800 break-words hover:text-indigo-700 hover:underline"
                    >
                      {ev.label}
                    </a>
                    <span className="text-xs text-slate-400">{ev.workspaceName}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}

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
                      <span
                        className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
                          isOverdueFollowUp(ev, today) ? "bg-red-100 text-red-800" : ev.color
                        }`}
                      >
                        {ev.kind === "follow_up"
                          ? isOverdueFollowUp(ev, today)
                            ? "Overdue"
                            : "Follow-up"
                          : ev.color.includes("purple")
                          ? "Social post"
                          : "Blog / snippet"}
                      </span>
                      {ev.kind === "content" && ev.platforms && ev.platforms.length > 0 && (
                        <PlatformChips platforms={ev.platforms} />
                      )}
                      <a
                        href={ev.href}
                        className="flex-1 min-w-0 text-sm text-slate-800 break-words hover:text-indigo-700 hover:underline"
                      >
                        {ev.label}
                      </a>
                      <span className="text-xs text-slate-400">{ev.workspaceName}</span>
                    </li>
                  ))}
              </ul>
            </div>
          )}

          {monthEvents.length === 0 && (
            <p className="text-sm text-slate-500">
              No follow-ups or scheduled content for {MONTH_NAMES[month]} {year}
              {" in this workspace."} Add a follow-up date to a
              lead, or schedule a draft in Content, and it will appear here.
            </p>
          )}
        </>
      )}
    </div>
  );
}
