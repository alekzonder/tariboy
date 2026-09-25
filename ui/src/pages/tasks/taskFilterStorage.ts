// The queue filter has to survive the Agents <-> All tasks switch, which mounts
// a different workspace, so it lives beside the session rather than in state
// alone. It is per-session on purpose: a filter that outlived a restart would
// greet the next launch with a list that looks empty for no visible reason.
const TASK_QUEUE_FILTER_KEY = "tasks:queue-filter:v1"

export function readTaskQueueFilter(): string {
  try {
    return globalThis.sessionStorage?.getItem(TASK_QUEUE_FILTER_KEY) ?? ""
  } catch {
    return ""
  }
}

export function persistTaskQueueFilter(prefix: string): void {
  try {
    globalThis.sessionStorage?.setItem(TASK_QUEUE_FILTER_KEY, prefix)
  } catch {
    // Web Storage is a best-effort Desktop convenience.
  }
}
