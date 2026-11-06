import type { MetricsReport, MetricsSlice, TaskMetrics } from "@/types";

import { NOW, hoursAgo } from "./time";

/** A metrics slice where every field has its own value, so a test that finds a
 *  shown value knows which field it came from. */
export const metricsSlice = (over: Partial<MetricsSlice> = {}): MetricsSlice => ({
  name: "worker",
  version: "0.3.0",
  tasks: {
    tasks_done: 7,
    mixed_tasks: 3,
    lead_time_median_s: 45_000,
    lead_time_avg_s: 51_600,
    avg: {
      cost_usd: 1.37,
      iterations: 2.31,
      rework_rounds: 0.43,
      rejected_advances: 0.14,
      customer_questions: 0.57,
      input_tokens: 400_000,
      output_tokens: 50_000,
      cache_write_tokens: 150_000,
      cache_read_tokens: 600_000,
    },
    sum: {
      cost_usd: 9.59,
      iterations: 16,
      rework_rounds: 3,
      rejected_advances: 1,
      customer_questions: 4,
      tokens: { input: 2_800_000, output: 350_000, cache_write: 1_050_000, cache_read: 4_200_000 },
    },
    quality: {
      image_process: { avg: 82, min: 61, count: 12 },
      role: { avg: 74, min: 55, count: 11 },
      task_fit: { avg: 90, min: 70, count: 10 },
    },
  },
  usage: {
    iterations: 23,
    cost_usd: 12.4,
    tokens: { input: 3_000_000, output: 400_000, cache_write: 200_000, cache_read: 5_000_000 },
  },
  usage_without_task: {
    iterations: 4,
    cost_usd: 1.86,
    tokens: { input: 450_000, output: 60_000, cache_write: 30_000, cache_read: 750_000 },
  },
  ...over,
});

/** A server metrics report with one slice in each list. */
export const metricsReport = (over: Partial<MetricsReport> = {}): MetricsReport => ({
  since: hoursAgo(30 * 24),
  until: NOW,
  total: metricsSlice({ name: undefined, version: undefined }),
  agents: [metricsSlice({ name: "alice", version: undefined })],
  images: [metricsSlice({ name: "developer", version: "0.23.2" })],
  workflows: [metricsSlice({ name: "development", version: "0.10.1" })],
  ...over,
});

/** The metrics of one task. */
export const taskMetrics = (over: Partial<TaskMetrics> = {}): TaskMetrics => ({
  key: "DEV-1",
  title: "Add metrics",
  status: "done",
  started_at: hoursAgo(48),
  completed_at: hoursAgo(24),
  lead_time_s: 86_400,
  status_time_s: { implement: 3600, review: 1800 },
  usage: metricsSlice().usage,
  rework_rounds: 2,
  rework_by_outcome: { changes_requested: 1, rework: 1, conflict: 0 },
  rejected_advances: 1,
  customer_questions: 3,
  quality: null,
  agents: [],
  images: [],
  workflows: [],
  ...over,
});

/** Edge sets of metrics slices: `empty`, `one`, `many` (50) and `longText`. */
export const metricsSlices = {
  empty: [] as MetricsSlice[],
  one: [metricsSlice()],
  many: Array.from({ length: 50 }, (_, i) => metricsSlice({ name: `worker-${i + 1}` })),
  longText: [
    metricsSlice({
      name: "an-agent-with-a-very-long-name-that-has-to-truncate-in-the-metrics-table",
    }),
  ],
};
