import { useNavigate, useParams } from "react-router-dom";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { useDaemons } from "@/components/daemonContext";
import type { HostAgents } from "@/lib/aggregate";
import { scopedPath } from "@/lib/navRoutes";
import { hostToParam } from "@/lib/terminalsHost";
import { defaultAgentTab } from "@/lib/uiMode";
import type { CloneAgentSource } from "./CreateAgentDialog";
import { AgentSidebar, type SidebarActions } from "./sidebar/AgentSidebar";
import type { ReorderKind } from "./sidebar/sidebarDnd";
import { useSharedSidebarState } from "./sidebarStateContext";
import { agentPath, fallbackHostLabel } from "./terminalsHosts";
import type { useServerActions } from "./useServerActions";

export type CreateDialogState = {
  hostId: string;
  imageRef?: string;
  cloneSource?: CloneAgentSource;
};

/**
 * The navigation beside the content island: a column on a wide screen, a
 * drawer on a compact one. A pick in the drawer leaves it, because navigation
 * or a dialog takes over the screen.
 */
export function TerminalsSidebar({
  hosts,
  labelHosts,
  hostId,
  pageHostId,
  counts,
  query,
  onQuery,
  server,
  onReorder,
  onCreate,
}: {
  /** The hosts of the current workspace, in sidebar order. */
  hosts: HostAgents[];
  /** Every host, to name a clone source. */
  labelHosts: HostAgents[];
  /** The route's host, from its path or `?host=`; none on Home. */
  hostId?: string;
  /** The host Library and Settings open on. */
  pageHostId: string;
  counts: { tasks: number | null; inbox: number | null };
  query: string;
  onQuery: (query: string) => void;
  server: ReturnType<typeof useServerActions>;
  onReorder: (kind: ReorderKind, id: string, ids: string[]) => void;
  onCreate: (state: CreateDialogState) => void;
}) {
  const { agent: agentName, tab: agentTab } = useParams();
  const navigate = useNavigate();
  const { daemons, appVersion } = useDaemons();
  const sidebar = useSharedSidebarState();

  const closing =
    <A extends unknown[]>(act: (...args: A) => void) =>
    (...args: A) => {
      sidebar.setDrawerOpen(false);
      act(...args);
    };
  const actions: SidebarActions = {
    selectedHostId: hostId,
    onSelectHost: closing((id: string) => navigate(scopedPath("tasks", id))),
    // Switching agents keeps the tab the operator is reading, so the same
    // view answers the same question about the next agent. Any query (a
    // selected task) belongs to the agent being left, so it is dropped.
    onSelect: closing((h: string, a: string) =>
      navigate(agentPath(h, a, agentName && agentTab ? agentTab : defaultAgentTab())),
    ),
    onSelectTeam: closing((h: string, team: string) => {
      navigate(`/agents/${hostToParam(h)}/teams/${encodeURIComponent(team)}`);
    }),
    onReorder,
    onClone: closing((cloneHostId: string, cloneAgentName: string) => {
      const hostLabel =
        labelHosts.find((entry) => entry.host.id === cloneHostId)?.host.label ??
        fallbackHostLabel(cloneHostId);
      onCreate({
        hostId: cloneHostId,
        cloneSource: { hostId: cloneHostId, agentName: cloneAgentName, hostLabel },
      });
    }),
    onCreate: closing((id: string) => onCreate({ hostId: id })),
    onEditServer: closing((id: string) => void server.editServer(id)),
    onRemoveServer: (id) => void server.removeServer(id),
    onConnectHost: (id) => void server.connectHost(id),
    onNavigate: () => sidebar.setDrawerOpen(false),
  };
  const props = {
    hosts,
    selected: hostId !== undefined && agentName ? { hostId, agent: agentName } : undefined,
    pageHostId,
    counts,
    query,
    onQuery,
    daemonViews: daemons,
    appVersion,
    actions,
    width: sidebar.width,
    onResize: sidebar.setWidth,
  };

  if (!sidebar.compact) return sidebar.hidden ? null : <AgentSidebar {...props} />;
  return (
    <Sheet open={sidebar.drawerOpen} onOpenChange={sidebar.setDrawerOpen}>
      <SheetContent
        side="left"
        showCloseButton={false}
        aria-describedby={undefined}
        // Focus in the search field would raise the on-screen keyboard.
        onOpenAutoFocus={(e) => e.preventDefault()}
        className="w-[85vw] max-w-85 gap-0 bg-background px-1.5 pt-3 pb-2"
      >
        <SheetTitle className="sr-only">Navigation</SheetTitle>
        <AgentSidebar {...props} drawer />
      </SheetContent>
    </Sheet>
  );
}
