import type { Meta, StoryObj } from "@storybook/react-vite";
import { Search } from "lucide-react";
import { useState } from "react";
import { Segmented } from "./segmented";

const meta = { component: Segmented } satisfies Meta<typeof Segmented>;
export default meta;

type Story = StoryObj;

// The real option set from src/pages/tasks/TaskFilterBar.tsx.
const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "closed", label: "Closed" },
  { value: "all", label: "All" },
] as const;

type StatusView = (typeof STATUS_OPTIONS)[number]["value"];

const TWO_OPTIONS = [
  { value: "mine", label: "Mine" },
  { value: "all", label: "Everyone" },
] as const;

// Segmented is a flex container, so a block parent stretches its trough to
// the full width. Every real call site sits in a flex row, so the stories
// mirror that.
function TaskStatusDemo() {
  const [view, setView] = useState<StatusView>("active");
  return (
    <div className="flex">
      <Segmented label="Task status" options={STATUS_OPTIONS} value={view} onChange={setView} />
    </div>
  );
}

function TwoOptionsDemo() {
  const [scope, setScope] = useState<"mine" | "all">("mine");
  return (
    <div className="flex">
      <Segmented label="Task scope" options={TWO_OPTIONS} value={scope} onChange={setScope} />
    </div>
  );
}

function FilterBarDemo() {
  const [view, setView] = useState<StatusView>("active");
  return (
    <div className="w-[560px] overflow-hidden rounded-(--panel-radius) border border-border bg-card">
      <div className="flex flex-wrap items-center gap-1.5 px-4 pt-[11px] pb-[9px]">
        <label className="flex h-7 w-[220px] shrink-0 items-center gap-[7px] rounded-[8px] bg-muted px-[9px] text-muted-foreground">
          <Search aria-hidden="true" className="size-3 shrink-0" />
          <span className="min-w-0 flex-1 text-[12px]">Search tasks</span>
        </label>
        <Segmented label="Task status" options={STATUS_OPTIONS} value={view} onChange={setView} />
      </div>
      <div className="border-t border-border px-4 py-2.5 text-xs text-muted-foreground">
        14 tasks
      </div>
    </div>
  );
}

export const TaskStatus: Story = { render: () => <TaskStatusDemo /> };

// The selected option is the variant axis: it is the only thing that moves.
export const Selection: Story = {
  render: () => (
    <div className="flex flex-col gap-2.5">
      {STATUS_OPTIONS.map((option) => (
        <div key={option.value} className="flex items-center gap-2.5">
          <span className="w-[52px] font-mono text-[11px] text-muted-foreground">
            {option.value}
          </span>
          <Segmented
            label="Task status"
            options={STATUS_OPTIONS}
            value={option.value}
            onChange={() => {}}
          />
        </div>
      ))}
    </div>
  ),
};

export const TwoOptions: Story = { render: () => <TwoOptionsDemo /> };

// How it sits in the console: the filter bar above the task table.
export const InFilterBar: Story = { render: () => <FilterBarDemo /> };
