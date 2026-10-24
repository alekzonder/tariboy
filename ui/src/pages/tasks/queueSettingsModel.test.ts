import { describe, expect, it } from "vitest";
import { ApiError, type AgentPool } from "@/lib/api";
import { classifyBindError, stillEmptyPools } from "./queueSettingsModel";

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
