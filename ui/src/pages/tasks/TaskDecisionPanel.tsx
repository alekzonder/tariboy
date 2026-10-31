import { useEffect, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { Task, WorkflowView } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { MarkdownContent } from "./TaskMarkdown";
import { refreshTasks, taskQueries } from "./taskQueries";
import { isClosed, requiredBy, stepArtifacts } from "./workflowGraph";
import { useWorkflowAdvance } from "./useWorkflowAdvance";
import { CheckingStatus, OutcomeNotice } from "./WorkflowOutcomes";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import { visibleNotice } from "./workflowShared";

export function TaskDecisionPanel({
  task,
  children,
  onTaskChanged,
  onDraftChange,
  onPendingChange,
}: {
  task: Task;
  children: ReactNode;
  onTaskChanged?: () => void;
  onDraftChange: (dirty: boolean) => void;
  onPendingChange: (pending: boolean) => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  const query = useApiQuery(taskQueries.workflow(api, task.key));
  const changed = () => {
    void refreshTasks(client, api);
    onTaskChanged?.();
  };
  const refresh = () => {
    void query.refetch();
  };
  return (
    <>
      <div className="sticky top-0 z-[3] max-h-[60vh] overflow-auto bg-card shadow-[var(--raise)]">
        {children}
        {query.data ? (
          <Decision
            task={task}
            view={query.data}
            changed={changed}
            refresh={refresh}
            onDraftChange={onDraftChange}
            onPendingChange={onPendingChange}
          />
        ) : (
          <div className="px-4 pb-3 text-ui">
            {query.error ? (
              <div role="alert">
                {errorText(query.error)}{" "}
                <Button type="button" size="sm" variant="ghost" onClick={refresh}>
                  Retry
                </Button>
              </div>
            ) : (
              <span role="status">Loading workflow…</span>
            )}
          </div>
        )}
      </div>
      {query.data && !isClosed(query.data) && <Materials view={query.data} />}
    </>
  );
}

function Decision({
  task,
  view,
  changed,
  refresh,
  onDraftChange,
  onPendingChange,
}: {
  task: Task;
  view: WorkflowView;
  changed: () => void;
  refresh: () => void;
  onDraftChange: (dirty: boolean) => void;
  onPendingChange: (pending: boolean) => void;
}) {
  const advance = useWorkflowAdvance(task.key, view, changed, refresh);
  useEffect(() => onDraftChange(!!advance.message.trim()), [advance.message, onDraftChange]);
  useEffect(() => onPendingChange(advance.checking), [advance.checking, onPendingChange]);
  const closed = isClosed(view);
  const editable = task.access !== "context" && task.access !== "respond";
  return (
    <section
      aria-label="Workflow decision"
      className="flex min-w-0 flex-col gap-2 px-4 pb-3 text-body-sm"
    >
      <p>
        Status: {view.status} · Owner: {view.owner}
        {view.holder ? ` · Holder: ${view.holder}` : ""} · {decisionStatus(view)}
      </p>
      {!closed && view.waiting_on === "pause" && editable && (
        <WorkflowPauseBanner
          taskKey={task.key}
          reason={view.paused_reason ?? task.workflow_paused_reason ?? ""}
          pool={view.owner.startsWith("pool:")}
          onChanged={changed}
          onBusyChange={onPendingChange}
        />
      )}
      <DecisionActions view={view} editable={editable} advance={advance} refresh={refresh} />
    </section>
  );
}

function DecisionActions({
  view,
  editable,
  advance,
  refresh,
}: {
  view: WorkflowView;
  editable: boolean;
  advance: ReturnType<typeof useWorkflowAdvance>;
  refresh: () => void;
}) {
  const customer =
    editable && !isClosed(view) && view.owner === "customer" && view.waiting_on !== "pause";
  const pendingElsewhere = !advance.checking && view.last_request?.state === "pending";
  const notice = visibleNotice(advance.notice, view.last_request);
  const names = [
    ...new Set([...stepArtifacts(view, view.status).enter.keys(), ...requiredBy(view).keys()]),
  ];
  const hasOutcomes = customer && view.outcomes.length > 0;
  return (
    <>
      {(hasOutcomes || !!advance.message) && (
        <Textarea
          aria-label="Outcome message"
          placeholder="Message for this decision (optional)"
          value={advance.message}
          disabled={advance.checking}
          maxLength={4096}
          onChange={(event) => advance.setMessage(event.target.value)}
          className="min-h-12 rounded-md border-0 bg-muted text-body-sm md:text-body-sm"
        />
      )}
      {hasOutcomes && (
        <>
          <DecisionButtons
            view={view}
            busy={advance.checking || pendingElsewhere}
            onAdvance={advance.advance}
          />
          <MaterialLinks names={names} />
        </>
      )}
      {(advance.checking || pendingElsewhere) && (
        <CheckingStatus onRefresh={pendingElsewhere ? refresh : undefined} />
      )}
      {notice && <OutcomeNotice notice={notice} onRefresh={refresh} />}
    </>
  );
}

function MaterialLinks({ names }: { names: string[] }) {
  return (
    <div className="flex flex-wrap gap-2">
      {names.map((name) => (
        <Button
          key={name}
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            const element = document.getElementById(
              `decision-material-${name}`,
            ) as HTMLDetailsElement | null;
            if (element) {
              element.open = true;
              element.scrollIntoView?.({ block: "nearest" });
              element.querySelector("summary")?.focus();
            }
          }}
        >
          {name === "plan" ? "View plan" : `View ${name}`}
        </Button>
      ))}
    </div>
  );
}

function decisionStatus(view: WorkflowView) {
  if (isClosed(view)) return "Task closed";
  if (view.waiting_on === "pause") return "Paused";
  if (view.owner === "customer") return "Waiting for your decision";
  if (view.owner === "script") return "The workflow script continues.";
  return "The agent continues.";
}

function DecisionButtons({
  view,
  busy,
  onAdvance,
}: {
  view: WorkflowView;
  busy: boolean;
  onAdvance: (outcome: string) => void;
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {view.outcomes.map((outcome) => (
        <div key={outcome.on} className="flex flex-col">
          <Button
            type="button"
            size="sm"
            className="h-7"
            disabled={busy || !!outcome.missing?.length}
            onClick={() => onAdvance(outcome.on)}
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
  );
}

function Materials({ view }: { view: WorkflowView }) {
  const names = [
    ...new Set([...stepArtifacts(view, view.status).enter.keys(), ...requiredBy(view).keys()]),
  ];
  if (!names.length) return null;
  return (
    <div className="flex min-w-0 flex-col gap-1 px-4 pt-3">
      {names.map((name) => {
        const artifact = view.artifacts.find((item) => item.name === name);
        return (
          <details
            key={name}
            id={`decision-material-${name}`}
            aria-label={name}
            className="min-w-0 rounded-md bg-muted"
          >
            <summary className="cursor-pointer px-2.5 py-1.5 text-ui font-medium">{name}</summary>
            <div className="max-h-96 overflow-auto px-3 pb-2.5 text-body-sm">
              {artifact ? <MarkdownContent>{artifact.value}</MarkdownContent> : "No value yet"}
            </div>
          </details>
        );
      })}
    </div>
  );
}
