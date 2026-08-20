import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { usePolling } from "@/hooks/usePolling";
import { getDaemonStatus, setLocalBaseURL } from "@/lib/api";
import {
  isDesktop, daemonState, daemonStart, daemonRestart, openDaemonLog,
  onDaemonState, versionMismatch, missingHTTPListener,
  type DesktopDaemonState,
} from "@/lib/desktop";

// One banner, two sources of truth: in a browser the only signal is whether the
// API answers; in the desktop app Rust owns the lifecycle and pushes every
// transition, so the banner can say WHY and offer the fix.
export function DaemonBanner() {
  return isDesktop() ? <DesktopBanner /> : <BrowserBanner />;
}

function BrowserBanner() {
  const { error } = usePolling(getDaemonStatus, 2000);
  if (!error) return null;
  return <Bar tone="error">daemon unreachable — is tariboyd running?</Bar>;
}

/** Status/liveness cadence, per the design spec's error-handling table. */
const DESKTOP_POLL_MS = 3000;

// DesktopBanner merges three signals, most authoritative first:
//
//  1. `daemon://state` pushes — Rust announces every transition IT causes.
//  2. A `daemon_status` read every 3 s. Push alone is not enough: registering the
//     listener is itself asynchronous (dynamic import + `listen()`), so a
//     transition can land in the gap and be lost forever.
//  3. A probe of the daemon's OWN HTTP API every 3 s. This is the only signal
//     that notices an ADOPTED daemon dying: `daemon_status` is a pure read of a
//     cached view (src-tauri/src/commands.rs) and `watch_daemon` only follows a
//     child the app spawned itself, so Rust never learns that a daemon it merely
//     attached to was stopped from a terminal or crashed.
function DesktopBanner() {
  const [st, setSt] = useState<DesktopDaemonState | null>(null);
  // Bumped by every push. A `daemon_status` response describes the state as of
  // the moment the read was ISSUED, so a push that lands while it is in flight
  // is strictly newer whichever promise settles first. Without this ordering a
  // slow initial read could overwrite a pushed `ready` with a stale `starting`,
  // stranding the app with no base URL until it was relaunched.
  const pushes = useRef(0);
  const mounted = useRef(true);

  const apply = useCallback((s: DesktopDaemonState) => {
    if (!mounted.current) return;
    // The port is chosen by Rust and can change across a restart, so every
    // transition re-publishes it to the api client.
    if (s.base_url) setLocalBaseURL(s.base_url);
    setSt(s);
  }, []);

  // Subscribed before the first read goes out (the poll below owns it), so the
  // window in which a transition can slip past both is as small as the bridge
  // allows; the poll then reconciles whatever still slipped through.
  useEffect(() => {
    mounted.current = true;
    const off = onDaemonState((s) => {
      pushes.current += 1;
      apply(s);
    });
    return () => {
      mounted.current = false;
      off();
    };
  }, [apply]);

  // usePolling's first tick IS the initial read, and the hook's hidden-tab
  // skipping comes along for free. The result is applied inside the fetcher
  // rather than through the returned `data` because only here do we still know
  // how many pushes had landed when the read went out.
  const readState = useCallback(async () => {
    const seen = pushes.current;
    const s = await daemonState();
    if (s && pushes.current === seen) apply(s);
    return s;
  }, [apply]);
  usePolling(readState, DESKTOP_POLL_MS);

  // Probing is skipped until Rust has published a URL to talk to: before that a
  // request would resolve against tauri://localhost and fail for the wrong
  // reason.
  const probeable = st?.state === "ready" && st.base_url !== "";
  const reachable = useCallback(
    () => (probeable ? getDaemonStatus() : Promise.resolve(null)),
    [probeable],
  );
  const { error: unreachable } = usePolling(reachable, DESKTOP_POLL_MS);

  // Rust still believes in a daemon that has stopped answering — the adopted
  // daemon someone stopped from the CLI. Reuse the `down` branch verbatim so the
  // user gets the same Start action.
  const view = st && probeable && unreachable ? { ...st, state: "down" as const } : st;

  if (!view) return null;

  if (view.state === "starting") {
    return <Bar tone="info">starting tariboyd…</Bar>;
  }
  if (view.state === "failed") {
    return (
      <Bar tone="error">
        tariboyd failed to start: {view.message}
        <Action onClick={() => void daemonStart()}>Retry</Action>
        <Action onClick={() => void openDaemonLog()}>Open log</Action>
      </Bar>
    );
  }
  if (view.state === "down") {
    return (
      <Bar tone="error">
        tariboyd is not running
        <Action onClick={() => void daemonStart()}>Start</Action>
        <Action onClick={() => void openDaemonLog()}>Open log</Action>
      </Bar>
    );
  }
  if (missingHTTPListener(view)) {
    return (
      <Bar tone="warn">
        the running daemon has no HTTP listener — restart it on a port this app can reach
        <Action onClick={() => void daemonRestart()}>Restart</Action>
      </Bar>
    );
  }
  if (versionMismatch(view)) {
    // Never restarted automatically: a daemon this app did not start may have
    // agent iterations in flight.
    return (
      <Bar tone="warn">
        daemon {view.daemon_version}, app bundles {view.app_version}
        <Action onClick={() => void daemonRestart()}>Restart on the bundled version</Action>
      </Bar>
    );
  }
  return null;
}

function Bar({ tone, children }: { tone: "error" | "warn" | "info"; children: ReactNode }) {
  const cls =
    tone === "error"
      ? "bg-destructive text-destructive-foreground"
      : tone === "warn"
        ? "bg-amber-500 text-black"
        : "bg-muted text-muted-foreground";
  return (
    <div className={`flex items-center justify-center gap-3 px-4 py-1 text-center text-sm ${cls}`}>
      {children}
    </div>
  );
}

function Action({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" onClick={onClick} className="underline underline-offset-2">
      {children}
    </button>
  );
}
