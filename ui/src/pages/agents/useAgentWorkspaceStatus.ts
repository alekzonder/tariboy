import { useCallback, useEffect, useState } from "react";
import { useDaemons } from "@/components/daemonContext";
import { useApi } from "@/components/apiContext";
import { isConnectionError, type AgentStatus, type AgentView } from "@/lib/api";
import { useAgentStatesSocket } from "@/hooks/useMailSocket";
import { useCoalesced } from "@/hooks/useCoalesced";

type Connection = "selecting" | "ready" | "unavailable";

// Selects the route's host and keeps it selected, while the agent's status and
// view refresh on a state hint or a poll.
export function useAgentWorkspaceStatus({
  hostId,
  agentName,
  unavailable,
}: {
  hostId: string;
  agentName: string;
  unavailable: boolean;
}) {
  const { activeId, daemons, select } = useDaemons();
  const [connection, setConnection] = useState<Connection>("selecting");
  const [status, setStatus] = useState<AgentStatus | null>(null);
  const [view, setView] = useState<{ key: string; view: AgentView | null }>({
    key: "",
    view: null,
  });
  const agentKey = `${hostId}\0${agentName}`;
  const currentView = view.key === agentKey ? view.view : null;
  const api = useApi();

  useEffect(() => {
    let cancelled = false;
    void select(hostId).then((ok) => {
      if (!cancelled) setConnection(ok ? "ready" : "unavailable");
    });
    return () => {
      cancelled = true;
    };
  }, [hostId, select]);

  // The URL is authoritative inside an agent workspace. A global host change
  // must never retarget legacy tab components to a same-named agent elsewhere.
  // Hide the tabs immediately and restore the host named by the route.
  useEffect(() => {
    if (connection !== "ready" || activeId === hostId) return;
    let cancelled = false;
    void select(hostId).then((ok) => {
      if (!cancelled && !ok) setConnection("unavailable");
    });
    return () => {
      cancelled = true;
    };
  }, [activeId, connection, hostId, select]);

  const refreshStatus = useCallback(async () => {
    if (connection !== "ready" || unavailable) return;
    try {
      setStatus(await api.agents.getAgentStatus(agentName));
    } catch (error) {
      // An unreachable host keeps the last status in the header.
      if (!isConnectionError(error)) setStatus(null);
    }
    try {
      setView({ key: agentKey, view: await api.agents.getAgent(agentName) });
    } catch {
      // Preserve the last successful value and retry with the next status poll.
    }
  }, [agentName, agentKey, api, connection, unavailable]);

  // A state hint for this agent refetches at once; the poll is then only a
  // fallback. A host without the socket keeps the short poll.
  const refreshOnHint = useCoalesced(() => void refreshStatus(), 500);
  const statesSocket = useAgentStatesSocket({
    enabled: connection === "ready" && !unavailable,
    onHint: (hint) => {
      if (hint.agent === agentName) refreshOnHint();
    },
  });
  const statusPollMs = statesSocket === "open" ? 15000 : 3000;
  useEffect(() => {
    if (connection !== "ready" || unavailable) return;
    void Promise.resolve().then(refreshStatus);
    const timer = window.setInterval(() => void refreshStatus(), statusPollMs);
    return () => window.clearInterval(timer);
  }, [connection, refreshStatus, unavailable, statusPollMs]);

  return { activeId, daemons, connection, status, currentView, refreshStatus };
}
