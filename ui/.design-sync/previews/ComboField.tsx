import { ComboField } from "tariboy-ui";

// ComboField is the labeled free-text input plus preset dropdown used for
// inline model/effort editing in the agent Overview header. It commits on blur,
// Enter, or preset select; the Overview remounts it (via `key`) when the
// committed value changes. Presets come from src/lib/runtimePresets.ts.

const MODEL_PRESETS = ["claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5", "claude-fable-5"];
const EFFORT_PRESETS = ["low", "medium", "high", "xhigh", "max", "ultracode"];

export const ModelField = () => (
  <ComboField label="model" value="claude-opus-4-8" presets={MODEL_PRESETS} onCommit={() => {}} />
);

export const EffortField = () => (
  <ComboField label="effort" value="high" presets={EFFORT_PRESETS} onCommit={() => {}} />
);

// Unset on a fresh agent: the input falls back to the label as its placeholder.
export const Unset = () => (
  <ComboField label="model" value="" presets={MODEL_PRESETS} onCommit={() => {}} />
);

// Both fields as they sit at the right end of the agent Overview header strip.
export const InOverviewHeader = () => (
  <div
    style={{
      width: 560,
      display: "flex",
      alignItems: "flex-end",
      gap: 12,
      borderRadius: 8,
      border: "1px solid var(--border)",
      padding: 12,
    }}
  >
    <div style={{ display: "flex", flexDirection: "column", gap: 2, marginRight: "auto" }}>
      <span style={{ fontSize: 14, fontWeight: 600 }}>builder</span>
      <span style={{ font: "12px ui-monospace, SFMono-Regular, Menlo, monospace", color: "var(--muted-foreground)" }}>
        claude · TB-142
      </span>
    </div>
    <ComboField label="model" value="claude-sonnet-5" presets={MODEL_PRESETS} onCommit={() => {}} />
    <ComboField label="effort" value="xhigh" presets={EFFORT_PRESETS} onCommit={() => {}} />
  </div>
);
