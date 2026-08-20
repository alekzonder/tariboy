import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import type { DesktopDaemonState } from "@/lib/desktop";

// Every listener `onDaemonState` hands out, so a test can emit a `daemon://state`
// push exactly the way Rust does. Hoisted because the mock factory below is
// lifted above the imports.
const bridge = vi.hoisted(() => ({
  listeners: [] as ((s: DesktopDaemonState) => void)[],
}));

// The bridge is mocked wholesale: isDesktop/daemonState are what the banner
// branches on, while the two derived predicates stay REAL so the tests exercise
// the actual mismatch rules rather than a restatement of them.
vi.mock("@/lib/desktop", async (importOriginal) => {
  const real = await importOriginal<typeof import("@/lib/desktop")>();
  return {
    ...real,
    isDesktop: () => true,
    daemonState: vi.fn(),
    daemonStart: vi.fn().mockResolvedValue(null),
    daemonRestart: vi.fn().mockResolvedValue(null),
    openDaemonLog: vi.fn().mockResolvedValue(null),
    onDaemonState: (cb: (s: DesktopDaemonState) => void) => {
      bridge.listeners.push(cb);
      return () => {
        bridge.listeners = bridge.listeners.filter((f) => f !== cb);
      };
    },
  };
});

import { DaemonBanner } from "./DaemonBanner";
import { daemonState } from "@/lib/desktop";
import { getLocalBaseURL, setLocalBaseURL } from "@/lib/api";

/** Must match DESKTOP_POLL_MS in DaemonBanner.tsx (the spec's 3 s cadence). */
const POLL_MS = 3000;

const base: DesktopDaemonState = {
  state: "ready", base_url: "http://127.0.0.1:9990", daemon_version: "1.0.0",
  app_version: "1.0.0", base_dir: "/base", pid: 42, adopted: false, message: "",
};

function stubDesktop(state: DesktopDaemonState) {
  vi.mocked(daemonState).mockResolvedValue(state);
}

/** A promise the test resolves by hand, to hold a status read in flight. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

/** Emit a `daemon://state` push, as Rust's AppState::set does. */
function push(s: DesktopDaemonState) {
  act(() => {
    for (const cb of [...bridge.listeners]) cb(s);
  });
}

/** Advance the fake clock and let every promise the tick started settle. */
async function settle(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

const okResponse = () =>
  ({
    ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: {} }),
  }) as unknown as Response;

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  bridge.listeners = [];
  setLocalBaseURL("");
  fetchMock = vi.fn().mockResolvedValue(okResponse());
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("DaemonBanner in the desktop app", () => {
  it("renders nothing when the daemon is ready and versions match", async () => {
    stubDesktop(base);
    const { container } = render(<DaemonBanner />);
    await waitFor(() => expect(container.textContent).toBe(""));
  });

  it("shows a starting notice while the daemon boots", async () => {
    stubDesktop({ ...base, state: "starting", base_url: "" });
    render(<DaemonBanner />);
    expect(await screen.findByText(/starting tariboyd/i)).toBeInTheDocument();
  });

  it("shows the error and a retry action when the start failed", async () => {
    stubDesktop({ ...base, state: "failed", base_url: "", message: "exec format error" });
    render(<DaemonBanner />);
    expect(await screen.findByText(/exec format error/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /open log/i })).toBeInTheDocument();
  });

  it("offers a start action when an adopted daemon has died", async () => {
    stubDesktop({ ...base, state: "down", base_url: "" });
    render(<DaemonBanner />);
    expect(await screen.findByRole("button", { name: /start/i })).toBeInTheDocument();
  });

  it("warns without blocking when the daemon version differs from the app's", async () => {
    stubDesktop({ ...base, daemon_version: "0.9.0", app_version: "1.0.0", adopted: true });
    render(<DaemonBanner />);
    expect(await screen.findByText(/0\.9\.0/)).toBeInTheDocument();
    expect(screen.getByText(/1\.0\.0/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /restart/i })).toBeInTheDocument();
  });

  it("warns when the adopted daemon has no HTTP listener", async () => {
    stubDesktop({ ...base, base_url: "", adopted: true });
    render(<DaemonBanner />);
    expect(await screen.findByText(/no http listener/i)).toBeInTheDocument();
  });
});

