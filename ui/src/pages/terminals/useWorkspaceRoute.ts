import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import { paramToHost } from "@/lib/terminalsHost";
import { inWorkspace, useWorkspaces, workspaceScopeHost } from "@/lib/workspaces";

/**
 * The host of the console route, kept inside the current workspace. An agent
 * or team route names its host in the path. A `scoped` page (Tasks, Inbox,
 * Activity, Metrics, Library, Settings) takes `?host=`; without it the page
 * shows the remembered host while the workspace holds it, otherwise the first
 * of `workspaceHostIds` in the workspace. In an empty workspace a scoped page
 * has no host, except Settings, which stays open so the workspace can be filled.
 */
export function useWorkspaceRoute({
  scoped,
  settings,
  workspaceHostIds,
}: {
  scoped: boolean;
  settings: boolean;
  /** Every known host id, the current workspace's sidebar order first. */
  workspaceHostIds: string[];
}) {
  const { hostId: hostParam } = useParams();
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();
  const { activeId } = useDaemons();
  const workspaces = useWorkspaces();
  const workspaceHost = workspaceScopeHost(workspaces, workspaceHostIds, activeId);
  const named = scoped ? searchParams.get("host") : (hostParam ?? null);
  const hostId =
    named !== null
      ? paramToHost(named)
      : scoped
        ? (workspaceHost ?? (settings ? activeId : undefined))
        : undefined;

  /** After a pick of workspace `id`: a route on a host outside it opens Home,
   *  and a scoped page stays open without its query, which named that host
   *  and its task or letter. */
  const followWorkspace = (id: string) => {
    if (hostId === undefined || inWorkspace(workspaces, hostId, id)) return;
    if (!scoped) navigate("/");
    else if (location.search) navigate(location.pathname);
  };

  return { hostId, pageHostId: hostId ?? workspaceHost ?? activeId, followWorkspace };
}
