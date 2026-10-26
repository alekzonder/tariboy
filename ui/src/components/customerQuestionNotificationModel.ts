import { letterTaskKey, type Letter, type MailUnread } from "@/lib/api";
import { principalName, questionCountsBySender } from "@/pages/mailbox/mailboxModel";

export const CUSTOMER_QUESTION_RETRY_MS = 3000;

export function customerQuestionAttentionKey(hostId: string, agentName: string): string {
  return JSON.stringify([hostId, agentName]);
}

export type FreshQuestion = {
  letter: Letter;
  agentName: string;
};

/** Attention key -> open task questions from that agent: the questions waiting
 *  for the customer's answer. Answering on the task closes the letter. */
export function questionAttention(hostId: string, unread: MailUnread): Map<string, number> {
  return new Map(
    [...questionCountsBySender(unread)].map(([agent, count]) => [
      customerQuestionAttentionKey(hostId, agent),
      count,
    ]),
  );
}

/** Whether any agent asks more than before: only then can a question be new. */
export function questionsRose(
  previous: ReadonlyMap<string, number>,
  next: ReadonlyMap<string, number>,
): boolean {
  return [...next].some(([key, count]) => count > (previous.get(key) ?? 0));
}

/** Reads the customer's inbox. A baseline records ids silently so a mount or
 *  recovery never replays alerts for questions that already existed. */
export function readFreshQuestions(
  letters: readonly Letter[],
  observed: ReadonlySet<string>,
  baseline: boolean,
): { fresh: FreshQuestion[]; observed: Set<string> } {
  const nextObserved = new Set(observed);
  const fresh: FreshQuestion[] = [];
  for (const letter of letters) {
    const isNew = !nextObserved.has(letter.id);
    nextObserved.add(letter.id);
    if (baseline || !isNew || letter.read_at || letter.kind !== "task.question") continue;
    if (!letter.from.startsWith("agent:") || !letterTaskKey(letter)) continue;
    const agentName = principalName(letter.from);
    if (agentName) fresh.push({ letter, agentName });
  }
  return { fresh, observed: nextObserved };
}
