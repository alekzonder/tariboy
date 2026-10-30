import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { needsYouRows } from "@/lib/needsYou";
import { hostToParam } from "@/lib/terminalsHost";
import { NeedsYouList } from "@/pages/home/NeedsYouList";
import { taskQueries } from "@/pages/tasks/taskQueries";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";
import Mailbox from "./Mailbox";

/**
 * Inbox: what needs the user on every host of the workspace, then the
 * customer's own mailbox on the page host. A letter row of another host
 * moves the page to that host.
 */
export default function InboxPage({
  multiHost,
  hostControl,
}: {
  multiHost: boolean;
  /** The host selector, shown when the workspace has two or more hosts. */
  hostControl?: ReactNode;
}) {
  const api = useApi();
  const [searchParams, setSearchParams] = useSearchParams();
  const agents = useApiQuery(taskQueries.principals(api)).data?.agents ?? [];
  const needs = useNeedsYou();
  const rows = needsYouRows(needs.hosts);
  const openLetter = (hostId: string, letterId: string) => {
    const next = new URLSearchParams({ letter: letterId });
    if (multiHost) next.set("host", hostToParam(hostId));
    setSearchParams(next);
  };
  return (
    <div className="flex h-full min-h-0 flex-col">
      <section
        aria-label="Needs you"
        className="max-h-[40%] shrink-0 overflow-y-auto border-b px-2 py-3"
      >
        <h2 className="mb-1.5 px-2 text-body font-semibold">Needs you ({rows.length})</h2>
        <NeedsYouList rows={rows} multiHost={multiHost} onOpenLetter={openLetter} />
      </section>
      <div className="min-h-0 flex-1">
        <Mailbox
          as=""
          mode="customer"
          recipients={agents.map((name) => `agent:${name}`)}
          toolbar={hostControl}
          letterId={searchParams.get("letter") ?? ""}
        />
      </div>
    </div>
  );
}
