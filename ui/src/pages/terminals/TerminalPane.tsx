import { ApiProvider } from "@/components/ApiProvider";
import { targetFor } from "@/lib/terminalsHost";
import type { AgentSummary } from "@/lib/types";
import AgentConsoleTab from "@/pages/agents/AgentConsoleTab";

// The pane is for an agent on an explicit host, so it is that host's boundary.
export function TerminalPane({ hostId, agent, refresh }: {
  hostId: string; hostLabel?: string; agent: AgentSummary; refresh: () => void;
}) {
  return (
    <ApiProvider target={targetFor(hostId)}>
      <AgentConsoleTab hostId={hostId} agent={agent} refresh={refresh} />
    </ApiProvider>
  );
}
