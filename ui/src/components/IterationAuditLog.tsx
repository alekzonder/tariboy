import { useEffect, useRef, useState } from "react";
import { agentGet, subscribeAgentEvents } from "@/lib/api";
import { ScrollArea } from "@/components/ui/scroll-area";
import { AuditRow } from "@/components/AuditRow";
import { collapseThinking, hasHarnessOutput, mergeProxyCalls, type AuditEvent } from "@/lib/audit";
import { fetchTranscript, type Call } from "@/lib/transcript";
import { AuditExportActions } from "@/components/AuditExportActions";

// shortIter condenses an iteration id (<agent>-<timestamp>-<n>) to a readable
// tail like "065606-3" — the HHMMSS of the timestamp plus the counter. Ids
// without a numeric timestamp segment fall back to their last two segments.
function shortIter(id: string): string {
  if (!id) return "";
  const parts = id.split("-");
  if (parts.length < 2) return id;
  const n = parts[parts.length - 1];
  let ts = parts[parts.length - 2];
  if (/^\d{8,}$/.test(ts)) ts = ts.slice(-6);
  return `${ts}-${n}`;
}

// IterationAuditLog shows the audit events of a single iteration — the running
// one on Overview, or the last one once it finished. A header chip names the
// iteration and its status; the events stream chat-style, pinned to the bottom.
export function IterationAuditLog({
  name,
  iterationId,
  iterationStatus,
  iterationProductive,
}: {
  name: string;
  iterationId: string;
  iterationStatus: string;
  // false when the iteration finished with `i-am-done --idle`; renders an
  // "idle" marker in the header. The backend always sends a bool (column is
  // NOT NULL DEFAULT 1); this prop is optional only because callers may omit
  // it when no iteration is selected, in which case the header is unchanged.
  iterationProductive?: boolean;
}) {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [calls, setCalls] = useState<Call[]>([]);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const rootRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const viewport = () =>
    rootRef.current?.querySelector<HTMLElement>('[data-slot="scroll-area-viewport"]') ?? null;

  useEffect(() => {
    if (!name || !iterationId) {
      setEvents([]);
      setCalls([]);
      return;
    }
    const load = () =>
      void agentGet<{ events: AuditEvent[] }>(name, `logs?iteration=${encodeURIComponent(iterationId)}`)
        .then((r) => setEvents(r.events ?? []))
        .catch(() => { /* keep last events on a transient failure */ });
    const loadTranscript = () =>
      void fetchTranscript(name, iterationId).then(setCalls).catch(() => { /* keep last */ });
    load();
    loadTranscript();
    const t = window.setInterval(() => { load(); loadTranscript(); }, 3000);
    const off = subscribeAgentEvents(name, ["audit", "iteration", "proxy"], () => { load(); loadTranscript(); });
    return () => { window.clearInterval(t); off(); };
  }, [name, iterationId]);

  useEffect(() => {
    const v = viewport();
    if (!v) return;
    const onScroll = () => { stick.current = v.scrollHeight - v.scrollTop - v.clientHeight < 40; };
    v.addEventListener("scroll", onScroll);
    return () => v.removeEventListener("scroll", onScroll);
  }, []);
  useEffect(() => {
    const v = viewport();
    if (v && stick.current) v.scrollTop = v.scrollHeight;
  }, [events, calls]);

  const toggle = (seq: number) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(seq)) next.delete(seq);
      else next.add(seq);
      return next;
    });

  const shortId = shortIter(iterationId);

  return (
    <div className="flex h-full flex-col gap-1">
      <div className="flex shrink-0 items-center justify-between gap-2">
        <div className="font-mono text-xs text-muted-foreground">
          {iterationId ? `iteration ${shortId} · ${iterationStatus || "?"}` : "no iterations yet"}
          {iterationId && iterationProductive === false && " · idle"}
        </div>
        {iterationId && <AuditExportActions name={name} iteration={iterationId} />}
      </div>
      <ScrollArea ref={rootRef} className="min-h-0 flex-1 rounded border bg-muted/30 p-2">
        {(() => {
          const mode = hasHarnessOutput(events) ? "enrich" : "full";
          const rows = mergeProxyCalls(collapseThinking(events), calls, mode);
          return rows.map((r) => (
            <AuditRow key={r.key} row={r} open={open.has(r.key)} onToggle={toggle} name={name} iteration={iterationId} />
          ));
        })()}
        {events.length === 0 && <p className="text-sm text-muted-foreground">No events.</p>}
      </ScrollArea>
    </div>
  );
}
