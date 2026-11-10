import { taskItem } from "@/fixtures/tasks";
import { hoursAgo, minutesAgo } from "@/fixtures/time";
import type {
  Artifact,
  ScriptRun,
  StatusView,
  StatusVisit,
  TaskDetail,
  TaskEvent,
  WorkflowView,
} from "@/types";

// The values of the task drawer mockup: one task of the `bugfix` workflow in
// each state the drawer has to explain, and one task without a workflow.

const LIMITS = {
  idle_iterations: 3,
  rejected_requests: 5,
  script_failures: 3,
  unavailable_grace: "5m0s",
};

const STATUSES: StatusView[] = [
  {
    id: "plan",
    owner: "pool:builders",
    terminal: false,
    initial: true,
    instructions_path: "statuses/plan.md",
    limits: LIMITS,
    transitions: [{ on: "planned", to: "plan_review", requires: ["plan"] }],
  },
  {
    id: "plan_review",
    owner: "customer",
    terminal: false,
    transitions: [
      { on: "approve", to: "implement", requires: ["plan"] },
      { on: "rework", to: "plan" },
    ],
  },
  {
    id: "implement",
    owner: "pool:builders",
    terminal: false,
    instructions_path: "statuses/implement.md",
    limits: LIMITS,
    transitions: [
      {
        on: "done",
        to: "review",
        requires: ["summary"],
        checks: [
          {
            script: "./checks/tests.sh",
            description: "The unit tests of the changed packages pass.",
            run_as: "queue",
            timeout: "10m0s",
          },
        ],
      },
    ],
  },
  {
    id: "review",
    owner: "customer",
    terminal: false,
    transitions: [
      { on: "approve", to: "done", requires: ["summary"] },
      { on: "rework", to: "implement" },
    ],
  },
  { id: "done", owner: "", terminal: true },
];

const PLAN: Artifact = {
  id: 1,
  name: "plan",
  author: "agent:packager",
  created_at: hoursAgo(20),
  value: [
    "### Plan",
    "",
    "1. Add a validator case: an image template with an empty `plugins:` block still ships the skills tree.",
    "2. Fix the short-circuit in the template writer.",
    "3. Fail the build when the built template has `skills: []` and the source has skills.",
    "",
    "Risk: `worker:v1` images built before the fix keep the empty tree until rebuilt.",
  ].join("\n"),
};

const SUMMARY: Artifact = {
  id: 2,
  name: "summary",
  author: "agent:packager",
  created_at: minutesAgo(14),
  value: [
    "### What changed",
    "",
    "- `internal/packager/template.go`: the writer no longer returns early on an empty `plugins` list.",
    "- `internal/packager/validate.go`: a build fails when the skills tree is dropped.",
    "- 4 new test cases, all green.",
    "",
    "Pull request: PR 318. Rebuild `worker:v2` after the merge.",
  ].join("\n"),
};

const visit = (sequence: number, status: string, over: Partial<StatusVisit> = {}): StatusVisit => ({
  id: sequence,
  sequence,
  status,
  entered_at: hoursAgo(26 - sequence * 5),
  entered_by: "agent:packager",
  ...over,
});

const VISITS_TO_IMPLEMENT: StatusVisit[] = [
  visit(1, "plan", { entered_by: "operator", left_at: hoursAgo(20), outcome: "planned" }),
  visit(2, "plan_review", { left_at: hoursAgo(19), outcome: "approve", message: "Go ahead." }),
  visit(3, "implement", { entered_by: "operator", entered_at: hoursAgo(19) }),
];

const TESTS_RUN: ScriptRun = {
  id: 12,
  task_key: "TB-142",
  kind: "check",
  script: "./checks/tests.sh",
  run_as: "queue",
  state: "finished",
  verdict: "reject",
  exit_code: 112,
  message: "3 tests fail in internal/packager: TestTemplateKeepsSkills, TestValidateEmptyPlugins",
  created_at: minutesAgo(9),
  started_at: minutesAgo(9),
  finished_at: minutesAgo(7),
  request_id: 31,
};

const baseView: WorkflowView = {
  name: "bugfix",
  version: "3",
  digest: "sha256:9ac71e3f52",
  status: "implement",
  category: "in_progress",
  owner: "pool:builders",
  holder: "agent:packager",
  outcomes: [{ on: "done", to: "review", requires: ["summary"], missing: ["summary"] }],
  artifacts: [PLAN],
  declared_artifacts: [
    { name: "plan", description: "What the agent is going to change." },
    { name: "summary", description: "What changed, for the review." },
  ],
  statuses: STATUSES,
  visits: VISITS_TO_IMPLEMENT,
  runs: [],
};

/** The agent works in `implement`; nothing is asked of the customer. */
export const agentWorksView: WorkflowView = baseView;

/** `review` belongs to the customer: two outcomes and the artifacts to read. */
export const needsYouView: WorkflowView = {
  ...baseView,
  status: "review",
  category: "wait_customer",
  waiting_on: "customer",
  owner: "customer",
  holder: undefined,
  outcomes: [
    { on: "approve", to: "done", requires: ["summary"], missing: [] },
    { on: "rework", to: "implement" },
  ],
  artifacts: [PLAN, SUMMARY],
  visits: [
    ...VISITS_TO_IMPLEMENT.slice(0, 2),
    visit(3, "implement", { entered_at: hoursAgo(19), left_at: minutesAgo(14), outcome: "done" }),
    visit(4, "review", { entered_at: minutesAgo(14) }),
  ],
  runs: [{ ...TESTS_RUN, verdict: "pass", exit_code: 0, message: "ok" }],
};

