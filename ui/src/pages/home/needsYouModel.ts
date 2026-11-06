import type { HostNeeds, NeedsYouRow } from "@/lib/needsYou";
import { needsYouRows } from "@/lib/needsYou";
import { letterTaskKey } from "@/lib/letters";
import { principalName } from "@/pages/mailbox/mailboxModel";
import { formatAge } from "./homeModel";

/** Rows Home shows; the rest are behind "Show all". */
export const NEEDS_YOU_LIMIT = 7;
/** The inbox read of `NeedsYouLive` returns at most this many letters. */
export const INBOX_READ_LIMIT = 100;

/** One row of Needs you, formatted for the view. */
export interface NeedsYouViewRow {
  id: string;
  kind: "task" | "letter";
  hostId: string;
  /** The task key, or the letter that opens: the newest of a group. */
  ref: string;
  /** The task key, or the sender of a letter. */
  label: string;
  /** The task title, or the letter subject without its "Answer on KEY:" prefix. */
  title: string;
  meta: string;
}

export interface NeedsYouView {
  rows: NeedsYouViewRow[];
  /** All rows, not only the shown ones. */
  total: number;
  /** `total` is a lower bound: an inbox read hit its limit. */
  atLeast: boolean;
  /** Where "Show all" goes: Inbox when every row is a letter. */
  showAll: "inbox" | "tasks";
}

const PREFIX = /^((?:Answer|Question) on \S+?):\s*/;

function viewRow(row: NeedsYouRow, count: number, multiHost: boolean, now: Date): NeedsYouViewRow {
  const host = multiHost ? row.hostLabel : "";
  const age = formatAge(row.at, now);
  if (row.kind === "task") {
    const { task } = row;
    const state = task.waiting_on === "pause" ? "paused" : "waiting for you";
    return {
      id: `${row.hostId}\u0000task\u0000${task.key}`,
      kind: "task",
      hostId: row.hostId,
      ref: task.key,
      label: task.key,
      title: task.title,
      meta: [state, task.assignee, host, age].filter(Boolean).join(" · "),
    };
  }
  const subject = row.letter.subject || "(no subject)";
  const prefix = PREFIX.exec(subject);
  return {
    id: `${row.hostId}\u0000letter\u0000${row.letter.id}`,
    kind: "letter",
    hostId: row.hostId,
    ref: row.letter.id,
    label: principalName(row.letter.from),
    title: prefix ? subject.slice(prefix[0].length) || subject : subject,
    // The count rides in the meta: the 6rem label column cuts a long sender.
    meta: [count > 1 ? `${count} letters` : "", prefix?.[1], host, age].filter(Boolean).join(" · "),
  };
}

/** Letters of one sender about one task are one row (the newest, as the rows
 *  arrive newest first). Letters with no task key stay apart. */
function groupLetters(rows: NeedsYouRow[]): Array<{ row: NeedsYouRow; count: number }> {
  const groups = new Map<string, { row: NeedsYouRow; count: number }>();
  const out: Array<{ row: NeedsYouRow; count: number }> = [];
  for (const row of rows) {
    const key = row.kind === "letter" ? letterTaskKey(row.letter) : "";
    const groupKey = key
      ? `${row.hostId}\u0000${key}\u0000${row.kind === "letter" && row.letter.from}`
      : "";
    const group = groupKey ? groups.get(groupKey) : undefined;
    if (group) {
      group.count++;
      continue;
    }
    const entry = { row, count: 1 };
    if (groupKey) groups.set(groupKey, entry);
    out.push(entry);
  }
  return out;
}

/** What Home shows in Needs you: the most urgent rows (tasks, then letters,
 *  newest first), the count of all of them and the "Show all" target. */
export function needsYouView(
  hosts: HostNeeds[],
  multiHost: boolean,
  now: Date,
  limit = NEEDS_YOU_LIMIT,
): NeedsYouView {
  const groups = groupLetters(needsYouRows(hosts));
  return {
    rows: groups.slice(0, limit).map(({ row, count }) => viewRow(row, count, multiHost, now)),
    total: groups.length,
    atLeast: hosts.some((host) => host.letters.length >= INBOX_READ_LIMIT),
    showAll: groups.every(({ row }) => row.kind === "letter") ? "inbox" : "tasks",
  };
}
