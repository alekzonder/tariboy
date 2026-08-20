import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import { buildTaskForest, canDropTaskInside, canReorderTaskBeside, flattenVisible } from "@/lib/taskTree"
import type { Task } from "@/lib/tasks"
import TaskRow from "./TaskRow"

function preferSpecificDropTarget(args: Parameters<typeof pointerWithin>[0]) {
  const collisions = pointerWithin(args)
  const specific = collisions.filter((collision) => collision.id !== "__root__")
  return specific.length > 0 ? specific : collisions
}

/* @dnd-kit exposes callback refs and live `isOver` render state from hooks;
 * these are its documented component integration, not user-managed refs. */
/* eslint-disable react-hooks/refs */
function RootDrop({ children }: { children: React.ReactNode }) {
  const root = useDroppable({ id: "__root__" })
  return (
    <div
      ref={root.setNodeRef}
      data-testid="tasks-tree-root"
      className={`task-tree-list ${root.isOver ? "is-drop-target" : ""}`}
    >
      {children}
    </div>
  )
}

export default function TaskTree({
  tasks,
  activeQuestionTaskKeys,
  expanded,
  selectedKey,
  onToggle,
  onSelect,
  onAddChild,
  onMove,
}: {
  tasks: Task[]
  activeQuestionTaskKeys: ReadonlySet<string>
  expanded: ReadonlySet<string>
  selectedKey: string
  onToggle: (key: string) => void
  onSelect: (key: string) => void
  onAddChild: (key: string) => void
  onMove: (key: string, parentKey: string, beforeKey: string) => void
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor),
  )
  const forest = buildTaskForest(tasks)
  const visible = flattenVisible(forest, expanded)
  const byKey = new Map(tasks.map((task) => [task.key, task]))

  const finish = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const key = String(active.id)
    const destination = String(over.id)
    if (destination === "__root__") {
      onMove(key, "", "")
      return
    }
    const [placement, targetKey] = destination.split(":", 2)
    if (targetKey === key) return
    const target = byKey.get(targetKey)
    if (!target) return
    if ((placement === "before" || placement === "after") && !canReorderTaskBeside(byKey.get(key), target)) return
    let parentKey = targetKey
    let beforeKey = ""
    if (placement === "before" || placement === "after") {
      parentKey = target.parent_key
      beforeKey = targetKey
      if (placement === "after") {
        const siblings = tasks
          .filter((task) => task.parent_key === parentKey && task.key !== key)
          .sort((left, right) => left.position - right.position)
        beforeKey = siblings[siblings.findIndex((task) => task.key === targetKey) + 1]?.key ?? ""
      }
    }
    const moving = byKey.get(key)
    const parent = parentKey ? byKey.get(parentKey) : undefined
    if (!moving || (parent && parent.queue !== moving.queue)) return
    if (parentKey && !canDropTaskInside(forest, key, parentKey)) return
    onMove(key, parentKey, beforeKey)
  }

  return (
    <DndContext sensors={sensors} collisionDetection={preferSpecificDropTarget} onDragEnd={finish}>
      <RootDrop>
        {visible.map((row) => (
          <TaskRow
            key={row.task.key}
            row={row}
            hasActiveQuestion={activeQuestionTaskKeys.has(row.task.key)}
            expanded={expanded.has(row.task.key)}
            selected={selectedKey === row.task.key}
            onToggle={() => onToggle(row.task.key)}
            onSelect={() => onSelect(row.task.key)}
            onAddChild={() => onAddChild(row.task.key)}
          />
        ))}
        {visible.length === 0 && <div className="tasks-empty">No tasks match this view.</div>}
      </RootDrop>
    </DndContext>
  )
}
