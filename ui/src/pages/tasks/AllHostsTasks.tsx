import { useCallback, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { ApiProvider } from "@/components/ApiProvider";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import type { HostAgents } from "@/lib/aggregate";
import { errorText } from "@/lib/errors";
import { hostToParam, targetFor } from "@/lib/terminalsHost";
import { mergeHostSummaries, type HostTaskSummary } from "./allHostsTasksModel";
import { HostTasksSection } from "./HostTasksSection";
import { TaskFilterBar } from "./TaskFilterBar";
import TaskDrawer from "./TaskDrawer";
import TaskForm from "./TaskForm";
import TaskTableHeader from "./TaskTableHeader";
import { taskKeys, taskQueries, writeTask } from "./taskQueries";
import { readRememberedQueue, rememberQueue } from "./taskQueueMemory";
import { useTasksWorkspaceView } from "./useTasksWorkspaceView";
import "./tasks.css";

type OpenTask = { hostId: string; key: string };

/**
 * Tasks across every host of the workspace. The search, the status and the
 * Filters menu apply to every host; each host's rows come from its own
 * daemon, below its own ApiProvider, and form one group under the host's
 * name. A task opens in the drawer on its host.
 */
export default function AllHostsTasks({
  hosts,
  hostControl,
  onNotificationsChanged,
}: {
  /** The workspace's hosts, in sidebar order. */
  hosts: readonly HostAgents[];
  hostControl?: ReactNode;
  onNotificationsChanged?: (hostId: string) => void;
}) {
  const client = useQueryClient();
  const view = useTasksWorkspaceView();
  const [open, setOpen] = useState<OpenTask | null>(null);
  const [summaries, setSummaries] = useState<Record<string, HostTaskSummary>>({});
  const onSummary = useCallback(
    (hostId: string, summary: HostTaskSummary) =>
      setSummaries((current) => ({ ...current, [hostId]: summary })),
    [],
  );
  // A host the sidebar cannot reach is not asked; its group says so.
  const reachable = hosts.filter((entry) => !entry.error);
  const merged = mergeHostSummaries(
    summaries,
    reachable.map((entry) => entry.host.id),
  );
  const refresh = () => {
    for (const entry of reachable)
      void client.invalidateQueries({ queryKey: taskKeys.server(targetFor(entry.host.id)) });
  };

  return (
    <div className="grid h-full min-h-0 grid-cols-[minmax(0,1fr)]" data-testid="all-hosts-tasks">
      <main className="min-h-0 min-w-0 overflow-auto">
        <TaskFilterBar
          mode="all"
          hostControl={hostControl}
          query={view.query}
          onQuery={view.setQuery}
          statusView={view.statusView}
          onStatusView={view.setStatusView}
          assignee={view.assignee}
          onAssignee={view.setAssignee}
          agents={merged.agents}
          waiting={view.waiting}
          onWaiting={view.setWaiting}
          queues={merged.queues}
          queue={view.queue}
          onQueue={view.selectQueue}
          queueCounts={merged.queueCounts}
          refreshing={merged.refreshing}
          onRefresh={refresh}
          onCreate={view.startCreate}
        />
        {view.creatingParent === "" && reachable.length > 0 && (
          <NewTaskOnHost
            hosts={reachable}
            queue={view.queue}
            onCancel={() => view.setCreatingParent(null)}
            onCreated={(task) => {
              view.setCreatingParent(null);
              setOpen(task);
            }}
          />
        )}
        <TaskTableHeader mode="all" sticky />
        <div className="pb-20">
          {hosts.map((entry) => {
            const hostId = entry.host.id;
            if (entry.error) return <UnavailableHost key={hostToParam(hostId)} entry={entry} />;
            return (
              <ApiProvider key={hostToParam(hostId)} target={targetFor(hostId)}>
                <HostTasksSection
                  hostId={hostId}
                  label={entry.host.label}
                  view={view}
                  selectedKey={open?.hostId === hostId ? open.key : ""}
                  onSelect={(key) => setOpen({ hostId, key })}
                  onSummary={onSummary}
                  onNotificationsChanged={() => onNotificationsChanged?.(hostId)}
                />
              </ApiProvider>
            );
          })}
          {merged.empty && (
            <div className="px-5 py-10 text-center text-body text-muted-foreground">
              No tasks match this view.
            </div>
          )}
        </div>
      </main>
      {open && (
        // The task opens on its own host: everything inside addresses it.
        <ApiProvider target={targetFor(open.hostId)}>
          <TaskDrawer
            key={`${hostToParam(open.hostId)}\u0000${open.key}`}
            taskKey={open.key}
            onClose={() => setOpen(null)}
            onMoved={(hostId) => setOpen({ hostId, key: open.key })}
          />
        </ApiProvider>
      )}
    </div>
  );
}

function UnavailableHost({ entry }: { entry: HostAgents }) {
  return (
    <section aria-label={entry.host.label}>
      <h2 className="flex items-baseline gap-1.5 border-b border-border/60 px-4 pt-3 pb-1.5 text-body-sm font-semibold">
        {entry.host.label}
        <span className="text-ui font-normal text-muted-foreground" title={entry.error}>
          unavailable
        </span>
      </h2>
    </section>
  );
}

/** "+ New task" across hosts: the host is picked first, and its queues follow. */
function NewTaskOnHost({
  hosts,
  queue,
  onCancel,
  onCreated,
}: {
  hosts: readonly HostAgents[];
  /** The toolbar's queue filter, the first choice where the host has it. */
  queue: string;
  onCancel: () => void;
  onCreated: (task: OpenTask) => void;
}) {
  const [hostId, setHostId] = useState(hosts[0].host.id);
  const picker = (
    <select
      aria-label="Task host"
      value={hostToParam(hostId)}
      onChange={(event) => {
        const next = hosts.find((entry) => hostToParam(entry.host.id) === event.target.value);
        if (next) setHostId(next.host.id);
      }}
    >
      {hosts.map((entry) => (
        <option key={hostToParam(entry.host.id)} value={hostToParam(entry.host.id)}>
          {entry.host.label}
        </option>
      ))}
    </select>
  );
  return (
    <ApiProvider target={targetFor(hostId)}>
      <HostTaskForm
        key={hostToParam(hostId)}
        hostControl={picker}
        queue={queue}
        onCancel={onCancel}
        onCreated={(key) => onCreated({ hostId, key })}
      />
    </ApiProvider>
  );
}

function HostTaskForm({
  hostControl,
  queue,
  onCancel,
  onCreated,
}: {
  hostControl: ReactNode;
  queue: string;
  onCancel: () => void;
  onCreated: (key: string) => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  const server = hostToParam(api.target?.id ?? "");
  const queuesQuery = useApiQuery(taskQueries.queues(api));
  if (!queuesQuery.data) {
    return (
      <div className="task-inline-form mx-3.5 my-2.5 flex gap-1.5 rounded-panel border border-border bg-card p-2.5 text-ui text-muted-foreground">
        {hostControl}
        <span className="self-center">
          {queuesQuery.error ? errorText(queuesQuery.error) : "Loading queues…"}
        </span>
      </div>
    );
  }
  const queues = queuesQuery.data.queues ?? [];
  const exists = (prefix: string) => queues.some((item) => item.prefix === prefix);
  const remembered = readRememberedQueue(server);
  const initialQueue = exists(queue) ? queue : exists(remembered) ? remembered : "";
  return (
    <TaskForm
      hostControl={hostControl}
      queues={queues}
      initialQueue={initialQueue}
      onCancel={onCancel}
      onCreate={async (input) => {
        try {
          const created = await writeTask(client, api, () => api.tasks.createTask(input));
          rememberQueue(server, input.queue);
          onCreated(created.key);
        } catch (error) {
          toast.error(errorText(error));
        }
      }}
    />
  );
}
