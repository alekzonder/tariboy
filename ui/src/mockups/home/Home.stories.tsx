import type { Meta, StoryObj } from "@storybook/react-vite";
import { Users } from "lucide-react";
import { AgentRow } from "@/components/AgentRow.view";
import { taskActivityTotals } from "@/fixtures/activity";
import { agentLink, agentLinks } from "@/fixtures/agents";
import { NOW, minutesAgo } from "@/fixtures/time";
import type { AgentState } from "@/lib/agentState";
import { needsYouRow, busyNeedsYou } from "@/fixtures/needsYou";
import { HomeView, type HomeViewProps } from "@/pages/home/HomePage.view";
import { NeedsYouListView } from "@/pages/home/NeedsYouList.view";
import type { NeedsYouViewRow } from "@/pages/home/needsYouModel";
import { withWidth } from "@/storybook/decorators";

/**
 * Purpose: Home is the first screen of a workspace that has agents. It shows
 * what needs the operator now and whether the agents are in order.
 *
 * User: the operator who runs AI agents on one or more hosts.
 *
 * Task: open the application, find the task or the letter that waits for an
 * answer, see which agents run now and how the day goes, then go to the
 * right place.
 *
 * Success: the operator understands in 5 seconds where an action is needed.
 * At 1280 px all four sections (Needs you, Running now, Today, Recent) are
 * visible on the first screen, and Needs you shows the most urgent items
 * first, not every item.
 */

// The Home page as the operator sees it: the sidebar on the --background
// chrome and the content island with HomeView. No redesign: it repeats the
// current page. Below 768 px the sidebar is a drawer (DRAWER_QUERY), so the
// compact widths show the island alone.

const meta = {
  title: "Mockups/Home",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const DRAWER_BELOW = 768;
const SIDEBAR_WIDTH = 268;
const now = new Date(NOW);

interface SidebarAgent {
  name: string;
  agentState: AgentState;
}

interface PageData {
  sidebar: SidebarAgent[];
  needs: NeedsYouViewRow[];
  view: Omit<HomeViewProps, "needsYou" | "needsYouCount">;
  /** "Show all (N)" under the rows, when more items wait than are shown. */
  showAll?: string;
}

function Sidebar({ agents }: { agents: SidebarAgent[] }) {
  return (
    <aside
      aria-label="Navigation"
      style={{ width: SIDEBAR_WIDTH }}
      className="flex min-h-0 shrink-0 flex-col px-2"
    >
      <div className="flex h-7.5 items-center gap-2 px-2 text-body text-muted-foreground">
        <Users className="size-4 shrink-0" aria-hidden="true" />
        Agents
      </div>
      <div className="min-h-0 shrink overflow-y-auto" aria-label="Agents" role="group">
        {agents.map((agent) => (
          <AgentRow key={agent.name} name={agent.name} agentState={agent.agentState} now={now} />
        ))}
      </div>
    </aside>
  );
}

function HomeMockup({ compact, data }: { compact: boolean; data: PageData }) {
  return (
    <div className="flex h-screen px-2 pt-2 pb-2">
      {!compact && <Sidebar agents={data.sidebar} />}
      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-panel bg-card shadow-lift">
        <div className="flex min-h-0 flex-1 flex-col p-3">
          <HomeView
            {...data.view}
            needsYouCount={data.showAll ? "100+" : String(data.needs.length)}
            needsYou={
              <NeedsYouListView
                rows={data.needs}
                expanded={new Set()}
                onToggle={() => {}}
                onOpen={() => {}}
                showAll={data.showAll ? { label: data.showAll, href: "/inbox" } : null}
              />
            }
          />
        </div>
      </main>
    </div>
  );
}

const working = (minutes: number): AgentState => ({ kind: "working", since: minutesAgo(minutes) });
const needsOne: AgentState = { kind: "needs", needs: { count: 1 } };
const off: AgentState = { kind: "off" };

const base: PageData = {
  sidebar: [
    { name: "builder", agentState: working(12) },
    { name: "reviewer", agentState: working(3) },
    { name: "planner", agentState: needsOne },
    { name: "docs-bot", agentState: needsOne },
    { name: "packager", agentState: off },
  ],
  needs: [
    needsYouRow(),
    needsYouRow({
      id: "letter-m-1",
      kind: "letter",
      ref: "m-1",
      label: "docs-bot",
      title: "Which image builds the docs: worker:v1 or bare:latest?",
      meta: "Question on TB-150 · 45 min ago",
    }),
  ],
  view: {
    running: [
      agentLink({ key: "local/builder", name: "builder", meta: "12 min", href: "/a/builder" }),
      agentLink({ key: "local/reviewer", name: "reviewer", meta: "3 min", href: "/a/reviewer" }),
    ],
    runningTotal: 2,
    runningExpanded: false,
    onToggleRunning: () => {},
    recent: ["builder", "reviewer", "planner", "docs-bot", "packager"].map((name, i) =>
      agentLink({
        key: `local/${name}`,
        name,
        meta: `${i + 1} min ago`,
        href: `/a/${name}`,
      }),
    ),
    totals: taskActivityTotals(),
    todayError: null,
    activityHref: "/activity",
    onNewAgent: () => {},
  },
};

const isRunning = (link: { meta: string }) => !link.meta.startsWith("stopped");

// 50 agents, 17 of them running (8 shown), and 100+ things that wait for the
// operator (the 7 most urgent shown, repeated letters merged).
const busy: PageData = {
  sidebar: agentLinks.many.map((link, i) => ({
    name: link.name,
    agentState: isRunning(link) ? working(i + 1) : off,
  })),
  needs: busyNeedsYou,
  showAll: "Show all (100+)",
  view: {
    ...base.view,
    running: agentLinks.many.filter(isRunning).slice(0, 8),
    runningTotal: agentLinks.many.filter(isRunning).length,
    recent: agentLinks.many
      .filter((link) => !isRunning(link))
      .slice(0, 8)
      .map((link, i) => ({ ...link, meta: `${i + 1} h ago · build-01` })),
    totals: taskActivityTotals({
      tasks: 212,
      completed: 187,
      transitions: 64,
      comments: 41,
      iterations: 1_340,
      requests: 9_820,
      tokens: 48_600_000,
      cost_usd: 1_204.5,
    }),
  },
};

// A long agent name with a long meta in Running now and Recent, and a long
// task key, title and meta in Needs you: each row still names its agent or
// task (TARI-cset).
const longText: PageData = {
  ...base,
  needs: [
    needsYouRow({
      id: "task-TARIDESIGN-38fp",
      ref: "TARIDESIGN-38fp",
      label: "TARIDESIGN-38fp",
      title: "Approve the release notes for the nightly release candidate of worker:v2",
      meta: "waiting for you · release-candidate-packager-for-the-long-running-nightly · 12 min ago",
    }),
    base.needs[1],
  ],
  view: {
    ...base.view,
    running: [...agentLinks.longText, ...base.view.running],
    runningTotal: agentLinks.longText.length + base.view.running.length,
    recent: [...agentLinks.longText, ...base.view.recent],
  },
};

const atWidth = (width: number, data: PageData = base): Story => ({
  decorators: [withWidth(width)],
  render: () => <HomeMockup compact={width < DRAWER_BELOW} data={data} />,
});

export const Width360: Story = atWidth(360);

export const Width390: Story = atWidth(390);

export const Width820: Story = atWidth(820);

export const Width1024: Story = atWidth(1024);

export const Width1180: Story = atWidth(1180);

export const Busy: Story = atWidth(1024, busy);

export const LongText360: Story = atWidth(360, longText);

export const LongText1024: Story = atWidth(1024, longText);
