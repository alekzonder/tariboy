import { describe, expect, it } from "vitest";
import { taskItem } from "@/fixtures/tasks";
import type { HostNeeds } from "@/lib/needsYou";
import { letter } from "@/test/fixtures/mailFixtures";
import { INBOX_READ_LIMIT, NEEDS_YOU_LIMIT, needsYouView } from "./needsYouModel";

const NOW = new Date("2026-11-06T12:00:00Z");
const iso = (minutesAgo: number) => new Date(NOW.getTime() - minutesAgo * 60_000).toISOString();

const host = (over: Partial<HostNeeds> = {}): HostNeeds => ({
  hostId: "",
  hostLabel: "Local",
  tasks: [],
  letters: [],
  open: 0,
  ...over,
});
const note = (id: string, over: Parameters<typeof letter>[0] = {}) =>
  letter({
    id,
    from: "agent:image-creator",
    subject: "Answer on TARI-1: which image?",
    data: { task_key: "TARI-1" },
    created_at: iso(10),
    ...over,
  });
const waiting = (key: string, minutesAgo: number) =>
  taskItem({
    key,
    title: `Task ${key}`,
    status: "wait_customer",
    waiting_on: "customer",
    assignee: "bob",
    updated_at: iso(minutesAgo),
  });
const view = (hosts: HostNeeds[], multiHost = false) => needsYouView(hosts, multiHost, NOW);

describe("needsYouView", () => {
  it("keeps the 7 most urgent of 100 rows and counts all of them", () => {
    const letters = Array.from({ length: 100 }, (_, i) =>
      note(`m${i}`, { data: { task_key: `T-${i}` }, created_at: iso(i + 1) }),
    );
    const out = view([host({ tasks: [waiting("A-1", 5)], letters })]);
    expect(NEEDS_YOU_LIMIT).toBe(7);
    expect(out.rows).toHaveLength(7);
    expect(out.rows[0].label).toBe("A-1");
    expect(out.rows[1].id).toContain("m0");
    expect(out.total).toBe(101);
  });

  it("merges the letters of one sender about one task into one row of the newest letter", () => {
    const letters = [1, 2, 3, 4].map((i) => note(`m${i}`, { created_at: iso(i) }));
    const out = view([host({ letters })]);
    expect(out.rows).toHaveLength(1);
    expect(out.rows[0]).toMatchObject({ ref: "m1", label: "image-creator" });
    expect(out.rows[0].meta).toBe("4 letters · Answer on TARI-1 · 1 min ago");
    expect(out.total).toBe(1);
  });

  it("keeps two senders and letters without a task key apart", () => {
    const out = view([
      host({
        letters: [
          note("a"),
          note("b", { from: "agent:docs" }),
          note("c", { data: {}, subject: "Hello" }),
          note("d", { data: {}, subject: "Hello" }),
        ],
      }),
    ]);
    expect(out.rows).toHaveLength(4);
  });

  it("moves the Answer on / Question on prefix to the meta line", () => {
    const [answer, question, plain] = view([
      host({
        letters: [
          note("a", {
            created_at: iso(1),
            data: { task_key: "T-1" },
            subject: "Answer on T-1: which image?",
          }),
          note("b", {
            created_at: iso(2),
            data: { task_key: "T-2" },
            subject: "Question on T-2: which one?",
          }),
          note("c", { created_at: iso(3), data: {}, subject: "Which one?" }),
        ],
      }),
    ]).rows;
    expect(answer.title).toBe("which image?");
    expect(answer.meta).toBe("Answer on T-1 · 1 min ago");
    expect(question.title).toBe("which one?");
    expect(question.meta).toBe("Question on T-2 · 2 min ago");
    expect(plain.title).toBe("Which one?");
    expect(plain.meta).toBe("3 min ago");
  });

  it("names a letter with no subject", () => {
    const [row] = view([host({ letters: [note("a", { subject: "", data: {} })] })]).rows;
    expect(row.title).toBe("(no subject)");
  });

  it("words the meta of a task without the raw status", () => {
    const paused = { ...waiting("P-1", 120), waiting_on: "pause" as const };
    const rows = view([host({ tasks: [waiting("A-1", 12), paused] })], true).rows;
    expect(rows[0].meta).toBe("waiting for you · bob · Local · 12 min ago");
    expect(rows[1].meta).toBe("paused · bob · Local · 2 h ago");
  });

  it("shows the date of an item older than 24 hours", () => {
    const [row] = view([host({ tasks: [waiting("A-1", 3 * 24 * 60)] })]).rows;
    expect(row.meta).toBe("waiting for you · bob · Nov 3");
  });

  it("marks the count as a lower bound when the inbox read hit its limit", () => {
    const letters = Array.from({ length: INBOX_READ_LIMIT }, (_, i) =>
      note(`m${i}`, { data: { task_key: `T-${i}` } }),
    );
    expect(view([host({ letters })]).atLeast).toBe(true);
    expect(view([host({ letters: letters.slice(1) })]).atLeast).toBe(false);
  });

  it("opens Inbox when every row is a letter and Tasks otherwise", () => {
    expect(view([host({ letters: [note("a")] })]).showAll).toBe("inbox");
    expect(view([host({ letters: [note("a")], tasks: [waiting("A-1", 1)] })]).showAll).toBe(
      "tasks",
    );
  });
});
