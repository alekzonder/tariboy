import type {
  DaemonStatus,
  MaintenanceRun,
  MaintenanceSettings,
  MaintenanceView,
  RetentionPolicy,
  RetentionSetResult,
  RetentionView,
  ShellScriptSaveResult,
} from "../api-types.generated";
import type { Transport } from "./client";
import type { ShellScriptResult } from "./agents";

export type {
  DaemonConfig,
  DaemonStatus,
  MaintenanceRun,
  MaintenanceSettings,
  RetentionPolicy,
  RetentionView,
} from "../api-types.generated";

export function daemonApi(t: Transport) {
  return {
    getDaemonStatus: () => t.get<DaemonStatus>("/api/daemon/status"),
    getGlobalAgentShellScript: () => t.get<ShellScriptResult>("/api/daemon/agent-shell-script"),
    setGlobalAgentShellScript: (script: string) =>
      t.post<ShellScriptSaveResult>("/api/daemon/agent-shell-script", { script }),
    getMaintenance: () => t.get<MaintenanceView>("/api/maintenance"),
    setMaintenance: (settings: MaintenanceSettings) =>
      t.post<MaintenanceSettings>("/api/maintenance", {
        enabled: settings.enabled,
        time: settings.time,
        "keep-backups": settings.keep_backups,
        "retention-days": settings.retention_days,
        "chat-retention-days": settings.chat_retention_days,
        compact: settings.compact,
        "compact-threshold-pct": settings.compact_threshold_pct,
      }),
    runMaintenance: () => t.post<MaintenanceRun>("/api/maintenance/run"),
    getIterationRetentionDefault: () => t.agentGet<RetentionView>("default", "retention"),
    setIterationRetentionDefault: async (p: RetentionPolicy): Promise<RetentionPolicy> => {
      const r = await t.agentPost<RetentionSetResult>("default", "retention", {
        "keep-iterations": p.keep_iterations,
        "keep-days": p.keep_days,
        "max-bytes": p.max_bytes,
      });
      return { keep_iterations: r.keep_iterations, keep_days: r.keep_days, max_bytes: r.max_bytes };
    },
  };
}
export type DaemonApi = ReturnType<typeof daemonApi>;
