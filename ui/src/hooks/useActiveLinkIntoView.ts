import { useEffect, useRef } from "react";
import { useLocation } from "react-router-dom";

/** Keeps the current NavLink of a sideways-scrolling strip in sight. */
export function useActiveLinkIntoView<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const { pathname } = useLocation();
  useEffect(() => {
    // jsdom has no scrollIntoView.
    ref.current
      ?.querySelector('[aria-current="page"]')
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [pathname]);
  return ref;
}
