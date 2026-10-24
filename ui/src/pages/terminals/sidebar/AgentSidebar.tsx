import { useCallback, useRef, useState } from "react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import type { HostAgents } from "@/lib/aggregate";
import type { DaemonMeta } from "@/lib/daemons";
import {
  reorderFromDrag,
  rowCollision,
  sidebarAnnouncements,
  type ReorderKind,
} from "./sidebarDnd";
import { AgentsTab } from "./AgentsTab";
import { ServersTab } from "./ServersTab";
import { SidebarFooter } from "./SidebarFooter";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { SidebarSearch } from "./SidebarSearch";
import { SidebarTabs } from "./SidebarTabs";
import type { AgentRowActions } from "./SidebarAgentRow";
import { allAgents, filterHosts, rankAgents } from "./sidebarModel";
import {
  readPinnedAgents,
  readSidebarTab,
  writePinnedAgents,
  writeSidebarTab,
  type SidebarTab,
} from "./sidebarPrefs";

/**
 * The left column of the operator console: search, the two tabs, and the
 * foot. It is chrome, not a panel — it sits straight on `--background` with no
 * border and no surface of its own, and only the list between the tabs and the
 * foot scrolls.
 */
export function AgentSidebar({
  hosts,
  selectedHostId,
  selected,
  onSelectHost,
  onSelect,
  onSelectTeam,
  onReorder,
  onClone,
  onCreate,
  onAddServer,
  onEditServer,
  onUpdateAll,
  onRemoveServer,
  daemonViews,
  appVersion,
  onConnectHost,
  attention,
  width,
  onResize,
  drawer = false,
}: {
  hosts: HostAgents[];
  selectedHostId?: string;
  selected?: { hostId: string; agent: string };
  onSelectHost: (hostId: string) => void;
  onSelect: (hostId: string, agent: string) => void;
  onSelectTeam: (hostId: string, team: string) => void;
  onReorder: (kind: ReorderKind, hostId: string, ids: string[]) => void;
  onClone: (hostId: string, agentName: string) => void;
  onCreate: (hostId: string) => void;
  onAddServer: () => void;
  onEditServer: (hostId: string) => void;
  onUpdateAll: (hostIds: string[]) => void;
  onRemoveServer: (hostId: string) => void;
  daemonViews: DaemonMeta[];
  appVersion: string;
  onConnectHost: (hostId: string) => void;
  attention: ReadonlyMap<string, number>;
  width: number;
  onResize: (px: number) => void;
  /** Inside the compact drawer: fills it and has no resize handle. */
  drawer?: boolean;
}) {
  const asideRef = useRef<HTMLElement | null>(null);
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<SidebarTab>(readSidebarTab);
  const [pinned, setPinned] = useState<ReadonlySet<string>>(readPinnedAgents);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor),
  );

  const pickTab = (next: string) => {
    setTab(next as SidebarTab);
    writeSidebarTab(next as SidebarTab);
  };

  const togglePin = useCallback((key: string) => {
    setPinned((current) => {
      const next = new Set(current);
      if (!next.delete(key)) next.add(key);
      writePinnedAgents(next);
      return next;
    });
  }, []);

  const finishReorder = ({ active, over }: DragEndEvent) => {
    const order = reorderFromDrag(hosts, active.id, over?.id);
    if (order) onReorder(order.kind, order.hostId, order.ids);
  };

  // The reference opens a Command palette from the search box; this app has no
  // palette yet, and a dead control is worse than a live one — so it filters
  // the list the sidebar already has, by agent name or server label.
  const filtering = query.trim().length > 0;
  const visibleHosts = filterHosts(hosts, query);
  const agents = allAgents(visibleHosts);
  const ranked = rankAgents(agents, pinned, attention);
  const connectedCount = hosts.filter((host) => !host.error).length;
  const agentCount = hosts.reduce((total, host) => total + host.agents.length, 0);
  const rowActions: AgentRowActions = {
    selected,
    pinned,
    onSelect,
    onClone,
    onTogglePin: togglePin,
  };

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={rowCollision}
      accessibility={{ announcements: sidebarAnnouncements }}
      onDragEnd={finishReorder}
    >
      {/* The sidebar is chrome, not a panel: it sits straight on --background
        with no border and no surface of its own — the content island is the
        only raised thing on the screen. */}
      <aside
        ref={asideRef}
        aria-label="Agents"
        data-testid="agents-sidebar"
        style={drawer ? undefined : { width }}
        className={
          drawer ? "flex min-h-0 min-w-0 flex-1 flex-col" : "flex min-h-0 shrink-0 flex-col px-0.5"
        }
      >
        <div className="shrink-0 px-1.5 pt-0.5 pb-2">
          <SidebarSearch query={query} onQuery={setQuery} />
        </div>
        <Tabs value={tab} onValueChange={pickTab} className="flex min-h-0 flex-1 flex-col gap-0">
          <SidebarTabs onCreate={() => onCreate(selectedHostId ?? "")} />
          <div className="min-h-0 flex-1 overflow-y-auto px-1.5">
            <TabsContent value="agents">
              <AgentsTab pinned={ranked.pinned} rest={ranked.rest} actions={rowActions} />
            </TabsContent>
            <TabsContent value="servers">
              <ServersTab
                hosts={visibleHosts}
                filtering={filtering}
                daemonViews={daemonViews}
                appVersion={appVersion}
                actions={rowActions}
                servers={{
                  selectedHostId,
                  onSelectHost,
                  onSelectTeam,
                  onCreate,
                  onEditServer,
                  onUpdateAll,
                  onRemoveServer,
                  onConnectHost,
                }}
              />
            </TabsContent>
          </div>
        </Tabs>
        <SidebarFooter
          servers={hosts.length}
          connected={connectedCount}
          agents={agentCount}
          onAddServer={onAddServer}
        />
      </aside>
      {!drawer && <SidebarResizeHandle asideRef={asideRef} width={width} onResize={onResize} />}
    </DndContext>
  );
}
