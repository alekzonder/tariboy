import { useEffect } from "react";
import { TaskDetail } from "tariboy-ui";

// TaskDetail is the task panel TasksWorkspace docks on the right when a row is
// selected: sticky header (Back · key · title · Save task · close), the metadata
// strip, the editable title/description/properties fieldset, dependencies,
// comments and the event history. It is fully prop-driven — no fetching of its
// own — so unlike the other page surfaces it renders with REAL data here.
//
// One structural note for anyone reusing this preview: the panel is a Radix
// Dialog, so its content portals to document.body and is `position: fixed`,
// docked to the right edge of the VIEWPORT (tasks.css: `top/bottom: 0;
// width: var(--tasks-detail-width); height: 100dvh`). A wrapper frame cannot
// contain it, so these stories mount it bare and let it dock, which is exactly
// how it behaves in the app. `width` is the panel width the workspace's resize
// handle drives; the shipped max-width is `calc(100vw - 80px)`.
//
// Only the resize handle is scaffolding: TasksWorkspace passes its own
// TaskPanelResizeHandle into the `resizeHandle` slot, which is internal, so
// these stories pass a plain element carrying the shipped handle class.
// `display: block` is forced because tasks.css hides the handle under 1000px —
// and the panel is a two-column grid (`4px minmax(0, 1fr)`), so a hidden handle
// leaves the panel itself occupying the 4px track. The capture viewport is
// 900px, so without this the whole panel renders four pixels wide.
const ResizeHandle = (
  <div className="task-panel-resize-handle" aria-hidden="true" style={{ display: "block" }} />
);

const PRINCIPALS = {
  customer: "operator",
  agents: ["builder", "reviewer", "packager", "docs-bot"],
  groups: ["core", "release"],
};

const BASE_TASK = {
  key: "TB-142",
  queue: "TB",
  parent_key: "TB-140",
  position: 3,
  priority: "P1",
  title: "Packager drops skills when the image template declares no plugins",
  description: [
    "`packager` builds `worker:v2` from `/home/agent/github/tariboy` with an empty",
    "`plugins:` block and the skills tree never reaches the archive.",
    "",
    "### Repro",
    "",
    "1. Build `worker:v2` from the directory above.",
    "2. Inspect the built template — `skills` is `[]`.",
    "3. `builder` starts on the image and cannot resolve `code-review`.",
    "",
    "The validator should fail the build instead of shipping a silently empty",
    "skills tree.",
  ].join("\n"),
  status: "in_progress",
  pull_request: "https://github.com/alekzonder/tariboy/pull/318",
  author: "agent:builder",
  customer: "operator",
  group: "core",
  assignee: "agent:packager",
  manual_block_reason: "",
  blocked: false,
  revision: 7,
  created_at: "2026-09-08T09:12:00Z",
  updated_at: "2026-09-11T16:40:00Z",
  completed_at: "",
};

const COMMENTS = [
  {
    id: 41,
    task_key: "TB-142",
    author: "operator",
    body: "Reproduced on build-01 as well — it is not local to this daemon.",
    revision: 1,
    created_at: "2026-09-09T08:20:00Z",
    updated_at: "2026-09-09T08:20:00Z",
  },
  {
    id: 44,
    task_key: "TB-142",
    author: "agent:packager",
    body: "Root cause: the template writer short-circuits on an empty `plugins` list.\nFix drafted in PR 318, adding a validator case.",
    revision: 1,
    created_at: "2026-09-10T13:05:00Z",
    updated_at: "2026-09-10T13:05:00Z",
  },
];

const RELATIONS = [
  {
    id: 8,
    source_key: "TB-142",
    target_key: "TB-151",
    type: "blocks",
    created_by: "agent:builder",
    created_at: "2026-09-09T10:00:00Z",
  },
  {
    id: 9,
    source_key: "TB-139",
    target_key: "TB-142",
    type: "related",
    created_by: "operator",
    created_at: "2026-09-09T10:02:00Z",
  },
];

