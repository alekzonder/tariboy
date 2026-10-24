import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ServerDialog } from "./ServerDialog";
import type { DaemonMeta } from "@/lib/daemons";
import * as desktop from "@/lib/desktop";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderDialog(server?: DaemonMeta) {
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(<ServerDialog open server={server} onSaved={onSaved} onOpenChange={onOpenChange} />);
  return { onSaved, onOpenChange };
}
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

describe("ServerDialog desktop SSH connect", () => {
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
    expect(screen.getByRole("radio", { name: "Install Tariboy" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Connect to existing Tariboy" })).not.toBeChecked();
  });

  it("connects a saved SSH host without provisioning in existing-Tariboy mode", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    const connect = vi.spyOn(desktop, "hostConnect").mockResolvedValue(null);
    const provision = vi
      .spyOn(desktop, "hostProvision")
      .mockResolvedValue({ operation_id: "op-1" });
    renderDialog();

    fireEvent.click(screen.getByRole("radio", { name: "Connect to existing Tariboy" }));
    expect(screen.getAllByTestId("host-progress-step").map((node) => node.textContent)).toEqual([
      expect.stringContaining("Connect to host"),
      expect.stringContaining("Check existing Tariboy"),
    ]);
    expect(screen.queryByText("Upload release")).toBeNull();
    expect(screen.queryByText("Install release")).toBeNull();
    expect(screen.queryByText("Start Tariboy")).toBeNull();

    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "gpu" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "gpu-box" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));

    await waitFor(() => expect(connect).toHaveBeenCalledWith("ssh-1"));
    expect(provision).not.toHaveBeenCalled();

    await events.state(
      nativeHost({
        state: "failed",
        phase: "connect",
        message: "Host key verification failed.",
      }),
    );
    expect(screen.getAllByTestId("host-progress-step")[0]).toHaveTextContent("failed");
    expect(screen.getAllByTestId("host-progress-step")[1]).toHaveTextContent("pending");

    await events.state(
      nativeHost({
        state: "ready",
        base_url: "http://127.0.0.1:18444",
        local_port: 18444,
        last_daemon_version: "99.0.0",
      }),
    );
    expect(screen.getByText("Host connected")).toBeInTheDocument();
    expect(provision).not.toHaveBeenCalled();
  });

  it("keeps an unavailable existing host and offers retry or installation", async () => {
    const events = mockNativeEvents();
    const save = vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    const connect = vi.spyOn(desktop, "hostConnect").mockResolvedValue(null);
    const provision = vi
      .spyOn(desktop, "hostProvision")
      .mockResolvedValue({ operation_id: "op-install" });
    renderDialog();

    fireEvent.click(screen.getByRole("radio", { name: "Connect to existing Tariboy" }));
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "gpu" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "gpu-box" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(connect).toHaveBeenCalledWith("ssh-1"));

    await events.state(
      nativeHost({
        state: "connecting",
        message: "remote daemon health timed out: connection refused; retrying in 1s",
      }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Existing Tariboy is unavailable at 127.0.0.1:9990.",
    );
    const retry = screen.getByRole("button", { name: "Retry connection" });
    expect(screen.getByRole("button", { name: "Install Tariboy" })).toBeInTheDocument();
    expect(save).toHaveBeenCalledTimes(1);
    expect(provision).not.toHaveBeenCalled();

    fireEvent.click(retry);
    await waitFor(() => expect(connect).toHaveBeenCalledTimes(2));
    expect(save).toHaveBeenLastCalledWith({
      id: "ssh-1",
      label: "gpu",
      ssh_alias: "gpu-box",
    });
    await events.state(
      nativeHost({
        state: "connecting",
        message: "remote daemon health timed out: connection refused; retrying in 2s",
      }),
    );

    const install = screen.getByRole("button", { name: "Install Tariboy" });
    fireEvent.click(install);
    await waitFor(() => expect(provision).toHaveBeenCalledWith("ssh-1"));
    expect(save).toHaveBeenLastCalledWith({
      id: "ssh-1",
      label: "gpu",
      ssh_alias: "gpu-box",
    });

    await events.output({
      operation_id: "op-install",
      host_id: "ssh-1",
      stream: "error",
      text: "installation failed",
      prompt: null,
    });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(provision).toHaveBeenCalledTimes(2));
    expect(connect).toHaveBeenCalledTimes(2);
  });
});

