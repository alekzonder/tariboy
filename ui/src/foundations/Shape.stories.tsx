import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { scale } from "@/storybook/tokens";

// Radius, elevation, layers and control sizes: the shape of the interface.
const meta = {
  title: "Foundations/Shape",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const RADIUS_USE: Record<string, string> = {
  mark: "A progress bar, a tooltip arrow, a tiny mark.",
  chip: "A status pill, a tag, a kbd, a small hover target.",
  control: "A button, an input, a select, a row, a menu item.",
  card: "A card, a menu, a popover, a bordered box.",
  panel: "The content island, a dialog.",
};

export const Radius: Story = {
  render: () => (
    <div className="flex flex-wrap gap-6 bg-card p-6 text-foreground">
      {scale("radius").map((step) => (
        <div key={step.name} className="flex w-44 flex-col gap-2">
          <span
            className="h-20 border border-border bg-muted"
            style={{ borderRadius: `var(--radius-${step.name})` }}
          />
          <span className="font-mono text-ui">
            rounded-{step.name} · {step.value}
          </span>
          <span className="text-caption text-muted-foreground">
            {RADIUS_USE[step.name] ?? "No use written yet: add it to Shape.stories.tsx."}
          </span>
        </div>
      ))}
    </div>
  ),
};

/** Elevation has two levels, never three. */
export const Elevation: Story = {
  render: () => (
    <div className="flex flex-wrap gap-8 bg-background p-8 text-foreground">
      <div className="flex w-56 flex-col gap-2">
        <span className="flex h-24 items-center justify-center rounded-card bg-card font-mono text-ui shadow-none">
          shadow-none
        </span>
        <span className="text-caption text-muted-foreground">
          The default. A border or a fill separates things.
        </span>
      </div>
      <div className="flex w-56 flex-col gap-2">
        <span className="flex h-24 items-center justify-center rounded-card bg-card font-mono text-ui shadow-raise">
          shadow-raise
        </span>
        <span className="text-caption text-muted-foreground">
          A selected row and the active segment of a segmented control.
        </span>
      </div>
      <div className="flex w-56 flex-col gap-2">
        <span className="flex h-24 items-center justify-center rounded-panel bg-card font-mono text-ui shadow-lift">
          shadow-lift
        </span>
        <span className="text-caption text-muted-foreground">
          The content island, a menu, a popover and a dialog.
        </span>
      </div>
    </div>
  ),
};

const LAYER_USE: Record<string, string> = {
  sticky: "A pinned column, a sticky header, a toolbar popover.",
  overlay: "A menu, a popover, a tooltip, a sheet.",
  dialog: "A dialog and its scrim.",
  alert: "An alert dialog, above a dialog that opened it.",
};

export const Layers: Story = {
  render: () => (
    <div className="flex flex-col bg-card p-6 text-foreground">
      {scale("z").map((step) => (
        <div
          key={step.name}
          className="grid grid-cols-[8rem_4rem_minmax(0,1fr)] gap-4 border-b border-border py-2"
        >
          <span className="font-mono text-ui">z-{step.name}</span>
          <span className="font-mono text-caption text-muted-foreground">{step.value}</span>
          <span className="text-body text-muted-foreground">{LAYER_USE[step.name] ?? ""}</span>
        </div>
      ))}
      <p className="pt-3 text-caption text-muted-foreground">
        z-0 to z-9 order parts inside one component and are not layers.
      </p>
    </div>
  ),
};

/** Three control heights on the 4px grid: 24, 28 and 32px. */
export const ControlSizes: Story = {
  render: () => (
    <div className="flex flex-col gap-4 bg-card p-6 text-foreground">
      <div className="flex items-end gap-3">
        <Button size="xs">xs · 24px</Button>
        <Button size="sm">sm · 28px</Button>
        <Button>default · 32px</Button>
        <Input className="w-56" defaultValue="default · 32px" aria-label="Sample input" />
      </div>
      <p className="max-w-xl text-caption text-muted-foreground">
        A size is a variant of the control, not a height class. Icons are 12px at xs, 14px at sm and
        16px at the default size. Space between things is gap-1, gap-1.5, gap-2 and gap-3; padding
        is p-2, p-3 and p-4.
      </p>
    </div>
  ),
};
