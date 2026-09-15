import { useCallback, useEffect, useRef } from "react";
import { useMessagesSocket } from "@/hooks/useMessagesSocket";
import { targetFor } from "@/lib/terminalsHost";

/** One socket per server. It renders nothing: its only job is to turn a live
 *  publication into an immediate refresh of the authoritative HTTP snapshot, so
 *  the chat list reorders as a message lands instead of on the next poll. */
function HostMessagesWatch({ hostId, onChange }: { hostId: string; onChange: () => void }) {
  const target = targetFor(hostId);
  useMessagesSocket({
    target,
    enabled: !target || Boolean(target.baseURL),
    onHint: onChange,
  });
  return null;
}

/** Watch every configured server's messages at once. A burst of publications is
 *  one refresh, not one per message: the snapshot is the same either way. */
export function MessagesLiveRefresh({ hostIds, onChange }: {
  hostIds: string[];
  onChange: () => void;
}) {
  const latest = useRef(onChange);
  useEffect(() => { latest.current = onChange; }, [onChange]);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => { if (timer.current !== undefined) clearTimeout(timer.current); }, []);
  const coalesced = useCallback(() => {
    if (timer.current !== undefined) return;
    timer.current = setTimeout(() => {
      timer.current = undefined;
      latest.current();
    }, 250);
  }, []);
  return (
    <>
      {hostIds.map((hostId) => (
        <HostMessagesWatch key={hostId} hostId={hostId} onChange={coalesced} />
      ))}
    </>
  );
}