// Rust's `daemon_status` is a pure read of a cached view and `watch_daemon` only
// follows a child the app spawned, so nothing on that side notices an ADOPTED
// daemon being stopped from a terminal. The SPA's own probe is the only signal.
describe("DaemonBanner polling for an adopted daemon that dies", () => {
  it("turns a ready daemon that stops answering into a down banner with a start action", async () => {
    vi.useFakeTimers();
    stubDesktop({ ...base, adopted: true });
    render(<DaemonBanner />);
    await settle();
    expect(screen.queryByText(/tariboyd is not running/i)).toBeNull();

    // `tariboy daemon stop` in a terminal: Rust still believes it is ready.
    fetchMock.mockRejectedValue(new Error("connection refused"));
    await settle(POLL_MS);

    expect(screen.getByText(/tariboyd is not running/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^start$/i })).toBeInTheDocument();
  });

  it("clears the banner again when the daemon comes back", async () => {
    vi.useFakeTimers();
    stubDesktop({ ...base, adopted: true });
    const { container } = render(<DaemonBanner />);
    await settle();

    fetchMock.mockRejectedValue(new Error("connection refused"));
    await settle(POLL_MS);
    expect(screen.getByText(/tariboyd is not running/i)).toBeInTheDocument();

    // `tariboy daemon start` from the terminal: the API answers again.
    fetchMock.mockResolvedValue(okResponse());
    await settle(POLL_MS);
    expect(container.textContent).toBe("");
  });

  it("keeps polling a daemon whose status read never changes", async () => {
    vi.useFakeTimers();
    stubDesktop({ ...base, adopted: true });
    render(<DaemonBanner />);
    await settle();
    const afterBoot = vi.mocked(daemonState).mock.calls.length;

    await settle(POLL_MS * 3);
    expect(vi.mocked(daemonState).mock.calls.length).toBeGreaterThan(afterBoot);
  });
});

// Both sources are asynchronous, so "which promise settled last" says nothing
// about which state is newer. The banner has to order them itself.
describe("DaemonBanner state ordering", () => {
  it("does not let a stale in-flight status read clobber a newer pushed state", async () => {
    vi.useFakeTimers();
    const stale = deferred<DesktopDaemonState | null>();
    vi.mocked(daemonState).mockReturnValue(stale.promise);
    render(<DaemonBanner />);
    await settle(); // the read is issued and left hanging

    push({ ...base, state: "down", base_url: "" });
    expect(screen.getByText(/tariboyd is not running/i)).toBeInTheDocument();

    // The read finally answers with what Rust believed BEFORE that push.
    stale.resolve({ ...base, adopted: true });
    await settle();

    expect(screen.getByText(/tariboyd is not running/i)).toBeInTheDocument();
  });

  it("keeps a pushed ready that arrives before the slower initial status read", async () => {
    vi.useFakeTimers();
    const boot = deferred<DesktopDaemonState | null>();
    vi.mocked(daemonState).mockReturnValue(boot.promise);
    const { container } = render(<DaemonBanner />);
    await settle();

    // Rust finished starting the daemon while the boot read was still in flight;
    // the push carries the port every later request has to use.
    push({ ...base, base_url: "http://127.0.0.1:9971" });
    expect(getLocalBaseURL()).toBe("http://127.0.0.1:9971");
    expect(container.textContent).toBe("");

    // Only now does the boot read answer, with the state Rust held at the time.
    boot.resolve({ ...base, state: "starting", base_url: "" });
    await settle();

    expect(screen.queryByText(/starting tariboyd/i)).toBeNull();
    expect(getLocalBaseURL()).toBe("http://127.0.0.1:9971");
  });
});
