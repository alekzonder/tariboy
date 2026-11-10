import type { ReactNode } from "react";
import { ChevronDown, ExternalLink, Plus } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { StatusPill } from "@/components/ui/status";
import { taskTone } from "@/lib/statusTone";
import { taskStatusLabel } from "@/lib/tasks";
import { cn } from "@/lib/utils";
import type { Task, TaskBudget } from "@/types";

/** The fields a chip edits in place. */
export type ChipField = "budget" | "pull_request";

export interface PropertyChipsProps {
  task: Task;
  /** A workflow task: the workflow sets the status, so the status is not a menu. */
  workflow: boolean;
  /** The agents the task may be assigned to, without the `agent:` prefix. */
  agents: readonly string[];
  budget?: Pick<TaskBudget, "spent_usd" | "budget_usd" | "exhausted">;
  /** The chip that is an input now. */
  editing?: ChipField | null;
  disabled?: boolean;
  onEdit?: (field: ChipField | null) => void;
  onStatus?: (status: string) => void;
  onPriority?: (priority: string) => void;
  onAssignee?: (assignee: string) => void;
  onBudget?: (value: string) => void;
  onPullRequest?: (value: string) => void;
}

const CHIP =
  "inline-flex h-6 min-w-0 max-w-full shrink-0 items-center gap-1.5 rounded-control bg-muted px-2 text-ui text-muted-foreground outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";
const VALUE = "min-w-0 truncate font-mono text-meta tabular-nums text-foreground";
const CHIP_INPUT = "h-6 rounded-control border-0 bg-muted px-2 font-mono text-meta md:text-meta";

const STATUSES = [
  ["open", "Open"],
  ["in_progress", "In progress"],
  ["wait_customer", "Waiting for you"],
  ["done", "Done"],
  ["cancelled", "Cancelled"],
] as const;

const PRIORITIES = [
  ["P0", "Critical"],
  ["P1", "High"],
  ["P2", "Normal"],
  ["P3", "Low"],
] as const;

const money = (usd: number) => `$${usd.toFixed(2)}`;

