import type { ReactElement } from "react";
import { act, fireEvent, render as renderWith, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import { WorkflowPanel } from "./WorkflowPanel";
import {
  customerView,
  poolView,
  workflowTask,
  remoteTarget,
} from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({
  getTaskWorkflow: vi.fn(),
  advanceTask: vi.fn(),
  moveTaskWorkflow: vi.fn(),
  upgradeTaskWorkflow: vi.fn(),
  cancelWorkflowTask: vi.fn(),
  resumeTaskWorkflow: vi.fn(),
}));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  {
    workflows: {
      getTaskWorkflow: api.getTaskWorkflow,
      advanceTask: api.advanceTask,
      moveTaskWorkflow: api.moveTaskWorkflow,
      upgradeTaskWorkflow: api.upgradeTaskWorkflow,
      cancelWorkflowTask: api.cancelWorkflowTask,
      resumeTaskWorkflow: api.resumeTaskWorkflow,
    },
  },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });

beforeEach(() => {
  vi.clearAllMocks();
  api.getTaskWorkflow.mockResolvedValue(customerView);
  api.moveTaskWorkflow.mockResolvedValue(workflowTask);
  api.upgradeTaskWorkflow.mockResolvedValue(workflowTask);
  api.cancelWorkflowTask.mockResolvedValue(workflowTask);
});

function renderPanel(task = workflowTask) {
  const onTaskChanged = vi.fn();
  const view = render(<WorkflowPanel task={task} onTaskChanged={onTaskChanged} />);
  return { onTaskChanged, ...view };
}

it("renders the header, outcomes, artifacts, runs and visits of the view", async () => {
  renderPanel();
  expect(await screen.findByText("release@1.2.0")).toBeInTheDocument();
  expect(api.getTaskWorkflow).toHaveBeenCalledWith("REL-1", expect.any(AbortSignal));
  const header = screen.getByText("release@1.2.0").parentElement!;
  expect(header).toHaveTextContent("review");
  expect(header).toHaveTextContent("customer");
  expect(screen.getByRole("button", { name: "approve" })).toBeInTheDocument();
  expect(screen.getByText("Artifacts").closest("section")).toContainElement(
    screen.getByRole("heading", { name: "Notes" }),
  );
  expect(screen.getByText("Script runs").closest("section")).toHaveTextContent("./scripts/ci.sh");
  const visits = screen.getByText("Visits").closest("section")!;
  expect(visits).toHaveTextContent("draft");
  expect(visits).toHaveTextContent("drafted");
  expect(visits).toHaveTextContent("first cut");
  expect(within(visits).getAllByRole("listitem")).toHaveLength(2);
  expect(screen.queryByText(/paused/i)).not.toBeInTheDocument();
});

it("names the pool and the holder for a pool status", async () => {
  api.getTaskWorkflow.mockResolvedValue(poolView);
  renderPanel({ ...workflowTask, status: "draft", category: "in_progress", waiting_on: undefined });
  const header = (await screen.findByText("release@1.2.0")).parentElement!;
  expect(header).toHaveTextContent("pool:writers");
  expect(header).toHaveTextContent("agent:writer");
});

it("shows the pause banner for a paused task", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...poolView,
    waiting_on: "pause",
    paused_reason: "holder_unavailable",
  });
  renderPanel({
    ...workflowTask,
    waiting_on: "pause",
    workflow_paused_reason: "holder_unavailable",
  });
  expect(await screen.findByText(/holder of this status cannot work/i)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Release holder" })).toBeInTheDocument();
});

