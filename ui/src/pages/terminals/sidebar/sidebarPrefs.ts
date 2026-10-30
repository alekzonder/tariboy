/**
 * Sidebar preferences that belong to this browser rather than to a daemon:
 * whether the agent list is open, and which agents the operator pinned. Stored the same way
 * the server order already is — one versioned localStorage key, best-effort,
 * and a rejected or unreadable value falls back to the default instead of
 * taking the sidebar down with it.
 */

import { useSyncExternalStore } from "react";
import { readKey, storageKeys, writeKey } from "@/lib/storage";

export const SIDEBAR_AGENTS_KEY = storageKeys.terminalsSidebarAgents.name;
export const SIDEBAR_PINNED_KEY = storageKeys.terminalsSidebarPinned.name;

export function readAgentsOpen(): boolean {
  return readKey(storageKeys.terminalsSidebarAgents) === "open";
}

export function writeAgentsOpen(open: boolean): void {
  // A full or unavailable storage must not break opening the list.
  writeKey(storageKeys.terminalsSidebarAgents, open ? "open" : "closed");
}

export function readPinnedAgents(): Set<string> {
  const parsed = readKey(storageKeys.terminalsSidebarPinned) as {
    version?: number;
    keys?: unknown;
  } | null;
  if (parsed?.version !== 1 || !Array.isArray(parsed.keys)) return new Set();
  return new Set(parsed.keys.filter((key): key is string => typeof key === "string"));
}

export function writePinnedAgents(keys: ReadonlySet<string>): void {
  // Same as the list: a pin that cannot be saved still applies to this session.
  writeKey(storageKeys.terminalsSidebarPinned, { version: 1, keys: [...keys] });
}

// The sidebar and the agent header pin the same set, so it is one store read
// from storage. shortcut: a pin that storage refuses does not stick, keep a
// session copy if that matters.
const pinListeners = new Set<() => void>();
let pinnedSnapshot: ReadonlySet<string> = new Set();

function pinnedAgentsSnapshot(): ReadonlySet<string> {
  const read = readPinnedAgents();
  const same =
    read.size === pinnedSnapshot.size && [...read].every((key) => pinnedSnapshot.has(key));
  if (!same) pinnedSnapshot = read;
  return pinnedSnapshot;
}

function subscribePinned(listener: () => void) {
  pinListeners.add(listener);
  return () => {
    pinListeners.delete(listener);
  };
}

export function togglePinnedAgent(key: string): void {
  const next = new Set(pinnedAgentsSnapshot());
  if (!next.delete(key)) next.add(key);
  writePinnedAgents(next);
  pinnedSnapshot = next;
  pinListeners.forEach((listener) => listener());
}

export function usePinnedAgents(): ReadonlySet<string> {
  return useSyncExternalStore(subscribePinned, pinnedAgentsSnapshot);
}
