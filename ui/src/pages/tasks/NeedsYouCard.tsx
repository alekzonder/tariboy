import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useApiQuery } from "@/hooks/useApiQuery";
import type { PrincipalInfo, TaskDetail, WorkflowView } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { idempotencyKey } from "@/lib/ids";
import { isWorkflowTask } from "@/lib/tasks";
import { statusWords } from "@/pages/agents/work/workModel";
import { EMPTY, MONO } from "./panelStyles";
import { MarkdownContent } from "./TaskMarkdown";
import { refreshTasks, taskQueries, writeTask } from "./taskQueries";
import { formatTaskTime } from "./taskTime";
import { useWorkflowAdvance } from "./useWorkflowAdvance";
import { CheckingStatus, OutcomeNotice } from "./WorkflowOutcomes";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import { requiredBy, stepArtifacts } from "./workflowGraph";
import { visibleNotice, type Notice } from "./workflowShared";

/** The comment that asks the customer and is still unanswered. The
 *  workflow's own wait is the status, not a question. */
function openQuestion(detail: TaskDetail, customer: string) {
  const wait = detail.waiting_for.find(
    (item) =>
      !item.resolved_at &&
      item.expected_principal === customer &&
      item.requesting_principal !== "system:workflow",
  );
  return wait && detail.comments.find((comment) => comment.id === wait.requesting_comment_id);
}

/** The artifacts to review: those the step into the status required, then
 *  those its outcomes require, each with its latest value. */
function reviewArtifacts(view: WorkflowView) {
  const names = new Set([
    ...stepArtifacts(view, view.status).enter.keys(),
    ...requiredBy(view).keys(),
  ]);
  return [...names].flatMap((name) => view.artifacts.filter((artifact) => artifact.name === name));
}

const decides = (view: WorkflowView | undefined) =>
  view?.owner === "customer" && view.waiting_on !== "pause" && view.category !== "done";

/**
 * What a task waits on the user for, with the means to answer in place: the
 * question with an answer box, the status's artifacts with one button per
 * outcome, or the pause with Continue and Release. The Work tab, the task
 * drawer and Home show this one card.
 */
export default function NeedsYouCard({
  taskKey,
  detail: loaded,
  principals: knownPrincipals,
  onChanged,
  focus = false,
}: {
  taskKey: string;
  /** The task as its owner already holds it; the card reads it otherwise. */
  detail?: TaskDetail;
  principals?: PrincipalInfo | null;
  /** An answer or an outcome went through. */
  onChanged?: () => void;
  /** Scroll the card into view and focus its first button once it loads. */
  focus?: boolean;
}) {
  const api = useApi();
  const client = useQueryClient();
  const { detail, detailQuery, workflow, viewQuery, principals } = useCardReads(
    taskKey,
    loaded,
    knownPrincipals,
  );
  const task = detail?.task;
  const view = viewQuery.data;
  const changed = () => {
    void refreshTasks(client, api);
    onChanged?.();
  };
  const ref = useFocusOnLoad(focus && !!task && (!workflow || !!view));

  if (!detail || !task) {
    return (
      <CardFrame>
        <span className={EMPTY} role="status">
          {detailQuery.error ? errorText(detailQuery.error) : "Loading…"}
        </span>
      </CardFrame>
    );
  }
  const question = principals ? openQuestion(detail, principals.customer) : undefined;
  const refetchView = () => void viewQuery.refetch();
  return (
    <CardFrame cardRef={ref}>
      <div className="flex min-w-0 items-baseline gap-2">
        <span className={MONO}>{task.key}</span>
        <span className="min-w-0 flex-1 truncate text-body-sm font-medium">{task.title}</span>
        <span className="shrink-0 text-caption text-muted-foreground">
          {statusWords(task)} · {formatTaskTime(task.updated_at)}
        </span>
      </div>
      {question && (
        <div className="text-body-sm">
          <MarkdownContent>{question.body}</MarkdownContent>
        </div>
      )}
      {viewQuery.error && (
        <div role="alert" className="flex items-center gap-2 text-ui text-destructive">
          {errorText(viewQuery.error)}
          <Button size="sm" variant="outline" onClick={refetchView}>
            Retry
          </Button>
        </div>
      )}
      {view && (
        <WorkflowAsk taskKey={taskKey} view={view} assignee={task.assignee} onChanged={changed} />
      )}
      <CardActions
        taskKey={taskKey}
        view={decides(view) ? view : undefined}
        question={!!question}
        onChanged={changed}
        onRefresh={refetchView}
      />
    </CardFrame>
  );
}

