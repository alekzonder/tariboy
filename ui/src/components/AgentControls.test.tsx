import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen, fireEvent, waitFor, within } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import type { PrimaryAction } from "@/pages/agents/agentWorkspaceModel";
import { AgentControls, DeleteAgentButton } from "./AgentControls";

const target = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" };

const agentDelete = vi.fn();
// The client of the route host: every delete goes through it, never the active daemon.
const remoteApi = () => fakeApi({ agents: { agentDelete } }, target);

// Block body on purpose: an arrow returning the mock hands vitest a "cleanup
// function" that is the mock itself, which it then calls and awaits.
beforeEach(() => {
  agentDelete.mockReset();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const paths = {
  settingsPath: "/agents/local/alpha/settings",
  workPath: "/agents/local/alpha/work",
};

function renderControls(alive: boolean, primary: PrimaryAction = alive ? "give" : "start") {
  return render(
    <MemoryRouter>
      <AgentControls name="alpha" alive={alive} primary={primary} {...paths} />
    </MemoryRouter>,
  );
}

// Radix opens a DropdownMenu on pointerdown, which jsdom's fireEvent.click
// does not synthesize — drive it from the keyboard instead.
const openMenu = () =>
  fireEvent.keyDown(screen.getByRole("button", { name: "More agent actions" }), { key: "Enter" });

it("posts the run-state action of the toggle to the route host", async () => {
  const calls: Array<{ method: string; path: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation((path: string, init?: RequestInit) => {
      calls.push({ method: init?.method ?? "GET", path });
      return Promise.resolve({
        ok: true,
        status: 200,
        text: async () => JSON.stringify({ ok: true, result: {} }),
      } as Response);
    }),
  );
  const { unmount } = renderControls(false);

  // Off: Start is the primary action, and the only Start.
  expect(screen.getAllByRole("button", { name: "Start" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "Start" }));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/agents/alpha/start" && c.method === "POST")).toBe(
      true,
    ),
  );
  unmount();

  renderControls(true);
  fireEvent.click(screen.getByRole("button", { name: "Stop" }));
  await waitFor(() =>
    expect(calls.some((c) => c.path === "/api/agents/alpha/stop" && c.method === "POST")).toBe(
      true,
    ),
  );
});

it.each([
  ["answer", "Answer"],
  ["give", "Give task"],
])("links the %s primary action to Work", (primary, label) => {
  renderControls(true, primary as PrimaryAction);
  expect(screen.getByRole("link", { name: label })).toHaveAttribute(
    "href",
    "/agents/local/alpha/work",
  );
  expect(screen.getByRole("button", { name: "Stop" })).toBeInTheDocument();
});

it("posts restart for the Restart primary action", async () => {
  const agentPost = vi.fn().mockResolvedValue({});
  render(
    <MemoryRouter>
      <AgentControls name="alpha" alive primary="restart" {...paths} />
    </MemoryRouter>,
    { api: fakeApi({ agentPost }, target) },
  );
  fireEvent.click(screen.getByRole("button", { name: "Restart" }));
  await waitFor(() => expect(agentPost).toHaveBeenCalledWith("alpha", "restart"));
});

it("force-stops the current run only after the confirmation dialog", async () => {
  const agentPost = vi.fn().mockResolvedValue({});
  render(
    <MemoryRouter>
      <AgentControls name="alpha" alive primary="give" {...paths} />
    </MemoryRouter>,
    { api: fakeApi({ agentPost }, target) },
  );
  openMenu();
  fireEvent.click(await screen.findByText("Force-stop current run"));
  const dialog = screen.getByRole("alertdialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(agentPost).not.toHaveBeenCalled();

  openMenu();
  fireEvent.click(await screen.findByText("Force-stop current run"));
  fireEvent.click(
    within(screen.getByRole("alertdialog")).getByRole("button", { name: "Force-stop" }),
  );
  await waitFor(() => expect(agentPost).toHaveBeenCalledWith("alpha", "kill"));
});

it("offers Run once, VS Code, Clone, Pin, Settings and Delete in the menu", async () => {
  const onClone = vi.fn();
  const onTogglePin = vi.fn();
  const onOpenFolder = vi.fn();
  render(
    <MemoryRouter>
      <AgentControls
        name="alpha"
        alive
        image="basic:latest"
        primary="give"
        {...paths}
        onClone={onClone}
        pinned={false}
        onTogglePin={onTogglePin}
        onOpenFolder={onOpenFolder}
      />
    </MemoryRouter>,
  );
  openMenu();
  const items = (await screen.findAllByRole("menuitem")).map((item) => item.textContent);
  expect(items).toEqual([
    "Run once…",
    "Force-stop current run",
    "Open folder in VS Code",
    "Clone",
    "Pin",
    "Settings",
    "Delete agent",
  ]);
  expect(screen.getByRole("menuitem", { name: "Settings" })).toHaveAttribute(
    "href",
    "/agents/local/alpha/settings",
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "Pin" }));
  expect(onTogglePin).toHaveBeenCalledOnce();
  openMenu();
  fireEvent.click(await screen.findByRole("menuitem", { name: "Clone" }));
  expect(onClone).toHaveBeenCalledOnce();
  openMenu();
  fireEvent.click(await screen.findByRole("menuitem", { name: "Open folder in VS Code" }));
  expect(onOpenFolder).toHaveBeenCalledOnce();
});

