import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { busyNeedsYou, needsYouRow } from "@/fixtures/needsYou";
import { NeedsYouContext } from "@/pages/terminals/needsYouContext";
import { NeedsYouList } from "./NeedsYouList";

/**
 * Purpose: Needs you is the first section of Home. It lists the tasks and the
 * letters that wait for an answer from the operator, so the operator can
 * pick the next one and answer it in place. The goal of the page is in
 * `ui/src/mockups/home/Home.stories.tsx`.
 */

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

export const LongText: Story = {
  args: {
    rows: [
      needsYouRow({
        id: "task-TARIDESIGN-38fp",
        ref: "TARIDESIGN-38fp",
        label: "TARIDESIGN-38fp",
        title: "Approve the release notes for the nightly release candidate of worker:v2",
        meta: "waiting for you · release-candidate-packager-for-the-long-running-nightly · a-build-host-with-a-deliberately-long-label · 12 min ago",
      }),
    ],
    showAll: null,
    onOpenLetter: () => {},
  },
};

// 100+ items wait: the seven most urgent, with repeated letters merged.
export const Busy: Story = {
  args: {
    rows: busyNeedsYou,
    showAll: { label: "Show all (100+)", href: "/inbox" },
    onOpenLetter: () => {},
  },
};