it("shows a failed view request with a retry", async () => {
  api.getTaskWorkflow.mockRejectedValueOnce(
    new ApiError(503, "workflow_unavailable", "the image store is not available"),
  );
  renderPanel();
  expect(await screen.findByRole("alert")).toHaveTextContent("the image store is not available");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  });
  expect(await screen.findByText("release@1.2.0")).toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("refetches the view when the task's revision changes", async () => {
  const { rerender, onTaskChanged } = renderPanel();
  await screen.findByText("release@1.2.0");
  rerender(<WorkflowPanel task={{ ...workflowTask }} onTaskChanged={onTaskChanged} />);
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(1);
  rerender(<WorkflowPanel task={{ ...workflowTask, revision: 5 }} onTaskChanged={onTaskChanged} />);
  await waitFor(() => expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2));
});

it("refetches the view and reports the change after an outcome applies", async () => {
  api.advanceTask.mockResolvedValue({
    id: 1,
    task_key: "REL-1",
    outcome: "rework",
    actor: "user:owner",
    state: "applied",
    created_at: "",
  });
  const { onTaskChanged } = renderPanel();
  const rework = await screen.findByRole("button", { name: "rework" });
  await act(async () => {
    fireEvent.click(rework);
  });
  expect(onTaskChanged).toHaveBeenCalledTimes(1);
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2);
});

// The actions are labeled buttons in the panel header, not a menu: a click
// opens the move form or the cancel confirmation directly.
async function openAction(name: string) {
  await userEvent.click(await screen.findByRole("button", { name }));
}

it("moves to a declared status with a required reason after confirmation", async () => {
  const { onTaskChanged } = renderPanel();
  await openAction("Move to status…");
  const select = screen.getByLabelText("Target status");
  // Every declared status but the current one; terminal ones last, grouped.
  expect(
    within(select)
      .getAllByRole("option")
      .map((option) => option.getAttribute("value")),
  ).toEqual(["", "draft", "publish", "done", "dropped"]);
  const terminal = select.querySelector("optgroup")!;
  expect(terminal).toHaveAttribute("label", "Terminal");
  expect([...terminal.querySelectorAll("option")].map((option) => option.value)).toEqual([
    "done",
    "dropped",
  ]);
  expect(select).toHaveFocus();
  await userEvent.selectOptions(select, "draft");
  const move = screen.getByRole("button", { name: "Move" });
  expect(move).toBeDisabled();
  await userEvent.type(screen.getByLabelText("Reason"), "found a regression");
  await userEvent.click(move);
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Move task" }));
  await waitFor(() =>
    expect(api.moveTaskWorkflow).toHaveBeenCalledWith("REL-1", "draft", "found a regression"),
  );
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalledTimes(1));
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2);
});

it("filters the artifacts by the current step, by the step clicked, and shows every declared artifact on reset", async () => {
  renderPanel();
  const artifacts = (await screen.findByText("Artifacts")).closest("section")!;
  expect(screen.getByRole("button", { name: /^review/ })).toHaveAttribute("aria-pressed", "true");
  expect(within(artifacts).getByTestId("artifact-filter")).toHaveTextContent(
    "Step review · 2 of 3 artifacts",
  );
  const enter = within(artifacts).getByRole("list", { name: "Needed to enter review" });
  expect(enter).toHaveTextContent("summary");
  // `notes` is needed to enter and to leave: it shows once, with both transitions.
  expect(enter).toHaveTextContent("required by review · approve → publish");
  expect(within(artifacts).queryByText("changelog")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: /^publish/ }));
  expect(within(artifacts).getByTestId("artifact-filter")).toHaveTextContent(
    "Step publish · 1 of 3 artifacts",
  );
  expect(
    within(artifacts).getByRole("list", { name: "Needed to enter publish" }),
  ).toHaveTextContent("required by review · approve → publish");
  await userEvent.click(within(artifacts).getByRole("button", { name: "Show all artifacts" }));
  expect(screen.getByRole("button", { name: /^publish/ })).toHaveAttribute("aria-pressed", "false");
  const changelog = within(within(artifacts).getByRole("list", { name: "All artifacts" }))
    .getByText("changelog")
    .closest("li")!;
  expect(within(changelog).getByText("no value")).toBeInTheDocument();
  expect(within(changelog).getByRole("button", { name: "Set" })).toBeInTheDocument();
  // A click on a step filters again; a second click on it shows all.
  await userEvent.click(screen.getByRole("button", { name: /^draft/ }));
  expect(within(artifacts).getByRole("list", { name: "Needed to leave draft" })).toHaveTextContent(
    "summary",
  );
  await userEvent.click(screen.getByRole("button", { name: /^draft/ }));
  expect(within(artifacts).getByRole("list", { name: "All artifacts" })).toBeInTheDocument();
});

