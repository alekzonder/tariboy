import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { render, screen, fireEvent, waitFor, within } from "@/test/queryRender";
import { fakeApi } from "@/test/api";
import { AgentControls } from "./AgentControls";

const target = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" };

const agentDelete = vi.fn();
// The client of the route host: every delete goes through it, never the active daemon.
const remoteApi = () => fakeApi({ agents: { agentDelete } }, target);

// Block body on purpose: an arrow returning the mock hands vitest a "cleanup
// function" that is the mock itself, which it then calls and awaits.
beforeEach(() => { agentDelete.mockReset(); });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function renderControls(alive: boolean) {
  return render(
    <MemoryRouter>
      <AgentControls
        name="alpha"
        alive={alive}
        configurationPath="/agents/local/alpha/configuration"
      />
    </MemoryRouter>,
  );
}

it("posts the run-state action of the toggle to the route host", async () => {
  const calls: Array<{ method: string; path: string }> = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string, init?: RequestInit) => {
    calls.push({ method: init?.method ?? "GET", path });
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }) } as Response);
  }));
  const { unmount } = renderControls(false);

  fireEvent.click(screen.getByRole("button", { name: "Start" }));
  await waitFor(() => expect(calls.some((c) => c.path === "/api/agents/alpha/start" && c.method === "POST")).toBe(true));
  unmount();

  renderControls(true);
  fireEvent.click(screen.getByRole("button", { name: "Stop" }));
  await waitFor(() => expect(calls.some((c) => c.path === "/api/agents/alpha/stop" && c.method === "POST")).toBe(true));
});

it("confirms before killing the session and does nothing when the confirm is declined", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation((path: string) => {
    calls.push(path);
    return Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }) } as Response);
  }));
  vi.stubGlobal("confirm", vi.fn().mockReturnValue(false));
  renderControls(true);

  fireEvent.click(screen.getByRole("button", { name: "Kill" }));
  expect(calls).not.toContain("/api/agents/alpha/kill");

  vi.mocked(window.confirm).mockReturnValue(true);
  fireEvent.click(screen.getByRole("button", { name: "Kill" }));
  await waitFor(() => expect(calls).toContain("/api/agents/alpha/kill"));
});

describe("delete from the overflow menu", () => {
  // Radix opens a DropdownMenu on pointerdown, which jsdom's fireEvent.click
  // does not synthesize — drive it from the keyboard instead.
  const openDelete = async () => {
    fireEvent.keyDown(screen.getByRole("button", { name: "Agent settings" }), { key: "Enter" });
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
          configurationPath="/agents/remote/worker/configuration"
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
    vi.mocked(agentDelete).mockImplementation(() => new Promise((resolve) => { resolveDelete = resolve; }));
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

describe("Exec in the header", () => {
  const agentPost = vi.fn();
  beforeEach(() => { agentPost.mockReset(); });

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
          configurationPath="/agents/remote/worker/configuration"
          refresh={refresh}
          onExec={onExec}
        />
      </MemoryRouter>,
      { api },
    );
    return { refresh, onExec };
  }

  it("is shown before Stop only for an enabled agent that is not bare", () => {
    renderExec();
    const buttons = screen.getAllByRole("button").map((b) => b.textContent);
    expect(buttons.indexOf("Exec")).toBeGreaterThanOrEqual(0);
    expect(buttons.indexOf("Exec")).toBeLessThan(buttons.indexOf("Stop"));
  });

  it.each([
    ["disabled", { alive: false }],
    ["bare", { image: "bare:latest" }],
  ])("is hidden for a %s agent", (_label, opts) => {
    renderExec(opts);
    expect(screen.queryByRole("button", { name: "Exec" })).not.toBeInTheDocument();
  });

  it("sends the optional one-shot prompt from the modal and closes it", async () => {
    agentPost.mockResolvedValue({});
    const { refresh, onExec } = renderExec();
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));
    const input = screen.getByPlaceholderText("one-shot prompt (optional)");
    fireEvent.change(input, { target: { value: "continue this task" } });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Exec" }));

    await waitFor(() => expect(agentPost).toHaveBeenCalledWith(
      "worker", "exec", { prompt: "continue this task" },
    ));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(refresh).toHaveBeenCalledOnce();
    expect(onExec).toHaveBeenCalledOnce();
  });

  it("omits an empty prompt and prevents duplicate requests while pending", async () => {
    let resolveRequest!: (value: unknown) => void;
    agentPost.mockImplementation(() => new Promise((resolve) => { resolveRequest = resolve; }));
    renderExec();
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));
    const submit = within(screen.getByRole("dialog")).getByRole("button", { name: "Exec" });

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
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));
    const input = screen.getByPlaceholderText("one-shot prompt (optional)");
    fireEvent.change(input, { target: { value: "retry me" } });
    const submit = within(screen.getByRole("dialog")).getByRole("button", { name: "Exec" });
    fireEvent.click(submit);

    await waitFor(() => expect(agentPost).toHaveBeenCalledOnce());
    await waitFor(() => expect(submit).toBeEnabled());
    expect(input).toHaveValue("retry me");
    expect(onExec).not.toHaveBeenCalled();
  });
});
