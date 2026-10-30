import type {
  AgentRow,
  AgentView,
  ImageSummary,
  Operation,
  OperationStarted,
  Task,
} from "@/lib/api";

/** Complete API responses for tests; each test overrides only what it checks. */

export const agentBudget = (): AgentRow["budget"] => ({
  hour_usd: 0,
  day_usd: 0,
  week_usd: 0,
  month_usd: 0,
  hour_spent_usd: 0,
  day_spent_usd: 0,
  week_spent_usd: 0,
  month_spent_usd: 0,
  exhausted: null,
});

export const agentRow = (over: Partial<AgentRow> = {}): AgentRow => ({
  name: "worker",
  image: "base:1",
  state: "stopped",
  harness: "claude",
  enabled: true,
  group: "",
  alias: "",
  color: "",
  interactive: false,
  env: {},
  cwd: "",
  current_goal_task_key: "",
  ai_stall_timeout_s: 0,
  goal_delivery_cooldown_s: 0,
  goal_enabled: false,
  goal_wait_customer_timeout_s: 0,
  interval_s: 0,
  max_idle_iterations: 0,
  on_error: "",
  on_timeout: "",
  schedule: "",
  schedule_next_at: "",
  timeout_s: 0,
  budget: agentBudget(),
  ...over,
});

export const agentView = (over: Partial<AgentView> = {}): AgentView => ({
  ...agentRow(),
  digest: "",
  configured_cwd: "",
  model: "",
  effort: "",
  hard_timeout_s: 0,
  user_prompt: "",
  env: {},
  plugins: [],
  messages_batch: 0,
  messages_max_queue: 0,
  notes: "",
  ...over,
});

export const imageSummary = (over: Partial<ImageSummary> = {}): ImageSummary => ({
  name: "worker",
  tag: "v1",
  bare: false,
  built_at: "",
  digest: "",
  exportable: false,
  schema_version: 2,
  current_agents: null,
  pending_agents: null,
  ...over,
});

export const operation = (over: Partial<Operation> = {}): Operation => ({
  id: 1,
  kind: "image.build",
  title: "Build image reviewer:latest",
  status: "running",
  initiator: "ui",
  params: {},
  created_at: "2026-10-29T10:00:00Z",
  ...over,
});

/** What a build POST answers: the operation that now runs the build. */
export const operationStarted = (over: Partial<Operation> = {}): OperationStarted => ({
  operation: operation(over),
});

/** A flexible task list item. */
export const taskItem = (over: Partial<Task> = {}): Task => ({
  key: "TASK-1",
  queue: "TASK",
  parent_key: "",
  position: 0,
  priority: "P2",
  title: "Add a README",
  description: "",
  status: "open",
  category: "open",
  pull_request: "",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "agent:bob",
  manual_block_reason: "",
  blocked: false,
  revision: 1,
  created_at: "2026-10-30T09:00:00Z",
  updated_at: "2026-10-30T09:00:00Z",
  completed_at: "",
  started_at: "",
  ...over,
});
