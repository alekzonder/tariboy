import type { NavItem } from "@/lib/navRoutes";

/** The `data-tour` values the tour looks for. Components and steps take them
 *  from here, so a step cannot name an anchor that no element carries. */
export const TOUR_ANCHORS = {
  /** The sidebar footer's hosts line. */
  hostsLine: "hosts-line",
  /** The sidebar footer's menu, which holds Add host. */
  addHost: "add-host",
  /** The sidebar's "New agent" text button. */
  addAgent: "add-agent",
  /** The Home "New agent" and first-run "Start agent" buttons. */
  newAgent: "new-agent",
  agentRow: "agent-row",
  /** The sidebar's Tasks item. */
  sectionTasks: "section-tasks",
  /** The sidebar's Library item. */
  sectionImages: "section-images",
  /** The Library's Workflows and Sources tabs. */
  sectionWorkflows: "section-workflows",
  sectionStores: "section-stores",
  agentTabs: "agent-tabs",
  agentConfiguration: "agent-configuration",
  queueMenu: "queue-menu",
  helpMenu: "help-menu",
} as const;

/** The first agent row of the sidebar, or null when it shows no agent. */
export function firstAgentRow(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    `[data-tour="${TOUR_ANCHORS.agentRow}"]:not(:disabled)`,
  );
}

export type TourAnchor = (typeof TOUR_ANCHORS)[keyof typeof TOUR_ANCHORS];

/** The release whose navigation moved the anchors of the changed steps. */
const LEAN_NAVIGATION = "0.101.0";

export interface TourStep {
  id: string;
  title: string;
  body: string;
  /** The element to highlight; without one the card stands in the center. */
  anchor?: TourAnchor;
  /** A page to open, on the current host, before the highlight. */
  route?: NavItem;
  /** Opens the home screen (`/`) first. */
  home?: boolean;
  /** Opens the first agent first; the step is skipped when there is none. */
  agent?: boolean;
  /** The anchor is in the sidebar, so a hidden sidebar is shown. */
  sidebar?: boolean;
  /** The release that added (or, with `updated`, changed) this step. A step
   *  with `since` newer than the version a user saw is shown in What's new. */
  since?: string;
  updated?: boolean;
}

export const TOUR_STEPS: readonly TourStep[] = [
  {
    id: "welcome",
    title: "Welcome to Tariboy",
    body: "Tariboy runs AI coding agents on your hosts and gives them work through tasks. This tour takes two minutes.",
  },
  {
    id: "agents",
    title: "Agents",
    body: "An agent is one AI assistant, such as Claude or Codex, with its own folder and work loop. Start one with this button.",
    anchor: TOUR_ANCHORS.newAgent,
    home: true,
  },
  {
    id: "agent-workspace",
    title: "Agent workspace",
    body: "Work, Terminal, Activity and Settings of one agent: its tasks, its live screen, what it did and what it cost, and how it runs.",
    anchor: TOUR_ANCHORS.agentTabs,
    agent: true,
  },
  {
    id: "agent-settings",
    title: "Agent settings",
    body: "Settings sets the coding tool, model, working folder, Autopilot and spending limit. Everything else is under Advanced.",
    anchor: TOUR_ANCHORS.agentConfiguration,
    agent: true,
  },
  {
    id: "tasks",
    title: "Tasks",
    body: "A task is a piece of work for an agent. Agents ask you questions in the task, and you answer there. The number next to Tasks counts the tasks that wait for you.",
    anchor: TOUR_ANCHORS.sectionTasks,
    route: "tasks",
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "images",
    title: "Agent images",
    body: "An image is an agent template: prompt, skills, files and tools. Every agent made from it starts the same way. Images are in the Library.",
    anchor: TOUR_ANCHORS.sectionImages,
    route: "library-images",
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "stores",
    title: "Sources",
    body: "A source is a shared catalog, such as a Store. Install images and workflows from it, or publish your own.",
    anchor: TOUR_ANCHORS.sectionStores,
    route: "library-sources",
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "servers",
    title: "Hosts",
    body: "A host is a computer that runs the Tariboy daemon. Your computer is the first one. This line says whether your hosts are ready and opens their settings.",
    anchor: TOUR_ANCHORS.hostsLine,
    sidebar: true,
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "add-server",
    title: "Add a host",
    body: "Connect another computer over SSH with Add host on Settings → Hosts. Host workspaces are there too.",
    anchor: TOUR_ANCHORS.addHost,
    route: "settings",
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "queues",
    title: "Queues",
    body: "A queue groups tasks under a key, such as DEV, and picks the agents that take them. Open Manage queues… from the Filters menu.",
    anchor: TOUR_ANCHORS.queueMenu,
    route: "tasks",
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "workflows",
    title: "Task workflows",
    body: "A workflow is the process of a queue: statuses, owners and checks. You approve; agents do the work. Workflows are in the Library; attach one in queue settings.",
    anchor: TOUR_ANCHORS.sectionWorkflows,
    route: "library-workflows",
    since: LEAN_NAVIGATION,
    updated: true,
  },
  {
    id: "help",
    title: "Help",
    body: "Take this tour again, see what is new, or read the changelog.",
    anchor: TOUR_ANCHORS.helpMenu,
  },
];

