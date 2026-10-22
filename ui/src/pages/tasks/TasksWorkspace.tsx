import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { toast } from "sonner"
import { useOptionalDaemons } from "@/components/DaemonProvider"
import { keepPreviousData, useMutation, useQueries, useQueryClient } from "@tanstack/react-query"
import { useApiQuery, useApiTarget } from "@/hooks/useApiQuery"
import { errorText as errorMessage } from "@/lib/queryClient"
import { taskKeys, taskQueries, refreshTasks, writeTask } from "./taskQueries"
import { useTaskQueryEvents } from "./useTaskQueryEvents"
import { ApiError, isConnectionError, targetReady, type ApiTarget } from "@/lib/api"
import { buildTaskForest, canDropTaskInside } from "@/lib/taskTree"
import { resolveDaemon } from "@/lib/daemons"
import { hostToParam } from "@/lib/terminalsHost"
import {
  addTaskComment,
  addTaskRelation,
  createTask,
  createTaskQueue,
  deleteTaskRelation,
  markTaskNotificationRead,
  moveTask,
  requeueTask,
  transferTask,
  updateTaskQueue,
  updateTask,
  type CreateQueueInput,
  type Task,
  type TaskDetail as Detail,
  type TaskPriority,
  type TaskNotification,
  type TaskRelationType,
  type TaskStatus,
  type TaskStatusView,
} from "@/lib/tasks"
import QueueSettingsDialog from "./QueueSettingsDialog"
import { TaskFilterBar, type TaskPrincipalFilter } from "./TaskFilterBar"
import TaskDetail, { TaskDetailLoading } from "./TaskDetail"
import TaskForm from "./TaskForm"
import TaskTree from "./TaskTree"
import TaskPanelResizeHandle from "./TaskPanelResizeHandle"
import { persistTaskQueueFilter, readTaskQueueFilter } from "./taskFilterStorage"
import { readRememberedQueue, rememberQueue } from "./taskQueueMemory"
import { MIN_TASK_DETAIL_WIDTH, useTaskPanelWidths } from "./useTaskPanelWidths"
import "./tasks.css"

function actorAgent(name: string): string {
  return name.startsWith("agent:") ? name : `agent:${name}`
}

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `task-action-${Date.now()}-${Math.random()}`
}

type TasksWorkspaceProps = {
  scopeAgent?: string
  target?: ApiTarget
  initialTaskKey?: string
  onNotificationsChanged?: () => void
}

