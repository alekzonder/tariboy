import { useCallback, useState } from "react"
import type { TaskStatusView } from "@/lib/api"
import type { TaskPrincipalFilter } from "./TaskFilterBar"
import { persistTaskQueueFilter, readTaskQueueFilter } from "./taskFilterStorage"

export function useTasksWorkspaceView() {
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
  // "+ New task" from the handoff: the new row must be visible the moment it
  // lands, so the search that would hide it is cleared and a Closed-only view
  // switches back to Active. The expansion state is deliberately untouched.
  const startCreate = () => {
    setQuery("")
    setStatusView((current) => (current === "closed" ? "active" : current))
    setCreatingParent("")
  }

  const loadDetail = useCallback(async (key: string) => { setSelectedKey(key) }, [])
  const closeDetail = () => setSelectedKey("")
  const toggleExpanded = (key: string) => setExpanded((current) => {
    const next = new Set(current)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    return next
  })
  return {
    view, setView, queuesOpen, setQueuesOpen, statusView, setStatusView, queue, selectQueue,
    query, setQuery, expanded, setExpanded, selectedKey, creatingParent, setCreatingParent,
    startCreate, loadDetail, closeDetail, toggleExpanded,
  }
}

export type TasksWorkspaceView = ReturnType<typeof useTasksWorkspaceView>
