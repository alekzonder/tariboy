import { describe, expect, it } from "vitest";
import type { TaskEvent, TaskWait } from "./api";
import { taskCommentWaits, taskEventSequence } from "./taskDetail";

function wait(id: number, requestingPrincipal: string): TaskWait {
  return {
    id,
    task_key: "TEST-1",
    expected_principal: "user:owner",
    requesting_principal: requestingPrincipal,
    requesting_comment_id: id,
    requested_at: "2026-07-31T10:00:00Z",
  };
}

function event(sequence: number): TaskEvent {
  return {
    sequence,
    event_id: `event-${sequence}`,
    queue: "TEST",
    kind: "task.updated",
    actor: "user:owner",
    task_revision: 1,
    payload: {},
    created_at: "2026-07-31T10:00:00Z",
  };
}

describe("task detail model", () => {
  it("leaves the workflow's own waits out of a workflow task's comment waits", () => {
    const waits = [wait(1, "system:workflow"), wait(2, "agent:dev")];
    expect(taskCommentWaits(waits, true)).toEqual([waits[1]]);
  });

  it("keeps every wait of a task without a workflow", () => {
    const waits = [wait(1, "system:workflow"), wait(2, "agent:dev")];
    expect(taskCommentWaits(waits, false)).toEqual(waits);
  });

  it("takes the newest of the event sequences and the live sequence", () => {
    expect(taskEventSequence([event(3), event(7), event(5)], 4)).toBe(7);
    expect(taskEventSequence([event(3)], 9)).toBe(9);
  });

  it("starts at zero without events or a live sequence", () => {
    expect(taskEventSequence([], undefined)).toBe(0);
  });
});
