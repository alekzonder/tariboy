import { ChevronRight, TriangleAlert } from "lucide-react";
import { useState, type CSSProperties, type ReactNode } from "react";
import {
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { PriorityTag, StatusPill } from "@/components/ui/status";
import { cn } from "@/lib/utils";
import { TONE_TEXT, taskTone } from "@/lib/statusTone";
import type { TaskPriority, WorkflowAttention } from "@/lib/api";
import {
  isWorkflowTask,
  taskStatusLabel,
  workflowAttentionTone,
  workflowStatusBadge,
} from "@/lib/tasks";
import type { VisibleTaskRow } from "@/lib/taskTree";
import type { TaskColumnId } from "./taskColumnLayout";
import { INDENT_PX, MAX_INDENT_DEPTH } from "./taskColumns";
import { formatTaskDuration, formatTaskTime } from "./taskTime";

const priorityLabels = {
  P0: "Critical",
  P1: "High",
  P2: "Normal",
  P3: "Low",
} as const;

const ATTENTION_TITLE: Record<WorkflowAttention["kind"], string> = {
  paused: "Paused",
  failed: "A check could not run",
  rejected: "A check rejected the request",
  script_failed: "The watch script failed",
  idle: "No transition requested yet",
};

/** What every cell of a row may read; each cell takes the part it shows. */
interface TaskRowCellProps {
  row: VisibleTaskRow;
  style: CSSProperties;
  readOnly: boolean;
  server?: string;
  hasActiveQuestion: boolean;
  expanded: boolean;
  onToggle: () => void;
  onSelect: () => void;
  onPriorityChange?: (priority: TaskPriority) => Promise<void> | void;
}

/** A problem on a workflow task, as one mark the row has room for; the title
 *  carries the script's words. */
function AttentionMark({ attention }: { attention: WorkflowAttention }) {
  const tone = workflowAttentionTone(attention) ?? "quiet";
  const title = [
    ATTENTION_TITLE[attention.kind],
    attention.message,
    attention.count ? `(${attention.count})` : "",
  ]
    .filter(Boolean)
    .join(": ")
    .replace(": (", " (");
  return (
    <span
      role="img"
      aria-label={`Problem: ${title}`}
      title={title}
      data-attention={attention.kind}
      className={cn("inline-flex shrink-0 items-center", TONE_TEXT[tone])}
    >
      <TriangleAlert aria-hidden="true" className="size-3.5 [stroke-width:1.8]" />
    </span>
  );
}

// Priority leads every mode by default, unlike the reference, which drops it
// from the all-tasks table: in this app priority is what orders the tree, so a
// table without it cannot be read.
function PriorityCell({ row, style, readOnly, onPriorityChange }: TaskRowCellProps) {
  return (
    <span data-column="priority" style={style} className="z-2 shrink-0">
      {readOnly || !onPriorityChange ? (
        <PriorityTag
          className="task-priority"
          priority={row.task.priority}
          aria-label={priorityName(row.task.priority)}
        />
      ) : (
        <PriorityMenu priority={row.task.priority} onChange={onPriorityChange} />
      )}
    </span>
  );
}

function KeyCell({ row, style, expanded, onToggle }: TaskRowCellProps) {
  return (
    <span
      data-column="key"
      style={style}
      className="z-2 flex shrink-0 items-center gap-1.5 overflow-hidden"
    >
      {Array.from({ length: Math.min(row.depth, MAX_INDENT_DEPTH) }, (_, level) => (
        <span key={level} aria-hidden="true" className="shrink-0" style={{ width: INDENT_PX }} />
      ))}
      <button
        type="button"
        className="grid size-4 shrink-0 place-items-center text-muted-foreground opacity-65 disabled:opacity-0"
        aria-label={`${expanded ? "Collapse" : "Expand"} ${row.task.key}`}
        disabled={!row.hasChildren}
        onClick={onToggle}
      >
        <ChevronRight
          aria-hidden="true"
          className={cn("size-2.5 transition-transform duration-[120ms]", expanded && "rotate-90")}
        />
      </button>
      <span className="min-w-0 truncate font-mono text-meta text-muted-foreground tabular-nums">
        {row.task.key}
      </span>
    </span>
  );
}

function TitleCell({ row, hasActiveQuestion, onSelect }: TaskRowCellProps) {
  return (
    <button
      type="button"
      data-column="task"
      className="task-row-main relative z-2 flex min-w-0 flex-1 items-center gap-1.75 overflow-hidden text-left"
      onClick={onSelect}
    >
      <span className="min-w-0 truncate">{row.task.title}</span>
      {hasActiveQuestion && (
        <span
          className="size-1.25 shrink-0 rounded-full bg-primary"
          role="img"
          aria-label={`Unread question notification for ${row.task.key}`}
        />
      )}
      {row.task.blocked && (
        <StatusPill tone="danger" size="sm">
          blocked
        </StatusPill>
      )}
    </button>
  );
}

function AgentCell({ row, style }: TaskRowCellProps) {
  return (
    <span data-column="agent" style={style} className="shrink-0">
      {row.task.assignee && (
        <span className="inline-flex h-4.75 max-w-full items-center truncate rounded-sm bg-muted px-1.75 text-meta">
          {row.task.assignee.replace(/^agent:/, "")}
        </span>
      )}
    </span>
  );
}

function ServerCell({ style, server }: TaskRowCellProps) {
  return (
    <span
      data-column="server"
      style={style}
      className="shrink-0 truncate font-mono text-meta text-muted-foreground"
    >
      {server}
    </span>
  );
}

function WorkflowStatusBadge({ task }: { task: VisibleTaskRow["task"] }): ReactNode {
  switch (workflowStatusBadge(task)) {
    case "paused":
      return (
        <StatusPill tone="danger" size="sm" title={task.workflow_paused_reason || undefined}>
          paused
        </StatusPill>
      );
    case "attention":
      return <AttentionMark attention={task.workflow_attention!} />;
    case "waiting":
      return (
        <StatusPill tone="quiet" size="sm">
          {task.waiting_on}
        </StatusPill>
      );
    default:
      return null;
  }
}

function StatusCell({ row, style }: TaskRowCellProps) {
  const tone = taskTone(row.task.category);
  if (!isWorkflowTask(row.task)) {
    return (
      <span data-column="status" style={style} className="shrink-0">
        <StatusPill tone={tone}>{taskStatusLabel(row.task)}</StatusPill>
      </span>
    );
  }
  return (
    <span
      data-column="status"
      style={style}
      className="flex min-w-0 shrink-0 items-center gap-1 overflow-hidden"
    >
      <StatusPill tone={tone} className="min-w-0 shrink">
        {/* A workflow status ID can be 64 characters: CSS clips it, the
            title keeps the whole ID readable. */}
        <span title={row.task.status} className="min-w-0 max-w-[24ch] truncate">
          {taskStatusLabel(row.task)}
        </span>
      </StatusPill>
      {row.task.workflow_step && (
        <span
          title={`Step ${row.task.workflow_step.index} of ${row.task.workflow_step.total}`}
          className="hidden shrink-0 font-mono text-tiny tabular-nums text-muted-foreground sm:inline"
        >
          {row.task.workflow_step.index}/{row.task.workflow_step.total}
        </span>
      )}
      <WorkflowStatusBadge task={row.task} />
    </span>
  );
}

function DurationCell({ row, style }: TaskRowCellProps) {
  return (
    <span
      data-column="duration"
      style={style}
      className="shrink-0 truncate text-right font-mono text-meta text-muted-foreground tabular-nums"
    >
      {formatTaskDuration(row.task)}
    </span>
  );
}

function UpdatedCell({ row, style }: TaskRowCellProps) {
  return (
    <span
      data-column="updated"
      style={style}
      className="shrink-0 truncate text-right text-ui text-muted-foreground tabular-nums"
    >
      {formatTaskTime(row.task.updated_at)}
    </span>
  );
}

const TASK_ROW_CELLS: Record<TaskColumnId, (props: TaskRowCellProps) => ReactNode> = {
  priority: PriorityCell,
  key: KeyCell,
  task: TitleCell,
  agent: AgentCell,
  server: ServerCell,
  status: StatusCell,
  duration: DurationCell,
  updated: UpdatedCell,
};

/** The cell a column renders in a task row. */
export default function TaskRowCell({
  column,
  ...props
}: TaskRowCellProps & { column: TaskColumnId }) {
  const Cell = TASK_ROW_CELLS[column];
  return <Cell {...props} />;
}

const priorityName = (priority: TaskPriority) => `${priority} ${priorityLabels[priority]}`;

function PriorityMenu({
  priority,
  onChange,
}: {
  priority: TaskPriority;
  onChange: (priority: TaskPriority) => Promise<void> | void;
}) {
  const [saving, setSaving] = useState(false);
  const pick = async (next: TaskPriority) => {
    if (next === priority) return;
    setSaving(true);
    try {
      await onChange(next);
    } finally {
      setSaving(false);
    }
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={saving}>
        <button
          type="button"
          className="task-priority flex rounded-sm disabled:opacity-50"
          aria-label={priorityName(priority)}
        >
          <PriorityTag priority={priority} aria-hidden="true" className="cursor-pointer" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {(Object.keys(priorityLabels) as TaskPriority[]).map((value) => (
          <DropdownMenuCheckItem
            key={value}
            checked={value === priority}
            onSelect={() => void pick(value)}
          >
            <span className="font-mono">{value}</span>
            <span className="text-muted-foreground">{priorityLabels[value]}</span>
          </DropdownMenuCheckItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
