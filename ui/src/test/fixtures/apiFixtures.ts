import type {
  AgentRow,
  AgentView,
  ImageSummary,
  MetricsReport,
  MetricsSlice,
  Operation,
  OperationStarted,
  Task,
  TaskMetrics,
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

/** A metrics slice where every field has its own value, so a test that finds a
 *  shown value knows which field it came from. */
export const metricsSlice = (over: Partial<MetricsSlice> = {}): MetricsSlice => ({
  name: "worker",
  version: "0.3.0",
  tasks: {
    tasks_done: 7,
    mixed_tasks: 3,
    lead_time_median_s: 45_000,
    lead_time_avg_s: 51_600,
    avg: {
      cost_usd: 1.37,
      iterations: 2.31,
      rework_rounds: 0.43,
      rejected_advances: 0.14,
      customer_questions: 0.57,
      input_tokens: 400_000,
      output_tokens: 50_000,
      cache_write_tokens: 150_000,
      cache_read_tokens: 600_000,
    },
    sum: {
      cost_usd: 9.59,
      iterations: 16,
      rework_rounds: 3,
      rejected_advances: 1,
      customer_questions: 4,
      tokens: { input: 2_800_000, output: 350_000, cache_write: 1_050_000, cache_read: 4_200_000 },
    },
    quality: {
      image_process: { avg: 82, min: 61, count: 12 },
      role: { avg: 74, min: 55, count: 11 },
      task_fit: { avg: 90, min: 70, count: 10 },
    },
  },
  usage: {
    iterations: 23,
    cost_usd: 12.4,
    tokens: { input: 3_000_000, output: 400_000, cache_write: 200_000, cache_read: 5_000_000 },
  },
  ...over,
});

/** A server metrics report with one slice in each list. */
export const metricsReport = (over: Partial<MetricsReport> = {}): MetricsReport => ({
  since: "2026-10-03T14:05:00Z",
  until: "2026-11-02T14:05:00Z",
  total: metricsSlice({ name: undefined, version: undefined }),
  agents: [metricsSlice({ name: "alice", version: undefined })],
  images: [metricsSlice({ name: "developer", version: "0.23.2" })],
  workflows: [metricsSlice({ name: "development", version: "0.10.1" })],
  ...over,
});

/** The metrics of one task. */
export const taskMetrics = (over: Partial<TaskMetrics> = {}): TaskMetrics => ({
  key: "DEV-1",
  title: "Add metrics",
  status: "done",
  started_at: "2026-10-30T09:00:00Z",
  completed_at: "2026-10-31T09:00:00Z",
  lead_time_s: 86_400,
  status_time_s: { implement: 3600, review: 1800 },
  usage: metricsSlice().usage,
  rework_rounds: 2,
  rework_by_outcome: { changes_requested: 1, rework: 1, conflict: 0 },
  rejected_advances: 1,
  customer_questions: 3,
  quality: null,
  agents: [],
  images: [],
  workflows: [],
  ...over,
});
