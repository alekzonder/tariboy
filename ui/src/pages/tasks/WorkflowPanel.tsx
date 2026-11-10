import { useId } from "react";
import { Button } from "@/components/ui/button";
import type { Task, WorkflowView } from "@/types";
import { cn } from "@/lib/utils";
import { EMPTY, LABEL, MONO, QUIET_ACTION } from "./panelStyles";
import ReturnToAgent from "./ReturnToAgent";
import { SectionToggle } from "./SectionToggle.view";
import WorkflowActions, { ActionError } from "./WorkflowActions";
import WorkflowArtifacts from "./WorkflowArtifacts";
import { MoveForm, UpgradeForm } from "./WorkflowMoveForms";
import { isClosed, requiredBy, stepArtifacts, stepOf } from "./workflowGraph";
import WorkflowOutcomes from "./WorkflowOutcomes";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import WorkflowProblem from "./WorkflowProblem";
import WorkflowRuns from "./WorkflowRuns";
import WorkflowStepper from "./WorkflowStepper";
import { WorkflowSummary } from "./WorkflowSummary.view";
import WorkflowVisits from "./WorkflowVisits";
import { useWorkflowPanel, type WorkflowPanelState } from "./useWorkflowPanel";

/** A mono value that may be a 64-character ID: it wraps rather than overflows. */
const MONO_ID = cn(MONO, "min-w-0 break-all");

/**
 * The workflow of a task in one column: what is wrong, where it is, what it
 * may do next, what it produced, and what ran. The task detail does not use
 * this composition: it puts `WorkflowControls` in its sticky header,
 * `WorkflowFindings` at the top of its body and `WorkflowLog` at the bottom,
 * all three over one `useWorkflowPanel`. The status changes only through an
 * outcome, a return to the agent, a move, a cancel, or a pause decision. The
 * Desktop acts as the customer (operator routes).
 */
export function WorkflowPanel({
  task,
  eventSequence = 0,
  onTaskChanged,
  decisionsElsewhere,
}: {
  task: Task;
  /** The Needs you card above holds the outcomes and the pause decisions. */
  decisionsElsewhere?: boolean;
  /** The latest task event the detail holds; a new one refetches the view. */
  eventSequence?: number;
  onTaskChanged?: () => void;
}) {
  const panel = useWorkflowPanel(task, { eventSequence, onTaskChanged });
  const { view } = panel;
  return (
    <div className="flex min-w-0 flex-col gap-4.5">
      {view && !decisionsElsewhere && <PauseBanner task={task} view={view} panel={panel} />}
      <WorkflowTrouble panel={panel} />
      <WorkflowControls panel={panel} />
      {view && !decisionsElsewhere && (
        <WorkflowOutcomes
          taskKey={task.key}
          view={view}
          onChanged={panel.changed}
          onRefresh={panel.refresh}
        />
      )}
      <WorkflowProduce panel={panel} />
      <WorkflowLog panel={panel} />
    </div>
  );
}

/**
 * The Workflow block of the task header: the steps, the workflow version with
 * the status, its owner and holder, the actions, and the form an action
 * opened. With `onOpenChange` the block collapses to one row, the current
 * status and whose turn it is.
 */
export function WorkflowControls({
  panel,
  open = true,
  onOpenChange,
}: {
  panel: WorkflowPanelState;
  open?: boolean;
  /** The block can be collapsed; without it the block is always open. */
  onOpenChange?: (open: boolean) => void;
}) {
  const { task, view } = panel;
  const contentId = useId();
  const failure = panel.error && <LoadFailure error={panel.error} onRetry={panel.refresh} />;
  if (!view) {
    return (
      <section className="flex min-w-0 flex-col gap-1.5">
        <span className={LABEL}>Workflow</span>
        {failure || <Loading />}
      </section>
    );
  }
  return (
    <section aria-label="Workflow" className="flex min-w-0 flex-col gap-2">
      <div className="flex min-w-0 items-start gap-2">
        {onOpenChange && (
          <SectionToggle
            label="Workflow"
            open={open}
            onOpenChange={onOpenChange}
            controls={contentId}
          />
        )}
        {open ? (
          <div id={contentId} className="flex min-w-0 flex-1 flex-col gap-2">
            <WorkflowStepper view={view} selected={panel.selected} onSelect={panel.setChosen} />
            <div className="flex min-w-0 items-center gap-2">
              <WorkflowHeader view={view} />
              {/* Labeled, not behind a menu: the customer's way to send the task
                  back, and the manual way out of the workflow, have to be found
                  without hunting for them. */}
              {panel.editable && (
                <WorkflowActions
                  view={view}
                  upgradeTo={panel.upgradeTo}
                  cancelling={panel.cancellation.isPending}
                  watching={panel.watching.isPending}
                  onRunWatch={panel.runWatch}
                  onOpen={panel.open}
                  onCancel={panel.cancel}
                />
              )}
            </div>
          </div>
        ) : (
          <WorkflowSummary view={view} onOpen={() => onOpenChange?.(true)} />
        )}
      </div>
      {failure}
      <ActionError errors={[panel.cancellation.error, panel.watching.error]} />
      {open && <OpenFormPane panel={panel} taskKey={task.key} view={view} />}
      {panel.dialog}
    </section>
  );
}

