import { agentGet } from "@/lib/api";

export interface Block {
  type: "text" | "thinking" | "tool_use" | "tool_result";
  text?: string;
  tool_name?: string;
  input?: unknown;
  tool_use_id?: string;
  is_error?: boolean;
}
export interface Message { role: string; blocks: Block[] }
export interface Response { blocks: Block[]; stop_reason?: string }
export interface Usage { input: number; output: number; cache_read: number; cache_write: number }
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
  response: Response;
  truncated?: boolean;
  parse_error?: string;
}
export interface RawCall { seq: number; ts: string; request: string; response: string }

export async function fetchTranscript(name: string, iteration: string): Promise<Call[]> {
  const r = await agentGet<{ calls: Call[] }>(name, `iterations/${encodeURIComponent(iteration)}/transcript`);
  return r.calls ?? [];
}

export async function fetchTranscriptRaw(name: string, iteration: string): Promise<RawCall[]> {
  const r = await agentGet<{ calls: RawCall[] }>(name, `iterations/${encodeURIComponent(iteration)}/transcript?raw=1`);
  return r.calls ?? [];
}
