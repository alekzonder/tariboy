/** The agent tabs that carry a first-visit hint. */
export type HintTab = "console" | "tasks" | "configuration";

/** One line per tab: what to do there first. */
export const HINT_TEXT: Record<HintTab, string> = {
  console:
    "Type what you want done. To hand work over and let the agent run on its own, give it a task.",
  tasks: "Tasks you create here are given to this agent.",
  configuration:
    "Autopilot: set a schedule here to let the agent keep working without you. Everything else is optional.",
};
