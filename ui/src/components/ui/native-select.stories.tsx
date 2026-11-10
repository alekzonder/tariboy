import type { Meta, StoryObj } from "@storybook/react-vite";
import { withSurface } from "@/storybook/decorators";
import { NativeSelect } from "./native-select";

const meta = {
  component: NativeSelect,
  decorators: [withSurface("card")],
} satisfies Meta<typeof NativeSelect>;
export default meta;

type Story = StoryObj;

export const Default: Story = {
  render: () => (
    <div className="flex flex-col gap-3 bg-card p-4">
      <NativeSelect defaultValue="worker:v1" aria-label="Image">
        <option>worker:v1</option>
        <option>worker:v2</option>
        <option>bare:latest</option>
      </NativeSelect>
      <NativeSelect defaultValue="P2" aria-label="Priority" className="h-6.5 w-20 font-mono">
        <option>P1</option>
        <option>P2</option>
        <option>P3</option>
      </NativeSelect>
      <NativeSelect defaultValue="builder" aria-label="Assignee" disabled>
        <option>builder</option>
      </NativeSelect>
    </div>
  ),
};