it("moves the default step with the task until a step is chosen", async () => {
  const { rerender } = renderPanel();
  await screen.findByTestId("artifact-filter");
  api.getTaskWorkflow.mockResolvedValue(poolView);
  rerender(<WorkflowPanel task={{ ...workflowTask, revision: 5 }} onTaskChanged={vi.fn()} />);
  await waitFor(() =>
    expect(screen.getByTestId("artifact-filter")).toHaveTextContent("Step draft"),
  );
  await userEvent.click(screen.getByRole("button", { name: /^publish/ }));
  api.getTaskWorkflow.mockResolvedValue(customerView);
  rerender(<WorkflowPanel task={{ ...workflowTask, revision: 6 }} onTaskChanged={vi.fn()} />);
  await waitFor(() => expect(api.getTaskWorkflow).toHaveBeenCalledTimes(3));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /^review/ })).toHaveAttribute("aria-current", "step"),
  );
  expect(screen.getByTestId("artifact-filter")).toHaveTextContent("Step publish");
});

it("falls back to the current step when the chosen one leaves the manifest, and to every artifact without one", async () => {
  const { rerender } = renderPanel();
  await userEvent.click(await screen.findByRole("button", { name: /^publish/ }));
  const statuses = customerView.statuses.filter((status) => status.id !== "publish");
  api.getTaskWorkflow.mockResolvedValue({ ...customerView, statuses });
  rerender(<WorkflowPanel task={{ ...workflowTask, revision: 5 }} onTaskChanged={vi.fn()} />);
  await waitFor(() =>
    expect(screen.getByTestId("artifact-filter")).toHaveTextContent("Step review"),
  );
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    statuses,
    status: "gone",
    category: "done",
  });
  rerender(<WorkflowPanel task={{ ...workflowTask, revision: 6 }} onTaskChanged={vi.fn()} />);
  await waitFor(() =>
    expect(screen.getByRole("list", { name: "All artifacts" })).toBeInTheDocument(),
  );
});

it("shows the step, the stepper, and the problem block above the header", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    last_request: {
      id: 11,
      task_key: "REL-1",
      outcome: "approve",
      actor: "user:owner",
      state: "rejected",
      result_message: "no heading",
      created_at: "2026-10-01T10:06:00Z",
    },
  });
  renderPanel();
  const header = (await screen.findByText("release@1.2.0")).parentElement!;
  expect(header).toHaveTextContent("step 2 of 5");
  expect(
    within(screen.getByRole("list", { name: "Workflow steps" })).getByRole("button", {
      name: /^review/,
    }),
  ).toHaveAttribute("aria-current", "step");
  expect(screen.getByRole("alert")).toHaveTextContent("A check rejected the request: approve");
  expect(screen.getByRole("alert")).toHaveTextContent("no heading");
});

it("returns the task to the agent through the declared outcome, with the message", async () => {
  api.advanceTask.mockResolvedValue({
    id: 1,
    task_key: "REL-1",
    outcome: "rework",
    actor: "user:owner",
    state: "applied",
    created_at: "",
  });
  const { onTaskChanged } = renderPanel();
  await openAction("Return to agent…");
  expect(screen.getByLabelText("Return to status")).toHaveValue("draft");
  await userEvent.type(screen.getByLabelText("Message to the agent"), "add tests");
  await userEvent.click(screen.getByRole("button", { name: "Return to agent" }));
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Return to agent" }),
  );
  await waitFor(() =>
    expect(api.advanceTask).toHaveBeenCalledWith("REL-1", "rework", "add tests", "review"),
  );
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalledTimes(1));
  expect(screen.queryByLabelText("Message to the agent")).not.toBeInTheDocument();
});

