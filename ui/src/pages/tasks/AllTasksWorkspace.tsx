import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiProvider } from "@/components/ApiProvider";
import { Input } from "@/components/ui/input";
import { useTasksSocket } from "@/hooks/useTasksSocket";
import type { HostAgents } from "@/lib/aggregate";
import { createApi, type TaskPriority, type Queue } from "@/lib/api";
import { countTasksByQueue } from "@/lib/tasks";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import {
  allTasksQueues,
  crossServerAssignees,
  filterServerTasks,
  serverTaskRows,
} from "./allTasksView";
import { TaskFilterBar } from "./TaskFilterBar";
import TaskDrawer from "./TaskDrawer";
import TaskRow from "./TaskRow";
import TaskTableHeader from "./TaskTableHeader";
import { readRememberedQueue, rememberQueue } from "./taskQueueMemory";
import { useAllServersTasks, type ServerTask } from "./useAllServersTasks";
import { useAllTasksView } from "./useAllTasksView";
import "./tasks.css";

const rowId = (task: ServerTask) => `${task.serverId}\u0000${task.key}`;

/** One server's live stream. A hint only says "something changed", so the
 *  answer is to read that server again. */
function ServerTasksLive({
  serverId,
  after,
  onChange,
}: {
  serverId: string;
  after: number;
  onChange: () => void;
}) {
  useTasksSocket({ target: targetFor(serverId), after, onHint: onChange, onReset: onChange });
  return null;
}

/**
 * Every task on every server, in one table. The rows come from each server
 * separately, so trees are built per server and only their roots are merged,
 * newest first. A task opens in place, on its own server, and its assignee can
 * be any agent anywhere — one on another server moves the task there.
 */
export default function AllTasksWorkspace({ hosts }: { hosts: readonly HostAgents[] }) {
  const { view, setView, queue, setQueue, filtersActive, clearFilters } = useAllTasksView();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [open, setOpen] = useState<{ serverId: string; key: string } | null>(null);
  const [creating, setCreating] = useState(false);

  const servers = useMemo(
    () =>
      hosts.map((entry) => ({ id: entry.host.id, label: entry.host.label, error: entry.error })),
    [hosts],
  );
  const labelOf = (id: string) =>
    servers.find((server) => server.id === id)?.label ?? (id || "local");
  const { tasks, errors, sequences, loading, reload, refresh } = useAllServersTasks(servers, {
    text: view.query,
    statusView: view.statusView,
  });

  const visible = useMemo(
    () => filterServerTasks({ tasks, server: view.server, queue }),
    [tasks, queue, view.server],
  );
  const queueCounts = useMemo(() => countTasksByQueue(tasks), [tasks]);
  const queues = useMemo(() => allTasksQueues(queueCounts, queue), [queue, queueCounts]);
  const serverItems = servers.map((server) => ({
    id: hostToParam(server.id),
    label: server.label,
    count: tasks.filter((task) => task.serverId === server.id).length,
    unavailable: Boolean(errors[server.id]) && !tasks.some((task) => task.serverId === server.id),
  }));

  const rows = useMemo(() => serverTaskRows(visible, expanded), [expanded, visible]);
  const assignees = useMemo(
    () => (open ? crossServerAssignees(hosts, open.serverId) : []),
    [hosts, open],
  );

  const changePriority = async (task: ServerTask, priority: TaskPriority) => {
    try {
      await createApi(targetFor(task.serverId)).tasks.updateTask(task.key, {
        priority,
        revision: task.revision,
      });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    }
    reload(task.serverId);
  };

  return (
    <div
      className="grid h-full min-h-0 grid-cols-[minmax(0,1fr)]"
      data-testid="all-tasks-workspace"
    >
      {Object.entries(sequences).map(([serverId, after]) => (
        <ServerTasksLive
          key={serverId}
          serverId={serverId}
          after={after}
          onChange={() => refresh(serverId)}
        />
      ))}
      <main className="min-h-0 min-w-0 overflow-auto">
        <TaskFilterBar
          mode="servers"
          query={view.query}
          onQuery={(query) => setView({ query })}
          statusView={view.statusView}
          onStatusView={(statusView) => setView({ statusView })}
          queues={queues}
          queue={queue}
          onQueue={setQueue}
          queueCounts={queueCounts}
          servers={serverItems}
          server={view.server}
          onServer={(server) => setView({ server })}
          refreshing={false}
          onRefresh={() => servers.forEach((server) => reload(server.id))}
          onCreate={() => setCreating(true)}
        />
        {creating && (
          <NewTaskForm
            hosts={hosts}
            onCancel={() => setCreating(false)}
            onCreated={(serverId, key) => {
              setCreating(false);
              reload(serverId);
              setOpen({ serverId, key });
            }}
          />
        )}
        <ServerWarnings errors={errors} labelOf={labelOf} onRetry={reload} />
        {loading ? (
          <div aria-label="Loading tasks" role="status">
            {Array.from({ length: 6 }, (_, index) => (
              <div key={index} className="mx-4 my-1.25 h-5 animate-pulse rounded-sm bg-muted" />
            ))}
          </div>
        ) : (
          <div className="min-h-[calc(100%-60px)] pb-20">
            <TaskTableHeader mode="servers" sticky />
            {rows.map((row) => {
              const id = rowId(row.task);
              return (
                <TaskRow
                  key={id}
                  row={row}
                  mode="servers"
                  server={row.task.serverName}
                  hasActiveQuestion={false}
                  expanded={expanded.has(id)}
                  selected={open !== null && `${open.serverId}\u0000${open.key}` === id}
                  onToggle={() =>
                    setExpanded((current) => {
                      const next = new Set(current);
                      if (!next.delete(id)) next.add(id);
                      return next;
                    })
                  }
                  onSelect={() => setOpen({ serverId: row.task.serverId, key: row.task.key })}
                  onPriorityChange={(priority) => changePriority(row.task, priority)}
                />
              );
            })}
            {rows.length === 0 && (
              <NoTasks filtersActive={filtersActive} onClearFilters={clearFilters} />
            )}
          </div>
        )}
      </main>
      {open && (
        // The task opens on its own server: everything inside addresses it.
        <ApiProvider target={targetFor(open.serverId)}>
          <TaskDrawer
            key={`${open.serverId}\u0000${open.key}`}
            taskKey={open.key}
            assignees={assignees}
            onClose={() => setOpen(null)}
            onMoved={(serverId) => {
              reload(open.serverId);
              reload(serverId);
              setOpen({ serverId, key: open.key });
            }}
          />
        </ApiProvider>
      )}
    </div>
  );
}

