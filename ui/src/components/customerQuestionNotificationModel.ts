import type { TaskNotification } from "@/lib/api";

export const CUSTOMER_QUESTION_RETRY_MS = 3000;

export function customerQuestionAttentionKey(hostId: string, agentName: string): string {
  return JSON.stringify([hostId, agentName]);
}

function requestingAgent(notification: TaskNotification): string | null {
  if (notification.read_at || notification.type !== "task.question") return null;
  return notification.requesting_principal.startsWith("agent:")
    ? notification.requesting_principal.slice("agent:".length)
    : null;
}

export type FreshQuestion = {
  notification: TaskNotification;
  agentName: string;
};

export type QuestionSnapshot = {
  // Attention key -> number of that agent's tasks with an unread question.
  attention: Map<string, number>;
  // Questions first seen after the baseline, in inbox order.
  fresh: FreshQuestion[];
  // Every notification id seen so far, including this inbox.
  observed: Set<string>;
};

/** Reads one authoritative inbox. A baseline records ids silently so a mount or
 *  recovery never replays alerts for questions that already existed. */
export function readQuestionSnapshot(
  hostId: string,
  notifications: readonly TaskNotification[],
  observed: ReadonlySet<string>,
  baseline: boolean,
): QuestionSnapshot {
  const nextObserved = new Set(observed);
  const fresh: FreshQuestion[] = [];
  const tasks = new Map<string, Set<string>>();
  for (const notification of notifications) {
    const isNew = !nextObserved.has(notification.id);
    nextObserved.add(notification.id);
    const agentName = requestingAgent(notification);
    if (!agentName) continue;
    if (!baseline && isNew) fresh.push({ notification, agentName });
    const key = customerQuestionAttentionKey(hostId, agentName);
    tasks.set(key, (tasks.get(key) ?? new Set()).add(notification.task_key));
  }
  const attention = new Map([...tasks].map(([key, keys]) => [key, keys.size]));
  return { attention, fresh, observed: nextObserved };
}
