import type { Task, TaskCategory } from "./api";
import { flexibleStatusLabel } from "./statusTone";

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
