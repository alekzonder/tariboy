import type { Task, TaskFilters, TasksApi } from "@/lib/api";

// listAllTasks follows the cursor to the end and reports the newest event
// sequence seen, which is where a live socket for the same list resumes.
export async function listAllTasks(
  filters: TaskFilters,
  api: TasksApi,
  signal?: AbortSignal,
): Promise<{ tasks: Task[]; sequence: number }> {
  const tasks: Task[] = [];
  let after = "";
  let sequence = 0;
  const seenCursors = new Set<string>();
  do {
    signal?.throwIfAborted();
    const page = await (signal
      ? api.listTasks({ ...filters, limit: 500, after }, signal)
      : api.listTasks({ ...filters, limit: 500, after }));
    tasks.push(...(page.tasks ?? []));
    sequence = Math.max(sequence, page.sequence ?? 0);
    after = page.next_cursor ?? "";
    if (after && seenCursors.has(after)) throw new Error("task pagination cursor repeated");
    if (after) seenCursors.add(after);
  } while (after);
  return { tasks, sequence };
}
