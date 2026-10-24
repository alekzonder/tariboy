import { ApiError, type Transport } from "./client";
import { queryPath, type Task, type TaskCategory } from "./tasks";

export interface WorkflowOutcome {
  on: string
  to: string
  requires?: string[]
  missing?: string[]
  /** Script paths of the checks, in order. */
  checks?: string[]
  /** The same checks, in the same order, with description and resolved run mode and timeout. */
  check_details?: WorkflowCheckView[]
}
/** One check of a transition with its defaults resolved; `timeout` is a Go duration such as `1m0s`. */
export interface WorkflowCheckView { script: string; description?: string; run_as: "queue" | "agent"; timeout: string }
export interface WorkflowWatchView { script: string; description?: string; every: string; timeout: string }
export interface WorkflowTransitionView { on: string; to: string; requires?: string[]; checks?: WorkflowCheckView[] }
/** The effective limits of a status; `unavailable_grace` is a Go duration such as `5m0s`. */
export interface WorkflowLimitsView { idle_iterations: number; rejected_requests: number; script_failures: number; unavailable_grace: string }
export interface WorkflowArtifact { id: number; name: string; value: string; author: string; created_at: string }
export interface WorkflowVisit {
  id: number
  sequence: number
  status: string
  entered_at: string
  entered_by: string
  left_at?: string
  outcome?: string
  message?: string
  /** The workflow image the task was pinned to when it entered the status. */
  workflow_digest?: string
  workflow_version?: string
  /** The visit's counters, compared with the status limits; absent when zero. */
  idle_iterations?: number
  rejected_requests?: number
  script_failures?: number
}
export type TransitionState = "pending" | "applied" | "rejected" | "failed" | "cancelled"
export interface TransitionRequest {
  id: number
  task_key: string
  outcome: string
  message?: string
  actor: string
  state: TransitionState
  result_message?: string
  created_at: string
  finished_at?: string
  // Only while pending: how long the checks may take, in seconds.
  wait_seconds?: number
}
export type ScriptRunState = "pending" | "running" | "finished" | "interrupted" | "cancelled"
export type ScriptRunVerdict = "pass" | "reject" | "outcome" | "quiet" | "failure"
export interface WorkflowScriptRun {
  id: number
  task_key: string
  kind: "check" | "watch"
  script: string
  run_as: "queue" | "agent"
  state: ScriptRunState
  verdict?: ScriptRunVerdict
  exit_code?: number
  holder?: string
  message?: string
  created_at: string
  started_at?: string
  finished_at?: string
  log_path?: string
  /** The transition request a check run belongs to; absent for a watch run. */
  request_id?: number
}
export interface SourceRun {
  id: number
  queue: string
  source: string
  script: string
  state: "running" | "finished" | "interrupted" | "cancelled"
  verdict?: "items" | "quiet" | "failure"
  exit_code?: number
  message?: string
  tasks_created: number
  started_at: string
  finished_at?: string
  log_path?: string
}
/** A source of a queue's bound workflow, with its newest runs (at most 20). */
export interface QueueSource {
  queue: string
  name: string
  script: string
  every: string
  timeout?: string
  description?: string
  next_run_at?: string
  failures: number
  last_run?: SourceRun
  runs: SourceRun[]
}
export interface WorkflowView {
  name: string
  version: string
  digest: string
  /** The workflow the task's queue is bound to now; absent for an unbound queue. */
  queue_version?: string
  queue_digest?: string
  status: string
  category: TaskCategory
  waiting_on?: string
  paused_reason?: string
  owner: string
  holder?: string
  instructions_path?: string
  outcomes: WorkflowOutcome[]
  artifacts: WorkflowArtifact[]
  visits: WorkflowVisit[]
  last_request?: TransitionRequest
  runs?: WorkflowScriptRun[]
  /** Every artifact the manifest declares, in manifest order. */
  declared_artifacts: WorkflowDeclaredArtifact[]
  /** Every status the manifest declares, in manifest order. */
  statuses: WorkflowStatusInfo[]
}
export interface WorkflowDeclaredArtifact { name: string; description: string }
/**
 * One status of the pinned image. `owner` is `pool:NAME`, `customer`, `script`,
 * or "" for a terminal status. The graph fields are absent when empty, and a
 * terminal status has none of them.
 */
