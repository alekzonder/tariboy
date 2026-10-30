import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, screen, waitFor, fireEvent, within } from "@/test/queryRender";
import { UI_MODE_KEY } from "@/lib/uiMode";
import { fetchAllAgents } from "@/lib/aggregate";
import type {
  AgentActionResult,
  AgentRunResult,
  AgentView,
  CreateAgentSpec,
  ImageManifest,
  ImageSummary,
} from "@/lib/api";
import { addDaemon, type Daemon } from "@/lib/daemons";
import {
  cleanupTerminalsPage,
  setupTerminalsPage,
  renderAt,
  registerProd,
} from "@/test/terminals/terminalsPage";
import { chooseManagedFolder } from "@/test/terminals/createAgentDialog";
import { agentRow } from "@/test/fixtures/apiFixtures";

vi.mock("@/lib/aggregate", () => ({
  fetchAllAgents: vi.fn(),
}));

// Requests the page and its dialog send, each recorded with the host of the
// client that sent it. apiOn records the daemon config writes (sidebar order).
const calls = vi.hoisted(() => ({
  agentGetOn: vi.fn<(target: Daemon | null, name: string, action: string) => Promise<unknown>>(),
  apiOn:
    vi.fn<
      (target: Daemon | null, method: string, path: string, body?: unknown) => Promise<unknown>
    >(),
  createAgent: vi.fn<(spec: CreateAgentSpec, target: Daemon | null) => Promise<AgentRunResult>>(),
  imageManifestGet: vi.fn<(ref: string, target: Daemon | null) => Promise<ImageManifest>>(),
  listImages:
    vi.fn<(target: Daemon | null) => Promise<{ images: ImageSummary[]; count: number }>>(),
  startAgent: vi.fn<(name: string, target: Daemon | null) => Promise<AgentActionResult>>(),
}));
const { createAgent, imageManifestGet, listImages } = calls;

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  const createApi: typeof actual.createApi = (target) => {
    const client = actual.createApi(target);
    const host = client.target;
    return {
      ...client,
      post: <T,>(path: string, body?: unknown) =>
        path === "/api/daemon/config"
          ? (calls.apiOn(host, "POST", path, body) as Promise<T>)
          : client.post<T>(path, body),
      agents: {
        ...client.agents,
        getAgent: (name: string) => calls.agentGetOn(host, name, "") as Promise<AgentView>,
        createAgent: (spec: CreateAgentSpec) => calls.createAgent(spec, host),
        startAgent: (name: string) => calls.startAgent(name, host),
      },
      images: {
        ...client.images,
        listImages: () => calls.listImages(host),
        imageManifestGet: (ref: string) => calls.imageManifestGet(ref, host),
      },
    } as typeof client;
  };
  return { ...actual, createApi };
});

// Selecting an agent mounts the terminal, which dials a websocket and renders
// xterm.js — neither works in jsdom. Stub both.
vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: "open",
    absent: false,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name: "a1",
    reconnect: vi.fn(),
  })),
}));
vi.mock("@/components/TuiScreen", () => ({
  TuiScreen: () => <div data-testid="tui-screen" />,
}));

beforeEach(() => setupTerminalsPage(calls));
afterEach(cleanupTerminalsPage);

