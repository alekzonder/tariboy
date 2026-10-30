import type {
  AgentPool,
  CommentResult,
  PrincipalInfo,
  Queue,
  Relation,
  Task,
  TaskAgentPoolDeleted,
  TaskAgentPoolList,
  TaskCommentList,
  TaskDetail,
  TaskEventList,
  TaskPage,
  TaskQueueList,
  TaskRelationDeleted,
  TaskRelationList,
  TransferBundle,
} from "../api-types.generated";
import type { Transport } from "./client";

export type {
  AgentPool,
  CommentResult,
  PrincipalInfo,
  Queue,
  Relation,
  Task,
  TaskDetail,
  TaskPage,
  TransferBundle,
  WaitingFor,
  WorkflowAttention,
} from "../api-types.generated";
// Comment and Event would shadow the DOM globals of those names.
export type { Comment as TaskComment, Event as TaskEvent } from "../api-types.generated";

export type TaskCategory = Task["category"];
export type TaskStatus = TaskCategory;
export type TaskStatusView = "active" | "closed" | "all";
export type TaskPriority = "P0" | "P1" | "P2" | "P3";
// The daemon stores only P0-P3 (internal/tasks/model.go); the spec types it as a string.
export const taskPriority = (task: Pick<Task, "priority">) => task.priority as TaskPriority;
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
  revision: number;
}

export interface MoveTaskInput {
  parent_key?: string;
  before_key?: string;
  revision: number;
}

export function queryPath(path: string, values: object): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(values) as [
    string,
    string | number | boolean | undefined,
  ][]) {
    if (value !== undefined && value !== "") query.set(key, String(value));
  }
  const encoded = query.toString();
  return encoded ? `${path}?${encoded}` : path;
}

const taskPath = (key: string) => `/api/tasks/${encodeURIComponent(key)}`;
const queuePath = (queue: string) => `/api/task-queues/${encodeURIComponent(queue)}`;

export function tasksApi(t: Transport) {
  return {
    listTaskQueues: (signal?: AbortSignal) =>
      t.request<TaskQueueList>("GET", "/api/task-queues", undefined, signal),
    getTaskQueue: (prefix: string) => t.get<Queue>(queuePath(prefix)),
    createTaskQueue: (input: CreateQueueInput) => t.post<Queue>("/api/task-queues", input),
    updateTaskQueue: (prefix: string, input: UpdateQueueInput) =>
      t.request<Queue>("PATCH", queuePath(prefix), input),
    listAgentPools: (queue: string) => t.get<TaskAgentPoolList>(`${queuePath(queue)}/pools`),
    rebindAgentPool: (
      queue: string,
      pool: string,
      agents: string[],
      revision: number,
      idempotencyKey: string,
    ) =>
      t.request<AgentPool>("PATCH", `${queuePath(queue)}/pools/${encodeURIComponent(pool)}`, {
        agents,
        revision,
        idempotency_key: idempotencyKey,
      }),
    deleteAgentPool: (queue: string, pool: string, revision: number, idempotencyKey: string) =>
      t.delete<TaskAgentPoolDeleted>(
        queryPath(`${queuePath(queue)}/pools/${encodeURIComponent(pool)}`, {
          revision,
          idempotency_key: idempotencyKey,
        }),
      ),
    listTasks: (filters: TaskFilters = {}, signal?: AbortSignal) =>
      t.request<TaskPage>("GET", queryPath("/api/tasks", filters), undefined, signal),
    getTask: (key: string, signal?: AbortSignal) =>
      t.request<TaskDetail>("GET", taskPath(key), undefined, signal),
    createTask: (input: CreateTaskInput) => t.post<Task>("/api/tasks", input),
    updateTask: (key: string, input: UpdateTaskInput) =>
      t.request<Task>("PATCH", taskPath(key), input),
    claimTask: (key: string, revision: number) =>
      t.post<Task>(`${taskPath(key)}/claim`, { revision }),
    moveTask: (key: string, input: MoveTaskInput) => t.post<Task>(`${taskPath(key)}/move`, input),
    completeTask: (key: string, revision: number, completeAnyway = false) =>
      t.post<Task>(`${taskPath(key)}/complete`, { revision, complete_anyway: completeAnyway }),
    exportTask: (key: string) => t.get<TransferBundle>(`${taskPath(key)}/export`),
    importTask: (bundle: TransferBundle) => t.post<Task>("/api/tasks/import", { bundle }),
    /** Moves a task to another queue; the daemon returns the new task there. */
    requeueTask: (key: string, queue: string, revision: number) =>
      t.post<Task>(`${taskPath(key)}/requeue`, { queue, revision }),
    listTaskComments: (key: string) => t.get<TaskCommentList>(`${taskPath(key)}/comments`),
    addTaskComment: (key: string, body: string, idempotencyKey?: string) =>
      t.post<CommentResult>(`${taskPath(key)}/comments`, { body, idempotency_key: idempotencyKey }),
    listTaskRelations: (key: string) => t.get<TaskRelationList>(`${taskPath(key)}/relations`),
    addTaskRelation: (
      key: string,
      targetKey: string,
      type: TaskRelationType,
      revision: number,
      idempotencyKey?: string,
    ) =>
      t.post<Relation>(`${taskPath(key)}/relations`, {
        target_key: targetKey,
        type,
        revision,
        idempotency_key: idempotencyKey,
      }),
    deleteTaskRelation: (
      key: string,
      relationID: number,
      revision: number,
      idempotencyKey?: string,
    ) =>
      t.delete<TaskRelationDeleted>(
        queryPath(`${taskPath(key)}/relations`, {
          relation_id: relationID,
          revision,
          idempotency_key: idempotencyKey,
        }),
      ),
    listTaskEvents: (key: string, after = 0, limit = 200, signal?: AbortSignal) =>
      t.request<TaskEventList>(
        "GET",
        queryPath(`${taskPath(key)}/events`, { after, limit }),
        undefined,
        signal,
      ),
    listTaskPrincipals: (signal?: AbortSignal) =>
      t.request<PrincipalInfo>("GET", "/api/task-principals", undefined, signal),
  };
}
export type TasksApi = ReturnType<typeof tasksApi>;

// transferTask is the whole move, in the order that keeps a failure
// inspectable: read the tree on the source, write it on the destination, then
// close the copy that stays behind. Nothing is deleted, so a failure after the
// import leaves the task readable on both hosts. Each side is a client bound to
// its own host for the whole operation.
export async function transferTask(
  key: string,
  source: TasksApi,
  destination: TasksApi,
  destinationLabel: string,
  idempotencyKey?: string,
): Promise<void> {
  const bundle = await source.exportTask(key);
  await destination.importTask(bundle);
  await source.addTaskComment(key, `Moved to ${destinationLabel} as ${key}.`, idempotencyKey);
  const current = await source.getTask(key);
  await source.updateTask(key, { status: "cancelled", revision: current.task.revision });
}
