import {
  Check,
  ChevronDown,
  Inbox,
  ListFilter,
  Server,
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
import { cn } from "@/lib/utils";
import type { TaskQueue } from "@/lib/api";
import { queueMenuItems, serverMenuItems } from "@/lib/taskFilterMenus";
import type { TaskPrincipalFilter } from "./TaskFilterBar";

const PRINCIPAL_OPTIONS = [
  { value: "mine", label: "My tasks", icon: UserRound },
  { value: "waiting", label: "Waiting for me", icon: Inbox },
] as const satisfies ReadonlyArray<{
  value: Exclude<TaskPrincipalFilter, "">;
  label: string;
  icon: typeof UserRound;
}>;

/** The queue picker; an empty `queue` means every queue. */
export function TaskQueueMenu({
  queues,
  queue,
  onQueue,
  queueCounts,
  onManageQueues,
}: {
  queues: ReadonlyArray<Pick<TaskQueue, "prefix">>;
  queue: string;
  onQueue: (prefix: string) => void;
  queueCounts: ReadonlyMap<string, number>;
  onManageQueues?: () => void;
}) {
  const queueLabel = queue || "all";
  const items = queueMenuItems(queues, queueCounts);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Queue: ${queueLabel}`}
          className="flex h-7 shrink-0 items-center gap-1.75 rounded-md bg-muted px-2.25 text-ui hover:bg-accent"
        >
          <ListFilter aria-hidden="true" className="size-3 opacity-55" />
          <span className="text-muted-foreground">Queue:</span>
          <span className="font-mono text-meta font-medium">{queueLabel}</span>
          <ChevronDown aria-hidden="true" className="size-2.25 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-53.5 rounded-card p-1.25">
        {items.map((item) => (
          <DropdownMenuItem
            key={item.prefix || "__all__"}
            onSelect={() => onQueue(item.prefix)}
            className={cn(
              "h-7 gap-2 rounded-control px-2.25 text-body-sm",
              item.prefix === queue && "bg-accent",
            )}
          >
            <span className="min-w-0 flex-1 truncate font-mono text-meta font-medium">
              {item.label}
            </span>
            <span className="text-muted-foreground tabular-nums">{item.count}</span>
            {item.prefix === queue ? (
              <Check aria-hidden="true" className="size-2.75 text-primary" />
            ) : (
              <span aria-hidden="true" className="w-2.75" />
            )}
          </DropdownMenuItem>
        ))}
        {onManageQueues && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={onManageQueues}
              className="h-7 gap-2 rounded-control px-2.25 text-body-sm"
            >
              <Settings aria-hidden="true" className="size-3" />
              Manage queues…
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** The every-server table's server picker; an empty `server` means every
 *  server, and an unavailable server is listed but cannot be picked. */
export function TaskServerMenu({
  servers,
  server,
  onServer,
}: {
  servers: ReadonlyArray<{ id: string; label: string; count: number; unavailable: boolean }>;
  server: string;
  onServer: (id: string) => void;
}) {
  const serverLabel = (server && servers.find((item) => item.id === server)?.label) || "all";
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Server: ${serverLabel}`}
          className="flex h-7 shrink-0 items-center gap-1.75 rounded-md bg-muted px-2.25 text-ui hover:bg-accent"
        >
          <Server aria-hidden="true" className="size-3 opacity-55" />
          <span className="text-muted-foreground">Server:</span>
          <span className="font-mono text-meta font-medium">{serverLabel}</span>
          <ChevronDown aria-hidden="true" className="size-2.25 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-53.5 rounded-card p-1.25">
        {serverMenuItems(servers).map((item) => (
          <DropdownMenuItem
            key={item.id || "__all__"}
            disabled={item.unavailable}
            onSelect={() => onServer(item.id)}
            className={cn(
              "h-7 gap-2 rounded-control px-2.25 text-body-sm",
              item.id === server && "bg-accent",
            )}
          >
            {item.unavailable && (
              <span
                aria-hidden="true"
                className="size-1.5 shrink-0 rounded-full bg-status-failed"
              />
            )}
            <span className="min-w-0 flex-1 truncate font-mono text-meta font-medium">
              {item.label}
            </span>
            <span className="text-muted-foreground tabular-nums">
              {item.unavailable ? "unavailable" : item.count}
            </span>
            {item.id === server ? (
              <Check aria-hidden="true" className="size-2.75 text-primary" />
            ) : (
              <span aria-hidden="true" className="w-2.75" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** The "My tasks" and "Waiting for me" toggles; pressing the active one clears it. */
export function TaskPrincipalChips({
  principalFilter,
  onPrincipalFilter,
}: {
  principalFilter?: TaskPrincipalFilter;
  onPrincipalFilter?: (value: TaskPrincipalFilter) => void;
}) {
  return PRINCIPAL_OPTIONS.map(({ value, label, icon: Icon }) => (
    <button
      key={value}
      type="button"
      aria-pressed={principalFilter === value}
      onClick={() => onPrincipalFilter?.(principalFilter === value ? "" : value)}
      className={cn(
        "flex h-7 shrink-0 items-center gap-1.5 rounded-md px-2.25 text-ui",
        principalFilter === value
          ? "bg-primary/12 font-medium text-primary"
          : "bg-muted text-muted-foreground hover:bg-accent hover:text-foreground",
      )}
    >
      <Icon aria-hidden="true" className="size-3" />
      {label}
    </button>
  ));
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
    <span className="inline-flex h-7 shrink-0 items-center rounded-md bg-primary/12 pr-1.5 pl-2.5 text-ui font-medium text-primary">
      {scopeAgent}
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
