import type { Meta, StoryObj } from "@storybook/react-vite";
import { withSurface } from "@/storybook/decorators";
import { NOW, minutesAgo } from "@/fixtures/time";
import { AgentRow } from "./AgentRow.view";

// AgentRow is a sidebar row: at rest it has no background of its own (only a
// hover), so every cell sits on the sidebar surface it is designed against.
const meta = {
  component: AgentRow,
  decorators: [withSurface("sidebar", 260)],
} satisfies Meta<typeof AgentRow>;
export default meta;

type Story = StoryObj;

const now = new Date(NOW);
const working = { kind: "working", since: minutesAgo(12) } as const;
const idle = { kind: "idle" } as const;
const off = { kind: "off" } as const;
const needs = { kind: "needs", needs: { count: 1 } } as const;
const error = { kind: "error", reason: "Out of budget: day" } as const;

// The derived state: needs you is the accent, working is live, error is
// danger, off is quiet text, and an idle row carries no pill at all.
export const States: Story = {
  render: () => (
    <>
      <AgentRow name="planner" agentState={needs} now={now} />
      <AgentRow name="builder" agentState={working} now={now} />
      <AgentRow name="reviewer" agentState={idle} now={now} />
      <AgentRow name="packager" agentState={off} now={now} />
      <AgentRow name="docs-bot" agentState={error} now={now} />
    </>
  ),
};

// The selected row is the same island as the content panel, so the selection
// reads as the row the panel belongs to.
export const Selected: Story = {
  render: () => (
    <>
      <AgentRow name="builder" agentState={working} now={now} selected />
      <AgentRow name="reviewer" agentState={idle} now={now} />
      <AgentRow name="packager" agentState={off} now={now} />
    </>
  ),
};

// The quiet marker of an agent with no tty.
export const Markers: Story = {
  render: () => (
    <>
      <AgentRow name="builder" agentState={working} now={now} />
      <AgentRow name="reviewer" agentState={working} now={now} interactive={false} />
      <AgentRow name="packager" agentState={off} now={now} />
    </>
  ),
};

// The whole sidebar list as the console draws it, with the selected agent and
// a header above it.
export const InSidebar: Story = {
  render: () => (
    <>
      <div className="flex items-center justify-between px-2.5 pt-0.5 pb-2 text-[11px] tracking-[.04em] text-muted-foreground uppercase">
        <span>Agents</span>
        <span>4</span>
      </div>
      <AgentRow name="builder" agentState={needs} now={now} selected />
      <AgentRow name="reviewer" agentState={working} now={now} interactive={false} />
      <AgentRow name="packager" agentState={idle} now={now} />
      <AgentRow name="docs-bot" agentState={error} now={now} />
    </>
  ),
};
