import type { ReactElement } from "react";
import { cleanup, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import TaskDetail from "./TaskDetail";
import {
  ApiError,
  type PrincipalInfo,
  type Task,
  type TaskComment,
  type TaskDetail as Detail,
  type TaskEvent,
  type WaitingFor,
} from "@/lib/api";
import { agentPool, customerView, poolView, workflowTask } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({
  getTaskWorkflow: vi.fn(),
  resumeTaskWorkflow: vi.fn(),
  listAgentPools: vi.fn(),
  reassignTaskWorkflow: vi.fn(),
}));
const localApi = fakeApi({
  tasks: { listAgentPools: api.listAgentPools },
  workflows: {
    getTaskWorkflow: api.getTaskWorkflow,
    resumeTaskWorkflow: api.resumeTaskWorkflow,
    reassignTaskWorkflow: api.reassignTaskWorkflow,
  },
});
const render = (ui: ReactElement) => renderWith(ui, { api: localApi });

const flexible: Task = {
  ...workflowTask,
  key: "TEST-1",
  queue: "TEST",
  title: "Flexible task",
  status: "in_progress",
  category: "in_progress",
  waiting_on: undefined,
  workflow_name: undefined,
  workflow_version: undefined,
  workflow_digest: undefined,
  assignee: "agent:worker",
};

beforeEach(() => {
  vi.clearAllMocks();
  api.getTaskWorkflow.mockResolvedValue(customerView);
});

function renderDetail(
  task: Task,
  waiting_for: WaitingFor[] = [],
  comments: TaskComment[] = [],
  principals: PrincipalInfo | null = null,
) {
  const detail: Detail = {
    task,
    comments,
    waiting_for,
    relations: [],
    descendants: 0,
    active_descendants: 0,
  };
  const onSave = vi.fn().mockResolvedValue(task);
  const onTaskChanged = vi.fn();
  const onClose = vi.fn();
  const element = (events: TaskEvent[]) => (
    <TaskDetail
      detail={detail}
      events={events}
      principals={principals}
      width={600}
      resizeHandle={null}
      onClose={onClose}
      onSave={onSave}
      onComment={vi.fn()}
      onAddRelation={vi.fn()}
      onDeleteRelation={vi.fn()}
      onTransfer={vi.fn()}
      onRequeue={vi.fn()}
      onTaskChanged={onTaskChanged}
    />
  );
  const { rerender } = render(element([]));
  return {
    onSave,
    onTaskChanged,
    onClose,
    rerenderEvents: (events: TaskEvent[]) => rerender(element(events)),
  };
}

const event = (sequence: number, kind: string): TaskEvent => ({
  sequence,
  event_id: `e${sequence}`,
  task_key: "REL-1",
  queue: "REL",
  kind,
  actor: "system:workflow",
  task_revision: 4,
  payload: {},
  created_at: "2026-10-01T10:07:00Z",
});

it("refetches the workflow view when a workflow event arrives without a new revision", async () => {
  const { rerenderEvents } = renderDetail(workflowTask);
  await screen.findByText("release@1.2.0");
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(1);
  rerenderEvents([event(9, "workflow.transition_rejected")]);
  await waitFor(() => expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2));
});

