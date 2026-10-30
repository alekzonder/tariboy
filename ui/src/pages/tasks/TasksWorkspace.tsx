import { useEffect, useRef, type ReactNode } from "react";
import { useApi } from "@/components/apiContext";
import { hostToParam } from "@/lib/terminalsHost";
import { useTasksWorkspaceView } from "./useTasksWorkspaceView";
import { useTasksWorkspaceLayout } from "./useTasksWorkspaceLayout";
import { useTasksWorkspaceData } from "./useTasksWorkspaceData";
import { useTasksWorkspaceActions } from "./useTasksWorkspaceActions";
import { TasksWorkspaceContent } from "./TasksWorkspaceContent";
import "./tasks.css";

type TasksWorkspaceProps = {
  scopeAgent?: string;
  /** The scope agent's current Goal; its row is highlighted when listed. */
  goalTaskKey?: string;
  initialTaskKey?: string;
  onNotificationsChanged?: () => void;
  /** Shown at the start of the toolbar row, e.g. a host selector when the
   *  workspace has several hosts. */
  hostControl?: ReactNode;
};

function TasksWorkspaceController({
  scopeAgent,
  goalTaskKey,
  hostControl,
  initialTaskKey,
  onNotificationsChanged,
  server,
}: TasksWorkspaceProps & { server: string }) {
  const state = useTasksWorkspaceView();
  const layout = useTasksWorkspaceLayout();
  const data = useTasksWorkspaceData(scopeAgent, state, onNotificationsChanged);
  const actions = useTasksWorkspaceActions(server, state, data, scopeAgent);
  const { loadDetail } = state;
  const openedInitialKeyRef = useRef("");
  useEffect(() => {
    if (!initialTaskKey || openedInitialKeyRef.current === initialTaskKey) return;
    openedInitialKeyRef.current = initialTaskKey;
    void loadDetail(initialTaskKey);
  }, [initialTaskKey, loadDetail]);

  return (
    <TasksWorkspaceContent
      scopeAgent={scopeAgent}
      goalTaskKey={goalTaskKey}
      hostControl={hostControl}
      server={server}
      state={state}
      data={data}
      actions={actions}
      layout={layout}
    />
  );
}

/** The tasks of the server of the nearest ApiProvider. */
export default function TasksWorkspace(props: TasksWorkspaceProps) {
  // Not the endpoint: a reconnect moves it and must keep the open state.
  const server = hostToParam(useApi().target?.id ?? "");
  return (
    <TasksWorkspaceController
      key={`${server}:${props.scopeAgent ?? ""}`}
      {...props}
      server={server}
    />
  );
}