/** What a workflow status asks: the pause with its decisions, or the
 *  customer's decision with the artifacts to review. */
function WorkflowAsk({
  taskKey,
  view,
  assignee,
  onChanged,
}: {
  taskKey: string;
  view: WorkflowView;
  assignee: string;
  onChanged: () => void;
}) {
  if (view.waiting_on === "pause") {
    return (
      <WorkflowPauseBanner
        taskKey={taskKey}
        reason={view.paused_reason ?? ""}
        pool={view.owner.startsWith("pool")}
        onChanged={onChanged}
      />
    );
  }
  if (!decides(view)) return null;
  return (
    <>
      <p className="text-body-sm">
        {assignee ? assignee.replace(/^agent:/, "") : "The workflow"} waits for your decision in{" "}
        {view.status}.
      </p>
      <ArtifactViewer view={view} />
    </>
  );
}

/** The task, its workflow view and the principals: each read only when the
 *  owner did not pass it. */
function useCardReads(
  taskKey: string,
  loaded: TaskDetail | undefined,
  knownPrincipals: PrincipalInfo | null | undefined,
) {
  const api = useApi();
  const detailQuery = useApiQuery({ ...taskQueries.detail(api, taskKey), enabled: !loaded });
  const detail = loaded ?? detailQuery.data;
  const workflow = !!detail && isWorkflowTask(detail.task);
  const viewQuery = useApiQuery({ ...taskQueries.workflow(api, taskKey), enabled: workflow });
  const principalsQuery = useApiQuery({
    ...taskQueries.principals(api),
    enabled: !knownPrincipals,
  });
  return {
    detail,
    detailQuery,
    workflow,
    viewQuery,
    principals: knownPrincipals ?? principalsQuery.data,
  };
}

function CardFrame({
  children,
  cardRef,
}: {
  children: React.ReactNode;
  cardRef?: React.Ref<HTMLElement>;
}) {
  return (
    <section
      ref={cardRef}
      aria-label="Needs you"
      className="flex min-w-0 flex-col gap-2 rounded-md border border-primary/25 bg-card px-3 py-2.5"
    >
      {children}
    </section>
  );
}

function useFocusOnLoad(ready: boolean) {
  const ref = useRef<HTMLElement>(null);
  const done = useRef(false);
  useEffect(() => {
    if (!ready || done.current || !ref.current) return;
    done.current = true;
    ref.current.scrollIntoView?.({ block: "nearest" });
    ref.current.querySelector<HTMLElement>("button:not(:disabled), textarea")?.focus();
  }, [ready]);
  return ref;
}

/** Each artifact to review, the first one open. */
function ArtifactViewer({ view }: { view: WorkflowView }) {
  const artifacts = reviewArtifacts(view);
  if (artifacts.length === 0) return null;
  return (
    <div className="flex min-w-0 flex-col gap-1">
      {artifacts.map((artifact, index) => (
        <details key={artifact.name} open={index === 0} className="min-w-0 rounded-md bg-muted">
          <summary className="cursor-pointer px-2.5 py-1.5 text-ui font-medium">
            {artifact.name}
          </summary>
          <div className="max-h-96 overflow-auto px-3 pb-2.5 text-body-sm">
            <MarkdownContent>{artifact.value}</MarkdownContent>
          </div>
        </details>
      ))}
    </div>
  );
}

/** The message box with the outcome buttons of a customer status and the
 *  answer buttons of a question. */
