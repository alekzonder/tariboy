import { AgentControls } from "tariboy-ui";

// AgentControls is the full lifecycle strip for one agent: the RunStateActions
// trio, the destructive Kill/Remove pair (each behind an AlertDialog), and the
// one-shot exec prompt pushed to the right. `refresh` re-polls the agent
// snapshot after an action resolves.

export const AgentWorkspace = () => (
  <div style={{ width: 720, display: "flex", flexDirection: "column", gap: 8 }}>
    <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
      <span style={{ fontSize: 15, fontWeight: 600 }}>builder</span>
      <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--muted-foreground)" }}>
        worker:v2 · claude · running
      </span>
    </div>
    <AgentControls name="builder" refresh={() => {}} />
  </div>
);

export const StoppedAgent = () => (
  <div style={{ width: 720, display: "flex", flexDirection: "column", gap: 8 }}>
    <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
      <span style={{ fontSize: 15, fontWeight: 600 }}>docs-bot</span>
      <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--muted-foreground)" }}>
        bare:latest · codex · stopped
      </span>
    </div>
    <AgentControls name="docs-bot" refresh={() => {}} />
  </div>
);

// The strip is `flex-wrap`: in a narrow column (the agent sidebar width) the
// exec prompt drops onto its own line instead of squeezing the buttons.
export const NarrowColumn = () => (
  <div style={{ width: 380 }}>
    <AgentControls name="packager" refresh={() => {}} />
  </div>
);
