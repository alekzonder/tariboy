import { useCallback, useState } from "react";
import { readKey, storageKeys, writeKey } from "@/lib/storage";

export const SIDEBAR_STATE_KEY = storageKeys.terminalsSidebar.name;
/** The removed terminal canvas kept the sidebar inside its own record; it is
 *  read once so an existing width and hidden state survive the upgrade. */
export const LEGACY_WORKSPACE_STATE_KEY = storageKeys.terminalsLegacyWorkspace.name;

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

function sanitizeSidebarState(
  value: { width?: unknown; hidden?: unknown } | null | undefined,
): SidebarState | null {
  if (typeof value?.width !== "number" || !Number.isFinite(value.width)) return null;
  return { width: clampSidebarWidth(value.width), hidden: value.hidden === true };
}

/** Best-effort: a missing, malformed or unavailable store means the default. */
function readSidebarState(): SidebarState {
  return (
    sanitizeSidebarState(readKey(storageKeys.terminalsSidebar) as SidebarState | null) ??
    sanitizeSidebarState(
      (readKey(storageKeys.terminalsLegacyWorkspace) as { sidebar?: SidebarState } | null)?.sidebar,
    ) ?? { width: DEFAULT_SIDEBAR_WIDTH, hidden: false }
  );
}

function writeSidebarState(state: SidebarState) {
  // Unavailable storage keeps the in-memory state for this session.
  writeKey(storageKeys.terminalsSidebar, state);
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
