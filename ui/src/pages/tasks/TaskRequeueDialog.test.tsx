import type { ReactElement } from "react";
import { render as renderWith, screen, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import TaskRequeueDialog from "./TaskRequeueDialog";
import { fakeApi } from "@/test/api";

const api = vi.hoisted(() => ({ listTaskQueues: vi.fn() }));
const render = (ui: ReactElement) =>
  renderWith(ui, { api: fakeApi({ tasks: { listTaskQueues: api.listTaskQueues } }) });

const queue = (prefix: string, name: string) => ({
  prefix,
  name,
  description: "",
  owners: [],
  responsible_agent: "",
  revision: 1,
  created_at: "",
  updated_at: "",
});

beforeEach(() => {
  vi.clearAllMocks();
  api.listTaskQueues.mockResolvedValue({
    queues: [queue("DEV", "Development"), queue("OPS", "Operations")],
    count: 2,
  });
});

it("offers every queue except the task's own, warns that the workflow stops, and moves to the chosen one", async () => {
  const onRequeue = vi.fn().mockResolvedValue(undefined);
  const onOpenChange = vi.fn();
  render(
    <TaskRequeueDialog
      taskKey="DEV-1"
      queue="DEV"
      workflow
      open
      onOpenChange={onOpenChange}
      onRequeue={onRequeue}
    />,
  );

  const select = await screen.findByLabelText("Target queue");
  await within(select).findByText("OPS · Operations");
  expect(within(select).queryByText("DEV · Development")).toBeNull();
  expect(screen.getByText(/workflow stops/)).toBeInTheDocument();
  await userEvent.selectOptions(select, "OPS");
  await userEvent.click(screen.getByRole("button", { name: "Move task" }));

  expect(onRequeue).toHaveBeenCalledWith("OPS");
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it("reports a refusal and keeps the dialog open", async () => {
  const onRequeue = vi.fn().mockRejectedValue(new Error("task DEV-1 has open subtasks: DEV-2"));
  const onOpenChange = vi.fn();
  render(
    <TaskRequeueDialog
      taskKey="DEV-1"
      queue="DEV"
      workflow={false}
      open
      onOpenChange={onOpenChange}
      onRequeue={onRequeue}
    />,
  );

  const select = await screen.findByLabelText("Target queue");
  await within(select).findByText("OPS · Operations");
  expect(screen.queryByText(/workflow stops/)).toBeNull();
  await userEvent.selectOptions(select, "OPS");
  await userEvent.click(screen.getByRole("button", { name: "Move task" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("open subtasks: DEV-2");
  expect(onOpenChange).not.toHaveBeenCalledWith(false);
});
