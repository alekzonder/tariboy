export type { AuditLogEvent } from "@/lib/api-types.generated";

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
