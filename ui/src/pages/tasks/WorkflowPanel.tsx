import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { taskKeys, taskQueries, refreshTasks, writeTask } from "./taskQueries";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Task, WorkflowView } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, FIELD, LABEL, MONO, QUIET_ACTION, ROW, SelectShell } from "./panelStyles";
import ReturnToAgent from "./ReturnToAgent";
import { formatTaskTime } from "./taskTime";
import WorkflowArtifacts from "./WorkflowArtifacts";
import { useWorkflowConfirm } from "./WorkflowConfirm";
import {
  canReturnToAgent,
  canUpgrade,
  isClosed,
  requiredBy,
  stepArtifacts,
  stepOf,
} from "./workflowGraph";
import WorkflowOutcomes from "./WorkflowOutcomes";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import WorkflowProblem from "./WorkflowProblem";
import WorkflowRuns from "./WorkflowRuns";
import WorkflowStepper from "./WorkflowStepper";
import { errorText } from "@/lib/errors";
import { useMounted } from "./workflowShared";

/** A mono value that may be a 64-character ID: it wraps rather than overflows. */
const MONO_ID = cn(MONO, "min-w-0 break-all");

type OpenForm = "move" | "return" | "upgrade" | null;

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
}: {
  task: Task;
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
  const cancellation = useMutation({
    mutationFn: () => writeTask(client, api, () => api.workflows.cancelWorkflowTask(task.key)),
    onSuccess: () => onTaskChanged?.(),
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

  const failure = error && (
    <div role="alert" className="flex items-center gap-2 text-[12px] text-status-failed">
      <span className="min-w-0 flex-1">{error}</span>
      <Button type="button" variant="ghost" className={QUIET_ACTION} onClick={refresh}>
        Retry
      </Button>
    </div>
  );
  if (!view) {
    return (
      <section className="flex min-w-0 flex-col gap-1.5">
        <span className={LABEL}>Workflow</span>
        {failure || (
          <span className={EMPTY} role="status">
            Loading workflow…
          </span>
        )}
      </section>
    );
  }

  const closed = isClosed(view);
  const editable = task.access !== "context" && task.access !== "respond";
  const step = stepOf(view);
  const declares = (id: string | null | undefined) =>
    !!id && view.statuses.some((status) => status.id === id);
  const selected =
    chosen === null
      ? null
      : declares(chosen)
        ? chosen!
        : declares(view.status)
          ? view.status
          : null;
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
  // Resolves true once moved, false when the confirmation is declined.
  const move = (to: string, reason: string) =>
    new Promise<boolean>((resolve, reject) =>
      confirm({
        title: `Move this task to ${to}?`,
        description:
          "The move skips outcomes, required artifacts, and checks, stops the current scripts, and cancels a pending request.",
        action: "Move task",
        onCancel: () => resolve(false),
        run: () => {
          writeTask(client, api, () => api.workflows.moveTaskWorkflow(task.key, to, reason)).then(
            () => {
              resolve(true);
              onTaskChanged?.();
            },
            reject,
          );
        },
      }),
    );
  const upgradeTo = `${view.name}@${view.queue_version ?? ""}`;
  // Resolves true once upgraded, false when the confirmation is declined.
  const upgrade = (reason: string) =>
    new Promise<boolean>((resolve, reject) =>
      confirm({
        title: `Upgrade this task to ${upgradeTo}?`,
        description: `The task starts again at the first status of ${upgradeTo}. The current status ends, its scripts stop, and a pending request is cancelled. Artifacts stay.`,
        action: "Upgrade task",
        onCancel: () => resolve(false),
        run: () => {
          writeTask(client, api, () => api.workflows.upgradeTaskWorkflow(task.key, reason)).then(
            () => {
              resolve(true);
              onTaskChanged?.();
            },
            reject,
          );
        },
      }),
    );
  const open = (next: OpenForm) => {
    cancellation.reset();
    setForm(form === next ? null : next);
  };

  return (
    <div className="flex min-w-0 flex-col gap-[18px]">
      {view.waiting_on === "pause" && (
        <WorkflowPauseBanner
          taskKey={task.key}
          reason={view.paused_reason ?? task.workflow_paused_reason ?? ""}
          pool={view.owner.startsWith("pool:")}
          onChanged={changed}
        />
      )}
      <WorkflowProblem taskKey={task.key} view={view} onChanged={changed} onRefresh={refresh} />
      <WorkflowStepper view={view} selected={selected} onSelect={setChosen} />
      <div className="flex min-w-0 items-center gap-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2.5 gap-y-0.5 text-[12px] text-muted-foreground">
          <span className={cn(MONO, "font-medium text-foreground")} title={view.digest}>
            {view.name}@{view.version}
          </span>
          {step && !closed && (
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
        {/* Labeled, not behind a menu: the customer's way to send the task
            back, and the manual way out of the workflow, have to be found
            without hunting for them. */}
        {editable && (
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1">
            {canReturnToAgent(view) && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7"
                onClick={() => open("return")}
              >
                Return to agent…
              </Button>
            )}
            {canUpgrade(view) && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7"
                onClick={() => open("upgrade")}
              >
                Upgrade to {upgradeTo}…
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7"
              onClick={() => open("move")}
            >
              Move to status…
            </Button>
            {!closed && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 text-destructive hover:text-destructive"
                disabled={cancellation.isPending}
                onClick={cancel}
              >
                Cancel task
              </Button>
            )}
          </div>
        )}
      </div>
      {failure}
      {cancellation.error && (
        <p role="alert" className="text-[12px] text-status-failed">
          {errorText(cancellation.error)}
        </p>
      )}
      {form === "move" && <MoveForm view={view} onDone={() => setForm(null)} onMove={move} />}
      {form === "upgrade" && <UpgradeForm onDone={() => setForm(null)} onUpgrade={upgrade} />}
      {form === "return" && (
        <ReturnToAgent
          taskKey={task.key}
          view={view}
          onChanged={changed}
          onDone={() => setForm(null)}
        />
      )}
      <WorkflowOutcomes taskKey={task.key} view={view} onChanged={changed} onRefresh={refresh} />
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
      <section className="flex min-w-0 flex-col gap-1">
        <span className={LABEL}>Visits</span>
        <ol className="flex min-w-0 flex-col">
          {[...view.visits]
            .sort((a, b) => a.sequence - b.sequence)
            .map((visit) => (
              <li key={visit.id} className={cn(ROW, "flex-wrap py-1")}>
                <span className={cn(MONO_ID, "font-medium")}>{visit.status}</span>
                {visit.workflow_digest &&
                  visit.workflow_digest !== view.digest &&
                  visit.workflow_version && (
                    <span
                      className={cn(MONO, "text-muted-foreground")}
                      title={visit.workflow_digest}
                    >
                      @{visit.workflow_version}
                    </span>
                  )}
                <span className={cn(MONO, "text-muted-foreground")}>
                  {formatTaskTime(visit.entered_at)} · {visit.entered_by}
                </span>
                {visit.left_at ? (
                  <span className="text-[12px] text-muted-foreground">
                    {visit.outcome ? (
                      <>
                        left by <span className={MONO}>{visit.outcome}</span>
                      </>
                    ) : (
                      "left"
                    )}
                  </span>
                ) : (
                  <span className="text-[12px] text-muted-foreground">current</span>
                )}
                {visit.message && (
                  <span className="w-full min-w-0 text-[12px] break-words whitespace-pre-wrap">
                    {visit.message}
                  </span>
                )}
              </li>
            ))}
        </ol>
      </section>
      {dialog}
    </div>
  );
}

/**
 * The statuses a move may target: every declared status but the current one,
 * the terminal ones last. A move to a terminal status is the operator's
 * decision, so they stay on offer.
 */
function moveTargets(view: WorkflowView) {
  const others = (view.statuses ?? []).filter((status) => status.id !== view.status);
  return {
    open: others.filter((status) => !status.terminal),
    terminal: others.filter((status) => status.terminal),
  };
}

function MoveForm({
  view,
  onMove,
  onDone,
}: {
  view: WorkflowView;
  onMove: (to: string, reason: string) => Promise<boolean>;
  onDone: () => void;
}) {
  const [to, setTo] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const mountedRef = useMounted();
  const { open, terminal } = moveTargets(view);
  const submit = () => {
    setError("");
    setBusy(true);
    onMove(to, reason.trim())
      .then(
        (moved) => {
          if (moved && mountedRef.current) onDone();
        },
        (failure) => {
          if (mountedRef.current) setError(errorText(failure));
        },
      )
      .finally(() => {
        if (mountedRef.current) setBusy(false);
      });
  };
  return (
    <form
      className="flex min-w-0 flex-col gap-1.5 rounded-[8px] bg-muted/50 p-2.5"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <span className={LABEL}>Move to status</span>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <SelectShell
          autoFocus
          aria-label="Target status"
          value={to}
          className="h-[26px] w-auto max-w-full text-[12px] md:text-[12px]"
          onChange={(event) => setTo(event.target.value)}
        >
          <option value="">Choose a status</option>
          {open.map((status) => (
            <option key={status.id} value={status.id}>
              {status.id}
            </option>
          ))}
          {terminal.length > 0 && (
            <optgroup label="Terminal">
              {terminal.map((status) => (
                <option key={status.id} value={status.id}>
                  {status.id}
                </option>
              ))}
            </optgroup>
          )}
        </SelectShell>
        <Input
          aria-label="Reason"
          placeholder="Reason (required)"
          value={reason}
          className={cn(FIELD, "h-[26px] min-w-[160px] flex-1")}
          onChange={(event) => setReason(event.target.value)}
        />
        <Button
          type="submit"
          size="sm"
          className="h-[26px]"
          disabled={busy || !to || !reason.trim()}
        >
          Move
        </Button>
        <Button type="button" variant="ghost" size="sm" className="h-[26px]" onClick={onDone}>
          Close
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-[12px] text-status-failed">
          {error}
        </p>
      )}
    </form>
  );
}

function UpgradeForm({
  onUpgrade,
  onDone,
}: {
  onUpgrade: (reason: string) => Promise<boolean>;
  onDone: () => void;
}) {
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const mountedRef = useMounted();
  const submit = () => {
    setError("");
    setBusy(true);
    onUpgrade(reason.trim())
      .then(
        (upgraded) => {
          if (upgraded && mountedRef.current) onDone();
        },
        (failure) => {
          if (mountedRef.current) setError(errorText(failure));
        },
      )
      .finally(() => {
        if (mountedRef.current) setBusy(false);
      });
  };
  return (
    <form
      className="flex min-w-0 flex-col gap-1.5 rounded-[8px] bg-muted/50 p-2.5"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <span className={LABEL}>Upgrade workflow</span>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <Input
          autoFocus
          aria-label="Upgrade reason"
          placeholder="Reason (optional)"
          value={reason}
          className={cn(FIELD, "h-[26px] min-w-[160px] flex-1")}
          onChange={(event) => setReason(event.target.value)}
        />
        <Button type="submit" size="sm" className="h-[26px]" disabled={busy}>
          Upgrade
        </Button>
        <Button type="button" variant="ghost" size="sm" className="h-[26px]" onClick={onDone}>
          Close
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-[12px] text-status-failed">
          {error}
        </p>
      )}
    </form>
  );
}
