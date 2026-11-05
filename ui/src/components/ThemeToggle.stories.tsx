import type { Meta, StoryObj } from "@storybook/react-vite";
import { ThemeToggle } from "./ThemeToggle";

const meta = { component: ThemeToggle } satisfies Meta<typeof ThemeToggle>;
export default meta;

type Story = StoryObj<typeof meta>;

// The outline icon button on the right of the application titlebar.
export const Default: Story = {};

export const InTitlebar: Story = {
  render: (args) => (
    <div className="flex h-12 items-center justify-end gap-2 border-b border-border px-4">
      <span className="text-sm text-muted-foreground">Settings</span>
      <ThemeToggle {...args} />
    </div>
  ),
};
