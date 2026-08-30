import { Plus, RefreshCw, Search } from "lucide-react"
import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { toast } from "sonner"
import { useOptionalDaemons } from "@/components/DaemonProvider"
import { Input } from "@/components/ui/input"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import { ApiError, type ApiTarget } from "@/lib/api"
import { buildTaskForest, canDropTaskInside } from "@/lib/taskTree"
import {
  addTaskComment,
  addTaskRelation,
  createTask,
  createTaskQueue,
  deleteTaskRelation,
  dismissTaskNotification,
  getTask,
  getTaskWorkflow,
  listTaskNotifications,
  listTaskEvents,
  listTaskPrincipals,
  listTaskQueues,
  listWorkflowArtifacts,
  listWorkflowQuestions,
  listTasks,
  markTaskNotificationRead,
  moveTask,
  updateTaskQueue,
  updateTask,
  type CreateQueueInput,
  type Task,
  type TaskDetail as Detail,
  type TaskPriority,
  type TaskEvent,
  type TaskNotification,
  type TaskPrincipals,
  type TaskQueue,
  type TaskRelationType,
  type TaskStatus,
  type TaskStatusView,
  type WorkflowArtifact,
  type WorkflowExecutionView,
  type WorkflowQuestion,
} from "@/lib/tasks"
import QueueSettings from "./QueueSettings"
import TaskDetail from "./TaskDetail"
import TaskForm from "./TaskForm"
import TaskNotifications from "./TaskNotifications"
import TaskTree from "./TaskTree"
import TasksNavigation, { type TasksView } from "./TasksNavigation"
import {
  DEFAULT_TASK_DETAIL_WIDTH,
  DEFAULT_TASK_NAVIGATION_WIDTH,
  MAX_TASK_DETAIL_WIDTH,
  MAX_TASK_NAVIGATION_WIDTH,
  MIN_TASK_DETAIL_WIDTH,
  MIN_TASK_NAVIGATION_WIDTH,
  useTaskPanelWidths,
} from "./useTaskPanelWidths"
import "./tasks.css"

