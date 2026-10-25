import { Button } from "@/components/ui/button";
import type { ScriptRun } from "@/lib/api";
import { isRunActive, runDuration, when } from "./scriptsModel";

type RunActions = {
  onToggle: (run: ScriptRun) => Promise<void>;
  onCancel: (id: string) => Promise<void>;
  onCopy: (path: string) => Promise<void>;
  onDownload: (run: ScriptRun) => Promise<void>;
};

function RunDetails({
  run,
  log,
  onCopy,
  onDownload,
}: { run: ScriptRun; log?: string } & RunActions) {
  return (
    <div id={`run-${run.id}`} className="grid gap-3 border-t p-3">
      <dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 text-xs">
        <dt>Status</dt>
        <dd>
          {run.status}
          {run.cancel_requested ? " (cancelling)" : ""}
        </dd>
        <dt>Exit code</dt>
        <dd>{run.exit_code ?? "—"}</dd>
        <dt>Queued</dt>
        <dd>{when(run.created_at)}</dd>
        <dt>Started</dt>
        <dd>{when(run.started_at)}</dd>
        <dt>Finished</dt>
        <dd>{when(run.finished_at)}</dd>
        <dt>Duration</dt>
        <dd>{runDuration(run)}</dd>
        <dt>Log path</dt>
        <dd className="break-all font-mono">{run.log_path || "—"}</dd>
      </dl>
      <div className="flex gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={!run.log_path}
          onClick={() => void onCopy(run.log_path ?? "")}
        >
          Copy path
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={!run.log_path}
          onClick={() => void onDownload(run)}
        >
          Download log
        </Button>
      </div>
      <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded bg-muted p-3 text-xs">
        {log ?? "Loading log…"}
      </pre>
    </div>
  );
}

function RunItem({
  run,
  open,
  log,
  actions,
}: {
  run: ScriptRun;
  open: boolean;
  log?: string;
  actions: RunActions;
}) {
  return (
    <div className="rounded border bg-background">
      <div className="flex flex-wrap items-center gap-2 p-2">
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={open}
          aria-controls={`run-${run.id}`}
          onClick={() => void actions.onToggle(run)}
        >
          {open ? "▾" : "▸"} {run.id}
        </Button>
        <span>
          {run.status}
          {run.cancel_requested ? " (cancelling)" : ""}
        </span>
        {run.exit_code !== undefined && (
          <span className="text-muted-foreground">exit {run.exit_code}</span>
        )}
        <span className="ml-auto text-xs text-muted-foreground">
          {when(run.finished_at || run.started_at || run.created_at)}
        </span>
        {isRunActive(run.status) && !run.cancel_requested && (
          <Button size="sm" variant="outline" onClick={() => void actions.onCancel(run.id)}>
            Cancel run
          </Button>
        )}
      </div>
      {open && <RunDetails run={run} log={log} {...actions} />}
    </div>
  );
}

// ScriptRunList shows one script's runs, each expandable to its details and log.
export function ScriptRunList({
  id,
  runs,
  expanded,
  logs,
  ...actions
}: {
  id: string;
  runs?: ScriptRun[];
  expanded: Set<string>;
  logs: Record<string, string>;
} & RunActions) {
  if (!runs)
    return (
      <p id={id} className="text-sm text-muted-foreground">
        Loading runs…
      </p>
    );
  if (runs.length === 0)
    return (
      <p id={id} className="text-sm text-muted-foreground">
        No runs.
      </p>
    );
  return (
    <div id={id} className="grid gap-2">
      {runs.map((run) => (
        <RunItem
          key={run.id}
          run={run}
          open={expanded.has(run.id)}
          log={logs[run.id]}
          actions={actions}
        />
      ))}
    </div>
  );
}
