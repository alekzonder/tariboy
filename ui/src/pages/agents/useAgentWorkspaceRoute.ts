import { useParams, useSearchParams } from "react-router-dom";
import { hostToParam } from "@/lib/terminalsHost";
import { useUiMode } from "@/lib/uiMode";
import { agentTabs, resolveAgentTab } from "./agentWorkspaceModel";

// The workspace route: which tabs the UI mode shows, the tab the URL names (or
// where an unknown or hidden one redirects), and the base every tab link uses.
export function useAgentWorkspaceRoute({
  hostId,
  agentName,
}: {
  hostId: string;
  agentName: string;
}) {
  const simple = useUiMode() === "simple";
  const tabs = agentTabs(simple);
  const { tab = "console" } = useParams();
  const [searchParams] = useSearchParams();
  const base = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agentName)}`;
  const resolved = resolveAgentTab({ tab, simple, base });
  return { tabs, base, initialTaskKey: searchParams.get("task") ?? undefined, ...resolved };
}