export interface WorkflowStatusInfo {
  id: string
  owner: string
  terminal: boolean
  initial?: boolean
  cancelled?: boolean
  instructions_path?: string
  watch?: WorkflowWatchView
  transitions?: WorkflowTransitionView[]
  limits?: WorkflowLimitsView
}
/** `tag` is the tag the binding follows: new tasks get the version it names. Empty pins the digest. */
export interface QueueWorkflow { queue: string; name: string; version: string; digest: string; tag: string; revision: number; updated_at: string }
export interface QueueSecretInfo { key: string; updated_at: string }
export interface WorkflowImage { name: string; tag: string; version: string; digest: string; built_at: string }
/** `kind` is "" for a terminal status. */
export interface WorkflowOwner { kind: "pool" | "customer" | "script" | ""; pool?: string }
export interface WorkflowLimits { idle_iterations?: number; rejected_requests?: number; script_failures?: number; unavailable_grace?: string }
export interface WorkflowCheck { script: string; run_as?: string; timeout?: string; description?: string }
export interface WorkflowWatch { script: string; every: string; timeout?: string; description?: string }
export interface WorkflowTransition { on: string; to: string; requires?: string[]; checks?: WorkflowCheck[] }
export interface WorkflowDefinitionStatus {
  id: string
  owner: WorkflowOwner
  instructions?: string
  watch?: WorkflowWatch
  transitions?: WorkflowTransition[]
  limits?: WorkflowLimits
  terminal?: boolean
  cancelled?: boolean
}
/** A parsed Workflowfile.yaml as the manifest stores it. */
export interface WorkflowDefinition {
  schema_version: number
  name: string
  workflow_version: string
  initial_status: string
  requires_secrets?: string[]
  env?: Record<string, string>
  limits?: WorkflowLimits
  artifacts?: WorkflowDeclaredArtifact[]
  statuses: WorkflowDefinitionStatus[]
}
export interface WorkflowFileEntry { path: string; sha256: string; executable: boolean; size: number }
export interface WorkflowManifest {
  schema_version: number
  name: string
  version: string
  digest: string
  built_at: string
  definition: WorkflowDefinition
  files: WorkflowFileEntry[]
}
export interface WorkflowValidationError { code: string; path: string; message: string }
export interface WorkflowValidation {
  valid: boolean
  name: string
  version: string
  pools: string[]
  files: string[]
  errors: WorkflowValidationError[]
}
export interface WorkflowBuildResult { name: string; version: string; digest: string; tags: string[]; created: boolean }

// Secret values are write-only: a client lists keys and sets or removes them,
// and never reads a value back.
/** The queue's secrets, and by name the bound workflow's required secrets the daemon's global env holds. */
export interface QueueSecretList { secrets: QueueSecretInfo[]; global_env: string[] }
// A plain env var is not secret: the daemon returns its value.
export interface QueueEnvVar { key: string; value: string; updated_at: string }

const taskPath = (key: string) => `/api/tasks/${encodeURIComponent(key)}`
const queuePath = (queue: string) => `/api/task-queues/${encodeURIComponent(queue)}`
const workflowImagePath = (name: string, tag: string) =>
  `/api/workflow-images/${encodeURIComponent(name)}/${encodeURIComponent(tag)}`

