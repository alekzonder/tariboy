// v2 daemon response types. Shapes are authoritative from internal/commands/*.go.

export interface DaemonStatus {
  version: string;
  pid: number;
  started_at: string;
  uptime_seconds: number;
  base_dir: string;
  http_addr: string;
  schema_version: number;
}

export interface AgentSummary {
  name: string;
  image: string;
  state: string; // running | stopped | ...
  harness: string;
  loop_enabled: boolean;
  enabled?: boolean; // master on/off; missing (old daemon) → fall back to state
  group: string | null;
  alias?: string; // display alias; shown alias-first via displayName() when set
  color?: string; // per-agent accent hex (#rrggbb), empty/absent when unset
  interactive?: boolean;
  cwd?: string;
	budget?: AgentBudgetStatus;
}

export interface AgentBudgetStatus {
	hour_usd: number;
	day_usd: number;
	week_usd: number;
	month_usd: number;
	hour_spent_usd: number;
	day_spent_usd: number;
	week_spent_usd: number;
	month_spent_usd: number;
	exhausted?: string[] | null;
}

export interface AgentLifecycleResult {
  name: string;
  action: "start" | "stop" | "restart" | "kill";
}

export interface AgentView {
  name: string;
  image: string;
  digest: string;
  state: string;
  cwd: string;
  configured_cwd?: string;
  harness: string;
  model: string;
  effort: string;
  interactive: boolean;
  loop_enabled: boolean;
  enabled?: boolean;
  interval_s: number;
  timeout_s: number;
  hard_timeout_s: number;
  on_timeout: string;
  on_error: string;
  max_idle_iterations: number;
  user_prompt: string;
  env: Record<string, string>;
  plugins: string[];
  messages_batch?: number;
  messages_max_queue?: number;
  goal_enabled: boolean;
  goal_wait_customer_timeout_s: number;
  current_goal_task_key: string;
  group: string | null;
  alias: string;
  notes: string;
  color?: string; // per-agent accent hex (#rrggbb), empty/absent when unset
	budget?: AgentBudgetStatus;
}

export interface AgentStatus {
  name: string;
  state: string;
  loop_enabled: boolean;
  iterations: number;
  last_iteration: string | null;
  last_iteration_id: string | null;
  status_message: string;
  status_updated: string;
  // Why the daemon halted this agent's loop.  Both are absent unless the daemon
  // has a halt to report: halt_kind is "error" or "idle_limit".
  halt_kind?: string;
  halt_reason?: string;
  // Present while an iteration is running.  The daemon's clock accompanies the
  // snapshot so countdowns do not assume the browser clock is correct.
  server_now?: string;
  active_iteration?: ActiveIteration;
	budget?: AgentBudgetStatus;
}

export interface ActiveIteration {
  id: string;
  started_at: string;
  timeout_period_s?: number;
  timeout_deadline?: string;
  hard_timeout_deadline?: string;
  effective_deadline?: string;
  timeout_extensions: number;
}

export interface StatusHistoryEvent {
  ts: string;
  message: string;
  // Older daemons did not include this field; absent is treated as outside an
  // iteration so the history remains readable during a rolling upgrade.
  iteration_id?: string;
}

export interface IterationSummary {
  id: string;
  trigger: string;
  status: string;
  started_at: string;
  done: boolean;
  // productive is false for iterations finished with `i-am-done --idle`
  // (no real work), true otherwise. Always a bool from the backend: the
  // `productive` column is NOT NULL DEFAULT 1, so pre-field rows and
  // in-flight iterations serialize as true, never undefined.
  productive: boolean;
}

export interface IterationDetail {
  id: string;
  agent: string;
  trigger: string;
  status: string;
  started_at: string;
  ended_at: string | null;
  done: boolean;
  prompt_path: string;
  exit_code?: number;
  cpu_ms?: number;
  mem_peak_kb?: number;
}

export interface IterationLogs {
  stdout: string;
  stderr: string;
}

export interface UsageRow {
  agent: string;
  image: string;
  group_id: string | null;
  group_name: string | null;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  cost_usd: number;
}

export interface UsagePoint {
  bucket_start: string;
  requests: number;
  tokens: number;
  cost_usd: number;
}

export interface UsageRequest {
  id: string;
  ts: string;
  agent: string;
  image: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  cost_usd: number;
  status: string;
  group_id: string | null;
  group_name: string | null;
}

export interface UsageReport {
  rows: UsageRow[];
  count: number;
  total_requests: number;
  total_cost_usd: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_cache_write_tokens: number;
  total_cache_read_tokens: number;
  series: UsagePoint[];
  requests: UsageRequest[];
}

// Per-agent Usage tab (epic dev-t-3e1). Mirrors GET
// /api/agents/{name}/usage from internal/commands/usage.go: grouped rows
// (by iteration|task|epic|model) plus a time-bucketed cost/traffic series.
export interface AgentUsageTotals {
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  cost_usd: number;
}

export interface AgentUsageRow {
  key: string; // iteration id / task id / epic id / model; "untagged" when unset
  title?: string; // present only for task/epic groupings (human title, else bare id)
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  cost_usd: number;
}

export interface AgentUsagePoint {
  bucket_start: string; // RFC3339 UTC bucket start
  requests: number;
  tokens: number;
  cost_usd: number;
}

export interface AgentUsageReport {
  agent: string;
  group_by: "iteration" | "task" | "epic" | "model";
  bucket: "5m" | "15m" | "1h" | "1d";
  totals: AgentUsageTotals;
  rows: AgentUsageRow[];
  series: AgentUsagePoint[];
}

// SSE event (internal/events/hub.go).
export interface AgentEvent {
  agent: string;
  type: string; // message | stream | iteration | audit | proxy
  time: string;
  data?: Record<string, unknown>;
}
