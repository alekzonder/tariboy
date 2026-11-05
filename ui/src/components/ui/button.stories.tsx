import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileUp, Keyboard, Plus, RefreshCw, Settings, X } from "lucide-react";
import { withSurface } from "@/storybook/decorators";
import { Button } from "./button";

const meta = {
  component: Button,
  decorators: [withSurface("row")],
} satisfies Meta<typeof Button>;
export default meta;

type Story = StoryObj;

const rowClass = "flex flex-wrap items-center gap-2";

export const Variants: Story = {
  render: () => (
    <div className={rowClass}>
      <Button>New agent</Button>
      <Button variant="outline">Kill session</Button>
      <Button variant="secondary">Stop</Button>
      <Button variant="ghost">Cancel</Button>
      <Button variant="destructive">Delete agent</Button>
      <Button variant="link">Open in VS Code</Button>
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className={rowClass}>
      <Button size="xs">Update</Button>
      <Button size="sm">Send files</Button>
      <Button size="default">Create agent</Button>
      <Button size="lg">Save working directory</Button>
    </div>
  ),
};

export const WithIcons: Story = {
  render: () => (
    <div className={rowClass}>
      <Button>
        <Plus />
        New task
      </Button>
      <Button variant="outline">
        <FileUp />
        Send files
      </Button>
      <Button variant="outline">
        <Keyboard />
        Compose
      </Button>
      <Button variant="secondary">
        <RefreshCw />
        Refresh
      </Button>
    </div>
  ),
};

export const IconOnly: Story = {
  render: () => (
    <div className={rowClass}>
      <Button size="icon-xs" variant="ghost" aria-label="Close">
        <X />
      </Button>
      <Button size="icon-sm" variant="outline" aria-label="Settings">
        <Settings />
      </Button>
      <Button size="icon" variant="outline" aria-label="Refresh">
        <RefreshCw />
      </Button>
      <Button size="icon-lg" aria-label="Add">
        <Plus />
      </Button>
    </div>
  ),
};

export const Disabled: Story = {
  render: () => (
    <div className={rowClass}>
      <Button disabled>Creating…</Button>
      <Button variant="outline" disabled>
        Export
      </Button>
      <Button variant="destructive" disabled>
        Remove image
      </Button>
    </div>
  ),
};
