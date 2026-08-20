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

    expect(screen.getByTestId("send-files")).toHaveAttribute("data-agent-name", "worker");
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
    const upload = vi.spyOn(api, "agentUploadFile").mockResolvedValue({
      path: "staged.txt", abs: "/cwd/staged.txt", bytes: 1,
    });
    const success = vi.spyOn(toast, "success");
    renderTab(agent({ interactive: false }));

    fireEvent.drop(screen.getByTestId("agent-console-absent-drop-target"), {
      dataTransfer: { files: [new File(["staged"], "staged.txt")] },
    });

    await waitFor(() => expect(upload).toHaveBeenCalledWith("worker", expect.any(File), target));
    expect(success).toHaveBeenCalledWith("uploaded: /cwd/staged.txt");
  });
});

describe("AgentConsoleTab manual Exec", () => {
  it.each([
    ["interactive", agent({ interactive: true, state: "running" })],
    ["non-interactive", agent({ interactive: false })],
  ])("renders the composer for a stopped or running %s agent", (_label, value) => {
    renderTab(value);
    expect(screen.getByPlaceholderText("one-shot exec prompt (optional)")).toBeEnabled();
    expect(screen.getByRole("button", { name: "Exec" })).toBeEnabled();
  });

  it("sends a one-shot prompt to the route-selected host and reconnects an interactive terminal", async () => {
    vi.mocked(agentPostOn).mockResolvedValue({});
    const refresh = renderTab(agent({ interactive: true, state: "running" }));
    const input = screen.getByPlaceholderText("one-shot exec prompt (optional)");

    fireEvent.change(input, { target: { value: "continue this task" } });
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));

    await waitFor(() => expect(agentPostOn).toHaveBeenCalledWith(
      target, "worker", "exec", { prompt: "continue this task" },
    ));
    await waitFor(() => expect(input).toHaveValue(""));
    expect(refresh).toHaveBeenCalledOnce();
    expect(reconnect).toHaveBeenCalledOnce();
  });

  it("omits an empty prompt and prevents duplicate requests while Exec is pending", async () => {
    let resolveRequest!: (value: unknown) => void;
    vi.mocked(agentPostOn).mockImplementation(() => new Promise((resolve) => { resolveRequest = resolve; }));
    renderTab();
    const input = screen.getByPlaceholderText("one-shot exec prompt (optional)");
    const button = screen.getByRole("button", { name: "Exec" });

    fireEvent.click(button);
    fireEvent.click(button);

    expect(agentPostOn).toHaveBeenCalledOnce();
    expect(agentPostOn).toHaveBeenCalledWith(target, "worker", "exec", undefined);
    expect(input).toBeDisabled();
    expect(button).toBeDisabled();
    resolveRequest({});
    await waitFor(() => expect(button).toBeEnabled());
    expect(reconnect).not.toHaveBeenCalled();
  });

  it("retains the prompt and restores controls after an API failure", async () => {
    vi.mocked(agentPostOn).mockRejectedValue(new Error("iteration already running"));
    renderTab();
    const input = screen.getByPlaceholderText("one-shot exec prompt (optional)");
    const button = screen.getByRole("button", { name: "Exec" });

    fireEvent.change(input, { target: { value: "retry me" } });
    fireEvent.click(button);

    await waitFor(() => expect(agentPostOn).toHaveBeenCalledOnce());
    await waitFor(() => expect(button).toBeEnabled());
    expect(input).toHaveValue("retry me");
  });
});

describe("AgentConsoleTab delete confirmation", () => {
  it("opens an in-app confirmation and cancels without deleting", async () => {
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Delete agent worker?" })).toBeInTheDocument();
    expect(agentDeleteOn).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(agentDeleteOn).not.toHaveBeenCalled();
  });

  it("deletes once with force and purge, then navigates home", async () => {
    vi.mocked(agentDeleteOn).mockResolvedValue({});
    const refresh = renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    await waitFor(() => expect(agentDeleteOn).toHaveBeenCalledOnce());
    expect(agentDeleteOn).toHaveBeenCalledWith(target, "worker", { force: true, purge: true });
    await waitFor(() => expect(navigate).toHaveBeenCalledWith("/"));
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("disables dismissal and duplicate submission while deletion is pending", async () => {
    let resolveDelete!: (value: unknown) => void;
    vi.mocked(agentDeleteOn).mockImplementation(() => new Promise((resolve) => { resolveDelete = resolve; }));
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    const pending = screen.getByRole("button", { name: "Deleting…" });
    expect(pending).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(agentDeleteOn).toHaveBeenCalledOnce();

    resolveDelete({});
    await waitFor(() => expect(navigate).toHaveBeenCalledWith("/"));
  });

  it("keeps the dialog open and does not navigate after a deletion failure", async () => {
    vi.mocked(agentDeleteOn).mockRejectedValue(new Error("delete failed"));
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    await waitFor(() => expect(agentDeleteOn).toHaveBeenCalledOnce());
    await waitFor(() => expect(screen.getByRole("button", { name: "Delete agent" })).toBeEnabled());
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(navigate).not.toHaveBeenCalled();
  });
});
