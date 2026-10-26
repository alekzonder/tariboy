import { describe, expect, it } from "vitest";
import type { MailUnread } from "@/lib/api";
import { letter } from "@/test/fixtures/mailFixtures";
import {
  customerQuestionAttentionKey,
  questionAttention,
  questionsRose,
  readFreshQuestions,
} from "./customerQuestionNotificationModel";

const question = (id: string, over = {}) =>
  letter({
    id,
    kind: "task.question",
    from: "agent:alpha",
    data: { task_key: `T-${id}` },
    ...over,
  });

function unread(...rows: Array<[sender: string, kind: string, count: number]>): MailUnread {
  return {
    as: "user:customer",
    open: rows.reduce((sum, [, , count]) => sum + count, 0),
    unread: 0,
    by_sender: rows.map(([sender, kind, count]) => ({ sender, kind, count })),
  };
}

describe("questionAttention", () => {
  it("counts open task questions per asking agent", () => {
    const attention = questionAttention(
      "h1",
      unread(
        ["agent:alpha", "task.question", 2],
        ["agent:alpha", "message", 4],
        ["agent:", "task.question", 1],
        ["user:bob", "task.question", 1],
      ),
    );
    expect(attention).toEqual(new Map([[customerQuestionAttentionKey("h1", "alpha"), 2]]));
  });

  it("questionBadgeDropsWhenLetterCloses", () => {
    const before = questionAttention("h1", unread(["agent:alpha", "task.question", 1]));
    const after = questionAttention("h1", unread());
    expect(before.get(customerQuestionAttentionKey("h1", "alpha"))).toBe(1);
    expect(after.size).toBe(0);
    expect(questionsRose(before, after)).toBe(false);
  });

  it("rises only when an agent asks more than before", () => {
    const one = new Map([["a", 1]]);
    expect(questionsRose(one, new Map([["a", 1]]))).toBe(false);
    expect(questionsRose(one, new Map([["a", 2]]))).toBe(true);
    expect(questionsRose(one, new Map([["b", 1]]))).toBe(true);
  });
});

describe("readFreshQuestions", () => {
  it("keeps the baseline silent but records its ids", () => {
    const read = readFreshQuestions([question("n1")], new Set(), true);
    expect(read.fresh).toEqual([]);
    expect([...read.observed]).toEqual(["n1"]);
  });

  it("announces a question first seen after the baseline, once", () => {
    const n2 = question("n2", { from: "agent:beta" });
    const read = readFreshQuestions([question("n1"), n2], new Set(["n1"]), false);
    expect(read.fresh).toEqual([{ letter: n2, agentName: "beta" }]);
    expect(readFreshQuestions([n2], read.observed, false).fresh).toEqual([]);
  });

  it("skips read, non-question, non-agent and task-less letters", () => {
    const letters = [
      question("n1", { read_at: "2026-01-01T00:00:00Z" }),
      question("n2", { kind: "message" }),
      question("n3", { from: "user:bob" }),
      question("n4", { data: {} }),
      question("n5", { from: "agent:" }),
    ];
    const read = readFreshQuestions(letters, new Set(), false);
    expect(read.fresh).toEqual([]);
    expect(read.observed.size).toBe(5);
  });

  it("does not change the observed set it was given", () => {
    const observed = new Set(["n1"]);
    readFreshQuestions([question("n2")], observed, false);
    expect([...observed]).toEqual(["n1"]);
  });
});
