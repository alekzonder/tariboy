import { lazy, type ReactNode } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";
import type { HostAgents } from "@/lib/aggregate";
import { navPath } from "@/lib/navRoutes";
import { hostToParam } from "@/lib/terminalsHost";
import { defaultAgentTab } from "@/lib/uiMode";
import { useWorkspaces } from "@/lib/workspaces";
import AgentWorkspace from "@/pages/agents/AgentWorkspace";
import type { HostsOutletContext } from "@/pages/settings/hostsContext";
import { RouteHostBoundary } from "./RouteHostBoundary";
import type { ServerView } from "./TerminalsPage";
import { LibraryHeader } from "./LibraryHeader";
import { agentPath, type CreateHost } from "./terminalsHosts";

// Pages load on first open, so the initial bundle carries only the shell, the
// agent list and the agent workspace.
const InboxPage = lazy(() => import("@/pages/mailbox/InboxPage"));
const TasksWorkspace = lazy(() => import("@/pages/tasks/TasksWorkspace"));
const ServerActivityPage = lazy(() => import("@/pages/activity/ServerActivityPage"));
const LibraryImagesPage = lazy(() => import("@/pages/images/LibraryImagesPage"));
const StoresPage = lazy(() => import("@/pages/StoresPage"));
const WorkflowImagesPage = lazy(() => import("@/pages/workflows/WorkflowImagesPage"));
const WorkflowImageDetail = lazy(() => import("@/pages/workflows/WorkflowImageDetail"));
const ImageLayout = lazy(() =>
  import("@/components/ImageLayout").then((m) => ({ default: m.ImageLayout })),
);
const SettingsPage = lazy(() => import("@/pages/settings/SettingsPage"));
const HostScopeSelect = lazy(() =>
  import("./HostScopeSelect").then((m) => ({ default: m.HostScopeSelect })),
);
const HomePage = lazy(() => import("@/pages/home/HomePage"));
const FirstRunHome = lazy(() =>
  import("./FirstRunHome").then((m) => ({ default: m.FirstRunHome })),
);

const LIBRARY_WORKFLOWS = navPath("library-workflows", "");

interface ServerRoute {
  hostId: string;
  storeName?: string;
  routeTag?: string;
  initialTaskKey?: string;
  multiHost: boolean;
  /** The host selector; null with one host. */
  hostControl: ReactNode;
  onNotificationsChanged: () => void;
  /** Connects a page body to the route host. */
  boundary: (page: ReactNode) => ReactNode;
  hostsContext: HostsOutletContext;
}

/** Each page, or null when its route lacks what the page needs. */
const SERVER_VIEWS: Record<ServerView, (route: ServerRoute) => ReactNode> = {
  mail: (route) => (
    <InboxPage key={route.hostId} multiHost={route.multiHost} hostControl={route.hostControl} />
  ),
  tasks: (route) => (
    <TasksWorkspace
      initialTaskKey={route.initialTaskKey}
      hostControl={route.hostControl}
      onNotificationsChanged={route.onNotificationsChanged}
    />
  ),
  activity: (route) => (
    <ServerActivityPage key={route.hostId} hostId={route.hostId} hostControl={route.hostControl} />
  ),
  images: (route) => library(route, <LibraryImagesPage key={route.hostId} hostId={route.hostId} />),
  "image-detail": (route) => library(route, <ImageLayout hostId={route.hostId} />),
  workflows: (route) =>
    library(route, <WorkflowImagesPage key={route.hostId} basePath={LIBRARY_WORKFLOWS} />),
  "workflow-detail": (route) => {
    const { hostId, storeName, routeTag } = route;
    return storeName && routeTag
      ? library(
          route,
          <WorkflowImageDetail
            key={`${hostId}\u0000${storeName}\u0000${routeTag}`}
            name={storeName}
            tag={routeTag}
            basePath={LIBRARY_WORKFLOWS}
          />,
        )
      : null;
  },
  stores: (route) => library(route, storesPage(route)),
  "store-detail": (route) => library(route, storesPage(route)),
  settings: (route) => (
    <SettingsPage
      hostControl={route.hostControl}
      boundary={route.boundary}
      hostsContext={route.hostsContext}
    />
  ),
};

/** Settings → Teams opened on one team, naming its host when there are several. */
function teamSettingsPath(team: string, hostId?: string): string {
  const params = new URLSearchParams();
  if (hostId !== undefined) params.set("host", hostToParam(hostId));
  params.set("team", team);
  return `/settings/teams?${params.toString()}`;
}

/** A Library page under its Images · Workflows · Sources tabs. */
function library(route: ServerRoute, page: ReactNode) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <LibraryHeader hostId={route.hostId} hostControl={route.hostControl} />
      <div className="min-h-0 flex-1">{page}</div>
    </div>
  );
}

function storesPage({ hostId, storeName }: ServerRoute) {
  return (
    <StoresPage
      key={`${hostId}\u0000${storeName ?? ""}`}
      name={storeName}
      basePath={navPath("library-sources", hostId)}
    />
  );
}

/**
 * The content island's page for the current route: a page of the sidebar, a
 * team, or an agent. Every host page and the team page sit in the same
 * RouteHostBoundary slot, so moving between them keeps the boundary mounted.
 */
