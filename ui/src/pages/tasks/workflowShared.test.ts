import { describe, expect, it } from "vitest";
import type { Task, TransitionRequest, WorkflowView } from "@/lib/api";
import {
  assignablePool,
  followNoticeText,
  visibleNotice,
  type FollowResult,
  type Notice,
} from "./workflowShared";

describe("followNoticeText", () => {
  it.each<[string, Exclude<FollowResult, { kind: "applied" }>, string]>([
    [
      "a timeout",
      { kind: "timeout", requestId: 3 },
      "Still checking — the result appears here when the checks finish.",
    ],
    [
      "an error",
      { kind: "error", label: "Retry failed", message: "boom", stale: false },
      "Retry failed: boom",
    ],
    [
      "a result",
      {
        kind: "result",
        request: {
          id: 3,
          task_key: "REL-1",
          outcome: "drafted",
          actor: "u",
          state: "rejected",
          created_at: "2026-10-01T10:05:00Z",
          result_message: "no notes",
        },
      },
      "rejected: no notes",
    ],
    [
      "a result without a message",
      {
        kind: "result",
        request: {
          id: 3,
          task_key: "REL-1",
          outcome: "drafted",
          actor: "u",
          state: "failed",
          created_at: "2026-10-01T10:05:00Z",
        },
      },
      "failed: ",
    ],
  ])("reads %s", (_name, notice, text) => {
    expect(followNoticeText(notice)).toBe(text);
  });
});

describe("visibleNotice", () => {
  const req = (id: number, state: TransitionRequest["state"]): TransitionRequest => ({
    id,
    task_key: "REL-1",
    outcome: "approve",
    actor: "u",
    state,
    created_at: "2026-10-01T10:05:00Z",
  });
  const timeout: Notice = { kind: "timeout", requestId: 4 };
  const result: Notice = { kind: "result", request: req(4, "rejected") };
  const error: Notice = { kind: "error", label: "Advance failed", message: "boom" };

  it.each<[string, Notice | null, TransitionRequest | undefined, Notice | null]>([
    ["no notice", null, req(4, "rejected"), null],
    ["a timeout the view has not caught up with", timeout, undefined, timeout],
    ["a timeout whose request still runs", timeout, req(4, "pending"), timeout],
    ["a timeout whose request settled", timeout, req(4, "rejected"), null],
    ["a timeout about another request", timeout, req(5, "rejected"), timeout],
    ["a result the view carries", result, req(4, "rejected"), null],
    ["a result about another request", result, req(5, "failed"), result],
    ["an error", error, req(4, "rejected"), error],
  ])("shows %s as expected", (_name, notice, last, shown) => {
    expect(visibleNotice(notice, last)).toBe(shown);
  });
});

describe("assignablePool", () => {
  const task = { access: "write", category: "in_progress" } as Task;
  const view = { owner: "pool:developers" } as WorkflowView;

  it("names the pool of an open, unpaused pool status", () => {
    expect(assignablePool(task, view)).toBe("developers");
  });

  it.each<[string, Partial<Task>, Partial<WorkflowView> | undefined]>([
    ["context access", { access: "context" }, {}],
    ["respond access", { access: "respond" }, {}],
    ["no workflow view yet", {}, undefined],
    ["a paused task", {}, { paused_reason: "customer" }],
    ["a task waiting on a pause", {}, { waiting_on: "pause" }],
    ["a status not owned by a pool", {}, { owner: "customer" }],
    ["a done task", { category: "done" }, {}],
    ["a cancelled task", { category: "cancelled" }, {}],
  ])("is empty for %s", (_, taskPatch, viewPatch) => {
    expect(
      assignablePool({ ...task, ...taskPatch }, viewPatch && { ...view, ...viewPatch }),
    ).toBe("");
  });
});
