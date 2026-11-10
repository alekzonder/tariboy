import { cn } from "@/lib/utils";
import { EMPTY, LABEL, MONO, ROW } from "@/pages/tasks/panelStyles";
import type { Relation, Task } from "@/types";

export interface TaskDetailsPanelProps {
  task: Task;
  relations: readonly Relation[];
  onOpenTask: (key: string) => void;
}

function Pair({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-baseline gap-2.5">
      <dt className={cn(LABEL, "w-19 shrink-0 font-normal")}>{label}</dt>
      <dd className={cn(MONO, "min-w-0 truncate")}>{value || "—"}</dd>
    </div>
  );
}

/**
 * The open Details block of the drawer header, under the chips: the fields
 * nobody changes day to day and the relations of the task.
 */
export function TaskDetailsPanel({ task, relations, onOpenTask }: TaskDetailsPanelProps) {
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-md bg-muted/60 px-2.5 py-2">
      <dl className="grid grid-cols-1 gap-x-6 gap-y-1.5 @md:grid-cols-2">
        <Pair label="Author" value={task.author} />
        <Pair label="Customer" value={task.customer} />
        <Pair label="Queue" value={task.queue} />
        <Pair label="Group" value={task.group} />
        <Pair label="Parent" value={task.parent_key} />
        <Pair label="Block reason" value={task.manual_block_reason} />
      </dl>
      <div className="flex min-w-0 flex-col">
        <span className={LABEL}>Relations</span>
        {relations.map((relation) => (
          <button
            key={relation.id}
            type="button"
            onClick={() => onOpenTask(relation.target_key)}
            className={cn(
              ROW,
              "text-left outline-none focus-visible:ring-2 focus-visible:ring-ring",
            )}
          >
            <span className={cn(MONO, "shrink-0 text-muted-foreground")}>{relation.type}</span>
            <span className={cn(MONO, "shrink-0")}>{relation.target_key}</span>
            <span className="min-w-0 flex-1 truncate text-ui">{relation.target_title}</span>
          </button>
        ))}
        {relations.length === 0 && <span className={EMPTY}>No relations.</span>}
      </div>
    </div>
  );
}
