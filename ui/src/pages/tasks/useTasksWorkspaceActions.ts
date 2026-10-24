import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import {
  ApiError,
  createApi,
  transferTask,
  type CreateQueueInput,
  type UpdateQueueInput,
  type TaskPriority,
  type TaskRelationType,
  type TaskStatus,
} from "@/lib/api";
import { errorText } from "@/lib/errors";
import { buildTaskForest, canDropTaskInside } from "@/lib/taskTree";
import { resolveDaemon } from "@/lib/daemons";
import { taskKeys, refreshTasks, writeTask, retainTaskRevision } from "./taskQueries";
import { rememberQueue } from "./taskQueueMemory";
import type { TasksWorkspaceView } from "./useTasksWorkspaceView";
import type { TasksWorkspaceData } from "./useTasksWorkspaceData";
import { idempotencyKey } from "@/lib/ids";

export function useTasksWorkspaceActions(
  server: string,
  state: TasksWorkspaceView,
  data: TasksWorkspaceData,
) {
  const api = useApi();
  const client = useQueryClient();
  const { setCreatingParent, setExpanded, loadDetail, selectQueue } = state;
  const { tasks, detail, setTasks, loadMetadata } = data;
  const write = <T>(operation: () => Promise<T>) => writeTask(client, api, operation);
  const create = async (input: { queue: string; parent_key: string; title: string }) => {
    try {
      const created = await write(() => api.tasks.createTask(input));
      if (!input.parent_key) rememberQueue(server, input.queue);
      setCreatingParent(null);
      if (input.parent_key) {
        setExpanded((current) => new Set(current).add(input.parent_key));
      }
      await loadDetail(created.key);
    } catch (error) {
      toast.error(errorText(error));
    }
  };

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

  const performMove = async (key: string, parentKey: string, beforeKey: string) => {
    const moving = tasks.find((task) => task.key === key);
    const parent = tasks.find((task) => task.key === parentKey);
    if (!moving) return;
    const forest = buildTaskForest(tasks);
    if (
      (parent && parent.queue !== moving.queue) ||
      (parentKey && !canDropTaskInside(forest, key, parentKey))
    ) {
      toast.error("Tasks can only move inside their queue and outside their own subtree");
      return;
    }
    await client.cancelQueries({ queryKey: taskKeys.server(api.target) });
    const previous = tasks;
    setTasks((current) =>
      current.map((task) => (task.key === key ? { ...task, parent_key: parentKey } : task)),
    );
    try {
      const updated = await api.tasks.moveTask(key, {
        parent_key: parentKey,
        before_key: beforeKey,
        revision: moving.revision,
      });
      setTasks((current) => current.map((task) => (task.key === key ? updated : task)));
      if (parentKey) setExpanded((current) => new Set(current).add(parentKey));
    } catch (error) {
      setTasks(previous);
      toast.error(errorText(error));
    } finally {
      await refreshTasks(client, api);
    }
  };

  const changePriority = async (key: string, priority: TaskPriority) => {
    const changing = tasks.find((task) => task.key === key);
    if (!changing) return;
    try {
      const updated = await write(() =>
        api.tasks.updateTask(key, { priority, revision: changing.revision }),
      );
      retainTaskRevision(client, api, updated);
    } catch (error) {
      toast.error(errorText(error));
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

  const createQueue = async (input: CreateQueueInput) => {
    try {
      const created = await write(() => api.tasks.createTaskQueue(input));
      selectQueue(created.prefix);
    } catch (error) {
      toast.error(errorText(error));
    }
  };

  const updateQueue = async (prefix: string, input: UpdateQueueInput) => {
    try {
      await write(() => api.tasks.updateTaskQueue(prefix, input));
      toast.success("Queue updated");
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) await loadMetadata();
      toast.error(errorText(error));
    }
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
  return {
    create,
    saveDetail,
    comment,
    performMove,
    changePriority,
    transferDetail,
    requeueDetail,
    createQueue,
    updateQueue,
    addRelation,
    deleteRelation,
  };
}

export type TasksWorkspaceActions = ReturnType<typeof useTasksWorkspaceActions>;
