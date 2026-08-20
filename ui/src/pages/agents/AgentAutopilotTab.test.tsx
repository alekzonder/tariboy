import { afterEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import type { AgentStatus } from "@/lib/types";
import AgentAutopilotTab from "./AgentAutopilotTab";

// The tab's own halt-reason line is what these cases assert. Its children own
// their behaviour and their own tests, so they are stubbed to keep the render
// free of loop controls and subscription fetches.
vi.mock("@/components/LoopControls", () => ({
  LoopToggle: () => <div data-testid="loop-toggle" />,
}));
vi.mock("@/components/IterationTimeoutControl", () => ({
  IterationTimeoutControl: () => <div data-testid="iteration-timeout" />,
}));
vi.mock("@/components/AgentSubscriptions", () => ({
  AgentSubscriptions: () => <div data-testid="agent-subscriptions" />,
}));

afterEach(() => vi.restoreAllMocks());

const stopped: AgentStatus = {
  name: "worker",
  state: "stopped",
  loop_enabled: false,
  iterations: 3,
  last_iteration: null,
  last_iteration_id: null,
  status_message: "",
  status_updated: "",
};

function response(result: unknown, ok = true, status = 200) {
  const body = ok
    ? { ok: true, result }
    : { ok: false, error: result };
  return Promise.resolve({
    ok,
    status,
    text: async () => JSON.stringify(body),
  } as Response);
}

// AgentAutopilotTab fires apiGet("/api/budgets/status") in a mount effect; stub
// it so the render is not waiting on a real request.
function renderAutopilot(status: AgentStatus) {
  vi.stubGlobal("fetch", vi.fn(() => response({ budgets: [] })));
  return render(
    <AgentNameContext.Provider value="worker">
      <AgentStatusContext.Provider value={{ status, refresh: async () => {} }}>
        <AgentAutopilotTab />
      </AgentStatusContext.Provider>
    </AgentNameContext.Provider>,
  );
}

it("shows the idle-limit halt reason under the Running/Stopped line", async () => {
  renderAutopilot({
    ...stopped,
    halt_kind: "idle_limit",
    halt_reason: "idle_limit (3 idle iterations)",
  });
  expect(await screen.findByText("idle_limit (3 idle iterations)")).toBeInTheDocument();
  expect(screen.getByTestId("halt-reason")).toHaveClass("text-muted-foreground");
  expect(screen.getByTestId("halt-reason")).not.toHaveClass("text-destructive");
});

// The "error" halt_kind compared here is produced by (*Agent).HaltReason in
// internal/agent/agent.go (declared at line 208), whose first case returns the
// bare literal at line 211: `return "error", a.ErrorReason`. TypeScript cannot
// import that constant, so this comment is the only coupling: if the two
// literals ever diverge the line still renders, in the wrong tone, silently.
it("shows the error halt reason under the Running/Stopped line", async () => {
  renderAutopilot({ ...stopped, halt_kind: "error", halt_reason: "halted: boom" });
  expect(await screen.findByText("halted: boom")).toBeInTheDocument();
  expect(screen.getByTestId("halt-reason")).toHaveClass("text-destructive");
  expect(screen.getByTestId("halt-reason")).not.toHaveClass("text-muted-foreground");
});

it("renders no halt-reason element when the daemon reports no reason", async () => {
  renderAutopilot(stopped);
  expect(await screen.findByText("Stopped")).toBeInTheDocument();
  await waitFor(() => expect(screen.queryByTestId("halt-reason")).not.toBeInTheDocument());
});

it("renders no halt-reason element when the reason is an empty string", async () => {
  renderAutopilot({ ...stopped, halt_kind: "", halt_reason: "" });
  expect(await screen.findByText("Stopped")).toBeInTheDocument();
  await waitFor(() => expect(screen.queryByTestId("halt-reason")).not.toBeInTheDocument());
});
