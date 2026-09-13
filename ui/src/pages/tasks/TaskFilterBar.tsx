import { Plus, RefreshCw, Search, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Segmented } from "@/components/ui/segmented"
import type { TaskStatusView } from "@/lib/tasks"
import type { TaskRowMode } from "./TaskRow"

const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "closed", label: "Closed" },
  { value: "all", label: "All" },
] as const satisfies ReadonlyArray<{ value: TaskStatusView; label: string }>

/**
 * The bar above the task table: what to search, which statuses to show, and —
 * in the agent-scoped view — the chip that says the table is narrowed to one
 * agent. `Closed` is done + cancelled, which is what the daemon's `status_view`
 * already means.
 */
export function TaskFilterBar({
  mode,
  query,
  onQuery,
  statusView,
  onStatusView,
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
  /** Set when the table is scoped to one agent; shown as a removable chip. */
  scopeAgent?: string
  onClearScopeAgent?: () => void
  refreshing: boolean
  onRefresh: () => void
  onCreate: () => void
}) {
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