function CardActions({
  taskKey,
  view,
  question,
  onChanged,
  onRefresh,
}: {
  taskKey: string;
  view?: WorkflowView;
  question: boolean;
  onChanged: () => void;
  onRefresh: () => void;
}) {
  const advance = useWorkflowAdvance(taskKey, { status: view?.status ?? "" }, onChanged, onRefresh);
  const answer = useAnswer(taskKey, onChanged);
  if (!view && !question) return null;
  const { message, setMessage, checking } = advance;
  const last = view?.last_request;
  const shown = visibleNotice(advance.notice, last);
  const pendingElsewhere = !checking && last?.state === "pending";
  const busy = checking || pendingElsewhere || answer.busy;
  const send = (text: string) => void answer.send(text).then((sent) => sent && setMessage(""));
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Textarea
        aria-label="Message"
        placeholder="Message (optional)"
        value={message}
        disabled={busy}
        maxLength={4096}
        onChange={(event) => setMessage(event.target.value)}
        className="min-h-12 rounded-md border-0 bg-muted text-body-sm md:text-body-sm"
      />
      <div className="flex flex-wrap gap-1.5">
        {view && <OutcomeButtons view={view} busy={busy} onAdvance={advance.advance} />}
        {question && (
          <AnswerButtons message={message} busy={busy} secondary={!!view} onSend={send} />
        )}
      </div>
      <ActionStatus
        checking={checking}
        pendingElsewhere={pendingElsewhere}
        notice={shown}
        error={answer.error}
        onRefresh={onRefresh}
      />
    </div>
  );
}

/** A request still checking, how the last one settled, or a failed answer. */
function ActionStatus({
  checking,
  pendingElsewhere,
  notice,
  error,
  onRefresh,
}: {
  checking: boolean;
  pendingElsewhere: boolean;
  notice: Notice | null;
  error: string;
  onRefresh: () => void;
}) {
  return (
    <>
      {(checking || pendingElsewhere) && (
        <CheckingStatus onRefresh={pendingElsewhere ? onRefresh : undefined} />
      )}
      {notice && <OutcomeNotice notice={notice} onRefresh={onRefresh} />}
      {error && (
        <p role="alert" className="text-ui text-destructive">
          {error}
        </p>
      )}
    </>
  );
}

/** One button per outcome, "<outcome> → <status>"; one that still misses an
 *  artifact is disabled and names it. */
function OutcomeButtons({
  view,
  busy,
  onAdvance,
}: {
  view: WorkflowView;
  busy: boolean;
  onAdvance: (outcome: string) => void;
}) {
  return view.outcomes.map((outcome) => {
    const missing = outcome.missing ?? [];
    return (
      <Button
        key={outcome.on}
        type="button"
        size="sm"
        className="h-7"
        disabled={busy || missing.length > 0}
        title={missing.length ? `needs: ${missing.join(", ")}` : undefined}
        onClick={() => onAdvance(outcome.on)}
      >
        {outcome.on} → {outcome.to}
      </Button>
    );
  });
}

/** Send the message as the answer, or Ok while the box is empty. */
function AnswerButtons({
  message,
  busy,
  secondary,
  onSend,
}: {
  message: string;
  busy: boolean;
  secondary: boolean;
  onSend: (text: string) => void;
}) {
  const text = message.trim();
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant={secondary ? "secondary" : "default"}
        className="h-7"
        disabled={busy || !text}
        onClick={() => onSend(text)}
      >
        Send answer
      </Button>
      {!text && (
        <Button
          type="button"
          size="sm"
          variant="secondary"
          className="h-7"
          disabled={busy}
          onClick={() => onSend("Ok")}
        >
          Reply Ok to the question
        </Button>
      )}
    </>
  );
}

/** An answer is a customer comment: the daemon resolves the wait it answers. */
function useAnswer(taskKey: string, onChanged: () => void) {
  const api = useApi();
  const client = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const send = async (text: string) => {
    setBusy(true);
    setError("");
    try {
      await writeTask(client, api, () =>
        api.tasks.addTaskComment(taskKey, text, idempotencyKey("answer")),
      );
      onChanged();
      return true;
    } catch (failure) {
      setError(errorText(failure));
      return false;
    } finally {
      setBusy(false);
    }
  };
  return { busy, error, send };
}
