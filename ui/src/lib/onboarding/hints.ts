/** The agent tabs that carry a first-visit hint. */
export type HintTab = "work" | "console" | "settings";

/** One line per tab: what to do there first. */
export const HINT_TEXT: Record<HintTab, string> = {
  console:
    "Type what you want done. To hand work over and let the agent run on its own, give it a task.",
  work: "Tasks you create here are given to this agent.",
  settings:
    "Autopilot: choose whether the agent also runs every few minutes on its own. Everything else is optional.",
};