it("offers no return to the agent while an agent holds an unpaused pool status", async () => {
  api.getTaskWorkflow.mockResolvedValue(poolView);
  renderPanel({ ...workflowTask, status: "draft", category: "in_progress", waiting_on: undefined });
  await screen.findByText("release@1.2.0");
  expect(screen.queryByRole("button", { name: "Return to agent…" })).not.toBeInTheDocument();
});

it("shows a refused move inline", async () => {
  api.moveTaskWorkflow.mockRejectedValue(
    new ApiError(400, "status_unknown", "the image does not declare status x"),
  );
  const { onTaskChanged } = renderPanel();
  await openAction("Move to status…");
  await userEvent.selectOptions(screen.getByLabelText("Target status"), "publish");
  await userEvent.type(screen.getByLabelText("Reason"), "why not");
  await userEvent.click(screen.getByRole("button", { name: "Move" }));
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Move task" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent("the image does not declare status x");
  expect(onTaskChanged).not.toHaveBeenCalled();
});

it("cancels the task after confirmation, and not when declined", async () => {
  const { onTaskChanged } = renderPanel();
  await openAction("Cancel task");
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Go back" }),
  );
  expect(api.cancelWorkflowTask).not.toHaveBeenCalled();
  await openAction("Cancel task");
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel task" }),
  );
  await waitFor(() => expect(api.cancelWorkflowTask).toHaveBeenCalledWith("REL-1"));
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalledTimes(1));
});

it("settles a declined move: the form stays open and Move works again", async () => {
  renderPanel();
  await openAction("Move to status…");
  await userEvent.selectOptions(screen.getByLabelText("Target status"), "draft");
  await userEvent.type(screen.getByLabelText("Reason"), "why");
  await userEvent.click(screen.getByRole("button", { name: "Move" }));
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Go back" }),
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "Move" })).toBeEnabled());
  expect(api.moveTaskWorkflow).not.toHaveBeenCalled();
  expect(screen.getByLabelText("Target status")).toHaveValue("draft");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("disables Move while the move is in flight", async () => {
  let finish!: () => void;
  api.moveTaskWorkflow.mockReturnValue(
    new Promise<void>((resolve) => {
      finish = resolve;
    }),
  );
  renderPanel();
  await openAction("Move to status…");
  await userEvent.selectOptions(screen.getByLabelText("Target status"), "draft");
  await userEvent.type(screen.getByLabelText("Reason"), "why");
  await userEvent.click(screen.getByRole("button", { name: "Move" }));
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Move task" }),
  );
  await waitFor(() => expect(api.moveTaskWorkflow).toHaveBeenCalled());
  expect(screen.getByRole("button", { name: "Move" })).toBeDisabled();
  await act(async () => {
    finish();
  });
  await waitFor(() => expect(screen.queryByLabelText("Target status")).not.toBeInTheDocument());
});

it("shows a failed cancel next to the actions, not as a view error with a retry", async () => {
  api.cancelWorkflowTask.mockRejectedValue(
    new ApiError(409, "workflow_closed", "the task is already closed"),
  );
  const { onTaskChanged } = renderPanel();
  await openAction("Cancel task");
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel task" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent("the task is already closed");
  expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  expect(onTaskChanged).not.toHaveBeenCalled();
});

