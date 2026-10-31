import { lazy, useCallback, useState, type ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import { ApiProvider } from "@/components/ApiProvider";
import { useActiveLinkIntoView } from "@/hooks/useActiveLinkIntoView";
import { targetFor } from "@/lib/terminalsHost";
import type { AgentRow, AgentView } from "@/lib/api";
import { LazyBoundary } from "@/components/LazyBoundary";
import { FirstVisitHint } from "@/components/onboarding/FirstVisitHint";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";
import { agentNeeds, deriveAgentState, type AgentNeeds, type AgentState } from "@/lib/agentState";
import { useNeedsYou } from "@/pages/terminals/needsYouContext";
import type { AgentTab } from "./agentWorkspaceModel";
import { AgentWorkspaceHeader } from "./AgentWorkspaceHeader";
import { useAgentWorkspaceRoute } from "./useAgentWorkspaceRoute";
import { useAgentWorkspaceStatus } from "./useAgentWorkspaceStatus";

// Each tab loads on first open: the terminal brings xterm, Work the rich text
// editor.
const AgentMailboxTab = lazy(() => import("./AgentMailboxTab"));
const AgentConsoleTab = lazy(() => import("./AgentConsoleTab"));
const AgentActivityTab = lazy(() => import("./AgentActivityTab"));
const AgentConfigurationTab = lazy(() => import("./AgentConfigurationTab"));
const AgentAdvancedTab = lazy(() => import("./AgentAdvancedTab"));
const AgentWorkTab = lazy(() => import("./work/AgentWorkTab"));

type AgentWorkspaceProps = {
  hostId: string;
  agent: AgentRow;
  refresh: () => void;
  unavailable?: boolean;
  /** Opens the create dialog with this agent as the clone source. */
  onClone?: () => void;
};

type TabContext = {
  hostId: string;
  agent: AgentRow;
  agentState: AgentState;
  base: string;
  view: AgentView | null;
  refresh: () => void;
  execCount: number;
  terminalKey: string;
  knownAbsent: boolean;
  onAbsent: (absent: boolean) => void;
  needs: AgentNeeds;
  onNotificationsChanged: () => void;
};

const TAB_CONTENT: Record<AgentTab, (ctx: TabContext) => ReactNode> = {
  mailbox: ({ agent, view, base }) => (
    <AgentMailboxTab name={agent.name} batch={view?.messages_batch} base={base} />
  ),
  console: ({ hostId, agent, refresh, execCount, terminalKey, knownAbsent, onAbsent }) => (
    <AgentConsoleTab
      key={terminalKey}
      hostId={hostId}
      agent={agent}
      refresh={refresh}
      execCount={execCount}
      knownAbsent={knownAbsent}
      onAbsent={onAbsent}
    />
  ),
  activity: () => <AgentActivityTab />,
  work: ({ hostId, agent, base, needs, refresh, onNotificationsChanged }) => (
    <>
      <FirstVisitHint hostId={hostId} agent={agent.name} tab="work" className="mx-3 mt-3" />
      <AgentWorkTab
        agent={agent}
        base={base}
        needs={needs}
        refresh={refresh}
        onNotificationsChanged={onNotificationsChanged}
      />
    </>
  ),
  settings: ({ refresh, base }) => <AgentConfigurationTab base={base} refresh={refresh} />,
  advanced: ({ base }) => <AgentAdvancedTab base={base} />,
};

// The workspace is a server-selection boundary: the agent lives on the host the
// route names, which need not be the active one, so everything inside it
// (tabs, configuration, tasks, controls) addresses that host. A new
// endpoint or token for the same host swaps the client without remounting.
export default function AgentWorkspace(props: AgentWorkspaceProps) {
  return (
    <ApiProvider target={targetFor(props.hostId)}>
      <AgentWorkspaceContent {...props} />
    </ApiProvider>
  );
}

function ConnectionNotice({
  connection,
}: {
  connection: "selecting" | "unavailable" | "restoring";
}) {
  if (connection === "unavailable") {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
        <p>Host unavailable.</p>
        <p>The route was not reassigned to the local daemon.</p>
      </div>
    );
  }
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      {connection === "selecting" ? "Connecting to host…" : "Restoring route host…"}
    </div>
  );
}

function AgentWorkspaceContent({
  hostId,
  agent,
  refresh,
  unavailable = false,
  onClone,
}: AgentWorkspaceProps) {
  const route = useAgentWorkspaceRoute({ hostId, agentName: agent.name });
  const { activeId, daemons, connection, status, currentView, refreshStatus } =
    useAgentWorkspaceStatus({ hostId, agentName: agent.name, unavailable });
  const [execCount, setExecCount] = useState(0);
  const [absentSession, setAbsentSession] = useState<string | null>(null);
  const identity = JSON.stringify([hostId, agent.name]);
  const [lastIteration, setLastIteration] = useState("");
  const observedIteration =
    status?.name === agent.name
      ? status.active_iteration?.id || status.last_iteration_id || ""
      : "";
  const iteration = observedIteration || lastIteration;
  if (observedIteration && observedIteration !== lastIteration) setLastIteration(observedIteration);
  const terminalKey = JSON.stringify([identity, iteration, execCount]);
  const onAbsent = useCallback(
    (absent: boolean) => setAbsentSession(absent ? terminalKey : null),
    [terminalKey],
  );
  const { refreshHost } = useCustomerQuestionNotifications();
  const needsYou = useNeedsYou();
  const needs = agentNeeds(
    needsYou.hosts.find((host) => host.hostId === hostId),
    agent.name,
  );
  const agentState = deriveAgentState({ agent, status, needs });
  const tabStrip = useActiveLinkIntoView<HTMLElement>();

  if ("redirect" in route) return <Navigate to={route.redirect} replace />;
  if (connection !== "ready") return <ConnectionNotice connection={connection} />;
  if (activeId !== hostId) return <ConnectionNotice connection="restoring" />;

  const tab = route.tab;
  const content = TAB_CONTENT[tab]({
    hostId,
    agent,
    agentState,
    base: route.base,
    view: currentView,
    refresh,
    execCount,
    terminalKey,
    knownAbsent: absentSession === terminalKey,
    onAbsent,
    needs,
    onNotificationsChanged: () => {
      void refreshHost(hostId);
      needsYou.reload();
    },
  });

  return (
    <AgentNameContext.Provider value={agent.name}>
      <AgentStatusContext.Provider value={{ status, refresh: refreshStatus }}>
        <div className="flex h-full min-h-0 flex-col">
          <AgentWorkspaceHeader
            hostId={hostId}
            agent={agent}
            status={status}
            view={currentView}
            agentState={agentState}
            needsCount={needs.count}
            daemons={daemons}
            unavailable={unavailable}
            base={route.base}
            stripRef={tabStrip}
            refresh={() => {
              refresh();
              needsYou.reload();
            }}
            onExec={() => setExecCount((n) => n + 1)}
            onClone={onClone}
          />
          {unavailable && (
            <p role="status" className="mb-2 text-sm text-muted-foreground">
              This host is temporarily unavailable; actions are disabled until it reconnects.
            </p>
          )}
          <section
            data-testid="agent-workspace-content"
            inert={unavailable}
            aria-disabled={unavailable || undefined}
            className="min-h-0 flex-1 overflow-auto"
          >
            <LazyBoundary resetKey={tab}>{content}</LazyBoundary>
          </section>
        </div>
      </AgentStatusContext.Provider>
    </AgentNameContext.Provider>
  );
}
