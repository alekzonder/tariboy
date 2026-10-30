import { lazy, Suspense, useContext, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { Navigate, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { paramToHost } from "@/lib/terminalsHost";
import { legacyServerRedirect, useScopeHost } from "@/lib/navRoutes";
import { needsYouCounts, type HostNeeds } from "@/lib/needsYou";
import { ServerDialog } from "./ServerDialog";
import { useDaemons } from "@/components/daemonContext";
import { hasLocalDaemon } from "@/lib/desktop";
import { LazyBoundary } from "@/components/LazyBoundary";
import { MessagesLiveRefresh } from "./MessagesLiveRefresh";
import { NeedsYouContext, type NeedsYouValue } from "./needsYouContext";
import { TitlebarSlotContext } from "./titlebarSlotContext";
import { GlobalSearch } from "./GlobalSearch";
import type { WorkspaceHost } from "./HostWorkspaces";
import { inWorkspace, useWorkspaces } from "@/lib/workspaces";
import { defaultAgentTab } from "@/lib/uiMode";
import { TerminalsEmptyState, TerminalsRoutes } from "./TerminalsRoutes";
import { TerminalsSidebar, type CreateDialogState } from "./TerminalsSidebar";
import { useSharedSidebarState } from "./sidebarStateContext";
import {
  agentPath,
  buildCreateHosts,
  buildWorkspaceHosts,
  type CreateHost,
} from "./terminalsHosts";
import { useHostsPolling } from "./useHostsPolling";
import { useServerActions, type ServerDialogState } from "./useServerActions";
import { useSidebarReorder } from "./useSidebarReorder";

export type ServerView =
  | "mail"
  | "tasks"
  | "activity"
  | "images"
  | "image-detail"
  | "workflows"
  | "workflow-detail"
  | "stores"
  | "store-detail"
  | "settings";

/** `?new=1` or `?image=<ref>` opens the create dialog on arrival. */
// The New agent form loads when it first opens, so it stays out of the entry chunk.
const CreateAgentDialog = lazy(() =>
  import("./CreateAgentDialog").then((m) => ({ default: m.CreateAgentDialog })),
);
// The manager is opened from the footer menu only.
const WorkspaceManager = lazy(() =>
  import("./HostWorkspaces").then((m) => ({ default: m.WorkspaceManager })),
);
// The reads bring the API client: they load after the shell.
const NeedsYouLive = lazy(() => import("./NeedsYouLive"));

/** Shows a created agent at once and opens it on its Terminal in both modes,
 *  so the first run shows the coding tool starting. */
const openCreatedAgent =
  (navigate: (path: string) => void, refresh: () => Promise<void>) =>
  (hostId: string, name: string) => {
    // usePolling exposes `refresh` (its tick fn) — call it so the new agent
    // shows up immediately instead of waiting up to 3s for the next poll tick.
    void refresh();
    navigate(agentPath(hostId, name, "console"));
  };

function createFromQuery(searchParams: URLSearchParams): CreateDialogState | null {
  const imageRef = searchParams.get("image");
  if (!imageRef && searchParams.get("new") !== "1") return null;
  return {
    hostId: searchParams.get("host") ?? "",
    imageRef: imageRef ?? undefined,
  };
}

/** The console: the sidebar and the content island. `scoped` pages (Tasks,
 *  Inbox, Activity, Library) take their host from `?host=` or the active host. An old
 *  URL first moves to where its page lives now. */
export default function TerminalsPage(props: { serverView?: ServerView; scoped?: boolean }) {
  const location = useLocation();
  const target = legacyServerRedirect(location.pathname, location.search);
  if (target) return <Navigate to={target} replace />;
  return <ConsolePage {...props} />;
}

/** "Needs you" of the workspace's hosts, read by the lazy NeedsYouLive. */
function useNeedsYou(hosts: { id: string; label: string; error?: string }[], intervalMs: number) {
  const [needs, setNeeds] = useState<HostNeeds[] | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const value = useMemo<NeedsYouValue>(
    () => ({
      hosts: needs ?? [],
      loaded: needs !== null,
      reload: () => setReloadToken((token) => token + 1),
    }),
    [needs],
  );
  const live = (
    <Suspense fallback={null}>
      <NeedsYouLive
        hosts={hosts}
        intervalMs={intervalMs}
        reloadToken={reloadToken}
        onChange={setNeeds}
      />
    </Suspense>
  );
  return { value, live, counts: needsYouCounts(value.hosts) };
}

function ConsolePage({
  serverView,
  scoped = false,
}: {
  serverView?: ServerView;
  scoped?: boolean;
}) {
  const { hostId: hostParam, agent: agentName, tab: agentTab } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const scopeHost = useScopeHost();
  const hostId = scoped ? scopeHost : hostParam !== undefined ? paramToHost(hostParam) : undefined;
  const polling = useHostsPolling(hostId);
  const { hosts, refresh, liveSelectedHost, selectedHost, sidebarHosts } = polling;
  const [createFor, setCreateFor] = useState(() => createFromQuery(searchParams));
  const [serverDialog, setServerDialog] = useState<ServerDialogState | null>(null);
  const [query, setQuery] = useState("");
  const { daemons, activeId } = useDaemons();
  const workspaces = useWorkspaces();
  const titlebarSlot = useContext(TitlebarSlotContext);
  const sidebar = useSharedSidebarState();
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
  const needsHosts = useMemo(
    () =>
      orderedSidebarHosts.map((entry) => ({
        id: entry.host.id,
        label: entry.host.label,
        error: entry.error,
      })),
    [orderedSidebarHosts],
  );
  const needs = useNeedsYou(needsHosts, polling.agentStatesLive ? 15000 : 3000);
  const pageHostId = hostId ?? activeId;

  // A route on a host outside the chosen workspace moves to that workspace's
  // first agent, in the same tab, or to Home when it has none.
  const followWorkspace = (id: string) => {
    if (hostId === undefined || inWorkspace(workspaces, hostId, id)) return;
    const first = rankedHosts.find(
      (entry) => inWorkspace(workspaces, entry.host.id, id) && entry.agents.length > 0,
    );
    const tab = agentName && agentTab ? agentTab : defaultAgentTab();
    navigate(first ? agentPath(first.host.id, first.agents[0].name, tab) : "/");
  };
  const manageWorkspaces = (focusNew: boolean) =>
    setWorkspaceManager({ id: workspaces.active, focusNew });

  return (
    // Terminal body: the chrome shows through as an 8px gutter around the
    // content island, so neither the sidebar nor the island needs a border.
    <NeedsYouContext.Provider value={needs.value}>
      <div className="flex h-full px-2 pb-2">
        {/* Chat order and agent state are live: a published message or a state
            change refreshes the authoritative snapshot at once rather than
            waiting for the next poll. */}
        <MessagesLiveRefresh
          hostIds={hosts.map((entry) => entry.host.id)}
          onChange={refresh}
          onLive={polling.setAgentStatesLive}
        />
        {needs.live}
        <TerminalsSidebar
          hosts={orderedSidebarHosts}
          labelHosts={sidebarHosts}
          hostId={hostId}
          pageHostId={pageHostId}
          counts={needs.counts}
          query={query}
          onQuery={setQuery}
          server={server}
          onReorder={(kind, id, ids) => void reorderSidebar(kind, id, ids)}
          onCreate={setCreateFor}
          onAddServer={() => setServerDialog({ mode: "add" })}
          onSelectWorkspace={followWorkspace}
          onManageWorkspaces={manageWorkspaces}
        />
        {/* The content island: --card on the --background chrome, one radius and
            one of the two allowed shadows. Everything the operator works with
            lives inside it. */}
        <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-[var(--panel-radius)] bg-card shadow-[var(--lift)]">
          <div className={`flex min-h-0 flex-1 flex-col${serverView ? "" : " p-3"}`}>
            {hostError && (
              <p role="alert" className="mb-2 text-sm text-destructive">
                {hostError}
              </p>
            )}
            <section className="min-h-0 flex-1">
              <LazyBoundary resetKey={location.pathname}>
                <TerminalsRoutes
                  serverView={serverView}
                  workspaceHosts={orderedSidebarHosts}
                  hostId={hostId}
                  selectedHost={selectedHost}
                  routeUnavailable={Boolean(liveSelectedHost?.error)}
                  refresh={refresh}
                  fallback={
                    <TerminalsEmptyState
                      hosts={orderedSidebarHosts}
                      pageHostId={pageHostId}
                      firstHostId={createHosts[0]?.id}
                      createHosts={createHosts}
                      onCreated={openCreatedAgent(navigate, refresh)}
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
          !sidebar.compact &&
          createPortal(
            <GlobalSearch hosts={orderedSidebarHosts} query={query} onQuery={setQuery} />,
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
    </NeedsYouContext.Provider>
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
  return (
    <>
      {createFor !== null && (
        <Suspense fallback={null}>
          <CreateAgentDialog
            open
            hostId={createFor.hostId}
            imageRef={createFor.imageRef}
            cloneSource={createFor.cloneSource}
            hosts={createHosts}
            onOpenChange={(o) => {
              if (!o) setCreateFor(null);
            }}
            onCreated={openCreatedAgent(navigate, refresh)}
          />
        </Suspense>
      )}
      {workspaceManager !== null && (
        <Suspense fallback={null}>
          <WorkspaceManager
            hosts={workspaceHosts}
            open
            initialId={workspaceManager.id}
            focusNew={workspaceManager.focusNew}
            onOpenChange={(open) => {
              if (!open) setWorkspaceManager(null);
            }}
          />
        </Suspense>
      )}
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
