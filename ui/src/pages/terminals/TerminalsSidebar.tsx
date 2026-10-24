import type { ComponentProps } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { useDaemons } from "@/components/daemonContext";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";
import type { HostAgents } from "@/lib/aggregate";
import { hostToParam, paramToHost, serverPath } from "@/lib/terminalsHost";
import { defaultAgentTab } from "@/lib/uiMode";
import type { CloneAgentSource } from "./CreateAgentDialog";
import { AgentSidebar } from "./sidebar/AgentSidebar";
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
 * The agent list beside the content island: a column on a wide screen, a
 * drawer on a compact one. A pick in the drawer leaves it, because navigation
 * or a dialog takes over the screen.
 */
export function TerminalsSidebar({
  hosts,
  labelHosts,
  server,
  onReorder,
  onCreate,
  onAddServer,
}: {
  /** The hosts of the current workspace, in sidebar order. */
  hosts: HostAgents[];
  /** Every host, to name a clone source. */
  labelHosts: HostAgents[];
  server: ReturnType<typeof useServerActions>;
  onReorder: (kind: ReorderKind, id: string, ids: string[]) => void;
  onCreate: (state: CreateDialogState) => void;
  onAddServer: () => void;
}) {
  const { hostId: hostParam, agent: agentName, tab: agentTab } = useParams();
  const hostId = hostParam !== undefined ? paramToHost(hostParam) : undefined;
  const navigate = useNavigate();
  const { daemons, appVersion } = useDaemons();
  const { attention } = useCustomerQuestionNotifications();
  const sidebar = useSharedSidebarState();

  const closing =
    <A extends unknown[]>(act: (...args: A) => void) =>
    (...args: A) => {
      sidebar.setDrawerOpen(false);
      act(...args);
    };
  const sidebarProps: ComponentProps<typeof AgentSidebar> = {
    hosts,
    selectedHostId: hostId,
    selected: hostId !== undefined && agentName ? { hostId, agent: agentName } : undefined,
    onSelectHost: closing((id: string) => navigate(serverPath(id, "tasks"))),
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
    onAddServer: closing(onAddServer),
    onEditServer: closing((id: string) => void server.editServer(id)),
    onUpdateAll: (ids) => void server.updateAllServers(ids),
    onRemoveServer: (id) => void server.removeServer(id),
    daemonViews: daemons,
    appVersion,
    onConnectHost: (id) => void server.connectHost(id),
    attention,
    width: sidebar.width,
    onResize: sidebar.setWidth,
  };

  if (!sidebar.compact) return sidebar.hidden ? null : <AgentSidebar {...sidebarProps} />;
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
        <SheetTitle className="sr-only">Agents</SheetTitle>
        <AgentSidebar {...sidebarProps} drawer />
      </SheetContent>
    </Sheet>
  );
}
