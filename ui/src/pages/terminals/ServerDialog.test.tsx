import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ServerDialog } from "./ServerDialog";
import { addDaemon, listDaemons, getDaemonToken, type DaemonMeta } from "@/lib/daemons";
import * as desktop from "@/lib/desktop";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function seed(token = "tok"): Promise<DaemonMeta> {
  return addDaemon({ label: "prod", baseURL: "http://127.0.0.1:9992", token });
}

function renderDialog(server?: DaemonMeta) {
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(<ServerDialog open server={server} onSaved={onSaved} onOpenChange={onOpenChange} />);
  return { onSaved, onOpenChange };
}

describe("ServerDialog", () => {
  it("adds a new server with the typed token", async () => {
    const { onSaved, onOpenChange } = renderDialog();
    expect(screen.getByText("Add host")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("label"), { target: { value: "  prod  " } });
    fireEvent.change(screen.getByLabelText("base URL"), { target: { value: "http://h:9990/" } });
    fireEvent.change(screen.getByLabelText("token"), { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const [meta] = await listDaemons();
    expect(meta.label).toBe("prod");
    expect(meta.baseURL).toBe("http://h:9990");
    expect(await getDaemonToken(meta.id)).toBe("secret");
    expect(onSaved).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("rejects an empty label and a non-http base URL", async () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(screen.getByText("label is required")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("label"), { target: { value: "prod" } });
    fireEvent.change(screen.getByLabelText("base URL"), { target: { value: "h:9990" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(screen.getByText("base URL must start with http")).toBeInTheDocument();
    expect(await listDaemons()).toHaveLength(0);
  });

  it("seeds the form from the edited server and never renders the token back", async () => {
    const meta = await seed();
    renderDialog(meta);

    expect(screen.getByText("Edit host")).toBeInTheDocument();
    await waitFor(() =>
      expect((screen.getByLabelText("label") as HTMLInputElement).value).toBe("prod"),
    );
    expect((screen.getByLabelText("base URL") as HTMLInputElement).value).toBe("http://127.0.0.1:9992");
    expect((screen.getByLabelText("token") as HTMLInputElement).value).toBe("");
    expect(await screen.findByText("token set for this session")).toBeInTheDocument();
  });

  it("keeps the stored token when the token field is left blank", async () => {
    const meta = await seed();
    const { onSaved } = renderDialog(meta);

    await waitFor(() =>
      expect((screen.getByLabelText("label") as HTMLInputElement).value).toBe("prod"),
    );
    fireEvent.change(screen.getByLabelText("label"), { target: { value: "renamed" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    const saved = await listDaemons();
    expect(saved).toHaveLength(1);
    expect(saved[0].id).toBe(meta.id);
    expect(saved[0].label).toBe("renamed");
    expect(await getDaemonToken(meta.id)).toBe("tok");
  });

  it("overwrites the token when one is typed", async () => {
    const meta = await seed();
    const { onSaved } = renderDialog(meta);

    await waitFor(() =>
      expect((screen.getByLabelText("label") as HTMLInputElement).value).toBe("prod"),
    );
    fireEvent.change(screen.getByLabelText("token"), { target: { value: "fresh" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(await getDaemonToken(meta.id)).toBe("fresh");
  });

  it("clears the token on demand and reports it as unset", async () => {
    const meta = await seed();
    const { onSaved } = renderDialog(meta);

    fireEvent.click(await screen.findByRole("button", { name: "Clear token" }));

    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(await getDaemonToken(meta.id)).toBe("");
    expect(await screen.findByText("no token set")).toBeInTheDocument();
    // Clearing must not drop the registry entry itself.
    expect(await listDaemons()).toHaveLength(1);
  });

  it("surfaces credential-store errors while opening the editor", async () => {
    const meta = await seed();
    const getItem = Storage.prototype.getItem;
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (this: Storage, key: string) {
      if (this === sessionStorage && key.endsWith(meta.id)) throw new Error("credential store unavailable");
      return getItem.call(this, key);
    });

    renderDialog(meta);

    expect(await screen.findByText(/credential store unavailable/)).toBeInTheDocument();
  });
});

function nativeHost(overrides: Partial<desktop.DesktopHostView> = {}): desktop.DesktopHostView {
  return {
    id: "ssh-1",
    label: "gpu",
    kind: "ssh",
    ssh_alias: "gpu-box",
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
    ...overrides,
  };
}

function mockNativeEvents() {
  let stateListener: ((host: desktop.DesktopHostView) => void) | undefined;
  let outputListener: ((event: desktop.HostOutputEvent) => void) | undefined;
  vi.spyOn(desktop, "onHostState").mockImplementation((listener) => {
    stateListener = listener;
    return () => {};
  });
  vi.spyOn(desktop, "onHostProvisionOutput").mockImplementation((listener) => {
    outputListener = listener;
    return () => {};
  });
  return {
    state: (host: desktop.DesktopHostView) => act(() => stateListener?.(host)),
    output: (event: desktop.HostOutputEvent) => act(() => outputListener?.(event)),
  };
}

describe("ServerDialog desktop SSH flow", () => {
  beforeEach(() => vi.stubGlobal("__TAURI_INTERNALS__", {}));

  it("defaults to Label and SSH alias while keeping Advanced HTTPS available", () => {
    vi.spyOn(desktop, "onHostState").mockReturnValue(() => {});
    vi.spyOn(desktop, "onHostProvisionOutput").mockReturnValue(() => {});
    renderDialog();

    expect(screen.getByText("Add host")).toBeInTheDocument();
    expect(screen.getByLabelText("Label")).toBeInTheDocument();
    expect(screen.getByLabelText("SSH alias")).toBeInTheDocument();
    expect(screen.queryByLabelText("Base URL")).toBeNull();
    expect(screen.getByRole("button", { name: "Advanced HTTPS" })).toBeInTheDocument();
  });

  it("renders one linear provisioning flow and keeps diagnostic output collapsed", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    vi.spyOn(desktop, "hostProvision").mockResolvedValue({ operation_id: "op-1" });
    const reply = vi.spyOn(desktop, "hostPromptReply").mockResolvedValue(null);
    renderDialog();

    expect(screen.getAllByTestId("host-progress-step").map((node) => node.textContent)).toEqual([
      expect.stringContaining("Connect to host"),
      expect.stringContaining("Check server"),
      expect.stringContaining("Upload release"),
      expect.stringContaining("Install release"),
      expect.stringContaining("Start Tariboy"),
      expect.stringContaining("Connect to Tariboy"),
    ]);

    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "gpu" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "gpu-box" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(desktop.hostProvision).toHaveBeenCalledWith("ssh-1"));

    await events.output({
      operation_id: "op-1",
      host_id: "ssh-1",
      stream: "stderr",
      text: "tariboyd: OK",
      prompt: null,
    });
    await events.output({
      operation_id: "op-1",
      host_id: "ssh-1",
      stream: "prompt",
      text: "Verification code:",
      prompt: "authentication",
    });

    const details = screen.getByText("Technical details").closest("details");
    expect(details).not.toHaveAttribute("open");
    expect(details).toHaveTextContent("tariboyd: OK");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole("button", { name: "Connecting…" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("SSH reply"), { target: { value: "123456" } });
    fireEvent.click(screen.getByRole("button", { name: "Send reply" }));
    await waitFor(() => expect(reply).toHaveBeenCalledWith("op-1", "123456"));
  });

  it("runs update as one uninterrupted operation even when running work is reported", async () => {
    const events = mockNativeEvents();
    const update = vi.spyOn(desktop, "hostUpdate")
      .mockResolvedValue({ operation_id: "update-1" });
    const server: DaemonMeta = {
      id: "ssh-1",
      label: "gpu",
      baseURL: "http://127.0.0.1:18444",
      kind: "ssh",
      state: "ready",
      sshAlias: "gpu-box",
    };
    renderDialog(server);

    expect(await screen.findByRole("button", { name: "Update Tariboy" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save and reconnect" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "gpu renamed" } });
    expect(screen.getByRole("button", { name: "Save and reconnect" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update Tariboy" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "gpu" } });

    fireEvent.click(screen.getByRole("button", { name: "Update Tariboy" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith("ssh-1"));
    expect(update).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Updating…" })).toBeDisabled();
    await events.output({
      operation_id: "update-1",
      host_id: "ssh-1",
      stream: "result",
      text: JSON.stringify([
        { agent: "researcher", state: "running", running_iterations: ["it-1"] },
        { agent: "writer", state: "idle", running_iterations: [] },
      ]),
      prompt: null,
    });

    expect(screen.queryByText("Running work must be confirmed")).toBeNull();
    expect(screen.queryByRole("button", { name: "Continue update" })).toBeNull();
    expect(screen.getByRole("button", { name: "Updating…" })).toBeDisabled();
    expect(update).toHaveBeenCalledTimes(1);
  });

  it("keeps a failed update actionable after rollback reconnects the old daemon", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostUpdate").mockResolvedValue({ operation_id: "update-1" });
    const server: DaemonMeta = {
      id: "ssh-1",
      label: "gpu",
      baseURL: "http://127.0.0.1:18444",
      kind: "ssh",
      state: "ready",
      sshAlias: "gpu-box",
    };
    renderDialog(server);

    fireEvent.click(await screen.findByRole("button", { name: "Update Tariboy" }));
    await waitFor(() => expect(desktop.hostUpdate).toHaveBeenCalled());
    await events.output({
      operation_id: "update-1",
      host_id: "ssh-1",
      stream: "phase",
      text: "stage_release",
      prompt: null,
    });
    await events.output({
      operation_id: "update-1",
      host_id: "ssh-1",
      stream: "stderr",
      text: "tariboyd: OK\ntariboy: OK",
      prompt: null,
    });
    await events.output({
      operation_id: "update-1",
      host_id: "ssh-1",
      stream: "error",
      text: JSON.stringify({
        code: "ssh_failed",
        message: "tariboyd: OK\nrefusing to replace non-symlink /home/me/.local/bin/tariboyd",
      }),
      prompt: null,
    });

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Update failed");
    expect(alert).toHaveTextContent(
      "/home/me/.local/bin/tariboyd is an existing file and cannot be replaced automatically. Move it out of the way and try again.",
    );
    expect(alert).not.toHaveTextContent("ssh_failed");
    expect(alert).not.toHaveTextContent("tariboyd: OK");
    expect(
      screen.getAllByTestId("host-progress-step").find((step) =>
        step.textContent?.includes("Install release")),
    ).toHaveTextContent("failed");
    expect(screen.getByRole("button", { name: "Retry update" })).toBeInTheDocument();

    await events.state(nativeHost({
      state: "ready",
      phase: "connect",
      base_url: "http://127.0.0.1:18444",
    }));

    expect(screen.getByRole("alert")).toHaveTextContent("Update failed");
    expect(screen.getByRole("button", { name: "Retry update" })).toBeInTheDocument();
  });

  it("separates install blockers from optional agent tool prerequisites", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    vi.spyOn(desktop, "hostProvision").mockResolvedValue({ operation_id: "op-unsupported" });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "mac" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "mac-builder" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(desktop.hostProvision).toHaveBeenCalled());

    await events.output({
      operation_id: "op-unsupported",
      host_id: "ssh-1",
      stream: "stdout",
      text: JSON.stringify({
        platform: "Darwin",
        arch: "arm64",
        prerequisites: ["Linux", "x86_64", "tmux", "codex"],
      }),
      prompt: null,
    });
    await events.state(nativeHost({
      state: "failed",
      message: "unsupported_host: remote installation requires Linux x86_64",
    }));

    expect(screen.getByText("Unsupported host: Darwin/arm64")).toBeInTheDocument();
    expect(screen.getByText("Install blocked")).toBeInTheDocument();
    expect(screen.getByText("Missing install requirements: Linux, x86_64")).toBeInTheDocument();
    expect(screen.getByText("Optional agent tools not found: tmux, codex")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "remote installation requires Linux x86_64",
    );
  });
});
