import type { Dispatch, ReactNode, SetStateAction } from "react";
import { HelpCircle } from "lucide-react";
import type { Task, WaitingFor } from "@/lib/api";
import { needsYouTask } from "@/lib/needsYou";
import { isWorkflowTask } from "@/lib/tasks";
import { taskCommentWaits, taskEventSequence } from "@/lib/taskDetail";
import TaskComments from "./TaskComments";
import NeedsYouCard from "./NeedsYouCard";
import { WorkflowPanel } from "./WorkflowPanel";
import { MarkdownContent, type MarkdownMode } from "./TaskMarkdown";
import { Banner, Meta } from "./TaskDetailControls";
import { LABEL, EMPTY } from "./panelStyles";
import { TaskDetailFields, TaskDetailProperties } from "./TaskDetailFields";
import { TaskRelationsSection } from "./TaskRelationsSection";
import { TaskHistorySection } from "./TaskHistorySection";
import type { useTaskDetailDraft } from "./useTaskDetailDraft";
import type { useTaskDetailRelations } from "./useTaskDetailRelations";
import type { TaskDetailProps } from "./TaskDetailTypes";

/** A section that starts collapsed unless `open`; its content stays mounted. */
function Collapsed({
  label,
  open,
  children,
}: {
  label: string;
  open?: boolean;
  children: ReactNode;
}) {
  return (
    <details open={open} className="group min-w-0">
      <summary className={`${LABEL} cursor-pointer select-none py-1`}>{label}</summary>
      <div className="flex min-w-0 flex-col gap-4.5 pt-2.5">{children}</div>
    </details>
  );
}

/** The task waits for the customer: a workflow decision, a pause, or an open
 *  question addressed to them. */
function waitsOnCustomer(task: Task, waits: WaitingFor[], customer: string | undefined): boolean {
  return (
    needsYouTask(task) || (!!customer && waits.some((wait) => wait.expected_principal === customer))
  );
}

export function TaskDetailBody({
  detail,
  events,
  liveSequence,
  principals,
  assigneeOptions,
  onTaskChanged,
  onComment,
  draft,
  relations,
  descriptionMode,
  setDescriptionMode,
  setUploadingDescription,
  commentOrder,
  setCommentOrder,
  setCommentDirty,
  setUploadingComment,
  historyOpen,
  setHistoryOpen,
}: Pick<
  TaskDetailProps,
  | "detail"
  | "events"
  | "liveSequence"
  | "principals"
  | "assigneeOptions"
  | "onTaskChanged"
  | "onComment"
> & {
  draft: ReturnType<typeof useTaskDetailDraft>;
  relations: ReturnType<typeof useTaskDetailRelations>;
  descriptionMode: MarkdownMode;
  setDescriptionMode: (mode: MarkdownMode) => void;
  setUploadingDescription: Dispatch<SetStateAction<boolean>>;
  commentOrder: "newest" | "oldest";
  setCommentOrder: (order: "newest" | "oldest") => void;
  setCommentDirty: (dirty: boolean) => void;
  setUploadingComment: (uploading: boolean) => void;
  historyOpen: boolean;
  setHistoryOpen: (open: boolean) => void;
}) {
  const task = detail.task;
  const workflow = isWorkflowTask(task);
  const comments = commentOrder === "newest" ? [...detail.comments].reverse() : detail.comments;
  const commentWaits = taskCommentWaits(detail.waiting_for, workflow);
  const openWaits = commentWaits.filter((wait) => !wait.resolved_at);
  const eventSequence = taskEventSequence(events, liveSequence);
  const editable = task.access !== "respond";
  const pinned = editable && waitsOnCustomer(task, openWaits, principals?.customer);

  return (
    <>
      {/* The decision first: when the task waits on the user, the card that
        answers it sits above everything else. */}
      {pinned && (
        <NeedsYouCard
          taskKey={task.key}
          detail={detail}
          principals={principals}
          onChanged={onTaskChanged}
        />
      )}
      {!pinned && openWaits.length > 0 && (
        <Banner
          tone="primary"
          icon={<HelpCircle className="mt-0.5 size-3.5 [stroke-width:1.4]" />}
          text={`Waiting for an answer from ${openWaits.map((wait) => wait.expected_principal).join(", ")} — reply in the comments below.`}
        />
      )}
      {editable ? (
        <TaskDetailFields
          draft={draft}
          descriptionMode={descriptionMode}
          setDescriptionMode={setDescriptionMode}
          setUploadingDescription={setUploadingDescription}
        />
      ) : (
        <div className="flex min-w-0 flex-col gap-2">
          {/* No second title here: the sticky header already carries it, and
            it is the panel's only 600. */}
          <span className={LABEL}>Description</span>
          <div className="rounded-md bg-muted px-3 py-2.75">
            <MarkdownContent>{task.description}</MarkdownContent>
          </div>
          <p className={EMPTY}>Response access — comments only.</p>
        </div>
      )}
      <TaskComments
        comments={comments}
        waits={commentWaits}
        principals={principals}
        assignee={task.assignee}
        formFirst={commentOrder === "newest"}
        order={commentOrder}
        onOrderChange={setCommentOrder}
        onComment={onComment}
        onDirtyChange={setCommentDirty}
        onUploadingChange={setUploadingComment}
      />
      <Collapsed label="Details">
        <dl className="grid grid-cols-2 gap-x-6 gap-y-2.25">
          <Meta label="Author" value={task.author} />
          <Meta label="Customer" value={task.customer} />
          <Meta label="Queue" value={task.queue} />
          <Meta label="Group" value={task.group} />
          <Meta label="Blocked" value={task.blocked ? "Yes" : "No"} mono={false} />
          <Meta label="Parent" value={task.parent_key} />
        </dl>
        {editable && (
          <>
            <TaskDetailProperties
              task={task}
              draft={draft}
              principals={principals}
              assigneeOptions={assigneeOptions}
              onTaskChanged={onTaskChanged}
            />
            <TaskRelationsSection detail={detail} relations={relations} />
          </>
        )}
      </Collapsed>
      {/* In place of the status select: the workflow changes the status. */}
      {editable && workflow && (
        <Collapsed label="Workflow" open={!!task.workflow_attention}>
          <WorkflowPanel
            task={task}
            eventSequence={eventSequence}
            onTaskChanged={onTaskChanged}
            decisionsElsewhere={pinned}
          />
        </Collapsed>
      )}
      <TaskHistorySection
        events={events}
        historyOpen={historyOpen}
        setHistoryOpen={setHistoryOpen}
      />
    </>
  );
}
