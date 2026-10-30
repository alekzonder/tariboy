import type { ComponentProps } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import { TaskFilterBar } from "./TaskFilterBar";
import { resetTaskColumns } from "./taskColumnLayout";

beforeEach(() => resetTaskColumns());

function renderBar(props: Partial<ComponentProps<typeof TaskFilterBar>> = {}) {
  const handlers = {
    onQuery: vi.fn(),
    onStatusView: vi.fn(),
    onQueue: vi.fn(),
    onManageQueues: vi.fn(),
    onAssignee: vi.fn(),
    onWaiting: vi.fn(),
    onRefresh: vi.fn(),
    onCreate: vi.fn(),
  };
  render(
    <TaskFilterBar
      mode="all"
      query=""
      statusView="active"
      queues={[{ prefix: "TEST" }, { prefix: "OPS" }]}
      queue=""
      queueCounts={
        new Map([
          ["TEST", 3],
          ["OPS", 1],
        ])
      }
      assignee=""
      agents={["worker", "triager"]}
      waiting={false}
      refreshing={false}
      {...handlers}
      {...props}
    />,
  );
  return handlers;
}

const filtersButton = () => screen.getByRole("button", { name: /^Filters/ });

async function openSub(name: RegExp) {
  await userEvent.click(filtersButton());
  await userEvent.click(await screen.findByRole("menuitem", { name }));
}

describe("TaskFilterBar", () => {
  it("lays every control out in one toolbar row that cannot wrap", () => {
    renderBar({ hostControl: <button type="button">Host: local</button> });
    const toolbar = screen.getByRole("toolbar", { name: "Task filters" });
    expect(toolbar).toHaveAttribute("data-toolbar");
    expect(toolbar).toHaveClass("flex-nowrap");
    expect(toolbar).not.toHaveClass("flex-wrap");
    const host = within(toolbar).getByRole("button", { name: "Host: local" });
    const search = within(toolbar).getByLabelText("Search tasks");
    expect(host.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(search.closest("[data-slot=search-input]")).toHaveClass("min-w-0");
    expect(within(toolbar).getByRole("button", { name: "New task" })).toBeInTheDocument();
    expect(within(toolbar).getByRole("button", { name: "Refresh tasks" })).toBeInTheDocument();
  });

  it("offers Active, Done and All", async () => {
    const { onStatusView } = renderBar();
    const status = screen.getByRole("radiogroup", { name: "Task status" });
    expect(
      within(status)
        .getAllByRole("radio")
        .map((radio) => radio.textContent),
    ).toEqual(["Active", "Done", "All"]);
    await userEvent.click(within(status).getByRole("radio", { name: "Done" }));
    expect(onStatusView).toHaveBeenCalledWith("closed");
  });

  it("moves secondary filters into one Filters menu that carries the tour anchor", () => {
    renderBar();
    expect(filtersButton()).toHaveAttribute("data-tour", TOUR_ANCHORS.queueMenu);
    expect(filtersButton()).toHaveAccessibleName("Filters");
    expect(screen.queryByRole("button", { name: /^Queue/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "My tasks" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Waiting for me" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Columns" })).toBeNull();
  });

  it("counts the filters that are on", () => {
    renderBar({ queue: "TEST", assignee: "mine", waiting: true });
    expect(filtersButton()).toHaveAccessibleName("Filters, 3 on");
    expect(within(filtersButton()).getByText("3")).toBeInTheDocument();
  });

  it("picks a queue with its count and opens queue management", async () => {
    const { onQueue, onManageQueues } = renderBar({ queue: "OPS" });
    await openSub(/^Queue/);
    expect(screen.getByRole("menuitemradio", { name: /^OPS/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    const test = screen.getByRole("menuitemradio", { name: /^TEST/ });
    expect(test).toHaveTextContent("TEST3");
    await userEvent.click(test);
    expect(onQueue).toHaveBeenCalledWith("TEST");

    await openSub(/^Queue/);
    await userEvent.click(screen.getByRole("menuitem", { name: "Manage queues…" }));
    expect(onManageQueues).toHaveBeenCalled();
  });

  it("picks Anyone, My tasks or one agent as the assignee", async () => {
    const { onAssignee } = renderBar();
    await openSub(/^Assignee/);
    expect(screen.getAllByRole("menuitemradio").map((item) => item.textContent?.trim())).toEqual([
      "Anyone",
      "My tasks",
      "worker",
      "triager",
    ]);
    await userEvent.click(screen.getByRole("menuitemradio", { name: "triager" }));
    expect(onAssignee).toHaveBeenCalledWith("agent:triager");

    await openSub(/^Assignee/);
    await userEvent.click(screen.getByRole("menuitemradio", { name: "My tasks" }));
    expect(onAssignee).toHaveBeenLastCalledWith("mine");
  });

  it("toggles Waiting for me", async () => {
    const { onWaiting } = renderBar({ waiting: true });
    await userEvent.click(filtersButton());
    const waiting = await screen.findByRole("menuitemcheckbox", { name: "Waiting for me" });
    expect(waiting).toHaveAttribute("aria-checked", "true");
    await userEvent.click(waiting);
    expect(onWaiting).toHaveBeenCalledWith(false);
  });

  it("keeps the column layout under Columns", async () => {
    renderBar({ mode: "agent" });
    await openSub(/^Columns/);
    expect(screen.getAllByTestId("task-column-menu-row").map((row) => row.dataset.column)).toEqual([
      "priority",
      "key",
      "task",
      "status",
      "duration",
      "updated",
    ]);
  });

  it("keeps the scope agent chip in the row", () => {
    renderBar({ mode: "agent", scopeAgent: "worker" });
    const toolbar = screen.getByRole("toolbar", { name: "Task filters" });
    expect(within(toolbar).getByText("worker")).toBeInTheDocument();
  });
});
