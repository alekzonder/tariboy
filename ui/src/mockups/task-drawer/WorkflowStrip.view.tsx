import { Check, MoreHorizontal, Undo2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { StatusDot } from "@/components/ui/status";
import type { StatusTone } from "@/lib/statusTone";
import { cn } from "@/lib/utils";
import { explainProblem } from "@/pages/tasks/explainWorkflowProblem";
import {
  canReturnToAgent,
  canRunWatch,
  canUpgrade,
  isClosed,
  stepMarks,
  stepOf,
  type StepMark,
} from "@/pages/tasks/workflowGraph";
import type { StatusView, WorkflowView } from "@/types";
import { SectionToggle } from "./SectionToggle.view";

type WorkflowAction = "run_watch" | "return" | "upgrade" | "move" | "cancel";

export interface WorkflowStripProps {
  view: WorkflowView;
  /** The step whose description is open under the strip. */
  selected: string | null;
  onSelect: (id: string | null) => void;
  /** The block is open: every step shows. The customer's choice, kept for all tasks. */
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editable: boolean;
  onAction: (action: WorkflowAction) => void;
}

const MONO = "font-mono text-meta tabular-nums";

/** Whose turn it is, as the tone of the current step. */
function turnTone(view: WorkflowView): StatusTone {
  if (view.waiting_on === "pause") return "danger";
  if (view.owner === "customer") return "attention";
  const problem = explainProblem(view);
  if (problem && (problem.tone === "danger" || problem.tone === "attention")) return problem.tone;
  return "live";
}

/** One short line beside the steps: the turn, or what is wrong. */
function turnNote(view: WorkflowView): { text: string; tone: StatusTone } {
  if (isClosed(view)) return { text: "closed", tone: "quiet" };
  if (view.waiting_on === "pause") return { text: "paused", tone: "danger" };
  if (view.owner === "customer") return { text: "your decision", tone: "attention" };
  const problem = explainProblem(view);
  if (problem?.kind === "rejected" || problem?.kind === "failed") {
    const what = problem.kind === "rejected" ? "check rejected" : "check failed";
    return { text: problem.counter ? `${what} · ${problem.counter}` : what, tone: problem.tone };
  }
  if (view.owner === "script") return { text: "a script works", tone: "quiet" };
  const holder = view.holder?.replace(/^agent:/, "");
  return { text: holder ? `${holder} works` : "waits for an agent", tone: "quiet" };
}

const NOTE_TEXT: Record<StatusTone, string> = {
  live: "text-status-running",
  attention: "text-primary",
  danger: "text-status-failed",
  quiet: "text-muted-foreground",
  faint: "text-muted-foreground",
};

function StepMarker({ mark, tone }: { mark: StepMark; tone: StatusTone }) {
  if (mark.state === "visited") {
    return (
      <Check aria-hidden="true" className="size-3 shrink-0 text-status-running [stroke-width:2]" />
    );
  }
  if (mark.status.terminal) {
    return (
      <span aria-hidden="true" className="size-1.75 shrink-0 rounded-hair bg-status-stopped" />
    );
  }
  return <StatusDot tone={mark.state === "current" ? tone : "faint"} />;
}

function Step({
  mark,
  tone,
  open,
  onToggle,
}: {
  mark: StepMark;
  tone: StatusTone;
  open: boolean;
  onToggle: () => void;
}) {
  const { status, state, returns } = mark;
  return (
    <button
      type="button"
      aria-current={state === "current" ? "step" : undefined}
      aria-expanded={open}
      onClick={onToggle}
      className={cn(
        "flex min-w-0 max-w-full items-center gap-1.5 rounded-control px-1.5 py-0.5 text-left outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring",
        state === "current" && "bg-muted",
        open && "ring-1 ring-ring",
        state === "ahead" && "text-muted-foreground opacity-70",
      )}
    >
      <StepMarker mark={mark} tone={tone} />
      <span
        className={cn(
          MONO,
          "max-w-[18ch] min-w-0 truncate",
          state === "current" && "font-medium text-foreground",
        )}
        title={status.id}
      >
        {status.id}
      </span>
      {returns > 0 && (
        <span
          className={cn(MONO, "inline-flex items-center gap-0.5 text-muted-foreground")}
          title={`The task came back to this status ${returns === 1 ? "once" : `${returns} times`}`}
        >
          <Undo2 aria-hidden="true" className="size-2.5" />
          {returns}
        </span>
      )}
    </button>
  );
}

function ownerText(status: StatusView): string {
  if (status.terminal) return status.cancelled ? "terminal, counts as cancelled" : "terminal";
  if (status.owner.startsWith("pool:")) return `pool ${status.owner.slice(5)}`;
  if (status.owner === "customer") return "the customer";
  if (status.owner === "script") return "a watch script";
  return status.owner;
}

/** What a step needs: its owner, its instructions, and each way out. */
function StepDetail({ status, view }: { status: StatusView; view: WorkflowView }) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-md bg-muted/60 px-2.5 py-2 text-ui">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5">
        <span className={cn(MONO, "font-medium")}>{status.id}</span>
        <span className="text-muted-foreground">owned by {ownerText(status)}</span>
        {status.instructions_path && (
          <span className={cn(MONO, "min-w-0 break-all text-muted-foreground")}>
            {status.instructions_path}
          </span>
        )}
        <span className={cn(MONO, "ml-auto text-muted-foreground")} title={view.digest}>
          {view.name}@{view.version}
        </span>
      </div>
      {(status.transitions ?? []).map((transition) => (
        <div key={transition.on} className="flex min-w-0 flex-wrap items-center gap-x-1.5">
          <span className={cn(MONO, "font-medium")}>{transition.on}</span>
          <span className="text-muted-foreground">→</span>
          <span className={MONO}>{transition.to}</span>
          {(transition.requires ?? []).length > 0 && (
            <span className="text-muted-foreground">
              requires <span className={MONO}>{(transition.requires ?? []).join(", ")}</span>
            </span>
          )}
          {(transition.checks ?? []).map((check) => (
            <span key={check.script} className="text-muted-foreground">
              check <span className={MONO}>{check.script}</span>
            </span>
          ))}
        </div>
      ))}
    </div>
  );
}

