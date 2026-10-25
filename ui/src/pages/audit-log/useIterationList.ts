import { useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { IterationList, IterationRow } from "@/lib/api";

// useIterationList loads an agent's iterations for AuditLogPage. filterKey
// identifies the filter the rows belong to; queryString is what is sent.
export function useIterationList(name: string, filterKey: string, queryString: string) {
  const api = useApi();
  const descriptor = api.target;
  // A rolling period moves every minute; the list keeps its rows while the
  // moved window reloads instead of blanking, so the period is not in the key.
  const listKey = `${descriptor?.id ?? ""}\0${descriptor?.baseURL ?? ""}\0${descriptor?.token ?? ""}\0${name}\0${filterKey}`;
  const [listState, setListState] = useState<{ key: string; items: IterationRow[] } | null>(null);
  const items = listState?.key === listKey ? listState.items : [];
  const loaded = listState?.key === listKey;

  // Load iterations and keep them fresh: a light 5s poll plus the SSE
  // ["iteration"] stream so newly started/finished iterations appear promptly.
  // Backend returns oldest-first; sort newest-first for display.
  useEffect(() => {
    if (!name) return;
    let current = true;
    let generation = 0;
    const load = () => {
      const requestGeneration = ++generation;
      void api
        .agentGet<IterationList>(name, queryString ? `iterations?${queryString}` : "iterations")
        .then((r) => {
          if (current && requestGeneration === generation)
            setListState({
              key: listKey,
              items: r.iterations.slice().sort((a, b) => b.started_at.localeCompare(a.started_at)),
            });
        })
        .catch(() => {
          /* keep last on a transient failure */
        });
    };
    load();
    const t = window.setInterval(load, 5000);
    const off = api.agents.subscribeAgentEvents(name, ["iteration"], () => load());
    return () => {
      current = false;
      window.clearInterval(t);
      off();
    };
  }, [api, listKey, name, queryString]);

  // setTags patches one loaded row in place, so the badge list updates before
  // the next poll.
  const setTags = (id: string, tags: string[]) =>
    setListState(
      (prev) =>
        prev && {
          ...prev,
          items: prev.items.map((it) => (it.id === id ? { ...it, tags } : it)),
        },
    );

  return { items, loaded, setTags };
}
