import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { constants } from "node:fs";
import { access, chmod, mkdir, mkdtemp, open, readFile, readdir, readlink, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { delimiter, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test as base } from "playwright/test";

import { W3CClient } from "./w3c";

interface DesktopFixtures {
  desktop: W3CClient;
  desktopWorker: DesktopWorker;
  /** Optional request/group snapshots written to the fixture database before
   * the Desktop-owned daemon starts. */
  usageSeed: UsageSeed | undefined;
}

export interface UsageSeed {
  groups: Array<{ name: string; lead: string; agent: string }>;
  requests: Array<{
    id: string;
    ts: string;
    agent: string;
    model: string;
    inputTokens: number;
    outputTokens: number;
    costUSD: number;
    group?: string;
  }>;
}

interface DesktopWorker {
  client: W3CClient;
  log: TailBuffer;
  /** Isolated base directory owned and removed by this test fixture. */
  baseDir: string;
  /** Isolated runtime directory owned and removed by this test fixture. */
  runtimeDir: string;
  /** Registers an agent whose active iteration this fixture must kill before
   * stopping its daemon. Daemon shutdown intentionally preserves shims. */
  registerAgentForCleanup: (name: string) => void;
  /** Owner-only file below this fixture's private root that the Desktop app
   * appends one serialized URL to for every external open it accepts. Setting
   * it also replaces the platform opener, so an E2E run never launches a
   * browser. Removed with the root. */
  externalOpenLog: string;
}

const MAX_LOG_BYTES = 64 * 1024;
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const application = join(repositoryRoot, "desktop/src-tauri/target/debug/tariboy-desktop");
const bundledBin = join(repositoryRoot, "desktop/src-tauri/resources/bin/linux-x86_64");
const daemon = join(bundledBin, "tariboyd");
const cli = join(bundledBin, "tariboy");
const daemonWrapper = join(repositoryRoot, "scripts/desktop-e2e-daemon.sh");

export class TailBuffer {
  private value = Buffer.alloc(0);

  constructor(private readonly capacity = MAX_LOG_BYTES) {}

  append(chunk: string | Buffer): void {
    const incoming = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
    const combined = Buffer.concat([this.value, incoming]);
    this.value = combined.length <= this.capacity ? combined : combined.subarray(combined.length - this.capacity);
  }

  bytes(): Buffer {
    return Buffer.from(this.value);
  }

  toString(): string {
    return this.value.toString();
  }
}

export async function waitForMainWindow(desktop: W3CClient): Promise<void> {
  await expect.poll(() => desktop.execute<boolean>(
    `return typeof window.__TAURI_INTERNALS__ === "object"
      && document.body?.innerText.includes("New agent") === true`,
  )).toBe(true);
  await expect.poll(() => desktop.execute<boolean>(`
    if (window.__tariboyDaemonReady === true) return true;
    if (window.__tariboyDaemonProbePending !== true) {
      window.__tariboyDaemonProbePending = true;
      window.__TAURI_INTERNALS__.invoke("daemon_status")
        .then((status) => {
          window.__tariboyDaemonReady = typeof status?.base_url === "string"
            && status.base_url.startsWith("http://127.0.0.1:");
        })
        .catch(() => { window.__tariboyDaemonReady = false; })
        .finally(() => { window.__tariboyDaemonProbePending = false; });
    }
    return false;
  `), { timeout: 30_000 }).toBe(true);
}

function executable(name: string, override?: string): string {
  const candidates = override
    ? [override]
    : (process.env.PATH ?? "").split(delimiter).filter(Boolean).map((directory) => join(directory, name));
  for (const candidate of candidates) {
    if (spawnSync("test", ["-x", candidate], { stdio: "ignore" }).status === 0) return candidate;
  }
  throw new Error(`${name} is required for Desktop E2E tests${override ? `: ${override}` : " (not found in PATH)"}`);
}

async function assertExecutable(path: string, label: string): Promise<void> {
  try {
    await access(path, constants.X_OK);
  } catch {
    throw new Error(`${label} is missing or not executable: ${path}`);
  }
}

async function freePort(): Promise<number> {
  const server = createServer();
  await new Promise<void>((resolveListen, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolveListen);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("cannot allocate loopback port");
  const port = address.port;
  await new Promise<void>((resolveClose, reject) => server.close((error) => error ? reject(error) : resolveClose()));
  return port;
}

async function waitFor(description: string, timeoutMs: number, operation: () => Promise<boolean>): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastError: unknown;
  while (Date.now() < deadline) {
    try {
      if (await operation()) return;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 100));
  }
  const suffix = lastError instanceof Error ? `: ${lastError.message}` : "";
  throw new Error(`timed out waiting for ${description}${suffix}`);
}

