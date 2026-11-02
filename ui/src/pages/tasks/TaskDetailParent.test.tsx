import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import type { Queue, Task, TaskParent } from "@/lib/api";
import { TaskDetailHeader } from "./TaskDetailHeader";
import { TaskParentField } from "./TaskDetailParent";

const task: Task = {
  key: "TARIDESIGN-2",
  queue: "TARIDESIGN",
  parent_key: "TARI-1",
  position: 0,
  priority: "P2",
  title: "Design the tree",
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
    prefix: "TARI",
    name: "Tasks",
    description: "",
    owners: [],
    responsible_agent: "",
    revision: 1,
    created_at: "2026-09-19T08:00:00Z",
    updated_at: "2026-09-19T08:00:00Z",
  },
];
const parent: TaskParent = {
  key: "TARI-1",
  title: "Ship the tree",
  queue: "TARI",
  access: "write",
};

function header(over: { task?: Task; parent?: TaskParent }) {
  const onOpenParent = vi.fn();
  render(
    <Dialog open>
      <DialogContent aria-describedby={undefined}>
        <TaskDetailHeader
          task={over.task ?? task}
          parent={over.parent}
          queues={queues}
          onOpenParent={onOpenParent}
          pending={false}
          title={(over.task ?? task).title}
          setTitle={vi.fn()}
          dirty={false}
          save={vi.fn()}
          close={vi.fn()}
          initialFocusRef={createRef()}
          onRequeue={vi.fn()}
          onTransfer={vi.fn()}
        />
      </DialogContent>
    </Dialog>,
  );
  return onOpenParent;
}

describe("the parent line in the panel header", () => {
  it("opens the parent from one button with its key and title", async () => {
    const onOpenParent = header({ parent });

    const button = screen.getByRole("button", { name: "Open parent task TARI-1: Ship the tree" });
    expect(button).toHaveTextContent("Subtask of");
    expect(button).toHaveTextContent("TARI-1");
    expect(button).toHaveTextContent("Ship the tree");
    expect(screen.getByText("Ship the tree")).toHaveClass("truncate");
    await userEvent.click(button);
    button.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard(" ");
    expect(onOpenParent).toHaveBeenCalledTimes(3);
    expect(onOpenParent).toHaveBeenCalledWith("TARI-1");
  });

  it("marks a parent in another queue before its key", () => {
    header({ parent });

    const marker = screen.getByRole("img", { name: "Parent queue TARI · Tasks" });
    const key = screen.getByText("TARI-1");
    expect(marker.compareDocumentPosition(key) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("has no marker for a parent in the same queue", () => {
    header({ task: { ...task, queue: "TARI", key: "TARI-2" }, parent });
    expect(screen.queryByRole("img", { name: /^Parent queue/ })).toBeNull();
  });

  it("names a hidden parent as text without a button or title", () => {
    header({});

    expect(screen.queryByRole("button", { name: /Open parent task/ })).toBeNull();
    const line = screen.getByTitle("You cannot open this task");
    expect(line).toHaveTextContent("Subtask of TARI-1");
    expect(line).not.toHaveTextContent("Ship the tree");
  });

  it("is absent for a root task", () => {
    header({ task: { ...task, parent_key: "" } });
    expect(screen.queryByText(/Subtask of/)).toBeNull();
  });
});

describe("the Parent field", () => {
  it("is a button that opens the parent", async () => {
    const onOpen = vi.fn();
    render(<TaskParentField parentKey="TARI-1" parent={parent} onOpen={onOpen} />);

    await userEvent.click(
      screen.getByRole("button", { name: "Open parent task TARI-1: Ship the tree" }),
    );
    expect(onOpen).toHaveBeenCalledWith("TARI-1");
  });

  it("is text for a hidden parent", () => {
    render(<TaskParentField parentKey="TARI-1" onOpen={vi.fn()} />);

    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByTitle("You cannot open this task")).toHaveTextContent("TARI-1");
  });
});
