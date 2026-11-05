export type { AgentScript, AgentScriptRun } from "@/lib/api-types.generated";

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
