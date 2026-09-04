import { describe, expect, it } from "vitest";
import type { AgentView } from "@/lib/types";
import { cloneAgentDraft, newAgentDraft } from "./agentCreateDraft";

const source = {
  name: "source",
  image: "worker:v1",
  digest: "sha256",
  state: "stopped",
  cwd: "/managed/source/workdir",
  configured_cwd: "",
  harness: "codex",
  model: "gpt-5",
  effort: "high",
  interactive: true,
  loop_enabled: false,
  enabled: false,
  interval_s: 12,
  timeout_s: 34,
  hard_timeout_s: 56,
  on_timeout: "stop",
  on_error: "restart",
  max_idle_iterations: 7,
  user_prompt: "standing",
  env: { CSV: "a,b", EQ: "a=b", LINES: "one\ntwo" },
  plugins: ["context", "custom"],
  messages_batch: 8,
  messages_max_queue: 900,
  group: "reviewers",
  alias: "Clone",
  notes: "all fields",
  color: "#123abc",
  goal_enabled: false,
  goal_wait_customer_timeout_s: 120,
  current_goal_task_key: "TARI-43",
} satisfies AgentView;

describe("newAgentDraft", () => {
  it("defines the complete ordinary creation defaults", () => {
    expect(newAgentDraft()).toEqual({
      image: "",
      name: "",
      cwd: "",
      harness: "",
      model: "",
      effort: "",
      interactive: false,
      loop: true,
      startNow: true,
      intervalS: "0",
      timeoutS: "7200",
      hardTimeoutS: "10800",
      onTimeout: "restart",
      onError: "restart",
      maxIdleIterations: "0",
      userPrompt: "",
      envText: "{}",
      plugins: [],
      messagesBatch: "10",
      messagesMaxQueue: "1000",
      group: "",
      alias: "",
      notes: "",
      color: "",
      goalEnabled: true,
      goalWaitCustomerTimeoutS: "300",
    });
  });
});

describe("cloneAgentDraft", () => {
  it("copies every included field, leaves name blank, and uses raw configured cwd", () => {
    expect(cloneAgentDraft(source)).toEqual({
      image: "worker:v1",
      name: "",
      cwd: "",
      harness: "codex",
      model: "gpt-5",
      effort: "high",
      interactive: true,
      loop: false,
      startNow: false,
      intervalS: "12",
      timeoutS: "34",
      hardTimeoutS: "56",
      onTimeout: "stop",
      onError: "restart",
      maxIdleIterations: "7",
      userPrompt: "standing",
      envText: "{\n  \"CSV\": \"a,b\",\n  \"EQ\": \"a=b\",\n  \"LINES\": \"one\\ntwo\"\n}",
      plugins: ["context", "custom"],
      messagesBatch: "8",
      messagesMaxQueue: "900",
      group: "reviewers",
      alias: "Clone",
      notes: "all fields",
      color: "#123abc",
      goalEnabled: false,
      goalWaitCustomerTimeoutS: "120",
    });
  });

  it.each(["configured_cwd", "messages_batch", "messages_max_queue", "goal_enabled", "goal_wait_customer_timeout_s"] as const)(
    "requires current daemon projection field %s",
    (field) => {
      const incomplete = { ...source } as Record<string, unknown>;
      delete incomplete[field];
      expect(() => cloneAgentDraft(incomplete as unknown as AgentView)).toThrow(
        /update.*host.*complete clone/i,
      );
    },
  );
});
