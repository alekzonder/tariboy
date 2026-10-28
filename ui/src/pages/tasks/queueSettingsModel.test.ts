import { describe, expect, it } from "vitest";
import { ApiError, type AgentPool, type QueueWorkflow } from "@/lib/api";
import { agentPool, developmentManifest } from "@/test/fixtures/workflowFixtures";
import {
  classifyBindError,
  generalInput,
  newQueueInput,
  queueSummary,
  queueTabs,
  queueWarns,
  requiredWithMissing,
  shownTab,
  stillEmptyPools,
  workflowEnvHints,
  type QueueEntry,
} from "./queueSettingsModel";

describe("workflowEnvHints", () => {
  it("reads the env names with defaults and the required secrets, sorted", () => {
    expect(
      workflowEnvHints({
        ...developmentManifest.definition,
        env: { TIMEOUT: "30", AUTO_MERGE: "false" },
        requires_secrets: ["GH_TOKEN", "API_KEY"],
      }),
    ).toEqual({
      env: [
        { name: "AUTO_MERGE", title: "default: false" },
        { name: "TIMEOUT", title: "default: 30" },
      ],
      secrets: [{ name: "API_KEY" }, { name: "GH_TOKEN" }],
    });
  });

  it("has no hints without a definition or its lists", () => {
    expect(workflowEnvHints(null)).toEqual({ env: [], secrets: [] });
    expect(workflowEnvHints(developmentManifest.definition)).toEqual({ env: [], secrets: [] });
  });
});

describe("classifyBindError", () => {
  it.each<[string, unknown, ReturnType<typeof classifyBindError>]>([
    [
      "a revision conflict",
      new ApiError(409, "revision_conflict", "changed", { current_revision: 4 }),
      { kind: "conflict" },
    ],
    [
      "empty pools",
      new ApiError(409, "workflow_pool_empty", "no agents: dev", { pools: ["dev", 2] }),
      { kind: "pools", names: ["dev", "2"], message: "no agents: dev" },
    ],
    [
      "missing secrets",
      new ApiError(409, "workflow_secret_missing", "no value: TOKEN", { secrets: ["TOKEN"] }),
      { kind: "secrets", names: ["TOKEN"], message: "no value: TOKEN" },
    ],
    [
      "a detail that is not a list",
      new ApiError(409, "workflow_pool_empty", "no agents", { pools: "dev" }),
      { kind: "pools", names: [], message: "no agents" },
    ],
    [
      "another API error",
      new ApiError(500, "internal", "broke"),
      { kind: "other", message: "broke" },
    ],
    ["a plain error", new Error("offline"), { kind: "other", message: "offline" }],
  ])("reads %s", (_name, err, expected) => {
    expect(classifyBindError(err)).toEqual(expected);
  });
});

describe("stillEmptyPools", () => {
  const pool = (name: string, agents: string[]): AgentPool => ({
    id: 1,
    queue: "DEV",
    name,
    agents,
    revision: 1,
    created_at: "",
    updated_at: "",
  });
  it.each<[string, string[], AgentPool[], string[]]>([
    ["a pool that gained an agent", ["dev", "qa"], [pool("dev", ["jack"])], ["qa"]],
    ["a pool still without agents", ["dev"], [pool("dev", [])], ["dev"]],
    ["a pool that does not exist", ["dev"], [], ["dev"]],
    ["no named pools", [], [pool("dev", [])], []],
  ])("keeps %s", (_name, names, pools, expected) => {
    expect(stillEmptyPools(names, pools)).toEqual(expected);
  });
});

const definition = developmentManifest.definition;
const binding: QueueWorkflow = {
  queue: "DEV",
  name: "development",
  version: "0.3.0",
  digest: "9232ab",
  tag: "latest",
  revision: 2,
  updated_at: "",
};
const staffed = [agentPool("developers", ["jack"]), agentPool("reviewers", ["rita"])];

describe("queueSummary and queueWarns", () => {
  it.each<[string, QueueEntry | undefined, string, boolean]>([
    ["a queue still loading", undefined, "…", false],
    ["a queue that failed to load", { error: "boom" }, "unknown", false],
    [
      "a flexible queue",
      { state: { binding: null, definition: null, pools: [] } },
      "flexible",
      false,
    ],
    [
      "a bound queue with every pool staffed",
      { state: { binding, definition, pools: staffed } },
      "development@0.3.0",
      false,
    ],
    [
      "a bound queue with an empty pool",
      { state: { binding, definition, pools: [agentPool("developers", ["jack"])] } },
      "development@0.3.0",
      true,
    ],
  ])("reads %s", (_name, entry, summary, warns) => {
    expect(queueSummary(entry)).toBe(summary);
    expect(queueWarns(entry)).toBe(warns);
  });
});

describe("requiredWithMissing", () => {
  it.each<[string, Parameters<typeof requiredWithMissing>[0], string[], [string, string[]][]]>([
    ["no definition and no refusal", null, [], []],
    ["a refusal without a definition", null, ["qa"], [["qa", []]]],
    [
      "the definition's pools",
      definition,
      [],
      [
        ["developers", ["plan"]],
        ["reviewers", ["review"]],
      ],
    ],
    [
      "a refused pool the definition already names",
      definition,
      ["developers", "qa"],
      [
        ["developers", ["plan"]],
        ["reviewers", ["review"]],
        ["qa", []],
      ],
    ],
  ])("joins %s", (_name, def, missing, expected) => {
    expect([...requiredWithMissing(def, missing)]).toEqual(expected);
  });
});

describe("queueTabs and shownTab", () => {
  it.each([
    ["a flexible queue", null, ["general", "workflow", "pools", "environment"]],
    ["a bound queue", binding, ["general", "workflow", "pools", "environment", "sources"]],
  ] as const)("lists the tabs of %s", (_name, bound, tabs) => {
    expect(queueTabs(bound)).toEqual(tabs);
  });

  it.each([
    ["sources", null, "general"],
    ["sources", binding, "sources"],
    ["pools", null, "pools"],
  ] as const)("shows %s with binding %#", (tab, bound, shown) => {
    expect(shownTab(tab, bound)).toBe(shown);
  });
});

describe("generalInput and newQueueInput", () => {
  const form = (fields: Record<string, string>) => {
    const values = new FormData();
    for (const [key, value] of Object.entries(fields)) values.set(key, value);
    return values;
  };

  it("trims the general fields and splits the owners", () => {
    expect(
      generalInput(
        form({
          name: " Dev ",
          description: " Work ",
          owners: " alice, , agent:bob ",
          responsible: " jack ",
        }),
        7,
      ),
    ).toEqual({
      name: "Dev",
      description: "Work",
      owners: ["alice", "agent:bob"],
      responsible_agent: "jack",
      revision: 7,
    });
  });

  it("reads absent general fields as empty", () => {
    expect(generalInput(form({}), 1)).toEqual({
      name: "",
      description: "",
      owners: [],
      responsible_agent: "",
      revision: 1,
    });
  });

  it("builds a new queue from its prefix, name, and form fields", () => {
    expect(newQueueInput(" NEW ", " New ", form({ owners: "alice", responsible: "" }))).toEqual({
      prefix: "NEW",
      name: "New",
      owners: ["alice"],
      responsible_agent: "",
    });
  });
});