export function workflowsApi(t: Transport) {
  return {
    getTaskWorkflow: (key: string, signal?: AbortSignal) =>
      t.request<WorkflowView>("GET", `${taskPath(key)}/workflow`, undefined, signal),
    advanceTask: (key: string, outcome: string, message: string, from?: string) =>
      t.post<TransitionRequest>(`${taskPath(key)}/advance`, {
        outcome,
        message,
        ...(from ? { from } : {}),
      }),
    getTransitionRequest: (key: string, id: number) =>
      t.get<TransitionRequest>(`${taskPath(key)}/workflow/requests/${id}`),
    retryTransitionRequest: (key: string, id: number) =>
      t.post<TransitionRequest>(`${taskPath(key)}/workflow/requests/${id}/retry`, {}),
    setTaskArtifact: (key: string, name: string, value: string) =>
      t.put<WorkflowArtifact>(`${taskPath(key)}/artifacts/${encodeURIComponent(name)}`, { value }),
    getTaskArtifactHistory: async (key: string, name: string) =>
      (await t.get<{ artifact: WorkflowArtifact; history: WorkflowArtifact[] }>(
        `${taskPath(key)}/artifacts/${encodeURIComponent(name)}`,
      )).history,
    listTaskScriptRuns: async (key: string) =>
      (await t.get<{ runs: WorkflowScriptRun[]; count: number }>(`${taskPath(key)}/workflow/runs`)).runs,
    getTaskScriptRunLog: async (key: string, id: number, maxBytes?: number): Promise<{ text: string; truncated: boolean }> => {
      const { text, truncated } = await t.get<{ run_id: number; text: string; truncated: boolean }>(
        queryPath(`${taskPath(key)}/workflow/runs/${id}/log`, { max_bytes: maxBytes }),
      )
      return { text, truncated }
    },
    moveTaskWorkflow: (key: string, to: string, reason: string) =>
      t.post<Task>(`${taskPath(key)}/workflow/move`, { to, reason }),
    /** Restarts the task at the initial status of the workflow version its queue is bound to. */
    upgradeTaskWorkflow: (key: string, reason: string) =>
      t.post<Task>(`${taskPath(key)}/workflow/upgrade`, reason ? { reason } : {}),
    cancelWorkflowTask: (key: string) => t.post<Task>(`${taskPath(key)}/cancel`),
    resumeTaskWorkflow: (key: string, decision: "continue" | "release") =>
      t.post<Task>(`${taskPath(key)}/workflow/resume`, { decision }),
    /** Hands the pool status of a workflow task to another member of that pool. */
    reassignTaskWorkflow: (key: string, agent: string) =>
      t.post<Task>(`${taskPath(key)}/workflow/reassign`, { agent }),
    /** The workflow image a queue is bound to, or null when it is unbound. */
    getQueueWorkflow: async (queue: string): Promise<QueueWorkflow | null> => {
      try {
        return await t.get<QueueWorkflow>(`${queuePath(queue)}/workflow`)
      } catch (err) {
        if (err instanceof ApiError && err.code === "queue_workflow_not_found") return null
        throw err
      }
    },
    setQueueWorkflow: (queue: string, ref: string, revision: number) =>
      t.put<QueueWorkflow>(`${queuePath(queue)}/workflow`, { ref, revision }),
    clearQueueWorkflow: async (queue: string, revision: number): Promise<void> => {
      await t.delete(queryPath(`${queuePath(queue)}/workflow`, { revision }))
    },
    listQueueSecrets: async (queue: string): Promise<QueueSecretList> => {
      const { secrets, global_env } = await t.get<QueueSecretList & { count: number }>(`${queuePath(queue)}/secrets`)
      return { secrets, global_env: global_env ?? [] }
    },
    setQueueSecret: (queue: string, key: string, value: string) =>
      t.put<{ queue: string; key: string; updated_at: string }>(
        `${queuePath(queue)}/secrets/${encodeURIComponent(key)}`,
        { value },
      ),
    removeQueueSecret: async (queue: string, key: string): Promise<void> => {
      await t.delete(`${queuePath(queue)}/secrets/${encodeURIComponent(key)}`)
    },
    listQueueEnv: async (queue: string): Promise<QueueEnvVar[]> =>
      (await t.get<{ env: QueueEnvVar[]; count: number }>(`${queuePath(queue)}/env`)).env,
    setQueueEnv: (queue: string, key: string, value: string) =>
      t.put<QueueEnvVar & { queue: string }>(`${queuePath(queue)}/env/${encodeURIComponent(key)}`, { value }),
    removeQueueEnv: async (queue: string, key: string): Promise<void> => {
      await t.delete(`${queuePath(queue)}/env/${encodeURIComponent(key)}`)
    },
    listQueueSources: async (queue: string) =>
      (await t.get<{ sources: QueueSource[]; count: number }>(`${queuePath(queue)}/sources`)).sources,
    getQueueSourceRunLog: async (queue: string, id: number, maxBytes?: number): Promise<{ text: string; truncated: boolean }> => {
      const { text, truncated } = await t.get<{ run_id: number; text: string; truncated: boolean }>(
        queryPath(`${queuePath(queue)}/source-runs/${id}/log`, { max_bytes: maxBytes }),
      )
      return { text, truncated }
    },
    listWorkflowImages: async () =>
      (await t.get<{ workflows: WorkflowImage[]; count: number }>("/api/workflow-images")).workflows,
    /** The manifest a tag or a full digest names. */
    getWorkflowImage: (name: string, tag: string) => t.get<WorkflowManifest>(workflowImagePath(name, tag)),
    removeWorkflowImage: (name: string, tag: string) =>
      t.delete<{ name: string; tag: string; removed: boolean; content_removed: boolean }>(workflowImagePath(name, tag)),
    /** `path` is a Workflowfile.yaml, or its directory, on the host of this client. */
    validateWorkflowDirectory: (path: string) => t.post<WorkflowValidation>("/api/workflow-images/validate", { path }),
    buildWorkflowDirectory: (path: string) => t.post<WorkflowBuildResult>("/api/workflow-images/build", { path }),
    downloadWorkflowArchive: async (name: string, tag: string) =>
      (await t.raw("GET", `${workflowImagePath(name, tag)}/export`)).blob(),
    importWorkflowArchive: async (archive: Blob) => {
      const response = await t.raw("POST", "/api/workflow-image-imports", archive)
      return ((await response.json()) as { result: WorkflowBuildResult }).result
    },
  }
}
export type WorkflowsApi = ReturnType<typeof workflowsApi>
