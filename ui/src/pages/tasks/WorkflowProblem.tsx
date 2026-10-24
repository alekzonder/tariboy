import { TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import type { WorkflowView } from "@/lib/api";
import { TONE_TEXT } from "@/lib/statusTone";
import { cn } from "@/lib/utils";
import { DANGER_FILL, MONO, PRIMARY_FILL } from "./panelStyles";
import RunRow from "./RunRow";
import { explainProblem, type Problem } from "./explainWorkflowProblem";
import { useFollowRequest, type FollowResult } from "./workflowShared";

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
        "flex min-w-0 flex-col gap-1.5 rounded-[8px] px-2.5 py-2",
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
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
            <span className="text-[12.5px] font-medium">{problem.title}</span>
            {problem.counter && <span className={cn(MONO, "opacity-80")}>{problem.counter}</span>}
          </div>
          {problem.detail && (
            <pre className="min-w-0 font-mono text-[11.5px] whitespace-pre-wrap break-words">
              {problem.detail}
            </pre>
          )}
          <p
            className={cn(
              "min-w-0 text-[12px] leading-[1.45] text-pretty",
              fill ? "opacity-90" : TONE_TEXT.quiet,
            )}
          >
            {problem.advice}
          </p>
        </div>
        {problem.kind === "failed" && problem.request && view.waiting_on !== "pause" && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-6 shrink-0 px-2 text-[12px]"
            disabled={checking}
            onClick={() => retry(problem.request!.id)}
          >
            {checking ? "Checking…" : "Retry"}
          </Button>
        )}
      </div>
      {notice && notice.kind !== "applied" && (
        <p role="status" className="text-[12px]">
          {notice.kind === "timeout"
            ? "Still checking — the result appears here when the checks finish."
            : notice.kind === "error"
              ? `${notice.label}: ${notice.message}`
              : `${notice.request.state}: ${notice.request.result_message ?? ""}`}
        </p>
      )}
      {problem.run && (
        <ul className="flex min-w-0 flex-col text-foreground">
          <RunRow
            key={problem.run.id}
            autoLoad={problem.kind !== "rejected"}
            head={
              <>
                <span className={cn(MONO, "font-medium")}>{problem.run.kind}</span>
                <span className={cn(MONO, "min-w-0 truncate")} title={problem.run.script}>
                  {problem.run.script}
                </span>
              </>
            }
            state={problem.run.state}
            verdict={problem.run.verdict}
            exitCode={problem.run.exit_code}
            startedAt={problem.run.started_at ?? problem.run.created_at}
            finishedAt={problem.run.finished_at}
            going={problem.run.state === "pending" || problem.run.state === "running"}
            readLog={() => api.workflows.getTaskScriptRunLog(taskKey, problem.run!.id, undefined)}
          />
        </ul>
      )}
    </section>
  );
}
