import { act, fireEvent, render, screen, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError, type WorkflowView } from "@/lib/api";
import { fakeApi } from "@/test/api";
import { customerView, poolView, workflowTask } from "@/test/fixtures/workflowFixtures";
import TaskDetail from "./TaskDetail";

const api = {
  getTaskWorkflow: vi.fn(),
  advanceTask: vi.fn(),
  resumeTaskWorkflow: vi.fn(),
};
const client = fakeApi({ workflows: api });
beforeEach(() => {
  vi.clearAllMocks();
  api.getTaskWorkflow.mockResolvedValue(customerView);
});

function show(view: WorkflowView = customerView, readError?: Error) {
  if (readError) api.getTaskWorkflow.mockRejectedValue(readError);
  else api.getTaskWorkflow.mockResolvedValue(view);
  const onClose = vi.fn();
  const onComment = vi.fn();
  const element = () => (
    <TaskDetail
      detail={{
        task: { ...workflowTask, status: view.status, category: view.category },
        comments: [],
        waiting_for: [],
        relations: [],
        descendants: 0,
        active_descendants: 0,
      }}
      events={[]}
      principals={{ customer: "user:owner", agents: [], groups: [] }}
      width={700}
      resizeHandle={null}
      onClose={onClose}
      onSave={vi.fn()}
      onComment={onComment}
      onAddRelation={vi.fn()}
      onDeleteRelation={vi.fn()}
      onTransfer={vi.fn()}
      onRequeue={vi.fn()}
    />
  );
  const result = render(element(), { api: client });
  return {
    onClose,
    onComment,
    queryClient: result.client,
    rerender: () => result.rerender(element()),
  };
}

