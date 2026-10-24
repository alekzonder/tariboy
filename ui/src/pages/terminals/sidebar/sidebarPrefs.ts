/**
 * Sidebar preferences that belong to this browser rather than to a daemon:
 * which tab is open, and which agents the operator pinned. Stored the same way
 * the server order already is — one versioned localStorage key, best-effort,
 * and a rejected or unreadable value falls back to the default instead of
 * taking the sidebar down with it.
 */

import { readKey, storageKeys, writeKey } from "@/lib/storage";

export type SidebarTab = "servers" | "agents";

export const SIDEBAR_TAB_KEY = storageKeys.terminalsSidebarTab.name;
export const SIDEBAR_PINNED_KEY = storageKeys.terminalsSidebarPinned.name;

export function readSidebarTab(): SidebarTab {
  return readKey(storageKeys.terminalsSidebarTab);
}

export function writeSidebarTab(tab: SidebarTab): void {
  // A full or unavailable storage must not break switching tabs.
  writeKey(storageKeys.terminalsSidebarTab, tab);
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
  // Same as the tab: a pin that cannot be saved still applies to this session.
  writeKey(storageKeys.terminalsSidebarPinned, { version: 1, keys: [...keys] });
}
