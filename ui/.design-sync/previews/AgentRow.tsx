import { AgentRow } from "tariboy-ui";

// AgentRow is a sidebar row: at rest it has no background of its own (only a
// hover), so every cell sits on --sidebar, which is the surface it is designed
// against. Without that backdrop the unselected rows read as floating text.
const sidebar = (children: React.ReactNode, width = 260) => (
  <div
    style={{
      width,
      padding: 6,
      borderRadius: "var(--panel-radius)",
      background: "var(--sidebar)",
      border: "1px solid var(--sidebar-border)",
      color: "var(--sidebar-foreground)",
    }}
  >
    {children}
  </div>
);

const now = new Date("2026-10-30T12:30:00Z");
const working = { kind: "working", since: "2026-10-30T12:18:00Z" } as const;
const idle = { kind: "idle" } as const;
const off = { kind: "off" } as const;
const needs = { kind: "needs", needs: { count: 1 } } as const;
const error = { kind: "error", reason: "Out of budget: day" } as const;

// The derived state: needs you is the accent, working is live, error is
// danger, off is quiet text, and an idle row carries no pill at all.
export const States = () =>
  sidebar(
    <>
      <AgentRow name="planner" agentState={needs} now={now} />
      <AgentRow name="builder" agentState={working} now={now} />
      <AgentRow name="reviewer" agentState={idle} now={now} />
      <AgentRow name="packager" agentState={off} now={now} />
      <AgentRow name="docs-bot" agentState={error} now={now} />
    </>,
  );

// The selected row is the same island as the content panel — --card lifted by
// --raise — so the selection reads as the row the panel belongs to.
export const Selected = () =>
  sidebar(
    <>
      <AgentRow name="builder" agentState={working} now={now} selected />
      <AgentRow name="reviewer" agentState={idle} now={now} />
      <AgentRow name="packager" agentState={off} now={now} />
    </>,
  );

// The quiet marker of an agent with no tty.
export const Markers = () =>
  sidebar(
    <>
      <AgentRow name="builder" agentState={working} now={now} />
      <AgentRow name="reviewer" agentState={working} now={now} interactive={false} />
      <AgentRow name="packager" agentState={off} now={now} />
    </>,
  );

// The whole sidebar list as the console draws it, with the selected agent and
// a header above it.
export const InSidebar = () =>
  sidebar(
    <>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "2px 10px 8px",
          fontSize: 11,
          letterSpacing: ".04em",
          textTransform: "uppercase",
          color: "var(--muted-foreground)",
        }}
      >
        <span>Agents</span>
        <span>4</span>
      </div>
      <AgentRow name="builder" agentState={needs} now={now} selected />
      <AgentRow name="reviewer" agentState={working} now={now} interactive={false} />
      <AgentRow name="packager" agentState={idle} now={now} />
      <AgentRow name="docs-bot" agentState={error} now={now} />
    </>,
  );
