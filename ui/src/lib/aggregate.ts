import {
  ApiError,
  createApi,
  type AgentListResult,
  type AgentRow,
  type DaemonConfig,
  type GroupList,
  type GroupRow,
} from "@/lib/api";
import { listDaemons, resolveDaemon } from "@/lib/daemons";
import { hasLocalDaemon } from "@/lib/desktop";

export interface HostAgents {
  host: { id: string; label: string };
  agents: AgentRow[];
  groups?: GroupRow[];
  sidebarOrder?: { version: 1; groups: string[]; agents: string[] };
  error?: string;
}

// fetchAllAgents fans out GET /api/agents across the local daemon (same-origin)
// plus every registered daemon, each with its OWN token, and merges. A failed
// host degrades to an error row (agents: []) without breaking the others.
export async function fetchAllAgents(): Promise<HostAgents[]> {
  const registered = await listDaemons();
  const targets = [
    ...(hasLocalDaemon() ? [{ id: "", label: "This daemon (local)" }] : []),
    ...registered.map((m) => ({ id: m.id, label: m.label })),
  ];
  return Promise.all(
    targets.map(async (t) => {
      try {
        const api = createApi(await resolveDaemon(t.id));
        const [res, groupResult, config] = await Promise.all([
          api.request<AgentListResult>("GET", "/api/agents"),
          api.request<GroupList>("GET", "/api/groups"),
          api
            .request<DaemonConfig>("GET", "/api/daemon/config?key=sidebar_order_v1")
            .catch((): DaemonConfig => ({})),
        ]);
        let sidebarOrder: HostAgents["sidebarOrder"];
        try {
          const parsed = JSON.parse(
            config.sidebar_order_v1 ?? "null",
          ) as HostAgents["sidebarOrder"];
          if (
            parsed?.version === 1 &&
            Array.isArray(parsed.groups) &&
            parsed.groups.every((name) => typeof name === "string") &&
            Array.isArray(parsed.agents) &&
            parsed.agents.every((name) => typeof name === "string")
          ) {
            sidebarOrder = parsed;
          }
        } catch {
          // Invalid optional UI state must not hide an otherwise healthy host.
        }
        return {
          host: t,
          agents: res.agents ?? [],
          groups: groupResult.groups ?? [],
          sidebarOrder,
        };
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
