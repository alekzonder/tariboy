import { lazy, type ReactNode } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useDaemons } from "@/components/daemonContext";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";
import type { HostAgents } from "@/lib/aggregate";
import { hostToParam } from "@/lib/terminalsHost";
import { defaultAgentTab } from "@/lib/uiMode";
import { inWorkspace, useWorkspaces } from "@/lib/workspaces";
import AgentWorkspace from "@/pages/agents/AgentWorkspace";
import { RouteHostBoundary } from "./RouteHostBoundary";
import type { ServerView } from "./TerminalsPage";
import { agentPath } from "./terminalsHosts";

// Server pages load on first open, so the initial bundle carries only the
// shell, the agent list and the agent workspace.
const ServerMailboxTab = lazy(() => import("@/pages/mailbox/ServerMailboxTab"));
const TasksWorkspace = lazy(() => import("@/pages/tasks/TasksWorkspace"));
const AllTasksWorkspace = lazy(() => import("@/pages/tasks/AllTasksWorkspace"));
const ServerActivityPage = lazy(() => import("@/pages/activity/ServerActivityPage"));
const ImagesPage = lazy(() => import("@/pages/ImagesPage"));
const ImageTags = lazy(() => import("@/pages/images/ImageTags"));
const StoresPage = lazy(() => import("@/pages/StoresPage"));
const WorkflowImagesPage = lazy(() => import("@/pages/workflows/WorkflowImagesPage"));
const WorkflowImageDetail = lazy(() => import("@/pages/workflows/WorkflowImageDetail"));
const ImageLayout = lazy(() =>
  import("@/components/ImageLayout").then((m) => ({ default: m.ImageLayout })),
);
const SettingsPage = lazy(() => import("@/pages/settings/SettingsPage"));

interface ServerRoute {
  hostId: string;
  /** `/servers/<host>`, the root of every server page's own links. */
  basePath: string;
  storeName?: string;
  routeTag?: string;
  initialTaskKey?: string;
  onNotificationsChanged: () => void;
}

/** Each server page, or null when its route lacks what the page needs. */
const SERVER_VIEWS: Record<ServerView, (route: ServerRoute) => ReactNode> = {
  mail: (route) => <ServerMailboxTab key={route.hostId} />,
  tasks: (route) => (
    <TasksWorkspace
      initialTaskKey={route.initialTaskKey}
      onNotificationsChanged={route.onNotificationsChanged}
    />
  ),
  activity: (route) => <ServerActivityPage key={route.hostId} hostId={route.hostId} />,
  images: (route) => <ImagesPage hostId={route.hostId} basePath={`${route.basePath}/images`} />,
  "image-tags": (route) => (
    <ImageTags hostId={route.hostId} basePath={`${route.basePath}/images`} />
  ),
  "image-detail": (route) => (
    <ImageLayout hostId={route.hostId} basePath={`${route.basePath}/images`} />
  ),
  workflows: (route) => (
    <WorkflowImagesPage key={route.hostId} basePath={`${route.basePath}/workflows`} />
  ),
  "workflow-detail": ({ hostId, storeName, routeTag, basePath }) =>
    storeName && routeTag ? (
      <WorkflowImageDetail
        key={`${hostId}\u0000${storeName}\u0000${routeTag}`}
        name={storeName}
        tag={routeTag}
        basePath={`${basePath}/workflows`}
      />
    ) : null,
  stores: (route) => storesPage(route),
  "store-detail": (route) => storesPage(route),
  settings: (route) => <SettingsPage basePath={`${route.basePath}/settings`} />,
};

function storesPage({ hostId, storeName, basePath }: ServerRoute) {
  return (
    <StoresPage
      key={`${hostId}\u0000${storeName ?? ""}`}
      name={storeName}
      basePath={`${basePath}/stores`}
    />
  );
}

/**
 * The content island's page for the current route: All tasks, a server page,
 * a team, or an agent. Every server page and the team page sit in the same
 * RouteHostBoundary slot, so moving between them keeps the boundary mounted.
 */
export function TerminalsRoutes({
  serverView,
  hosts,
  hostId,
  selectedHost,
  routeUnavailable,
  refresh,
  fallback,
}: {
  serverView?: ServerView;
  hosts: HostAgents[];
  hostId?: string;
  selectedHost?: HostAgents;
  routeUnavailable: boolean;
  refresh: () => Promise<void>;
  /** What shows when the route names no page this component can open. */
  fallback: ReactNode;
}) {
  const { agent: agentName, team: teamName, name: storeName, tag: routeTag } = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { select: selectDaemon } = useDaemons();
  const { refreshHost } = useCustomerQuestionNotifications();
  const workspaces = useWorkspaces();

  if (searchParams.get("view") === "all") {
    // Tasks, like the sidebar, come only from the current workspace's servers.
    const workspaceHosts = hosts.filter((entry) =>
      inWorkspace(workspaces, entry.host.id, workspaces.active),
    );
    // Once the first poll found hosts, an empty list is an empty workspace,
    // not one still loading.
    return (
      <AllTasksWorkspace
        hosts={workspaceHosts}
        noServers={hosts.length > 0 && workspaceHosts.length === 0}
      />
    );
  }
  if (hostId === undefined) return fallback;
  const page = serverView
    ? SERVER_VIEWS[serverView]({
        hostId,
        basePath: `/servers/${encodeURIComponent(hostToParam(hostId))}`,
        storeName,
        routeTag,
        initialTaskKey: searchParams.get("task") ?? undefined,
        onNotificationsChanged: () => void refreshHost(hostId),
      })
    : null;
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
          onManage={() =>
            void selectDaemon(hostId).then((selected) => {
              if (selected)
                navigate(`/settings/advanced/groups?team=${encodeURIComponent(teamName)}`);
            })
          }
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
 *  servers at all, or the invitation to pick an agent. */
export function TerminalsEmptyState({
  hosts,
  firstHostId,
  onManage,
  onAddServer,
  onNewAgent,
}: {
  /** The current workspace's hosts. */
  hosts: HostAgents[];
  /** The first host an agent can be created on, if any. */
  firstHostId?: string;
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
        <p>Add a server to start.</p>
        <Button onClick={onAddServer}>Add server</Button>
      </div>
    );
  }
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
      <p>Pick an agent on the left, or create one.</p>
      <Button onClick={() => onNewAgent(firstHostId)}>New agent</Button>
    </div>
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
