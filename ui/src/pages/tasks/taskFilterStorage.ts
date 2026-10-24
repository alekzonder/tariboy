import { readKey, storageKeys, writeKey } from "@/lib/storage";

// The queue filter has to survive the Agents <-> All tasks switch, which mounts
// a different workspace, so it lives beside the session rather than in state
// alone. It is per-session on purpose: a filter that outlived a restart would
// greet the next launch with a list that looks empty for no visible reason.
export function readTaskQueueFilter(): string {
  return readKey(storageKeys.taskQueueFilter);
}

export function persistTaskQueueFilter(prefix: string): void {
  // Web Storage is a best-effort Desktop convenience.
  writeKey(storageKeys.taskQueueFilter, prefix);
}