it("offers Move to another server and Move to queue for an open flexible task", async () => {
  renderDetail(flexible);
  await userEvent.click(screen.getByRole("button", { name: "Task actions" }));
  expect(
    await screen.findByRole("menuitem", { name: "Move to another server…" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("menuitem", { name: "Move to queue…" })).toBeInTheDocument();
});

it("offers only Move to queue for a workflow task", async () => {
  renderDetail(workflowTask);
  await screen.findByText("release@1.2.0");
  await userEvent.click(screen.getByRole("button", { name: "Task actions" }));
  expect(await screen.findByRole("menuitem", { name: "Move to queue…" })).toBeInTheDocument();
  expect(screen.queryByText("Move to another server…")).not.toBeInTheDocument();
});

it("offers no Move to queue for a closed task", async () => {
  renderDetail({ ...flexible, status: "done", category: "done" });
  await userEvent.click(screen.getByRole("button", { name: "Task actions" }));
  expect(
    await screen.findByRole("menuitem", { name: "Move to another server…" }),
  ).toBeInTheDocument();
  expect(screen.queryByRole("menuitem", { name: "Move to queue…" })).not.toBeInTheDocument();
});

it("puts no waiting chip on the workflow's own question, but keeps it on an agent's", async () => {
  const comment = (id: number, author: string, body: string): TaskComment => ({
    id,
    task_key: "REL-1",
    author,
    body,
    revision: 1,
    created_at: "2026-10-01T10:05:00Z",
    updated_at: "2026-10-01T10:05:00Z",
  });
  const agentWait = { ...wait("agent:writer"), id: 2, requesting_comment_id: 4 };
  renderDetail(
    workflowTask,
    [wait("system:workflow"), agentWait],
    [
      comment(3, "system:workflow", "Approve the release?"),
      comment(4, "agent:writer", "Which date?"),
    ],
  );
  await screen.findByText("release@1.2.0");
  const chipOn = (text: string) =>
    within(screen.getByText(text).closest("article")!).queryByText("waiting for answer");
  expect(chipOn("Approve the release?")).not.toBeInTheDocument();
  expect(chipOn("Which date?")).toBeInTheDocument();
  cleanup();
  renderDetail(
    flexible,
    [wait("system:workflow")],
    [comment(3, "system:workflow", "Approve the release?")],
  );
  expect(chipOn("Approve the release?")).toBeInTheDocument();
});

it("keeps a flexible task's status select, fields and sections as they were", async () => {
  const { onSave } = renderDetail(flexible);
  const status = screen.getByLabelText("Status");
  expect(
    within(status)
      .getAllByRole("option")
      .map((option) => option.textContent),
  ).toEqual(["Open", "In progress", "Waiting for you", "Done", "Cancelled"]);
  expect(status).toHaveValue("in_progress");
  for (const label of ["Priority", "Assignee", "Pull request", "Manual block reason"]) {
    expect(screen.getByLabelText(label)).toBeInTheDocument();
  }
  expect(screen.getByText("Dependencies")).toBeInTheDocument();
  expect(screen.getByText("History")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Save task" })).not.toBeInTheDocument();
  expect(screen.queryByText("release@1.2.0")).not.toBeInTheDocument();
  expect(api.getTaskWorkflow).not.toHaveBeenCalled();
  await userEvent.selectOptions(status, "done");
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));
  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ status: "done" }));
});

it("drops the old engine's unmanaged line for both kinds, since it meant nothing for a flexible task either", async () => {
  renderDetail(flexible);
  expect(screen.queryByText(/unmanaged/)).not.toBeInTheDocument();
  cleanup();
  renderDetail(workflowTask);
  await screen.findByText("release@1.2.0");
  expect(screen.queryByText(/unmanaged/)).not.toBeInTheDocument();
});

it("replaces the status select with the workflow panel for a workflow task", async () => {
  const { onSave } = renderDetail(workflowTask);
  expect(await screen.findByText("release@1.2.0")).toBeInTheDocument();
  expect(screen.queryByLabelText("Status")).not.toBeInTheDocument();
  expect(screen.queryByText(/unmanaged/)).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Edit title" }));
  expect(screen.getByLabelText("Priority")).toBeInTheDocument();
  expect(screen.getByLabelText("Pull request")).toBeInTheDocument();
  expect(screen.getByText("Dependencies")).toBeInTheDocument();
  await userEvent.clear(screen.getByLabelText("Title"));
  await userEvent.type(screen.getByLabelText("Title"), "Renamed");
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ title: "Renamed", status: undefined, assignee: undefined }),
  );
});

it("confirms a pause decision inside the open sheet without closing it", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...poolView,
    waiting_on: "pause",
    paused_reason: "idle_iterations",
  });
  api.resumeTaskWorkflow.mockResolvedValue(workflowTask);
  const { onTaskChanged, onClose } = renderDetail({ ...workflowTask, waiting_on: "pause" });
  await userEvent.click(await screen.findByRole("button", { name: "Continue" }));
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Continue" }),
  );
  await waitFor(() => expect(api.resumeTaskWorkflow).toHaveBeenCalledWith("REL-1", "continue"));
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalled());
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog")).toHaveTextContent("Release notes");
});

const wait = (requesting_principal: string): WaitingFor => ({
  id: 1,
  task_key: "REL-1",
  expected_principal: "user:owner",
  requesting_principal,
  requesting_comment_id: 3,
  requested_at: "2026-10-01T10:05:00Z",
});

it("leaves the workflow's own wait to the panel instead of asking for a comment", async () => {
  renderDetail(workflowTask, [wait("system:workflow")]);
  await screen.findByText("release@1.2.0");
  expect(screen.queryByText(/Waiting for an answer/)).not.toBeInTheDocument();
});

