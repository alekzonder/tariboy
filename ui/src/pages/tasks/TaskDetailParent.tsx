import type { Queue, Task, TaskParent } from "@/types";
import { parentQueueLabel } from "@/lib/taskTree";
import { cn } from "@/lib/utils";
import { MONO } from "@/components/ui/recipes";
import QueueMarker from "./QueueMarker";

const FOCUS =
  "rounded-chip focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none";
const HIDDEN_TITLE = "You cannot open this task";

/**
 * `Subtask of KEY title` above the panel title. A parent the reader cannot
 * see is named by key only, as text.
 */
export function TaskParentLine({
  task,
  parent,
  queues,
  onOpen,
}: {
  task: Task;
  parent?: TaskParent;
  queues?: readonly Queue[];
  onOpen?: (key: string) => void;
}) {
  if (!task.parent_key) return null;
  const line = "flex min-w-0 items-center gap-1.5 text-caption text-muted-foreground";
  if (!parent || !onOpen) {
    return (
      <span title={HIDDEN_TITLE} className={line}>
        Subtask of <span className={MONO}>{task.parent_key}</span>
      </span>
    );
  }
  return (
    <button
      type="button"
      aria-label={`Open parent task ${parent.key}: ${parent.title}`}
      className={cn(line, FOCUS, "w-full text-left hover:text-foreground")}
      onClick={() => onOpen(parent.key)}
    >
      <span className="shrink-0">Subtask of</span>
      {parent.queue !== task.queue && (
        <QueueMarker label={parentQueueLabel(parent.queue, queues ?? [])} />
      )}
      <span className={cn(MONO, "shrink-0")}>{parent.key}</span>
      <span className="min-w-0 truncate">{parent.title}</span>
    </button>
  );
}

/** The value of the Parent field in Details: the parent's key, as a button
 *  when the reader can open it. */
export function TaskParentField({
  parentKey,
  parent,
  onOpen,
}: {
  parentKey: string;
  parent?: TaskParent;
  onOpen?: (key: string) => void;
}) {
  if (!parentKey) return <>—</>;
  if (!parent || !onOpen) return <span title={HIDDEN_TITLE}>{parentKey}</span>;
  return (
    <button
      type="button"
      aria-label={`Open parent task ${parent.key}: ${parent.title}`}
      className={cn(FOCUS, "underline-offset-2 hover:underline")}
      onClick={() => onOpen(parent.key)}
    >
      {parent.key}
    </button>
  );
}
