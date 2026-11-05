import type { HomeAgentLink } from "@/pages/home/HomePage.view";
import type { AgentRow, AgentView } from "@/types";

/** Complete agent responses; a story overrides only the fields that matter. */

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
  mail_instant_senders: null,
  notes: "",
  ...over,
});

const NAMES = ["builder", "reviewer", "packager", "docs-bot"];

/** Edge sets of agent rows: `empty`, `one`, `many` (50) and `longText`. */
export const agentRows = {
  empty: [] as AgentRow[],
  one: [agentRow({ name: "builder", state: "running" })],
  many: Array.from({ length: 50 }, (_, i) =>
    agentRow({
      name: `${NAMES[i % NAMES.length]}-${i + 1}`,
      state: i % 3 === 0 ? "running" : "stopped",
    }),
  ),
  longText: [
    agentRow({
      name: "release-candidate-packager-for-the-long-running-nightly-build-pipeline",
      alias: "Nightly release candidate packager with a deliberately long display name",
      image: "registry.example.invalid/team/very-long-image-name-for-truncation:2026.11.05-rc1",
      cwd: "/home/user/projects/a-very-long-project-directory/with/several/nested/folders/for/wrapping",
    }),
  ],
};

/** A Home list entry, already formatted by the container. */
export const agentLink = (over: Partial<HomeAgentLink> = {}): HomeAgentLink => ({
  key: "local/worker",
  name: "worker",
  meta: "idle",
  href: "/servers/local/agents/worker/console",
  ...over,
});

/** Edge sets of Home list entries: `empty`, `one`, `many` (50) and `longText`. */
export const agentLinks = {
  empty: [] as HomeAgentLink[],
  one: [agentLink({ key: "local/builder", name: "builder", meta: "12 min" })],
  many: Array.from({ length: 50 }, (_, i) => {
    const name = `${NAMES[i % NAMES.length]}-${i + 1}`;
    return agentLink({
      key: `local/${name}`,
      name,
      meta: i % 3 === 0 ? `${i + 1} min · build-01` : "stopped · build-01",
      href: `/servers/local/agents/${name}/console`,
    });
  }),
  longText: [
    agentLink({
      key: "local/release-candidate-packager",
      name: "release-candidate-packager-for-the-long-running-nightly-build-pipeline",
      meta: "1 h 12 min · a-build-host-with-a-deliberately-long-label",
    }),
  ],
};
