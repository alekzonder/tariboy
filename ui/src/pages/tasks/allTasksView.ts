import type { HostAgents } from "@/lib/aggregate";
import { buildTaskForest, flattenVisible, type VisibleTaskRow } from "@/lib/taskTree";
import { hostToParam } from "@/lib/terminalsHost";
import type { AssigneeChoice } from "./TaskDrawer";
import type { ServerTask } from "./useAllServersTasks";

export type ServerTaskRow = VisibleTaskRow & { task: ServerTask };

/** The tasks the server and queue filters keep; `server` is a host parameter. */
export function filterServerTasks({
  tasks,
  server,
  queue,
}: {
  tasks: ServerTask[];
  server: string;
  queue: string;
}): ServerTask[] {
  return tasks.filter(
    (task) =>
      (!server || hostToParam(task.serverId) === server) && (!queue || task.queue === queue),
  );
}

/** Every counted queue, plus the selected one even when it has no task. */
export function allTasksQueues(
  queueCounts: ReadonlyMap<string, number>,
  queue: string,
): Array<{ prefix: string }> {
  return [...new Set([...queueCounts.keys(), ...(queue ? [queue] : [])])]
    .sort()
    .map((prefix) => ({ prefix }));
}

/** Builds a tree per server and merges only their roots, newest first.
 *  `expanded` holds `serverId` + NUL + key ids. */
export function serverTaskRows(
  visible: ServerTask[],
  expanded: ReadonlySet<string>,
): ServerTaskRow[] {
  const byServer = new Map<string, ServerTask[]>();
  for (const task of visible)
    byServer.set(task.serverId, [...(byServer.get(task.serverId) ?? []), task]);
  const roots: Array<{ serverId: string; node: ReturnType<typeof buildTaskForest>[number] }> = [];
  for (const [serverId, list] of byServer) {
    for (const node of buildTaskForest(list)) roots.push({ serverId, node });
  }
  roots.sort((left, right) => right.node.task.updated_at.localeCompare(left.node.task.updated_at));
  return roots.flatMap(({ serverId, node }) => {
    const open = new Set(
      [...expanded]
        .filter((id) => id.startsWith(`${serverId}\u0000`))
        .map((id) => id.slice(serverId.length + 1)),
    );
    return flattenVisible([node], open).map((row) => ({ ...row, task: row.task as ServerTask }));
  });
}

/** Every reachable agent anywhere, as an assignee of a task on `serverId`. */
export function crossServerAssignees(
  hosts: readonly HostAgents[],
  serverId: string,
): AssigneeChoice[] {
  return hosts
    .filter((entry) => !entry.error)
    .flatMap((entry) =>
      entry.agents.map((item) => {
        const assignee = `agent:${item.name}`;
        const local = entry.host.id === serverId;
        return {
          // Same-server values are plain assignees, so an untouched field reads
          // as unchanged; another server's carry its id to stay distinct.
          value: local ? assignee : `${assignee}@${entry.host.id || "local"}`,
          label: `${item.name} · ${entry.host.label}`,
          assignee,
          ...(local ? {} : { hostId: entry.host.id, hostLabel: entry.host.label }),
        };
      }),
    );
}
