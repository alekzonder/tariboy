import { useCallback, useRef, useState } from "react";
import { toast } from "sonner";
import type { ApiClient, AgentScript, AgentScriptRun } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { logNeedsRefresh, nextGeneration, toggledSet } from "./scriptsModel";

// useScriptRuns owns the expanded scripts and runs, their loaded runs and logs,
// and the generation counters that drop responses a newer request replaced.
export function useScriptRuns(api: ApiClient, name: string) {
  const [runs, setRuns] = useState<Record<string, AgentScriptRun[]>>({});
  const [expandedScripts, setExpandedScripts] = useState<Set<string>>(new Set());
  const [expandedRuns, setExpandedRuns] = useState<Set<string>>(new Set());
  const [logs, setLogs] = useState<Record<string, string>>({});
  const runsGenerations = useRef<Record<string, number>>({});
  const logGenerations = useRef<Record<string, number>>({});

  const refreshLog = useCallback(
    async (runID: string) => {
      const generation = nextGeneration(logGenerations.current, runID);
      const log = await api.scripts.getAgentScriptLog(name, runID);
      if (generation !== logGenerations.current[runID]) return;
      setLogs((current) => ({ ...current, [runID]: log.log }));
    },
    [api, name],
  );

  const refreshScriptRuns = useCallback(
    async (scriptID: string, forceLogs: boolean) => {
      const generation = nextGeneration(runsGenerations.current, scriptID);
      try {
        const result = await api.scripts.listAgentScriptRuns(name, scriptID);
        if (generation !== runsGenerations.current[scriptID]) return;
        const nextRuns = result.runs ?? [];
        const previous = new Map((runs[scriptID] ?? []).map((run) => [run.id, run.status]));
        setRuns((current) => ({ ...current, [scriptID]: nextRuns }));
        const stale = nextRuns.filter((run) =>
          logNeedsRefresh({
            run,
            expanded: expandedRuns,
            forceLogs,
            previousStatus: previous.get(run.id),
          }),
        );
        await Promise.all(stale.map((run) => refreshLog(run.id)));
      } catch {
        // Polling is best-effort. An explicit expand still reports load errors.
      }
    },
    [api, expandedRuns, name, refreshLog, runs],
  );

  const refreshExpandedRuns = useCallback(
    async (forceLogs = false) => {
      await Promise.all([...expandedScripts].map((id) => refreshScriptRuns(id, forceLogs)));
    },
    [expandedScripts, refreshScriptRuns],
  );

  const loadRuns = async (scriptID: string) => {
    const generation = nextGeneration(runsGenerations.current, scriptID);
    try {
      const result = await api.scripts.listAgentScriptRuns(name, scriptID);
      if (generation !== runsGenerations.current[scriptID]) return;
      setRuns((current) => ({ ...current, [scriptID]: result.runs ?? [] }));
    } catch (error) {
      toast.error(`Could not load runs: ${errorText(error)}`);
    }
  };

  const loadRunLog = async (run: AgentScriptRun) => {
    const logGeneration = nextGeneration(logGenerations.current, run.id);
    const runsGeneration = nextGeneration(runsGenerations.current, run.script_id);
    try {
      const result = await api.scripts.getAgentScriptLog(name, run.id);
      if (logGeneration === logGenerations.current[run.id]) {
        setLogs((current) => ({ ...current, [run.id]: result.log }));
      }
      if (runsGeneration === runsGenerations.current[run.script_id]) {
        setRuns((current) => ({
          ...current,
          [run.script_id]: (current[run.script_id] ?? []).map((item) =>
            item.id === run.id ? result.run : item,
          ),
        }));
      }
    } catch (error) {
      toast.error(`Could not load run log: ${errorText(error)}`);
    }
  };

  const toggleScript = async (definition: AgentScript) => {
    const open = expandedScripts.has(definition.id);
    setExpandedScripts((current) => toggledSet(current, definition.id, open));
    if (!open && !runs[definition.id]) await loadRuns(definition.id);
  };

  const toggleRun = async (run: AgentScriptRun) => {
    const open = expandedRuns.has(run.id);
    setExpandedRuns((current) => toggledSet(current, run.id, open));
    if (!open && logs[run.id] === undefined) await loadRunLog(run);
  };

  // forgetRuns drops a script's loaded runs and any request still in flight for
  // them, so the next expand reloads the list.
  const forgetRuns = (scriptID: string) => {
    nextGeneration(runsGenerations.current, scriptID);
    setRuns((current) => {
      const next = { ...current };
      delete next[scriptID];
      return next;
    });
  };

  return {
    runs,
    logs,
    expandedScripts,
    expandedRuns,
    refreshExpandedRuns,
    toggleScript,
    toggleRun,
    forgetRuns,
  };
}
