import type { PluginContributionList, PluginRoutes } from "../api-types.generated";
import type { Transport } from "./client";

export type {
  PluginContribution,
  PluginList,
  PluginLogs,
  PluginSummary,
  SettingField,
} from "../api-types.generated";

export function pluginsApi(t: Transport) {
  return {
    getPluginContributions: () => t.get<PluginContributionList>("/api/plugin-contributions"),
    getPluginStatus: async (name: string): Promise<Record<string, unknown>> => {
      const result =
        (await t.get<PluginRoutes>(`/api/plugins/${encodeURIComponent(name)}/routes`)) ?? {};
      const status = result.status;
      return status && typeof status === "object" && !Array.isArray(status)
        ? (status as Record<string, unknown>)
        : result;
    },
    runPluginAction: (name: string, action: string, data: Record<string, unknown>) =>
      t.post<Record<string, unknown>>(`/api/plugins/${encodeURIComponent(name)}/action`, {
        name,
        action,
        data: JSON.stringify(data),
      }),
  };
}
