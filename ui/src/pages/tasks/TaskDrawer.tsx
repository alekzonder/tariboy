import { useEffect } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { useApiQuery, useApiTarget } from "@/hooks/useApiQuery"
import { errorText as errorMessage, isAccessError } from "@/lib/queryClient"
import { taskQueries, refreshTasks, writeTask, retainTaskRevision } from "./taskQueries"
import { useTaskQueryEvents } from "./useTaskQueryEvents"
import { toast } from "sonner"
import { isConnectionError, type ApiTarget } from "@/lib/api"
import { resolveDaemon } from "@/lib/daemons"
import { targetFor } from "@/lib/terminalsHost"
import {
  addTaskComment,
  addTaskRelation,
  deleteTaskRelation,
  exportTask,
  getTask,
  importTask,
  requeueTask,
  transferTask,
  updateTask,
  type Task,
  type TaskPriority,
  type TaskRelationType,
  type TaskStatus,
} from "@/lib/tasks"
import TaskDetail, { TaskDetailLoading } from "./TaskDetail"
import TaskPanelResizeHandle from "./TaskPanelResizeHandle"
import { clampTaskDetailWidth, useTaskPanelWidths } from "./useTaskPanelWidths"
import "./tasks.css"

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `task-action-${Date.now()}-${Math.random()}`
}

type TaskEdit = {
  revision: number
  title: string
  description: string
  pull_request: string
  status?: TaskStatus
  assignee?: string
  manual_block_reason?: string
  priority: TaskPriority
}

/** An assignee the drawer may offer. `hostId` is set when the agent lives on
 *  another server: picking it moves the task there first. */
export interface AssigneeChoice {
  value: string
  label: string
  assignee: string
  hostId?: string
  hostLabel?: string
}

/**
 * One task, opened where it was mentioned. It is the same panel the Tasks
 * workspace shows, with shared server queries and independent selection.
 */
