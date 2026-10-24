import type { Transport } from "./client";

export type TaskStatus = "open" | "in_progress" | "wait_customer" | "done" | "cancelled"
export type TaskCategory = TaskStatus
export type WaitingOn = "customer" | "script" | "pause"
export type TaskStatusView = "active" | "closed" | "all"
export type TaskAccess = "write" | "respond" | "context"
export type TaskPriority = "P0" | "P1" | "P2" | "P3"

export interface TaskQueue {
  prefix: string
  name: string
  description: string
  owners: string[]
  responsible_agent: string
  revision: number
  created_at: string
  updated_at: string
}

/**
 * What needs a person on an open workflow task, the most urgent kind first:
 * `paused` (the daemon waits for a decision), `failed` (a check could not run),
 * `rejected` (a check rejected the latest request), `script_failed` (the
 * status's watch failed), `idle` (the holder ends iterations without a
 * transition request).
 */
export interface WorkflowAttention {
  kind: "paused" | "failed" | "rejected" | "script_failed" | "idle"
  /** The first line of the script's message, or what the pause means. */
  message?: string
  /** The visit counter behind the kind: rejected requests, script failures, or idle iterations. */
  count?: number
  /** The failed or rejected transition request. */
  request_id?: number
  /** The script run whose log explains it. */
  run_id?: number
}
/** The position of the current status in the manifest, counted from 1. */
export interface WorkflowStep { index: number; total: number }

export interface Task {
  key: string
  queue: string
  parent_key: string
  position: number
  priority: TaskPriority
  title: string
  description: string
  // The flexible status, or the workflow status ID for a workflow task.
  status: string
  category: TaskCategory
  waiting_on?: WaitingOn
  workflow_name?: string
  workflow_version?: string
  workflow_digest?: string
  workflow_paused_reason?: string
  /** What needs a person on an open workflow task; absent when nothing does, and on a flexible or closed task. */
  workflow_attention?: WorkflowAttention
  /** The position of the current status in the manifest, from 1; absent on a flexible or closed task. */
  workflow_step?: WorkflowStep
  pull_request?: string
  author: string
  customer: string
  group: string
  assignee: string
  manual_block_reason: string
  blocked: boolean
  revision: number
  created_at: string
  started_at?: string
  updated_at: string
  completed_at: string
  access?: TaskAccess
}

export interface AgentPool {
  id: number
  queue: string
  name: string
  agents: string[]
  revision: number
  created_at: string
  updated_at: string
}

export interface TaskComment {
  id: number
  task_key: string
  author: string
  body: string
  revision: number
  created_at: string
  updated_at: string
}

export interface TaskWait {
  id: number
  task_key: string
  expected_principal: string
  requesting_principal: string
  requesting_comment_id: number
  requested_at: string
  resolving_comment_id?: number
  resolved_at?: string
}

export type TaskRelationType = "blocks" | "related"
export interface TaskRelation {
  id: number
  source_key: string
  source_title: string
  source_status: TaskStatus
  target_key: string
  target_title: string
  target_status: TaskStatus
  type: TaskRelationType
  created_by: string
  created_at: string
}

export interface TaskDetail {
  task: Task
  comments: TaskComment[]
  waiting_for: TaskWait[]
  relations: TaskRelation[]
}

export interface TaskEvent {
  sequence: number
  event_id: string
  task_key?: string
  queue: string
  kind: string
  actor: string
  task_revision: number
  payload: Record<string, unknown>
  created_at: string
}

export interface TaskEventHint {
  type: "event"
  sequence: number
  kind: string
  task_key?: string
  queue?: string
  task_revision?: number
}

export interface TaskNotification {
  id: string
  channel: string
  type: string
  text: string
  requesting_principal: string
  task_key: string
  event_sequence: number
  created_at: string
  published_at: string
  read_at: string
  dismissed_at: string
}

export interface TaskPrincipals {
  customer: string
  agents: string[]
  groups: string[]
}

export interface TaskFilters {
  queue?: string
  status?: TaskStatus
  status_view?: TaskStatusView
  assignee?: string
  author?: string
  group?: string
  text?: string
  waiting_for?: string
  scope_agent?: string
  blocked?: boolean
  limit?: number
  after?: string
}

export interface TaskPage {
  tasks: Task[]
  next_cursor?: string
  sequence: number
}

export interface CreateQueueInput {
  prefix: string
  name: string
  description?: string
  owners?: string[]
  responsible_agent?: string
}

export interface UpdateQueueInput {
  name?: string
  description?: string
  owners?: string[]
  responsible_agent?: string
  revision: number
}

export interface CreateTaskInput {
  queue: string
  parent_key?: string
  title: string
  description?: string
  pull_request?: string
  assignee?: string
  group?: string
  priority?: TaskPriority
  idempotency_key?: string
}

export interface UpdateTaskInput {
  title?: string
  description?: string
  status?: TaskStatus
  pull_request?: string
  assignee?: string
  manual_block_reason?: string
  priority?: TaskPriority
  revision: number
}

export interface MoveTaskInput {
  parent_key?: string
  before_key?: string
  revision: number
}

export interface CommentResult {
  comment: TaskComment
  created_waits: TaskWait[]
  resolved_waits: TaskWait[]
}

