import { useState } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { AuditRow } from "@/components/AuditRow";
import { collapseThinking } from "@/lib/audit";
import { AuditFilterBar } from "@/components/audit/AuditFilterBar";
import { useAuditFilter } from "@/components/audit/useAuditFilter";
import { useAuditPaging } from "@/components/audit/useAuditPaging";

// FullAuditLog is the full audit history for one agent: recent events load first
// (chat order, newest at the bottom), scrolling to the top pages older events in
// with before=, and the 3s poll / SSE tail appends newer events with since=.
//
// A type/text filter switches the pane into a query view: the whole log is
// scanned server-side (logs?type=&q=) and only the matches render, since the
// paged window only holds a slice of the log. The paging/tail machinery pauses
// while a filter is active and resumes when it is cleared.
export function FullAuditLog({ name }: { name: string }) {
  const [open, setOpen] = useState<Set<number>>(new Set());
  const { type, query, presets, filtered, capped, filtering, setFilter } = useAuditFilter(name);
  const { events, rootRef } = useAuditPaging(name, filtering);

  const toggle = (seq: number) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(seq)) next.delete(seq);
      else next.add(seq);
      return next;
    });

  const rows = filtering ? (filtered ?? []) : events;

  return (
    <div className="flex h-full flex-col gap-2">
      <AuditFilterBar
        name={name}
        type={type}
        query={query}
        presets={presets}
        filtering={filtering}
        setFilter={setFilter}
      />
      {filtering && (
        <p className="shrink-0 text-xs text-muted-foreground">
          {filtered === null
            ? "Searching…"
            : `${rows.length} match${rows.length === 1 ? "" : "es"}${capped ? " (showing newest 500)" : ""}`}
        </p>
      )}
      <ScrollArea ref={rootRef} className="min-h-0 flex-1 rounded border bg-muted/30 p-2">
        {collapseThinking(rows).map((r) => (
          <AuditRow key={r.key} row={r} open={open.has(r.key)} onToggle={toggle} />
        ))}
        {rows.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {filtering && filtered !== null ? "No matching events." : "No events."}
          </p>
        )}
      </ScrollArea>
    </div>
  );
}
