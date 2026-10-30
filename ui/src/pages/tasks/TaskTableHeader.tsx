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
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { cn } from "@/lib/utils";
import TaskColumnResizeHandle from "./TaskColumnResizeHandle";
import {
  moveTaskColumn,
  PHONE_TASK_QUERY,
  TASK_COLUMN_LABELS,
  useTaskColumns,
  type TaskColumn,
  type TaskColumnId,
} from "./taskColumnLayout";
import { TASK_COLUMNS, type TaskRowMode } from "./taskColumns";

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
export default function TaskTableHeader({
  mode,
  sticky = false,
}: {
  mode: TaskRowMode;
  sticky?: boolean;
}) {
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
        <span aria-hidden="true" style={{ width: TASK_COLUMNS.add }} className="shrink-0" />
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
