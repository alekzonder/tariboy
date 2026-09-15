/**
 * Sidebar preferences that belong to this browser rather than to a daemon:
 * which tab is open, and which agents the operator pinned. Stored the same way
 * the server order already is — one versioned localStorage key, best-effort,
 * and a rejected or unreadable value falls back to the default instead of
 * taking the sidebar down with it.
 */

export type SidebarTab = "agents" | "groups" | "servers";

export const SIDEBAR_TAB_KEY = "terminals:sidebar-tab:v1";
export const SIDEBAR_PINNED_KEY = "terminals:sidebar-pinned:v1";

const TABS: readonly SidebarTab[] = ["agents", "groups", "servers"];

export function readSidebarTab(): SidebarTab {
  try {
    const raw = localStorage.getItem(SIDEBAR_TAB_KEY);
    return TABS.find((tab) => tab === raw) ?? "agents";
  } catch {
    return "agents";
  }
}

export function writeSidebarTab(tab: SidebarTab): void {
  try {
    localStorage.setItem(SIDEBAR_TAB_KEY, tab);
  } catch {
    // A full or unavailable localStorage must not break switching tabs.
  }
}

export function readPinnedAgents(): Set<string> {
  try {
    const parsed = JSON.parse(localStorage.getItem(SIDEBAR_PINNED_KEY) ?? "null") as {
      version?: number;
      keys?: unknown;
    } | null;
    if (parsed?.version !== 1 || !Array.isArray(parsed.keys)) return new Set();
    return new Set(parsed.keys.filter((key): key is string => typeof key === "string"));
  } catch {
    return new Set();
  }
}

export function writePinnedAgents(keys: ReadonlySet<string>): void {
  try {
    localStorage.setItem(SIDEBAR_PINNED_KEY, JSON.stringify({ version: 1, keys: [...keys] }));
  } catch {
    // Same as the tab: a pin that cannot be saved still applies to this session.
  }
}
