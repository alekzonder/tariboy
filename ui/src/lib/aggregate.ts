import { apiOn, ApiError } from "@/lib/api";
import { listDaemons, resolveDaemon } from "@/lib/daemons";
import type { AgentSummary } from "@/lib/types";

export interface HostAgents {
  host: { id: string; label: string };
  agents: AgentSummary[];
  groups?: Array<{ name: string; lead: string; members: number }>;
  error?: string;
}

// fetchAllAgents fans out GET /api/agents across the local daemon (same-origin)
// plus every registered daemon, each with its OWN token, and merges. A failed
// host degrades to an error row (agents: []) without breaking the others.
export async function fetchAllAgents(): Promise<HostAgents[]> {
  const registered = await listDaemons();
  const targets = [
    { id: "", label: "This daemon (local)" },
    ...registered.map((m) => ({ id: m.id, label: m.label })),
  ];
  return Promise.all(
    targets.map(async (t) => {
      try {
        const target = await resolveDaemon(t.id);
        const [res, groupResult] = await Promise.all([
          apiOn<{ agents: AgentSummary[]; count: number }>(target, "GET", "/api/agents"),
          apiOn<{ groups: Array<{ name: string; lead: string; members: number }>; count: number }>(target, "GET", "/api/groups"),
        ]);
        return { host: t, agents: res.agents ?? [], groups: groupResult.groups ?? [] };
      } catch (e) {
        // Surface the error code alongside the message (e.g. "unauthorized:
        // nope") so a degraded host is diagnosable without leaking the token.
        const msg =
          e instanceof ApiError && e.code ? `${e.code}: ${e.message}` : (e as Error).message;
        return { host: t, agents: [], error: msg || "unreachable" };
      }
    }),
  );
}
