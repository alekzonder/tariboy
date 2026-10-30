import { describe, expect, it } from "vitest";
import {
  AGENT_TABS,
  headerSentence,
  primaryAction,
  agentRuntimeLabel,
  hasBudgetLimits,
  resolveAgentTab,
} from "./agentWorkspaceModel";
import type { AgentBudget } from "@/lib/api";

const base = "/agents/local/dev";

describe("AGENT_TABS", () => {
  it("lists Work, Terminal, Activity and Settings", () => {
    expect(AGENT_TABS).toEqual([
      ["work", "Work"],
      ["console", "Terminal"],
      ["activity", "Activity"],
      ["settings", "Settings"],
    ]);
  });
});

describe("resolveAgentTab", () => {
  it.each<[string, Parameters<typeof resolveAgentTab>[0], ReturnType<typeof resolveAgentTab>]>([
    ["a tab", { tab: "activity", base, search: "" }, { tab: "activity" }],
    ["Mailbox, a route without a tab", { tab: "mailbox", base, search: "" }, { tab: "mailbox" }],
    ["Advanced, a route without a tab", { tab: "advanced", base, search: "" }, { tab: "advanced" }],
    [
      "the old Tasks tab, with its query",
      { tab: "tasks", base, search: "?task=T-1" },
      { redirect: `${base}/work?task=T-1` },
    ],
    [
      "the old Configuration tab",
      { tab: "configuration", base, search: "?section=budget" },
      { redirect: `${base}/settings?section=budget` },
    ],
    ["no tab", { tab: "", base, search: "" }, { redirect: `${base}/work` }],
    ["an unknown tab", { tab: "nope", base, search: "?x=1" }, { redirect: `${base}/work` }],
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

describe("headerSentence", () => {
  const now = new Date("2026-10-30T12:30:00Z");
  const ctx = { goalTaskKey: "", runs: 0, now };
  const item = { key: "DEV-1", at: "2026-10-30T12:01:00Z", status: "approval", question: false };

  it("names the one thing the agent waits for, with its age", () => {
    expect(headerSentence({ kind: "needs", needs: { count: 1, first: item } }, ctx)).toBe(
      "Waiting for you: approval for DEV-1 (29 min)",
    );
    expect(
      headerSentence(
        {
          kind: "needs",
          needs: { count: 1, first: { ...item, status: undefined, question: true } },
        },
        ctx,
      ),
    ).toBe("Waiting for you: a question for DEV-1 (29 min)");
  });

  it("counts several things", () => {
    expect(headerSentence({ kind: "needs", needs: { count: 3, first: item } }, ctx)).toBe(
      "Waiting for you: 3 things",
    );
  });

  it("names the Goal task and the run while working", () => {
    const since = new Date(2026, 9, 30, 9, 5).toISOString();
    expect(
      headerSentence({ kind: "working", since }, { goalTaskKey: "DEV-2", runs: 14, now }),
    ).toBe("Working on DEV-2 since 09:05 · run 14");
    expect(headerSentence({ kind: "working", since }, ctx)).toBe("Working since 09:05");
    expect(headerSentence({ kind: "working" }, ctx)).toBe("Working");
  });

  it.each([
    [{ kind: "idle" }, "Idle. Give it a task."],
    [{ kind: "off" }, "Off. Start to let it work on tasks."],
    [{ kind: "error", reason: "Out of budget: day" }, "Out of budget: day"],
  ] as const)("says %o", (state, text) => {
    expect(headerSentence(state, ctx)).toBe(text);
  });
});

describe("primaryAction", () => {
  it.each([
    [{ kind: "needs", needs: { count: 1 } }, "answer"],
    [{ kind: "working" }, "give"],
    [{ kind: "idle" }, "give"],
    [{ kind: "off" }, "start"],
    [{ kind: "error", reason: "x" }, "restart"],
  ] as const)("%o → %s", (state, action) => {
    expect(primaryAction(state)).toBe(action);
  });
});
