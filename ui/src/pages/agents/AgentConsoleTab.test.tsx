import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { ApiProvider } from "@/components/ApiProvider";
import { useApi } from "@/components/apiContext";
import type { AgentRow, ApiClient } from "@/lib/api";
import { fakeApi } from "@/test/api";
import { agentRow } from "@/test/fixtures/apiFixtures";
import AgentConsoleTab from "./AgentConsoleTab";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";

const reconnect = vi.fn();
const socket = vi.hoisted(() => ({ status: "open", absent: false }));
const navigate = vi.fn();

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => navigate };
});

vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: socket.status,
    absent: socket.absent,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name: "worker",
    reconnect,
  })),
}));
vi.mock("@/components/TuiScreen", () => ({ TuiScreen: () => <div data-testid="tui-screen" /> }));
vi.mock("@/components/SendFilesButton", () => ({
  SendFilesButton: function SendFiles({ name }: { name: string }) {
    // The button uploads to the server of the surrounding ApiProvider.
    const daemon = useApi().target;
    return (
      <button data-testid="send-files" data-agent-name={name} data-daemon-id={daemon?.id}>
        Send files
      </button>
    );
  },
}));

const target = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" };
const serverUploadFile = vi.fn();
const startAgent = vi.fn();
let client: ApiClient;

function agent(overrides: Partial<AgentRow> = {}): AgentRow {
  return agentRow({ name: "worker", image: "basic:latest", harness: "stub", ...overrides });
}

function renderTab(value = agent()) {
  const refresh = vi.fn();
  render(
    <MemoryRouter>
      <ApiProvider client={client}>
        <AgentConsoleTab hostId="remote" agent={value} refresh={refresh} />
      </ApiProvider>
    </MemoryRouter>,
  );
  return refresh;
}

beforeEach(() => {
  serverUploadFile.mockReset();
  startAgent.mockReset();
  // The agent workspace is the boundary that names the route host.
  client = fakeApi({ files: { serverUploadFile }, agents: { startAgent } }, target);
  navigate.mockReset();
  reconnect.mockReset();
  socket.status = "open";
  socket.absent = false;
});

afterEach(() => vi.restoreAllMocks());

describe("AgentConsoleTab non-interactive uploads", () => {
  it("keeps the non-interactive panel reachable while offering Send files for its route-selected agent", () => {
    renderTab(agent({ interactive: false }));

    expect(screen.getByTestId("send-files")).toHaveAttribute("data-daemon-id", "remote");
    expect(screen.getByRole("status")).toHaveTextContent(
      "No interactive terminal. Enable it in Settings → Autopilot.",
    );
    expect(screen.getByRole("link", { name: "Settings → Autopilot" })).toHaveAttribute(
      "href",
      "/agents/remote/worker/settings",
    );
  });

  it("continues to mount TuiScreen, not the non-interactive panel, for a live interactive agent", () => {
    renderTab(agent({ interactive: true, state: "running" }));

    expect(screen.getByTestId("tui-screen")).toBeInTheDocument();
    expect(screen.queryByText(/No interactive terminal/)).not.toBeInTheDocument();
    expect(screen.queryByTestId("send-files")).not.toBeInTheDocument();
  });

  it("case 6: drops on the non-interactive panel, uploads the files, and toasts the paths", async () => {
    const upload = serverUploadFile.mockResolvedValue({
      path: "staged.txt",
      abs: "/cwd/staged.txt",
      bytes: 1,
    });
    const success = vi.spyOn(toast, "success");
    renderTab(agent({ interactive: false }));

    fireEvent.drop(screen.getByTestId("agent-console-absent-drop-target"), {
      dataTransfer: { files: [new File(["staged"], "staged.txt")] },
    });

    await waitFor(() => expect(upload).toHaveBeenCalledWith(expect.any(File)));
    expect(client.target).toEqual(target);
    expect(success).toHaveBeenCalledWith("uploaded: /cwd/staged.txt");
  });
});

