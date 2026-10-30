import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@/test/queryRender";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import type { AgentRow, Task } from "@/lib/api";
import type { HostNeeds } from "@/lib/needsYou";
import { NeedsYouContext } from "@/pages/terminals/needsYouContext";
import { addDaemon } from "@/lib/daemons";
import AgentWorkspace from "./AgentWorkspace";
import { canOpenAgentCwdInVSCode } from "./agentCwdVSCode";
import { ApiError } from "@/lib/api";
import { agentRow } from "@/test/fixtures/apiFixtures";
import type { Daemon } from "@/lib/daemons";
import * as desktop from "@/lib/desktop";

// Every agent-scoped GET the workspace sends, recorded with the host of the
// client that sent it: (host, agent, action).
const agentGetOn = vi.hoisted(() =>
  vi.fn<(target: Daemon | null, name: string, action: string) => Promise<unknown>>(),
);
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  const createApi: typeof actual.createApi = (target) => {
    const client = actual.createApi(target);
    const get = <T,>(name: string, action: string) =>
      agentGetOn(client.target, name, action) as Promise<T>;
    return {
      ...client,
      agentGet: get,
      agents: {
        ...client.agents,
        getAgent: (name: string) => get(name, ""),
        getAgentStatus: (name: string) => get(name, "status"),
      },
    } as typeof client;
  };
  return { ...actual, createApi };
});
vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: "closed",
    absent: false,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name: "worker",
    reconnect: vi.fn(),
  })),
}));
vi.mock("@/components/TuiScreen", () => ({ TuiScreen: () => <div>terminal</div> }));

function Location() {
  return (
    <output data-testid="location">
      {useLocation().pathname}
      {useLocation().search}
    </output>
  );
}

const agent = agentRow({
  name: "worker",
  image: "worker:v1",
  state: "running",
  harness: "codex",
  cwd: "/srv/worker project",
  current_goal_task_key: "IMPROVE-37",
});

const runningStatus = {
  name: "worker",
  state: "running",
  enabled: true,
  iterations: 0,
  last_iteration: null,
  last_iteration_id: null,
  status_message: "",
  status_updated: "",
};

function renderAt(
  path: string,
  {
    hostId = "",
    row = agent,
    unavailable = false,
    needs = [],
  }: { hostId?: string; row?: AgentRow; unavailable?: boolean; needs?: HostNeeds[] } = {},
) {
  return render(
    <DaemonProvider>
      <NeedsYouContext.Provider value={{ hosts: needs, loaded: true, reload: () => {} }}>
        <MemoryRouter initialEntries={[path]}>
          <Location />
          <Routes>
            <Route
              path="/agents/:hostId/:agent/:tab"
              element={
                <AgentWorkspace
                  hostId={hostId}
                  agent={row}
                  refresh={vi.fn()}
                  unavailable={unavailable}
                />
              }
            />
          </Routes>
        </MemoryRouter>
      </NeedsYouContext.Provider>
    </DaemonProvider>,
  );
}

