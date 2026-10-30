import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { useTaskActivity } from "@/hooks/useTaskActivity";
import type { ActivityWindow } from "@/lib/activityPeriod";
import { refreshTasks, taskQueries } from "@/pages/tasks/taskQueries";
import { useTaskQueryEvents } from "@/pages/tasks/useTaskQueryEvents";

const DAY_MS = 24 * 60 * 60 * 1000;

/** The window the cost of a row comes from: the last seven days, or every
 *  day with Show all. Fixed at mount so the report's key stays put. */
function costWindow(showAll: boolean, now: number): ActivityWindow {
  const since = showAll ? new Date(0) : new Date(now - 7 * DAY_MS);
  return {
    period: "custom",
    since: since.toISOString(),
    until: new Date(now + DAY_MS).toISOString(),
    customSince: "",
    customUntil: "",
    error: "",
  };
}

/** The agent's assigned tasks, active and closed, kept fresh by the task
 *  event stream, and the cost of each from the task activity report. */
export function useAgentWork(agent: string, showAll: boolean) {
  const api = useApi();
  const client = useQueryClient();
  const assignee = `agent:${agent}`;
  const activeQuery = useApiQuery(taskQueries.list(api, { assignee, status_view: "active" }));
  const closedQuery = useApiQuery(taskQueries.list(api, { assignee, status_view: "closed" }));
  useTaskQueryEvents(activeQuery.data?.sequence);
  const [now] = useState(() => Date.now());
  const { report } = useTaskActivity(costWindow(showAll, now), agent);
  const costs = new Map((report?.tasks ?? []).map((row) => [row.key, row.cost_usd]));
  return {
    now,
    active: activeQuery.data?.tasks ?? [],
    closed: closedQuery.data?.tasks ?? [],
    loading: activeQuery.isPending,
    error: activeQuery.error ?? closedQuery.error,
    costs,
    reload: () => void refreshTasks(client, api),
  };
}
