import type { CreateAgentSpec } from "@/lib/api";
import type { AgentCreateDraft } from "../agentCreateDraft";

function integerField(raw: string, label: string, minimum: number): number {
  const value = Number(raw.trim());
  if (!Number.isSafeInteger(value) || value < minimum) {
    throw new Error(`${label} must be a whole number of at least ${minimum}`);
  }
  return value;
}

function parseEnvironment(text: string): Record<string, string> {
  let value: unknown;
  try {
    value = JSON.parse(text || "{}");
  } catch {
    throw new Error("Environment must be valid JSON");
  }
  if (
    value === null ||
    Array.isArray(value) ||
    typeof value !== "object" ||
    Object.values(value).some((entry) => typeof entry !== "string")
  ) {
    throw new Error("Environment must be a JSON object whose values are strings");
  }
  return value as Record<string, string>;
}

export function buildCreateAgentSpec(
  draft: AgentCreateDraft,
  bare: boolean,
  schemaV2: boolean,
): CreateAgentSpec {
  const env = parseEnvironment(draft.envText);
  const intervalS = integerField(draft.intervalS, "Interval seconds", 0);
  const timeoutS = integerField(draft.timeoutS, "Soft timeout seconds", 0);
  const hardTimeoutS = integerField(draft.hardTimeoutS, "Hard timeout seconds", 0);
  const maxIdleIterations = integerField(draft.maxIdleIterations, "Maximum idle iterations", 0);
  const aiStallTimeoutS = integerField(draft.aiStallTimeoutS, "AI inactivity timeout seconds", 1);
  const messagesBatch = integerField(draft.messagesBatch, "Message batch size", 1);
  const messagesMaxQueue = integerField(draft.messagesMaxQueue, "Maximum queued messages", 1);
  const goalWaitCustomerTimeoutS = integerField(
    draft.goalWaitCustomerTimeoutS,
    "Wait customer timeout seconds",
    1,
  );
  const goalDeliveryCooldownS = integerField(
    draft.goalDeliveryCooldownS,
    "Goal delivery cooldown seconds",
    1,
  );

  const spec: CreateAgentSpec = {
    image: draft.image,
    cwd: draft.managedFolder ? "" : draft.cwd.trim(),
    harness: draft.harness.trim(),
    model: draft.model.trim(),
    effort: draft.effort.trim(),
    interactive: bare ? true : draft.interactive,
    env,
    interval_s: intervalS,
    timeout_s: timeoutS,
    hard_timeout_s: hardTimeoutS,
    on_timeout: draft.onTimeout,
    on_error: draft.onError,
    max_idle_iterations: maxIdleIterations,
    ai_stall_timeout_s: aiStallTimeoutS,
    user_prompt: draft.userPrompt,
    messages_batch: messagesBatch,
    messages_max_queue: messagesMaxQueue,
    goal_enabled: draft.goalEnabled,
    goal_wait_customer_timeout_s: goalWaitCustomerTimeoutS,
    goal_delivery_cooldown_s: goalDeliveryCooldownS,
    group: draft.group.trim(),
    alias: draft.alias,
    notes: draft.notes,
    color: draft.color.trim(),
  };
  if (draft.name.trim()) spec.name = draft.name.trim();
  if (spec.cwd && draft.createCwd) spec.create_cwd = true;
  if (!schemaV2) spec.plugins = [...draft.plugins];

  return spec;
}