/** What is wrong with the workflow, in full: the first thing of the task body. */
export function WorkflowTrouble({ panel }: { panel: WorkflowPanelState }) {
  if (!panel.view) return null;
  return (
    <WorkflowProblem
      taskKey={panel.task.key}
      view={panel.view}
      onChanged={panel.changed}
      onRefresh={panel.refresh}
    />
  );
}

/** What the workflow produced: the artifacts, filtered by the selected step. */
export function WorkflowProduce({ panel }: { panel: WorkflowPanelState }) {
  const { task, view, selected } = panel;
  if (!view) return null;
  const closed = isClosed(view);
  return (
    <WorkflowArtifacts
      taskKey={task.key}
      artifacts={view.artifacts}
      declared={view.declared_artifacts ?? []}
      required={closed ? undefined : requiredBy(view)}
      step={selected ? { status: selected, ...stepArtifacts(view, selected) } : undefined}
      onShowAll={() => panel.setChosen(null)}
      editable={panel.editable && !closed}
      onChanged={panel.changed}
    />
  );
}

/** What ran and where the task has been: the script runs and the visits. */
export function WorkflowLog({ panel }: { panel: WorkflowPanelState }) {
  if (!panel.view) return null;
  return (
    <>
      <WorkflowRuns taskKey={panel.task.key} runs={panel.view.runs ?? []} />
      <WorkflowVisits view={panel.view} />
    </>
  );
}

function Loading() {
  return (
    <span className={EMPTY} role="status">
      Loading workflow…
    </span>
  );
}

/** The pause and its reason, while the workflow is paused. */
function PauseBanner({
  task,
  view,
  panel,
}: {
  task: Task;
  view: WorkflowView;
  panel: WorkflowPanelState;
}) {
  if (view.waiting_on !== "pause") return null;
  return (
    <WorkflowPauseBanner
      taskKey={task.key}
      reason={view.paused_reason ?? task.workflow_paused_reason ?? ""}
      pool={view.owner.startsWith("pool:")}
      onChanged={panel.changed}
    />
  );
}

/** The form an action opened: move, upgrade, or return to agent. */
function OpenFormPane({
  panel,
  taskKey,
  view,
}: {
  panel: WorkflowPanelState;
  taskKey: string;
  view: WorkflowView;
}) {
  switch (panel.form) {
    case "move":
      return <MoveForm view={view} onDone={panel.closeForm} onMove={panel.move} />;
    case "upgrade":
      return <UpgradeForm onDone={panel.closeForm} onUpgrade={panel.upgrade} />;
    case "return":
      return (
        <ReturnToAgent
          taskKey={taskKey}
          view={view}
          onChanged={panel.changed}
          onDone={panel.closeForm}
        />
      );
    default:
      return null;
  }
}

function LoadFailure({ error, onRetry }: { error: string; onRetry: () => void }) {
  return (
    <div role="alert" className="flex items-center gap-2 text-ui text-status-failed">
      <span className="min-w-0 flex-1">{error}</span>
      <Button type="button" variant="ghost" className={QUIET_ACTION} onClick={onRetry}>
        Retry
      </Button>
    </div>
  );
}

/** The workflow version, the step, the status, its owner and holder. */
function WorkflowHeader({ view }: { view: WorkflowView }) {
  const step = stepOf(view);
  return (
    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2.5 gap-y-0.5 text-ui text-muted-foreground">
      <span className={cn(MONO, "font-medium text-foreground")} title={view.digest}>
        {view.name}@{view.version}
      </span>
      {step && !isClosed(view) && (
        <span className={MONO}>
          step {step.index} of {step.total}
        </span>
      )}
      <span className="min-w-0">
        status <span className={cn(MONO_ID, "text-foreground")}>{view.status}</span>
      </span>
      {view.owner && (
        <span className="min-w-0">
          owner <span className={cn(MONO_ID, "text-foreground")}>{view.owner}</span>
        </span>
      )}
      {view.holder && (
        <span className="min-w-0">
          holder <span className={cn(MONO_ID, "text-foreground")}>{view.holder}</span>
        </span>
      )}
    </div>
  );
}
