import type { Meta, StoryObj } from "@storybook/react-vite";
import { IterationAuditLog } from "./IterationAuditLog";

// IterationAuditLog streams one iteration's audit events. With no daemon the
// fetches never resolve, so what renders is the component's frame — the mono
// iteration chip, the audit export actions, and the bottom-pinned scroll pane
// in its "No events." state. The component is `h-full` and is always mounted
// into a flex column that has already claimed the page's leftover height, so
// the story supplies one.

const meta = {
  component: IterationAuditLog,
  parameters: { tariboy: { daemon: true } },
  decorators: [
    (Story) => (
      <div className="flex h-65 max-w-180 flex-col">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof IterationAuditLog>;
export default meta;

type Story = StoryObj<typeof meta>;

// The Overview / Audit-log pane while the current iteration is running.
export const RunningIteration: Story = {
  args: {
    name: "builder",
    iterationId: "builder-20260911182241-48",
    iterationStatus: "running",
  },
};

// A fresh agent: no iteration selected, so the chip says so and the export
// actions are withheld (there is nothing to export yet).
export const NoIterations: Story = {
  args: { name: "docs-bot", iterationId: "", iterationStatus: "" },
};
