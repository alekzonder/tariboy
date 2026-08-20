import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { usePolling } from "@/hooks/usePolling";
import { fetchAllAgents, type HostAgents } from "@/lib/aggregate";
import { paramToHost, hostToParam, serverPath, targetFor } from "@/lib/terminalsHost";
import { TerminalsSidebar } from "./TerminalsSidebar";
import { useSharedSidebarState } from "./sidebarStateContext";
import { CreateAgentDialog } from "./CreateAgentDialog";
import { ServerDialog } from "./ServerDialog";
import { useDaemons } from "@/components/DaemonProvider";
import { listDaemons, removeDaemon, type DaemonMeta } from "@/lib/daemons";
import { Button } from "@/components/ui/button";
import { hostConnect } from "@/lib/desktop";
import AgentWorkspace from "@/pages/agents/AgentWorkspace";
import {
  TerminalWorkspace,
  type TerminalWorkspaceHandle,
} from "./TerminalWorkspace";
import type { TerminalIdentity } from "./workspaceState";
import { ServerContextBar } from "./ServerContextBar";
import { RouteHostBoundary } from "./RouteHostBoundary";
import TasksWorkspace from "@/pages/tasks/TasksWorkspace";
import ImagesPage from "@/pages/ImagesPage";
import { ImageLayout } from "@/components/ImageLayout";
import SettingsPage from "@/pages/settings/SettingsPage";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";

type ServerDialogState = { mode: "add" } | { mode: "edit"; server: DaemonMeta };
type CreateDialogState = { hostId: string; imageRef?: string };
export type ServerView = "tasks" | "images" | "image-detail" | "settings";

