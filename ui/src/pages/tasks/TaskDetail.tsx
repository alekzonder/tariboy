import { AlertCircle, ArrowLeft, HelpCircle, X } from "lucide-react"
import { type ComponentProps, type CSSProperties, type ReactNode, useEffect, useRef, useState } from "react"
import type {
  TaskDetail as Detail,
  Task,
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
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { MarkdownEditor, MarkdownContent, MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown"
import { SendFilesButton } from "@/components/SendFilesButton"
import { StatusPill } from "@/components/ui/status"
import { WORKFLOW_DOT, taskStatusLabel, taskTone, workflowRowTone } from "@/lib/statusTone"
import { formatTaskTime } from "./taskTime"
import { cn } from "@/lib/utils"
import { COUNT, DANGER_FILL, EMPTY, FIELD, FIELD_MONO, LABEL, MONO, PRIMARY_FILL, QUIET_ACTION, ROW, SelectShell } from "./panelStyles"
import type { StatusTone } from "@/lib/statusTone"
import type { ApiTarget } from "@/lib/api"

/**
 * The task detail panel of the Tariboy style layer. It is the same island as
 * the console content — `--card` on `--panel-radius` under `--lift` — with its
 * own scroll and a sticky identity header. Nothing inside it is separated by a
 * border: background, spacing and small labels do that work, so the panel
 * carries exactly two shadows (the header rule and the island) and exactly one
 * `600` weight (the task title).
 */

export default function TaskDetail({
  detail,
  target,
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
  width,
  resizeHandle,
  onClose,
  onSave,
  onComment,
  onAddRelation,
  onDeleteRelation,
}: {
  detail: Detail
  target?: ApiTarget
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
  width: number
  resizeHandle: ReactNode
  onClose: () => void
  onSave: (input: {
    revision: number
    title: string
    description: string
    pull_request: string
    status?: TaskStatus
    assignee?: string
    manual_block_reason?: string
    priority: TaskPriority
  }) => Promise<Task>
  onComment: (body: string, idempotencyKey: string) => Promise<void>
  onAddRelation: (targetKey: string, type: TaskRelationType) => Promise<void>
  onDeleteRelation: (relationID: number) => Promise<void>
}) {
  const task = detail.task
  const managed = Boolean(task.workflow_version_id)
  const [baseline, setBaseline] = useState(task)
  const [returnFocus] = useState(() => document.activeElement as HTMLElement | null)
  const initialFocusRef = useRef<HTMLButtonElement>(null)
  const [confirmClose, setConfirmClose] = useState(false)
  const [commentDirty, setCommentDirty] = useState(false)
  const [title, setTitle] = useState(task.title)
  const [description, setDescription] = useState(task.description)
  const [descriptionMode, setDescriptionMode] = useState<MarkdownMode>("rich")
  const [status, setStatus] = useState<TaskStatus>(task.status)
  const [pullRequest, setPullRequest] = useState(task.pull_request ?? "")
  const [priority, setPriority] = useState<TaskPriority>(task.priority)
  const [assignee, setAssignee] = useState(task.assignee)
  const [blockReason, setBlockReason] = useState(task.manual_block_reason)
  const [saving, setSaving] = useState(false)
  const [uploadingDescription, setUploadingDescription] = useState(false)
  const [uploadingComment, setUploadingComment] = useState(false)
  const pending = saving || uploadingDescription || uploadingComment
  const [relationType, setRelationType] = useState<TaskRelationType>("blocks")
  const [relationTarget, setRelationTarget] = useState("")
  const [relationBusy, setRelationBusy] = useState(false)
  const [relationError, setRelationError] = useState("")
  const [commentOrder, setCommentOrder] = useState<"newest" | "oldest">("newest")
  const [historyOpen, setHistoryOpen] = useState(true)
  const dirty = title !== baseline.title || description !== baseline.description
    || status !== baseline.status || pullRequest !== (baseline.pull_request ?? "")
    || priority !== baseline.priority || assignee !== baseline.assignee
    || blockReason !== baseline.manual_block_reason
  const hasDraft = dirty || commentDirty || Boolean(relationTarget.trim())
  const adopt = (next: Task) => {
    setBaseline(next)
    setTitle(next.title)
    setDescription(next.description)
    setStatus(next.status)
    setPullRequest(next.pull_request ?? "")
    setPriority(next.priority)
    setAssignee(next.assignee)
    setBlockReason(next.manual_block_reason)
  }
  // Refresh pristine forms; keep the revision that an unsaved draft was based on.
  if (baseline !== task && !dirty && !saving) adopt(task)
  const close = () => {
    if (pending) return
    if (hasDraft) setConfirmClose(true)
    else onClose()
  }
  useEffect(() => {
    if (!hasDraft) return
    const prevent = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = "" }
    window.addEventListener("beforeunload", prevent)
    return () => window.removeEventListener("beforeunload", prevent)
  }, [hasDraft])
  const keepEdits = () => {
    const merged = { ...task,
      title: title !== baseline.title ? title : task.title,
      description: description !== baseline.description ? description : task.description,
      status: status !== baseline.status ? status : task.status,
      pull_request: pullRequest !== (baseline.pull_request ?? "") ? pullRequest : task.pull_request,
      priority: priority !== baseline.priority ? priority : task.priority,
      assignee: assignee !== baseline.assignee ? assignee : task.assignee,
      manual_block_reason: blockReason !== baseline.manual_block_reason ? blockReason : task.manual_block_reason,
    }
    adopt(merged)
    setBaseline(task)
  }
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
      const updated = await onSave({
        revision: baseline.revision,
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
      adopt(updated)
    } catch {
      // The workspace reports the API error; keep the draft for retry.
    } finally {
      setSaving(false)
    }
  }

  const frozen = workflow?.status_executions.some((execution) => execution.state === "frozen") ?? false
  const openWaits = detail.waiting_for.filter((wait) => !wait.resolved_at)
  const editable = task.access !== "context" && task.access !== "respond"

  return (
    <Dialog open onOpenChange={(open) => { if (!open) close() }}>
    <DialogContent className="task-detail-dialog bg-card ring-0" showCloseButton={false} aria-describedby={undefined}
      /* The sheet dims the whole window — topbar and sidebar included — with the
         theme's own foreground, so both read as one tone behind it. */
      overlayClassName="bg-[color-mix(in_oklab,var(--foreground)_7%,transparent)] supports-backdrop-filter:backdrop-blur-none"
      style={{ "--tasks-detail-width": `${width}px` } as CSSProperties}
      onOpenAutoFocus={(event) => { event.preventDefault(); initialFocusRef.current?.focus() }}
      onCloseAutoFocus={(event) => { event.preventDefault(); if (returnFocus?.isConnected) returnFocus.focus() }}>
    {resizeHandle}
    <div className="task-detail-panel">
      {/* Sticky: whatever the panel is scrolled to, the key, the title and the
          status stay in view. The one shadow here is the same rule the selected
          row and the active segment carry. */}
      <header className="sticky top-0 z-[3] flex items-start gap-2.5 bg-card px-3.5 pt-[11px] pb-2.5 shadow-[var(--raise)]">
        <Button ref={initialFocusRef} variant="ghost" size="sm" disabled={pending} onClick={close}
          className="h-[26px] shrink-0 gap-1.5 rounded-[7px] pr-2 pl-1.5 text-[12.5px] font-normal text-muted-foreground hover:bg-accent hover:text-foreground">
          <ArrowLeft className="size-[13px] [stroke-width:1.4]" /> Back
        </Button>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-w-0 items-center gap-[9px]">
            <DialogTitle asChild>
              <h2 className="shrink-0 font-mono text-[12px] font-medium tabular-nums text-muted-foreground">{task.key}</h2>
            </DialogTitle>
            {/* The only 600 in the panel. */}
            <span className="min-w-0 truncate text-[15px] font-semibold tracking-[-.01em]">{task.title}</span>
          </div>
          <div className="flex min-w-0 flex-wrap items-center gap-2 text-[12px]">
            <StatusPill tone={taskTone(task.status)}>{taskStatusLabel(task.status)}</StatusPill>
            {managed
              ? <span title="Managed workflow version · revision"
                  className="inline-flex h-5 shrink-0 items-center gap-1.5 rounded-[6px] bg-muted px-[7px] font-mono text-[11.5px] tabular-nums">
                  {task.workflow_version || (workflow ? `${workflow.workflow.name}@${workflow.workflow.version}` : `#${task.workflow_version_id}`)}
                  <span className="opacity-60">rev {task.workflow_revision ?? 0}</span>
                </span>
              : <span className="text-[11.5px] text-muted-foreground">unmanaged · status set by hand</span>}
            <MetaInline label="agent" value={task.assignee || "unassigned"} />
            {task.parent_key && <span className="inline-flex items-center gap-1.5 text-muted-foreground">
              parent
              <span className="border-b border-dotted border-border font-mono text-[11.5px] font-medium text-foreground tabular-nums">{task.parent_key}</span>
            </span>}
            <MetaInline label="updated" value={formatTaskTime(task.updated_at)} />
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {editable && <Button size="sm" className="h-7" disabled={pending || !title.trim()} onClick={() => void save()}>Save task</Button>}
          <Button variant="ghost" size="icon" aria-label="Close task detail" disabled={pending} onClick={close}
            className="size-7 rounded-[8px] text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></Button>
        </div>
      </header>
      <div className="task-detail-body flex min-w-0 flex-col gap-[18px] px-4 pt-3.5 pb-7">
      {confirmClose && <Banner tone="danger" role="alert" icon={<AlertCircle className="size-3.5 [stroke-width:1.4]" />}
        text="Are you sure you want to close this task? Unsaved changes will be discarded."
        actions={<>
          {/* Keeping the draft is the safe default, so it stays quiet; only the
              action that throws work away carries the solid fill. */}
          <BannerButton tone="quiet" onClick={() => setConfirmClose(false)}>Keep editing</BannerButton>
          <BannerButton tone="danger" onClick={onClose}>Discard changes</BannerButton>
        </>} />}
      {dirty && baseline.revision !== task.revision && <div role="alert"
        className={cn("flex flex-col gap-2 rounded-[8px] px-2.5 py-2", DANGER_FILL)}>
        <div className="flex items-start gap-[9px]">
          <AlertCircle className="mt-px size-3.5 shrink-0 [stroke-width:1.4]" />
          <p className="min-w-0 flex-1 text-[12.5px] leading-[1.45] font-medium text-pretty">This task changed while you were editing. Keep your edited fields over the latest values, or reload to discard your edits.</p>
        </div>
        <details className="text-[12px]"><summary className="cursor-pointer">Latest values</summary>
          <dl className="mt-1.5 grid grid-cols-[76px_minmax(0,1fr)] gap-x-2.5 gap-y-1 text-foreground">
            <dt className={LABEL}>Title</dt><dd className="min-w-0 text-[12.5px]">{task.title}</dd>
            <dt className={LABEL}>Description</dt><dd className="min-w-0"><MarkdownContent>{task.description}</MarkdownContent></dd>
            <dt className={LABEL}>Status</dt><dd className={cn(MONO, "min-w-0")}>{task.status}</dd>
            <dt className={LABEL}>Priority</dt><dd className={cn(MONO, "min-w-0")}>{task.priority}</dd>
            <dt className={LABEL}>Assignee</dt><dd className={cn(MONO, "min-w-0")}>{task.assignee || "Unassigned"}</dd>
            <dt className={LABEL}>Pull request</dt><dd className={cn(MONO, "min-w-0 break-all")}>{task.pull_request || "None"}</dd>
            <dt className={LABEL}>Block reason</dt><dd className="min-w-0 text-[12.5px]">{task.manual_block_reason || "None"}</dd>
          </dl>
        </details>
        <div className="flex gap-1.5">
          <BannerButton tone="danger" disabled={saving} onClick={keepEdits}>Keep my edits</BannerButton>
          <BannerButton tone="quiet" disabled={saving} onClick={() => adopt(task)}>Reload task</BannerButton>
        </div>
      </div>}
      {task.access === "context" ? (
        <div className="tasks-empty">Context ancestor — open a visible descendant to edit.</div>
      ) : (
        <>
          {/* One banner at a time, and only for what is failing or waiting on a
              person — open, in progress and done speak through the status pill. */}
          {frozen
            ? <WorkflowFreezeBanner events={events} />
            : openWaits.length > 0 && <Banner tone="primary" icon={<HelpCircle className="mt-0.5 size-3.5 [stroke-width:1.4]" />}
                text={`Waiting for an answer from ${openWaits.map((wait) => wait.expected_principal).join(", ")} — reply in the comments below.`} />}
          <dl className="grid grid-cols-2 gap-x-6 gap-y-[9px]">
            <Meta label="Author" value={task.author} />
            <Meta label="Customer" value={task.customer} />
            <Meta label="Queue" value={task.queue} />
            <Meta label="Group" value={task.group} />
            <Meta label="Blocked" value={task.blocked ? "Yes" : "No"} mono={false} />
            <Meta label="Parent" value={task.parent_key} />
          </dl>
          {task.access !== "respond" ? <>
          <fieldset className="contents" disabled={saving}>
            <label className="flex min-w-0 flex-col gap-1.5">
              <span className={LABEL}>Title</span>
              <Input className={FIELD} value={title} onChange={(event) => setTitle(event.target.value)} />
            </label>
            <div className="task-description flex min-w-0 flex-col gap-[7px]">
              <div className="flex items-center gap-2">
                <label htmlFor="task-description" className={LABEL}>Description</label>
                <MarkdownModeSegment label="Description editing mode" mode={descriptionMode} onModeChange={setDescriptionMode} disabled={saving} />
                <SendFilesButton daemon={target} disabled={saving} onUploadingChange={setUploadingDescription}
                  className={cn(QUIET_ACTION, "ml-auto border-0 bg-transparent")}
                  onUploaded={(paths) => setDescription((current) => current + (current && !current.endsWith("\n") ? "\n" : "") + paths.join("\n"))} />
              </div>
              <MarkdownEditor id="task-description" placeholder="Description" value={description} onChange={setDescription}
                disabled={saving} mode={descriptionMode} />
            </div>
            <div className="task-properties grid min-w-0 grid-cols-2 gap-x-4 gap-y-2.5">
              {/* A managed task's status belongs to its workflow. */}
              {!managed && <SelectField label="Status" value={status} onChange={(value) => setStatus(value as TaskStatus)}>
                <option value="open">Open</option>
                <option value="in_progress">In progress</option>
                <option value="wait_customer">Wait customer</option>
                <option value="done">Done</option>
                <option value="cancelled">Cancelled</option>
              </SelectField>}
              <SelectField label="Priority" value={priority} onChange={(value) => setPriority(value as TaskPriority)} mono>
                <option value="P0">P0 Critical</option>
                <option value="P1">P1 High</option>
                <option value="P2">P2 Normal</option>
                <option value="P3">P3 Low</option>
              </SelectField>
              {!managed && <label className="flex min-w-0 flex-col gap-[5px]">
                <span className={LABEL}>Assignee</span>
                <Input aria-label="Assignee" list="task-assignees" className={cn(FIELD, FIELD_MONO)}
                  value={assignee} onChange={(event) => setAssignee(event.target.value)} />
                <datalist id="task-assignees">
                  {principals?.agents.map((agent) => <option key={agent} value={agent} />)}
                </datalist>
              </label>}
              <label className="flex min-w-0 flex-col gap-[5px]">
                <span className={LABEL}>Pull request</span>
                <Input className={cn(FIELD, FIELD_MONO)} value={pullRequest} onChange={(event) => setPullRequest(event.target.value)} />
              </label>
              {!managed && <label className="flex min-w-0 flex-col gap-[5px]">
                <span className={LABEL}>Manual block reason</span>
                <Input className={FIELD} value={blockReason} onChange={(event) => setBlockReason(event.target.value)} />
              </label>}
            </div>
          </fieldset>
          {managed && (
            /* A nested level reads as a quieter fill, never as a box. */
            <section className="task-workflow flex min-w-0 flex-col gap-2.5 rounded-[8px] bg-[color-mix(in_oklab,var(--muted)_55%,transparent)] p-3" aria-label="Workflow execution">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[12.5px] font-medium">Managed workflow</span>
                <span className="inline-flex h-[19px] items-center rounded-[6px] bg-card px-[7px] font-mono text-[11px] tabular-nums">
                  {task.workflow_version || (workflow ? `${workflow.workflow.name}@${workflow.workflow.version}` : `#${task.workflow_version_id}`)}
                </span>
                {task.workflow_status && <StatusPill tone={workflowRowTone(task.workflow_status)} size="sm">{task.workflow_status}</StatusPill>}
                <span className="ml-auto font-mono text-[11px] tabular-nums text-muted-foreground">rev {task.workflow_revision ?? 0}</span>
              </div>
              {/* Assignments, holds and observations all come from the one
                  execution projection, so its loading and failure speak once
                  for the three rather than repeating the same line. */}
              {executionLoading && <span className={EMPTY}>Loading execution…</span>}
              {executionError && <span role="alert" className="text-[12px] text-status-failed">Execution unavailable: {executionError}</span>}
              <div className="grid min-w-0 grid-cols-2 gap-x-5 gap-y-3">
                {!executionLoading && !executionError && <WorkflowList title="Assignments" empty="No assignments"
                  items={(workflow?.assignments ?? []).map((assignment) => ({
                    key: String(assignment.id),
                    primary: assignment.agent || "Unclaimed",
                    mono: Boolean(assignment.agent),
                    tone: workflowRowTone(assignment.state),
                    secondary: `${assignment.state} · attempt ${assignment.attempt}${assignment.outcome ? ` · ${assignment.outcome}` : ""}`,
                  }))} />}
                {!executionLoading && !executionError && <WorkflowList title="Holds" empty="No active holds"
                  items={(workflow?.holds ?? []).filter((hold) => !hold.released_at).map((hold) => ({
                    key: String(hold.id), primary: hold.reason || hold.scope, secondary: hold.scope, tone: "attention" as StatusTone,
                  }))} />}
                <WorkflowList title="Artifacts" empty="No artifacts" loading={artifactsLoading}
                  error={artifactsError ? `Artifacts unavailable: ${artifactsError}` : ""}
                  items={workflowArtifacts.map((artifact) => ({
                    key: String(artifact.id), primary: artifact.name, mono: true, secondary: artifact.content || artifact.type,
                  }))} />
                <WorkflowList title="Questions" empty="No questions" loading={questionsLoading}
                  error={questionsError ? `Questions unavailable: ${questionsError}` : ""}
                  items={workflowQuestions.map((question) => ({
                    key: String(question.id), primary: question.question, tone: workflowRowTone(question.state),
                    secondary: `${question.state} · ${question.blocking_scope}`,
                  }))} />
                {!executionLoading && !executionError && <WorkflowList title="Observations" empty="No observations"
                  items={(workflow?.observations ?? []).map((observation) => ({
                    key: String(observation.id), primary: observation.kind, mono: true, secondary: summarizePayload(observation.payload),
                  }))} />}
              </div>
            </section>
          )}
          <section className="flex min-w-0 flex-col gap-1.5">
            <SectionHeading label="Dependencies" count={detail.relations.length} />
            {detail.relations.length === 0 && <span className={EMPTY}>Nothing blocks this task.</span>}
            {detail.relations.map((relation) => {
              const other = relation.source_key === task.key ? relation.target_key : relation.source_key
              return (
                <div key={relation.id} className={ROW}>
                  <span className={cn("inline-flex h-[19px] shrink-0 items-center rounded-[6px] px-[7px] text-[11px]",
                    relation.type === "blocks"
                      ? cn(DANGER_FILL, "font-medium")
                      : "bg-muted text-muted-foreground")}>{relation.type}</span>
                  {/* The key is underlined only as far as it reads; the rest of
                      the row is the space the reference gives a title. */}
                  <span className="shrink-0 border-b border-dotted border-border font-mono text-[11.5px] font-medium tabular-nums">{other}</span>
                  <span className="min-w-0 flex-1" />
                  <Button type="button" variant="ghost" size="icon-xs" aria-label={`Remove relation to ${other}`}
                    className="size-6 shrink-0 rounded-[7px] text-muted-foreground hover:bg-accent hover:text-destructive"
                    onClick={() => {
                      setRelationError("")
                      void onDeleteRelation(relation.id).catch((error) =>
                        setRelationError(error instanceof Error ? error.message : String(error)))
                    }}><X className="size-3" /></Button>
                </div>
              )
            })}
            <form className="flex items-center gap-1.5 pt-0.5" onSubmit={(event) => { event.preventDefault(); submitRelation() }}>
              <SelectShell aria-label="Relation type" value={relationType}
                className="h-[26px] w-auto text-[12px] md:text-[12px]"
                onChange={(event) => setRelationType(event.target.value as TaskRelationType)}>
                <option value="blocks">Blocks</option>
                <option value="related">Related</option>
              </SelectShell>
              <Input name="target_key" aria-label="Related task key" placeholder="TEST-2" value={relationTarget}
                className={cn(FIELD, FIELD_MONO, "h-[26px] w-[118px]")}
                onChange={(event) => setRelationTarget(event.target.value)} />
              <Button type="button" variant="ghost" onClick={submitRelation} disabled={relationBusy || !relationTarget.trim()}
                className="h-[26px] rounded-[7px] bg-accent px-2.5 text-[12px] hover:bg-secondary">Add relation</Button>
            </form>
            {relationError && <p role="alert" className="text-[12px] text-status-failed">{relationError}</p>}
          </section>
          </> : <div className="flex min-w-0 flex-col gap-2">
            {/* No second title here: the sticky header already carries it, and
                it is the panel's only 600. */}
            <span className={LABEL}>Description</span>
            <div className="rounded-[8px] bg-muted px-3 py-[11px]"><MarkdownContent>{task.description}</MarkdownContent></div>
            <p className={EMPTY}>Response access — comments only.</p>
          </div>}
          <TaskComments comments={comments} waits={detail.waiting_for} principals={principals} assignee={task.assignee}
            formFirst={commentOrder === "newest"} order={commentOrder} onOrderChange={setCommentOrder}
            onComment={onComment} onDirtyChange={setCommentDirty} target={target} onUploadingChange={setUploadingComment} />
          <section className="flex min-w-0 flex-col gap-1">
            <SectionHeading label="History" count={events.length} action={
              <Button type="button" variant="ghost" className={QUIET_ACTION} aria-expanded={historyOpen}
                onClick={() => setHistoryOpen(!historyOpen)}>
                {historyOpen ? "Collapse" : "Expand"}
              </Button>} />
            {historyOpen && events.map((event) => (
              <div key={event.event_id} className={ROW}>
                <time dateTime={event.created_at} title={new Date(event.created_at).toLocaleString()}
                  className="w-[52px] shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">{formatTaskTime(event.created_at)}</time>
                <span className="shrink-0 font-mono text-[11.5px] font-medium">{event.kind}</span>
                <span className="min-w-0 flex-1 truncate text-[12px] text-muted-foreground">{summarizePayload(event.payload)}</span>
                <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{event.actor}</span>
              </div>
            ))}
            {historyOpen && events.length === 0 && <span className={EMPTY}>Nothing has happened yet.</span>}
          </section>
        </>
      )}
      </div>
    </div>
    </DialogContent>
    </Dialog>
  )
}

/** `label value` in the header strip: the word is quiet, the value is mono. */
function MetaInline({ label, value }: { label: string; value: string }) {
  return <span className="inline-flex min-w-0 items-center gap-1.5 text-muted-foreground">
    {label}<span className="min-w-0 truncate font-mono text-[11.5px] tabular-nums text-foreground">{value}</span>
  </span>
}

/** One pair of the overview grid. An empty value is an em dash, not a hole. */
function Meta({ label, value, mono = true }: { label: string; value: string; mono?: boolean }) {
  return <div className="flex min-w-0 items-baseline gap-2.5">
    <dt className={cn(LABEL, "w-[76px] shrink-0 font-normal")}>{label}</dt>
    <dd className={cn("min-w-0 truncate", mono ? MONO : "text-[12.5px]")}>{value || "—"}</dd>
  </div>
}

function SectionHeading({ label, count, action }: { label: string; count: number; action?: ReactNode }) {
  return <div className="flex items-center gap-1.5">
    <span className={LABEL}>{label}</span>
    <span className={COUNT}>{count}</span>
    {action && <span className="ml-auto">{action}</span>}
  </div>
}

/** The banner shape: one fill, one icon, one line, optional actions. */
function Banner({ tone, icon, text, actions, role }: {
  tone: "danger" | "primary"
  icon: ReactNode
  text: string
  actions?: ReactNode
  role?: string
}) {
  return <div role={role}
    className={cn("flex items-start gap-[9px] rounded-[8px] px-2.5 py-2", tone === "danger" ? DANGER_FILL : PRIMARY_FILL)}>
    <span className="shrink-0">{icon}</span>
    <span className="min-w-0 flex-1 self-center text-[12.5px] leading-[1.45] font-medium text-pretty">{text}</span>
    {actions}
  </div>
}

/** The action a banner carries. `quiet` is the one that changes nothing. */
function BannerButton({ tone, children, className, ...props }: {
  tone: "danger" | "quiet"
} & ComponentProps<typeof Button>) {
  return <Button type="button" variant="ghost" {...props}
    className={cn("h-6 shrink-0 rounded-[7px] px-2.5 text-[12px]",
      tone === "danger"
        ? "bg-[color-mix(in_oklab,var(--status-failed)_14%,transparent)] text-status-failed hover:bg-[color-mix(in_oklab,var(--status-failed)_20%,transparent)] hover:text-status-failed"
        : "text-status-failed hover:bg-[color-mix(in_oklab,var(--status-failed)_10%,transparent)] hover:text-status-failed",
      className)}>
    {children}
  </Button>
}

/** A select is the same fill as an input, with the chevron drawn over it. */
function SelectField({ label, value, onChange, mono = false, children }: {
  label: string
  value: string
  onChange: (value: string) => void
  mono?: boolean
  children: ReactNode
}) {
  return <label className="flex min-w-0 flex-col gap-[5px]">
    <span className={LABEL}>{label}</span>
    <SelectShell className={cn("flex", mono && FIELD_MONO)} value={value} onChange={(event) => onChange(event.target.value)}>
      {children}
    </SelectShell>
  </label>
}

type WorkflowItem = { key: string; primary: string; secondary: string; tone?: StatusTone; mono?: boolean }

/** One of the five managed-workflow lists. Loading and failure speak in the
 *  list's own place rather than replacing the section. */
function WorkflowList({ title, empty, items, loading = false, error = "" }: {
  title: string
  empty: string
  items: WorkflowItem[]
  loading?: boolean
  error?: string
}) {
  return <div className="flex min-w-0 flex-col">
    <div className="flex h-6 items-center gap-1.5 px-0.5">
      <span className={LABEL}>{title}</span>
      <span className={COUNT}>{items.length}</span>
    </div>
    {loading ? <span className={EMPTY}>Loading {title.toLowerCase()}…</span>
      : error ? <span role="alert" className="text-[12px] text-status-failed">{error}</span>
      : items.length === 0 ? <span className={EMPTY}>{empty}</span>
      : items.map((item) => (
        <div key={item.key} className={ROW}>
          <span className={cn("size-1.5 shrink-0 rounded-full", WORKFLOW_DOT[item.tone ?? "quiet"])} />
          <span className={cn("min-w-0 flex-1 truncate", item.mono ? "font-mono text-[11.5px]" : "text-[12.5px]")}>{item.primary}</span>
          <span className="max-w-[46%] shrink-0 truncate font-mono text-[11px] tabular-nums text-muted-foreground">{item.secondary}</span>
        </div>
      ))}
  </div>
}

/** Event payloads read as `key value · key value`; a JSON dump is a log, not a
 *  row, and the panel never shows one. */
function summarizePayload(payload: Record<string, unknown> | undefined): string {
  if (!payload) return ""
  return Object.entries(payload)
    .filter(([, value]) => value !== null && value !== undefined && value !== "")
    .map(([key, value]) => `${key} ${typeof value === "object" ? JSON.stringify(value) : String(value)}`)
    .join(" · ")
}

function WorkflowFreezeBanner({ events }: { events: TaskEvent[] }) {
  const escalation = [...events].reverse().find((event) => event.kind === "workflow.escalated")
  const code = typeof escalation?.payload.error_code === "string" ? escalation.payload.error_code : "unknown_error"
  const message = typeof escalation?.payload.message === "string" ? escalation.payload.message : "Workflow execution is frozen"
  return <Banner tone="danger" role="alert" icon={<AlertCircle className="size-3.5 [stroke-width:1.4]" />} text={message}
    actions={<span className="shrink-0 self-center font-mono text-[11px] opacity-80">{code}</span>} />
}
