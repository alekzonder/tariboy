import type { Queue } from "@/lib/api";

/** The queue menu's entries: every queue with the listed total, then each
 *  queue with its own count. Empty prefix means every queue. */
export function queueMenuItems(
  queues: ReadonlyArray<Pick<Queue, "prefix">>,
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
