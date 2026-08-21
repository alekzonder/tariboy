import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { DaemonProvider } from "@/components/DaemonProvider";
import TerminalsPage from "./TerminalsPage";
import { fetchAllAgents } from "@/lib/aggregate";
import {
  agentGetOn,
  createAgent,
  imageManifestGet,
  listImages,
  startAgent,
} from "@/lib/api";
import { addDaemon, listDaemons, getDaemonToken } from "@/lib/daemons";
import * as desktop from "@/lib/desktop";
import { SidebarStateProvider } from "./SidebarStateProvider";
import { CustomerQuestionNotificationsContext } from "@/components/customerQuestionNotificationsContext";
import { targetFor } from "@/lib/terminalsHost";

vi.mock("@/lib/aggregate", () => ({
  fetchAllAgents: vi.fn(),
}));

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return {
    ...actual,
    agentGetOn: vi.fn(),
    createAgent: vi.fn(),
    imageManifestGet: vi.fn(),
    listImages: vi.fn(),
    startAgent: vi.fn(),
  };
});

// Selecting an agent mounts TerminalPane, which dials a websocket and renders
// xterm.js — neither works in jsdom. Stub both, same approach as
// TerminalPane.test.tsx.
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

const cloneProjection = {
  name: "source",
  image: "worker:v1",
  digest: "sha256",
  state: "stopped",
  cwd: "/managed/source",
  configured_cwd: "",
  harness: "codex",
  model: "gpt-5",
  effort: "high",
  interactive: true,
  loop_enabled: false,
  enabled: false,
  interval_s: 0,
  timeout_s: 0,
  hard_timeout_s: 0,
  on_timeout: "restart",
  on_error: "restart",
  max_idle_iterations: 0,
  user_prompt: "",
  env: {},
  plugins: [],
  messages_batch: 10,
  messages_max_queue: 1000,
  group: null,
  alias: "",
  notes: "",
  color: "",
} as const;

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  vi.mocked(fetchAllAgents).mockResolvedValue([
    {
      host: { id: "", label: "This daemon (local)" },
      agents: [
        { name: "a1", image: "bare:latest", state: "running", harness: "claude", loop_enabled: false, group: null, interactive: true },
        { name: "a2", image: "img", state: "running", harness: "claude", loop_enabled: false, group: null, interactive: false },
      ],
    },
    { host: { id: "d1", label: "prod" }, agents: [], error: "unauthorized: nope" },
  ]);
  vi.mocked(listImages).mockResolvedValue({
    images: [{ name: "worker", tag: "v1", bare: false }],
    count: 1,
  });
  vi.mocked(imageManifestGet).mockResolvedValue({
    schema_version: 1,
    name: "worker",
    tag: "v1",
    built_at: "2026-07-29T00:00:00Z",
    parents: null,
    plugins: null,
	 skills: null,
    requires_secrets: null,
    harness: { type: "codex", interactive: false },
    env: null,
    policy: {},
    evals: null,
    layers: null,
  });
  vi.mocked(startAgent).mockResolvedValue({ name: "new-agent", action: "start" });
  vi.mocked(agentGetOn).mockResolvedValue(cloneProjection);
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderAt(path: string, attention: ReadonlySet<string> = new Set()) {
  return render(
    <DaemonProvider>
      <CustomerQuestionNotificationsContext.Provider value={{ attention, refreshHost: async () => {} }}>
        <SidebarStateProvider>
          <MemoryRouter initialEntries={[path]}>
            <RoutedLocation />
            <Routes>
              <Route path="/" element={<TerminalsPage />} />
              <Route path="/workspace" element={<TerminalsPage />} />
              <Route path="/terminals" element={<TerminalsPage />} />
              <Route path="/terminals/:hostId/:agent" element={<TerminalsPage />} />
              <Route path="/agents/:hostId/:agent/:tab" element={<TerminalsPage />} />
              <Route path="/agents/:hostId/teams/:team" element={<TerminalsPage />} />
              <Route path="/servers/:hostId/tasks" element={<TerminalsPage />} />
            </Routes>
          </MemoryRouter>
        </SidebarStateProvider>
      </CustomerQuestionNotificationsContext.Provider>
    </DaemonProvider>,
  );
}

