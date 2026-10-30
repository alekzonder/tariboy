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
import { cleanupTerminalsPage, setupTerminalsPage, renderAt } from "@/test/terminals/terminalsPage";
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
    expect(screen.getByText("Local · ready")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Hosts menu" }));
    await userEvent.click(await screen.findByRole("menuitemradio", { name: /Production/ }));

    // The open agent is not in Production, so its first agent opens in the same tab.
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/agents/d1/b1/activity"),
    );
    expect(await screen.findByRole("button", { name: "Open b1" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open a1" })).toBeNull();
  });

  it("lists only the workspace's hosts and offers no workspace switch with one workspace", async () => {
    const production = createWorkspace("Production")!;
    addHostToWorkspace("d1", production);
    selectWorkspace(production);
    renderAt("/");
    expect(await screen.findByRole("button", { name: "Open b1" })).toBeInTheDocument();
    expect(screen.getByText("prod · ready")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Open a1" })).toBeNull();
  });

  it("opens the workspace manager from the hosts menu", async () => {
    renderAt("/");
    await screen.findByRole("button", { name: "Open a1" });
    expect(screen.getByText("2 hosts · ready")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Hosts menu" }));
    expect(screen.queryByRole("menuitemradio")).toBeNull();
    await userEvent.click(await screen.findByRole("menuitem", { name: "Manage workspaces…" }));
    expect(await screen.findByRole("dialog", { name: "Manage workspaces" })).toBeInTheDocument();
  });

  it("shows an empty workspace in the island with a way to manage it", async () => {
    createWorkspace("Lab");
    renderAt("/agents/local/a1/console");
    await screen.findByRole("button", { name: "Open a1" });

    await userEvent.click(screen.getByRole("button", { name: "Hosts menu" }));
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
    await screen.findByRole("button", { name: "Open a1" });
    expect(screen.getByTestId("agents-sidebar")).toHaveStyle({ width: "380px" });
  });

  it("drags to a new width and persists it", async () => {
    renderAt("/terminals");
    await screen.findByRole("button", { name: "Open a1" });
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
    await screen.findByRole("button", { name: "Open a1" });
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
    await screen.findByRole("button", { name: "Open a1" });
    fireEvent.doubleClick(screen.getByRole("separator", { name: "resize sidebar" }));
    expect(JSON.parse(localStorage.getItem("terminals:sidebar:v1")!)).toMatchObject({
      width: 268,
      hidden: false,
    });
  });

  it("resizes with arrow keys", async () => {
    localStorage.setItem("terminals:sidebar:v1", JSON.stringify({ width: 300, hidden: false }));
    renderAt("/terminals");
    await screen.findByRole("button", { name: "Open a1" });
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

describe("sidebar agent list", () => {
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

  it("groups agents under host headings with two hosts, with no tabs", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    renderAt("/");

    expect(await screen.findByRole("button", { name: "Open host prod" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open gamma" })).toBeInTheDocument();
    expect(screen.queryAllByRole("tab")).toEqual([]);
  });

  it("shows one host's teams and agents without a host heading", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts.slice(0, 1));
    renderAt("/");

    expect(await screen.findByRole("button", { name: "Open alpha" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open team release" })).toBeInTheDocument();
    expect(screen.getByText("Individual agents")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Open host/ })).toBeNull();
  });

  it("opens and closes the agent list under Agents and remembers it", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    const first = renderAt("/");
    expect(await screen.findByRole("button", { name: "Open gamma" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Agents" }));
    expect(screen.getByRole("button", { name: "Agents" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.queryByRole("button", { name: "Open gamma" })).toBeNull();
    first.unmount();

    renderAt("/");
    await screen.findByRole("link", { name: "Tasks" });
    expect(screen.queryByRole("button", { name: "Open gamma" })).toBeNull();
  });

  it("puts a pinned agent on top and shows it only there", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue(twoHosts);
    renderAt("/");

    const names = () => screen.getAllByTestId("sidebar-agent").map((row) => row.textContent);
    await waitFor(() => expect(names()).toHaveLength(3));
    expect(names()[0]).toMatch(/alpha/);

    fireEvent.contextMenu(screen.getByRole("button", { name: "Open beta" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Pin" }));
    await waitFor(() => expect(names()[0]).toMatch(/beta/));
    expect(names().filter((name) => /beta/.test(name ?? ""))).toHaveLength(1);
    expect(JSON.parse(localStorage.getItem("terminals:sidebar-pinned:v1")!)).toEqual({
      version: 1,
      keys: [JSON.stringify(["", "beta"])],
    });
  });
});
