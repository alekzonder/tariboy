// The schedule builder edits the same RRULE text the daemon stores. This
// module maps that text to a builder mode and its fields and back, in the
// normalized form internal/schedule/rrule.go writes (rrule-go's part order,
// BYSECOND=0), so an untouched saved rule rebuilds to the identical string.

export type ScheduleMode = "none" | "every" | "daily" | "weekly" | "monthly" | "once" | "custom";

export const MODES: readonly { value: ScheduleMode; label: string }[] = [
  { value: "none", label: "None" },
  { value: "every", label: "Every N minutes / hours" },
  { value: "daily", label: "Daily" },
  { value: "weekly", label: "Weekly" },
  { value: "monthly", label: "Monthly" },
  { value: "once", label: "Once" },
  { value: "custom", label: "Custom RRULE" },
];

export const WEEKDAYS = [
  ["MO", "Mon"],
  ["TU", "Tue"],
  ["WE", "Wed"],
  ["TH", "Thu"],
  ["FR", "Fri"],
  ["SA", "Sat"],
  ["SU", "Sun"],
] as const;

export interface ScheduleModel {
  mode: ScheduleMode;
  // The saved DTSTART line, kept while the mode is unchanged.
  dtstart: string;
  every: string;
  unit: "MINUTELY" | "HOURLY";
  // HH:MM in the daemon host's time zone.
  time: string;
  days: string[];
  // "1" to "31", or "-1" for the last day.
  monthDay: string;
  // YYYY-MM-DDTHH:MM, the value of a datetime-local input.
  date: string;
  custom: string;
}

const EMPTY: ScheduleModel = {
  mode: "none",
  dtstart: "",
  every: "1",
  unit: "HOURLY",
  time: "09:00",
  days: ["MO"],
  monthDay: "1",
  date: "",
  custom: "",
};

const pad = (n: number) => String(n).padStart(2, "0");

function hourMinute(time: string): string {
  const [h, m] = time.split(":");
  return `BYHOUR=${Number(h)};BYMINUTE=${Number(m)}`;
}

function onceStart(date: string): string {
  const [day, time] = date.split("T");
  return `DTSTART:${day.replaceAll("-", "")}T${time.replace(":", "")}00`;
}

function ruleOf(model: ScheduleModel): string {
  switch (model.mode) {
    case "every":
      return `FREQ=${model.unit};INTERVAL=${model.every}`;
    case "daily":
      return `FREQ=DAILY;${hourMinute(model.time)}`;
    case "weekly": {
      const days = WEEKDAYS.map(([code]) => code).filter((code) => model.days.includes(code));
      return `FREQ=WEEKLY;BYDAY=${days.join(",")};${hourMinute(model.time)}`;
    }
    case "monthly":
      return `FREQ=MONTHLY;BYMONTHDAY=${model.monthDay};${hourMinute(model.time)}`;
    default:
      return "FREQ=DAILY;COUNT=1";
  }
}

// formatSchedule returns the text to save; "" clears the schedule.
export function formatSchedule(model: ScheduleModel): string {
  if (model.mode === "none") return "";
  if (model.mode === "custom") return model.custom;
  const dtstart = model.mode === "once" ? onceStart(model.date) : model.dtstart;
  return `${dtstart ? `${dtstart}\n` : ""}RRULE:${ruleOf(model)};BYSECOND=0`;
}

function fields(rule: string): Record<string, string> {
  return Object.fromEntries(rule.split(";").map((part) => part.split("=", 2)));
}

function candidate(dtstart: string, rule: string): ScheduleModel | undefined {
  const f = fields(rule.replace(/^RRULE:/, ""));
  const time = `${pad(Number(f.BYHOUR))}:${pad(Number(f.BYMINUTE))}`;
  const base = { ...EMPTY, dtstart, time };
  const once = /^DTSTART:(\d{4})(\d\d)(\d\d)T(\d\d)(\d\d)00$/.exec(dtstart);
  switch (f.FREQ) {
    case "MINUTELY":
    case "HOURLY":
      return { ...base, mode: "every", unit: f.FREQ, every: f.INTERVAL ?? "" };
    case "DAILY":
      if (f.COUNT !== "1") return { ...base, mode: "daily" };
      if (!once) return undefined;
      return {
        ...base,
        mode: "once",
        dtstart: "",
        date: `${once[1]}-${once[2]}-${once[3]}T${once[4]}:${once[5]}`,
      };
    case "WEEKLY":
      return { ...base, mode: "weekly", days: (f.BYDAY ?? "").split(",") };
    case "MONTHLY":
      return { ...base, mode: "monthly", monthDay: f.BYMONTHDAY ?? "" };
  }
  return undefined;
}

// parseSchedule opens a mode only when that mode rebuilds the text exactly;
// any other rule stays verbatim in Custom RRULE, so nothing is lost.
export function parseSchedule(text: string): ScheduleModel {
  const trimmed = text.trim();
  if (!trimmed) return EMPTY;
  const parts = trimmed.split(/\s+/);
  const [dtstart, rule] = parts.length === 2 ? parts : ["", parts[0]];
  const found =
    parts.length <= 2 && (parts.length === 1 || dtstart.startsWith("DTSTART"))
      ? candidate(dtstart, rule)
      : undefined;
  if (found && formatSchedule(found) === parts.join("\n")) return found;
  return { ...EMPTY, mode: "custom", custom: text };
}

function tomorrowAt(time: string, now: Date): string {
  const day = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  return `${day.getFullYear()}-${pad(day.getMonth() + 1)}-${pad(day.getDate())}T${time}`;
}

// switchMode keeps the fields already entered but drops the saved DTSTART, so
// the daemon anchors the new rule at the current minute.
export function switchMode(model: ScheduleModel, mode: ScheduleMode, now: Date): ScheduleModel {
  const next = { ...model, mode, dtstart: "" };
  if (mode === "custom") next.custom = formatSchedule(model);
  if (mode === "once" && !next.date) next.date = tomorrowAt(model.time, now);
  return next;
}

function weekdays(days: readonly string[]): string {
  const picked = WEEKDAYS.filter(([code]) => days.includes(code));
  if (picked.length === 7) return "day";
  if (picked.map(([code]) => code).join() === "MO,TU,WE,TH,FR") return "weekday";
  return picked.map(([, name]) => name).join(", ");
}

// describeSchedule is the one-line summary under the builder.
export function describeSchedule(model: ScheduleModel): string {
  const at = `at ${model.time}`;
  switch (model.mode) {
    case "none":
      return "No schedule";
    case "every": {
      const unit = model.unit === "MINUTELY" ? "minute" : "hour";
      return model.every === "1" ? `Every ${unit}` : `Every ${model.every} ${unit}s`;
    }
    case "daily":
      return `Every day ${at}`;
    case "weekly":
      return `Every ${weekdays(model.days)} ${at}`;
    case "monthly": {
      const day = model.monthDay === "-1" ? "the last day" : `day ${model.monthDay}`;
      return `Every month on ${day} ${at}`;
    }
    case "once":
      return `Once on ${model.date.replace("T", " at ")}`;
    default:
      return "Custom rule";
  }
}
