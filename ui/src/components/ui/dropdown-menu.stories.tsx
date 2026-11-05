import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Copy, Download, MoonIcon, Pencil, SunIcon, Trash2, Upload } from "lucide-react";
import { Button } from "./button";
import {
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./dropdown-menu";

const meta = { component: DropdownMenu } satisfies Meta<typeof DropdownMenu>;
export default meta;

type Story = StoryObj;

// The sidebar host row the "manage host" menu hangs off.
const HostRow = ({ children }: { children?: ReactNode }) => (
  <div className="flex w-[280px] items-center gap-2 rounded-md border border-border px-2 py-1">
    <span className="text-[13px] font-medium">build-01</span>
    <span className="text-xs text-muted-foreground">2 agents</span>
    <span className="ml-auto flex gap-1">{children}</span>
  </div>
);

export const HostActions: Story = {
  render: () => (
    <HostRow>
      <DropdownMenu defaultOpen>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" aria-label="manage build-01">
            ⋯
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem>Edit host</DropdownMenuItem>
          <DropdownMenuItem>Remove host</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </HostRow>
  ),
};

export const CheckedItems: Story = {
  render: () => (
    <div className="flex w-[280px] items-center gap-2">
      <span className="text-[13px] text-muted-foreground">Appearance</span>
      <span className="ml-auto">
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon-sm" aria-label="Toggle theme" title="Theme">
              <SunIcon />
              <MoonIcon className="hidden" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuCheckItem checked>System</DropdownMenuCheckItem>
            <DropdownMenuCheckItem>Light</DropdownMenuCheckItem>
            <DropdownMenuCheckItem>Dark</DropdownMenuCheckItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    </div>
  ),
};

export const LabelledSections: Story = {
  render: () => (
    <div className="w-[280px]">
      <DropdownMenu defaultOpen>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm">
            Image actions
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>worker:v2</DropdownMenuLabel>
          <DropdownMenuItem>
            <Copy />
            Clone into a new agent
          </DropdownMenuItem>
          <DropdownMenuItem>
            <Pencil />
            Edit manifest
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuLabel>Transfer</DropdownMenuLabel>
          <DropdownMenuItem>
            <Download />
            Export archive
          </DropdownMenuItem>
          <DropdownMenuItem>
            <Upload />
            Send to build-01
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem>
            <Trash2 />
            Remove image
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  ),
};

export const DisabledItems: Story = {
  render: () => (
    <div className="w-[280px]">
      <DropdownMenu defaultOpen>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" aria-label="manage build-02">
            ⋯
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuLabel>build-02 is offline</DropdownMenuLabel>
          <DropdownMenuItem>Edit host</DropdownMenuItem>
          <DropdownMenuItem disabled>New agent</DropdownMenuItem>
          <DropdownMenuItem disabled>Send files</DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem>Remove host</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  ),
};
