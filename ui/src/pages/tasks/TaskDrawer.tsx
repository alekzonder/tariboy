import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { errorText as errorMessage, isAccessError } from "@/lib/queryClient";
import { taskQueries, refreshTasks, writeTask, retainTaskRevision } from "./taskQueries";
import { useTaskQueryEvents } from "./useTaskQueryEvents";
import { toast } from "sonner";
import {
  createApi,
  isConnectionError,
  transferTask,
  type Task,
  type TaskPriority,
  type TaskRelationType,
  type TaskStatus,
} from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import { targetFor } from "@/lib/terminalsHost";
import TaskDetail, { TaskDetailLoading } from "./TaskDetail";
import TaskPanelResizeHandle from "./TaskPanelResizeHandle";
import { clampTaskDetailWidth, useTaskPanelWidths } from "./useTaskPanelWidths";
import "./tasks.css";

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `task-action-${Date.now()}-${Math.random()}`;
}

type TaskEdit = {
  revision: number;
  title: string;
  description: string;
  pull_request: string;
  status?: TaskStatus;
  assignee?: string;
  manual_block_reason?: string;
  priority: TaskPriority;
};

/** An assignee the drawer may offer. `hostId` is set when the agent lives on
 *  another server: picking it moves the task there first. */
export interface AssigneeChoice {
  value: string;
  label: string;
  assignee: string;
  hostId?: string;
  hostLabel?: string;
}

/**
 * One task, opened where it was mentioned. It is the same panel the Tasks
 * workspace shows, with shared server queries and independent selection.
 */
