import { useCallback, useEffect, useRef } from "react";

/** useCoalesced returns a callback that runs fn at most once per window,
 *  however many calls land in it. Hints come in bursts; one refetch is enough. */
export function useCoalesced(fn: () => void, windowMs: number) {
  const latest = useRef(fn);
  useEffect(() => { latest.current = fn; }, [fn]);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => { if (timer.current !== undefined) clearTimeout(timer.current); }, []);
  return useCallback(() => {
    if (timer.current !== undefined) return;
    timer.current = setTimeout(() => {
      timer.current = undefined;
      latest.current();
    }, windowMs);
  }, [windowMs]);
}
