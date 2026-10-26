import type { ReactElement } from "react";
import type { Mock } from "vitest";
import { ApiProvider } from "@/components/ApiProvider";
import type { Task, TaskDetail, TaskNotification, Queue } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

// Fixtures shared by the TasksWorkspace test files. Each file keeps its own
// vi.hoisted mocks and vi.mock calls, because Vitest hoists them only within
// the test file itself; mockWorkspaceDefaults gives those mocks their answers.

/** The workspace below a server-selection boundary for `target`. */
export const onHost = (target: Daemon | null, ui: ReactElement) => (
  <ApiProvider target={target}>{ui}</ApiProvider>
);

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

export const queue: Queue = {
  prefix: "TEST",
  name: "Test work",
  description: "",
  owners: ["user:owner"],
  responsible_agent: "agent:triager",
  revision: 1,
  created_at: "2026-07-31T10:00:00Z",
  updated_at: "2026-07-31T10:00:00Z",
};
export const root: Task = {
  key: "TEST-1",
  queue: "TEST",
  parent_key: "",
  position: 0,
  priority: "P0",
  title: "Ship native tasks",
  description: "Central work system",
  status: "in_progress",
  category: "in_progress",
  pull_request: "",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "agent:worker",
  manual_block_reason: "",
  blocked: false,
  revision: 2,
  created_at: "2026-07-31T10:00:00Z",
  updated_at: "2026-07-31T10:00:00Z",
  completed_at: "",
  started_at: "",
};
export const child: Task = {
  ...root,
  key: "TEST-2",
  parent_key: "TEST-1",
  position: 0,
  title: "Desktop tree",
  status: "open",
  category: "open",
  assignee: "",
  revision: 1,
};
export const detail: TaskDetail = {
  task: root,
  comments: [
    {
      id: 1,
      task_key: root.key,
      author: "agent:worker",
      body: "Starting now",
      revision: 1,
      created_at: root.created_at,
      updated_at: root.updated_at,
    },
  ],
  waiting_for: [],
  relations: [
    {
      id: 9,
      source_key: "TEST-1",
      source_title: root.title,
      source_status: "open",
      target_key: "TEST-2",
      target_title: "Child task",
      target_status: "in_progress",
      type: "blocks",
      created_by: "user:owner",
      created_at: root.created_at,
    },
  ],
  descendants: 0,
  active_descendants: 0,
};
export const notification: TaskNotification = {
  id: "notification-1",
  type: "task.question",
  text: "Need a product decision",
  requesting_principal: "agent:worker",
  task_key: root.key,
  created_at: root.created_at,
  read_at: "",
};

/** Answers every mocked task api call the workspace makes on load. */
export function mockWorkspaceDefaults(api: Record<string, Mock>) {
  api.listTaskQueues.mockResolvedValue({ queues: [queue], count: 1 });
  api.listTaskPrincipals.mockResolvedValue({
    customer: "user:owner",
    agents: ["worker", "triager"],
    groups: ["platform"],
  });
  api.listTasks.mockResolvedValue({ tasks: [root, child], sequence: 10 });
  api.getTask.mockResolvedValue(detail);
  api.listAgentPools.mockResolvedValue({ items: [], count: 0 });
  api.getQueueWorkflow.mockResolvedValue(null);
  api.listTaskNotifications.mockResolvedValue({ notifications: [notification], count: 1 });
  api.listTaskEvents.mockResolvedValue({
    events: [
      {
        sequence: 3,
        event_id: "event-3",
        task_key: root.key,
        queue: "TEST",
        kind: "task.updated",
        actor: "user:owner",
        task_revision: 2,
        payload: { status: "in_progress" },
        created_at: root.updated_at,
      },
    ],
    count: 1,
  });
  api.createTask.mockResolvedValue(child);
  api.updateTask.mockResolvedValue({ ...root, assignee: "freelance-reviewer", revision: 3 });
  api.updateTaskQueue.mockResolvedValue({ ...queue, name: "Tests updated", revision: 2 });
  api.addTaskComment.mockResolvedValue({
    comment: detail.comments[0],
    created_waits: [],
    resolved_waits: [],
  });
  api.addTaskRelation.mockResolvedValue(detail.relations[0]);
  api.deleteTaskRelation.mockResolvedValue({ deleted: true, relation_id: 9 });
  api.moveTask.mockResolvedValue({ ...child, parent_key: "", revision: 2 });
  api.markTaskNotificationRead.mockResolvedValue({ ...notification, read_at: root.updated_at });
  api.createTaskQueue.mockResolvedValue({ ...queue, prefix: "OPS", name: "Operations" });
}
