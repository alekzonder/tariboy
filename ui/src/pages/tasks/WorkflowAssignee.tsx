import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useApiQuery, useApiTarget } from "@/hooks/useApiQuery"
import type { ApiTarget } from "@/lib/api"
import { reassignTaskWorkflow, type Task } from "@/lib/tasks"
import { cn } from "@/lib/utils"
import { FIELD_MONO, LABEL, MONO, SelectShell } from "./panelStyles"
import { taskQueries, writeTask } from "./taskQueries"
import { errorText } from "./workflowShared"

/**
 * The assignee of a workflow task. In an open, unpaused pool status the
 * customer picks another member of that pool, and the daemon hands the status
 * to it at once; anywhere else the workflow alone decides, so it is text.
 */
export default function WorkflowAssignee({ task, target: inputTarget, onTaskChanged }: {
  task: Task
  target?: ApiTarget
  onTaskChanged?: () => void
}) {
  const target = useApiTarget(inputTarget)
  const client = useQueryClient()
  const view = useApiQuery(taskQueries.workflow(target, task.key), target).data
  const editable = task.access !== "context" && task.access !== "respond"
  const pool = editable && view && !view.paused_reason && view.waiting_on !== "pause" && view.owner.startsWith("pool:")
    && task.category !== "done" && task.category !== "cancelled" ? view.owner.slice("pool:".length) : ""
  const pools = useApiQuery(taskQueries.pools(target, pool ? task.queue : ""), target).data
  const members = pools?.items.find((item) => item.name === pool)?.agents ?? []
  const reassign = useMutation({
    mutationFn: (agent: string) => writeTask(client, target, () => reassignTaskWorkflow(task.key, agent, target)),
    onSuccess: () => onTaskChanged?.(),
  })

  if (!pool) {
    return <dl className="flex min-w-0 flex-col gap-[5px]">
      <dt className={LABEL}>Assignee</dt>
      <dd className={cn(MONO, "flex h-[30px] min-w-0 items-center truncate")}>{task.assignee || "Unassigned"}</dd>
    </dl>
  }
  const options = members.map((agent) => `agent:${agent}`)
  return <label className="flex min-w-0 flex-col gap-[5px]">
    <span className={LABEL}>Assignee</span>
    <SelectShell className={cn("flex", FIELD_MONO)} value={task.assignee} disabled={reassign.isPending}
      onChange={(event) => { reassign.reset(); reassign.mutate(event.target.value.slice("agent:".length)) }}>
      {!options.includes(task.assignee) && <option value={task.assignee} disabled>{task.assignee || "Unassigned"}</option>}
      {options.map((value) => <option key={value} value={value}>{value}</option>)}
    </SelectShell>
    {reassign.error && <span role="alert" className="text-[12px] text-status-failed">{errorText(reassign.error)}</span>}
  </label>
}
