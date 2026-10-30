import type { ReactNode } from "react";
import { Plus, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SearchInput } from "@/components/ui/search-input";
import { Segmented } from "@/components/ui/segmented";
import type { TaskStatusView } from "@/lib/api";
import { TaskFiltersMenu, TaskScopeAgentChip, type TaskFiltersMenuProps } from "./TaskFilterMenus";

const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "closed", label: "Done" },
  { value: "all", label: "All" },
] as const satisfies ReadonlyArray<{ value: TaskStatusView; label: string }>;

/**
 * The one row above the task table: an optional host picker, the search, which
 * statuses to show, the Filters menu (queue, assignee, "Waiting for me",
 * columns), the scope agent's chip in the agent view, then refresh and New
 * task. `Done` is done + cancelled, which is what the daemon's `closed` status
 * view already means. The row never wraps; the search gives up width first.
 */
export function TaskFilterBar({
  hostControl,
  query,
  onQuery,
  statusView,
  onStatusView,
  scopeAgent,
  onClearScopeAgent,
  refreshing,
  onRefresh,
  onCreate,
  ...filters
}: TaskFiltersMenuProps & {
  /** Rendered first, e.g. a host selector when the workspace has several. */
  hostControl?: ReactNode;
  query: string;
  onQuery: (value: string) => void;
  statusView: TaskStatusView;
  onStatusView: (value: TaskStatusView) => void;
  /** Set when the table is scoped to one agent; shown as a removable chip. */
  scopeAgent?: string;
  onClearScopeAgent?: () => void;
  refreshing: boolean;
  onRefresh: () => void;
  onCreate: () => void;
}) {
  return (
    <div
      role="toolbar"
      aria-label="Task filters"
      data-toolbar
      className="flex flex-nowrap items-center gap-1.5 px-4 pt-2.75 pb-2.25"
    >
      {hostControl && <div className="flex h-7 shrink-0 items-center">{hostControl}</div>}
      <SearchInput
        className="h-7 w-55 min-w-0 shrink"
        inputClassName="text-ui"
        aria-label="Search tasks"
        placeholder={filters.mode === "agent" ? "Search tasks" : "Search all tasks"}
        value={query}
        onChange={(event) => onQuery(event.target.value)}
      />
      <Segmented
        label="Task status"
        options={STATUS_OPTIONS}
        value={statusView}
        onChange={onStatusView}
      />
      <TaskFiltersMenu {...filters} />
      {scopeAgent && (
        <TaskScopeAgentChip scopeAgent={scopeAgent} onClearScopeAgent={onClearScopeAgent} />
      )}
      <div className="ml-auto flex h-7 shrink-0 items-center gap-1.5">
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label="Refresh tasks"
          aria-busy={refreshing}
          className="size-6 rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
          onClick={onRefresh}
        >
          <RefreshCw
            aria-hidden="true"
            className={refreshing ? "motion-safe:animate-[spin_0.8s_linear_infinite]" : undefined}
          />
        </Button>
        <Button type="button" size="xs" onClick={onCreate}>
          <Plus aria-hidden="true" />
          New task
        </Button>
      </div>
    </div>
  );
}
