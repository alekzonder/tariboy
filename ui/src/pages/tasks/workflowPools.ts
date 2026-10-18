import type { AgentPool, WorkflowDefinition } from "@/lib/tasks"

/** Each pool the workflow's statuses name, with those statuses in manifest order. */
export function requiredPools(definition: WorkflowDefinition): Map<string, string[]> {
  const pools = new Map<string, string[]>()
  for (const status of definition.statuses) {
    if (status.owner.kind === "pool" && status.owner.pool) {
      pools.set(status.owner.pool, [...pools.get(status.owner.pool) ?? [], status.id])
    }
  }
  return pools
}

/** The required pools that are missing or have no agents. */
export function emptyPools(definition: WorkflowDefinition, pools: AgentPool[]): string[] {
  return [...requiredPools(definition).keys()].filter((name) => !pools.find((pool) => pool.name === name)?.agents.length)
}
