import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { taskItem } from "@/fixtures/tasks";
import type { NeedsYouRow } from "@/lib/needsYou";
import { NeedsYouContext } from "@/pages/terminals/needsYouContext";
import { NeedsYouList } from "./NeedsYouList";

// The real container, not the mockup copy: the rows come from props and the
// context carries no host, so the story never reaches a daemon. The task
// drawer and the answer card open only on a click.
const withLoadedContext: Decorator = (Story) => (
  <NeedsYouContext.Provider value={{ hosts: [], loaded: true, reload: () => {} }}>
    <div className="bg-background p-4 text-foreground">
      <Story />
    </div>
  </NeedsYouContext.Provider>
);

const meta = {
  component: NeedsYouList,
  decorators: [withLoadedContext],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof NeedsYouList>;
export default meta;

type Story = StoryObj<typeof meta>;

const HOST = "a-build-host-with-a-deliberately-long-label";

// The data of `longText` in mockups/home/Home.stories.tsx.
const longText: NeedsYouRow[] = [
  {
    kind: "task",
    id: "TARIDESIGN-38fp",
    hostId: "",
    hostLabel: HOST,
    at: "2026-11-05T09:42:00Z",
    task: taskItem({
      key: "TARIDESIGN-38fp",
      title: "Approve the release notes for the nightly release candidate of worker:v2",
      status: "wait_customer",
      assignee: "release-candidate-packager-for-the-long-running-nightly",
    }),
  },
];

export const LongText: Story = {
  args: { rows: longText, multiHost: true, onOpenLetter: () => {} },
};
