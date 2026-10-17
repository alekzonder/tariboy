import {
  agentApiPath,
  agentImageSetOn,
  agentPostOn,
  apiOn,
  resolveTarget,
  setAgentCwdOn,
  setAgentShellScriptOn,
  type ApiTarget,
} from "@/lib/api";
import { rememberRuntimePreset } from "@/lib/runtimePresets";
import type { AgentImageStatus } from "@/lib/api";
import type { AgentView } from "@/lib/types";
import { normalizeField, type Values } from "./registry";
import type { RetentionPolicy } from "./useConfigSources";
import type { CommitUnit } from "./useConfigDraft";

const text = (value: unknown) => (value === undefined || value === null ? "" : String(value));

// readBaseline turns the loaded sources into the page's flat, normalized
// values. A source that has not loaded contributes no keys, so its fields can
// be neither dirty nor saved.
export function readBaseline(sources: {
  view: AgentView | null;
  imageStatus: AgentImageStatus | null;
  script: string | null;
  retention: RetentionPolicy | null;
  secrets: string[] | null;
}): Values {
  const raw: Values = {};
  const { view, imageStatus, script, retention, secrets } = sources;
  if (view) {
    Object.assign(raw, {
      cwd: text(view.cwd),
      harness: text(view.harness),
      model: text(view.model),
      effort: text(view.effort),
      interactive: String(Boolean(view.interactive)),
      interval: text(view.interval_s),
      timeout: text(view.timeout_s),
      "hard-timeout": text(view.hard_timeout_s),
      "on-timeout": view.on_timeout || "restart",
      "on-error": view.on_error || "restart",
      "max-idle": text(view.max_idle_iterations ?? 0),
      "ai-stall-timeout": text(view.ai_stall_timeout_s ?? 1800),
      "goal-enabled": String(Boolean(view.goal_enabled)),
      "goal-wait-customer-timeout": text(view.goal_wait_customer_timeout_s),
      "goal-delivery-cooldown": text(view.goal_delivery_cooldown_s),
      "messages-batch": text(view.messages_batch),
      "messages-max-queue": text(view.messages_max_queue),
    });
    if (view.budget) {
      Object.assign(raw, {
        "budget-hour": text(view.budget.hour_usd),
        "budget-day": text(view.budget.day_usd),
        "budget-week": text(view.budget.week_usd),
        "budget-month": text(view.budget.month_usd),
      });
    }
  }
  if (imageStatus) raw.image = imageStatus.pending.ref || imageStatus.current.ref;
  if (script !== null) raw["shell-script"] = script;
  if (retention) {
    raw["keep-iterations"] = text(retention.override?.keep_iterations ?? 0);
    raw["keep-days"] = text(retention.override?.keep_days ?? 0);
  }
  if (secrets) Object.assign(raw, { "secrets-remove": "", "secret-key": "", "secret-value": "" });
  return Object.fromEntries(Object.entries(raw).map(([key, value]) => [key, normalizeField(key, value)]));
}

// Keys queued for removal are kept one per line in the "secrets-remove" draft.
export const removedSecrets = (value: string) => value.split("\n").map((key) => key.trim()).filter(Boolean);

const one = (key: string, submit: (value: string, values: Values) => Promise<unknown>): CommitUnit => ({
  id: key,
  keys: [key],
  submit: (values) => submit(values[key], values),
});

// buildUnits lists every request a Save can send, in page order. Harness goes
// before model and effort, so a model is remembered for the harness it is
// saved with. The backend has no batch endpoint: what Save changes is when and
// how many of these requests are sent, never which ones exist.
export function buildUnits(target: ApiTarget, name: string): CommitUnit[] {
  const post = (path: string, body: unknown) => agentPostOn(target, name, path, body);
  const loopNumber = (field: string) => one(field, (value) => post(`loop/${field}`, { value: Number(value) }));
  const loopText = (field: string) => one(field, (value) => post(`loop/${field}`, { value }));
  return [
    one("image", (value) => agentImageSetOn(target, name, value)),
    one("cwd", (value) => setAgentCwdOn(target, name, value)),
    one("harness", (value) => post("harness", { value })),
    one("model", async (value, values) => {
      await post("model", { value });
      rememberRuntimePreset(values.harness, "models", value);
    }),
    one("effort", async (value, values) => {
      await post("effort", { value });
      rememberRuntimePreset(values.harness, "efforts", value);
    }),
    one("interactive", (value) => post("interactive", { value: value === "true" })),
    loopNumber("interval"),
    loopNumber("timeout"),
    loopNumber("hard-timeout"),
    loopText("on-timeout"),
    loopText("on-error"),
    loopNumber("max-idle"),
    loopNumber("ai-stall-timeout"),
    one("goal-enabled", (value) => post("goal-enabled", { enabled: value === "true" })),
    one("goal-wait-customer-timeout", (value) => post("goal-wait-customer-timeout", { seconds: Number(value) })),
    one("goal-delivery-cooldown", (value) => post("goal-delivery-cooldown", { seconds: Number(value) })),
    one("messages-batch", (value) => post("messages/batch", { value: Number(value) })),
    one("messages-max-queue", (value) => post("messages/max-queue", { value: Number(value) })),
    {
      id: "budget",
      keys: ["budget-hour", "budget-day", "budget-week", "budget-month"],
      submit: (values) => post("budget", {
        hour_usd: values["budget-hour"],
        day_usd: values["budget-day"],
        week_usd: values["budget-week"],
        month_usd: values["budget-month"],
      }),
    },
    one("shell-script", (value) => setAgentShellScriptOn(target, name, value)),
    one("secrets-remove", async (value) => {
      for (const key of removedSecrets(value)) {
        await apiOn(resolveTarget(target), "DELETE", `${agentApiPath(name, "secrets")}?key=${encodeURIComponent(key)}`);
      }
    }),
    {
      // SECURITY: the value is sent in a POST body only, never in a URL, and
      // its draft is dropped as soon as the daemon acknowledges it.
      id: "secret",
      keys: ["secret-key", "secret-value"],
      submit: (values) => post("secrets", { key: values["secret-key"], value: values["secret-value"] }),
    },
    {
      id: "retention",
      keys: ["keep-iterations", "keep-days"],
      submit: (values) => post("retention", {
        "keep-iterations": Number(values["keep-iterations"]),
        "keep-days": Number(values["keep-days"]),
      }),
    },
  ];
}
