import { useDraggable, useDroppable } from "@dnd-kit/core";
import { GripVertical, Plus } from "lucide-react";
import { cn } from "@/lib/utils";
import type { TaskPriority } from "@/lib/api";
import type { VisibleTaskRow } from "@/lib/taskTree";
import { useTaskColumns } from "./taskColumnLayout";
import { TASK_COLUMNS, type TaskRowMode } from "./taskColumns";
import TaskRowCell from "./TaskRowCells";

/** Marks the scope agent's current Goal row; a plain row gets nothing. */
function goalRow(goal: boolean, selected: boolean) {
  if (!goal) return { attrs: {}, className: undefined };
  return {
    attrs: { "data-goal": true, title: "Current Goal of this agent" },
    // The tint gives way to the selection; the left stripe stays.
    className: cn(
      "shadow-[inset_2px_0_0_var(--status-running)]",
      !selected && "bg-status-running/8 hover:bg-status-running/12",
    ),
  };
}

export default function TaskRow({
  row,
  mode,
  hasActiveQuestion,
  expanded,
  selected,
  goal = false,
  onToggle,
  onSelect,
  onAddChild,
  onPriorityChange,
}: {
  row: VisibleTaskRow;
  mode: TaskRowMode;
  hasActiveQuestion: boolean;
  expanded: boolean;
  selected: boolean;
  /** The row is the scope agent's current Goal task. */
  goal?: boolean;
  onToggle: () => void;
  onSelect: () => void;
  onAddChild?: () => void;
  /** Saves a priority picked in the row's menu; without it the value is a
   *  plain tag. The menu stays disabled until the returned promise settles. */
  onPriorityChange?: (priority: TaskPriority) => Promise<void> | void;
}) {
  const readOnly = row.task.access === "context" || row.task.access === "respond";
  const { setNodeRef, setActivatorNodeRef, isDragging, listeners, attributes } = useDraggable({
    id: row.task.key,
    disabled: readOnly,
  });
  const { setNodeRef: setBeforeRef, isOver: overBefore } = useDroppable({
    id: `before:${row.task.key}`,
    disabled: readOnly,
  });
  const { setNodeRef: setInsideRef, isOver: overInside } = useDroppable({
    id: `inside:${row.task.key}`,
    disabled: readOnly,
  });
  const { setNodeRef: setAfterRef, isOver: overAfter } = useDroppable({
    id: `after:${row.task.key}`,
    disabled: readOnly,
  });
  const columns = useTaskColumns(mode);
  const goalMark = goalRow(goal, selected);
  const cell = {
    row,
    readOnly,
    hasActiveQuestion,
    expanded,
    onToggle,
    onSelect,
    onPriorityChange,
  };
  return (
    <div
      ref={setNodeRef}
      data-testid={`task-row-${row.task.key}`}
      {...goalMark.attrs}
      // A mouse or finger opens the task anywhere on the row; the keyboard keeps
      // the title button. The row's own buttons, and clicks React bubbles up
      // from the priority menu's portal, keep their own action.
      onClick={(event) => {
        const target = event.target as Element;
        if (!event.currentTarget.contains(target) || target.closest("button, a, input")) return;
        onSelect();
      }}
      className={cn(
        "group/row relative flex min-h-7.5 cursor-pointer items-center gap-2.5 px-4 text-body",
        selected ? "bg-accent" : "hover:bg-muted",
        goalMark.className,
        overInside && "ring-1 ring-primary ring-inset",
        row.task.access === "context" && "opacity-60",
        // Last, so a dragged context row still dims to the drag opacity.
        isDragging && "opacity-45",
      )}
    >
      <span
        ref={setBeforeRef}
        data-testid={`drop-before-${row.task.key}`}
        className={`task-drop-zone before ${overBefore ? "is-over" : ""}`}
      />
      <span
        ref={setInsideRef}
        data-testid={`drop-inside-${row.task.key}`}
        className="task-drop-zone inside"
      />
      <span
        ref={setAfterRef}
        data-testid={`drop-after-${row.task.key}`}
        className={`task-drop-zone after ${overAfter ? "is-over" : ""}`}
      />
      {/* The grip lives in the row's own left padding: the reference table has
          no column for it, and it is only there under the pointer anyway. */}
      <button
        type="button"
        ref={setActivatorNodeRef}
        className="task-row-grip absolute top-1/2 left-0 z-2 grid h-6 w-4 -translate-y-1/2 cursor-grab place-items-center text-muted-foreground opacity-0 group-hover/row:opacity-60"
        aria-label={`Move ${row.task.key}`}
        {...listeners}
        {...attributes}
      >
        <GripVertical aria-hidden="true" className="size-3" />
      </button>
      {columns.map((column) => (
        <TaskRowCell key={column.id} column={column.id} {...cell} style={{ width: column.width }} />
      ))}
      <span style={{ width: TASK_COLUMNS.add }} className="shrink-0">
        {!readOnly && (
          <button
            type="button"
            className="relative z-2 grid size-6 place-items-center rounded-control text-muted-foreground opacity-0 hover:bg-accent hover:text-foreground group-hover/row:opacity-100"
            aria-label={`Add child to ${row.task.key}`}
            onClick={onAddChild}
          >
            <Plus aria-hidden="true" className="size-3.5" />
          </button>
        )}
      </span>
    </div>
  );
}
