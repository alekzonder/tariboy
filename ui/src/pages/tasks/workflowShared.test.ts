import { describe, expect, it } from "vitest";
import type { TransitionRequest } from "@/lib/api";
import { followNoticeText, visibleNotice, type FollowResult, type Notice } from "./workflowShared";

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
