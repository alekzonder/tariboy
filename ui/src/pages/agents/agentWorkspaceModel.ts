import type { AgentNeeds, AgentState } from "@/lib/agentState";
import type { AgentBudget, AgentView } from "@/lib/api";
import { elapsed } from "@/pages/home/homeModel";

export const AGENT_TABS = [
  ["work", "Work"],
  ["console", "Terminal"],
  ["activity", "Activity"],
  ["settings", "Settings"],
] as const;
// Routes kept without a tab: Settings links to them.
const HIDDEN_TABS = ["mailbox", "advanced"] as const;
// Former tab names and the tab that replaced each.
const RENAMED: Record<string, AgentTab> = { tasks: "work", configuration: "settings" };

export type AgentTab = (typeof AGENT_TABS)[number][0] | (typeof HIDDEN_TABS)[number];
export const DEFAULT_AGENT_TAB: AgentTab = "work";

// A renamed tab keeps its query (`?task=`, `?view=`); an unknown one, or none,
// opens Work.
export function resolveAgentTab({
  tab,
  base,
  search,
}: {
  tab: string;
  base: string;
  search: string;
}): { tab: AgentTab } | { redirect: string } {
  const known = [...AGENT_TABS.map(([key]) => key), ...HIDDEN_TABS].find((key) => key === tab);
  if (known) return { tab: known };
  const renamed = RENAMED[tab];
  if (renamed) return { redirect: `${base}/${renamed}${search}` };
  return { redirect: `${base}/${DEFAULT_AGENT_TAB}` };
}

// Configured runtime, compactly: whatever the harness actually reports.
// Codex has no model until one is chosen; only Codex reports an effort.
export function agentRuntimeLabel({
  agentHarness,
  view,
}: {
  agentHarness: string;
  view: Partial<Pick<AgentView, "harness" | "model" | "effort">> | null;
}): string {
  return [view?.harness || agentHarness, view?.model, view?.effort].filter(Boolean).join(" · ");
}

export function hasBudgetLimits(budget: AgentBudget | undefined): budget is AgentBudget {
  if (!budget) return false;
  return budget.hour_usd > 0 || budget.day_usd > 0 || budget.week_usd > 0 || budget.month_usd > 0;
}

const clock = (iso: string) => {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(at.getHours())}:${pad(at.getMinutes())}`;
};

function needsSentence({ count, first }: AgentNeeds, now: Date): string {
  if (count > 1 || !first) return `Waiting for you: ${count} things`;
  const what = first.question ? "a question" : first.status || "a decision";
  const age = elapsed(first.at, now);
  return `Waiting for you: ${what} for ${first.key}${age ? ` (${age})` : ""}`;
}

function workingSentence(since: string | undefined, goalTaskKey: string, runs: number): string {
  const time = since ? clock(since) : "";
  const head = goalTaskKey ? `Working on ${goalTaskKey}` : "Working";
  const run = goalTaskKey && runs > 0 ? ` · run ${runs}` : "";
  return `${head}${time ? ` since ${time}` : ""}${run}`;
}

/** The header's one sentence: what the agent does, or what it waits for. */
export function headerSentence(
  state: AgentState,
  { goalTaskKey, runs, now }: { goalTaskKey: string; runs: number; now: Date },
): string {
  switch (state.kind) {
    case "needs":
      return needsSentence(state.needs, now);
    case "working":
      return workingSentence(state.since, goalTaskKey, runs);
    case "idle":
      return "Idle. Give it a task.";
    case "off":
      return "Off. Start to let it work on tasks.";
    case "error":
      return state.reason;
  }
}

export type PrimaryAction = "answer" | "give" | "start" | "restart";

const PRIMARY: Record<AgentState["kind"], PrimaryAction> = {
  needs: "answer",
  working: "give",
  idle: "give",
  off: "start",
  error: "restart",
};

/** The header's one primary button for a state. */
export const primaryAction = (state: AgentState): PrimaryAction => PRIMARY[state.kind];
