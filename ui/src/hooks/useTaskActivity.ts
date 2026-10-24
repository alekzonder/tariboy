import { useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { ActivityWindow } from "@/lib/activityPeriod";
import type { TaskActivityReport } from "@/lib/types";

const POLL_MS = 10000;

/** Loads and polls the task summary for one window. A response for an older
 *  window or host never replaces the current one; a failed poll keeps the last
 *  report on screen and reports the error. */
export function useTaskActivity(window: ActivityWindow, agent?: string) {
  const api = useApi();
  const target = api.target;
  const { since, until, error: windowError } = window;
  const key = `${target?.id ?? ""}\0${since}\0${until}\0${agent ?? ""}`;
  const [state, setState] = useState<{ key: string; report: TaskActivityReport | null; error: string }>(
    { key: "", report: null, error: "" },
  );

  useEffect(() => {
    if (windowError || !since) return;
    const controller = new AbortController();
    const load = () => {
      void api.usage.getTaskActivity({ since, until, agent }, controller.signal)
        .then((report) => {
          if (!controller.signal.aborted) setState({ key, report, error: "" });
        })
        .catch((e: unknown) => {
          if (controller.signal.aborted) return;
          const error = e instanceof Error ? e.message : "could not load the summary";
          setState((prev) => ({ key, report: prev.report, error }));
        });
    };
    load();
    const timer = globalThis.setInterval(load, POLL_MS);
    return () => {
      controller.abort();
      globalThis.clearInterval(timer);
    };
  }, [key, api, since, until, agent, windowError]);

  return { report: state.report, error: state.key === key ? state.error : "", current: state.key === key };
}
