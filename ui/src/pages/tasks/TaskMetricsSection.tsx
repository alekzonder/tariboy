import { lazy, useId, useState } from "react";
import { ChevronRight, RefreshCw } from "lucide-react";
import { LazyBoundary } from "@/components/LazyBoundary";
import { Button } from "@/components/ui/button";
import { useTaskMetrics } from "@/hooks/useTaskMetrics";
import type { Task } from "@/lib/api";
import { fmtMoney } from "@/lib/metricsFormat";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { cn } from "@/lib/utils";
import { COUNT, LABEL, QUIET_ACTION } from "./panelStyles";

// The numbers, tables and states load with the first open, not with the task.
const TaskMetricsBody = lazy(() => import("./TaskMetricsBody"));

/** The collapsed Metrics section of a task. It asks for the task's metrics
 *  only while open; an open section loads them on every open of the task. */
export function TaskMetricsSection({ task }: { task: Task }) {
  const [open, setOpen] = useState(() => readKey(storageKeys.taskMetricsOpen) === "open");
  const state = useTaskMetrics(task.key, open);
  const bodyId = useId();
  const { metrics } = state;

  const toggle = () => {
    const next = !open;
    writeKey(storageKeys.taskMetricsOpen, next ? "open" : "closed");
    setOpen(next);
    // A reopened section loads again rather than show the numbers of an hour ago.
    if (next) state.retry();
  };

  return (
    <section className="flex min-w-0 flex-col gap-1">
      <div className="flex min-w-0 items-center gap-1.5">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={toggle}
          className={cn(LABEL, "flex min-w-0 items-center gap-1 py-1 text-left")}
        >
          <ChevronRight
            aria-hidden="true"
            className={cn("size-3.5 shrink-0 transition-transform", open && "rotate-90")}
          />
          Metrics
          {metrics && (
            <span className={cn(COUNT, "truncate")}>
              {" "}
              {fmtMoney(metrics.usage.cost_usd)} · {metrics.usage.iterations} runs ·{" "}
              {metrics.rework_rounds} rework
            </span>
          )}
        </button>
        {open && (
          <Button
            type="button"
            variant="ghost"
            className={cn(QUIET_ACTION, "ml-auto")}
            aria-label="Refresh metrics"
            title="Refresh metrics"
            onClick={state.retry}
          >
            <RefreshCw aria-hidden="true" className="size-3.5" />
          </Button>
        )}
      </div>
      {open && (
        <section
          id={bodyId}
          aria-label="Task metrics"
          aria-busy={state.loading || state.updating}
          className="min-w-0"
        >
          <LazyBoundary resetKey="task-metrics">
            <TaskMetricsBody task={task} state={state} />
          </LazyBoundary>
        </section>
      )}
    </section>
  );
}
