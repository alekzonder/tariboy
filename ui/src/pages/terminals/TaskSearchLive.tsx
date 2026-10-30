import { useEffect, useRef } from "react";
import type { HostAgents } from "@/lib/aggregate";
import { createApi } from "@/lib/api";
import { targetFor } from "@/lib/terminalsHost";

export interface TaskHit {
  hostId: string;
  hostLabel: string;
  key: string;
  title: string;
  status: string;
}

const DEBOUNCE_MS = 250;
const LIMIT = 8;

/**
 * Looks the search text up among the tasks of every reachable host, open or
 * closed, 250 ms after the last keystroke. A host that fails adds nothing and
 * says nothing: the search is a shortcut, not the task list. Renders nothing.
 */
export default function TaskSearchLive({
  hosts,
  query,
  onResults,
}: {
  hosts: HostAgents[];
  query: string;
  onResults: (hits: TaskHit[]) => void;
}) {
  const latest = useRef({ hosts, onResults });
  useEffect(() => {
    latest.current = { hosts, onResults };
  });

  useEffect(() => {
    const text = query.trim();
    let cancelled = false;
    const timer = window.setTimeout(() => {
      const reads = latest.current.hosts
        .filter((host) => !host.error)
        .map(async (host): Promise<TaskHit[]> => {
          try {
            const page = await createApi(targetFor(host.host.id)).tasks.listTasks({
              text,
              status_view: "all",
              limit: LIMIT,
            });
            return (page.tasks ?? []).map((task) => ({
              hostId: host.host.id,
              hostLabel: host.host.label,
              key: task.key,
              title: task.title,
              status: task.status,
            }));
          } catch {
            return [];
          }
        });
      void Promise.all(reads).then((all) => {
        if (!cancelled) latest.current.onResults(all.flat());
      });
    }, DEBOUNCE_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [query]);

  return null;
}
