import type { AgentUsage, TaskActivityReport, UsageReport } from "../api-types.generated";
import type { Transport } from "./client";

export type {
  AgentUsage,
  BudgetSpend,
  BudgetStatusList,
  TaskActivityAgent,
  TaskActivityReport,
  TaskActivityRow,
  TaskActivityTotals,
  UsageReport,
  UsageSeriesPoint,
} from "../api-types.generated";

export interface UsageParams {
  group?: string;
}

export function usageApi(t: Transport) {
  return {
    getUsage: (params: UsageParams = {}) => {
      const query = new URLSearchParams();
      if (params.group) query.set("group", params.group);
      const q = query.toString();
      return t.get<UsageReport>(`/api/usage${q ? `?${q}` : ""}`);
    },
    // Per-agent Usage tab: grouped rows + time-bucketed series over one agent's
    // AI usage. Drops empty params so the backend applies its defaults
    // (group_by=iteration, bucket=1h, unbounded window).
    getAgentUsage: (name: string, params: Record<string, string> = {}) => {
      const q = new URLSearchParams();
      for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
      const s = q.toString();
      return t.agentGet<AgentUsage>(name, `usage${s ? `?${s}` : ""}`);
    },
    // Work on tasks in a window; without agent, server-wide.
    getTaskActivity: (
      params: { since: string; until: string; agent?: string },
      signal?: AbortSignal,
    ) => {
      const q = new URLSearchParams({ since: params.since, until: params.until });
      if (params.agent) q.set("agent", params.agent);
      return t.request<TaskActivityReport>("GET", `/api/task-activity?${q}`, undefined, signal);
    },
  };
}
