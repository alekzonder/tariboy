import type { ComponentProps } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProxyCallRow } from "./ProxyCallRow";

// Call / Block / Message shapes come from src/lib/transcript.ts.
type Call = ComponentProps<typeof ProxyCallRow>["call"];

const usage = { input: 18422, output: 1163, cache_read: 96000, cache_write: 4200 };

const call: Call = {
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
      blocks: [{ type: "text", text: "Goal TB-142 — ship the desktop updater contract." }],
    },
  ],
  response: {
    blocks: [
      {
        type: "thinking",
        text: "The shim expects every updater command to resolve after acknowledgement.",
      },
      { type: "text", text: "Running the desktop updater contract tests before editing." },
      {
        type: "tool_use",
        tool_name: "bash",
        tool_use_id: "call_1",
        input: { command: "cargo test -p tariboy-desktop updater" },
      },
      {
        type: "tool_result",
        tool_use_id: "call_1",
        text: "running 9 tests … ok. 9 passed; 0 failed",
      },
    ],
    stop_reason: "end_turn",
  },
};

const meta = {
  component: ProxyCallRow,
  parameters: { tariboy: { daemon: true } },
  args: { call, mode: "enrich", name: "builder", iteration: "48" },
} satisfies Meta<typeof ProxyCallRow>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Enriched: Story = {};

export const FullTranscript: Story = { args: { mode: "full" } };

export const InstructionsChanged: Story = {
  args: { call: { ...call, seq: 49, instructions_changed: true } },
};

export const FailedCall: Story = {
  args: {
    call: {
      ...call,
      seq: 50,
      status: "error",
      cost_usd: 0,
      latency_ms: 240,
      delta: [],
      response: {
        blocks: [{ type: "tool_result", text: "", is_error: true }],
        stop_reason: "error",
      },
    },
  },
};
