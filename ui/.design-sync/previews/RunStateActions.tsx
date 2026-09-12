import { RunStateActions } from "tariboy-ui";

// RunStateActions is the start/stop/restart trio on its own — what the
// Configuration page's run-state strip mounts so it shares AgentControls'
// handlers without carrying Kill, Remove or the exec prompt.

export const RunStatePanel = () => (
  <section style={{ width: 560, borderRadius: 8, border: "1px solid var(--border)", padding: 16 }}>
    <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>Run state</h3>
    <p style={{ margin: "4px 0 0", fontSize: 13, color: "var(--muted-foreground)" }}>
      The master switch permits the agent to run; Loop schedules new autonomous iterations.
    </p>
    <div style={{ marginTop: 16, display: "flex", flexDirection: "column", gap: 6 }}>
      <p style={{ margin: 0, fontSize: 13 }}>
        <span style={{ color: "var(--muted-foreground)" }}>Master switch</span>{" "}
        <span style={{ fontWeight: 500 }}>Enabled</span>
      </p>
      <RunStateActions name="builder" refresh={() => {}} />
      <p style={{ margin: 0, fontSize: 12, color: "var(--muted-foreground)" }}>Takes effect immediately.</p>
    </div>
  </section>
);

export const DisabledAgentPanel = () => (
  <section style={{ width: 560, borderRadius: 8, border: "1px solid var(--border)", padding: 16 }}>
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <p style={{ margin: 0, fontSize: 13 }}>
        <span style={{ color: "var(--muted-foreground)" }}>Master switch</span>{" "}
        <span style={{ fontWeight: 500 }}>Disabled</span>
      </p>
      <RunStateActions name="docs-bot" refresh={() => {}} />
      <p style={{ margin: 0, fontSize: 12, color: "var(--muted-foreground)" }}>
        docs-bot will not schedule iterations until it is started.
      </p>
    </div>
  </section>
);

export const InlineOnAgentRow = () => (
  <div
    style={{
      width: 560,
      display: "flex",
      alignItems: "center",
      gap: 12,
      borderRadius: 8,
      border: "1px solid var(--border)",
      padding: "10px 12px",
    }}
  >
    <span style={{ fontSize: 14, fontWeight: 500 }}>reviewer</span>
    <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--muted-foreground)" }}>
      build-01
    </span>
    <span style={{ marginLeft: "auto" }}>
      <RunStateActions name="reviewer" refresh={() => {}} />
    </span>
  </div>
);
