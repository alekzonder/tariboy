import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, fireEvent, within } from "@/test/queryRender";
import { fetchAllAgents } from "@/lib/aggregate";
import type {
  AgentActionResult,
  AgentRunResult,
  AgentView,
  CreateAgentSpec,
  ImageManifest,
  ImageSummary,
} from "@/lib/api";
import { type Daemon } from "@/lib/daemons";
import { addHostToWorkspace, createWorkspace, selectWorkspace } from "@/lib/workspaces";
import userEvent from "@testing-library/user-event";
import {
  cleanupTerminalsPage,
  setupTerminalsPage,
  renderAt,
  agentsTab,
  pickSidebarTab,
} from "@/test/terminals/terminalsPage";
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

describe("host workspaces", () => {
  beforeEach(() => {
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "This daemon (local)" },
        agents: [
          agentRow({
            name: "a1",
            image: "img",
            state: "running",
            harness: "claude",
            interactive: true,
          }),
        ],
      },
      {
        host: { id: "d1", label: "prod" },
        agents: [
          agentRow({
            name: "b1",
            image: "img",
            state: "running",
            harness: "claude",
            interactive: true,
          }),
        ],
      },
    ]);
  });

  it("filters the sidebar and its counts to the current workspace and follows the switch", async () => {
    addHostToWorkspace("d1", createWorkspace("Production")!);
    renderAt("/agents/local/a1/activity");
    expect(await screen.findByRole("button", { name: "Open a1" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open b1" })).toBeNull();
    expect(screen.getByText("1 server · 1 agent")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Workspace: Default" }));
    await userEvent.click(await screen.findByRole("menuitemradio", { name: /Production/ }));

    // The open agent is not in Production, so its first agent opens in the same tab.
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/agents/d1/b1/activity"),
    );
    expect(await screen.findByRole("button", { name: "Open b1" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open a1" })).toBeNull();
  });

  it("limits All tasks to the current workspace's servers", async () => {
    const production = createWorkspace("Production")!;
    addHostToWorkspace("d1", production);
    selectWorkspace(production);
    renderAt("/?view=all");
    await screen.findByTestId("all-tasks-workspace");
    await waitFor(() => expect(fetchAllAgents).toHaveBeenCalled());

    await userEvent.click(await screen.findByRole("button", { name: "Server: all" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByRole("menuitem", { name: /prod/ })).toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: /This daemon/ })).toBeNull();
  });

  it("shows an empty workspace in the island with a way to manage it", async () => {
    createWorkspace("Lab");
    renderAt("/agents/local/a1/console");
    await screen.findByRole("button", { name: "Open a1" });

    await userEvent.click(screen.getByRole("button", { name: "Workspace: Default" }));
    await userEvent.click(await screen.findByRole("menuitemradio", { name: /Lab/ }));

    await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/"));
    expect(await screen.findByText("Lab has no agents yet")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Manage workspaces" }));
    const dialog = await screen.findByRole("dialog", { name: "Manage workspaces" });
    expect(within(dialog).getByLabelText("Workspace name")).toHaveValue("Lab");

    // Adding a host updates the island without a reload.
    await userEvent.click(within(dialog).getByRole("button", { name: /^Default/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Add prod to…" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Lab" }));
    expect(
      await screen.findByRole("button", { name: "Open b1", hidden: true }),
    ).toBeInTheDocument();
  });
});

describe("sidebar width", () => {
  it("starts at the persisted width", async () => {
    localStorage.setItem("terminals:sidebar:v1", JSON.stringify({ width: 380, hidden: false }));
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    expect(screen.getByTestId("agents-sidebar")).toHaveStyle({ width: "380px" });
  });

  it("drags to a new width and persists it", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    const handle = screen.getByRole("separator", { name: "resize sidebar" });

    // jsdom has no layout, so the aside's rect.left is 0 and clientX is the width.
    fireEvent.pointerDown(handle);
    fireEvent.pointerMove(window, { clientX: 400 });
    fireEvent.pointerUp(window);

    expect(screen.getByTestId("agents-sidebar")).toHaveStyle({ width: "400px" });
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 400,
      hidden: false,
    });

    // Drag ended: further pointer moves must not keep resizing.
    fireEvent.pointerMove(window, { clientX: 200 });
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 400,
      hidden: false,
    });
  });

  it("clamps a drag past the maximum", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    fireEvent.pointerDown(screen.getByRole("separator", { name: "resize sidebar" }));
    fireEvent.pointerMove(window, { clientX: 5000 });
    fireEvent.pointerUp(window);
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 640,
      hidden: false,
    });
  });

  it("resets to the default on double-click", async () => {
    localStorage.setItem("terminals:sidebar:v1", JSON.stringify({ width: 500, hidden: false }));
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    fireEvent.doubleClick(screen.getByRole("separator", { name: "resize sidebar" }));
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 268,
      hidden: false,
    });
  });

  it("resizes with arrow keys", async () => {
    localStorage.setItem("terminals:sidebar:v1", JSON.stringify({ width: 300, hidden: false }));
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    const handle = screen.getByRole("separator", { name: "resize sidebar" });
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 308,
      hidden: false,
    });
    fireEvent.keyDown(handle, { key: "ArrowLeft", shiftKey: true });
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 276,
      hidden: false,
    });
  });
});

