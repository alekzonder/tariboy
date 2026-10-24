import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type Modifier,
} from "@dnd-kit/core";
import { ChevronRight, GripVertical, Plus, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import {
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { PriorityTag, StatusPill } from "@/components/ui/status";
import { cn } from "@/lib/utils";
import { TONE_TEXT, taskTone, type StatusTone } from "@/lib/statusTone";
import type { TaskPriority, WorkflowAttention } from "@/lib/api";
import { isWorkflowTask, taskStatusLabel } from "@/lib/tasks";
import type { VisibleTaskRow } from "@/lib/taskTree";
import TaskColumnResizeHandle from "./TaskColumnResizeHandle";
import {
  moveTaskColumn,
  PHONE_TASK_QUERY,
  TASK_COLUMN_LABELS,
  useTaskColumns,
  type TaskColumn,
  type TaskColumnId,
} from "./taskColumnLayout";
import { INDENT_PX, MAX_INDENT_DEPTH, TASK_COLUMNS, type TaskRowMode } from "./taskColumns";
import { formatTaskDuration, formatTaskTime } from "./taskTime";

const priorityLabels = {
  P0: "Critical",
  P1: "High",
  P2: "Normal",
  P3: "Low",
} as const;

/** The tone a row's problem mark takes, or null when the summary earns no mark:
 *  an idle holder is the panel's business, not the list's. */
function attentionTone(attention: WorkflowAttention | undefined): StatusTone | null {
  switch (attention?.kind) {
    case "failed":
    case "script_failed":
      return "danger";
    case "rejected":
      return "attention";
    default:
      return null;
  }
}

const ATTENTION_TITLE: Record<WorkflowAttention["kind"], string> = {
  paused: "Paused",
  failed: "A check could not run",
  rejected: "A check rejected the request",
  script_failed: "The watch script failed",
  idle: "No transition requested yet",
};

/** A problem on a workflow task, as one mark the row has room for; the title
 *  carries the script's words. */
function AttentionMark({ attention }: { attention: WorkflowAttention }) {
  const tone = attentionTone(attention) ?? "quiet";
  const title = [
    ATTENTION_TITLE[attention.kind],
    attention.message,
    attention.count ? `(${attention.count})` : "",
  ]
    .filter(Boolean)
    .join(": ")
    .replace(": (", " (");
  return (
    <span
      role="img"
      aria-label={`Problem: ${title}`}
      title={title}
      data-attention={attention.kind}
      className={cn("inline-flex shrink-0 items-center", TONE_TEXT[tone])}
    >
      <TriangleAlert aria-hidden="true" className="size-3.5 [stroke-width:1.8]" />
    </span>
  );
}

export default function TaskRow({
  row,
  mode,
  server,
  hasActiveQuestion,
  expanded,
  selected,
  onToggle,
  onSelect,
  onAddChild,
  onPriorityChange,
}: {
  row: VisibleTaskRow;
  mode: TaskRowMode;
  /** The server the task lives on; shown only in the `servers` table. */
  server?: string;
  hasActiveQuestion: boolean;
  expanded: boolean;
  selected: boolean;
  onToggle: () => void;
  onSelect: () => void;
  onAddChild?: () => void;
  /** Saves a priority picked in the row's menu; without it the value is a
   *  plain tag. The menu stays disabled until the returned promise settles. */
  onPriorityChange?: (priority: TaskPriority) => Promise<void> | void;
}) {
  const readOnly = row.task.access === "context" || row.task.access === "respond";
  // The every-server table has no drag and no add-child: neither can cross the
  // server boundary the rows sit on.
  const disabled = readOnly || mode === "servers";
  const { setNodeRef, setActivatorNodeRef, isDragging, listeners, attributes } = useDraggable({
    id: row.task.key,
    disabled,
  });
  const { setNodeRef: setBeforeRef, isOver: overBefore } = useDroppable({
    id: `before:${row.task.key}`,
    disabled,
  });
  const { setNodeRef: setInsideRef, isOver: overInside } = useDroppable({
    id: `inside:${row.task.key}`,
    disabled,
  });
  const { setNodeRef: setAfterRef, isOver: overAfter } = useDroppable({
    id: `after:${row.task.key}`,
    disabled,
  });
  const tone = taskTone(row.task.category);
  const workflow = isWorkflowTask(row.task);
  const columns = useTaskColumns(mode);
  return (
    <div
      ref={setNodeRef}
      data-testid={`task-row-${row.task.key}`}
      style={{ opacity: isDragging ? 0.45 : undefined }}
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
        overInside && "ring-1 ring-primary ring-inset",
        row.task.access === "context" && "opacity-60",
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
      {mode !== "servers" && (
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
      )}
      {columns.map((column) => {
        const style = { width: column.width };
        switch (column.id) {
          // Priority leads every mode by default, unlike the reference, which
          // drops it from the all-tasks table: in this app priority is what
          // orders the tree, so a table without it cannot be read.
          case "priority":
            return (
              <span key="priority" data-column="priority" style={style} className="z-2 shrink-0">
                {readOnly || !onPriorityChange ? (
                  <PriorityTag
                    className="task-priority"
                    priority={row.task.priority}
                    aria-label={priorityName(row.task.priority)}
                  />
                ) : (
                  <PriorityMenu priority={row.task.priority} onChange={onPriorityChange} />
                )}
              </span>
            );
          case "key":
            return (
              <span
                key="key"
                data-column="key"
                style={style}
                className="z-2 flex shrink-0 items-center gap-1.5 overflow-hidden"
              >
                {Array.from({ length: Math.min(row.depth, MAX_INDENT_DEPTH) }, (_, level) => (
                  <span
                    key={level}
                    aria-hidden="true"
                    className="shrink-0"
                    style={{ width: INDENT_PX }}
                  />
                ))}
                <button
                  type="button"
                  className="grid size-4 shrink-0 place-items-center text-muted-foreground opacity-65 disabled:opacity-0"
                  aria-label={`${expanded ? "Collapse" : "Expand"} ${row.task.key}`}
                  disabled={!row.hasChildren}
                  onClick={onToggle}
                >
                  <ChevronRight
                    aria-hidden="true"
                    className={cn(
                      "size-2.5 transition-transform duration-[120ms]",
                      expanded && "rotate-90",
                    )}
                  />
                </button>
                <span className="min-w-0 truncate font-mono text-meta text-muted-foreground tabular-nums">
                  {row.task.key}
                </span>
              </span>
            );
          case "task":
            return (
              <button
                key="task"
                type="button"
                data-column="task"
                className="task-row-main relative z-2 flex min-w-0 flex-1 items-center gap-1.75 overflow-hidden text-left"
                onClick={onSelect}
              >
                <span className="min-w-0 truncate">{row.task.title}</span>
                {hasActiveQuestion && (
                  <span
                    className="size-1.25 shrink-0 rounded-full bg-primary"
                    role="img"
                    aria-label={`Unread question notification for ${row.task.key}`}
                  />
                )}
                {row.task.blocked && (
                  <StatusPill tone="danger" size="sm">
                    blocked
                  </StatusPill>
                )}
              </button>
            );
          case "agent":
            return (
              <span key="agent" data-column="agent" style={style} className="shrink-0">
                {row.task.assignee && (
                  <span className="inline-flex h-4.75 max-w-full items-center truncate rounded-sm bg-muted px-1.75 text-meta">
                    {row.task.assignee.replace(/^agent:/, "")}
                  </span>
                )}
              </span>
            );
          case "server":
            return (
              <span
                key="server"
                data-column="server"
                style={style}
                className="shrink-0 truncate font-mono text-meta text-muted-foreground"
              >
                {server}
              </span>
            );
          case "status":
            return workflow ? (
              <span
                key="status"
                data-column="status"
                style={style}
                className="flex min-w-0 shrink-0 items-center gap-1 overflow-hidden"
              >
                <StatusPill tone={tone} className="min-w-0 shrink">
                  {/* A workflow status ID can be 64 characters: CSS clips it, the
                      title keeps the whole ID readable. */}
                  <span title={row.task.status} className="min-w-0 max-w-[24ch] truncate">
                    {taskStatusLabel(row.task)}
                  </span>
                </StatusPill>
                {row.task.workflow_step && (
                  <span
                    title={`Step ${row.task.workflow_step.index} of ${row.task.workflow_step.total}`}
                    className="hidden shrink-0 font-mono text-tiny tabular-nums text-muted-foreground sm:inline"
                  >
                    {row.task.workflow_step.index}/{row.task.workflow_step.total}
                  </span>
                )}
                {row.task.waiting_on === "pause" ? (
                  <StatusPill
                    tone="danger"
                    size="sm"
                    title={row.task.workflow_paused_reason || undefined}
                  >
                    paused
                  </StatusPill>
                ) : attentionTone(row.task.workflow_attention) ? (
                  <AttentionMark attention={row.task.workflow_attention!} />
                ) : row.task.waiting_on ? (
                  <StatusPill tone="quiet" size="sm">
                    {row.task.waiting_on}
                  </StatusPill>
                ) : null}
              </span>
            ) : (
              <span key="status" data-column="status" style={style} className="shrink-0">
                <StatusPill tone={tone}>{taskStatusLabel(row.task)}</StatusPill>
              </span>
            );
          case "duration":
            return (
              <span
                key="duration"
                data-column="duration"
                style={style}
                className="shrink-0 truncate text-right font-mono text-meta text-muted-foreground tabular-nums"
              >
                {formatTaskDuration(row.task)}
              </span>
            );
          case "updated":
            return (
              <span
                key="updated"
                data-column="updated"
                style={style}
                className="shrink-0 truncate text-right text-ui text-muted-foreground tabular-nums"
              >
                {formatTaskTime(row.task.updated_at)}
              </span>
            );
        }
      })}
      {mode !== "servers" && (
        <span style={{ width: TASK_COLUMNS.add }} className="shrink-0">
          {!disabled && (
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
      )}
    </div>
  );
}

const priorityName = (priority: TaskPriority) => `${priority} ${priorityLabels[priority]}`;

function PriorityMenu({
  priority,
  onChange,
}: {
  priority: TaskPriority;
  onChange: (priority: TaskPriority) => Promise<void> | void;
}) {
  const [saving, setSaving] = useState(false);
  const pick = async (next: TaskPriority) => {
    if (next === priority) return;
    setSaving(true);
    try {
      await onChange(next);
    } finally {
      setSaving(false);
    }
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={saving}>
        <button
          type="button"
          className="task-priority flex rounded-sm disabled:opacity-50"
          aria-label={priorityName(priority)}
        >
          <PriorityTag priority={priority} aria-hidden="true" className="cursor-pointer" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {(Object.keys(priorityLabels) as TaskPriority[]).map((value) => (
          <DropdownMenuCheckItem
            key={value}
            checked={value === priority}
            onSelect={() => void pick(value)}
          >
            <span className="font-mono">{value}</span>
            <span className="text-muted-foreground">{priorityLabels[value]}</span>
          </DropdownMenuCheckItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** A header cell's label is its drag grip; a mouse moves it after 5px, a
 *  finger after a 250ms press, so scrolling the table never starts a move. */
const headerSensorOptions = {
  mouse: { activationConstraint: { distance: 5 } },
  touch: { activationConstraint: { delay: 250, tolerance: 5 } },
};

const horizontalOnly: Modifier = ({ transform }) => ({ ...transform, y: 0 });

/** The table header. Same widths, same order, same gaps as the rows under it:
 *  both read the one column layout. Its own drag context keeps a column move
 *  apart from the row drags under it. */
export function TaskTableHeader({ mode, sticky = false }: { mode: TaskRowMode; sticky?: boolean }) {
  const columns = useTaskColumns(mode);
  // A phone shows fixed columns, so moving or resizing one there would only
  // rewrite the layout the wider screens use.
  const editable = !useMediaQuery(PHONE_TASK_QUERY);
  const sensors = useSensors(
    useSensor(MouseSensor, headerSensorOptions.mouse),
    useSensor(TouchSensor, headerSensorOptions.touch),
    useSensor(KeyboardSensor),
  );
  const finish = ({ active, over }: DragEndEvent) => {
    if (over && active.id !== over.id)
      moveTaskColumn(active.id as TaskColumnId, over.id as TaskColumnId);
  };
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      modifiers={[horizontalOnly]}
      onDragEnd={finish}
    >
      <div
        data-testid="task-table-header"
        className={cn(
          "flex h-7.5 items-center gap-2.5 px-4 text-meta text-muted-foreground",
          sticky && "sticky top-0 z-2 bg-card",
        )}
      >
        {columns.map((column) => (
          <TaskHeaderCell key={column.id} column={column} editable={editable} />
        ))}
        {mode !== "servers" && (
          <span aria-hidden="true" style={{ width: TASK_COLUMNS.add }} className="shrink-0" />
        )}
      </div>
    </DndContext>
  );
}

function TaskHeaderCell({ column, editable }: { column: TaskColumn; editable: boolean }) {
  const { setNodeRef, setActivatorNodeRef, isDragging, listeners, attributes, transform } =
    useDraggable({ id: column.id, disabled: !editable });
  const { setNodeRef: setDropRef, isOver } = useDroppable({ id: column.id });
  const label = TASK_COLUMN_LABELS[column.id];
  const end = column.id === "duration" || column.id === "updated";
  return (
    <span
      ref={(node) => {
        setNodeRef(node);
        setDropRef(node);
      }}
      data-column={column.id}
      style={{
        width: column.width,
        transform: transform ? `translate3d(${transform.x}px, 0, 0)` : undefined,
      }}
      className={cn(
        "relative flex h-full shrink-0 items-center",
        column.id === "task" && "min-w-0 flex-1",
        end && "justify-end",
        isDragging && "z-4 rounded-sm bg-muted opacity-80",
        isOver && !isDragging && "rounded-sm ring-1 ring-primary ring-inset",
      )}
    >
      {editable ? (
        <button
          type="button"
          ref={setActivatorNodeRef}
          aria-label={`Move ${label} column`}
          className="min-w-0 cursor-grab touch-manipulation truncate select-none [-webkit-touch-callout:none]"
          {...listeners}
          {...attributes}
        >
          {label}
        </button>
      ) : (
        <span className="min-w-0 truncate">{label}</span>
      )}
      {editable && column.id !== "task" && column.width !== undefined && (
        <TaskColumnResizeHandle id={column.id} label={label} width={column.width} />
      )}
    </span>
  );
}