function TaskDrawerContent({
  taskKey,
  onClose,
  assignees,
  onMoved,
}: {
  taskKey: string;
  onClose: () => void;
  assignees?: readonly AssigneeChoice[];
  /** The task now lives on `hostId`; the owner reopens it there. */
  onMoved?: (hostId: string) => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  // The same stored width and grip as the Tasks tab, so the panel is one size
  // wherever it opens; the drawer spans the window rather than a workspace.
  const { detailWidth, setDetailWidth } = useTaskPanelWidths();
  const resizeHandle = (
    <TaskPanelResizeHandle
      width={detailWidth}
      maximum={clampTaskDetailWidth(Infinity)}
      onResize={setDetailWidth}
    />
  );
  const detailQuery = useApiQuery(taskQueries.detail(api, taskKey));
  const historyQuery = useApiQuery(taskQueries.history(api, taskKey, "first"));
  const peopleQuery = useApiQuery(taskQueries.principals(api));
  const sequenceQuery = useApiQuery(taskQueries.sequence(api));
  const detail = detailQuery.data;
  const events = historyQuery.data ?? [];
  const principals = peopleQuery.data ?? null;
  useTaskQueryEvents(sequenceQuery.data?.sequence, taskKey);
  const write = <T,>(operation: () => Promise<T>) => writeTask(client, api, operation);
  useEffect(() => {
    const error = detailQuery.error ?? historyQuery.error;
    if (!error || isConnectionError(error)) return;
    toast.error(errorMessage(error));
    if (!detailQuery.data && !isAccessError(error)) onClose();
  }, [detailQuery.error, detailQuery.data, historyQuery.error, onClose]);

  // An agent on another server can only work a task that lives there. Other
  // edits are saved here first; then the tree is exported and imported with
  // its root unassigned, so no same-named agent over there takes it before
  // the real assignment, which is an ordinary update and notifies the agent.
  // Once the import succeeds the task lives there: a later failure is
  // reported, and the sheet follows the task all the same.
  const moveToAgent = async (
    task: Task,
    input: TaskEdit,
    remote: AssigneeChoice,
  ): Promise<Task> => {
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
    const tasks = (bundle.tasks as Array<{ key: string; assignee: string }>).map((item) =>
      item.key === bundle.root_key ? { ...item, assignee: "" } : item,
    );
    const imported = await destination.tasks.importTask({ ...bundle, tasks });
    let result = imported;
    try {
      await api.tasks.addTaskComment(key, `Moved to ${label} as ${key}.`, idempotencyKey());
      const left = await api.tasks.getTask(key);
      await api.tasks.updateTask(key, { status: "cancelled", revision: left.task.revision });
      const arrived = await destination.tasks.getTask(key);
      result = await destination.tasks.updateTask(key, {
        assignee: remote.assignee,
        revision: arrived.task.revision,
      });
      toast.success(`${key} moved to ${label}`);
    } catch (error) {
      toast.error(`${key} moved to ${label}, but the move did not finish: ${errorMessage(error)}`);
    }
    await refreshTasks(client, destination);
    onMoved?.(hostId);
    return result;
  };

  if (detail?.task.key !== taskKey || !historyQuery.data) {
    return (
      <TaskDetailLoading
        taskKey={taskKey}
        width={detailWidth}
        resizeHandle={resizeHandle}
        onClose={onClose}
        error={
          detailQuery.error || historyQuery.error
            ? errorMessage(detailQuery.error ?? historyQuery.error)
            : undefined
        }
      />
    );
  }

  return (
    <TaskDetail
      key={detail.task.key}
      detail={detail}
      principals={principals}
      events={events}
      width={detailWidth}
      resizeHandle={resizeHandle}
      onClose={onClose}
      assigneeOptions={assignees}
      onTransfer={async (hostID: string) => {
        const host = await resolveDaemon(hostID);
        if (!host) throw new Error("That server is no longer registered");
        const label = host.label || host.id;
        const destination = createApi(host);
        await write(() =>
          transferTask(detail.task.key, api.tasks, destination.tasks, label, idempotencyKey()),
        );
        await refreshTasks(client, destination);
        toast.success(`${detail.task.key} moved to ${label}`);
      }}
      onRequeue={async (queue: string) => {
        // The drawer shows the key its owner chose, so it stays on the
        // cancelled task, whose comment links the new one.
        const moved = await write(() =>
          api.tasks.requeueTask(detail.task.key, queue, detail.task.revision),
        );
        toast.success(`${detail.task.key} moved to ${queue} as ${moved.key}`);
      }}
      onSave={async (input: TaskEdit) => {
        const task = detail.task;
        const remote = assignees?.find(
          (choice) => choice.hostId !== undefined && choice.value === input.assignee,
        );
        // A closed list can lose an entry between the pick and Save (its server
        // stopped answering). Its value is not a real assignee, so never write it.
        if (
          assignees &&
          !remote &&
          input.assignee &&
          input.assignee !== task.assignee &&
          input.assignee !== principals?.customer &&
          !assignees.some((choice) => choice.value === input.assignee)
        ) {
          const message = "That agent's server is unavailable; pick the assignee again";
          toast.error(message);
          throw new Error(message);
        }
        try {
          if (remote?.hostId !== undefined) {
            return await write(() => moveToAgent(task, input, remote));
          }
          const updated = await write(() => api.tasks.updateTask(detail.task.key, input));
          toast.success("Task updated");
          return retainTaskRevision(client, api, updated);
        } catch (error) {
          toast.error(errorMessage(error));
          throw error;
        }
      }}
      onComment={async (body: string, key: string) => {
        try {
          await write(() => api.tasks.addTaskComment(detail.task.key, body, key));
        } catch (error) {
          toast.error(errorMessage(error));
          throw error;
        }
      }}
      onAddRelation={async (targetKey: string, type: TaskRelationType) => {
        await write(() =>
          api.tasks.addTaskRelation(
            detail.task.key,
            targetKey,
            type,
            detail.task.revision,
            idempotencyKey(),
          ),
        );
      }}
      onDeleteRelation={async (relationID: number) => {
        await write(() =>
          api.tasks.deleteTaskRelation(
            detail.task.key,
            relationID,
            detail.task.revision,
            idempotencyKey(),
          ),
        );
      }}
    />
  );
}

export default function TaskDrawer(props: Parameters<typeof TaskDrawerContent>[0]) {
  const api = useApi();
  return (
    <TaskDrawerContent
      key={JSON.stringify([api.target?.id ?? "local", props.taskKey])}
      {...props}
    />
  );
}
