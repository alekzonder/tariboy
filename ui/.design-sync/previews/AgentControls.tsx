import { AgentControls } from "tariboy-ui";

// AgentControls is the header's action cluster for one agent: the primary
// action of its derived state, the Stop/Start toggle, and the `⋯` menu (Run
// once…, Force-stop, VS Code, Clone, Pin, Settings, Delete). `refresh` re-polls
// the agent snapshot after an action resolves.

export const AgentWorkspace = () => (
  <div style={{ width: 720, display: "flex", flexDirection: "column", gap: 8 }}>
    <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
      <span style={{ fontSize: 15, fontWeight: 600 }}>builder</span>
      <span
        style={{
          font: "12px ui-monospace, SFMono-Regular, Menlo, monospace",
          color: "var(--muted-foreground)",
        }}
      >
        worker:v2 · claude · running
      </span>
    </div>
    <AgentControls
      name="builder"
      alive
      image="worker:v2"
      primary="give"
      workPath="/agents/local/builder/work"
      settingsPath="/agents/local/builder/settings"
      refresh={() => {}}
    />
  </div>
);

export const StoppedAgent = () => (
  <div style={{ width: 720, display: "flex", flexDirection: "column", gap: 8 }}>
    <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
      <span style={{ fontSize: 15, fontWeight: 600 }}>docs-bot</span>
      <span
        style={{
          font: "12px ui-monospace, SFMono-Regular, Menlo, monospace",
          color: "var(--muted-foreground)",
        }}
      >
        bare:latest · codex · stopped
      </span>
    </div>
    <AgentControls
      name="docs-bot"
      alive={false}
      image="bare:latest"
      primary="start"
      workPath="/agents/local/docs-bot/work"
      settingsPath="/agents/local/docs-bot/settings"
      refresh={() => {}}
    />
  </div>
);

// An agent that waits for the user: Answer is the primary action.
export const NeedsYou = () => (
  <div style={{ width: 380 }}>
    <AgentControls
      name="packager"
      alive
      primary="answer"
      workPath="/agents/local/packager/work"
      settingsPath="/agents/local/packager/settings"
      refresh={() => {}}
    />
  </div>
);
