import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { agentGet, subscribeAgentEvents } from "@/lib/api";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Input } from "@/components/ui/input";
import { AuditRow } from "@/components/AuditRow";
import { collapseThinking, type AuditEvent } from "@/lib/audit";
import { AuditExportActions } from "@/components/AuditExportActions";

const PAGE = 50;
const FILTER_DATALIST = "audit-type-presets";

// FullAuditLog is the full audit history for one agent: recent events load first
// (chat order, newest at the bottom), scrolling to the top pages older events in
// with before=, and the 3s poll / SSE tail appends newer events with since=.
//
// A type/text filter switches the pane into a query view: the whole log is
// scanned server-side (logs?type=&q=) and only the matches render, since the
// paged window only holds a slice of the log. The paging/tail machinery pauses
// while a filter is active and resumes when it is cleared.
export function FullAuditLog({ name }: { name: string }) {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [open, setOpen] = useState<Set<number>>(new Set());
  // Filter state: `type` (a preset from the datalist or a free-form value) and a
  // full-text `query`; either non-empty puts the pane in filtered mode.
  const [type, setType] = useState("");
  const [query, setQuery] = useState("");
  const [presets, setPresets] = useState<string[]>([]);
  const [filtered, setFiltered] = useState<AuditEvent[] | null>(null);
  const [capped, setCapped] = useState(false);
  const filtering = type.trim() !== "" || query.trim() !== "";
  const rootRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const hasMore = useRef(true);
  const loadingOlder = useRef(false);
  const anchor = useRef<number | null>(null); // scrollHeight captured before a prepend
  const viewport = () =>
    rootRef.current?.querySelector<HTMLElement>('[data-slot="scroll-area-viewport"]') ?? null;

  // eventsRef mirrors events so the tail/scroll closures read the latest seqs
  // without re-subscribing the SSE stream on every append.
  const eventsRef = useRef<AuditEvent[]>([]);
  eventsRef.current = events;
  const newestSeq = () => (eventsRef.current.length ? eventsRef.current[eventsRef.current.length - 1].seq : 0);
  const oldestSeq = () => (eventsRef.current.length ? eventsRef.current[0].seq : 0);

  // Initial load: recent 50, reversed to chronological (oldest first).
  useEffect(() => {
    if (!name) return;
    let live = true;
    void agentGet<{ events: AuditEvent[] }>(name, "logs")
      .then((r) => {
        if (!live) return;
        const recent = (r.events ?? []).slice().reverse();
        hasMore.current = (r.events?.length ?? 0) >= PAGE;
        setEvents(recent);
      })
      .catch(() => { if (live) setEvents([]); });
    return () => { live = false; };
  }, [name]);

  // Preset type list, enumerated from the real audit data (distinct types),
  // backing the type filter's datalist. Free-form entries stay allowed.
  useEffect(() => {
    if (!name) return;
    let live = true;
    void agentGet<{ types: string[] }>(name, "logs?distinct=types")
      .then((r) => { if (live) setPresets(r.types ?? []); })
      .catch(() => {});
    return () => { live = false; };
  }, [name]);

  // Filtered mode: debounce, then scan the whole log server-side and show the
  // matches. Runs only while a filter is active; clearing it drops back to the
  // live paged view.
  useEffect(() => {
    if (!name || !filtering) { setFiltered(null); setCapped(false); return; }
    let live = true;
    const t = window.setTimeout(() => {
      const qs = new URLSearchParams();
      if (type.trim()) qs.set("type", type.trim());
      if (query.trim()) qs.set("q", query.trim());
      void agentGet<{ events: AuditEvent[]; capped?: boolean }>(name, `logs?${qs.toString()}`)
        .then((r) => {
          if (!live) return;
          setFiltered((r.events ?? []).slice().reverse()); // newest-first → chronological
          setCapped(!!r.capped);
        })
        .catch(() => { if (live) { setFiltered([]); setCapped(false); } });
    }, 250);
    return () => { live = false; window.clearTimeout(t); };
  }, [name, filtering, type, query]);

  // Live tail: append events newer than what we hold. Reads newestSeq via the
  // ref so this effect subscribes once per agent, not on every append. Paused
  // while filtering so the query view is not disturbed by appends.
  useEffect(() => {
    if (!name || filtering) return;
    const tail = () =>
      void agentGet<{ events: AuditEvent[] }>(name, `logs?since=${newestSeq()}`)
        .then((r) => {
          const fresh = r.events ?? [];
          if (fresh.length) setEvents((prev) => [...prev, ...fresh]);
        })
        .catch(() => {});
    const t = window.setInterval(tail, 3000);
    const off = subscribeAgentEvents(name, ["audit", "iteration"], () => tail());
    return () => { window.clearInterval(t); off(); };
  }, [name, filtering]);

  const loadOlder = () => {
    if (!name || loadingOlder.current || !hasMore.current || eventsRef.current.length === 0) return;
    loadingOlder.current = true;
    const v = viewport();
    anchor.current = v ? v.scrollHeight : null;
    void agentGet<{ events: AuditEvent[] }>(name, `logs?before=${oldestSeq()}&limit=${PAGE}`)
      .then((r) => {
        const older = r.events ?? [];
        hasMore.current = older.length >= PAGE;
        if (older.length) setEvents((prev) => [...older, ...prev]);
        else anchor.current = null;
      })
      .catch(() => { anchor.current = null; })
      .finally(() => { loadingOlder.current = false; });
  };

  // Scroll: track stick-to-bottom, and page older when near the top. Inactive
  // while filtering (the query view is a flat, non-paged list).
  useEffect(() => {
    if (filtering) return;
    const v = viewport();
    if (!v) return;
    const onScroll = () => {
      stick.current = v.scrollHeight - v.scrollTop - v.clientHeight < 40;
      if (v.scrollTop < 40) loadOlder();
    };
    v.addEventListener("scroll", onScroll);
    return () => v.removeEventListener("scroll", onScroll);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, filtering]);

  // After a render: restore the scroll anchor on a prepend, else stick to
  // bottom. Skipped while filtering — the query view manages its own scroll.
  useLayoutEffect(() => {
    if (filtering) return;
    const v = viewport();
    if (!v) return;
    if (anchor.current != null) {
      v.scrollTop += v.scrollHeight - anchor.current;
      anchor.current = null;
    } else if (stick.current) {
      v.scrollTop = v.scrollHeight;
    }
  }, [events, filtering]);

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
      <div className="flex shrink-0 items-center gap-2">
        <Input
          aria-label="Filter by type"
          placeholder="type… (pick or type any)"
          list={FILTER_DATALIST}
          value={type}
          onChange={(e) => setType(e.target.value)}
          className="h-8 w-56"
        />
        <datalist id={FILTER_DATALIST}>
          {presets.map((t) => <option key={t} value={t} />)}
        </datalist>
        <Input
          aria-label="Search text"
          placeholder="search all fields…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="h-8 flex-1"
        />
        {filtering && (
          <button
            onClick={() => { setType(""); setQuery(""); }}
            className="shrink-0 rounded border px-2 py-1 text-xs hover:bg-accent"
          >
            Clear
          </button>
        )}
        <AuditExportActions name={name} />
      </div>
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