describe("TerminalsPage agent routes", () => {
  it("retains the selected agent shell when its next aggregate refresh is unavailable", async () => {
    vi.useFakeTimers();
    const remote = await addDaemon({
      label: "prod",
      baseURL: "http://127.0.0.1:19992",
      token: "tok",
    });
    let state: "ready" | "unavailable" | "recovered" = "ready";
    vi.mocked(fetchAllAgents).mockImplementation(async () => [
      {
        host: { id: remote.id, label: "prod" },
        agents:
          state === "unavailable"
            ? []
            : [
                agentRow({
                  name: "worker",
                  image: state === "recovered" ? "worker:v2" : "worker:v1",
                  state: "running",
                  harness: "codex",
                  interactive: false,
                }),
              ],
        error: state === "unavailable" ? "unreachable" : undefined,
      },
    ]);

    renderAt(`/agents/${remote.id}/worker/console`);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByRole("heading", { name: "worker" })).toBeInTheDocument();

    state = "unavailable";
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(1);

    expect(screen.getByRole("heading", { name: "worker" })).toBeInTheDocument();
    expect(screen.getByText(/temporarily unavailable/i)).toHaveAttribute("role", "status");
    expect(screen.getByTestId("agent-workspace-content")).toHaveAttribute("inert", "");
    expect(screen.getByRole("button", { name: "Open worker" })).toBeDisabled();

    state = "recovered";
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(2);
    expect(screen.getByRole("heading", { name: "worker" })).toBeInTheDocument();
    expect(screen.getByText("worker:v2")).toBeInTheDocument();
    expect(screen.queryByText(/temporarily unavailable/i)).toBeNull();
    expect(screen.getByTestId("agent-workspace-content")).not.toHaveAttribute("inert");
  }, 10_000);

  it("opens a host's Tasks from its heading", async () => {
    renderAt("/");

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Open host This daemon (local)",
      }),
    );

    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/tasks?host=local"),
    );
    expect(screen.queryByRole("navigation", { name: "Server workspace" })).toBeNull();
    expect(screen.getByRole("button", { name: "Open host This daemon (local)" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("sends the old server Mailbox to the Inbox of that host", async () => {
    renderAt("/servers/local/mail");

    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/inbox?host=local"),
    );
    expect(await screen.findByRole("radiogroup", { name: "Mailbox" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Compose" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Needs you" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Inbox" })).toHaveAttribute("aria-current", "page");
  });

  it("shows only the agent's own tabs above an agent, with its host and row selected", async () => {
    renderAt("/agents/local/a1/console");

    expect(await screen.findByRole("navigation", { name: "Agent workspace" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Server workspace" })).toBeNull();
    expect(screen.getByRole("button", { name: "Open host This daemon (local)" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("button", { name: "Open a1" })).toHaveAttribute("aria-current", "page");
  });

  it("counts the agent's open questions on the Tasks tab", async () => {
    renderAt("/agents/local/a1/console", new Map([[JSON.stringify(["", "a1"]), 2]]));

    const count = await screen.findByLabelText("2 open agent questions");
    expect(count).toHaveTextContent("2");
    const nav = screen.getByRole("navigation", { name: "Agent workspace" });
    expect(within(nav).getByRole("link", { name: /Tasks/ })).toContainElement(count);
  });

  it("shows no Tasks count without open agent questions", async () => {
    renderAt("/agents/local/a1/console");

    expect(await screen.findByRole("navigation", { name: "Agent workspace" })).toBeInTheDocument();
    expect(screen.queryByLabelText(/open agent question/)).toBeNull();
  });

  it("opens a sidebar agent in Terminal", async () => {
    renderAt("/");
    await screen.findByRole("button", { name: "Open a1" });

    fireEvent.click(screen.getByRole("button", { name: "Open a1" }));

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/a1/console"),
    );
  });

  it("opens a non-interactive sidebar agent in Terminal", async () => {
    renderAt("/");
    await screen.findByRole("button", { name: "Open a2", hidden: true });

    fireEvent.click(screen.getByRole("button", { name: "Open a2" }));

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/a2/console"),
    );
    expect(await screen.findByRole("navigation", { name: "Agent workspace" })).toBeInTheDocument();
  });
});

describe("TerminalsPage route changes", () => {
  it("keeps the current agent tab when switching to another agent", async () => {
    renderAt("/agents/local/a1/activity");
    await screen.findByRole("button", { name: "Open a2", hidden: true });

    fireEvent.click(screen.getByRole("button", { name: "Open a2" }));

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/a2/activity"),
    );
  });

  it("opens the former All tasks view of an agent route as Tasks", async () => {
    renderAt("/agents/local/a1/console?view=all");
    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/tasks"));
  });

  it("drops the selected task when switching agents on the Tasks tab", async () => {
    renderAt("/agents/local/a1/tasks?task=DEV-1");
    await screen.findByRole("button", { name: "Open a2", hidden: true });

    fireEvent.click(screen.getByRole("button", { name: "Open a2", hidden: true }));

    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/agents/local/a2/tasks"),
    );
  });

  it("honors a hidden sidebar from the shared persisted state", async () => {
    localStorage.setItem("terminals:sidebar:v1", JSON.stringify({ width: 268, hidden: true }));
    renderAt("/");

    expect(await screen.findByRole("heading", { name: "Home" })).toBeInTheDocument();
    expect(screen.queryByTestId("agents-sidebar")).toBeNull();
    expect(screen.queryByRole("separator", { name: "resize sidebar" })).toBeNull();
  });

  it("asks for a server first in the Android app, which has no local daemon", async () => {
    vi.stubEnv("VITE_TARIBOY_SHELL", "android");
    try {
      vi.mocked(fetchAllAgents).mockResolvedValue([]);
      renderAt("/");

      const main = await screen.findByRole("main");
      fireEvent.click(await within(main).findByRole("button", { name: "Add host" }));
      expect(await screen.findByRole("dialog", { name: "Add host" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "New agent" })).toBeNull();
    } finally {
      vi.unstubAllEnvs();
    }
  });

  it("opens a sidebar agent in the product Terminal route", async () => {
    renderAt("/");
    fireEvent.click(await screen.findByRole("button", { name: "Open a1" }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/a1/console"),
    );
  });

  it("opens a sidebar agent on Tasks in Simple mode", async () => {
    localStorage.setItem(UI_MODE_KEY, "simple");
    renderAt("/");
    fireEvent.click(await screen.findByRole("button", { name: "Open a1" }));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/a1/tasks"),
    );
  });
});

describe("TerminalsPage empty state and create", () => {
  it("shows Home with New agent when no agent is selected", async () => {
    renderAt("/terminals");
    const main = await screen.findByRole("main");
    expect(await within(main).findByRole("heading", { name: "Home" })).toBeInTheDocument();
    expect(within(main).getByRole("button", { name: "New agent" })).toBeInTheDocument();
    for (const section of ["Needs you", "Running now", "Today", "Recent"]) {
      expect(within(main).getByRole("heading", { name: new RegExp(`^${section}`) })).toBeVisible();
    }
  });

  it("re-polls immediately after creating an agent instead of waiting for the next tick", async () => {
    createAgent.mockResolvedValue({
      name: "new-agent",
      state: "running",
      goal_enabled: false,
      goal_wait_customer_timeout_s: 0,
      goal_delivery_cooldown_s: 0,
    });
    renderAt("/terminals");
    await screen.findByRole("button", { name: "Open a1" });
    const callsBefore = vi.mocked(fetchAllAgents).mock.calls.length;

    fireEvent.click(within(screen.getByRole("main")).getByRole("button", { name: "New agent" }));
    // The form picks the only built image by itself.
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).getByLabelText("image")).toHaveValue("worker"));
    await chooseManagedFolder();
    const create = within(dialog).getByRole("button", { name: "Start agent" });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);

    await waitFor(() => expect(createAgent).toHaveBeenCalled());
    await waitFor(() =>
      expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });

  it("opens a new agent on its Terminal in Simple mode, where a sidebar agent opens on Tasks", async () => {
    localStorage.setItem(UI_MODE_KEY, "simple");
    createAgent.mockResolvedValue({
      name: "new-agent",
      state: "running",
      goal_enabled: false,
      goal_wait_customer_timeout_s: 0,
      goal_delivery_cooldown_s: 0,
    });
    renderAt("/terminals");
    await screen.findByRole("button", { name: "Open a1" });

    fireEvent.click(within(screen.getByRole("main")).getByRole("button", { name: "New agent" }));
    // The form picks the only built image by itself.
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).getByLabelText("image")).toHaveValue("worker"));
    await chooseManagedFolder();
    const create = within(dialog).getByRole("button", { name: "Start agent" });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/new-agent/console"),
    );
  });

  it("opens Run Agent with both host and image preselected", async () => {
    const id = await registerProd();
    renderAt(`/terminals?new=1&host=${id}&image=worker%3Av1`);

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("combobox", { name: "Run on" })).toHaveTextContent("prod");
    await waitFor(() => expect(within(dialog).getByLabelText("image")).toHaveValue("worker"));
    await waitFor(() => expect(listImages).toHaveBeenCalledWith(expect.objectContaining({ id })));
    expect(imageManifestGet).toHaveBeenCalledWith("worker:v1", expect.objectContaining({ id }));
  });
});
