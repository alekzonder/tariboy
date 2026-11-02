import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { activityWindow, type ActivityWindow } from "./activityPeriod";
import { byVersionDesc } from "./metricsFormat";

// The URL state of /metrics and /metrics/compare: the period, the view, the
// filters and the compared versions. Every change replaces the history entry,
// as in Activity, so a link reproduces the screen after a reload.

export type MetricsPeriod = "week" | "month" | "quarter" | "custom";

export const METRICS_PERIODS: Array<{ period: MetricsPeriod; label: string }> = [
  { period: "week", label: "Week" },
  { period: "month", label: "Month" },
  { period: "quarter", label: "Quarter" },
  { period: "custom", label: "Custom" },
];

export type MetricsWindow = Omit<ActivityWindow, "period"> & { period: MetricsPeriod };

const QUARTER_MS = 90 * 24 * 3600e3;

/** The Metrics period of a query; a period the bar does not offer is Month. */
export function metricsWindow(search: URLSearchParams, now: Date): MetricsWindow {
  const raw = search.get("period") ?? "";
  if (raw === "quarter") {
    return {
      period: "quarter",
      since: new Date(now.getTime() - QUARTER_MS).toISOString(),
      until: now.toISOString(),
      customSince: search.get("since") ?? "",
      customUntil: search.get("until") ?? "",
      error: "",
    };
  }
  const known = raw === "week" || raw === "month" || raw === "custom";
  const params = new URLSearchParams(search);
  if (!known) params.set("period", "month");
  return activityWindow(params, now, "month") as MetricsWindow;
}

/** Whether "Show 90 days" would show more. */
export function shorterThanQuarter(window: MetricsWindow): boolean {
  if (!window.since) return true;
  return new Date(window.until).getTime() - new Date(window.since).getTime() < QUARTER_MS - 60e3;
}

/** The period of the URL. The window ends at the time of the load, the last
 *  period change or `refresh()`; it does not move by itself, because the
 *  request is heavy and every new window is a new request. */
export function useMetricsWindow() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [now, setNow] = useState(() => new Date());
  const search = searchParams.toString();
  const window = useMemo(() => metricsWindow(new URLSearchParams(search), now), [search, now]);
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
    window,
    setPeriod: (period: MetricsPeriod) => update({ period }),
    setCustom: (bound: "since" | "until", value: string) => update({ [bound]: value }),
    /** Moves the window to end now. */
    refresh: () => setNow(new Date()),
  };
}

export type MetricsView = "agents" | "images" | "workflows";

export function metricsView(search: URLSearchParams): MetricsView {
  const view = search.get("view");
  return view === "images" || view === "workflows" ? view : "agents";
}

export interface MetricsFilters {
  agent: string;
  image: string;
  workflow: string;
}

export const FILTER_NAMES = ["agent", "image", "workflow"] as const;

export function metricsFilters(search: URLSearchParams): MetricsFilters {
  return {
    agent: search.get("agent") ?? "",
    image: search.get("image") ?? "",
    workflow: search.get("workflow") ?? "",
  };
}

/** `NAME[:VERSION]` as the daemon reads it: the version follows the last
 *  colon after the last slash, so a registry port stays in the name. */
export function splitRef(value: string): { name: string; version: string } {
  const colon = value.lastIndexOf(":");
  if (colon <= value.lastIndexOf("/")) return { name: value, version: "" };
  return { name: value.slice(0, colon), version: value.slice(colon + 1) };
}

/** `?host=` and the period of the current screen, for a link to another
 *  metrics screen. */
export function periodParams(search: URLSearchParams, window: MetricsWindow): URLSearchParams {
  const params = new URLSearchParams();
  const host = search.get("host");
  if (host !== null) params.set("host", host);
  params.set("period", window.period);
  if (window.period === "custom") {
    if (window.customSince) params.set("since", window.customSince);
    if (window.customUntil) params.set("until", window.customUntil);
  }
  return params;
}

/** The query of an agent's Activity → Metrics view on the same window. The
 *  agent tab has no Quarter, so a quarter goes as a custom range. */
export function agentMetricsQuery(window: MetricsWindow): string {
  const params = new URLSearchParams({ view: "metrics" });
  if (window.period === "week" || window.period === "month") {
    params.set("period", window.period);
  } else if (window.period === "custom") {
    params.set("period", "custom");
    if (window.customSince) params.set("since", window.customSince);
    if (window.customUntil) params.set("until", window.customUntil);
  } else {
    params.set("period", "custom");
    if (window.since) params.set("since", localInput(window.since));
    if (window.until) params.set("until", localInput(window.until));
  }
  return params.toString();
}

/** An RFC 3339 time as a `datetime-local` value in local time, to the minute. */
function localInput(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export type CompareKind = "image" | "workflow";

export interface CompareQuery {
  kind: CompareKind;
  name: string;
  a: string;
  b: string;
}

/** The compared name and versions, or null without a kind or a name. */
export function parseCompare(search: URLSearchParams): CompareQuery | null {
  const kind = search.get("kind");
  const name = search.get("name") ?? "";
  if ((kind !== "image" && kind !== "workflow") || !name) return null;
  return { kind, name, a: search.get("a") ?? "", b: search.get("b") ?? "" };
}

/** The query of /metrics/compare, after the given host and period. */
export function compareQuery(
  base: URLSearchParams,
  kind: CompareKind,
  name: string,
  a: string,
  b: string,
): string {
  const params = new URLSearchParams(base);
  params.set("kind", kind);
  params.set("name", name);
  if (a) params.set("a", a);
  if (b) params.set("b", b);
  return params.toString();
}

const known = (versions: string[]) => versions.filter((v) => v !== "unknown").sort(byVersionDesc);

/** Without versions in the URL: B is the newest version, A the one before.
 *  `unknown` is never picked, but it stays in the lists. */
export function defaultPair(versions: string[]): { a: string; b: string } {
  const sorted = known(versions);
  return { b: sorted[0] ?? "", a: sorted[1] ?? "" };
}

/** The nearest version older than `version`, or "" when there is none. */
export function olderVersion(versions: string[], version: string): string {
  const sorted = known(versions);
  const index = sorted.indexOf(version);
  return index < 0 ? "" : (sorted[index + 1] ?? "");
}
