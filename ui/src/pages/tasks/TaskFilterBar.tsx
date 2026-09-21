import { Check, ChevronDown, Inbox, ListFilter, Plus, RefreshCw, Search, Settings, UserRound, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Segmented } from "@/components/ui/segmented"
import { cn } from "@/lib/utils"
import type { TaskQueue, TaskStatusView } from "@/lib/tasks"
import type { TaskRowMode } from "./taskColumns"

const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "closed", label: "Closed" },
  { value: "all", label: "All" },
] as const satisfies ReadonlyArray<{ value: TaskStatusView; label: string }>

/** The two daemon-side narrowings that used to be rail entries. They are
 *  filters, not sections, so they live here as chips and are off by default. */
export type TaskPrincipalFilter = "" | "mine" | "waiting"

const PRINCIPAL_OPTIONS = [
  { value: "mine", label: "My tasks", icon: UserRound },
  { value: "waiting", label: "Waiting for me", icon: Inbox },
] as const satisfies ReadonlyArray<{ value: Exclude<TaskPrincipalFilter, "">; label: string; icon: typeof UserRound }>

/**
 * The bar above the task table: what to search, which statuses to show, which
 * queue, whether the table is narrowed to the signed-in principal, and — in the
 * agent-scoped view — the chip that says it is narrowed to one agent. `Closed`
 * is done + cancelled, which is what the daemon's `status_view` already means.
 */
export function TaskFilterBar({
  mode,
  query,
  onQuery,
  statusView,
  onStatusView,
  principalFilter,
  onPrincipalFilter,
  queues,
  queue,
  onQueue,
  queueCounts,
  onManageQueues,
  scopeAgent,
  onClearScopeAgent,
  refreshing,
  onRefresh,
  onCreate,
}: {
  mode: TaskRowMode
  query: string
  onQuery: (value: string) => void
  statusView: TaskStatusView
  onStatusView: (value: TaskStatusView) => void
  principalFilter: TaskPrincipalFilter
  onPrincipalFilter: (value: TaskPrincipalFilter) => void
  queues: TaskQueue[]
  /** Empty means every queue. */
  queue: string
  onQueue: (prefix: string) => void
  /** Listed tasks per queue prefix, for the menu's trailing counts. */
  queueCounts: ReadonlyMap<string, number>
  onManageQueues: () => void
  /** Set when the table is scoped to one agent; shown as a removable chip. */
  scopeAgent?: string
  onClearScopeAgent?: () => void
  refreshing: boolean
  onRefresh: () => void
  onCreate: () => void
}) {
  const queueLabel = queue || "all"
  const total = [...queueCounts.values()].reduce((sum, count) => sum + count, 0)
  const items = [{ prefix: "", label: "All queues", count: total }].concat(
    queues.map((item) => ({ prefix: item.prefix, label: item.prefix, count: queueCounts.get(item.prefix) ?? 0 })),
  )
  return (
    <div className="flex flex-wrap items-center gap-1.5 px-4 pt-[11px] pb-[9px]">
      <label className="flex h-7 w-[220px] shrink-0 items-center gap-[7px] rounded-[8px] bg-muted px-[9px] text-muted-foreground">
        <Search aria-hidden="true" className="size-3 shrink-0" />
        <input
          type="search"
          aria-label="Search tasks"
          placeholder={mode === "all" ? "Search all tasks" : "Search tasks"}
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          className="min-w-0 flex-1 bg-transparent text-[12px] text-foreground outline-none placeholder:text-muted-foreground"
        />
      </label>
      <Segmented
        label="Task status"
        options={STATUS_OPTIONS}
        value={statusView}
        onChange={onStatusView}
      />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={`Queue: ${queueLabel}`}
            className="flex h-7 shrink-0 items-center gap-[7px] rounded-[8px] bg-muted px-[9px] text-[12px] hover:bg-accent"
          >
            <ListFilter aria-hidden="true" className="size-3 opacity-55" />
            <span className="text-muted-foreground">Queue:</span>
            <span className="font-mono text-[11.5px] font-medium">{queueLabel}</span>
            <ChevronDown aria-hidden="true" className="size-[9px] opacity-50" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="min-w-[214px] rounded-[12px] p-[5px]">
          {items.map((item) => (
            <DropdownMenuItem
              key={item.prefix || "__all__"}
              aria-label={item.label}
              onSelect={() => onQueue(item.prefix)}
              className={cn(
                "h-7 gap-2 rounded-[7px] px-[9px] text-[12.5px]",
                item.prefix === queue && "bg-accent",
              )}
            >
              <span className="min-w-0 flex-1 truncate font-mono text-[11.5px] font-medium">{item.label}</span>
              <span className="text-muted-foreground tabular-nums">{item.count}</span>
              {item.prefix === queue
                ? <Check aria-hidden="true" className="size-[11px] text-primary" />
                : <span aria-hidden="true" className="w-[11px]" />}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onSelect={onManageQueues}
            className="h-7 gap-2 rounded-[7px] px-[9px] text-[12.5px]"
          >
            <Settings aria-hidden="true" className="size-3" />
            Manage queues…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {PRINCIPAL_OPTIONS.map(({ value, label, icon: Icon }) => (
        <button
          key={value}
          type="button"
          aria-pressed={principalFilter === value}
          onClick={() => onPrincipalFilter(principalFilter === value ? "" : value)}
          className={cn(
            "flex h-7 shrink-0 items-center gap-[6px] rounded-[8px] px-[9px] text-[12px]",
            principalFilter === value
              ? "bg-primary/12 font-medium text-primary"
              : "bg-muted text-muted-foreground hover:bg-accent hover:text-foreground",
          )}
        >
          <Icon aria-hidden="true" className="size-3" />
          {label}
        </button>
      ))}
      {scopeAgent && (
        <span className="inline-flex h-7 shrink-0 items-center rounded-[8px] bg-primary/12 pr-1.5 pl-2.5 text-[12px] font-medium text-primary">
          {scopeAgent}
          {onClearScopeAgent && (
            <button
              type="button"
              aria-label="Clear agent filter"
              onClick={onClearScopeAgent}
              className="ml-1 grid size-[18px] place-items-center rounded-[5px]"
            >
              <X aria-hidden="true" className="size-3" />
            </button>
          )}
        </span>
      )}
      <div className="ml-auto flex items-center gap-1.5">
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label="Refresh tasks"
          aria-busy={refreshing}
          className="size-6 rounded-[7px] text-muted-foreground hover:bg-accent hover:text-foreground"
          onClick={onRefresh}
        >
          <RefreshCw aria-hidden="true" className={refreshing ? "is-refreshing" : undefined} />
        </Button>
        <Button type="button" size="xs" onClick={onCreate}>
          <Plus aria-hidden="true" />
          New task
        </Button>
      </div>
    </div>
  )
}
