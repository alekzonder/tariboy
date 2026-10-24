import { useEffect, useMemo } from "react"
import { toast } from "sonner"
import { keepPreviousData, useMutation, useQueries, useQueryClient } from "@tanstack/react-query"
import { useApi } from "@/components/apiContext"
import { useApiQuery } from "@/hooks/useApiQuery"
import { isConnectionError, type Task, type TaskNotification } from "@/lib/api"
import { errorText as errorMessage } from "@/lib/queryClient"
import { taskKeys, taskQueries } from "./taskQueries"
import { useTaskQueryEvents } from "./useTaskQueryEvents"
import type { TasksWorkspaceView } from "./useTasksWorkspaceView"

function actorAgent(name: string): string {
  return name.startsWith("agent:") ? name : `agent:${name}`
}

export function useTasksWorkspaceData(scopeAgent: string | undefined, state: TasksWorkspaceView, onNotificationsChanged?: () => void) {
  const api = useApi()
  const client = useQueryClient()
  const { view, query, statusView, selectedKey, queue } = state
  const queueQuery = useApiQuery(taskQueries.queues(api))
  const peopleQuery = useApiQuery(taskQueries.principals(api))
  const inboxQuery = useApiQuery(taskQueries.notifications(api))
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
  const treeQuery = useApiQuery({ ...taskQueries.list(api, filters), enabled: !waitingForPrincipal, placeholderData: keepPreviousData })
  const detailQuery = useApiQuery(taskQueries.detail(api, selectedKey))
  const historyQuery = useApiQuery(taskQueries.history(api, selectedKey, "latest"))
  const tasks = useMemo(() => treeQuery.data?.tasks ?? [], [treeQuery.data])
  const detail = detailQuery.data ?? null
  const events = historyQuery.data ?? []
  const loadingKey = selectedKey && (!detail || !historyQuery.data) ? selectedKey : ""
  const loading = waitingForPrincipal ? !principalMetadataError : treeQuery.isPending
  const refreshing = treeQuery.isFetching && !!treeQuery.data
  const loadTree = async () => {
    if (!api.ready) return
    await client.cancelQueries({ queryKey: taskKeys.list(api.target, filters) })
    await treeQuery.refetch()
  }
  const loadMetadata = async () => {
    if (!api.ready) return
    await client.cancelQueries({ queryKey: taskKeys.principals(api.target) })
    await Promise.all([queueQuery.refetch(), peopleQuery.refetch(), inboxQuery.refetch()])
  }
  const setTasks = (update: Task[] | ((current: Task[]) => Task[])) => client.setQueryData(taskKeys.list(api.target, filters), (page: { tasks: Task[]; sequence: number } | undefined) =>
    page ? { ...page, tasks: typeof update === "function" ? update(page.tasks) : update } : page)
  useTaskQueryEvents(treeQuery.data?.sequence)
  useEffect(() => {
    const error = treeQuery.error ?? peopleQuery.error ?? detailQuery.error ?? historyQuery.error
    if (error && !isConnectionError(error)) toast.error(errorMessage(error))
  }, [treeQuery.error, peopleQuery.error, detailQuery.error, historyQuery.error])
  const markRead = useMutation({
    mutationFn: async (unread: TaskNotification[]) => {
      await client.cancelQueries({ queryKey: taskKeys.notifications(api.target) })
      const read = new Set<string>()
      for (const notification of unread) {
        try { await api.tasks.markTaskNotificationRead(notification.id); read.add(notification.id) }
        catch { /* Leave the indicator for the next open. */ }
      }
      if (!read.size) return
      await client.cancelQueries({ queryKey: taskKeys.notifications(api.target) })
      client.setQueryData(taskKeys.notifications(api.target), (page: { notifications: TaskNotification[]; count: number } | undefined) => page ? {
        ...page, notifications: page.notifications.map((item) => read.has(item.id) ? { ...item, read_at: new Date().toISOString() } : item),
      } : page)
      // The next socket/manual read reconciles the inbox; no extra cycle is started here.
      await client.invalidateQueries({ queryKey: taskKeys.notifications(api.target), refetchType: "none" })
      onNotificationsChanged?.()
    },
  })
  const markNotifications = markRead.mutate
  useEffect(() => {
    if (!selectedKey || !detail) return
    const unread = notifications.filter((item) => item.task_key === selectedKey && item.type === "task.question" && !item.read_at && !item.dismissed_at)
    if (unread.length) markNotifications(unread)
  }, [selectedKey, detail, notifications, markNotifications])
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
  const questionQueries = useQueries({ queries: missingQuestionKeys.map((key) => ({ ...taskQueries.detail(api, key), enabled: api.ready })) })
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

  return {
    queues, principals, tasks, detail, events, loadingKey, loading, refreshing,
    principalFilter, principalMetadataError, treeQuery, detailQuery, historyQuery,
    loadTree, loadMetadata, setTasks, activeQuestionTaskKeys, visibleTasks, queueCounts,
  }
}

export type TasksWorkspaceData = ReturnType<typeof useTasksWorkspaceData>