export async function stopChild(child: ChildProcess | null): Promise<void> {
  if (!child || child.exitCode !== null) return;
  const waitForExit = (timeoutMs: number): Promise<boolean> => new Promise((resolveExit) => {
    const finish = (exited: boolean): void => {
      clearTimeout(timer);
      child.off("exit", onExit);
      resolveExit(exited);
    };
    const onExit = (): void => finish(true);
    child.once("exit", onExit);
    const timer = setTimeout(() => finish(child.exitCode !== null), timeoutMs);
    if (child.exitCode !== null) finish(true);
  });

  const terminated = waitForExit(5_000);
  child.kill("SIGTERM");
  if (await terminated || child.exitCode !== null) return;

  const killed = waitForExit(2_000);
  child.kill("SIGKILL");
  if (!(await killed) && child.exitCode === null) {
    throw new Error(`child process ${String(child.pid)} did not exit after SIGKILL`);
  }
}

async function startDisplay(log: TailBuffer): Promise<{ child: ChildProcess | null; display: string; lock?: string }> {
  if (process.env.DISPLAY) return { child: null, display: process.env.DISPLAY };
  const xvfb = executable("Xvfb", process.env.TARIBOY_XVFB_BIN);
  let displayNumber = 0;
  let lock = "";
  for (let attempt = 0; attempt < 500; attempt += 1) {
    displayNumber = 100 + Math.floor(Math.random() * 9000);
    lock = join(tmpdir(), `tariboy-xvfb-${displayNumber}.lock`);
    try {
      await access(`/tmp/.X11-unix/X${displayNumber}`);
      continue;
    } catch {
      // A missing socket is available only after its ownership lock succeeds.
    }
    try {
      await mkdir(lock);
      break;
    } catch {
      lock = "";
    }
  }
  if (!lock) throw new Error("cannot reserve a unique Xvfb display");

  const display = `:${displayNumber}`;
  const child = spawn(xvfb, [display, "-screen", "0", "1440x900x24", "-nolisten", "tcp"], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  let spawnError: Error | null = null;
  child.once("error", (error) => { spawnError = error; });
  child.stdout?.on("data", (chunk: Buffer) => log.append(chunk));
  child.stderr?.on("data", (chunk: Buffer) => log.append(chunk));
  try {
    await waitFor("Xvfb display", 10_000, async () => {
      if (spawnError) throw spawnError;
      if (child.exitCode !== null) throw new Error(`Xvfb exited with code ${child.exitCode}`);
      await access(`/tmp/.X11-unix/X${displayNumber}`);
      return true;
    });
    return { child, display, lock };
  } catch (error) {
    await stopChild(child);
    await rm(lock, { recursive: true, force: true });
    throw error;
  }
}

async function processExists(pid: number): Promise<boolean> {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    return (error as NodeJS.ErrnoException).code === "EPERM";
  }
}

async function fixtureAgentProcesses(root: string, name: string): Promise<number[]> {
  const matches: number[] = [];
  for (const entry of await readdir("/proc")) {
    if (!/^[1-9][0-9]*$/.test(entry)) continue;
    try {
      const command = (await readFile(`/proc/${entry}/cmdline`)).toString().replaceAll("\0", " ");
      if (command.includes(`${root}/`) && command.includes(name)) matches.push(Number(entry));
    } catch {
      // Processes can exit between listing /proc and reading their command.
    }
  }
  return matches;
}

async function readTail(path: string, limit = MAX_LOG_BYTES): Promise<Buffer> {
  const handle = await open(path, "r");
  try {
    const { size } = await handle.stat();
    const length = Math.min(size, limit);
    const buffer = Buffer.alloc(length);
    await handle.read(buffer, 0, length, size - length);
    return buffer;
  } finally {
    await handle.close();
  }
}

