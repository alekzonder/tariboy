import { useSyncExternalStore } from "react";
import { readKey, storageKeys, writeKey } from "./storage";

/** Host workspaces: independent lists of registered hosts in one window. Hosts
 *  are the set of daemons, so no single daemon can own this; it lives in the
 *  app's WebView storage next to the server order. A host may be in several
 *  workspaces; a host in none, such as one registered before workspaces
 *  existed, is in the built-in Default. */
export const WORKSPACES_KEY = storageKeys.workspaces.name;
export const DEFAULT_WORKSPACE_ID = "default";

interface Workspace {
  id: string;
  name: string;
  hosts: readonly string[];
}

export interface WorkspacesState {
  /** Default first, then custom workspaces in creation order. */
  workspaces: Workspace[];
  active: string;
}

const DEFAULT_NAME = "Default";

function stringList(value: unknown): string[] {
  return Array.isArray(value)
    ? [...new Set(value.filter((v): v is string => typeof v === "string"))]
    : [];
}

function sanitizeWorkspaces(value: unknown): WorkspacesState {
  const raw = (typeof value === "object" && value !== null ? value : {}) as {
    version?: unknown;
    workspaces?: unknown;
    active?: unknown;
  };
  const entries = raw.version === 2 && Array.isArray(raw.workspaces) ? raw.workspaces : [];
  const valid = entries.filter(
    (w): w is { id: string; name?: unknown; hosts?: unknown } =>
      typeof w?.id === "string" && w.id !== "",
  );
  const defaultEntry = valid.find((w) => w.id === DEFAULT_WORKSPACE_ID);
  const custom: Workspace[] = valid
    .filter(
      (w): w is { id: string; name: string; hosts?: unknown } =>
        w.id !== DEFAULT_WORKSPACE_ID && typeof w.name === "string" && w.name.trim() !== "",
    )
    .map((w) => ({ id: w.id, name: w.name, hosts: stringList(w.hosts) }));
  const ids = new Set(custom.map((w) => w.id));
  const active =
    typeof raw.active === "string" && ids.has(raw.active) ? raw.active : DEFAULT_WORKSPACE_ID;
  const defaults = {
    id: DEFAULT_WORKSPACE_ID,
    name: DEFAULT_NAME,
    hosts: stringList(defaultEntry?.hosts),
  };
  return { workspaces: [defaults, ...custom], active };
}

/** Version 1 kept one workspace per host in `hostWorkspace`; a host without an
 *  entry was in Default and stays there through the fallback. */
function migrateV1(value: unknown): WorkspacesState {
  const raw = (typeof value === "object" && value !== null ? value : {}) as {
    version?: unknown;
    workspaces?: unknown;
    hostWorkspace?: unknown;
    active?: unknown;
  };
  if (raw.version !== 1) return sanitizeWorkspaces(null);
  const hostWorkspace =
    typeof raw.hostWorkspace === "object" && raw.hostWorkspace !== null
      ? Object.entries(raw.hostWorkspace)
      : [];
  const workspaces = (Array.isArray(raw.workspaces) ? raw.workspaces : []).map((w: unknown) => {
    const id = (w as { id?: unknown } | null)?.id;
    return {
      ...(w as object),
      hosts: hostWorkspace.filter(([, ws]) => ws === id).map(([host]) => host),
    };
  });
  return sanitizeWorkspaces({ version: 2, workspaces, active: raw.active });
}

function read(): WorkspacesState {
  const stored = readKey(storageKeys.workspaces);
  if (stored !== null) return sanitizeWorkspaces(stored);
  return migrateV1(readKey(storageKeys.workspacesLegacy));
}

let state: WorkspacesState | null = null;
const listeners = new Set<() => void>();

function current(): WorkspacesState {
  state ??= read();
  return state;
}

function update(next: WorkspacesState) {
  const value = { version: 2, workspaces: next.workspaces, active: next.active };
  state = sanitizeWorkspaces(value);
  // Unavailable storage keeps the change for this session only.
  writeKey(storageKeys.workspaces, value);
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function useWorkspaces(): WorkspacesState {
  return useSyncExternalStore(subscribe, current);
}

/** Whether `hostId` is in workspace `id`: listed there, or in Default when no
 *  workspace lists it. */
export function inWorkspace(value: WorkspacesState, hostId: string, id: string): boolean {
  const listed = value.workspaces.filter((w) => w.hosts.includes(hostId));
  if (listed.some((w) => w.id === id)) return true;
  return id === DEFAULT_WORKSPACE_ID && listed.length === 0;
}

/** The hosts that belong to workspace `id`. */
export function hostsIn<T extends { id: string }>(
  value: WorkspacesState,
  hosts: T[],
  id: string,
): T[] {
  return hosts.filter((host) => inWorkspace(value, host.id, id));
}

/** Returns the new workspace id, or undefined for a blank name. */
export function createWorkspace(name: string): string | undefined {
  const trimmed = name.trim();
  if (!trimmed) return undefined;
  const id = `ws-${crypto.randomUUID()}`;
  const s = current();
  update({ ...s, workspaces: [...s.workspaces, { id, name: trimmed, hosts: [] }] });
  return id;
}

export function renameWorkspace(id: string, name: string) {
  const trimmed = name.trim();
  if (id === DEFAULT_WORKSPACE_ID || !trimmed) return;
  const s = current();
  update({
    ...s,
    workspaces: s.workspaces.map((w) => (w.id === id ? { ...w, name: trimmed } : w)),
  });
}

/** Hosts listed nowhere else fall back to Default. */
export function deleteWorkspace(id: string) {
  if (id === DEFAULT_WORKSPACE_ID) return;
  const s = current();
  update({ ...s, workspaces: s.workspaces.filter((w) => w.id !== id) });
}

function setHosts(workspaceId: string, hosts: (list: readonly string[]) => readonly string[]) {
  const s = current();
  update({
    ...s,
    workspaces: s.workspaces.map((w) =>
      w.id === workspaceId ? { ...w, hosts: hosts(w.hosts) } : w,
    ),
  });
}

/** Adds the host to the workspace and keeps its other workspaces. */
export function addHostToWorkspace(hostId: string, workspaceId: string) {
  setHosts(workspaceId, (list) => (list.includes(hostId) ? list : [...list, hostId]));
}

/** A host removed from its last workspace falls back to Default. */
export function removeHostFromWorkspace(hostId: string, workspaceId: string) {
  setHosts(workspaceId, (list) => list.filter((id) => id !== hostId));
}

/** Adds a newly registered host to the current workspace only. */
export function addHostToCurrentWorkspace(hostId: string) {
  addHostToWorkspace(hostId, current().active);
}

export function selectWorkspace(id: string) {
  update({ ...current(), active: id });
}

export function resetWorkspacesForTest() {
  state = null;
}
