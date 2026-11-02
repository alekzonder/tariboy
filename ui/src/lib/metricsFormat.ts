// How the metrics screens show a value. `null` (not known) is never shown as
// 0 and 0 is never shown as unknown; the components show unknown as "—".
import { ApiError } from "@/lib/api/client";
import type { MetricsTokens } from "@/lib/api";

/** Below this many done tasks a version comparison warns that it can be random. */
export const SMALL_SAMPLE_TASKS = 5;

/** Why a value is not known, for the hint and the screen reader. */
export const UNKNOWN_REASON = {
  leadTime: "No completed task with a start time in this period.",
  taskLeadTime: "The task has no start time.",
  perTask: "No completed tasks, so there is no per-task value.",
  qualityAxis: "No scores on this axis.",
  quality: "No quality scores yet.",
  change: "Not enough data to compare.",
} as const;

/** `3d 4h`, `12h 05m`, `42m` or `<1m`, like the CLI. */
export function fmtDuration(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  if (days > 0) return `${days}d ${hours % 24}h`;
  if (hours > 0) return `${hours}h ${String(minutes % 60).padStart(2, "0")}m`;
  if (minutes > 0) return `${minutes}m`;
  return "<1m";
}

/** `$12.40`; a cost above 0 and below a cent is `<$0.01`, so it never reads as free. */
export function fmtMoney(usd: number): string {
  if (usd > 0 && usd < 0.01) return "<$0.01";
  return `$${usd.toFixed(2)}`;
}

/** A per-task average: one decimal. */
export function fmtAvg(value: number): string {
  return value.toFixed(1);
}

/** `1.2M`, `340k` or `900`. */
export function fmtTokens(tokens: number): string {
  if (tokens >= 1e6) return `${(tokens / 1e6).toFixed(1)}M`;
  if (tokens >= 1e3) return `${Math.round(tokens / 1e3)}k`;
  return String(Math.round(tokens));
}

export function tokenSum(tokens: MetricsTokens): number {
  return tokens.input + tokens.output + tokens.cache_write + tokens.cache_read;
}

/** The four kinds of tokens, for a hint. */
export function tokenTitle(tokens: MetricsTokens): string {
  const n = (value: number) => Math.round(value).toLocaleString("en-US");
  return `Input ${n(tokens.input)} · Output ${n(tokens.output)} · Cache write ${n(tokens.cache_write)} · Cache read ${n(tokens.cache_read)}`;
}

/** The per-task token averages as one MetricsTokens. */
export function avgTokens(avg: {
  input_tokens: number;
  output_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
}): MetricsTokens {
  return {
    input: avg.input_tokens,
    output: avg.output_tokens,
    cache_write: avg.cache_write_tokens,
    cache_read: avg.cache_read_tokens,
  };
}

/** A sort comparator: newer versions first by their numeric parts
 *  (`0.10.0` before `0.9.0`), `unknown` last. */
export function byVersionDesc(a: string, b: string): number {
  if (a === b) return 0;
  if (a === "unknown") return 1;
  if (b === "unknown") return -1;
  return b.localeCompare(a, "en", { numeric: true });
}

export function versionLabel(version: string): string {
  return version === "unknown" ? "unknown version" : version;
}

export const VERSION_HINT = {
  unknown: "The run's image version is not recorded, for example after data retention.",
  noWorkflow: "Runs and tasks without a workflow.",
  unknownWorkflow: "The workflow of this digest is not on the host any more.",
} as const;

export function workflowLabel(name: string): string {
  if (name === "none") return "No workflow";
  if (name === "unknown") return "Unknown workflow";
  return name;
}

export type MetricsErrorKind = "bad_time" | "invalid_param" | "old_daemon" | "timeout" | "other";

/** How a failed metrics request shows: in the period bar, at a filter chip,
 *  as an older host, as a timeout, or as a plain error. */
export function metricsErrorKind(error: unknown): MetricsErrorKind {
  if (!(error instanceof ApiError)) return "other";
  if (error.code === "bad_time" || error.code === "invalid_param") return error.code;
  if (error.code === "not_found" && error.message.startsWith("unknown route")) return "old_daemon";
  if (error.code === "network_error" && error.message === "request timed out") return "timeout";
  return "other";
}
