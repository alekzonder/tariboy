import type { TaskActivityTotals } from "@/types";

/** A complete rolling-day total of the task activity report; a story
 *  overrides only the counts that matter. The totals carry no dates. */
export const taskActivityTotals = (over: Partial<TaskActivityTotals> = {}): TaskActivityTotals => ({
  tasks: 35,
  completed: 32,
  transitions: 4,
  comments: 2,
  iterations: 101,
  requests: 410,
  tokens: 1_250_000,
  cost_usd: 145.23,
  ...over,
});
