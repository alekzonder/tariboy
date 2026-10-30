import { readKey, storageKeys, writeKey } from "@/lib/storage";

// The queue the last task was created in, per server: each server has its own
// queues. Unlike the queue filter it lives in localStorage, so the choice
// survives a restart. An agent's Tasks tab also keeps its own choice per agent.
function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

const readAll = () => asRecord(readKey(storageKeys.newTaskQueue));
const readByAgent = () => asRecord(readKey(storageKeys.newTaskQueueByAgent));

/** The queue last used for this agent on this server, or "". */
export function readAgentQueue(server: string, agent: string): string {
  const own = asRecord(readByAgent()[server])[agent];
  return typeof own === "string" ? own : "";
}

export function readRememberedQueue(server: string, agent?: string): string {
  const own = agent ? readAgentQueue(server, agent) : "";
  if (own) return own;
  const prefix = readAll()[server];
  return typeof prefix === "string" ? prefix : "";
}

export function rememberQueue(server: string, prefix: string, agent?: string): void {
  // Web Storage is a best-effort Desktop convenience.
  writeKey(storageKeys.newTaskQueue, { ...readAll(), [server]: prefix });
  if (!agent) return;
  const byAgent = readByAgent();
  writeKey(storageKeys.newTaskQueueByAgent, {
    ...byAgent,
    [server]: { ...asRecord(byAgent[server]), [agent]: prefix },
  });
}
