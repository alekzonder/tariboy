import type { Meta, StoryObj } from "@storybook/react-vite";
import { agentLink, agentLinks } from "@/fixtures/agents";
import { taskActivityTotals } from "@/fixtures/activity";
import { busyNeedsYou } from "@/fixtures/needsYou";
import { HomeView } from "./HomePage.view";
import { NeedsYouListView } from "./NeedsYouList.view";

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

const recent = agentLinks.many.slice(0, 8).map((row, i) => ({ ...row, meta: `${i + 1} h ago` }));

const base = {
  needsYouCount: "0",
  needsYou: <p className="px-2 text-body-sm text-muted-foreground">Nothing needs you.</p>,
  running,
  runningTotal: running.length,
  runningExpanded: false,
  onToggleRunning: () => {},
  recent,
  totals,
  todayError: null,
  activityHref: "/activity",
  onNewAgent: () => {},
};

export const Default: Story = { args: base };

export const Empty: Story = {
  args: {
    ...base,
    running: agentLinks.empty,
    runningTotal: 0,
    recent: agentLinks.empty,
  },
};

export const LongText: Story = {
  args: {
    ...base,
    running: agentLinks.longText,
    runningTotal: agentLinks.longText.length,
    recent: agentLinks.longText,
  },
};

// 100+ items wait and 17 agents run: 7 and 8 rows, with Show all under each.
export const Busy: Story = {
  args: {
    ...base,
    needsYouCount: "100+",
    needsYou: (
      <NeedsYouListView
        rows={busyNeedsYou}
        expanded={new Set()}
        onToggle={() => {}}
        onOpen={() => {}}
        showAll={{ label: "Show all (100+)", href: "/inbox" }}
      />
    ),
    running: agentLinks.many.filter((row) => !row.meta.startsWith("stopped")).slice(0, 8),
    runningTotal: 17,
    recent,
  },
};

export const TodayError: Story = {
  args: { ...base, totals: null, todayError: "request timed out" },
};
