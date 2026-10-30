import { useEffect, useRef } from "react";
import { createApi } from "@/lib/api";
import type { HostNeeds } from "@/lib/needsYou";
import { targetFor } from "@/lib/terminalsHost";
import { listAllTasks } from "@/pages/tasks/listAllTasks";

export interface NeedsYouHost {
  id: string;
  label: string;
  /** The agent poll could not reach the host, so it is not asked either. */
  error?: string;
}

async function readHost(host: NeedsYouHost): Promise<HostNeeds> {
  const base = { hostId: host.id, hostLabel: host.label, tasks: [], letters: [], open: 0 };
  if (host.error) return { ...base, error: host.error };
  const api = createApi(targetFor(host.id));
  try {
    const [tasks, unread, inbox] = await Promise.all([
      listAllTasks({ status_view: "active" }, api.tasks),
      api.mail.unread(),
      api.mail.list({ box: "inbox" }),
    ]);
    return { ...base, tasks: tasks.tasks, letters: inbox.letters ?? [], open: unread.open };
  } catch (error) {
    return { ...base, error: error instanceof Error ? error.message : String(error) };
  }
}

/**
 * Reads every host's active tasks and the customer's inbox at the sidebar's
 * poll interval and hands the result up. It renders nothing; it loads after
 * the shell so the entry chunk does not carry the API client.
 */
export default function NeedsYouLive({
  hosts,
  intervalMs,
  reloadToken,
  onChange,
}: {
  hosts: NeedsYouHost[];
  intervalMs: number;
  /** A new value reads at once. */
  reloadToken: number;
  onChange: (hosts: HostNeeds[]) => void;
}) {
  const latest = useRef({ hosts, onChange });
  useEffect(() => {
    latest.current = { hosts, onChange };
  });
  // The sidebar poll hands over a fresh array every few seconds; only a change
  // of hosts or of their reachability restarts the reads.
  const hostsKey = hosts
    .map((host) => `${host.id}\u0001${host.label}\u0001${host.error ?? ""}`)
    .join("\u0000");

  useEffect(() => {
    let cancelled = false;
    let reading = false;
    const tick = async () => {
      // A slow host must not pile reads on top of each other.
      if (reading) return;
      reading = true;
      try {
        const result = await Promise.all(latest.current.hosts.map(readHost));
        if (!cancelled) latest.current.onChange(result);
      } finally {
        reading = false;
      }
    };
    void tick();
    const timer = window.setInterval(() => void tick(), intervalMs);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [hostsKey, intervalMs, reloadToken]);

  return null;
}
