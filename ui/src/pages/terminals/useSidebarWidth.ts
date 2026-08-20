import { useCallback, useState } from "react";
import {
  DEFAULT_SIDEBAR_WIDTH,
  LEGACY_SIDEBAR_WIDTH_KEY,
  MAX_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
  clampSidebarWidth,
  readWorkspaceState,
  updateWorkspaceState,
} from "./workspaceState";

/** Legacy key read during migration when no versioned workspace exists. */
export const SIDEBAR_WIDTH_KEY = LEGACY_SIDEBAR_WIDTH_KEY;
export {
  DEFAULT_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
  MAX_SIDEBAR_WIDTH,
  clampSidebarWidth,
};

/** Read the persisted width. Falls back to the default for a missing key, a
 *  non-numeric value, or an unavailable localStorage (best-effort, same as the
 *  color cache). */
export function readSidebarWidth(): number {
  return readWorkspaceState().sidebar.width;
}

export function useSidebarState(): {
  width: number;
  hidden: boolean;
  setWidth: (px: number) => void;
  setHidden: (hidden: boolean) => void;
} {
  const [sidebar, setSidebar] = useState(() => readWorkspaceState().sidebar);
  const setWidth = useCallback((px: number) => {
    const next = clampSidebarWidth(px);
    setSidebar((current) => ({ ...current, width: next }));
    updateWorkspaceState((current) => ({
      ...current,
      sidebar: { ...current.sidebar, width: next },
    }));
  }, []);
  const setHidden = useCallback((hidden: boolean) => {
    setSidebar((current) => ({ ...current, hidden }));
    updateWorkspaceState((current) => ({
      ...current,
      sidebar: { ...current.sidebar, hidden },
    }));
  }, []);
  return { ...sidebar, setWidth, setHidden };
}

/** Compatibility wrapper for callers that only need width. */
export function useSidebarWidth(): [number, (px: number) => void] {
  const { width, setWidth } = useSidebarState();
  return [width, setWidth];
}
