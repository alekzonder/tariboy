import type { Dispatch, SetStateAction } from "react";
import { HelpCircle } from "lucide-react";
import { isWorkflowTask } from "@/lib/tasks";
import { taskCommentWaits, taskEventSequence } from "@/lib/taskDetail";
import TaskComments from "./TaskComments";
import { WorkflowPanel } from "./WorkflowPanel";
import { MarkdownContent, type MarkdownMode } from "./TaskMarkdown";
import { Banner, Meta } from "./TaskDetailControls";
import { LABEL, EMPTY } from "./panelStyles";
import { TaskDetailFields } from "./TaskDetailFields";
import { TaskRelationsSection } from "./TaskRelationsSection";
import { TaskHistorySection } from "./TaskHistorySection";
import type { useTaskDetailDraft } from "./useTaskDetailDraft";
import type { useTaskDetailRelations } from "./useTaskDetailRelations";
import type { TaskDetailProps } from "./TaskDetailTypes";

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

  return (
    <>
      {/* One banner at a time, and only for what is failing or waiting on a
        person — open, in progress and done speak through the status pill. */}
      {openWaits.length > 0 && (
        <Banner
          tone="primary"
          icon={<HelpCircle className="mt-0.5 size-3.5 [stroke-width:1.4]" />}
          text={`Waiting for an answer from ${openWaits.map((wait) => wait.expected_principal).join(", ")} — reply in the comments below.`}
        />
      )}
      <dl className="grid grid-cols-2 gap-x-6 gap-y-2.25">
        <Meta label="Author" value={task.author} />
        <Meta label="Customer" value={task.customer} />
        <Meta label="Queue" value={task.queue} />
        <Meta label="Group" value={task.group} />
        <Meta label="Blocked" value={task.blocked ? "Yes" : "No"} mono={false} />
        <Meta label="Parent" value={task.parent_key} />
      </dl>
      {task.access !== "respond" ? (
        <>
          <TaskDetailFields
            task={task}
            draft={draft}
            principals={principals}
            assigneeOptions={assigneeOptions}
            onTaskChanged={onTaskChanged}
            descriptionMode={descriptionMode}
            setDescriptionMode={setDescriptionMode}
            setUploadingDescription={setUploadingDescription}
          />
          {/* In place of the status select: the workflow changes the status. */}
          {workflow && (
            <WorkflowPanel
              task={task}
              eventSequence={eventSequence}
              onTaskChanged={onTaskChanged}
            />
          )}
          <TaskRelationsSection detail={detail} relations={relations} />
        </>
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
      <TaskHistorySection
        events={events}
        historyOpen={historyOpen}
        setHistoryOpen={setHistoryOpen}
      />
    </>
  );
}
