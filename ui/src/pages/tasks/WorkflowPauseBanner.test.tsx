import type { ReactElement } from "react";
import { act, cleanup, fireEvent, render as renderWith, screen, within } from "@/test/queryRender";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import WorkflowPauseBanner from "./WorkflowPauseBanner";
import { remoteTarget } from "@/test/fixtures/workflowFixtures";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({ resumeTaskWorkflow: vi.fn(), cancelWorkflowTask: vi.fn() }));
// The mocks are methods of a client bound to the remote host, so each call
// they record went to that host.
const remoteApi = fakeApi(
  {
    workflows: {
      resumeTaskWorkflow: api.resumeTaskWorkflow,
      cancelWorkflowTask: api.cancelWorkflowTask,
    },
  },
  remoteTarget,
);
const render = (ui: ReactElement) => renderWith(ui, { api: remoteApi });

beforeEach(() => {
  vi.clearAllMocks();
  api.resumeTaskWorkflow.mockResolvedValue({});
  api.cancelWorkflowTask.mockResolvedValue({});
});

function renderBanner(reason = "rejected_requests", pool = true) {
  const onChanged = vi.fn();
  render(<WorkflowPauseBanner taskKey="REL-1" reason={reason} pool={pool} onChanged={onChanged} />);
  return { onChanged };
}

async function confirm(action: string) {
  const dialog = await screen.findByRole("alertdialog");
  await act(async () => {
    fireEvent.click(within(dialog).getByRole("button", { name: action }));
  });
}

it("says why the task is paused in words", () => {
  renderBanner("rejected_requests");
  expect(screen.getByRole("alert")).toHaveTextContent(
    /checks rejected too many transition requests/i,
  );
});

it("shows an unknown reason as it came", () => {
  renderBanner("cosmic_rays");
  expect(screen.getByText(/cosmic_rays/)).toBeInTheDocument();
});

it("offers Release holder only for a pool status", () => {
  renderBanner("script_failures", false);
  expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Cancel task" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Release holder" })).not.toBeInTheDocument();
});

it("describes Continue by the owner of the status", async () => {
  renderBanner("script_failures", true);
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  expect(await screen.findByRole("alertdialog")).toHaveTextContent("The same holder continues");
  await confirm("Go back");
  cleanup();
  renderBanner("script_failures", false);
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog).toHaveTextContent("Resume the task in its current status");
  expect(dialog).not.toHaveTextContent("holder");
});

it("continues after confirmation and reports the change", async () => {
  const { onChanged } = renderBanner();
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await confirm("Continue");
  expect(api.resumeTaskWorkflow).toHaveBeenCalledWith("REL-1", "continue");
  expect(onChanged).toHaveBeenCalled();
});

it("releases the holder after confirmation", async () => {
  const { onChanged } = renderBanner();
  fireEvent.click(screen.getByRole("button", { name: "Release holder" }));
  await confirm("Release holder");
  expect(api.resumeTaskWorkflow).toHaveBeenCalledWith("REL-1", "release");
  expect(onChanged).toHaveBeenCalled();
});

it("cancels the task after confirmation", async () => {
  const { onChanged } = renderBanner();
  fireEvent.click(screen.getByRole("button", { name: "Cancel task" }));
  await confirm("Cancel task");
  expect(api.cancelWorkflowTask).toHaveBeenCalledWith("REL-1");
  expect(api.resumeTaskWorkflow).not.toHaveBeenCalled();
  expect(onChanged).toHaveBeenCalled();
});

it("does nothing when the confirmation is declined", async () => {
  const { onChanged } = renderBanner();
  fireEvent.click(screen.getByRole("button", { name: "Cancel task" }));
  await confirm("Go back");
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  expect(api.cancelWorkflowTask).not.toHaveBeenCalled();
  expect(onChanged).not.toHaveBeenCalled();
});

it("shows a refused decision inline", async () => {
  api.resumeTaskWorkflow.mockRejectedValue(
    new ApiError(409, "workflow_not_paused", "the task is not paused"),
  );
  const { onChanged } = renderBanner();
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await confirm("Continue");
  expect(
    screen.getAllByRole("alert").some((node) => node.textContent === "the task is not paused"),
  ).toBe(true);
  expect(onChanged).not.toHaveBeenCalled();
});
