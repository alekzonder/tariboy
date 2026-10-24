import { useState } from "react"
import type { Task, TaskStatus, TaskPriority } from "@/lib/api"
import { isWorkflowTask } from "@/lib/tasks"
import type { TaskDetailProps } from "./TaskDetailTypes"

export function useTaskDetailDraft(task: Task, onSave: TaskDetailProps["onSave"]) {
  const workflow = isWorkflowTask(task)
  const [baseline, setBaseline] = useState(task)
  const [title, setTitle] = useState(task.title)
  const [description, setDescription] = useState(task.description)
  const [status, setStatus] = useState<TaskStatus>(task.category)
  const [pullRequest, setPullRequest] = useState(task.pull_request ?? "")
  const [priority, setPriority] = useState<TaskPriority>(task.priority)
  const [assignee, setAssignee] = useState(task.assignee)
  const [blockReason, setBlockReason] = useState(task.manual_block_reason)
  const [saving, setSaving] = useState(false)
  const dirty = title !== baseline.title || description !== baseline.description
    || status !== baseline.category || pullRequest !== (baseline.pull_request ?? "")
    || priority !== baseline.priority || assignee !== baseline.assignee
    || blockReason !== baseline.manual_block_reason
  const adopt = (next: Task) => {
    setBaseline(next)
    setTitle(next.title)
    setDescription(next.description)
    setStatus(next.category)
    setPullRequest(next.pull_request ?? "")
    setPriority(next.priority)
    setAssignee(next.assignee)
    setBlockReason(next.manual_block_reason)
  }
  // Refresh pristine forms; keep the revision that an unsaved draft was based on.
  if (baseline !== task && !dirty && !saving) adopt(task)
  const keepEdits = () => {
    const merged = { ...task,
      title: title !== baseline.title ? title : task.title,
      description: description !== baseline.description ? description : task.description,
      status: status !== baseline.category ? status : task.status,
      category: status !== baseline.category ? status : task.category,
      pull_request: pullRequest !== (baseline.pull_request ?? "") ? pullRequest : task.pull_request,
      priority: priority !== baseline.priority ? priority : task.priority,
      assignee: assignee !== baseline.assignee ? assignee : task.assignee,
      manual_block_reason: blockReason !== baseline.manual_block_reason ? blockReason : task.manual_block_reason,
    }
    adopt(merged)
    setBaseline(task)
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
        // A workflow owns its status: the panel changes it, Save never does.
        status: workflow ? undefined : status,
        // The workflow assigns its task: Save never sends an assignee for it.
        assignee: workflow ? undefined : assignee.trim(),
        manual_block_reason: blockReason,
      })
      adopt(updated)
    } catch {
      // The workspace reports the API error; keep the draft for retry.
    } finally {
      setSaving(false)
    }
  }

  return {
    baseline, title, setTitle, description, setDescription, status, setStatus,
    pullRequest, setPullRequest, priority, setPriority, assignee, setAssignee,
    blockReason, setBlockReason, dirty, saving, adopt, keepEdits, save,
  }
}
