import { describe, expect, it } from "vitest";
import { newAgentDraft } from "../agentCreateDraft";
import * as specModule from "./createAgentSpec";

describe("buildCreateAgentSpec", () => {
  it("preserves typed settings while applying bare and schema-v2 rules", () => {
    const draft = { ...newAgentDraft("worker:latest"), name: " worker ", cwd: " /work ", createCwd: true, interactive: false, plugins: ["git"], envText: '{"KEY":"value"}' };
    expect(specModule.buildCreateAgentSpec(draft, true, true)).toMatchObject({
      name: "worker", cwd: "/work", create_cwd: true, interactive: true,
      env: { KEY: "value" }, interval_s: 0, messages_batch: 10,
    });
    expect(specModule.buildCreateAgentSpec(draft, true, true)).not.toHaveProperty("plugins");
    expect(specModule.buildCreateAgentSpec(draft, false, false).plugins).toEqual(["git"]);
  });

  it("rejects invalid environment values and fractional numbers", () => {
    const draft = newAgentDraft();
    expect(() => specModule.buildCreateAgentSpec({ ...draft, envText: '{"KEY":3}' }, false, false)).toThrow("values are strings");
    expect(() => specModule.buildCreateAgentSpec({ ...draft, messagesBatch: "1.5" }, false, false)).toThrow("Message batch size must be a whole number");
  });
});