const EVENTS = [
  {
    sequence: 210,
    event_id: "evt-210",
    task_key: "TB-142",
    queue: "TB",
    kind: "task.created",
    actor: "agent:builder",
    task_revision: 1,
    payload: {},
    created_at: "2026-09-08T09:12:00Z",
  },
  {
    sequence: 233,
    event_id: "evt-233",
    task_key: "TB-142",
    queue: "TB",
    kind: "task.assigned",
    actor: "operator",
    task_revision: 4,
    payload: {},
    created_at: "2026-09-09T09:58:00Z",
  },
  {
    sequence: 251,
    event_id: "evt-251",
    task_key: "TB-142",
    queue: "TB",
    kind: "task.status_changed",
    actor: "agent:packager",
    task_revision: 7,
    payload: {},
    created_at: "2026-09-11T16:40:00Z",
  },
];

const WORKFLOW = {
  workflow: { name: "bugfix", version: "3" },
  status_executions: [{ id: 1, state: "running" }],
  assignments: [
    { id: 11, agent: "packager", state: "claimed", attempt: 1, outcome: "" },
    { id: 12, agent: "", state: "queued", attempt: 0, outcome: "" },
    { id: 13, agent: "reviewer", state: "done", attempt: 1, outcome: "approved" },
  ],
  holds: [
    { id: 3, reason: "Waiting on release freeze", scope: "queue:TB", released_at: null },
  ],
  observations: [
    { id: 5, kind: "build.failed", payload: { image: "worker:v2", code: 2 } },
  ],
};

const ARTIFACTS = [
  { id: 21, name: "worker:v2 template", type: "template", content: "sha256:9ac71e3f52…" },
  { id: 22, name: "build log", type: "log", content: "312 lines · 48 KB" },
];

const QUESTIONS = [
  {
    id: 31,
    question: "Should an empty skills tree fail the build or only warn?",
    state: "open",
    blocking_scope: "task",
  },
];

const noop = async () => {};
const saveTask = async () => BASE_TASK;

const Panel = ({ detail, width = 820, ...rest }: Record<string, unknown> & {
  detail: unknown;
  width?: number;
}) => (
  <TaskDetail
    detail={detail}
    target={null}
    principals={PRINCIPALS}
    events={EVENTS}
    workflow={null}
    workflowArtifacts={[]}
    workflowQuestions={[]}
    executionLoading={false}
    executionError=""
    artifactsLoading={false}
    artifactsError=""
    questionsLoading={false}
    questionsError=""
    width={width}
    resizeHandle={ResizeHandle}
    onClose={() => {}}
    onSave={saveTask}
    onComment={noop}
    onAddRelation={noop}
    onDeleteRelation={noop}
    {...rest}
  />
);

export const OpenTask = () => (
  <Panel detail={{ task: BASE_TASK, comments: COMMENTS, waiting_for: [], relations: RELATIONS }} />
);

// A managed task's distinguishing surface — the "Managed workflow" section with
// its assignment, hold, artifact, question and observation lists — sits below
// the description, past the fold of a 700px capture. The panel scrolls for
// real, so scroll it there: the header is sticky, so the card still shows the
// task identity above the section. `.task-workflow` exists only on the managed
// story, so querying the document cannot reach a neighbouring cell.
function useScrollToWorkflow() {
  useEffect(() => {
    const timer = window.setTimeout(() => {
      for (const section of document.querySelectorAll(".task-workflow")) {
        section.scrollIntoView({ block: "start" });
        // The panel header is sticky: back off so it does not cover the
        // section's own title and version/status strip.
        const panel = section.closest(".task-detail-panel");
        if (panel) panel.scrollTop -= 100;
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, []);
}

export const ManagedWorkflow = () => {
  useScrollToWorkflow();
  return (
    <Panel
      detail={{
        task: {
          ...BASE_TASK,
          key: "TB-151",
          title: "Ship the skills-tree validator with worker:v2",
          description: "Blocked on TB-142. Ship the validator case with the next `worker:v2` build.",
          parent_key: "",
          workflow_version_id: 3,
          workflow_version: "bugfix@3",
          workflow_status: "running",
          workflow_revision: 12,
        },
        comments: COMMENTS,
        waiting_for: [],
        relations: RELATIONS,
      }}
      workflow={WORKFLOW}
      workflowArtifacts={ARTIFACTS}
      workflowQuestions={QUESTIONS}
    />
  );
};
