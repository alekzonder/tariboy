import type { ScriptDefinition, ScriptRun } from "@/lib/api";
import { fmtDateTime } from "@/lib/time";

const activeRunStatuses = new Set(["pending", "running"]);

export function isRunActive(status?: string) {
  return activeRunStatuses.has(status ?? "");
}

// scriptsNeedPolling is true while any script or its latest run can still change.
export function scriptsNeedPolling(scripts: ScriptDefinition[]) {
  return scripts.some(
    (definition) => definition.state === "active" || isRunActive(definition.latest_run?.status),
  );
}

// canRerun allows Exec on a completed script or on an idle active schedule.
export function canRerun(definition: ScriptDefinition) {
  if (definition.state === "completed") return true;
  return (
    definition.mode === "every" &&
    definition.state === "active" &&
    !isRunActive(definition.latest_run?.status)
  );
}

// logNeedsRefresh says whether a polled run's open log may have changed.
export function logNeedsRefresh({
  run,
  expanded,
  forceLogs,
  previousStatus,
}: {
  run: ScriptRun;
  expanded: Set<string>;
  forceLogs: boolean;
  previousStatus?: string;
}) {
  if (!expanded.has(run.id)) return false;
  return forceLogs || isRunActive(run.status) || isRunActive(previousStatus);
}

export function toggledSet(current: Set<string>, id: string, open: boolean) {
  const next = new Set(current);
  if (open) next.delete(id);
  else next.add(id);
  return next;
}

export function nextGeneration(generations: Record<string, number>, key: string) {
  const generation = (generations[key] ?? 0) + 1;
  generations[key] = generation;
  return generation;
}

export function when(value?: string) {
  return value ? fmtDateTime(value) : "—";
}

export function runDuration(run: ScriptRun) {
  if (!run.started_at || !run.finished_at) return "—";
  const milliseconds = Date.parse(run.finished_at) - Date.parse(run.started_at);
  return milliseconds >= 0 ? `${(milliseconds / 1000).toFixed(1)}s` : "—";
}
