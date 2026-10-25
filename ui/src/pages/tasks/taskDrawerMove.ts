import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  createApi,
  type ApiClient,
  type Task,
  type TaskPriority,
  type TaskStatus,
} from "@/lib/api";
import { errorText } from "@/lib/errors";
import { idempotencyKey } from "@/lib/ids";
import { targetFor } from "@/lib/terminalsHost";
import type { AssigneeChoice } from "./TaskDrawer";
import { refreshTasks } from "./taskQueries";

export type TaskEdit = {
  revision: number;
  title: string;
  description: string;
  pull_request: string;
  status?: TaskStatus;
  assignee?: string;
  manual_block_reason?: string;
  priority: TaskPriority;
};

/**
 * An agent on another server can only work a task that lives there. Other
 * edits are saved here first; then the tree is exported and imported with
 * its root unassigned, so no same-named agent over there takes it before
 * the real assignment, which is an ordinary update and notifies the agent.
 * Once the import succeeds the task lives there: a later failure is
 * reported, and the sheet follows the task all the same.
 */
export async function moveTaskToHost({
  api,
  client,
  task,
  input,
  remote,
  onMoved,
}: {
  api: ApiClient;
  client: QueryClient;
  task: Task;
  input: TaskEdit;
  remote: AssigneeChoice;
  onMoved?: (hostId: string) => void;
}): Promise<Task> {
  const key = task.key;
  const edited = (
    ["title", "description", "pull_request", "status", "priority", "manual_block_reason"] as const
  ).some((field) => input[field] !== undefined && input[field] !== (task[field] ?? ""));
  if (edited) await api.tasks.updateTask(key, { ...input, assignee: task.assignee });
  const hostId = remote.hostId!;
  // The source and the destination keep one client each for the whole move.
  const destination = createApi(targetFor(hostId));
  const label = remote.hostLabel || hostId || "local";
  const bundle = await api.tasks.exportTask(key);
  const tasks = bundle.tasks.map((item) =>
    item.key === bundle.root_key ? { ...item, assignee: "" } : item,
  );
  const imported = await destination.tasks.importTask({ ...bundle, tasks });
  let result = imported;
  try {
    await api.tasks.addTaskComment(
      key,
      `Moved to ${label} as ${key}.`,
      idempotencyKey("task-action"),
    );
    const left = await api.tasks.getTask(key);
    await api.tasks.updateTask(key, { status: "cancelled", revision: left.task.revision });
    const arrived = await destination.tasks.getTask(key);
    result = await destination.tasks.updateTask(key, {
      assignee: remote.assignee,
      revision: arrived.task.revision,
    });
    toast.success(`${key} moved to ${label}`);
  } catch (error) {
    toast.error(`${key} moved to ${label}, but the move did not finish: ${errorText(error)}`);
  }
  await refreshTasks(client, destination);
  onMoved?.(hostId);
  return result;
}
