import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useAgentName } from "@/lib/agent";
import { listIterations, subscribeAgentEvents } from "@/lib/api";
import type { IterationSummary } from "@/lib/types";
import { FullAuditLog } from "@/components/FullAuditLog";
import { IterationAuditLog } from "@/components/IterationAuditLog";
import { Badge } from "@/components/ui/badge";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";

// Map a backend iteration status to a badge variant. Backend Status is one of
// running|done|no_i_am_done|harness_error|timeout|killed (internal/agent/
// agent.go): a completed-clean run reads as 'default', the failure states stand
// out as 'destructive', and in-flight/incomplete ('running', 'no_i_am_done')
// stay neutral 'secondary'.
function statusVariant(
  status: string,
): "default" | "secondary" | "destructive" {
  if (status === "done") return "default";
  if (status === "harness_error" || status === "timeout" || status === "killed")
    return "destructive";
  return "secondary";
}

// AuditLogPage is the merged Iterations + Audit Log view: one tab, a left column
// listing "Full log" plus every iteration (newest first), and a right pane that
// shows the full cross-iteration stream (default) or a single iteration's log.
export default function AuditLogPage() {
  const name = useAgentName();
  const [searchParams] = useSearchParams();
  const [items, setItems] = useState<IterationSummary[]>([]);
  // selected iteration id; null = the Full log (default). Preselect from the
  // ?iteration= query param (e.g. an iteration row on the Usage tab links here);
  // absent param keeps the default Full-log pane.
  const [selected, setSelected] = useState<string | null>(() => searchParams.get("iteration"));

  // Load iterations and keep them fresh: a light 5s poll plus the SSE
  // ["iteration"] stream so newly started/finished iterations appear promptly.
  // Backend returns oldest-first; sort newest-first for display.
  useEffect(() => {
    if (!name) return;
    const load = () =>
      void listIterations(name)
        .then((r) =>
          setItems(
            r.iterations
              .slice()
              .sort((a, b) => b.started_at.localeCompare(a.started_at)),
          ),
        )
        .catch(() => { /* keep last on a transient failure */ });
    load();
    const t = window.setInterval(load, 5000);
    const off = subscribeAgentEvents(name, ["iteration"], () => load());
    return () => { window.clearInterval(t); off(); };
  }, [name]);

  const selectedItem = selected ? items.find((it) => it.id === selected) : null;

  // If the selected iteration is pruned from items (e.g. the 5s poll drops it),
  // fall back to the Full log rather than leaving the right pane pinned to a
  // stale iteration id with a blank status chip. Wait for items to load first so
  // a ?iteration= preselection isn't cleared before the initial fetch lands.
  useEffect(() => {
    if (selected !== null && items.length > 0 && !selectedItem) setSelected(null);
  }, [selected, selectedItem, items.length]);

  return (
    <div className="flex h-full gap-4">
      <div className="w-64 shrink-0 overflow-auto">
        <button
          onClick={() => setSelected(null)}
          className={cn(
            "flex w-full items-center px-2 py-1.5 text-left text-sm hover:bg-accent",
            selected === null && "bg-accent",
          )}
        >
          Full log
        </button>
        {items.map((it) => (
          <button
            key={it.id}
            onClick={() => setSelected(it.id)}
            className={cn(
              "flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-sm hover:bg-accent",
              selected === it.id && "bg-accent",
            )}
          >
            <span className="font-mono text-xs">{fmtDateTime(it.started_at)}</span>
            <span className="flex items-center gap-1">
              {it.productive === false && (
                <Badge variant="outline" title="finished with i-am-done --idle (no productive work)">
                  idle
                </Badge>
              )}
              <Badge variant={statusVariant(it.status)}>{it.status}</Badge>
            </span>
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-hidden">
        {selected === null ? (
          <FullAuditLog name={name} />
        ) : (
          <IterationAuditLog
            name={name}
            iterationId={selected}
            iterationStatus={selectedItem?.status ?? ""}
            iterationProductive={selectedItem?.productive}
          />
        )}
      </div>
    </div>
  );
}
