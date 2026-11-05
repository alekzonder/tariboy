import type { Meta, StoryObj } from "@storybook/react-vite";
import { CircleDot, GitBranch, ShieldAlert, Square } from "lucide-react";
import { withSurface } from "@/storybook/decorators";
import { Badge } from "./badge";

const meta = {
  component: Badge,
  decorators: [withSurface("row")],
} satisfies Meta<typeof Badge>;
export default meta;

type Story = StoryObj;

const rowClass = "flex flex-wrap items-center gap-2";

export const AgentState: Story = {
  render: () => (
    <div className={rowClass}>
      <Badge>running</Badge>
      <Badge variant="secondary">stopped</Badge>
      <Badge variant="destructive">out of budget</Badge>
      <Badge variant="outline">starting</Badge>
    </div>
  ),
};

export const Variants: Story = {
  render: () => (
    <div className={rowClass}>
      <Badge variant="default">running</Badge>
      <Badge variant="secondary">Terminal-only</Badge>
      <Badge variant="destructive">over</Badge>
      <Badge variant="outline">idle finish</Badge>
      <Badge variant="ghost">draft</Badge>
      <Badge variant="link">TB-142</Badge>
    </div>
  ),
};

export const WithIcons: Story = {
  render: () => (
    <div className={rowClass}>
      <Badge>
        <CircleDot data-icon="inline-start" />
        running
      </Badge>
      <Badge variant="secondary">
        <Square data-icon="inline-start" />
        stopped
      </Badge>
      <Badge variant="destructive">
        <ShieldAlert data-icon="inline-start" />
        out of budget
      </Badge>
      <Badge variant="outline">
        <GitBranch data-icon="inline-start" />
        main
      </Badge>
    </div>
  ),
};

const agents = [
  { name: "builder", image: "worker:v2", badge: <Badge>running</Badge> },
  { name: "reviewer", image: "worker:v1", badge: <Badge variant="secondary">stopped</Badge> },
  {
    name: "packager",
    image: "bare:latest",
    badge: <Badge variant="destructive">out of budget</Badge>,
  },
  {
    name: "docs-bot",
    image: "bare:latest",
    badge: <Badge variant="secondary">Terminal-only</Badge>,
  },
];

export const InAgentList: Story = {
  render: () => (
    <div className="grid max-w-[380px] gap-1.5 rounded-[10px] border border-border p-2.5 text-[13px]">
      {agents.map((agent) => (
        <div key={agent.name} className="flex items-center gap-2">
          <span className="font-medium">{agent.name}</span>
          <span className="text-xs text-muted-foreground">{agent.image}</span>
          <span className="ml-auto">{agent.badge}</span>
        </div>
      ))}
    </div>
  ),
};
