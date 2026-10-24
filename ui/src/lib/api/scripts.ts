import { agentApiPath, type Transport } from "./client";

// ---- Durable agent scripts ----
export type ScriptMode = "once" | "every";
type ScriptState = "active" | "completed" | "cancelled";
type ScriptRunStatus =
  | "pending"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "timed_out"
  | "interrupted";
export interface ScriptRun {
  id: string;
  script_id: string;
  agent: string;
  status: ScriptRunStatus;
  cancel_requested?: boolean;
  pid?: number;
  exit_code?: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  log_path?: string;
}
export interface ScriptDefinition {
  id: string;
  agent: string;
  name: string;
  description: string;
  command: string;
  mode: ScriptMode;
  interval_seconds: number;
  state: ScriptState;
  created_at: string;
  next_run_at?: string;
  latest_run?: ScriptRun;
}
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
    listAgentScripts: (name: string) =>
      t.agentGet<{ scripts: ScriptDefinition[]; count: number }>(name, "scripts"),
    runAgentScriptOnce: (name: string, spec: RunOnceScriptSpec) =>
      t.agentPost<{ script: ScriptDefinition; run: ScriptRun }>(name, "scripts/run", spec),
    scheduleAgentScript: (name: string, spec: ScheduleScriptSpec) =>
      t.agentPost<{ script: ScriptDefinition; run: ScriptRun }>(name, "scripts/schedule", spec),
    rerunAgentScript: (name: string, id: string) =>
      t.agentPost<ScriptRun>(name, `scripts/${encodeURIComponent(id)}/rerun`),
    listAgentScriptRuns: (name: string, id: string) =>
      t.agentGet<{ runs: ScriptRun[]; count: number }>(
        name,
        `scripts/${encodeURIComponent(id)}/runs`,
      ),
    getAgentScriptRun: (name: string, id: string) =>
      t.agentGet<ScriptRun>(name, `script-runs/${encodeURIComponent(id)}`),
    getAgentScriptLog: (name: string, id: string) =>
      t.agentGet<{ run: ScriptRun; log: string }>(
        name,
        `script-runs/${encodeURIComponent(id)}/logs`,
      ),
    downloadAgentScriptLog: async (name: string, id: string) =>
      (
        await t.raw("GET", agentApiPath(name, `script-runs/${encodeURIComponent(id)}/download`))
      ).blob(),
    cancelAgentScript: (name: string, id: string) =>
      t.agentPost<{ id: string; cancelled: boolean }>(
        name,
        `script-targets/${encodeURIComponent(id)}/cancel`,
      ),
    removeAgentScript: (name: string, id: string) =>
      t.agentDelete<{ id: string; removed: boolean }>(name, `scripts/${encodeURIComponent(id)}`),
  };
}
