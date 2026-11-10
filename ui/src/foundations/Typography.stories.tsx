import type { Meta, StoryObj } from "@storybook/react-vite";
import { scale } from "@/storybook/tokens";

// The text scale of index.css. A size sets its own line height, so a class is
// one decision: `text-ui`, never a size and a leading.
const meta = {
  title: "Foundations/Typography",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const USE: Record<string, string> = {
  caption: "Meta under a title, section labels, chips, counters, timestamps.",
  ui: "Dense controls, table cells, secondary lines, sm and xs buttons.",
  body: "The default. Body text, inputs, default buttons, menu items.",
  title: "The title of a card, a dialog, a page section, the agent in its header.",
  heading: "The heading of a page.",
  display: "One number that a page is about: a total, a first-run headline.",
};

export const Scale: Story = {
  render: () => (
    <div className="flex flex-col bg-card p-6 text-foreground">
      {scale("text").map((step) => (
        <div
          key={step.name}
          className="grid grid-cols-[8rem_7rem_minmax(0,1fr)] items-baseline gap-4 border-b border-border py-3"
        >
          <span className="font-mono text-ui">text-{step.name}</span>
          <span className="font-mono text-caption text-muted-foreground">
            {step.value} / {step.lineHeight}
          </span>
          <span className="flex min-w-0 flex-col gap-1">
            <span
              style={{
                fontSize: `var(--text-${step.name})`,
                lineHeight: `var(--text-${step.name}--line-height)`,
              }}
            >
              builder finished TB-142 and waits for you
            </span>
            <span className="text-caption text-muted-foreground">
              {USE[step.name] ?? "No use written yet: add it to Typography.stories.tsx."}
            </span>
          </span>
        </div>
      ))}
    </div>
  ),
};

/** Two families and three weights. Identifiers are mono with tabular figures. */
export const FamiliesAndWeights: Story = {
  render: () => (
    <div className="flex flex-col gap-6 bg-card p-6 text-body text-foreground">
      <div className="flex flex-col gap-1">
        <span className="font-mono text-caption text-muted-foreground">font-sans · Geist</span>
        <span className="font-normal">font-normal — body text and quiet actions</span>
        <span className="font-medium">font-medium — labels, buttons, a selected item</span>
        <span className="font-semibold">font-semibold — titles and headings</span>
      </div>
      <div className="flex flex-col gap-1">
        <span className="font-mono text-caption text-muted-foreground">
          font-mono tabular-nums · identifiers line up
        </span>
        <span className="font-mono text-caption tabular-nums">
          TB-142 · 00:12:41 · sha256:9ac71e3f52
        </span>
        <span className="font-mono text-caption tabular-nums">
          TB-98 · 01:03:07 · sha256:11f0c2ab90
        </span>
      </div>
      <div className="flex flex-col gap-1">
        <span className="font-mono text-caption text-muted-foreground">
          section label · text-caption font-medium tracking-wide
        </span>
        <span className="text-caption font-medium tracking-wide text-muted-foreground">
          Dependencies
        </span>
      </div>
    </div>
  ),
};
