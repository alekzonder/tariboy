import type { Daemon } from "../daemons";
import { subscribeResume } from "../resume";
import type {
  AgentActionResult,
  AgentAliasResult,
  AgentColorResult,
  AgentContextResult,
  AgentCwdResult,
  AgentImageStatus,
  AgentListResult,
  AgentNotesResult,
  AgentPromptLayers,
  AgentPromptPreview,
  AgentRunResult,
  AgentStatus,
  AgentStatusHistory,
  AgentUserPromptResult,
  AgentView,
  ShellScriptResult,
  ShellScriptSaveResult,
} from "../api-types.generated";
import { agentApiPath, type Transport } from "./client";
import { getLocalBaseURL } from "./hosts";

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
} from "../api-types.generated";

export type AgentPromptLayer = NonNullable<AgentPromptLayers>[number];

// Not in /api/openapi.json: served by internal/api/server.go GET /api/agents/{name}/events.
export interface AgentEvent {
  agent: string;
  type: string; // message | stream | iteration | audit | proxy
  time: string;
  data?: Record<string, unknown>;
}

// ---- SSE live events ----
// Opens an EventSource on <daemon.baseURL>/api/agents/<name>/events. For a
// cross-origin daemon the bearer rides in the URL as ?token= (EventSource cannot
// set an Authorization header — the daemon accepts it only on the /events route,
// and federated daemons therefore MUST run over TLS). Named events
// (iteration|message|stream|audit|proxy) invoke onEvent. The hub drops on a full
// buffer, so callers treat this as a refetch hint, not the source of truth.
export function subscribeAgentEventsOn(
  daemon: Daemon | null,
  name: string,
  types: string[],
  onEvent: (ev: AgentEvent) => void,
): () => void {
  if (daemon && !daemon.baseURL) return () => {};
  const params = new URLSearchParams();
  if (types.length) params.set("types", types.join(","));
  if (daemon && daemon.baseURL && daemon.token) params.set("token", daemon.token);
  const path = agentApiPath(name, "events");
  const base = daemon && daemon.baseURL ? daemon.baseURL.replace(/\/+$/, "") : getLocalBaseURL();
  const qs = params.toString();
  const url = `${base}${path}${qs ? `?${qs}` : ""}`;
  const kinds = ["message", "stream", "iteration", "audit", "proxy"];
  const handler = (e: MessageEvent) => {
    try {
      onEvent(JSON.parse(e.data) as AgentEvent);
    } catch {
      /* ignore malformed frame */
    }
  };
  const open = () => {
    const source = new EventSource(url);
    for (const k of kinds) source.addEventListener(k, handler as EventListener);
    return source;
  };
  let es = open();
  // The old stream may be dead without an error; a fresh one is cheap.
  const stopResume = subscribeResume(() => {
    es.close();
    es = open();
  });
  return () => {
    stopResume();
    es.close();
  };
}

export function agentsApi(t: Transport) {
  return {
    listAgents: () => t.get<AgentListResult>("/api/agents"),
    getAgent: (name: string) => t.agentGet<AgentView>(name, ""),
    getAgentStatus: (name: string) => t.agentGet<AgentStatus>(name, "status"),
    createAgent: (spec: CreateAgentSpec) => t.post<AgentRunResult>("/api/agents", spec),
    startAgent: (name: string) => t.agentPost<AgentActionResult>(name, "start"),
    // force=true kills a live agent so it can be removed at all (a plain DELETE
    // 400s on a running/idle agent: "stop it first or use --force"). purge=true
    // hard-deletes the DB row + durable data (iterations/audit) + whole tree, so
    // the agent actually disappears from the list — WITHOUT purge the daemon
    // only does a preserving reset (strips the rebuildable tree, keeps the row
    // listed as "stopped").
    agentDelete: <T>(name: string, opts?: { force?: boolean; purge?: boolean }) => {
      const q = new URLSearchParams();
      if (opts?.force) q.set("force", "true");
      if (opts?.purge) q.set("purge", "true");
      const qs = q.toString();
      return t.request<T>("DELETE", agentApiPath(name, "") + (qs ? `?${qs}` : ""));
    },
    setAgentCwd: (name: string, value: string, create = false) =>
      t.agentPost<AgentCwdResult>(name, "cwd", create ? { value, create: true } : { value }),
    // ---- Live config setters (persist now; launch-mode changes apply next start) ----
    setAgentModel: (name: string, value: string) => t.agentPost(name, "model", { value }),
    setAgentEffort: (name: string, value: string) => t.agentPost(name, "effort", { value }),
    setAgentInteractive: (name: string, value: boolean) =>
      t.agentPost(name, "interactive", { value }),
    setAgentHarness: (name: string, value: string) => t.agentPost(name, "harness", { value }),
    // ---- Alias / notes / color / status-history ----
    getAlias: (name: string) => t.agentGet<AgentAliasResult>(name, "alias"),
    setAlias: (name: string, value: string) => t.agentPost(name, "alias", { value }),
    getNotes: (name: string) => t.agentGet<AgentNotesResult>(name, "notes"),
    setNotes: (name: string, value: string) => t.agentPost(name, "notes", { value }),
    // The setter's body field is `value` (matching the backend agent.color.set
    // param); an empty string clears the color.
    getColor: (name: string) => t.agentGet<AgentColorResult>(name, "color"),
    setColor: (name: string, value: string) => t.agentPost(name, "color", { value }),
    getStatusHistory: (name: string) => t.agentGet<AgentStatusHistory>(name, "status/history"),
    agentPromptGet: (name: string) => t.agentGet<AgentPromptPreview>(name, "prompt"),
    agentUserPromptGet: (name: string) => t.agentGet<AgentUserPromptResult>(name, "user-prompt"),
    agentUserPromptSet: (name: string, text: string) => t.agentPost(name, "user-prompt", { text }),
    agentContextGet: (name: string) => t.agentGet<AgentContextResult>(name, "context"),
    agentContextSet: (name: string, text: string) => t.agentPost(name, "context", { text }),
    getAgentShellScript: (name: string) => t.agentGet<ShellScriptResult>(name, "shell-script"),
    setAgentShellScript: (name: string, script: string) =>
      t.agentPost<ShellScriptSaveResult>(name, "shell-script", { script }),
    agentImageStatusGet: (name: string) => t.agentGet<AgentImageStatus>(name, "image"),
    agentImageSet: (name: string, image: string) =>
      t.agentPost<AgentImageStatus>(name, "image", { image }),
    agentImageCancel: (name: string) => t.agentDelete<AgentImageStatus>(name, "image"),
    subscribeAgentEvents: (name: string, types: string[], onEvent: (ev: AgentEvent) => void) =>
      subscribeAgentEventsOn(t.target, name, types, onEvent),
  };
}