async function stopIsolatedDaemon(runtimeDir: string, environment: NodeJS.ProcessEnv, log: TailBuffer): Promise<void> {
  const pidFile = join(runtimeDir, "tariboyd.pid");
  const socket = join(runtimeDir, "tariboyd.sock");
  let pidText: string;
  try {
    pidText = (await readFile(pidFile, "utf8")).trim();
  } catch {
    try {
      await access(socket);
      throw new Error(`isolated daemon socket exists without an owned PID: ${socket}`);
    } catch (error) {
      if (error instanceof Error && error.message.startsWith("isolated daemon")) throw error;
      return;
    }
  }
  if (!/^[1-9][0-9]*$/.test(pidText)) throw new Error(`invalid isolated daemon PID: ${pidText}`);
  const pid = Number(pidText);
  if (!(await processExists(pid))) return;

  let processExecutable: string;
  try {
    processExecutable = await readlink(`/proc/${pid}/exe`);
  } catch (error) {
    throw new Error(`cannot verify isolated daemon PID ${pid}: ${String(error)}`, { cause: error });
  }
  if (resolve(processExecutable) !== resolve(daemon)) {
    throw new Error(`refusing to signal unverified daemon PID ${pid}: ${processExecutable}`);
  }

  const stopped = spawnSync(cli, ["daemon", "stop"], { env: environment, encoding: "utf8", timeout: 10_000 });
  log.append(`daemon stop status=${String(stopped.status)} signal=${String(stopped.signal)} error=${String(stopped.error ?? "")}\n`);
  log.append(stopped.stdout ?? "");
  log.append(stopped.stderr ?? "");
  try {
    await waitFor("isolated daemon exit", 5_000, async () => !(await processExists(pid)));
    return;
  } catch {
    process.kill(pid, "SIGTERM");
  }
  try {
    await waitFor("isolated daemon SIGTERM", 5_000, async () => !(await processExists(pid)));
    return;
  } catch {
    process.kill(pid, "SIGKILL");
  }
  await waitFor("isolated daemon SIGKILL", 2_000, async () => !(await processExists(pid)));
}

async function seedUsageDatabase(
  seed: UsageSeed,
  baseDir: string,
  runtimeDir: string,
  environment: NodeJS.ProcessEnv,
  log: TailBuffer,
): Promise<void> {
  const bootstrap = spawn(daemon, ["--http-addr", ""], {
    env: environment,
    stdio: ["ignore", "pipe", "pipe"],
  });
  bootstrap.stdout?.on("data", (chunk: Buffer) => log.append(chunk));
  bootstrap.stderr?.on("data", (chunk: Buffer) => log.append(chunk));
  bootstrap.once("exit", (code, signal) => {
    log.append(`usage seed bootstrap exited code=${String(code)} signal=${String(signal)}\n`);
  });
  try {
    await waitFor("usage seed bootstrap daemon", 30_000, async () => {
      if (bootstrap.exitCode !== null) {
        throw new Error(`usage seed bootstrap exited with code ${bootstrap.exitCode}`);
      }
      await Promise.all([
        access(join(runtimeDir, "tariboyd.sock")),
        access(join(baseDir, "tariboyd.db")),
      ]);
      return true;
    });
    await stopIsolatedDaemon(runtimeDir, environment, log);
    await waitFor("usage seed bootstrap process", 10_000, async () => bootstrap.exitCode !== null);
  } catch (error) {
    await stopChild(bootstrap);
    throw error;
  }

  const seeded = spawnSync("python3", ["-c", `
import json, sqlite3, sys

payload = json.loads(sys.argv[2])
db = sqlite3.connect(sys.argv[1], timeout=10)
try:
    db.execute("PRAGMA foreign_keys=ON")
    with db:
        for group in payload["groups"]:
            db.execute("INSERT INTO groups(name, lead) VALUES(?, ?)", (group["name"], group["lead"]))
            db.execute('INSERT INTO agents(name, image_ref, harness_type, loop_enabled, "group") VALUES(?, ?, ?, 0, ?)',
                       (group["agent"], "basic:latest", "stub", group["name"]))
        for request in payload["requests"]:
            group = request.get("group")
            db.execute("""INSERT INTO ai_requests(
                id, ts, agent, image_name, image_tag, provider, model,
                input_tokens, output_tokens, cost_usd, latency_ms, status,
                upstream_status, group_id, group_name
            ) VALUES(?, ?, ?, 'basic', 'latest', 'openai', ?, ?, ?, ?, 10, 'ok', 200, ?, ?)""",
            (request["id"], request["ts"], request["agent"], request["model"],
             request["inputTokens"], request["outputTokens"], request["costUSD"], group, group))
finally:
    db.close()
`, join(baseDir, "tariboyd.db"), JSON.stringify(seed)], {
    encoding: "utf8",
    timeout: 10_000,
  });
  log.append(`usage seed status=${String(seeded.status)} signal=${String(seeded.signal)} error=${String(seeded.error ?? "")}\n`);
  log.append(seeded.stdout ?? "");
  log.append(seeded.stderr ?? "");
  if (seeded.status !== 0) {
    throw new Error(`cannot seed isolated Desktop usage database: ${seeded.stderr || seeded.stdout}`);
  }
}

