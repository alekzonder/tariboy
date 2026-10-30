import type { ReactElement } from "react";
import { cleanup, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import TaskDetail from "./TaskDetail";
import {
  ApiError,
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

function renderDetail(task: Task, waiting_for: WaitingFor[] = [], comments: TaskComment[] = []) {
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
      principals={null}
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
  for (const label of ["Title", "Priority", "Assignee", "Pull request", "Manual block reason"]) {
    expect(screen.getByLabelText(label)).toBeInTheDocument();
  }
  expect(screen.getByText("Dependencies")).toBeInTheDocument();
  expect(screen.getByText("History")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Save task" })).toBeInTheDocument();
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
  expect(screen.getByLabelText("Title")).toBeInTheDocument();
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
  expect(screen.getByText("Release notes")).toBeInTheDocument();
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
  renderDetail(workflowTask, [wait("agent:writer")]);
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
