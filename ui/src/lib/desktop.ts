// The single seam between the SPA and the Tauri shell. Outside the desktop app
// (browser dev server, vitest) every call is a no-op returning null, so no page
// needs to know which host it is running in and no test needs a Tauri mock.
//
// The @tauri-apps/api imports are DYNAMIC on purpose: the module must be safe to
// import in a plain browser bundle, and lazy loading keeps the Tauri chunk out
// of the critical path.

/** Payload of the Rust `daemon_status` command (state.rs DaemonView). */
export interface DesktopDaemonState {
  /** starting = spawning/waiting, ready = answering, failed = start failed, down = was up and died. */
  state: "starting" | "ready" | "failed" | "down";
  /** http://127.0.0.1:PORT of the local daemon; "" when it has no listener. */
  base_url: string;
  /** Version reported by the RUNNING daemon ("" when unknown). */
  daemon_version: string;
  /** Version of the binaries this app bundles. */
  app_version: string;
  base_dir: string;
  pid: number;
  /** true when the app attached to a daemon it did not start. */
  adopted: boolean;
  /** Human-readable detail: the error for `failed`, otherwise "". */
  message: string;
}

export interface InstallCliResult {
  /** Result for the four-binary local install/update: created | already-installed | occupied. */
  outcome: string;
  link: string;
  target: string;
  /** Describes what already occupies the path when outcome is "occupied". */
  existing: string;
}

export interface DesktopHostView {
  id: string;
  label: string;
  kind: "ssh" | "https";
  ssh_alias: string;
  remote_install_dir: string;
  remote_port: number;
  https_base_url: string;
  last_daemon_version: string;
  state: "disconnected" | "connecting" | "provisioning" | "ready" | "degraded" | "needs_auth" | "failed";
  base_url: string;
  local_port: number;
  phase: string;
  platform: string;
  arch: string;
  prerequisites: string[];
  message: string;
}

export interface SaveSshHostInput {
  id?: string;
  label: string;
  ssh_alias: string;
  remote_install_dir?: string;
  remote_port?: number;
}

export interface SaveHttpsHostInput {
  id?: string;
  label: string;
  https_base_url: string;
}

export interface HostSessionCredentials {
  base_url: string;
  token: string;
}

export interface HostOperationResult {
  operation_id: string;
}

export interface SupportBundleResult {
  path: string;
}

export interface HostOutputEvent {
  operation_id: string;
  host_id: string;
  stream: "phase" | "stdout" | "stderr" | "prompt" | "result" | "error" | string;
  text: string;
  prompt?: string | null;
}

export interface TaskNotificationInput {
  host_id: string;
  notification_id: string;
  task_key: string;
  server_label: string;
  agent_name: string;
}

export interface TaskNotificationActivation {
  host_id: string;
  notification_id: string;
  task_key: string;
}

export interface TaskNotificationResult {
  outcome: "shown" | "denied" | "unavailable";
}

