import { ChevronLeft, ChevronRight, Minus, Plus, RotateCcw } from "lucide-react";
import { DropdownMenuItem, DropdownMenuSeparator } from "@/components/ui/dropdown-menu";
import {
  clampTaskColumnWidth,
  moveTaskColumn,
  resetTaskColumns,
  setTaskColumnWidth,
  TASK_COLUMN_LABELS,
  useTaskColumns,
} from "./taskColumnLayout";
import type { TaskRowMode } from "./taskColumns";

const WIDTH_STEP = 16;

// Each step is one tap on a full-size target, so a phone never has to hit the
// narrow edge of a header cell. Steps keep the menu open for the next one.
const keepOpen = (action: () => void) => (event: Event) => {
  event.preventDefault();
  action();
};

const stepClass = "size-7 justify-center rounded-control p-0";

/** The touch path to the column layout, as the items of a menu: order and
 *  width per column of the current table, and a reset. */
export default function TaskColumnsMenuItems({ mode }: { mode: TaskRowMode }) {
  const columns = useTaskColumns(mode);
  return (
    <>
      {columns.map((column, index) => {
        const label = TASK_COLUMN_LABELS[column.id];
        const id = column.id;
        const width = column.width;
        return (
          <div
            key={id}
            data-testid="task-column-menu-row"
            data-column={id}
            className="flex h-8 items-center gap-0.5 pl-2.25 text-body-sm"
          >
            <span className="min-w-0 flex-1 truncate">{label}</span>
            {id !== "task" && width !== undefined && (
              <>
                <span className="w-9 text-right text-meta text-muted-foreground tabular-nums">
                  {width}
                </span>
                <DropdownMenuItem
                  aria-label={`Narrow ${label}`}
                  disabled={width <= clampTaskColumnWidth(id, 0)}
                  onSelect={keepOpen(() => setTaskColumnWidth(id, width - WIDTH_STEP))}
                  className={stepClass}
                >
                  <Minus aria-hidden="true" className="size-3" />
                </DropdownMenuItem>
                <DropdownMenuItem
                  aria-label={`Widen ${label}`}
                  disabled={width >= clampTaskColumnWidth(id, Infinity)}
                  onSelect={keepOpen(() => setTaskColumnWidth(id, width + WIDTH_STEP))}
                  className={stepClass}
                >
                  <Plus aria-hidden="true" className="size-3" />
                </DropdownMenuItem>
              </>
            )}
            <DropdownMenuItem
              aria-label={`Move ${label} left`}
              disabled={index === 0}
              onSelect={keepOpen(() => moveTaskColumn(id, columns[index - 1].id))}
              className={stepClass}
            >
              <ChevronLeft aria-hidden="true" className="size-3.5" />
            </DropdownMenuItem>
            <DropdownMenuItem
              aria-label={`Move ${label} right`}
              disabled={index === columns.length - 1}
              onSelect={keepOpen(() => moveTaskColumn(id, columns[index + 1].id))}
              className={stepClass}
            >
              <ChevronRight aria-hidden="true" className="size-3.5" />
            </DropdownMenuItem>
          </div>
        );
      })}
      <DropdownMenuSeparator />
      <DropdownMenuItem
        onSelect={resetTaskColumns}
        className="h-7 gap-2 rounded-control px-2.25 text-body-sm"
      >
        <RotateCcw aria-hidden="true" className="size-3" />
        Reset columns
      </DropdownMenuItem>
    </>
  );
}
