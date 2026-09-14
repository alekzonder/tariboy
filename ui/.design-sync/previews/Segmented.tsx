import { useState } from "react";
import { Segmented } from "tariboy-ui";
import { Search } from "lucide-react";

// The real option set from src/pages/tasks/TaskFilterBar.tsx.
const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "closed", label: "Closed" },
  { value: "all", label: "All" },
] as const;

type StatusView = (typeof STATUS_OPTIONS)[number]["value"];

export const TaskStatus = () => {
  const [view, setView] = useState<StatusView>("active");
  return (
    <Segmented
      label="Task status"
      options={STATUS_OPTIONS}
      value={view}
      onChange={setView}
    />
  );
};

// The selected option is the variant axis: it is the only thing that moves,
// and it is what lifts one button onto --card with --raise.
export const Selection = () => (
  <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
    {STATUS_OPTIONS.map((option) => (
      <div key={option.value} style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <span
          style={{
            width: 52,
            fontSize: 11,
            fontFamily: "var(--font-mono)",
            color: "var(--muted-foreground)",
          }}
        >
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
);

export const TwoOptions = () => {
  const [scope, setScope] = useState<"mine" | "all">("mine");
  return (
    <Segmented
      label="Task scope"
      options={[
        { value: "mine", label: "Mine" },
        { value: "all", label: "Everyone" },
      ]}
      value={scope}
      onChange={setScope}
    />
  );
};

// How it actually sits in the console: the filter bar above the task table,
// next to the search field. Spacing mirrors TaskFilterBar.
export const InFilterBar = () => {
  const [view, setView] = useState<StatusView>("active");
  return (
    <div
      style={{
        width: 560,
        border: "1px solid var(--border)",
        borderRadius: "var(--panel-radius)",
        background: "var(--card)",
        overflow: "hidden",
      }}
    >
      <div className="flex flex-wrap items-center gap-1.5 px-4 pt-[11px] pb-[9px]">
        <label className="flex h-7 w-[220px] shrink-0 items-center gap-[7px] rounded-[8px] bg-muted px-[9px] text-muted-foreground">
          <Search aria-hidden="true" className="size-3 shrink-0" />
          <span className="min-w-0 flex-1 text-[12px]">Search tasks</span>
        </label>
        <Segmented
          label="Task status"
          options={STATUS_OPTIONS}
          value={view}
          onChange={setView}
        />
      </div>
      <div
        style={{
          borderTop: "1px solid var(--border)",
          padding: "10px 16px",
          fontSize: 12,
          color: "var(--muted-foreground)",
        }}
      >
        14 tasks
      </div>
    </div>
  );
};
