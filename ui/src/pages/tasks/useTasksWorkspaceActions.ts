import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import { ApiError, type ApiTarget } from "@/lib/api"
import { errorText as errorMessage } from "@/lib/queryClient"
import { buildTaskForest, canDropTaskInside } from "@/lib/taskTree"
import { resolveDaemon } from "@/lib/daemons"
import {
  addTaskComment, addTaskRelation, createTask, createTaskQueue, deleteTaskRelation,
  moveTask, requeueTask, transferTask, updateTaskQueue, updateTask,
  type CreateQueueInput, type TaskPriority, type TaskRelationType, type TaskStatus,
} from "@/lib/tasks"
import { taskKeys, refreshTasks, writeTask, retainTaskRevision } from "./taskQueries"
import { rememberQueue } from "./taskQueueMemory"
import type { TasksWorkspaceView } from "./useTasksWorkspaceView"
import type { TasksWorkspaceData } from "./useTasksWorkspaceData"

function idempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `task-action-${Date.now()}-${Math.random()}`
}

export function useTasksWorkspaceActions(target: Exclude<ApiTarget, undefined>, server: string, state: TasksWorkspaceView, data: TasksWorkspaceData) {
  const client = useQueryClient()
  const { setCreatingParent, setExpanded, loadDetail, selectQueue } = state
  const { tasks, detail, setTasks, loadMetadata } = data
  const write = <T,>(operation: () => Promise<T>) => writeTask(client, target, operation)
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
      toast.success("Task updated")
      return retainTaskRevision(client, target, updated)
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
      retainTaskRevision(client, target, updated)
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

  const addRelation = async (targetKey: string, type: TaskRelationType) => {
    if (!detail) return
    await write(() => addTaskRelation(detail.task.key, targetKey, type, detail.task.revision, target, idempotencyKey()))
  }
  const deleteRelation = async (relationID: number) => {
    if (!detail) return
    await write(() => deleteTaskRelation(detail.task.key, relationID, detail.task.revision, target, idempotencyKey()))
  }
  return { create, saveDetail, comment, performMove, changePriority, transferDetail, requeueDetail, createQueue, updateQueue, addRelation, deleteRelation }
}

export type TasksWorkspaceActions = ReturnType<typeof useTasksWorkspaceActions>
