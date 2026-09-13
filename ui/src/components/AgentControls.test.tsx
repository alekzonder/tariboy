import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { agentDeleteOn } from "@/lib/api";
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