it("checks for a surviving terminal even when the agent is stopped", () => {
  renderTab(agent({ interactive: true, state: "stopped", enabled: false }));

  expect(useTerminalSocket).toHaveBeenLastCalledWith("worker", true, undefined, false);
  expect(screen.getByTestId("tui-screen")).toBeInTheDocument();
  expect(startAgent).not.toHaveBeenCalled();
});

describe("AgentConsoleTab and header Exec", () => {
  it("no longer renders its own Exec composer", () => {
    renderTab(agent({ interactive: true, state: "running" }));
    expect(screen.queryByRole("button", { name: "Exec" })).not.toBeInTheDocument();
    expect(
      screen.queryByPlaceholderText("one-shot exec prompt (optional)"),
    ).not.toBeInTheDocument();
  });

  it("reconnects an interactive terminal when the header reports a started Exec", () => {
    const value = agent({ interactive: true, state: "running" });
    const { rerender } = render(
      <MemoryRouter>
        <ApiProvider client={client}>
          <AgentConsoleTab hostId="remote" agent={value} refresh={vi.fn()} execCount={2} />
        </ApiProvider>
      </MemoryRouter>,
    );
    expect(reconnect).not.toHaveBeenCalled();

    rerender(
      <MemoryRouter>
        <ApiProvider client={client}>
          <AgentConsoleTab hostId="remote" agent={value} refresh={vi.fn()} execCount={3} />
        </ApiProvider>
      </MemoryRouter>,
    );
    expect(reconnect).toHaveBeenCalledOnce();
  });
});

describe("AgentConsoleTab leaves work status in the header", () => {
  const live = { interactive: true, enabled: true, state: "running", cwd: "~/code/my-app" };

  it("does not claim that opening a socket starts the coding tool", () => {
    socket.status = "connecting";
    renderTab(agent({ ...live, harness: "claude" }));

    expect(screen.queryByText(/Starting/)).not.toBeInTheDocument();
  });

  it("does not claim startup after confirmed absence", () => {
    socket.status = "closed";
    socket.absent = true;
    renderTab(agent({ ...live, harness: "codex" }));

    expect(screen.queryByText(/Starting/)).not.toBeInTheDocument();
  });

  it("does not claim startup for a plain terminal either", () => {
    socket.status = "connecting";
    renderTab(agent({ ...live, image: "bare:latest", harness: "claude" }));

    expect(screen.queryByText(/Starting/)).not.toBeInTheDocument();
  });

  it("does not repeat Working on below the tabs", () => {
    renderTab(agent({ ...live, harness: "claude", current_goal_task_key: "DEV-1" }));

    expect(screen.queryByText(/Working on/)).not.toBeInTheDocument();
  });

  it("does not hide an idle agent's surviving terminal", () => {
    renderTab(agent({ ...live, state: "idle", harness: "claude" }));

    expect(screen.queryByText(/Idle\./)).not.toBeInTheDocument();
    expect(useTerminalSocket).toHaveBeenLastCalledWith("worker", true, undefined, false);
  });

  it("does not turn the header's halt reason into a terminal start failure", () => {
    socket.status = "connecting";
    startAgent.mockResolvedValue({ name: "worker", action: "start" });
    const refresh = renderTab(agent({ ...live, harness: "claude", halt_reason: "auth expired" }));

    expect(screen.queryByText(/did not start/)).not.toBeInTheDocument();
    expect(startAgent).not.toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
  });

  it("does not repeat the header's error reason for a stopped agent", () => {
    socket.status = "closed";
    renderTab(
      agent({
        ...live,
        enabled: false,
        state: "stopped",
        harness: "opencode",
        error_reason: "boom",
      }),
    );

    expect(screen.queryByText(/did not start/)).not.toBeInTheDocument();
  });
});

it("keeps an idle_limit halt out of the error banner", () => {
  renderTab(
    agent({
      interactive: true,
      enabled: false,
      state: "stopped",
      halt_kind: "idle_limit",
      halt_reason: "idle_limit: 3 idle runs",
    }),
  );

  expect(screen.queryByText("Off.")).not.toBeInTheDocument();
  expect(screen.queryByText(/did not start/)).not.toBeInTheDocument();
});
