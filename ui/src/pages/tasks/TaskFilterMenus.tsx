import type { ReactNode } from "react";
import { DropdownMenu as MenuPrimitive } from "radix-ui";
import {
  Check,
  ChevronRight,
  Columns3,
  Inbox,
  ListFilter,
  Settings,
  UserRound,
  X,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { cn } from "@/lib/utils";
import type { Queue } from "@/lib/api";
import { queueMenuItems } from "@/lib/taskFilterMenus";
import TaskColumnsMenuItems from "./TaskColumnsMenu";
import type { TaskRowMode } from "./taskColumns";
import type { TaskAssigneeFilter } from "./taskListScope";

/** What the Filters menu narrows: the queue (local), the assignee and the
 *  viewer's waits (daemon-side). */
export interface TaskFiltersMenuProps {
  mode: TaskRowMode;
  queues: ReadonlyArray<Pick<Queue, "prefix">>;
  /** Empty means every queue. */
  queue: string;
  onQueue: (prefix: string) => void;
  /** Listed tasks per queue prefix, for the menu's trailing counts. */
  queueCounts: ReadonlyMap<string, number>;
  onManageQueues?: () => void;
  assignee: TaskAssigneeFilter;
  onAssignee: (value: TaskAssigneeFilter) => void;
  /** The server's agents, from its principals. */
  agents: readonly string[];
  waiting: boolean;
  onWaiting: (value: boolean) => void;
}

const ITEM = "h-7 gap-2 rounded-control px-2.25 text-body-sm";
const SUB_CONTENT =
  "z-[70] max-h-(--radix-dropdown-menu-content-available-height) min-w-53.5 overflow-y-auto rounded-card bg-popover p-1.25 text-popover-foreground shadow-md ring-1 ring-foreground/10";

/** How many of the menu's filters are on; the trigger shows it as a badge. */
function activeFilterCount({
  queue,
  assignee,
  waiting,
}: Pick<TaskFiltersMenuProps, "queue" | "assignee" | "waiting">): number {
  return [queue, assignee, waiting].filter(Boolean).length;
}

const assigneeLabel = (assignee: TaskAssigneeFilter) =>
  assignee === "" ? "Anyone" : assignee === "mine" ? "My tasks" : assignee.slice("agent:".length);

/** A row of the menu that opens a submenu; `value` is what is chosen there. */
function FilterSub({
  label,
  value,
  icon,
  className,
  children,
}: {
  label: string;
  value?: string;
  icon: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <MenuPrimitive.Sub>
      <MenuPrimitive.SubTrigger
        className={cn(
          "flex cursor-default items-center outline-hidden select-none focus:bg-accent data-[state=open]:bg-accent",
          ITEM,
          className,
        )}
      >
        {icon}
        <span className="flex-1">{label}</span>
        {value && (
          <span className="max-w-28 truncate font-mono text-meta text-muted-foreground">
            {value}
          </span>
        )}
        <ChevronRight aria-hidden="true" className="size-3 opacity-50" />
      </MenuPrimitive.SubTrigger>
      <MenuPrimitive.Portal>
        <MenuPrimitive.SubContent sideOffset={4} className={SUB_CONTENT}>
          {children}
        </MenuPrimitive.SubContent>
      </MenuPrimitive.Portal>
    </MenuPrimitive.Sub>
  );
}

/** One choice of a submenu's radio group, with an optional trailing count. */
function FilterRadio({
  value,
  label,
  count,
  mono = false,
}: {
  value: string;
  label: string;
  count?: number;
  mono?: boolean;
}) {
  return (
    <MenuPrimitive.RadioItem
      value={value}
      className={cn(
        "flex cursor-default items-center outline-hidden select-none focus:bg-accent data-[state=checked]:bg-accent",
        ITEM,
      )}
    >
      <span className={cn("min-w-0 flex-1 truncate", mono && "font-mono text-meta font-medium")}>
        {label}
      </span>
      {count !== undefined && <span className="text-muted-foreground tabular-nums">{count}</span>}
      <span className="grid w-2.75 place-items-center">
        <MenuPrimitive.ItemIndicator>
          <Check aria-hidden="true" className="size-2.75 text-primary" />
        </MenuPrimitive.ItemIndicator>
      </span>
    </MenuPrimitive.RadioItem>
  );
}

function QueueSub({
  queues,
  queue,
  onQueue,
  queueCounts,
  onManageQueues,
}: Pick<TaskFiltersMenuProps, "queues" | "queue" | "onQueue" | "queueCounts" | "onManageQueues">) {
  return (
    <FilterSub
      label="Queue"
      value={queue || "all"}
      icon={<ListFilter aria-hidden="true" className="size-3 opacity-55" />}
    >
      <MenuPrimitive.RadioGroup value={queue} onValueChange={onQueue}>
        {queueMenuItems(queues, queueCounts).map((item) => (
          <FilterRadio
            key={item.prefix || "__all__"}
            value={item.prefix}
            label={item.label}
            count={item.count}
            mono
          />
        ))}
      </MenuPrimitive.RadioGroup>
      {onManageQueues && (
        <>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={onManageQueues} className={ITEM}>
            <Settings aria-hidden="true" className="size-3" />
            Manage queues…
          </DropdownMenuItem>
        </>
      )}
    </FilterSub>
  );
}

function AssigneeSub({
  assignee,
  onAssignee,
  agents,
}: Pick<TaskFiltersMenuProps, "assignee" | "onAssignee" | "agents">) {
  return (
    <FilterSub
      label="Assignee"
      value={assignee ? assigneeLabel(assignee) : undefined}
      icon={<UserRound aria-hidden="true" className="size-3 opacity-55" />}
    >
      <MenuPrimitive.RadioGroup
        value={assignee}
        onValueChange={(value) => onAssignee(value as TaskAssigneeFilter)}
      >
        <FilterRadio value="" label="Anyone" />
        <FilterRadio value="mine" label="My tasks" />
        {agents.length > 0 && <DropdownMenuSeparator />}
        {agents.map((agent) => (
          <FilterRadio key={agent} value={`agent:${agent}`} label={agent} mono />
        ))}
      </MenuPrimitive.RadioGroup>
    </FilterSub>
  );
}

/**
 * The toolbar's secondary filters behind one button: the queue, the assignee,
 * "Waiting for me", and the column layout. The badge counts the filters on.
 */
export function TaskFiltersMenu(props: TaskFiltersMenuProps) {
  const { mode, waiting, onWaiting } = props;
  const count = activeFilterCount(props);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={count ? `Filters, ${count} on` : "Filters"}
          data-tour={TOUR_ANCHORS.queueMenu}
          className={cn(
            "flex h-7 shrink-0 items-center gap-1.5 rounded-md px-2.25 text-ui",
            count
              ? "bg-primary/12 font-medium text-primary"
              : "bg-muted text-muted-foreground hover:bg-accent hover:text-foreground",
          )}
        >
          <ListFilter aria-hidden="true" className="size-3" />
          Filters
          {count > 0 && (
            <span className="grid h-4 min-w-4 place-items-center rounded-full bg-primary px-1 text-tiny leading-none text-primary-foreground tabular-nums">
              {count}
            </span>
          )}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-53.5 rounded-card p-1.25">
        <QueueSub {...props} />
        <AssigneeSub {...props} />
        <MenuPrimitive.CheckboxItem
          checked={waiting}
          onCheckedChange={(checked) => onWaiting(checked === true)}
          className={cn(
            "flex cursor-default items-center outline-hidden select-none focus:bg-accent",
            ITEM,
          )}
        >
          <Inbox aria-hidden="true" className="size-3 opacity-55" />
          <span className="flex-1">Waiting for me</span>
          <span className="grid w-2.75 place-items-center">
            <MenuPrimitive.ItemIndicator>
              <Check aria-hidden="true" className="size-2.75 text-primary" />
            </MenuPrimitive.ItemIndicator>
          </span>
        </MenuPrimitive.CheckboxItem>
        <DropdownMenuSeparator className="max-sm:hidden" />
        <FilterSub
          label="Columns"
          className="max-sm:hidden"
          icon={<Columns3 aria-hidden="true" className="size-3 opacity-55" />}
        >
          <TaskColumnsMenuItems mode={mode} />
        </FilterSub>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** The chip naming the one agent the table is scoped to. */
export function TaskScopeAgentChip({
  scopeAgent,
  onClearScopeAgent,
}: {
  scopeAgent: string;
  onClearScopeAgent?: () => void;
}) {
  return (
    <span className="inline-flex h-7 max-w-40 shrink-0 items-center rounded-md bg-primary/12 pr-1.5 pl-2.5 text-ui font-medium text-primary">
      <span className="truncate">{scopeAgent}</span>
      {onClearScopeAgent && (
        <button
          type="button"
          aria-label="Clear agent filter"
          onClick={onClearScopeAgent}
          className="ml-1 grid size-4.5 place-items-center rounded-chip"
        >
          <X aria-hidden="true" className="size-3" />
        </button>
      )}
    </span>
  );
}
