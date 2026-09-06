import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useAgentName } from "@/lib/agent";
import { agentGetOn, subscribeAgentEventsOn } from "@/lib/api";
import type { IterationSummary } from "@/lib/types";
import { FullAuditLog } from "@/components/FullAuditLog";
import { IterationAuditLog } from "@/components/IterationAuditLog";
import { Badge } from "@/components/ui/badge";
import { fmtDateTime } from "@/lib/time";
import { cn } from "@/lib/utils";
import { IterationJudgePanel } from "@/components/IterationJudgePanel";
import { paramToHost, targetFor } from "@/lib/terminalsHost";

const terminalStatuses = new Set(["done", "no_i_am_done", "harness_error", "timeout", "killed"]);

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
  const { hostId: hostParam = "local" } = useParams();
  const hostId = paramToHost(hostParam);
  const descriptor = targetFor(hostId);
  const [searchParams, setSearchParams] = useSearchParams();
  const listKey = `${hostId}\0${descriptor?.baseURL ?? ""}\0${descriptor?.token ?? ""}\0${name}`;
  const [listState, setListState] = useState<{ key: string; items: IterationSummary[] } | null>(null);
  const items = listState?.key === listKey ? listState.items : [];
  const loaded = listState?.key === listKey;
  // selected iteration id; null = the Full log (default). Preselect from the
  // ?iteration= query param (e.g. an iteration row on the Usage tab links here);
  // absent param keeps the default Full-log pane.
  const selected = searchParams.get("iteration");
  const select = (iteration: string | null) => {
    const next = new URLSearchParams(searchParams);
    if (iteration) next.set("iteration", iteration);
    else next.delete("iteration");
    setSearchParams(next);
  };

  // Load iterations and keep them fresh: a light 5s poll plus the SSE
  // ["iteration"] stream so newly started/finished iterations appear promptly.
  // Backend returns oldest-first; sort newest-first for display.
  useEffect(() => {
    if (!name) return;
    let current = true;
    let generation = 0;
    const target = targetFor(hostId);
    const load = () => {
      const requestGeneration = ++generation;
      void agentGetOn<{ iterations: IterationSummary[]; count: number }>(target, name, "iterations")
        .then((r) => {
          if (current && requestGeneration === generation) setListState({
            key: listKey,
            items: r.iterations
              .slice()
              .sort((a, b) => b.started_at.localeCompare(a.started_at)),
          });
        })
        .catch(() => { /* keep last on a transient failure */ });
    };
    load();
    const t = window.setInterval(load, 5000);
    const off = subscribeAgentEventsOn(target, name, ["iteration"], () => load());
    return () => { current = false; window.clearInterval(t); off(); };
  }, [hostId, listKey, name]);

  const selectedItem = selected ? items.find((it) => it.id === selected) : null;

  return (
    <div className="flex h-full gap-4">
      <div className="w-64 shrink-0 overflow-auto">
        <button
          onClick={() => select(null)}
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
            onClick={() => select(it.id)}
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
              {it.judge?.latest_completed && <Badge variant="outline">{it.judge.latest_completed.score ?? "—"} {it.judge.latest_completed.verdict || "—"}</Badge>}
              <Badge variant={statusVariant(it.status)}>{it.status}</Badge>
            </span>
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-hidden">
        {selected === null ? (
          <FullAuditLog name={name} />
        ) : selectedItem ? (
          <div className="flex h-full flex-col gap-3">
            <IterationJudgePanel
              agentName={name}
              iterationId={selected}
              terminal={terminalStatuses.has(selectedItem.status)}
              judge={selectedItem.judge ?? { latest_completed: null, active: null }}
            />
            <div className="min-h-0 flex-1">
              <IterationAuditLog
                name={name}
                iterationId={selected}
                iterationStatus={selectedItem.status}
                iterationProductive={selectedItem.productive}
              />
            </div>
          </div>
        ) : loaded ? (
          <p role="status" className="text-sm text-muted-foreground">Iteration {selected} was not found.</p>
        ) : (
          <p className="text-sm text-muted-foreground">Loading iteration…</p>
        )}
      </div>
    </div>
  );
}
