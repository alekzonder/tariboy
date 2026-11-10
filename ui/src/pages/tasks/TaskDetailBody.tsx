import type { Dispatch, ReactNode, SetStateAction } from "react";
import { HelpCircle } from "lucide-react";
import { isWorkflowTask } from "@/lib/tasks";
import type { Task, WaitingFor } from "@/types";
import DismissQuestionButton from "./DismissQuestionButton";
import { taskCommentWaits } from "@/lib/taskDetail";
import TaskComments from "./TaskComments";
import { WorkflowLog, WorkflowProduce, WorkflowTrouble } from "./WorkflowPanel";
import { MarkdownContent, type MarkdownMode } from "./TaskMarkdown";
import { Banner } from "./TaskDetailControls";
import { LABEL, EMPTY } from "./panelStyles";
import { TaskDescription } from "./TaskDetailFields";
import { TaskHistorySection } from "./TaskHistorySection";
import { TaskDetailSubtasks } from "./TaskSubtasksSection";
import { TaskMetricsSection } from "./TaskMetricsSection";
import type { useTaskDetailDraft } from "./useTaskDetailDraft";
import type { WorkflowPanelState } from "./useWorkflowPanel";
import type { TaskDetailProps } from "./TaskDetailTypes";

/** A section that starts collapsed; its content stays mounted. */
function Collapsed({ label, id, children }: { label: string; id?: string; children: ReactNode }) {
  return (
    <details id={id} className="group min-w-0">
      <summary className={`${LABEL} cursor-pointer select-none py-1`}>{label}</summary>
      <div className="flex min-w-0 flex-col gap-4.5 pt-2.5">{children}</div>
    </details>
  );
}

/** The open questions of the task. The customer's question can be dismissed
 *  in any status, closed tasks included, without an answer. */
function QuestionBanner({
  task,
  openWaits,
  onTaskChanged,
}: {
  task: Task;
  openWaits: WaitingFor[];
  onTaskChanged?: () => void;
}) {
  const asksCustomer = openWaits.some((wait) => wait.expected_principal === task.customer);
  return (
    <Banner
      tone="primary"
      icon={<HelpCircle className="mt-0.5 size-3.5 [stroke-width:1.4]" />}
      text={`Waiting for an answer from ${openWaits.map((wait) => wait.expected_principal).join(", ")} — reply in the comments below.`}
      actions={
        asksCustomer && (
          <DismissQuestionButton
            taskKey={task.key}
            className="h-6 px-2.5 text-ui"
            onChanged={onTaskChanged}
          />
        )
      }
    />
  );
}

export function TaskDetailBody({
  detail,
  events,
  principals,
  onTaskChanged,
  onComment,
  draft,
  panel,
  descriptionMode,
  setDescriptionMode,
  setUploadingDescription,
  commentOrder,
  setCommentOrder,
  setCommentDirty,
  setUploadingComment,
  historyOpen,
  setHistoryOpen,
  queues,
  onOpenTask,
  subtasksStatus,
}: Pick<
  TaskDetailProps,
  | "detail"
  | "events"
  | "principals"
  | "onTaskChanged"
  | "onComment"
  | "queues"
  | "onOpenTask"
  | "subtasksStatus"
> & {
  draft: ReturnType<typeof useTaskDetailDraft>;
  /** The workflow of the task, when it has one the reader may steer. */
  panel?: WorkflowPanelState;
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
  const editable = task.access !== "respond";

  return (
    <>
      {openWaits.length > 0 && (
        <QuestionBanner task={task} openWaits={openWaits} onTaskChanged={onTaskChanged} />
      )}
      {/* What is wrong and what the workflow produced come before the task's
          own text: they are what the customer acts on. */}
      {panel && (
        <>
          <WorkflowTrouble panel={panel} />
          <WorkflowProduce panel={panel} />
        </>
      )}
      {editable ? (
        <TaskDescription
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
      <TaskDetailSubtasks {...{ detail, queues, onOpenTask, subtasksStatus }} />
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
      <TaskMetricsSection key={task.key} task={task} />
      {panel && (
        <Collapsed id="task-workflow-log" label="Workflow log">
          <WorkflowLog panel={panel} />
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
