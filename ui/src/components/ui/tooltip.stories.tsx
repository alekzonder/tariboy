import type { Meta, StoryObj } from "@storybook/react-vite";
import { CircleHelp, Trash2 } from "lucide-react";
import { Button } from "./button";
import { Tooltip, TooltipContent, TooltipTrigger } from "./tooltip";

const meta = { component: Tooltip } satisfies Meta<typeof Tooltip>;
export default meta;

type Story = StoryObj;

export const GoalHelp: Story = {
  render: () => (
    <div className="flex items-center gap-1 pt-14 text-body">
      <span className="text-muted-foreground">Goal:</span>
      <span className="font-mono text-primary">TB-142</span>
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            aria-label="What is the current goal?"
            className="size-6"
          >
            <CircleHelp className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top" sideOffset={6}>
          The task the agent works toward each iteration until it is closed.
        </TooltipContent>
      </Tooltip>
    </div>
  ),
};

export const DestructiveAction: Story = {
  render: () => (
    <div className="pb-14">
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <Button variant="outline" size="sm" aria-label="Remove image">
            <Trash2 className="size-4" />
            Remove
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom" sideOffset={6}>
          Remove worker:v2 from This daemon (local)
        </TooltipContent>
      </Tooltip>
    </div>
  ),
};

export const TruncatedDigest: Story = {
  render: () => (
    <div className="flex w-105 justify-start">
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <span className="cursor-default border-b border-dotted border-border font-mono text-body">
            sha256:9ac71e3f52…
          </span>
        </TooltipTrigger>
        <TooltipContent side="right" sideOffset={8} className="break-all">
          sha256:9ac71e3f5286b1d0a4ef77c3b9042de1c8a5f31b
        </TooltipContent>
      </Tooltip>
    </div>
  ),
};

export const DisabledControl: Story = {
  render: () => (
    <div className="pt-14">
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <span className="inline-block">
            <Button variant="outline" size="sm" disabled className="pointer-events-none">
              Restart builder
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" sideOffset={6}>
          build-01 is unavailable — actions resume when it reconnects.
        </TooltipContent>
      </Tooltip>
    </div>
  ),
};