function RoutedLocation() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname + location.search}</output>;
}

/** Register "prod" in the real (jsdom) registry under the id the mocked
 *  fetchAllAgents reports, so the manage menu operates on a real entry. */
async function registerProd(): Promise<string> {
  const meta = await addDaemon({ label: "prod", baseURL: "http://127.0.0.1:9992", token: "tok" });
  vi.mocked(fetchAllAgents).mockResolvedValue([
    { host: { id: "", label: "This daemon (local)" }, agents: [] },
    { host: { id: meta.id, label: "prod" }, agents: [] },
  ]);
  return meta.id;
}

// Radix opens a DropdownMenu on pointerdown, which jsdom's fireEvent.click
// does not synthesize — drive it from the keyboard instead.
async function openManageMenu(): Promise<void> {
  fireEvent.keyDown(await screen.findByRole("button", { name: "manage prod" }), { key: "Enter" });
}

describe("TerminalsPage", () => {
  it("retains the selected agent shell when its next aggregate refresh is unavailable", async () => {
    const remote = await addDaemon({ label: "prod", baseURL: "http://127.0.0.1:19992", token: "tok" });
    let state: "ready" | "unavailable" | "recovered" = "ready";
    vi.mocked(fetchAllAgents).mockImplementation(async () => [{
      host: { id: remote.id, label: "prod" },
      agents: state === "unavailable" ? [] : [{
        name: "worker", image: state === "recovered" ? "worker:v2" : "worker:v1", state: "running", harness: "codex",
        loop_enabled: false, group: null, interactive: false,
      }],
      error: state === "unavailable" ? "unreachable" : undefined,
    }]);

    renderAt(`/agents/${remote.id}/worker/console`);
    expect(await screen.findByRole("heading", { name: "worker" })).toBeInTheDocument();

    state = "unavailable";
    await waitFor(() => expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(1), {
      timeout: 4_500,
    });

    expect(screen.getByRole("heading", { name: "worker" })).toBeInTheDocument();
    expect(screen.getByText(/temporarily unavailable/i)).toHaveAttribute("role", "status");
    expect(screen.getByTestId("agent-workspace-content")).toHaveAttribute("inert", "");
    expect(screen.getByRole("button", { name: "Open worker" })).toBeDisabled();

    state = "recovered";
    await waitFor(() => expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(2), {
      timeout: 4_500,
    });
    expect(screen.getByRole("heading", { name: "worker" })).toBeInTheDocument();
    expect(screen.getByText("worker:v2")).toBeInTheDocument();
    expect(screen.queryByText(/temporarily unavailable/i)).toBeNull();
    expect(screen.getByTestId("agent-workspace-content")).not.toHaveAttribute("inert");
  }, 10_000);

  it("renders Workspace only at its global route", async () => {
    renderAt("/workspace");

    expect(await screen.findByTestId("terminal-workspace")).toBeInTheDocument();
    expect(screen.getByText("Drag an interactive agent here")).toBeInTheDocument();
    expect(screen.queryByRole("tablist", { name: "Agents view" })).toBeNull();
    expect(screen.getByTestId("location")).toHaveTextContent("/workspace");
    expect(screen.queryByRole("navigation", { name: "Server workspace" })).toBeNull();
  });

  it("opens a server workspace from a selectable server heading", async () => {
    renderAt("/");

    fireEvent.click(await screen.findByRole("button", {
      name: "Open server This daemon (local)",
    }));

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/servers/local/tasks"),
    );
    expect(screen.getByRole("navigation", { name: "Server workspace" }))
      .toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open server This daemon (local)" }))
      .toHaveAttribute("aria-current", "page");
  });

  it("shows server context above agent context with distinct selected states", async () => {
    renderAt("/agents/local/a1/console");

    expect(await screen.findByRole("navigation", { name: "Server workspace" }))
      .toBeInTheDocument();
    expect(await screen.findByRole("navigation", { name: "Agent workspace" }))
      .toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open server This daemon (local)" }))
      .toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "Open a1" }))
      .toHaveAttribute("aria-current", "page");
  });

  it("adds and focuses a sidebar agent in Workspace without duplicating it", async () => {
    renderAt("/workspace");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Open a1" }));
    fireEvent.click(screen.getByRole("button", { name: "Open a1" }));

    expect(screen.getAllByRole("tab", { name: "a1" })).toHaveLength(1);
    expect(screen.getByTestId("location")).toHaveTextContent("/workspace");
  });

  it("opens a non-interactive sidebar agent in Configuration from Workspace", async () => {
    renderAt("/workspace");
    await waitFor(() => expect(screen.getByText("a2")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Open a2" }));

    await waitFor(() =>
      expect(screen.getByTestId("location"))
        .toHaveTextContent("/agents/local/a2/configuration"),
    );
    expect(await screen.findByRole("navigation", { name: "Agent workspace" }))
      .toBeInTheDocument();
  });

  it("starts a real pointer drag only from an interactive Workspace row", async () => {
    renderAt("/workspace");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    const root = screen.getByTestId("terminal-workspace");
    vi.spyOn(root, "getBoundingClientRect").mockReturnValue({
      left: 0,
      top: 0,
      width: 800,
      height: 600,
      right: 800,
      bottom: 600,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    Object.defineProperty(document, "elementsFromPoint", {
      configurable: true,
      value: vi.fn(() => [root]),
    });
    const interactive = screen.getByRole("button", { name: "Open a1" });

    expect(interactive).not.toHaveAttribute("draggable");
    expect(screen.queryByRole("button", { name: /Add a1 to Workspace/ })).toBeNull();
    fireEvent.pointerDown(interactive, {
      button: 0,
      pointerId: 9,
      clientX: 10,
      clientY: 10,
    });
    fireEvent.pointerMove(window, { pointerId: 9, clientX: 200, clientY: 200 });
    expect(screen.getByTestId("workspace-drop-preview")).toBeInTheDocument();
    fireEvent.pointerUp(window, { pointerId: 9, clientX: 200, clientY: 200 });
    expect(screen.getAllByRole("tab", { name: "a1" })).toHaveLength(1);

    fireEvent.pointerDown(screen.getByRole("button", { name: "Open a2" }), {
      button: 0,
      pointerId: 10,
      clientX: 10,
      clientY: 10,
    });
    fireEvent.pointerMove(window, { pointerId: 10, clientX: 200, clientY: 200 });
    expect(screen.queryByTestId("workspace-drop-preview")).toBeNull();
  });

  it("honors a hidden sidebar from the shared persisted state", async () => {
    localStorage.setItem("terminals:workspace:v1", JSON.stringify({
      schemaVersion: 1,
      layout: {
        global: {},
        borders: [],
        layout: { type: "row", weight: 100, children: [] },
      },
      activeTerminal: null,
      sidebar: { width: 256, hidden: true },
    }));
    renderAt("/workspace");

    await waitFor(() => expect(screen.getByTestId("terminal-workspace")).toBeInTheDocument());
    expect(screen.queryByText("Agents")).toBeNull();
    expect(screen.queryByRole("separator", { name: "resize sidebar" })).toBeNull();
  });

  it("opens a sidebar agent in the product Console route", async () => {
    renderAt("/");
    fireEvent.click(await screen.findByText("a1"));
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/agents/local/a1/console"),
    );
  });

  it("groups agents under server sections and shows host errors", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("This daemon (local)")).toBeInTheDocument());
    expect(screen.getByText("prod")).toBeInTheDocument();
    expect(screen.getByText("a1")).toBeInTheDocument();
    expect(screen.getByText("unauthorized: nope")).toBeInTheDocument();
  });

  it("separates team members from individual agents", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([{
      host: { id: "", label: "This daemon (local)" },
      groups: [{ name: "empty-team", lead: "", members: 0 }],
      agents: [
        { name: "lead", image: "img", state: "running", harness: "claude", loop_enabled: true, group: "dev-team", interactive: true },
        { name: "solo", image: "img", state: "stopped", harness: "codex", loop_enabled: false, group: null, interactive: true },
      ],
    }]);
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
  ])("opens Clone for the $agentName sidebar row without navigating", async ({ agentName, group }) => {
    vi.mocked(fetchAllAgents).mockResolvedValue([{
      host: { id: "", label: "This daemon (local)" },
      agents: [
        { name: "lead", image: "worker:v1", state: "running", harness: "codex", loop_enabled: true, group: "dev-team", interactive: true },
        { name: "solo", image: "worker:v1", state: "stopped", harness: "codex", loop_enabled: false, group: null, interactive: true },
      ],
    }]);
    vi.mocked(agentGetOn).mockResolvedValueOnce({ ...cloneProjection, name: agentName, group });
    renderAt("/");

    const row = await screen.findByRole("button", { name: `Open ${agentName}` });
    fireEvent.contextMenu(row);
    fireEvent.click(await screen.findByRole("menuitem", { name: "Clone" }));

    expect(await screen.findByRole("heading", { name: "Clone agent" })).toBeInTheDocument();
    expect(agentGetOn).toHaveBeenCalledWith(targetFor(""), agentName, "");
    expect(screen.getByTestId("location")).toHaveTextContent("/");
  });

  it("does not navigate or start a Workspace drag for a context-menu gesture", async () => {
    renderAt("/workspace");
    const row = await screen.findByRole("button", { name: "Open a1" });

    fireEvent.pointerDown(row, { button: 2, pointerId: 12, clientX: 10, clientY: 10 });
    fireEvent.pointerMove(window, { pointerId: 12, clientX: 200, clientY: 200 });
    fireEvent.contextMenu(row);

    expect(await screen.findByRole("menuitem", { name: "Clone" })).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/workspace");
    expect(screen.queryByTestId("workspace-drop-preview")).toBeNull();
  });

  it("keeps the individual agents subsection visible when a host only has an empty team", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([{
      host: { id: "", label: "This daemon (local)" },
      groups: [{ name: "empty-team", lead: "", members: 0 }],
      agents: [],
    }]);
    renderAt("/");
    expect(await screen.findByRole("button", { name: "Open team empty-team" })).toBeInTheDocument();
    expect(screen.getByText("Individual agents")).toBeInTheDocument();
  });

  it("shows native SSH connection failures in the terminal workspace", async () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    vi.spyOn(desktop, "hostsList").mockResolvedValue([{
      id: "d1",
      label: "prod",
      kind: "ssh",
      ssh_alias: "prod-box",
      remote_install_dir: "~/.local/lib/tariboy",
      remote_port: 9990,
      https_base_url: "",
      last_daemon_version: "",
      state: "disconnected",
      base_url: "",
      local_port: 0,
      phase: "",
      platform: "",
      arch: "",
      prerequisites: [],
      message: "",
    }]);
    vi.spyOn(desktop, "hostConnect").mockRejectedValue(new Error("SSH timed out"));

    renderAt("/terminals");
    fireEvent.click(await screen.findByRole("button", { name: "Connect prod" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not connect to prod: Error: SSH timed out",
    );
  });

  it("hides Update in the agent sidebar when the remote version is current", async () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    vi.spyOn(desktop, "daemonState").mockResolvedValue({
      state: "ready",
      base_url: "http://127.0.0.1:9990",
      daemon_version: "0.11.5",
      app_version: "0.11.5",
      base_dir: "/tmp/tariboy",
      pid: 42,
      adopted: false,
      message: "",
    });
    vi.spyOn(desktop, "hostsList").mockResolvedValue([{
      id: "d1",
      label: "prod",
      kind: "ssh",
      ssh_alias: "prod-box",
      remote_install_dir: "~/.local/lib/tariboy",
      remote_port: 9990,
      https_base_url: "",
      last_daemon_version: "0.11.5",
      state: "ready",
      base_url: "http://127.0.0.1:18444",
      local_port: 18444,
      phase: "connect",
      platform: "Linux",
      arch: "x86_64",
      prerequisites: [],
      message: "",
    }]);

    renderAt("/terminals");

    expect(await screen.findByText("ready")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update prod" })).toBeNull();
  });

  it("empty state prompts to pick or create an agent when none selected", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "New agent" })).toBeInTheDocument();
  });

  it("marks non-interactive agents in the sidebar and leaves interactive ones unmarked", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a2")).toBeInTheDocument());
    const a1Row = screen.getByText("a1").closest("button")!;
    const a2Row = screen.getByText("a2").closest("button")!;
    expect(a2Row.textContent).toMatch(/non-tty/);
    expect(a1Row.textContent).not.toMatch(/non-tty/);
  });

  it("shows one host-scoped customer-question dot beside the matching same-named remote agent", async () => {
    vi.mocked(fetchAllAgents).mockResolvedValue([
      {
        host: { id: "", label: "This daemon (local)" },
        agents: [{ name: "alice", image: "bare:latest", state: "running", harness: "claude", loop_enabled: false, group: null, interactive: true }],
      },
      {
        host: { id: "remote-1", label: "prod" },
        groups: [{ name: "platform", lead: "alice", members: 1 }],
        agents: [{ name: "alice", image: "bare:latest", state: "running", harness: "claude", loop_enabled: false, group: "platform", interactive: true }],
      },
    ])

    renderAt("/", new Set([JSON.stringify(["remote-1", "alice"])]))

    expect(await screen.findAllByRole("img", { name: "Unread customer question for alice on prod" }))
      .toHaveLength(1)
    expect(screen.queryByRole("img", { name: /This daemon|platform/ })).toBeNull()
  });

  it("re-polls immediately after creating an agent instead of waiting for the next tick", async () => {
    vi.mocked(createAgent).mockResolvedValue({ name: "new-agent", state: "running" });
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    const callsBefore = vi.mocked(fetchAllAgents).mock.calls.length;

    fireEvent.click(screen.getByRole("button", { name: "New agent" }));
    fireEvent.focus(await screen.findByLabelText("image"));
    fireEvent.click(await screen.findByRole("option", { name: "worker:v1" }));
    const create = screen.getByRole("button", { name: "Create agent" });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);

    await waitFor(() => expect(createAgent).toHaveBeenCalled());
    await waitFor(() =>
      expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });

  it("opens Run Agent with both host and image preselected", async () => {
    const id = await registerProd();
    renderAt(`/terminals?new=1&host=${id}&image=worker%3Av1`);

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Host" })).toHaveTextContent("prod");
    expect(screen.getByLabelText("image")).toHaveValue("worker:v1");
    await waitFor(() =>
      expect(listImages).toHaveBeenCalledWith(expect.objectContaining({ id })),
    );
    expect(imageManifestGet).toHaveBeenCalledWith(
      "worker:v1",
      expect.objectContaining({ id }),
    );
  });

  it("offers a manage menu for remote servers but not for the local daemon", async () => {
    await registerProd();
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("prod")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "manage prod" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "manage This daemon (local)" })).toBeNull();
  });

  it("edits a server and re-polls immediately with the new values", async () => {
    const id = await registerProd();
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("prod")).toBeInTheDocument());
    const callsBefore = vi.mocked(fetchAllAgents).mock.calls.length;

    await openManageMenu();
    fireEvent.click(await screen.findByText("Edit host"));

    const label = (await screen.findByLabelText("label")) as HTMLInputElement;
    const url = screen.getByLabelText("base URL") as HTMLInputElement;
    expect(label.value).toBe("prod");
    expect(url.value).toBe("http://127.0.0.1:9992");

    fireEvent.change(label, { target: { value: "prod-renamed" } });
    fireEvent.change(url, { target: { value: "http://127.0.0.1:9993/" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(async () =>
      expect((await listDaemons()).find((m) => m.id === id)?.label).toBe("prod-renamed"),
    );
    const meta = (await listDaemons()).find((m) => m.id === id)!;
    expect(meta.label).toBe("prod-renamed");
    expect(meta.baseURL).toBe("http://127.0.0.1:9993");
    // Blank token field must not wipe the stored token.
    expect(await getDaemonToken(id)).toBe("tok");
    await waitFor(() =>
      expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });

  it("removes a server after confirmation and re-polls", async () => {
    const id = await registerProd();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("prod")).toBeInTheDocument());
    const callsBefore = vi.mocked(fetchAllAgents).mock.calls.length;

    await openManageMenu();
    fireEvent.click(await screen.findByText("Remove host"));

    await waitFor(() => expect(confirm).toHaveBeenCalled());
    expect(confirm).toHaveBeenCalledWith(expect.stringMatching(/remote daemon and data remain/i));
    await waitFor(async () =>
      expect((await listDaemons()).find((m) => m.id === id)).toBeUndefined(),
    );
    expect(await getDaemonToken(id)).toBe("");
    await waitFor(() =>
      expect(vi.mocked(fetchAllAgents).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });

  it("keeps the server when the confirm is declined", async () => {
    const id = await registerProd();
    vi.spyOn(window, "confirm").mockReturnValue(false);
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("prod")).toBeInTheDocument());

    await openManageMenu();
    fireEvent.click(await screen.findByText("Remove host"));

    await waitFor(() => expect(window.confirm).toHaveBeenCalled());
    expect((await listDaemons()).find((m) => m.id === id)).toBeDefined();
  });

  it("navigates away when the server open in the route is removed", async () => {
    const id = (await addDaemon({
      label: "prod", baseURL: "http://127.0.0.1:9992", token: "tok",
    })).id;
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
      {
        host: { id, label: "prod" },
        agents: [
          { name: "a1", image: "bare:latest", state: "running", harness: "claude", loop_enabled: false, group: null, interactive: true },
        ],
      },
    ]);
    vi.spyOn(window, "confirm").mockReturnValue(true);

    renderAt(`/agents/${id}/a1/console`);
    // The pane is up: "prod" is both the sidebar heading and the pane's host label.
    await waitFor(() => expect(screen.getByTestId("tui-screen")).toBeInTheDocument());

    await openManageMenu();
    fireEvent.click(await screen.findByText("Remove host"));

    // Back on the empty state — the terminal pane must not linger on a host id
    // that now resolves to the local daemon.
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "New agent" })).toBeInTheDocument(),
    );
  });
});

