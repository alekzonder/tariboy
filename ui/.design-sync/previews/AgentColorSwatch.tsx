import { AgentColorSwatch } from "tariboy-ui";

// AgentColorSwatch is the round per-agent color affordance that sits at the
// right end of the agent workspace tab strip (AgentLayout). It is 20px, so every
// cell composes it in the row it actually ships in rather than on its own.
// The hex editor lives behind a Dialog the viewer opens by clicking the swatch.

const row = {
  display: "flex",
  alignItems: "center",
  gap: 8,
  fontSize: 14,
} as const;

const mono = {
  font: "12px ui-monospace, SFMono-Regular, Menlo, monospace",
  color: "var(--muted-foreground)",
} as const;

export const InAgentTabStrip = () => (
  <div style={{ width: 460, border: "1px solid var(--border)", borderRadius: 8, overflow: "hidden" }}>
    <div style={{ ...row, borderBottom: "1px solid var(--border)", padding: "0 4px 0 12px" }}>
      <span style={{ padding: "8px 12px 6px 0", fontWeight: 500, borderBottom: "2px solid var(--primary)" }}>
        Overview
      </span>
      <span style={{ padding: "8px 12px", color: "var(--muted-foreground)" }}>Audit</span>
      <span style={{ padding: "8px 12px", color: "var(--muted-foreground)" }}>Configuration</span>
      <span style={{ marginLeft: "auto", display: "flex", alignItems: "center", paddingRight: 4 }}>
        <AgentColorSwatch name="builder" color="#4f46e5" onSaved={() => {}} />
      </span>
    </div>
    <div style={{ padding: 12, ...mono }}>agent builder · goal TB-142</div>
  </div>
);

export const AgentPalette = () => (
  <div style={{ display: "flex", flexDirection: "column", gap: 10, width: 260 }}>
    {[
      ["builder", "#4f46e5"],
      ["reviewer", "#059669"],
      ["packager", "#d97706"],
      ["docs-bot", "#db2777"],
    ].map(([name, hex]) => (
      <div key={name} style={row}>
        <AgentColorSwatch name={name} color={hex} onSaved={() => {}} />
        <span style={{ fontWeight: 500 }}>{name}</span>
        <span style={{ marginLeft: "auto", ...mono }}>{hex}</span>
      </div>
    ))}
  </div>
);

export const NoColorSet = () => (
  <div style={{ ...row, width: 260 }}>
    <AgentColorSwatch name="docs-bot" />
    <span style={{ fontWeight: 500 }}>docs-bot</span>
    <span style={{ marginLeft: "auto", ...mono }}>no color set</span>
  </div>
);