/** Tauri v2 injects __TAURI_INTERNALS__ into the webview before any app code runs. */
export function isDesktop(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

export async function invokeDesktop<T>(
  cmd: string,
  args?: Record<string, unknown>,
): Promise<T | null> {
  if (!isDesktop()) return null;
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<T>(cmd, args);
}

export const daemonState = () => invokeDesktop<DesktopDaemonState>("daemon_status");
export const daemonStart = () => invokeDesktop<DesktopDaemonState>("daemon_start");
export const daemonRestart = () => invokeDesktop<DesktopDaemonState>("daemon_restart");
export const daemonStop = () => invokeDesktop<DesktopDaemonState>("daemon_stop");
export const daemonLogTail = (lines = 40) => invokeDesktop<string>("daemon_log", { lines });
export const openDaemonLog = () => invokeDesktop<null>("open_daemon_log");
export const openHostPathInVSCode = (hostId: string, path: string) =>
  invokeDesktop<null>("open_host_path_in_vscode", { hostId, path });
export const openAgentCwdInVSCode = openHostPathInVSCode;
export const supportBundleExport = (hostId: string, includeAgentData: boolean) =>
  invokeDesktop<SupportBundleResult | null>("support_bundle_export", {
    hostId,
    includeAgentData,
  });
/**
 * Hand an absolute http(s) URL to the platform's browser through the native
 * `open_external_url` command, which re-validates the scheme before launching
 * an opener. Never use `window.open` or WebView navigation for terminal text:
 * the native command, not the WebView, is the authorization boundary.
 */
export const openExternalUrl = (url: string) =>
  invokeDesktop<null>("open_external_url", { url });
export const installCli = () => invokeDesktop<InstallCliResult>("install_cli");
export const hostsList = () => invokeDesktop<DesktopHostView[]>("hosts_list");
export const hostSaveSsh = (input: SaveSshHostInput) =>
  invokeDesktop<DesktopHostView>("host_save_ssh", { input });
export const hostSaveHttps = (input: SaveHttpsHostInput, token?: string) =>
  invokeDesktop<DesktopHostView>("host_save_https", { input, token });
export const hostSessionCredentials = (id: string) =>
  invokeDesktop<HostSessionCredentials>("host_session_credentials", { id });
export const hostHasToken = (id: string) =>
  invokeDesktop<boolean>("host_has_token", { id });
export const hostRemove = (id: string) => invokeDesktop<null>("host_remove", { id });
export const hostProvision = (id: string) =>
  invokeDesktop<HostOperationResult>("host_provision", { id });
export const hostConnect = (id: string) => invokeDesktop<null>("host_connect", { id });
export const hostUpdate = (id: string) =>
  invokeDesktop<HostOperationResult>("host_update", { id });
export const hostPromptReply = (operationId: string, text: string) =>
  invokeDesktop<null>("host_prompt_reply", { operationId, text });

export async function showTaskNotification(
  input: TaskNotificationInput,
): Promise<TaskNotificationResult> {
  return (await invokeDesktop<TaskNotificationResult>("task_notification_show", { input })) ?? {
    outcome: "unavailable",
  };
}

/**
 * Subscribe to `daemon://state`, which Rust emits on every lifecycle transition.
 * Returns a SYNCHRONOUS unsubscribe so callers can use it straight from a
 * useEffect cleanup; unsubscribing before the listener has been registered is
 * handled by the `cancelled` latch.
 */
export function onDaemonState(cb: (s: DesktopDaemonState) => void): () => void {
  return subscribeDesktop("daemon://state", cb);
}

export function onHostState(cb: (host: DesktopHostView) => void): () => void {
  return subscribeDesktop("host://state", cb);
}

export function onHostProvisionOutput(cb: (event: HostOutputEvent) => void): () => void {
  return subscribeDesktop("host://provision-output", cb);
}

export function onTaskNotificationActivated(
  cb: (activation: TaskNotificationActivation) => void,
): () => void {
  return subscribeDesktop("desktop://task-notification-activated", cb);
}

function subscribeDesktop<T>(event: string, cb: (payload: T) => void): () => void {
  if (!isDesktop()) return () => {};
  let un: (() => void) | null = null;
  let cancelled = false;
  void import("@tauri-apps/api/event")
    .then(({ listen }) =>
      listen<T>(event, (e) => cb(e.payload)).then((f) => {
        if (cancelled) f();
        else un = f;
      }),
    )
    .catch(() => {
      // The browser build and partially mocked desktop tests have no Tauri
      // event bridge. A missing bridge means no native events, not an
      // unhandled application error.
    });
  return () => {
    cancelled = true;
    un?.();
  };
}

/** Derived: the bundled binaries are newer/older than the daemon actually running. */
export function versionMismatch(s: DesktopDaemonState): boolean {
  return s.daemon_version !== "" && s.daemon_version !== s.app_version;
}

/** Derived: adopted a daemon started with `--http-addr ""`, so the UI cannot reach it. */
export function missingHTTPListener(s: DesktopDaemonState): boolean {
  return s.state === "ready" && s.base_url === "";
}
