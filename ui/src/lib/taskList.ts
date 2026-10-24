import type { Task, TaskFilters, TaskNotification, TaskStatusView } from "./api";
import { errorText } from "./errors";

export function actorAgent(name: string): string {
  return name.startsWith("agent:") ? name : `agent:${name}`;
}

/** Without a scoped agent, "mine" and "waiting" filter by the customer, so the
 *  list waits until the principals load names them. */
export function principalScope({
  scopeAgent,
  view,
  customer,
  error,
}: {
  scopeAgent: string | undefined;
  view: "" | "mine" | "waiting";
  customer: string | undefined;
  error: unknown;
}) {
  const byCustomer = !scopeAgent && view !== "";
  const principalFilter = byCustomer ? customer : undefined;
  return {
    principalFilter,
    principalMetadataError: byCustomer && error ? errorText(error) : "",
    waitingForPrincipal: byCustomer && !principalFilter,
  };
}

/** The list request for a view: "mine" and "waiting" name the scoped agent, or
 *  the customer when no agent scopes the page. */
export function taskListFilters({
  view,
  scopeAgent,
  principal,
  query,
  statusView,
}: {
  view: "" | "mine" | "waiting";
  scopeAgent: string | undefined;
  principal?: string;
  query: string;
  statusView: TaskStatusView;
}): TaskFilters {
  const who = scopeAgent ? actorAgent(scopeAgent) : principal;
  return {
    text: query,
    scope_agent: scopeAgent,
    status_view: statusView,
    ...(view === "mine" ? { assignee: who } : {}),
    ...(view === "waiting" ? { waiting_for: who } : {}),
  };
}

const isUnreadQuestion = (notification: TaskNotification) =>
  notification.type === "task.question" && !notification.read_at && !notification.dismissed_at;

/** The tasks with a question nobody has read yet. */
export function unreadQuestionTaskKeys(notifications: readonly TaskNotification[]): Set<string> {
  return new Set(notifications.filter(isUnreadQuestion).map((item) => item.task_key));
}

/** The unread questions on one task, which opening it reads. */
export function unreadQuestionsFor(
  notifications: readonly TaskNotification[],
  taskKey: string,
): TaskNotification[] {
  return notifications.filter((item) => item.task_key === taskKey && isUnreadQuestion(item));
}

/** Tasks with an unread question that the list did not return, sorted; with a
 *  `requester`, only the questions that principal asked. */
export function missingQuestionTaskKeys({
  notifications,
  tasks,
  requester,
}: {
  notifications: readonly TaskNotification[];
  tasks: readonly Task[];
  requester: string | undefined;
}): string[] {
  const listed = new Set(tasks.map((task) => task.key));
  const keys = notifications
    .filter(
      (item) => isUnreadQuestion(item) && (!requester || item.requesting_principal === requester),
    )
    .map((item) => item.task_key);
  return [...new Set(keys)].filter((key) => !listed.has(key)).sort();
}

/** The list plus the question tasks and the open task it left out, once each. */
export function tasksWithQuestionsAndOpen({
  tasks,
  questionTasks,
  openTask,
}: {
  tasks: Task[];
  questionTasks: Task[];
  openTask: Task | undefined;
}): Task[] {
  const listed = new Set(tasks.map((task) => task.key));
  const result = [...tasks, ...questionTasks.filter((task) => !listed.has(task.key))];
  if (openTask && !result.some((task) => task.key === openTask.key)) result.push(openTask);
  return result;
}