export default function TerminalsPage({ serverView }: { serverView?: ServerView }) {
  const { hostId: hostParam, agent: agentName, team: teamName } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [lastSuccessfulHostsById, setLastSuccessfulHostsById] = useState<Map<string, HostAgents>>(
    () => new Map(),
  );
  const fetchHosts = useCallback(async () => {
    const data = await fetchAllAgents();
    setLastSuccessfulHostsById((previous) => {
      const next = new Map(previous);
      for (const entry of data) {
        if (!entry.error) next.set(entry.host.id, entry);
      }
      return next;
    });
    return data;
  }, []);
  const { data, refresh } = usePolling(fetchHosts, 3000);
  const hosts = useMemo(() => data ?? [], [data]);
  const hostId = hostParam !== undefined ? paramToHost(hostParam) : undefined;
  const [createFor, setCreateFor] = useState<CreateDialogState | null>(() => {
    const imageRef = searchParams.get("image");
    if (!imageRef && searchParams.get("new") !== "1") return null;
    return {
      hostId: searchParams.get("host") ?? "",
      imageRef: imageRef ?? undefined,
    };
  });
  const [serverDialog, setServerDialog] = useState<ServerDialogState | null>(null);
  // DaemonProvider caches the registry in React state and is the only thing
  // that pushes the active daemon into the api client, so registry edits made
  // here have to notify it too — otherwise the rest of the SPA keeps using a
  // stale (or removed) host as its target.
  const { daemons, appVersion, select: selectDaemon, refresh: refreshDaemons } = useDaemons();
  const { attention, refreshHost } = useCustomerQuestionNotifications();
  const sidebar = useSharedSidebarState();
  const workspaceMode = location.pathname === "/workspace";
  const workspaceRef = useRef<TerminalWorkspaceHandle | null>(null);
  const pendingWorkspaceAgent = useRef<TerminalIdentity | null>(null);
  const [hostError, setHostError] = useState("");
  const createHosts = useMemo(() => {
    const aggregateById = new Map(hosts.map((entry) => [entry.host.id, entry]));
    const metadataById = new Map(daemons.map((entry) => [entry.id, entry]));
    const ids = new Set([
      "",
      ...hosts.map((entry) => entry.host.id),
      ...daemons.map((entry) => entry.id),
    ]);
    return [...ids].map((id) => {
      const aggregate = aggregateById.get(id);
      const metadata = metadataById.get(id);
      return {
        id,
        label:
          aggregate?.host.label
          ?? metadata?.label
          ?? (id === "" ? "This daemon (local)" : id),
        revision: [
          aggregate ? aggregate.error ?? "reachable" : "pending",
          metadata?.state ?? "",
          metadata?.baseURL ?? "",
        ].join("\u0000"),
      };
    });
  }, [daemons, hosts]);

  // Re-read after any registry mutation: refresh() runs one immediate
  // fetchAllAgents() tick, which resolves every daemon straight from storage,
  // so labels/reachability update without waiting up to 3s for the next poll.
  const registryChanged = async () => {
    refresh();
    await refreshDaemons();
  };

  const connectHost = async (id: string) => {
    setHostError("");
    try {
      await hostConnect(id);
      await refreshDaemons();
    } catch (cause) {
      const label = daemons.find((host) => host.id === id)?.label ?? id;
      setHostError(`Could not connect to ${label}: ${String(cause)}`);
    }
  };

  const editServer = async (id: string) => {
    const server = (await listDaemons()).find((m) => m.id === id);
    if (server) setServerDialog({ mode: "edit", server });
  };

  const removeServer = async (id: string) => {
    const server = (await listDaemons()).find((m) => m.id === id);
    if (!window.confirm(
      `Remove host ${server?.label ?? id} from this app? The remote daemon and data remain on the server.`,
    )) return;
    await removeDaemon(id);
    // Leave a route whose host no longer exists. targetFor() also fails closed,
    // but navigating away gives the user a useful screen instead of a dead pane.
    if (hostId === id) navigate("/");
    await registryChanged();
  };

  const liveSelectedHost = useMemo(() => {
    if (hostId === undefined) return undefined;
    return hosts.find((h) => h.host.id === hostId);
  }, [hosts, hostId]);
  const selectedHost = useMemo(() => {
    if (hostId === undefined) return undefined;
    if (liveSelectedHost && !liveSelectedHost.error) return liveSelectedHost;
    return lastSuccessfulHostsById.get(hostId) ?? liveSelectedHost;
  }, [hostId, lastSuccessfulHostsById, liveSelectedHost]);
  const sidebarHosts = useMemo(() => hosts.map((entry) => {
    if (!entry.error) return entry;
    const snapshot = lastSuccessfulHostsById.get(entry.host.id);
    return snapshot ? { ...snapshot, error: entry.error } : entry;
  }), [hosts, lastSuccessfulHostsById]);
  const selectedAgent = useMemo(() => {
    if (!agentName) return undefined;
    return selectedHost?.agents.find((a) => a.name === agentName);
  }, [selectedHost, agentName]);
  const serverBasePath = hostId === undefined
    ? ""
    : `/servers/${encodeURIComponent(hostToParam(hostId))}`;
  const selectedHostLabel = selectedHost?.host.label
    ?? daemons.find((daemon) => daemon.id === hostId)?.label
    ?? (hostId === "" ? "This daemon (local)" : hostId);
  const routeUnavailable = Boolean(liveSelectedHost?.error);

  const addToWorkspace = (identity: TerminalIdentity) => {
    if (workspaceMode && workspaceRef.current) {
      workspaceRef.current.addOrFocus(identity);
      return;
    }
    pendingWorkspaceAgent.current = identity;
    navigate("/workspace");
  };

  const openConfiguration = (identity: TerminalIdentity) => {
    navigate(
      `/agents/${hostToParam(identity.hostId)}/${encodeURIComponent(identity.agentName)}/configuration`,
    );
  };

  useEffect(() => {
    if (!workspaceMode || !pendingWorkspaceAgent.current) return;
    const identity = pendingWorkspaceAgent.current;
    pendingWorkspaceAgent.current = null;
    workspaceRef.current?.addOrFocus(identity);
  }, [workspaceMode]);

  return (
    <div className="flex h-full">
      {!sidebar.hidden && <TerminalsSidebar
        hosts={sidebarHosts}
        selectedHostId={hostId}
        selected={hostId !== undefined && agentName ? { hostId, agent: agentName } : undefined}
        onSelectHost={(id) => navigate(serverPath(id, "tasks"))}
        onSelect={(h, a) => {
          const identity = { hostId: h, agentName: a };
          const selected = hosts
            .find((entry) => entry.host.id === h)
            ?.agents.find((agent) => agent.name === a);
          if (workspaceMode && selected?.interactive === false) {
            openConfiguration(identity);
          } else if (workspaceMode) {
            addToWorkspace(identity);
          } else {
            navigate(`/agents/${hostToParam(h)}/${encodeURIComponent(a)}/console`);
          }
        }}
        onSelectTeam={(h, team) => {
          navigate(`/agents/${hostToParam(h)}/teams/${encodeURIComponent(team)}`);
        }}
        workspaceMode={workspaceMode}
        onBeginWorkspaceDrag={(identity, event) => {
          workspaceRef.current?.beginExternalPointerDrag(identity, event.nativeEvent);
        }}
        onCreate={(hostId) => setCreateFor({ hostId })}
        onAddServer={() => setServerDialog({ mode: "add" })}
        onEditServer={(id) => void editServer(id)}
        onRemoveServer={(id) => void removeServer(id)}
        daemonViews={daemons}
        appVersion={appVersion}
        onConnectHost={(id) => void connectHost(id)}
        attention={attention}
        width={sidebar.width}
        onResize={sidebar.setWidth}
      />}
      <main className="flex min-h-0 min-w-0 flex-1 flex-col">
        {!workspaceMode && hostId !== undefined && (
          <ServerContextBar hostId={hostId} label={selectedHostLabel ?? "Unknown server"} />
        )}
        <div className={`flex min-h-0 flex-1 flex-col${serverView ? "" : " p-3"}`}>
        {hostError && (
          <p role="alert" className="mb-2 text-sm text-destructive">{hostError}</p>
        )}
        <section className="min-h-0 flex-1">
        {workspaceMode ? (
          <TerminalWorkspace
            ref={workspaceRef}
            hosts={hosts}
            refresh={refresh}
            onOpenConfiguration={openConfiguration}
          />
        ) : serverView === "tasks" && hostId !== undefined ? (
          <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
            <TasksWorkspace
              target={targetFor(hostId)}
              initialTaskKey={searchParams.get("task") ?? undefined}
              onNotificationsChanged={() => void refreshHost(hostId)}
            />
          </RouteHostBoundary>
        ) : serverView === "images" && hostId !== undefined ? (
          <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
            <ImagesPage hostId={hostId} basePath={`${serverBasePath}/images`} />
          </RouteHostBoundary>
        ) : serverView === "image-detail" && hostId !== undefined ? (
          <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
            <ImageLayout hostId={hostId} basePath={`${serverBasePath}/images`} />
          </RouteHostBoundary>
        ) : serverView === "settings" && hostId !== undefined ? (
          <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
            <SettingsPage basePath={`${serverBasePath}/settings`} />
          </RouteHostBoundary>
        ) : teamName && selectedHost && hostId !== undefined ? (
          <RouteHostBoundary hostId={hostId} unavailable={routeUnavailable}>
            <TeamWorkspace
              name={teamName}
              hostLabel={selectedHost.host.label}
              members={selectedHost.agents.filter((agent) => agent.group === teamName)}
              onOpen={(agent) => navigate(`/agents/${hostToParam(hostId)}/${encodeURIComponent(agent)}/console`)}
              onManage={() => void selectDaemon(hostId).then((selected) => { if (selected) navigate(`/settings/advanced/groups?team=${encodeURIComponent(teamName)}`); })}
            />
          </RouteHostBoundary>
        ) : selectedAgent && hostId !== undefined ? (
          <AgentWorkspace
            key={`${hostId}\u0000${selectedAgent.name}`}
            hostId={hostId}
            hostLabel={selectedHost?.host.label}
            agent={selectedAgent}
            refresh={refresh}
            unavailable={routeUnavailable}
          />
        ) : hostId !== undefined && agentName ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
            <p>Host or agent unavailable.</p>
            <p className="text-xs">The route was kept unchanged so it cannot silently target the local daemon.</p>
          </div>
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
            <p>Pick an agent on the left, or create one.</p>
            <Button onClick={() => setCreateFor({ hostId: "" })}>New agent</Button>
          </div>
        )}
        </section>
        </div>
      </main>
      <CreateAgentDialog
        open={createFor !== null}
        hostId={createFor?.hostId ?? ""}
        imageRef={createFor?.imageRef}
        hosts={createHosts}
        onOpenChange={(o) => { if (!o) setCreateFor(null); }}
        onCreated={(h, name) => {
          // usePolling exposes `refresh` (its tick fn) — call it so the new
          // agent shows up immediately instead of waiting up to 3s for the
          // next poll tick.
          refresh();
          navigate(`/agents/${hostToParam(h)}/${encodeURIComponent(name)}/console`);
        }}
      />
      <ServerDialog
        open={serverDialog !== null}
        server={serverDialog?.mode === "edit" ? serverDialog.server : undefined}
        onOpenChange={(o) => { if (!o) setServerDialog(null); }}
        onSaved={registryChanged}
      />
    </div>
  );
}

function TeamWorkspace({ name, hostLabel, members, onOpen, onManage }: {
  name: string;
  hostLabel: string;
  members: Array<{ name: string; image: string; state: string; interactive?: boolean }>;
  onOpen: (agent: string) => void;
  onManage: () => void;
}) {
  return <div className="space-y-4 p-3">
    <div className="flex items-center justify-between">
      <div><h1 className="text-xl font-semibold">{name}</h1><p className="text-sm text-muted-foreground">Team on {hostLabel}</p></div>
      <Button variant="outline" onClick={onManage}>Manage team</Button>
    </div>
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {members.map((member) => <button key={member.name} type="button" aria-label={`Open team member ${member.name}`}
        onClick={() => onOpen(member.name)} className="rounded-lg border p-3 text-left hover:bg-accent">
        <div className="font-medium">{member.name}</div>
        <div className="text-xs text-muted-foreground">{member.image} · {member.state}{member.interactive === false ? " · non-tty" : ""}</div>
      </button>)}
    </div>
    {members.length === 0 && <p className="text-sm text-muted-foreground">No members are currently assigned.</p>}
  </div>;
}
