import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import {
  defaultTaskColumnLayout,
  readTaskColumnLayout,
  resetTaskColumns,
  setTaskColumnWidth,
} from "./taskColumnLayout";
import TaskColumnsMenu from "./TaskColumnsMenu";

beforeEach(() => resetTaskColumns());

async function openMenu(mode: "agent" | "all" | "servers" = "agent") {
  render(<TaskColumnsMenu mode={mode} />);
  await userEvent.click(screen.getByRole("button", { name: "Columns" }));
}

const item = (name: string) => screen.getByRole("menuitem", { name });

describe("task columns menu", () => {
  it("lists the columns of the current table in their order", async () => {
    await openMenu("all");
    const names = screen.getAllByTestId("task-column-menu-row").map((row) => row.dataset.column);
    expect(names).toEqual(["priority", "key", "task", "agent", "status", "updated"]);
  });

  it("moves a column left and right with one tap each and stays open", async () => {
    await openMenu();
    expect(item("Move Pri left")).toHaveAttribute("data-disabled");
    expect(item("Move Updated right")).toHaveAttribute("data-disabled");

    await userEvent.click(item("Move Status left"));
    // `task` and `status` swap; columns hidden in this table keep their place.
    expect(readTaskColumnLayout().order).toEqual([
      "priority",
      "key",
      "status",
      "task",
      "agent",
      "server",
      "duration",
      "updated",
    ]);
    await userEvent.click(item("Move Pri right"));
    expect(readTaskColumnLayout().order.slice(0, 2)).toEqual(["key", "priority"]);
    const names = screen.getAllByTestId("task-column-menu-row").map((row) => row.dataset.column);
    expect(names).toEqual(["key", "priority", "status", "task", "duration", "updated"]);
  });

  it("narrows and widens a column in 16px steps within its bounds", async () => {
    setTaskColumnWidth("key", 104);
    await openMenu();

    await userEvent.click(item("Widen Status"));
    expect(readTaskColumnLayout().widths.status).toBe(164);
    await userEvent.click(item("Narrow Key"));
    expect(readTaskColumnLayout().widths.key).toBe(96);
    expect(item("Narrow Key")).toHaveAttribute("data-disabled");
    expect(screen.queryByRole("menuitem", { name: "Widen Task" })).toBeNull();
  });

  it("resets every column", async () => {
    setTaskColumnWidth("status", 300);
    await openMenu();
    await userEvent.click(item("Move Updated left"));
    await userEvent.click(item("Reset columns"));
    expect(readTaskColumnLayout()).toEqual(defaultTaskColumnLayout());
  });
});
