import type { MetricsQuality, MetricsSlice } from "@/lib/api";
import { QUALITY_AXES, UNKNOWN_REASON, fmtAvg, fmtDuration, fmtMoney } from "@/lib/metricsFormat";
import type { Column } from "./sliceSort";
import { Known } from "./Unknown";

/** The lowest axis average: the weakest side of the work. */
function lowestQuality(quality: MetricsQuality): number | null {
  if (!quality) return null;
  const avgs = QUALITY_AXES.flatMap(({ key }) => (quality[key] ? [quality[key].avg] : []));
  return avgs.length ? Math.min(...avgs) : null;
}

function qualityTitle(quality: MetricsQuality): string {
  return QUALITY_AXES.map(({ key, label }) => {
    const axis = quality?.[key];
    return `${label} ${axis ? Math.round(axis.avg) : "—"}`;
  }).join(" · ");
}

type AvgField =
  | "cost_usd"
  | "iterations"
  | "rework_rounds"
  | "rejected_advances"
  | "customer_questions";

function avgColumn(id: string, label: string, field: AvgField): Column<MetricsSlice> {
  const format = field === "cost_usd" ? fmtMoney : fmtAvg;
  return {
    id,
    label,
    value: (s) => s.tasks.avg?.[field] ?? null,
    cell: (s) => (
      <Known value={s.tasks.avg?.[field]} format={format} reason={UNKNOWN_REASON.perTask} />
    ),
  };
}

export const TASKS_DONE: Column<MetricsSlice> = {
  id: "tasks_done",
  label: "Tasks done",
  value: (s) => s.tasks.tasks_done,
  cell: (s) => s.tasks.tasks_done,
};

export const MIXED_TASKS: Column<MetricsSlice> = {
  id: "mixed_tasks",
  label: "Mixed tasks",
  hint: "Done tasks that ran on this and other versions. They are not counted in this row.",
  value: (s) => s.tasks.mixed_tasks,
  cell: (s) => s.tasks.mixed_tasks,
};

/** The columns every slice table shares, from Lead time to Cost in period. */
export const SLICE_COLUMNS: Column<MetricsSlice>[] = [
  {
    id: "lead_time",
    label: "Lead time",
    hint: "Median",
    value: (s) => s.tasks.lead_time_median_s,
    cell: (s) => (
      <Known
        value={s.tasks.lead_time_median_s}
        format={fmtDuration}
        reason={UNKNOWN_REASON.leadTime}
      />
    ),
  },
  avgColumn("cost_task", "Cost / task", "cost_usd"),
  avgColumn("runs_task", "Runs / task", "iterations"),
  avgColumn("rework_task", "Rework / task", "rework_rounds"),
  avgColumn("rejected_task", "Rejected / task", "rejected_advances"),
  avgColumn("questions_task", "Questions / task", "customer_questions"),
  {
    id: "quality",
    label: "Quality",
    value: (s) => lowestQuality(s.tasks.quality),
    cell: (s) => {
      const lowest = lowestQuality(s.tasks.quality);
      return lowest === null ? (
        <Known value={null} format={String} reason={UNKNOWN_REASON.quality} />
      ) : (
        <span title={qualityTitle(s.tasks.quality)}>{Math.round(lowest)}</span>
      );
    },
  },
  {
    id: "runs_period",
    label: "Runs in period",
    value: (s) => s.usage.iterations,
    cell: (s) => s.usage.iterations,
  },
  {
    id: "cost_period",
    label: "Cost in period",
    value: (s) => s.usage.cost_usd,
    cell: (s) => fmtMoney(s.usage.cost_usd),
  },
];
