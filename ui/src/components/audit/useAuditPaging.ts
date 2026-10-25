import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { AuditLogEvent } from "@/lib/audit";

const PAGE = 50;

// useAuditPaging holds FullAuditLog's paged window: recent events load first
// (chat order, newest at the bottom), scrolling to the top pages older events in
// with before=, and the 3s poll / SSE tail appends newer events with since=.
// The paging/tail machinery pauses while `filtering` and resumes when it is
// cleared. rootRef goes on the ScrollArea whose viewport is tracked.
export function useAuditPaging(name: string, filtering: boolean) {
  const api = useApi();
  const [events, setEvents] = useState<AuditLogEvent[]>([]);
  const rootRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const hasMore = useRef(true);
  const loadingOlder = useRef(false);
  const anchor = useRef<number | null>(null); // scrollHeight captured before a prepend
  const viewport = () =>
    rootRef.current?.querySelector<HTMLElement>('[data-slot="scroll-area-viewport"]') ?? null;

  // eventsRef mirrors events so the tail/scroll closures read the latest seqs
  // without re-subscribing the SSE stream on every append.
  const eventsRef = useRef<AuditLogEvent[]>([]);
  useEffect(() => {
    eventsRef.current = events;
  }, [events]);
  const newestSeq = () =>
    eventsRef.current.length ? eventsRef.current[eventsRef.current.length - 1].seq : 0;
  const oldestSeq = () => (eventsRef.current.length ? eventsRef.current[0].seq : 0);

  // Initial load: recent 50, reversed to chronological (oldest first).
  useEffect(() => {
    if (!name) return;
    let live = true;
    void api.audit
      .listAuditEvents(name)
      .then((r) => {
        if (!live) return;
        const recent = (r.events ?? []).slice().reverse();
        hasMore.current = (r.events?.length ?? 0) >= PAGE;
        setEvents(recent);
      })
      .catch(() => {
        if (live) setEvents([]);
      });
    return () => {
      live = false;
    };
  }, [api, name]);

  // Live tail: append events newer than what we hold. Reads newestSeq via the
  // ref so this effect subscribes once per agent, not on every append. Paused
  // while filtering so the query view is not disturbed by appends.
  useEffect(() => {
    if (!name || filtering) return;
    const tail = () =>
      void api.audit
        .listAuditEvents(name, `since=${newestSeq()}`)
        .then((r) => {
          const fresh = r.events ?? [];
          if (fresh.length) setEvents((prev) => [...prev, ...fresh]);
        })
        .catch(() => {});
    const t = window.setInterval(tail, 3000);
    const off = api.agents.subscribeAgentEvents(name, ["audit", "iteration"], () => tail());
    return () => {
      window.clearInterval(t);
      off();
    };
  }, [api, name, filtering]);

  const loadOlder = useCallback(() => {
    if (!name || loadingOlder.current || !hasMore.current || eventsRef.current.length === 0) return;
    loadingOlder.current = true;
    const v = viewport();
    anchor.current = v ? v.scrollHeight : null;
    void api.audit
      .listAuditEvents(name, `before=${oldestSeq()}&limit=${PAGE}`)
      .then((r) => {
        const older = r.events ?? [];
        hasMore.current = older.length >= PAGE;
        if (older.length) setEvents((prev) => [...older, ...prev]);
        else anchor.current = null;
      })
      .catch(() => {
        anchor.current = null;
      })
      .finally(() => {
        loadingOlder.current = false;
      });
  }, [api, name]);

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
  }, [filtering, loadOlder]);

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

  return { events, rootRef };
}
