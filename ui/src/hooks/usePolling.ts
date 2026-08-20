import { useCallback, useEffect, useRef, useState } from "react";

// Short-poll a fetcher on an interval, skipping ticks while the tab is hidden.
export function usePolling<T>(fn: () => Promise<T>, ms = 1000) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const fnRef = useRef(fn);
  fnRef.current = fn;

  const tick = useCallback(async () => {
    if (typeof document !== "undefined" && document.hidden) return;
    try {
      setData(await fnRef.current());
      setError(null);
    } catch (e) {
      setError(e as Error);
    }
  }, []);

  useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    const loop = async () => {
      if (!alive) return;
      await tick();
      if (alive) timer = setTimeout(loop, ms);
    };
    void loop();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [tick, ms]);

  return { data, error, refresh: tick };
}
