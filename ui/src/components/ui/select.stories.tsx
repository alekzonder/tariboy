import type { Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "./label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";

const meta = { component: Select } satisfies Meta<typeof Select>;
export default meta;

type Story = StoryObj;

const IMAGES = ["worker:v1", "worker:v2", "reviewer:v3", "bare:latest"];

function ImageSelect({ id, open }: { id: string; open?: boolean }) {
  return (
    <div className="grid w-72 gap-1">
      <Label htmlFor={id}>Agent image</Label>
      <Select defaultValue="worker:v1" open={open}>
        <SelectTrigger id={id} aria-label="Agent image">
          <SelectValue placeholder="Select image" />
        </SelectTrigger>
        <SelectContent>
          {IMAGES.map((ref) => (
            <SelectItem key={ref} value={ref}>
              {ref}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

export const Closed: Story = { render: () => <ImageSelect id="agent-image" /> };

export const Open: Story = { render: () => <ImageSelect id="agent-image-open" open /> };

export const Placeholder: Story = {
  render: () => (
    <div className="grid w-72 gap-1">
      <Label htmlFor="harness">Harness</Label>
      <Select>
        <SelectTrigger id="harness" aria-label="Harness">
          <SelectValue placeholder="image default" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="claude">claude</SelectItem>
          <SelectItem value="codex">codex</SelectItem>
        </SelectContent>
      </Select>
    </div>
  ),
};

export const Disabled: Story = {
  render: () => (
    <div className="grid w-72 gap-1">
      <Label htmlFor="host">Host</Label>
      <Select defaultValue="local" disabled>
        <SelectTrigger id="host" aria-label="Host">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="local">This daemon (local)</SelectItem>
        </SelectContent>
      </Select>
    </div>
  ),
};