function TasksWorkspaceContent({
  scopeAgent,
  target: inputTarget,
  initialTaskKey,
  onNotificationsChanged,
  server,
}: TasksWorkspaceProps & { server: string }) {
  const target = useApiTarget(inputTarget)
  const client = useQueryClient()
  const workspaceRef = useRef<HTMLDivElement | null>(null)
  const { detailWidth, setDetailWidth } = useTaskPanelWidths()
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
  const detailMaximum = workspaceWidth > 0
    ? Math.max(MIN_TASK_DETAIL_WIDTH, workspaceWidth - 80)
    : Math.max(MIN_TASK_DETAIL_WIDTH, (globalThis.innerWidth || 820) - 80)
  const effectiveDetailWidth = Math.min(detailWidth, detailMaximum)
  const [view, setView] = useState<TaskPrincipalFilter>("")
  const [queuesOpen, setQueuesOpen] = useState(false)
  const [statusView, setStatusView] = useState<TaskStatusView>("active")
  const [queue, setQueue] = useState(readTaskQueueFilter)
  const [query, setQuery] = useState("")
  const selectQueue = useCallback((prefix: string) => {
    setQueue(prefix)
    persistTaskQueueFilter(prefix)
  }, [])
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [selectedKey, setSelectedKey] = useState("")
  const [creatingParent, setCreatingParent] = useState<string | null>(null)
  const queueQuery = useApiQuery(taskQueries.queues(target), target)
  const peopleQuery = useApiQuery(taskQueries.principals(target), target)
  const inboxQuery = useApiQuery(taskQueries.notifications(target), target)
  const queues = queueQuery.data?.queues ?? []
  const principals = peopleQuery.data ?? null
  const notifications = useMemo(() => inboxQuery.data?.notifications ?? [], [inboxQuery.data])
  const principalFilter = !scopeAgent && view !== "" ? principals?.customer : undefined
  const principalMetadataError = !scopeAgent && view !== "" && peopleQuery.error ? errorMessage(peopleQuery.error) : ""
  const waitingForPrincipal = !scopeAgent && view !== "" && !principalFilter
  const filters = {
    text: query, scope_agent: scopeAgent, status_view: statusView,
    ...(view === "mine" ? { assignee: scopeAgent ? actorAgent(scopeAgent) : principalFilter } : {}),
    ...(view === "waiting" ? { waiting_for: scopeAgent ? actorAgent(scopeAgent) : principalFilter } : {}),
  }
  const treeQuery = useApiQuery({ ...taskQueries.list(target, filters), enabled: !waitingForPrincipal, placeholderData: keepPreviousData }, target)
  const detailQuery = useApiQuery(taskQueries.detail(target, selectedKey), target)
  const historyQuery = useApiQuery(taskQueries.history(target, selectedKey, "latest"), target)
  const tasks = useMemo(() => treeQuery.data?.tasks ?? [], [treeQuery.data])
  const detail = detailQuery.data ?? null
  const events = historyQuery.data ?? []
  const loadingKey = selectedKey && (!detail || !historyQuery.data) ? selectedKey : ""
  const loading = waitingForPrincipal ? !principalMetadataError : treeQuery.isPending
  const refreshing = treeQuery.isFetching && !!treeQuery.data
  const loadTree = async () => {
    if (!targetReady(target)) return
    await client.cancelQueries({ queryKey: taskKeys.list(target, filters) })
    await treeQuery.refetch()
  }
  const loadMetadata = async () => {
    if (!targetReady(target)) return
    await client.cancelQueries({ queryKey: taskKeys.principals(target) })
    await Promise.all([queueQuery.refetch(), peopleQuery.refetch(), inboxQuery.refetch()])
  }
  const loadDetail = useCallback(async (key: string) => { setSelectedKey(key) }, [])
  const setTasks = (update: Task[] | ((current: Task[]) => Task[])) => client.setQueryData(taskKeys.list(target, filters), (page: { tasks: Task[]; sequence: number } | undefined) =>
    page ? { ...page, tasks: typeof update === "function" ? update(page.tasks) : update } : page)
  const setDetail = (update: (current: Detail | null) => Detail | null) => client.setQueryData(taskKeys.detail(target, selectedKey), (current: Detail | undefined) => update(current ?? null) ?? undefined)
  const write = <T,>(operation: () => Promise<T>) => writeTask(client, target, operation)
  useTaskQueryEvents(target, treeQuery.data?.sequence)
  useEffect(() => {
    const error = treeQuery.error ?? peopleQuery.error ?? detailQuery.error ?? historyQuery.error
    if (error && !isConnectionError(error)) toast.error(errorMessage(error))
  }, [treeQuery.error, peopleQuery.error, detailQuery.error, historyQuery.error])
  const markRead = useMutation({
    mutationFn: async (unread: TaskNotification[]) => {
      await client.cancelQueries({ queryKey: taskKeys.notifications(target) })
      const read = new Set<string>()
      for (const notification of unread) {
        try { await markTaskNotificationRead(notification.id, target); read.add(notification.id) }
        catch { /* Leave the indicator for the next open. */ }
      }
      if (!read.size) return
      client.setQueryData(taskKeys.notifications(target), (page: { notifications: TaskNotification[]; count: number } | undefined) => page ? {
        ...page, notifications: page.notifications.map((item) => read.has(item.id) ? { ...item, read_at: new Date().toISOString() } : item),
      } : page)
      onNotificationsChanged?.()
    },
  })
  const markNotifications = markRead.mutate
  useEffect(() => {
    if (!selectedKey || !detail) return
    const unread = notifications.filter((item) => item.task_key === selectedKey && item.type === "task.question" && !item.read_at && !item.dismissed_at)
    if (unread.length) markNotifications(unread)
  }, [selectedKey, detail, notifications, markNotifications])
  const openedInitialKeyRef = useRef("")
  useEffect(() => {
    if (!initialTaskKey || openedInitialKeyRef.current === initialTaskKey) return
    openedInitialKeyRef.current = initialTaskKey
    void loadDetail(initialTaskKey)
  }, [initialTaskKey, loadDetail])

  const create = async (input: { queue: string; parent_key: string; title: string }) => {
    try {
      const created = await write(() => createTask(input, target))
      if (!input.parent_key) rememberQueue(server, input.queue)
      setCreatingParent(null)
      if (input.parent_key) {
        setExpanded((current) => new Set(current).add(input.parent_key))
      }
      await loadDetail(created.key)
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  const saveDetail = async (input: {
    revision: number
    title: string
    description: string
    pull_request: string
    status?: TaskStatus
    assignee?: string
    manual_block_reason?: string
    priority: TaskPriority
  }) => {
    if (!detail) throw new Error("Task is no longer selected")
    try {
      const updated = await write(() => updateTask(detail.task.key, { ...input }, target))
      setDetail((current) => current ? { ...current, task: updated } : current)
      setTasks((current) => current.map((task) => task.key === updated.key ? updated : task))
      toast.success("Task updated")
      return updated
    } catch (error) {
      toast.error(errorMessage(error))
      throw error
    }
  }

  const comment = async (body: string, idempotencyKey: string) => {
    if (!detail) return
    try {
      await write(() => addTaskComment(detail.task.key, body, target, idempotencyKey))
    } catch (error) {
      toast.error(errorMessage(error))
      throw error
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
    await client.cancelQueries({ queryKey: taskKeys.server(target) })
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
    } catch (error) {
      setTasks(previous)
      toast.error(errorMessage(error))
    } finally {
      await refreshTasks(client, target)
    }
  }

  const changePriority = async (key: string, priority: TaskPriority) => {
    const changing = tasks.find((task) => task.key === key)
    if (!changing) return
    try {
      const updated = await write(() => updateTask(key, { priority, revision: changing.revision }, target))
      setTasks((current) => current.map((task) => task.key === key ? updated : task))
      setDetail((current) => current?.task.key === key ? { ...current, task: updated } : current)
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  const transferDetail = async (hostID: string) => {
    if (!detail) throw new Error("Task is no longer selected")
    const key = detail.task.key
    const host = await resolveDaemon(hostID)
    if (!host) throw new Error("That server is no longer registered")
    const label = host.label || host.id
    await write(() => transferTask(key, target, host, label, idempotencyKey()))
    await refreshTasks(client, host)
    toast.success(`${key} moved to ${label}`)
  }

  const requeueDetail = async (queue: string) => {
    if (!detail) throw new Error("Task is no longer selected")
    const moved = await write(() => requeueTask(detail.task.key, queue, detail.task.revision, target))
    toast.success(`${detail.task.key} moved to ${queue} as ${moved.key}`)
    await loadDetail(moved.key)
  }

  const createQueue = async (input: CreateQueueInput) => {
    try {
      const created = await write(() => createTaskQueue(input, target))
      selectQueue(created.prefix)
    } catch (error) {
      toast.error(errorMessage(error))
    }
  }

  const updateQueue = async (prefix: string, input: Parameters<typeof updateTaskQueue>[1]) => {
    try {
      await write(() => updateTaskQueue(prefix, input, target))
      toast.success("Queue updated")
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) await loadMetadata()
      toast.error(errorMessage(error))
    }
  }

  // Which shape the table takes: one agent's tasks, or a whole server's.
  const mode = scopeAgent ? "agent" : "all"

  // "+ New task" from the handoff: the new row must be visible the moment it
  // lands, so the search that would hide it is cleared and a Closed-only view
  // switches back to Active. The expansion state is deliberately untouched.
  const startCreate = () => {
    setQuery("")
    setStatusView((current) => (current === "closed" ? "active" : current))
    setCreatingParent("")
  }

  const activeQuestionTaskKeys = useMemo(
    () => new Set(notifications
      .filter((notification) => notification.type === "task.question" && !notification.read_at && !notification.dismissed_at)
      .map((notification) => notification.task_key)),
    [notifications],
  )

  // An unread question stays until its task is opened, but the Active list
  // leaves out closed tasks, so their questions would count in the badge with
  // no row to open. Such tasks are read one by one and kept in the Active list
  // until their question is read.
  const missingQuestionKeys = useMemo(() => {
    if (statusView !== "active" || query || view !== "") return []
    const listed = new Set(tasks.map((task) => task.key))
    return [...new Set(notifications
      .filter((notification) => notification.type === "task.question" && !notification.read_at && !notification.dismissed_at
        && (!scopeAgent || notification.requesting_principal === actorAgent(scopeAgent)))
      .map((notification) => notification.task_key))]
      .filter((key) => !listed.has(key))
      .sort()
  }, [notifications, query, scopeAgent, statusView, tasks, view])
  const questionQueries = useQueries({ queries: missingQuestionKeys.map((key) => ({ ...taskQueries.detail(target, key), enabled: targetReady(target) })) })
  const questionTasks = questionQueries.flatMap((result) => result.data ? [result.data.task] : [])
  const listed = new Set(tasks.map((task) => task.key))
  const listedTasks = [...tasks, ...questionTasks.filter((task) => !listed.has(task.key))]
  if (detail && !listedTasks.some((task) => task.key === detail.task.key) && statusView === "active" && !query && !view) listedTasks.push(detail.task)

  // The queue filter is applied here rather than in `listTasks`, so the queue
  // menu can count the queues from the same load instead of asking again.
  const visibleTasks = queue ? listedTasks.filter((task) => task.queue === queue) : listedTasks
  const queueCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const task of tasks) counts.set(task.queue, (counts.get(task.queue) ?? 0) + 1)
    return counts
  }, [tasks])

  const closeDetail = () => setSelectedKey("")
  const detailResizeHandle = <TaskPanelResizeHandle
    width={effectiveDetailWidth}
    maximum={detailMaximum}
    workspaceRef={workspaceRef}
    onResize={setDetailWidth}
  />

  return (
    <div
      ref={workspaceRef}
      className="tasks-workspace"
      data-testid="tasks-workspace"
      data-scope-agent={scopeAgent ?? ""}
    >
      <main className="tasks-center">
        <TaskFilterBar
          mode={mode}
          query={query}
          onQuery={setQuery}
          statusView={statusView}
          onStatusView={setStatusView}
          principalFilter={view}
          onPrincipalFilter={setView}
          queues={queues}
          queue={queue}
          onQueue={selectQueue}
          queueCounts={queueCounts}
          onManageQueues={() => setQueuesOpen(true)}
          scopeAgent={scopeAgent}
          refreshing={refreshing}
          onRefresh={() => {
            if (!scopeAgent && view !== "" && !principalFilter) void loadMetadata()
            else void loadTree()
          }}
          onCreate={startCreate}
        />
        {creatingParent === "" && (
          <TaskForm
            queues={queues}
            // A toolbar filter is the explicit context; otherwise the queue of the last task made here.
            initialQueue={queue || queues.find((item) => item.prefix === readRememberedQueue(server))?.prefix || ""}
            onCreate={create}
            onCancel={() => setCreatingParent(null)}
          />
        )}
        {principalMetadataError && !principalFilter
          ? <div className="tasks-empty" role="alert">{principalMetadataError}</div>
          : treeQuery.error && !treeQuery.data ? <div className="tasks-empty" role="alert">{errorMessage(treeQuery.error)}</div>
          : loading ? <div className="tasks-empty">Loading tasks…</div> : (
          <TaskTree
            tasks={visibleTasks}
            mode={mode}
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
            onPriorityChange={changePriority}
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
      </main>
      <QueueSettingsDialog open={queuesOpen} onOpenChange={setQueuesOpen} queues={queues}
        onCreate={createQueue} onUpdate={updateQueue} target={target} />
      {loadingKey ? (
        <TaskDetailLoading
          taskKey={loadingKey}
          error={detailQuery.error || historyQuery.error ? errorMessage(detailQuery.error ?? historyQuery.error) : undefined}
          width={effectiveDetailWidth}
          resizeHandle={detailResizeHandle}
          onClose={closeDetail}
        />
      ) : detail && (
        <TaskDetail
          key={detail.task.key}
          target={target}
          detail={detail}
          principals={principals}
          width={effectiveDetailWidth}
          resizeHandle={detailResizeHandle}
          onClose={closeDetail}
          events={events}
          onSave={saveDetail}
          onComment={comment}
          onAddRelation={async (targetKey: string, type: TaskRelationType) => {
            await write(() => addTaskRelation(
              detail.task.key, targetKey, type, detail.task.revision, target, idempotencyKey(),
            ))
          }}
          onTransfer={transferDetail}
          onRequeue={requeueDetail}
          // The row shows the status and category too: reload the tree so the
          // list and the drawer agree before the socket reports the change.
          onTaskChanged={() => { void refreshTasks(client, target) }}
          onDeleteRelation={async (relationID: number) => {
            await write(() => deleteTaskRelation(
              detail.task.key, relationID, detail.task.revision, target, idempotencyKey(),
            ))
          }}
        />
      )}
    </div>
  )
}

export default function TasksWorkspace(props: TasksWorkspaceProps) {
  const activeId = useOptionalDaemons()?.activeId ?? ""
  const server = props.target === undefined
    ? hostToParam(activeId)
    : props.target === null
      ? hostToParam("")
      // Not the endpoint: a reconnect moves it and must keep the open state.
      : props.target.id
  const targetKey = `${props.target === undefined ? "active" : "explicit"}:${server}`
  return <TasksWorkspaceContent key={`${targetKey}:${props.scopeAgent ?? ""}`} {...props} server={server} />
}
