import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { Home, Plus } from "lucide-react";
import type { HostAgents } from "@/lib/aggregate";
import { hostUpdateAvailable, type DaemonMeta } from "@/lib/daemons";
import { activeNav } from "@/lib/navRoutes";
import { TOUR_ANCHORS } from "@/lib/onboarding/tour";
import {
  reorderFromDrag,
  rowCollision,
  sidebarAnnouncements,
  type ReorderKind,
} from "./sidebarDnd";
import { AgentList, type HostActions } from "./AgentList";
import { SidebarFooter } from "./SidebarFooter";
import { AgentsHeading, SectionsNav, SidebarNavItem } from "./SidebarNav";
import { SidebarResizeHandle } from "./SidebarResizeHandle";
import { SidebarSearch } from "./SidebarSearch";
import type { AgentRowActions } from "./SidebarAgentRow";
import { filterHosts, hostsLine, sidebarLayout } from "./sidebarModel";
import { agentNeeds } from "@/lib/agentState";
import { useNeedsYou } from "../needsYouContext";
import {
  readAgentsOpen,
  togglePinnedAgent,
  usePinnedAgents,
  writeAgentsOpen,
} from "./sidebarPrefs";

const MINUTE_MS = 60_000;

export interface SidebarActions extends HostActions {
  onSelect: (hostId: string, agent: string) => void;
  onReorder: (kind: ReorderKind, hostId: string, ids: string[]) => void;
  onClone: (hostId: string, agentName: string) => void;
  onCreate: (hostId: string) => void;
  /** A pick that leaves the sidebar: the drawer closes on it. */
  onNavigate?: () => void;
}

/**
 * The left column of the console: Home, the Agents list, Tasks, Inbox,
 * Library, Settings, and the hosts line. It is chrome, not a panel — it sits
 * straight on `--background` with no border and no surface of its own, and
 * only the agent list scrolls.
 */
export function AgentSidebar({
  hosts,
  selected,
  pageHostId,
  counts,
  query,
  onQuery,
  daemonViews,
  appVersion,
  actions,
  width,
  onResize,
  drawer = false,
}: {
  /** The current workspace's hosts, in sidebar order. */
  hosts: HostAgents[];
  selected?: { hostId: string; agent: string };
  /** The host of the open page, for Library and Settings. */
  pageHostId: string;
  counts: { tasks: number | null; inbox: number | null };
  /** The titlebar search, which filters this list. */
  query: string;
  onQuery: (query: string) => void;
  daemonViews: DaemonMeta[];
  appVersion: string;
  actions: SidebarActions;
  width: number;
  onResize: (px: number) => void;
  /** Inside the compact drawer: fills it, has its own search field and no
   *  resize handle. */
  drawer?: boolean;
}) {
  const asideRef = useRef<HTMLElement | null>(null);
  const { pathname } = useLocation();
  const [agentsOpen, setAgentsOpen] = useState(readAgentsOpen);
  const pinned = usePinnedAgents();
  const { hosts: needsHosts } = useNeedsYou();
  // "working · 12 min" moves once a minute.
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), MINUTE_MS);
    return () => window.clearInterval(timer);
  }, []);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor),
  );

  const toggleAgents = () => {
    setAgentsOpen(!agentsOpen);
    writeAgentsOpen(!agentsOpen);
  };

  const finishReorder = ({ active, over }: DragEndEvent) => {
    const order = reorderFromDrag(hosts, active.id, over?.id);
    if (order) actions.onReorder(order.kind, order.hostId, order.ids);
  };

  const filtering = query.trim().length > 0;
  const layout = sidebarLayout(filterHosts(hosts, query), pinned);
  const behind = hosts
    .filter((host) => {
      const view = daemonViews.find((candidate) => candidate.id === host.host.id);
      return view ? hostUpdateAvailable(view, appVersion) : false;
    })
    .map((host) => host.host.id);
  const rowActions: AgentRowActions = {
    selected,
    pinned,
    onSelect: actions.onSelect,
    onClone: actions.onClone,
    onTogglePin: togglePinnedAgent,
    now,
    needsFor: (hostId, agent) =>
      agentNeeds(
        needsHosts.find((host) => host.hostId === hostId),
        agent,
      ),
  };

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={rowCollision}
      accessibility={{ announcements: sidebarAnnouncements }}
      onDragEnd={finishReorder}
    >
      <aside
        ref={asideRef}
        aria-label="Navigation"
        data-testid="agents-sidebar"
        style={drawer ? undefined : { width }}
        className={
          drawer ? "flex min-h-0 min-w-0 flex-1 flex-col" : "flex min-h-0 shrink-0 flex-col px-0.5"
        }
      >
        {drawer && (
          <div className="shrink-0 px-1.5 pt-0.5 pb-2">
            <SidebarSearch query={query} onQuery={onQuery} />
          </div>
        )}
        <nav aria-label="Main" className="flex shrink-0 flex-col gap-0.5 px-1.5">
          <SidebarNavItem
            to="/"
            label="Home"
            icon={Home}
            active={activeNav(pathname) === "home"}
            onNavigate={actions.onNavigate}
          />
          <AgentsHeading open={agentsOpen || filtering} onToggle={toggleAgents} />
        </nav>
        {(agentsOpen || filtering) && (
          <div className="min-h-0 shrink overflow-y-auto px-1.5" aria-label="Agents" role="group">
            <button
              type="button"
              data-tour={TOUR_ANCHORS.addAgent}
              onClick={() => actions.onCreate(actions.selectedHostId ?? "")}
              className="flex h-7 w-full items-center gap-2 rounded-control px-2 text-body-sm text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
            >
              <Plus className="size-3.25" aria-hidden="true" />
              New agent
            </button>
            <AgentList
              layout={layout}
              filtering={filtering}
              daemonViews={daemonViews}
              appVersion={appVersion}
              rowActions={rowActions}
              actions={actions}
            />
          </div>
        )}
        <SectionsNav pageHostId={pageHostId} counts={counts} onNavigate={actions.onNavigate} />
        <div className="min-h-0 flex-1" />
        <SidebarFooter
          line={hostsLine(hosts)}
          healthy={hosts.every((host) => !host.error)}
          behind={behind}
          onNavigate={actions.onNavigate}
        />
      </aside>
      {!drawer && <SidebarResizeHandle asideRef={asideRef} width={width} onResize={onResize} />}
    </DndContext>
  );
}
