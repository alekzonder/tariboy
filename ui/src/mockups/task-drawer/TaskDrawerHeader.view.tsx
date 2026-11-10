import { FileText, MoreHorizontal, PauseCircle, Pencil, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { isClosed, requiredBy, stepArtifacts } from "@/pages/tasks/workflowGraph";
import type { Task, TaskParent, WorkflowView } from "@/types";
import { PropertyChips, type PropertyChipsProps } from "./PropertyChips.view";
import { SectionToggle } from "./SectionToggle.view";
import { TaskDetailsPanel, type TaskDetailsPanelProps } from "./TaskDetailsPanel.view";
import { WorkflowStrip, type WorkflowStripProps } from "./WorkflowStrip.view";

interface DecisionProps {
  message: string;
  busy?: boolean;
  onMessage: (message: string) => void;
  onAdvance: (outcome: string) => void;
  /** Scrolls the body to the artifact and opens it. */
  onShowArtifact: (name: string) => void;
  onResume: (how: "continue" | "release") => void;
  onCancel: () => void;
}

export interface TaskDrawerHeaderProps {
  task: Task;
  parent?: Pick<TaskParent, "key" | "title">;
  /** The workflow of the task; a task without one has no workflow row. */
  view?: WorkflowView;
  chips: Omit<PropertyChipsProps, "task" | "workflow">;
  /** The Details block is open. The customer's choice, kept for all tasks. */
  detailsOpen: boolean;
  onDetailsOpenChange: (open: boolean) => void;
  details: Omit<TaskDetailsPanelProps, "task">;
  strip: Omit<WorkflowStripProps, "view" | "editable">;
  decision: DecisionProps;
  /** A field of the task differs from the saved one. */
  dirty: boolean;
  onSave: () => void;
  onClose: () => void;
  onEditTitle: () => void;
  onOpenParent: (key: string) => void;
  onRequeue: () => void;
  onTransfer: () => void;
}

const PAUSE_REASONS: Record<string, string> = {
  idle_iterations: "The holder worked several iterations in a row without requesting a transition.",
  rejected_requests: "Checks rejected too many transition requests in this status.",
  script_failures: "Scripts failed too many times in a row in this status.",
  holder_unavailable:
    "The holder of this status cannot work and has been unavailable for too long.",
};

const DANGER_BUTTON =
  "h-6 shrink-0 rounded-control px-2.5 text-ui text-status-failed hover:bg-status-failed/10 hover:text-status-failed";

/** The artifacts to read before a decision: those the step into the status
 *  required, then those its outcomes require. */
function decisionArtifacts(view: WorkflowView): string[] {
  return [
    ...new Set([...stepArtifacts(view, view.status).enter.keys(), ...requiredBy(view).keys()]),
  ];
}

function TitleRow({
  task,
  parent,
  dirty,
  onSave,
  onClose,
  onEditTitle,
  onOpenParent,
  onRequeue,
  onTransfer,
}: Pick<
  TaskDrawerHeaderProps,
  | "task"
  | "parent"
  | "dirty"
  | "onSave"
  | "onClose"
  | "onEditTitle"
  | "onOpenParent"
  | "onRequeue"
  | "onTransfer"
>) {
  return (
    <div className="flex items-start gap-2.5">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        {parent && (
          <button
            type="button"
            onClick={() => onOpenParent(parent.key)}
            className="flex min-w-0 items-center gap-1.5 self-start rounded-sm text-ui text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span className="font-mono text-meta tabular-nums">{parent.key}</span>
            <span className="min-w-0 truncate">{parent.title}</span>
          </button>
        )}
        <div className="flex min-w-0 items-start gap-2">
          <h2 className="mt-0.75 shrink-0 font-mono text-ui font-medium tabular-nums text-muted-foreground">
            {task.key}
          </h2>
          {/* Two lines, not one: a title cut at half the window hides the task. */}
          <span className="line-clamp-2 min-w-0 text-title font-semibold tracking-[-.01em] text-pretty">
            {task.title}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Edit title"
            onClick={onEditTitle}
            className="size-6 shrink-0 text-muted-foreground"
          >
            <Pencil className="size-3" />
          </Button>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {dirty && (
          <Button size="sm" className="h-7" onClick={onSave}>
            Save task
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Task actions"
              className="size-7 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <MoreHorizontal className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={onRequeue}>Move to queue…</DropdownMenuItem>
            <DropdownMenuItem onSelect={onTransfer}>Move to another server…</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Close task detail"
          onClick={onClose}
          className="size-7 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <X className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

/** The customer's turn: what to read, an optional message, one button per outcome. */
function CustomerDecision({ view, decision }: { view: WorkflowView; decision: DecisionProps }) {
  const names = decisionArtifacts(view);
  return (
    <section
      aria-label="Workflow decision"
      className="flex min-w-0 flex-col gap-2 rounded-md bg-primary/8 p-2.5"
    >
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <span className="text-body-sm font-medium text-primary">
          Waiting for your decision in {view.status}
        </span>
        {names.map((name) => (
          <Button
            key={name}
            type="button"
            variant="ghost"
            size="sm"
            className="h-6 gap-1 bg-card px-2 font-mono text-meta"
            onClick={() => decision.onShowArtifact(name)}
          >
            <FileText aria-hidden="true" className="size-3" />
            {name}
          </Button>
        ))}
      </div>
      <Textarea
        aria-label="Outcome message"
        placeholder="Message for this decision (optional)"
        value={decision.message}
        disabled={decision.busy}
        maxLength={4096}
        onChange={(event) => decision.onMessage(event.target.value)}
        className="min-h-9 rounded-md border-0 bg-card text-body-sm md:text-body-sm"
      />
      <div className="flex flex-wrap items-start gap-1.5">
        {view.outcomes.map((outcome, index) => (
          <div key={outcome.on} className="flex flex-col gap-0.5">
            <Button
              type="button"
              size="sm"
              variant={index === 0 ? "default" : "outline"}
              className="h-7"
              disabled={decision.busy || !!outcome.missing?.length}
              onClick={() => decision.onAdvance(outcome.on)}
            >
              {outcome.on} → {outcome.to}
            </Button>
            {!!outcome.missing?.length && (
              <span className="text-meta text-status-failed">
                needs: {outcome.missing.join(", ")}
              </span>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}

/** The pause with its reason and the three ways out of it. */
function PauseDecision({ view, decision }: { view: WorkflowView; decision: DecisionProps }) {
  const reason = view.paused_reason ?? "";
  return (
    <section
      aria-label="Workflow decision"
      className="flex min-w-0 flex-col gap-2 rounded-md bg-status-failed/12 px-2.5 py-2 text-status-failed"
    >
      <div role="alert" className="flex items-start gap-2.25">
        <PauseCircle aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 [stroke-width:1.4]" />
        <p className="min-w-0 flex-1 text-body-sm leading-[1.45] font-medium text-pretty">
          Paused: {PAUSE_REASONS[reason] ?? reason} The workflow waits for your decision.
        </p>
      </div>
      <div className="flex flex-wrap gap-1.5">
        <Button
          type="button"
          variant="ghost"
          className={cn(DANGER_BUTTON, "bg-status-failed/14")}
          onClick={() => decision.onResume("continue")}
        >
          Continue
        </Button>
        {view.owner.startsWith("pool:") && (
          <Button
            type="button"
            variant="ghost"
            className={DANGER_BUTTON}
            onClick={() => decision.onResume("release")}
          >
            Release holder
          </Button>
        )}
        <Button type="button" variant="ghost" className={DANGER_BUTTON} onClick={decision.onCancel}>
          Cancel task
        </Button>
      </div>
    </section>
  );
}

function Decision({ view, decision }: { view: WorkflowView; decision: DecisionProps }) {
  if (isClosed(view)) return null;
  if (view.waiting_on === "pause") return <PauseDecision view={view} decision={decision} />;
  if (view.owner === "customer" && view.outcomes.length > 0) {
    return <CustomerDecision view={view} decision={decision} />;
  }
  return null;
}

/**
 * The sticky header of the task drawer: the title, then two blocks that close
 * to one row each. Details is the parameters as chips, with the rare fields
 * and the relations when open. Workflow is the current step, with every step
 * when open. The decision follows only while the task waits for the customer,
 * and it shows whether the blocks are open or not.
 */
export function TaskDrawerHeader(props: TaskDrawerHeaderProps) {
  const { task, view, chips, strip, decision, detailsOpen, onDetailsOpenChange, details } = props;
  const editable = task.access !== "context" && task.access !== "respond";
  return (
    <header className="@container sticky top-0 z-[3] flex max-h-[70vh] min-w-0 flex-col gap-2 overflow-auto bg-card px-3.5 pt-2.75 pb-2.5 shadow-[var(--raise)]">
      <TitleRow {...props} />
      <section aria-label="Details" className="flex min-w-0 flex-col gap-1.5">
        <div className="flex min-w-0 items-start gap-2">
          <SectionToggle label="Details" open={detailsOpen} onOpenChange={onDetailsOpenChange} />
          <PropertyChips task={task} workflow={!!view} disabled={!editable} {...chips} />
        </div>
        {detailsOpen && <TaskDetailsPanel task={task} {...details} />}
      </section>
      {view && <WorkflowStrip view={view} editable={editable} {...strip} />}
      {view && editable && <Decision view={view} decision={decision} />}
    </header>
  );
}
