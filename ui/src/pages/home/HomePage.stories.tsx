import type { Meta, StoryObj } from "@storybook/react-vite";
import { agentLink, agentLinks } from "@/fixtures/agents";
import { taskActivityTotals } from "@/fixtures/activity";
import { HomeView } from "./HomePage.view";

/**
 * Purpose: the markup of Home, the first screen of a workspace that has
 * agents. Home shows what needs the operator now and whether the agents are
 * in order. The full statement of the goal of the page is in
 * `ui/src/mockups/home/Home.stories.tsx`.
 */
const meta = { component: HomeView } satisfies Meta<typeof HomeView>;
export default meta;

type Story = StoryObj<typeof meta>;

const totals = taskActivityTotals();

const running = [
  agentLink({ key: "local/builder", name: "builder", meta: "12 min", href: "/a/builder" }),
  agentLink({ key: "local/reviewer", name: "reviewer", meta: "3 min", href: "/a/reviewer" }),
];

const recent = agentLinks.many.slice(0, 8).map((row) => ({ ...row, meta: "stopped" }));

const base = {
  needsYouCount: 0,
  needsYou: <p className="px-2 text-body-sm text-muted-foreground">Nothing needs you.</p>,
  running,
  recent,
  totals,
  todayError: null,
  activityHref: "/activity",
  onNewAgent: () => {},
};

export const Default: Story = { args: base };

export const Empty: Story = {
  args: { ...base, running: agentLinks.empty, recent: agentLinks.empty, needsYouCount: 0 },
};

export const LongText: Story = {
  args: { ...base, running: agentLinks.longText, recent: agentLinks.longText },
};

export const TodayError: Story = {
  args: { ...base, totals: null, todayError: "request timed out" },
};
