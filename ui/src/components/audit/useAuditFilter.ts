import { useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { AuditLogEvent } from "@/lib/audit";

// useAuditFilter owns FullAuditLog's type/text filter. Either non-empty puts
// the pane in filtered mode: the whole log is scanned server-side
// (logs?type=&q=) and only the matches are returned in `filtered`.
export function useAuditFilter(name: string) {
  const api = useApi();
  // Filter state: `type` (a preset from the datalist or a free-form value) and a
  // full-text `query`; either non-empty puts the pane in filtered mode.
  const [type, setType] = useState("");
  const [query, setQuery] = useState("");
  const [presets, setPresets] = useState<string[]>([]);
  const [filtered, setFiltered] = useState<AuditLogEvent[] | null>(null);
  const [capped, setCapped] = useState(false);
  const filtering = type.trim() !== "" || query.trim() !== "";
  // Leaving filtered mode drops the old matches, so the next filter starts at "Searching…".
  const setFilter = (nextType: string, nextQuery: string) => {
    setType(nextType);
    setQuery(nextQuery);
    if (nextType.trim() === "" && nextQuery.trim() === "") {
      setFiltered(null);
      setCapped(false);
    }
  };

  // Preset type list, enumerated from the real audit data (distinct types),
  // backing the type filter's datalist. Free-form entries stay allowed.
  useEffect(() => {
    if (!name) return;
    let live = true;
    void api.audit
      .listAuditEventTypes(name)
      .then((r) => {
        if (live) setPresets(r.types ?? []);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [api, name]);

  // Filtered mode: debounce, then scan the whole log server-side and show the
  // matches. Runs only while a filter is active; clearing it drops back to the
  // live paged view.
  useEffect(() => {
    if (!name || !filtering) return;
    let live = true;
    const t = window.setTimeout(() => {
      const qs = new URLSearchParams();
      if (type.trim()) qs.set("type", type.trim());
      if (query.trim()) qs.set("q", query.trim());
      void api.audit
        .listAuditEvents(name, qs.toString())
        .then((r) => {
          if (!live) return;
          setFiltered((r.events ?? []).slice().reverse()); // newest-first → chronological
          setCapped(!!r.capped);
        })
        .catch(() => {
          if (live) {
            setFiltered([]);
            setCapped(false);
          }
        });
    }, 250);
    return () => {
      live = false;
      window.clearTimeout(t);
    };
  }, [api, name, filtering, type, query]);

  return { type, query, presets, filtered, capped, filtering, setFilter };
}