describe("sidebar width", () => {
  it("starts at the persisted width", async () => {
    localStorage.setItem("terminals:sidebarWidth", "380");
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    expect(screen.getByText("Agents").parentElement).toHaveStyle({ width: "380px" });
  });

  it("drags to a new width and persists it", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    const handle = screen.getByRole("separator", { name: "resize sidebar" });

    // jsdom has no layout, so the aside's rect.left is 0 and clientX is the width.
    fireEvent.pointerDown(handle);
    fireEvent.pointerMove(window, { clientX: 400 });
    fireEvent.pointerUp(window);

    expect(screen.getByText("Agents").parentElement).toHaveStyle({ width: "400px" });
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ schemaVersion: 1, sidebar: { width: 400, hidden: false } });

    // Drag ended: further pointer moves must not keep resizing.
    fireEvent.pointerMove(window, { clientX: 200 });
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ sidebar: { width: 400, hidden: false } });
  });

  it("clamps a drag past the maximum", async () => {
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    fireEvent.pointerDown(screen.getByRole("separator", { name: "resize sidebar" }));
    fireEvent.pointerMove(window, { clientX: 5000 });
    fireEvent.pointerUp(window);
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ sidebar: { width: 640, hidden: false } });
  });

  it("resets to the default on double-click", async () => {
    localStorage.setItem("terminals:sidebarWidth", "500");
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    fireEvent.doubleClick(screen.getByRole("separator", { name: "resize sidebar" }));
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ sidebar: { width: 256, hidden: false } });
  });

  it("resizes with arrow keys", async () => {
    localStorage.setItem("terminals:sidebarWidth", "300");
    renderAt("/terminals");
    await waitFor(() => expect(screen.getByText("a1")).toBeInTheDocument());
    const handle = screen.getByRole("separator", { name: "resize sidebar" });
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ sidebar: { width: 308, hidden: false } });
    fireEvent.keyDown(handle, { key: "ArrowLeft", shiftKey: true });
    expect(JSON.parse(localStorage.getItem("terminals:workspace:v1")!))
      .toMatchObject({ sidebar: { width: 276, hidden: false } });
  });
});
