import type { HostAgents } from "@/lib/aggregate";
import { allAgents, type SidebarAgent } from "@/pages/terminals/sidebar/sidebarModel";

/** The agents with the newest iteration first; agents that never ran keep
 *  their sidebar order after them. */
export function recentAgents(hosts: HostAgents[], count: number): SidebarAgent[] {
  return allAgents(hosts)
    .map((row, index) => ({ row, index, at: row.agent.last_iteration_at ?? "" }))
    .sort((left, right) => right.at.localeCompare(left.at) || left.index - right.index)
    .slice(0, count)
    .map(({ row }) => row);
}

/** The agents running an iteration now, in sidebar order. */
export function runningAgents(hosts: HostAgents[]): SidebarAgent[] {
  return allAgents(hosts).filter((row) => row.agent.state === "running");
}

/** "12 min" since `start`, or "" when it is not a time. */
export function elapsed(start: string, now: Date): string {
  const at = start ? Date.parse(start) : Number.NaN;
  if (Number.isNaN(at)) return "";
  const minutes = Math.floor((now.getTime() - at) / 60000);
  if (minutes < 1) return "under a minute";
  if (minutes < 60) return `${minutes} min`;
  if (minutes < 48 * 60) return `${Math.floor(minutes / 60)} h`;
  return `${Math.floor(minutes / (24 * 60))} d`;
}