// A transfer bundle travels between daemons unchanged: the source exports it,
// the target imports it under the same keys. Only the desktop app is
// authenticated against both hosts, so it drives the two calls.
export interface TransferBundle {
  root_key: string
  queue: string
  tasks: unknown[]
  relations: unknown[]
}

export function queryPath(path: string, values: object): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(values) as [string, string | number | boolean | undefined][]) {
    if (value !== undefined && value !== "") query.set(key, String(value))
  }
  const encoded = query.toString()
  return encoded ? `${path}?${encoded}` : path
}

const taskPath = (key: string) => `/api/tasks/${encodeURIComponent(key)}`
const queuePath = (queue: string) => `/api/task-queues/${encodeURIComponent(queue)}`

export function tasksApi(t: Transport) {
  return {
    listTaskQueues: (signal?: AbortSignal) =>
      t.request<{ queues: TaskQueue[]; count: number }>("GET", "/api/task-queues", undefined, signal),
    getTaskQueue: (prefix: string) => t.get<TaskQueue>(queuePath(prefix)),
    createTaskQueue: (input: CreateQueueInput) => t.post<TaskQueue>("/api/task-queues", input),
    updateTaskQueue: (prefix: string, input: UpdateQueueInput) => t.request<TaskQueue>("PATCH", queuePath(prefix), input),
    listAgentPools: (queue: string) => t.get<{ items: AgentPool[]; count: number }>(`${queuePath(queue)}/pools`),
    rebindAgentPool: (queue: string, pool: string, agents: string[], revision: number, idempotencyKey: string) =>
      t.request<AgentPool>("PATCH", `${queuePath(queue)}/pools/${encodeURIComponent(pool)}`, {
        agents, revision, idempotency_key: idempotencyKey,
      }),
    listTasks: (filters: TaskFilters = {}, signal?: AbortSignal) =>
      t.request<TaskPage>("GET", queryPath("/api/tasks", filters), undefined, signal),
    getTask: (key: string, signal?: AbortSignal) => t.request<TaskDetail>("GET", taskPath(key), undefined, signal),
    createTask: (input: CreateTaskInput) => t.post<Task>("/api/tasks", input),
    updateTask: (key: string, input: UpdateTaskInput) => t.request<Task>("PATCH", taskPath(key), input),
    claimTask: (key: string, revision: number) => t.post<Task>(`${taskPath(key)}/claim`, { revision }),
    moveTask: (key: string, input: MoveTaskInput) => t.post<Task>(`${taskPath(key)}/move`, input),
    completeTask: (key: string, revision: number, completeAnyway = false) =>
      t.post<Task>(`${taskPath(key)}/complete`, { revision, complete_anyway: completeAnyway }),
    exportTask: (key: string) => t.get<TransferBundle>(`${taskPath(key)}/export`),
    importTask: (bundle: TransferBundle) => t.post<Task>("/api/tasks/import", { bundle }),
    /** Moves a task to another queue; the daemon returns the new task there. */
    requeueTask: (key: string, queue: string, revision: number) =>
      t.post<Task>(`${taskPath(key)}/requeue`, { queue, revision }),
    listTaskComments: (key: string) =>
      t.get<{ comments: TaskComment[]; count: number }>(`${taskPath(key)}/comments`),
    addTaskComment: (key: string, body: string, idempotencyKey?: string) =>
      t.post<CommentResult>(`${taskPath(key)}/comments`, { body, idempotency_key: idempotencyKey }),
    listTaskRelations: (key: string) =>
      t.get<{ relations: TaskRelation[]; count: number }>(`${taskPath(key)}/relations`),
    addTaskRelation: (key: string, targetKey: string, type: TaskRelationType, revision: number, idempotencyKey?: string) =>
      t.post<TaskRelation>(`${taskPath(key)}/relations`, {
        target_key: targetKey,
        type,
        revision,
        idempotency_key: idempotencyKey,
      }),
    deleteTaskRelation: (key: string, relationID: number, revision: number, idempotencyKey?: string) =>
      t.delete<{ deleted: boolean; relation_id: number }>(queryPath(`${taskPath(key)}/relations`, {
        relation_id: relationID,
        revision,
        idempotency_key: idempotencyKey,
      })),
    listTaskEvents: (key: string, after = 0, limit = 200, signal?: AbortSignal) =>
      t.request<{ events: TaskEvent[]; count: number }>(
        "GET",
        queryPath(`${taskPath(key)}/events`, { after, limit }),
        undefined, signal,
      ),
    listTaskPrincipals: (signal?: AbortSignal) =>
      t.request<TaskPrincipals>("GET", "/api/task-principals", undefined, signal),
    listTaskNotifications: (includeDismissed = false, signal?: AbortSignal) =>
      t.request<{ notifications: TaskNotification[]; count: number }>(
        "GET",
        queryPath("/api/task-notifications", { include_dismissed: includeDismissed || undefined }),
        undefined, signal,
      ),
    markTaskNotificationRead: (id: string) =>
      t.post<TaskNotification>(`/api/task-notifications/${encodeURIComponent(id)}/read`),
  }
}
export type TasksApi = ReturnType<typeof tasksApi>

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
  const bundle = await source.exportTask(key)
  await destination.importTask(bundle)
  await source.addTaskComment(key, `Moved to ${destinationLabel} as ${key}.`, idempotencyKey)
  const current = await source.getTask(key)
  await source.updateTask(key, { status: "cancelled", revision: current.task.revision })
}
