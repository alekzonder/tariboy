import { useApi } from "@/components/apiContext";
import type { Task } from "@/types";
import { useKeyedRequest } from "./useKeyedRequest";

/**
 * Loads `GET /api/tasks/{key}/budget` for the task, and again after each
 * change of the task, so a saved limit shows its new state. The budget of
 * another task or host never shows for this one.
 */
export function useTaskBudget(task: Pick<Task, "key" | "revision">) {
  const api = useApi();
  const key = task.key ? JSON.stringify([api.target?.id ?? "", task.key, task.revision]) : "";
  const { data, dataKey } = useKeyedRequest(key, (signal) =>
    api.tasks.getTaskBudget(task.key, signal),
  );
  return dataKey === key || data?.key === task.key ? data : null;
}
