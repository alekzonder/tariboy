import type { Task, TaskCategory, WorkflowAttention } from "./api";
import { flexibleStatusLabel, type StatusTone } from "./statusTone";

/**
 * A workflow task reports a workflow status ID in `status`, a flexible task its
 * own status. The daemon marks one by its digest; the name is a fallback.
 */
export const isWorkflowTask = (task: Task): boolean =>
  !!(task.workflow_digest || task.workflow_name);

/** The status as a person reads it, for either kind of task. */
export function taskStatusLabel(task: Task): string {
  if (!isWorkflowTask(task)) return flexibleStatusLabel(task.status);
  const text = task.status.replace(/[_-]+/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}
export const taskCategoryLabel = (category: TaskCategory): string => flexibleStatusLabel(category);
export const isActiveCategory = (category: TaskCategory): boolean =>
  category === "open" || category === "in_progress";

/** How many of `tasks` each queue holds, for the queue menu. */
export function countTasksByQueue(tasks: readonly Task[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const task of tasks) counts.set(task.queue, (counts.get(task.queue) ?? 0) + 1);
  return counts;
}

/** The tone a row's problem mark takes, or null when the summary earns no mark:
 *  an idle holder is the panel's business, not the list's. */
export function workflowAttentionTone(attention: WorkflowAttention | undefined): StatusTone | null {
  switch (attention?.kind) {
    case "failed":
    case "script_failed":
      return "danger";
    case "rejected":
      return "attention";
    default:
      return null;
  }
}

/** The one badge a workflow task's status cell has room for: a pause wins over
 *  a problem, a problem over what the task waits on. */
export function workflowStatusBadge(task: Task): "paused" | "attention" | "waiting" | null {
  if (task.waiting_on === "pause") return "paused";
  if (workflowAttentionTone(task.workflow_attention)) return "attention";
  return task.waiting_on ? "waiting" : null;
}
