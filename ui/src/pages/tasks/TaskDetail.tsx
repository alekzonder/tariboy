import { X } from "lucide-react"
import { useState } from "react"
import type {
  TaskDetail as Detail,
  TaskEvent,
  TaskPrincipals,
  TaskPriority,
  TaskRelationType,
  TaskStatus,
  WorkflowArtifact,
  WorkflowExecutionView,
  WorkflowQuestion,
} from "@/lib/tasks"
import TaskComments from "./TaskComments"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

export default function TaskDetail({
  detail,
  events,
  workflow,
  workflowArtifacts,
  workflowQuestions,
  executionLoading,
  executionError,
  artifactsLoading,
  artifactsError,
  questionsLoading,
  questionsError,
  principals,
  onClose,
  onSave,
  onComment,
  onAddRelation,
  onDeleteRelation,
}: {
  detail: Detail
  events: TaskEvent[]
  workflow: WorkflowExecutionView | null
  workflowArtifacts: WorkflowArtifact[]
  workflowQuestions: WorkflowQuestion[]
  executionLoading: boolean
  executionError: string
  artifactsLoading: boolean
  artifactsError: string
  questionsLoading: boolean
  questionsError: string
  principals: TaskPrincipals | null
  onClose: () => void
  onSave: (input: {
    title: string
    description: string
    pull_request: string
    status?: TaskStatus
    assignee?: string
    manual_block_reason?: string
    priority: TaskPriority
  }) => Promise<void>
  onComment: (body: string, idempotencyKey: string) => Promise<void>
  onAddRelation: (targetKey: string, type: TaskRelationType) => Promise<void>
  onDeleteRelation: (relationID: number) => Promise<void>
}) {
  const task = detail.task
  const managed = Boolean(task.workflow_version_id)
  const [title, setTitle] = useState(task.title)
  const [description, setDescription] = useState(task.description)
  const [status, setStatus] = useState<TaskStatus>(task.status)
  const [pullRequest, setPullRequest] = useState(task.pull_request ?? "")
  const [priority, setPriority] = useState<TaskPriority>(task.priority)
  const [assignee, setAssignee] = useState(task.assignee)
  const [blockReason, setBlockReason] = useState(task.manual_block_reason)
  const [saving, setSaving] = useState(false)
  const [relationType, setRelationType] = useState<TaskRelationType>("blocks")
  const [relationTarget, setRelationTarget] = useState("")
  const [relationBusy, setRelationBusy] = useState(false)
  const [relationError, setRelationError] = useState("")
  const [commentOrder, setCommentOrder] = useState<"newest" | "oldest">("newest")
  const comments = commentOrder === "newest" ? [...detail.comments].reverse() : detail.comments
  const submitRelation = () => {
    if (!relationTarget.trim()) return
    setRelationBusy(true)
    setRelationError("")
    void onAddRelation(relationTarget.trim().toUpperCase(), relationType)
      .then(() => setRelationTarget(""))
      .catch((error) => setRelationError(error instanceof Error ? error.message : String(error)))
      .finally(() => setRelationBusy(false))
  }

  const save = async () => {
    setSaving(true)
    try {
      await onSave({
        title: title.trim(),
        description,
        pull_request: pullRequest.trim(),
        priority,
        ...(managed ? {} : {
          status,
          assignee: assignee.trim(),
          manual_block_reason: blockReason,
        }),
      })
    } finally {
      setSaving(false)
    }
  }

  return (
    <aside className="task-detail-panel">
      <header className="task-detail-header">
        <div>
          <h2>{task.key}</h2>
          <span>rev {task.revision} · {task.author}</span>
        </div>
        <button type="button" aria-label="Close task detail" onClick={onClose}><X /></button>
      </header>
      {task.access === "context" ? (
        <div className="tasks-empty">Context ancestor — open a visible descendant to edit.</div>
      ) : (
        <>
          <dl className="task-metadata">
            <div><dt>Author</dt><dd>{task.author}</dd></div>
            <div><dt>Customer</dt><dd>{task.customer}</dd></div>
            <div><dt>Queue</dt><dd>{task.queue}</dd></div>
            <div><dt>Group</dt><dd>{task.group || "—"}</dd></div>
            <div><dt>Blocked</dt><dd>{task.blocked ? "Yes" : "No"}</dd></div>
          </dl>
          {task.access !== "respond" ? <><div className="task-fields">
            <label>Title<Input value={title} onChange={(event) => setTitle(event.target.value)} /></label>
            <label>Description<Textarea value={description} onChange={(event) => setDescription(event.target.value)} /></label>
            <div className="task-field-grid">
              {!managed && <label>Status
                <select value={status} onChange={(event) => setStatus(event.target.value as TaskStatus)}>
                  <option value="open">Open</option>
                  <option value="in_progress">In progress</option>
                  <option value="wait_customer">Wait customer</option>
                  <option value="done">Done</option>
                  <option value="cancelled">Cancelled</option>
                </select>
              </label>}
              <label>Priority
                <select value={priority} onChange={(event) => setPriority(event.target.value as TaskPriority)}>
                  <option value="P0">P0 Critical</option>
                  <option value="P1">P1 High</option>
                  <option value="P2">P2 Normal</option>
                  <option value="P3">P3 Low</option>
                </select>
              </label>
              {!managed && <label>Assignee
                <Input aria-label="Assignee" list="task-assignees" value={assignee} onChange={(event) => setAssignee(event.target.value)} />
                <datalist id="task-assignees">
                  {principals?.agents.map((agent) => <option key={agent} value={agent} />)}
                </datalist>
              </label>}
            </div>
            <label>Pull request URL<Input value={pullRequest} onChange={(event) => setPullRequest(event.target.value)} /></label>
            {!managed && <label>Manual block reason<Input value={blockReason} onChange={(event) => setBlockReason(event.target.value)} /></label>}
            <button type="button" className="task-primary-action" disabled={saving || !title.trim()} onClick={() => void save()}>
              Save task
            </button>
          </div>
          {managed && (
            <section className="task-workflow" aria-label="Workflow execution">
              <div className="task-section-title">Managed workflow</div>
              <dl className="task-metadata">
                <div><dt>Version</dt><dd>{task.workflow_version || (workflow ? `${workflow.workflow.name}@${workflow.workflow.version}` : `#${task.workflow_version_id}`)}</dd></div>
                <div><dt>Status</dt><dd>{task.workflow_status || "—"}</dd></div>
                <div><dt>Revision</dt><dd>{task.workflow_revision ?? 0}</dd></div>
              </dl>
              {executionLoading ? <p>Loading execution…</p> : executionError ? <p role="alert">Execution unavailable: {executionError}</p> : null}
              {workflow?.status_executions.some((execution) => execution.state === "frozen") && <WorkflowFreezeError events={events} />}
              {!executionLoading && !executionError && <WorkflowList title="Assignments" empty="No assignments" items={(workflow?.assignments ?? []).map((assignment) => ({
                key: String(assignment.id),
                primary: assignment.agent || "Unclaimed",
                secondary: `${assignment.state} · attempt ${assignment.attempt}${assignment.outcome ? ` · ${assignment.outcome}` : ""}`,
              }))} />}
              {!executionLoading && !executionError && <WorkflowList title="Holds" empty="No active holds" items={(workflow?.holds ?? []).filter((hold) => !hold.released_at).map((hold) => ({
                key: String(hold.id), primary: hold.reason || hold.scope, secondary: hold.scope,
              }))} />}
              <WorkflowList title="Artifacts" empty="No artifacts" loading={artifactsLoading} error={artifactsError ? `Artifacts unavailable: ${artifactsError}` : ""} items={workflowArtifacts.map((artifact) => ({
                key: String(artifact.id), primary: artifact.name, secondary: artifact.content || artifact.type,
              }))} />
              <WorkflowList title="Questions" empty="No questions" loading={questionsLoading} error={questionsError ? `Questions unavailable: ${questionsError}` : ""} items={workflowQuestions.map((question) => ({
                key: String(question.id), primary: question.question, secondary: `${question.state} · ${question.blocking_scope}`,
              }))} />
              {!executionLoading && !executionError && <WorkflowList title="Observations" empty="No observations" items={(workflow?.observations ?? []).map((observation) => ({
                key: String(observation.id), primary: observation.kind, secondary: JSON.stringify(observation.payload ?? {}),
              }))} />}
            </section>
          )}
          <section className="task-relations">
            <div className="task-section-title">Dependencies <span>{detail.relations.length}</span></div>
            <ul>
              {detail.relations.map((relation) => {
                const other = relation.source_key === task.key ? relation.target_key : relation.source_key
                return (
                  <li key={relation.id}>
                    <span>{relation.type} {other}</span>
                    <button
                      type="button"
                      aria-label={`Remove relation to ${other}`}
                      onClick={() => {
                        setRelationError("")
                        void onDeleteRelation(relation.id).catch((error) =>
                          setRelationError(error instanceof Error ? error.message : String(error)))
                      }}
                    >
                      Remove
                    </button>
                  </li>
                )
              })}
            </ul>
            <form onSubmit={(event) => {
              event.preventDefault()
              submitRelation()
            }}>
              <select
                aria-label="Relation type"
                value={relationType}
                onChange={(event) => setRelationType(event.target.value as TaskRelationType)}
              >
                <option value="blocks">Blocks</option>
                <option value="related">Related</option>
              </select>
              <Input
                name="target_key"
                aria-label="Related task key"
                placeholder="TEST-2"
                value={relationTarget}
                onChange={(event) => setRelationTarget(event.target.value)}
              />
              <button type="button" onClick={submitRelation} disabled={relationBusy || !relationTarget.trim()}>Add relation</button>
            </form>
            {relationError && <p role="alert">{relationError}</p>}
          </section>
          </> : <div className="tasks-empty">Response access — comments only.</div>}
          <label>Comment order
            <select aria-label="Comment order" value={commentOrder} onChange={(event) => setCommentOrder(event.target.value as "newest" | "oldest")}>
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
            </select>
          </label>
          <TaskComments comments={comments} waits={detail.waiting_for} principals={principals} onComment={onComment} />
          <section className="task-history">
            <div className="task-section-title">History <span>{events.length}</span></div>
            <ol>
              {events.map((event) => (
                <li key={event.event_id}>
                  <strong>{event.kind}</strong>
                  <span>{event.actor}</span>
                  <time>{new Date(event.created_at).toLocaleString()}</time>
                  {event.kind.startsWith("workflow.") && Object.keys(event.payload).length > 0 && <code>{safeJSON(event.payload)}</code>}
                </li>
              ))}
            </ol>
          </section>
        </>
      )}
    </aside>
  )
}

function WorkflowList({ title, empty, items, loading = false, error = "" }: { title: string; empty: string; items: { key: string; primary: string; secondary: string }[]; loading?: boolean; error?: string }) {
  return <div className="task-workflow-list"><h3>{title} <span>{items.length}</span></h3>{loading ? <p>Loading {title.toLowerCase()}…</p> : error ? <p role="alert">{error}</p> : items.length === 0 ? <p>{empty}</p> : <ul>{items.map((item) => <li key={item.key}><strong>{item.primary}</strong><span>{item.secondary}</span></li>)}</ul>}</div>
}

function safeJSON(value: unknown): string {
  try { return JSON.stringify(value) } catch { return "[unavailable payload]" }
}

function WorkflowFreezeError({ events }: { events: TaskEvent[] }) {
  const escalation = [...events].reverse().find((event) => event.kind === "workflow.escalated")
  const code = typeof escalation?.payload.error_code === "string" ? escalation.payload.error_code : "unknown_error"
  const message = typeof escalation?.payload.message === "string" ? escalation.payload.message : "Workflow execution is frozen"
  return <p className="task-workflow-error" role="alert">{message} · {code}</p>
}
