import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import type { AgentView, AgentStatus } from "@/lib/types";
import AgentOverview from "./AgentOverview";

class SpyES {
  constructor() {}
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

function mkView(interactive: boolean): AgentView {
  return {
    name: "foo", image: "img", digest: "sha256:x", state: "running", cwd: "/w",
    harness: "claude", model: "sonnet", effort: "medium", interactive,
    loop_enabled: true, interval_s: 60, timeout_s: 300, hard_timeout_s: 600, max_idle_iterations: 0,
    on_timeout: "skip", on_error: "skip", user_prompt: "", env: {}, plugins: [], group: null,
    alias: "", notes: "",
  };
}
const status: AgentStatus = {
  name: "foo", state: "running", loop_enabled: true, iterations: 1, last_iteration: null,
  last_iteration_id: null, status_message: "doing work", status_updated: "2026-07-09T10:00:00Z",
};

function stub(interactive: boolean) {
  vi.stubGlobal("EventSource", SpyES as unknown as typeof EventSource);
  vi.stubGlobal("fetch", vi.fn().mockImplementation((url: string) => {
    const u = url as string;
    let result: unknown = {};
    if (u.includes("/status/history")) result = { events: [], count: 0 };
    else if (u.endsWith("/status")) result = status;
    else if (u.includes("/subscriptions") || u.includes("/channels")) result = { channels: [] };
    else if (u.includes("/logs")) result = { events: [] };
    else if (u.includes("/alias")) result = { name: "foo", alias: "" };
    else if (u.includes("/notes")) result = { name: "foo", notes: "" };
    else if (u.includes("/screen")) result = { screen: "" };
    else result = mkView(interactive);
    return Promise.resolve({
      ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result }),
    } as Response);
  }));
}

afterEach(() => vi.restoreAllMocks());

function renderOverview() {
  // The status snapshot is polled once in AgentLayout and handed down via
  // AgentStatusContext; Overview consumes it and no longer opens its own /status
  // poll. Provide the context here the way the layout would.
  return render(
    <MemoryRouter initialEntries={["/agent/foo"]}>
      <DaemonProvider>
        <AgentNameContext.Provider value="foo">
          <AgentStatusContext.Provider value={{ status, refresh: async () => {} }}>
            <AgentOverview />
          </AgentStatusContext.Provider>
        </AgentNameContext.Provider>
      </DaemonProvider>
    </MemoryRouter>,
  );
}

describe("AgentOverview", () => {
  it("renders the status message, model/effort and notes (alias moved to the layout header)", async () => {
    stub(true);
    renderOverview();
    // The alias + loop-status line moved up to AgentLayout; Overview no longer
    // renders the 'Agent:' heading. The cwd-card status message still shows here.
    await waitFor(() => expect(screen.getByText(/doing work/)).toBeInTheDocument());
    expect(screen.queryByText(/Agent:/)).not.toBeInTheDocument();
    expect(screen.getByText("Notes")).toBeInTheDocument();
    // Single-poll proof: the status now comes from context, so Overview must NOT
    // open its own bare /status poll. StatusChatHistory still hits
    // /status/history, so we exclude that path explicitly.
    const statusPolls = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls
      .map((c) => String(c[0]))
      .filter((u) => u.endsWith("/status"));
    expect(statusPolls).toHaveLength(0);
  });

  it("non-interactive agent renders the audit-log body, not the terminal", async () => {
    stub(false);
    renderOverview();
    await waitFor(() => expect(screen.getByTestId("agent-noninteractive-view")).toBeInTheDocument());
    // The body is the iteration-scoped audit log; the stub returns no
    // last_iteration_id, so its chip reads "no iterations yet".
    expect(screen.getByText(/no iterations yet|iteration /)).toBeInTheDocument();
    const screenRequests = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls
      .map((c) => String(c[0]))
      .filter((u) => u.includes("/screen"));
    expect(screenRequests).toHaveLength(0);
  });
});
