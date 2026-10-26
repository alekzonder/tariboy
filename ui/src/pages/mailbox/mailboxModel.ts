import type { Letter, MailBox, MailUnread } from "@/lib/api";
import { letterTaskKey } from "@/lib/letters";

export const BOXES: ReadonlyArray<{ value: MailBox; label: string }> = [
  { value: "inbox", label: "Inbox" },
  { value: "sent", label: "Sent" },
  { value: "archive", label: "Archive" },
];

/** Whose mailbox it is: the customer's own, or an agent's read by the operator. */
export type MailboxMode = "customer" | "agent";

const QUESTION_KIND = "task.question";

/** `agent:dev` reads `dev`; a broadcast lists each recipient. */
export function principalName(principal: string): string {
  return principal
    .split(",")
    .map((part) => part.slice(part.indexOf(":") + 1))
    .join(", ");
}

const isOpen = (letter: Letter) => !letter.answered_at && !letter.dead_at;

export function letterState(letter: Letter): string {
  if (letter.dead_at) return `dead · ${letter.dead_reason}`;
  if (letter.answer_id) return "answered";
  if (letter.answered_at) return letter.close_note ? `archived · ${letter.close_note}` : "archived";
  return "open";
}

export interface LetterActions {
  reply: boolean;
  archive: boolean;
  requeue: boolean;
  /** A task question is answered on its task: the daemon refuses a reply. */
  answerOnTask: boolean;
  taskLink: boolean;
}

export function letterActions(letter: Letter, box: MailBox, mode: MailboxMode): LetterActions {
  const received = box !== "sent";
  const open = received && isOpen(letter);
  const question = letter.kind === QUESTION_KIND && letterTaskKey(letter) !== "";
  return {
    reply: open && letter.needs_reply && !question,
    archive: open,
    requeue: mode === "agent" && received && !!letter.dead_at,
    answerOnTask: open && question,
    taskLink: !(open && question) && letterTaskKey(letter) !== "",
  };
}

/** Opening an unread received letter reads it; the daemon closes a read notice. */
export function marksReadOnOpen(letter: Letter, box: MailBox): boolean {
  return box === "inbox" && !letter.read_at;
}

/** Open task questions per asking agent name, from GET /api/mail/unread. */
export function questionCountsBySender(unread: MailUnread): Map<string, number> {
  const counts = new Map<string, number>();
  for (const row of unread.by_sender) {
    if (row.kind !== QUESTION_KIND || !row.sender.startsWith("agent:")) continue;
    const name = principalName(row.sender);
    if (!name) continue;
    counts.set(name, (counts.get(name) ?? 0) + row.count);
  }
  return counts;
}
