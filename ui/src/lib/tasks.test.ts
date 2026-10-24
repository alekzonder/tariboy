import { describe, expect, it } from "vitest";
import type { Task } from "./api";
import {
  isActiveCategory,
  isWorkflowTask,
  taskCategoryLabel,
  taskStatusLabel,
  workflowAttentionTone,
  workflowStatusBadge,
} from "./tasks";

// Request tests for tasks and workflows live in lib/api/tasks.test.ts.
describe("workflow task helpers", () => {
  const task = (over: Partial<Task>): Task =>
    ({ key: "T-1", status: "open", category: "open", ...over }) as Task;

  it("tells workflow tasks from flexible ones", () => {
    expect(isWorkflowTask(task({}))).toBe(false);
    expect(isWorkflowTask(task({ workflow_name: "development" }))).toBe(true);
    // The daemon marks a workflow task by its digest; the name is a fallback.
    expect(isWorkflowTask(task({ workflow_digest: "abc" }))).toBe(true);
  });

  it("labels the status of either kind", () => {
    expect(taskStatusLabel(task({ status: "wait_customer", category: "wait_customer" }))).toBe(
      "Wait customer",
    );
    expect(
      taskStatusLabel(
        task({ status: "code-review_pending", category: "in_progress", workflow_name: "d" }),
      ),
    ).toBe("Code review pending");
    const long = "x".repeat(64);
    expect(taskStatusLabel(task({ status: long, workflow_name: "d" }))).toBe(
      "X".concat("x".repeat(63)),
    );
  });

  it("labels categories and tells active ones", () => {
    expect(taskCategoryLabel("in_progress")).toBe("In progress");
    expect(["open", "in_progress"].every((c) => isActiveCategory(c as never))).toBe(true);
    expect(["wait_customer", "done", "cancelled"].some((c) => isActiveCategory(c as never))).toBe(
      false,
    );
  });

  it("tones only the attention kinds a row marks", () => {
    expect(workflowAttentionTone({ kind: "failed" } as never)).toBe("danger");
    expect(workflowAttentionTone({ kind: "script_failed" } as never)).toBe("danger");
    expect(workflowAttentionTone({ kind: "rejected" } as never)).toBe("attention");
    expect(workflowAttentionTone({ kind: "idle" } as never)).toBeNull();
    expect(workflowAttentionTone(undefined)).toBeNull();
  });

  it("picks one status badge: paused, then a problem, then the wait", () => {
    const rejected = { kind: "rejected" } as never;
    expect(workflowStatusBadge(task({ waiting_on: "pause", workflow_attention: rejected }))).toBe(
      "paused",
    );
    expect(
      workflowStatusBadge(task({ waiting_on: "customer", workflow_attention: rejected })),
    ).toBe("attention");
    expect(
      workflowStatusBadge(
        task({ waiting_on: "customer", workflow_attention: { kind: "idle" } as never }),
      ),
    ).toBe("waiting");
    expect(workflowStatusBadge(task({}))).toBeNull();
  });
});