function TaskDrawerContent({
  taskKey,
  target: inputTarget,
  onClose,
  assignees,
  onMoved,
}: {
  taskKey: string
  target?: ApiTarget
  onClose: () => void
  assignees?: readonly AssigneeChoice[]
  /** The task now lives on `hostId`; the owner reopens it there. */
  onMoved?: (hostId: string) => void
}) {
  const target = useApiTarget(inputTarget)
  const client = useQueryClient()
  // The same stored width and grip as the Tasks tab, so the panel is one size
  // wherever it opens; the drawer spans the window rather than a workspace.
  const { detailWidth, setDetailWidth } = useTaskPanelWidths()
  const resizeHandle = <TaskPanelResizeHandle
    width={detailWidth}
    maximum={clampTaskDetailWidth(Infinity)}
    onResize={setDetailWidth}
  />
  const detailQuery = useApiQuery(taskQueries.detail(target, taskKey), target)
  const historyQuery = useApiQuery(taskQueries.history(target, taskKey, "first"), target)
  const peopleQuery = useApiQuery(taskQueries.principals(target), target)
  const sequenceQuery = useApiQuery(taskQueries.sequence(target), target)
  const detail = detailQuery.data
  const events = historyQuery.data ?? []
  const principals = peopleQuery.data ?? null
  useTaskQueryEvents(target, sequenceQuery.data?.sequence, taskKey)
  const write = <T,>(operation: () => Promise<T>) => writeTask(client, target, operation)
  useEffect(() => {
    const error = detailQuery.error ?? historyQuery.error
    if (!error || isConnectionError(error)) return
    toast.error(errorMessage(error))
    if (!detailQuery.data && !isAccessError(error)) onClose()
  }, [detailQuery.error, detailQuery.data, historyQuery.error, onClose])

  // An agent on another server can only work a task that lives there. Other
  // edits are saved here first; then the tree is exported and imported with
  // its root unassigned, so no same-named agent over there takes it before
  // the real assignment, which is an ordinary update and notifies the agent.
  // Once the import succeeds the task lives there: a later failure is
  // reported, and the sheet follows the task all the same.
  const moveToAgent = async (task: Task, input: TaskEdit, remote: AssigneeChoice): Promise<Task> => {
    const key = task.key
    const edited = (["title", "description", "pull_request", "status", "priority", "manual_block_reason"] as const)
      .some((field) => input[field] !== undefined && input[field] !== (task[field] ?? ""))
    if (edited) await updateTask(key, { ...input, assignee: task.assignee }, target)
    const hostId = remote.hostId!
    const destination = targetFor(hostId)
    const label = remote.hostLabel || hostId || "local"
    const bundle = await exportTask(key, target)
    const tasks = (bundle.tasks as Array<{ key: string; assignee: string }>)
      .map((item) => item.key === bundle.root_key ? { ...item, assignee: "" } : item)
    const imported = await importTask({ ...bundle, tasks }, destination)
    let result = imported
    try {
      await addTaskComment(key, `Moved to ${label} as ${key}.`, target, idempotencyKey())
      const left = await getTask(key, target)
      await updateTask(key, { status: "cancelled", revision: left.task.revision }, target)
      const arrived = await getTask(key, destination)
      result = await updateTask(key, { assignee: remote.assignee, revision: arrived.task.revision }, destination)
      toast.success(`${key} moved to ${label}`)
    } catch (error) {
      toast.error(`${key} moved to ${label}, but the move did not finish: ${errorMessage(error)}`)
    }
    await refreshTasks(client, destination)
    onMoved?.(hostId)
    return result
  }

  if (detail?.task.key !== taskKey || !historyQuery.data) {
    return <TaskDetailLoading taskKey={taskKey} width={detailWidth} resizeHandle={resizeHandle} onClose={onClose}
      error={detailQuery.error || historyQuery.error ? errorMessage(detailQuery.error ?? historyQuery.error) : undefined} />
  }

  return (
    <TaskDetail
      key={detail.task.key}
      target={target}
      detail={detail}
      principals={principals}
      events={events}
      width={detailWidth}
      resizeHandle={resizeHandle}
      onClose={onClose}
      assigneeOptions={assignees}
      onTransfer={async (hostID: string) => {
        const host = await resolveDaemon(hostID)
        if (!host) throw new Error("That server is no longer registered")
        const label = host.label || host.id
        await write(() => transferTask(detail.task.key, target, host, label, idempotencyKey()))
        await refreshTasks(client, host)
        toast.success(`${detail.task.key} moved to ${label}`)
      }}
      onRequeue={async (queue: string) => {
        // The drawer shows the key its owner chose, so it stays on the
        // cancelled task, whose comment links the new one.
        const moved = await write(() => requeueTask(detail.task.key, queue, detail.task.revision, target))
        toast.success(`${detail.task.key} moved to ${queue} as ${moved.key}`)
      }}
      onSave={async (input: TaskEdit) => {
        const task = detail.task
        const remote = assignees?.find((choice) => choice.hostId !== undefined && choice.value === input.assignee)
        // A closed list can lose an entry between the pick and Save (its server
        // stopped answering). Its value is not a real assignee, so never write it.
        if (assignees && !remote && input.assignee && input.assignee !== task.assignee
          && input.assignee !== principals?.customer && !assignees.some((choice) => choice.value === input.assignee)) {
          const message = "That agent's server is unavailable; pick the assignee again"
          toast.error(message)
          throw new Error(message)
        }
        try {
          if (remote?.hostId !== undefined) {
            return await write(() => moveToAgent(task, input, remote))
          }
          const updated = await write(() => updateTask(detail.task.key, input, target))
          toast.success("Task updated")
          return retainTaskRevision(client, target, updated)
        } catch (error) {
          toast.error(errorMessage(error))
          throw error
        }
      }}
      onComment={async (body: string, key: string) => {
        try {
          await write(() => addTaskComment(detail.task.key, body, target, key))
        } catch (error) {
          toast.error(errorMessage(error))
          throw error
        }
      }}
      onAddRelation={async (targetKey: string, type: TaskRelationType) => {
        await write(() => addTaskRelation(
          detail.task.key, targetKey, type, detail.task.revision, target, idempotencyKey(),
        ))
      }}
      onDeleteRelation={async (relationID: number) => {
        await write(() => deleteTaskRelation(
          detail.task.key, relationID, detail.task.revision, target, idempotencyKey(),
        ))
      }}
    />
  )
}

export default function TaskDrawer(props: Parameters<typeof TaskDrawerContent>[0]) {
  const target = useApiTarget(props.target)
  return <TaskDrawerContent key={JSON.stringify([target?.id ?? "local", props.taskKey])} {...props} target={target} />
}
