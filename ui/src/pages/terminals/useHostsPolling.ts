import { useCallback, useMemo, useState } from "react";
import { usePolling } from "@/hooks/usePolling";
import { fetchAllAgents, type HostAgents } from "@/lib/aggregate";
import { routeHost, withLastSuccessful } from "./terminalsHosts";

/** Every host's agents, polled, with the last good snapshot of each host kept
 *  so a host that stops answering does not empty its sidebar section. */
export function useHostsPolling(hostId: string | undefined) {
  const [lastSuccessfulHostsById, setLastSuccessfulHostsById] = useState<Map<string, HostAgents>>(
    () => new Map(),
  );
  const fetchHosts = useCallback(async () => {
    const data = await fetchAllAgents();
    setLastSuccessfulHostsById((previous) => {
      const next = new Map(previous);
      for (const entry of data) {
        if (!entry.error) next.set(entry.host.id, entry);
      }
      return next;
    });
    return data;
  }, []);
  // While every server pushes agent state hints, the sidebar refetches on a
  // hint and polls only as a fallback.
  const [agentStatesLive, setAgentStatesLive] = useState(false);
  const { data, refresh } = usePolling(fetchHosts, agentStatesLive ? 15000 : 3000);
  const hosts = useMemo(() => data ?? [], [data]);
  const { liveSelectedHost, selectedHost } = useMemo(
    () => routeHost(hostId, hosts, lastSuccessfulHostsById),
    [hostId, hosts, lastSuccessfulHostsById],
  );
  const sidebarHosts = useMemo(
    () => withLastSuccessful(hosts, lastSuccessfulHostsById),
    [hosts, lastSuccessfulHostsById],
  );
  return { hosts, refresh, setAgentStatesLive, liveSelectedHost, selectedHost, sidebarHosts };
}
