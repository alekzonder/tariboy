import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react-vite";
import { DesktopUpdatesProvider } from "@/components/DesktopUpdates";
import { withWidth } from "@/storybook/decorators";
import { SidebarStateProvider } from "./SidebarStateProvider";
import TerminalsPage from "./TerminalsPage";

// The application shell: agents sidebar, resize handle, server context bar and
// the server surface the route selects. It sits under App.tsx's provider stack
// (daemons, sidebar state, desktop updates) and inside a matched route, because
// the surface comes from useParams(). There is no daemon behind Storybook, so
// the agent list and each surface show their empty or error state.
const withFrame: Decorator = (Story) => (
  <div className="h-155 overflow-hidden rounded-control border border-border bg-background">
    <Story />
  </div>
);

const withProviders: Decorator = (Story) => (
  <SidebarStateProvider>
    <DesktopUpdatesProvider>
      <Story />
    </DesktopUpdatesProvider>
  </SidebarStateProvider>
);

const meta = {
  component: TerminalsPage,
  decorators: [withFrame, withProviders, withWidth(1024)],
  parameters: { layout: "fullscreen", tariboy: { daemon: true } },
} satisfies Meta<typeof TerminalsPage>;
export default meta;

type Story = StoryObj;

export const Workspace: Story = {
  parameters: { tariboy: { daemon: true, route: { path: "/", pattern: "/" } } },
  render: () => <TerminalsPage />,
};

export const ServerTasks: Story = {
  parameters: {
    tariboy: {
      daemon: true,
      // /servers/:hostId/tasks is a legacy URL: TerminalsPage redirects it to /tasks.
      route: { path: "/tasks", pattern: "/tasks" },
    },
  },
  render: () => <TerminalsPage serverView="tasks" scoped />,
};

export const ServerImages: Story = {
  parameters: {
    tariboy: { daemon: true, route: { path: "/library/images", pattern: "/library/images" } },
  },
  render: () => <TerminalsPage serverView="images" scoped />,
};
