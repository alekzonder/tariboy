import { useCallback, useEffect, useRef } from "react";
import type { StatusTone } from "@/lib/statusTone";

const KIND_LABELS: Record<string, string> = {
  "image.build": "Image build",
  "workflow.build": "Workflow build",
};

export const kindLabel = (kind: string) => KIND_LABELS[kind] ?? kind;

export function operationTone(status: string): StatusTone {
  if (status === "running") return "live";
  if (status === "failed" || status === "interrupted") return "danger";
  return "quiet";
}

export const formatTime = (value?: string) => (value ? new Date(value).toLocaleString() : "—");

/** Wall time from start to finish; a running operation counts up to now. */
export function formatDuration(start: string, finish?: string, now = Date.now()): string {
  const begin = Date.parse(start);
  const end = finish ? Date.parse(finish) : now;
  if (Number.isNaN(begin) || Number.isNaN(end)) return "—";
  const seconds = Math.max(0, Math.round((end - begin) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

/**
 * Wrap a refresh so calls never overlap: a call while one is in flight marks
 * it stale, and the running refresh runs once more when it ends. `run` must
 * handle its own errors.
 */
export function useCoalesced(run: () => Promise<void>): () => Promise<void> {
  const runRef = useRef(run);
  useEffect(() => {
    runRef.current = run;
  }, [run]);
  const state = useRef({ busy: false, again: false });
  return useCallback(async () => {
    const current = state.current;
    if (current.busy) {
      current.again = true;
      return;
    }
    current.busy = true;
    try {
      do {
        current.again = false;
        await runRef.current();
      } while (current.again);
    } finally {
      current.busy = false;
    }
  }, []);
}
