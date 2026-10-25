import type { StatusTone } from "@/lib/statusTone";
import type { ScriptRun, TransitionRequest, WorkflowView } from "@/lib/api";
import {
  counterText,
  currentStatus,
  currentVisit,
  inCurrentVisit,
  isClosed,
} from "./workflowGraph";

/**
 * What is wrong with a workflow task, in words a person can act on. The
 * daemon records states, verdicts, exit codes, and counters; this turns the
 * one that matters most into a title, the script's own words, and a line
 * that says what to do next. A paused task is explained by its banner, not
 * here.
 */
type ProblemKind = "failed" | "rejected" | "watch_failed" | "no_agent" | "idle" | "checking";

export interface Problem {
  kind: ProblemKind;
  tone: StatusTone;
  title: string;
  /** The script's message, or what the daemon recorded; shown as text. */
  detail?: string;
  /** What to do next. */
  advice: string;
  /** The request behind a failure or rejection; Retry acts on it. */
  request?: TransitionRequest;
  /** The run whose log explains it; shown with its log open. */
  run?: ScriptRun;
  /** `2 of 5 rejections`: how close the task is to a pause. */
  counter?: string;
}

/** The first line of a message, without the `log: ...` line a failure appends. */
function firstLine(message: string | undefined): string {
  return (message ?? "").trim().split("\n")[0]?.trim() ?? "";
}

/** The latest finished run of the open visit, newest first. */
function latestFinishedRun(view: WorkflowView): ScriptRun | undefined {
  return [...(view.runs ?? [])]
    .filter(
      (run) => run.state === "finished" && inCurrentVisit(view, run.started_at ?? run.created_at),
    )
    .sort((a, b) => b.id - a.id)[0];
}

/** The run that served a request: the check that failed or rejected it. */
function runOfRequest(view: WorkflowView, request: TransitionRequest): ScriptRun | undefined {
  return [...(view.runs ?? [])]
    .filter((run) => run.kind === "check" && run.request_id === request.id)
    .sort((a, b) => b.id - a.id)[0];
}

/** How a failed check could not run, from its run when the view has it. */
function failureDetail(
  view: WorkflowView,
  request: TransitionRequest,
  run: ScriptRun | undefined,
): string {
  if (run?.state === "interrupted") return "The daemon restarted during the check.";
  const line = firstLine(request.result_message) || firstLine(run?.message);
  const timeout = view.outcomes
    .find((outcome) => outcome.on === request.outcome)
    ?.check_details?.find((check) => check.script === run?.script)?.timeout;
  if (timeout && /timed out/.test(line)) return `${line} (the check's timeout is ${timeout})`;
  return line;
}

export function explainProblem(view: WorkflowView): Problem | null {
  if (isClosed(view) || view.waiting_on === "pause") return null;
  const pool = view.owner.startsWith("pool:");
  return (
    requestProblem(view, pool) ??
    watchProblem(view) ??
    noAgentProblem(view, pool) ??
    idleProblem(view, pool)
  );
}

/** The last request of the open visit: failed, rejected, or still checking. */
function requestProblem(view: WorkflowView, pool: boolean): Problem | null {
  const last = view.last_request;
  if (!last || !inCurrentVisit(view, last.created_at)) return null;
  if (last.state === "failed") return failedProblem(view, last);
  if (last.state === "rejected") return rejectedProblem(view, last, pool);
  if (last.state === "pending") {
    return {
      kind: "checking",
      tone: "quiet",
      title: "Checking…",
      request: last,
      advice: "The checks of the request are running; the result appears here when they finish.",
    };
  }
  return null;
}

function failedProblem(view: WorkflowView, last: TransitionRequest): Problem {
  const run = runOfRequest(view, last);
  return {
    kind: "failed",
    tone: "danger",
    request: last,
    run,
    title: `A check could not run: ${last.outcome}`,
    detail: failureDetail(view, last, run),
    advice:
      "Repeating the request unchanged will not help. Read the log, fix the cause (the script, a secret, or the environment), then Retry.",
    counter: counterText(view, "script_failures"),
  };
}

function rejectedProblem(view: WorkflowView, last: TransitionRequest, pool: boolean): Problem {
  return {
    kind: "rejected",
    tone: "attention",
    request: last,
    run: runOfRequest(view, last),
    title: `A check rejected the request: ${last.outcome}`,
    detail: last.result_message || "The check's condition does not hold.",
    advice: pool
      ? "The agent sees this in its goal and should fix what the check names, then advance again."
      : "Fix the artifact the check names, or send the task back to the agent with what to change.",
    counter: counterText(view, "rejected_requests"),
  };
}

/** The latest finished run of the open visit, when it is a failed watch. */
function watchProblem(view: WorkflowView): Problem | null {
  const run = latestFinishedRun(view);
  if (run?.kind !== "watch" || run.verdict !== "failure") return null;
  return {
    kind: "watch_failed",
    tone: "danger",
    run,
    title: "The watch script failed",
    detail: firstLine(run.message),
    advice:
      "The script keeps trying every run. Read the log and check the queue's secrets and environment; the task pauses when the failures reach the limit.",
    counter: counterText(view, "script_failures"),
  };
}

/** An open pool status that no agent holds. */
function noAgentProblem(view: WorkflowView, pool: boolean): Problem | null {
  if (!pool || view.holder || view.category !== "open") return null;
  const name = currentStatus(view)?.owner.replace(/^pool:/, "") ?? "the pool";
  return {
    kind: "no_agent",
    tone: "attention",
    title: "Waiting for an agent",
    advice: `No agent of pool ${name} can take the task: each needs to be enabled with its loop and Goal on and no other current goal. Add agents in the queue's Pools tab.`,
  };
}

/** A pool status whose holder ended iterations without an outcome. */
function idleProblem(view: WorkflowView, pool: boolean): Problem | null {
  const idle = currentVisit(view)?.idle_iterations ?? 0;
  if (!pool || idle <= 0) return null;
  return {
    kind: "idle",
    tone: "quiet",
    title: "No transition requested yet",
    advice:
      "The holder ended iterations without setting an outcome. The task pauses when the idle iterations reach the limit.",
    counter: counterText(view, "idle_iterations"),
  };
}
