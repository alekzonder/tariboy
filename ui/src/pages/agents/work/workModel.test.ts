import { describe, expect, it } from "vitest";
import type { Queue, Task } from "@/lib/api";
import { taskItem } from "@/test/fixtures/apiFixtures";
import {
  composerRequest,
  defaultQueue,
  outcomeLabel,
  primaryOutcome,
  statusWords,
  workSections,
} from "./workModel";

const task = (over: Partial<Task>): Task =>
  taskItem({ updated_at: "2026-10-30T10:00:00Z", ...over });
const NOW = Date.parse("2026-10-30T12:00:00Z");

describe("defaultQueue", () => {
  const queues = [{ prefix: "TASK" }, { prefix: "DEV" }] as Queue[];
  it("keeps a remembered queue that exists", () => {
    expect(defaultQueue("DEV", queues)).toBe("DEV");
  });
  it("falls back to TASK for a missing or empty one", () => {
    expect(defaultQueue("GONE", queues)).toBe("TASK");
    expect(defaultQueue("", queues)).toBe("TASK");
  });
});

describe("composerRequest", () => {
  const base = {
    title: "  Add a README ",
    description: "",
    priority: "P2" as const,
    queue: "TASK",
    agent: "bob",
    idempotencyKey: "k-1",
  };
  it("assigns a flexible queue's task to the agent", () => {
    expect(composerRequest({ ...base, bound: false })).toEqual({
      queue: "TASK",
      title: "Add a README",
      priority: "P2",
      idempotency_key: "k-1",
      assignee: "agent:bob",
    });
  });
  it("sends no assignee to a bound queue and keeps a description", () => {
    expect(composerRequest({ ...base, queue: "DEV", bound: true, description: "Why" })).toEqual({
      queue: "DEV",
      title: "Add a README",
      priority: "P2",
      idempotency_key: "k-1",
      description: "Why",
    });
  });
});

describe("statusWords", () => {
  it.each([
    [{ workflow_digest: "d", status: "backlog", workflow_initial: true }, "Not started"],
    [
      { workflow_digest: "d", status: "approval", waiting_on: "customer" as const },
      "Waiting for you",
    ],
    [{ workflow_digest: "d", status: "plan", waiting_on: "pause" as const }, "Paused"],
    [
      { workflow_digest: "d", status: "review", waiting_on: "script" as const },
      "Waiting on a check",
    ],
    [{ workflow_digest: "d", status: "in_review" }, "In review"],
    [{ status: "wait_customer" }, "Waiting for you"],
    [{ status: "in_progress" }, "In progress"],
  ])("%o reads %s", (over, words) => {
    expect(statusWords(task(over))).toBe(words);
  });
});

describe("primaryOutcome", () => {
  it("picks the first outcome that does not end the task", () => {
    const backlog = task({
      workflow_outcomes: [
        { on: "drop", to: "dropped", terminal: true },
        { on: "start", to: "plan" },
      ],
    });
    expect(primaryOutcome(backlog)).toEqual({ on: "start", to: "plan" });
    expect(outcomeLabel({ on: "start", to: "plan" })).toBe("Start");
    expect(outcomeLabel({ on: "approved", to: "implement" })).toBe("approved → implement");
  });
  it("falls back to the first when all end the task, and to none without outcomes", () => {
    expect(
      primaryOutcome(task({ workflow_outcomes: [{ on: "drop", to: "x", terminal: true }] })),
    ).toEqual({ on: "drop", to: "x", terminal: true });
    expect(primaryOutcome(task({}))).toBeUndefined();
  });
});

describe("workSections", () => {
  const active = [
    task({ key: "A-1", category: "in_progress", status: "in_progress" }),
    task({ key: "A-2", category: "open", status: "open" }),
    task({ key: "A-3", workflow_digest: "d", category: "wait_customer", workflow_initial: true }),
    task({ key: "A-4", workflow_digest: "d", category: "wait_customer", waiting_on: "customer" }),
  ];
  const closed = [
    task({ key: "D-1", title: "Old", category: "done", completed_at: "2026-10-01T00:00:00Z" }),
    task({ key: "D-2", title: "Readme", category: "done", completed_at: "2026-10-29T00:00:00Z" }),
    task({
      key: "D-3",
      title: "Lint",
      category: "cancelled",
      completed_at: "2026-10-30T00:00:00Z",
    }),
  ];
  const keys = (tasks: Task[]) => tasks.map((item) => item.key);

  it("puts initial-status and open tasks under Queued and leaves out Needs you", () => {
    const sections = workSections({
      active,
      closed,
      needs: new Set(["A-4"]),
      now: NOW,
      showAll: false,
      search: "",
    });
    expect(keys(sections.inProgress)).toEqual(["A-1"]);
    expect(keys(sections.queued).sort()).toEqual(["A-2", "A-3"]);
    expect(keys(sections.done)).toEqual(["D-3", "D-2"]);
  });

  it("shows every closed task with Show all and filters Done by key or title", () => {
    const all = workSections({
      active,
      closed,
      needs: new Set(),
      now: NOW,
      showAll: true,
      search: "",
    });
    expect(keys(all.done)).toEqual(["D-3", "D-2", "D-1"]);
    const searched = workSections({
      active,
      closed,
      needs: new Set(),
      now: NOW,
      showAll: true,
      search: "readme",
    });
    expect(keys(searched.done)).toEqual(["D-2"]);
    expect(
      keys(
        workSections({ active, closed, needs: new Set(), now: NOW, showAll: true, search: "d-1" })
          .done,
      ),
    ).toEqual(["D-1"]);
  });
});
