import type { TaskQueue } from "@/lib/api";

interface ServerMenuItem {
  id: string;
  label: string;
  count: number;
  unavailable: boolean;
}

/** The queue menu's entries: every queue with the listed total, then each
 *  queue with its own count. Empty prefix means every queue. */
export function queueMenuItems(
  queues: ReadonlyArray<Pick<TaskQueue, "prefix">>,
  queueCounts: ReadonlyMap<string, number>,
) {
  const total = [...queueCounts.values()].reduce((sum, count) => sum + count, 0);
  return [{ prefix: "", label: "All queues", count: total }].concat(
    queues.map((item) => ({
      prefix: item.prefix,
      label: item.prefix,
      count: queueCounts.get(item.prefix) ?? 0,
    })),
  );
}

/** The server menu's entries: every server with the summed count, then each
 *  server as given. Empty id means every server. */
export function serverMenuItems(servers: ReadonlyArray<ServerMenuItem>): ServerMenuItem[] {
  return [
    {
      id: "",
      label: "All servers",
      count: servers.reduce((sum, item) => sum + item.count, 0),
      unavailable: false,
    },
  ].concat(servers);
}
