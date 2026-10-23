import { queryOptions, type QueryClient } from "@tanstack/react-query"
import type { ApiTarget } from "@/lib/api"
import { apiServerKey } from "@/lib/queryClient"
import { getTask, getTaskWorkflow, listAgentPools, listTaskEvents, listTaskNotifications, listTaskPrincipals, listTaskQueues, listTasks, type Task, type TaskDetail, type TaskEvent, type TaskFilters } from "@/lib/tasks"
import { listAllTasks } from "./useAllServersTasks"

type Target = Exclude<ApiTarget, undefined>
export const taskKeys = {
  server: (target: Target) => [...apiServerKey(target), "tasks"] as const,
  list: (target: Target, filters: TaskFilters) => [...taskKeys.server(target), "list", filters] as const,
  detail: (target: Target, key: string) => [...taskKeys.server(target), "detail", key] as const,
  history: (target: Target, key: string, policy: "first" | "latest") => [...taskKeys.server(target), "history", key, policy] as const,
  workflow: (target: Target, key: string) => [...taskKeys.server(target), "workflow", key] as const,
  queues: (target: Target) => [...taskKeys.server(target), "queues"] as const,
  pools: (target: Target, queue: string) => [...taskKeys.server(target), "pools", queue] as const,
  principals: (target: Target) => [...taskKeys.server(target), "principals"] as const,
  notifications: (target: Target) => [...taskKeys.server(target), "notifications"] as const,
}

export const taskQueries = {
  list: (target: Target, filters: TaskFilters) => queryOptions({
    queryKey: taskKeys.list(target, filters), queryFn: ({ signal }) => listAllTasks(filters, target, signal),
  }),
  detail: (target: Target, key: string) => queryOptions({
    queryKey: taskKeys.detail(target, key), queryFn: ({ signal }) => getTask(key, target, signal), enabled: !!key,
  }),
  history: (target: Target, key: string, policy: "first" | "latest") => queryOptions({
    queryKey: taskKeys.history(target, key, policy), enabled: !!key,
    queryFn: async ({ signal }) => {
      const events: TaskEvent[] = []
      let after = 0
      for (;;) {
        signal.throwIfAborted()
        const page = await listTaskEvents(key, after, 200, target, signal)
        const batch = page.events ?? []
        events.push(...batch)
        if (policy === "first" || batch.length < 200) return policy === "first" ? events : events.slice(-200)
        const next = batch[batch.length - 1]?.sequence ?? after
        if (next <= after) throw new Error("task history cursor did not advance")
        after = next
      }
    },
  }),
  workflow: (target: Target, key: string) => queryOptions({
    queryKey: taskKeys.workflow(target, key), queryFn: ({ signal }) => getTaskWorkflow(key, target, signal),
  }),
  queues: (target: Target) => queryOptions({
    queryKey: taskKeys.queues(target), queryFn: ({ signal }) => listTaskQueues(target, signal),
  }),
  pools: (target: Target, queue: string) => queryOptions({
    queryKey: taskKeys.pools(target, queue), queryFn: () => listAgentPools(queue, target), enabled: !!queue,
  }),
  principals: (target: Target) => queryOptions({
    queryKey: taskKeys.principals(target), queryFn: ({ signal }) => listTaskPrincipals(target, signal),
  }),
  notifications: (target: Target) => queryOptions({
    queryKey: taskKeys.notifications(target), queryFn: ({ signal }) => listTaskNotifications(false, target, signal),
  }),
  sequence: (target: Target) => queryOptions({
    queryKey: [...taskKeys.server(target), "sequence"], queryFn: ({ signal }) => listTasks({ limit: 1 }, target, signal),
  }),
}

export async function refreshTasks(client: QueryClient, target: Target) {
  await client.cancelQueries({ queryKey: taskKeys.server(target) })
  await client.invalidateQueries({ queryKey: taskKeys.server(target) })
}

export async function writeTask<T>(client: QueryClient, target: Target, write: () => Promise<T>): Promise<T> {
  await client.cancelQueries({ queryKey: taskKeys.server(target) })
  try { return await write() }
  finally { await refreshTasks(client, target) }
}

/** A mutation response can fill an older snapshot, but never lower a revision. */
export function retainTaskRevision(client: QueryClient, target: Target, updated: Task): Task {
  const detailKey = taskKeys.detail(target, updated.key)
  client.setQueryData<TaskDetail>(detailKey, (current) => current && current.task.revision < updated.revision
    ? { ...current, task: updated } : current)
  client.setQueriesData<{ tasks: Task[] }>({ queryKey: [...taskKeys.server(target), "list"] }, (page) => page ? {
    ...page, tasks: page.tasks.map((task) => task.key === updated.key && task.revision < updated.revision ? updated : task),
  } : page)
  return client.getQueryData<TaskDetail>(detailKey)?.task ?? updated
}
