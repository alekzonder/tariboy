import { useCallback, useRef, useState } from "react";
import {
  DndContext, KeyboardSensor, PointerSensor, useSensor, useSensors, type DragEndEvent,
} from "@dnd-kit/core";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import type { HostAgents } from "@/lib/aggregate";
import type { DaemonMeta } from "@/lib/daemons";
import {
  DEFAULT_SIDEBAR_WIDTH, MAX_SIDEBAR_WIDTH, MIN_SIDEBAR_WIDTH,
} from "../useSidebarWidth";
import { identityFor, rowCollision, sameScope, sidebarAnnouncements, type ReorderKind } from "./sidebarDnd";
import { AgentsTab } from "./AgentsTab";
import { GroupsTab } from "./GroupsTab";
import { ServersTab } from "./ServersTab";
import { SidebarFooter } from "./SidebarFooter";
import { SidebarSearch } from "./SidebarSearch";
import { SidebarTabs } from "./SidebarTabs";
import type { AgentRowActions } from "./SidebarAgentRow";
import {
  allAgents, filterHosts, groupSections, hostAgents, ordered, rankAgents,
} from "./sidebarModel";
import {
  readPinnedAgents, readSidebarTab, writePinnedAgents, writeSidebarTab, type SidebarTab,
} from "./sidebarPrefs";

function move(ids: string[], active: string, over: string): string[] {
  const from = ids.indexOf(active);
  const to = ids.indexOf(over);
  if (from < 0 || to < 0 || from === to) return ids;
  const next = [...ids];
  next.splice(to, 0, next.splice(from, 1)[0]);
  return next;
}

/**
 * The left column of the operator console: search, the three tabs, and the
 * foot. It is chrome, not a panel — it sits straight on `--background` with no
 * border and no surface of its own, and only the list between the tabs and the
 * foot scrolls.
 */
export function AgentSidebar({ hosts, selectedHostId, selected, onSelectHost, onSelect, onSelectTeam, onReorder, onClone, onCreate, onAddServer, onEditServer, onUpdateAll, onRemoveServer, daemonViews, appVersion, onConnectHost, attention, width, onResize }: {
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

  // Drag state lives on window listeners rather than pointer capture: capture
  // is spotty in jsdom, and window listeners keep tracking the drag when the
  // pointer leaves the 4px handle (which it always does).
  const startDrag = (e: React.PointerEvent) => {
    e.preventDefault();
    const left = asideRef.current?.getBoundingClientRect().left ?? 0;
    const onMove = (ev: PointerEvent) => onResize(ev.clientX - left);
    const onUp = () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    // Keep the resize cursor (and kill text selection) for the whole drag, not
    // just while the pointer is over the handle.
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 32 : 8;
    if (e.key === "ArrowLeft") { e.preventDefault(); onResize(width - step); }
    else if (e.key === "ArrowRight") { e.preventDefault(); onResize(width + step); }
    else if (e.key === "Home") { e.preventDefault(); onResize(DEFAULT_SIDEBAR_WIDTH); }
  };

  const finishReorder = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const source = identityFor(active.id);
    const target = identityFor(over.id);
    if (!sameScope(source, target)) return;
    if (source[0] === "servers") {
      onReorder("servers", "", move(hosts.map((host) => host.host.id), source[2], target[2]));
      return;
    }
    const host = hosts.find((entry) => entry.host.id === source[1]);
    if (!host) return;
    if (source[0] === "groups") {
      const names = [...new Set([
        ...(host.groups ?? []).map((group) => group.name),
        ...hostAgents(host).map((row) => row.group).filter(Boolean),
      ])];
      const current = ordered(names, host.sidebarOrder?.groups ?? [], (name) => name);
      onReorder("groups", source[1], move(current, source[2], target[2]));
      return;
    }
    const current = hostAgents(host).map((row) => row.agent.name);
    onReorder("agents", source[1], move(current, source[2], target[2]));
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
      style={{ width }}
      className="flex min-h-0 shrink-0 flex-col px-0.5"
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
          <TabsContent value="groups">
            <GroupsTab
              sections={groupSections(visibleHosts, agents)}
              actions={rowActions}
              onOpenGroup={onSelectTeam}
              onCreate={onCreate}
            />
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
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="resize sidebar"
      aria-valuenow={width}
      aria-valuemin={MIN_SIDEBAR_WIDTH}
      aria-valuemax={MAX_SIDEBAR_WIDTH}
      tabIndex={0}
      onPointerDown={startDrag}
      onKeyDown={onKeyDown}
      onDoubleClick={() => onResize(DEFAULT_SIDEBAR_WIDTH)}
      className="w-2 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-primary/30 focus-visible:bg-primary/30 focus-visible:outline-none"
    />
    </DndContext>
  );
}
