import type { CreateTaskInput, Queue, Task, TaskPriority } from "@/lib/api";
import { flexibleStatusLabel } from "@/lib/statusTone";
import { isWorkflowTask, taskStatusLabel } from "@/lib/tasks";

/** The queue every daemon has (`EnsureDefaultQueue`). */
export const DEFAULT_QUEUE = "TASK";
const WEEK_MS = 7 * 24 * 60 * 60 * 1000;

/** The composer's queue: the one remembered for this agent while it exists,
 *  else `TASK`. */
export function defaultQueue(remembered: string, queues: readonly Queue[]): string {
  return remembered && queues.some((queue) => queue.prefix === remembered)
    ? remembered
    : DEFAULT_QUEUE;
}

/** The create request of the composer. A bound queue refuses an assignee
 *  (`workflow_managed`): its workflow dispatches the task to a pool. */
export function composerRequest({
  title,
  description,
  priority,
  queue,
  agent,
  bound,
  idempotencyKey,
}: {
  title: string;
  description: string;
  priority: TaskPriority;
  queue: string;
  agent: string;
  bound: boolean;
  idempotencyKey: string;
}): CreateTaskInput {
  return {
    queue,
    title: title.trim(),
    priority,
    idempotency_key: idempotencyKey,
    ...(description.trim() ? { description } : {}),
    ...(bound ? {} : { assignee: `agent:${agent}` }),
  };
}

/** A task's status in words, as a row shows it. */
export function statusWords(task: Task): string {
  if (task.workflow_initial) return "Not started";
  if (task.waiting_on === "customer") return "Waiting for you";
  if (task.waiting_on === "pause") return "Paused";
  if (task.waiting_on === "script") return "Waiting on a check";
  return isWorkflowTask(task) ? taskStatusLabel(task) : flexibleStatusLabel(task.status);
}

type Outcome = NonNullable<Task["workflow_outcomes"]>[number];

/** The outcome a Queued row offers first: the first declared one that does
 *  not end the task, else the first. */
export function primaryOutcome(task: Task): Outcome | undefined {
  const outcomes = task.workflow_outcomes ?? [];
  return outcomes.find((outcome) => !outcome.terminal) ?? outcomes[0];
}

/** "Start" for an outcome named start, otherwise "<outcome> → <status>". */
export const outcomeLabel = (outcome: Outcome) =>
  outcome.on === "start" ? "Start" : `${outcome.on} → ${outcome.to}`;

export interface WorkSections {
  inProgress: Task[];
  queued: Task[];
  done: Task[];
}

const newest = (left: string, right: string) => right.localeCompare(left);

/** The rows of In progress, Queued and Done. A task the Needs you list shows
 *  is left out of the others. Done holds the last seven days unless `showAll`,
 *  filtered by `search` on the key and the title. */
export function workSections({
  active,
  closed,
  needs,
  now,
  showAll,
  search,
}: {
  active: readonly Task[];
  closed: readonly Task[];
  needs: ReadonlySet<string>;
  now: number;
  showAll: boolean;
  search: string;
}): WorkSections {
  const inProgress: Task[] = [];
  const queued: Task[] = [];
  for (const task of active) {
    if (needs.has(task.key)) continue;
    if (task.workflow_initial || task.category === "open") queued.push(task);
    else inProgress.push(task);
  }
  const text = search.trim().toLowerCase();
  const done = closed
    .filter((task) => showAll || now - Date.parse(task.completed_at || task.updated_at) <= WEEK_MS)
    .filter(
      (task) =>
        !text || task.key.toLowerCase().includes(text) || task.title.toLowerCase().includes(text),
    )
    .sort((left, right) =>
      newest(left.completed_at || left.updated_at, right.completed_at || right.updated_at),
    );
  return {
    inProgress: inProgress.sort((left, right) => newest(left.updated_at, right.updated_at)),
    queued: queued.sort((left, right) => newest(left.created_at, right.created_at)),
    done,
  };
}
