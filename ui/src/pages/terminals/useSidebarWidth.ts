import { useCallback, useState } from "react";

export const SIDEBAR_STATE_KEY = "terminals:sidebar:v1";
/** The removed terminal canvas kept the sidebar inside its own record; it is
 *  read once so an existing width and hidden state survive the upgrade. */
export const LEGACY_WORKSPACE_STATE_KEY = "terminals:workspace:v1";

export const DEFAULT_SIDEBAR_WIDTH = 268;
export const MIN_SIDEBAR_WIDTH = 160;
export const MAX_SIDEBAR_WIDTH = 640;

interface SidebarState {
  width: number;
  hidden: boolean;
}

export function clampSidebarWidth(px: number): number {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, Math.round(px)));
}

function sanitize(value: { width?: unknown; hidden?: unknown } | null | undefined): SidebarState | null {
  if (typeof value?.width !== "number" || !Number.isFinite(value.width)) return null;
  return { width: clampSidebarWidth(value.width), hidden: value.hidden === true };
}

/** Best-effort: a missing, malformed or unavailable store means the default. */
function readSidebarState(): SidebarState {
  try {
    const legacy = JSON.parse(localStorage.getItem(LEGACY_WORKSPACE_STATE_KEY) ?? "null") as
      { sidebar?: SidebarState } | null;
    return sanitize(JSON.parse(localStorage.getItem(SIDEBAR_STATE_KEY) ?? "null"))
      ?? sanitize(legacy?.sidebar)
      ?? { width: DEFAULT_SIDEBAR_WIDTH, hidden: false };
  } catch {
    return { width: DEFAULT_SIDEBAR_WIDTH, hidden: false };
  }
}

function writeSidebarState(state: SidebarState) {
  try {
    localStorage.setItem(SIDEBAR_STATE_KEY, JSON.stringify(state));
  } catch {
    // Unavailable storage keeps the in-memory state for this session.
  }
}

export function readSidebarWidth(): number {
  return readSidebarState().width;
}

export function useSidebarState(): {
  width: number;
  hidden: boolean;
  setWidth: (px: number) => void;
  setHidden: (hidden: boolean) => void;
} {
  const [sidebar, setSidebar] = useState(readSidebarState);
  const setWidth = useCallback((px: number) => {
    setSidebar((current) => {
      const next = { ...current, width: clampSidebarWidth(px) };
      writeSidebarState(next);
      return next;
    });
  }, []);
  const setHidden = useCallback((hidden: boolean) => {
    setSidebar((current) => {
      const next = { ...current, hidden };
      writeSidebarState(next);
      return next;
    });
  }, []);
  return { ...sidebar, setWidth, setHidden };
}

/** Compatibility wrapper for callers that only need width. */
export function useSidebarWidth(): [number, (px: number) => void] {
  const { width, setWidth } = useSidebarState();
  return [width, setWidth];
}