it("still shows an agent's question on a workflow task, and every wait on a flexible task", async () => {
  renderDetail({ ...workflowTask, waiting_on: "script" }, [wait("agent:writer")]);
  expect(await screen.findByText(/Waiting for an answer from user:owner/)).toBeInTheDocument();
  cleanup();
  renderDetail(flexible, [wait("system:workflow")]);
  expect(screen.getByText(/Waiting for an answer from user:owner/)).toBeInTheDocument();
});

it("shows a workflow task's assignee as text, with no control to change it", async () => {
  renderDetail(workflowTask);
  await screen.findByText("release@1.2.0");
  expect(screen.queryByLabelText("Assignee")).not.toBeInTheDocument();
  const field = screen.getByText("Assignee", { selector: "dt" }).parentElement!;
  expect(field).toHaveTextContent("agent:writer");
  expect(within(field).queryByRole("textbox")).not.toBeInTheDocument();
  expect(within(field).queryByRole("combobox")).not.toBeInTheDocument();
});

const heldTask: Task = {
  ...workflowTask,
  status: "draft",
  category: "in_progress",
  waiting_on: undefined,
};

it("changes the agent of a workflow task in a pool status to another member of that pool", async () => {
  api.getTaskWorkflow.mockResolvedValue(poolView);
  api.listAgentPools.mockResolvedValue({
    items: [
      agentPool("writers", ["writer", "editor"], 1, "REL"),
      agentPool("other", ["x"], 2, "REL"),
    ],
    count: 2,
  });
  api.reassignTaskWorkflow.mockResolvedValue({
    ...heldTask,
    assignee: "agent:editor",
    revision: 5,
  });
  const { onSave, onTaskChanged } = renderDetail(heldTask);
  const select = await screen.findByRole("combobox", { name: "Assignee" });
  await waitFor(() =>
    expect(
      within(select)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["agent:writer", "agent:editor"]),
  );
  expect(select).toHaveValue("agent:writer");
  await userEvent.selectOptions(select, "agent:editor");
  await waitFor(() => expect(api.reassignTaskWorkflow).toHaveBeenCalledWith("REL-1", "editor"));
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalled());
  expect(onSave).not.toHaveBeenCalled();
});

it("assigns an unassigned workflow task in a pool status to a member of that pool", async () => {
  api.getTaskWorkflow.mockResolvedValue(poolView);
  api.listAgentPools.mockResolvedValue({
    items: [agentPool("writers", ["writer", "editor"], 1, "REL")],
    count: 1,
  });
  const waiting: Task = { ...heldTask, assignee: "", category: "open" };
  api.reassignTaskWorkflow.mockResolvedValue({
    ...waiting,
    assignee: "agent:editor",
    category: "in_progress",
    revision: 5,
  });
  const { onTaskChanged } = renderDetail(waiting);
  const select = await screen.findByRole("combobox", { name: "Assignee" });
  await waitFor(() =>
    expect(
      within(select)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Unassigned", "agent:writer", "agent:editor"]),
  );
  expect(select).toHaveValue("");
  expect(within(select).getByRole("option", { name: "Unassigned" })).toBeDisabled();
  await userEvent.selectOptions(select, "agent:editor");
  await waitFor(() => expect(api.reassignTaskWorkflow).toHaveBeenCalledWith("REL-1", "editor"));
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalled());
});

it("shows why the daemon refused to change the agent", async () => {
  api.getTaskWorkflow.mockResolvedValue(poolView);
  api.listAgentPools.mockResolvedValue({
    items: [agentPool("writers", ["writer", "editor"], 1, "REL")],
    count: 1,
  });
  api.reassignTaskWorkflow.mockRejectedValue(
    new ApiError(
      409,
      "agent_unavailable",
      "agent editor cannot take status draft of pool writers: halted",
    ),
  );
  renderDetail(heldTask);
  const select = await screen.findByRole("combobox", { name: "Assignee" });
  await waitFor(() => expect(within(select).getAllByRole("option")).toHaveLength(2));
  await userEvent.selectOptions(select, "agent:editor");
  expect(await screen.findByRole("alert")).toHaveTextContent("halted");
  expect(select).toHaveValue("agent:writer");
});

