import { useAgentStatus } from "@/lib/agent";
import Mailbox from "@/pages/mailbox/Mailbox";
import { ClearInbox } from "./ClearInbox";

/** Agent → Mailbox: the agent's mailbox, read and answered by the operator on
 *  its behalf. The toolbar shows what one iteration takes and the inbox cap. */
export default function AgentMailboxTab({
  name,
  batch,
}: {
  name: string;
  /** messages_batch from the agent view, once loaded. */
  batch?: number;
}) {
  const { status } = useAgentStatus();
  const pending = status?.messages_pending ?? 0;
  const max = status?.messages_max_queue ?? 0;
  const principal = `agent:${name}`;
  return (
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
  );
}
