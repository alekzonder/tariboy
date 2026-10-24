import type { Daemon } from "../daemons";

// The "active daemon" a client created with an undefined target binds to.
// null = single-daemon same-origin, no-auth behavior (relative /api, no
// Authorization). Set by the host switcher; an empty registry leaves it null.
let activeDaemon: Daemon | null = null;
export function setActiveDaemon(d: Daemon | null): void {
  activeDaemon = d;
}
export function getActiveDaemon(): Daemon | null {
  return activeDaemon;
}

// The origin of the LOCAL daemon. Empty ("") means same-origin, which is what a
// browser wants: relative /api paths through the dev proxy or the daemon's own
// listener. The desktop app has no same origin to fall back on — its page is
// served from tauri://localhost — so its Rust side hands the resolved
// http://127.0.0.1:PORT here at boot, before the first render.
let localBaseURL = "";
export function setLocalBaseURL(u: string): void {
  localBaseURL = u.replace(/\/+$/, "");
}
export function getLocalBaseURL(): string {
  return localBaseURL;
}

// ApiTarget selects which daemon a client addresses: undefined = the active
// daemon at the moment the client is created, null = same-origin local, a
// Daemon = that host. Only client creation and server-selection boundaries
// accept it; domain methods never do.
export type ApiTarget = Daemon | null | undefined;
export function resolveTarget(t: ApiTarget): Daemon | null {
  return t === undefined ? activeDaemon : t;
}
