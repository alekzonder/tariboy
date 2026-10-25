import { lazy, useState, type ReactNode } from "react";
import { Navigate } from "react-router-dom";
import { AgentNameContext, AgentStatusContext } from "@/lib/agent";
import { ApiProvider } from "@/components/ApiProvider";
import { useActiveLinkIntoView } from "@/hooks/useActiveLinkIntoView";
import { targetFor } from "@/lib/terminalsHost";
import type { AgentRow } from "@/lib/api";
import { LazyBoundary } from "@/components/LazyBoundary";
import { useCustomerQuestionNotifications } from "@/components/customerQuestionNotificationsContext";
import { customerQuestionAttentionKey } from "@/components/customerQuestionNotificationModel";
import { useChatUnread } from "./chat/useChatUnread";
import type { AgentTab } from "./agentWorkspaceModel";
import { AgentWorkspaceHeader } from "./AgentWorkspaceHeader";
import { useAgentWorkspaceRoute } from "./useAgentWorkspaceRoute";
import { useAgentWorkspaceStatus } from "./useAgentWorkspaceStatus";

// Each tab loads on first open: the terminal brings xterm, the chat Markdown
// and highlighting, Tasks the rich text editor.
const AgentConsoleTab = lazy(() => import("./AgentConsoleTab"));
const AgentActivityTab = lazy(() => import("./AgentActivityTab"));
const AgentConfigurationTab = lazy(() => import("./AgentConfigurationTab"));
const AgentAdvancedTab = lazy(() => import("./AgentAdvancedTab"));
const AgentChatTab = lazy(() => import("./AgentChatTab"));
const TasksWorkspace = lazy(() => import("@/pages/tasks/TasksWorkspace"));
const TaskDrawer = lazy(() => import("@/pages/tasks/TaskDrawer"));

type AgentWorkspaceProps = {
  hostId: string;
  hostLabel?: string;
  agent: AgentRow;
  refresh: () => void;
  unavailable?: boolean;
};

type TabContext = {
  hostId: string;
  agent: AgentRow;
  refresh: () => void;
  execCount: number;
  initialTaskKey?: string;
  onNotificationsChanged: () => void;
};

const TAB_CONTENT: Record<AgentTab, (ctx: TabContext) => ReactNode> = {
  console: ({ hostId, agent, refresh, execCount }) => (
    <AgentConsoleTab hostId={hostId} agent={agent} refresh={refresh} execCount={execCount} />
  ),
  activity: () => <AgentActivityTab />,
  tasks: ({ agent, initialTaskKey, onNotificationsChanged }) => (
    <TasksWorkspace
      scopeAgent={agent.name}
      goalTaskKey={agent.current_goal_task_key}
      initialTaskKey={initialTaskKey}
      onNotificationsChanged={onNotificationsChanged}
    />
  ),
  chat: ({ hostId }) => <AgentChatTab hostId={hostId} />,
  configuration: ({ refresh }) => <AgentConfigurationTab refresh={refresh} />,
  advanced: () => <AgentAdvancedTab />,
};

// The workspace is a server-selection boundary: the agent lives on the host the
// route names, which need not be the active one, so everything inside it
// (tabs, chat, configuration, tasks, controls) addresses that host. A new
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
  hostLabel,
  agent,
  refresh,
  unavailable = false,
}: AgentWorkspaceProps) {
  const route = useAgentWorkspaceRoute({ hostId, agentName: agent.name });
  const { activeId, daemons, connection, status, currentView, refreshStatus } =
    useAgentWorkspaceStatus({ hostId, agentName: agent.name, unavailable });
  const [execCount, setExecCount] = useState(0);
  // The Goal task opens over whichever tab is showing, not in Tasks.
  const [goalTask, setGoalTask] = useState("");
  const { attention, refreshHost } = useCustomerQuestionNotifications();
  // The Chat tab's dot. It is the daemon's own unread count rather than tab
  // state, so it is right while another tab is open and survives a remount.
  const unreadChat = useChatUnread(agent.name, connection === "ready" && !unavailable);
  const tabStrip = useActiveLinkIntoView<HTMLElement>();

  if ("redirect" in route) return <Navigate to={route.redirect} replace />;
  if (connection !== "ready") return <ConnectionNotice connection={connection} />;
  if (activeId !== hostId) return <ConnectionNotice connection="restoring" />;

  const tab = route.tab;
  const content = TAB_CONTENT[tab]({
    hostId,
    agent,
    refresh,
    execCount,
    initialTaskKey: route.initialTaskKey,
    onNotificationsChanged: () => void refreshHost(hostId),
  });

  return (
    <AgentNameContext.Provider value={agent.name}>
      <AgentStatusContext.Provider value={{ status, refresh: refreshStatus }}>
        <div className="flex h-full min-h-0 flex-col">
          <AgentWorkspaceHeader
            hostId={hostId}
            hostLabel={hostLabel}
            agent={agent}
            status={status}
            view={currentView}
            daemons={daemons}
            unavailable={unavailable}
            base={route.base}
            tabs={route.tabs}
            stripRef={tabStrip}
            unreadChat={unreadChat}
            questionTasks={attention.get(customerQuestionAttentionKey(hostId, agent.name)) ?? 0}
            refresh={refresh}
            onExec={() => setExecCount((n) => n + 1)}
            onOpenGoal={setGoalTask}
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
          {goalTask && (
            <LazyBoundary resetKey={goalTask}>
              <TaskDrawer key={goalTask} taskKey={goalTask} onClose={() => setGoalTask("")} />
            </LazyBoundary>
          )}
        </div>
      </AgentStatusContext.Provider>
    </AgentNameContext.Provider>
  );
}
