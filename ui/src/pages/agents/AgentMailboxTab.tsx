import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useAgentStatus } from "@/lib/agent";
import Mailbox from "@/pages/mailbox/Mailbox";
import { ClearInbox } from "./ClearInbox";

/** Agent → Mailbox: the agent's mailbox, read and answered by the operator on
 *  its behalf. The toolbar shows what one iteration takes and the inbox cap. */
export default function AgentMailboxTab({
  name,
  batch,
  base,
}: {
  name: string;
  base: string;
  /** messages_batch from the agent view, once loaded. */
  batch?: number;
}) {
  const { status } = useAgentStatus();
  const pending = status?.messages_pending ?? 0;
  const max = status?.messages_max_queue ?? 0;
  const principal = `agent:${name}`;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <nav aria-label="Mailbox navigation" className="flex shrink-0 flex-wrap gap-2 px-4 pt-3 pb-4">
        <Button asChild size="sm" variant="outline">
          <Link to={`${base}/advanced`}>Diagnostics</Link>
        </Button>
        <Button asChild size="sm" variant="outline">
          <Link to={`${base}/work`}>Back to Work</Link>
        </Button>
      </nav>
      <div className="min-h-0 flex-1">
        <Mailbox
          as={principal}
          mode="agent"
          fixedTo={principal}
          toolbar={
            <>
              <span className="text-meta text-muted-foreground tabular-nums">
                batch {batch ?? "—"} · inbox {pending}/{max || "—"}
              </span>
              <ClearInbox name={name} disabled={pending === 0} />
            </>
          }
        />
      </div>
    </div>
  );
}
