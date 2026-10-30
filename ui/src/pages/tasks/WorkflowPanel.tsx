import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { taskKeys, taskQueries, refreshTasks, writeTask } from "./taskQueries";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import type { Task, WorkflowView, StatusVisit } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, LABEL, MONO, QUIET_ACTION, ROW } from "./panelStyles";
import ReturnToAgent from "./ReturnToAgent";
import { formatTaskTime } from "./taskTime";
import WorkflowActions, { ActionError, type OpenForm } from "./WorkflowActions";
import WorkflowArtifacts from "./WorkflowArtifacts";
import { MoveForm, UpgradeForm } from "./WorkflowMoveForms";
import { useWorkflowConfirm, type ConfirmRequest } from "./WorkflowConfirm";
import { isClosed, requiredBy, selectedStep, stepArtifacts, stepOf } from "./workflowGraph";
import WorkflowOutcomes from "./WorkflowOutcomes";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import WorkflowProblem from "./WorkflowProblem";
import WorkflowRuns from "./WorkflowRuns";
import WorkflowStepper from "./WorkflowStepper";
import { errorText } from "@/lib/errors";

/** A mono value that may be a 64-character ID: it wraps rather than overflows. */
const MONO_ID = cn(MONO, "min-w-0 break-all");

/**
 * The workflow of a task, in the task detail: what is wrong, where it is,
 * what it may do next, what it produced, and what ran. It replaces the status
 * control of a flexible task; the status changes only through an outcome, a
 * return to the agent, a move, a cancel, or a pause decision. The Desktop
 * acts as the customer (operator routes).
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
  /**
   * The latest task event the detail holds. A workflow event (a rejected
   * request, a script run) need not change the task's revision, so a new
   * sequence refetches the view too.
   */
  eventSequence?: number;
  onTaskChanged?: () => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  const workflow = useApiQuery(taskQueries.workflow(api, task.key));
  const view = workflow.data;
  const error = workflow.error ? errorText(workflow.error) : "";
  const [form, setForm] = useState<OpenForm>(null);
  const [chosen, setChosen] = useState<string | null | undefined>(undefined);
  const { confirm, dialog } = useWorkflowConfirm();
  useRefetchOnChange(task, eventSequence);
  const cancellation = useMutation({
    mutationFn: () => writeTask(client, api, () => api.workflows.cancelWorkflowTask(task.key)),
    onSuccess: () => onTaskChanged?.(),
  });
  const watching = useMutation({
    mutationFn: () => api.workflows.runTaskWatch(task.key),
    onSuccess: () => void workflow.refetch(),
  });
  const changed = () => {
    cancellation.reset();
    void refreshTasks(client, api);
    onTaskChanged?.();
  };
  const refresh = () => {
    cancellation.reset();
    void workflow.refetch();
  };

  const failure = error && <LoadFailure error={error} onRetry={refresh} />;
  if (!view) {
    return (
      <section className="flex min-w-0 flex-col gap-1.5">
        <span className={LABEL}>Workflow</span>
        {failure || <Loading />}
      </section>
    );
  }

  const closed = isClosed(view);
  const editable = task.access !== "context" && task.access !== "respond";
  const selected = selectedStep(view, chosen);
  const cancel = () =>
    confirm({
      title: "Cancel this task?",
      description:
        "The task closes as cancelled and its scripts stop. The workflow status stays where it stopped.",
      action: "Cancel task",
      run: () => {
        cancellation.mutate();
      },
    });
  const write = (request: Omit<ConfirmRequest, "run" | "onCancel">, send: () => Promise<unknown>) =>
    confirmWrite(confirm, request, () => writeTask(client, api, send), onTaskChanged);
  const move = (to: string, reason: string) =>
    write(
      {
        title: `Move this task to ${to}?`,
        description:
          "The move skips outcomes, required artifacts, and checks, stops the current scripts, and cancels a pending request.",
        action: "Move task",
      },
      () => api.workflows.moveTaskWorkflow(task.key, to, reason),
    );
  const upgradeTo = `${view.name}@${view.queue_version ?? ""}`;
  const upgrade = (reason: string) =>
    write(
      {
        title: `Upgrade this task to ${upgradeTo}?`,
        description: `The task starts again at the first status of ${upgradeTo}. The current status ends, its scripts stop, and a pending request is cancelled. Artifacts stay.`,
        action: "Upgrade task",
      },
      () => api.workflows.upgradeTaskWorkflow(task.key, reason),
    );
  const open = (next: OpenForm) => {
    cancellation.reset();
    watching.reset();
    setForm(form === next ? null : next);
  };

  return (
    <div className="flex min-w-0 flex-col gap-4.5">
      <PauseBanner
        task={task}
        view={view}
        hidden={decisionsElsewhere === true}
        onChanged={changed}
      />
      <WorkflowProblem taskKey={task.key} view={view} onChanged={changed} onRefresh={refresh} />
      <WorkflowStepper view={view} selected={selected} onSelect={setChosen} />
      <div className="flex min-w-0 items-center gap-2">
        <WorkflowHeader view={view} />
        {/* Labeled, not behind a menu: the customer's way to send the task
            back, and the manual way out of the workflow, have to be found
            without hunting for them. */}
        {editable && (
          <WorkflowActions
            view={view}
            upgradeTo={upgradeTo}
            cancelling={cancellation.isPending}
            watching={watching.isPending}
            onRunWatch={() => {
              cancellation.reset();
              watching.mutate();
            }}
            onOpen={open}
            onCancel={cancel}
          />
        )}
      </div>
      {failure}
      <ActionError errors={[cancellation.error, watching.error]} />
      <OpenFormPane
        form={form}
        taskKey={task.key}
        view={view}
        actions={{ move, upgrade, changed, close: () => setForm(null) }}
      />
      <Outcomes
        hidden={decisionsElsewhere === true}
        taskKey={task.key}
        view={view}
        onChanged={changed}
        onRefresh={refresh}
      />
      <WorkflowArtifacts
        taskKey={task.key}
        artifacts={view.artifacts}
        declared={view.declared_artifacts ?? []}
        required={closed ? undefined : requiredBy(view)}
        step={selected ? { status: selected, ...stepArtifacts(view, selected) } : undefined}
        onShowAll={() => setChosen(null)}
        editable={editable && !closed}
        onChanged={changed}
      />
      <WorkflowRuns taskKey={task.key} runs={view.runs ?? []} />
      <VisitList view={view} />
      {dialog}
    </div>
  );
}

