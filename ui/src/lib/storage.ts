// The one owner of Web Storage in the WebView. Every key the UI and the Store
// UI persist is declared below, so its name, area, version and parse rule are
// visible in one place; nothing else touches localStorage or sessionStorage.
//
// Names are kept as they shipped: users already hold column layouts, widths,
// workspaces, the host registry and tokens under them. A new key is named
// `<area>:<purpose>:v<N>`; an incompatible format gets a new name with the next
// N, and the old name stays here only to be read once. Unversioned names
// (`version: null`) predate that rule.
//
// Access is best effort: missing, broken or blocked storage reads as the
// fallback and a rejected write returns false, so the interface keeps its
// in-memory state. Values are never logged — some of them are tokens.

import type { Theme } from "@/components/themeContext";

type StorageArea = "local" | "session";

export interface StorageKey<T> {
  area: StorageArea;
  name: string;
  version: number | null;
  /** Returns undefined for any value it does not accept. */
  parse(raw: string): T | undefined;
  serialize(value: T): string;
  fallback: T;
}

interface StorageKeyFamily<T, A extends string[]> {
  area: StorageArea;
  prefix: string;
  key(...parts: A): StorageKey<T>;
}

function stringKey(area: StorageArea, name: string, version: number | null): StorageKey<string> {
  return { area, name, version, parse: (raw) => raw, serialize: (value) => value, fallback: "" };
}

function oneOfKey<T extends string>(
  name: string,
  version: number | null,
  values: readonly T[],
  fallback: T,
): StorageKey<T> {
  return {
    area: "local",
    name,
    version,
    parse: (raw) => values.find((value) => value === raw),
    serialize: (value) => value,
    fallback,
  };
}

/** Raw JSON: the owning module sanitizes the decoded value, because the rules
 *  (column ids, width limits, known hosts) belong to it, not to storage. */
function jsonKey(area: StorageArea, name: string, version: number | null): StorageKey<unknown> {
  return {
    area,
    name,
    version,
    parse: (raw) => JSON.parse(raw) as unknown,
    serialize: (value) => JSON.stringify(value),
    fallback: null,
  };
}

export const storageKeys = {
  theme: oneOfKey<Theme>("tariboy-theme", null, ["light", "dark", "system"], "system"),
  uiMode: oneOfKey("app:ui-mode:v1", 1, ["simple", "expert"], "simple"),
  workspaces: jsonKey("local", "app:workspaces:v1", 1),
  desktopAutoDownload: {
    area: "local",
    name: "desktop:updates:auto-download:v1",
    version: 1,
    parse: (raw) => raw === "true",
    serialize: String,
    fallback: true,
  } satisfies StorageKey<boolean>,
  runtimePresets: jsonKey("local", "tariboy:runtime-presets:v1", 1),
  daemons: jsonKey("local", "tariboy_daemons", null),
  activeDaemon: stringKey("local", "tariboy_active_daemon", null),
  daemonsRejectedCleartext: jsonKey("local", "tariboy_daemons_rejected_cleartext", null),
  teamImport: {
    area: "local",
    name: "tariboy:team-import",
    version: null,
    parse: (raw) => {
      const value = JSON.parse(raw) as { id?: unknown; hostId?: unknown } | null;
      return typeof value?.id === "string" && typeof value.hostId === "string"
        ? { id: value.id, hostId: value.hostId }
        : undefined;
    },
    serialize: (value) => JSON.stringify(value),
    fallback: null,
  } satisfies StorageKey<{ id: string; hostId: string } | null>,
  terminalsServerOrder: jsonKey("local", "terminals:server-order:v1", 1),
  terminalsSidebar: jsonKey("local", "terminals:sidebar:v1", 1),
  /** Legacy: read once so the sidebar of the removed canvas survives. */
  terminalsLegacyWorkspace: jsonKey("local", "terminals:workspace:v1", 1),
  terminalsSidebarTab: oneOfKey("terminals:sidebar-tab:v1", 1, ["servers", "agents"], "servers"),
  terminalsSidebarPinned: jsonKey("local", "terminals:sidebar-pinned:v1", 1),
  chatTypes: jsonKey("local", "terminals:chat-types:v1", 1),
  taskColumns: jsonKey("local", "tasks:columns:v1", 1),
  taskPanelWidths: jsonKey("local", "tasks:workspace:v1", 1),
  newTaskQueue: jsonKey("local", "tasks:new-task-queue:v1", 1),
  taskQueueFilter: stringKey("session", "tasks:queue-filter:v1", 1),
  allTasksView: jsonKey("session", "tasks:all-view:v1", 1),
  storeToken: stringKey("session", "tariboy_store_token", null),
};

export const storageKeyFamilies = {
  /** The retained inject-modal draft of one agent's section. */
  tuiDraft: {
    area: "local",
    prefix: "tui:draft:",
    key: (agent, section) => stringKey("local", `tui:draft:${agent}:${section}`, null),
  } satisfies StorageKeyFamily<string, [string, string]>,
  /** Browser-mode host token; Desktop keeps tokens in the Keychain instead. */
  daemonToken: {
    area: "session",
    prefix: "tariboy_daemon_token_",
    key: (id) => stringKey("session", `tariboy_daemon_token_${id}`, null),
  } satisfies StorageKeyFamily<string, [string]>,
};

function storageFor(area: StorageArea): Storage {
  return area === "local" ? globalThis.localStorage : globalThis.sessionStorage;
}

/** The stored value or the fallback; `ok` is false when the storage threw. */
export function readKeyResult<T>(key: StorageKey<T>): { value: T; ok: boolean } {
  let raw: string | null;
  try {
    raw = storageFor(key.area).getItem(key.name);
  } catch {
    return { value: key.fallback, ok: false };
  }
  if (raw === null) return { value: key.fallback, ok: true };
  try {
    return { value: key.parse(raw) ?? key.fallback, ok: true };
  } catch {
    return { value: key.fallback, ok: true };
  }
}

export function readKey<T>(key: StorageKey<T>): T {
  return readKeyResult(key).value;
}

export function writeKey<T>(key: StorageKey<T>, value: T): boolean {
  try {
    storageFor(key.area).setItem(key.name, key.serialize(value));
    return true;
  } catch {
    return false;
  }
}

export function removeKey(key: StorageKey<unknown>): void {
  try {
    storageFor(key.area).removeItem(key.name);
  } catch {
    // Nothing to keep consistent: the next read falls back the same way.
  }
}
