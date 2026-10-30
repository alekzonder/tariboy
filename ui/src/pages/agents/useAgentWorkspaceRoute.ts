import { useLocation, useParams } from "react-router-dom";
import { hostToParam } from "@/lib/terminalsHost";
import { resolveAgentTab } from "./agentWorkspaceModel";

// The workspace route: the tab the URL names (or where an old or unknown one
// redirects), and the base every tab link uses.
export function useAgentWorkspaceRoute({
  hostId,
  agentName,
}: {
  hostId: string;
  agentName: string;
}) {
  const { tab = "" } = useParams();
  const { search } = useLocation();
  const base = `/agents/${hostToParam(hostId)}/${encodeURIComponent(agentName)}`;
  const resolved = resolveAgentTab({ tab, base, search });
  return { base, ...resolved };
}
