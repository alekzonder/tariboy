import type { Meta, StoryObj } from "@storybook/react-vite";
import { Users } from "lucide-react";
import { AgentRow } from "@/components/AgentRow.view";
import { taskActivityTotals } from "@/fixtures/activity";
import { agentLink, agentLinks } from "@/fixtures/agents";
import { NOW, minutesAgo } from "@/fixtures/time";
import type { AgentState } from "@/lib/agentState";
import { HomeView, type HomeViewProps } from "@/pages/home/HomePage.view";
import { withWidth } from "@/storybook/decorators";
import { NeedsYouListView, type NeedsYouViewRow } from "./NeedsYouList.view";

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
      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-[var(--panel-radius)] bg-card shadow-[var(--lift)]">
        <div className="flex min-h-0 flex-1 flex-col p-3">
          <HomeView
            {...data.view}
            needsYouCount={data.needs.length}
            needsYou={<NeedsYouListView rows={data.needs} />}
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
    {
      id: "task/TB-148",
      kind: "task",
      label: "TB-148",
      title: "Approve the release notes for worker:v2",
      meta: "wait_customer · planner · 09:42",
    },
    {
      id: "letter/m-1",
      kind: "letter",
      label: "docs-bot",
      title: "Which image builds the docs: worker:v1 or bare:latest?",
      meta: "letter · 09:15",
    },
  ],
  view: {
    running: [
      agentLink({ key: "local/builder", name: "builder", meta: "12 min", href: "/a/builder" }),
      agentLink({ key: "local/reviewer", name: "reviewer", meta: "3 min", href: "/a/reviewer" }),
    ],
    recent: ["builder", "reviewer", "planner", "docs-bot", "packager"].map((name) =>
      agentLink({
        key: `local/${name}`,
        name,
        meta: name === "builder" || name === "reviewer" ? "running" : "stopped",
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

// 50 agents, 17 of them running, and six things that wait for the operator.
const busy: PageData = {
  sidebar: agentLinks.many.map((link, i) => ({
    name: link.name,
    agentState: isRunning(link) ? working(i + 1) : off,
  })),
  needs: Array.from({ length: 6 }, (_, i) => ({
    id: `task/TB-${160 + i}`,
    kind: "task" as const,
    label: `TB-${160 + i}`,
    title: `Review the build output of packager-${4 * i + 3}`,
    meta: `wait_customer · packager-${4 * i + 3} · 09:${String(10 + i * 7).padStart(2, "0")}`,
  })),
  view: {
    ...base.view,
    running: agentLinks.many.filter(isRunning),
    recent: agentLinks.many.filter((link) => !isRunning(link)).slice(0, 8),
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
    {
      id: "task/TARIDESIGN-38fp",
      kind: "task",
      label: "TARIDESIGN-38fp",
      title: "Approve the release notes for the nightly release candidate of worker:v2",
      meta: "wait_customer · release-candidate-packager-for-the-long-running-nightly · 09:42",
    },
    base.needs[1],
  ],
  view: {
    ...base.view,
    running: [...agentLinks.longText, ...base.view.running],
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
