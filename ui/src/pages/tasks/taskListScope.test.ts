import { describe, expect, it } from "vitest";
import { taskListScope } from "./taskListScope";

const base = { query: "", statusView: "active" as const, customer: "user:owner", error: null };

describe("taskListScope", () => {
  it("asks for every task when no filter is on", () => {
    const scope = taskListScope({ ...base, assignee: "", waiting: false });
    expect(scope.filters).toEqual({ text: "", scope_agent: undefined, status_view: "active" });
    expect(scope.filtering).toBe(false);
    expect(scope.waitingForPrincipal).toBe(false);
  });

  it("names the customer for My tasks and Waiting for me, together", () => {
    const scope = taskListScope({ ...base, assignee: "mine", waiting: true });
    expect(scope.filters).toMatchObject({ assignee: "user:owner", waiting_for: "user:owner" });
    expect(scope.filtering).toBe(true);
  });

  it("names the scoped agent instead of the customer in an agent's view", () => {
    const scope = taskListScope({
      ...base,
      scopeAgent: "worker",
      customer: undefined,
      assignee: "mine",
      waiting: true,
    });
    expect(scope.filters).toMatchObject({
      scope_agent: "worker",
      assignee: "agent:worker",
      waiting_for: "agent:worker",
    });
    expect(scope.waitingForPrincipal).toBe(false);
  });

  it("filters by one agent without waiting for the customer", () => {
    const scope = taskListScope({
      ...base,
      customer: undefined,
      assignee: "agent:triager",
      waiting: false,
    });
    expect(scope.filters).toMatchObject({ assignee: "agent:triager" });
    expect(scope.filters.waiting_for).toBeUndefined();
    expect(scope.waitingForPrincipal).toBe(false);
    expect(scope.filtering).toBe(true);
  });

  it("waits for the customer before a customer filter can be sent", () => {
    const scope = taskListScope({ ...base, customer: undefined, assignee: "", waiting: true });
    expect(scope.waitingForPrincipal).toBe(true);
  });
});