function ServerWarnings({
  errors,
  labelOf,
  onRetry,
}: {
  errors: Record<string, string>;
  labelOf: (serverId: string) => string;
  onRetry: (serverId: string) => void;
}) {
  return Object.entries(errors).map(([serverId, message]) => (
    <div
      key={serverId}
      data-testid="server-warning"
      title={message}
      className="px-4 pb-1.5 text-ui text-muted-foreground"
    >
      {labelOf(serverId)} unavailable ·{" "}
      <button
        type="button"
        className="underline-offset-2 hover:underline"
        onClick={() => onRetry(serverId)}
      >
        Retry
      </button>
    </div>
  ));
}

function NoTasks({
  filtersActive,
  onClearFilters,
}: {
  filtersActive: boolean;
  onClearFilters: () => void;
}) {
  return (
    <div className="flex flex-col items-center gap-1 py-12">
      <span className="text-body font-medium">No tasks</span>
      <span className="text-ui text-muted-foreground">
        {filtersActive ? "Nothing matches the current filters." : "No server has tasks yet."}
      </span>
      {filtersActive && (
        <button
          type="button"
          className="text-ui text-primary hover:underline"
          onClick={onClearFilters}
        >
          Clear filters
        </button>
      )}
    </div>
  );
}

/** "+ New task" across servers: the agent decides the server, the server
 *  decides which queues exist. */
function NewTaskForm({
  hosts,
  onCancel,
  onCreated,
}: {
  hosts: readonly HostAgents[];
  onCancel: () => void;
  onCreated: (serverId: string, key: string) => void;
}) {
  const choices = hosts
    .filter((entry) => !entry.error)
    .flatMap((entry) =>
      entry.agents.map((item) => ({
        id: `${entry.host.id}\u0000${item.name}`,
        hostId: entry.host.id,
        name: item.name,
        label: `${item.name} · ${entry.host.label}`,
      })),
    );
  const [choice, setChoice] = useState("");
  const picked = choices.find((item) => item.id === choice);
  const [queues, setQueues] = useState<Queue[]>([]);
  const [queue, setQueue] = useState("");
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);

  const hostId = picked?.hostId;
  useEffect(() => {
    if (hostId === undefined) return;
    let alive = true;
    createApi(targetFor(hostId))
      .tasks.listTaskQueues()
      .then((page) => {
        if (!alive) return;
        const queues = page.queues ?? [];
        const has = (prefix: string) => queues.some((item) => item.prefix === prefix);
        const remembered = readRememberedQueue(hostToParam(hostId));
        setQueues(queues);
        setQueue((current) =>
          has(remembered) ? remembered : has(current) ? current : (queues[0]?.prefix ?? ""),
        );
      })
      .catch((error: unknown) => {
        // The previous agent's queues do not exist on this server.
        if (alive) {
          setQueues([]);
          setQueue("");
        }
        toast.error(error instanceof Error ? error.message : String(error));
      });
    return () => {
      alive = false;
    };
  }, [hostId]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!picked || !queue || !title.trim()) return;
    setBusy(true);
    try {
      const created = await createApi(targetFor(picked.hostId)).tasks.createTask({
        queue,
        title: title.trim(),
        assignee: `agent:${picked.name}`,
      });
      rememberQueue(hostToParam(picked.hostId), queue);
      onCreated(picked.hostId, created.key);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form
      className="task-inline-form mx-3.5 my-2.5 flex gap-1.5 rounded-panel border border-border bg-card p-2.5 shadow-[0_8px_30px_oklch(0_0_0/0.07)]"
      onSubmit={(event) => void submit(event)}
    >
      <select
        aria-label="Task agent"
        value={choice}
        onChange={(event) => setChoice(event.target.value)}
      >
        <option value="">Choose an agent</option>
        {choices.map((item) => (
          <option key={item.id} value={item.id}>
            {item.label}
          </option>
        ))}
      </select>
      <select
        aria-label="Task queue"
        value={queue}
        disabled={!picked}
        onChange={(event) => setQueue(event.target.value)}
      >
        {queues.map((item) => (
          <option key={item.prefix} value={item.prefix}>
            {item.prefix}
          </option>
        ))}
      </select>
      <Input
        autoFocus
        aria-label="Task title"
        placeholder="What needs to be done?"
        value={title}
        onChange={(event) => setTitle(event.target.value)}
      />
      <button type="submit" disabled={busy || !picked || !queue || !title.trim()}>
        Create task
      </button>
      <button type="button" onClick={onCancel}>
        Cancel
      </button>
    </form>
  );
}
