import type { Queue } from "@/types";

/** What one host's group reports to the table across hosts: the toolbar's
 *  menus list the queues and agents of every host, and the empty state waits
 *  for every group. */
export interface HostTaskSummary {
  queues: readonly Queue[];
  queueCounts: ReadonlyMap<string, number>;
  agents: readonly string[];
  /** The rows the group shows under the current filters. */
  shown: number;
  loading: boolean;
  refreshing: boolean;
}

/** The toolbar's view of every host: queues by prefix (the first host's name
 *  wins), counts summed per prefix, agents by name. Only `hostIds` count, so
 *  a host that left the workspace leaves the menus too. */
export function mergeHostSummaries(
  summaries: Readonly<Record<string, HostTaskSummary>>,
  hostIds: readonly string[],
) {
  const queues = new Map<string, Queue>();
  const queueCounts = new Map<string, number>();
  const agents = new Set<string>();
  let shown = 0;
  let settled = hostIds.length > 0;
  let refreshing = false;
  for (const id of hostIds) {
    const summary = summaries[id];
    if (!summary) {
      settled = false;
      continue;
    }
    for (const queue of summary.queues)
      if (!queues.has(queue.prefix)) queues.set(queue.prefix, queue);
    for (const [prefix, count] of summary.queueCounts)
      queueCounts.set(prefix, (queueCounts.get(prefix) ?? 0) + count);
    for (const agent of summary.agents) agents.add(agent);
    shown += summary.shown;
    settled &&= !summary.loading;
    refreshing ||= summary.refreshing;
  }
  return {
    queues: [...queues.values()].sort((a, b) => a.prefix.localeCompare(b.prefix)),
    queueCounts,
    agents: [...agents].sort((a, b) => a.localeCompare(b)),
    /** Every group loaded and none shows a row. */
    empty: settled && shown === 0,
    refreshing,
  };
}
