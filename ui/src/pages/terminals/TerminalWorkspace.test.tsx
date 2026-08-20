import { act, fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { agentPostOn } from "@/lib/api";
import type { HostAgents } from "@/lib/aggregate";
import type { AgentSummary } from "@/lib/types";
import {
  TerminalWorkspace,
  type TerminalWorkspaceHandle,
} from "./TerminalWorkspace";
import {
  addTerminalToModel,
  createWorkspaceModel,
  moveTerminalInModel,
} from "./workspaceModel";
import {
  WORKSPACE_STATE_KEY,
  terminalKey,
  type TerminalIdentity,
} from "./workspaceState";

// Only moveTerminalInModel is spied on: the rollback branch needs a move that is
// rejected *after* it mutated, and flexlayout performs the real ones faithfully.
vi.mock("./workspaceModel", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./workspaceModel")>();
  return { ...actual, moveTerminalInModel: vi.fn(actual.moveTerminalInModel) };
});

// FlexLayout's real view deliberately waits for measured browser geometry
// before mounting tab contents. jsdom has zero-sized boxes, so keep the real
// Model/Actions policy and replace only the visual renderer with a deterministic
// renderer that calls the application's factory and tab-header customization.
vi.mock("flexlayout-react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("flexlayout-react")>();
  const ReactModule = await import("react");
  const Layout = ReactModule.forwardRef<unknown, {
    model: import("flexlayout-react").Model;
    factory: (node: import("flexlayout-react").TabNode) => React.ReactNode;
    onRenderTab?: (
      node: import("flexlayout-react").TabNode,
      values: import("flexlayout-react").ITabRenderValues,
    ) => void;
  }>(function TestLayout({ model, factory, onRenderTab }) {
    const tabs: import("flexlayout-react").TabNode[] = [];
    model.visitNodes((node) => {
      if (node instanceof actual.TabNode) tabs.push(node);
    });
    return (
      <div data-testid="layout">
        {tabs.map((node) => {
          const values: import("flexlayout-react").ITabRenderValues = {
            leading: null,
            content: node.getName(),
            buttons: [],
          };
          onRenderTab?.(node, values);
          return (
            <div
              key={node.getId()}
              data-workspace-node-id={node.getId()}
              data-testid={`workspace-pane-${node.getName()}`}
            >
              <div>{values.content}{values.buttons}</div>
              {factory(node)}
            </div>
          );
        })}
      </div>
    );
  });
  return { ...actual, Layout };
});

vi.mock("@/hooks/useTerminalSocket", () => ({
  useTerminalSocket: vi.fn((name: string) => ({
    status: "open",
    absent: false,
    send: vi.fn(),
    sendResize: vi.fn(),
    attachTerm: vi.fn(),
    name,
    reconnect: vi.fn(),
  })),
}));
vi.mock("@/components/TuiScreen", () => ({
  TuiScreen: ({ controller }: { controller: { name: string } }) => (
    <div data-testid={`terminal-${controller.name}`} />
  ),
}));
vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, agentPostOn: vi.fn() };
});

function agent(name: string, overrides: Partial<AgentSummary> = {}): AgentSummary {
  return {
    name,
    image: "bare:latest",
    state: "running",
    harness: "codex",
    loop_enabled: false,
    group: null,
    interactive: true,
    ...overrides,
  };
}

const hosts: HostAgents[] = [
  {
    host: { id: "", label: "Local" },
    agents: [agent("alpha"), agent("non-tty", { interactive: false })],
  },
  {
    host: { id: "remote-a", label: "Prod" },
    agents: [agent("beta")],
  },
];

function renderWorkspace(value: HostAgents[] = hosts) {
  const ref = createRef<TerminalWorkspaceHandle>();
  const refresh = vi.fn();
  const onOpenConfiguration = vi.fn();
  render(
    <MemoryRouter>
      <div style={{ width: 1000, height: 700 }}>
        <TerminalWorkspace
          ref={ref}
          hosts={value}
          refresh={refresh}
          onOpenConfiguration={onOpenConfiguration}
        />
      </div>
    </MemoryRouter>,
  );
  return { ref, refresh, onOpenConfiguration };
}

function add(ref: React.RefObject<TerminalWorkspaceHandle | null>, identity: TerminalIdentity) {
  act(() => {
    ref.current?.addOrFocus(identity);
  });
}

