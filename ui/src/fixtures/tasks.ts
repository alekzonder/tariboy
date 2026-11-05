import type { Task } from "@/types";

import { hoursAgo } from "./time";

/** A flexible task list item. */
export const taskItem = (over: Partial<Task> = {}): Task => ({
  key: "TASK-1",
  queue: "TASK",
  parent_key: "",
  position: 0,
  priority: "P2",
  title: "Add a README",
  description: "",
  status: "open",
  category: "open",
  pull_request: "",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "agent:bob",
  manual_block_reason: "",
  blocked: false,
  revision: 1,
  created_at: hoursAgo(26),
  updated_at: hoursAgo(26),
  completed_at: "",
  started_at: "",
  ...over,
});

/** Edge sets of tasks: `empty`, `one`, `many` (50) and `longText`. */
export const taskItems = {
  empty: [] as Task[],
  one: [taskItem({ key: "TB-142", title: "Add a README" })],
  many: Array.from({ length: 50 }, (_, i) =>
    taskItem({ key: `TB-${142 + i}`, title: `Task number ${i + 1}`, position: i }),
  ),
  longText: [
    taskItem({
      key: "TB-999",
      title:
        "Replace the retry loop of the mail delivery worker with a bounded backoff and add a metric for each give-up",
      description: "A long description that wraps over several lines. ".repeat(8),
    }),
  ],
};