function actorAgent(name: string): string {
  return name.startsWith("agent:") ? name : `agent:${name}`
}

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `task-action-${Date.now()}-${Math.random()}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

type TasksWorkspaceProps = {
  scopeAgent?: string
  target?: ApiTarget
  initialTaskKey?: string
  onNotificationsChanged?: () => void
}

const TASK_CENTER_MIN_WIDTH = 360
const TASK_PANEL_HANDLE_WIDTHS = 8

function effectiveTaskPanelWidths(
  workspaceWidth: number,
  navigationWidth: number,
  detailWidth: number,
) {
  if (workspaceWidth <= 0) return { navigationWidth, detailWidth }
  const sidePanelBudget = Math.max(
    MIN_TASK_NAVIGATION_WIDTH + MIN_TASK_DETAIL_WIDTH,
    Math.floor(workspaceWidth) - TASK_CENTER_MIN_WIDTH - TASK_PANEL_HANDLE_WIDTHS,
  )
  const effectiveDetailWidth = Math.min(
    detailWidth,
    sidePanelBudget - MIN_TASK_NAVIGATION_WIDTH,
  )
  return {
    navigationWidth: Math.min(navigationWidth, sidePanelBudget - effectiveDetailWidth),
    detailWidth: effectiveDetailWidth,
  }
}

function TaskPanelResizeHandle({
  panel,
  width,
  maximum,
  workspaceRef,
  onResize,
}: {
  panel: "navigation" | "detail"
  width: number
  maximum: number
  workspaceRef: React.RefObject<HTMLDivElement | null>
  onResize: (width: number) => void
}) {
  const cleanupDragRef = useRef<(() => void) | null>(null)
  const navigation = panel === "navigation"
  const minimum = navigation ? MIN_TASK_NAVIGATION_WIDTH : MIN_TASK_DETAIL_WIDTH
  const defaultWidth = navigation ? DEFAULT_TASK_NAVIGATION_WIDTH : DEFAULT_TASK_DETAIL_WIDTH
  const label = navigation ? "Resize task navigation" : "Resize task details"
  const resize = (requestedWidth: number) => {
    onResize(Math.min(requestedWidth, maximum))
  }

  useEffect(() => () => cleanupDragRef.current?.(), [])

  const startDrag = (event: React.PointerEvent<HTMLDivElement>) => {
    if ((Number.isFinite(event.button) && event.button !== 0) || event.isPrimary === false) return
    event.preventDefault()
    cleanupDragRef.current?.()
    const pointerId = event.pointerId
    const target = event.currentTarget
    let finished = false
    const isDifferentPointer = (candidate: number) => (
      Number.isFinite(pointerId) && Number.isFinite(candidate) && candidate !== pointerId
    )
    const onMove = (moveEvent: PointerEvent) => {
      if (isDifferentPointer(moveEvent.pointerId)) return
      const bounds = workspaceRef.current?.getBoundingClientRect()
      if (!bounds) return
      resize(navigation ? moveEvent.clientX - bounds.left : bounds.right - moveEvent.clientX)
    }
    const cleanup = () => {
      if (finished) return
      finished = true
      cleanupDragRef.current = null
      window.removeEventListener("pointermove", onMove)
      window.removeEventListener("pointerup", onEnd)
      window.removeEventListener("pointercancel", onEnd)
      window.removeEventListener("blur", cleanup)
      document.removeEventListener("visibilitychange", onVisibilityChange)
      target.removeEventListener("lostpointercapture", onLostPointerCapture)
      document.body.style.removeProperty("cursor")
      document.body.style.removeProperty("user-select")
      try {
        if (target.hasPointerCapture?.(pointerId)) target.releasePointerCapture(pointerId)
      } catch {
        // The WebView may already have released capture during cancellation.
      }
    }
    const onEnd = (endEvent: PointerEvent) => {
      if (!isDifferentPointer(endEvent.pointerId)) cleanup()
    }
    const onLostPointerCapture = (lostEvent: PointerEvent) => {
      if (!isDifferentPointer(lostEvent.pointerId)) cleanup()
    }
    const onVisibilityChange = () => {
      if (document.visibilityState !== "visible") cleanup()
    }
    window.addEventListener("pointermove", onMove)
    window.addEventListener("pointerup", onEnd)
    window.addEventListener("pointercancel", onEnd)
    window.addEventListener("blur", cleanup)
    document.addEventListener("visibilitychange", onVisibilityChange)
    target.addEventListener("lostpointercapture", onLostPointerCapture)
    cleanupDragRef.current = cleanup
    try {
      target.setPointerCapture?.(pointerId)
    } catch {
      // Window listeners still own the session when capture is unavailable.
    }
    document.body.style.cursor = "col-resize"
    document.body.style.userSelect = "none"
  }

  const onKeyDown = (event: React.KeyboardEvent) => {
    const step = event.shiftKey ? 32 : 8
    const direction = navigation ? 1 : -1
    if (event.key === "ArrowLeft") {
      event.preventDefault()
      resize(width - step * direction)
    } else if (event.key === "ArrowRight") {
      event.preventDefault()
      resize(width + step * direction)
    } else if (event.key === "Home") {
      event.preventDefault()
      resize(defaultWidth)
    }
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={minimum}
      aria-valuemax={maximum}
      tabIndex={0}
      className="task-panel-resize-handle"
      onPointerDown={startDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => resize(defaultWidth)}
    />
  )
}

function TasksWorkspaceContent({
  scopeAgent,
  target,
  initialTaskKey,
  onNotificationsChanged,
}: TasksWorkspaceProps) {
  const workspaceRef = useRef<HTMLDivElement | null>(null)
  const {
    navigationWidth,
    detailWidth,
    setNavigationWidth,
    setDetailWidth,
  } = useTaskPanelWidths()
  const [workspaceWidth, setWorkspaceWidth] = useState(0)
  useEffect(() => {
    const workspace = workspaceRef.current
    if (!workspace) return
    const measure = () => setWorkspaceWidth(workspace.getBoundingClientRect().width)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(workspace)
    window.addEventListener("resize", measure)
    return () => {
      observer.disconnect()
      window.removeEventListener("resize", measure)
    }
  }, [])
  const effectiveWidths = effectiveTaskPanelWidths(
    workspaceWidth,
    navigationWidth,
    detailWidth,
  )
  const sidePanelBudget = workspaceWidth > 0
    ? Math.max(
      MIN_TASK_NAVIGATION_WIDTH + MIN_TASK_DETAIL_WIDTH,
      Math.floor(workspaceWidth) - TASK_CENTER_MIN_WIDTH - TASK_PANEL_HANDLE_WIDTHS,
    )
    : MAX_TASK_NAVIGATION_WIDTH + MAX_TASK_DETAIL_WIDTH
  const navigationMaximum = Math.min(
    MAX_TASK_NAVIGATION_WIDTH,
    Math.max(MIN_TASK_NAVIGATION_WIDTH, sidePanelBudget - effectiveWidths.detailWidth),
  )
  const detailMaximum = Math.min(
    MAX_TASK_DETAIL_WIDTH,
    Math.max(MIN_TASK_DETAIL_WIDTH, sidePanelBudget - effectiveWidths.navigationWidth),
  )
  const [queues, setQueues] = useState<TaskQueue[]>([])
  const [principals, setPrincipals] = useState<TaskPrincipals | null>(null)
  const [metadataError, setMetadataError] = useState("")
  const [tasks, setTasks] = useState<Task[]>([])
  const [notifications, setNotifications] = useState<TaskNotification[]>([])
  const [view, setView] = useState<TasksView>("all")
  const [statusView, setStatusView] = useState<TaskStatusView>("active")
  const [queue, setQueue] = useState("")
  const [query, setQuery] = useState("")
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [selectedKey, setSelectedKey] = useState("")
  const selectedKeyRef = useRef("")
  const [detail, setDetail] = useState<Detail | null>(null)
  const [events, setEvents] = useState<TaskEvent[]>([])
  const [workflow, setWorkflow] = useState<WorkflowExecutionView | null>(null)
  const [workflowArtifacts, setWorkflowArtifacts] = useState<WorkflowArtifact[]>([])
  const [workflowQuestions, setWorkflowQuestions] = useState<WorkflowQuestion[]>([])
  const [executionLoading, setExecutionLoading] = useState(false)
  const [executionError, setExecutionError] = useState("")
  const [artifactsLoading, setArtifactsLoading] = useState(false)
  const [artifactsError, setArtifactsError] = useState("")
  const [questionsLoading, setQuestionsLoading] = useState(false)
  const [questionsError, setQuestionsError] = useState("")
  const [creatingParent, setCreatingParent] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [sequence, setSequence] = useState(0)
  const [socketEnabled, setSocketEnabled] = useState(false)
  const hasLoadedTreeRef = useRef(false)
  const treeRequestRef = useRef(0)
  const metadataRequestRef = useRef(0)
  const detailRequestRef = useRef(0)
  const mountedRef = useRef(true)
  const realtimeRefreshRef = useRef({ running: false, pending: false, scheduled: false })
  const principalFilter = !scopeAgent && (view === "mine" || view === "waiting")
    ? principals?.customer
    : undefined
  const principalMetadataError = !scopeAgent && (view === "mine" || view === "waiting")
    ? metadataError
    : ""

  const loadMetadata = useCallback(async () => {
    const request = ++metadataRequestRef.current
    try {
      const [queuePage, people, inbox] = await Promise.all([
        listTaskQueues(target),
        listTaskPrincipals(target),
        listTaskNotifications(false, target),
      ])
      if (!mountedRef.current || request !== metadataRequestRef.current) return
      const nextQueues = queuePage.queues ?? []
      setQueues(nextQueues)
      setPrincipals(people)
      setNotifications(inbox.notifications ?? [])
      setMetadataError("")
    } catch (error) {
      if (!mountedRef.current || request !== metadataRequestRef.current) return
      const message = error instanceof Error ? error.message : String(error)
      setMetadataError(message)
      toast.error(message)
    }
  }, [target])

  const loadTree = useCallback(async () => {
    if (view === "notifications" || view === "queues") return
    const request = ++treeRequestRef.current
    if (!scopeAgent && (view === "mine" || view === "waiting") && !principalFilter) {
      setLoading(!principalMetadataError)
      setRefreshing(false)
      return
    }
    const initial = !hasLoadedTreeRef.current
    if (initial) setLoading(true)
    else setRefreshing(true)
    try {
      const filters: Parameters<typeof listTasks>[0] = {
        queue,
        text: query,
        scope_agent: scopeAgent,
        status_view: statusView,
      }
      if (view === "mine") filters.assignee = scopeAgent ? actorAgent(scopeAgent) : principalFilter
      if (view === "waiting") filters.waiting_for = scopeAgent ? actorAgent(scopeAgent) : principalFilter
      const loaded: Task[] = []
      let after = ""
      let latestSequence = 0
      const seenCursors = new Set<string>()
      do {
        const page = await listTasks({ ...filters, limit: 500, after }, target)
        loaded.push(...(page.tasks ?? []))
        latestSequence = Math.max(latestSequence, page.sequence ?? 0)
        after = page.next_cursor ?? ""
        if (after && seenCursors.has(after)) throw new Error("task pagination cursor repeated")
        if (after) seenCursors.add(after)
      } while (after)
      if (request !== treeRequestRef.current) return
      setTasks(loaded)
      setSequence((current) => Math.max(current, latestSequence))
      hasLoadedTreeRef.current = true
      setSocketEnabled(true)
    } catch (error) {
      if (mountedRef.current && request === treeRequestRef.current) {
        toast.error(error instanceof Error ? error.message : String(error))
      }
    } finally {
      if (request === treeRequestRef.current) {
        setLoading(false)
        if (!initial) setRefreshing(false)
      }
    }
  }, [principalFilter, principalMetadataError, query, queue, scopeAgent, statusView, target, view])

  const loadDetail = useCallback(async (key: string) => {
    selectedKeyRef.current = key
    const request = ++detailRequestRef.current
    try {
      const loadHistory = async (): Promise<TaskEvent[]> => {
        const history: TaskEvent[] = []
        let after = 0
        for (;;) {
          const page = await listTaskEvents(key, after, 200, target)
          const batch = page.events ?? []
          history.push(...batch)
          if (batch.length < 200) return history.slice(-200)
          const next = batch[batch.length - 1]?.sequence ?? after
          if (next <= after) throw new Error("task history cursor did not advance")
          after = next
        }
      }
      const [next, history] = await Promise.all([
        getTask(key, target),
        loadHistory(),
      ])
      if (!mountedRef.current || request !== detailRequestRef.current || selectedKeyRef.current !== key) return
      setDetail(next)
      setEvents(history)
      setWorkflow(null)
      setWorkflowArtifacts([])
      setWorkflowQuestions([])
      setExecutionError("")
      setArtifactsError("")
      setQuestionsError("")
      setSelectedKey(key)
      if (next.task.workflow_version_id) {
        setExecutionLoading(true)
        setArtifactsLoading(true)
        setQuestionsLoading(true)
        const current = () => mountedRef.current && request === detailRequestRef.current
        await Promise.all([
          getTaskWorkflow(key, target).then((value) => { if (current()) setWorkflow(value) })
            .catch((error) => { if (current()) setExecutionError(errorMessage(error)) })
            .finally(() => { if (current()) setExecutionLoading(false) }),
          listWorkflowArtifacts(key, target).then((page) => { if (current()) setWorkflowArtifacts(page.items ?? []) })
            .catch((error) => { if (current()) setArtifactsError(errorMessage(error)) })
            .finally(() => { if (current()) setArtifactsLoading(false) }),
          listWorkflowQuestions(key, target).then((page) => { if (current()) setWorkflowQuestions(page.items ?? []) })
            .catch((error) => { if (current()) setQuestionsError(errorMessage(error)) })
            .finally(() => { if (current()) setQuestionsLoading(false) }),
        ])
        if (!current()) return
      }
      if (!next.task.workflow_version_id) {
        setExecutionLoading(false)
        setArtifactsLoading(false)
        setQuestionsLoading(false)
      }
    } catch (error) {
      if (!mountedRef.current || request !== detailRequestRef.current) return
      selectedKeyRef.current = ""
      setSelectedKey("")
      setDetail(null)
      setEvents([])
      setWorkflow(null)
      setWorkflowArtifacts([])
      setWorkflowQuestions([])
      setExecutionLoading(false)
      setExecutionError("")
      setArtifactsLoading(false)
      setArtifactsError("")
      setQuestionsLoading(false)
      setQuestionsError("")
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }, [target])

  const refreshInbox = useCallback(async () => {
    try {
      const page = await listTaskNotifications(false, target)
      setNotifications(page.notifications)
    } catch {
      // HTTP remains primary; an inbox refresh failure must not blank the tree.
    }
  }, [target])

  const refreshFromRealtimeHint = useCallback(() => {
    const refresh = realtimeRefreshRef.current
    if (refresh.running) {
      refresh.pending = true
      return
    }
    if (refresh.scheduled) return
    refresh.scheduled = true
    void Promise.resolve().then(async () => {
      refresh.scheduled = false
      if (!mountedRef.current) return
      refresh.running = true
      do {
        refresh.pending = false
        await Promise.all([
          loadTree(),
          loadMetadata(),
          refreshInbox(),
          selectedKeyRef.current ? loadDetail(selectedKeyRef.current) : Promise.resolve(),
        ])
      } while (refresh.pending && mountedRef.current)
      refresh.running = false
    })
  }, [loadDetail, loadMetadata, loadTree, refreshInbox])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])
  useEffect(() => {
    void Promise.resolve().then(loadMetadata)
  }, [loadMetadata])
  useEffect(() => {
    void Promise.resolve().then(loadTree)
  }, [loadTree])
  useEffect(() => {
    if (initialTaskKey) void Promise.resolve().then(() => loadDetail(initialTaskKey))
  }, [initialTaskKey, loadDetail])

  useTasksSocket({
    target,
    after: sequence,
    enabled: socketEnabled,
    onHint: (hint) => {
      setSequence(hint.sequence)
      refreshFromRealtimeHint()
    },
    onReset: () => {
      setSequence(0)
      refreshFromRealtimeHint()
    },
  })

  const create = async (input: { queue: string; parent_key: string; title: string }) => {
    try {
      const created = await createTask(input, target)
      setCreatingParent(null)
      if (input.parent_key) {
        setExpanded((current) => new Set(current).add(input.parent_key))
      }
      await loadTree()
      await loadDetail(created.key)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  const saveDetail = async (input: {
    title: string
    description: string
    status?: TaskStatus
    assignee?: string
    manual_block_reason?: string
    priority: TaskPriority
  }) => {
    if (!detail) return
    try {
      const updated = await updateTask(detail.task.key, {
        ...input,
        revision: detail.task.revision,
      }, target)
      setDetail((current) => current ? { ...current, task: updated } : current)
      setTasks((current) => current.map((task) => task.key === updated.key ? updated : task))
      toast.success("Task updated")
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        await loadTree()
        await loadDetail(detail.task.key)
      }
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  const comment = async (body: string, idempotencyKey: string) => {
    if (!detail) return
    try {
      await addTaskComment(detail.task.key, body, target, idempotencyKey)
      await loadDetail(detail.task.key)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  const performMove = async (key: string, parentKey: string, beforeKey: string) => {
    const moving = tasks.find((task) => task.key === key)
    const parent = tasks.find((task) => task.key === parentKey)
    if (!moving) return
    const forest = buildTaskForest(tasks)
    if ((parent && parent.queue !== moving.queue) || (parentKey && !canDropTaskInside(forest, key, parentKey))) {
      toast.error("Tasks can only move inside their queue and outside their own subtree")
      return
    }
    const previous = tasks
    setTasks((current) => current.map((task) => task.key === key ? { ...task, parent_key: parentKey } : task))
    try {
      const updated = await moveTask(key, {
        parent_key: parentKey,
        before_key: beforeKey,
        revision: moving.revision,
      }, target)
      setTasks((current) => current.map((task) => task.key === key ? updated : task))
      if (parentKey) setExpanded((current) => new Set(current).add(parentKey))
      await loadTree()
    } catch (error) {
      setTasks(previous)
      if (error instanceof ApiError && error.status === 409) await loadTree()
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  const createQueue = async (input: CreateQueueInput) => {
    try {
      const created = await createTaskQueue(input, target)
      setQueues((current) => [...current, created])
      setQueue(created.prefix)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  const updateQueue = async (prefix: string, input: Parameters<typeof updateTaskQueue>[1]) => {
    try {
      const updated = await updateTaskQueue(prefix, input, target)
      setQueues((current) => current.map((item) => item.prefix === prefix ? updated : item))
      toast.success("Queue updated")
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) await loadMetadata()
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  const unread = useMemo(
    () => notifications.filter((notification) => !notification.read_at && !notification.dismissed_at).length,
    [notifications],
  )
  const activeQuestionTaskKeys = useMemo(
    () => new Set(notifications
      .filter((notification) => notification.type === "task.question" && !notification.read_at && !notification.dismissed_at)
      .map((notification) => notification.task_key)),
    [notifications],
  )

  return (
    <div
      ref={workspaceRef}
      className="tasks-workspace"
      data-testid="tasks-workspace"
      data-scope-agent={scopeAgent ?? ""}
      style={{
        "--tasks-navigation-width": `${effectiveWidths.navigationWidth}px`,
        "--tasks-detail-width": `${effectiveWidths.detailWidth}px`,
      } as CSSProperties}
    >
      <TasksNavigation
        view={view}
        onView={setView}
        queues={queues}
        queue={queue}
        onQueue={setQueue}
        unread={unread}
      />
      <TaskPanelResizeHandle
        panel="navigation"
        width={effectiveWidths.navigationWidth}
        maximum={navigationMaximum}
        workspaceRef={workspaceRef}
        onResize={setNavigationWidth}
      />
      <main className="tasks-center">
        {(view === "all" || view === "mine" || view === "waiting") && (
          <>
            <header className="tasks-toolbar">
              <div>
                <h2>{view === "all" ? "All tasks" : view === "mine" ? "My tasks" : "Waiting for me"}</h2>
                {scopeAgent && <span>Scoped to {scopeAgent}</span>}
              </div>
              <label className="tasks-search">
                <Search aria-hidden="true" />
                <Input aria-label="Search tasks" placeholder="Filter tasks" value={query} onChange={(event) => setQuery(event.target.value)} />
              </label>
              <label>
                Task status
                <select value={statusView} onChange={(event) => setStatusView(event.target.value as TaskStatusView)}>
                  <option value="active">Active</option>
                  <option value="closed">Closed</option>
                  <option value="all">All</option>
                </select>
              </label>
              <button
                type="button"
                aria-label="Refresh tasks"
                aria-busy={refreshing}
                onClick={() => {
                  if (!scopeAgent && (view === "mine" || view === "waiting") && !principalFilter) void loadMetadata()
                  else void loadTree()
                }}
              >
                <RefreshCw className={refreshing ? "is-refreshing" : undefined} />
              </button>
              <button type="button" className="task-primary-action" onClick={() => setCreatingParent("")}>
                <Plus /> New task
              </button>
            </header>
            {creatingParent === "" && (
              <TaskForm queues={queues} initialQueue={queue} onCreate={create} onCancel={() => setCreatingParent(null)} />
            )}
            {principalMetadataError && !principalFilter
              ? <div className="tasks-empty" role="alert">{principalMetadataError}</div>
              : loading ? <div className="tasks-empty">Loading tasks…</div> : (
              <TaskTree
                tasks={tasks}
                activeQuestionTaskKeys={activeQuestionTaskKeys}
                expanded={expanded}
                selectedKey={selectedKey}
                onToggle={(key) => setExpanded((current) => {
                  const next = new Set(current)
                  if (next.has(key)) next.delete(key)
                  else next.add(key)
                  return next
                })}
                onSelect={(key) => void loadDetail(key)}
                onAddChild={setCreatingParent}
                onMove={(key, parentKey, beforeKey) => void performMove(key, parentKey, beforeKey)}
              />
            )}
            {creatingParent && (
              <TaskForm
                queues={queues}
                initialQueue={tasks.find((task) => task.key === creatingParent)?.queue || queue}
                parentKey={creatingParent}
                onCreate={create}
                onCancel={() => setCreatingParent(null)}
              />
            )}
          </>
        )}
        {view === "notifications" && (
          <TaskNotifications
            notifications={notifications}
            onOpen={(key) => {
              setView("all")
              void loadDetail(key)
            }}
            onRead={async (id) => {
              await markTaskNotificationRead(id, target)
              setNotifications((current) => current.map((item) => item.id === id ? { ...item, read_at: new Date().toISOString() } : item))
              onNotificationsChanged?.()
            }}
            onDismiss={async (id) => {
              await dismissTaskNotification(id, target)
              setNotifications((current) => current.filter((item) => item.id !== id))
              onNotificationsChanged?.()
            }}
          />
        )}
        {view === "queues" && (
          <QueueSettings queues={queues} onCreate={createQueue} onUpdate={updateQueue} target={target} />
        )}
      </main>
      <TaskPanelResizeHandle
        panel="detail"
        width={effectiveWidths.detailWidth}
        maximum={detailMaximum}
        workspaceRef={workspaceRef}
        onResize={setDetailWidth}
      />
      {detail ? (
        <TaskDetail
          key={`${detail.task.key}:${detail.task.revision}`}
          detail={detail}
          principals={principals}
          onClose={() => {
            detailRequestRef.current += 1
            selectedKeyRef.current = ""
            setSelectedKey("")
            setDetail(null)
            setEvents([])
            setWorkflow(null)
            setWorkflowArtifacts([])
            setWorkflowQuestions([])
            setExecutionLoading(false)
            setExecutionError("")
            setArtifactsLoading(false)
            setArtifactsError("")
            setQuestionsLoading(false)
            setQuestionsError("")
          }}
          events={events}
          workflow={workflow}
          workflowArtifacts={workflowArtifacts}
          workflowQuestions={workflowQuestions}
          executionLoading={executionLoading}
          executionError={executionError}
          artifactsLoading={artifactsLoading}
          artifactsError={artifactsError}
          questionsLoading={questionsLoading}
          questionsError={questionsError}
          onSave={saveDetail}
          onComment={comment}
          onAddRelation={async (targetKey: string, type: TaskRelationType) => {
            await addTaskRelation(
              detail.task.key, targetKey, type, detail.task.revision, target, idempotencyKey(),
            )
            await loadDetail(detail.task.key)
          }}
          onDeleteRelation={async (relationID: number) => {
            await deleteTaskRelation(
              detail.task.key, relationID, detail.task.revision, target, idempotencyKey(),
            )
            await loadDetail(detail.task.key)
          }}
        />
      ) : (
        <aside className="task-detail-empty">
          <span>Select a task</span>
          <p>Details, dependencies, questions, and the full conversation stay here.</p>
        </aside>
      )}
    </div>
  )
}

export default function TasksWorkspace(props: TasksWorkspaceProps) {
  const activeId = useOptionalDaemons()?.activeId ?? ""
  const targetKey = props.target === undefined
    ? `active:${activeId || "local"}`
    : props.target === null
      ? "explicit:local"
      : `explicit:${props.target.id}:${props.target.baseURL}:${props.target.token}`
  return <TasksWorkspaceContent key={`${targetKey}:${props.scopeAgent ?? ""}`} {...props} />
}
