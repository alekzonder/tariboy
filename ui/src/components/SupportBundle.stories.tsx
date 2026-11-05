import type { Meta, StoryObj } from "@storybook/react-vite";
import { SupportBundle } from "./SupportBundle";

// SupportBundle takes no props. It reads `isDesktop()` on mount and only then
// lists hosts from the Tauri side; in a browser there is no desktop shell, so
// the component renders its full authored card with every control disabled and
// the reason stated at the bottom. That is the exact state the web build ships.

const meta = {
  component: SupportBundle,
  decorators: [
    (Story) => (
      <div className="max-w-[660px]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SupportBundle>;
export default meta;

type Story = StoryObj<typeof meta>;

export const SettingsPanel: Story = {};
