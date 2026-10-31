import { act, fireEvent, render, screen, waitFor } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import type { TaskDetail, TransitionRequest } from "@/lib/api";
import { fakeApi } from "@/test/api";
import { taskItem } from "@/test/fixtures/apiFixtures";
import { customerView, workflowTask } from "@/test/fixtures/workflowFixtures";
import NeedsYouCard from "./NeedsYouCard";

const api = vi.hoisted(() => ({
  getTask: vi.fn(),
  listTaskPrincipals: vi.fn(),
  addTaskComment: vi.fn(),
  getTaskWorkflow: vi.fn(),
  advanceTask: vi.fn(),
  getTransitionRequest: vi.fn(),
}));
const client = fakeApi({
  tasks: {
    getTask: api.getTask,
    listTaskPrincipals: api.listTaskPrincipals,
    addTaskComment: api.addTaskComment,
  },
  workflows: {
    getTaskWorkflow: api.getTaskWorkflow,
    advanceTask: api.advanceTask,
    getTransitionRequest: api.getTransitionRequest,
  },
});

const detail = (over: Partial<TaskDetail>): TaskDetail => ({
  task: workflowTask,
  comments: [],
  relations: [],
  waiting_for: [],
  descendants: 0,
  active_descendants: 0,
  ...over,
});

beforeEach(() => {
  vi.clearAllMocks();
  api.listTaskPrincipals.mockResolvedValue({ customer: "user:owner", agents: ["writer"] });
});

it("shows the plan of a customer status and advances from it with the message", async () => {
  api.getTask.mockResolvedValue(detail({}));
  api.getTaskWorkflow.mockResolvedValue(customerView);
  api.advanceTask.mockResolvedValue({
    id: 1,
    task_key: "REL-1",
    outcome: "approve",
    actor: "user:owner",
    state: "applied",
    created_at: "",
  } satisfies TransitionRequest);
  const onChanged = vi.fn();
  render(<NeedsYouCard taskKey="REL-1" onChanged={onChanged} />, { api: client });

  expect(await screen.findByText("writer waits for your decision in review.")).toBeInTheDocument();
  // The artifact the outcome requires is open, so the plan reads without a click.
  expect(screen.getByRole("heading", { name: "Notes" })).toBeVisible();
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "ship it" } });
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "approve → publish" }));
  });
  expect(api.advanceTask).toHaveBeenCalledWith("REL-1", "approve", "ship it", "review");
  expect(onChanged).toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "rework → draft" })).toBeInTheDocument();
});

it("shows an open question and answers it with a customer comment", async () => {
  const flexible = taskItem({ key: "TASK-3", title: "Pick a name", status: "in_progress" });
  api.getTask.mockResolvedValue(
    detail({
      task: flexible,
      comments: [
        {
          id: 9,
          task_key: "TASK-3",
          author: "agent:bob",
          body: "@user:owner\n\nWhich **name** do you want?",
          created_at: "2026-10-30T10:00:00Z",
        } as TaskDetail["comments"][number],
      ],
      waiting_for: [
        {
          id: 1,
          task_key: "TASK-3",
          expected_principal: "user:owner",
          requesting_principal: "agent:bob",
          requesting_comment_id: 9,
          requested_at: "2026-10-30T10:00:00Z",
        },
      ],
    }),
  );
  api.addTaskComment.mockResolvedValue({});
  render(<NeedsYouCard taskKey="TASK-3" />, { api: client });

  expect(await screen.findByText("name")).toBeInTheDocument();
  expect(api.getTaskWorkflow).not.toHaveBeenCalled();
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reply Ok to the question" }));
  });
  expect(api.addTaskComment).toHaveBeenCalledWith("TASK-3", "Ok", expect.any(String));
  fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Call it Lark" } });
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Send answer" }));
  });
  expect(api.addTaskComment).toHaveBeenLastCalledWith("TASK-3", "Call it Lark", expect.any(String));
});

it("shows the pause reason with Continue for a paused task", async () => {
  api.getTask.mockResolvedValue(
    detail({
      task: { ...workflowTask, waiting_on: "pause", workflow_paused_reason: "idle_iterations" },
    }),
  );
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    waiting_on: "pause",
    paused_reason: "idle_iterations",
  });
  render(<NeedsYouCard taskKey="REL-1" />, { api: client });
  await waitFor(() => expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument());
  expect(screen.getByText(/several iterations in a row/)).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "approve → publish" })).not.toBeInTheDocument();
});

it("opens from the key and title without sending the inline response", async () => {
  api.getTask.mockResolvedValue(detail({}));
  api.getTaskWorkflow.mockResolvedValue(customerView);
  const onOpen = vi.fn();
  render(<NeedsYouCard taskKey="REL-1" onOpen={onOpen} />, { api: client });
  const message = await screen.findByRole("textbox", { name: "Message" });
  fireEvent.change(message, { target: { value: "My unfinished response" } });
  const header = screen.getByRole("button", { name: "REL-1 Release notes" });
  expect(header).toHaveAttribute("type", "button");
  fireEvent.click(message);
  expect(onOpen).not.toHaveBeenCalled();
  fireEvent.click(header);
  expect(onOpen).toHaveBeenCalledOnce();
  header.focus();
  await userEvent.keyboard("{Enter} ");
  expect(onOpen).toHaveBeenCalledTimes(3);
  expect(message).toHaveValue("My unfinished response");
  expect(api.advanceTask).not.toHaveBeenCalled();
  expect(api.addTaskComment).not.toHaveBeenCalled();
});

it("keeps the header as text when no open action is provided", async () => {
  api.getTask.mockResolvedValue(detail({}));
  api.getTaskWorkflow.mockResolvedValue(customerView);
  render(<NeedsYouCard taskKey="REL-1" />, { api: client });
  expect(await screen.findByText("Release notes")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "REL-1 Release notes" })).not.toBeInTheDocument();
});

it("focuses the response rather than the open button for an answer link", async () => {
  api.getTask.mockResolvedValue(detail({}));
  api.getTaskWorkflow.mockResolvedValue(customerView);
  render(<NeedsYouCard taskKey="REL-1" onOpen={vi.fn()} focus />, { api: client });
  expect(await screen.findByRole("textbox", { name: "Message" })).toHaveFocus();
});
