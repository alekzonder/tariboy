import type {
  AgentScriptCancelResult,
  AgentScriptList,
  AgentScriptRemoveResult,
  AgentScriptRun,
  AgentScriptRunList,
  AgentScriptRunLog,
  AgentScriptStarted,
} from "../api-types.generated";
import { agentApiPath, type Transport } from "./client";

import type { RunOnceScriptSpec, ScheduleScriptSpec } from "@/types/scripts";

export type * from "@/types/scripts";

export function scriptsApi(t: Transport) {
  return {
    listAgentScripts: (name: string) => t.agentGet<AgentScriptList>(name, "scripts"),
    runAgentScriptOnce: (name: string, spec: RunOnceScriptSpec) =>
      t.agentPost<AgentScriptStarted>(name, "scripts/run", spec),
    scheduleAgentScript: (name: string, spec: ScheduleScriptSpec) =>
      t.agentPost<AgentScriptStarted>(name, "scripts/schedule", spec),
    rerunAgentScript: (name: string, id: string) =>
      t.agentPost<AgentScriptRun>(name, `scripts/${encodeURIComponent(id)}/rerun`),
    listAgentScriptRuns: (name: string, id: string) =>
      t.agentGet<AgentScriptRunList>(name, `scripts/${encodeURIComponent(id)}/runs`),
    getAgentScriptRun: (name: string, id: string) =>
      t.agentGet<AgentScriptRun>(name, `script-runs/${encodeURIComponent(id)}`),
    getAgentScriptLog: (name: string, id: string) =>
      t.agentGet<AgentScriptRunLog>(name, `script-runs/${encodeURIComponent(id)}/logs`),
    downloadAgentScriptLog: async (name: string, id: string) =>
      (
        await t.raw("GET", agentApiPath(name, `script-runs/${encodeURIComponent(id)}/download`))
      ).blob(),
    cancelAgentScript: (name: string, id: string) =>
      t.agentPost<AgentScriptCancelResult>(name, `script-targets/${encodeURIComponent(id)}/cancel`),
    removeAgentScript: (name: string, id: string) =>
      t.agentDelete<AgentScriptRemoveResult>(name, `scripts/${encodeURIComponent(id)}`),
  };
}
