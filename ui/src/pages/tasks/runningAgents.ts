import { createContext, useContext } from "react";
import { ApiContext } from "@/components/apiContext";
import type { HostAgents } from "@/lib/aggregate";

/** Host ID → names of its agents in state `running`. */
export type RunningAgents = ReadonlyMap<string, ReadonlySet<string>>;

/** The running agents of every host the page polls; the task table colors its
 *  Agent badge from it. Empty outside the console page. */
export const RunningAgentsContext = createContext<RunningAgents>(new Map());

export function runningAgentsByHost(hosts: readonly HostAgents[]): RunningAgents {
  return new Map(
    hosts.map((entry) => [
      entry.host.id,
      new Set(entry.agents.filter((a) => a.state === "running").map((a) => a.name)),
    ]),
  );
}

/** True when `agent:NAME` runs on the host of the nearest ApiProvider. */
export function useAgentRunning(assignee: string): boolean {
  const hostId = useContext(ApiContext)?.target?.id ?? "";
  const running = useContext(RunningAgentsContext);
  if (!assignee.startsWith("agent:")) return false;
  return running.get(hostId)?.has(assignee.slice("agent:".length)) ?? false;
}
