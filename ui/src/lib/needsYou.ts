import type { Letter, Task } from "./api";
import { letterTaskKey } from "./letters";

// Type-only imports: the shell's eager modules use this file without pulling
// the API client into the entry chunk.

/** One host's share of "needs you": its active tasks and the customer's
 *  inbox, or the error that kept them from loading. */
export interface HostNeeds {
  hostId: string;
  hostLabel: string;
  tasks: Task[];
  letters: Letter[];
  /** The customer's open letters, as `GET /api/mail/unread` counts them. */
  open: number;
  error?: string;
}

export type NeedsYouRow =
  | { kind: "task"; id: string; hostId: string; hostLabel: string; at: string; task: Task }
  | { kind: "letter"; id: string; hostId: string; hostLabel: string; at: string; letter: Letter };

/** A task waits on the user: a workflow waits for the customer past its
 *  first status (a task in its initial status waits for the customer to start
 *  it, which is not a request), or a paused workflow. */
export function needsYouTask(task: Task): boolean {
  if (task.waiting_on === "pause") return true;
  return task.waiting_on === "customer" && !task.workflow_initial;
}

const openLetter = (letter: Letter) => !letter.answered_at && !letter.dead_at;
const newestFirst = (left: NeedsYouRow, right: NeedsYouRow) => right.at.localeCompare(left.at);

/** The rows of Home and Inbox: tasks that need the user, then the open
 *  letters, each newest first. A question letter about a listed task is that
 *  task's row already. */
export function needsYouRows(hosts: HostNeeds[]): NeedsYouRow[] {
  const tasks: NeedsYouRow[] = [];
  const letters: NeedsYouRow[] = [];
  for (const { hostId, hostLabel, ...host } of hosts) {
    const listed = new Set<string>();
    for (const task of host.tasks.filter(needsYouTask)) {
      listed.add(task.key);
      tasks.push({ kind: "task", id: task.key, hostId, hostLabel, at: task.updated_at, task });
    }
    for (const letter of host.letters) {
      if (!openLetter(letter) || listed.has(letterTaskKey(letter))) continue;
      letters.push({
        kind: "letter",
        id: letter.id,
        hostId,
        hostLabel,
        at: letter.created_at,
        letter,
      });
    }
  }
  return [...tasks.sort(newestFirst), ...letters.sort(newestFirst)];
}

/** The sidebar badges. A host that failed adds nothing; with no host
 *  answering there is no count at all, rather than a misleading 0. */
export function needsYouCounts(hosts: HostNeeds[]): {
  tasks: number | null;
  inbox: number | null;
} {
  const answered = hosts.filter((host) => !host.error);
  if (answered.length === 0) return { tasks: null, inbox: null };
  return {
    tasks: answered.reduce((sum, host) => sum + host.tasks.filter(needsYouTask).length, 0),
    inbox: answered.reduce((sum, host) => sum + host.open, 0),
  };
}