function StripActions({ view, onAction }: Pick<WorkflowStripProps, "view" | "onAction">) {
  const closed = isClosed(view);
  return (
    <div className="flex shrink-0 items-center gap-1">
      {canReturnToAgent(view) && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-6.5 px-2 text-ui"
          onClick={() => onAction("return")}
        >
          <Undo2 aria-hidden="true" />
          Return to agent…
        </Button>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Workflow actions"
            className="size-6.5 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <MoreHorizontal className="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {canRunWatch(view) && (
            <DropdownMenuItem onSelect={() => onAction("run_watch")}>
              Run watch now
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={() => onAction("move")}>Move to status…</DropdownMenuItem>
          {canUpgrade(view) && (
            <DropdownMenuItem onSelect={() => onAction("upgrade")}>
              Upgrade to {view.name}@{view.queue_version}…
            </DropdownMenuItem>
          )}
          {!closed && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onSelect={() => onAction("cancel")}
              >
                Cancel task
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

/**
 * The Workflow block of the sticky header. Closed, it is one row: the current
 * step with its number, whose turn it is, and the workflow actions. Open, it
 * shows every step, and a step opens what it needs. It is in the same place
 * whoever owns the status, so the customer never scrolls for it.
 */
export function WorkflowStrip({
  view,
  selected,
  onSelect,
  open,
  onOpenChange,
  editable,
  onAction,
}: WorkflowStripProps) {
  const marks = stepMarks(view);
  const tone = turnTone(view);
  const note = turnNote(view);
  const step = stepOf(view);
  const current = marks.find((mark) => mark.state === "current");
  const opened = open ? marks.find((mark) => mark.status.id === selected) : undefined;
  return (
    <section aria-label="Workflow" className="flex min-w-0 flex-col gap-1.5">
      <div className="flex min-w-0 items-start gap-2">
        <SectionToggle label="Workflow" open={open} onOpenChange={onOpenChange} />
        <div className="flex min-h-6 min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1">
          {open ? (
            <ol
              aria-label="Workflow steps"
              className="flex min-w-0 flex-wrap items-center gap-x-0.5 gap-y-1"
            >
              {marks.map((mark, index) => (
                <li key={mark.status.id} className="flex min-w-0 items-center gap-0.5">
                  {index > 0 && <span aria-hidden="true" className="h-px w-2 shrink-0 bg-border" />}
                  <Step
                    mark={mark}
                    tone={tone}
                    open={selected === mark.status.id}
                    onToggle={() => onSelect(selected === mark.status.id ? null : mark.status.id)}
                  />
                </li>
              ))}
            </ol>
          ) : (
            <button
              type="button"
              aria-label="Show all workflow steps"
              onClick={() => onOpenChange(true)}
              className="flex min-w-0 items-center gap-1.5 rounded-control bg-muted px-1.5 py-0.5 outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
            >
              {current ? <StepMarker mark={current} tone={tone} /> : null}
              <span className={cn(MONO, "min-w-0 truncate font-medium")}>{view.status}</span>
              {step && (
                <span className={cn(MONO, "shrink-0 text-muted-foreground")}>
                  {step.index}/{step.total}
                </span>
              )}
            </button>
          )}
          {/* The turn and the actions wrap together, never one without the other. */}
          <div className="ml-auto flex min-w-0 items-center gap-2">
            <span className={cn("min-w-0 truncate text-ui", NOTE_TEXT[note.tone])}>
              {note.text}
            </span>
            {editable && <StripActions view={view} onAction={onAction} />}
          </div>
        </div>
      </div>
      {opened && <StepDetail status={opened.status} view={view} />}
    </section>
  );
}
