import { useEffect, useRef } from "react"
import { useOptionalDaemons } from "@/components/DaemonProvider"
import { useApiTarget } from "@/hooks/useApiQuery"
import type { ApiTarget } from "@/lib/api"
import { hostToParam } from "@/lib/terminalsHost"
import { useTasksWorkspaceView } from "./useTasksWorkspaceView"
import { useTasksWorkspaceLayout } from "./useTasksWorkspaceLayout"
import { useTasksWorkspaceData } from "./useTasksWorkspaceData"
import { useTasksWorkspaceActions } from "./useTasksWorkspaceActions"
import { TasksWorkspaceContent } from "./TasksWorkspaceContent"
import "./tasks.css"

type TasksWorkspaceProps = {
  scopeAgent?: string
  target?: ApiTarget
  initialTaskKey?: string
  onNotificationsChanged?: () => void
}

function TasksWorkspaceController({ scopeAgent, target: inputTarget, initialTaskKey, onNotificationsChanged, server }: TasksWorkspaceProps & { server: string }) {
  const target = useApiTarget(inputTarget)
  const state = useTasksWorkspaceView()
  const layout = useTasksWorkspaceLayout()
  const data = useTasksWorkspaceData(target, scopeAgent, state, onNotificationsChanged)
  const actions = useTasksWorkspaceActions(target, server, state, data)
  const { loadDetail } = state
  const openedInitialKeyRef = useRef("")
  useEffect(() => {
    if (!initialTaskKey || openedInitialKeyRef.current === initialTaskKey) return
    openedInitialKeyRef.current = initialTaskKey
    void loadDetail(initialTaskKey)
  }, [initialTaskKey, loadDetail])

  return <TasksWorkspaceContent scopeAgent={scopeAgent} target={target} server={server}
    state={state} data={data} actions={actions} layout={layout} />
}

export default function TasksWorkspace(props: TasksWorkspaceProps) {
  const activeId = useOptionalDaemons()?.activeId ?? ""
  const server = props.target === undefined
    ? hostToParam(activeId)
    : props.target === null
      ? hostToParam("")
      // Not the endpoint: a reconnect moves it and must keep the open state.
      : props.target.id
  const targetKey = `${props.target === undefined ? "active" : "explicit"}:${server}`
  return <TasksWorkspaceController key={`${targetKey}:${props.scopeAgent ?? ""}`} {...props} server={server} />
}
