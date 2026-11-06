// The rows of a version comparison: each metric of version A and B, the change
// B − A, and whether that change is better or worse.
import type { MetricsSlice } from "@/types";
import {
  QUALITY_AXES,
  SMALL_SAMPLE_TASKS,
  UNKNOWN_REASON,
  avgTokens,
  fmtAvg,
  fmtDuration,
  fmtMoney,
  fmtTokens,
  tokenSum,
} from "@/lib/metricsFormat";

type Format = "count" | "duration" | "money" | "avg" | "tokens" | "score";

export interface CompareRow {
  id: string;
  label: string;
  format: Format;
  /** Which way is better; null for a metric without a direction. */
  better: "lower" | "higher" | null;
  a: number | null;
  b: number | null;
  /** The score counts of a quality row. */
  countA?: number;
  countB?: number;
  /** A row that holds a text instead of numbers. */
  note?: string;
  /** Why a null value of A or B is not known. */
  reasonA?: string;
  reasonB?: string;
}

const MISSING = "No data for this version in this period.";

/** Why a version with data has no value in this row. */
function reasonFor(id: string): string | undefined {
  if (id.startsWith("lead_")) return UNKNOWN_REASON.leadTime;
  if (id.startsWith("quality_")) return UNKNOWN_REASON.qualityAxis;
  if (id.endsWith("_task")) return UNKNOWN_REASON.perTask;
  return undefined;
}

type Pick = (s: MetricsSlice) => number | null | undefined;

const METRICS: Array<[string, string, Format, CompareRow["better"], Pick]> = [
  ["tasks_done", "Tasks done", "count", null, (s) => s.tasks.tasks_done],
  ["lead_median", "Lead time, median", "duration", "lower", (s) => s.tasks.lead_time_median_s],
  ["lead_avg", "Lead time, average", "duration", "lower", (s) => s.tasks.lead_time_avg_s],
  ["cost_task", "Cost per task", "money", "lower", (s) => s.tasks.avg?.cost_usd],
  ["runs_task", "Runs per task", "avg", "lower", (s) => s.tasks.avg?.iterations],
  [
    "tokens_task",
    "Tokens per task",
    "tokens",
    "lower",
    (s) => (s.tasks.avg ? tokenSum(avgTokens(s.tasks.avg)) : null),
  ],
  ["rework_task", "Rework rounds per task", "avg", "lower", (s) => s.tasks.avg?.rework_rounds],
  [
    "rejected_task",
    "Rejected advances per task",
    "avg",
    "lower",
    (s) => s.tasks.avg?.rejected_advances,
  ],
  ["questions_task", "Questions per task", "avg", "lower", (s) => s.tasks.avg?.customer_questions],
];

const PERIOD: Array<[string, string, Format, Pick]> = [
  ["runs_period", "Runs in period", "count", (s) => s.usage.iterations],
  ["cost_period", "Cost in period", "money", (s) => s.usage.cost_usd],
  ["cost_no_task", "Cost without task", "money", (s) => s.usage_without_task.cost_usd],
];

/** The rows for slices A and B; a version without data is undefined. */
export function compareRows(a?: MetricsSlice, b?: MetricsSlice): CompareRow[] {
  const read = (s: MetricsSlice | undefined, pick: Pick) => (s ? (pick(s) ?? null) : null);
  const rows: CompareRow[] = METRICS.map(([id, label, format, better, pick]) => ({
    id,
    label,
    format,
    better,
    a: read(a, pick),
    b: read(b, pick),
  }));
  rows.push(...qualityRows(a, b));
  for (const [id, label, format, pick] of PERIOD) {
    rows.push({ id, label, format, better: null, a: read(a, pick), b: read(b, pick) });
  }
  for (const row of rows) {
    if (row.note) continue;
    if (row.a === null) row.reasonA = a ? reasonFor(row.id) : MISSING;
    if (row.b === null) row.reasonB = b ? reasonFor(row.id) : MISSING;
  }
  return rows;
}

/** One row per axis, or one Quality row when neither version has scores. */
function qualityRows(a?: MetricsSlice, b?: MetricsSlice): CompareRow[] {
  if (!a?.tasks.quality && !b?.tasks.quality) {
    return [
      {
        id: "quality",
        label: "Quality",
        format: "score",
        better: null,
        a: null,
        b: null,
        note: "No quality scores yet",
      },
    ];
  }
  return QUALITY_AXES.map(({ key, label }) => {
    const axisA = a?.tasks.quality?.[key];
    const axisB = b?.tasks.quality?.[key];
    return {
      id: `quality_${key}`,
      label: `Quality: ${label}`,
      format: "score",
      better: "higher",
      a: axisA?.avg ?? null,
      b: axisB?.avg ?? null,
      countA: axisA?.count,
      countB: axisB?.count,
    };
  });
}

function fmt(format: Format, value: number): string {
  switch (format) {
    case "duration":
      return fmtDuration(value);
    case "money":
      return fmtMoney(value);
    case "avg":
      return fmtAvg(value);
    case "tokens":
      return fmtTokens(value);
    default:
      return String(Math.round(value));
  }
}

/** The shown value of one side, or "" when it is not known. */
export function fmtCell(row: CompareRow, side: "a" | "b"): string {
  if (row.note) return row.note;
  const value = row[side];
  if (value === null) return "";
  const text = fmt(row.format, value);
  const count = side === "a" ? row.countA : row.countB;
  return count === undefined ? text : `${text} · ${count} scores`;
}

export type Verdict = "better" | "worse" | "no change";

/** B − A with its percent of A, its sign as a direction, and the verdict;
 *  null when a side is unknown. */
export function changeOf(
  row: CompareRow,
): { text: string; direction: "up" | "down" | "none"; verdict: Verdict | null } | null {
  if (row.note || row.a === null || row.b === null) return null;
  const diff = row.b - row.a;
  const sign = diff > 0 ? "+" : diff < 0 ? "−" : "";
  const zero = row.format === "money" ? fmt("money", 0) : "0";
  const amount = diff === 0 ? zero : `${sign}${fmt(row.format, Math.abs(diff))}`;
  const percent = row.a === 0 ? "" : ` (${sign}${Math.round(Math.abs((diff / row.a) * 100))}%)`;
  let verdict: Verdict | null = null;
  if (row.better) {
    if (diff === 0) verdict = "no change";
    else verdict = diff < 0 === (row.better === "lower") ? "better" : "worse";
  }
  const direction = diff > 0 ? "up" : diff < 0 ? "down" : "none";
  return { text: amount + percent, direction, verdict };
}

/** The warning that the samples are too small to trust, or null. */
export function smallSample(a?: MetricsSlice, b?: MetricsSlice): string | null {
  if (!a || !b) return null;
  const [na, nb] = [a.tasks.tasks_done, b.tasks.tasks_done];
  if (na >= SMALL_SAMPLE_TASKS && nb >= SMALL_SAMPLE_TASKS) return null;
  return `Few completed tasks (A: ${na}, B: ${nb}). The change can be random.`;
}
