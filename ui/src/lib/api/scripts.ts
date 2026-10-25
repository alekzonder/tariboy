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

export type { AgentScript, AgentScriptRun } from "../api-types.generated";

// ---- Durable agent scripts ----
export type ScriptMode = "once" | "every";
export interface RunOnceScriptSpec {
  script_name: string;
  description: string;
  command: string;
}
export interface ScheduleScriptSpec extends RunOnceScriptSpec {
  interval_seconds: number;
}

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
