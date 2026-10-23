import type { Ref } from "react"
import { MoreHorizontal, X } from "lucide-react"
import { isWorkflowTask, taskStatusLabel, type Task } from "@/lib/tasks"
import { Button } from "@/components/ui/button"
import { DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { StatusPill } from "@/components/ui/status"
import { taskTone } from "@/lib/statusTone"
import { formatTaskTime } from "./taskTime"
import { MetaInline } from "./TaskDetailControls"

export function TaskDetailHeader({ task, pending, title, save, close, initialFocusRef, onRequeue, onTransfer }: {
  task: Task
  pending: boolean
  title: string
  save: () => Promise<void>
  close: () => void
  initialFocusRef: Ref<HTMLButtonElement>
  onRequeue: () => void
  onTransfer: () => void
}) {
  const editable = task.access !== "context" && task.access !== "respond"
  const workflow = isWorkflowTask(task)
  const closed = task.category === "done" || task.category === "cancelled"
  return (
    <header className="sticky top-0 z-[3] flex items-start gap-2.5 bg-card px-3.5 pt-[11px] pb-2.5 shadow-[var(--raise)]">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex min-w-0 items-center gap-[9px]">
          <DialogTitle asChild>
            <h2 className="shrink-0 font-mono text-[12px] font-medium tabular-nums text-muted-foreground">{task.key}</h2>
          </DialogTitle>
          {/* The only 600 in the panel. */}
          <span className="min-w-0 truncate text-[15px] font-semibold tracking-[-.01em]">{task.title}</span>
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-2 text-[12px]">
          <StatusPill tone={taskTone(task.category)}>{taskStatusLabel(task)}</StatusPill>
          <MetaInline label="agent" value={task.assignee || "unassigned"} />
          <MetaInline label="updated" value={formatTaskTime(task.updated_at)} />
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {editable && <Button size="sm" className="h-7" disabled={pending || !title.trim()} onClick={() => void save()}>Save task</Button>}
        {/* Everything that is not Save or Close lives behind one menu, so the
            header keeps its two primary controls at any panel width. */}
        {/* A workflow task cannot move to another server: the export is
            refused with workflow_managed. Only an open task changes queue. */}
        {editable && (!workflow || !closed) && <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Task actions" disabled={pending}
              className="size-7 rounded-[8px] text-muted-foreground hover:bg-accent hover:text-foreground"><MoreHorizontal className="size-3.5" /></Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {!closed && <DropdownMenuItem onSelect={() => onRequeue()}>Move to queue…</DropdownMenuItem>}
            {!workflow && <DropdownMenuItem onSelect={() => onTransfer()}>Move to another server…</DropdownMenuItem>}
          </DropdownMenuContent>
        </DropdownMenu>}
        {/* The close button is the panel entry point now that Back is gone,
            so it takes the focus Back used to hold. */}
        <Button ref={initialFocusRef} variant="ghost" size="icon" aria-label="Close task detail" disabled={pending} onClick={close}
          className="size-7 rounded-[8px] text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></Button>
      </div>
    </header>
  )
}
