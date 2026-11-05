import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { AgentControls } from "./AgentControls";

// AgentControls is the header's action cluster for one agent: the primary
// action of its derived state, the Stop/Start toggle, and the `⋯` menu (Run
// once…, Force-stop, VS Code, Clone, Pin, Settings, Delete). `refresh` re-polls
// the agent snapshot after an action resolves.
const meta = {
  component: AgentControls,
  parameters: { tariboy: { daemon: true } },
} satisfies Meta<typeof AgentControls>;
export default meta;

type Story = StoryObj;

const paths = (name: string) => ({
  workPath: `/agents/local/${name}/work`,
  settingsPath: `/agents/local/${name}/settings`,
  diagnosticsPath: `/agents/local/${name}/advanced`,
});

const Heading = ({ name, detail }: { name: string; detail: string }) => (
  <div className="flex items-baseline gap-2">
    <span className="text-[15px] font-semibold">{name}</span>
    <span className="font-mono text-xs text-muted-foreground">{detail}</span>
  </div>
);

const Column = ({ children }: { children: ReactNode }) => (
  <div className="flex w-[720px] flex-col gap-2">{children}</div>
);

export const AgentWorkspace: Story = {
  render: () => (
    <Column>
      <Heading name="builder" detail="worker:v2 · claude · running" />
      <AgentControls
        name="builder"
        alive
        image="worker:v2"
        primary="give"
        {...paths("builder")}
        refresh={() => {}}
      />
    </Column>
  ),
};

export const StoppedAgent: Story = {
  render: () => (
    <Column>
      <Heading name="docs-bot" detail="bare:latest · codex · stopped" />
      <AgentControls
        name="docs-bot"
        alive={false}
        image="bare:latest"
        primary="start"
        {...paths("docs-bot")}
        refresh={() => {}}
      />
    </Column>
  ),
};

// An agent that waits for the user: Answer is the primary action.
export const NeedsYou: Story = {
  render: () => (
    <div className="w-[380px]">
      <AgentControls
        name="packager"
        alive
        primary="answer"
        {...paths("packager")}
        refresh={() => {}}
      />
    </div>
  ),
};