it("refetches the view on a workflow event and shows a rejection that arrives while checking", async () => {
  const pending = {
    id: 11,
    task_key: "REL-1",
    outcome: "approve",
    actor: "user:owner",
    state: "pending" as const,
    created_at: "2026-10-01T10:06:00Z",
    wait_seconds: 60,
  };
  api.getTaskWorkflow.mockResolvedValueOnce({ ...customerView, last_request: pending });
  const { rerender, onTaskChanged } = renderPanel();
  expect(await screen.findByText("checking…")).toBeInTheDocument();
  api.getTaskWorkflow.mockResolvedValueOnce({
    ...customerView,
    last_request: {
      ...pending,
      state: "rejected",
      result_message: "no heading",
      wait_seconds: undefined,
    },
  });
  // The task's revision is unchanged: only the event sequence moved.
  rerender(<WorkflowPanel task={workflowTask} eventSequence={12} onTaskChanged={onTaskChanged} />);
  expect(await screen.findByRole("alert")).toHaveTextContent("no heading");
  expect(screen.queryByText("checking…")).not.toBeInTheDocument();
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2);
});

it("drops a view response that arrives after a newer one", async () => {
  let resolveOld!: (view: typeof customerView) => void;
  api.getTaskWorkflow
    .mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
    )
    .mockResolvedValueOnce({ ...customerView, status: "draft", owner: "pool:writers" });
  const { rerender, onTaskChanged } = renderPanel();
  await waitFor(() => expect(api.getTaskWorkflow).toHaveBeenCalledTimes(1));
  rerender(<WorkflowPanel task={{ ...workflowTask, revision: 5 }} onTaskChanged={onTaskChanged} />);
  const header = (await screen.findByText("release@1.2.0")).parentElement!;
  await waitFor(() => expect(header).toHaveTextContent("pool:writers"));
  await act(async () => {
    resolveOld(customerView);
  });
  expect(screen.getByText("release@1.2.0").parentElement).toHaveTextContent("pool:writers");
  expect(screen.getByText("release@1.2.0").parentElement).not.toHaveTextContent("owner customer");
});

it("guards Cancel while it runs and clears its error when another action starts", async () => {
  let fail!: (error: Error) => void;
  api.cancelWorkflowTask.mockReturnValueOnce(
    new Promise((_, reject) => {
      fail = reject;
    }),
  );
  renderPanel();
  await openAction("Cancel task");
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel task" }),
  );
  await waitFor(() => expect(api.cancelWorkflowTask).toHaveBeenCalledTimes(1));
  expect(screen.getByRole("button", { name: "Cancel task" })).toBeDisabled();
  await act(async () => {
    fail(new ApiError(409, "workflow_closed", "the task is already closed"));
  });
  expect(await screen.findByText("the task is already closed")).toBeInTheDocument();
  await openAction("Move to status…");
  expect(screen.queryByText("the task is already closed")).not.toBeInTheDocument();
});

it("says a visit was left without inventing a move when it has no outcome", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    visits: [
      {
        id: 1,
        sequence: 1,
        status: "draft",
        entered_at: "2026-10-01T10:00:00Z",
        entered_by: "user:owner",
        left_at: "2026-10-01T10:04:30Z",
      },
      {
        id: 2,
        sequence: 2,
        status: "review",
        entered_at: "2026-10-01T10:04:30Z",
        entered_by: "agent:writer",
      },
    ],
  });
  renderPanel();
  const first = within(
    await screen.findByText("Visits").then((label) => label.closest("section")!),
  ).getAllByRole("listitem")[0];
  expect(first).toHaveTextContent(/left/);
  expect(first).not.toHaveTextContent(/move/);
});

it("lets a long status ID wrap in the header and the visits", async () => {
  const id = "s".repeat(64);
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    status: id,
    visits: [
      {
        id: 2,
        sequence: 1,
        status: id,
        entered_at: "2026-10-01T10:04:30Z",
        entered_by: "agent:writer",
      },
    ],
  });
  renderPanel();
  const spans = await screen.findAllByText(id);
  expect(spans).toHaveLength(2);
  for (const span of spans) expect(span).toHaveClass("min-w-0", "break-all");
});

