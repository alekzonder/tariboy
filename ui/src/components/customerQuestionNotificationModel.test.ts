import { describe, expect, it } from "vitest";
import type { TaskNotification } from "@/lib/api";
import {
  customerQuestionAttentionKey,
  readQuestionSnapshot,
} from "./customerQuestionNotificationModel";

function question(id: string, overrides: Partial<TaskNotification> = {}): TaskNotification {
  return {
    id,
    channel: "customer",
    type: "task.question",
    text: "question",
    requesting_principal: "agent:alpha",
    task_key: `T-${id}`,
    event_sequence: 1,
    created_at: "",
    published_at: "",
    read_at: "",
    dismissed_at: "",
    ...overrides,
  };
}

describe("readQuestionSnapshot", () => {
  it("keeps the first snapshot silent but records its ids and attention", () => {
    const snapshot = readQuestionSnapshot("h1", [question("n1")], new Set(), true);

    expect(snapshot.fresh).toEqual([]);
    expect([...snapshot.observed]).toEqual(["n1"]);
    expect(snapshot.attention).toEqual(new Map([[customerQuestionAttentionKey("h1", "alpha"), 1]]));
  });

  it("gives no new question when a refresh repeats an observed notification", () => {
    const first = readQuestionSnapshot("h1", [question("n1")], new Set(), true);
    const repeat = readQuestionSnapshot("h1", [question("n1")], first.observed, false);

    expect(repeat.fresh).toEqual([]);
  });

  it("announces a notification first seen after the baseline", () => {
    const n2 = question("n2", { requesting_principal: "agent:beta" });
    const snapshot = readQuestionSnapshot("h1", [question("n1"), n2], new Set(["n1"]), false);

    expect(snapshot.fresh).toEqual([{ notification: n2, agentName: "beta" }]);
    expect([...snapshot.observed]).toEqual(["n1", "n2"]);
  });

  it("skips a notification without an agent name in attention and announcements", () => {
    const notifications = [
      question("n1", { requesting_principal: "agent:" }),
      question("n2", { requesting_principal: "customer:bob" }),
      question("n3", { read_at: "2026-01-01T00:00:00Z" }),
      question("n4", { type: "task.comment" }),
    ];
    const snapshot = readQuestionSnapshot("h1", notifications, new Set(), false);

    expect(snapshot.fresh).toEqual([]);
    expect(snapshot.attention.size).toBe(0);
    expect(snapshot.observed.size).toBe(4);
  });

  it("counts an agent's distinct tasks, not its notifications", () => {
    const notifications = [
      question("n1", { task_key: "T-1" }),
      question("n2", { task_key: "T-1" }),
      question("n3", { task_key: "T-2" }),
    ];
    const snapshot = readQuestionSnapshot("h1", notifications, new Set(), true);

    expect(snapshot.attention).toEqual(new Map([[customerQuestionAttentionKey("h1", "alpha"), 2]]));
  });

  it("does not change the observed set it was given", () => {
    const observed = new Set(["n1"]);
    readQuestionSnapshot("h1", [question("n2")], observed, false);

    expect([...observed]).toEqual(["n1"]);
  });
});
