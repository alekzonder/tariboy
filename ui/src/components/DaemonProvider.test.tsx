import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const model = vi.hoisted(() => ({
  host: {
    id: "ssh-1",
    label: "gpu",
    baseURL: "",
    kind: "ssh" as const,
    state: "disconnected" as string,
  },
  daemon: {
    id: "ssh-1",
    label: "gpu",
    baseURL: "",
    token: "",
  },
  listeners: [] as Array<(host: unknown) => void>,
  unsubscribe: vi.fn(),
  setActiveDaemon: vi.fn(),
  setActiveId: vi.fn(async (id: string) => { void id; }),
  resolveDaemon: vi.fn(),
  daemonState: vi.fn(),
}));

vi.mock("@/lib/daemons", () => ({
  listDaemons: async () => [model.host],
  getActiveId: async () => "ssh-1",
  peekActiveId: () => "ssh-1",
  setActiveId: (id: string) => model.setActiveId(id),
  resolveActive: async () => model.daemon,
  resolveDaemon: (id: string) => model.resolveDaemon(id),
  unresolvedDaemon: (id: string, label = id) => ({ id, label, baseURL: "", token: "" }),
}));

vi.mock("@/lib/api", () => ({
  setActiveDaemon: (daemon: unknown) => model.setActiveDaemon(daemon),
}));

vi.mock("@/lib/desktop", () => ({
  daemonState: () => model.daemonState(),
  onHostState: (listener: (host: unknown) => void) => {
    model.listeners.push(listener);
    return model.unsubscribe;
  },
}));

import { DaemonProvider, useDaemons } from "./DaemonProvider";

beforeEach(() => {
  model.host = {
    id: "ssh-1",
    label: "gpu",
    baseURL: "",
    kind: "ssh",
    state: "disconnected",
  };
  model.daemon = { id: "ssh-1", label: "gpu", baseURL: "", token: "" };
  model.listeners = [];
  model.unsubscribe.mockReset();
  model.setActiveDaemon.mockReset();
  model.setActiveId.mockReset();
  model.setActiveId.mockResolvedValue();
  model.resolveDaemon.mockReset();
  model.resolveDaemon.mockImplementation(async () => model.daemon);
  model.daemonState.mockReset();
  model.daemonState.mockResolvedValue({
    state: "ready",
    base_url: "http://127.0.0.1:9990",
    daemon_version: "0.11.5",
    app_version: "0.11.5",
    base_dir: "/tmp/tariboy",
    pid: 42,
    adopted: false,
    message: "",
  });
});

describe("DaemonProvider native host events", () => {
  it("exposes the current Desktop bundle version", async () => {
    function VersionProbe() {
      const { appVersion } = useDaemons();
      return <output>bundle {appVersion}</output>;
    }

    render(
      <DaemonProvider>
        <VersionProbe />
      </DaemonProvider>,
    );

    expect(await screen.findByText("bundle 0.11.5")).toBeInTheDocument();
  });

  it("re-resolves the selected host when its tunnel becomes ready and unsubscribes", async () => {
    const view = render(
      <DaemonProvider>
        <div>child</div>
      </DaemonProvider>,
    );
    await waitFor(() => expect(model.listeners).toHaveLength(1));

    model.host = { ...model.host, baseURL: "http://127.0.0.1:18444", state: "ready" };
    model.daemon = {
      id: "ssh-1",
      label: "gpu",
      baseURL: "http://127.0.0.1:18444",
      token: "",
    };
    await act(async () => model.listeners[0]({ id: "ssh-1", state: "ready" }));

    await waitFor(() =>
      expect(model.setActiveDaemon).toHaveBeenLastCalledWith(
        expect.objectContaining({ baseURL: "http://127.0.0.1:18444" }),
      ),
    );

    view.unmount();
    expect(model.unsubscribe).toHaveBeenCalledOnce();
  });

  it("keeps a known selected host outage out of the global registry alert", async () => {
    render(
      <DaemonProvider>
        <div>route content</div>
      </DaemonProvider>,
    );
    await waitFor(() => expect(model.listeners).toHaveLength(1));

    model.host = { ...model.host, baseURL: "", state: "disconnected" };
    model.daemon = { ...model.daemon, baseURL: "" };
    await act(async () => model.listeners[0]({ id: "ssh-1", state: "disconnected" }));

    await waitFor(() => expect(screen.getByText("route content")).toBeInTheDocument());
    expect(screen.queryByText(/Host registry:/)).toBeNull();
  });
});

function SelectionProbe() {
  const { activeId, select } = useDaemons();
  return (
    <>
      <output>{activeId}</output>
      <button onClick={() => void select("host-a")}>A</button>
      <button onClick={() => void select("host-b")}>B</button>
    </>
  );
}

it("does not let a slow older selection overwrite a newer host", async () => {
  let resolveA!: (value: typeof model.daemon) => void;
  let resolveB!: (value: typeof model.daemon) => void;
  model.resolveDaemon.mockImplementation((id: string) => new Promise((resolve) => {
    if (id === "host-a") resolveA = resolve;
    else if (id === "host-b") resolveB = resolve;
    else resolve(model.daemon);
  }));
  render(<DaemonProvider><SelectionProbe /></DaemonProvider>);
  await waitFor(() => expect(screen.getByText("ssh-1")).toBeInTheDocument());

  fireEvent.click(screen.getByRole("button", { name: "A" }));
  await waitFor(() => expect(resolveA).toBeTypeOf("function"));
  fireEvent.click(screen.getByRole("button", { name: "B" }));
  await act(async () => resolveA({ ...model.daemon, id: "host-a", label: "A" }));
  await waitFor(() => expect(resolveB).toBeTypeOf("function"));
  await act(async () => resolveB({ ...model.daemon, id: "host-b", label: "B" }));

  await waitFor(() => expect(screen.getByText("host-b")).toBeInTheDocument());
  expect(model.setActiveId).toHaveBeenCalledTimes(1);
  expect(model.setActiveId).toHaveBeenLastCalledWith("host-b");
  expect(model.setActiveDaemon).toHaveBeenLastCalledWith(expect.objectContaining({ id: "host-b" }));
});
