import type { AgentView } from "@/lib/types";

export type AgentPolicy = "restart" | "stop";

export interface AgentCreateDraft {
  image: string;
  name: string;
  cwd: string;
  harness: string;
  model: string;
  effort: string;
  interactive: boolean;
  loop: boolean;
  startNow: boolean;
  intervalS: string;
  timeoutS: string;
  hardTimeoutS: string;
  onTimeout: AgentPolicy;
  onError: AgentPolicy;
  maxIdleIterations: string;
  userPrompt: string;
  envText: string;
  plugins: string[];
  messagesBatch: string;
  messagesMaxQueue: string;
  goalEnabled: boolean;
  goalWaitCustomerTimeoutS: string;
  goalDeliveryCooldownS: string;
  group: string;
  alias: string;
  notes: string;
  color: string;
}

export function newAgentDraft(image = ""): AgentCreateDraft {
  return {
    image,
    name: "",
    cwd: "",
    harness: "",
    model: "",
    effort: "",
    interactive: false,
    loop: true,
    startNow: true,
    intervalS: "0",
    timeoutS: "0",
    hardTimeoutS: "0",
    onTimeout: "restart",
    onError: "restart",
    maxIdleIterations: "0",
    userPrompt: "",
    envText: "{}",
    plugins: [],
    messagesBatch: "10",
    messagesMaxQueue: "1000",
    goalEnabled: true,
    goalWaitCustomerTimeoutS: "300",
    goalDeliveryCooldownS: "60",
    group: "",
    alias: "",
    notes: "",
    color: "",
  };
}

export function cloneAgentDraft(source: AgentView): AgentCreateDraft {
  if (
    source.configured_cwd === undefined ||
    source.messages_batch === undefined ||
    source.messages_max_queue === undefined ||
    source.goal_enabled === undefined ||
    source.goal_wait_customer_timeout_s === undefined ||
    source.goal_delivery_cooldown_s === undefined
  ) {
    throw new Error("Update the source host before making a complete clone");
  }
  return {
    image: source.image,
    name: "",
    cwd: source.configured_cwd,
    harness: source.harness,
    model: source.model,
    effort: source.effort,
    interactive: source.interactive,
    loop: source.loop_enabled,
    startNow: source.enabled ?? false,
    intervalS: String(source.interval_s),
    timeoutS: String(source.timeout_s),
    hardTimeoutS: String(source.hard_timeout_s),
    onTimeout: source.on_timeout === "stop" ? "stop" : "restart",
    onError: source.on_error === "stop" ? "stop" : "restart",
    maxIdleIterations: String(source.max_idle_iterations),
    userPrompt: source.user_prompt,
    envText: JSON.stringify(source.env, null, 2),
    plugins: [...source.plugins],
    messagesBatch: String(source.messages_batch),
    messagesMaxQueue: String(source.messages_max_queue),
    goalEnabled: source.goal_enabled,
    goalWaitCustomerTimeoutS: String(source.goal_wait_customer_timeout_s),
    goalDeliveryCooldownS: String(source.goal_delivery_cooldown_s),
    group: source.group ?? "",
    alias: source.alias,
    notes: source.notes,
    color: source.color ?? "",
  };
}
