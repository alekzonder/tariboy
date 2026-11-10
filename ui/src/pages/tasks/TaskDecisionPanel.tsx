import { useEffect } from "react";
import { FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { Task, WorkflowView } from "@/types";
import { isClosed, requiredBy, stepArtifacts } from "./workflowGraph";
import { useWorkflowAdvance } from "./useWorkflowAdvance";
import { CheckingStatus, OutcomeNotice } from "./WorkflowOutcomes";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import { artifactElementId, visibleNotice } from "./workflowShared";
import type { WorkflowPanelState } from "./useWorkflowPanel";

/**
 * What the workflow asks of the customer, under the Workflow block of the
 * task header: the pause with its ways out, or the outcomes of a customer
 * status with a message and the artifacts to read. While the task waits for
 * nobody the section is empty and takes no room.
 */
export function TaskDecisionPanel({
  panel,
  onDraftChange,
  onPendingChange,
}: {
  panel: WorkflowPanelState;
  onDraftChange: (dirty: boolean) => void;
  onPendingChange: (pending: boolean) => void;
}) {
  if (!panel.view) return null;
  return (
    <Decision
      task={panel.task}
      view={panel.view}
      panel={panel}
      onDraftChange={onDraftChange}
      onPendingChange={onPendingChange}
    />
  );
}

function Decision({
  task,
  view,
  panel,
  onDraftChange,
  onPendingChange,
}: {
  task: Task;
  view: WorkflowView;
  panel: WorkflowPanelState;
  onDraftChange: (dirty: boolean) => void;
  onPendingChange: (pending: boolean) => void;
}) {
  const advance = useWorkflowAdvance(task.key, view, panel.changed, panel.refresh);
  useEffect(() => onDraftChange(!!advance.message.trim()), [advance.message, onDraftChange]);
  useEffect(() => onPendingChange(advance.checking), [advance.checking, onPendingChange]);
  const closed = isClosed(view);
  return (
    <section
      aria-label="Workflow decision"
      className="flex min-w-0 flex-col gap-2 text-body-sm empty:hidden"
    >
      {!closed && view.waiting_on === "pause" && panel.editable && (
        <WorkflowPauseBanner
          taskKey={task.key}
          reason={view.paused_reason ?? task.workflow_paused_reason ?? ""}
          pool={view.owner.startsWith("pool:")}
          onChanged={panel.changed}
          onBusyChange={onPendingChange}
        />
      )}
      <DecisionActions
        view={view}
        editable={panel.editable}
        advance={advance}
        refresh={panel.refresh}
        onShowArtifact={(name) => showArtifact(panel, name)}
      />
    </section>
  );
}

/** Shows the artifact in the task body: the step filter gives way if it hides it. */
function showArtifact(panel: WorkflowPanelState, name: string) {
  const find = () => document.getElementById(artifactElementId(name));
  if (!find()) panel.setChosen(null);
  // The filter change renders first; the element exists on the next frame.
  requestAnimationFrame(() => {
    const element = find();
    element?.scrollIntoView?.({ block: "nearest" });
    element?.focus();
  });
}

/** What the decision block has to show, from the view and the request in flight. */
function decisionState(
  view: WorkflowView,
  editable: boolean,
  advance: ReturnType<typeof useWorkflowAdvance>,
) {
  const customer =
    editable && !isClosed(view) && view.owner === "customer" && view.waiting_on !== "pause";
  const pendingElsewhere = !advance.checking && view.last_request?.state === "pending";
  const notice = visibleNotice(advance.notice, view.last_request);
  const hasOutcomes = customer && view.outcomes.length > 0;
  const busy = advance.checking || pendingElsewhere;
  return {
    hasOutcomes,
    pendingElsewhere,
    notice,
    busy,
    // A draft message stays reachable after the owner changed under it.
    shown: hasOutcomes || !!advance.message || busy || !!notice,
  };
}

function DecisionActions({
  view,
  editable,
  advance,
  refresh,
  onShowArtifact,
}: {
  view: WorkflowView;
  editable: boolean;
  advance: ReturnType<typeof useWorkflowAdvance>;
  refresh: () => void;
  onShowArtifact: (name: string) => void;
}) {
  const { hasOutcomes, pendingElsewhere, notice, busy, shown } = decisionState(
    view,
    editable,
    advance,
  );
  if (!shown) return null;
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-md bg-primary/8 p-2.5">
      {hasOutcomes && <DecisionAsk view={view} onShowArtifact={onShowArtifact} />}
      {(hasOutcomes || !!advance.message) && (
        <Textarea
          aria-label="Outcome message"
          placeholder="Message for this decision (optional)"
          value={advance.message}
          disabled={advance.checking}
          maxLength={4096}
          onChange={(event) => advance.setMessage(event.target.value)}
          className="min-h-9 rounded-md border-0 bg-card text-body-sm md:text-body-sm"
        />
      )}
      {hasOutcomes && <DecisionButtons view={view} busy={busy} onAdvance={advance.advance} />}
      {busy && <CheckingStatus onRefresh={pendingElsewhere ? refresh : undefined} />}
      {notice && <OutcomeNotice notice={notice} onRefresh={refresh} />}
    </div>
  );
}

/** What is asked, and the artifacts to read first: those the step into the
 *  status required, then those its outcomes require. */
function DecisionAsk({
  view,
  onShowArtifact,
}: {
  view: WorkflowView;
  onShowArtifact: (name: string) => void;
}) {
  const names = [
    ...new Set([...stepArtifacts(view, view.status).enter.keys(), ...requiredBy(view).keys()]),
  ];
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
      <span className="font-medium text-primary">Waiting for your decision in {view.status}</span>
      {names.map((name) => (
        <Button
          key={name}
          type="button"
          variant="ghost"
          size="sm"
          className="h-6 gap-1 bg-card px-2 text-meta"
          onClick={() => onShowArtifact(name)}
        >
          <FileText aria-hidden="true" className="size-3" />
          {name === "plan" ? "View plan" : `View ${name}`}
        </Button>
      ))}
    </div>
  );
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
      {view.outcomes.map((outcome, index) => (
        <div key={outcome.on} className="flex flex-col">
          {/* The first outcome of the manifest is the way forward. */}
          <Button
            type="button"
            size="sm"
            variant={index === 0 ? "default" : "outline"}
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
