import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { agentDeleteOn } from "@/lib/api";
import * as api from "@/lib/api";
import { AgentControls } from "./AgentControls";

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();
  return { ...actual, agentDeleteOn: vi.fn() };
});

const target = { id: "remote", label: "Remote", baseURL: "https://remote.test", token: "secret" };

// Block body on purpose: an arrow returning the mock hands vitest a "cleanup
// function" that is the mock itself, which it then calls and awaits.
beforeEach(() => { vi.mocked(agentDeleteOn).mockReset(); });
afterEach(() => vi.restoreAllMocks());

function renderControls(alive: boolean) {
  return render(
    <MemoryRouter>
      <AgentControls
        target={null}
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
    render(
      <MemoryRouter>
        <AgentControls
          target={target}
          name="worker"
          alive
          configurationPath="/agents/remote/worker/configuration"
          refresh={refresh}
          onDeleted={onDeleted}
        />
      </MemoryRouter>,
    );
    return { onDeleted, refresh };
  }

  it("opens an in-app confirmation and cancels without deleting", async () => {
    renderWithDelete();
    await openDelete();

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Delete agent worker?" })).toBeInTheDocument();
    expect(agentDeleteOn).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(agentDeleteOn).not.toHaveBeenCalled();
  });

  it("deletes once with force and purge, then leaves the route", async () => {
    vi.mocked(agentDeleteOn).mockResolvedValue({});
    const { onDeleted, refresh } = renderWithDelete();
    await openDelete();
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    await waitFor(() => expect(agentDeleteOn).toHaveBeenCalledOnce());
    expect(agentDeleteOn).toHaveBeenCalledWith(target, "worker", { force: true, purge: true });
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("disables dismissal and duplicate submission while deletion is pending", async () => {
    let resolveDelete!: (value: unknown) => void;
    vi.mocked(agentDeleteOn).mockImplementation(() => new Promise((resolve) => { resolveDelete = resolve; }));
    const { onDeleted } = renderWithDelete();
    await openDelete();
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    const pending = screen.getByRole("button", { name: "Deleting…" });
    expect(pending).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(agentDeleteOn).toHaveBeenCalledOnce();

    resolveDelete({});
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
  });

  it("keeps the dialog open and does not leave the route after a deletion failure", async () => {
    vi.mocked(agentDeleteOn).mockImplementation(() => Promise.reject(new Error("delete failed")));
    const { onDeleted } = renderWithDelete();
    await openDelete();
    fireEvent.click(screen.getByRole("button", { name: "Delete agent" }));

    await waitFor(() => expect(agentDeleteOn).toHaveBeenCalledOnce());
    await waitFor(() => expect(screen.getByRole("button", { name: "Delete agent" })).toBeEnabled());
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(onDeleted).not.toHaveBeenCalled();
  });
});

describe("Exec in the header", () => {
  let agentPostOn: ReturnType<typeof vi.spyOn<typeof api, "agentPostOn">>;
  beforeEach(() => { agentPostOn = vi.spyOn(api, "agentPostOn"); });

  function renderExec({ alive = true, image = "basic:latest" } = {}) {
    const refresh = vi.fn();
    const onExec = vi.fn();
    render(
      <MemoryRouter>
        <AgentControls
          target={target}
          name="worker"
          alive={alive}
          image={image}
          configurationPath="/agents/remote/worker/configuration"
          refresh={refresh}
          onExec={onExec}
        />
      </MemoryRouter>,
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
    agentPostOn.mockResolvedValue({});
    const { refresh, onExec } = renderExec();
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));
    const input = screen.getByPlaceholderText("one-shot prompt (optional)");
    fireEvent.change(input, { target: { value: "continue this task" } });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Exec" }));

    await waitFor(() => expect(agentPostOn).toHaveBeenCalledWith(
      target, "worker", "exec", { prompt: "continue this task" },
    ));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(refresh).toHaveBeenCalledOnce();
    expect(onExec).toHaveBeenCalledOnce();
  });

  it("omits an empty prompt and prevents duplicate requests while pending", async () => {
    let resolveRequest!: (value: unknown) => void;
    agentPostOn.mockImplementation(() => new Promise((resolve) => { resolveRequest = resolve; }));
    renderExec();
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));
    const submit = within(screen.getByRole("dialog")).getByRole("button", { name: "Exec" });

    fireEvent.click(submit);
    fireEvent.click(submit);

    expect(agentPostOn).toHaveBeenCalledOnce();
    expect(agentPostOn).toHaveBeenCalledWith(target, "worker", "exec", undefined);
    expect(submit).toBeDisabled();
    resolveRequest({});
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("keeps the modal and the prompt after an API failure", async () => {
    agentPostOn.mockRejectedValue(new Error("iteration already running"));
    const { onExec } = renderExec();
    fireEvent.click(screen.getByRole("button", { name: "Exec" }));
    const input = screen.getByPlaceholderText("one-shot prompt (optional)");
    fireEvent.change(input, { target: { value: "retry me" } });
    const submit = within(screen.getByRole("dialog")).getByRole("button", { name: "Exec" });
    fireEvent.click(submit);

    await waitFor(() => expect(agentPostOn).toHaveBeenCalledOnce());
    await waitFor(() => expect(submit).toBeEnabled());
    expect(input).toHaveValue("retry me");
    expect(onExec).not.toHaveBeenCalled();
  });
});
