import type { StatusVisit, WorkflowView } from "@/types";
import { cn } from "@/lib/utils";
import { LABEL, MONO, ROW } from "./panelStyles";
import { formatTaskTime } from "./taskTime";

/** A mono value that may be a 64-character ID: it wraps rather than overflows. */
const MONO_ID = cn(MONO, "min-w-0 break-all");

/** Every stay of the task in a status, oldest first. */
export default function WorkflowVisits({ view }: { view: WorkflowView }) {
  return (
    <section className="flex min-w-0 flex-col gap-1">
      <span className={LABEL}>Visits</span>
      <ol className="flex min-w-0 flex-col">
        {[...view.visits]
          .sort((a, b) => a.sequence - b.sequence)
          .map((visit) => (
            <VisitRow key={visit.id} visit={visit} digest={view.digest} />
          ))}
      </ol>
    </section>
  );
}

/** One stay in a status: when, by whom, how it ended, and its message. */
function VisitRow({ visit, digest }: { visit: StatusVisit; digest: string }) {
  return (
    <li className={cn(ROW, "flex-wrap py-1")}>
      <span className={cn(MONO_ID, "font-medium")}>{visit.status}</span>
      {visit.workflow_digest && visit.workflow_digest !== digest && visit.workflow_version && (
        <span className={cn(MONO, "text-muted-foreground")} title={visit.workflow_digest}>
          @{visit.workflow_version}
        </span>
      )}
      <span className={cn(MONO, "text-muted-foreground")}>
        {formatTaskTime(visit.entered_at)} · {visit.entered_by}
      </span>
      <span className="text-ui text-muted-foreground">
        <VisitEnd visit={visit} />
      </span>
      {visit.message && (
        <span className="w-full min-w-0 text-ui break-words whitespace-pre-wrap">
          {visit.message}
        </span>
      )}
    </li>
  );
}

function VisitEnd({ visit }: { visit: StatusVisit }) {
  if (!visit.left_at) return "current";
  if (!visit.outcome) return "left";
  return (
    <>
      left by <span className={MONO}>{visit.outcome}</span>
    </>
  );
}
