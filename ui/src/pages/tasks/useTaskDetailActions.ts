import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import {
  createApi,
  transferTask,
  type TaskPriority,
  type TaskRelationType,
  type TaskStatus,
} from "@/lib/api";
import { errorText } from "@/lib/errors";
import { resolveDaemon } from "@/lib/daemons";
import { idempotencyKey } from "@/lib/ids";
import { refreshTasks, writeTask, retainTaskRevision } from "./taskQueries";
import type { TasksWorkspaceData } from "./useTasksWorkspaceData";

/** What the open task's panel does to that task. */
export function useTaskDetailActions(
  detail: TasksWorkspaceData["detail"],
  loadDetail: (key: string) => Promise<void>,
) {
  const api = useApi();
  const client = useQueryClient();
  const write = <T>(operation: () => Promise<T>) => writeTask(client, api, operation);
  const saveDetail = async (input: {
    revision: number;
    title: string;
    description: string;
    pull_request: string;
    status?: TaskStatus;
    assignee?: string;
    manual_block_reason?: string;
    priority: TaskPriority;
  }) => {
    if (!detail) throw new Error("Task is no longer selected");
    try {
      const updated = await write(() => api.tasks.updateTask(detail.task.key, { ...input }));
      toast.success("Task updated");
      return retainTaskRevision(client, api, updated);
    } catch (error) {
      toast.error(errorText(error));
      throw error;
    }
  };

  const comment = async (body: string, idempotencyKey: string) => {
    if (!detail) return;
    try {
      await write(() => api.tasks.addTaskComment(detail.task.key, body, idempotencyKey));
    } catch (error) {
      toast.error(errorText(error));
      throw error;
    }
  };

  const transferDetail = async (hostID: string) => {
    if (!detail) throw new Error("Task is no longer selected");
    const key = detail.task.key;
    const host = await resolveDaemon(hostID);
    if (!host) throw new Error("That server is no longer registered");
    const label = host.label || host.id;
    // One client per side for the whole move: the source as it is now, the
    // destination as it resolved.
    const destination = createApi(host);
    await write(() =>
      transferTask(key, api.tasks, destination.tasks, label, idempotencyKey("task-action")),
    );
    await refreshTasks(client, destination);
    toast.success(`${key} moved to ${label}`);
  };

  const requeueDetail = async (queue: string) => {
    if (!detail) throw new Error("Task is no longer selected");
    const moved = await write(() =>
      api.tasks.requeueTask(detail.task.key, queue, detail.task.revision),
    );
    toast.success(`${detail.task.key} moved to ${queue} as ${moved.key}`);
    await loadDetail(moved.key);
  };

  const addRelation = async (targetKey: string, type: TaskRelationType) => {
    if (!detail) return;
    await write(() =>
      api.tasks.addTaskRelation(
        detail.task.key,
        targetKey,
        type,
        detail.task.revision,
        idempotencyKey("task-action"),
      ),
    );
  };
  const deleteRelation = async (relationID: number) => {
    if (!detail) return;
    await write(() =>
      api.tasks.deleteTaskRelation(
        detail.task.key,
        relationID,
        detail.task.revision,
        idempotencyKey("task-action"),
      ),
    );
  };
  return { saveDetail, comment, transferDetail, requeueDetail, addRelation, deleteRelation };
}
