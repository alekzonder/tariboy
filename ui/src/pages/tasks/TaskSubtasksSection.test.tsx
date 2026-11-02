import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Queue, Task } from "@/lib/api";
import TaskSubtasksSection from "./TaskSubtasksSection";

const base: Task = {
  key: "TARI-1",
  queue: "TARI",
  parent_key: "",
  position: 0,
  priority: "P2",
  title: "Parent",
  description: "",
  status: "open",
  category: "open",
  author: "user:owner",
  customer: "user:owner",
  group: "",
  assignee: "",
  manual_block_reason: "",
  blocked: false,
  revision: 1,
  created_at: "2026-09-19T08:00:00Z",
  updated_at: "2026-09-19T08:00:00Z",
  completed_at: "",
  started_at: "",
  pull_request: "",
};
const queues: Queue[] = [
  {
    prefix: "TARIDESIGN",
    name: "Design",
    description: "",
    owners: [],
    responsible_agent: "",
    revision: 1,
    created_at: "2026-09-19T08:00:00Z",
    updated_at: "2026-09-19T08:00:00Z",
  },
];
const design: Task = {
  ...base,
  key: "TARIDESIGN-2",
  queue: "TARIDESIGN",
  parent_key: "TARI-1",
  title: "Design it",
  assignee: "agent:designer",
  status: "review",
  workflow_name: "design-flow",
  workflow_digest: "abc",
};
const closed: Task = {
  ...base,
  key: "TARI-3",
  parent_key: "TARI-1",
  title: "Old work",
  status: "done",
  category: "done",
};

function show(
  props: Partial<Parameters<typeof TaskSubtasksSection>[0]> = {},
): ReturnType<typeof vi.fn> {
  const onOpen = vi.fn();
  render(
    <TaskSubtasksSection
      task={base}
      subtasks={[design, closed]}
      hidden={0}
      queues={queues}
      fetching={false}
      error={false}
      onRetry={vi.fn()}
      onOpen={onOpen}
      {...props}
    />,
  );
  return onOpen;
}

describe("Subtasks section", () => {
  it("lists every direct subtask in the order the daemon gave", () => {
    show();

    const section = screen.getByRole("region", { name: "Subtasks" });
    expect(within(section).getByText("Subtasks")).toBeInTheDocument();
    expect(within(section).getByText("2")).toBeInTheDocument();
    const rows = within(section).getAllByRole("button", { name: /^Open subtask/ });
    expect(rows.map((row) => row.getAttribute("aria-label"))).toEqual([
      "Open subtask TARIDESIGN-2: Design it, Review",
      "Open subtask TARI-3: Old work, Done",
    ]);
    expect(within(rows[0]).getByText("designer")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Done")).toBeInTheDocument();
  });

  it("marks only a subtask from another queue", () => {
    show();

    const rows = screen.getAllByRole("button", { name: /^Open subtask/ });
    expect(
      within(rows[0]).getByRole("img", {
        name: "Queue TARIDESIGN · Design · workflow design-flow (parent queue TARI)",
      }),
    ).toBeInTheDocument();
    expect(within(rows[1]).queryByRole("img")).toBeNull();
  });

  it("opens a subtask by click, Enter and Space", async () => {
    const onOpen = show();

    const row = screen.getByRole("button", { name: /TARIDESIGN-2/ });
    await userEvent.click(row);
    row.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard(" ");

    expect(onOpen).toHaveBeenCalledTimes(3);
    expect(onOpen).toHaveBeenCalledWith("TARIDESIGN-2");
  });

  it("lays a row out in two lines on a phone and hides the assignee", () => {
    show();

    const row = screen.getByRole("button", { name: /TARIDESIGN-2/ });
    expect(row).toHaveClass("flex-wrap", "sm:flex-nowrap", "w-full");
    expect(within(row).getByText("Design it")).toHaveClass("order-3", "basis-full", "sm:order-2");
    expect(within(row).getByText("designer")).toHaveClass("hidden", "sm:inline-flex");
  });

  it("is hidden without subtasks", () => {
    show({ subtasks: [], hidden: 0 });
    expect(screen.queryByRole("region", { name: "Subtasks" })).toBeNull();
  });

  it("says how many subtasks are hidden", () => {
    show({ hidden: 1 });
    expect(screen.getByText("1 more subtask is hidden from you.")).toBeInTheDocument();
  });

  it("shows only the heading and the hidden count when every subtask is hidden", () => {
    show({ subtasks: [], hidden: 3 });

    const section = screen.getByRole("region", { name: "Subtasks" });
    expect(within(section).queryByRole("button")).toBeNull();
    expect(within(section).getByText("3 more subtasks are hidden from you.")).toBeInTheDocument();
  });

  it("shows two placeholders while the list is loading", () => {
    show({ subtasks: undefined, fetching: true });

    const section = screen.getByRole("region", { name: "Subtasks" });
    expect(within(section).getAllByTestId("subtask-placeholder")).toHaveLength(2);
  });

  it("is hidden when a finished read has no list", () => {
    show({ subtasks: undefined, fetching: false });
    expect(screen.queryByRole("region", { name: "Subtasks" })).toBeNull();
  });

  it("reports a failed read with a retry and no stale list", async () => {
    const onRetry = vi.fn();
    show({ error: true, onRetry });

    expect(screen.getByRole("alert")).toHaveTextContent("Could not load subtasks.");
    expect(screen.queryByRole("button", { name: /^Open subtask/ })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
