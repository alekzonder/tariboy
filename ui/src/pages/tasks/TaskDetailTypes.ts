import type { ReactNode } from "react"
import type { TaskDetail as Detail, Task, TaskEvent, TaskPrincipals, TaskPriority, TaskRelationType, TaskStatus } from "@/lib/api"

export interface TaskDetailProps {
  detail: Detail
  events: TaskEvent[]
  /**
   * The sequence of the last realtime hint about this task, for an owner
   * whose `events` may not include the newest ones.
   */
  liveSequence?: number
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
  onTransfer: (hostID: string) => Promise<void>
  /** Moves the task to another queue of this server; the owner opens the new task. */
  onRequeue: (queue: string) => Promise<void>
  /** A workflow action changed the task on the daemon; the owner reloads it. */
  onTaskChanged?: () => void
  /** A closed list of assignees instead of the free-text field; the owner of
   *  `onSave` gives each value its meaning. */
  assigneeOptions?: ReadonlyArray<{ value: string; label: string }>
}
