import type { DaemonMeta } from "@/lib/daemons";

export function canOpenAgentCwdInVSCode(hostId: string, daemons: DaemonMeta[]): boolean {
  if (!hostId) return true;
  return daemons.find((daemon) => daemon.id === hostId)?.kind === "ssh";
}
