import { useState, type Ref } from "react";
import { MoreHorizontal, Pencil, X } from "lucide-react";
import { Input } from "@/components/ui/input";
import type { Task } from "@/lib/api";
import { isWorkflowTask, taskStatusLabel } from "@/lib/tasks";
import { Button } from "@/components/ui/button";
import { DialogTitle } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { StatusPill } from "@/components/ui/status";
import { taskTone } from "@/lib/statusTone";
import { formatTaskTime } from "./taskTime";
import { MetaInline } from "./TaskDetailControls";

export function TaskDetailHeader({
  task,
  pending,
  title,
  setTitle,
  dirty,
  save,
  close,
  initialFocusRef,
  onRequeue,
  onTransfer,
}: {
  task: Task;
  pending: boolean;
  title: string;
  setTitle: (title: string) => void;
  dirty: boolean;
  save: () => Promise<void>;
  close: () => void;
  initialFocusRef: Ref<HTMLButtonElement>;
  onRequeue: () => void;
  onTransfer: () => void;
}) {
  const editable = task.access !== "context" && task.access !== "respond";
  const [editing, setEditing] = useState(false);
  const workflow = isWorkflowTask(task);
  const closed = task.category === "done" || task.category === "cancelled";
  return (
    <header className="flex items-start gap-2.5 bg-card px-3.5 pt-2.75 pb-2.5">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex min-w-0 items-center gap-2.25">
          <DialogTitle asChild>
            <h2 className="shrink-0 font-mono text-ui font-medium tabular-nums text-muted-foreground">
              {task.key}
            </h2>
          </DialogTitle>
          {/* The only 600 in the panel. */}
          {editing ? (
            <Input
              autoFocus
              aria-label="Title"
              value={title}
              disabled={pending}
              onChange={(event) => setTitle(event.target.value)}
              onBlur={() => setEditing(false)}
            />
          ) : (
            <span className="min-w-0 truncate text-title font-semibold tracking-[-.01em]">
              {title}
            </span>
          )}
          {editable && !editing && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Edit title"
              disabled={pending}
              onClick={() => setEditing(true)}
              className="size-7 shrink-0"
            >
              <Pencil className="size-3.5" />
            </Button>
          )}
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-2 text-ui">
          <StatusPill tone={taskTone(task.category)}>{taskStatusLabel(task)}</StatusPill>
          <MetaInline label="agent" value={task.assignee || "unassigned"} />
          <MetaInline label="updated" value={formatTaskTime(task.updated_at)} />
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {editable && dirty && (
          <Button
            size="sm"
            className="h-7"
            disabled={pending || !title.trim()}
            onClick={() => void save()}
          >
            Save task
          </Button>
        )}
        {/* Everything that is not Save or Close lives behind one menu, so the
            header keeps its two primary controls at any panel width. */}
        {/* A workflow task cannot move to another server: the export is
            refused with workflow_managed. Only an open task changes queue. */}
        {editable && (!workflow || !closed) && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Task actions"
                disabled={pending}
                className="size-7 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
              >
                <MoreHorizontal className="size-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {!closed && (
                <DropdownMenuItem onSelect={() => onRequeue()}>Move to queue…</DropdownMenuItem>
              )}
              {!workflow && (
                <DropdownMenuItem onSelect={() => onTransfer()}>
                  Move to another server…
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
        {/* The close button is the panel entry point now that Back is gone,
            so it takes the focus Back used to hold. */}
        <Button
          ref={initialFocusRef}
          variant="ghost"
          size="icon"
          aria-label="Close task detail"
          disabled={pending}
          onClick={close}
          className="size-7 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <X className="size-3.5" />
        </Button>
      </div>
    </header>
  );
}
