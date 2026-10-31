import { useId, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import type { Task } from "@/lib/api";
import { formatTaskTime } from "@/pages/tasks/taskTime";
import { useWorkflowAdvance } from "@/pages/tasks/useWorkflowAdvance";
import { OutcomeNotice } from "@/pages/tasks/WorkflowOutcomes";
import { outcomeLabel, primaryOutcome, statusWords } from "./workModel";

/** One section of the Work tab: a heading with its count, then its rows. */
export function WorkSection({
  title,
  count,
  aside,
  children,
}: {
  title: string;
  count: number;
  aside?: ReactNode;
  children?: ReactNode;
}) {
  const id = useId();
  return (
    <section aria-labelledby={id} className="flex min-w-0 flex-col gap-1">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <h2
          id={id}
          className="text-meta font-medium tracking-[.02em] text-muted-foreground uppercase"
        >
          {title} ({count})
        </h2>
        {aside && (
          <div className="ml-auto flex w-96 max-w-full min-w-0 items-center gap-1.5">{aside}</div>
        )}
      </div>
      {children}
    </section>
  );
}

/** A task row: the key as a muted prefix, the title, the status in words, the
 *  age and the cost when known. A click opens the drawer. */
export function WorkRow({
  task,
  at,
  cost,
  onOpen,
  onChanged,
}: {
  task: Task;
  at: string;
  cost?: number;
  onOpen: () => void;
  onChanged: () => void;
}) {
  return (
    <li className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 rounded-control px-2 py-1 hover:bg-accent">
      <button
        type="button"
        className="flex min-w-0 flex-1 basis-60 items-baseline gap-2 text-left text-body-sm"
        onClick={onOpen}
      >
        <span className="shrink-0 font-mono text-caption text-muted-foreground">{task.key}</span>
        <span className="min-w-0 flex-1 truncate">{task.title}</span>
        <span className="shrink-0 text-caption text-muted-foreground">
          {statusWords(task)} · {formatTaskTime(at)}
          {cost ? ` · $${cost.toFixed(2)}` : ""}
        </span>
      </button>
      {task.workflow_initial && primaryOutcome(task) && (
        <QueuedOutcomes task={task} onChanged={onChanged} />
      )}
    </li>
  );
}

/** The outcomes of a task's initial status, the primary one first. */
function QueuedOutcomes({ task, onChanged }: { task: Task; onChanged: () => void }) {
  const { checking, notice, advance } = useWorkflowAdvance(task.key, task, onChanged, onChanged);
  const primary = primaryOutcome(task)!;
  const others = (task.workflow_outcomes ?? []).filter((outcome) => outcome.on !== primary.on);
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1">
      {[primary, ...others].map((outcome) => (
        <Button
          key={outcome.on}
          type="button"
          size="sm"
          variant={outcome === primary ? "default" : "ghost"}
          className="h-6 px-2 text-ui"
          disabled={checking}
          onClick={() => advance(outcome.on)}
        >
          {outcomeLabel(outcome)}
        </Button>
      ))}
      {notice && <OutcomeNotice notice={notice} onRefresh={onChanged} />}
    </div>
  );
}
