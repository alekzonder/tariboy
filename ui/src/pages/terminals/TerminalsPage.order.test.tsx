import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@/test/queryRender";
import { fetchAllAgents } from "@/lib/aggregate";
import type { CreateAgentResult, CreateAgentSpec, ImageManifest, ImageRow } from "@/lib/api";
import type { AgentLifecycleResult, AgentView } from "@/lib/types";
import { type Daemon } from "@/lib/daemons";
import { createWorkspace, moveHost } from "@/lib/workspaces";
import {
  cleanupTerminalsPage,
  setupTerminalsPage,
  renderAt,
  mockRowRects,
  keyboardMove,
  serversTab,
} from "@/test/terminals/terminalsPage";

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
  createAgent:
    vi.fn<(spec: CreateAgentSpec, target: Daemon | null) => Promise<CreateAgentResult>>(),
  imageManifestGet: vi.fn<(ref: string, target: Daemon | null) => Promise<ImageManifest>>(),
  listImages: vi.fn<(target: Daemon | null) => Promise<{ images: ImageRow[]; count: number }>>(),
  startAgent: vi.fn<(name: string, target: Daemon | null) => Promise<AgentLifecycleResult>>(),
}));
const { apiOn } = calls;

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

describe("TerminalsPage restored order", () => {
  it("restores server, team, and agent order", async () => {
    serversTab();
    localStorage.setItem(
      "terminals:server-order:v1",
      JSON.stringify({
        version: 1,
        ids: ["d1", ""],
      }),
    );
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "local" },
        groups: [
          { name: "alpha", lead: "", members: 1 },
          { name: "beta", lead: "", members: 1 },
        ],
        agents: [
          {
            name: "a1",
            image: "img",
            state: "running",
            harness: "codex",
            group: "alpha",
            interactive: true,
          },
          {
            name: "b1",
            image: "img",
            state: "running",
            harness: "codex",
            group: "beta",
            interactive: true,
          },
          {
            name: "solo1",
            image: "img",
            state: "running",
            harness: "codex",
            group: null,
            interactive: true,
          },
          {
            name: "solo2",
            image: "img",
            state: "running",
            harness: "codex",
            group: null,
            interactive: true,
          },
        ],
        sidebarOrder: { version: 1, groups: ["beta", "alpha"], agents: ["solo2", "solo1"] },
      },
      { host: { id: "d1", label: "prod" }, agents: [], groups: [] },
    ] as unknown as Awaited<ReturnType<typeof fetchAllAgents>>);

    renderAt("/");

    const prod = await screen.findByRole("button", { name: "Open server prod" });
    const local = screen.getByRole("button", { name: "Open server local" });
    expect(prod.compareDocumentPosition(local) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const beta = screen.getByRole("button", { name: "Open team beta" });
    const alpha = screen.getByRole("button", { name: "Open team alpha" });
    expect(beta.compareDocumentPosition(alpha) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const solo2 = screen.getByRole("button", { name: "Open solo2" });
    const solo1 = screen.getByRole("button", { name: "Open solo1" });
    expect(solo2.compareDocumentPosition(solo1) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("uses the whole server row as an accessible drag surface", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "local" }, agents: [], groups: [] },
      { host: { id: "d1", label: "prod" }, agents: [], groups: [] },
    ]);
    renderAt("/");

    const local = await screen.findByRole("button", { name: "Open server local" });
    expect(local).toHaveAttribute("aria-roledescription", "draggable");
    expect(screen.queryByRole("button", { name: /Move server/ })).toBeNull();
  });
});

describe("TerminalsPage keyboard reorder", () => {
  it("persists server order after a keyboard drag", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "local" }, agents: [], groups: [] },
      { host: { id: "d1", label: "prod" }, agents: [], groups: [] },
    ]);
    mockRowRects({ "Open server local": 0, "Open server prod": 30 });
    renderAt("/");
    await screen.findByRole("button", { name: "Open server local" });

    await keyboardMove("Open server local", "ArrowDown");

    expect(JSON.parse(localStorage.getItem("terminals:server-order:v1")!)).toEqual({
      version: 1,
      ids: ["d1", ""],
    });
  });

  it("keeps the saved position of hosts in other workspaces when reordering servers", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "local" }, agents: [], groups: [] },
      { host: { id: "d1", label: "prod" }, agents: [], groups: [] },
      { host: { id: "d2", label: "lab" }, agents: [], groups: [] },
    ]);
    localStorage.setItem(
      "terminals:server-order:v1",
      JSON.stringify({ version: 1, ids: ["", "d2", "d1"] }),
    );
    moveHost("d2", createWorkspace("Lab")!);
    mockRowRects({ "Open server local": 0, "Open server prod": 30 });
    renderAt("/");
    await screen.findByRole("button", { name: "Open server local" });

    await keyboardMove("Open server local", "ArrowDown");

    expect(JSON.parse(localStorage.getItem("terminals:server-order:v1")!)).toEqual({
      version: 1,
      ids: ["d1", "d2", ""],
    });
  });

  it("persists team order after a keyboard drag", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "local" },
        agents: [],
        groups: [
          { name: "a", lead: "", members: 0 },
          { name: "b", lead: "", members: 0 },
        ],
      },
    ]);
    mockRowRects({ "Open team a": 0, "Open team b": 30 });
    apiOn.mockResolvedValueOnce(undefined);
    renderAt("/");
    await screen.findByRole("button", { name: "Open team a" });

    await keyboardMove("Open team a", "ArrowDown");

    await waitFor(() =>
      expect(apiOn).toHaveBeenCalledWith(null, "POST", "/api/daemon/config", {
        key: "sidebar_order_v1",
        value: JSON.stringify({ version: 1, groups: ["b", "a"], agents: [] }),
      }),
    );
  });
});
