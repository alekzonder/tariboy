import { readKey, storageKeys, writeKey } from "@/lib/storage";

// The queue the last task was created in, per server: each server has its own
// queues. Unlike the queue filter it lives in localStorage, so the choice
// survives a restart.
function readAll(): Record<string, unknown> {
  const parsed = readKey(storageKeys.newTaskQueue);
  return parsed && typeof parsed === "object" && !Array.isArray(parsed)
    ? (parsed as Record<string, unknown>)
    : {};
}

export function readRememberedQueue(server: string): string {
  const prefix = readAll()[server];
  return typeof prefix === "string" ? prefix : "";
}

export function rememberQueue(server: string, prefix: string): void {
  // Web Storage is a best-effort Desktop convenience.
  writeKey(storageKeys.newTaskQueue, { ...readAll(), [server]: prefix });
}
