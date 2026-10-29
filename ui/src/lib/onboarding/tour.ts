import type { ServerSection } from "@/lib/terminalsHost";

/** The `data-tour` values the tour looks for. Components and steps take them
 *  from here, so a step cannot name an anchor that no element carries. */
export const TOUR_ANCHORS = {
  serversTab: "servers-tab",
  addHost: "add-host",
  addAgent: "add-agent",
  agentRow: "agent-row",
  serverSections: "server-sections",
  sectionTasks: "section-tasks",
  sectionImages: "section-images",
  sectionWorkflows: "section-workflows",
  sectionStores: "section-stores",
  agentTabs: "agent-tabs",
  agentConfiguration: "agent-configuration",
  queueMenu: "queue-menu",
  helpMenu: "help-menu",
} as const;

export type TourAnchor = (typeof TOUR_ANCHORS)[keyof typeof TOUR_ANCHORS];

/** The server-bar links that a step points at. */
export const SECTION_ANCHORS: Partial<Record<ServerSection, TourAnchor>> = {
  tasks: TOUR_ANCHORS.sectionTasks,
  images: TOUR_ANCHORS.sectionImages,
  workflows: TOUR_ANCHORS.sectionWorkflows,
  stores: TOUR_ANCHORS.sectionStores,
};

export interface TourStep {
  id: string;
  title: string;
  body: string;
  /** The element to highlight; without one the card stands in the center. */
  anchor?: TourAnchor;
  /** A section of the current server to open before the highlight. */
  route?: ServerSection;
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
    body: "Tariboy runs AI coding agents on your servers and gives them work through tasks. This tour takes two minutes.",
  },
  {
    id: "servers",
    title: "Servers",
    body: "A server is a computer that runs the Tariboy daemon. Your computer is the first one.",
    anchor: TOUR_ANCHORS.serversTab,
    sidebar: true,
  },
  {
    id: "add-server",
    title: "Add a server",
    body: "Connect another computer over SSH.",
    anchor: TOUR_ANCHORS.addHost,
    sidebar: true,
  },
  {
    id: "server-sections",
    title: "Server sections",
    body: "Everything a server has is here: mail, tasks, images, stores and settings.",
    anchor: TOUR_ANCHORS.serverSections,
    route: "tasks",
  },
  {
    id: "images",
    title: "Agent images",
    body: "An image is an agent template: prompt, skills, files and tools. Every agent made from it starts the same way.",
    anchor: TOUR_ANCHORS.sectionImages,
    route: "images",
  },
  {
    id: "stores",
    title: "Store",
    body: "A store is a shared catalog. Install images and workflows from it, or publish your own.",
    anchor: TOUR_ANCHORS.sectionStores,
    route: "stores",
  },
  {
    id: "agents",
    title: "Agents",
    body: "An agent is one AI assistant, such as Claude or Codex, with its own folder and work loop. Create one from an image with this button.",
    anchor: TOUR_ANCHORS.addAgent,
    sidebar: true,
  },
  {
    id: "agent-workspace",
    title: "Agent workspace",
    body: "Mail, tasks and the live terminal of one agent.",
    anchor: TOUR_ANCHORS.agentTabs,
    agent: true,
  },
  {
    id: "agent-settings",
    title: "Agent settings",
    body: "Configuration sets the harness, model, schedule, environment and budget of the agent.",
    anchor: TOUR_ANCHORS.agentConfiguration,
    agent: true,
  },
  {
    id: "tasks",
    title: "Tasks",
    body: "A task is a piece of work for an agent. Agents ask you questions in the task, and you answer there.",
    anchor: TOUR_ANCHORS.sectionTasks,
    route: "tasks",
  },
  {
    id: "queues",
    title: "Queues",
    body: "A queue groups tasks under a key, such as DEV, and picks the agents that take them. Open Manage queues… from this menu.",
    anchor: TOUR_ANCHORS.queueMenu,
    route: "tasks",
  },
  {
    id: "workflows",
    title: "Task workflows",
    body: "A workflow is the process of a queue: statuses, owners and checks. You approve; agents do the work. Attach one in queue settings.",
    anchor: TOUR_ANCHORS.sectionWorkflows,
    route: "workflows",
  },
  {
    id: "help",
    title: "Help",
    body: "Take this tour again, see what is new, or read the changelog.",
    anchor: TOUR_ANCHORS.helpMenu,
  },
];

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

export type TourMode = "full" | "whats-new";

/** What starts by itself: the full tour on the first run, What's new after an
 *  update that changed a step, otherwise nothing. */
export function tourMode(
  saved: string,
  current: string,
  steps: readonly TourStep[],
): TourMode | "none" {
  if (saved === "") return "full";
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
