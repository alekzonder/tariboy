import { describe, expect, it } from "vitest";
import type { AgentRow, Letter, Task } from "@/lib/api";
import type { HostNeeds } from "./needsYou";
import { agentNeeds, deriveAgentState, type AgentNeeds } from "./agentState";

const task = (over: Partial<Task>): Task =>
  ({ key: "T-1", updated_at: "", assignee: "agent:bob", ...over }) as Task;
const question = (key: string, over: Partial<Letter> = {}): Letter =>
  ({
    id: `m-${key}`,
    kind: "task.question",
    from: "agent:bob",
    created_at: "2026-10-30T10:00:00Z",
    data: { task_key: key },
    answered_at: "",
    dead_at: "",
    ...over,
  }) as Letter;
const host = (over: Partial<HostNeeds>): HostNeeds => ({
  hostId: "",
  hostLabel: "Local",
  tasks: [],
  letters: [],
  open: 0,
  ...over,
});

describe("agentNeeds", () => {
  it("counts a customer wait, a pause and an open question", () => {
    const needs = agentNeeds(
      host({
        tasks: [
          task({ key: "A-1", waiting_on: "customer", updated_at: "2026-10-30T12:00:00Z" }),
          task({ key: "A-2", waiting_on: "pause", updated_at: "2026-10-30T11:00:00Z" }),
        ],
        letters: [question("A-3", { created_at: "2026-10-30T09:00:00Z" })],
      }),
      "bob",
    );
    expect(needs.count).toBe(3);
    expect(needs.first).toMatchObject({ key: "A-3", question: true });
  });

  it("does not count a task in the initial status or one that waits on a script", () => {
    const needs = agentNeeds(
      host({
        tasks: [
          task({ key: "A-1", waiting_on: "customer", workflow_initial: true }),
          task({ key: "A-2", waiting_on: "script" }),
          task({ key: "A-3" }),
        ],
      }),
      "bob",
    );
    expect(needs).toEqual({ count: 0 });
  });

  it("counts a question on an already counted task once", () => {
    const needs = agentNeeds(
      host({
        tasks: [task({ key: "A-1", waiting_on: "customer", status: "approval" })],
        letters: [question("A-1")],
      }),
      "bob",
    );
    expect(needs.count).toBe(1);
    expect(needs.first).toMatchObject({ key: "A-1", status: "approval" });
  });

  it("does not count another agent's task, an answered question or a host that failed", () => {
    expect(
      agentNeeds(
        host({
          tasks: [task({ key: "A-1", waiting_on: "customer", assignee: "agent:alice" })],
          letters: [
            question("A-2", { from: "agent:alice" }),
            question("A-3", { answered_at: "2026-10-30T11:00:00Z" }),
          ],
        }),
        "bob",
      ),
    ).toEqual({ count: 0 });
    expect(agentNeeds(host({ error: "down" }), "bob")).toEqual({ count: 0 });
    expect(agentNeeds(undefined, "bob")).toEqual({ count: 0 });
  });
});

const agent = (over: Partial<AgentRow>): AgentRow =>
  ({ name: "bob", state: "idle", enabled: true, budget: { exhausted: [] }, ...over }) as AgentRow;
const none: AgentNeeds = { count: 0 };
const one: AgentNeeds = { count: 1, first: { key: "A-1", at: "", question: false } };

describe("deriveAgentState", () => {
  it.each([
    ["state error", { state: "error" }, "error"],
    ["state failed", { state: "failed" }, "error"],
    ["a halt", { halt_reason: "too many errors", halt_kind: "errors" }, "error"],
    ["an error reason", { error_reason: "launch failed" }, "error"],
    ["an exhausted budget", { budget: { exhausted: ["day"] } }, "error"],
    ["running", { state: "running" }, "working"],
    ["disabled", { enabled: false, state: "stopped" }, "off"],
    ["idle", {}, "idle"],
    ["an idle_limit halt", { halt_kind: "idle_limit", halt_reason: "idle_limit: 3" }, "idle"],
  ] as const)("%s", (_name, over, kind) => {
    expect(deriveAgentState({ agent: agent(over as Partial<AgentRow>), needs: none }).kind).toBe(
      kind,
    );
  });

  it("names the error reason", () => {
    expect(deriveAgentState({ agent: agent({ halt_reason: "boom" }), needs: none })).toEqual({
      kind: "error",
      reason: "boom",
    });
    expect(
      deriveAgentState({
        agent: agent({ budget: { exhausted: ["day", "week"] } as AgentRow["budget"] }),
        needs: none,
      }),
    ).toEqual({ kind: "error", reason: "Out of budget: day, week" });
  });

  it("puts needs above working and off, and error above needs", () => {
    expect(deriveAgentState({ agent: agent({ state: "running" }), needs: one })).toEqual({
      kind: "needs",
      needs: one,
    });
    expect(deriveAgentState({ agent: agent({ enabled: false }), needs: one }).kind).toBe("needs");
    expect(deriveAgentState({ agent: agent({ state: "error" }), needs: one }).kind).toBe("error");
  });

  it("carries the start of the current run", () => {
    expect(
      deriveAgentState({
        agent: agent({ state: "running", current_iteration_started_at: "2026-10-30T10:00:00Z" }),
        needs: none,
      }),
    ).toEqual({ kind: "working", since: "2026-10-30T10:00:00Z" });
  });

  it("prefers the fresher status over the row", () => {
    const status = { state: "running", enabled: true, budget: { exhausted: [] } };
    expect(deriveAgentState({ agent: agent({ state: "idle" }), status, needs: none }).kind).toBe(
      "working",
    );
  });
});
