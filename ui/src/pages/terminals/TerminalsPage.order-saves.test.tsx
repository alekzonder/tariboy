import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, screen, waitFor, fireEvent } from "@/test/queryRender";
import { fetchAllAgents } from "@/lib/aggregate";
import type { CreateAgentResult, CreateAgentSpec, ImageManifest, ImageRow } from "@/lib/api";
import type { AgentLifecycleResult, AgentView } from "@/lib/types";
import { type Daemon } from "@/lib/daemons";
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

describe("TerminalsPage order rollback", () => {
  it("rolls back agent order when daemon persistence fails", async () => {
    serversTab();
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "local" },
        agents: [
          {
            name: "a1",
            image: "img",
            state: "running",
            harness: "codex",
            group: null,
            interactive: true,
          },
          {
            name: "a2",
            image: "img",
            state: "running",
            harness: "codex",
            group: null,
            interactive: true,
          },
        ],
        groups: [],
      },
    ]);
    mockRowRects({ "Open a1": 0, "Open a2": 30 });
    let rejectSave!: (cause: Error) => void;
    apiOn.mockImplementationOnce(
      () =>
        new Promise<void>((_, reject) => {
          rejectSave = reject;
        }),
    );
    renderAt("/");
    const a1 = await screen.findByRole("button", { name: "Open a1" });
    const a2 = screen.getByRole("button", { name: "Open a2" });

    await keyboardMove("Open a1", "ArrowDown");
    expect(a2.compareDocumentPosition(a1) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    rejectSave(new Error("offline"));

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not save agents order");
    expect(a1.compareDocumentPosition(a2) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

it("serializes daemon order writes so an older request cannot finish last", async () => {
  serversTab();
  vi.mocked(fetchAllAgents).mockResolvedValue([
    {
      host: { id: "", label: "local" },
      agents: [
        {
          name: "a1",
          image: "img",
          state: "running",
          harness: "codex",
          group: "a",
          interactive: true,
        },
        {
          name: "a2",
          image: "img",
          state: "running",
          harness: "codex",
          group: "a",
          interactive: true,
        },
        {
          name: "b1",
          image: "img",
          state: "running",
          harness: "codex",
          group: "b",
          interactive: true,
        },
        {
          name: "b2",
          image: "img",
          state: "running",
          harness: "codex",
          group: "b",
          interactive: true,
        },
      ],
      groups: [
        { name: "a", lead: "", members: 2 },
        { name: "b", lead: "", members: 2 },
      ],
    },
  ]);
  let finishFirst!: () => void;
  let failSecond!: (cause: Error) => void;
  apiOn
    .mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          finishFirst = resolve;
        }),
    )
    .mockImplementationOnce(
      () =>
        new Promise<void>((_, reject) => {
          failSecond = reject;
        }),
    );
  mockRowRects({
    "Open a1": 0,
    "Open a2": 30,
    "Open b1": 60,
    "Open b2": 90,
    "Open team a": 120,
    "Open team b": 150,
  });
  renderAt("/");
  const a1 = await screen.findByRole("button", { name: "Open a1" });

  fireEvent.keyDown(a1, { code: "Space" });
  await waitFor(() => expect(a1).toHaveAttribute("aria-pressed", "true"));
  fireEvent.keyDown(document, { code: "ArrowDown" });
  fireEvent.keyDown(document, { code: "Space" });
  await waitFor(() => expect(apiOn).toHaveBeenCalledTimes(1));

  const b1 = screen.getByRole("button", { name: "Open b1" });
  fireEvent.keyDown(b1, { code: "Space" });
  await waitFor(() => expect(b1).toHaveAttribute("aria-pressed", "true"));
  fireEvent.keyDown(document, { code: "ArrowDown" });
  fireEvent.keyDown(document, { code: "Space" });
  await waitFor(() => expect(b1).not.toHaveAttribute("aria-pressed"));
  await act(async () => {});

  expect(apiOn).toHaveBeenCalledTimes(1);
  finishFirst();
  await waitFor(() => expect(apiOn).toHaveBeenCalledTimes(2));
  expect(
    screen
      .getByRole("button", { name: "Open a2" })
      .compareDocumentPosition(screen.getByRole("button", { name: "Open a1" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "Open b2" })
      .compareDocumentPosition(screen.getByRole("button", { name: "Open b1" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  failSecond(new Error("offline"));
  expect(await screen.findByRole("alert")).toHaveTextContent("Could not save agents order");
  expect(
    screen
      .getByRole("button", { name: "Open a2" })
      .compareDocumentPosition(screen.getByRole("button", { name: "Open a1" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "Open b1" })
      .compareDocumentPosition(screen.getByRole("button", { name: "Open b2" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();

  let failThird!: (cause: Error) => void;
  let failFourth!: (cause: Error) => void;
  apiOn
    .mockImplementationOnce(
      () =>
        new Promise<void>((_, reject) => {
          failThird = reject;
        }),
    )
    .mockImplementationOnce(
      () =>
        new Promise<void>((_, reject) => {
          failFourth = reject;
        }),
    );
  await keyboardMove("Open b1", "ArrowDown");
  await waitFor(() => expect(apiOn).toHaveBeenCalledTimes(3));
  await keyboardMove("Open team a", "ArrowDown");
  expect(apiOn).toHaveBeenCalledTimes(3);
  failThird(new Error("offline"));
  await waitFor(() => expect(apiOn).toHaveBeenCalledTimes(4));
  failFourth(new Error("offline"));
  await waitFor(() =>
    expect(screen.getByRole("alert")).toHaveTextContent("Could not save groups order"),
  );
  expect(
    screen
      .getByRole("button", { name: "Open a2" })
      .compareDocumentPosition(screen.getByRole("button", { name: "Open a1" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "Open b1" })
      .compareDocumentPosition(screen.getByRole("button", { name: "Open b2" })) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});
