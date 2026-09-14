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

// agentTone(): running is live, failed is danger, everything else quiet —
// and an exhausted budget outranks the state in both the dot and the pill.
export const States = () =>
  sidebar(
    <>
      <AgentRow name="builder" state="running" />
      <AgentRow name="reviewer" state="stopped" />
      <AgentRow name="packager" state="failed" />
      <AgentRow name="docs-bot" state="running" outOfBudget />
    </>,
  );

// The selected row is the same island as the content panel — --card lifted by
// --raise — so the selection reads as the row the panel belongs to.
export const Selected = () =>
  sidebar(
    <>
      <AgentRow name="builder" state="running" selected />
      <AgentRow name="reviewer" state="stopped" />
      <AgentRow name="packager" state="stopped" />
    </>,
  );

// The two quiet markers: an unread customer question, and an agent with no tty.
export const Markers = () =>
  sidebar(
    <>
      <AgentRow
        name="builder"
        state="running"
        unread
        unreadLabel="Unread customer question for builder"
      />
      <AgentRow name="reviewer" state="running" interactive={false} />
      <AgentRow name="packager" state="stopped" />
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
      <AgentRow name="builder" state="running" selected unread />
      <AgentRow name="reviewer" state="running" interactive={false} />
      <AgentRow name="packager" state="failed" />
      <AgentRow name="docs-bot" state="stopped" outOfBudget />
    </>,
  );
