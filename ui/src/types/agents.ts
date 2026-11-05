import type { AgentPromptLayers } from "@/lib/api-types.generated";

export type {
  AgentActionResult,
  AgentBudget,
  AgentImageStatus,
  AgentListResult,
  AgentPromptPreview,
  AgentRow,
  AgentRunResult,
  AgentStatus,
  AgentStatusEvent,
  AgentView,
  SecretKeyList,
  ShellScriptResult,
} from "@/lib/api-types.generated";

// ---- Agent creation (create forms; POST /api/agents = agent.run) ----
// The daemon keeps legacy scalar env/plugins forms for CLI and older callers,
// while the complete Desktop dialog uses structured values so commas, equals
// signs, whitespace, and newlines round-trip without reparsing.
export interface CreateAgentSpec {
  image: string;
  name?: string;
  cwd?: string;
  create_cwd?: boolean;
  harness?: string;
  model?: string;
  effort?: string;
  interactive?: boolean;
  env?: string | Record<string, string>;
  plugins?: string | string[];
  timeout?: string;
  interval_s?: number;
  timeout_s?: number;
  hard_timeout_s?: number;
  on_timeout?: "restart" | "stop";
  on_error?: "restart" | "stop";
  max_idle_iterations?: number;
  ai_stall_timeout_s?: number;
  user_prompt?: string;
  messages_batch?: number;
  messages_max_queue?: number;
  goal_enabled?: boolean;
  goal_wait_customer_timeout_s?: number;
  goal_delivery_cooldown_s?: number;
  group?: string;
  alias?: string;
  notes?: string;
  color?: string;
}

export type AgentPromptLayer = NonNullable<AgentPromptLayers>[number];

// Not in /api/openapi.json: served by internal/api/server.go GET /api/agents/{name}/events.
export interface AgentEvent {
  agent: string;
  type: string; // message | stream | iteration | audit | proxy
  time: string;
  data?: Record<string, unknown>;
}
