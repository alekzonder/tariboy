import { useCallback, useState } from "react";
import type { TaskStatusView } from "@/lib/api";
import { persistTaskQueueFilter, readTaskQueueFilter } from "./taskFilterStorage";
import type { TaskAssigneeFilter } from "./taskListScope";

export function useTasksWorkspaceView() {
  const [assignee, setAssignee] = useState<TaskAssigneeFilter>("");
  const [waiting, setWaiting] = useState(false);
  const [queuesOpen, setQueuesOpen] = useState(false);
  const [statusView, setStatusView] = useState<TaskStatusView>("active");
  const [queue, setQueue] = useState(readTaskQueueFilter);
  const [query, setQuery] = useState("");
  const selectQueue = useCallback((prefix: string) => {
    setQueue(prefix);
    persistTaskQueueFilter(prefix);
  }, []);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selectedKey, setSelectedKey] = useState("");
  const [creatingParent, setCreatingParent] = useState<string | null>(null);
  // "+ New task" from the handoff: the new row must be visible the moment it
  // lands, so the search that would hide it is cleared and a Done-only view
  // switches back to Active. The expansion state is deliberately untouched.
  const startCreate = () => {
    setQuery("");
    setStatusView((current) => (current === "closed" ? "active" : current));
    setCreatingParent("");
  };

  const loadDetail = useCallback(async (key: string) => {
    setSelectedKey(key);
  }, []);
  const closeDetail = () => setSelectedKey("");
  const toggleExpanded = (key: string) =>
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  return {
    assignee,
    setAssignee,
    waiting,
    setWaiting,
    queuesOpen,
    setQueuesOpen,
    statusView,
    setStatusView,
    queue,
    selectQueue,
    query,
    setQuery,
    expanded,
    setExpanded,
    selectedKey,
    creatingParent,
    setCreatingParent,
    startCreate,
    loadDetail,
    closeDetail,
    toggleExpanded,
  };
}

export type TasksWorkspaceView = ReturnType<typeof useTasksWorkspaceView>;
