import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, fireEvent } from "@/test/queryRender";
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
import { targetFor } from "@/lib/terminalsHost";
import {
  cleanupTerminalsPage,
  setupTerminalsPage,
  renderAt,
  serversTab,
  cloneProjection,
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
const { agentGetOn } = calls;

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

describe("TerminalsPage server sections", () => {
  it("groups agents under server sections and shows host errors", async () => {
    serversTab();
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("This daemon (local)")).toBeInTheDocument());
    expect(screen.getByText("prod")).toBeInTheDocument();
    expect(screen.getByText("a1")).toBeInTheDocument();
    expect(screen.getByText("unauthorized: nope")).toBeInTheDocument();
  });

  it("separates team members from individual agents", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "This daemon (local)" },
        groups: [{ name: "empty-team", lead: "", members: 0 }],
        agents: [
          agentRow({
            name: "lead",
            image: "img",
            state: "running",
            harness: "claude",
            group: "dev-team",
            interactive: true,
          }),
          agentRow({
            name: "solo",
            image: "img",
            state: "stopped",
            harness: "codex",
            interactive: true,
          }),
        ],
      },
    ]);
    renderAt("/");
    expect(await screen.findByText("Teams")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open team empty-team" })).toBeInTheDocument();
    expect(screen.getByText("dev-team")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Open team dev-team" }));
    expect(await screen.findByRole("heading", { name: "dev-team" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open team member lead" })).toBeInTheDocument();
    expect(screen.getByText("Individual agents")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open lead" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open solo" })).toBeInTheDocument();
  });

  it.each([
    { agentName: "lead", group: "dev-team" },
    { agentName: "solo", group: null },
  ])(
    "opens Clone for the $agentName sidebar row without navigating",
    async ({ agentName, group }) => {
      vi.mocked(fetchAllAgents).mockResolvedValue([
        {
          host: { id: "", label: "This daemon (local)" },
          agents: [
            agentRow({
              name: "lead",
              image: "worker:v1",
              state: "running",
              harness: "codex",
              group: "dev-team",
              interactive: true,
            }),
            agentRow({
              name: "solo",
              image: "worker:v1",
              state: "stopped",
              harness: "codex",
              interactive: true,
            }),
          ],
        },
      ]);
      agentGetOn.mockResolvedValueOnce({ ...cloneProjection, name: agentName, group });
      renderAt("/");

      const row = await screen.findByRole("button", { name: `Open ${agentName}` });
      fireEvent.contextMenu(row);
      fireEvent.click(await screen.findByRole("menuitem", { name: "Clone" }));

      expect(await screen.findByRole("heading", { name: "Clone agent" })).toBeInTheDocument();
      expect(agentGetOn).toHaveBeenCalledWith(targetFor(""), agentName, "");
      expect(screen.getByTestId("location")).toHaveTextContent("/");
    },
  );

  it("does not navigate for a context-menu gesture", async () => {
    renderAt("/");
    const row = await screen.findByRole("button", { name: "Open a1" });

    fireEvent.pointerDown(row, { button: 2, pointerId: 12, clientX: 10, clientY: 10 });
    fireEvent.pointerMove(window, { pointerId: 12, clientX: 200, clientY: 200 });
    fireEvent.contextMenu(row);

    expect(await screen.findByRole("menuitem", { name: "Clone" })).toBeInTheDocument();
    expect(screen.getByTestId("location").textContent).toBe("/");
  });

  it("keeps the individual agents subsection visible when a host only has an empty team", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "This daemon (local)" },
        groups: [{ name: "empty-team", lead: "", members: 0 }],
        agents: [],
      },
    ]);
    renderAt("/");
    expect(await screen.findByRole("button", { name: "Open team empty-team" })).toBeInTheDocument();
    expect(screen.getByText("Individual agents")).toBeInTheDocument();
  });
});

describe("TerminalsPage agent rows", () => {
  it("marks non-interactive agents in the sidebar and leaves interactive ones unmarked", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a2")).toBeInTheDocument());
    const a1Row = screen.getByText("a1").closest("button")!;
    const a2Row = screen.getByText("a2").closest("button")!;
    expect(a2Row.textContent).toMatch(/non-tty/);
    expect(a1Row.textContent).not.toMatch(/non-tty/);
  });

  it("carries only the unread message count in an agent row, never a question dot", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "remote-1", label: "prod" },
        agents: [
          agentRow({
            name: "alice",
            image: "bare:latest",
            state: "running",
            harness: "claude",
            interactive: true,
          }),
        ],
        chats: [
          {
            id: "dm:alice",
            kind: "direct",
            title: "alice",
            agent: "alice",
            last_ts: "2026-09-15T10:00:00Z",
            last_from: "agent:alice",
            last_type: "message",
            last_text: "hi",
            unread: 3,
          },
        ],
      },
    ]);

    renderAt("/", new Map([[JSON.stringify(["remote-1", "alice"]), 1]]));

    expect(await screen.findByLabelText("3 unread messages from alice")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: /customer question/ })).toBeNull();
  });
});
