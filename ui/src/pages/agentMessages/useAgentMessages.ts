import { useCallback, useEffect, useState } from "react";
import { useAgentName } from "@/lib/agent";
import type { InboxItem } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { VIEW_STATUS, type View } from "./agentMessagesModel";

// The active view's inbox rows. Rows loaded for another view are hidden until
// the active view's first load lands. Live refresh rides the SSE `message`
// stream with a 3s poll fallback (the SSE hub drops on a full buffer, so it is
// only a hint).
export function useAgentMessages(view: View) {
  const api = useApi();
  const name = useAgentName();
  const [page, setPage] = useState<{ view: View; items: InboxItem[] }>({
    view: "queue",
    items: [],
  });

  const load = useCallback(
    (v: View) =>
      api.messages
        .agentInboxList(name, VIEW_STATUS[v])
        .then((r) => setPage({ view: v, items: r.messages ?? [] }))
        .catch(() => {
          /* keep last rows on a transient failure */
        }),
    [api, name],
  );
  const items = page.view === view ? page.items : [];

  useEffect(() => {
    if (!name) return;
    void load(view);
    const t = window.setInterval(() => void load(view), 3000);
    const off = api.agents.subscribeAgentEvents(name, ["message"], () => void load(view));
    return () => {
      window.clearInterval(t);
      off();
    };
  }, [api, name, view, load]);

  const reload = useCallback(() => load(view), [load, view]);
  return { items, reload };
}
