import type { ComponentProps } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { taskItem } from "@/fixtures/tasks";
import { withWidth } from "@/storybook/decorators";
import TaskDetail from "./TaskDetail";

// The task panel TasksWorkspace docks on the right when a row is selected. The
// panel is a Radix Dialog: its content portals to document.body and is
// `position: fixed`, docked to the right edge of the viewport, so the stories
// mount it bare and let it dock, as in the app. `width` is the panel width the
// workspace's resize handle drives.
//
// Only the resize handle is scaffolding: TasksWorkspace passes its own handle
// into the `resizeHandle` slot, so the stories pass a plain element with the
// shipped handle class. `display: block` is forced because tasks.css hides the
// handle under 1000px, which would leave the two-column panel 4px wide.
const ResizeHandle = (
  <div className="task-panel-resize-handle block" aria-hidden="true" style={{ display: "block" }} />
);

const PRINCIPALS = {
  customer: "operator",
  agents: ["builder", "reviewer", "packager", "docs-bot"],
  groups: ["core", "release"],
};

type Props = ComponentProps<typeof TaskDetail>;
type Detail = Props["detail"];

const BASE_TASK: Detail["task"] = taskItem({
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
  category: "in_progress",
  pull_request: "https://github.com/alekzonder/tariboy/pull/318",
  author: "agent:builder",
  customer: "operator",
  group: "core",
  assignee: "agent:packager",
  revision: 7,
  created_at: "2026-09-08T09:12:00Z",
  updated_at: "2026-09-11T16:40:00Z",
});

const COMMENTS: Detail["comments"] = [
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

const RELATIONS: Detail["relations"] = [
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
    created_at: "2026-09-09T10:00:00Z",
  },
  {
    id: 9,
    source_key: "TB-139",
    source_title: "Template writer ignores an empty plugins block",
    source_status: "done",
    target_key: "TB-142",
    target_title: "Packager drops skills when the image template declares no plugins",
    target_status: "in_progress",
    type: "related",
    created_by: "operator",
    created_at: "2026-09-09T10:02:00Z",
  },
];

const event = (
  sequence: number,
  kind: string,
  actor: string,
  task_revision: number,
  created_at: string,
): NonNullable<Props["events"]>[number] => ({
  sequence,
  event_id: `evt-${sequence}`,
  task_key: "TB-142",
  queue: "TB",
  kind,
  actor,
  task_revision,
  payload: {},
  created_at,
});

const EVENTS: Props["events"] = [
  event(210, "task.created", "agent:builder", 1, "2026-09-08T09:12:00Z"),
  event(233, "task.assigned", "operator", 4, "2026-09-09T09:58:00Z"),
  event(251, "task.status_changed", "agent:packager", 7, "2026-09-11T16:40:00Z"),
];

const noop = async () => {};
const saveTask = async () => BASE_TASK;

const Panel = ({ detail, width = 820 }: { detail: Detail; width?: number }) => (
  <TaskDetail
    detail={detail}
    principals={PRINCIPALS}
    events={EVENTS}
    width={width}
    resizeHandle={ResizeHandle}
    onClose={() => {}}
    onSave={saveTask}
    onComment={noop}
    onAddRelation={noop}
    onDeleteRelation={noop}
    onTransfer={noop}
    onRequeue={noop}
  />
);

const detailOf = (task: Detail["task"]): Detail => ({
  task,
  comments: COMMENTS,
  waiting_for: [],
  relations: RELATIONS,
  descendants: 0,
  active_descendants: 0,
  children: [],
  hidden_children: 0,
});

const meta = {
  component: TaskDetail,
  decorators: [withWidth(1024)],
  parameters: { layout: "fullscreen", tariboy: { daemon: true, query: true } },
} satisfies Meta<typeof TaskDetail>;
export default meta;

type Story = StoryObj;

export const OpenTask: Story = {
  render: () => <Panel detail={detailOf(BASE_TASK)} />,
};

// A workflow task: the status select gives way to the Workflow panel, which
// loads the workflow itself through the surrounding ApiProvider.
export const ManagedWorkflow: Story = {
  render: () => (
    <Panel
      detail={detailOf({
        ...BASE_TASK,
        key: "TB-151",
        title: "Ship the skills-tree validator with worker:v2",
        description: "Blocked on TB-142. Ship the validator case with the next `worker:v2` build.",
        parent_key: "",
        status: "implement",
        workflow_name: "bugfix",
        workflow_version: "3",
      })}
    />
  ),
};