/** The number of a pull request out of its link; the link itself when it has no number. */
function pullRequestLabel(link: string): string {
  const number = /\/pull\/(\d+)/.exec(link)?.[1];
  return number ? `#${number}` : link.replace(/^https?:\/\//, "");
}

/** A chip that opens a closed list: the label is quiet, the value is mono. */
function MenuChip({
  label,
  value,
  bare,
  disabled,
  children,
}: {
  label: string;
  /** The value explains itself (`P1`), so the label is only read aloud. */
  bare?: boolean;
  value: ReactNode;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className={CHIP} disabled={disabled} aria-label={`Change ${label}`}>
          {bare ? null : label}
          <span className={VALUE}>{value}</span>
          <ChevronDown aria-hidden="true" className="size-2.5 shrink-0 opacity-45" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">{children}</DropdownMenuContent>
    </DropdownMenu>
  );
}

function StatusChip({
  task,
  workflow,
  disabled,
  onStatus,
}: Pick<PropertyChipsProps, "task" | "workflow" | "disabled" | "onStatus">) {
  const pill = <StatusPill tone={taskTone(task.category)}>{taskStatusLabel(task)}</StatusPill>;
  if (workflow) return pill;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          aria-label="Change status"
          className="inline-flex shrink-0 items-center gap-1 rounded-control outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {pill}
          <ChevronDown aria-hidden="true" className="size-2.5 opacity-45" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {STATUSES.map(([value, text]) => (
          <DropdownMenuCheckItem
            key={value}
            checked={task.category === value}
            onSelect={() => onStatus?.(value)}
          >
            {text}
          </DropdownMenuCheckItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function BudgetChip({
  budget,
  editing,
  disabled,
  onEdit,
  onBudget,
}: Pick<PropertyChipsProps, "budget" | "disabled" | "onEdit" | "onBudget"> & {
  editing: boolean;
}) {
  if (editing) {
    return (
      <Input
        autoFocus
        aria-label="Budget, USD"
        type="number"
        min={0}
        step="0.01"
        inputMode="decimal"
        placeholder="No limit"
        defaultValue={budget?.budget_usd || ""}
        className={cn(CHIP_INPUT, "w-24")}
        onChange={(event) => onBudget?.(event.target.value)}
        onBlur={() => onEdit?.(null)}
      />
    );
  }
  const limit = budget && budget.budget_usd > 0 ? money(budget.budget_usd) : "";
  return (
    <button
      type="button"
      className={CHIP}
      disabled={disabled}
      aria-label="Change budget"
      onClick={() => onEdit?.("budget")}
    >
      budget
      <span className={cn(VALUE, budget?.exhausted && "text-status-failed")}>
        {budget ? money(budget.spent_usd) : "$0.00"}
        {limit ? ` / ${limit}` : ""}
      </span>
    </button>
  );
}

function PullRequestChip({
  link,
  editing,
  disabled,
  onEdit,
  onPullRequest,
}: Pick<PropertyChipsProps, "disabled" | "onEdit" | "onPullRequest"> & {
  link: string;
  editing: boolean;
}) {
  if (editing) {
    return (
      <Input
        autoFocus
        aria-label="Pull request"
        placeholder="Pull request link"
        defaultValue={link}
        className={cn(CHIP_INPUT, "w-64 max-w-full")}
        onChange={(event) => onPullRequest?.(event.target.value)}
        onBlur={() => onEdit?.(null)}
      />
    );
  }
  if (!link) {
    return (
      <button
        type="button"
        className={cn(CHIP, "bg-transparent")}
        disabled={disabled}
        onClick={() => onEdit?.("pull_request")}
      >
        <Plus aria-hidden="true" className="size-3" />
        Pull request
      </button>
    );
  }
  return (
    <MenuChip label="PR" value={pullRequestLabel(link)} disabled={disabled}>
      <DropdownMenuItem asChild>
        <a href={link} target="_blank" rel="noreferrer">
          <ExternalLink aria-hidden="true" className="size-3.5" />
          Open pull request
        </a>
      </DropdownMenuItem>
      <DropdownMenuItem onSelect={() => onEdit?.("pull_request")}>Change link…</DropdownMenuItem>
    </MenuChip>
  );
}

/**
 * The parameters of a task as one row of chips under its title: the status,
 * the priority, the assignee, the budget with what is spent, and the pull
 * request. A chip changes its value in place, so no parameter needs a scroll.
 */
export function PropertyChips({
  task,
  workflow,
  agents,
  budget,
  editing = null,
  disabled,
  onEdit,
  onStatus,
  onPriority,
  onAssignee,
  onBudget,
  onPullRequest,
}: PropertyChipsProps) {
  const assignee = task.assignee.replace(/^agent:/, "");
  return (
    <div
      role="group"
      aria-label="Task parameters"
      className="flex min-w-0 flex-wrap items-center gap-1.5"
    >
      <StatusChip {...{ task, workflow, disabled, onStatus }} />
      <MenuChip label="priority" bare value={task.priority} disabled={disabled}>
        {PRIORITIES.map(([value, text]) => (
          <DropdownMenuCheckItem
            key={value}
            checked={task.priority === value}
            onSelect={() => onPriority?.(value)}
          >
            <span className="font-mono text-meta">{value}</span>
            {text}
          </DropdownMenuCheckItem>
        ))}
      </MenuChip>
      <MenuChip label="agent" value={assignee || "unassigned"} disabled={disabled}>
        <DropdownMenuCheckItem checked={!task.assignee} onSelect={() => onAssignee?.("")}>
          Unassigned
        </DropdownMenuCheckItem>
        {agents.map((agent) => (
          <DropdownMenuCheckItem
            key={agent}
            checked={assignee === agent}
            onSelect={() => onAssignee?.(`agent:${agent}`)}
          >
            {agent}
          </DropdownMenuCheckItem>
        ))}
      </MenuChip>
      <BudgetChip editing={editing === "budget"} {...{ budget, disabled, onEdit, onBudget }} />
      <PullRequestChip
        link={task.pull_request}
        editing={editing === "pull_request"}
        {...{ disabled, onEdit, onPullRequest }}
      />
    </div>
  );
}