/** The holder idled to the limit: the workflow is paused. */
export const pausedView: WorkflowView = {
  ...baseView,
  waiting_on: "pause",
  paused_reason: "idle_iterations",
};

/** A check rejected the agent's request to leave `implement`. */
export const rejectedView: WorkflowView = {
  ...baseView,
  artifacts: [PLAN, SUMMARY],
  outcomes: [{ on: "done", to: "review", requires: ["summary"], missing: [] }],
  visits: [
    ...VISITS_TO_IMPLEMENT.slice(0, 2),
    visit(3, "implement", { entered_at: hoursAgo(19), rejected_requests: 2 }),
  ],
  runs: [TESTS_RUN],
  last_request: {
    id: 31,
    task_key: "TB-142",
    actor: "agent:packager",
    outcome: "done",
    state: "rejected",
    created_at: minutesAgo(9),
    finished_at: minutesAgo(7),
    result_message:
      "3 tests fail in internal/packager: TestTemplateKeepsSkills, TestValidateEmptyPlugins, TestValidateSkillsTree",
  },
};

const DESCRIPTION = [
  "`packager` builds `worker:v2` from `/home/agent/github/tariboy` with an empty",
  "`plugins:` block and the skills tree never reaches the archive.",
  "",
  "### Repro",
  "",
  "1. Build `worker:v2` from the directory above.",
  "2. Inspect the built template — `skills` is `[]`.",
  "3. `builder` starts on the image and cannot resolve `code-review`.",
  "",
  "### Expected",
  "",
  "The validator fails the build instead of shipping a silently empty skills tree.",
].join("\n");

const workflowTask = taskItem({
  key: "TB-142",
  queue: "TB",
  parent_key: "TB-140",
  priority: "P1",
  title: "Packager drops skills when the image template declares no plugins",
  description: DESCRIPTION,
  status: "implement",
  category: "in_progress",
  workflow_name: "bugfix",
  workflow_version: "3",
  pull_request: "https://github.com/alekzonder/tariboy/pull/318",
  author: "agent:builder",
  customer: "operator",
  group: "core",
  assignee: "agent:packager",
  budget_usd: 10,
  revision: 7,
  created_at: hoursAgo(26),
  updated_at: minutesAgo(14),
});

const comment = (id: number, author: string, body: string, created_at: string) => ({
  id,
  task_key: "TB-142",
  author,
  body,
  revision: 1,
  created_at,
  updated_at: created_at,
});

const detailOf = (task: TaskDetail["task"]): TaskDetail => ({
  task,
  parent: { key: "TB-140", queue: "TB", title: "Release worker:v2", access: "full" },
  comments: [
    comment(
      44,
      "agent:packager",
      "Root cause: the template writer short-circuits on an empty `plugins` list.\nFix drafted in PR 318, adding a validator case.",
      hoursAgo(3),
    ),
    comment(
      41,
      "operator",
      "Reproduced on build-01 as well — it is not local to this daemon.",
      hoursAgo(22),
    ),
  ],
  waiting_for: [],
  relations: [
    {
      id: 8,
      source_key: "TB-142",
      source_title: "Packager drops skills when the image template declares no plugins",
      source_status: "in_progress",
      target_key: "TB-151",
      target_title: "Ship the skills-tree validator with worker:v2",
      target_status: "open",
      type: "blocks",
      created_by: "agent:builder",
      created_at: hoursAgo(25),
    },
  ],
  descendants: 2,
  active_descendants: 1,
  children: [
    taskItem({
      key: "TB-143",
      queue: "TB",
      parent_key: "TB-142",
      title: "Validator case for an empty plugins block",
      status: "done",
      category: "done",
      assignee: "agent:packager",
    }),
    taskItem({
      key: "TB-144",
      queue: "TB",
      parent_key: "TB-142",
      title: "Rebuild worker:v2 on build-01",
      status: "open",
      category: "open",
      assignee: "agent:builder",
    }),
  ],
  hidden_children: 0,
});

export const workflowDetail = detailOf(workflowTask);

export const reviewDetail = detailOf({
  ...workflowTask,
  status: "review",
  category: "wait_customer",
  waiting_on: "customer",
});

/** A task without a workflow: its status is a field the customer sets. */
export const plainDetail = detailOf({
  ...workflowTask,
  status: "in_progress",
  workflow_name: undefined,
  workflow_version: undefined,
});

const event = (sequence: number, kind: string, actor: string, created_at: string): TaskEvent => ({
  sequence,
  event_id: `evt-${sequence}`,
  task_key: "TB-142",
  queue: "TB",
  kind,
  actor,
  task_revision: sequence - 209,
  payload: {},
  created_at,
});

export const EVENTS: TaskEvent[] = [
  event(210, "task.created", "agent:builder", hoursAgo(26)),
  event(211, "task.assigned", "operator", hoursAgo(25)),
  event(214, "workflow.advanced", "agent:packager", hoursAgo(20)),
  event(215, "workflow.advanced", "operator", hoursAgo(19)),
  event(223, "task.commented", "agent:packager", hoursAgo(3)),
];

export const AGENTS = ["builder", "reviewer", "packager", "docs-bot"];
