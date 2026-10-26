import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { taskQueries } from "@/pages/tasks/taskQueries";
import Mailbox from "./Mailbox";

/** Server → Mailbox: the customer's own mailbox on this host. */
export default function ServerMailboxTab({ hostId }: { hostId: string }) {
  const api = useApi();
  const agents = useApiQuery(taskQueries.principals(api)).data?.agents ?? [];
  return (
    <Mailbox
      as=""
      mode="customer"
      hostId={hostId}
      recipients={agents.map((name) => `agent:${name}`)}
    />
  );
}
