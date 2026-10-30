import { closestBucket } from "@/lib/activityPeriod";

// Window presets: label → (span in ms, series bucket). The bucket widens with
// the window so the chart never renders thousands of bars (spec §4).
const PRESETS = {
  "1h": { ms: 3600e3, bucket: "5m" },
  "5h": { ms: 5 * 3600e3, bucket: "15m" },
  "24h": { ms: 24 * 3600e3, bucket: "1h" },
  "7d": { ms: 7 * 24 * 3600e3, bucket: "1d" },
} as const;
export type Preset = keyof typeof PRESETS | "custom";
export const PRESET_ORDER: Preset[] = ["1h", "5h", "24h", "7d", "custom"];

export type GroupBy = "iteration" | "task" | "epic" | "model";
export const GROUPINGS: { key: GroupBy; label: string }[] = [
  { key: "iteration", label: "Runs" },
  { key: "task", label: "Tasks" },
  { key: "epic", label: "Epics" },
  { key: "model", label: "Models" },
];

type UsageWindow = {
  period?: { since: string; until: string };
  preset: Preset;
  customSince: string;
  customUntil: string;
  groupBy: GroupBy;
  now: number;
};

function periodParams({
  period,
  groupBy,
  now,
}: UsageWindow & { period: UsageWindow["period"] & {} }) {
  const span = period.since
    ? Date.parse(period.until || new Date(now).toISOString()) - Date.parse(period.since)
    : 0;
  return {
    since: period.since,
    until: period.until,
    group_by: groupBy,
    bucket: span ? closestBucket(span) : "1h",
  };
}

function customParams({ customSince, customUntil, groupBy, now }: UsageWindow) {
  const since = customSince ? new Date(customSince).toISOString() : "";
  const until = customUntil ? new Date(customUntil).toISOString() : "";
  // Derive the bucket from the span so an open-ended custom start (from set,
  // to empty) still gets a coarse bucket via closestBucket instead of the
  // 1h default rendering hundreds of hourly bars (spec §4). Treat a missing
  // 'to' as now.
  let bucket = "1h";
  if (since) {
    const end = until ? new Date(until).getTime() : now;
    bucket = closestBucket(end - new Date(since).getTime());
  }
  return { since, until, group_by: groupBy, bucket };
}

// The usage query params. Callers recompute them on every poll tick so preset
// windows slide with the wall clock (since=now-Δ), and custom ranges convert
// their local datetime-local values to RFC3339 (UTC) for the backend. A
// controlled period (the Activity tab's shared window) replaces the presets.
export function usageParams(w: UsageWindow): Record<string, string> {
  if (w.period) return periodParams({ ...w, period: w.period });
  if (w.preset === "custom") return customParams(w);
  const { ms, bucket } = PRESETS[w.preset];
  return { since: new Date(w.now - ms).toISOString(), group_by: w.groupBy, bucket };
}
