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
import { addDaemon, listDaemons, getDaemonToken, type Daemon } from "@/lib/daemons";
import * as desktop from "@/lib/desktop";
import {
  cleanupTerminalsPage,
  setupTerminalsPage,
  renderAt,
  registerProd,
  openManageMenu,
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

describe("TerminalsPage host status", () => {
  it("shows native SSH connection failures in the terminal workspace", async () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    vi.spyOn(desktop, "hostsList").mockResolvedValue([
      {
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
      },
    ]);
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
    vi.spyOn(desktop, "hostsList").mockResolvedValue([
      {
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
      },
    ]);

    renderAt("/terminals");

    expect(await screen.findByText("ready")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update prod" })).toBeNull();
  });
});

describe("TerminalsPage Update all", () => {
  /** Two SSH hosts behind the app version: the state "Update all (N)" renders for. */
  async function staleHosts() {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    vi.spyOn(desktop, "daemonState").mockResolvedValue({
      state: "ready",
      base_url: "http://127.0.0.1:9990",
      daemon_version: "0.12.0",
      app_version: "0.12.0",
      base_dir: "/tmp/tariboy",
      pid: 42,
      adopted: false,
      message: "",
    });
    const host = (id: string, label: string) => ({
      id,
      label,
      kind: "ssh" as const,
      ssh_alias: `${label}-box`,
      remote_install_dir: "~/.local/lib/tariboy",
      remote_port: 9990,
      https_base_url: "",
      last_daemon_version: "0.11.5",
      state: "ready" as const,
      base_url: "http://127.0.0.1:18444",
      local_port: 18444,
      phase: "connect",
      platform: "Linux",
      arch: "x86_64",
      prerequisites: [],
      message: "",
    });
    vi.spyOn(desktop, "hostsList").mockResolvedValue([host("d1", "alpha"), host("d2", "beta")]);
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "d1", label: "alpha" }, agents: [] },
      { host: { id: "d2", label: "beta" }, agents: [] },
    ]);
    renderAt("/terminals");
    // Update all lives in the footer's hosts menu; Radix opens it from the keyboard.
    await screen.findByRole("img", { name: "2 hosts can be updated" });
    fireEvent.keyDown(screen.getByRole("button", { name: "Hosts menu" }), { key: "Enter" });
    return screen.findByRole("menuitem", { name: "Update all (2)" });
  }

  it("updates every outdated server once Update all is confirmed", async () => {
    const update = vi.spyOn(desktop, "hostUpdate").mockResolvedValue({ operation_id: "op" });
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );

    fireEvent.click(await staleHosts());

    await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
    expect(update.mock.calls.map(([id]) => id)).toEqual(["d1", "d2"]);
  });

  it("updates nothing when Update all is cancelled", async () => {
    const update = vi.spyOn(desktop, "hostUpdate").mockResolvedValue({ operation_id: "op" });
    vi.stubGlobal(
      "confirm",
      vi.fn(() => false),
    );

    fireEvent.click(await staleHosts());

    await waitFor(() => expect(window.confirm).toHaveBeenCalled());
    expect(update).not.toHaveBeenCalled();
  });

  it("reports a failed server and still updates the rest", async () => {
    const update = vi
      .spyOn(desktop, "hostUpdate")
      .mockRejectedValueOnce(new Error("ssh closed"))
      .mockResolvedValue({ operation_id: "op" });
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );

    fireEvent.click(await staleHosts());

    expect(await screen.findByRole("alert")).toHaveTextContent("alpha");
    expect(update).toHaveBeenCalledTimes(2);
  });
});

describe("TerminalsPage server management", () => {
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
    await waitFor(() => {
      expect(label).toHaveValue("prod");
      expect(url).toHaveValue("http://127.0.0.1:9992");
    });

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
    const id = (
      await addDaemon({
        label: "prod",
        baseURL: "http://127.0.0.1:9992",
        token: "tok",
      })
    ).id;
    vi.mocked(fetchAllAgents).mockResolvedValue([
      { host: { id: "", label: "This daemon (local)" }, agents: [] },
      {
        host: { id, label: "prod" },
        agents: [
          agentRow({
            name: "a1",
            image: "bare:latest",
            state: "running",
            harness: "claude",
            interactive: true,
          }),
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
