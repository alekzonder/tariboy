import type { Queue, Task } from "@/types";
import { taskStatusLabel } from "@/lib/tasks";
import { queueMarkerLabel } from "@/lib/taskTree";
import { cn } from "@/lib/utils";
import { BannerButton, SectionHeading } from "./TaskDetailControls";
import { EMPTY, MONO } from "./panelStyles";
import QueueMarker from "./QueueMarker";
import { TaskStatusView } from "./TaskRowCells";
import type { TaskDetailProps } from "./TaskDetailTypes";

const hiddenText = (count: number) =>
  count === 1
    ? "1 more subtask is hidden from you."
    : `${count} more subtasks are hidden from you.`;

/** One subtask: one line on a wide panel; on a phone the marker, key and
 *  status on the first line and the title on the second. */
function SubtaskRow({
  task,
  parent,
  queues,
  onOpen,
}: {
  task: Task;
  parent: Task;
  queues: readonly Queue[];
  onOpen: (key: string) => void;
}) {
  return (
    <button
      type="button"
      aria-label={`Open subtask ${task.key}: ${task.title}, ${taskStatusLabel(task)}`}
      className="flex w-full flex-wrap items-center gap-x-2 gap-y-0.5 rounded-md px-2 py-1.5 text-left text-body hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none sm:flex-nowrap"
      onClick={() => onOpen(task.key)}
    >
      <span className="order-1 flex min-w-0 shrink-0 items-center gap-1.5">
        {task.queue !== parent.queue && (
          <QueueMarker label={queueMarkerLabel(task, parent.queue, queues)} />
        )}
        <span className={cn(MONO, "text-muted-foreground")}>{task.key}</span>
      </span>
      <span className="order-3 min-w-0 basis-full truncate sm:order-2 sm:flex-1 sm:basis-auto">
        {task.title}
      </span>
      <TaskStatusView task={task} className="order-2 ml-auto sm:order-3 sm:ml-0" />
      {task.assignee && (
        <span className="order-4 hidden h-4.75 max-w-[12ch] shrink-0 items-center truncate rounded-sm bg-muted px-1.75 text-meta sm:inline-flex">
          {task.assignee.replace(/^agent:/, "")}
        </span>
      )}
    </button>
  );
}

/** The Subtasks section of a loaded panel. */
export function TaskDetailSubtasks({
  detail,
  queues,
  onOpenTask,
  subtasksStatus: status,
}: Pick<TaskDetailProps, "detail" | "queues" | "onOpenTask" | "subtasksStatus">) {
  return (
    <TaskSubtasksSection
      task={detail.task}
      // An older daemon sends no list.
      subtasks={detail.children as Task[] | undefined}
      hidden={detail.hidden_children ?? 0}
      queues={queues ?? []}
      fetching={status?.fetching ?? false}
      error={status?.error ?? false}
      onRetry={() => status?.retry()}
      onOpen={(key) => onOpenTask?.(key)}
    />
  );
}

/**
 * The direct subtasks of the panel's task, of every category and queue, in
 * the order the daemon gives. `subtasks` is undefined when the read has not
 * brought the list (yet); `error` means the latest read of the task failed.
 */
export default function TaskSubtasksSection({
  task,
  subtasks,
  hidden,
  queues,
  fetching,
  error,
  onRetry,
  onOpen,
}: {
  task: Task;
  subtasks: Task[] | undefined;
  hidden: number;
  queues: readonly Queue[];
  fetching: boolean;
  error: boolean;
  onRetry: () => void;
  onOpen: (key: string) => void;
}) {
  const loading = !error && !subtasks && fetching;
  const list = subtasks ?? [];
  if (!error && !loading && list.length === 0 && hidden === 0) return null;
  return (
    <section aria-label="Subtasks" className="flex min-w-0 flex-col gap-1.5">
      <SectionHeading label="Subtasks" count={list.length} />
      {error ? (
        <div role="alert" className="flex items-center gap-2 text-body-sm">
          <span className="text-status-failed">Could not load subtasks.</span>
          <BannerButton tone="quiet" onClick={onRetry}>
            Retry
          </BannerButton>
        </div>
      ) : loading ? (
        [0, 1].map((index) => (
          <div
            key={index}
            data-testid="subtask-placeholder"
            aria-hidden="true"
            className="h-8 animate-pulse rounded-md bg-muted"
          />
        ))
      ) : (
        <>
          {list.length > 0 && (
            <div className="-mx-2 flex min-w-0 flex-col">
              {list.map((child) => (
                <SubtaskRow
                  key={child.key}
                  task={child}
                  parent={task}
                  queues={queues}
                  onOpen={onOpen}
                />
              ))}
            </div>
          )}
          {hidden > 0 && <p className={EMPTY}>{hiddenText(hidden)}</p>}
        </>
      )}
    </section>
  );
}
