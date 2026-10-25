import { useState } from "react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { WorkflowView } from "@/lib/api";
import { cn } from "@/lib/utils";
import { EMPTY, LABEL, SelectShell } from "./panelStyles";
import { useWorkflowConfirm } from "./WorkflowConfirm";
import { defaultReturnTarget, returnTargets, type ReturnTarget } from "./workflowGraph";
import { errorText } from "@/lib/errors";
import { useFollowRequest, useMounted } from "./workflowShared";

/** The transition message limit of the daemon, in characters. */
const MAX_MESSAGE = 4096;

/** The pool statuses to return to: those an outcome reaches, then those only a move reaches. */
function ReturnTargetSelect({
  targets,
  value,
  onChange,
}: {
  targets: ReturnTarget[];
  value: string;
  onChange: (status: string) => void;
}) {
  const byOutcome = targets.filter((candidate) => candidate.via === "advance");
  const byMove = targets.filter((candidate) => candidate.via === "move");
  return (
    <SelectShell
      autoFocus
      aria-label="Return to status"
      value={value}
      className="h-6.5 w-auto max-w-full text-ui md:text-ui"
      onChange={(event) => onChange(event.target.value)}
    >
      {byOutcome.length > 0 && (
        <optgroup label="By outcome">
          {byOutcome.map((candidate) => (
            <option key={candidate.status} value={candidate.status}>
              {candidate.status} ({candidate.outcome})
            </option>
          ))}
        </optgroup>
      )}
      {byMove.length > 0 && (
        <optgroup label="By move">
          {byMove.map((candidate) => (
            <option key={candidate.status} value={candidate.status}>
              {candidate.status}
              {candidate.reason === "restart" ? " (restart this step)" : ""}
            </option>
          ))}
        </optgroup>
      )}
    </SelectShell>
  );
}

/**
 * Sends the task back to an agent with what to change. Where the current
 * status declares an outcome to a pool status and the customer may declare
 * it, that outcome is used, so the history reads as the workflow intends;
 * otherwise the operator's move does it, with the message as its reason.
 * Either way the agent reads the message in its goal as the transition
 * message of the status it enters.
 */
export default function ReturnToAgent({
  taskKey,
  view,
  onChanged,
  onDone,
}: {
  taskKey: string;
  view: WorkflowView;
  /** The task went back: refetch the view and the task. */
  onChanged: () => void;
  onDone: () => void;
}) {
  const api = useApi();
  const targets = returnTargets(view);
  const [status, setStatus] = useState(() => defaultReturnTarget(view, targets)?.status ?? "");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const { checking, follow } = useFollowRequest(taskKey);
  const { confirm, dialog } = useWorkflowConfirm();
  const mountedRef = useMounted();
  const chosen = targets.find((candidate) => candidate.status === status);

  const send = (candidate: ReturnTarget) => {
    const text = message.trim();
    setError("");
    setSending(true);
    const done = (failure?: unknown) => {
      if (!mountedRef.current) return;
      setSending(false);
      if (failure !== undefined) {
        setError(errorText(failure));
        return;
      }
      onChanged();
      onDone();
    };
    if (candidate.via === "move") {
      api.workflows.moveTaskWorkflow(taskKey, candidate.status, text).then(() => done(), done);
      return;
    }
    void follow(
      () => api.workflows.advanceTask(taskKey, candidate.outcome!, text, view.status),
      (result) => {
        if (result.kind === "applied") {
          done();
          return;
        }
        done(
          result.kind === "timeout"
            ? "The checks of the outcome are still running; the task moves once they pass."
            : result.kind === "error"
              ? result.message
              : `${result.request.state}: ${result.request.result_message ?? "the outcome did not apply"}`,
        );
        if (result.kind === "result" || (result.kind === "error" && result.stale)) onChanged();
      },
    );
  };
  const submit = () => {
    if (!chosen) return;
    confirm({
      title: `Return this task to ${chosen.status}?`,
      description:
        chosen.via === "advance"
          ? `The outcome ${chosen.outcome} is declared. The agent that takes the task reads your message in its goal as the transition message.`
          : "The move skips outcomes, required artifacts, and checks, stops the current scripts, and cancels a pending request. The agent that takes the task reads your message in its goal as the transition message.",
      action: "Return to agent",
      run: () => send(chosen),
    });
  };
  const busy = sending || checking;
  return (
    <form
      className="flex min-w-0 flex-col gap-1.5 rounded-md bg-muted/50 p-2.5"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <span className={LABEL}>Return to agent</span>
      {targets.length === 0 && <span className={EMPTY}>No pool status to return to.</span>}
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <ReturnTargetSelect targets={targets} value={status} onChange={setStatus} />
        {chosen && (
          <span className={EMPTY}>
            {chosen.via === "advance" ? `declares ${chosen.outcome}` : "operator move"}
          </span>
        )}
      </div>
      <Textarea
        aria-label="Message to the agent"
        placeholder="What to change (required)"
        value={message}
        disabled={busy}
        maxLength={MAX_MESSAGE}
        onChange={(event) => setMessage(event.target.value)}
        className="min-h-16 rounded-md border-0 bg-muted text-body-sm md:text-body-sm"
      />
      <div className="flex items-center gap-1.5">
        <Button
          type="submit"
          size="sm"
          className={cn("h-6.5")}
          disabled={busy || !chosen || !message.trim()}
        >
          {checking ? "Checking…" : "Return to agent"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-6.5"
          disabled={busy}
          onClick={onDone}
        >
          Close
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-ui text-status-failed">
          {error}
        </p>
      )}
      {dialog}
    </form>
  );
}