/**
 * Refetches the view when the task's revision or event sequence changes,
 * unless a mounted shared detail already refreshes it.
 */
function useRefetchOnChange(task: Task, eventSequence: number) {
  const api = useApi();
  const client = useQueryClient();
  const lastChange = useRef([task.revision, eventSequence]);
  useEffect(() => {
    if (lastChange.current[0] === task.revision && lastChange.current[1] === eventSequence) return;
    lastChange.current = [task.revision, eventSequence];
    const queryKey = taskKeys.workflow(api.target, task.key);
    // A mounted shared detail owns server-wide event/write refreshes, including
    // workflow. Standalone panels still react to changes in their input props.
    const sharedDetail = client
      .getQueryCache()
      .find({ queryKey: taskKeys.detail(api.target, task.key), exact: true });
    if (sharedDetail?.isActive()) return;
    void client.cancelQueries({ queryKey }).then(() => client.invalidateQueries({ queryKey }));
  }, [client, api, task.key, task.revision, eventSequence]);
}

/**
 * Asks before a write; resolves true once it is done, false when the
 * confirmation is declined.
 */
function confirmWrite(
  confirm: (request: ConfirmRequest) => void,
  request: Omit<ConfirmRequest, "run" | "onCancel">,
  write: () => Promise<unknown>,
  onDone?: () => void,
): Promise<boolean> {
  return new Promise<boolean>((resolve, reject) =>
    confirm({
      ...request,
      onCancel: () => resolve(false),
      run: () => {
        write().then(() => {
          resolve(true);
          onDone?.();
        }, reject);
      },
    }),
  );
}

function Loading() {
  return (
    <span className={EMPTY} role="status">
      Loading workflow…
    </span>
  );
}

/** The outcome block, unless the Needs you card above already offers it. */
function Outcomes({
  hidden,
  ...props
}: { hidden: boolean } & Parameters<typeof WorkflowOutcomes>[0]) {
  return hidden ? null : <WorkflowOutcomes {...props} />;
}

/** The pause and its reason, while the workflow is paused. */
function PauseBanner({
  task,
  view,
  hidden,
  onChanged,
}: {
  task: Task;
  view: WorkflowView;
  hidden: boolean;
  onChanged: () => void;
}) {
  if (hidden || view.waiting_on !== "pause") return null;
  return (
    <WorkflowPauseBanner
      taskKey={task.key}
      reason={view.paused_reason ?? task.workflow_paused_reason ?? ""}
      pool={view.owner.startsWith("pool:")}
      onChanged={onChanged}
    />
  );
}

/** The form an action opened: move, upgrade, or return to agent. */
function OpenFormPane({
  form,
  taskKey,
  view,
  actions,
}: {
  form: OpenForm;
  taskKey: string;
  view: WorkflowView;
  actions: {
    move: (to: string, reason: string) => Promise<boolean>;
    upgrade: (reason: string) => Promise<boolean>;
    changed: () => void;
    close: () => void;
  };
}) {
  switch (form) {
    case "move":
      return <MoveForm view={view} onDone={actions.close} onMove={actions.move} />;
    case "upgrade":
      return <UpgradeForm onDone={actions.close} onUpgrade={actions.upgrade} />;
    case "return":
      return (
        <ReturnToAgent
          taskKey={taskKey}
          view={view}
          onChanged={actions.changed}
          onDone={actions.close}
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

function VisitList({ view }: { view: WorkflowView }) {
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
