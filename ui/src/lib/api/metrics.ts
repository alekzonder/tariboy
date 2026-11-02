import type { MetricsReport, TaskMetrics } from "../api-types.generated";
import type { Transport } from "./client";

export type {
  MetricsImageVersionUsage,
  MetricsQuality,
  MetricsReport,
  MetricsSlice,
  MetricsTaskStats,
  MetricsTokens,
  TaskMetrics,
} from "../api-types.generated";

/** The period and filters of `GET /api/metrics`. `image` and `workflow` are
 *  `NAME` or `NAME:VERSION`; `workflow` may be `none`. */
export interface MetricsFilter {
  since: string;
  until: string;
  agent?: string;
  image?: string;
  workflow?: string;
}

export function metricsApi(t: Transport) {
  return {
    getMetrics: (filter: MetricsFilter, signal?: AbortSignal) => {
      const q = new URLSearchParams({ since: filter.since, until: filter.until });
      for (const name of ["agent", "image", "workflow"] as const) {
        if (filter[name]) q.set(name, filter[name]);
      }
      return t.get<MetricsReport>(`/api/metrics?${q}`, signal);
    },
    getTaskMetrics: (key: string, signal?: AbortSignal) =>
      t.get<TaskMetrics>(`/api/metrics/tasks/${encodeURIComponent(key)}`, signal),
  };
}
