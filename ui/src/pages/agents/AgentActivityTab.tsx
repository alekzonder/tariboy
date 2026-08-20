import { useEffect, useState } from "react";
import AuditLogPage from "@/pages/AuditLogPage";
import AgentUsagePage from "@/pages/AgentUsagePage";
import { getStatusHistory } from "@/lib/api";
import { useAgentName } from "@/lib/agent";
import type { StatusHistoryEvent } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { fmtDateTime } from "@/lib/time";

type View = "timeline" | "status" | "usage";

export default function AgentActivityTab() {
  const name = useAgentName();
  const [view, setView] = useState<View>("timeline");
  const [history, setHistory] = useState<StatusHistoryEvent[]>([]);
  useEffect(() => {
    if (view !== "status") return;
    void getStatusHistory(name).then((result) => setHistory(result.events)).catch(() => setHistory([]));
  }, [name, view]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex gap-2">
        <Button size="sm" variant={view === "timeline" ? "default" : "outline"} onClick={() => setView("timeline")}>Timeline</Button>
        <Button size="sm" variant={view === "status" ? "default" : "outline"} onClick={() => setView("status")}>Status history</Button>
        <Button size="sm" variant={view === "usage" ? "default" : "outline"} onClick={() => setView("usage")}>Usage</Button>
      </div>
      <div className="min-h-0 flex-1">
        {view === "timeline" ? <AuditLogPage /> : view === "usage" ? <AgentUsagePage /> : (
          <div className="mx-auto max-w-3xl space-y-2">
            {history.map((event, index) => (
              <div key={`${event.ts}-${index}`} className="rounded-md border p-3 text-sm">
                <div className="text-xs text-muted-foreground">{fmtDateTime(event.ts)}</div>
                <div>{event.message}</div>
              </div>
            ))}
            {history.length === 0 && <p className="text-sm text-muted-foreground">No status changes recorded.</p>}
          </div>
        )}
      </div>
    </div>
  );
}
