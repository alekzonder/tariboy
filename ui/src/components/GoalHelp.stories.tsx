import type { Meta, StoryObj } from "@storybook/react-vite";
import { GoalHelp } from "./GoalHelp";

// GoalHelp takes no props: it is the "?" affordance beside the Goal field in the
// agent workspace header, and its explanation lives behind a Dialog the viewer
// opens. Composed here in the header row it actually ships in.
const meta = { component: GoalHelp } satisfies Meta<typeof GoalHelp>;
export default meta;

type Story = StoryObj<typeof meta>;

export const InAgentHeader: Story = {
  render: () => (
    <span className="flex items-center gap-0.5 text-sm">
      <span className="text-muted-foreground">Goal:</span>
      <span className="font-mono text-primary">TB-142</span>
      <GoalHelp />
    </span>
  ),
};

export const NoCurrentGoal: Story = {
  render: () => (
    <span className="flex items-center gap-0.5 text-sm">
      <span className="text-muted-foreground">Goal:</span>
      <span className="text-muted-foreground">No current goal</span>
      <GoalHelp />
    </span>
  ),
};
