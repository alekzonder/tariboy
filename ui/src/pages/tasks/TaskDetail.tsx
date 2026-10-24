import { type CSSProperties, useState } from "react";
import { HelpCircle } from "lucide-react";
import { isWorkflowTask } from "@/lib/tasks";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import TaskComments from "./TaskComments";
import { WorkflowPanel } from "./WorkflowPanel";
import { MarkdownContent, type MarkdownMode } from "./TaskMarkdown";
import { Banner, Meta } from "./TaskDetailControls";
import { LABEL, EMPTY } from "./panelStyles";
import { TaskDetailHeader } from "./TaskDetailHeader";
import { TaskDetailFields } from "./TaskDetailFields";
import { TaskRelationsSection } from "./TaskRelationsSection";
import { TaskHistorySection } from "./TaskHistorySection";
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
}: TaskDetailProps) {
  const task = detail.task;
  const workflow = isWorkflowTask(task);
  const draft = useTaskDetailDraft(task, onSave);
  const relations = useTaskDetailRelations(onAddRelation, onDeleteRelation);
  const [commentDirty, setCommentDirty] = useState(false);
  const [descriptionMode, setDescriptionMode] = useState<MarkdownMode>("rich");
  const [uploadingDescription, setUploadingDescription] = useState(false);
  const [uploadingComment, setUploadingComment] = useState(false);
  const [commentOrder, setCommentOrder] = useState<"newest" | "oldest">("newest");
  const [historyOpen, setHistoryOpen] = useState(true);
  const [transferOpen, setTransferOpen] = useState(false);
  const [requeueOpen, setRequeueOpen] = useState(false);
  const pending = draft.saving || uploadingDescription || uploadingComment;
  const hasDraft = draft.dirty || commentDirty || Boolean(relations.relationTarget.trim());
  const { returnFocus, initialFocusRef, confirmClose, setConfirmClose, close } = useTaskDetailClose(
    hasDraft,
    pending,
    onClose,
  );
  const comments = commentOrder === "newest" ? [...detail.comments].reverse() : detail.comments;
  // The workflow's own wait (a customer status or a pause) is answered in the
  // workflow panel, never by a comment, so neither the banner nor a comment
  // chip asks for one.
  const commentWaits = detail.waiting_for.filter(
    (wait) => !(workflow && wait.requesting_principal === "system:workflow"),
  );
  const openWaits = commentWaits.filter((wait) => !wait.resolved_at);
  // The workflow panel refetches when a new event arrives.
  const eventSequence = Math.max(
    events.reduce((latest, event) => Math.max(latest, event.sequence), 0),
    liveSequence ?? 0,
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
          initialFocusRef.current?.focus();
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
        <div className="task-detail-panel">
          {/* Sticky: whatever the panel is scrolled to, the key, the title and the
          status stay in view. The one shadow here is the same rule the selected
          row and the active segment carry. */}
          <TaskDetailHeader
            task={task}
            pending={pending}
            title={draft.title}
            save={draft.save}
            close={close}
            initialFocusRef={initialFocusRef}
            onRequeue={() => setRequeueOpen(true)}
            onTransfer={() => setTransferOpen(true)}
          />
          <div className="task-detail-body flex min-w-0 flex-col gap-4.5 px-4 pt-3.5 pb-7">
            <TaskDetailDraftAlerts
              task={task}
              draft={draft}
              confirmClose={confirmClose}
              setConfirmClose={setConfirmClose}
              onClose={onClose}
            />
            {task.access === "context" ? (
              <div className="tasks-empty">
                Context ancestor — open a visible descendant to edit.
              </div>
            ) : (
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
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
