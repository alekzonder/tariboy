import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./badge";
import { Button } from "./button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./tabs";

const meta = { component: Tabs } satisfies Meta<typeof Tabs>;
export default meta;

type Story = StoryObj;

const WORKSPACE_TABS = [
  ["console", "Console"],
  ["autopilot", "Autopilot"],
  ["activity", "Activity"],
  ["tasks", "Tasks"],
  ["configuration", "Configuration"],
  ["advanced", "Advanced"],
] as const;

export const AgentWorkspace: Story = {
  render: () => (
    <Tabs defaultValue="console" className="w-160">
      <TabsList>
        {WORKSPACE_TABS.map(([value, label]) => (
          <TabsTrigger key={value} value={value}>
            {label}
          </TabsTrigger>
        ))}
      </TabsList>
      <TabsContent value="console">
        <div className="rounded-card border border-border px-3 py-2.5 font-mono text-body/5 whitespace-pre-wrap text-foreground">
          {"builder · iteration 42 · worker:v2\n$ tariboy task claim TB-142"}
        </div>
      </TabsContent>
    </Tabs>
  ),
};

export const MessageQueueViews: Story = {
  render: () => (
    <Tabs defaultValue="queue" className="w-105">
      <div className="flex items-center gap-2">
        <TabsList>
          <TabsTrigger value="queue">Queue</TabsTrigger>
          <TabsTrigger value="archive">Archive</TabsTrigger>
          <TabsTrigger value="dlq">DLQ</TabsTrigger>
        </TabsList>
        <Badge variant="secondary">12 pending</Badge>
      </div>
      <TabsContent value="queue">
        <div className="grid gap-1.5 text-body">
          <div>
            <span className="font-mono">reviewer</span>
            <span className="text-muted-foreground"> · review TB-142 before packaging</span>
          </div>
          <div>
            <span className="font-mono">docs-bot</span>
            <span className="text-muted-foreground"> · regenerate the CLI reference</span>
          </div>
        </div>
      </TabsContent>
    </Tabs>
  ),
};

export const WithDisabledTab: Story = {
  render: () => (
    <Tabs defaultValue="configuration" className="w-120">
      <TabsList>
        <TabsTrigger value="configuration">Configuration</TabsTrigger>
        <TabsTrigger value="activity">Activity</TabsTrigger>
        <TabsTrigger value="advanced" disabled>
          Advanced
        </TabsTrigger>
      </TabsList>
      <TabsContent value="configuration">
        <p className="mb-2 text-body text-muted-foreground">
          Host build-01 is unavailable; advanced actions are disabled until it reconnects.
        </p>
        <Button variant="outline" size="sm" disabled>
          Restart builder
        </Button>
      </TabsContent>
    </Tabs>
  ),
};
