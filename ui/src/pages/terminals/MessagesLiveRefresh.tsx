import { useCallback, useEffect, useState } from "react";
import { useCoalesced } from "@/hooks/useCoalesced";
import { useAgentStatesSocket, useMailSocket } from "@/hooks/useMessagesSocket";
import { targetFor } from "@/lib/terminalsHost";

/** Two sockets per server. It renders nothing: its only job is to turn a new
 *  letter or agent state change into an immediate refresh of the
 *  authoritative HTTP snapshot, so the sidebar updates as it happens instead of
 *  on the next poll. */
function HostMessagesWatch({
  hostId,
  onMessage,
  onState,
  onAgentStates,
}: {
  hostId: string;
  onMessage: () => void;
  onState: () => void;
  onAgentStates: (hostId: string, open: boolean) => void;
}) {
  const target = targetFor(hostId);
  const enabled = !target || Boolean(target.baseURL);
  useMailSocket({ target, enabled, onHint: onMessage });
  const states = useAgentStatesSocket({ target, enabled, onHint: onState });
  useEffect(() => {
    onAgentStates(hostId, states === "open");
  }, [hostId, states, onAgentStates]);
  return null;
}

/** Watch every configured server's mail and agent states at once. A burst
 *  of hints is one refresh, not one per hint: the snapshot is the same either
 *  way. onLive reports whether every server's agent states socket is open, so
 *  the caller can poll slowly while hints keep the sidebar current. */
export function MessagesLiveRefresh({
  hostIds,
  onChange,
  onLive,
}: {
  hostIds: string[];
  onChange: () => void;
  onLive?: (live: boolean) => void;
}) {
  // Mail reorders quickly; agent state hints come in bursts from busy
  // agents, so they refetch at most once a second.
  const onMessage = useCoalesced(onChange, 250);
  const onState = useCoalesced(onChange, 1000);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const onAgentStates = useCallback((hostId: string, isOpen: boolean) => {
    setOpen((previous) =>
      previous[hostId] === isOpen ? previous : { ...previous, [hostId]: isOpen },
    );
  }, []);
  const live = hostIds.length > 0 && hostIds.every((id) => open[id]);
  useEffect(() => {
    onLive?.(live);
  }, [live, onLive]);
  return (
    <>
      {hostIds.map((hostId) => (
        <HostMessagesWatch
          key={hostId}
          hostId={hostId}
          onMessage={onMessage}
          onState={onState}
          onAgentStates={onAgentStates}
        />
      ))}
    </>
  );
}
