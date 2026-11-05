import type { AuditLogResult, TranscriptResult } from "../api-types.generated";
import type { Call, RawCall } from "@/types/audit";
import type { Transport } from "./client";

export type * from "@/types/audit";

function exportPath(name: string, iteration?: string, format?: "markdown"): string {
  const query = new URLSearchParams();
  if (iteration) query.set("iteration", iteration);
  if (format) query.set("format", format);
  const suffix = query.toString();
  return `/api/agents/${encodeURIComponent(name)}/audit-export${suffix ? `?${suffix}` : ""}`;
}

export function auditApi(t: Transport) {
  return {
    // `query` is the logs route's own query string (iteration, since, before,
    // limit, kind filters) without the leading "?".
    listAuditEvents: (name: string, query = "", signal?: AbortSignal) =>
      t.agentGet<AuditLogResult>(name, `logs${query ? `?${query}` : ""}`, signal),
    listAuditEventTypes: (name: string) => t.agentGet<AuditLogResult>(name, "logs?distinct=types"),
    fetchAuditMarkdown: async (name: string, iteration?: string): Promise<string> =>
      (await t.raw("GET", exportPath(name, iteration, "markdown"))).text(),
    fetchAuditArchive: async (name: string, iteration?: string): Promise<Blob> =>
      (await t.raw("GET", exportPath(name, iteration))).blob(),
    fetchTranscript: async (
      name: string,
      iteration: string,
      signal?: AbortSignal,
    ): Promise<Call[]> => {
      const r = await t.agentGet<TranscriptResult>(
        name,
        `iterations/${encodeURIComponent(iteration)}/transcript`,
        signal,
      );
      return (r.calls ?? []) as unknown as Call[];
    },
    fetchTranscriptRaw: async (name: string, iteration: string): Promise<RawCall[]> => {
      const r = await t.agentGet<TranscriptResult>(
        name,
        `iterations/${encodeURIComponent(iteration)}/transcript?raw=1`,
      );
      return (r.calls ?? []) as unknown as RawCall[];
    },
  };
}
