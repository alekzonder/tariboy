import { useCallback, useEffect, useRef, useState } from "react"
import { toast } from "sonner"
import { useTasksSocket } from "@/hooks/useTasksSocket"
import type { ApiTarget } from "@/lib/api"
import { resolveDaemon } from "@/lib/daemons"
import {
  addTaskComment,
  addTaskRelation,
  deleteTaskRelation,
  getTask,
  getTaskWorkflow,
  listTaskEvents,
  listTaskPrincipals,
  listTasks,
  transferTask,
  updateTask,
  type TaskDetail as Detail,
  type TaskEvent,
  type TaskPrincipals,
  type TaskPriority,
  type TaskRelationType,
  type TaskStatus,
  type WorkflowExecutionView,
} from "@/lib/tasks"
import TaskDetail, { TaskDetailLoading } from "./TaskDetail"
import { useTaskPanelWidths } from "./useTaskPanelWidths"
import "./tasks.css"

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `task-action-${Date.now()}-${Math.random()}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

/**
 * One task, opened where it was mentioned. It is the same panel the Tasks
 * workspace shows, but it owns its own load and its own writes, so a chat — or
 * anything else that names a task key — can open it without navigating to
 * Tasks and without borrowing that workspace's tree state.
 */
export default function TaskDrawer({
  taskKey,
  target,
  onClose,
}: {
  taskKey: string
  target?: ApiTarget
  onClose: () => void
}) {
  const { detailWidth } = useTaskPanelWidths()
  const [detail, setDetail] = useState<Detail | null>(null)
  const [events, setEvents] = useState<TaskEvent[]>([])
  const [workflow, setWorkflow] = useState<WorkflowExecutionView | null>(null)
  const [principals, setPrincipals] = useState<TaskPrincipals | null>(null)
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const load = useCallback(async () => {
    try {
      const [next, history] = await Promise.all([
        getTask(taskKey, target),
        listTaskEvents(taskKey, 0, 200, target),
      ])
      if (!mountedRef.current) return
      setDetail(next)
      setEvents(history.events ?? [])
      setWorkflow(null)
      // The execution projection is read for one thing the panel shows: whether
      // the workflow is frozen. A failure leaves that banner off.
      if (next.task.workflow_version_id) {
        await getTaskWorkflow(taskKey, target)
          .then((value) => { if (mountedRef.current) setWorkflow(value) })
          .catch(() => {})
      }
    } catch (error) {
      if (!mountedRef.current) return
      toast.error(errorMessage(error))
      onClose()
    }
  }, [onClose, target, taskKey])

  useEffect(() => { void Promise.resolve().then(load) }, [load])
  useEffect(() => {
    // The assignee picker needs the host's principals; without them the panel
    // still edits everything else.
    listTaskPrincipals(target)
      .then((people) => { if (mountedRef.current) setPrincipals(people) })
      .catch(() => {})
  }, [target])

  // The socket replays everything after the sequence it is given, so it opens
  // only once the current one is known: from zero it would replay the host's
  // whole task log and reload this panel once per event.
  const [sequence, setSequence] = useState(0)
  const [live, setLive] = useState(false)
  useEffect(() => {
    listTasks({ limit: 1 }, target)
      .then((page) => {
        if (!mountedRef.current) return
        setSequence(page.sequence ?? 0)
        setLive(true)
      })
      .catch(() => {})
  }, [target])
  useTasksSocket({
    target,
    after: sequence,
    enabled: live,
    onHint: (hint) => {
      setSequence(hint.sequence)
      // Every task on the host shares this stream; only this one's events
      // change what the panel shows.
      if (!hint.task_key || hint.task_key === taskKey) void load()
    },
    // A reset says the host pruned events this panel may have missed; the
    // reload covers that, and the stream stays where it is.
    onReset: () => { void load() },
  })

  if (detail?.task.key !== taskKey) {
    return <TaskDetailLoading taskKey={taskKey} width={detailWidth} resizeHandle={null} onClose={onClose} />
  }

  return (
    <TaskDetail
      key={detail.task.key}
      target={target}
      detail={detail}
      principals={principals}
      events={events}
      workflow={workflow}
      width={detailWidth}
      resizeHandle={null}
      onClose={onClose}
      onTransfer={async (hostID: string) => {
        const host = await resolveDaemon(hostID)
        if (!host) throw new Error("That server is no longer registered")
        const label = host.label || host.id
        await transferTask(detail.task.key, target, host, label, idempotencyKey())
        toast.success(`${detail.task.key} moved to ${label}`)
        await load()
      }}
      onSave={async (input: {
        revision: number
        title: string
        description: string
        pull_request: string
        status?: TaskStatus
        assignee?: string
        manual_block_reason?: string
        priority: TaskPriority
      }) => {
        try {
          const updated = await updateTask(detail.task.key, input, target)
          setDetail((current) => current ? { ...current, task: updated } : current)
          toast.success("Task updated")
          return updated
        } catch (error) {
          toast.error(errorMessage(error))
          await load()
          throw error
        }
      }}
      onComment={async (body: string, key: string) => {
        try {
          await addTaskComment(detail.task.key, body, target, key)
          await load()
        } catch (error) {
          toast.error(errorMessage(error))
          throw error
        }
      }}
      onAddRelation={async (targetKey: string, type: TaskRelationType) => {
        await addTaskRelation(
          detail.task.key, targetKey, type, detail.task.revision, target, idempotencyKey(),
        )
        await load()
      }}
      onDeleteRelation={async (relationID: number) => {
        await deleteTaskRelation(
          detail.task.key, relationID, detail.task.revision, target, idempotencyKey(),
        )
        await load()
      }}
    />
  )
}
