import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import type { Task } from "@/lib/api";
import { cn } from "@/lib/utils";
import { FIELD_MONO, LABEL, MONO, SelectShell } from "./panelStyles";
import { taskQueries, writeTask } from "./taskQueries";
import { assignablePool } from "./workflowShared";
import { errorText } from "@/lib/errors";

/**
 * The assignee of a workflow task. In an open, unpaused pool status the
 * customer picks another member of that pool, and the daemon hands the status
 * to it at once; anywhere else the workflow alone decides, so it is text.
 */
export default function WorkflowAssignee({
  task,
  onTaskChanged,
}: {
  task: Task;
  onTaskChanged?: () => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  const view = useApiQuery(taskQueries.workflow(api, task.key)).data;
  const pool = assignablePool(task, view);
  const pools = useApiQuery(taskQueries.pools(api, pool ? task.queue : "")).data;
  const members = pools?.items.find((item) => item.name === pool)?.agents ?? [];
  const reassign = useMutation({
    mutationFn: (agent: string) =>
      writeTask(client, api, () => api.workflows.reassignTaskWorkflow(task.key, agent)),
    onSuccess: () => onTaskChanged?.(),
  });

  if (!pool) {
    return (
      <dl className="flex min-w-0 flex-col gap-1.25">
        <dt className={LABEL}>Assignee</dt>
        <dd className={cn(MONO, "flex h-7.5 min-w-0 items-center truncate")}>
          {task.assignee || "Unassigned"}
        </dd>
      </dl>
    );
  }
  const options = members.map((agent) => `agent:${agent}`);
  return (
    <label className="flex min-w-0 flex-col gap-1.25">
      <span className={LABEL}>Assignee</span>
      <SelectShell
        className={cn("flex", FIELD_MONO)}
        value={task.assignee}
        disabled={reassign.isPending}
        onChange={(event) => {
          reassign.reset();
          reassign.mutate(event.target.value.slice("agent:".length));
        }}
      >
        {!options.includes(task.assignee) && (
          <option value={task.assignee} disabled>
            {task.assignee || "Unassigned"}
          </option>
        )}
        {options.map((value) => (
          <option key={value} value={value}>
            {value}
          </option>
        ))}
      </SelectShell>
      {reassign.error && (
        <span role="alert" className="text-ui text-status-failed">
          {errorText(reassign.error)}
        </span>
      )}
    </label>
  );
}
