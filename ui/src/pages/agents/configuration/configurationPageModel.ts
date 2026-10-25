import type { AgentStatus, AgentView } from "@/lib/types";
import { blockOfKey } from "./registry";

// countDirtyByBlock counts the unsaved fields of each block.
export function countDirtyByBlock(dirtyKeys: readonly string[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const key of dirtyKeys) {
    const id = blockOfKey(key);
    counts.set(id, (counts.get(id) ?? 0) + 1);
  }
  return counts;
}

// isStopped prefers the live status; without it the view's master switch
// decides, and an older view without the switch falls back to its state.
export function isStopped(status: AgentStatus | null, view: AgentView | null): boolean {
  if (status) return status.state === "stopped";
  return view?.enabled === false || (view?.enabled === undefined && view?.state === "stopped");
}
