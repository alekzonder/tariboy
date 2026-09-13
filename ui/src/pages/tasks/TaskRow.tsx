import { useDraggable, useDroppable } from "@dnd-kit/core"
import { ChevronRight, GripVertical, Plus } from "lucide-react"
import { PriorityTag, StatusPill } from "@/components/ui/status"
import { cn } from "@/lib/utils"
import { taskStatusLabel, taskTone } from "@/lib/statusTone"
import type { VisibleTaskRow } from "@/lib/taskTree"
import { formatTaskTime } from "./taskTime"

const priorityLabels = {
  P0: "Critical",
  P1: "High",
  P2: "Normal",
  P3: "Low",
} as const

/** Which columns a row carries. `agent` is one agent's tasks (priority and the
 *  work itself matter); `all` is a whole server's (who owns it and where it
 *  queues matter). The table header renders the same two shapes. */
export type TaskRowMode = "agent" | "all"

/** One indent step per level of nesting, capped so a deep tree still leaves
 *  room for the title. */
export const INDENT_PX = 14
export const MAX_INDENT_DEPTH = 6

/* @dnd-kit intentionally exposes callback refs and live attributes from its
 * hook for render-time spreading; they are not mutable React ref reads. */
/* eslint-disable react-hooks/refs */
export default function TaskRow({
  row,
  mode,
  hasActiveQuestion,
  expanded,
  selected,
  onToggle,
  onSelect,
  onAddChild,
}: {
  row: VisibleTaskRow
  mode: TaskRowMode
  hasActiveQuestion: boolean
  expanded: boolean
  selected: boolean
  onToggle: () => void
  onSelect: () => void
  onAddChild: () => void
}) {
  const disabled = row.task.access === "context" || row.task.access === "respond"
  const drag = useDraggable({ id: row.task.key, disabled })
  const dropBefore = useDroppable({ id: `before:${row.task.key}`, disabled })
  const dropInside = useDroppable({ id: `inside:${row.task.key}`, disabled })
  const dropAfter = useDroppable({ id: `after:${row.task.key}`, disabled })
  const tone = taskTone(row.task.status)
  return (
    <div
      ref={drag.setNodeRef}
      data-testid={`task-row-${row.task.key}`}
      style={{ opacity: drag.isDragging ? 0.45 : undefined }}
      className={cn(
        "group/row relative flex min-h-[30px] items-center gap-2.5 px-4 text-[13px]",
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
      {Array.from({ length: Math.min(row.depth, MAX_INDENT_DEPTH) }, (_, level) => (
        <span key={level} aria-hidden="true" className="shrink-0" style={{ width: INDENT_PX }} />
      ))}
      <button
        type="button"
        className="relative z-2 grid size-4 shrink-0 place-items-center text-muted-foreground opacity-65 disabled:opacity-0"
        aria-label={`${expanded ? "Collapse" : "Expand"} ${row.task.key}`}
        disabled={!row.hasChildren}
        onClick={onToggle}
      >
        <ChevronRight
          aria-hidden="true"
          className={cn("size-2.5 transition-transform duration-[120ms]", expanded && "rotate-90")}
        />
      </button>
      <button
        type="button"
        ref={drag.setActivatorNodeRef}
        className="relative z-2 grid h-6 w-[18px] shrink-0 cursor-grab place-items-center text-muted-foreground opacity-0 group-hover/row:opacity-60"
        aria-label={`Move ${row.task.key}`}
        {...drag.listeners}
        {...drag.attributes}
      >
        <GripVertical aria-hidden="true" className="size-3" />
      </button>
      <button type="button" className="task-row-main relative z-2 flex min-w-0 flex-1 items-center gap-2.5 text-left" onClick={onSelect}>
        <span className="w-[62px] shrink-0 font-mono text-[11.5px] text-muted-foreground tabular-nums">
          {row.task.key}
        </span>
        <span className="flex min-w-0 flex-1 items-center gap-[7px] overflow-hidden">
          <span className="min-w-0 truncate">{row.task.title}</span>
          {hasActiveQuestion && (
            <span
              className="size-[5px] shrink-0 rounded-full bg-primary"
              role="img"
              aria-label={`Unread question notification for ${row.task.key}`}
            />
          )}
          {row.task.blocked && <StatusPill tone="danger" size="sm">blocked</StatusPill>}
        </span>
      </button>
      {/* Priority stays in both modes, unlike the reference, which drops it from
          the all-tasks table: in this app priority is what orders the tree, so
          a table without it cannot be read. */}
      <span className="w-[30px] shrink-0">
        <PriorityTag
          className="task-priority"
          priority={row.task.priority}
          aria-label={`${row.task.priority} ${priorityLabels[row.task.priority]}`}
        />
      </span>
      {mode === "all" && (
        <>
          <span className="w-[104px] shrink-0">
            {row.task.assignee && (
              <span className="inline-flex h-[19px] max-w-full items-center truncate rounded-[6px] bg-muted px-[7px] text-[11.5px]">
                {row.task.assignee.replace(/^agent:/, "")}
              </span>
            )}
          </span>
          <span className="w-24 shrink-0 truncate font-mono text-[11.5px] text-muted-foreground">
            {row.task.queue}
          </span>
        </>
      )}
      <span className="w-[110px] shrink-0">
        <StatusPill tone={tone}>{taskStatusLabel(row.task.status)}</StatusPill>
      </span>
      <span className="w-[76px] shrink-0 text-right text-[12px] text-muted-foreground tabular-nums">
        {formatTaskTime(row.task.updated_at)}
      </span>
      <span className="w-6 shrink-0">
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
      </span>
    </div>
  )
}

/** The table header. Same columns, same widths, same gaps as the rows above —
 *  including the spacers standing in for the chevron, the drag grip and the
 *  add-child button, so a column never drifts from its heading. */
export function TaskTableHeader({ mode, sticky = false }: { mode: TaskRowMode; sticky?: boolean }) {
  return (
    <div
      className={cn(
        "flex h-[30px] items-center gap-2.5 px-4 text-[11.5px] text-muted-foreground",
        sticky && "sticky top-0 z-2 bg-card",
      )}
    >
      <span aria-hidden="true" className="w-4 shrink-0" />
      <span aria-hidden="true" className="w-[18px] shrink-0" />
      <span className="w-[62px] shrink-0">Key</span>
      <span className="min-w-0 flex-1">Task</span>
      <span className="w-[30px] shrink-0">Pri</span>
      {mode === "all" && (
        <>
          <span className="w-[104px] shrink-0">Agent</span>
          <span className="w-24 shrink-0">Queue</span>
        </>
      )}
      <span className="w-[110px] shrink-0">Status</span>
      <span className="w-[76px] shrink-0 text-right">Updated</span>
      <span aria-hidden="true" className="w-6 shrink-0" />
    </div>
  )
}