describe("sidebar tabs", () => {
  const twoHosts = [
    {
      host: { id: "", label: "This daemon (local)" },
      groups: [{ name: "release", lead: "alpha", members: 1 }],
      agents: [
        agentRow({
          name: "alpha",
          image: "img",
          state: "running",
          harness: "claude",
          group: "release",
          interactive: true,
        }),
        agentRow({
          name: "beta",
          image: "img",
          state: "stopped",
          harness: "claude",
          interactive: true,
        }),
      ],
    },
    {
      host: { id: "prod", label: "prod" },
      agents: [
        agentRow({
          name: "gamma",
          image: "img",
          state: "running",
          harness: "claude",
          group: "release",
          interactive: true,
        }),
      ],
    },
  ];

  it("opens on Servers, the first of two tabs, with Groups hidden", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    renderAt("/");

    expect(await screen.findByRole("button", { name: "Open server prod" })).toBeInTheDocument();
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Servers", "Agents"]);
    expect(screen.getByRole("tab", { name: "Servers" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByRole("tab", { name: "Groups" })).toBeNull();
  });

  it("lists every server's agents without server sections on Agents", async () => {
    agentsTab();
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    renderAt("/");

    expect(await screen.findByRole("button", { name: "Open alpha" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open gamma" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open server prod" })).toBeNull();
  });

  it("remembers the chosen tab across mounts", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    const first = renderAt("/");
    expect(await screen.findByRole("button", { name: "Open server prod" })).toBeInTheDocument();
    pickSidebarTab("Agents");
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Open server prod" })).toBeNull(),
    );
    first.unmount();

    renderAt("/");
    expect(await screen.findByRole("button", { name: "Open gamma" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open server prod" })).toBeNull();
  });

  it("puts a called-for agent above the rest, and a pinned one above that", async () => {
    agentsTab();
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    renderAt("/", new Map([[JSON.stringify(["prod", "gamma"]), 1]]));

    const names = () => screen.getAllByTestId("sidebar-agent").map((row) => row.textContent);
    await waitFor(() => expect(names()).toHaveLength(3));
    expect(names()[0]).toMatch(/gamma/);

    fireEvent.contextMenu(screen.getByRole("button", { name: "Open beta" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Pin" }));
    await waitFor(() => expect(names()[0]).toMatch(/beta/));
    expect(names()[1]).toMatch(/gamma/);
    expect(JSON.parse(localStorage.getItem("terminals:sidebar-pinned:v1")!)).toEqual({
      version: 1,
      keys: [JSON.stringify(["", "beta"])],
    });
  });
});
