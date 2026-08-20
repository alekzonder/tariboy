import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { invoke } from "@tauri-apps/api/core";
import {
  isDesktop, invokeDesktop, daemonState, daemonStart, daemonRestart,
  daemonLogTail, installCli, hostsList, hostSaveSsh, hostSaveHttps,
  hostSessionCredentials, hostHasToken, hostRemove, onDaemonState,
  hostProvision, hostConnect, hostUpdate, hostPromptReply,
  openAgentCwdInVSCode, openExternalUrl,
  showTaskNotification, onTaskNotificationActivated,
  onHostState, onHostProvisionOutput,
  versionMismatch, missingHTTPListener,
  type DesktopDaemonState,
} from "./desktop";

// The only Tauri module desktop.ts loads dynamically. Mocking it lets the tests
// below assert the exact command name and payload without a Tauri host, and —
// just as importantly — prove the browser path never reaches it at all.
vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn(async () => null) }));

beforeEach(() => vi.mocked(invoke).mockClear());
afterEach(() => vi.unstubAllGlobals());

describe("desktop feature detection", () => {
  it("reports false in a plain browser/jsdom environment", () => {
    expect(isDesktop()).toBe(false);
  });

  it("reports true once Tauri injects its internals", () => {
    vi.stubGlobal("__TAURI_INTERNALS__", {});
    expect(isDesktop()).toBe(true);
  });
});

describe("desktop calls outside Tauri", () => {
  it("invokeDesktop resolves null instead of throwing", async () => {
    await expect(invokeDesktop("daemon_status")).resolves.toBeNull();
  });

  it("every daemon helper resolves null", async () => {
    await expect(daemonState()).resolves.toBeNull();
    await expect(daemonStart()).resolves.toBeNull();
    await expect(daemonRestart()).resolves.toBeNull();
    await expect(daemonLogTail()).resolves.toBeNull();
    await expect(openAgentCwdInVSCode("", "/work/agent")).resolves.toBeNull();
    await expect(installCli()).resolves.toBeNull();
    await expect(hostsList()).resolves.toBeNull();
    await expect(hostSaveSsh({ label: "dev", ssh_alias: "devbox" })).resolves.toBeNull();
    await expect(hostSaveHttps({ label: "prod", https_base_url: "https://prod" }, "token"))
      .resolves.toBeNull();
    await expect(hostSessionCredentials("host-id")).resolves.toBeNull();
    await expect(hostHasToken("host-id")).resolves.toBeNull();
    await expect(hostRemove("host-id")).resolves.toBeNull();
    await expect(hostProvision("host-id")).resolves.toBeNull();
    await expect(hostConnect("host-id")).resolves.toBeNull();
    await expect(hostUpdate("host-id")).resolves.toBeNull();
    await expect(hostPromptReply("operation-id", "123456")).resolves.toBeNull();
  });

  it("openExternalUrl resolves null in a browser without loading the Tauri API", async () => {
    await expect(openExternalUrl("https://example.test/a")).resolves.toBeNull();
    expect(invoke).not.toHaveBeenCalled();
  });

  it("onDaemonState never fires and returns a safe unsubscribe", () => {
    const cb = vi.fn();
    const off = onDaemonState(cb);
    expect(cb).not.toHaveBeenCalled();
    expect(() => off()).not.toThrow();
  });

  it("host listeners never fire and safely unsubscribe outside Tauri", () => {
    const state = vi.fn();
    const output = vi.fn();
    expect(() => onHostState(state)()).not.toThrow();
    expect(() => onHostProvisionOutput(output)()).not.toThrow();
    expect(state).not.toHaveBeenCalled();
    expect(output).not.toHaveBeenCalled();
  });

  it("task notifications are unavailable and activation safely unsubscribes outside Tauri", async () => {
    await expect(showTaskNotification({
      host_id: "remote-1",
      notification_id: "tn_1",
      task_key: "ASK-1",
      server_label: "production",
      agent_name: "alice",
    })).resolves.toEqual({ outcome: "unavailable" });

    const activated = vi.fn();
    const unsubscribe = onTaskNotificationActivated(activated);
    expect(activated).not.toHaveBeenCalled();
    expect(() => unsubscribe()).not.toThrow();
  });
});

describe("openExternalUrl inside Tauri", () => {
  beforeEach(() => vi.stubGlobal("__TAURI_INTERNALS__", {}));

  it("invokes only open_external_url with the url payload", async () => {
    await openExternalUrl("https://example.test/a?token=secret");
    expect(invoke).toHaveBeenCalledTimes(1);
    expect(invoke).toHaveBeenCalledWith("open_external_url", {
      url: "https://example.test/a?token=secret",
    });
  });
});

/** Builds a DesktopDaemonState with sensible defaults; override individual fields per test. */
function makeState(overrides: Partial<DesktopDaemonState> = {}): DesktopDaemonState {
  return {
    state: "ready",
    base_url: "http://127.0.0.1:9990",
    daemon_version: "1.2.3",
    app_version: "1.2.3",
    base_dir: "/tmp/base",
    pid: 1234,
    adopted: false,
    message: "",
    ...overrides,
  };
}

describe("versionMismatch", () => {
  it("is false when daemon_version equals app_version", () => {
    expect(versionMismatch(makeState({ daemon_version: "1.2.3", app_version: "1.2.3" }))).toBe(false);
  });

  it("is false when daemon_version is unknown (empty string)", () => {
    expect(versionMismatch(makeState({ daemon_version: "", app_version: "1.2.3" }))).toBe(false);
  });

  it("is true when daemon_version differs from app_version", () => {
    expect(versionMismatch(makeState({ daemon_version: "1.2.3", app_version: "1.3.0" }))).toBe(true);
  });
});

describe("missingHTTPListener", () => {
  it("is true when state is ready and base_url is empty", () => {
    expect(missingHTTPListener(makeState({ state: "ready", base_url: "" }))).toBe(true);
  });

  it("is false when a base_url is present", () => {
    expect(missingHTTPListener(makeState({ state: "ready", base_url: "http://127.0.0.1:9990" }))).toBe(false);
  });

  it("is false when starting even with an empty base_url", () => {
    expect(missingHTTPListener(makeState({ state: "starting", base_url: "" }))).toBe(false);
  });

  it("is false when failed even with an empty base_url", () => {
    expect(missingHTTPListener(makeState({ state: "failed", base_url: "" }))).toBe(false);
  });

  it("is false when down even with an empty base_url", () => {
    expect(missingHTTPListener(makeState({ state: "down", base_url: "" }))).toBe(false);
  });
});