describe("ServerDialog desktop SSH provisioning and update", () => {
  beforeEach(() => vi.stubGlobal("__TAURI_INTERNALS__", {}));

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
    await events.state(
      nativeHost({
        state: "needs_auth",
        message: "authentication reply required",
      }),
    );

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
    const update = vi.spyOn(desktop, "hostUpdate").mockResolvedValue({ operation_id: "update-1" });
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
      screen
        .getAllByTestId("host-progress-step")
        .find((step) => step.textContent?.includes("Install release")),
    ).toHaveTextContent("failed");
    expect(screen.getByRole("button", { name: "Retry update" })).toBeInTheDocument();

    await events.state(
      nativeHost({
        state: "ready",
        phase: "connect",
        base_url: "http://127.0.0.1:18444",
      }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent("Update failed");
    expect(screen.getByRole("button", { name: "Retry update" })).toBeInTheDocument();
  });
});

describe("ServerDialog desktop SSH prerequisites", () => {
  beforeEach(() => vi.stubGlobal("__TAURI_INTERNALS__", {}));

  it("separates install blockers from optional agent tool prerequisites", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    vi.spyOn(desktop, "hostProvision").mockResolvedValue({ operation_id: "op-unsupported" });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "pi" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "pi-builder" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(desktop.hostProvision).toHaveBeenCalled());

    await events.output({
      operation_id: "op-unsupported",
      host_id: "ssh-1",
      stream: "stdout",
      text: JSON.stringify({
        platform: "Linux",
        arch: "armv7l",
        prerequisites: ["Linux x86_64/aarch64 or macOS arm64", "tmux", "codex"],
      }),
      prompt: null,
    });
    await events.state(
      nativeHost({
        state: "failed",
        message:
          "unsupported_host: remote installation requires Linux x86_64/aarch64 or macOS arm64",
      }),
    );

    expect(screen.getByText("Unsupported host: Linux/armv7l")).toBeInTheDocument();
    expect(screen.getByText("Install blocked")).toBeInTheDocument();
    expect(
      screen.getByText("Missing install requirements: Linux x86_64/aarch64 or macOS arm64"),
    ).toBeInTheDocument();
    expect(screen.getByText("Optional agent tools not found: tmux, codex")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "remote installation requires Linux x86_64/aarch64 or macOS arm64",
    );
  });

  it("installs on a Linux aarch64 server without blocking", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    vi.spyOn(desktop, "hostProvision").mockResolvedValue({ operation_id: "op-arm" });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "graviton" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "graviton" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(desktop.hostProvision).toHaveBeenCalled());

    await events.output({
      operation_id: "op-arm",
      host_id: "ssh-1",
      stream: "stdout",
      text: JSON.stringify({ platform: "Linux", arch: "aarch64", prerequisites: ["codex"] }),
      prompt: null,
    });

    expect(screen.queryByText(/Unsupported host/)).toBeNull();
    expect(screen.queryByText("Install blocked")).toBeNull();
    expect(screen.getByText("Optional agent tools not found: codex")).toBeInTheDocument();
  });

  it("installs on a macOS arm64 server without blocking", async () => {
    const events = mockNativeEvents();
    vi.spyOn(desktop, "hostSaveSsh").mockResolvedValue(nativeHost());
    vi.spyOn(desktop, "hostProvision").mockResolvedValue({ operation_id: "op-mac" });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "studio" } });
    fireEvent.change(screen.getByLabelText("SSH alias"), { target: { value: "studio" } });
    fireEvent.click(screen.getByRole("button", { name: "Add and connect" }));
    await waitFor(() => expect(desktop.hostProvision).toHaveBeenCalled());

    await events.output({
      operation_id: "op-mac",
      host_id: "ssh-1",
      stream: "stdout",
      text: JSON.stringify({ platform: "Darwin", arch: "arm64", prerequisites: ["codex"] }),
      prompt: null,
    });

    expect(screen.queryByText(/Unsupported host/)).toBeNull();
    expect(screen.queryByText("Install blocked")).toBeNull();
    expect(screen.getByText("Optional agent tools not found: codex")).toBeInTheDocument();
  });
});
