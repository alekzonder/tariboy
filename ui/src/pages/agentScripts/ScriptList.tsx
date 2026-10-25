import { Fragment } from "react";
import { Button } from "@/components/ui/button";
import type { AgentScript } from "@/lib/api";
import { canRerun, when } from "./scriptsModel";
import { ScriptRunList } from "./ScriptRunList";
import type { AgentScriptsState } from "./useAgentScripts";

function ScriptActions({
  definition,
  state,
}: {
  definition: AgentScript;
  state: AgentScriptsState;
}) {
  return (
    <td className="flex flex-wrap gap-1 p-2">
      {definition.state === "active" && (
        <Button size="sm" variant="outline" onClick={() => void state.cancel(definition.id)}>
          Cancel
        </Button>
      )}
      {canRerun(definition) && (
        <Button size="sm" variant="outline" onClick={() => void state.rerun(definition)}>
          Exec
        </Button>
      )}
      {definition.state !== "active" && (
        <Button size="sm" variant="destructive" onClick={() => state.setRemove(definition)}>
          Remove
        </Button>
      )}
    </td>
  );
}

function ScriptRow({ definition, state }: { definition: AgentScript; state: AgentScriptsState }) {
  const open = state.expandedScripts.has(definition.id);
  return (
    <Fragment>
      <tr className="border-t align-top">
        <td className="p-2">
          <Button
            variant="ghost"
            className="h-auto px-1 font-medium"
            aria-expanded={open}
            aria-controls={`runs-${definition.id}`}
            onClick={() => void state.toggleScript(definition)}
          >
            {open ? "▾" : "▸"} {definition.name}
          </Button>
          <div className="max-w-md whitespace-pre-wrap pl-7 text-muted-foreground">
            {definition.description}
          </div>
          <div className="max-w-md whitespace-pre-wrap break-words pl-7 text-xs text-muted-foreground">
            <span className="font-medium text-foreground">Command: </span>
            <code className="font-mono">{definition.command}</code>
          </div>
        </td>
        <td className="p-2">
          {definition.mode === "every" ? `Every ${definition.interval_seconds}s` : "Once"}
        </td>
        <td className="p-2">
          {definition.state}
          <div className="text-xs text-muted-foreground">
            latest {definition.latest_run?.status ?? "—"}
          </div>
        </td>
        <td className="p-2 text-xs text-muted-foreground">{when(definition.next_run_at)}</td>
        <ScriptActions definition={definition} state={state} />
      </tr>
      {open && (
        <tr className="border-t bg-muted/10">
          <td colSpan={5} className="p-3">
            <ScriptRunList
              id={`runs-${definition.id}`}
              runs={state.runs[definition.id]}
              expanded={state.expandedRuns}
              logs={state.logs}
              onToggle={state.toggleRun}
              onCancel={state.cancel}
              onCopy={state.copyPath}
              onDownload={state.download}
            />
          </td>
        </tr>
      )}
    </Fragment>
  );
}

// ScriptList is the table of the agent's scripts with their expandable runs.
export function ScriptList({ state }: { state: AgentScriptsState }) {
  return (
    <section>
      <h2 className="mb-3 text-lg font-semibold">Scripts</h2>
      {state.scripts.length === 0 ? (
        <p className="text-sm text-muted-foreground">No scripts yet.</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="p-2">Script</th>
                <th className="p-2">Mode</th>
                <th className="p-2">State</th>
                <th className="p-2">Next run</th>
                <th className="p-2">Actions</th>
              </tr>
            </thead>
            <tbody>
              {state.scripts.map((definition) => (
                <ScriptRow key={definition.id} definition={definition} state={state} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
