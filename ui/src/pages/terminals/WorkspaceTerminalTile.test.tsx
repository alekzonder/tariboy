import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { useTerminalSocket } from "@/hooks/useTerminalSocket";
import { agentPostOn } from "@/lib/api";
import type { AgentSummary } from "@/lib/types";
import { WorkspaceTerminalTile } from "./WorkspaceTerminalTile";

const localTarget = null;
const remoteTarget = {
  id: "remote-a",
  label: "Prod",
  baseURL: "https://127.0.0.1:18000",
  token: "redacted",
};
const reconnect = vi.fn();

vi.mock("@/lib/terminalsHost", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/terminalsHost")>();
  return {
    ...actual,
    targetFor: vi.fn((hostId: string) => hostId === "" ? localTarget : remoteTarget),
  };
});
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, agentPostOn: vi.fn() };
});
vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn((name: string) => ({
    status: "open",
    absent: false,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name,
    reconnect,
  })),
}));
vi.mock("@/components/TuiScreen", () => ({
  TuiScreen: ({
    controller,
    persistDraft,
    surface,
  }: {
    controller: { name: string };
    persistDraft?: boolean;
    surface?: string;
  }) => (
    <div
      data-testid={`terminal-${controller.name}`}
      data-persist-draft={String(persistDraft)}
      data-surface={surface}
    />
  ),
}));

function agent(overrides: Partial<AgentSummary> = {}): AgentSummary {
  return {
    name: "worker",
    image: "bare:latest",
    state: "running",
    harness: "codex",
    loop_enabled: false,
    group: null,
    interactive: true,
    ...overrides,
  };
}

function renderTile(overrides: Partial<React.ComponentProps<typeof WorkspaceTerminalTile>> = {}) {
  const props: React.ComponentProps<typeof WorkspaceTerminalTile> = {
    identity: { hostId: "remote-a", agentName: "worker" },
    hostLabel: "Prod",
    agent: agent(),
    selected: false,
    onFocus: vi.fn(),
    onRetry: vi.fn(),
    onReplace: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  };
  render(<MemoryRouter><WorkspaceTerminalTile {...props} /></MemoryRouter>);
  return props;
}

function ConfigurationHistoryHarness() {
  const navigate = useNavigate();
  const location = useLocation();
  return (
    <>
      <output data-testid="location">{location.pathname}</output>
      <WorkspaceTerminalTile
        identity={{ hostId: "remote-a", agentName: "worker" }}
        hostLabel="Prod"
        agent={agent({ interactive: false })}
        selected={false}
        onFocus={vi.fn()}
        onRetry={vi.fn()}
        onReplace={vi.fn()}
        onClose={vi.fn()}
        onOpenConfiguration={() =>
          navigate("/agents/remote-a/worker/configuration")}
      />
      <button type="button" onClick={() => navigate(-1)}>Back</button>
    </>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(agentPostOn).mockResolvedValue({});
});

describe("WorkspaceTerminalTile", () => {
  it("opens independent terminal sockets against each tile's explicit host target", () => {
    const local = agent({ name: "local-agent" });
    const remote = agent({ name: "remote-agent" });
    render(
      <MemoryRouter>
        <WorkspaceTerminalTile
          identity={{ hostId: "", agentName: local.name }}
          hostLabel="Local"
          agent={local}
          selected={false}
          onFocus={vi.fn()}
          onRetry={vi.fn()}
          onReplace={vi.fn()}
          onClose={vi.fn()}
        />
        <WorkspaceTerminalTile
          identity={{ hostId: "remote-a", agentName: remote.name }}
          hostLabel="Prod"
          agent={remote}
          selected
          onFocus={vi.fn()}
          onRetry={vi.fn()}
          onReplace={vi.fn()}
          onClose={vi.fn()}
        />
      </MemoryRouter>,
    );

    expect(useTerminalSocket).toHaveBeenNthCalledWith(1, "local-agent", true, localTarget);
    expect(useTerminalSocket).toHaveBeenNthCalledWith(2, "remote-agent", true, remoteTarget);
    expect(screen.getByTestId("terminal-local-agent")).toBeInTheDocument();
    expect(screen.getByTestId("terminal-remote-agent")).toBeInTheDocument();
    expect(screen.getByTestId("terminal-local-agent")).toHaveAttribute(
      "data-persist-draft",
      "false",
    );
    expect(screen.getByTestId("terminal-remote-agent")).toHaveAttribute(
      "data-persist-draft",
      "false",
    );
    expect(screen.getByTestId("terminal-local-agent")).toHaveAttribute(
      "data-surface",
      "workspace",
    );
    expect(screen.getByTestId("terminal-remote-agent")).toHaveAttribute(
      "data-surface",
      "workspace",
    );
  });

  it("starts a stopped agent on its explicit host and reconnects the same tile", async () => {
    const onRetry = vi.fn();
    renderTile({ agent: agent({ state: "stopped" }), onRetry });

    fireEvent.click(screen.getByRole("button", { name: "Start worker" }));

    await waitFor(() =>
      expect(agentPostOn).toHaveBeenCalledWith(remoteTarget, "worker", "start"),
    );
    expect(onRetry).toHaveBeenCalledOnce();
    expect(reconnect).toHaveBeenCalledOnce();
    expect(useTerminalSocket).toHaveBeenCalledWith("worker", false, remoteTarget);
  });

  it("retains an unavailable identity with retry, replace, and close actions", () => {
    const props = renderTile({ agent: undefined });

    fireEvent.click(screen.getByRole("button", { name: "Retry worker" }));
    fireEvent.click(screen.getByRole("button", { name: "Replace worker" }));
    fireEvent.click(screen.getByRole("button", { name: "Close worker terminal" }));

    expect(props.onRetry).toHaveBeenCalledOnce();
    expect(props.onReplace).toHaveBeenCalledOnce();
    expect(props.onClose).toHaveBeenCalledOnce();
    expect(agentPostOn).not.toHaveBeenCalled();
  });

  it("links a restored non-interactive identity to Configuration", () => {
    const onOpenConfiguration = vi.fn();
    renderTile({ agent: agent({ interactive: false }), onOpenConfiguration });

    expect(screen.getByText("This agent has no interactive terminal.")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Open Configuration" });
    expect(link).toHaveAttribute(
      "href",
      "/agents/remote-a/worker/configuration",
    );
    fireEvent.click(link);
    expect(onOpenConfiguration).toHaveBeenCalledOnce();
  });

  it("adds only one history entry when Configuration is opened", async () => {
    render(
      <MemoryRouter initialEntries={["/origin"]}>
        <ConfigurationHistoryHarness />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("link", { name: "Open Configuration" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/agents/remote-a/worker/configuration",
    );
    fireEvent.click(screen.getByRole("button", { name: "Back" }));

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/origin"),
    );
  });

  it("closes only the tile without any agent lifecycle request", () => {
    const props = renderTile();

    fireEvent.click(screen.getByRole("button", { name: "Close worker terminal" }));

    expect(props.onClose).toHaveBeenCalledOnce();
    expect(agentPostOn).not.toHaveBeenCalled();
  });
});
