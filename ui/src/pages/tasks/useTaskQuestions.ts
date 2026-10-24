import { useEffect, useMemo } from "react";
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import type { Task, TaskNotification } from "@/lib/api";
import { actorAgent, missingQuestionTaskKeys, unreadQuestionsFor } from "@/lib/taskList";
import { taskKeys, taskQueries } from "./taskQueries";

/**
 * An unread question stays until its task is opened, but the Active list
 * leaves out closed tasks, so their questions would count in the badge with
 * no row to open. Such tasks are read one by one and kept in the Active list
 * until their question is read; `enabled` is false for every other view.
 */
export function useMissingQuestionTasks({
  enabled,
  notifications,
  tasks,
  scopeAgent,
}: {
  enabled: boolean;
  notifications: TaskNotification[];
  tasks: Task[];
  scopeAgent: string | undefined;
}): Task[] {
  const api = useApi();
  const keys = useMemo(
    () =>
      enabled
        ? missingQuestionTaskKeys({
            notifications,
            tasks,
            requester: scopeAgent ? actorAgent(scopeAgent) : undefined,
          })
        : [],
    [enabled, notifications, scopeAgent, tasks],
  );
  const results = useQueries({
    queries: keys.map((key) => ({ ...taskQueries.detail(api, key), enabled: api.ready })),
  });
  return results.flatMap((result) => (result.data ? [result.data.task] : []));
}

/** Opening a loaded task reads its unread questions. */
export function useMarkTaskQuestionsRead({
  selectedKey,
  detail,
  notifications,
  onNotificationsChanged,
}: {
  selectedKey: string;
  /** The open task's loaded detail; each new load checks again. */
  detail: object | null;
  notifications: TaskNotification[];
  onNotificationsChanged?: () => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  const markRead = useMutation({
    mutationFn: async (unread: TaskNotification[]) => {
      await client.cancelQueries({ queryKey: taskKeys.notifications(api.target) });
      const read = new Set<string>();
      for (const notification of unread) {
        try {
          await api.tasks.markTaskNotificationRead(notification.id);
          read.add(notification.id);
        } catch {
          /* Leave the indicator for the next open. */
        }
      }
      if (!read.size) return;
      await client.cancelQueries({ queryKey: taskKeys.notifications(api.target) });
      client.setQueryData(
        taskKeys.notifications(api.target),
        (page: { notifications: TaskNotification[]; count: number } | undefined) =>
          page
            ? {
                ...page,
                notifications: page.notifications.map((item) =>
                  read.has(item.id) ? { ...item, read_at: new Date().toISOString() } : item,
                ),
              }
            : page,
      );
      // The next socket/manual read reconciles the inbox; no extra cycle is started here.
      await client.invalidateQueries({
        queryKey: taskKeys.notifications(api.target),
        refetchType: "none",
      });
      onNotificationsChanged?.();
    },
  });
  const markNotifications = markRead.mutate;
  useEffect(() => {
    if (!selectedKey || !detail) return;
    const unread = unreadQuestionsFor(notifications, selectedKey);
    if (unread.length) markNotifications(unread);
  }, [selectedKey, detail, notifications, markNotifications]);
}