export const test = base.extend<DesktopFixtures>({
  usageSeed: [undefined, { option: true }],
  desktopWorker: [async ({ usageSeed }, provide) => {
    await Promise.all([
      assertExecutable(application, "Tauri Desktop E2E application"),
      assertExecutable(daemon, "bundled test daemon"),
      assertExecutable(cli, "bundled test CLI"),
      assertExecutable(daemonWrapper, "Desktop E2E daemon wrapper"),
    ]);
    const driver = executable("tauri-driver", process.env.TAURI_DRIVER_BIN);
    const nativeDriver = executable("WebKitWebDriver", process.env.WEBKIT_WEBDRIVER_BIN);
    const root = await mkdtemp(join(tmpdir(), "tariboy-desktop-e2e-"));
    const baseDir = join(root, "base");
    const runtimeDir = join(root, "runtime");
    const appDataDir = join(root, "app-data");
    await Promise.all([baseDir, runtimeDir, appDataDir].map((path) => mkdir(path, { mode: 0o700 })));
    await chmod(root, 0o700);
    // Created empty and owner-only here rather than left to the app, so a test
    // can tell "no open was accepted" from "the observer was never wired up".
    const externalOpenLog = join(root, "external-opens.log");
    await (await open(externalOpenLog, "w", 0o600)).close();

    const log = new TailBuffer();
    let display: Awaited<ReturnType<typeof startDisplay>> | null = null;
    let driverProcess: ChildProcess | null = null;
    let session: W3CClient | null = null;
    let primaryError: unknown;
    const cleanupAgents = new Set<string>();
    const environment: NodeJS.ProcessEnv = {
      ...process.env,
      TARIBOY_BASE_DIR: baseDir,
      TARIBOY_RUNTIME_DIR: runtimeDir,
      TARIBOY_DESKTOP_APP_DATA_DIR: appDataDir,
      TARIBOY_DESKTOP_EXTERNAL_OPEN_LOG: externalOpenLog,
      TARIBOY_DESKTOP_NOTIFICATION_TEST: "1",
      TARIBOY_DESKTOP_IMAGE_TRANSFER_TEST: "1",
      // Never let a Desktop E2E process discover or contact the host's real
      // notification service. Tests that need activation use the guarded
      // Tauri command instead.
      DBUS_SESSION_BUS_ADDRESS: `unix:path=${join(root, "unavailable-notification-bus")}`,
      TARIBOY_DAEMON_BIN: daemonWrapper,
      TARIBOY_REAL_DAEMON_BIN: daemon,
      TARIBOY_STUB_HARNESS: join(repositoryRoot, "scripts/stub-harness.sh"),
    };

    try {
      if (usageSeed) {
        await seedUsageDatabase(usageSeed, baseDir, runtimeDir, environment, log);
      }
      display = await startDisplay(log);
      environment.DISPLAY = display.display;
      const port = await freePort();
      const nativePort = await freePort();
      driverProcess = spawn(driver, ["--port", String(port), "--native-port", String(nativePort), "--native-driver", nativeDriver], {
        env: environment,
        stdio: ["ignore", "pipe", "pipe"],
      });
      driverProcess.stdout?.on("data", (chunk: Buffer) => log.append(chunk));
      driverProcess.stderr?.on("data", (chunk: Buffer) => log.append(chunk));
      driverProcess.once("exit", (code, signal) => log.append(`tauri-driver exited code=${String(code)} signal=${String(signal)}\n`));
      await waitFor("tauri-driver", 15_000, async () => (await fetch(`http://127.0.0.1:${port}/status`)).ok);
      session = new W3CClient(`http://127.0.0.1:${port}`, 60_000);
      await session.createSession({ "tauri:options": { application } });
      const handles = await session.windowHandles();
      if (handles.length !== 1) {
        throw new Error(`Desktop must expose exactly one main window, got ${JSON.stringify(handles)}`);
      }
      await session.switchToWindow(handles[0]);
      await provide({
        client: session,
        log,
        baseDir,
        runtimeDir,
        registerAgentForCleanup: (name) => cleanupAgents.add(name),
        externalOpenLog,
      });
    } catch (error) {
      primaryError = error;
    }

    const cleanupErrors: unknown[] = [];
    for (const name of cleanupAgents) {
      const killed = spawnSync(cli, ["agent", "kill", name], {
        env: environment,
        encoding: "utf8",
        timeout: 10_000,
      });
      log.append(`agent kill ${name} status=${String(killed.status)} signal=${String(killed.signal)} error=${String(killed.error ?? "")}\n`);
      log.append(killed.stdout ?? "");
      log.append(killed.stderr ?? "");
      if (killed.status !== 0) {
        if ((await fixtureAgentProcesses(root, name)).length > 0) {
          cleanupErrors.push(new Error(`cannot kill fixture agent ${name}: ${killed.stderr || killed.stdout}`));
        }
        continue;
      }
      try {
        await waitFor(`fixture agent ${name} shim cleanup`, 10_000, async () => {
          try {
            await access(join(runtimeDir, `${name}.shim.sock`));
            return false;
          } catch {
            return true;
          }
        });
        await waitFor(`fixture agent ${name} process cleanup`, 10_000, async () => (
          (await fixtureAgentProcesses(root, name)).length === 0
        ));
      } catch (error) {
        cleanupErrors.push(error);
      }
    }
    if (session) {
      try { await session.deleteSession(); } catch (error) { log.append(`delete WebDriver session: ${String(error)}\n`); }
    }
    try { await stopChild(driverProcess); } catch (error) { cleanupErrors.push(error); }
    try { await stopIsolatedDaemon(runtimeDir, environment, log); } catch (error) { cleanupErrors.push(error); }
    try { await stopChild(display?.child ?? null); } catch (error) { cleanupErrors.push(error); }
    if (display?.lock) {
      try { await rm(display.lock, { recursive: true, force: true }); } catch (error) { cleanupErrors.push(error); }
    }
    try { log.append(await readTail(join(runtimeDir, "tariboyd.log"))); } catch { /* daemon may not have logged */ }
    if (cleanupErrors.length === 0) await rm(root, { recursive: true, force: true });
    if (cleanupErrors.length > 0) {
      const cleanupError = new AggregateError(cleanupErrors, "Desktop E2E cleanup failed");
      throw new Error(`Desktop E2E could not prove process shutdown; state retained at ${root}\n${String(cleanupError)}\n${log.toString()}`, { cause: cleanupError });
    }
    if (primaryError) {
      const detail = primaryError instanceof Error ? `${primaryError.name}: ${primaryError.message}` : String(primaryError);
      throw new Error(`${detail}\nDesktop E2E process log (tail):\n${log.toString()}`, { cause: primaryError });
    }
  }, { scope: "test" }],

  desktop: async ({ desktopWorker }, provide, testInfo) => {
    await provide(desktopWorker.client);
    if (testInfo.status !== testInfo.expectedStatus) {
      await testInfo.attach("desktop-e2e.log", { body: desktopWorker.log.bytes(), contentType: "text/plain" });
    }
  },
});

export { expect } from "playwright/test";