function pointerDown(
  clientX: number,
  clientY: number,
  target?: HTMLElement,
): PointerEvent {
  return {
    button: 0,
    pointerId: 7,
    clientX,
    clientY,
    target,
    preventDefault: vi.fn(),
  } as unknown as PointerEvent;
}

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("TerminalWorkspace", () => {
  it("adds one terminal and focuses instead of duplicating it", () => {
    const { ref } = renderWorkspace();
    expect(screen.getByText("Drag an interactive agent here")).toBeInTheDocument();

    add(ref, { hostId: "", agentName: "alpha" });
    add(ref, { hostId: "", agentName: "alpha" });

    expect(screen.getAllByTestId("terminal-alpha")).toHaveLength(1);
    expect(screen.queryByText("Drag an interactive agent here")).toBeNull();
  });

  it("uses a real pointer session to preview and add at a pane edge", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });
    const pane = screen.getByTestId("workspace-pane-alpha");
    vi.spyOn(pane, "getBoundingClientRect").mockReturnValue({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [pane]),
    });

    act(() => {
      ref.current?.beginExternalPointerDrag(
        { hostId: "remote-a", agentName: "beta" },
        pointerDown(20, 20),
      );
    });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 495, clientY: 200 });

    expect(screen.getByTestId("workspace-drop-preview")).toHaveAttribute(
      "data-dock",
      "right",
    );

    fireEvent.pointerUp(window, { pointerId: 7, clientX: 495, clientY: 200 });

    expect(screen.getByTestId("terminal-beta")).toBeInTheDocument();
  });

  it("cancels a pointer session with Escape without changing the layout", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });
    const pane = screen.getByTestId("workspace-pane-alpha");
    vi.spyOn(pane, "getBoundingClientRect").mockReturnValue({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [pane]),
    });

    act(() => {
      ref.current?.beginExternalPointerDrag(
        { hostId: "remote-a", agentName: "beta" },
        pointerDown(20, 20),
      );
    });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 105, clientY: 200 });
    fireEvent.keyDown(window, { key: "Escape" });

    expect(screen.queryByTestId("workspace-drop-preview")).toBeNull();
    expect(screen.queryByTestId("terminal-beta")).toBeNull();
  });

  it("cancels an active pointer session when the window loses focus", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });
    const captureTarget = document.createElement("button");
    const setPointerCapture = vi.fn();
    const releasePointerCapture = vi.fn();
    Object.assign(captureTarget, {
      setPointerCapture,
      hasPointerCapture: vi.fn(() => true),
      releasePointerCapture,
    });
    const pane = screen.getByTestId("workspace-pane-alpha");
    vi.spyOn(pane, "getBoundingClientRect").mockReturnValue({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [pane]),
    });

    act(() => {
      ref.current?.beginExternalPointerDrag(
        { hostId: "remote-a", agentName: "beta" },
        pointerDown(20, 20, captureTarget),
      );
    });
    expect(setPointerCapture).toHaveBeenCalledWith(7);
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 105, clientY: 200 });
    expect(screen.getByTestId("workspace-drop-preview")).toBeInTheDocument();
    expect(document.body.style.cursor).toBe("grabbing");

    fireEvent.blur(window);
    fireEvent.pointerUp(window, { pointerId: 7, clientX: 105, clientY: 200 });

    expect(screen.queryByTestId("workspace-drop-preview")).toBeNull();
    expect(screen.queryByTestId("terminal-beta")).toBeNull();
    expect(document.body.style.cursor).toBe("");
    expect(document.body.style.userSelect).toBe("");
    expect(releasePointerCapture).toHaveBeenCalledWith(7);
  });

  it("adopts the rolled back model when a move is rejected after mutating", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });
    add(ref, { hostId: "remote-a", agentName: "beta" });
    const pane = screen.getByTestId("workspace-pane-alpha");
    vi.spyOn(pane, "getBoundingClientRect").mockReturnValue({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [pane]),
    });
    // Stand in for a move flexlayout mutates and moveTerminalInModel then
    // refuses. The replacement model deliberately holds a single terminal, so
    // "the workspace switched to the returned model" is visible rather than
    // inferred: keeping the old one would still show both.
    const rolledBack = createWorkspaceModel();
    addTerminalToModel(rolledBack, { hostId: "", agentName: "alpha" });
    vi.mocked(moveTerminalInModel).mockReturnValueOnce({
      outcome: "restored",
      model: rolledBack,
    });

    fireEvent.pointerDown(screen.getByTestId("workspace-drag-beta"), {
      button: 0,
      pointerId: 7,
      clientX: 20,
      clientY: 20,
    });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 105, clientY: 200 });
    fireEvent.pointerUp(window, { pointerId: 7, clientX: 105, clientY: 200 });

    expect(moveTerminalInModel).toHaveBeenCalled();
    expect(screen.getByText("That split would have broken the layout, so it was restored."))
      .toBeInTheDocument();
    expect(screen.getByTestId("terminal-alpha")).toBeInTheDocument();
    expect(screen.queryByTestId("terminal-beta")).toBeNull();
  });

  it("persists the restored layout, never the one the rejected move left behind", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });
    add(ref, { hostId: "remote-a", agentName: "beta" });
    const pane = screen.getByTestId("workspace-pane-alpha");
    vi.spyOn(pane, "getBoundingClientRect").mockReturnValue({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [pane]),
    });
    const rolledBack = createWorkspaceModel();
    addTerminalToModel(rolledBack, { hostId: "", agentName: "alpha" });
    addTerminalToModel(rolledBack, { hostId: "remote-a", agentName: "beta" });
    // The rejection has to arrive *after* the model was mutated, as flexlayout
    // does it: the change listener has already fired by then and queued the
    // broken layout for persistence. Returning "restored" without mutating
    // would leave nothing for the handler to defuse.
    vi.mocked(moveTerminalInModel).mockImplementationOnce((model) => {
      addTerminalToModel(model, { hostId: "", agentName: "ghost" });
      return { outcome: "restored", model: rolledBack };
    });

    fireEvent.pointerDown(screen.getByTestId("workspace-drag-beta"), {
      button: 0,
      pointerId: 7,
      clientX: 20,
      clientY: 20,
    });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 105, clientY: 200 });
    fireEvent.pointerUp(window, { pointerId: 7, clientX: 105, clientY: 200 });

    // Rolling back on screen is only half of it: the queued write would put the
    // broken layout back at the next reload, undoing the rollback silently.
    const saved = localStorage.getItem(WORKSPACE_STATE_KEY) ?? "";
    expect(saved).not.toContain("ghost");
    // And the restored layout is written out, rather than the storage keeping
    // some older state until the next change happens to touch it.
    expect(saved).toContain('"hostId":"remote-a"');
  });

  it("keeps rendering its own model when a move changes nothing", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });
    add(ref, { hostId: "remote-a", agentName: "beta" });
    const pane = screen.getByTestId("workspace-pane-alpha");
    vi.spyOn(pane, "getBoundingClientRect").mockReturnValue({
      left: 100,
      top: 50,
      width: 400,
      height: 300,
      right: 500,
      bottom: 350,
      x: 100,
      y: 50,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [pane]),
    });
    vi.mocked(moveTerminalInModel).mockImplementationOnce((model) => ({
      outcome: "unchanged",
      model,
    }));

    fireEvent.pointerDown(screen.getByTestId("workspace-drag-beta"), {
      button: 0,
      pointerId: 7,
      clientX: 20,
      clientY: 20,
    });
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 105, clientY: 200 });
    fireEvent.pointerUp(window, { pointerId: 7, clientX: 105, clientY: 200 });

    // A refusal that changed nothing is not worth a notice, and nothing is swapped.
    expect(screen.queryByText("That split would have broken the layout, so it was restored."))
      .toBeNull();
    expect(screen.getByTestId("terminal-alpha")).toBeInTheDocument();
    expect(screen.getByTestId("terminal-beta")).toBeInTheDocument();
  });

  it("restores local and remote terminal identities from persisted layout", () => {
    localStorage.setItem(WORKSPACE_STATE_KEY, JSON.stringify({
      schemaVersion: 1,
      layout: {
        global: {},
        borders: [],
        layout: {
          type: "row",
          weight: 100,
          children: [
            {
              type: "tabset",
              id: "left",
              weight: 50,
              selected: 0,
              children: [{
                type: "tab",
                id: "local-alpha",
                name: "alpha",
                component: "agent-terminal",
                config: { hostId: "", agentName: "alpha" },
              }],
            },
            {
              type: "tabset",
              id: "right",
              weight: 50,
              selected: 0,
              children: [{
                type: "tab",
                id: "remote-beta",
                name: "beta",
                component: "agent-terminal",
                config: { hostId: "remote-a", agentName: "beta" },
              }],
            },
          ],
        },
      },
      activeTerminal: "[\"remote-a\",\"beta\"]",
      sidebar: { width: 320, hidden: false },
    }));

    renderWorkspace();

    expect(screen.getByTestId("terminal-alpha")).toBeInTheDocument();
    expect(screen.getByTestId("terminal-beta")).toBeInTheDocument();
    expect(screen.getByLabelText("beta terminal on Prod")).toHaveAttribute(
      "data-selected",
      "true",
    );
    expect(screen.getByLabelText("beta terminal on Prod")).not.toHaveClass("ring-2");
  });

  it("persists layout and active identity after the debounce", () => {
    vi.useFakeTimers();
    const { ref } = renderWorkspace();

    add(ref, { hostId: "remote-a", agentName: "beta" });
    expect(localStorage.getItem(WORKSPACE_STATE_KEY)).toBeNull();
    act(() => vi.advanceTimersByTime(200));

    const saved = JSON.parse(localStorage.getItem(WORKSPACE_STATE_KEY)!);
    expect(saved.activeTerminal).toBe(terminalKey({ hostId: "remote-a", agentName: "beta" }));
    expect(JSON.stringify(saved.layout)).toContain('"hostId":"remote-a"');
    expect(JSON.stringify(saved.layout)).not.toContain("redacted");
  });

  it("keeps an unavailable tile and retries aggregate refresh", () => {
    localStorage.setItem(WORKSPACE_STATE_KEY, JSON.stringify({
      schemaVersion: 1,
      layout: {
        global: {},
        borders: [],
        layout: {
          type: "row",
          children: [{
            type: "tabset",
            id: "missing-set",
            selected: 0,
            children: [{
              type: "tab",
              id: "missing-tab",
              name: "missing",
              component: "agent-terminal",
              config: { hostId: "gone", agentName: "missing" },
            }],
          }],
        },
      },
      activeTerminal: null,
      sidebar: { width: 256, hidden: false },
    }));
    const { refresh } = renderWorkspace();

    expect(screen.getByText(/workspace position was kept/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry missing" }));
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("replaces an unavailable terminal in the same workspace slot", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "gone", agentName: "missing" });

    fireEvent.click(screen.getByRole("button", { name: "Replace missing" }));
    fireEvent.click(screen.getByRole("button", { name: "Use beta on Prod" }));

    expect(screen.queryByText(/workspace position was kept/i)).toBeNull();
    expect(screen.getByTestId("terminal-beta")).toBeInTheDocument();
  });

  it("closes only the workspace tile and never calls an agent lifecycle API", () => {
    const { ref } = renderWorkspace();
    add(ref, { hostId: "", agentName: "alpha" });

    fireEvent.click(screen.getByRole("button", { name: "Close alpha terminal" }));

    expect(screen.getByText("Drag an interactive agent here")).toBeInTheDocument();
    expect(agentPostOn).not.toHaveBeenCalled();
  });

  it("rejects adding a non-interactive agent", () => {
    const { ref } = renderWorkspace();
    let added = true;

    act(() => {
      added = ref.current?.addOrFocus({ hostId: "", agentName: "non-tty" }) ?? true;
    });

    expect(added).toBe(false);
    expect(screen.getByText(/does not have an interactive terminal/i)).toBeInTheDocument();
    expect(screen.queryByTestId("terminal-non-tty")).toBeNull();
  });

  it("routes a restored non-interactive tile through the page configuration action", () => {
    localStorage.setItem(WORKSPACE_STATE_KEY, JSON.stringify({
      schemaVersion: 1,
      layout: {
        global: {},
        borders: [],
        layout: {
          type: "row",
          children: [{
            type: "tabset",
            id: "non-tty-set",
            children: [{
              type: "tab",
              id: "non-tty-tab",
              name: "non-tty",
              component: "agent-terminal",
              config: { hostId: "", agentName: "non-tty" },
            }],
          }],
        },
      },
      activeTerminal: null,
      sidebar: { width: 256, hidden: false },
    }));
    const { onOpenConfiguration } = renderWorkspace();

    fireEvent.click(screen.getByRole("link", { name: "Open Configuration" }));

    expect(onOpenConfiguration).toHaveBeenCalledWith({
      hostId: "",
      agentName: "non-tty",
    });
  });
});
