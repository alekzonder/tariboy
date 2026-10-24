import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { ApiProvider } from "@/components/ApiProvider";
import { useApi } from "@/components/apiContext";
import type { ApiClient } from "@/lib/api";
import { fakeApi } from "@/test/api";
import type { AgentSummary } from "@/lib/types";
import AgentConsoleTab from "./AgentConsoleTab";

const reconnect = vi.fn();
const navigate = vi.fn();

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => navigate };
});

vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: "open",
    absent: false,
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

function agent(overrides: Partial<AgentSummary> = {}): AgentSummary {
  return {
    name: "worker",
    image: "basic:latest",
    state: "stopped",
    harness: "stub",
    group: null,
    interactive: false,
    ...overrides,
  };
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
});

afterEach(() => vi.restoreAllMocks());

describe("AgentConsoleTab non-interactive uploads", () => {
  it("keeps the non-interactive panel reachable while offering Send files for its route-selected agent", () => {
    renderTab(agent({ interactive: false }));

    expect(screen.getByTestId("send-files")).toHaveAttribute("data-daemon-id", "remote");
    expect(screen.getByText("This agent has no interactive terminal.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Configuration" })).toHaveAttribute(
      "href",
      "/agents/remote/worker/configuration",
    );
  });

  it("continues to mount TuiScreen, not the non-interactive panel, for a live interactive agent", () => {
    renderTab(agent({ interactive: true, state: "running" }));

    expect(screen.getByTestId("tui-screen")).toBeInTheDocument();
    expect(screen.queryByText("This agent has no interactive terminal.")).not.toBeInTheDocument();
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

it("leaves starting a stopped interactive agent to the header", () => {
  renderTab(agent({ interactive: true, state: "stopped", enabled: false }));

  expect(screen.getByText("Agent is stopped.")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Start" })).not.toBeInTheDocument();
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
