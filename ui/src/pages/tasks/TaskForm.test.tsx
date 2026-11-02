import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Queue } from "@/lib/api";
import TaskForm from "./TaskForm";

const queue = (prefix: string): Queue => ({
  prefix,
  name: prefix,
  description: "",
  owners: [],
  responsible_agent: "",
  revision: 1,
  created_at: "2026-09-19T08:00:00Z",
  updated_at: "2026-09-19T08:00:00Z",
});

describe("task form", () => {
  it("fixes a child to its parent's queue and names it", async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <TaskForm
        queues={[queue("TARIDESIGN"), queue("TARI")]}
        initialQueue="TARI"
        parentKey="TARI-1"
        onCreate={onCreate}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.queryByRole("combobox", { name: "Task queue" })).toBeNull();
    expect(screen.getByText("Queue: TARI")).toBeInTheDocument();
    await userEvent.type(screen.getByRole("textbox", { name: "Task title" }), "Child");
    await userEvent.click(screen.getByRole("button", { name: "Create task" }));

    expect(onCreate).toHaveBeenCalledWith({ queue: "TARI", parent_key: "TARI-1", title: "Child" });
  });

  it("lets a root task pick its queue", () => {
    render(
      <TaskForm
        queues={[queue("TARIDESIGN"), queue("TARI")]}
        initialQueue="TARI"
        onCreate={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole("combobox", { name: "Task queue" })).toHaveValue("TARI");
    expect(screen.queryByText(/^Queue: /)).toBeNull();
  });
});
