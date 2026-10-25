import type { RunOnceScriptSpec, ScheduleScriptSpec, ScriptMode } from "@/lib/api";

type ScriptFormState = {
  script_name: string;
  description: string;
  command: string;
  mode: ScriptMode;
  interval_seconds: string;
};

export const emptyScriptForm: ScriptFormState = {
  script_name: "",
  description: "",
  command: "",
  mode: "once",
  interval_seconds: "",
};

export type ScriptRequest =
  | { mode: "once"; spec: RunOnceScriptSpec }
  | { mode: "every"; spec: ScheduleScriptSpec }
  | { error: string };

// scriptRequest turns the launch form into the API request for its mode, or the
// validation error a scheduled script without a positive whole interval gets.
export function scriptRequest(form: ScriptFormState): ScriptRequest {
  const spec = {
    script_name: form.script_name,
    description: form.description,
    command: form.command,
  };
  if (form.mode === "once") return { mode: "once", spec };
  const interval = Number(form.interval_seconds);
  if (!Number.isInteger(interval) || interval <= 0) {
    return { error: "Scheduled scripts need a positive interval in seconds" };
  }
  return { mode: "every", spec: { ...spec, interval_seconds: interval } };
}
