import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { agentDeleteOn, agentPostOn } from "@/lib/api";
import * as api from "@/lib/api";
import { toast } from "sonner";
import { resolveDaemon } from "@/lib/daemons";
import type { AgentSummary } from "@/lib/types";
import AgentConsoleTab from "./AgentConsoleTab";

const reconnect = vi.fn();
const navigate = vi.fn();

vi.mock("react-router-dom", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router-dom")>();
  return { ...actual, useNavigate: () => navigate };
});

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, agentPostOn: vi.fn(), agentDeleteOn: vi.fn() };
});
vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn(() => ({
    status: "open", absent: false, send: vi.fn(), sendResize: vi.fn(),
    attachTerm: vi.fn(), name: "worker", reconnect,
  })),
}));
vi.mock("@/components/TuiScreen", () => ({ TuiScreen: () => <div data-testid="tui-screen" /> }));
vi.mock("@/components/SendFilesButton", () => ({
  SendFilesButton: ({ name, daemon }: { name: string; daemon?: { id: string } | null }) => (
    <button data-testid="send-files" data-agent-name={name} data-daemon-id={daemon?.id}>Send files</button>
  ),
}));

const target = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" };

function agent(overrides: Partial<AgentSummary> = {}): AgentSummary {
  return {
    name: "worker", image: "basic:latest", state: "stopped", harness: "stub",
    loop_enabled: false, group: null, interactive: false, ...overrides,
  };
}

function renderTab(value = agent()) {
  const refresh = vi.fn();
  render(
    <MemoryRouter>
      <AgentConsoleTab hostId="remote" agent={value} refresh={refresh} />
    </MemoryRouter>,
  );
  return refresh;
}

beforeEach(async () => {
  localStorage.clear();
  sessionStorage.clear();
  localStorage.setItem("tariboy_daemons", JSON.stringify([
    { id: target.id, label: target.label, baseURL: target.baseURL },
  ]));
  sessionStorage.setItem(`tariboy_daemon_token_${target.id}`, target.token);
  await resolveDaemon(target.id);
  vi.mocked(agentDeleteOn).mockReset();
  vi.mocked(agentPostOn).mockReset();
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
      "href", "/agents/remote/worker/configuration",
    );
  });

  it("continues to mount TuiScreen, not the non-interactive panel, for a live interactive agent", () => {
    renderTab(agent({ interactive: true, state: "running" }));

    expect(screen.getByTestId("tui-screen")).toBeInTheDocument();
    expect(screen.queryByText("This agent has no interactive terminal.")).not.toBeInTheDocument();
    expect(screen.queryByTestId("send-files")).not.toBeInTheDocument();
  });

  it("case 6: drops on the non-interactive panel, uploads the files, and toasts the paths", async () => {
    const upload = vi.spyOn(api, "serverUploadFile").mockResolvedValue({
      path: "staged.txt", abs: "/cwd/staged.txt", bytes: 1,
    });
    const success = vi.spyOn(toast, "success");
    renderTab(agent({ interactive: false }));

    fireEvent.drop(screen.getByTestId("agent-console-absent-drop-target"), {
      dataTransfer: { files: [new File(["staged"], "staged.txt")] },
    });

    await waitFor(() => expect(upload).toHaveBeenCalledWith(expect.any(File), target));
    expect(success).toHaveBeenCalledWith("uploaded: /cwd/staged.txt");
  });
});

describe("AgentConsoleTab and header Exec", () => {
  it("no longer renders its own Exec composer", () => {
    renderTab(agent({ interactive: true, state: "running" }));
    expect(screen.queryByRole("button", { name: "Exec" })).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("one-shot exec prompt (optional)")).not.toBeInTheDocument();
  });

  it("reconnects an interactive terminal when the header reports a started Exec", () => {
    const value = agent({ interactive: true, state: "running" });
    const { rerender } = render(
      <MemoryRouter><AgentConsoleTab hostId="remote" agent={value} refresh={vi.fn()} execCount={2} /></MemoryRouter>,
    );
    expect(reconnect).not.toHaveBeenCalled();

    rerender(
      <MemoryRouter><AgentConsoleTab hostId="remote" agent={value} refresh={vi.fn()} execCount={3} /></MemoryRouter>,
    );
    expect(reconnect).toHaveBeenCalledOnce();
  });
});
