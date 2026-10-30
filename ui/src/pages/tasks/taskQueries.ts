import { queryOptions, type QueryClient } from "@tanstack/react-query";
import type { ApiClient, Task, TaskDetail, TaskEvent, TaskFilters } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";
import { apiServerKey } from "@/lib/queryClient";
import { listAllTasks } from "./listAllTasks";

// Keys are built from the client's host; credentials never enter a key.
type Target = Daemon | null;
export const taskKeys = {
  server: (target: Target) => [...apiServerKey(target), "tasks"] as const,
  list: (target: Target, filters: TaskFilters) =>
    [...taskKeys.server(target), "list", filters] as const,
  detail: (target: Target, key: string) => [...taskKeys.server(target), "detail", key] as const,
  history: (target: Target, key: string, policy: "first" | "latest") =>
    [...taskKeys.server(target), "history", key, policy] as const,
  workflow: (target: Target, key: string) => [...taskKeys.server(target), "workflow", key] as const,
  queues: (target: Target) => [...taskKeys.server(target), "queues"] as const,
  pools: (target: Target, queue: string) => [...taskKeys.server(target), "pools", queue] as const,
  principals: (target: Target) => [...taskKeys.server(target), "principals"] as const,
  notifications: (target: Target) => [...taskKeys.server(target), "notifications"] as const,
};

export const taskQueries = {
  list: (api: ApiClient, filters: TaskFilters) =>
    queryOptions({
      queryKey: taskKeys.list(api.target, filters),
      queryFn: ({ signal }) => listAllTasks(filters, api.tasks, signal),
    }),
  detail: (api: ApiClient, key: string) =>
    queryOptions({
      queryKey: taskKeys.detail(api.target, key),
      queryFn: ({ signal }) => api.tasks.getTask(key, signal),
      enabled: !!key,
    }),
  history: (api: ApiClient, key: string, policy: "first" | "latest") =>
    queryOptions({
      queryKey: taskKeys.history(api.target, key, policy),
      enabled: !!key,
      queryFn: async ({ signal }) => {
        const events: TaskEvent[] = [];
        let after = 0;
        for (;;) {
          signal.throwIfAborted();
          const page = await api.tasks.listTaskEvents(key, after, 200, signal);
          const batch = page.events ?? [];
          events.push(...batch);
          if (policy === "first" || batch.length < 200)
            return policy === "first" ? events : events.slice(-200);
          const next = batch[batch.length - 1]?.sequence ?? after;
          if (next <= after) throw new Error("task history cursor did not advance");
          after = next;
        }
      },
    }),
  workflow: (api: ApiClient, key: string) =>
    queryOptions({
      queryKey: taskKeys.workflow(api.target, key),
      queryFn: ({ signal }) => api.workflows.getTaskWorkflow(key, signal),
    }),
  queues: (api: ApiClient) =>
    queryOptions({
      queryKey: taskKeys.queues(api.target),
      queryFn: ({ signal }) => api.tasks.listTaskQueues(signal),
    }),
  pools: (api: ApiClient, queue: string) =>
    queryOptions({
      queryKey: taskKeys.pools(api.target, queue),
      queryFn: () => api.tasks.listAgentPools(queue),
      enabled: !!queue,
    }),
  principals: (api: ApiClient) =>
    queryOptions({
      queryKey: taskKeys.principals(api.target),
      queryFn: ({ signal }) => api.tasks.listTaskPrincipals(signal),
    }),
  notifications: (api: ApiClient) =>
    queryOptions({
      queryKey: taskKeys.notifications(api.target),
      // The customer's open task questions, newest first.
      queryFn: async ({ signal }) =>
        (await api.mail.list({ box: "inbox", limit: 500 }, signal)).letters
          .filter((letter) => letter.kind === "task.question")
          .reverse(),
    }),
  sequence: (api: ApiClient) =>
    queryOptions({
      queryKey: [...taskKeys.server(api.target), "sequence"],
      queryFn: ({ signal }) => api.tasks.listTasks({ limit: 1 }, signal),
    }),
};

export async function refreshTasks(client: QueryClient, api: ApiClient) {
  await client.cancelQueries({ queryKey: taskKeys.server(api.target) });
  await client.invalidateQueries({ queryKey: taskKeys.server(api.target) });
}

export async function writeTask<T>(
  client: QueryClient,
  api: ApiClient,
  write: () => Promise<T>,
): Promise<T> {
  await client.cancelQueries({ queryKey: taskKeys.server(api.target) });
  try {
    return await write();
  } finally {
    await refreshTasks(client, api);
  }
}

/** A mutation response can fill an older snapshot, but never lower a revision. */
export function retainTaskRevision(client: QueryClient, api: ApiClient, updated: Task): Task {
  const detailKey = taskKeys.detail(api.target, updated.key);
  client.setQueryData<TaskDetail>(detailKey, (current) =>
    current && current.task.revision < updated.revision ? { ...current, task: updated } : current,
  );
  client.setQueriesData<{ tasks: Task[] }>(
    { queryKey: [...taskKeys.server(api.target), "list"] },
    (page) =>
      page
        ? {
            ...page,
            tasks: page.tasks.map((task) =>
              task.key === updated.key && task.revision < updated.revision ? updated : task,
            ),
          }
        : page,
  );
  return client.getQueryData<TaskDetail>(detailKey)?.task ?? updated;
}
