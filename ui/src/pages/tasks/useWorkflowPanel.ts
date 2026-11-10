import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { errorText } from "@/lib/errors";
import type { Task } from "@/types";
import { refreshTasks, taskKeys, taskQueries, writeTask } from "./taskQueries";
import type { OpenForm } from "./WorkflowActions";
import { useWorkflowConfirm, type ConfirmRequest } from "./WorkflowConfirm";
import { selectedStep } from "./workflowGraph";

/**
 * The state behind the workflow of one task: its view, the step the customer
 * picked, the open form, and the writes (cancel, move, upgrade, run watch)
 * with their confirmations. One owner calls it and gives the result to every
 * part that draws the workflow, so the controls in the task header and the
 * artifacts in the task body agree on the selected step.
 */
export function useWorkflowPanel(
  task: Task,
  {
    eventSequence = 0,
    onTaskChanged,
    enabled = true,
  }: {
    /**
     * The latest task event the detail holds. A workflow event (a rejected
     * request, a script run) need not change the task's revision, so a new
     * sequence refetches the view too.
     */
    eventSequence?: number;
    onTaskChanged?: () => void;
    /** False for a task without a workflow: nothing is read. */
    enabled?: boolean;
  } = {},
) {
  const api = useApi();
  const client = useQueryClient();
  const workflow = useApiQuery({ ...taskQueries.workflow(api, task.key), enabled });
  const view = workflow.data;
  const [form, setForm] = useState<OpenForm>(null);
  const [chosen, setChosen] = useState<string | null | undefined>(undefined);
  const { confirm, dialog } = useWorkflowConfirm();
  useRefetchOnChange(task, eventSequence, enabled);
  const cancellation = useMutation({
    mutationFn: () => writeTask(client, api, () => api.workflows.cancelWorkflowTask(task.key)),
    onSuccess: () => onTaskChanged?.(),
  });
  const watching = useMutation({
    mutationFn: () => api.workflows.runTaskWatch(task.key),
    onSuccess: () => void workflow.refetch(),
  });
  const write = (request: Omit<ConfirmRequest, "run" | "onCancel">, send: () => Promise<unknown>) =>
    confirmWrite(confirm, request, () => writeTask(client, api, send), onTaskChanged);
  const upgradeTo = `${view?.name ?? ""}@${view?.queue_version ?? ""}`;
  return {
    task,
    view,
    error: workflow.error ? errorText(workflow.error) : "",
    editable: task.access !== "context" && task.access !== "respond",
    form,
    chosen,
    setChosen,
    selected: view ? selectedStep(view, chosen) : null,
    cancellation,
    watching,
    upgradeTo,
    dialog,
    /** The task changed on the daemon: reload the lists and tell the owner. */
    changed: () => {
      cancellation.reset();
      void refreshTasks(client, api);
      onTaskChanged?.();
    },
    /** Refetch the view only. */
    refresh: () => {
      cancellation.reset();
      void workflow.refetch();
    },
    /** Opens a form, or closes the one that is open. */
    open: (next: OpenForm) => {
      cancellation.reset();
      watching.reset();
      setForm(form === next ? null : next);
    },
    closeForm: () => setForm(null),
    runWatch: () => {
      cancellation.reset();
      watching.mutate();
    },
    cancel: () =>
      confirm({
        title: "Cancel this task?",
        description:
          "The task closes as cancelled and its scripts stop. The workflow status stays where it stopped.",
        action: "Cancel task",
        run: () => {
          cancellation.mutate();
        },
      }),
    move: (to: string, reason: string) =>
      write(
        {
          title: `Move this task to ${to}?`,
          description:
            "The move skips outcomes, required artifacts, and checks, stops the current scripts, and cancels a pending request.",
          action: "Move task",
        },
        () => api.workflows.moveTaskWorkflow(task.key, to, reason),
      ),
    upgrade: (reason: string) =>
      write(
        {
          title: `Upgrade this task to ${upgradeTo}?`,
          description: `The task starts again at the first status of ${upgradeTo}. The current status ends, its scripts stop, and a pending request is cancelled. Artifacts stay.`,
          action: "Upgrade task",
        },
        () => api.workflows.upgradeTaskWorkflow(task.key, reason),
      ),
  };
}

export type WorkflowPanelState = ReturnType<typeof useWorkflowPanel>;

/**
 * Refetches the view when the task's revision or event sequence changes,
 * unless a mounted shared detail already refreshes it.
 */
function useRefetchOnChange(task: Task, eventSequence: number, enabled: boolean) {
  const api = useApi();
  const client = useQueryClient();
  const lastChange = useRef([task.revision, eventSequence]);
  useEffect(() => {
    if (lastChange.current[0] === task.revision && lastChange.current[1] === eventSequence) return;
    lastChange.current = [task.revision, eventSequence];
    if (!enabled) return;
    const queryKey = taskKeys.workflow(api.target, task.key);
    // A mounted shared detail owns server-wide event/write refreshes, including
    // workflow. Standalone panels still react to changes in their input props.
    const sharedDetail = client
      .getQueryCache()
      .find({ queryKey: taskKeys.detail(api.target, task.key), exact: true });
    if (sharedDetail?.isActive()) return;
    void client.cancelQueries({ queryKey }).then(() => client.invalidateQueries({ queryKey }));
  }, [client, api, task.key, task.revision, eventSequence, enabled]);
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