describe("delete from the overflow menu", () => {
  const openDelete = async () => {
    openMenu();
    fireEvent.click(await screen.findByText("Delete agent"));
  };

  function renderWithDelete() {
    const onDeleted = vi.fn();
    const refresh = vi.fn();
    const api = remoteApi();
    render(
      <MemoryRouter>
        <AgentControls
          name="worker"
          alive
          primary="give"
          {...paths}
          refresh={refresh}
          onDeleted={onDeleted}
        />
      </MemoryRouter>,
      { api },
    );
    return { onDeleted, refresh, api };
  }

  it("opens an in-app confirmation and cancels without deleting", async () => {
    renderWithDelete();
    await openDelete();

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Delete agent worker?" })).toBeInTheDocument();
    expect(agentDelete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(agentDelete).not.toHaveBeenCalled();
  });

  it("deletes once with force and purge, then leaves the route", async () => {
    vi.mocked(agentDelete).mockResolvedValue({});
    const { onDeleted, refresh, api } = renderWithDelete();
    expect(api.target).toBe(target);
    await openDelete();
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    await waitFor(() => expect(agentDelete).toHaveBeenCalledOnce());
    expect(agentDelete).toHaveBeenCalledWith("worker", { force: true, purge: true });
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("disables dismissal and duplicate submission while deletion is pending", async () => {
    let resolveDelete!: (value: unknown) => void;
    vi.mocked(agentDelete).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveDelete = resolve;
        }),
    );
    const { onDeleted } = renderWithDelete();
    await openDelete();
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    const pending = screen.getByRole("button", { name: "Deleting…" });
    expect(pending).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(agentDelete).toHaveBeenCalledOnce();

    resolveDelete({});
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
  });

  it("keeps the dialog open and does not leave the route after a deletion failure", async () => {
    vi.mocked(agentDelete).mockImplementation(() => Promise.reject(new Error("delete failed")));
    const { onDeleted } = renderWithDelete();
    await openDelete();
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    await waitFor(() => expect(agentDelete).toHaveBeenCalledOnce());
    await waitFor(() => expect(screen.getByRole("button", { name: "Delete agent" })).toBeEnabled());
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(onDeleted).not.toHaveBeenCalled();
  });
});

describe("DeleteAgentButton", () => {
  it("deletes through the same confirmation as the menu, then leaves the route", async () => {
    vi.mocked(agentDelete).mockResolvedValue({});
    const onDeleted = vi.fn();
    render(
      <MemoryRouter>
        <DeleteAgentButton name="worker" onDeleted={onDeleted} />
      </MemoryRouter>,
      { api: remoteApi() },
    );
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));
    const dialog = screen.getByRole("alertdialog");
    expect(within(dialog).getByRole("heading", { name: "Delete agent worker?" })).toBeVisible();
    expect(agentDelete).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete agent" }));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
    expect(agentDelete).toHaveBeenCalledWith("worker", { force: true, purge: true });
  });
});

describe("Run once… in the menu", () => {
  const agentPost = vi.fn();
  beforeEach(() => {
    agentPost.mockReset();
  });

  function renderExec({ alive = true, image = "basic:latest" } = {}) {
    const refresh = vi.fn();
    const onExec = vi.fn();
    const api = fakeApi({ agentPost }, target);
    expect(api.target).toBe(target);
    render(
      <MemoryRouter>
        <AgentControls
          name="worker"
          alive={alive}
          image={image}
          primary="give"
          {...paths}
          refresh={refresh}
          onExec={onExec}
        />
      </MemoryRouter>,
      { api },
    );
    return { refresh, onExec };
  }

  const openRunOnce = async () => {
    openMenu();
    fireEvent.click(await screen.findByRole("menuitem", { name: "Run once…" }));
  };

  it.each([
    ["disabled", { alive: false }],
    ["bare", { image: "bare:latest" }],
  ])("is hidden for a %s agent", async (_label, opts) => {
    renderExec(opts);
    openMenu();
    await screen.findByRole("menuitem", { name: "Settings" });
    expect(screen.queryByRole("menuitem", { name: "Run once…" })).not.toBeInTheDocument();
  });

  it("sends the optional one-shot prompt from the modal and closes it", async () => {
    agentPost.mockResolvedValue({});
    const { refresh, onExec } = renderExec();
    await openRunOnce();
    const input = screen.getByPlaceholderText("one-shot prompt (optional)");
    fireEvent.change(input, { target: { value: "continue this task" } });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Run once" }));

    await waitFor(() =>
      expect(agentPost).toHaveBeenCalledWith("worker", "exec", { prompt: "continue this task" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(refresh).toHaveBeenCalledOnce();
    expect(onExec).toHaveBeenCalledOnce();
  });

  it("omits an empty prompt and prevents duplicate requests while pending", async () => {
    let resolveRequest!: (value: unknown) => void;
    agentPost.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveRequest = resolve;
        }),
    );
    renderExec();
    await openRunOnce();
    const submit = within(screen.getByRole("dialog")).getByRole("button", { name: "Run once" });

    fireEvent.click(submit);
    fireEvent.click(submit);

    expect(agentPost).toHaveBeenCalledOnce();
    expect(agentPost).toHaveBeenCalledWith("worker", "exec", undefined);
    expect(submit).toBeDisabled();
    resolveRequest({});
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("keeps the modal and the prompt after an API failure", async () => {
    agentPost.mockRejectedValue(new Error("iteration already running"));
    const { onExec } = renderExec();
    await openRunOnce();
    const input = screen.getByPlaceholderText("one-shot prompt (optional)");
    fireEvent.change(input, { target: { value: "retry me" } });
    const submit = within(screen.getByRole("dialog")).getByRole("button", { name: "Run once" });
    fireEvent.click(submit);

    await waitFor(() => expect(agentPost).toHaveBeenCalledOnce());
    await waitFor(() => expect(submit).toBeEnabled());
    expect(input).toHaveValue("retry me");
    expect(onExec).not.toHaveBeenCalled();
  });
});
