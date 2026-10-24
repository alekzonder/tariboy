import { TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import type { WorkflowScriptRun, WorkflowView } from "@/lib/api";
import { TONE_TEXT } from "@/lib/statusTone";
import { cn } from "@/lib/utils";
import { DANGER_FILL, MONO, PRIMARY_FILL } from "./panelStyles";
import RunRow from "./RunRow";
import { explainProblem, type Problem } from "./explainWorkflowProblem";
import { followNoticeText, useFollowRequest, type FollowResult } from "./workflowShared";

const FILL: Partial<Record<Problem["tone"], string>> = {
  danger: DANGER_FILL,
  attention: PRIMARY_FILL,
};

/**
 * What is wrong with the task and what to do about it, above everything else
 * in the panel: a failed or rejected request with its script's words and
 * log, a failing watch, a pool with nobody free, or a holder that is idling.
 * A failed request has Retry, which runs its checks again once the cause is
 * fixed. A pause is explained by its own banner.
 */
export default function WorkflowProblem({
  taskKey,
  view,
  onChanged,
  onRefresh,
}: {
  taskKey: string;
  view: WorkflowView;
  /** The transition applied: refetch the view and the task. */
  onChanged: () => void;
  /** Refetch the view only. */
  onRefresh: () => void;
}) {
  const api = useApi();
  const problem = explainProblem(view);
  const { checking, follow } = useFollowRequest(taskKey);
  const [notice, setNotice] = useState<FollowResult | null>(null);
  if (!problem || problem.kind === "checking") return null;
  const retry = (id: number) => {
    setNotice(null);
    void follow(
      () => api.workflows.retryTransitionRequest(taskKey, id),
      (result) => {
        if (result.kind === "applied") {
          onChanged();
          return;
        }
        setNotice(result);
        if (result.kind === "result" || (result.kind === "error" && result.stale)) onRefresh();
      },
    );
  };
  const fill = FILL[problem.tone];
  const alert = problem.tone === "danger" || problem.tone === "attention";
  return (
    <section
      role={alert ? "alert" : "status"}
      data-problem={problem.kind}
      className={cn(
        "flex min-w-0 flex-col gap-1.5 rounded-md px-2.5 py-2",
        fill ?? "bg-muted/50 text-muted-foreground",
      )}
    >
      <div className="flex min-w-0 items-start gap-2">
        {alert && (
          <TriangleAlert
            aria-hidden="true"
            className="mt-0.5 size-3.5 shrink-0 [stroke-width:1.6]"
          />
        )}
        <ProblemText problem={problem} filled={!!fill} />
        {problem.kind === "failed" && problem.request && view.waiting_on !== "pause" && (
          <RetryButton checking={checking} onRetry={() => retry(problem.request!.id)} />
        )}
      </div>
      {notice && notice.kind !== "applied" && (
        <p role="status" className="text-ui">
          {followNoticeText(notice)}
        </p>
      )}
      {problem.run && (
        <ProblemRun taskKey={taskKey} run={problem.run} autoLoad={problem.kind !== "rejected"} />
      )}
    </section>
  );
}

function RetryButton({ checking, onRetry }: { checking: boolean; onRetry: () => void }) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-6 shrink-0 px-2 text-ui"
      disabled={checking}
      onClick={onRetry}
    >
      {checking ? "Checking…" : "Retry"}
    </Button>
  );
}

/** The run that explains the problem, with its log. */
function ProblemRun({
  taskKey,
  run,
  autoLoad,
}: {
  taskKey: string;
  run: WorkflowScriptRun;
  autoLoad: boolean;
}) {
  const api = useApi();
  return (
    <ul className="flex min-w-0 flex-col text-foreground">
      <RunRow
        key={run.id}
        autoLoad={autoLoad}
        head={
          <>
            <span className={cn(MONO, "font-medium")}>{run.kind}</span>
            <span className={cn(MONO, "min-w-0 truncate")} title={run.script}>
              {run.script}
            </span>
          </>
        }
        state={run.state}
        verdict={run.verdict}
        exitCode={run.exit_code}
        startedAt={run.started_at ?? run.created_at}
        finishedAt={run.finished_at}
        going={run.state === "pending" || run.state === "running"}
        readLog={() => api.workflows.getTaskScriptRunLog(taskKey, run.id, undefined)}
      />
    </ul>
  );
}

/** The title, counter, the script's words, and what to do next. */
function ProblemText({ problem, filled }: { problem: Problem; filled: boolean }) {
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
        <span className="text-body-sm font-medium">{problem.title}</span>
        {problem.counter && <span className={cn(MONO, "opacity-80")}>{problem.counter}</span>}
      </div>
      {problem.detail && (
        <pre className="min-w-0 font-mono text-meta whitespace-pre-wrap break-words">
          {problem.detail}
        </pre>
      )}
      <p
        className={cn(
          "min-w-0 text-ui leading-[1.45] text-pretty",
          filled ? "opacity-90" : TONE_TEXT.quiet,
        )}
      >
        {problem.advice}
      </p>
    </div>
  );
}
