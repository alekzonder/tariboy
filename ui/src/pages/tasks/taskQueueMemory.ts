// The queue the last task was created in, per server: each server has its own
// queues. Unlike the queue filter it lives in localStorage, so the choice
// survives a restart.
const NEW_TASK_QUEUE_KEY = "tasks:new-task-queue:v1"

function readAll(): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(globalThis.localStorage?.getItem(NEW_TASK_QUEUE_KEY) ?? "{}")
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {}
  } catch {
    return {}
  }
}

export function readRememberedQueue(server: string): string {
  const prefix = readAll()[server]
  return typeof prefix === "string" ? prefix : ""
}

export function rememberQueue(server: string, prefix: string): void {
  try {
    globalThis.localStorage?.setItem(NEW_TASK_QUEUE_KEY, JSON.stringify({ ...readAll(), [server]: prefix }))
  } catch {
    // Web Storage is a best-effort Desktop convenience.
  }
}
