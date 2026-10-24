import { AlertCircle } from "lucide-react"
import type { Task } from "@/lib/api"
import { cn } from "@/lib/utils"
import TaskTransferDialog from "./TaskTransferDialog"
import TaskRequeueDialog from "./TaskRequeueDialog"
import { MarkdownContent } from "./TaskMarkdown"
import { Banner, BannerButton } from "./TaskDetailControls"
import { DANGER_FILL, LABEL, MONO } from "./panelStyles"
import type { TaskDetailProps } from "./TaskDetailTypes"
import type { useTaskDetailDraft } from "./useTaskDetailDraft"

export function TaskDetailDialogs({ task, workflow, transferOpen, setTransferOpen,
  requeueOpen, setRequeueOpen, onTransfer, onRequeue,
}: Pick<TaskDetailProps, "onTransfer" | "onRequeue"> & {
  task: Task
  workflow: boolean
  transferOpen: boolean
  setTransferOpen: (open: boolean) => void
  requeueOpen: boolean
  setRequeueOpen: (open: boolean) => void
}) {
  return <>
    <TaskTransferDialog taskKey={task.key} queue={task.queue} open={transferOpen}
      onOpenChange={setTransferOpen} onTransfer={onTransfer} />
    <TaskRequeueDialog taskKey={task.key} queue={task.queue} workflow={workflow} open={requeueOpen}
      onOpenChange={setRequeueOpen} onRequeue={onRequeue} />
  </>
}

export function TaskDetailDraftAlerts({ task, draft, confirmClose, setConfirmClose, onClose }: {
  task: Task
  draft: ReturnType<typeof useTaskDetailDraft>
  confirmClose: boolean
  setConfirmClose: (open: boolean) => void
  onClose: () => void
}) {
  const { dirty, baseline, saving, keepEdits, adopt } = draft
  return <>
      {confirmClose && <Banner tone="danger" role="alert" icon={<AlertCircle className="size-3.5 [stroke-width:1.4]" />}
        text="Are you sure you want to close this task? Unsaved changes will be discarded."
        actions={<>
          {/* Keeping the draft is the safe default, so it stays quiet; only the
              action that throws work away carries the solid fill. */}
          <BannerButton tone="quiet" onClick={() => setConfirmClose(false)}>Keep editing</BannerButton>
          <BannerButton tone="danger" onClick={onClose}>Discard changes</BannerButton>
        </>} />}
      {dirty && baseline.revision !== task.revision && <div role="alert"
        className={cn("flex flex-col gap-2 rounded-[8px] px-2.5 py-2", DANGER_FILL)}>
        <div className="flex items-start gap-[9px]">
          <AlertCircle className="mt-px size-3.5 shrink-0 [stroke-width:1.4]" />
          <p className="min-w-0 flex-1 text-[12.5px] leading-[1.45] font-medium text-pretty">This task changed while you were editing. Keep your edited fields over the latest values, or reload to discard your edits.</p>
        </div>
        <details className="text-[12px]"><summary className="cursor-pointer">Latest values</summary>
          <dl className="mt-1.5 grid grid-cols-[76px_minmax(0,1fr)] gap-x-2.5 gap-y-1 text-foreground">
            <dt className={LABEL}>Title</dt><dd className="min-w-0 text-[12.5px]">{task.title}</dd>
            <dt className={LABEL}>Description</dt><dd className="min-w-0"><MarkdownContent>{task.description}</MarkdownContent></dd>
            <dt className={LABEL}>Status</dt><dd className={cn(MONO, "min-w-0")}>{task.status}</dd>
            <dt className={LABEL}>Priority</dt><dd className={cn(MONO, "min-w-0")}>{task.priority}</dd>
            <dt className={LABEL}>Assignee</dt><dd className={cn(MONO, "min-w-0")}>{task.assignee || "Unassigned"}</dd>
            <dt className={LABEL}>Pull request</dt><dd className={cn(MONO, "min-w-0 break-all")}>{task.pull_request || "None"}</dd>
            <dt className={LABEL}>Block reason</dt><dd className="min-w-0 text-[12.5px]">{task.manual_block_reason || "None"}</dd>
          </dl>
        </details>
        <div className="flex gap-1.5">
          <BannerButton tone="danger" disabled={saving} onClick={keepEdits}>Keep my edits</BannerButton>
          <BannerButton tone="quiet" disabled={saving} onClick={() => adopt(task)}>Reload task</BannerButton>
        </div>
      </div>}
  </>
}
