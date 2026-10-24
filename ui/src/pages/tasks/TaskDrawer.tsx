import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { errorText } from "@/lib/errors";
import { isAccessError } from "@/lib/queryClient";
import { taskQueries, refreshTasks, writeTask, retainTaskRevision } from "./taskQueries";
import { useTaskQueryEvents } from "./useTaskQueryEvents";
import { toast } from "sonner";
import { createApi, isConnectionError, transferTask, type TaskRelationType } from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import TaskDetail, { TaskDetailLoading } from "./TaskDetail";
import TaskPanelResizeHandle from "./TaskPanelResizeHandle";
import { clampTaskDetailWidth, useTaskPanelWidths } from "./useTaskPanelWidths";
import { moveTaskToHost, type TaskEdit } from "./taskDrawerMove";
import "./tasks.css";
import { idempotencyKey } from "@/lib/ids";

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
    toast.error(errorText(error));
    if (!detailQuery.data && !isAccessError(error)) onClose();
  }, [detailQuery.error, detailQuery.data, historyQuery.error, onClose]);

  if (detail?.task.key !== taskKey || !historyQuery.data) {
    return (
      <TaskDetailLoading
        taskKey={taskKey}
        width={detailWidth}
        resizeHandle={resizeHandle}
        onClose={onClose}
        error={
          detailQuery.error || historyQuery.error
            ? errorText(detailQuery.error ?? historyQuery.error)
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
          transferTask(
            detail.task.key,
            api.tasks,
            destination.tasks,
            label,
            idempotencyKey("task-action"),
          ),
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
            return await write(() => moveTaskToHost({ api, client, task, input, remote, onMoved }));
          }
          const updated = await write(() => api.tasks.updateTask(detail.task.key, input));
          toast.success("Task updated");
          return retainTaskRevision(client, api, updated);
        } catch (error) {
          toast.error(errorText(error));
          throw error;
        }
      }}
      onComment={async (body: string, key: string) => {
        try {
          await write(() => api.tasks.addTaskComment(detail.task.key, body, key));
        } catch (error) {
          toast.error(errorText(error));
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
            idempotencyKey("task-action"),
          ),
        );
      }}
      onDeleteRelation={async (relationID: number) => {
        await write(() =>
          api.tasks.deleteTaskRelation(
            detail.task.key,
            relationID,
            detail.task.revision,
            idempotencyKey("task-action"),
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