export function TerminalsRoutes({
  serverView,
  workspaceHosts,
  hostId,
  selectedHost,
  routeUnavailable,
  refresh,
  hostsContext,
  fallback,
}: {
  serverView?: ServerView;
  /** The current workspace's hosts, for the host selector. */
  workspaceHosts: HostAgents[];
  hostId?: string;
  selectedHost?: HostAgents;
  routeUnavailable: boolean;
  refresh: () => Promise<void>;
  /** The host workspaces, for Settings → Hosts. */
  hostsContext: HostsOutletContext;
  /** What shows when the route names no page this component can open. */
  fallback: ReactNode;
}) {
  const { agent: agentName, team: teamName, name: storeName, tag: routeTag } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { refreshHost } = useCustomerQuestionNotifications();

  if (hostId === undefined) return fallback;
  const multiHost = workspaceHosts.length >= 2;
  const page = serverView
    ? SERVER_VIEWS[serverView]({
        hostId,
        storeName,
        routeTag,
        initialTaskKey: searchParams.get("task") ?? undefined,
        multiHost,
        hostControl: multiHost ? <HostScopeSelect hosts={workspaceHosts} hostId={hostId} /> : null,
        onNotificationsChanged: () => void refreshHost(hostId),
        boundary: (body) => (
          <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
            {body}
          </RouteHostBoundary>
        ),
        hostsContext,
      })
    : null;
  // Settings puts its host pages in the boundary itself: Hosts opens without one.
  if (page && serverView === "settings") return page;
  if (page) {
    return (
      <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
        {page}
      </RouteHostBoundary>
    );
  }
  if (teamName && selectedHost) {
    return (
      <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
        <TeamWorkspace
          name={teamName}
          hostLabel={selectedHost.host.label}
          members={selectedHost.agents.filter((agent) => agent.group === teamName)}
          onOpen={(agent) => navigate(agentPath(hostId, agent, defaultAgentTab()))}
          onManage={() => navigate(teamSettingsPath(teamName, multiHost ? hostId : undefined))}
        />
      </RouteHostBoundary>
    );
  }
  const selectedAgent = agentName
    ? selectedHost?.agents.find((a) => a.name === agentName)
    : undefined;
  if (!selectedAgent) return fallback;
  return (
    <AgentWorkspace
      key={`${hostId}\u0000${selectedAgent.name}`}
      hostId={hostId}
      hostLabel={selectedHost?.host.label}
      agent={selectedAgent}
      refresh={refresh}
      unavailable={routeUnavailable}
    />
  );
}

/** The island when no page is open: a dead agent route, an empty workspace, no
 *  hosts at all, the first-run card, or Home. */
export function TerminalsEmptyState({
  hosts,
  pageHostId,
  firstHostId,
  createHosts,
  onCreated,
  onManage,
  onAddServer,
  onNewAgent,
}: {
  /** The current workspace's hosts. */
  hosts: HostAgents[];
  /** The host Home's numbers are read from. */
  pageHostId: string;
  /** The first host an agent can be created on, if any. */
  firstHostId?: string;
  /** Every host an agent can be created on, for the first-run card. */
  createHosts: CreateHost[];
  onCreated: (hostId: string, name: string) => void;
  onManage: () => void;
  onAddServer: () => void;
  onNewAgent: (hostId: string) => void;
}) {
  const { hostId: hostParam, agent: agentName } = useParams();
  const workspaces = useWorkspaces();
  if (hostParam !== undefined && agentName) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
        <p>Host or agent unavailable.</p>
        <p className="text-xs">
          The route was kept unchanged so it cannot silently target the local daemon.
        </p>
      </div>
    );
  }
  const workspaceEmpty = hosts.every((entry) => entry.agents.length === 0);
  if (workspaceEmpty && workspaces.workspaces.length > 1) {
    const name = workspaces.workspaces.find((w) => w.id === workspaces.active)?.name ?? "";
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
        <p>{name} has no agents yet</p>
        <Button onClick={onManage}>Manage workspaces</Button>
      </div>
    );
  }
  if (firstHostId === undefined) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
        <p>Add a host to start.</p>
        <Button onClick={onAddServer}>Add host</Button>
      </div>
    );
  }
  if (workspaceEmpty) {
    return <FirstRunHome hostId={firstHostId} hosts={createHosts} onCreated={onCreated} />;
  }
  return (
    <HomePage hosts={hosts} pageHostId={pageHostId} onNewAgent={() => onNewAgent(firstHostId)} />
  );
}

function TeamWorkspace({
  name,
  hostLabel,
  members,
  onOpen,
  onManage,
}: {
  name: string;
  hostLabel: string;
  members: Array<{ name: string; image: string; state: string; interactive?: boolean }>;
  onOpen: (agent: string) => void;
  onManage: () => void;
}) {
  return (
    <div className="space-y-4 p-3">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">{name}</h1>
          <p className="text-sm text-muted-foreground">Team on {hostLabel}</p>
        </div>
        <Button variant="outline" onClick={onManage}>
          Manage team
        </Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {members.map((member) => (
          <button
            key={member.name}
            type="button"
            aria-label={`Open team member ${member.name}`}
            onClick={() => onOpen(member.name)}
            className="rounded-lg border p-3 text-left hover:bg-accent"
          >
            <div className="font-medium">{member.name}</div>
            <div className="text-xs text-muted-foreground">
              {member.image} · {member.state}
              {member.interactive === false ? " · non-tty" : ""}
            </div>
          </button>
        ))}
      </div>
      {members.length === 0 && (
        <p className="text-sm text-muted-foreground">No members are currently assigned.</p>
      )}
    </div>
  );
}
