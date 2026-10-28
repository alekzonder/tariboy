import { describe, expect, it } from "vitest";
import { describeSchedule, formatSchedule, parseSchedule, switchMode } from "./scheduleModel";

// Strings in the normalized form the daemon stores (internal/schedule/rrule.go).
const SAVED = {
  every: "DTSTART:20261018T000000\nRRULE:FREQ=MINUTELY;INTERVAL=15;BYSECOND=0",
  daily: "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0",
  weekly:
    "DTSTART:20261018T000000\nRRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=30;BYSECOND=0",
  monthly:
    "DTSTART:20261018T000000\nRRULE:FREQ=MONTHLY;BYMONTHDAY=-1;BYHOUR=23;BYMINUTE=45;BYSECOND=0",
  once: "DTSTART:20261020T093000\nRRULE:FREQ=DAILY;COUNT=1;BYSECOND=0",
} as const;

describe("parseSchedule", () => {
  it("reads an empty schedule as None", () => {
    expect(parseSchedule("  ").mode).toBe("none");
  });

  it.each(Object.entries(SAVED))(
    "reads a saved %s rule into its mode and back unchanged",
    (mode, text) => {
      const model = parseSchedule(text);
      expect(model.mode).toBe(mode);
      expect(formatSchedule(model)).toBe(text);
    },
  );

  it("fills the fields of each mode", () => {
    expect(parseSchedule(SAVED.every)).toMatchObject({ every: "15", unit: "MINUTELY" });
    expect(parseSchedule(SAVED.daily)).toMatchObject({ time: "09:30" });
    expect(parseSchedule(SAVED.weekly)).toMatchObject({ days: ["MO", "TU", "WE", "TH", "FR"] });
    expect(parseSchedule(SAVED.monthly)).toMatchObject({ monthDay: "-1", time: "23:45" });
    expect(parseSchedule(SAVED.once)).toMatchObject({ date: "2026-10-20T09:30" });
  });

  it.each([
    "DTSTART:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9,18;BYMINUTE=0;BYSECOND=0",
    "DTSTART:20261018T000000\nRRULE:FREQ=MONTHLY;BYDAY=TU;BYSETPOS=2;BYHOUR=9;BYMINUTE=0;BYSECOND=0",
    "DTSTART;TZID=Europe/Moscow:20261020T093000\nRRULE:FREQ=DAILY;COUNT=1;BYSECOND=0",
    "RRULE:FREQ=WEEKLY;BYDAY=TU,MO;BYHOUR=8;BYMINUTE=0;BYSECOND=0",
    "RRULE:FREQ=DAILY;UNTIL=20261231T000000Z;BYHOUR=9;BYMINUTE=0;BYSECOND=0",
    "not a rule",
  ])("keeps a rule no mode fits as Custom RRULE, unchanged: %s", (text) => {
    const model = parseSchedule(text);
    expect(model.mode).toBe("custom");
    expect(formatSchedule(model)).toBe(text);
  });
});

describe("formatSchedule", () => {
  it("builds each mode in the daemon's part order with BYSECOND=0", () => {
    const base = parseSchedule("");
    expect(formatSchedule({ ...base, mode: "every", every: "2", unit: "HOURLY" })).toBe(
      "RRULE:FREQ=HOURLY;INTERVAL=2;BYSECOND=0",
    );
    expect(formatSchedule({ ...base, mode: "weekly", days: ["MO"], time: "08:00" })).toBe(
      "RRULE:FREQ=WEEKLY;BYDAY=MO;BYHOUR=8;BYMINUTE=0;BYSECOND=0",
    );
    expect(formatSchedule({ ...base, mode: "monthly", monthDay: "15", time: "08:00" })).toBe(
      "RRULE:FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=8;BYMINUTE=0;BYSECOND=0",
    );
    expect(formatSchedule({ ...base, mode: "once", date: "2026-10-20T09:30" })).toBe(
      "DTSTART:20261020T093000\nRRULE:FREQ=DAILY;COUNT=1;BYSECOND=0",
    );
    expect(formatSchedule({ ...base, mode: "none" })).toBe("");
  });

  it("lists weekdays in calendar order whatever order they were picked in", () => {
    const base = parseSchedule("");
    expect(formatSchedule({ ...base, mode: "weekly", days: ["FR", "MO"], time: "08:00" })).toBe(
      "RRULE:FREQ=WEEKLY;BYDAY=MO,FR;BYHOUR=8;BYMINUTE=0;BYSECOND=0",
    );
  });
});

describe("switchMode", () => {
  const now = new Date(2026, 9, 28, 14, 5);

  it("drops the saved DTSTART so the daemon starts the new rule now", () => {
    const next = switchMode(parseSchedule(SAVED.daily), "weekly", now);
    expect(formatSchedule(next)).toBe("RRULE:FREQ=WEEKLY;BYDAY=MO;BYHOUR=9;BYMINUTE=30;BYSECOND=0");
  });

  it("opens Custom RRULE with the rule built so far", () => {
    const next = switchMode(parseSchedule(SAVED.daily), "custom", now);
    expect(formatSchedule(next)).toBe(SAVED.daily);
  });

  it("starts Once tomorrow at the chosen time", () => {
    const next = switchMode(parseSchedule(SAVED.daily), "once", now);
    expect(next.date).toBe("2026-10-29T09:30");
  });
});

describe("describeSchedule", () => {
  it.each([
    [SAVED.every, "Every 15 minutes"],
    ["RRULE:FREQ=HOURLY;INTERVAL=1;BYSECOND=0", "Every hour"],
    [SAVED.daily, "Every day at 09:30"],
    [SAVED.weekly, "Every weekday at 09:30"],
    ["RRULE:FREQ=WEEKLY;BYDAY=MO,WE;BYHOUR=8;BYMINUTE=0;BYSECOND=0", "Every Mon, Wed at 08:00"],
    [SAVED.monthly, "Every month on the last day at 23:45"],
    [
      "RRULE:FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=8;BYMINUTE=0;BYSECOND=0",
      "Every month on day 15 at 08:00",
    ],
    [SAVED.once, "Once on 2026-10-20 at 09:30"],
    ["", "No schedule"],
  ])("describes %s", (text, summary) => {
    expect(describeSchedule(parseSchedule(text))).toBe(summary);
  });
});
