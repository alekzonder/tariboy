import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { TerminalPane } from "./TerminalPane";
import { agentDeleteOn, agentPostOn } from "@/lib/api";
import { resolveDaemon } from "@/lib/daemons";
import type { AgentSummary } from "@/lib/types";

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, agentPostOn: vi.fn(), agentDeleteOn: vi.fn() };
});
vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: "open", absent: false, send: vi.fn(), sendResize: vi.fn(),
    attachTerm: vi.fn(), name: "a1", reconnect: vi.fn(),
  })),
}));
vi.mock("@/components/TuiScreen", () => ({ TuiScreen: () => <div data-testid="tui-screen" /> }));

const remoteTarget = {
  id: "d1", label: "prod", baseURL: "https://prod:8765", token: "token",
};

function agent(over: Partial<AgentSummary> = {}): AgentSummary {
  return {
    name: "a1", image: "bare:latest", state: "running", harness: "claude",
    loop_enabled: false, group: null, interactive: true, ...over,
  };
}

function renderPane(value = agent()) {
  const refresh = vi.fn();
  render(
    <MemoryRouter initialEntries={["/agents/d1/a1/console"]}>
      <Routes>
        <Route
          path="/agents/:hostId/:agent/console"
          element={<TerminalPane hostId="d1" agent={value} refresh={refresh} />}
        />
        <Route path="/" element={<div>Agents list</div>} />
      </Routes>
    </MemoryRouter>,
  );
  return refresh;
}

beforeEach(async () => {
  localStorage.clear();
  sessionStorage.clear();
  localStorage.setItem(
    "tariboy_daemons",
    JSON.stringify([{ id: remoteTarget.id, label: remoteTarget.label, baseURL: remoteTarget.baseURL }]),
  );
  sessionStorage.setItem(`tariboy_daemon_token_${remoteTarget.id}`, remoteTarget.token);
  await resolveDaemon(remoteTarget.id);
  vi.mocked(agentPostOn).mockReset();
  vi.mocked(agentDeleteOn).mockReset();
});
afterEach(() => vi.restoreAllMocks());

describe("TerminalPane compatibility wrapper", () => {
  // Stop / Kill / Delete are asserted in AgentControls.test.tsx: they moved out
  // of the console pane and into the agent header, so they are reachable from
  // every tab rather than only this one.

  it("starts a stopped interactive agent with start, not restart", async () => {
    vi.mocked(agentPostOn).mockResolvedValue({});
    renderPane(agent({ state: "stopped" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Start" })[0]);
    await waitFor(() => expect(agentPostOn).toHaveBeenCalledWith(remoteTarget, "a1", "start"));
    expect(agentPostOn).not.toHaveBeenCalledWith(remoteTarget, "a1", "restart");
  });

  it("keeps a live terminal attached for an idle interactive agent", () => {
    renderPane(agent({ state: "idle" }));
    expect(screen.getByTestId("tui-screen")).toBeInTheDocument();
  });

  it("links non-interactive agents directly to Configuration", () => {
    renderPane(agent({ interactive: false }));
    expect(screen.getByRole("link", { name: "Open Configuration" })).toHaveAttribute(
      "href",
      "/agents/d1/a1/configuration",
    );
  });
});
