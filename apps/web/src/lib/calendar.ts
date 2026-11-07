export type CalendarEventKind = "follow_up" | "content";

export interface CalendarEvent {
  /** YYYY-MM-DD on the viewer's calendar. */
  date: string;
  label: string;
  kind: CalendarEventKind;
  workspaceId: string;
  workspaceName: string;
  href: string;
  color: string;
}

/** YYYY-MM-DD for the viewer's local calendar day (never UTC). */
export function localDayKey(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/**
 * Follow-ups are date-only values (recorded as midnight UTC), so their calendar
 * day is the stored date. Scheduled content is a real instant, so it belongs to
 * the viewer's local day — a 10pm publish in UTC-5 must not drift to tomorrow.
 */
export function calendarEventDay(iso: string, kind: CalendarEventKind): string {
  if (kind === "follow_up") return iso.slice(0, 10);
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? iso.slice(0, 10) : localDayKey(at);
}

/** A follow-up whose day has passed and is still open. */
export function isOverdueFollowUp(event: CalendarEvent, today: string): boolean {
  return event.kind === "follow_up" && event.date < today;
}

/** RFC 5545 text escaping for calendar summaries and descriptions. */
export function escapeICS(value: string): string {
  return value
    .replace(/\\/g, "\\\\")
    .replace(/;/g, "\\;")
    .replace(/,/g, "\\,")
    .replace(/\r?\n/g, "\\n");
}

function icsDate(day: string): string {
  return day.replace(/-/g, "");
}

function icsNextDay(day: string): string {
  const at = new Date(`${day}T00:00:00Z`);
  at.setUTCDate(at.getUTCDate() + 1);
  return at.toISOString().slice(0, 10).replace(/-/g, "");
}

function icsStamp(at: Date): string {
  return at
    .toISOString()
    .replace(/[-:]/g, "")
    .replace(/\.\d{3}Z$/, "Z");
}

/**
 * Builds an all-day ICS calendar for the visible events. Nothing is uploaded:
 * the browser creates the file locally, so workspaces and lead names never
 * leave the machine to export a schedule.
 */
export function buildCalendarICS(events: CalendarEvent[], now: Date = new Date()): string {
  const lines = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//Marketing Operations//Calendar//EN",
    "CALSCALE:GREGORIAN",
    "METHOD:PUBLISH",
    "X-WR-CALNAME:Marketing Operations",
  ];
  const stamp = icsStamp(now);
  events.forEach((event, index) => {
    lines.push(
      "BEGIN:VEVENT",
      `UID:${event.kind}-${event.workspaceId}-${event.date}-${index}@marketing-ops`,
      `DTSTAMP:${stamp}`,
      `DTSTART;VALUE=DATE:${icsDate(event.date)}`,
      `DTEND;VALUE=DATE:${icsNextDay(event.date)}`,
      `SUMMARY:${escapeICS(`${event.label} (${event.workspaceName})`)}`,
      `CATEGORIES:${event.kind === "follow_up" ? "Follow-up" : "Content"}`,
      `DESCRIPTION:${escapeICS(event.href)}`,
      "END:VEVENT",
    );
  });
  lines.push("END:VCALENDAR", "");
  // RFC 5545 uses CRLF line endings.
  return lines.join("\r\n");
}
