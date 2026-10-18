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
} from "@dnd-kit/core"
import { ChevronRight, GripVertical, Plus } from "lucide-react"
import { useState } from "react"
import { useMediaQuery } from "@/hooks/useMediaQuery"
import {
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { PriorityTag, StatusPill } from "@/components/ui/status"
import { cn } from "@/lib/utils"
import { taskTone } from "@/lib/statusTone"
import { isWorkflowTask, taskStatusLabel, type TaskPriority } from "@/lib/tasks"
import type { VisibleTaskRow } from "@/lib/taskTree"
import TaskColumnResizeHandle from "./TaskColumnResizeHandle"
import {
  moveTaskColumn,
  PHONE_TASK_QUERY,
  TASK_COLUMN_LABELS,
  useTaskColumns,
  type TaskColumn,
  type TaskColumnId,
} from "./taskColumnLayout"
import { INDENT_PX, MAX_INDENT_DEPTH, TASK_COLUMNS, type TaskRowMode } from "./taskColumns"
import { formatTaskDuration, formatTaskTime } from "./taskTime"

const priorityLabels = {
  P0: "Critical",
  P1: "High",
  P2: "Normal",
  P3: "Low",
} as const

/* @dnd-kit intentionally exposes callback refs and live attributes from its
 * hook for render-time spreading; they are not mutable React ref reads. */
/* eslint-disable react-hooks/refs */
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
  row: VisibleTaskRow
  mode: TaskRowMode
  /** The server the task lives on; shown only in the `servers` table. */
  server?: string
  hasActiveQuestion: boolean
  expanded: boolean
  selected: boolean
  onToggle: () => void
  onSelect: () => void
  onAddChild?: () => void
  /** Saves a priority picked in the row's menu; without it the value is a
   *  plain tag. The menu stays disabled until the returned promise settles. */
  onPriorityChange?: (priority: TaskPriority) => Promise<void> | void
}) {
  const readOnly = row.task.access === "context" || row.task.access === "respond"
  // The every-server table has no drag and no add-child: neither can cross the
  // server boundary the rows sit on.
  const disabled = readOnly || mode === "servers"
  const drag = useDraggable({ id: row.task.key, disabled })
  const dropBefore = useDroppable({ id: `before:${row.task.key}`, disabled })
  const dropInside = useDroppable({ id: `inside:${row.task.key}`, disabled })
  const dropAfter = useDroppable({ id: `after:${row.task.key}`, disabled })
  const tone = taskTone(row.task.category)
  const workflow = isWorkflowTask(row.task)
  const columns = useTaskColumns(mode)
  return (
    <div
      ref={drag.setNodeRef}
      data-testid={`task-row-${row.task.key}`}
      style={{ opacity: drag.isDragging ? 0.45 : undefined }}
      // A mouse or finger opens the task anywhere on the row; the keyboard keeps
      // the title button. The row's own buttons, and clicks React bubbles up
      // from the priority menu's portal, keep their own action.
      onClick={(event) => {
        const target = event.target as Element
        if (!event.currentTarget.contains(target) || target.closest("button, a, input")) return
        onSelect()
      }}
      className={cn(
        "group/row relative flex min-h-[30px] cursor-pointer items-center gap-2.5 px-4 text-[13px]",
        selected ? "bg-accent" : "hover:bg-muted",
        dropInside.isOver && "ring-1 ring-primary ring-inset",
        row.task.access === "context" && "opacity-60",
      )}
    >
      <span
        ref={dropBefore.setNodeRef}
        data-testid={`drop-before-${row.task.key}`}
        className={`task-drop-zone before ${dropBefore.isOver ? "is-over" : ""}`}
      />
      <span
        ref={dropInside.setNodeRef}
        data-testid={`drop-inside-${row.task.key}`}
        className="task-drop-zone inside"
      />
      <span
        ref={dropAfter.setNodeRef}
        data-testid={`drop-after-${row.task.key}`}
        className={`task-drop-zone after ${dropAfter.isOver ? "is-over" : ""}`}
      />
      {/* The grip lives in the row's own left padding: the reference table has
          no column for it, and it is only there under the pointer anyway. */}
      {mode !== "servers" && (
        <button
          type="button"
          ref={drag.setActivatorNodeRef}
          className="task-row-grip absolute top-1/2 left-0 z-2 grid h-6 w-4 -translate-y-1/2 cursor-grab place-items-center text-muted-foreground opacity-0 group-hover/row:opacity-60"
          aria-label={`Move ${row.task.key}`}
          {...drag.listeners}
          {...drag.attributes}
        >
          <GripVertical aria-hidden="true" className="size-3" />
        </button>
      )}
      {columns.map((column) => {
        const style = { width: column.width }
        switch (column.id) {
          // Priority leads every mode by default, unlike the reference, which
          // drops it from the all-tasks table: in this app priority is what
          // orders the tree, so a table without it cannot be read.
          case "priority":
            return (
              <span key="priority" data-column="priority" style={style} className="z-2 shrink-0">
                {readOnly || !onPriorityChange
                  ? <PriorityTag className="task-priority" priority={row.task.priority} aria-label={priorityName(row.task.priority)} />
                  : <PriorityMenu priority={row.task.priority} onChange={onPriorityChange} />}
              </span>
            )
          case "key":
            return (
              <span
                key="key"
                data-column="key"
                style={style}
                className="z-2 flex shrink-0 items-center gap-1.5 overflow-hidden"
              >
                {Array.from({ length: Math.min(row.depth, MAX_INDENT_DEPTH) }, (_, level) => (
                  <span key={level} aria-hidden="true" className="shrink-0" style={{ width: INDENT_PX }} />
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
                    className={cn("size-2.5 transition-transform duration-[120ms]", expanded && "rotate-90")}
                  />
                </button>
                <span className="min-w-0 truncate font-mono text-[11.5px] text-muted-foreground tabular-nums">
                  {row.task.key}
                </span>
              </span>
            )
          case "task":
            return (
              <button
                key="task"
                type="button"
                data-column="task"
                className="task-row-main relative z-2 flex min-w-0 flex-1 items-center gap-[7px] overflow-hidden text-left"
                onClick={onSelect}
              >
                <span className="min-w-0 truncate">{row.task.title}</span>
                {hasActiveQuestion && (
                  <span
                    className="size-[5px] shrink-0 rounded-full bg-primary"
                    role="img"
                    aria-label={`Unread question notification for ${row.task.key}`}
                  />
                )}
                {row.task.blocked && <StatusPill tone="danger" size="sm">blocked</StatusPill>}
              </button>
            )
          case "agent":
            return (
              <span key="agent" data-column="agent" style={style} className="shrink-0">
                {row.task.assignee && (
                  <span className="inline-flex h-[19px] max-w-full items-center truncate rounded-[6px] bg-muted px-[7px] text-[11.5px]">
                    {row.task.assignee.replace(/^agent:/, "")}
                  </span>
                )}
              </span>
            )
          case "server":
            return (
              <span key="server" data-column="server" style={style} className="shrink-0 truncate font-mono text-[11.5px] text-muted-foreground">
                {server}
              </span>
            )
          case "status":
            return workflow ? (
              <span key="status" data-column="status" style={style} className="flex min-w-0 shrink-0 items-center gap-1 overflow-hidden">
                <StatusPill tone={tone} className="min-w-0 shrink">
                  {/* A workflow status ID can be 64 characters: CSS clips it, the
                      title keeps the whole ID readable. */}
                  <span title={row.task.status} className="min-w-0 max-w-[24ch] truncate">{taskStatusLabel(row.task)}</span>
                </StatusPill>
                {row.task.waiting_on === "pause" ? (
                  <StatusPill tone="danger" size="sm" title={row.task.workflow_paused_reason || undefined}>paused</StatusPill>
                ) : row.task.waiting_on ? (
                  <StatusPill tone="quiet" size="sm">{row.task.waiting_on}</StatusPill>
                ) : null}
              </span>
            ) : (
              <span key="status" data-column="status" style={style} className="shrink-0">
                <StatusPill tone={tone}>{taskStatusLabel(row.task)}</StatusPill>
              </span>
            )
          case "duration":
            return (
              <span
                key="duration"
                data-column="duration"
                style={style}
                className="shrink-0 truncate text-right font-mono text-[11.5px] text-muted-foreground tabular-nums"
              >
                {formatTaskDuration(row.task)}
              </span>
            )
          case "updated":
            return (
              <span
                key="updated"
                data-column="updated"
                style={style}
                className="shrink-0 truncate text-right text-[12px] text-muted-foreground tabular-nums"
              >
                {formatTaskTime(row.task.updated_at)}
              </span>
            )
        }
      })}
      {mode !== "servers" && <span style={{ width: TASK_COLUMNS.add }} className="shrink-0">
        {!disabled && (
          <button
            type="button"
            className="relative z-2 grid size-6 place-items-center rounded-[7px] text-muted-foreground opacity-0 hover:bg-accent hover:text-foreground group-hover/row:opacity-100"
            aria-label={`Add child to ${row.task.key}`}
            onClick={onAddChild}
          >
            <Plus aria-hidden="true" className="size-3.5" />
          </button>
        )}
      </span>}
    </div>
  )
}

const priorityName = (priority: TaskPriority) => `${priority} ${priorityLabels[priority]}`

function PriorityMenu({ priority, onChange }: {
  priority: TaskPriority
  onChange: (priority: TaskPriority) => Promise<void> | void
}) {
  const [saving, setSaving] = useState(false)
  const pick = async (next: TaskPriority) => {
    if (next === priority) return
    setSaving(true)
    try {
      await onChange(next)
    } finally {
      setSaving(false)
    }
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={saving}>
        <button type="button" className="task-priority flex rounded-[6px] disabled:opacity-50" aria-label={priorityName(priority)}>
          <PriorityTag priority={priority} aria-hidden="true" className="cursor-pointer" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {(Object.keys(priorityLabels) as TaskPriority[]).map((value) => (
          <DropdownMenuCheckItem key={value} checked={value === priority} onSelect={() => void pick(value)}>
            <span className="font-mono">{value}</span>
            <span className="text-muted-foreground">{priorityLabels[value]}</span>
          </DropdownMenuCheckItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** A header cell's label is its drag grip; a mouse moves it after 5px, a
 *  finger after a 250ms press, so scrolling the table never starts a move. */
const headerSensorOptions = {
  mouse: { activationConstraint: { distance: 5 } },
  touch: { activationConstraint: { delay: 250, tolerance: 5 } },
}

const horizontalOnly: Modifier = ({ transform }) => ({ ...transform, y: 0 })

/** The table header. Same widths, same order, same gaps as the rows under it:
 *  both read the one column layout. Its own drag context keeps a column move
 *  apart from the row drags under it. */
export function TaskTableHeader({ mode, sticky = false }: { mode: TaskRowMode; sticky?: boolean }) {
  const columns = useTaskColumns(mode)
  // A phone shows fixed columns, so moving or resizing one there would only
  // rewrite the layout the wider screens use.
  const editable = !useMediaQuery(PHONE_TASK_QUERY)
  const sensors = useSensors(
    useSensor(MouseSensor, headerSensorOptions.mouse),
    useSensor(TouchSensor, headerSensorOptions.touch),
    useSensor(KeyboardSensor),
  )
  const finish = ({ active, over }: DragEndEvent) => {
    if (over && active.id !== over.id) moveTaskColumn(active.id as TaskColumnId, over.id as TaskColumnId)
  }
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} modifiers={[horizontalOnly]} onDragEnd={finish}>
      <div
        data-testid="task-table-header"
        className={cn(
          "flex h-[30px] items-center gap-2.5 px-4 text-[11.5px] text-muted-foreground",
          sticky && "sticky top-0 z-2 bg-card",
        )}
      >
        {columns.map((column) => <TaskHeaderCell key={column.id} column={column} editable={editable} />)}
        {mode !== "servers" && <span aria-hidden="true" style={{ width: TASK_COLUMNS.add }} className="shrink-0" />}
      </div>
    </DndContext>
  )
}

function TaskHeaderCell({ column, editable }: { column: TaskColumn; editable: boolean }) {
  const drag = useDraggable({ id: column.id, disabled: !editable })
  const drop = useDroppable({ id: column.id })
  const label = TASK_COLUMN_LABELS[column.id]
  const end = column.id === "duration" || column.id === "updated"
  return (
    <span
      ref={(node) => {
        drag.setNodeRef(node)
        drop.setNodeRef(node)
      }}
      data-column={column.id}
      style={{
        width: column.width,
        transform: drag.transform ? `translate3d(${drag.transform.x}px, 0, 0)` : undefined,
      }}
      className={cn(
        "relative flex h-full shrink-0 items-center",
        column.id === "task" && "min-w-0 flex-1",
        end && "justify-end",
        drag.isDragging && "z-4 rounded-[6px] bg-muted opacity-80",
        drop.isOver && !drag.isDragging && "rounded-[6px] ring-1 ring-primary ring-inset",
      )}
    >
      {editable ? (
        <button
          type="button"
          ref={drag.setActivatorNodeRef}
          aria-label={`Move ${label} column`}
          className="task-column-grip min-w-0 cursor-grab truncate"
          {...drag.listeners}
          {...drag.attributes}
        >
          {label}
        </button>
      ) : <span className="min-w-0 truncate">{label}</span>}
      {editable && column.id !== "task" && column.width !== undefined && (
        <TaskColumnResizeHandle id={column.id} label={label} width={column.width} />
      )}
    </span>
  )
}
