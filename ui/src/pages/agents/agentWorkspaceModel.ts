import type { AgentBudget, AgentView } from "@/lib/api";

const TABS = [
  ["chat", "Chat"],
  ["tasks", "Tasks"],
  ["console", "Terminal"],
  ["activity", "Activity"],
  ["configuration", "Configuration"],
  ["advanced", "Advanced"],
] as const;
// The tabs Simple mode keeps; the header, Start/Stop included, is the same in both.
const SIMPLE_TABS: ReadonlySet<string> = new Set(["chat", "tasks", "console", "configuration"]);

export type AgentTab = (typeof TABS)[number][0];
export type AgentTabEntry = (typeof TABS)[number];

export function agentTabs(simple: boolean): readonly AgentTabEntry[] {
  return simple ? TABS.filter(([key]) => SIMPLE_TABS.has(key)) : TABS;
}

// An unknown or hidden tab opens the default one: Terminal, or Chat in Simple.
export function resolveAgentTab({
  tab,
  simple,
  base,
}: {
  tab: string;
  simple: boolean;
  base: string;
}): { tab: AgentTab } | { redirect: string } {
  const known = agentTabs(simple).find(([key]) => key === tab);
  if (known) return { tab: known[0] };
  return { redirect: `${base}/${simple ? "chat" : "console"}` };
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
