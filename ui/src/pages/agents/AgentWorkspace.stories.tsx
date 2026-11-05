import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react-vite";
import { agentRow } from "@/fixtures/agents";
import { withWidth } from "@/storybook/decorators";
import AgentWorkspace from "./AgentWorkspace";

// The per-agent screen. It takes its agent as a prop but reads the active tab
// from useParams(), so each story mounts it in the matched route of App.tsx,
// `/agents/:hostId/:agent/:tab/*`. hostId "" is the same-origin local daemon.
// There is no daemon behind Storybook, so each tab shows its own empty or error
// state under a complete header and tab strip.
const BUILDER = agentRow({
  name: "builder",
  image: "worker:v2",
  state: "running",
  harness: "claude-code",
  group: "core",
  interactive: true,
  cwd: "/home/agent/github/tariboy",
  current_goal_task_key: "TB-142",
  goal_enabled: true,
  on_error: "restart",
  on_timeout: "restart",
});

const withFrame: Decorator = (Story) => (
  <div className="h-[620px] overflow-hidden rounded-md border border-border bg-background p-4">
    <Story />
  </div>
);

const meta = {
  component: AgentWorkspace,
  decorators: [withFrame, withWidth(1024)],
  parameters: { layout: "fullscreen", tariboy: { daemon: true } },
} satisfies Meta<typeof AgentWorkspace>;
export default meta;

type Story = StoryObj;

const onTab = (tab: string): Story => ({
  parameters: {
    tariboy: {
      daemon: true,
      route: {
        path: `/agents/storybook/builder/${tab}`,
        pattern: "/agents/:hostId/:agent/:tab/*",
      },
    },
  },
  // "storybook" is the host that withDaemon registers. The host "" is the
  // same-origin daemon, so it would request the relative /api.
  render: () => <AgentWorkspace hostId="storybook" agent={BUILDER} refresh={() => {}} />,
});

export const Console: Story = onTab("console");

export const Work: Story = onTab("work");

export const Settings: Story = onTab("settings");
