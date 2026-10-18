import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";

// One time window shared by every Activity view. The choice lives in the URL
// (?period=hour|day|week|custom, plus ?since/?until as datetime-local values
// for custom) so a view can be shared and survives a reload.

export type ActivityPeriod = "hour" | "day" | "week" | "custom";

export const ACTIVITY_PERIODS: Array<{ period: ActivityPeriod; label: string }> = [
  { period: "hour", label: "Hour" },
  { period: "day", label: "Day" },
  { period: "week", label: "Week" },
  { period: "custom", label: "Custom" },
];

const SPAN_MS: Record<Exclude<ActivityPeriod, "custom">, number> = {
  hour: 3600e3,
  day: 24 * 3600e3,
  week: 7 * 24 * 3600e3,
};

export interface ActivityWindow {
  period: ActivityPeriod;
  /** RFC3339 bounds: since inclusive, until exclusive. */
  since: string;
  until: string;
  /** The raw custom datetime-local inputs, kept for the form. */
  customSince: string;
  customUntil: string;
  /** Set when the custom range cannot be used; since/until are then empty. */
  error: string;
}

export function activityWindow(search: URLSearchParams, now: Date): ActivityWindow {
  const raw = search.get("period");
  const period: ActivityPeriod =
    raw === "hour" || raw === "week" || raw === "custom" ? raw : "day";
  const customSince = search.get("since") ?? "";
  const customUntil = search.get("until") ?? "";
  const base = { period, customSince, customUntil, error: "" };
  if (period !== "custom") {
    return { ...base, since: new Date(now.getTime() - SPAN_MS[period]).toISOString(), until: now.toISOString() };
  }
  const start = customSince ? new Date(customSince) : null;
  const end = customUntil ? new Date(customUntil) : now;
  const fail = (error: string) => ({ ...base, since: "", until: "", error });
  if (!start || Number.isNaN(start.getTime())) return fail("Enter a valid start time.");
  if (Number.isNaN(end.getTime())) return fail("Enter a valid end time.");
  if (start >= end) return fail("The start must be before the end.");
  return { ...base, since: start.toISOString(), until: end.toISOString() };
}

// Pick the series bucket for a span so a cost chart never draws thousands of bars.
export function closestBucket(spanMs: number): string {
  if (spanMs <= 2 * 3600e3) return "5m";
  if (spanMs <= 8 * 3600e3) return "15m";
  if (spanMs <= 36 * 3600e3) return "1h";
  return "1d";
}

// Rolling windows move with the clock once a minute, not on every render, so
// views keyed on the window do not reload on each poll.
const TICK_MS = 60e3;

export function useActivityWindow() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), TICK_MS);
    return () => window.clearInterval(timer);
  }, []);
  const search = searchParams.toString();
  const current = useMemo(() => activityWindow(new URLSearchParams(search), now), [search, now]);
  // A period change replaces the history entry, like the other Activity filters.
  const update = (changes: Record<string, string>) => {
    const next = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(changes)) {
      if (value) next.set(key, value);
      else next.delete(key);
    }
    setSearchParams(next, { replace: true });
    setNow(new Date());
  };
  return {
    window: current,
    setPeriod: (period: ActivityPeriod) => update({ period }),
    setCustom: (bound: "since" | "until", value: string) => update({ [bound]: value }),
  };
}

/** The period's own query, for links that open another Activity view on it. */
export function periodQuery(window: ActivityWindow): string {
  const q = new URLSearchParams({ period: window.period });
  if (window.period === "custom") {
    if (window.customSince) q.set("since", window.customSince);
    if (window.customUntil) q.set("until", window.customUntil);
  }
  return q.toString();
}
