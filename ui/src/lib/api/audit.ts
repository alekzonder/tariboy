import type { AuditLogResult, TranscriptResult } from "../api-types.generated";
import type { Transport } from "./client";

export type { AuditLogEvent } from "../api-types.generated";

// ---- Iteration transcript (AI proxy calls) ----
// /api/openapi.json describes a transcript call only as an object; these types
// are the UI's reading of internal/commands transcript projection.
export interface Block {
  type: "text" | "thinking" | "tool_use" | "tool_result";
  text?: string;
  tool_name?: string;
  input?: unknown;
  tool_use_id?: string;
  is_error?: boolean;
}
export interface Message {
  role: string;
  blocks: Block[];
}
interface CallResponse {
  blocks: Block[];
  stop_reason?: string;
}
interface Usage {
  input: number;
  output: number;
  cache_read: number;
  cache_write: number;
}
export interface Call {
  seq: number;
  ts: string;
  provider: string;
  model: string;
  usage?: Usage;
  cost_usd?: number;
  latency_ms?: number;
  status?: string;
  instructions: string;
  instructions_changed: boolean;
  delta: Message[];
  response: CallResponse;
  truncated?: boolean;
  parse_error?: string;
}
export interface RawCall {
  seq: number;
  ts: string;
  request: string;
  response: string;
}

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
