import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { taskItems } from "@/fixtures/tasks";
import type { MarkdownMode } from "@/pages/tasks/TaskMarkdown";
import { requiredBy, stepArtifacts } from "@/pages/tasks/workflowGraph";
import { withWidth } from "@/storybook/decorators";
import type { TaskDetail, WorkflowView } from "@/types";
import {
  AGENTS,
  EVENTS,
  agentWorksView,
  needsYouView,
  pausedView,
  plainDetail,
  rejectedView,
  reviewDetail,
  workflowDetail,
} from "./drawerData";
import type { ChipField } from "./PropertyChips.view";
import { TaskDrawerBody } from "./TaskDrawerBody.view";
import { TaskDrawerHeader } from "./TaskDrawerHeader.view";
import { TaskDrawerMore } from "./TaskDrawerMore.view";

/**
 * Purpose: the task drawer shows one task where it was mentioned and lets the
 * customer steer it.
 *
 * User: the operator who runs agents on tasks, most of them in a workflow.
 *
 * Task: open a task, see where it is in its workflow and whose turn it is,
 * change a parameter (priority, agent, budget), send the task back or move it,
 * decide an outcome, then read the description and the comments.
 *
 * Success: the workflow, its actions and the parameters are on the first
 * screen in every state, with no scroll and no section to open. The decision
 * block keeps its place under them when the task waits for the customer.
 */

// Today the parameters are in a closed Details section and the workflow in a
// closed Workflow section, both under the comments. The mockup moves them into
// the sticky header as two rows and keeps the log at the bottom. The panel is
// drawn as the app docks it: half the window at the right edge, and the whole
// window at 1000 px and below.

