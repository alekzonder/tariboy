import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HostStatus } from "./HostStatus";
import type { DaemonMeta } from "@/lib/daemons";

function host(overrides: Partial<DaemonMeta> = {}): DaemonMeta {
  return {
    id: "ssh-1",
    label: "gpu",
    baseURL: "",
    kind: "ssh",
    state: "disconnected",
    sshAlias: "gpu-box",
    prerequisites: [],
    ...overrides,
  };
}

describe("HostStatus", () => {
  it("renders the native state and offers connect for a disconnected SSH host", () => {
    const connect = vi.fn();
    render(<HostStatus host={host()} appVersion="0.11.5" onConnect={connect} />);

    expect(screen.getByText("disconnected")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Connect gpu" }));
    expect(connect).toHaveBeenCalledOnce();
  });

  it("names every missing harness and tmux prerequisite exactly", () => {
    render(
      <HostStatus
        host={host({
          state: "degraded",
          platform: "Linux",
          arch: "x86_64",
          prerequisites: ["tmux", "claude", "codex", "opencode"],
        })}
        appVersion="0.11.5"
      />,
    );

    expect(screen.getByText("Missing: tmux, claude, codex, opencode")).toBeInTheDocument();
  });

  it("blocks install on unsupported OS/arch and shows both values", () => {
    render(
      <HostStatus
        host={host({
          state: "failed",
          platform: "Darwin",
          arch: "arm64",
          prerequisites: ["Linux", "x86_64"],
          message: "unsupported_host",
        })}
        appVersion="0.11.5"
      />,
    );

    expect(screen.getByText("Unsupported host: Darwin/arm64")).toBeInTheDocument();
    expect(screen.getByText("Install blocked")).toBeInTheDocument();
  });

  it("blocks install when python3 is missing", () => {
    render(
      <HostStatus
        host={host({
          state: "failed",
          platform: "Linux",
          arch: "x86_64",
          prerequisites: ["python3"],
        })}
        appVersion="0.11.5"
      />,
    );

    expect(screen.getByText("Install blocked")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect gpu" })).toBeNull();
  });

  it("never offers an insecure bypass for host-key mismatch", () => {
    render(
      <HostStatus
        host={host({
          state: "failed",
          message: "host_key_mismatch: SSH host key mismatch",
        })}
        appVersion="0.11.5"
      />,
    );

    expect(screen.getByText(/SSH host key mismatch/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /ignore|bypass|insecure/i })).toBeNull();
  });

  it("offers an explicit update action for a confirmed version mismatch", () => {
    const update = vi.fn();
    render(
      <HostStatus
        host={host({
          state: "ready",
          baseURL: "http://127.0.0.1:18444",
          lastDaemonVersion: "0.11.4",
        })}
        appVersion="0.11.5"
        onUpdate={update}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Update gpu" }));
    expect(update).toHaveBeenCalledOnce();
  });

  it.each([
    ["matching", "0.11.5", "0.11.5"],
    ["unknown app", "", "0.11.4"],
    ["unknown remote", "0.11.5", ""],
  ])("does not offer update for %s version state", (_name, appVersion, remoteVersion) => {
    render(
      <HostStatus
        host={host({
          state: "ready",
          baseURL: "http://127.0.0.1:18444",
          lastDaemonVersion: remoteVersion,
        })}
        appVersion={appVersion}
        onUpdate={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Update gpu" })).toBeNull();
  });

  it("reopens the live authentication UI instead of starting another connection", () => {
    const connect = vi.fn();
    const authenticate = vi.fn();
    render(
      <HostStatus
        host={host({ state: "needs_auth" })}
        appVersion="0.11.5"
        onConnect={connect}
        onUpdate={authenticate}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Authenticate gpu" }));
    expect(authenticate).toHaveBeenCalledOnce();
    expect(connect).not.toHaveBeenCalled();
  });

  it.each(["provisioning", "connecting"] as const)(
    "does not offer a second action while the host is %s",
    (state) => {
      render(
        <HostStatus
          host={host({ state })}
          appVersion="0.11.5"
          onConnect={vi.fn()}
          onUpdate={vi.fn()}
        />,
      );

      expect(screen.queryByRole("button")).toBeNull();
    },
  );
});