it("keeps the assignee as text on a paused task in a pool status", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...poolView,
    waiting_on: "pause",
    paused_reason: "idle_iterations",
  });
  api.listAgentPools.mockResolvedValue({
    items: [agentPool("writers", ["writer", "editor"], 1, "REL")],
    count: 1,
  });
  renderDetail({ ...heldTask, waiting_on: "pause" });
  await screen.findByRole("button", { name: "Continue" });
  expect(screen.queryByRole("combobox", { name: "Assignee" })).not.toBeInTheDocument();
});

const people: PrincipalInfo = { customer: "user:owner", agents: ["writer", "editor"], groups: [] };

it("pins the workflow decision above the task fields", async () => {
  renderDetail(workflowTask, [], [], people);
  const card = await screen.findByRole("region", { name: "Workflow decision" });
  expect(
    await within(card).findByRole("button", { name: "approve → publish" }),
  ).toBeInTheDocument();
  // The card holds the decision, so the workflow section does not repeat it.
  expect(screen.queryByRole("button", { name: "approve" })).not.toBeInTheDocument();
  const title = screen.getByText("Description", { selector: "label" });
  expect(card.compareDocumentPosition(title) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

it("pins no card on a task that waits on nobody", () => {
  renderDetail(flexible, [], [], people);
  expect(screen.queryByRole("region", { name: "Needs you" })).not.toBeInTheDocument();
});

it("collapses Details and History and keeps the workflow in its own section", async () => {
  renderDetail(workflowTask, [], [], people);
  await screen.findByText("release@1.2.0");
  const details = screen.getByText("Details", { selector: "summary" }).closest("details")!;
  expect(details).not.toHaveAttribute("open");
  expect(within(details).getByLabelText("Priority")).toBeInTheDocument();
  expect(within(details).getByText("Dependencies")).toBeInTheDocument();
  const workflow = screen.getByText("Workflow", { selector: "summary" }).closest("details")!;
  expect(within(workflow).getByText("release@1.2.0")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Expand" })).toHaveAttribute("aria-expanded", "false");
});

it("opens the workflow section when the workflow reports a problem", async () => {
  renderDetail(
    { ...workflowTask, workflow_attention: { kind: "rejected", message: "CI is red", count: 1 } },
    [],
    [],
    people,
  );
  await screen.findByText("release@1.2.0");
  expect(screen.getByText("Workflow", { selector: "summary" }).closest("details")).toHaveAttribute(
    "open",
  );
});

it("keeps the workflow section open after its problem clears", async () => {
  const troubled = {
    ...workflowTask,
    workflow_attention: { kind: "rejected" as const, message: "CI is red", count: 1 },
  };
  const element = (task: Task) => (
    <TaskDetail
      detail={{
        task,
        comments: [],
        waiting_for: [],
        relations: [],
        descendants: 0,
        active_descendants: 0,
      }}
      events={[]}
      principals={people}
      width={600}
      resizeHandle={null}
      onClose={vi.fn()}
      onSave={vi.fn()}
      onComment={vi.fn()}
      onAddRelation={vi.fn()}
      onDeleteRelation={vi.fn()}
      onTransfer={vi.fn()}
      onRequeue={vi.fn()}
    />
  );
  const { rerender } = render(element(troubled));
  await screen.findByText("release@1.2.0");
  rerender(element({ ...troubled, workflow_attention: undefined, revision: 5 }));
  expect(screen.getByText("Workflow", { selector: "summary" }).closest("details")).toHaveAttribute(
    "open",
  );
});

it("picks a flexible task's assignee from me and the agents, never from typed text", async () => {
  const { onSave } = renderDetail({ ...flexible, assignee: "agent:gone" }, [], [], people);
  const select = screen.getByRole("combobox", { name: "Assignee" });
  expect(
    within(select)
      .getAllByRole("option")
      .map((option) => [option.textContent, option.getAttribute("value")]),
  ).toEqual([
    ["Unassigned", ""],
    ["me", "user:owner"],
    ["writer", "agent:writer"],
    ["editor", "agent:editor"],
    ["agent:gone", "agent:gone"],
  ]);
  expect(within(select).getByRole("option", { name: "agent:gone" })).toBeDisabled();
  expect(screen.queryByRole("textbox", { name: "Assignee" })).not.toBeInTheDocument();
  await userEvent.selectOptions(select, "agent:writer");
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));
  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ assignee: "agent:writer" }));
});

it("labels the comment form's Ok and Ask controls by what they do", () => {
  renderDetail(flexible, [], [], people);
  expect(screen.getByRole("button", { name: "Reply Ok to the question" })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "Ask an agent" })).toBeInTheDocument();
});
