import { describe, expect, it } from "vitest"
import type { Task } from "./api"
import { isActiveCategory, isWorkflowTask, taskCategoryLabel, taskStatusLabel } from "./tasks"

// Request tests for tasks and workflows live in lib/api/tasks.test.ts.
describe("workflow task helpers", () => {
  const task = (over: Partial<Task>): Task => ({ key: "T-1", status: "open", category: "open", ...over }) as Task

  it("tells workflow tasks from flexible ones", () => {
    expect(isWorkflowTask(task({}))).toBe(false)
    expect(isWorkflowTask(task({ workflow_name: "development" }))).toBe(true)
    // The daemon marks a workflow task by its digest; the name is a fallback.
    expect(isWorkflowTask(task({ workflow_digest: "abc" }))).toBe(true)
  })

  it("labels the status of either kind", () => {
    expect(taskStatusLabel(task({ status: "wait_customer", category: "wait_customer" }))).toBe("Wait customer")
    expect(taskStatusLabel(task({ status: "code-review_pending", category: "in_progress", workflow_name: "d" }))).toBe("Code review pending")
    const long = "x".repeat(64)
    expect(taskStatusLabel(task({ status: long, workflow_name: "d" }))).toBe("X".concat("x".repeat(63)))
  })

  it("labels categories and tells active ones", () => {
    expect(taskCategoryLabel("in_progress")).toBe("In progress")
    expect(["open", "in_progress"].every((c) => isActiveCategory(c as never))).toBe(true)
    expect(["wait_customer", "done", "cancelled"].some((c) => isActiveCategory(c as never))).toBe(false)
  })
})
