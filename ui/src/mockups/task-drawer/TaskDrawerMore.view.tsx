import type { ReactNode } from "react";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { COUNT, EMPTY, LABEL, MONO, ROW } from "@/pages/tasks/panelStyles";
import { formatTaskTime } from "@/pages/tasks/taskTime";
import type { TaskEvent, WorkflowView } from "@/types";

export interface TaskDrawerMoreProps {
  view?: WorkflowView;
  events: TaskEvent[];
  /** `$4.20 · 6 runs · 1 rework`, when the metrics are known. */
  metrics?: string;
}

/** One closed line of the log: a label, a count or a summary, and the rows. */
function Row({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <details className="group min-w-0">
      <summary className="-mx-1.5 flex min-h-7.5 cursor-pointer list-none items-center gap-1.5 rounded-md px-2 select-none hover:bg-muted [&::-webkit-details-marker]:hidden">
        <ChevronRight
          aria-hidden="true"
          className="size-3 shrink-0 text-muted-foreground group-open:rotate-90"
        />
        <span className="text-ui">{label}</span>
        {hint !== undefined && <span className={cn(COUNT, "truncate")}>{hint}</span>}
      </summary>
      <div className="flex min-w-0 flex-col gap-0.5 pt-1 pb-2 pl-4.5">{children}</div>
    </details>
  );
}

/**
 * What is read rarely, closed at the bottom of the drawer: the log of the
 * workflow and of the task, and the metrics. Each line opens on its own.
 */
export function TaskDrawerMore({ view, events, metrics }: TaskDrawerMoreProps) {
  return (
    <section className="flex min-w-0 flex-col">
      <span className={cn(LABEL, "pb-1")}>More</span>
      {view && (
        <>
          <Row label="Script runs" hint={view.runs.length}>
            {view.runs.map((run) => (
              <div key={run.id} className={ROW}>
                <span className={cn(MONO, "shrink-0 text-muted-foreground")}>{run.kind}</span>
                <span className={cn(MONO, "min-w-0 flex-1 truncate")}>{run.script}</span>
                <span className={cn(MONO, "shrink-0 text-muted-foreground")}>
                  {run.verdict ?? run.state}
                </span>
              </div>
            ))}
            {view.runs.length === 0 && <span className={EMPTY}>No script ran yet.</span>}
          </Row>
          <Row label="Status visits" hint={view.visits.length}>
            {view.visits.map((visit) => (
              <div key={visit.id} className={ROW}>
                <span className={cn(MONO, "shrink-0 font-medium")}>{visit.status}</span>
                <span className={cn(MONO, "min-w-0 flex-1 truncate text-muted-foreground")}>
                  {formatTaskTime(visit.entered_at)} · {visit.entered_by}
                </span>
                <span className="shrink-0 text-ui text-muted-foreground">
                  {visit.left_at ? `left by ${visit.outcome ?? "a move"}` : "current"}
                </span>
              </div>
            ))}
          </Row>
        </>
      )}
      <Row label="History" hint={events.length}>
        {events.map((event) => (
          <div key={event.event_id} className={ROW}>
            <time className={cn(MONO, "w-13 shrink-0 text-muted-foreground")}>
              {formatTaskTime(event.created_at)}
            </time>
            <span className={cn(MONO, "min-w-0 flex-1 truncate font-medium")}>{event.kind}</span>
            <span className={cn(MONO, "shrink-0 text-muted-foreground")}>{event.actor}</span>
          </div>
        ))}
      </Row>
      <Row label="Metrics" hint={metrics}>
        <span className={EMPTY}>The metrics tables load here, as they do now.</span>
      </Row>
    </section>
  );
}
