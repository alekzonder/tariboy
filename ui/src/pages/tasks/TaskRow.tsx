import { useDraggable, useDroppable } from "@dnd-kit/core"
import { ChevronDown, ChevronRight, Circle, CircleCheck, GripVertical, Plus } from "lucide-react"
import type { VisibleTaskRow } from "@/lib/taskTree"

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
  hasActiveQuestion,
  expanded,
  selected,
  onToggle,
  onSelect,
  onAddChild,
}: {
  row: VisibleTaskRow
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
  const style = {
    paddingLeft: `${row.depth * 22 + 8}px`,
    opacity: drag.isDragging ? 0.45 : 1,
  }
  const setNodeRef = (node: HTMLDivElement | null) => {
    drag.setNodeRef(node)
  }
  return (
    <div
      ref={setNodeRef}
      data-testid={`task-row-${row.task.key}`}
      className={[
        "task-tree-row",
        selected ? "is-selected" : "",
        dropInside.isOver ? "is-drop-target" : "",
        row.task.access === "context" ? "is-context" : "",
      ].join(" ")}
      style={style}
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
      <button
        type="button"
        className="task-expand"
        aria-label={`${expanded ? "Collapse" : "Expand"} ${row.task.key}`}
        disabled={!row.hasChildren}
        onClick={onToggle}
      >
        {row.hasChildren
          ? expanded ? <ChevronDown /> : <ChevronRight />
          : <span aria-hidden="true" />}
      </button>
      <button
        type="button"
        ref={drag.setActivatorNodeRef}
        className="task-grip"
        aria-label={`Move ${row.task.key}`}
        {...drag.listeners}
        {...drag.attributes}
      >
        <GripVertical aria-hidden="true" />
      </button>
      {row.task.status === "done" ? <CircleCheck className="task-status done" /> : <Circle className="task-status" />}
      <span
        className={`task-priority priority-${row.task.priority.toLowerCase()}`}
        aria-label={`${row.task.priority} ${priorityLabels[row.task.priority]}`}
      >
        {row.task.priority}
      </span>
      <button type="button" className="task-row-main" onClick={onSelect}>
        <span className="task-key">{row.task.key}</span>
        <span className="task-title">{row.task.title}</span>
        {hasActiveQuestion && (
          <span className="task-question-indicator" role="img" aria-label={`Unread question notification for ${row.task.key}`} />
        )}
        {row.task.status === "in_progress" && <span className="task-badge in-progress">In progress</span>}
        {row.task.blocked && <span className="task-badge blocked">blocked</span>}
        {row.task.assignee && <span className="task-assignee">{row.task.assignee.replace(/^agent:/, "")}</span>}
      </button>
      {!disabled && (
        <button type="button" className="task-add-child" aria-label={`Add child to ${row.task.key}`} onClick={onAddChild}>
          <Plus />
        </button>
      )}
    </div>
  )
}
