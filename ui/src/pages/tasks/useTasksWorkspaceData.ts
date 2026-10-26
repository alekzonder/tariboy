import { useEffect, useMemo } from "react";
import { toast } from "sonner";
import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { isConnectionError, type Task } from "@/lib/api";
import { errorText } from "@/lib/errors";
import { countTasksByQueue } from "@/lib/tasks";
import {
  principalScope,
  taskListFilters,
  tasksWithQuestionsAndOpen,
  unreadQuestionTaskKeys,
} from "@/lib/taskList";
import { taskKeys, taskQueries } from "./taskQueries";
import { useMarkTaskQuestionsRead, useMissingQuestionTasks } from "./useTaskQuestions";
import { useTaskQueryEvents } from "./useTaskQueryEvents";
import type { TasksWorkspaceView } from "./useTasksWorkspaceView";

/** Toasts the first load error that is not a connection error. */
function useLoadErrorToast(
  tree: Error | null,
  people: Error | null,
  detail: Error | null,
  history: Error | null,
) {
  useEffect(() => {
    const error = tree ?? people ?? detail ?? history;
    if (error && !isConnectionError(error)) toast.error(errorText(error));
  }, [tree, people, detail, history]);
}

/** Queues, principals and the inbox: what the page reads beside the list. */
function useTaskMetadata() {
  const api = useApi();
  const queueQuery = useApiQuery(taskQueries.queues(api));
  const peopleQuery = useApiQuery(taskQueries.principals(api));
  const inboxQuery = useApiQuery(taskQueries.notifications(api));
  const notifications = useMemo(() => inboxQuery.data ?? [], [inboxQuery.data]);
  return {
    queueQuery,
    peopleQuery,
    inboxQuery,
    queues: queueQuery.data?.queues ?? [],
    principals: peopleQuery.data ?? null,
    notifications,
  };
}

export function useTasksWorkspaceData(
  scopeAgent: string | undefined,
  state: TasksWorkspaceView,
  onNotificationsChanged?: () => void,
) {
  const api = useApi();
  const client = useQueryClient();
  const { view, query, statusView, selectedKey, queue } = state;
  const { queueQuery, peopleQuery, inboxQuery, queues, principals, notifications } =
    useTaskMetadata();
  const { principalFilter, principalMetadataError, waitingForPrincipal } = principalScope({
    scopeAgent,
    view,
    customer: principals?.customer,
    error: peopleQuery.error,
  });
  const filters = taskListFilters({
    view,
    scopeAgent,
    principal: principalFilter,
    query,
    statusView,
  });
  const treeQuery = useApiQuery({
    ...taskQueries.list(api, filters),
    enabled: !waitingForPrincipal,
    placeholderData: keepPreviousData,
  });
  const detailQuery = useApiQuery(taskQueries.detail(api, selectedKey));
  const historyQuery = useApiQuery(taskQueries.history(api, selectedKey, "latest"));
  const tasks = useMemo(() => treeQuery.data?.tasks ?? [], [treeQuery.data]);
  const detail = detailQuery.data ?? null;
  const events = historyQuery.data ?? [];
  const loadingKey = selectedKey && (!detail || !historyQuery.data) ? selectedKey : "";
  const loading = waitingForPrincipal ? !principalMetadataError : treeQuery.isPending;
  const refreshing = treeQuery.isFetching && !!treeQuery.data;
  const loadTree = async () => {
    if (!api.ready) return;
    await client.cancelQueries({ queryKey: taskKeys.list(api.target, filters) });
    await treeQuery.refetch();
  };
  const loadMetadata = async () => {
    if (!api.ready) return;
    await client.cancelQueries({ queryKey: taskKeys.principals(api.target) });
    await Promise.all([queueQuery.refetch(), peopleQuery.refetch(), inboxQuery.refetch()]);
  };
  const setTasks = (update: Task[] | ((current: Task[]) => Task[])) =>
    client.setQueryData(
      taskKeys.list(api.target, filters),
      (page: { tasks: Task[]; sequence: number } | undefined) =>
        page
          ? { ...page, tasks: typeof update === "function" ? update(page.tasks) : update }
          : page,
    );
  useTaskQueryEvents(treeQuery.data?.sequence);
  useLoadErrorToast(treeQuery.error, peopleQuery.error, detailQuery.error, historyQuery.error);
  useMarkTaskQuestionsRead({ selectedKey, detail, notifications, onNotificationsChanged });
  const activeQuestionTaskKeys = useMemo(
    () => unreadQuestionTaskKeys(notifications),
    [notifications],
  );

  // The Active list with no search or principal filter is the one view that
  // keeps every task with an unread question, and the open task, in its rows.
  const everyQuestion = statusView === "active" && !query && !view;
  const questionTasks = useMissingQuestionTasks({
    enabled: everyQuestion,
    notifications,
    tasks,
    scopeAgent,
  });
  const listedTasks = tasksWithQuestionsAndOpen({
    tasks,
    questionTasks,
    openTask: everyQuestion ? detail?.task : undefined,
  });

  // The queue filter is applied here rather than in `listTasks`, so the queue
  // menu can count the queues from the same load instead of asking again.
  const visibleTasks = queue ? listedTasks.filter((task) => task.queue === queue) : listedTasks;
  const queueCounts = useMemo(() => countTasksByQueue(tasks), [tasks]);

  return {
    queues,
    principals,
    tasks,
    detail,
    events,
    loadingKey,
    loading,
    refreshing,
    principalFilter,
    principalMetadataError,
    treeQuery,
    detailQuery,
    historyQuery,
    loadTree,
    loadMetadata,
    setTasks,
    activeQuestionTaskKeys,
    visibleTasks,
    queueCounts,
  };
}

export type TasksWorkspaceData = ReturnType<typeof useTasksWorkspaceData>;
