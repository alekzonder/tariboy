import type { AgentStatus, AgentView } from "@/lib/api";
import { SECTIONS, sectionOfKey } from "./registry";

// groupOfKey names where a field sits on the page: one of the four groups, or
// Advanced for every other section.
export function groupOfKey(key: string): string {
  const section = sectionOfKey(key);
  if (!section) return "";
  return section.advanced ? "Advanced" : section.title;
}

const GROUPS = [...new Set(SECTIONS.map((s) => (s.advanced ? "Advanced" : s.title)))];

// countDirtyByGroup counts the unsaved fields of each group, in page order.
export function countDirtyByGroup(dirtyKeys: readonly string[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const group of GROUPS) {
    const count = dirtyKeys.filter((key) => groupOfKey(key) === group).length;
    if (count > 0) counts.set(group, count);
  }
  return counts;
}

// isStopped prefers the live status; without it the view's master switch
// decides, and an older view without the switch falls back to its state.
export function isStopped(status: AgentStatus | null, view: AgentView | null): boolean {
  if (status) return status.state === "stopped";
  return view?.enabled === false || (view?.enabled === undefined && view?.state === "stopped");
}
