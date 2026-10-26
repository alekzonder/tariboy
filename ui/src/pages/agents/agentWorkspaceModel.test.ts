import { describe, expect, it } from "vitest";
import {
  agentRuntimeLabel,
  agentTabs,
  hasBudgetLimits,
  resolveAgentTab,
} from "./agentWorkspaceModel";
import type { AgentBudget } from "@/lib/api";

const base = "/agents/local/dev";

describe("agentTabs", () => {
  it("lists every tab in Expert mode", () => {
    expect(agentTabs(false).map(([key]) => key)).toEqual([
      "mailbox",
      "tasks",
      "console",
      "activity",
      "configuration",
      "advanced",
    ]);
  });

  it("keeps only the Simple tabs in Simple mode", () => {
    expect(agentTabs(true).map(([key]) => key)).toEqual([
      "mailbox",
      "tasks",
      "console",
      "configuration",
    ]);
  });
});

describe("resolveAgentTab", () => {
  it.each<[string, Parameters<typeof resolveAgentTab>[0], ReturnType<typeof resolveAgentTab>]>([
    ["a known tab", { tab: "activity", simple: false, base }, { tab: "activity" }],
    ["a Simple tab", { tab: "tasks", simple: true, base }, { tab: "tasks" }],
    ["Mailbox in Expert", { tab: "mailbox", simple: false, base }, { tab: "mailbox" }],
    ["Mailbox in Simple", { tab: "mailbox", simple: true, base }, { tab: "mailbox" }],
    [
      "an unknown tab in Expert",
      { tab: "nope", simple: false, base },
      { redirect: `${base}/console` },
    ],
    [
      "an unknown tab in Simple",
      { tab: "nope", simple: true, base },
      { redirect: `${base}/tasks` },
    ],
    ["a tab Simple hides", { tab: "advanced", simple: true, base }, { redirect: `${base}/tasks` }],
  ])("resolves %s", (_name, input, expected) => {
    expect(resolveAgentTab(input)).toEqual(expected);
  });
});

describe("agentRuntimeLabel", () => {
  it("prefers the view harness and joins model and effort", () => {
    expect(
      agentRuntimeLabel({
        agentHarness: "claude",
        view: { harness: "codex", model: "gpt-5", effort: "high" },
      }),
    ).toBe("codex · gpt-5 · high");
  });

  it("falls back to the agent harness and skips empty parts", () => {
    expect(agentRuntimeLabel({ agentHarness: "claude", view: { harness: "", model: "" } })).toBe(
      "claude",
    );
    expect(agentRuntimeLabel({ agentHarness: "claude", view: null })).toBe("claude");
  });

  it("is empty when nothing is known", () => {
    expect(agentRuntimeLabel({ agentHarness: "", view: null })).toBe("");
  });
});

describe("hasBudgetLimits", () => {
  const none: AgentBudget = {
    hour_usd: 0,
    day_usd: 0,
    week_usd: 0,
    month_usd: 0,
    hour_spent_usd: 1,
    day_spent_usd: 1,
    week_spent_usd: 1,
    month_spent_usd: 1,
    exhausted: null,
  };

  it("is false without a budget or with no limits", () => {
    expect(hasBudgetLimits(undefined)).toBe(false);
    expect(hasBudgetLimits(none)).toBe(false);
  });

  it.each(["hour_usd", "day_usd", "week_usd", "month_usd"] as const)(
    "is true when %s is set",
    (key) => {
      expect(hasBudgetLimits({ ...none, [key]: 5 })).toBe(true);
    },
  );
});
