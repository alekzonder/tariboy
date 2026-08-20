import type { AgentSummary } from "@/lib/types";
import AgentConsoleTab from "@/pages/agents/AgentConsoleTab";

export function TerminalPane({ hostId, agent, refresh }: {
  hostId: string; hostLabel?: string; agent: AgentSummary; refresh: () => void;
}) {
  return <AgentConsoleTab hostId={hostId} agent={agent} refresh={refresh} />;
}