it("offers no cancel for a closed task", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    status: "publish",
    category: "done",
    owner: "",
    outcomes: [],
  });
  renderPanel({ ...workflowTask, status: "publish", category: "done", waiting_on: undefined });
  expect(await screen.findByRole("button", { name: "Move to status…" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Cancel task" })).not.toBeInTheDocument();
});

/** customerView whose queue is bound to release@1.3.0 now. */
const upgradable = { ...customerView, queue_version: "1.3.0", queue_digest: "sha256:new" };

it("offers no upgrade while the task follows its queue's version or the queue is unbound", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    queue_version: "1.2.0",
    queue_digest: "sha256:abc",
  });
  const { unmount } = renderPanel();
  await screen.findByText("release@1.2.0");
  expect(screen.queryByRole("button", { name: /Upgrade to/ })).not.toBeInTheDocument();
  unmount();
  api.getTaskWorkflow.mockResolvedValue(customerView);
  renderPanel();
  await screen.findByText("release@1.2.0");
  expect(screen.queryByRole("button", { name: /Upgrade to/ })).not.toBeInTheDocument();
});

it("offers no upgrade on a closed task", async () => {
  api.getTaskWorkflow.mockResolvedValue({ ...upgradable, status: "done", category: "done" });
  renderPanel({ ...workflowTask, status: "done", category: "done" });
  await screen.findByText("release@1.2.0");
  expect(screen.queryByRole("button", { name: /Upgrade to/ })).not.toBeInTheDocument();
});

it("upgrades to the queue's version with an optional reason after confirmation", async () => {
  api.getTaskWorkflow.mockResolvedValue(upgradable);
  const { onTaskChanged } = renderPanel();
  await openAction("Upgrade to release@1.3.0…");
  await userEvent.type(screen.getByLabelText("Upgrade reason"), "new review step");
  await userEvent.click(screen.getByRole("button", { name: "Upgrade" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog).toHaveTextContent(/first status of release@1.3.0/);
  await userEvent.click(within(dialog).getByRole("button", { name: "Upgrade task" }));
  await waitFor(() =>
    expect(api.upgradeTaskWorkflow).toHaveBeenCalledWith("REL-1", "new review step"),
  );
  await waitFor(() => expect(onTaskChanged).toHaveBeenCalledTimes(1));
  expect(api.getTaskWorkflow).toHaveBeenCalledTimes(2);
});

it("shows a refused upgrade inline", async () => {
  api.getTaskWorkflow.mockResolvedValue(upgradable);
  api.upgradeTaskWorkflow.mockRejectedValue(
    new ApiError(409, "workflow_secret_missing", "workflow secrets have no value: GH_TOKEN"),
  );
  const { onTaskChanged } = renderPanel();
  await openAction("Upgrade to release@1.3.0…");
  await userEvent.click(screen.getByRole("button", { name: "Upgrade" }));
  await userEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Upgrade task" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "workflow secrets have no value: GH_TOKEN",
  );
  expect(api.upgradeTaskWorkflow).toHaveBeenCalledWith("REL-1", "");
  expect(onTaskChanged).not.toHaveBeenCalled();
});

it("labels the visits of an earlier workflow version", async () => {
  api.getTaskWorkflow.mockResolvedValue({
    ...customerView,
    visits: [
      { ...customerView.visits[0], workflow_digest: "sha256:old", workflow_version: "1.1.0" },
      { ...customerView.visits[1], workflow_digest: "sha256:abc", workflow_version: "1.2.0" },
    ],
  });
  renderPanel();
  const visits = (await screen.findByText("Visits")).closest("section")!;
  const [old, current] = within(visits).getAllByRole("listitem");
  expect(old).toHaveTextContent("@1.1.0");
  expect(current).not.toHaveTextContent("@1.2.0");
});
