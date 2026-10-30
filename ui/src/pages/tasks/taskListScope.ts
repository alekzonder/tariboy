import type { TaskFilters, TaskStatusView } from "@/lib/api";
import { actorAgent, principalScope, taskListFilters } from "@/lib/taskList";

/** Whose tasks the list shows: anyone, the viewer ("mine"), or one agent as
 *  `agent:<name>`. */
export type TaskAssigneeFilter = "" | "mine" | `agent:${string}`;

/**
 * The daemon request for the toolbar's filters. "My tasks" and "Waiting for me"
 * name the scoped agent, or the customer when no agent scopes the page, and
 * combine; a picked agent is named as is and needs no customer.
 */
export function taskListScope({
  scopeAgent,
  assignee,
  waiting,
  customer,
  error,
  query,
  statusView,
}: {
  scopeAgent?: string;
  assignee: TaskAssigneeFilter;
  waiting: boolean;
  customer: string | undefined;
  error: unknown;
  query: string;
  statusView: TaskStatusView;
}) {
  const byViewer = assignee === "mine" || waiting;
  // principalScope only asks whether a viewer filter is on.
  const scope = principalScope({ scopeAgent, view: byViewer ? "mine" : "", customer, error });
  const who = scopeAgent ? actorAgent(scopeAgent) : scope.principalFilter;
  const filters: TaskFilters = {
    ...taskListFilters({ view: "", scopeAgent, query, statusView }),
    ...(assignee === "mine" ? { assignee: who } : assignee ? { assignee } : {}),
    ...(waiting ? { waiting_for: who } : {}),
  };
  return { ...scope, filters, filtering: !!assignee || waiting };
}
