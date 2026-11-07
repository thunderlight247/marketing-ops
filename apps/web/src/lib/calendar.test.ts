import { describe, expect, it } from "vitest";
import {
  buildCalendarICS,
  calendarEventDay,
  escapeICS,
  isOverdueFollowUp,
  localDayKey,
  type CalendarEvent,
} from "./calendar";

describe("calendar event days", () => {
  it("places a scheduled instant on the viewer's local day", () => {
    const localEvening = new Date(2026, 10, 10, 22, 30);
    expect(calendarEventDay(localEvening.toISOString(), "content")).toBe("2026-11-10");
  });

  it("keeps date-only follow-ups on their stored date", () => {
    expect(calendarEventDay("2030-04-05T00:00:00Z", "follow_up")).toBe("2030-04-05");
  });

  it("formats local day keys with leading zeros", () => {
    expect(localDayKey(new Date(2026, 0, 7))).toBe("2026-01-07");
  });

  it("falls back to the date part for an unreadable timestamp", () => {
    expect(calendarEventDay("not-a-date", "content")).toBe("not-a-date");
  });
});

const event: CalendarEvent = {
  date: "2026-11-10",
  label: "Follow up: Example, Inc.",
  kind: "follow_up",
  workspaceId: "w1",
  workspaceName: "Redeem",
  href: "/workspaces/w1",
  color: "bg-amber-100 text-amber-800",
};

describe("overdue follow-ups", () => {
  it("marks only follow-ups before today", () => {
    expect(isOverdueFollowUp(event, "2026-11-11")).toBe(true);
    expect(isOverdueFollowUp(event, "2026-11-10")).toBe(false);
    expect(isOverdueFollowUp({ ...event, kind: "content" }, "2026-11-11")).toBe(false);
  });
});

describe("ICS export", () => {
  it("escapes calendar text", () => {
    expect(escapeICS("a,b;c\\nd")).toBe("a\\,b\\;c\\\\nd");
  });

  it("writes an all-day event with CRLF endings", () => {
    const ics = buildCalendarICS([event], new Date("2026-11-07T12:00:00Z"));
    expect(ics).toContain("BEGIN:VCALENDAR\r\n");
    expect(ics).toContain("DTSTART;VALUE=DATE:20261110");
    expect(ics).toContain("DTEND;VALUE=DATE:20261111");
    expect(ics).toContain("SUMMARY:Follow up: Example\\, Inc. (Redeem)");
    expect(ics.endsWith("END:VCALENDAR\r\n")).toBe(true);
  });
});
