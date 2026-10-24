import type { DaemonStatus } from "../types";
import type { Transport } from "./client";
import type { AgentShellScript } from "./agents";

export interface MaintenanceSettings {
  enabled: boolean;
  time: string;
  keep_backups: number;
  retention_days: number;
  chat_retention_days: number;
  compact: boolean;
  compact_threshold_pct: number;
}
export interface MaintenanceRun {
  started_at: string;
  finished_at: string;
  trigger: string;
  backup: string;
  deleted: Record<string, number>;
  size_before: number;
  size_after: number;
  compacted: boolean;
  error?: string;
}
// Server-wide iteration retention: 0 is unlimited; an agent's non-zero field
// overrides it.
export interface IterationRetention {
  keep_iterations: number;
  keep_days: number;
  max_bytes: number;
}

export function daemonApi(t: Transport) {
  return {
    getDaemonStatus: () => t.get<DaemonStatus>("/api/daemon/status"),
    getGlobalAgentShellScript: () => t.get<AgentShellScript>("/api/daemon/agent-shell-script"),
    setGlobalAgentShellScript: (script: string) =>
      t.post<{ saved: true }>("/api/daemon/agent-shell-script", { script }),
    getMaintenance: () =>
      t.get<{ settings: MaintenanceSettings; last_run: MaintenanceRun | null }>("/api/maintenance"),
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
    getIterationRetentionDefault: () => t.agentGet<IterationRetention>("default", "retention"),
    setIterationRetentionDefault: async (p: IterationRetention): Promise<IterationRetention> => {
      const r = await t.agentPost<IterationRetention>("default", "retention", {
        "keep-iterations": p.keep_iterations,
        "keep-days": p.keep_days,
        "max-bytes": p.max_bytes,
      });
      return { keep_iterations: r.keep_iterations, keep_days: r.keep_days, max_bytes: r.max_bytes };
    },
  };
}
export type DaemonApi = ReturnType<typeof daemonApi>;