/** The steps a tour shows: without an agent the agent steps go, so the counter
 *  counts only what the user sees. */
export function tourSteps(steps: readonly TourStep[], hasAgents: boolean): TourStep[] {
  return steps.filter((step) => hasAgents || !step.agent);
}

/** Compares `MAJOR.MINOR.PATCH` numerically.
 *  shortcut: a pre-release suffix is ignored, compare it if releases ever use one. */
export function compareVersions(a: string, b: string): number {
  const parts = (v: string) => v.split("-")[0].split(".").map(Number);
  const [x, y] = [parts(a), parts(b)];
  for (let i = 0; i < 3; i++) {
    const diff = (x[i] || 0) - (y[i] || 0);
    if (diff !== 0) return diff;
  }
  return 0;
}

/** The steps added or changed after `saved`, up to the running version. */
export function whatsNewSteps(
  steps: readonly TourStep[],
  saved: string,
  current: string,
): TourStep[] {
  return steps.filter(
    (step) =>
      step.since !== undefined &&
      compareVersions(step.since, saved) > 0 &&
      compareVersions(step.since, current) <= 0,
  );
}

/** What starts by itself: What's new after an update that changed a step,
 *  otherwise nothing. The full tour never starts by itself, not even on the
 *  first run; it opens from Help. */
export function tourMode(
  saved: string,
  current: string,
  steps: readonly TourStep[],
): "whats-new" | "none" {
  if (saved === "") return "none";
  return whatsNewSteps(steps, saved, current).length > 0 ? "whats-new" : "none";
}

export interface Rect {
  top: number;
  left: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
}

interface Size {
  width: number;
  height: number;
}

const GAP = 12;

/** Where the card stands: right of the element, else below, left or above it,
 *  wherever it fits whole; in the center when there is no element or no room. */
export function cardPosition(
  target: Rect | null,
  viewport: Size,
  card: Size,
): { top: number; left: number } {
  const clamp = (value: number, room: number) => Math.max(GAP, Math.min(value, room - GAP));
  const top = clamp(target?.top ?? 0, viewport.height - card.height);
  const left = clamp(target?.left ?? 0, viewport.width - card.width);
  if (target) {
    if (target.right + 2 * GAP + card.width <= viewport.width) {
      return { top, left: target.right + GAP };
    }
    if (target.bottom + 2 * GAP + card.height <= viewport.height) {
      return { top: target.bottom + GAP, left };
    }
    if (target.left - 2 * GAP - card.width >= 0) {
      return { top, left: target.left - GAP - card.width };
    }
    if (target.top - 2 * GAP - card.height >= 0) {
      return { top: target.top - GAP - card.height, left };
    }
  }
  return {
    top: Math.round((viewport.height - card.height) / 2),
    left: Math.round((viewport.width - card.width) / 2),
  };
}
