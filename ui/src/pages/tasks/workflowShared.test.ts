import { describe, expect, it } from "vitest";
import { followNoticeText, type FollowResult } from "./workflowShared";

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
