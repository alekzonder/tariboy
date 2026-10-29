import { ApiError, type Transport } from "./client";
import type {
  Artifact,
  QueueEnvVar,
  QueueWorkflow,
  ScriptRun,
  Task,
  TaskArtifactHistory,
  TaskQueueEnvList,
  TaskQueueEnvSet,
  TaskQueueSecretList,
  TaskQueueSecretSet,
  TaskQueueSourceList,
  TaskRunLog,
  TaskScriptRunList,
  TransitionRequest,
  WorkflowImageBuildResult,
  WorkflowImageList,
  WorkflowImageManifest,
  WorkflowImageRemoveResult,
  WorkflowImageValidation,
  WorkflowView,
} from "../api-types.generated";
import { UI_INITIATOR, type OperationStarted } from "./operations";
import { queryPath } from "./tasks";

export type {
  Artifact,
  DeclaredArtifact,
  OutcomeView,
  QueueEnvVar,
  QueueSecretInfo,
  QueueSource,
  QueueWorkflow,
  ScriptRun,
  SourceRun,
  StatusView,
  StatusVisit,
  TransitionRequest,
  WorkflowImageBuildResult,
  WorkflowImageDefinition,
  WorkflowImageLimits,
  WorkflowImageListing,
  WorkflowImageManifest,
  WorkflowImageStatus,
  WorkflowImageValidation,
  WorkflowImageValidationError,
  WorkflowView,
} from "../api-types.generated";

const taskPath = (key: string) => `/api/tasks/${encodeURIComponent(key)}`;
const queuePath = (queue: string) => `/api/task-queues/${encodeURIComponent(queue)}`;
const workflowImagePath = (name: string, tag: string) =>
  `/api/workflow-images/${encodeURIComponent(name)}/${encodeURIComponent(tag)}`;

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
      t.put<Artifact>(`${taskPath(key)}/artifacts/${encodeURIComponent(name)}`, { value }),
    getTaskArtifactHistory: async (key: string, name: string) =>
      (await t.get<TaskArtifactHistory>(`${taskPath(key)}/artifacts/${encodeURIComponent(name)}`))
        .history,
    listTaskScriptRuns: async (key: string) =>
      (await t.get<TaskScriptRunList>(`${taskPath(key)}/workflow/runs`)).runs,
    getTaskScriptRunLog: async (
      key: string,
      id: number,
      maxBytes?: number,
    ): Promise<{ text: string; truncated: boolean }> => {
      const { text, truncated } = await t.get<TaskRunLog>(
        queryPath(`${taskPath(key)}/workflow/runs/${id}/log`, { max_bytes: maxBytes }),
      );
      return { text, truncated };
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
    /** Starts the watch script of the task's script status now, instead of at its next scheduled time. */
    runTaskWatch: (key: string) => t.post<ScriptRun>(`${taskPath(key)}/workflow/watch`),
    /** The workflow image a queue is bound to, or null when it is unbound. */
    getQueueWorkflow: async (queue: string): Promise<QueueWorkflow | null> => {
      try {
        return await t.get<QueueWorkflow>(`${queuePath(queue)}/workflow`);
      } catch (err) {
        if (err instanceof ApiError && err.code === "queue_workflow_not_found") return null;
        throw err;
      }
    },
    setQueueWorkflow: (queue: string, ref: string, revision: number) =>
      t.put<QueueWorkflow>(`${queuePath(queue)}/workflow`, { ref, revision }),
    clearQueueWorkflow: async (queue: string, revision: number): Promise<void> => {
      await t.delete(queryPath(`${queuePath(queue)}/workflow`, { revision }));
    },
    listQueueSecrets: async (queue: string) => {
      const { secrets, global_env } = await t.get<TaskQueueSecretList>(
        `${queuePath(queue)}/secrets`,
      );
      return { secrets, global_env: global_env ?? [] };
    },
    setQueueSecret: (queue: string, key: string, value: string) =>
      t.put<TaskQueueSecretSet>(`${queuePath(queue)}/secrets/${encodeURIComponent(key)}`, {
        value,
      }),
    removeQueueSecret: async (queue: string, key: string): Promise<void> => {
      await t.delete(`${queuePath(queue)}/secrets/${encodeURIComponent(key)}`);
    },
    listQueueEnv: async (queue: string): Promise<QueueEnvVar[]> =>
      (await t.get<TaskQueueEnvList>(`${queuePath(queue)}/env`)).env,
    setQueueEnv: (queue: string, key: string, value: string) =>
      t.put<TaskQueueEnvSet>(`${queuePath(queue)}/env/${encodeURIComponent(key)}`, {
        value,
      }),
    removeQueueEnv: async (queue: string, key: string): Promise<void> => {
      await t.delete(`${queuePath(queue)}/env/${encodeURIComponent(key)}`);
    },
    listQueueSources: async (queue: string) =>
      (await t.get<TaskQueueSourceList>(`${queuePath(queue)}/sources`)).sources,
    getQueueSourceRunLog: async (
      queue: string,
      id: number,
      maxBytes?: number,
    ): Promise<{ text: string; truncated: boolean }> => {
      const { text, truncated } = await t.get<TaskRunLog>(
        queryPath(`${queuePath(queue)}/source-runs/${id}/log`, { max_bytes: maxBytes }),
      );
      return { text, truncated };
    },
    listWorkflowImages: async () =>
      (await t.get<WorkflowImageList>("/api/workflow-images")).workflows,
    /** The manifest a tag or a full digest names. */
    getWorkflowImage: (name: string, tag: string) =>
      t.get<WorkflowImageManifest>(workflowImagePath(name, tag)),
    removeWorkflowImage: (name: string, tag: string) =>
      t.delete<WorkflowImageRemoveResult>(workflowImagePath(name, tag)),
    /** `path` is a Workflowfile.yaml, or its directory, on the host of this client. */
    validateWorkflowDirectory: (path: string) =>
      t.post<WorkflowImageValidation>("/api/workflow-images/validate", { path }),
    /** Starts a background build; the result arrives on the returned operation. */
    buildWorkflowDirectory: (path: string) =>
      t.post<OperationStarted>("/api/workflow-images/build", { path, initiator: UI_INITIATOR }),
    downloadWorkflowArchive: async (name: string, tag: string) =>
      (await t.raw("GET", `${workflowImagePath(name, tag)}/export`)).blob(),
    importWorkflowArchive: async (archive: Blob) => {
      const response = await t.raw("POST", "/api/workflow-image-imports", archive);
      // Not in /api/openapi.json: served by internal/api/server.go POST /api/workflow-image-imports.
      return ((await response.json()) as { result: WorkflowImageBuildResult }).result;
    },
  };
}
