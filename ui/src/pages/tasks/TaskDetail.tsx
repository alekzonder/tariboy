import { type CSSProperties, useState } from "react";
import { isWorkflowTask } from "@/lib/tasks";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import type { MarkdownMode } from "./TaskMarkdown";
import { TaskDetailHeader } from "./TaskDetailHeader";
import { taskEventSequence } from "@/lib/taskDetail";
import { TaskDetailSticky } from "./TaskDetailSticky";
import { useWorkflowPanel } from "./useWorkflowPanel";
import { TaskDetailBody } from "./TaskDetailBody";
import { TaskDetailDialogs, TaskDetailDraftAlerts } from "./TaskDetailDialogs";
import { useTaskDetailDraft } from "./useTaskDetailDraft";
import { useTaskDetailRelations } from "./useTaskDetailRelations";
import { useTaskDetailClose } from "./useTaskDetailClose";
import type { TaskDetailProps } from "./TaskDetailTypes";

export { TaskDetailLoading } from "./TaskDetailLoading";

export default function TaskDetail({
  detail,
  events,
  liveSequence,
  principals,
  width,
  resizeHandle,
  onClose,
  onSave,
  onComment,
  onAddRelation,
  onDeleteRelation,
  onTransfer,
  onRequeue,
  onTaskChanged,
  assigneeOptions,
  onOpenTask,
  queues,
  focusTitle,
  subtasksStatus,
}: TaskDetailProps) {
  const task = detail.task;
  const workflow = isWorkflowTask(task);
  const draft = useTaskDetailDraft(task, onSave);
  const relations = useTaskDetailRelations(onAddRelation, onDeleteRelation);
  const [commentDirty, setCommentDirty] = useState(false);
  const [decisionDirty, setDecisionDirty] = useState(false);
  const [decisionPending, setDecisionPending] = useState(false);
  const [descriptionMode, setDescriptionMode] = useState<MarkdownMode>("rich");
  const [uploadingDescription, setUploadingDescription] = useState(false);
  const [uploadingComment, setUploadingComment] = useState(false);
  const [commentOrder, setCommentOrder] = useState<"newest" | "oldest">("newest");
  const [historyOpen, setHistoryOpen] = useState(false);
  const [transferOpen, setTransferOpen] = useState(false);
  const [requeueOpen, setRequeueOpen] = useState(false);
  const pending = draft.saving || uploadingDescription || uploadingComment || decisionPending;
  const hasDraft =
    draft.dirty || commentDirty || decisionDirty || Boolean(relations.relationTarget.trim());
  const closing = useTaskDetailClose(hasDraft, pending, onClose, onOpenTask);
  // One read of the workflow serves the header controls and the body.
  const steered = workflow && task.access !== "context" && task.access !== "respond";
  const workflowPanel = useWorkflowPanel(task, {
    eventSequence: taskEventSequence(events, liveSequence),
    onTaskChanged,
    enabled: steered,
  });
  const panel = steered ? workflowPanel : undefined;
  const { returnFocus, initialFocusRef, titleRef, close, requestOpen } = closing;
  const header = (
    <TaskDetailHeader
      {...{ task, queues, titleRef }}
      parent={detail.parent}
      onOpenParent={requestOpen}
      pending={pending}
      title={draft.title}
      setTitle={draft.setTitle}
      dirty={draft.dirty}
      save={draft.save}
      close={close}
      initialFocusRef={initialFocusRef}
      onRequeue={() => setRequeueOpen(true)}
      onTransfer={() => setTransferOpen(true)}
    />
  );

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      {/* Without the resize grip the panel would land in the column the grip
        owns, 9px wide, and the drawer would read as an empty sheet. */}
      <DialogContent
        className="task-detail-dialog bg-card ring-0"
        showCloseButton={false}
        aria-describedby={undefined}
        /* The sheet dims the whole window — topbar and sidebar included — with the
         theme's own foreground, so both read as one tone behind it. */
        overlayClassName="bg-[color-mix(in_oklab,var(--foreground)_7%,transparent)] supports-backdrop-filter:backdrop-blur-none"
        style={{ "--tasks-detail-width": `${width}px` } as CSSProperties}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          (focusTitle ? titleRef : initialFocusRef).current?.focus();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (returnFocus?.isConnected) returnFocus.focus();
        }}
      >
        {resizeHandle}
        <TaskDetailDialogs
          task={task}
          workflow={workflow}
          transferOpen={transferOpen}
          setTransferOpen={setTransferOpen}
          requeueOpen={requeueOpen}
          setRequeueOpen={setRequeueOpen}
          onTransfer={onTransfer}
          onRequeue={onRequeue}
        />
        <div className="task-detail-panel relative min-h-0 min-w-0 overflow-auto bg-card">
          <TaskDetailSticky
            header={header}
            panel={panel}
            onDecisionDraftChange={setDecisionDirty}
            onDecisionPendingChange={setDecisionPending}
            {...{ detail, draft, relations, principals, assigneeOptions, onTaskChanged }}
            onOpenTask={requestOpen}
          />
          <div className="flex min-w-0 flex-col gap-4.5 px-4 pt-3.5 pb-7">
            <TaskDetailDraftAlerts
              task={task}
              draft={draft}
              confirmClose={closing.confirmClose}
              setConfirmClose={closing.setConfirmClose}
              onClose={closing.discard}
              pending={pending}
            />
            {task.access === "context" ? (
              <div className="px-5 py-10 text-center text-body text-muted-foreground">
                Context ancestor — open a visible descendant to edit.
              </div>
            ) : (
              <TaskDetailBody
                detail={detail}
                events={events}
                principals={principals}
                onTaskChanged={onTaskChanged}
                onComment={onComment}
                draft={draft}
                panel={panel}
                descriptionMode={descriptionMode}
                setDescriptionMode={setDescriptionMode}
                setUploadingDescription={setUploadingDescription}
                commentOrder={commentOrder}
                setCommentOrder={setCommentOrder}
                setCommentDirty={setCommentDirty}
                setUploadingComment={setUploadingComment}
                historyOpen={historyOpen}
                setHistoryOpen={setHistoryOpen}
                {...{ queues, subtasksStatus }}
                onOpenTask={requestOpen}
              />
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
