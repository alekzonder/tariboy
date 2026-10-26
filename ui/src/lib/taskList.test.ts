import { describe, expect, it } from "vitest";
import type { Task, TaskNotification } from "./api";
import {
  missingQuestionTaskKeys,
  tasksWithQuestionsAndOpen,
  taskListFilters,
  unreadQuestionTaskKeys,
  unreadQuestionsFor,
} from "./taskList";

const question = (over: Partial<TaskNotification>): TaskNotification =>
  ({
    id: "n-1",
    type: "task.question",
    task_key: "T-1",
    requesting_principal: "agent:dev",
    read_at: "",
    ...over,
  }) as TaskNotification;

const task = (key: string): Task => ({ key }) as Task;

describe("task list filters", () => {
  const base = { query: "bug", statusView: "active" as const };

  it("sends the text, scope and status view for the whole list", () => {
    expect(taskListFilters({ ...base, view: "", scopeAgent: "dev" })).toEqual({
      text: "bug",
      scope_agent: "dev",
      status_view: "active",
    });
  });

  it("filters mine and waiting by the scoped agent or the customer", () => {
    expect(taskListFilters({ ...base, view: "mine", scopeAgent: "dev" })).toMatchObject({
      assignee: "agent:dev",
    });
    expect(taskListFilters({ ...base, view: "mine", scopeAgent: "agent:dev" })).toMatchObject({
      assignee: "agent:dev",
    });
    expect(
      taskListFilters({ ...base, view: "waiting", scopeAgent: undefined, principal: "user:me" }),
    ).toMatchObject({ waiting_for: "user:me" });
  });
});

describe("task questions", () => {
  const notifications = [
    question({ id: "1", task_key: "T-1" }),
    question({ id: "2", task_key: "T-2", read_at: "2026-01-01T00:00:00Z" }),
    question({ id: "3", task_key: "T-3", read_at: "2026-01-02T00:00:00Z" }),
    question({ id: "4", task_key: "T-4", type: "task.comment" }),
    question({ id: "5", task_key: "T-5", requesting_principal: "agent:other" }),
    question({ id: "6", task_key: "T-1" }),
  ];

  it("keeps only unread questions", () => {
    expect([...unreadQuestionTaskKeys(notifications)]).toEqual(["T-1", "T-5"]);
    expect(unreadQuestionsFor(notifications, "T-1").map((item) => item.id)).toEqual(["1", "6"]);
  });

  it("finds question tasks missing from the list, sorted and scoped to the requester", () => {
    expect(missingQuestionTaskKeys({ notifications, tasks: [], requester: undefined })).toEqual([
      "T-1",
      "T-5",
    ]);
    expect(
      missingQuestionTaskKeys({ notifications, tasks: [task("T-1")], requester: undefined }),
    ).toEqual(["T-5"]);
    expect(missingQuestionTaskKeys({ notifications, tasks: [], requester: "agent:dev" })).toEqual([
      "T-1",
    ]);
  });

  it("appends question tasks and the open task once each", () => {
    const keys = (list: Task[]) => list.map((item) => item.key);
    expect(
      keys(
        tasksWithQuestionsAndOpen({
          tasks: [task("T-1")],
          questionTasks: [task("T-1"), task("T-2")],
          openTask: task("T-3"),
        }),
      ),
    ).toEqual(["T-1", "T-2", "T-3"]);
    expect(
      keys(
        tasksWithQuestionsAndOpen({
          tasks: [task("T-1")],
          questionTasks: [],
          openTask: task("T-1"),
        }),
      ),
    ).toEqual(["T-1"]);
    expect(
      keys(
        tasksWithQuestionsAndOpen({ tasks: [task("T-1")], questionTasks: [], openTask: undefined }),
      ),
    ).toEqual(["T-1"]);
  });
});