const meta = {
  title: "Mockups/TaskDrawer",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const FULL_WINDOW_BELOW = 1000;

interface Scene {
  detail: TaskDetail;
  view?: WorkflowView;
  /** The description is in the editor. */
  editing?: boolean;
  /** The step whose description is open. */
  step?: string;
  /** The story fixes a block open or closed instead of the remembered choice. */
  details?: boolean;
  workflow?: boolean;
}

// The app keeps the two choices for all tasks in the browser storage. The
// mockup keeps them for the Storybook session: close a block in one story and
// it is closed in the next, unless that story fixes it.
const remembered = { details: false, workflow: true };

function useBlockOpen(block: keyof typeof remembered, fixed: boolean | undefined) {
  const [open, setOpen] = useState(fixed ?? remembered[block]);
  const change = (next: boolean) => {
    remembered[block] = next;
    setOpen(next);
  };
  return [open, change] as const;
}

/** The artifacts the customer's decision needs are open from the start. */
function decisionArtifacts(view: WorkflowView | undefined): string[] {
  if (!view || view.owner !== "customer") return [];
  return [
    ...new Set([...requiredBy(view).keys(), ...stepArtifacts(view, view.status).enter.keys()]),
  ];
}

/** The task list behind the drawer, dimmed as the sheet dims it. */
function Backdrop() {
  return (
    <div aria-hidden="true" className="absolute inset-0 overflow-hidden px-4 pt-14">
      {taskItems.many.slice(0, 24).map((task) => (
        <div key={task.key} className="flex h-7.5 items-center gap-3 text-body">
          <span className="w-16 font-mono text-meta text-muted-foreground">{task.key}</span>
          <span className="truncate">{task.title}</span>
        </div>
      ))}
      <div className="absolute inset-0 bg-foreground/7" />
    </div>
  );
}

function DrawerMockup({ width, scene }: { width: number; scene: Scene }) {
  const { detail, view } = scene;
  const [selected, setSelected] = useState<string | null>(scene.step ?? null);
  const [detailsOpen, setDetailsOpen] = useBlockOpen("details", scene.details);
  const [workflowOpen, setWorkflowOpen] = useBlockOpen("workflow", scene.workflow);
  const [chip, setChip] = useState<ChipField | null>(null);
  const [dirty, setDirty] = useState(!!scene.editing);
  const [editing, setEditing] = useState(!!scene.editing);
  const [description, setDescription] = useState(detail.task.description);
  const [mode, setMode] = useState<MarkdownMode>("rich");
  const [message, setMessage] = useState("");
  const [comment, setComment] = useState("");
  const [open, setOpen] = useState<ReadonlySet<string>>(
    () => new Set(decisionArtifacts(view).slice(0, 1)),
  );
  const touch = () => setDirty(true);
  const toggleArtifact = (name: string) =>
    setOpen((current) => {
      const next = new Set(current);
      if (!next.delete(name)) next.add(name);
      return next;
    });
  const showArtifact = (name: string) => {
    setOpen((current) => new Set(current).add(name));
    document.getElementById(`artifact-${name}`)?.scrollIntoView({ block: "nearest" });
  };
  const full = width <= FULL_WINDOW_BELOW;

  return (
    <div className="relative h-screen overflow-hidden">
      {!full && <Backdrop />}
      <div
        role="dialog"
        aria-label={`Task ${detail.task.key}`}
        className={
          full
            ? "absolute inset-0 overflow-auto bg-card"
            : "absolute top-[50px] right-2 bottom-2 w-1/2 overflow-auto rounded-[var(--panel-radius)] bg-card shadow-[var(--lift)]"
        }
      >
        <TaskDrawerHeader
          task={detail.task}
          parent={detail.parent}
          view={view}
          dirty={dirty}
          chips={{
            agents: AGENTS,
            budget: { spent_usd: 4.2, budget_usd: detail.task.budget_usd, exhausted: false },
            editing: chip,
            onEdit: setChip,
            onStatus: touch,
            onPriority: touch,
            onAssignee: touch,
            onBudget: touch,
            onPullRequest: touch,
          }}
          detailsOpen={detailsOpen}
          onDetailsOpenChange={setDetailsOpen}
          details={{ relations: detail.relations, onOpenTask: () => {} }}
          strip={{
            selected,
            onSelect: setSelected,
            open: workflowOpen,
            onOpenChange: setWorkflowOpen,
            onAction: () => {},
          }}
          decision={{
            message,
            onMessage: setMessage,
            onAdvance: () => {},
            onShowArtifact: showArtifact,
            onResume: () => {},
            onCancel: () => {},
          }}
          onSave={() => {
            setDirty(false);
            setEditing(false);
          }}
          onClose={() => {}}
          onEditTitle={() => {}}
          onOpenParent={() => {}}
          onRequeue={() => {}}
          onTransfer={() => {}}
        />
        <div className="flex min-w-0 flex-col gap-4.5 px-4 pt-3.5 pb-7">
          <TaskDrawerBody
            detail={detail}
            view={view}
            openArtifacts={open}
            onToggleArtifact={toggleArtifact}
            description={{
              editing,
              value: description,
              mode,
              onEdit: () => setEditing(true),
              onCancel: () => {
                setEditing(false);
                setDescription(detail.task.description);
              },
              onChange: (value) => {
                setDescription(value);
                touch();
              },
              onModeChange: setMode,
            }}
            comment={comment}
            onCommentChange={setComment}
            onComment={() => setComment("")}
            onOpenTask={() => {}}
            onProblemAction={() => {}}
          />
          <TaskDrawerMore view={view} events={EVENTS} metrics="$4.20 · 6 runs · 1 rework" />
        </div>
      </div>
    </div>
  );
}

const story = (width: number, scene: Scene): Story => ({
  decorators: [withWidth(width)],
  render: () => <DrawerMockup width={width} scene={scene} />,
});

const NEEDS_YOU: Scene = { detail: reviewDetail, view: needsYouView };

// The states, at the widest supported width: the drawer is half the window.

/** The agent works: no decision block, the workflow row and the chips stay. */
export const AgentWorks = story(1180, { detail: workflowDetail, view: agentWorksView });

/** The customer's turn: the decision block opens under the workflow row. */
export const NeedsYou = story(1180, NEEDS_YOU);

/** The workflow is paused: the pause and its ways out take the decision place. */
export const Paused = story(1180, { detail: workflowDetail, view: pausedView });

/** A check rejected the request: the row says so, the body opens with the reason. */
export const CheckRejected = story(1180, { detail: workflowDetail, view: rejectedView });

/** A step is open: what the status needs and each way out of it. */
export const StepOpen = story(1180, {
  detail: workflowDetail,
  view: agentWorksView,
  step: "implement",
  workflow: true,
});

/** The description in the editor, after Edit: Save task appears in the header. */
export const EditingDescription = story(1180, {
  detail: workflowDetail,
  view: agentWorksView,
  editing: true,
});

/** Both blocks closed: two rows under the title, the body starts higher. */
export const BlocksClosed = story(1180, {
  detail: workflowDetail,
  view: agentWorksView,
  details: false,
  workflow: false,
});

/** Both blocks open: the rare fields and the relations, and every step. */
export const BlocksOpen = story(1180, {
  detail: workflowDetail,
  view: agentWorksView,
  details: true,
  workflow: true,
});

/** A task without a workflow: the status is a chip with a menu, no workflow row. */
export const PlainTask = story(1180, { detail: plainDetail });

// The customer's turn at each supported width.

export const Width360 = story(360, NEEDS_YOU);
export const Width390 = story(390, NEEDS_YOU);
export const Width820 = story(820, NEEDS_YOU);
export const Width1024 = story(1024, NEEDS_YOU);
export const Width1180 = story(1180, NEEDS_YOU);
