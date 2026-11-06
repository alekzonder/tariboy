import type { Task } from "@/lib/api-types.generated";

export type {
  AgentPool,
  CommentResult,
  PrincipalInfo,
  Queue,
  Relation,
  Task,
  TaskBudget,
  TaskDetail,
  TaskPage,
  TaskParent,
  TransferBundle,
  WaitingFor,
  WorkflowAttention,
} from "@/lib/api-types.generated";
// Comment and Event would shadow the DOM globals of those names.
export type { Comment as TaskComment, Event as TaskEvent } from "@/lib/api-types.generated";

export type TaskCategory = Task["category"];
export type TaskStatus = TaskCategory;
export type TaskStatusView = "active" | "closed" | "all";
// The daemon stores only P0-P3 (internal/tasks/model.go); the spec types it as a string.
export type TaskPriority = "P0" | "P1" | "P2" | "P3";
export type TaskRelationType = "blocks" | "related";

// Not in /api/openapi.json: a frame of internal/api/server.go /api/tasks/ws.
export interface TaskEventHint {
  type: "event";
  sequence: number;
  kind: string;
  task_key?: string;
  queue?: string;
  task_revision?: number;
}

export interface TaskFilters {
  queue?: string;
  status?: TaskStatus;
  status_view?: TaskStatusView;
  assignee?: string;
  author?: string;
  group?: string;
  text?: string;
  waiting_for?: string;
  scope_agent?: string;
  blocked?: boolean;
  limit?: number;
  after?: string;
}

export interface CreateQueueInput {
  prefix: string;
  name: string;
  description?: string;
  owners?: string[];
  responsible_agent?: string;
}

export interface UpdateQueueInput {
  name?: string;
  description?: string;
  owners?: string[];
  responsible_agent?: string;
  revision: number;
}

export interface CreateTaskInput {
  queue: string;
  parent_key?: string;
  title: string;
  description?: string;
  pull_request?: string;
  assignee?: string;
  group?: string;
  priority?: TaskPriority;
  idempotency_key?: string;
}

export interface UpdateTaskInput {
  title?: string;
  description?: string;
  status?: TaskStatus;
  pull_request?: string;
  assignee?: string;
  manual_block_reason?: string;
  priority?: TaskPriority;
  budget_usd?: number;
  revision: number;
}

export interface MoveTaskInput {
  parent_key?: string;
  before_key?: string;
  revision: number;
}
