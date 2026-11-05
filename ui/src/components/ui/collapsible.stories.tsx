import type { Meta, StoryObj } from "@storybook/react-vite";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "./button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "./collapsible";
import { Input } from "./input";
import { Label } from "./label";
import { Switch } from "./switch";

const meta = { component: Collapsible } satisfies Meta<typeof Collapsible>;
export default meta;

type Story = StoryObj;

export const AdvancedOptionsOpen: Story = {
  render: () => (
    <Collapsible defaultOpen className="w-[420px]">
      <CollapsibleTrigger asChild>
        <Button variant="outline" size="sm" type="button">
          <ChevronDown className="size-4" />
          Advanced
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-3 grid gap-3">
        <div className="flex items-center gap-2">
          <Switch id="interactive" defaultChecked />
          <Label htmlFor="interactive">interactive (tmux TUI)</Label>
        </div>
        <div className="grid gap-1.5">
          <Label>env (K=V)</Label>
          <div className="flex items-center gap-2">
            <Input
              aria-label="env key 0"
              defaultValue="TARIBOY_PROFILE"
              className="h-8 w-[168px]"
            />
            <span className="text-muted-foreground">=</span>
            <Input aria-label="env value 0" defaultValue="team" className="h-8 w-[120px]" />
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  ),
};

export const AdvancedOptionsClosed: Story = {
  render: () => (
    <Collapsible className="w-[420px]">
      <CollapsibleTrigger asChild>
        <Button variant="outline" size="sm" type="button">
          <ChevronRight className="size-4" />
          Advanced
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-3 grid gap-3">
        <div className="flex items-center gap-2">
          <Switch id="interactive-closed" />
          <Label htmlFor="interactive-closed">interactive (tmux TUI)</Label>
        </div>
      </CollapsibleContent>
    </Collapsible>
  ),
};

export const IterationDetail: Story = {
  render: () => (
    <Collapsible defaultOpen className="w-[440px] rounded-[10px] border border-border p-3">
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" type="button" className="pl-1">
          <ChevronDown className="size-4" />
          Iteration 42 · builder · 1m 18s
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2">
        <pre className="m-0 whitespace-pre-wrap font-mono text-xs leading-[18px] text-muted-foreground">
          {
            "claimed TB-142 from queue tasks\nimage worker:v2 · host This daemon (local)\nexit 0 · $0.14"
          }
        </pre>
      </CollapsibleContent>
    </Collapsible>
  ),
};
