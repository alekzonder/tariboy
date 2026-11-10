import { StatusDot } from "@/components/ui/status";
import type { StatusTone } from "@/lib/statusTone";
import { cn } from "@/lib/utils";
import type { WorkflowView } from "@/types";
import { explainProblem } from "./explainWorkflowProblem";
import { MONO } from "./panelStyles";
import { isClosed, stepOf } from "./workflowGraph";

/** One short line beside the step: whose turn it is, or what is wrong. */
function turnNote(view: WorkflowView): { text: string; tone: StatusTone } {
  if (isClosed(view)) return { text: "closed", tone: "quiet" };
  if (view.waiting_on === "pause") return { text: "paused", tone: "danger" };
  if (view.owner === "customer") return { text: "your decision", tone: "attention" };
  const problem = explainProblem(view);
  if (problem?.kind === "rejected" || problem?.kind === "failed") {
    const what = problem.kind === "rejected" ? "check rejected" : "check failed";
    return { text: problem.counter ? `${what} · ${problem.counter}` : what, tone: problem.tone };
  }
  if (view.owner === "script") return { text: "a script works", tone: "live" };
  const holder = view.holder?.replace(/^agent:/, "");
  return holder
    ? { text: `${holder} works`, tone: "live" }
    : { text: "waits for an agent", tone: "quiet" };
}

const NOTE_TEXT: Record<StatusTone, string> = {
  live: "text-muted-foreground",
  attention: "text-primary",
  danger: "text-status-failed",
  quiet: "text-muted-foreground",
  faint: "text-muted-foreground",
};

/**
 * The closed Workflow block of the task header, as one row: the current
 * status with its number among the steps, and whose turn it is. The status
 * opens the block.
 */
export function WorkflowSummary({ view, onOpen }: { view: WorkflowView; onOpen: () => void }) {
  const step = stepOf(view);
  const note = turnNote(view);
  return (
    <div className="flex min-h-6 min-w-0 flex-1 items-center gap-2">
      <button
        type="button"
        aria-label={`Show all workflow steps, current status ${view.status}`}
        onClick={onOpen}
        className="flex min-w-0 items-center gap-1.5 rounded-control bg-muted px-1.5 py-0.5 outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
      >
        {!isClosed(view) && <StatusDot tone={note.tone === "quiet" ? "faint" : note.tone} />}
        <span className={cn(MONO, "min-w-0 truncate font-medium")}>{view.status}</span>
        {step && !isClosed(view) && (
          <span className={cn(MONO, "shrink-0 text-muted-foreground")}>
            {step.index}/{step.total}
          </span>
        )}
      </button>
      <span className={cn("min-w-0 truncate text-ui", NOTE_TEXT[note.tone])}>{note.text}</span>
    </div>
  );
}
