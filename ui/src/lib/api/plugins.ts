import type { Transport } from "./client";

interface PluginOperatorArg {
  name: string;
  flag?: string;
  type: "string" | "integer" | "integer-list" | "boolean" | "secret-file";
  required?: boolean;
  help?: string;
}
interface PluginOperatorCommand {
  path: string;
  summary: string;
  action: string;
  args?: PluginOperatorArg[];
}
export interface PluginSettingField {
  name: string;
  label: string;
  type: "string" | "password" | "integer-list";
  required?: boolean;
  help?: string;
}
interface PluginSettingsContribution {
  title: string;
  status?: Array<{ name: string; label: string }>;
  sections?: Array<{
    title: string;
    fields?: PluginSettingField[];
    actions?: Array<{ label: string; action: string; fields?: string[] }>;
  }>;
}
export interface PluginContribution {
  name: string;
  description?: string;
  operator_commands?: PluginOperatorCommand[];
  settings?: PluginSettingsContribution;
}

export function pluginsApi(t: Transport) {
  return {
    getPluginContributions: () =>
      t.get<{ plugins: PluginContribution[]; count: number }>("/api/plugin-contributions"),
    getPluginStatus: async (name: string): Promise<Record<string, unknown>> => {
      const result = await t.get<Record<string, unknown>>(
        `/api/plugins/${encodeURIComponent(name)}/routes`,
      );
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
