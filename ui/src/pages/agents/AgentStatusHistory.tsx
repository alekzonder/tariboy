import { useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import { useAgentName } from "@/lib/agent";
import type { AgentStatusEvent } from "@/lib/api";
import { fmtDateTime } from "@/lib/time";

/** The agent's status changes inside the Activity period. An empty since (an
 *  invalid custom range) shows every change. */
export default function AgentStatusHistory({ since, until }: { since: string; until: string }) {
  const api = useApi();
  const name = useAgentName();
  const [history, setHistory] = useState<AgentStatusEvent[]>([]);
  useEffect(() => {
    void api.agents
      .getStatusHistory(name)
      .then((result) => setHistory(result.events))
      .catch(() => setHistory([]));
  }, [api, name]);
  const inPeriod = history.filter((event) => {
    const at = Date.parse(event.ts);
    return !since || (at >= Date.parse(since) && at < Date.parse(until));
  });

  return (
    <div className="mx-auto w-full max-w-3xl space-y-2">
      {inPeriod.map((event, index) => (
        <div key={`${event.ts}-${index}`} className="rounded-md border p-3 text-sm">
          <div className="text-xs text-muted-foreground">{fmtDateTime(event.ts)}</div>
          <div>{event.message}</div>
        </div>
      ))}
      {inPeriod.length === 0 && (
        <p className="text-sm text-muted-foreground">No status changes in this period.</p>
      )}
    </div>
  );
}