it("shows the initial customer's real outcomes above the description with no duplicate title", async () => {
  show({
    ...customerView,
    status: "inbox",
    outcomes: [{ on: "begin", to: "draft" }],
    statuses: [{ id: "inbox", initial: true, owner: "customer", terminal: false }],
  });
  const panel = await screen.findByRole("region", { name: "Workflow decision" });
  expect(within(panel).getByRole("button", { name: "begin → draft" })).toBeEnabled();
  expect(
    panel.compareDocumentPosition(screen.getByText("Description", { selector: "label" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(screen.getAllByText(workflowTask.title)).toHaveLength(1);
  expect(
    screen.getByText("Workflow", { selector: "summary" }).closest("details"),
  ).not.toHaveAttribute("open");
  expect(api.advanceTask).not.toHaveBeenCalled();
});

it("opens the plan outside the sticky controls and focuses its material", async () => {
  show({
    ...customerView,
    outcomes: [
      { on: "approve", to: "publish", requires: ["plan", "evidence"], missing: ["evidence"] },
    ],
    artifacts: [{ ...customerView.artifacts[0], name: "plan", value: "The detailed plan" }],
  });
  const panel = await screen.findByRole("region", { name: "Workflow decision" });
  expect(within(panel).queryByText("The detailed plan")).not.toBeInTheDocument();
  expect(within(panel).getByRole("button", { name: "approve → publish" })).toBeDisabled();
  expect(within(panel).getByText("needs: evidence")).toBeInTheDocument();
  await userEvent.click(within(panel).getByRole("button", { name: "View plan" }));
  const material = screen.getByRole("group", { name: "plan" });
  expect(material).toHaveAttribute("open");
  expect(material.querySelector("summary")).toHaveFocus();
  expect(within(material).getByText("The detailed plan")).toBeVisible();
  expect(screen.getByRole("group", { name: "evidence" })).toHaveTextContent("No value yet");
  expect(api.advanceTask).not.toHaveBeenCalled();
});

it("keeps the decision draft after failure and refresh, and guards closing", async () => {
  api.advanceTask.mockRejectedValue(
    new ApiError(409, "status_changed", "Changed by another participant"),
  );
  const { onClose, rerender } = show();
  const message = await screen.findByRole("textbox", { name: "Outcome message" });
  fireEvent.change(message, { target: { value: "Please adjust the plan" } });
  await userEvent.click(screen.getByRole("button", { name: "rework → draft" }));
  expect(await screen.findByText("Changed by another participant")).toBeInTheDocument();
  expect(api.advanceTask).toHaveBeenCalledWith(
    "REL-1",
    "rework",
    "Please adjust the plan",
    "review",
  );
  rerender();
  expect(message).toHaveValue("Please adjust the plan");
  await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
  expect(await screen.findByText(/Unsaved changes will be discarded/)).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});

it("keeps a stale decision draft accessible when another participant changes the owner", async () => {
  api.advanceTask.mockRejectedValue(new ApiError(409, "status_changed", "Already moved"));
  show();
  const message = await screen.findByRole("textbox", { name: "Outcome message" });
  fireEvent.change(message, { target: { value: "My decision" } });
  api.getTaskWorkflow.mockResolvedValue(poolView);
  await userEvent.click(screen.getByRole("button", { name: "rework → draft" }));
  expect(await screen.findByText(/The agent continues/)).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Outcome message" })).toHaveValue("My decision");
});

it.each(["rejected", "failed"] as const)(
  "shows a settled %s request by the outcome and opens its details",
  async (state) => {
    const request = {
      id: 11,
      task_key: "REL-1",
      outcome: "approve",
      actor: "user:owner",
      state,
      created_at: "2026-10-01T10:06:00Z",
      result_message: "The check needs attention",
    };
    show();
    const panel = await screen.findByRole("region", { name: "Workflow decision" });
    fireEvent.change(within(panel).getByRole("textbox", { name: "Outcome message" }), {
      target: { value: "Keep the decision" },
    });
    api.advanceTask.mockResolvedValue(request);
    api.getTaskWorkflow.mockResolvedValue({ ...customerView, last_request: request });
    await userEvent.click(within(panel).getByRole("button", { name: "approve → publish" }));
    const details = await within(panel).findByRole("button", { name: "View failure details" });
    expect(within(panel).getByText("The check needs attention")).toBeVisible();
    expect(within(panel).getByRole("textbox", { name: "Outcome message" })).toHaveValue(
      "Keep the decision",
    );
    await userEvent.click(details);
    const summary = screen.getByText("Workflow", { selector: "summary" });
    expect(summary.closest("details")).toHaveAttribute("open");
    expect(summary).toHaveFocus();
    expect(api.advanceTask).toHaveBeenCalledOnce();
  },
);

it("blocks duplicate transitions and closing while the request is pending", async () => {
  let resolve!: (value: unknown) => void;
  api.advanceTask.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { onClose } = show();
  const button = await screen.findByRole("button", { name: "approve → publish" });
  await userEvent.click(button);
  expect(button).toBeDisabled();
  expect(screen.getByRole("button", { name: "Close task detail" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(onClose).not.toHaveBeenCalled();
  await act(async () => resolve({ id: 1, state: "applied" }));
});

it("does not discard a draft while an outcome started after the close confirmation is pending", async () => {
  let resolve!: (value: unknown) => void;
  api.advanceTask.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { onClose } = show();
  fireEvent.change(await screen.findByRole("textbox", { name: "Outcome message" }), {
    target: { value: "Keep this" },
  });
  await userEvent.click(screen.getByRole("button", { name: "Close task detail" }));
  const discard = screen.getByRole("button", { name: "Discard changes" });
  await userEvent.click(screen.getByRole("button", { name: "approve → publish" }));
  expect(discard).toBeDisabled();
  await userEvent.click(discard);
  expect(onClose).not.toHaveBeenCalled();
  await act(async () => resolve({ id: 1, state: "applied" }));
});

it("shows the pause reason and keeps Continue confirmation in the compact panel", async () => {
  show({ ...poolView, waiting_on: "pause", paused_reason: "script_failures" });
  const panel = await screen.findByRole("region", { name: "Workflow decision" });
  expect(within(panel).getByText(/Scripts failed too many times/)).toBeInTheDocument();
  await userEvent.click(within(panel).getByRole("button", { name: "Continue" }));
  expect(await screen.findByRole("alertdialog")).toHaveTextContent("The same holder continues");
  expect(api.resumeTaskWorkflow).not.toHaveBeenCalled();
});

it("keeps closed workflow tasks free of outcomes even if the view still lists them", async () => {
  show({ ...customerView, category: "done" });
  const panel = await screen.findByRole("region", { name: "Workflow decision" });
  expect(within(panel).queryByRole("button", { name: /approve/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: "Outcome message" })).not.toBeInTheDocument();
});

it("keeps the task visible on a workflow read failure and retries", async () => {
  show(customerView, new Error("workflow unavailable"));
  expect((await screen.findAllByText("workflow unavailable")).length).toBeGreaterThan(0);
  expect(screen.getByText(workflowTask.title)).toBeInTheDocument();
  api.getTaskWorkflow.mockResolvedValue(customerView);
  await userEvent.click(screen.getAllByRole("button", { name: "Retry" })[0]);
  expect(await screen.findByRole("button", { name: "approve → publish" })).toBeEnabled();
});

it("shows a background workflow read failure and retries without losing the draft", async () => {
  const { queryClient } = show();
  const message = await screen.findByRole("textbox", { name: "Outcome message" });
  const sticky = screen.getByRole("region", { name: "Workflow decision" }).parentElement!;
  fireEvent.change(message, { target: { value: "Preserve this" } });
  api.getTaskWorkflow.mockRejectedValue(new Error("background read failed"));
  await act(async () => {
    await queryClient.invalidateQueries();
  });
  const error = await within(sticky).findByText("background read failed");
  expect(message).toHaveValue("Preserve this");
  api.getTaskWorkflow.mockResolvedValue(customerView);
  await userEvent.click(within(error.parentElement!).getByRole("button", { name: "Retry" }));
  expect(await screen.findByRole("textbox", { name: "Outcome message" })).toHaveValue(
    "Preserve this",
  );
});

it("prevents closing while a confirmed pause decision is being sent", async () => {
  let resolve!: (value: unknown) => void;
  api.resumeTaskWorkflow.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { onClose } = show({ ...poolView, waiting_on: "pause", paused_reason: "script_failures" });
  await userEvent.click(await screen.findByRole("button", { name: "Continue" }));
  await userEvent.click(
    within(screen.getByRole("alertdialog")).getByRole("button", { name: "Continue" }),
  );
  expect(screen.getByRole("button", { name: "Close task detail" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(onClose).not.toHaveBeenCalled();
  await act(async () => resolve({}));
});
