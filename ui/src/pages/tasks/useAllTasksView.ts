import { useState } from "react";
import type { TaskStatusView } from "@/lib/api";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { readTaskQueueFilter, persistTaskQueueFilter } from "./taskFilterStorage";

// Search, status and server survive the Agents <-> All tasks switch the same
// way the queue filter does: per session, beside it.
type ViewFilters = { query: string; statusView: TaskStatusView; server: string };
const DEFAULT_VIEW: ViewFilters = { query: "", statusView: "active", server: "" };

function readView(): ViewFilters {
  const stored = readKey(storageKeys.allTasksView);
  return stored && typeof stored === "object"
    ? { ...DEFAULT_VIEW, ...(stored as Partial<ViewFilters>) }
    : DEFAULT_VIEW;
}

function persistView(view: ViewFilters): void {
  // Web Storage is a best-effort Desktop convenience.
  writeKey(storageKeys.allTasksView, view);
}

/** The every-server table's filters, kept for the session. */
export function useAllTasksView() {
  const [view, setViewState] = useState(readView);
  const setView = (patch: Partial<ViewFilters>) =>
    setViewState((current) => {
      const next = { ...current, ...patch };
      persistView(next);
      return next;
    });
  const [queue, setQueueState] = useState(readTaskQueueFilter);
  const setQueue = (prefix: string) => {
    setQueueState(prefix);
    persistTaskQueueFilter(prefix);
  };
  const filtersActive = Boolean(view.query || view.statusView !== "active" || view.server || queue);
  const clearFilters = () => {
    setView(DEFAULT_VIEW);
    setQueue("");
  };
  return { view, setView, queue, setQueue, filtersActive, clearFilters };
}
