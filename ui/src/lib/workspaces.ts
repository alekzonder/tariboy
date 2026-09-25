import { useSyncExternalStore } from "react";

/** Host workspaces: a window-level grouping of registered hosts. Hosts are the
 *  set of daemons, so no single daemon can own this; it lives in the app's
 *  WebView storage next to the server order. A host absent from the map is in
 *  the built-in Default, so new hosts land there without any migration. */
export const WORKSPACES_KEY = "app:workspaces:v1";
export const DEFAULT_WORKSPACE_ID = "default";

export interface Workspace {
  id: string;
  name: string;
}

export interface WorkspacesState {
  /** Default first, then custom workspaces in creation order. */
  workspaces: Workspace[];
  hostWorkspace: Readonly<Record<string, string>>;
  active: string;
}

const DEFAULT_WORKSPACE: Workspace = { id: DEFAULT_WORKSPACE_ID, name: "Default" };

function sanitize(value: unknown): WorkspacesState {
  const raw = (typeof value === "object" && value !== null ? value : {}) as {
    version?: unknown; workspaces?: unknown; hostWorkspace?: unknown; active?: unknown;
  };
  const custom = raw.version === 1 && Array.isArray(raw.workspaces)
    ? raw.workspaces.filter((w): w is Workspace =>
      typeof w?.id === "string" && w.id !== DEFAULT_WORKSPACE_ID && w.id !== ""
      && typeof w.name === "string" && w.name.trim() !== "")
    : [];
  const ids = new Set(custom.map((w) => w.id));
  const hostWorkspace: Record<string, string> = {};
  if (raw.version === 1 && typeof raw.hostWorkspace === "object" && raw.hostWorkspace !== null) {
    for (const [host, id] of Object.entries(raw.hostWorkspace)) {
      if (typeof id === "string" && ids.has(id)) hostWorkspace[host] = id;
    }
  }
  const active = typeof raw.active === "string" && ids.has(raw.active)
    ? raw.active
    : DEFAULT_WORKSPACE_ID;
  return { workspaces: [DEFAULT_WORKSPACE, ...custom], hostWorkspace, active };
}

function read(): WorkspacesState {
  try {
    return sanitize(JSON.parse(localStorage.getItem(WORKSPACES_KEY) ?? "null"));
  } catch {
    return sanitize(null);
  }
}

let state: WorkspacesState | null = null;
const listeners = new Set<() => void>();

function current(): WorkspacesState {
  state ??= read();
  return state;
}

function update(next: WorkspacesState) {
  state = sanitize({ version: 1, ...next, workspaces: next.workspaces.slice(1) });
  try {
    localStorage.setItem(WORKSPACES_KEY, JSON.stringify({
      version: 1,
      workspaces: state.workspaces.slice(1),
      hostWorkspace: state.hostWorkspace,
      active: state.active,
    }));
  } catch {
    // Unavailable storage keeps the change for this session only.
  }
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function useWorkspaces(): WorkspacesState {
  return useSyncExternalStore(subscribe, current);
}

export function workspaceOf(value: WorkspacesState, hostId: string): string {
  return value.hostWorkspace[hostId] ?? DEFAULT_WORKSPACE_ID;
}

/** Returns the new workspace id, or undefined for a blank name. */
export function createWorkspace(name: string): string | undefined {
  const trimmed = name.trim();
  if (!trimmed) return undefined;
  const id = `ws-${crypto.randomUUID()}`;
  const s = current();
  update({ ...s, workspaces: [...s.workspaces, { id, name: trimmed }] });
  return id;
}

export function renameWorkspace(id: string, name: string) {
  const trimmed = name.trim();
  if (id === DEFAULT_WORKSPACE_ID || !trimmed) return;
  const s = current();
  update({ ...s, workspaces: s.workspaces.map((w) => w.id === id ? { id, name: trimmed } : w) });
}

/** Its hosts fall back to Default because their map entries now dangle. */
export function deleteWorkspace(id: string) {
  if (id === DEFAULT_WORKSPACE_ID) return;
  const s = current();
  update({ ...s, workspaces: s.workspaces.filter((w) => w.id !== id) });
}

export function moveHost(hostId: string, workspaceId: string) {
  const s = current();
  const hostWorkspace = { ...s.hostWorkspace };
  if (workspaceId === DEFAULT_WORKSPACE_ID) delete hostWorkspace[hostId];
  else hostWorkspace[hostId] = workspaceId;
  update({ ...s, hostWorkspace });
}

export function selectWorkspace(id: string) {
  update({ ...current(), active: id });
}

export function resetWorkspacesForTest() {
  state = null;
}
