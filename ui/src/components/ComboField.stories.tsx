import type { Meta, StoryObj } from "@storybook/react-vite";
import { ComboField } from "./ComboField";

// ComboField is the labeled free-text input plus preset dropdown used for
// inline model/effort editing in the agent Overview header. It commits on blur,
// Enter, or preset select. Presets come from src/lib/runtimePresets.ts.
const meta = { component: ComboField } satisfies Meta<typeof ComboField>;
export default meta;

type Story = StoryObj;

const MODEL_PRESETS = ["claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5", "claude-fable-5"];
const EFFORT_PRESETS = ["low", "medium", "high", "xhigh", "max", "ultracode"];

export const ModelField: Story = {
  render: () => (
    <ComboField label="model" value="claude-opus-4-8" presets={MODEL_PRESETS} onCommit={() => {}} />
  ),
};

export const EffortField: Story = {
  render: () => (
    <ComboField label="effort" value="high" presets={EFFORT_PRESETS} onCommit={() => {}} />
  ),
};

// Unset on a fresh agent: the input falls back to the label as its placeholder.
export const Unset: Story = {
  render: () => <ComboField label="model" value="" presets={MODEL_PRESETS} onCommit={() => {}} />,
};

// Both fields as they sit at the right end of the agent Overview header strip.
export const InOverviewHeader: Story = {
  render: () => (
    <div className="flex w-[560px] items-end gap-3 rounded-lg border border-border p-3">
      <div className="mr-auto flex flex-col gap-0.5">
        <span className="text-sm font-semibold">builder</span>
        <span className="font-mono text-xs text-muted-foreground">claude · TB-142</span>
      </div>
      <ComboField
        label="model"
        value="claude-sonnet-5"
        presets={MODEL_PRESETS}
        onCommit={() => {}}
      />
      <ComboField label="effort" value="xhigh" presets={EFFORT_PRESETS} onCommit={() => {}} />
    </div>
  ),
};
