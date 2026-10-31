import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Dialog } from "@/components/ui/dialog";
import { workflowTask } from "@/test/fixtures/workflowFixtures";
import { TaskDetailHeader } from "./TaskDetailHeader";

function renderHeader({
  access = "manage",
  dirty = false,
  title = "Task title",
  pending = false,
} = {}) {
  const save = vi.fn().mockResolvedValue(undefined);
  function Header() {
    const [draft, setTitle] = useState(title);
    return (
      <Dialog open>
        <TaskDetailHeader
          task={{ ...workflowTask, title, access: access as typeof workflowTask.access }}
          title={draft}
          setTitle={setTitle}
          dirty={dirty || draft !== title}
          pending={pending}
          save={save}
          close={vi.fn()}
          initialFocusRef={null}
          onRequeue={vi.fn()}
          onTransfer={vi.fn()}
        />
      </Dialog>
    );
  }
  render(<Header />);
  return { save };
}

it("edits the single header title as a draft until Save task is clicked", async () => {
  const { save } = renderHeader();
  expect(screen.getAllByText("Task title")).toHaveLength(1);
  expect(screen.queryByRole("textbox", { name: "Title" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Save task" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Edit title" }));
  const input = screen.getByRole("textbox", { name: "Title" });
  expect(input).toHaveFocus();
  await userEvent.clear(input);
  await userEvent.type(input, "Changed title");
  await userEvent.tab();
  expect(screen.queryByRole("textbox", { name: "Title" })).not.toBeInTheDocument();
  expect(screen.getAllByText("Changed title")).toHaveLength(1);
  expect(save).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "Save task" }));
  expect(save).toHaveBeenCalledOnce();
});

it("disables saving a blank draft title", async () => {
  renderHeader();
  await userEvent.click(screen.getByRole("button", { name: "Edit title" }));
  await userEvent.clear(screen.getByRole("textbox", { name: "Title" }));
  expect(screen.getByRole("button", { name: "Save task" })).toBeDisabled();
});

it("shows Save task for dirty fields even when the title is unchanged", () => {
  renderHeader({ dirty: true });
  expect(screen.getByRole("button", { name: "Save task" })).toBeEnabled();
});

it.each(["context", "respond"])("keeps %s access read-only", (access) => {
  renderHeader({ access, dirty: true });
  expect(screen.getByText("Task title")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Edit title" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Save task" })).not.toBeInTheDocument();
});

it("disables title editing and saving while pending", () => {
  renderHeader({ pending: true, dirty: true });
  expect(screen.getByRole("button", { name: "Edit title" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Save task" })).toBeDisabled();
});
