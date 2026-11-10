import { useId, type ReactNode } from "react";
import { useStoredOpen } from "@/hooks/useStoredOpen";
import { storageKeys } from "@/lib/storage";
import { Meta } from "./TaskDetailControls";
import { TaskDecisionPanel } from "./TaskDecisionPanel";
import { TaskDetailChips, TaskDetailProperties } from "./TaskDetailFields";
import { TaskParentField } from "./TaskDetailParent";
import { TaskRelationsSection } from "./TaskRelationsSection";
import { SectionToggle } from "./SectionToggle.view";
import { WorkflowControls } from "./WorkflowPanel";
import type { TaskDetailProps } from "./TaskDetailTypes";
import type { useTaskDetailDraft } from "./useTaskDetailDraft";
import type { useTaskDetailRelations } from "./useTaskDetailRelations";
import type { WorkflowPanelState } from "./useWorkflowPanel";

type DetailsProps = Pick<
  TaskDetailProps,
  "detail" | "principals" | "assigneeOptions" | "onTaskChanged" | "onOpenTask"
> & {
  draft: ReturnType<typeof useTaskDetailDraft>;
  relations: ReturnType<typeof useTaskDetailRelations>;
};

/**
 * The Details block of the task header. Closed, it is the row of parameter
 * chips. Open, it adds the fields nobody changes day to day and the
 * dependencies. Its content stays mounted while closed, so an unsaved field
 * is not lost. Whether it is open is one choice for every task.
 */
function TaskDetailsBlock({
  detail,
  draft,
  relations,
  principals,
  assigneeOptions,
  onTaskChanged,
  onOpenTask,
}: DetailsProps) {
  const [open, setOpen] = useStoredOpen(storageKeys.taskDetailsOpen);
  const contentId = useId();
  const task = detail.task;
  const editable = task.access !== "respond";
  return (
    <section aria-label="Details" className="flex min-w-0 flex-col gap-2.5">
      <div className="flex min-w-0 items-start gap-2">
        <SectionToggle label="Details" open={open} onOpenChange={setOpen} controls={contentId} />
        <TaskDetailChips
          task={task}
          draft={draft}
          principals={principals}
          assigneeOptions={assigneeOptions}
          onTaskChanged={onTaskChanged}
        />
      </div>
      <div id={contentId} hidden={!open} className="flex min-w-0 flex-col gap-3 pb-1 pl-6.5">
        <dl className="grid grid-cols-2 gap-x-6 gap-y-2.25">
          <Meta label="Author" value={task.author} />
          <Meta label="Customer" value={task.customer} />
          <Meta label="Queue" value={task.queue} />
          <Meta label="Group" value={task.group} />
          <Meta label="Blocked" value={task.blocked ? "Yes" : "No"} mono={false} />
          <Meta
            label="Parent"
            value={
              <TaskParentField
                parentKey={task.parent_key}
                parent={detail.parent}
                onOpen={onOpenTask}
              />
            }
          />
        </dl>
        {editable && (
          <>
            <TaskDetailProperties draft={draft} />
            <TaskRelationsSection detail={detail} relations={relations} />
          </>
        )}
      </div>
    </section>
  );
}

/**
 * The sticky top of the task panel. Whatever the panel is scrolled to, it
 * keeps in view what the customer reads and changes most: the title, the
 * Details block, the Workflow block, and the decision the workflow asks for.
 * The one shadow here is the same rule the selected row and the active
 * segment carry. A long decision scrolls inside it rather than cover the task.
 */
export function TaskDetailSticky({
  header,
  panel,
  onDecisionDraftChange,
  onDecisionPendingChange,
  ...details
}: DetailsProps & {
  header: ReactNode;
  /** The workflow of the task; absent for a task without one. */
  panel?: WorkflowPanelState;
  onDecisionDraftChange: (dirty: boolean) => void;
  onDecisionPendingChange: (pending: boolean) => void;
}) {
  const [workflowOpen, setWorkflowOpen] = useStoredOpen(storageKeys.taskWorkflowOpen);
  return (
    <div className="sticky top-0 z-[3] max-h-[70vh] overflow-auto bg-card shadow-[var(--raise)]">
      {header}
      {details.detail.task.access !== "context" && (
        <div className="flex min-w-0 flex-col gap-2 px-3.5 pb-2.5">
          <TaskDetailsBlock {...details} />
          {panel && (
            <>
              <WorkflowControls panel={panel} open={workflowOpen} onOpenChange={setWorkflowOpen} />
              <TaskDecisionPanel
                panel={panel}
                onDraftChange={onDecisionDraftChange}
                onPendingChange={onDecisionPendingChange}
              />
            </>
          )}
        </div>
      )}
    </div>
  );
}
