import { useSyncExternalStore } from "react";

/** Below Tailwind's `lg` the agent list is a drawer, not a column. */
export const COMPACT_QUERY = "(max-width: 1023.98px)";

/** Whether a CSS media query matches now, following later changes. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const list = window.matchMedia(query);
      list.addEventListener("change", onChange);
      return () => list.removeEventListener("change", onChange);
    },
    () => window.matchMedia(query).matches,
    () => false,
  );
}
