import { Plus, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SearchInput } from "@/components/ui/search-input";
import { Segmented } from "@/components/ui/segmented";
import type { Queue, TaskStatusView } from "@/lib/api";
import TaskColumnsMenu from "./TaskColumnsMenu";
import {
  TaskPrincipalChips,
  TaskQueueMenu,
  TaskScopeAgentChip,
  TaskServerMenu,
} from "./TaskFilterMenus";
import type { TaskRowMode } from "./taskColumns";

const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "closed", label: "Closed" },
  { value: "all", label: "All" },
] as const satisfies ReadonlyArray<{ value: TaskStatusView; label: string }>;

/** The two daemon-side narrowings that used to be rail entries. They are
 *  filters, not sections, so they live here as chips and are off by default. */
export type TaskPrincipalFilter = "" | "mine" | "waiting";

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
  servers,
  server = "",
  onServer,
  scopeAgent,
  onClearScopeAgent,
  refreshing,
  onRefresh,
  onCreate,
}: {
  mode: TaskRowMode;
  query: string;
  onQuery: (value: string) => void;
  statusView: TaskStatusView;
  onStatusView: (value: TaskStatusView) => void;
  principalFilter?: TaskPrincipalFilter;
  onPrincipalFilter?: (value: TaskPrincipalFilter) => void;
  queues: ReadonlyArray<Pick<Queue, "prefix">>;
  /** Empty means every queue. */
  queue: string;
  onQueue: (prefix: string) => void;
  /** Listed tasks per queue prefix, for the menu's trailing counts. */
  queueCounts: ReadonlyMap<string, number>;
  /** Queues belong to one server, so the every-server table cannot manage them. */
  onManageQueues?: () => void;
  /** The every-server table's server menu; an unavailable server is listed but
   *  cannot be picked. */
  servers?: ReadonlyArray<{ id: string; label: string; count: number; unavailable: boolean }>;
  /** Empty means every server. */
  server?: string;
  onServer?: (id: string) => void;
  /** Set when the table is scoped to one agent; shown as a removable chip. */
  scopeAgent?: string;
  onClearScopeAgent?: () => void;
  refreshing: boolean;
  onRefresh: () => void;
  onCreate: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5 px-4 pt-2.75 pb-2.25">
      <SearchInput
        className="h-7 w-55 shrink-0"
        inputClassName="text-ui"
        aria-label="Search tasks"
        placeholder={mode === "agent" ? "Search tasks" : "Search all tasks"}
        value={query}
        onChange={(event) => onQuery(event.target.value)}
      />
      <Segmented
        label="Task status"
        options={STATUS_OPTIONS}
        value={statusView}
        onChange={onStatusView}
      />
      <TaskQueueMenu
        queues={queues}
        queue={queue}
        onQueue={onQueue}
        queueCounts={queueCounts}
        onManageQueues={onManageQueues}
      />
      {servers && onServer && (
        <TaskServerMenu servers={servers} server={server} onServer={onServer} />
      )}
      {mode !== "servers" && (
        <TaskPrincipalChips
          principalFilter={principalFilter}
          onPrincipalFilter={onPrincipalFilter}
        />
      )}
      {scopeAgent && (
        <TaskScopeAgentChip scopeAgent={scopeAgent} onClearScopeAgent={onClearScopeAgent} />
      )}
      <div className="ml-auto flex items-center gap-1.5">
        <TaskColumnsMenu mode={mode} />
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