// Radix opens a DropdownMenu on pointerdown, which jsdom's click does not
// synthesize — drive it from the keyboard instead.
const openMenu = () =>
  fireEvent.keyDown(screen.getByRole("button", { name: "More agent actions" }), { key: "Enter" });

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  agentGetOn.mockResolvedValue(runningStatus);
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("AgentWorkspace header", () => {
  it("says what the agent works on and offers Give task", async () => {
    renderAt("/agents/local/worker/console");

    expect(await screen.findByTestId("agent-sentence")).toHaveTextContent("Working on IMPROVE-37");
    expect(screen.getByRole("link", { name: "Give task" })).toHaveAttribute(
      "href",
      "/agents/local/worker/work",
    );
    expect(screen.getByRole("button", { name: "Stop" })).toBeInTheDocument();
  });

  it("offers Answer for a task that waits for approval and counts it on Work", async () => {
    const task = {
      key: "DEV-1",
      assignee: "agent:worker",
      status: "approval",
      waiting_on: "customer",
      updated_at: new Date().toISOString(),
    } as Task;
    renderAt("/agents/local/worker/console", {
      needs: [{ hostId: "", hostLabel: "Local", tasks: [task], letters: [], open: 0 }],
    });

    expect(await screen.findByTestId("agent-sentence")).toHaveTextContent(
      "Waiting for you: approval for DEV-1",
    );
    await userEvent.click(screen.getByRole("link", { name: "Answer" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/agents/local/worker/work?task=DEV-1",
    );
    const nav = screen.getByRole("navigation", { name: "Agent workspace" });
    expect(within(nav).getByRole("link", { name: /Work/ })).toContainElement(
      screen.getByRole("img", { name: "1 waiting for you" }),
    );
  });

  it("offers one Start for an agent that is off", async () => {
    agentGetOn.mockResolvedValue({ ...runningStatus, state: "stopped", enabled: false });
    renderAt("/agents/local/worker/console", {
      row: { ...agent, state: "stopped", enabled: false },
    });

    expect(await screen.findByTestId("agent-sentence")).toHaveTextContent(
      "Off. Start to let it work on tasks.",
    );
    expect(screen.getAllByRole("button", { name: "Start" })).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });

  it("shows the configured harness, model, and effort on the facts line", async () => {
    agentGetOn.mockImplementation(async (_target, _name, action) =>
      action === ""
        ? { ...agent, harness: "codex", model: "gpt-5-codex", effort: "high" }
        : runningStatus,
    );
    renderAt("/agents/local/worker/console");

    await waitFor(() =>
      expect(screen.getByTestId("agent-runtime")).toHaveTextContent("codex · gpt-5-codex · high"),
    );
    expect(screen.getByTestId("agent-facts")).toContainElement(screen.getByTestId("agent-cwd"));
  });

  it("omits model and effort the harness does not report", async () => {
    agentGetOn.mockImplementation(async (_target, _name, action) =>
      action === "" ? { ...agent, harness: "opencode", model: "", effort: "" } : runningStatus,
    );
    renderAt("/agents/local/worker/console");

    await waitFor(() => expect(screen.getByTestId("agent-runtime")).toHaveTextContent("opencode"));
    expect(screen.getByTestId("agent-runtime")).not.toHaveTextContent("·");
  });

  it("shows daemon-authoritative message queue saturation", async () => {
    agentGetOn.mockResolvedValue({
      ...runningStatus,
      messages_pending: 100,
      messages_max_queue: 100,
      messages_queue_full: true,
    });
    renderAt("/agents/local/worker/console");

    expect(await screen.findByText("Inbox full: 100 / 100")).toHaveClass("text-destructive");
  });

  it("says why the loop halted in red and offers Restart, on every tab", async () => {
    const reason =
      "halted: image activation failed: prepare claude image skill bridge: skills missing";
    agentGetOn.mockResolvedValue({
      ...runningStatus,
      state: "error",
      halt_kind: "error",
      halt_reason: reason,
      error_reason: reason,
    });
    renderAt("/agents/local/worker/console");

    await waitFor(() => expect(screen.getByTestId("agent-sentence")).toHaveTextContent(reason));
    expect(screen.getByTestId("agent-sentence")).toHaveClass("text-destructive");
    expect(screen.getByRole("button", { name: "Restart" })).toBeInTheDocument();
  });

  it("keeps an idle_limit halt muted", async () => {
    agentGetOn.mockResolvedValue({
      ...runningStatus,
      state: "stopped",
      enabled: false,
      halt_kind: "idle_limit",
      halt_reason: "idle_limit: 3 idle runs",
    });
    renderAt("/agents/local/worker/console");

    expect(await screen.findByTestId("agent-halt-reason")).toHaveTextContent(
      "idle_limit: 3 idle runs",
    );
    expect(screen.getByTestId("agent-halt-reason")).toHaveClass("text-muted-foreground");
    expect(screen.getByTestId("agent-sentence")).not.toHaveClass("text-destructive");
  });

  it("keeps the last status in the header while a status poll cannot reach the host", async () => {
    vi.useFakeTimers();
    let reachable = true;
    agentGetOn.mockImplementation(async (_target, _name, action) => {
      if (!reachable) throw new ApiError(0, "network_error", "network error: Failed to fetch");
      return action === "status"
        ? { ...runningStatus, state: "error", halt_kind: "error", halt_reason: "halted: boom" }
        : agent;
    });
    renderAt("/agents/local/worker/console");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId("agent-sentence")).toHaveTextContent("halted: boom");

    reachable = false;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(screen.getByTestId("agent-sentence")).toHaveTextContent("halted: boom");
  });

  it("shows today's spend when the agent has a budget, even from an older daemon", async () => {
    agentGetOn.mockResolvedValue({
      ...runningStatus,
      budget: {
        hour_usd: 1,
        day_usd: 0,
        week_usd: 0,
        month_usd: 0,
        hour_spent_usd: 0.25,
        day_spent_usd: 0.25,
        week_spent_usd: 0.25,
        month_spent_usd: 0.25,
      },
    });
    renderAt("/agents/local/worker/console");

    await waitFor(() => expect(screen.getByTestId("agent-facts")).toHaveTextContent("$0.25 today"));
    expect(screen.getByTestId("agent-sentence")).not.toHaveTextContent(/Out of budget/);
  });

  it("offers the action only for local and resolved SSH hosts", () => {
    const daemons = [
      { id: "ssh-1", label: "ssh", baseURL: "http://127.0.0.1:1", kind: "ssh" as const },
      { id: "https-1", label: "https", baseURL: "https://agents.example", kind: "https" as const },
    ];

    expect(canOpenAgentCwdInVSCode("", daemons)).toBe(true);
    expect(canOpenAgentCwdInVSCode("ssh-1", daemons)).toBe(true);
    expect(canOpenAgentCwdInVSCode("https-1", daemons)).toBe(false);
    expect(canOpenAgentCwdInVSCode("missing", daemons)).toBe(false);
  });

  it("shows the effective cwd and opens the local directory in VS Code from the menu", async () => {
    const open = vi.spyOn(desktop, "openHostPathInVSCode").mockResolvedValue(null);
    renderAt("/agents/local/worker/console");

    const cwd = await screen.findByTestId("agent-cwd");
    expect(cwd).toHaveTextContent("/srv/worker project");
    expect(cwd).toHaveAttribute("title", "/srv/worker project");
    openMenu();
    await userEvent.click(await screen.findByRole("menuitem", { name: "Open folder in VS Code" }));
    expect(open).toHaveBeenCalledWith("", "/srv/worker project");
  });

  it("disables the VS Code action while the host is unavailable", async () => {
    renderAt("/agents/local/worker/console", { unavailable: true });

    await screen.findByTestId("agent-sentence");
    openMenu();
    expect(await screen.findByRole("menuitem", { name: "Open folder in VS Code" })).toHaveAttribute(
      "data-disabled",
    );
  });

  it("retries effective cwd inspection after a transient failure", async () => {
    vi.useFakeTimers();
    let inspections = 0;
    agentGetOn.mockImplementation(async (_target, _name, action) => {
      if (action !== "") return runningStatus;
      inspections += 1;
      if (inspections === 1) throw new Error("temporary inspect failure");
      return { ...agent, cwd: "/managed/worker" };
    });
    renderAt("/agents/local/worker/console", { row: { ...agent, cwd: "" } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(inspections).toBe(1);
    expect(screen.queryByTestId("agent-cwd")).not.toBeInTheDocument();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(screen.getByTestId("agent-cwd")).toHaveTextContent("/managed/worker");
    expect(inspections).toBeGreaterThanOrEqual(2);
  });
});

describe("AgentWorkspace tabs", () => {
  it("shows Work, Terminal, Activity and Settings with host and agent in every link", async () => {
    renderAt("/agents/local/worker/console");

    const nav = await screen.findByRole("navigation", { name: "Agent workspace" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((link) => link.textContent)).toEqual([
      "Work",
      "Terminal",
      "Activity",
      "Settings",
    ]);
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/agents/local/worker/work",
      "/agents/local/worker/console",
      "/agents/local/worker/activity",
      "/agents/local/worker/settings",
    ]);
  });

  it("shows the agent's task list on Work", async () => {
    renderAt("/agents/local/worker/work");

    const workspace = await screen.findByTestId("tasks-workspace");
    expect(workspace).toHaveAttribute("data-scope-agent", "worker");
    expect(workspace).toHaveAttribute("data-goal-task-key", "IMPROVE-37");
    expect(screen.getByText("Tasks you create here are given to this agent.")).toBeInTheDocument();
  });

  it("loads Settings from the host identified by the route and links the other views", async () => {
    const host = await addDaemon({
      label: "Remote",
      baseURL: "https://remote.example",
      token: "secret",
    });
    renderAt(`/agents/${host.id}/worker/settings`, { hostId: host.id });

    await waitFor(() =>
      expect(agentGetOn.mock.calls).toContainEqual([
        expect.objectContaining({ id: host.id }),
        "worker",
        "",
      ]),
    );
    expect(await screen.findByRole("link", { name: "Open mailbox" })).toHaveAttribute(
      "href",
      `/agents/${host.id}/worker/mailbox`,
    );
    expect(screen.getByRole("link", { name: "Advanced" })).toHaveAttribute(
      "href",
      `/agents/${host.id}/worker/advanced`,
    );
  });

  it.each([
    [
      "the old Tasks tab",
      "/agents/local/worker/tasks?task=DEV-1",
      "/agents/local/worker/work?task=DEV-1",
    ],
    [
      "the old Configuration tab",
      "/agents/local/worker/configuration",
      "/agents/local/worker/settings",
    ],
    ["the removed autopilot tab", "/agents/local/worker/autopilot", "/agents/local/worker/work"],
  ])("redirects %s", async (_name, from, to) => {
    renderAt(from);
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(to));
  });

  it.each(["mailbox", "advanced"])("opens the %s route without a tab", async (tab) => {
    renderAt(`/agents/local/worker/${tab}`);

    await screen.findByRole("navigation", { name: "Agent workspace" });
    expect(screen.getByTestId("location")).toHaveTextContent(`/agents/local/worker/${tab}`);
    expect(screen.queryByText("Pick an agent")).not.toBeInTheDocument();
  });
});
