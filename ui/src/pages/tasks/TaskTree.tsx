import { cn } from "@/lib/utils";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { buildTaskForest, flattenVisible, resolveTaskDrop } from "@/lib/taskTree";
import type { Task, TaskPriority } from "@/lib/api";
import type { TaskRowMode } from "./taskColumns";
import TaskRow from "./TaskRow";
import TaskTableHeader from "./TaskTableHeader";

function preferSpecificDropTarget(args: Parameters<typeof pointerWithin>[0]) {
  const collisions = pointerWithin(args);
  const specific = collisions.filter((collision) => collision.id !== "__root__");
  return specific.length > 0 ? specific : collisions;
}

function RootDrop({ children }: { children: React.ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: "__root__" });
  return (
    <div
      ref={setNodeRef}
      data-testid="tasks-tree-root"
      className={cn(
        "min-h-[calc(100%-60px)] pb-20",
        isOver && "shadow-[inset_0_0_0_2px_color-mix(in_oklch,var(--primary)_30%,transparent)]",
      )}
    >
      {children}
    </div>
  );
}

export default function TaskTree({
  tasks,
  mode,
  activeQuestionTaskKeys,
  expanded,
  selectedKey,
  onToggle,
  onSelect,
  onAddChild,
  onMove,
  onPriorityChange,
}: {
  tasks: Task[];
  mode: TaskRowMode;
  activeQuestionTaskKeys: ReadonlySet<string>;
  expanded: ReadonlySet<string>;
  selectedKey: string;
  onToggle: (key: string) => void;
  onSelect: (key: string) => void;
  onAddChild: (key: string) => void;
  onMove: (key: string, parentKey: string, beforeKey: string) => void;
  onPriorityChange: (key: string, priority: TaskPriority) => Promise<void>;
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor),
  );
  const forest = buildTaskForest(tasks);
  const visible = flattenVisible(forest, expanded);

  const finish = ({ active, over }: DragEndEvent) => {
    if (!over) return;
    const drop = resolveTaskDrop({
      tasks,
      forest,
      activeKey: String(active.id),
      overId: String(over.id),
    });
    if (drop) onMove(drop.key, drop.parentKey, drop.beforeKey);
  };

  // The header moves columns in its own drag context, outside the rows' one.
  return (
    <>
      <TaskTableHeader mode={mode} sticky={mode === "all"} />
      <DndContext
        sensors={sensors}
        collisionDetection={preferSpecificDropTarget}
        onDragEnd={finish}
      >
        <RootDrop>
          {visible.map((row) => (
            <TaskRow
              key={row.task.key}
              row={row}
              mode={mode}
              hasActiveQuestion={activeQuestionTaskKeys.has(row.task.key)}
              expanded={expanded.has(row.task.key)}
              selected={selectedKey === row.task.key}
              onToggle={() => onToggle(row.task.key)}
              onSelect={() => onSelect(row.task.key)}
              onAddChild={() => onAddChild(row.task.key)}
              onPriorityChange={(priority) => onPriorityChange(row.task.key, priority)}
            />
          ))}
          {visible.length === 0 && (
            <div className="px-5 py-10 text-center text-body text-muted-foreground">
              No tasks match this view.
            </div>
          )}
        </RootDrop>
      </DndContext>
    </>
  );
}
