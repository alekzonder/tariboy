import { ProxyCallRow } from "tariboy-ui";

// Call / Block / Message shapes come from src/lib/transcript.ts.
const usage = { input: 18422, output: 1163, cache_read: 96000, cache_write: 4200 };

const call = {
  seq: 48,
  ts: "2026-09-11T18:22:41Z",
  provider: "anthropic",
  model: "claude-opus-5",
  usage,
  cost_usd: 0.42,
  latency_ms: 8140,
  status: "ok",
  instructions: "You are the builder agent for the tariboy repository.",
  instructions_changed: false,
  delta: [
    {
      role: "user",
      blocks: [{ type: "text" as const, text: "Goal TB-142 — ship the desktop updater contract." }],
    },
  ],
  response: {
    blocks: [
      { type: "thinking" as const, text: "The shim expects every updater command to resolve after acknowledgement." },
      { type: "text" as const, text: "Running the desktop updater contract tests before editing." },
      {
        type: "tool_use" as const,
        tool_name: "bash",
        tool_use_id: "call_1",
        input: { command: "cargo test -p tariboy-desktop updater" },
      },
      { type: "tool_result" as const, tool_use_id: "call_1", text: "running 9 tests … ok. 9 passed; 0 failed" },
    ],
    stop_reason: "end_turn",
  },
};

export const Enriched = () => (
  <ProxyCallRow call={call} mode="enrich" name="builder" iteration="48" />
);

export const FullTranscript = () => (
  <ProxyCallRow call={call} mode="full" name="builder" iteration="48" />
);

export const InstructionsChanged = () => (
  <ProxyCallRow
    call={{ ...call, seq: 49, instructions_changed: true }}
    mode="enrich"
    name="builder"
    iteration="48"
  />
);

export const FailedCall = () => (
  <ProxyCallRow
    call={{
      ...call,
      seq: 50,
      status: "error",
      cost_usd: 0,
      latency_ms: 240,
      delta: [],
      response: {
        blocks: [{ type: "tool_result" as const, text: "", is_error: true }],
        stop_reason: "error",
      },
    }}
    mode="enrich"
    name="builder"
    iteration="48"
  />
);
