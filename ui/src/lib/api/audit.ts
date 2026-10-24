import type { Transport } from "./client";

// One audit record as GET /api/agents/<name>/logs returns it. `data` is a JSON
// string; lib/audit.ts parses and presents it.
export interface AuditEvent {
  seq: number;
  kind: string;
  source: string;
  data: string;
  at: string;
  iteration_id: string;
}

// ---- Iteration transcript (AI proxy calls) ----
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
      t.agentGet<{ events: AuditEvent[]; capped?: boolean }>(
        name,
        `logs${query ? `?${query}` : ""}`,
        signal,
      ),
    listAuditEventTypes: (name: string) =>
      t.agentGet<{ types: string[] }>(name, "logs?distinct=types"),
    fetchAuditMarkdown: async (name: string, iteration?: string): Promise<string> =>
      (await t.raw("GET", exportPath(name, iteration, "markdown"))).text(),
    fetchAuditArchive: async (name: string, iteration?: string): Promise<Blob> =>
      (await t.raw("GET", exportPath(name, iteration))).blob(),
    fetchTranscript: async (
      name: string,
      iteration: string,
      signal?: AbortSignal,
    ): Promise<Call[]> => {
      const r = await t.agentGet<{ calls: Call[] }>(
        name,
        `iterations/${encodeURIComponent(iteration)}/transcript`,
        signal,
      );
      return r.calls ?? [];
    },
    fetchTranscriptRaw: async (name: string, iteration: string): Promise<RawCall[]> => {
      const r = await t.agentGet<{ calls: RawCall[] }>(
        name,
        `iterations/${encodeURIComponent(iteration)}/transcript?raw=1`,
      );
      return r.calls ?? [];
    },
  };
}
