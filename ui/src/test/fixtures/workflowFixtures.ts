import type { Daemon } from "@/lib/daemons";
import type { AgentPool, ScriptRun, Task, WorkflowImageManifest, WorkflowView } from "@/lib/api";

/** Test fixtures for the workflow panel and its sections. */

export const remoteTarget: Daemon = {
  id: "remote",
  label: "remote",
  baseURL: "http://remote",
  token: "t",
};

export const workflowTask: Task = {
  key: "REL-1",
  queue: "REL",
  parent_key: "",
  position: 0,
  priority: "P2",
  title: "Release notes",
  description: "Write them",
  status: "review",
  category: "wait_customer",
  waiting_on: "customer",
  workflow_name: "release",
  workflow_version: "1.2.0",
  workflow_digest: "sha256:abc",
  pull_request: "",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "agent:writer",
  manual_block_reason: "",
  blocked: false,
  revision: 4,
  created_at: "2026-10-01T10:00:00Z",
  updated_at: "2026-10-01T10:00:00Z",
  completed_at: "",
  started_at: "",
};

export const checkRun: ScriptRun = {
  id: 7,
  task_key: "REL-1",
  kind: "check",
  script: "./checks/notes.sh",
  run_as: "queue",
  state: "finished",
  verdict: "reject",
  exit_code: 112,
  message: "the notes must start with a heading",
  created_at: "2026-10-01T10:05:00Z",
  started_at: "2026-10-01T10:05:01Z",
  finished_at: "2026-10-01T10:05:03Z",
  request_id: 11,
};

export const watchRun: ScriptRun = {
  id: 9,
  task_key: "REL-1",
  kind: "watch",
  script: "./scripts/ci.sh",
  run_as: "queue",
  state: "running",
  created_at: "2026-10-01T10:09:00Z",
  started_at: "2026-10-01T10:09:01Z",
};

/** A task in the customer status `review`, with two outcomes. */
export const customerView: WorkflowView = {
  name: "release",
  version: "1.2.0",
  digest: "sha256:abc",
  status: "review",
  category: "wait_customer",
  waiting_on: "customer",
  owner: "customer",
  outcomes: [
    {
      on: "approve",
      to: "publish",
      requires: ["notes"],
      missing: [],
      checks: ["./checks/notes.sh"],
    },
    { on: "rework", to: "draft" },
  ],
  artifacts: [
    {
      id: 3,
      name: "notes",
      value: "# Notes\nline 2",
      author: "agent:writer",
      created_at: "2026-10-01T10:04:00Z",
    },
  ],
  visits: [
    {
      id: 1,
      sequence: 1,
      status: "draft",
      entered_at: "2026-10-01T10:00:00Z",
      entered_by: "user:owner",
      left_at: "2026-10-01T10:04:30Z",
      outcome: "drafted",
      message: "first cut",
    },
    {
      id: 2,
      sequence: 2,
      status: "review",
      entered_at: "2026-10-01T10:04:30Z",
      entered_by: "agent:writer",
    },
  ],
  runs: [watchRun, checkRun],
  declared_artifacts: [
    { name: "notes", description: "The release notes." },
    { name: "summary", description: "" },
    { name: "changelog", description: "Optional changelog entry." },
  ],
  statuses: [
    {
      id: "draft",
      owner: "pool:writers",
      terminal: false,
      initial: true,
      instructions_path: "statuses/draft.md",
      transitions: [
        {
          on: "drafted",
          to: "review",
          requires: ["notes", "summary"],
          checks: [
            {
              script: "./checks/notes.sh",
              description: "The notes start with a heading.",
              run_as: "queue",
              timeout: "30s",
            },
          ],
        },
      ],
      limits: {
        idle_iterations: 3,
        rejected_requests: 5,
        script_failures: 3,
        unavailable_grace: "5m0s",
      },
    },
    {
      id: "review",
      owner: "customer",
      terminal: false,
      transitions: [
        {
          on: "approve",
          to: "publish",
          requires: ["notes"],
          checks: [{ script: "./checks/notes.sh", run_as: "queue", timeout: "1m0s" }],
        },
        { on: "rework", to: "draft" },
      ],
      limits: {
        idle_iterations: 3,
        rejected_requests: 5,
        script_failures: 3,
        unavailable_grace: "5m0s",
      },
    },
    {
      id: "publish",
      owner: "script",
      terminal: false,
      watch: {
        script: "./scripts/ci.sh",
        description: "Waits for the CI of the notes.",
        every: "10s",
        timeout: "30s",
      },
      transitions: [{ on: "published", to: "done" }],
      limits: {
        idle_iterations: 3,
        rejected_requests: 5,
        script_failures: 3,
        unavailable_grace: "5m0s",
      },
    },
    { id: "done", owner: "", terminal: true },
    { id: "dropped", owner: "", terminal: true, cancelled: true },
  ],
};

/** The same task in the pool status `draft`, held by an agent. */
export const poolView: WorkflowView = {
  ...customerView,
  status: "draft",
  category: "in_progress",
  waiting_on: undefined,
  owner: "pool:writers",
  holder: "agent:writer",
  outcomes: [
    {
      on: "drafted",
      to: "review",
      requires: ["notes", "summary"],
      missing: ["summary"],
      checks: ["./checks/notes.sh"],
    },
  ],
};

/** A development workflow: two pools, a customer approval, and a terminal status. */
export const developmentManifest: WorkflowImageManifest = {
  schema_version: 1,
  name: "development",
  version: "0.3.0",
  digest: "9232ab7783af091984a1e2f57292bbb2",
  built_at: "",
  files: [],
  definition: {
    schema_version: 1,
    name: "development",
    workflow_version: "0.3.0",
    initial_status: "plan",
    statuses: [
      {
        id: "plan",
        owner: { kind: "pool", pool: "developers" },
        transitions: [{ on: "planned", to: "approval" }],
      },
      {
        id: "approval",
        owner: { kind: "customer" },
        transitions: [
          { on: "approved", to: "review" },
          { on: "changes_requested", to: "plan" },
        ],
      },
      {
        id: "review",
        owner: { kind: "pool", pool: "reviewers" },
        transitions: [{ on: "merged", to: "done" }],
      },
      { id: "done", owner: { kind: "" }, terminal: true },
    ],
  },
};

export const agentPool = (
  name: string,
  agents: string[],
  revision = 1,
  queue = "DEV",
): AgentPool => ({ id: revision, queue, name, agents, revision, created_at: "", updated_at: "" });
