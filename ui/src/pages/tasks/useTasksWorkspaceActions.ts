import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import {
  ApiError,
  type CreateQueueInput,
  type UpdateQueueInput,
  type TaskPriority,
} from "@/lib/api";
import { errorText } from "@/lib/errors";
import { buildTaskForest, canDropTaskInside } from "@/lib/taskTree";
import { taskKeys, refreshTasks, writeTask, retainTaskRevision } from "./taskQueries";
import { useTaskDetailActions } from "./useTaskDetailActions";
import { rememberQueue } from "./taskQueueMemory";
import type { TasksWorkspaceView } from "./useTasksWorkspaceView";
import type { TasksWorkspaceData } from "./useTasksWorkspaceData";

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
  const detailActions = useTaskDetailActions(detail, loadDetail);
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

  return {
    create,
    ...detailActions,
    performMove,
    changePriority,
    createQueue,
    updateQueue,
  };
}

export type TasksWorkspaceActions = ReturnType<typeof useTasksWorkspaceActions>;
