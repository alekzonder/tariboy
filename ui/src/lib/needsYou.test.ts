import { describe, expect, it } from "vitest";
import type { Letter, Task } from "@/lib/api";
import { needsYouCounts, needsYouRows, needsYouTask, type HostNeeds } from "./needsYou";

const task = (over: Partial<Task>): Task => ({ key: "T-1", updated_at: "", ...over }) as Task;
const letter = (over: Partial<Letter>): Letter =>
  ({
    id: "m-1",
    kind: "",
    created_at: "",
    data: {},
    answered_at: "",
    dead_at: "",
    ...over,
  }) as Letter;

describe("needsYouTask", () => {
  it.each([
    ["customer wait after the first status", { waiting_on: "customer" }, true],
    [
      "customer wait in the initial status",
      { waiting_on: "customer", workflow_initial: true },
      false,
    ],
    ["paused", { waiting_on: "pause" }, true],
    ["paused in the initial status", { waiting_on: "pause", workflow_initial: true }, true],
    ["a script", { waiting_on: "script" }, false],
    ["a flexible task", {}, false],
  ] as const)("%s", (_name, over, expected) => {
    expect(needsYouTask(task(over as Partial<Task>))).toBe(expected);
  });
});

const host = (over: Partial<HostNeeds>): HostNeeds => ({
  hostId: "",
  hostLabel: "Local",
  tasks: [],
  letters: [],
  open: 0,
  ...over,
});

describe("needsYouRows", () => {
  it("lists tasks newest first, then open letters newest first", () => {
    const rows = needsYouRows([
      host({
        tasks: [
          task({ key: "A-1", waiting_on: "customer", updated_at: "2026-10-30T10:00:00Z" }),
          task({ key: "A-2", waiting_on: "script", updated_at: "2026-10-30T12:00:00Z" }),
          task({ key: "A-3", waiting_on: "pause", updated_at: "2026-10-30T11:00:00Z" }),
        ],
        letters: [
          letter({ id: "m-old", created_at: "2026-10-30T09:00:00Z" }),
          letter({ id: "m-new", created_at: "2026-10-30T13:00:00Z" }),
          letter({ id: "m-closed", answered_at: "2026-10-30T13:00:00Z" }),
        ],
      }),
    ]);
    expect(rows.map((row) => row.id)).toEqual(["A-3", "A-1", "m-new", "m-old"]);
  });

  it("does not list a question letter whose task is already listed on that host", () => {
    const question = (id: string, key: string) =>
      letter({ id, kind: "task.question", data: { task_key: key } });
    const rows = needsYouRows([
      host({
        hostId: "d1",
        tasks: [task({ key: "A-1", waiting_on: "customer" })],
        letters: [question("m-1", "A-1"), question("m-2", "B-1")],
      }),
      host({ hostId: "d2", letters: [question("m-3", "A-1")] }),
    ]);
    expect(rows.map((row) => `${row.hostId}/${row.id}`)).toEqual(["d1/A-1", "d1/m-2", "d2/m-3"]);
  });
});

describe("needsYouCounts", () => {
  it("adds every host that answered and skips one that failed", () => {
    expect(
      needsYouCounts([
        host({
          tasks: [task({ waiting_on: "customer" }), task({ waiting_on: "script" })],
          open: 2,
        }),
        host({ hostId: "d1", tasks: [task({ waiting_on: "pause" })], open: 1 }),
        host({ hostId: "d2", error: "unreachable" }),
      ]),
    ).toEqual({ tasks: 2, inbox: 3 });
  });

  it("counts a task with an open question once, and a question on a task not listed", () => {
    const question = (id: string, key: string, over: Partial<Letter> = {}) =>
      letter({ id, kind: "task.question", data: { task_key: key }, ...over });
    expect(
      needsYouCounts([
        host({
          tasks: [task({ key: "A-1", waiting_on: "customer" })],
          letters: [
            question("m-1", "A-1"),
            question("m-2", "B-1"),
            question("m-3", "C-1", { answered_at: "2026-10-30T10:00:00Z" }),
            letter({ id: "m-4", data: { task_key: "D-1" } }),
          ],
        }),
      ]).tasks,
    ).toBe(2);
  });

  it("has no count when no host answered", () => {
    expect(needsYouCounts([host({ error: "down" })])).toEqual({ tasks: null, inbox: null });
    expect(needsYouCounts([])).toEqual({ tasks: null, inbox: null });
  });
});
