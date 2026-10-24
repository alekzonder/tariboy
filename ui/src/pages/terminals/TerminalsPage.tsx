import { useContext, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { paramToHost } from "@/lib/terminalsHost";
import { CreateAgentDialog } from "./CreateAgentDialog";
import { ServerDialog } from "./ServerDialog";
import { useDaemons } from "@/components/daemonContext";
import { hasLocalDaemon } from "@/lib/desktop";
import { ServerContextBar } from "./ServerContextBar";
import { LazyBoundary } from "@/components/LazyBoundary";
import { MessagesLiveRefresh } from "./MessagesLiveRefresh";
import { TitlebarSlotContext } from "./titlebarSlotContext";
import { WorkspaceManager, WorkspaceSwitcher, type WorkspaceHost } from "./HostWorkspaces";
import { useWorkspaces, workspaceOf } from "@/lib/workspaces";
import { defaultAgentTab } from "@/lib/uiMode";
import { TerminalsEmptyState, TerminalsRoutes } from "./TerminalsRoutes";
import { TerminalsSidebar, type CreateDialogState } from "./TerminalsSidebar";
import {
  agentPath,
  buildCreateHosts,
  buildWorkspaceHosts,
  hostLabelFor,
  type CreateHost,
} from "./terminalsHosts";
import { useHostsPolling } from "./useHostsPolling";
import { useServerActions, type ServerDialogState } from "./useServerActions";
import { useSidebarReorder } from "./useSidebarReorder";

export type ServerView =
  | "tasks"
  | "activity"
  | "images"
  | "image-tags"
  | "image-detail"
  | "workflows"
  | "workflow-detail"
  | "stores"
  | "store-detail"
  | "settings";

/** `?new=1` or `?image=<ref>` opens the create dialog on arrival. */
function createFromQuery(searchParams: URLSearchParams): CreateDialogState | null {
  const imageRef = searchParams.get("image");
  if (!imageRef && searchParams.get("new") !== "1") return null;
  return {
    hostId: searchParams.get("host") ?? "",
    imageRef: imageRef ?? undefined,
  };
}

export default function TerminalsPage({ serverView }: { serverView?: ServerView }) {
  const { hostId: hostParam, agent: agentName, tab: agentTab } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const allTasks = searchParams.get("view") === "all";
  const hostId = hostParam !== undefined ? paramToHost(hostParam) : undefined;
  const { hosts, refresh, setAgentStatesLive, liveSelectedHost, selectedHost, sidebarHosts } =
    useHostsPolling(hostId);
  const [createFor, setCreateFor] = useState(() => createFromQuery(searchParams));
  const [serverDialog, setServerDialog] = useState<ServerDialogState | null>(null);
  const { daemons } = useDaemons();
  const workspaces = useWorkspaces();
  const titlebarSlot = useContext(TitlebarSlotContext);
  const [workspaceManager, setWorkspaceManager] = useState<WorkspaceManagerState | null>(null);
  const [hostError, setHostError] = useState("");
  const { rankedHosts, orderedSidebarHosts, reorderSidebar } = useSidebarReorder(
    sidebarHosts,
    refresh,
    setHostError,
  );
  const server = useServerActions({ hostId, refresh, setHostError, setServerDialog });
  const createHosts = useMemo(
    () => buildCreateHosts(hosts, daemons, hasLocalDaemon()),
    [daemons, hosts],
  );
  const workspaceHosts = useMemo(
    () => buildWorkspaceHosts(createHosts, hosts, daemons),
    [createHosts, daemons, hosts],
  );

  // A route on a host outside the chosen workspace moves to that workspace's
  // first agent, in the same tab, or to the empty console when it has none.
  const followWorkspace = (id: string) => {
    if (hostId === undefined || workspaceOf(workspaces, hostId) === id) return;
    const first = rankedHosts.find(
      (entry) => workspaceOf(workspaces, entry.host.id) === id && entry.agents.length > 0,
    );
    const query = allTasks ? "?view=all" : "";
    const tab = agentName && agentTab ? agentTab : defaultAgentTab();
    navigate(first ? agentPath(first.host.id, first.agents[0].name, tab, query) : `/${query}`);
  };
  const manageWorkspaces = (focusNew: boolean) =>
    setWorkspaceManager({ id: workspaces.active, focusNew });

  return (
    // Terminal body: the chrome shows through as an 8px gutter around the
    // content island, so neither the sidebar nor the island needs a border.
    <div className="flex h-full px-2 pb-2">
      {/* Chat order and agent state are live: a published message or a state
          change refreshes the authoritative snapshot at once rather than
          waiting for the next poll. */}
      <MessagesLiveRefresh
        hostIds={hosts.map((entry) => entry.host.id)}
        onChange={refresh}
        onLive={setAgentStatesLive}
      />
      {/* All tasks spans every server, so no agent is the subject there. */}
      {!allTasks && (
        <TerminalsSidebar
          hosts={orderedSidebarHosts}
          labelHosts={sidebarHosts}
          server={server}
          onReorder={(kind, id, ids) => void reorderSidebar(kind, id, ids)}
          onCreate={setCreateFor}
          onAddServer={() => setServerDialog({ mode: "add" })}
        />
      )}
      {/* The content island: --card on the --background chrome, one radius and
          one of the two allowed shadows. Everything the operator works with
          lives inside it. */}
      <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-[var(--panel-radius)] bg-card shadow-[var(--lift)]">
        {!allTasks && hostId !== undefined && (
          <ServerContextBar hostId={hostId} label={hostLabelFor(hostId, selectedHost, daemons)} />
        )}
        <div className={`flex min-h-0 flex-1 flex-col${serverView || allTasks ? "" : " p-3"}`}>
          {hostError && (
            <p role="alert" className="mb-2 text-sm text-destructive">
              {hostError}
            </p>
          )}
          <section className="min-h-0 flex-1">
            <LazyBoundary resetKey={location.pathname}>
              <TerminalsRoutes
                serverView={serverView}
                hosts={hosts}
                hostId={hostId}
                selectedHost={selectedHost}
                routeUnavailable={Boolean(liveSelectedHost?.error)}
                refresh={refresh}
                fallback={
                  <TerminalsEmptyState
                    hosts={orderedSidebarHosts}
                    firstHostId={createHosts[0]?.id}
                    onManage={() => manageWorkspaces(false)}
                    onAddServer={() => setServerDialog({ mode: "add" })}
                    onNewAgent={(id) => setCreateFor({ hostId: id })}
                  />
                }
              />
            </LazyBoundary>
          </section>
        </div>
      </main>
      {titlebarSlot &&
        createPortal(
          <WorkspaceSwitcher
            hosts={workspaceHosts}
            onSelect={followWorkspace}
            onManage={manageWorkspaces}
          />,
          titlebarSlot,
        )}
      <TerminalsDialogs
        createFor={createFor}
        setCreateFor={setCreateFor}
        createHosts={createHosts}
        serverDialog={serverDialog}
        setServerDialog={setServerDialog}
        workspaceManager={workspaceManager}
        setWorkspaceManager={setWorkspaceManager}
        workspaceHosts={workspaceHosts}
        onRegistryChanged={server.registryChanged}
        refresh={refresh}
      />
    </div>
  );
}

type WorkspaceManagerState = { id: string; focusNew: boolean };

/** The page's three dialogs: create an agent, manage workspaces, edit a server. */
function TerminalsDialogs({
  createFor,
  setCreateFor,
  createHosts,
  serverDialog,
  setServerDialog,
  workspaceManager,
  setWorkspaceManager,
  workspaceHosts,
  onRegistryChanged,
  refresh,
}: {
  createFor: CreateDialogState | null;
  setCreateFor: (state: CreateDialogState | null) => void;
  createHosts: CreateHost[];
  serverDialog: ServerDialogState | null;
  setServerDialog: (state: ServerDialogState | null) => void;
  workspaceManager: WorkspaceManagerState | null;
  setWorkspaceManager: (state: WorkspaceManagerState | null) => void;
  workspaceHosts: WorkspaceHost[];
  onRegistryChanged: () => Promise<void>;
  refresh: () => Promise<void>;
}) {
  const navigate = useNavigate();
  const workspaces = useWorkspaces();
  return (
    <>
      <CreateAgentDialog
        open={createFor !== null}
        hostId={createFor?.hostId ?? ""}
        imageRef={createFor?.imageRef}
        cloneSource={createFor?.cloneSource}
        hosts={createHosts}
        onOpenChange={(o) => {
          if (!o) setCreateFor(null);
        }}
        onCreated={(h, name) => {
          // usePolling exposes `refresh` (its tick fn) — call it so the new
          // agent shows up immediately instead of waiting up to 3s for the
          // next poll tick.
          void refresh();
          navigate(agentPath(h, name, defaultAgentTab()));
        }}
      />
      <WorkspaceManager
        hosts={workspaceHosts}
        open={workspaceManager !== null}
        initialId={workspaceManager?.id ?? workspaces.active}
        focusNew={workspaceManager?.focusNew ?? false}
        onOpenChange={(open) => {
          if (!open) setWorkspaceManager(null);
        }}
      />
      <ServerDialog
        open={serverDialog !== null}
        server={serverDialog?.mode === "edit" ? serverDialog.server : undefined}
        onOpenChange={(o) => {
          if (!o) setServerDialog(null);
        }}
        onSaved={onRegistryChanged}
      />
    </>
  );
}
